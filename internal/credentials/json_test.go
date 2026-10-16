package credentials

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCredentialJSONRejectsGoUnicodeFoldAliases(t *testing.T) {
	for _, body := range []string{
		`{"access_token":"first","acce\u017fs_token":"second"}`,
		`{"access_token":"first","access_to\u212aen":"second"}`,
		`{"access_token":"first","ACCESS_TOKEN":"second"}`,
		`{"outer":{"access_token":"first","acce\u017fs_token":"second"}}`,
		`{"outer":[{"refresh_token":"first","refresh_to\u212aen":"second"}]}`,
		`{"https://api.openai.com/auth":{"chatgpt_account_id":"first","CHATGPT_ACCOUNT_ID":"second"}}`,
		`{"error":{"code":"access_denied","CODE":"pending"}}`,
		`{"\u03a3":1,"\u03c2":2}`,
	} {
		var token tokenResponse
		if e := decodeJSON([]byte(body), &token, false); e == nil {
			t.Fatal("Unicode-fold alias was accepted")
		}
		if token.Access != "" || token.Refresh != "" {
			t.Fatal("ambiguous input modified output before rejection")
		}
	}
	// Match SimpleFold, not locale/full folding that would merge distinct names.
	for _, pair := range [][2]string{{"access_token", "acceſs_token"}, {"token", "toKen"}, {"Σ", "ς"}, {"Σ", "σ"}, {"Å", "å"}, {"ss", "ß"}, {"i", "İ"}} {
		if (foldedJSONName(pair[0]) == foldedJSONName(pair[1])) != strings.EqualFold(pair[0], pair[1]) {
			t.Fatal("canonicalization diverged from Go Unicode folding")
		}
	}
}
func TestCredentialJSONCanonicalAdditionsAndNumbersPreserved(t *testing.T) {
	body := []byte(`{"access_token":"synthetic","refresh_token":"replacement","expires_in":3600,"additive":{"ss":1,"ß":2,"i":3,"İ":4,"number":9007199254740993,"decimal":1.2300e+04,"negative_zero":-0,"nested":[{"name":1},{"name":2}]}}`)
	var token tokenResponse
	if e := decodeJSON(body, &token, false); e != nil || token.Access != "synthetic" || string(token.Expires) != "3600" {
		t.Fatal("canonical additive response rejected", e)
	}
	var v map[string]any
	if e := decodeJSON(body, &v, false); e != nil {
		t.Fatal(e)
	}
	additive, ok := v["additive"].(map[string]any)
	if !ok {
		t.Fatal("additive object not decoded")
	}
	for name, want := range map[string]string{"number": "9007199254740993", "decimal": "1.2300e+04", "negative_zero": "-0"} {
		number, ok := additive[name].(json.Number)
		if !ok || number.String() != want {
			t.Fatal("numeric lexeme changed")
		}
	}
}
func TestPersistedJSONExactFieldsSurviveFoldHardening(t *testing.T) {
	r := activeRecord(testNow.Add(time.Hour))
	body, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	var loaded Record
	if e := decodeJSON(body, &loaded, true); e != nil || loaded != r {
		t.Fatal("canonical record changed", e)
	}
	for _, alias := range []string{`Schema`, `\u017fchema`, `SCHEMA`} {
		bad := strings.Replace(string(body), `"schema"`, `"`+alias+`"`, 1)
		if e := decodeJSON([]byte(bad), &loaded, true); e == nil {
			t.Fatal("persisted schema accepted non-exact name")
		}
	}
}
func TestCredentialJSONRejectsAmbiguityUnicodeAndDepth(t *testing.T) {
	for _, b := range [][]byte{
		[]byte(`{"a":1,"a":2}`), []byte(`{"access_token":"one","ACCESS_TOKEN":"two"}`), []byte(`{"a":{"b":1,"b":2}}`), []byte(`{"a":"\ud800"}`), []byte("{\"a\":\"\xff\"}"), []byte(strings.Repeat("[", 18) + strings.Repeat("]", 18)), []byte(`{} {}`),
	} {
		var v any
		if e := decodeJSON(b, &v, false); e == nil {
			t.Fatal("ambiguous/invalid JSON accepted")
		}
	}
	var v map[string]json.RawMessage
	if e := decodeJSON([]byte(`{"additive":{"field":[1,true,null]}}`), &v, false); e != nil {
		t.Fatal("valid additive JSON rejected", e)
	}
}
