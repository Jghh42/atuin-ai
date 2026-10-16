package credentials

import (
	"fmt"
	"io"
	"log/slog"
)

// Formatter takes precedence over String/GoString, including unsupported verbs
// whose default fmt diagnostics could otherwise recursively expose fields.
// Ignore flags/width/precision/verb and write ONLY the fixed redaction label.
// This deliberately does not alter encoding/json credential-store serialization.
func writeRedacted(s fmt.State, label string)            { _, _ = io.WriteString(s, label) }
func (v Identity) Format(s fmt.State, _ rune)            { writeRedacted(s, v.String()) }
func (v Credential) Format(s fmt.State, _ rune)          { writeRedacted(s, v.String()) }
func (v Record) Format(s fmt.State, _ rune)              { writeRedacted(s, v.String()) }
func (v DevicePrompt) Format(s fmt.State, _ rune)        { writeRedacted(s, v.String()) }
func (v *Manager) Format(s fmt.State, _ rune)            { writeRedacted(s, v.String()) }
func (v *DiskStore) Format(s fmt.State, _ rune)          { writeRedacted(s, v.String()) }
func (v deviceAuthorization) Format(s fmt.State, _ rune) { writeRedacted(s, v.String()) }
func (v authorizationCode) Format(s fmt.State, _ rune)   { writeRedacted(s, v.String()) }
func (v tokenResponse) Format(s fmt.State, _ rune)       { writeRedacted(s, v.String()) }
func (v accessMetadata) Format(s fmt.State, _ rune)      { writeRedacted(s, v.String()) }

// JSON slog handlers do not necessarily honor fmt.Stringer. Explicit log
// values protect secret-bearing structures in both text and JSON logging.
func (Identity) LogValue() slog.Value     { return slog.StringValue("[private credential identity]") }
func (Credential) LogValue() slog.Value   { return slog.StringValue("[private credential]") }
func (Record) LogValue() slog.Value       { return slog.StringValue("[private credential record]") }
func (DevicePrompt) LogValue() slog.Value { return slog.StringValue("[private device prompt]") }
func (*Manager) LogValue() slog.Value     { return slog.StringValue("[private credential manager]") }
func (*DiskStore) LogValue() slog.Value   { return slog.StringValue("[private credential store]") }
func (deviceAuthorization) LogValue() slog.Value {
	return slog.StringValue("[private device authorization]")
}
func (authorizationCode) LogValue() slog.Value {
	return slog.StringValue("[private authorization code]")
}
func (tokenResponse) LogValue() slog.Value  { return slog.StringValue("[private token response]") }
func (accessMetadata) LogValue() slog.Value { return slog.StringValue("[private access metadata]") }
