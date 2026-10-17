package codex

import (
	"context"
	"encoding/json"
	"errors"
	"net"

	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

// DiagnosticStage and DiagnosticResult are closed enums. Unknown values encode
// as "unknown", never arbitrary upstream or caller-provided strings.
type DiagnosticStage uint8
type DiagnosticResult uint8

const (
	StageUnknown DiagnosticStage = iota
	StageConfiguration
	StageCredentials
	StageRequest
	StageHTTP
	StageBody
	StageSchema
	StageMetadata
	StageSelection
	StageComplete
)

const (
	ResultUnknown DiagnosticResult = iota
	ResultConfigurationUnavailable
	ResultCredentialUnavailable
	ResultRefreshRequired
	ResultLifecycleInvalidated
	ResultCanceled
	ResultInvalidRequest
	ResultTransportError
	ResultTimeout
	ResultHTTPUnauthorized
	ResultHTTPForbidden
	ResultHTTPRateLimited
	ResultHTTPServerError
	ResultHTTPRejected
	ResultBodyReadError
	ResultBodyTooLarge
	ResultInvalidUTF8
	ResultInvalidJSON
	ResultInvalidSchema
	ResultInvalidMetadata
	ResultEmptyCatalog
	ResultProvenModelAbsent
	ResultProvenModelHidden
	ResultInvalidDefaultOverride
	ResultSuccess
)

func (s DiagnosticStage) String() string {
	switch s {
	case StageConfiguration:
		return "configuration"
	case StageCredentials:
		return "credentials"
	case StageRequest:
		return "request"
	case StageHTTP:
		return "http"
	case StageBody:
		return "body"
	case StageSchema:
		return "schema"
	case StageMetadata:
		return "metadata"
	case StageSelection:
		return "selection"
	case StageComplete:
		return "complete"
	default:
		return "unknown"
	}
}
func (r DiagnosticResult) String() string {
	switch r {
	case ResultConfigurationUnavailable:
		return "configuration_unavailable"
	case ResultCredentialUnavailable:
		return "credential_unavailable"
	case ResultRefreshRequired:
		return "refresh_required"
	case ResultLifecycleInvalidated:
		return "lifecycle_invalidated"
	case ResultCanceled:
		return "canceled"
	case ResultInvalidRequest:
		return "invalid_request"
	case ResultTransportError:
		return "transport_error"
	case ResultTimeout:
		return "timeout"
	case ResultHTTPUnauthorized:
		return "http_unauthorized"
	case ResultHTTPForbidden:
		return "http_forbidden"
	case ResultHTTPRateLimited:
		return "http_rate_limited"
	case ResultHTTPServerError:
		return "http_server_error"
	case ResultHTTPRejected:
		return "http_rejected"
	case ResultBodyReadError:
		return "body_read_error"
	case ResultBodyTooLarge:
		return "body_too_large"
	case ResultInvalidUTF8:
		return "invalid_utf8"
	case ResultInvalidJSON:
		return "invalid_json"
	case ResultInvalidSchema:
		return "invalid_schema"
	case ResultInvalidMetadata:
		return "invalid_metadata"
	case ResultEmptyCatalog:
		return "empty_catalog"
	case ResultProvenModelAbsent:
		return "proven_model_absent"
	case ResultProvenModelHidden:
		return "proven_model_hidden"
	case ResultInvalidDefaultOverride:
		return "invalid_default_override"
	case ResultSuccess:
		return "success"
	default:
		return "unknown"
	}
}
func (s DiagnosticStage) MarshalJSON() ([]byte, error)  { return json.Marshal(s.String()) }
func (r DiagnosticResult) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

// DiagnosticReport is the complete output allowlist: no errors, strings from
// responses, URLs, headers, credential fields, or model metadata are retained.
// Pointers encode unknown/not evaluated as null instead of false/zero.
type DiagnosticReport struct {
	Stage                     DiagnosticStage  `json:"stage"`
	Result                    DiagnosticResult `json:"result"`
	HTTPStatus                *int             `json:"http_status"`
	SchemaValidated           *bool            `json:"schema_validated"`
	MetadataValidated         *bool            `json:"metadata_validated"`
	ModelCount                *int             `json:"model_count"`
	ListedCount               *int             `json:"listed_count"`
	ProvenModelPresent        *bool            `json:"proven_model_present"`
	ProvenModelListed         *bool            `json:"proven_model_listed"`
	EligibleCount             *int             `json:"eligible_count"`
	DefaultOverrideConfigured *bool            `json:"default_override_configured"`
	DefaultOverrideValid      *bool            `json:"default_override_valid"`
}

func diagnosticValue[T any](v T) *T { return &v }
func (r *DiagnosticReport) fail(stage DiagnosticStage, result DiagnosticResult) {
	if r != nil {
		r.Stage, r.Result = stage, result
	}
}
func diagnosticIOResult(ctx context.Context, err error, fallback DiagnosticResult) DiagnosticResult {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return ResultCanceled
	}
	var timeout net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return ResultTimeout
	}
	return fallback
}

// Diagnose makes exactly the same bounded native metadata request/decoding as
// Fetch, then historical replay-proof selection (NOT bundled runtime membership).
// Credential lifecycle/no-refresh
// policy belongs to the application manager, not this provider function.
func (d Discovery) Diagnose(ctx context.Context, c credentials.Credential, override string) DiagnosticReport {
	r := DiagnosticReport{DefaultOverrideConfigured: diagnosticValue(override != "")}
	entries, err := d.fetch(ctx, c, &r)
	if err != nil {
		return r
	}
	e := catalog.InspectEligibility(entries)
	r.ListedCount = diagnosticValue(e.ListedCount)
	r.ProvenModelPresent = diagnosticValue(e.ProvenPresent)
	r.ProvenModelListed = diagnosticValue(e.ProvenListed)
	r.EligibleCount = diagnosticValue(e.EligibleCount)
	_, err = catalog.Build(entries, override)
	switch {
	case e.EligibleCount == 0 && !e.ProvenPresent:
		r.fail(StageSelection, ResultProvenModelAbsent)
	case e.EligibleCount == 0:
		r.fail(StageSelection, ResultProvenModelHidden)
	case errors.Is(err, catalog.ErrOverride):
		r.DefaultOverrideValid = diagnosticValue(false)
		r.fail(StageSelection, ResultInvalidDefaultOverride)
	case err != nil:
		r.fail(StageMetadata, ResultInvalidMetadata)
	default:
		if override != "" {
			r.DefaultOverrideValid = diagnosticValue(true)
		}
		r.fail(StageComplete, ResultSuccess)
	}
	return r
}
