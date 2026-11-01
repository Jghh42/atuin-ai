package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

type privateError struct{}

func (privateError) Error() string { panic("SYNTHETIC-token-account-session-path") }

type cycleError struct {
	next  error
	calls int
}

func (*cycleError) Error() string   { panic("private") }
func (e *cycleError) Unwrap() error { e.calls++; return e.next }

type wideError struct {
	children []error
	calls    int
}

func (*wideError) Error() string     { panic("private") }
func (e *wideError) Unwrap() []error { e.calls++; return e.children }

func TestFailureClosedPresentations(t *testing.T) {
	sentinel := errors.New("fixed sentinel")
	for p := 0; p < 256; p++ {
		for c := 0; c < 256; c++ {
			f := NewFailure(Phase(p), Category(c), 999)
			if p > int(PhaseContinuation) && f.Phase() != PhaseUnknown {
				t.Fatal("phase not closed")
			}
			if c > int(UnsupportedNamespace) && f.Category() != CategoryUnknown {
				t.Fatal("category not closed")
			}
			if f.HTTPStatus() != 0 {
				t.Fatal("invalid status retained")
			}
		}
	}
	for _, status := range []int{-1, 0, 99, 100, 401, 599, 600} {
		f := NewFailure(PhaseHTTP, HTTPUnauthorized, status)
		if (status < 100 || status > 599) && f.HTTPStatus() != 0 {
			t.Fatal(status)
		}
		for _, value := range []any{f, WithFailure(privateError{}, f), Phase(255), Category(255)} {
			for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%x", "%T", "%p", "%w"} {
				text := fmt.Sprintf(verb, value)
				if strings.Contains(text, "SYNTHETIC") || strings.Contains(text, "PANIC") {
					t.Fatal(text)
				}
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if status == 0 && len(encoded) > 0 && encoded[0] == '{' && !bytes.Contains(encoded, []byte(`"upstream_http_status":null`)) {
				t.Fatal(string(encoded))
			}
			var buf bytes.Buffer
			slog.New(slog.NewJSONHandler(&buf, nil)).Info("safe", "failure", value)
			if strings.Contains(buf.String(), "SYNTHETIC") || strings.Contains(buf.String(), "PANIC") {
				t.Fatal(buf.String())
			}
		}
		wrapped := WithFailure(sentinel, f)
		if !errors.Is(wrapped, sentinel) {
			t.Fatal("sentinel lost")
		}
		got, ok := FailureOf(fmt.Errorf("outer: %w", errors.Join(privateError{}, wrapped)))
		if !ok || got != f {
			t.Fatal("metadata lost")
		}
	}
}
func TestFailureInspectionSkipsNilJoinedChildren(t *testing.T) {
	f := NewFailure(PhaseHTTP, HTTPRateLimited, 429)
	typed := WithFailure(nil, f)
	cycle := &cycleError{}
	cycle.next = cycle
	for _, err := range []error{
		&wideError{children: []error{nil, typed}},
		&wideError{children: []error{nil, &wideError{children: []error{nil, typed, cycle}}, cycle}},
	} {
		got, ok := FailureOf(err)
		if !ok || got != f {
			t.Errorf("lost typed failure after nil sibling: %v %v", got, ok)
		}
	}
	for _, n := range []int{62, 63, 1_000_000} {
		children := make([]error, n+1)
		children[n] = typed
		err := &wideError{children: children}
		got, ok := FailureOf(err)
		if ok != (n == 62) || (ok && got != f) {
			t.Errorf("nil breadth budget n=%d found=%v", n, ok)
		}
	}
	err := &wideError{children: []error{nil, cycle, typed}}
	if _, ok := FailureOf(err); ok || cycle.calls > 64 {
		t.Fatal("cycle escaped total budget")
	}
}

func TestFailureInspection64thNodeNilFirstChild(t *testing.T) {
	for _, depth := range []int{62, 63, 64} {
		last := &wideError{children: []error{nil, privateError{}}}
		var root error = last
		for range depth {
			root = &cycleError{next: root}
		}
		visited := 0
		WalkErrors(root, func(error) bool { visited++; return false })
		want := depth + 1
		if want > 64 {
			want = 64
		}
		if visited != want {
			t.Fatalf("depth=%d visited=%d want=%d", depth, visited, want)
		}
		if (last.calls == 1) != (depth < 64) {
			t.Fatal("unwrap boundary changed")
		}
	}
}

func TestFailureInspectionTotalBudget(t *testing.T) {
	cycle := &cycleError{}
	cycle.next = cycle
	if _, ok := FailureOf(cycle); ok || cycle.calls > 64 {
		t.Fatal("cycle not bounded", cycle.calls)
	}
	children := make([]error, 1_000_000)
	for i := range children {
		children[i] = privateError{}
	}
	children[len(children)-1] = WithFailure(nil, NewFailure(PhaseHTTP, HTTPForbidden, 403))
	wide := &wideError{children: children}
	count := 0
	WalkErrors(wide, func(error) bool { count++; return false })
	if count > 64 || wide.calls != 1 {
		t.Fatal("breadth not bounded", count, wide.calls)
	}
	if _, ok := FailureOf(wide); ok {
		t.Fatal("searched beyond budget")
	}
	nilWide := &wideError{children: make([]error, 1_000_000)}
	WalkErrors(nilWide, func(error) bool { return false })
	var nested error = privateError{}
	for range 1000 {
		nested = &cycleError{next: nested}
	}
	if _, ok := FailureOf(nested); ok {
		t.Fatal("unexpected metadata")
	}
}
