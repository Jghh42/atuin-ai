package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

type diagnosticTransport func(*http.Request) (*http.Response, error)

func (f diagnosticTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const diagnosticSecret = "PRIVATE-CATALOG-CONTENT"
const diagnosticModel = `{"slug":"gpt-6.1-sol","display_name":"PRIVATE-CATALOG-CONTENT","description":"PRIVATE-CATALOG-CONTENT","visibility":"list","priority":2}`

func diagnosticBody(model string) string { return `{"models":[` + model + `]}` }
func diagnosticResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Location": {"https://PRIVATE-CATALOG-CONTENT.invalid/"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func checkDiagnosticRedaction(t *testing.T, report DiagnosticReport) map[string]any {
	t.Helper()
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if json.Unmarshal(b, &fields) != nil || len(fields) != 12 {
		t.Fatal("diagnostic JSON shape changed")
	}
	var logged strings.Builder
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("diagnostic", "report", report)
	outputs := []string{string(b), logged.String()}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d"} {
		outputs = append(outputs, fmt.Sprintf(verb, report))
	}
	for _, output := range outputs {
		for _, secret := range []string{diagnosticSecret, "fake-token", "fake-account", "gpt-6.1-sol", "Bearer", "https://"} {
			if strings.Contains(output, secret) {
				t.Fatal("diagnostic disclosed private source data")
			}
		}
	}
	return fields
}
func TestDiagnosticHTTPAndValidationClasses(t *testing.T) {
	valid := diagnosticBody(diagnosticModel)
	cases := []struct {
		name, body, override string
		status               int
		stage                DiagnosticStage
		result               DiagnosticResult
	}{
		{name: "success", body: valid, stage: StageComplete, result: ResultSuccess},
		{name: "valid override", body: valid, override: "gpt-6.1-sol", stage: StageComplete, result: ResultSuccess},
		{name: "invalid override", body: valid, override: diagnosticSecret, stage: StageSelection, result: ResultInvalidDefaultOverride},
		{name: "proven absent", body: strings.ReplaceAll(valid, "gpt-6.1-sol", "other-model"), stage: StageSelection, result: ResultProvenModelAbsent},
		{name: "proven hidden", body: strings.ReplaceAll(valid, `"list"`, `"hide"`), stage: StageSelection, result: ResultProvenModelHidden},
		{name: "proven none", body: strings.ReplaceAll(valid, `"list"`, `"none"`), stage: StageSelection, result: ResultProvenModelHidden},
		{name: "empty", body: `{"models":[]}`, stage: StageMetadata, result: ResultEmptyCatalog},
		{name: "malformed JSON", body: `{` + diagnosticSecret, stage: StageSchema, result: ResultInvalidJSON},
		{name: "trailing JSON", body: valid + `{}`, stage: StageSchema, result: ResultInvalidJSON},
		{name: "outer array", body: `[]`, stage: StageSchema, result: ResultInvalidSchema},
		{name: "missing models", body: `{}`, stage: StageSchema, result: ResultInvalidSchema},
		{name: "null models", body: `{"models":null}`, stage: StageSchema, result: ResultInvalidSchema},
		{name: "object models", body: `{"models":{}}`, stage: StageSchema, result: ResultInvalidSchema},
		{name: "missing field", body: `{"models":[{}]}`, stage: StageSchema, result: ResultInvalidSchema},
		{name: "null priority", body: strings.Replace(valid, `"priority":2`, `"priority":null`, 1), stage: StageSchema, result: ResultInvalidSchema},
		{name: "float priority", body: strings.Replace(valid, `"priority":2`, `"priority":2.5`, 1), stage: StageSchema, result: ResultInvalidSchema},
		{name: "overflow priority", body: strings.Replace(valid, `"priority":2`, `"priority":2147483648`, 1), stage: StageSchema, result: ResultInvalidSchema},
		{name: "count bound", body: diagnosticBody(strings.Repeat(diagnosticModel+",", 256) + diagnosticModel), stage: StageSchema, result: ResultInvalidSchema},
		{name: "duplicate IDs", body: diagnosticBody(diagnosticModel + "," + diagnosticModel), stage: StageMetadata, result: ResultInvalidMetadata},
		{name: "invalid unrelated metadata", body: diagnosticBody(diagnosticModel + `,{"slug":"bad id","display_name":"PRIVATE-CATALOG-CONTENT","visibility":"list","priority":0}`), stage: StageMetadata, result: ResultInvalidMetadata},
		{name: "invalid unrelated schema", body: diagnosticBody(diagnosticModel + `,{"slug":"other"}`), stage: StageSchema, result: ResultInvalidSchema},
		{name: "invalid visibility", body: strings.Replace(valid, `"list"`, `"new-visibility"`, 1), stage: StageMetadata, result: ResultInvalidMetadata},
		{name: "empty display", body: strings.Replace(valid, `"display_name":"PRIVATE-CATALOG-CONTENT"`, `"display_name":""`, 1), stage: StageMetadata, result: ResultInvalidMetadata},
		{name: "name bound", body: strings.Replace(valid, `"display_name":"PRIVATE-CATALOG-CONTENT"`, `"display_name":"`+strings.Repeat("n", 257)+`"`, 1), stage: StageMetadata, result: ResultInvalidMetadata},
		{name: "description bound", body: strings.Replace(valid, `"description":"PRIVATE-CATALOG-CONTENT"`, `"description":"`+strings.Repeat("d", 4097)+`"`, 1), stage: StageMetadata, result: ResultInvalidMetadata},
		{name: "body bound", body: strings.Repeat("x", MaxCatalogBytes+1), stage: StageBody, result: ResultBodyTooLarge},
		{name: "UTF8", body: string([]byte{0xff}), stage: StageBody, result: ResultInvalidUTF8},
		{name: "401", status: 401, stage: StageHTTP, result: ResultHTTPUnauthorized},
		{name: "403", status: 403, stage: StageHTTP, result: ResultHTTPForbidden},
		{name: "429", status: 429, stage: StageHTTP, result: ResultHTTPRateLimited},
		{name: "500", status: 500, stage: StageHTTP, result: ResultHTTPServerError},
		{name: "503", status: 503, stage: StageHTTP, result: ResultHTTPServerError},
		{name: "404", status: 404, stage: StageHTTP, result: ResultHTTPRejected},
		{name: "redirect", status: 302, stage: StageHTTP, result: ResultHTTPRejected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			status := tc.status
			if status == 0 {
				status = 200
			}
			d := Discovery{Transport: diagnosticTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != ModelsURL || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer fake-token" || r.Header.Get("ChatGPT-Account-Id") != "fake-account" || r.Header.Get("User-Agent") != "atuin-ai/phase3a" || r.Header.Get("Accept") != "application/json" || r.Header.Get("originator") != "" {
					t.Error("diagnostic changed native request contract")
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 8*time.Second {
					t.Error("unbounded diagnostic request")
				}
				return diagnosticResponse(status, tc.body), nil
			})}
			report := d.Diagnose(context.Background(), credential(), tc.override)
			fields := checkDiagnosticRedaction(t, report)
			if calls != 1 || report.Stage != tc.stage || report.Result != tc.result || report.HTTPStatus == nil || *report.HTTPStatus != status {
				t.Fatalf("wrong diagnostic class: %v", report)
			}
			if *report.DefaultOverrideConfigured != (tc.override != "") {
				t.Fatal("override presence missing")
			}
			switch tc.stage {
			case StageHTTP, StageBody:
				if fields["schema_validated"] != nil || fields["metadata_validated"] != nil || fields["model_count"] != nil || fields["eligible_count"] != nil {
					t.Fatal("unknown validation emitted as false/zero")
				}
			case StageSchema:
				if fields["schema_validated"] != false || fields["metadata_validated"] != nil || fields["model_count"] != nil || fields["eligible_count"] != nil {
					t.Fatal("schema failure claimed validated metadata")
				}
			case StageMetadata:
				if fields["schema_validated"] != true || fields["metadata_validated"] != false || fields["proven_model_present"] != nil || fields["eligible_count"] != nil {
					t.Fatal("invalid metadata claimed eligibility")
				}
				if tc.result == ResultEmptyCatalog && fields["model_count"] != float64(0) {
					t.Fatal("empty array count not distinguished")
				}
			default:
				if fields["schema_validated"] != true || fields["metadata_validated"] != true || fields["model_count"] != float64(1) {
					t.Fatal("valid metadata not reported")
				}
			}
			if tc.result == ResultProvenModelAbsent && (fields["proven_model_present"] != false || fields["listed_count"] != float64(1) || fields["eligible_count"] != float64(0)) {
				t.Fatal("missing proven model not distinguished")
			}
			if tc.result == ResultProvenModelHidden && (fields["proven_model_present"] != true || fields["proven_model_listed"] != false || fields["eligible_count"] != float64(0)) {
				t.Fatal("hidden proven model not distinguished")
			}
			if tc.result == ResultInvalidDefaultOverride && fields["default_override_valid"] != false {
				t.Fatal("override failure not distinguished")
			}
			if tc.result == ResultSuccess && (fields["eligible_count"] != float64(1) || fields["proven_model_listed"] != true) {
				t.Fatal("successful eligibility missing")
			}
			if tc.override == "" && fields["default_override_valid"] != nil {
				t.Fatal("absent override validity should be unknown")
			}
		})
	}
}

type diagnosticErrorReader struct{}

func (diagnosticErrorReader) Read([]byte) (int, error) { return 0, errors.New(diagnosticSecret) }
func (diagnosticErrorReader) Close() error             { return nil }

func TestDiagnosticIOErrorsAndEnumDenyDefault(t *testing.T) {
	for _, tc := range []struct {
		name      string
		transport diagnosticTransport
		want      DiagnosticResult
		stage     DiagnosticStage
	}{
		{"transport", func(*http.Request) (*http.Response, error) { return nil, errors.New(diagnosticSecret) }, ResultTransportError, StageRequest},
		{"timeout", func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }, ResultTimeout, StageRequest},
		{"canceled", func(*http.Request) (*http.Response, error) { return nil, context.Canceled }, ResultCanceled, StageRequest},
		{"body read", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: diagnosticErrorReader{}}, nil
		}, ResultBodyReadError, StageBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := (Discovery{Transport: tc.transport}).Diagnose(context.Background(), credential(), "")
			checkDiagnosticRedaction(t, report)
			if report.Result != tc.want || report.Stage != tc.stage || report.ModelCount != nil {
				t.Fatal("wrong IO diagnostic")
			}
			if tc.stage == StageRequest && report.HTTPStatus != nil {
				t.Fatal("invented HTTP status")
			}
		})
	}
	for _, cancel := range []bool{false, true} {
		ctx, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
		calls := 0
		if cancel {
			stop()
		}
		report := (Discovery{Transport: diagnosticTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			<-r.Context().Done()
			return nil, r.Context().Err()
		})}).Diagnose(ctx, credential(), "")
		stop()
		want := ResultTimeout
		if cancel {
			want = ResultCanceled
		}
		if report.Result != want || calls > 1 {
			t.Fatal("request cancellation/timeout failed")
		}
	}
	r := DiagnosticReport{Stage: DiagnosticStage(255), Result: DiagnosticResult(255)}
	fields := checkDiagnosticRedaction(t, r)
	if fields["stage"] != "unknown" || fields["result"] != "unknown" {
		t.Fatal("enum failed deny-default")
	}
	invalid := (Discovery{URL: ":" + diagnosticSecret}).Diagnose(context.Background(), credential(), "")
	checkDiagnosticRedaction(t, invalid)
	if invalid.Result != ResultInvalidRequest {
		t.Fatal("invalid request not sanitized")
	}
}
