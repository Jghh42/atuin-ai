package codexproof

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoveryMetadataValidationReport(t *testing.T) {
	model := `{"slug":"unproven","display_name":"Synthetic","description":"","visibility":"list","priority":0}`
	bodies := []string{
		`{"models":[` + strings.Replace(model, `"list"`, `"future-enum"`, 1) + `]}`,
		`{"models":[` + strings.Replace(model, `"unproven"`, `"bad id"`, 1) + `]}`,
		`{"models":[` + strings.Replace(model, `"Synthetic"`, `""`, 1) + `]}`,
		`{"models":[` + strings.Replace(model, `"Synthetic"`, `"`+strings.Repeat("x", 257)+`"`, 1) + `]}`,
		`{"models":[` + strings.Replace(model, `"description":""`, `"description":"`+strings.Repeat("x", 4097)+`"`, 1) + `]}`,
		`{"models":[` + model + `,` + model + `]}`,
	}
	for _, body := range bodies {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
		report, _, err := discover(context.Background(), Record{AccessToken: "fake-token", AccountID: "fake-account"}, s.Client().Transport, s.URL)
		s.Close()
		if err == nil || report.SchemaValidated {
			t.Fatal("malformed metadata reported validated", report, err)
		}
	}
}

func TestDiscoverySafeReport(t *testing.T) {
	for _, id := range []string{"unproven-sensitive-model", "gpt-6.1-sol"} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer secret-access" || r.Header.Get("ChatGPT-Account-Id") != "secret-account" {
				t.Error("credential binding")
			}
			_, _ = io.WriteString(w, `{"models":[{"slug":"`+id+`","display_name":"sensitive display","description":"sensitive description","visibility":"list","priority":2}]}`)
		}))
		report, _, err := discover(context.Background(), Record{AccessToken: "secret-access", AccountID: "secret-account", Generation: 1}, s.Client().Transport, s.URL)
		s.Close()
		if (err == nil) != (id == "gpt-6.1-sol") || !report.SchemaValidated || report.HTTPStatus != 200 || report.ModelCount != 1 {
			t.Fatal(report, err)
		}
		b, e := json.Marshal(report)
		if e != nil {
			t.Fatal(e)
		}
		for _, secret := range []string{"secret-account", "secret-access", "sensitive", "unproven-sensitive-model"} {
			if strings.Contains(string(b), secret) {
				t.Fatal("unsafe discovery report")
			}
		}
	}
}
