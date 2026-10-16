package codexproof

import (
	"context"
	"net/http"

	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
)

// DiscoveryReport retains only fixed-schema counters, never native field values.
type DiscoveryReport struct {
	SchemaValidated    bool `json:"schema_validated"`
	HTTPStatus         int  `json:"http_status"`
	ModelCount         int  `json:"model_count"`
	ProvenModelPresent bool `json:"proven_model_present"`
	ProvenModelListed  bool `json:"proven_model_listed"`
	IntegerPriorities  bool `json:"integer_priorities"`
}
type discoveryTransport struct {
	report *DiscoveryReport
	base   http.RoundTripper
}

func (t discoveryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.base.RoundTrip(req)
	if err == nil {
		t.report.HTTPStatus = res.StatusCode
	}
	return res, err
}
func Discover(ctx context.Context, r Record) (DiscoveryReport, catalog.Snapshot, error) {
	return discover(ctx, r, http.DefaultTransport, "")
}

// Inject HTTP in automated tests; the opt-in command alone uses real transport.
func discover(ctx context.Context, r Record, transport http.RoundTripper, url string) (DiscoveryReport, catalog.Snapshot, error) {
	report := DiscoveryReport{}
	credential := credentials.Credential{Identity: credentials.Identity{Provider: "codex", Account: r.AccountID, Epoch: r.Generation}, AccessToken: r.AccessToken}
	entries, err := (codex.Discovery{Transport: discoveryTransport{&report, transport}, URL: url}).Fetch(ctx, credential)
	report.SchemaValidated = err == nil
	if err != nil {
		return report, catalog.Snapshot{}, err
	}
	report.ModelCount = len(entries)
	report.IntegerPriorities = true
	for _, m := range entries {
		if m.ID == "gpt-6.1-sol" {
			report.ProvenModelPresent = true
			report.ProvenModelListed = m.Visibility == "list"
		}
	}
	snapshot, err := catalog.Build(entries, "")
	return report, snapshot, err
}
