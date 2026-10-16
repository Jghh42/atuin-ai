package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/config"
)

// Original validation cases based on the pinned schema contract, not copied
// provider/client fixtures. See testdata/compat/manifest.json for provenance.
var validArguments = map[string]string{
	"suggest_command":       `{"command":"ls -la","description":"List files","confidence":"high","danger":"low"}`,
	"read_file":             `{"file_path":"notes.txt"}`,
	"edit_file":             `{"file_path":"notes.txt","old_string":"old","new_string":"new"}`,
	"write_file":            `{"file_path":"notes.txt","content":"new"}`,
	"atuin_history":         `{"query":"","filter_modes":["workspace"]}`,
	"atuin_output":          `{"history_id":"h1"}`,
	"execute_shell_command": `{"command":"pwd","shell":"bash","description":"Get working directory"}`,
	"load_skill":            `{"name":"review"}`,
}

func call(name, input string) Call {
	return Call{ID: "call-1", Name: name, Input: json.RawMessage(input)}
}

func assertInvalid(t *testing.T, calls []Call, caps atuin.Capabilities) {
	t.Helper()
	before, _ := json.Marshal(calls)
	batch, err := ValidateBatch(calls, caps)
	if err != ErrInvalidBatch || batch.Kind != "" || len(batch.Calls) != 0 {
		t.Fatalf("got batch=%v err=%v", batch, err)
	}
	after, _ := json.Marshal(calls)
	if !bytes.Equal(before, after) {
		t.Fatal("input mutated on failure")
	}
}

func TestValidateCapabilities(t *testing.T) {
	for mask := 0; mask < 256; mask++ {
		caps := atuin.Capabilities{"client_invocations": mask&128 != 0, "client_v2_atuin_history": true, "web_search": true}
		for i, name := range expectedToolNames[1:] {
			caps["client_v1_"+name] = mask&(1<<i) != 0
		}
		for _, name := range expectedToolNames {
			c := call(name, validArguments[name])
			shouldAllow := name == "suggest_command" || caps.Has("client_v1_"+name)
			t.Run(fmt.Sprintf("%08b/%s", mask, name), func(t *testing.T) {
				if !shouldAllow {
					assertInvalid(t, []Call{c}, caps)
					return
				}
				batch, err := ValidateBatch([]Call{c}, caps)
				if err != nil || len(batch.Calls) != 1 || batch.Calls[0].Name != name {
					t.Fatalf("batch=%v err=%v", batch, err)
				}
				kind := LocalTools
				if name == "suggest_command" {
					kind = Suggestion
				}
				if batch.Kind != kind {
					t.Fatal(batch.Kind)
				}
			})
		}
	}
}

func TestValidateInputs(t *testing.T) {
	cases := []struct {
		name, input string
		valid       bool
	}{
		{"suggest_command", `{"command":"pwd","description":"dir","confidence":"med","danger":"high","confidence_notes":"","danger_notes":"note"}`, true},
		{"suggest_command", `{"command":"pwd","description":"dir","confidence":"medium","danger":"low"}`, false},
		{"suggest_command", `{"command":"pwd","description":"dir","confidence":"high","danger":"LOW"}`, false},
		{"suggest_command", `{"command":"pwd","description":"dir","confidence":"high","danger":"low","danger_notes":null}`, false},
		{"suggest_command", `{"command":"pwd","description":"dir","confidence":"high","danger":"low","confidence_notes":false}`, false},
		{"read_file", `{"file_path":"p","limit":1,"offset":-1}`, true},
		{"read_file", `{"file_path":"p","limit":1000,"offset":9223372036854775807}`, true},
		{"read_file", `{"file_path":"p","offset":-9223372036854775808}`, true},
		{"read_file", `{"file_path":"p","offset":9007199254740993}`, true},
		{"read_file", `{"file_path":"p","limit":0}`, false},
		{"read_file", `{"file_path":"p","limit":1001}`, false},
		{"read_file", `{"file_path":"p","limit":1.5}`, false},
		{"read_file", `{"file_path":"p","limit":1.0}`, false},
		{"read_file", `{"file_path":"p","limit":1e2}`, false},
		{"read_file", `{"file_path":"p","offset":9223372036854775808}`, true},
		{"read_file", `{"file_path":"p","offset":18446744073709551615}`, true},
		{"read_file", `{"file_path":"p","offset":18446744073709551616}`, false},
		{"read_file", `{"file_path":"p","offset":null}`, false},
		{"read_file", `{"file_path":"p","limit":"100"}`, false},
		{"edit_file", `{"file_path":"p","old_string":"","new_string":"","replace_all":true}`, true},
		{"edit_file", `{"file_path":"p","old_string":"a","new_string":"b","replace_all":null}`, false},
		{"write_file", `{"file_path":"p","content":"","overwrite":false}`, true},
		{"write_file", `{"file_path":"p","content":"x","overwrite":1}`, false},
		{"atuin_history", `{"query":"","filter_modes":[],"limit":1}`, true},
		{"atuin_history", `{"query":"q","filter_modes":["workspace","directory","session","host","global"],"limit":50}`, true},
		{"atuin_history", `{"query":"q","filter_modes":["local"]}`, false},
		{"atuin_history", `{"query":"q","filter_modes":[1]}`, false},
		{"atuin_history", `{"query":"q","filter_modes":null}`, false},
		{"atuin_history", `{"query":"q","filter_modes":[],"limit":0}`, false},
		{"atuin_history", `{"query":"q","filter_modes":[],"limit":51}`, false},
		{"atuin_output", `{"history_id":"h","ranges":[]}`, true},
		{"atuin_output", `{"history_id":"h","ranges":[[0,1000],[-200,-1]]}`, true},
		{"atuin_output", `{"history_id":"h","ranges":[[0]]}`, false},
		{"atuin_output", `{"history_id":"h","ranges":[[0,1,2]]}`, false},
		{"atuin_output", `{"history_id":"h","ranges":[[0,"1"]]}`, false},
		{"atuin_output", `{"history_id":"h","ranges":[null]}`, false},
		{"atuin_output", `{"history_id":"h","ranges":null}`, false},
		{"atuin_output", `{"history_id":"h","ranges":[` + strings.TrimSuffix(strings.Repeat(`[0,-1],`, 10), ",") + `]}`, true},
		{"atuin_output", `{"history_id":"h","ranges":[` + strings.TrimSuffix(strings.Repeat(`[0,-1],`, 11), ",") + `]}`, false},
		{"execute_shell_command", `{"command":"pwd","shell":"fish","description":"dir","timeout":1,"dir":"/tmp"}`, true},
		{"execute_shell_command", `{"command":"pwd","shell":"zsh","description":"dir","timeout":600}`, true},
		{"execute_shell_command", `{"command":"pwd","shell":"nu","description":"dir"}`, true},
		{"execute_shell_command", `{"command":"pwd","shell":"sh","description":"dir"}`, false},
		{"execute_shell_command", `{"command":"pwd","shell":"bash","description":"dir","timeout":0}`, false},
		{"execute_shell_command", `{"command":"pwd","shell":"bash","description":"dir","timeout":601}`, false},
		{"execute_shell_command", `{"command":"pwd","shell":"bash","description":"dir","timeout":true}`, false},
		{"load_skill", `{"name":"review","extra":{"n":9007199254740993}}`, true},
		{"load_skill", `{"name":false}`, false},
	}
	caps := allToolCapabilities()
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%s/%d", tc.name, i), func(t *testing.T) {
			c := call(tc.name, tc.input)
			if !tc.valid {
				assertInvalid(t, []Call{c}, caps)
				return
			}
			batch, err := ValidateBatch([]Call{c}, caps)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(batch.Calls[0].Input, c.Input) {
				t.Fatal("non-escalated input rewritten")
			}
		})
	}
}

func TestRequiredToolFields(t *testing.T) {
	required := map[string][]string{
		"suggest_command": {"command", "description", "confidence", "danger"},
		"read_file":       {"file_path"}, "edit_file": {"file_path", "old_string", "new_string"},
		"write_file": {"file_path", "content"}, "atuin_history": {"query", "filter_modes"},
		"atuin_output": {"history_id"}, "execute_shell_command": {"command", "shell", "description"}, "load_skill": {"name"},
	}
	for name, fields := range required {
		for _, field := range fields {
			t.Run(name+"/"+field, func(t *testing.T) {
				for _, replacement := range []json.RawMessage{nil, json.RawMessage(`null`)} {
					var input map[string]json.RawMessage
					if err := json.Unmarshal([]byte(validArguments[name]), &input); err != nil {
						t.Fatal(err)
					}
					if replacement == nil {
						delete(input, field)
					} else {
						input[field] = replacement
					}
					raw, err := json.Marshal(input)
					if err != nil {
						t.Fatal(err)
					}
					assertInvalid(t, []Call{call(name, string(raw))}, allToolCapabilities())
				}
			})
		}
	}
}

func TestAtomicToolBatches(t *testing.T) {
	caps := allToolCapabilities()
	local := call("read_file", validArguments["read_file"])
	suggest := call("suggest_command", validArguments["suggest_command"])
	suggest.ID = "s"
	second := call("load_skill", validArguments["load_skill"])
	second.ID = "second"
	for _, calls := range [][]Call{
		{local, suggest}, {suggest, local}, {suggest, {ID: "s2", Name: suggest.Name, Input: suggest.Input}},
		{local, local}, {local, {ID: "second", Name: "web_search", Input: json.RawMessage(`{}`)}},
		{local, {ID: "second", Name: "read_file", Input: json.RawMessage(`{}`)}},
		{{ID: "", Name: "read_file", Input: local.Input}}, {{ID: " \n", Name: "read_file", Input: local.Input}},
	} {
		assertInvalid(t, calls, caps)
	}
	for _, name := range []string{"web_search", "web_scrape", "mcp_exec", "unknown", "READ_FILE", " read_file"} {
		assertInvalid(t, []Call{call(name, `{}`)}, caps)
	}
	batch, err := ValidateBatch([]Call{local, second}, caps)
	if err != nil || batch.Kind != LocalTools || !reflect.DeepEqual(batch.Calls, []Call{local, second}) {
		t.Fatalf("%v %v", batch, err)
	}
	batch.Calls[0].Input[0] = '!'
	if !json.Valid(local.Input) {
		t.Fatal("output aliases caller input")
	}
	batch, err = ValidateBatch(nil, nil)
	if err != nil || batch.Kind != TextOnly || len(batch.Calls) != 0 {
		t.Fatal(batch, err)
	}
}

func TestToolJSONValidation(t *testing.T) {
	for _, raw := range []string{
		``, `null`, `[]`, `"x"`, `42`, `true`, `{"file_path":"p"} {}`, `{"file_path":"p",}`,
		"{\"file_path\":\"\xff\"}",
		`{"file_path":"p","file_path":"q"}`, `{"file_path":"p","file_\u0070ath":"q"}`,
		`{"file_path":"p","extra":{"x":1,"x":2}}`, `{"file_path":"p","extra":[{"x":1,"x":2}]}`,
		`{"file_path":"p","extra":` + strings.Repeat(`[`, atuin.MaxJSONDepth+1) + `0` + strings.Repeat(`]`, atuin.MaxJSONDepth+1) + `}`,
		`{"file_path":"p","extra":[` + strings.Repeat(`0,`, atuin.MaxJSONStructure) + `0]}`,
	} {
		t.Run(fmt.Sprintf("%.60s", raw), func(t *testing.T) { assertInvalid(t, []Call{call("read_file", raw)}, allToolCapabilities()) })
	}
}

// Go's JSON parser is more permissive than serde_json for these values. A
// rejected CLI event cannot be allowed to leave half of a parallel batch alive.
func TestClientJSONCompatibility(t *testing.T) {
	for _, raw := range []string{
		`{"file_path":"\ud800"}`,
		`{"file_path":"\udfff"}`,
		`{"file_path":"\ud800\u0041"}`,
		`{"file_path":"p","\ud800":1}`,
		`{"file_path":"p","extra":[{"name":"\udfff"}]}`,
		`{"file_path":"p","extra":1e400}`,
		`{"file_path":"p","extra":[{"n":-1E400}]}`,
		`{"file_path":"p","extra":` + strings.Repeat("9", 400) + `}`,
	} {
		bad := call("read_file", raw)
		bad.ID = "second"
		assertInvalid(t, []Call{call("read_file", validArguments["read_file"]), bad}, allToolCapabilities())
	}
	for _, raw := range []string{
		`{"file_path":"\ud83d\udc69"}`,
		`{"file_path":"\\ud800"}`,
		`{"file_path":"p","extra":{"n":1e308,"small":1e-400,"precise":9007199254740993}}`,
	} {
		batch, err := ValidateBatch([]Call{call("read_file", raw)}, allToolCapabilities())
		if err != nil || string(batch.Calls[0].Input) != raw {
			t.Fatalf("compatible JSON changed/rejected: %s %v", raw, err)
		}
	}
}

func TestToolResourceBounds(t *testing.T) {
	raw := []byte(validArguments["read_file"])
	atLimit := append(bytes.Clone(raw), bytes.Repeat([]byte(" "), config.MaxCallArgumentsBytes-len(raw))...)
	if _, err := ValidateBatch([]Call{call("read_file", string(atLimit))}, allToolCapabilities()); err != nil {
		t.Fatal(err)
	}
	assertInvalid(t, []Call{call("read_file", string(atLimit)+" ")}, allToolCapabilities())
	calls := make([]Call, config.MaxCallsPerResponse)
	for i := range calls {
		calls[i] = call("read_file", string(raw))
		calls[i].ID = fmt.Sprint(i)
	}
	if _, err := ValidateBatch(calls, allToolCapabilities()); err != nil {
		t.Fatal(err)
	}
	calls = append(calls, call("read_file", string(raw)))
	assertInvalid(t, calls, allToolCapabilities())
}

func TestSuggestionDangerBackstop(t *testing.T) {
	raw := `{"command":"rm -rf /","description":"Remove","confidence":"high","danger":"low","danger_notes":"Model note","other":{"n":9007199254740993}}`
	original := call("suggest_command", raw)
	batch, err := ValidateBatch([]Call{original}, nil)
	if err != nil || batch.Kind != Suggestion {
		t.Fatal(batch, err)
	}
	if string(original.Input) != raw {
		t.Fatal("original suggestion mutated")
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(batch.Calls[0].Input, &result); err != nil {
		t.Fatal(err)
	}
	if string(result["danger"]) != `"high"` || !bytes.Contains(result["danger_notes"], []byte("Model note")) || !bytes.Contains(result["other"], []byte("9007199254740993")) {
		t.Fatal(string(batch.Calls[0].Input))
	}
	again, err := ValidateBatch(batch.Calls, nil)
	if err != nil || !bytes.Equal(again.Calls[0].Input, batch.Calls[0].Input) {
		t.Fatal("non-idempotent backstop", err)
	}
	for _, risk := range []string{"low", "med", "high"} {
		input := strings.Replace(validArguments["suggest_command"], `"danger":"low"`, `"danger":"`+risk+`"`, 1)
		batch, err := ValidateBatch([]Call{call("suggest_command", input)}, nil)
		if err != nil || string(batch.Calls[0].Input) != input {
			t.Fatal("benign assessment changed")
		}
	}
	// Adding warning text may not break the argument cap. Fail closed rather
	// than silently dropping the warning or the model's notes.
	// Whitespace disappears on rewrite, so use a large actual notes value.
	fields := map[string]string{"command": "rm -rf /", "description": "x", "confidence": "high", "danger": "low", "danger_notes": ""}
	base, _ := json.Marshal(fields)
	fields["danger_notes"] = strings.Repeat("x", config.MaxCallArgumentsBytes-len(base))
	encoded, _ := json.Marshal(fields)
	if len(encoded) != config.MaxCallArgumentsBytes {
		t.Fatal("invalid test boundary")
	}
	assertInvalid(t, []Call{call("suggest_command", string(encoded))}, nil)
}

func FuzzValidateBatch(f *testing.F) {
	for name, input := range validArguments {
		f.Add(name, []byte(input))
	}
	f.Add("read_file", []byte(`{"file_path":"a","file_path":"b"}`))
	f.Add("read_file", []byte(`{"file_path":"\ud800"}`))
	f.Add("read_file", []byte(`{"file_path":"p","extra":1e400}`))
	f.Fuzz(func(t *testing.T, name string, input []byte) {
		calls := []Call{call(name, string(input))}
		batch, err := ValidateBatch(calls, allToolCapabilities())
		if err != nil {
			if batch.Kind != "" || len(batch.Calls) != 0 {
				t.Fatal("partial batch on error")
			}
			return
		}
		if len(batch.Calls) != 1 || !json.Valid(batch.Calls[0].Input) || len(batch.Calls[0].Input) > config.MaxCallArgumentsBytes {
			t.Fatal("invalid successful batch")
		}
		second, err := ValidateBatch(batch.Calls, allToolCapabilities())
		if err != nil || !reflect.DeepEqual(second, batch) {
			t.Fatal("non-idempotent normalization")
		}
	})
}
