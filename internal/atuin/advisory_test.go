package atuin

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Independently recreated from atuinsh/atuin-ai-core (Apache-2.0), commit
// 565877e97ed3ff4840481b8d47e12eb2f9854039,
// test/atuin_ai_core/domain/config_test.gleam; see NOTICE and fixture manifest:
// empty_params_gives_default_test, non_map_config_gives_default_test,
// parses_nested_fields_test, invalid_field_values_fall_back_test,
// rejects_raw_provider_model_ids_test, unions_legacy_and_nested_capabilities_test,
// user_contexts_truncate_and_skip_invalid_test, user_contexts_take_ten_before_validity_test,
// skills_name_only_gets_empty_description_test, skills_overflow_truncates_test,
// without_llm_selection_clears_prompt_fn_test.
// Context cases follow http/controller.gleam context_decoder/lenient_context_field;
// the pinned source has no dedicated context-decoder tests.
var advisoryTestCatalog = ModelCatalog{Default: "max", Models: map[string]string{
	"max": "provider:large", "fast": "provider:small",
}}

func decodeAdvisoryTest(t *testing.T, value any) (Context, Config) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	return decodeAdvisory(root, advisoryTestCatalog)
}

func TestAdvisoryDefaults(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"config":null}`, `{"config":42}`, `{"config":[]}`, `{"config":"bad"}`} {
		t.Run(raw, func(t *testing.T) {
			var root map[string]json.RawMessage
			if err := json.Unmarshal([]byte(raw), &root); err != nil {
				t.Fatal(err)
			}
			ctx, got := decodeAdvisory(root, advisoryTestCatalog)
			want := Config{Capabilities: Capabilities{}, RunPreference: "auto", ModelAlias: "max", ProviderModel: "provider:large"}
			if !reflect.DeepEqual(ctx, Context{}) || !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v, %#v", ctx, got)
			}
		})
	}
}

func TestAdvisoryContext(t *testing.T) {
	for _, value := range []any{nil, 42, false, "bad", []any{}, map[string]any{}} {
		ctx, _ := decodeAdvisoryTest(t, map[string]any{"context": value})
		if !reflect.DeepEqual(ctx, Context{}) {
			t.Fatalf("%#v: %#v", value, ctx)
		}
	}
	fields := []string{"os", "distro", "shell", "preferred_language", "pwd", "last_command"}
	for _, field := range fields {
		for _, value := range []any{nil, 42, false, []any{}, map[string]any{}, "", "日本語", "  unchanged\n"} {
			t.Run(field+"/"+stringMustJSON(t, value), func(t *testing.T) {
				ctx, _ := decodeAdvisoryTest(t, map[string]any{"context": map[string]any{field: value, "installed_commands": true}})
				values := []*string{ctx.OS, ctx.Distro, ctx.Shell, ctx.PreferredLanguage, ctx.PWD, ctx.LastCommand}
				for i, key := range fields {
					got := values[i]
					if text, ok := value.(string); ok && key == field {
						if got == nil || *got != text {
							t.Fatalf("%s: %v", key, got)
						}
					} else if got != nil {
						t.Fatalf("unexpected %s: %v", key, got)
					}
				}
			})
		}
	}
}

func stringMustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAdvisoryIndependentConfigFields(t *testing.T) {
	for _, field := range []string{"capabilities", "run_preference", "model", "user_contexts", "skills", "skills_overflow"} {
		for _, value := range []any{nil, true, 7, map[string]any{}} {
			t.Run(field+"/"+stringMustJSON(t, value), func(t *testing.T) {
				nested := map[string]any{"run_preference": "suggest", "model": "fast", "capabilities": []any{"client_v1_read_file"}, "skills_overflow": "", "prompt_fn": "concise"}
				nested[field] = value
				_, got := decodeAdvisoryTest(t, map[string]any{"config": nested})
				pref, alias, provider := "suggest", "fast", "provider:small"
				if field == "run_preference" {
					pref = "auto"
				}
				if field == "model" {
					alias, provider = "max", "provider:large"
				}
				if got.RunPreference != pref || got.ModelAlias != alias || got.ProviderModel != provider || got.PromptFn != "" {
					t.Fatalf("%#v", got)
				}
				if got.Capabilities.Has("client_v1_read_file") != (field != "capabilities") {
					t.Fatalf("caps: %v", got.Capabilities)
				}
				if (got.SkillsOverflow == nil) != (field == "skills_overflow") {
					t.Fatalf("overflow: %v", got.SkillsOverflow)
				}
			})
		}
	}
}

func TestAdvisorySelection(t *testing.T) {
	for _, tc := range []struct{ value, want string }{{"auto", "auto"}, {"suggest", "suggest"}, {"run", "run"}, {"RUN", "auto"}, {" run ", "auto"}, {"unknown", "auto"}, {"", "auto"}} {
		_, got := decodeAdvisoryTest(t, map[string]any{"config": map[string]any{"run_preference": tc.value}})
		if got.RunPreference != tc.want {
			t.Errorf("%q: %q", tc.value, got.RunPreference)
		}
	}
	for _, alias := range []string{"max", "fast", "provider:small", "openai/gpt-4o", "unknown", " fast ", ""} {
		_, got := decodeAdvisoryTest(t, map[string]any{"config": map[string]any{"model": alias, "prompt_fn": "default"}})
		want := alias
		if _, ok := advisoryTestCatalog.Models[want]; !ok {
			want = "max"
		}
		if got.ModelAlias != want || got.ProviderModel != advisoryTestCatalog.Models[want] || got.PromptFn != "" {
			t.Fatalf("%q: %#v", alias, got)
		}
	}
	// No catalog validation here, even if deployment configuration is broken.
	_, got := decodeAdvisory(nil, ModelCatalog{Default: "missing"})
	if got.ModelAlias != "missing" || got.ProviderModel != "" {
		t.Fatalf("%#v", got)
	}
}

func TestAdvisoryCapabilities(t *testing.T) {
	names := []string{"client_v1_read_file", "client_v1_edit_file", "client_v1_write_file", "client_v1_execute_shell_command", "client_v1_atuin_history", "client_v1_atuin_output", "client_invocations", "client_v1_load_skill"}
	nested := []any{nil, 7, true, map[string]any{}, "unknown", "client_v1_invocations", "CLIENT_V1_READ_FILE"}
	for _, name := range names {
		nested = append(nested, " \t"+name+"\n", name)
	}
	for _, legacy := range []any{nil, "bad", []any{"client_v1_read_file", 3}} {
		_, got := decodeAdvisoryTest(t, map[string]any{"capabilities": legacy, "config": map[string]any{"capabilities": nested}})
		if len(got.Capabilities) != len(names) {
			t.Fatalf("%v", got.Capabilities)
		}
		for _, name := range names {
			if !got.Capabilities.Has(name) || got.Capabilities.Has(" "+name) {
				t.Fatalf("missing/extraneous %q", name)
			}
		}
	}
	_, got := decodeAdvisoryTest(t, map[string]any{"capabilities": []string{"client_v1_edit_file"}, "config": map[string]any{"capabilities": []string{"client_v1_read_file"}}})
	if len(got.Capabilities) != 2 {
		t.Fatalf("union: %v", got.Capabilities)
	}
}

func TestAdvisoryLists(t *testing.T) {
	_, got := decodeAdvisoryTest(t, map[string]any{"config": map[string]any{
		"user_contexts": []any{nil, true, "bad", map[string]any{"path": 1, "data": "d"}, map[string]any{"path": "p", "data": nil}, map[string]any{"path": "", "data": ""}, map[string]any{"path": "p", "data": "d"}},
		"skills":        []any{nil, 2, map[string]any{"description": "no name"}, map[string]any{"name": nil}, map[string]any{"name": "deploy", "description": "ship"}, map[string]any{"name": "bare"}, map[string]any{"name": "wrong", "description": 42}, map[string]any{"name": "", "description": nil}},
	}})
	if !reflect.DeepEqual(got.UserContexts, []UserContext{{"", ""}, {"p", "d"}}) {
		t.Fatalf("%#v", got.UserContexts)
	}
	if !reflect.DeepEqual(got.Skills, []Skill{{"deploy", "ship"}, {"bare", ""}, {"wrong", ""}, {"", ""}}) {
		t.Fatalf("%#v", got.Skills)
	}
	for _, tc := range []struct {
		key   string
		limit int
		valid any
	}{
		{"user_contexts", 10, map[string]any{"path": "p", "data": "d"}},
		{"skills", 50, map[string]any{"name": "s"}},
	} {
		for _, invalid := range []bool{false, true} {
			entries := make([]any, tc.limit+1)
			for i := range entries {
				entries[i] = tc.valid
				if invalid && i < tc.limit {
					entries[i] = nil
				}
			}
			_, got := decodeAdvisoryTest(t, map[string]any{"config": map[string]any{tc.key: entries}})
			count := len(got.UserContexts)
			if tc.key == "skills" {
				count = len(got.Skills)
			}
			want := tc.limit
			if invalid {
				want = 0
			}
			if count != want {
				t.Fatalf("%s invalid=%v: %d", tc.key, invalid, count)
			}
		}
	}
}

func TestAdvisoryTruncation(t *testing.T) {
	for _, char := range []string{"x", "界", "🙂", "e\u0301", "👩‍💻"} {
		for _, delta := range []int{-1, 0, 1} {
			text := func(limit int) string { return strings.Repeat(char, limit+delta) }
			want := func(limit int, suffix string) string {
				if delta > 0 {
					return strings.Repeat(char, limit) + "... " + suffix
				}
				return text(limit)
			}
			_, got := decodeAdvisoryTest(t, map[string]any{"config": map[string]any{
				"user_contexts": []any{map[string]any{"path": text(150), "data": text(10000)}},
				"skills":        []any{map[string]any{"name": text(64), "description": text(1024)}}, "skills_overflow": text(500),
			}})
			if got.UserContexts[0].Path != want(150, "(truncated due to length)") || got.UserContexts[0].Data != want(10000, "(truncated due to length)") || got.Skills[0].Name != want(64, "") || got.Skills[0].Description != want(1024, "") || got.SkillsOverflow == nil || *got.SkillsOverflow != want(500, "") {
				t.Fatalf("char=%q delta=%d: truncation mismatch", char, delta)
			}
		}
	}
	// A combining mark must stay with its base character.
	if got := advisoryTruncate("e\u0301", 1, ""); got != "e\u0301" {
		t.Fatalf("%q", got)
	}
}
