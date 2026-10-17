// Package codex implements native Codex discovery, not public /v1/models.
package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

const ModelsURL = "https://chatgpt.com/backend-api/codex/models?client_version=0.134.0"
const MaxCatalogBytes = 2 << 20

// Discovery uses an injected transport in tests. Redirects are never followed:
// credentials must not escape the reviewed origin. URL is only for test servers.
type Discovery struct {
	Transport http.RoundTripper
	URL       string
}

func (d Discovery) Fetch(ctx context.Context, c credentials.Credential) ([]catalog.Entry, error) {
	return d.fetch(ctx, c, nil)
}

func (d Discovery) fetch(ctx context.Context, c credentials.Credential, report *DiagnosticReport) ([]catalog.Entry, error) {
	ctx, cancel := context.WithTimeout(ctx, catalog.FetchTimeout)
	defer cancel()
	url := d.URL
	if url == "" {
		url = ModelsURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		report.fail(StageRequest, ResultInvalidRequest)
		return nil, catalog.ErrInvalid
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("ChatGPT-Account-Id", c.Identity.Account)
	req.Header.Set("User-Agent", "atuin-ai/phase3a")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Transport: d.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		report.fail(StageRequest, diagnosticIOResult(ctx, err, ResultTransportError))
		return nil, catalog.ErrTransient
	}
	defer res.Body.Close()
	if report != nil {
		report.HTTPStatus = diagnosticValue(res.StatusCode)
	}
	if res.StatusCode == 401 || res.StatusCode == 403 {
		result := ResultHTTPUnauthorized
		if res.StatusCode == 403 {
			result = ResultHTTPForbidden
		}
		report.fail(StageHTTP, result)
		return nil, catalog.ErrUnauthorized
	}
	if res.StatusCode == 429 || res.StatusCode >= 500 {
		result := ResultHTTPServerError
		if res.StatusCode == 429 {
			result = ResultHTTPRateLimited
		}
		report.fail(StageHTTP, result)
		return nil, catalog.ErrTransient
	}
	if res.StatusCode != 200 {
		report.fail(StageHTTP, ResultHTTPRejected)
		return nil, catalog.ErrInvalid
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxCatalogBytes+1))
	if err != nil {
		report.fail(StageBody, diagnosticIOResult(ctx, err, ResultBodyReadError))
		return nil, catalog.ErrTransient
	}
	if len(body) > MaxCatalogBytes {
		report.fail(StageBody, ResultBodyTooLarge)
		return nil, catalog.ErrInvalid
	}
	if !utf8.Valid(body) {
		report.fail(StageBody, ResultInvalidUTF8)
		return nil, catalog.ErrInvalid
	}
	var wire struct {
		Models json.RawMessage `json:"models"`
	}
	if json.Unmarshal(body, &wire) != nil {
		result := ResultInvalidSchema
		if report != nil && !json.Valid(body) {
			result = ResultInvalidJSON
		}
		report.fail(StageSchema, result)
		if report != nil {
			report.SchemaValidated = diagnosticValue(false)
		}
		return nil, catalog.ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(wire.Models))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		report.fail(StageSchema, ResultInvalidSchema)
		if report != nil {
			report.SchemaValidated = diagnosticValue(false)
		}
		return nil, catalog.ErrInvalid
	}
	entries := make([]catalog.Entry, 0, 32)
	for decoder.More() {
		// Enforce count before allocating/decoding another model, not afterward.
		if len(entries) >= 256 {
			report.fail(StageSchema, ResultInvalidSchema)
			if report != nil {
				report.SchemaValidated = diagnosticValue(false)
			}
			return nil, catalog.ErrInvalid
		}
		var m struct {
			ID          *string `json:"slug"`
			Name        *string `json:"display_name"`
			Description string  `json:"description"`
			Visibility  *string `json:"visibility"`
			Priority    *int32  `json:"priority"`
		}
		if decoder.Decode(&m) != nil || m.ID == nil || m.Name == nil || m.Visibility == nil || m.Priority == nil {
			report.fail(StageSchema, ResultInvalidSchema)
			if report != nil {
				report.SchemaValidated = diagnosticValue(false)
			}
			return nil, catalog.ErrInvalid
		}
		entries = append(entries, catalog.Entry{ID: *m.ID, Name: *m.Name, Description: m.Description, Visibility: *m.Visibility, Priority: *m.Priority})
	}
	if _, err := decoder.Token(); err != nil {
		report.fail(StageSchema, ResultInvalidSchema)
		if report != nil {
			report.SchemaValidated = diagnosticValue(false)
		}
		return nil, catalog.ErrInvalid
	}
	if report != nil {
		report.SchemaValidated = diagnosticValue(true)
	}
	if len(entries) == 0 {
		report.fail(StageMetadata, ResultEmptyCatalog)
		if report != nil {
			report.ModelCount = diagnosticValue(0)
			report.MetadataValidated = diagnosticValue(false)
		}
		return nil, catalog.ErrInvalid
	}
	// Metadata validity is independent of replay eligibility: native decoding
	// alone must not make a malformed catalog's diagnostic claim validation.
	if err := catalog.Validate(entries); err != nil {
		report.fail(StageMetadata, ResultInvalidMetadata)
		if report != nil {
			report.MetadataValidated = diagnosticValue(false)
		}
		return nil, err
	}
	if report != nil {
		report.MetadataValidated = diagnosticValue(true)
		report.ModelCount = diagnosticValue(len(entries))
	}
	return entries, nil
}
