package application

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
	"github.com/Jghh42/atuin-ai/tests/projection"
)

func TestRemoteDiagnosticCannotDisableBundledModels(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		result     codex.DiagnosticResult
	}{
		{"absent", `{"models":[{"slug":"gpt-6-luna","display_name":"Luna","visibility":"list","priority":0}]}`, 200, codex.ResultProvenModelAbsent},
		{"hidden", `{"models":[{"slug":"gpt-6.1-sol","display_name":"Sol","visibility":"hide","priority":0}]}`, 200, codex.ResultProvenModelHidden},
		{"empty", `{"models":[]}`, 200, codex.ResultEmptyCatalog},
		{"malformed", `{`, 200, codex.ResultInvalidJSON},
		{"401", `{}`, 401, codex.ResultHTTPUnauthorized},
		{"403", `{}`, 403, codex.ResultHTTPForbidden},
		{"429", `{}`, 429, codex.ResultHTTPRateLimited},
		{"500", `{}`, 500, codex.ResultHTTPServerError},
		{"503", `{}`, 503, codex.ResultHTTPServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
			path := privatePath(t)
			seedApplicationDiagnostic(t, path, now)
			calls := 0
			r, err := NewWithOptions(path, config.Default(), Options{Now: func() time.Time { return now }, Transport: diagnosticRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.String() != codex.ModelsURL || req.Method != "GET" {
					t.Error("unexpected endpoint")
				}
				return response(tc.status, tc.body), nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			base, _ := startServer(t, r)
			for range 2 {
				if status, _ := api(t, base, "/api/cli/models", ""); status != 200 {
					t.Fatal(status)
				}
			}
			if calls != 0 {
				t.Fatal("normal listing requested metadata")
			}
			before := r.Manager.Status()
			report := r.Diagnose(context.Background())
			if calls != 1 || report.Result != tc.result || report.HTTPStatus == nil || *report.HTTPStatus != tc.status {
				t.Fatal(report, calls)
			}
			if before != r.Manager.Status() {
				t.Fatal("diagnose changed credentials")
			}
			for range 2 {
				status, body := api(t, base, "/api/cli/models", "")
				var got struct {
					Default string `json:"default"`
					Models  []struct {
						Alias string `json:"alias"`
					} `json:"models"`
				}
				if json.Unmarshal([]byte(body), &got) != nil || status != 200 || got.Default != "gpt-6.1-sol" || len(got.Models) != 2 || got.Models[0].Alias != "gpt-6.1-sol" || got.Models[1].Alias != "gpt-6-luna" {
					t.Fatal("remote result gated bundled catalog")
				}
			}
			if calls != 1 {
				t.Fatal("catalog fetched after diagnostic", calls)
			}
		})
	}
}

func TestBothBundledModelsNativeProjectionAndFailClosed(t *testing.T) {
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-luna"} {
		for _, status := range []int{200, 401, 403} {
			t.Run(fmt.Sprintf("%s/%d", model, status), func(t *testing.T) {
				now := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
				path := privatePath(t)
				seedApplicationDiagnostic(t, path, now)
				raw := fmt.Sprintf(`{"config":{"model":%q,"capabilities":["client_v1_read_file"]},"messages":[{"role":"user","content":"start"},{"role":"assistant","content":[{"type":"text","text":"checking"},{"type":"tool_use","id":"read-1","name":"read_file","input":{"file_path":"fixture.txt"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"read-1","content":"synthetic contents"},{"type":"text","text":"continue"}]}]}`, model)
				generations, metadata := 0, 0
				r, err := NewWithOptions(path, config.Default(), Options{Now: func() time.Time { return now }, Transport: diagnosticRoundTrip(func(req *http.Request) (*http.Response, error) {
					if req.URL.String() == codex.ModelsURL {
						metadata++
						return response(500, `{}`), nil
					}
					if req.URL.String() != codex.ResponsesURL {
						t.Error("unexpected endpoint")
					}
					generations++
					b, _ := io.ReadAll(req.Body)
					if e := projection.Validate("responses", []byte(raw), b); e != nil {
						t.Error(e)
					}
					if req.Header.Get("ChatGPT-Account-Id") != "test-account-private" || req.Header.Get("Authorization") != "Bearer "+syntheticJWT(now) {
						t.Error("account/token projection")
					}
					if status != 200 {
						return response(status, `{"error":"synthetic-private-detail"}`), nil
					}
					return response(200, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"id\":\"msg\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"synthetic answer\"}]}]}}\n\n"), nil
				})})
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
				base, _ := startServer(t, r)
				_, body := api(t, base, "/api/cli/chat", raw)
				if generations != 1 || metadata != 0 {
					t.Fatal("retry, fallback, or automatic metadata", generations, metadata)
				}
				if status == 200 {
					if !strings.Contains(body, "event: done") {
						t.Fatal("missing completion")
					}
				} else {
					if !strings.Contains(body, "error") || strings.Contains(body, "event: done") || strings.Contains(body, "event: tool_call") || strings.Contains(body, "synthetic-private-detail") {
						t.Fatal("unsafe auth failure")
					}
				}
			})
		}
	}
}
