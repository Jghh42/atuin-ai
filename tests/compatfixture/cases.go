// Package compatfixture defines the language-neutral synthetic Phase 8 corpus.
package compatfixture

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Jghh42/atuin-ai/tests/testutil"
)

type Case struct {
	Name       string          `json:"name"`
	Difference string          `json:"difference"`
	Request    json.RawMessage `json:"request"`
	Turn       testutil.Turn   `json:"turn"`
}

func Load(path string) ([]Case, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var cases []Case
	e = json.Unmarshal(b, &cases)
	return cases, e
}

type Event struct {
	Event string         `json:"event"`
	Data  map[string]any `json:"data"`
}

func Events(body string) ([]Event, error) {
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	var events []Event
	var kind string
	var data []string
	flush := func() error {
		if kind == "" && len(data) == 0 {
			return nil
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &v); err != nil {
			return err
		}
		events = append(events, Event{kind, v})
		kind = ""
		data = nil
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if e := flush(); e != nil {
				return nil, e
			}
		} else if strings.HasPrefix(line, "event:") {
			kind = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if e := scanner.Err(); e != nil {
		return nil, e
	}
	if kind != "" || len(data) > 0 {
		return nil, fmt.Errorf("unterminated SSE frame")
	}
	return events, nil
}

var uuidShape = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidateSession must precede comparison: shape, header/done identity and
// explicit continuation identity are behavior, not volatile normalization.
func ValidateSession(events []Event, header, requested string) error {
	if !uuidShape.MatchString(header) {
		return fmt.Errorf("missing/malformed session header")
	}
	if requested != "" && (!uuidShape.MatchString(requested) || !strings.EqualFold(header, requested)) {
		return fmt.Errorf("explicit continuation identity changed")
	}
	terminal := 0
	for i, e := range events {
		if id, exists := e.Data["session_id"]; exists {
			s, ok := id.(string)
			if !ok || !uuidShape.MatchString(s) || !strings.EqualFold(s, header) {
				return fmt.Errorf("event/header session mismatch")
			}
		}
		if e.Event == "done" {
			if _, ok := e.Data["session_id"]; !ok {
				return fmt.Errorf("done session missing")
			}
		}
		if e.Event == "done" || e.Event == "error" {
			terminal++
			if i != len(events)-1 {
				return fmt.Errorf("nonterminal terminal event")
			}
		}
	}
	if terminal != 1 {
		return fmt.Errorf("expected exactly one terminal")
	}
	return nil
}
func RequestedSession(raw []byte) string {
	var v struct {
		Session string `json:"session_id"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return "<invalid-request-session>"
	}
	return v.Session
}

// Normalize permits only VALID UUID/session IDs, usage and adjacent text segmentation.
// Call ValidateSession first: invalid/missing/changing IDs must never pass a gate.
// JSON property ordering is already removed by structured decoding. No events,
// risks, tool arguments, errors or waiting states are discarded.
func Normalize(events []Event) []Event {
	out := make([]Event, 0, len(events))
	for _, e := range events {
		data := make(map[string]any, len(e.Data))
		for k, v := range e.Data {
			data[k] = v
		}
		e.Data = data
		if id, ok := data["session_id"].(string); ok && uuidShape.MatchString(id) {
			data["session_id"] = "<session-uuid>"
		}
		if _, ok := data["usage"]; ok {
			data["usage"] = "<usage>"
		}
		if e.Event == "text" && len(out) > 0 && out[len(out)-1].Event == "text" {
			a, aok := out[len(out)-1].Data["content"].(string)
			b, bok := data["content"].(string)
			// Segmentation is ignorable ONLY for content-only payloads. Never
			// discard metadata from either the preceding or subsequent chunk.
			if aok && bok && len(out[len(out)-1].Data) == 1 && len(data) == 1 {
				out[len(out)-1].Data["content"] = a + b
				continue
			}
		}
		out = append(out, e)
	}
	return out
}
