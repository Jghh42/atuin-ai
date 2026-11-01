package continuation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

func testKeys() *keys {
	k := ownedPointer(newKeys(keysPayload{}))
	for j := range k.root {
		k.root[j] = byte(j + 1)
	}
	copy(k.id[:], "key-id01")
	return k
}
func textMessage(role, text string) atuin.Message {
	return atuin.Message{Role: role, Content: []atuin.Block{{Type: "text", Text: text}}}
}
func TestCanonicalBoundariesAndNumbers(t *testing.T) {
	k := testKeys()
	a, _ := k.prefix([]atuin.Message{textMessage("user", "ab"), textMessage("user", "c")})
	b, _ := k.prefix([]atuin.Message{textMessage("user", "a"), textMessage("user", "bc")})
	if equal(a.digest, b.digest) {
		t.Fatal("user boundaries collapsed")
	}
	c, _ := k.prefix([]atuin.Message{textMessage("user", "abc")})
	if equal(a.digest, c.digest) {
		t.Fatal("messages merged")
	}
	for _, n := range []string{"0", "1", "-1", "-9223372036854775808", "18446744073709551615"} {
		if _, e := CanonicalArguments([]byte(`{"nested":[{"n":`+n+`}],"empty":null}`), true); e != nil {
			t.Fatal(n, e)
		}
	}
	for _, n := range []string{"-0", "1.0", "1e0", "1E+0", "-9223372036854775809", "18446744073709551616"} {
		raw := []byte(`{"nested":[{"n":` + n + `}]}`)
		if _, e := CanonicalArguments(raw, true); e == nil {
			t.Fatal("protected number", n)
		}
		if _, e := CanonicalArguments(raw, false); e != nil {
			t.Fatal("legacy lexeme changed", n)
		}
	}
	x, _ := CanonicalArguments([]byte(`{"z":[0,"\u0061"],"a":1}`), true)
	y, _ := CanonicalArguments([]byte(`{ "a":1,"z":[0,"a"]}`), true)
	if !bytes.Equal(x, y) {
		t.Fatal("canonical object/string mismatch")
	}
	for _, raw := range []string{`{"a":1,"a":2}`, `{"a":"\ud800"}`, `[] true`, strings.Repeat("[", 66) + "0" + strings.Repeat("]", 66)} {
		if _, e := CanonicalArguments([]byte(raw), true); e == nil {
			t.Fatal("malformed/depth accepted")
		}
	}
}
func TestRustLeadingAndSpans(t *testing.T) {
	k := testKeys()
	whitespace := []rune{9, 10, 11, 12, 13, 0x20, 0x85, 0xa0, 0x1680, 0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007, 0x2008, 0x2009, 0x200a, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000}
	for _, r := range whitespace {
		if !RustWhiteSpace(r) {
			t.Fatal("missing pinned whitespace")
		}
	}
	for _, r := range []rune{0x200b, 0xfeff, 'x', 0} {
		if RustWhiteSpace(r) {
			t.Fatal("over-trim")
		}
	}
	leading := string(whitespace)
	parts := []string{leading[:5], leading[5:] + "hello", " \n"}
	capture := NewTextCapture()
	defer capture.Close()
	for _, s := range []string{leading[:5], leading[5:], "hel", "lo ", "\n"} {
		if e := capture.Add(s); e != nil {
			t.Fatal(e)
		}
	}
	expected, recipe, e := capture.Finish(k, parts)
	if e != nil {
		t.Fatal(e)
	}
	client := "hello \n"
	want, e := k.text(client)
	if e != nil {
		t.Fatal(e)
	}
	if expected.bytes != want.bytes || !equal(expected.digest, want.digest) {
		t.Fatal("trim mismatch")
	}
	got, e := recipe.Restore(client)
	if e != nil {
		t.Fatal(e)
	}
	for j, p := range got {
		if string(p) != parts[j] {
			t.Fatal("part/trailing bytes lost")
		}
	}
	if _, e = recipe.Restore("hello"); e == nil {
		t.Fatal("trailing edit accepted")
	}
	for _, s := range []string{"\u200b ", "\ufeff "} {
		c := NewTextCapture()
		_ = c.Add(s)
		ex, r, e := c.Finish(k, []string{s})
		if e != nil || ex.bytes != uint32(len(s)) || len(r.leading) != 0 {
			t.Fatal("non-whitespace trimmed")
		}
		c.Close()
	}
	overflow := NewTextCapture()
	_ = overflow.Add(strings.Repeat(" ", MaxLeadingBytes))
	_ = overflow.Add(" ")
	_ = overflow.Add("x")
	if len(overflow.leading) > MaxLeadingBytes {
		t.Fatal("unbounded capture")
	}
	if _, _, e := overflow.Finish(k, nil); e == nil {
		t.Fatal("overflow supported")
	}
	overflow.Close()
	mismatch := NewTextCapture()
	_ = mismatch.Add("ab")
	if _, _, e := mismatch.Finish(k, []string{"ba"}); e == nil {
		t.Fatal("provider order mismatch")
	}
}
func TestScopeAndPolicyBinding(t *testing.T) {
	k := testKeys()
	base := atuin.Request{Messages: []atuin.Message{textMessage("user", "private")}, Config: atuin.Config{Capabilities: atuin.Capabilities{"read": true}, RunPreference: "auto", UserContexts: []atuin.UserContext{{Path: "path", Data: "data"}}, Skills: []atuin.Skill{{Name: "skill", Description: "description"}}}}
	original, e := k.policy(base, []byte("rendered"), []byte("definitions"))
	if e != nil {
		t.Fatal(e)
	}
	changed := func(r atuin.Request, rendered, defs string) {
		p, e := k.policy(r, []byte(rendered), []byte(defs))
		if e != nil || equal(original, p) {
			t.Fatal("stable policy change not bound")
		}
	}
	changed(base, "other", "definitions")
	changed(base, "rendered", "other")
	r := base
	r.Config.RunPreference = "changed"
	changed(r, "rendered", "definitions")
	r = base
	r.Config.Capabilities = atuin.Capabilities{"read": false}
	changed(r, "rendered", "definitions")
	empty := ""
	for _, set := range []func(*atuin.Request){func(r *atuin.Request) { r.Context.OS = &empty }, func(r *atuin.Request) { r.Context.Distro = &empty }, func(r *atuin.Request) { r.Context.Shell = &empty }, func(r *atuin.Request) { r.Context.PreferredLanguage = &empty }, func(r *atuin.Request) { r.Config.SkillsOverflow = &empty }, func(r *atuin.Request) { r.Config.UserContexts = nil }, func(r *atuin.Request) { r.Config.Skills = nil }} {
		r = base
		set(&r)
		changed(r, "rendered", "definitions")
	}
	r = base
	r.Context.PWD = &empty
	r.Context.LastCommand = &empty
	r.InvocationID = &empty
	p, _ := k.policy(r, []byte("rendered"), []byte("definitions"))
	if !equal(original, p) {
		t.Fatal("volatile fields bound")
	}
	baseIdentity := credentials.Identity{Provider: "codex", Account: "a", Epoch: 1}
	freshInput := func() ScopeInput { return NewScopeInput(baseIdentity, "m", "s", original) }
	in := freshInput()
	scope := k.scope(in)
	assertBaseline := func(t *testing.T) {
		t.Helper()
		if in.identity != baseIdentity || in.model != "m" || in.session != "s" || !equal(in.policy, original) || !equal(scope, k.scope(in)) {
			t.Fatal("scope test mutated its baseline through an opaque alias")
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*ScopeInput)
	}{
		{"provider", func(i *ScopeInput) { i.identity.Provider = "p" }},
		{"account", func(i *ScopeInput) { i.identity.Account = "b" }},
		{"epoch", func(i *ScopeInput) { i.identity.Epoch++ }},
		{"model", func(i *ScopeInput) { i.model = "n" }},
		{"session", func(i *ScopeInput) { i.session = "t" }},
		{"policy", func(i *ScopeInput) { i.policy = newCommitment(commitmentPayload{}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertBaseline(t)
			// Opaque value copies share their immutable payload. This same-package
			// mutation test needs fresh owned construction, not altered:=in.
			altered := freshInput()
			if !equal(scope, k.scope(altered)) {
				t.Fatal("fresh row did not start from baseline")
			}
			tc.change(&altered)
			if equal(scope, k.scope(altered)) {
				t.Fatal("individual scope dimension unbound")
			}
			assertBaseline(t)
		})
	}
}
func TestOpaqueHandlesAndPrivatePresentations(t *testing.T) {
	k := testKeys()
	scope := keyed(k.derive("binding"), "scope", "private-marker")
	id := newSnapshotID(snapshotIDPayload{})
	id.bytes[0] = 1
	h, e := k.handle(id, 31, scope)
	if e != nil || len(h.Value()) != 90 {
		t.Fatal("handle format")
	}
	if _, e = k.verify(h, scope); e != nil {
		t.Fatal(e)
	}
	for j := range h.value {
		b := []byte(h.value)
		b[j] = '!'
		if _, e := k.verify(newPublicHandle(publicHandlePayload{value: string(b)}), scope); e == nil {
			t.Fatal("tamper accepted")
		}
	}
	for _, s := range []string{"aic1_", "aic1_bad", strings.ToUpper(h.value), h.value + "x"} {
		if _, e := parseHandle(s); e == nil {
			t.Fatal("malformed handle")
		}
	}
	marker := "PRIVATE-MARKER-DO-NOT-PRINT"
	values := []any{newBackendCallID(backendCallIDPayload{value: marker}), newPublicHandle(publicHandlePayload{value: marker}), newItem(itemPayload{iD: marker, arguments: marker, encrypted: marker}), newRecord(recordPayload{segments: []Segment{newSegment(segmentPayload{calls: []Call{newCall(callPayload{name: marker})}, items: []Item{newItem(itemPayload{encrypted: marker})}})}}), []Item{newItem(itemPayload{iD: marker})}, map[string]any{"payload": newItem(itemPayload{iD: marker})}, newError(errorPayload{code: 255})}
	for _, v := range values {
		for _, verb := range []string{"%v", "%#v", "%+v", "%s", "%q", "%x", "%d", "%w"} {
			s := fmt.Sprintf(verb, v)
			if strings.Contains(s, marker) {
				t.Fatalf("fmt leak: verb=%s type=%T", verb, v)
			}
		}
		b, e := json.Marshal(v)
		if e != nil || bytes.Contains(b, []byte(marker)) {
			t.Fatal("JSON leak")
		}
		var buf bytes.Buffer
		slog.New(slog.NewJSONHandler(&buf, nil)).Info("safe", "value", v)
		if strings.Contains(buf.String(), marker) {
			t.Fatal("slog leak")
		}
	}
}
func FuzzHandle(f *testing.F) {
	f.Add("aic1_")
	k := testKeys()
	h, _ := k.handle(newSnapshotID(snapshotIDPayload{}), 0, newCommitment(commitmentPayload{}))
	f.Add(h.value)
	f.Fuzz(func(t *testing.T, s string) {
		p, e := parseHandle(s)
		if e == nil {
			if len(s) != 90 || p.ordinal >= 32 {
				t.Fatal("invalid grammar")
			}
			_, _ = k.verify(newPublicHandle(publicHandlePayload{value: s}), newCommitment(commitmentPayload{}))
		}
	})
}
