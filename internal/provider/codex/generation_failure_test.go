package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/provider"
)

type diagnosticPrivateError struct{}

func (diagnosticPrivateError) Error() string { panic("SYNTHETIC-ACCOUNT-TOKEN-PATH") }

type diagnosticTimeout struct{ diagnosticPrivateError }

func (diagnosticTimeout) Timeout() bool   { return true }
func (diagnosticTimeout) Temporary() bool { return false }

type diagnosticCycle struct{}

func (e *diagnosticCycle) Error() string { panic("SYNTHETIC") }
func (e *diagnosticCycle) Unwrap() error { return e }

type rejectedDiagnosticBody struct{ reads, closes int }

func (b *rejectedDiagnosticBody) Read([]byte) (int, error) {
	b.reads++
	return 0, diagnosticPrivateError{}
}
func (b *rejectedDiagnosticBody) Close() error { b.closes++; return nil }

type diagnosticReader struct{ err error }

func (r diagnosticReader) Read([]byte) (int, error) { return 0, r.err }

func assertFailure(t *testing.T, err error, phase provider.Phase, category provider.Category, status int) {
	t.Helper()
	f, ok := provider.FailureOf(err)
	if !ok || f.Phase() != phase || f.Category() != category || f.HTTPStatus() != status {
		t.Fatalf("unexpected safe failure: %v %v", f, ok)
	}
	if strings.Contains(fmt.Sprintf("%+v %#v %s", err, err, err), "SYNTHETIC") {
		t.Fatal("leak")
	}
}
func diagnosticClient() RequestClient {
	return RequestClient{Credentials: credentials.SourceFunc(func(context.Context) (credentials.Credential, error) { return credential(), nil })}
}

type statusMutatingBody struct {
	rejectedDiagnosticBody
	response *http.Response
}

func (b *statusMutatingBody) Close() error { b.closes++; b.response.StatusCode = 500; return nil }
func TestGenerationHTTPCapturedBeforeClose(t *testing.T) {
	response := &http.Response{StatusCode: 429, Header: make(http.Header)}
	body := &statusMutatingBody{response: response}
	response.Body = body
	c := diagnosticClient()
	requests := 0
	c.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { requests++; return response, nil })
	_, err := c.Open(context.Background(), decodedRequest(t, `[{"role":"user","content":"SYNTHETIC"}]`))
	assertFailure(t, err, provider.PhaseHTTP, provider.HTTPRateLimited, 429)
	if requests != 1 || body.reads != 0 || body.closes != 1 || response.StatusCode != 500 {
		t.Fatal("capture/close contract")
	}
}

func TestGenerationHTTPFailureSnapshots(t *testing.T) {
	req := decodedRequest(t, `[{"role":"user","content":"SYNTHETIC-prompt"}]`)
	for _, tc := range []struct {
		status   int
		category provider.Category
	}{
		{401, provider.HTTPUnauthorized}, {403, provider.HTTPForbidden}, {429, provider.HTTPRateLimited}, {500, provider.HTTPServerError}, {503, provider.HTTPServerError}, {599, provider.HTTPServerError}, {302, provider.HTTPRejected}, {400, provider.HTTPRejected}, {418, provider.HTTPRejected},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			body := &rejectedDiagnosticBody{}
			count := 0
			c := diagnosticClient()
			c.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				count++
				return &http.Response{StatusCode: tc.status, Header: http.Header{"SYNTHETIC-header": []string{"SYNTHETIC-secret"}}, Body: body}, nil
			})
			_, err := c.Open(context.Background(), req)
			assertFailure(t, err, provider.PhaseHTTP, tc.category, tc.status)
			if !errors.Is(err, ErrResponse) || count != 1 || body.reads != 0 || body.closes != 1 {
				t.Fatal("reject body/retry contract", count, body)
			}
			var actual atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actual.Add(1)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, "SYNTHETIC-error-body")
			}))
			defer upstream.Close()
			c = diagnosticClient()
			c.URL = upstream.URL
			_, err = c.Stream(context.Background(), req, func(provider.Event) error { return nil })
			assertFailure(t, err, provider.PhaseHTTP, tc.category, tc.status)
			if actual.Load() != 1 {
				t.Fatal("extra generation")
			}
		})
	}
}
func TestGenerationTimeoutInspectionTotalBudget(t *testing.T) {
	children := make([]error, 1_000_000)
	for i := range children {
		children[i] = diagnosticPrivateError{}
	}
	children[len(children)-1] = diagnosticTimeout{}
	if isTimeout(errors.Join(children...)) {
		t.Fatal("searched beyond breadth budget")
	}
	cycle := &url.Error{}
	cycle.Err = cycle
	if isTimeout(cycle) {
		t.Fatal("cyclic URL timeout")
	}
	op := &net.OpError{}
	op.Err = op
	if isTimeout(op) {
		t.Fatal("cyclic network timeout")
	}
	if !isTimeout(errors.Join(diagnosticPrivateError{}, diagnosticTimeout{})) {
		t.Fatal("timeout precedence")
	}
}

func TestGenerationCaptureBeforeDiscard(t *testing.T) {
	req := decodedRequest(t, `[{"role":"user","content":"hello"}]`)
	for _, tc := range []struct {
		err     error
		timeout bool
	}{
		{diagnosticPrivateError{}, false},
		{&net.OpError{Op: "SYNTHETIC", Net: "SYNTHETIC", Err: &url.Error{Op: "SYNTHETIC", URL: "SYNTHETIC", Err: diagnosticTimeout{}}}, true},
		{errors.Join(diagnosticPrivateError{}, diagnosticTimeout{}), true},
		{&diagnosticCycle{}, false},
		{&net.OpError{Err: &net.DNSError{IsTimeout: true}}, true},
	} {
		c := diagnosticClient()
		c.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, tc.err })
		_, err := c.Open(context.Background(), req)
		category := provider.TransportFailure
		if tc.timeout {
			category = provider.TransportTimeout
		}
		assertFailure(t, err, provider.PhaseTransport, category, 0)
		if !errors.Is(err, ErrTransport) {
			t.Fatal("sentinel lost")
		}
		// Production wrapper retains only the fixed sentinel, never the raw cause.
		if errors.Unwrap(err) != ErrTransport {
			t.Fatal("raw cause retained")
		}
	}
	c := diagnosticClient()
	c.Credentials = credentials.SourceFunc(func(context.Context) (credentials.Credential, error) {
		return credentials.Credential{}, diagnosticPrivateError{}
	})
	_, err := c.Open(context.Background(), req)
	assertFailure(t, err, provider.PhaseCredentials, provider.CredentialsUnavailable, 0)
	_, err = c.Open(context.Background(), provider.Request{})
	assertFailure(t, err, provider.PhaseRequest, provider.RequestInvalid, 0)
}
func TestGenerationStreamClassifications(t *testing.T) {
	for _, tc := range []struct {
		r        io.Reader
		category provider.Category
	}{
		{strings.NewReader(""), provider.StreamIncomplete},
		{strings.NewReader("data: {}\n"), provider.StreamIncomplete},
		{strings.NewReader("data: invalid\n\n"), provider.StreamMalformed},
		{strings.NewReader("data: \xff\n\n"), provider.StreamMalformed},
		{diagnosticReader{diagnosticPrivateError{}}, provider.StreamReadFailure},
		{diagnosticReader{diagnosticTimeout{}}, provider.StreamReadTimeout},
		{strings.NewReader(frame("error", map[string]any{"message": "SYNTHETIC-error"})), provider.UpstreamError},
		{strings.NewReader(frame("response.failed", nil)), provider.UpstreamFailed},
		{strings.NewReader(frame("response.incomplete", nil)), provider.UpstreamIncomplete},
		{strings.NewReader(frame("response.refusal.delta", nil)), provider.Refusal},
		{strings.NewReader(frame("response.web_search_call.in_progress", nil)), provider.UnsupportedOutput},
	} {
		_, err := parseStream(tc.r, func(provider.Event) error { return nil })
		assertFailure(t, err, provider.PhaseStream, tc.category, 0)
	}
	// Even when a sink returns a parser sentinel, preserve exact sink identity.
	for _, sinkErr := range []error{ErrStream, ErrRefusal, ErrUnsupported, diagnosticPrivateError{}} {
		_, err := parseStream(strings.NewReader(added(0, msg(""))+textDelta("SYNTHETIC")), func(provider.Event) error { return sinkErr })
		if err != sinkErr {
			t.Fatal("sink identity lost")
		}
	}
}
func TestGenerationBudgetClassificationsAndPreOpenWatchdog(t *testing.T) {
	req := decodedRequest(t, `[{"role":"user","content":"hello"}]`)
	for _, category := range []provider.Category{provider.InactivityTimeout, provider.GenerationTimeout} {
		c := diagnosticClient()
		opened := 0
		c.Credentials = credentials.SourceFunc(func(ctx context.Context) (credentials.Credential, error) {
			<-ctx.Done()
			return credentials.Credential{}, diagnosticPrivateError{}
		})
		c.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { opened++; return nil, diagnosticPrivateError{} })
		generation, inactivity := time.Hour, time.Millisecond
		if category == provider.GenerationTimeout {
			generation, inactivity = time.Millisecond, time.Hour
		}
		_, err := c.stream(context.Background(), req, func(provider.Event) error { return nil }, generation, inactivity)
		assertFailure(t, err, provider.PhaseStream, category, 0)
		if opened != 0 || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("budget/open semantics")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := diagnosticClient().Stream(ctx, req, func(provider.Event) error { return nil })
	if err != context.Canceled {
		t.Fatal("parent identity")
	}
	if _, ok := provider.FailureOf(err); ok {
		t.Fatal("classified parent cancel")
	}
}
