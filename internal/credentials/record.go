package credentials

import (
	"errors"
	"fmt"
	"time"
)

const ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
const maxRecordBytes = 192 << 10

var (
	ErrUnavailable = errors.New("application login required")
	ErrReauth      = errors.New("application reauthentication required; stop server and log in again")
	ErrStore       = errors.New("credential state unavailable; check private directory and stop other application processes")
	// ErrLocked distinguishes genuine flock contention without changing public error text.
	ErrLocked  = fmt.Errorf("%w", ErrStore)
	ErrOAuth   = errors.New("codex authorization failed")
	ErrDenied  = errors.New("codex authorization denied")
	ErrExpired = errors.New("codex device authorization expired")
)

// Record is private application state, never a status or API response.
// Generation advances on every durable lifecycle transaction; AuthEpoch advances
// only on fresh login. An intent has no secrets and overrides the token record.
type Record struct {
	Schema       int       `json:"schema"`
	Provider     string    `json:"provider"`
	Registration string    `json:"registration"`
	State        string    `json:"state"`
	Generation   uint64    `json:"generation"`
	AuthEpoch    uint64    `json:"auth_epoch"`
	Account      string    `json:"account,omitempty"`
	Access       string    `json:"access,omitempty"`
	Refresh      string    `json:"refresh,omitempty"`
	IssuedAt     time.Time `json:"issued_at"`
	SavedAt      time.Time `json:"saved_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (Record) String() string   { return "[private credential record]" }
func (Record) GoString() string { return "[private credential record]" }
func emptyRecord() Record {
	return Record{Schema: 1, Provider: "codex", Registration: ClientID, State: "logged_out"}
}
func tombstone(old Record, state string, now time.Time) Record {
	r := emptyRecord()
	r.Generation = old.Generation
	r.AuthEpoch = old.AuthEpoch
	r.State = state
	r.SavedAt = now.UTC()
	return r
}
func validAtom(s string, max int) bool {
	if len(s) == 0 || len(s) > max {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}
func validSecret(s string) bool {
	if len(s) == 0 || len(s) > 65536 {
		return false
	}
	for _, c := range s {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func validTime(t time.Time) bool {
	return !t.IsZero() && t.Year() >= 2020 && t.Year() <= 9999 && t.Location() == time.UTC
}
func (r Record) validate() error {
	if r.Schema != 1 || r.Provider != "codex" || r.Registration != ClientID || r.Generation == 0 || r.AuthEpoch > r.Generation || !validTime(r.SavedAt) {
		return ErrStore
	}
	switch r.State {
	case "active":
		if r.AuthEpoch == 0 || !validAtom(r.Account, 256) || !validSecret(r.Access) || !validSecret(r.Refresh) || !validTime(r.IssuedAt) || !validTime(r.ExpiresAt) || r.SavedAt.Before(r.IssuedAt) || !r.ExpiresAt.After(r.IssuedAt) || r.ExpiresAt.Sub(r.IssuedAt) > 366*24*time.Hour {
			return ErrStore
		}
		account, exp, err := accessClaims(r.Access)
		if err != nil || account != r.Account || r.ExpiresAt.After(exp) {
			return ErrStore
		}
	case "logged_out", "unusable":
		if r.Account != "" || r.Access != "" || r.Refresh != "" || !r.IssuedAt.IsZero() || !r.ExpiresAt.IsZero() {
			return ErrStore
		}
	default:
		return ErrStore
	}
	return nil
}
