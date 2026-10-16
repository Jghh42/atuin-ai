package codexproof

import (
	"context"
	"errors"
	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"testing"
	"time"
)

type candidateFetcher []catalog.Entry

func (f candidateFetcher) Fetch(context.Context, credentials.Credential) ([]catalog.Entry, error) {
	return f, nil
}
func TestCandidateSelection(t *testing.T) {
	entry := func(id string, p int32) catalog.Entry {
		return catalog.Entry{ID: id, Name: "Synthetic", Visibility: "list", Priority: p}
	}
	for _, tc := range []struct {
		entries []catalog.Entry
		want    string
	}{
		{[]catalog.Entry{entry("other", 0), entry("a-sol", 10), entry("b-sol", 2)}, "b-sol"},
		{[]catalog.Entry{entry("z", 2), entry("a", 2)}, "a"},
		{[]catalog.Entry{entry("z", 10), entry("a", 2)}, "a"},
	} {
		got, err := selectReplayCandidate(tc.entries)
		if err != nil || got != tc.want {
			t.Fatal(got, err)
		}
	}
	if _, err := selectReplayCandidate([]catalog.Entry{{ID: "hidden", Name: "Hidden", Visibility: "hide"}}); err == nil {
		t.Fatal("hidden selected")
	}
}
func TestDiscoveredReplayIsOneAttemptAndRedactsFailure(t *testing.T) {
	record := Record{AccessToken: "fake-token", AccountID: "fake-account", ExpiresAt: time.Now().Add(time.Hour)}
	entries := candidateFetcher{{ID: "candidate-a", Name: "Synthetic", Visibility: "list"}, {ID: "candidate-b", Name: "Synthetic", Visibility: "list", Priority: 1}}
	for _, success := range []bool{false, true} {
		calls := 0
		report, err := proveDiscovered(context.Background(), record, entries, func(_ context.Context, id string) (InferenceReport, error) {
			calls++
			if id != "candidate-a" {
				t.Fatal("unexpected selection")
			}
			r := InferenceReport{Model: id, TextCompleted: true, ToolCompleted: true, ContinuationCompleted: true, CallIDCorrelated: true, ReplayWithoutOpaqueState: true}
			if !success {
				return r, errors.New("safe failure")
			}
			return r, nil
		})
		if calls != 1 || (err == nil) != success {
			t.Fatal(calls, err)
		}
		if !success && report.Model != "" {
			t.Fatal("unproven candidate leaked")
		}
	}
}
