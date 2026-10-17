// fakecc is an oracle-only deterministic Chat Completions service. It has no
// upstream client, credentials, tool execution, or retry path.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Jghh42/atuin-ai/tests/compatfixture"
	"github.com/Jghh42/atuin-ai/tests/projection"
)

func request(rawURL string) {
	u, e := url.Parse(rawURL)
	if e != nil || u.Scheme != "http" || !((u.Host == "oracle:8080" && (u.Path == "/api/cli/models" || u.Path == "/api/cli/chat")) || (u.Host == "127.0.0.1:8081" && u.Path == "/counts")) {
		log.Fatal("denied oracle-only destination")
	}
	b, e := io.ReadAll(io.LimitReader(os.Stdin, 4<<20))
	if e != nil {
		log.Fatal(e)
	}
	method := "GET"
	if len(b) > 0 {
		method = "POST"
	}
	req, e := http.NewRequest(method, rawURL, strings.NewReader(string(b)))
	if e != nil {
		log.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := client.Do(req)
	result := struct {
		Status  int    `json:"status"`
		Body    string `json:"body"`
		Session string `json:"session"`
	}{Body: "oracle unavailable"}
	if e == nil {
		defer res.Body.Close()
		response, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		if err == nil {
			result.Status = res.StatusCode
			result.Session = res.Header.Get("X-Atuin-AI-Session-Id")
			result.Body = string(response)
		}
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
}
func main() {
	if len(os.Args) == 3 && os.Args[1] == "request" {
		request(os.Args[2])
		return
	}
	if len(os.Args) != 2 {
		log.Fatal("usage: fakecc CASES.json")
	}
	cases, e := compatfixture.Load(os.Args[1])
	if e != nil {
		log.Fatal(e)
	}
	byName := map[string]compatfixture.Case{}
	for _, c := range cases {
		byName[c.Name] = c
	}
	counts := map[string]int{}
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/counts", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(counts)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts["__received"]++
		mu.Unlock()
		if r.Method != "POST" {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
		defer r.Body.Close()
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil || json.Unmarshal(body, &req) != nil {
			http.Error(w, "json", http.StatusBadRequest)
			return
		}
		name := ""
		marker := regexp.MustCompile(`case:([a-z_-]+)`)
		for _, m := range req.Messages {
			if m.Role == "user" {
				b, _ := json.Marshal(m.Content)
				match := marker.FindSubmatch(b)
				if len(match) == 2 {
					name = string(match[1])
				}
			}
		}
		c, ok := byName[name]
		if !ok {
			mu.Lock()
			counts["__unknown"]++
			mu.Unlock()
			http.Error(w, "unknown synthetic case", http.StatusBadRequest)
			return
		}
		if e := projection.Validate("chat-completions", c.Request, body); e != nil {
			mu.Lock()
			counts["__invalid"]++
			mu.Unlock()
			log.Printf("synthetic outbound validation %s: %v", name, e)
			http.Error(w, "semantic projection rejected", http.StatusBadRequest)
			return
		}
		mu.Lock()
		counts["__validated"]++
		counts[name]++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(delta any, finish any, usage any) {
			b, _ := json.Marshal(map[string]any{"id": "synthetic-chat", "object": "chat.completion.chunk", "model": "gpt-6.1-sol", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}, "usage": usage})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if c.Turn.Text != "" {
			emit(map[string]any{"content": c.Turn.Text}, nil, nil)
		}
		for i, call := range c.Turn.Calls {
			emit(map[string]any{"tool_calls": []any{map[string]any{"index": i, "id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": string(call.Input)}}}}, nil, nil)
		}
		finish := "stop"
		if len(c.Turn.Calls) > 0 {
			finish = "tool_calls"
		}
		var usage any
		if !c.Turn.UnknownUsage {
			usage = map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}
		emit(map[string]any{}, finish, usage)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		counts["__unknown"]++
		mu.Unlock()
		http.Error(w, "unknown fake route", http.StatusNotFound)
	})
	s := http.Server{Addr: ":8081", Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10}
	log.Fatal(s.ListenAndServe())
}
