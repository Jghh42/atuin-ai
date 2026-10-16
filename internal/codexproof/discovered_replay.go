package codexproof

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
)

// ProveDiscovered attempts ONE actual picker-visible candidate, with at most
// three harmless inference generations and no retries or local execution.
// A candidate ID is included in the report only after successful replay proof.
func ProveDiscovered(ctx context.Context, r Record) (InferenceReport, error) {
	return proveDiscovered(ctx, r, codex.Discovery{}, func(ctx context.Context, model string) (InferenceReport, error) {
		return ProveInference(ctx, r.AccessToken, r.AccountID, model)
	})
}
func proveDiscovered(ctx context.Context, r Record, fetcher catalog.Fetcher, prove func(context.Context, string) (InferenceReport, error)) (InferenceReport, error) {
	if r.AccessToken == "" || r.AccountID == "" || !time.Now().Before(r.ExpiresAt) {
		return InferenceReport{}, errors.New("current isolated credential required")
	}
	entries, err := fetcher.Fetch(ctx, credentials.Credential{Identity: credentials.Identity{Provider: "codex", Account: r.AccountID, Epoch: r.Generation}, AccessToken: r.AccessToken})
	if err != nil {
		return InferenceReport{}, errors.New("native discovery failed")
	}
	model, err := selectReplayCandidate(entries)
	if err != nil {
		return InferenceReport{}, err
	}
	report, err := prove(ctx, model)
	if err != nil {
		report.Model = ""
		return report, err
	}
	if report.Model != model || !report.TextCompleted || !report.ToolCompleted || !report.ContinuationCompleted || !report.CallIDCorrelated || !report.ReplayWithoutOpaqueState {
		report.Model = ""
		return report, errors.New("incomplete replay proof")
	}
	return report, nil
}
func selectReplayCandidate(entries []catalog.Entry) (string, error) {
	if err := catalog.Validate(entries); err != nil {
		return "", errors.New("invalid native candidate metadata")
	}
	candidates := make([]catalog.Entry, 0, len(entries))
	for _, e := range entries {
		if e.Visibility == "list" {
			candidates = append(candidates, e)
		}
	}
	if len(candidates) == 0 {
		return "", errors.New("no visible native candidate")
	}
	// No verified release-order convention exists. Use the documented native
	// priority fallback within actual Sol candidates, then the remaining picker.
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		as, bs := strings.HasSuffix(a.ID, "-sol"), strings.HasSuffix(b.ID, "-sol")
		if as != bs {
			return as
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.ID < b.ID
	})
	return candidates[0].ID, nil
}
