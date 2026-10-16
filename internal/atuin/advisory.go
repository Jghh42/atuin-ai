package atuin

import (
	"encoding/json"
	"strings"

	"github.com/rivo/uniseg"
)

// decodeAdvisory normalizes optional hints independently: malformed hints never
// invalidate a request. The main decoder is responsible for catalog validation.
func decodeAdvisory(root map[string]json.RawMessage, catalog ModelCatalog) (Context, Config) {
	ctx := advisoryObject(root["context"])
	context := Context{
		OS: advisoryString(ctx["os"]), Distro: advisoryString(ctx["distro"]),
		Shell: advisoryString(ctx["shell"]), PreferredLanguage: advisoryString(ctx["preferred_language"]),
		PWD: advisoryString(ctx["pwd"]), LastCommand: advisoryString(ctx["last_command"]),
	}
	nested := advisoryObject(root["config"])
	config := Config{Capabilities: make(Capabilities), RunPreference: "auto", ModelAlias: catalog.Default}
	for _, raw := range []json.RawMessage{root["capabilities"], nested["capabilities"]} {
		for _, item := range advisoryList(raw) {
			if value := advisoryString(item); value != nil {
				switch name := strings.TrimSpace(*value); name {
				case "client_v1_read_file", "client_v1_edit_file", "client_v1_write_file",
					"client_v1_execute_shell_command", "client_v1_atuin_history", "client_v1_atuin_output",
					"client_invocations", "client_v1_load_skill":
					config.Capabilities[name] = true
				}
			}
		}
	}
	if value := advisoryString(nested["run_preference"]); value != nil && (*value == "run" || *value == "suggest") {
		config.RunPreference = *value
	}
	if value := advisoryString(nested["model"]); value != nil {
		if _, ok := catalog.Models[*value]; ok {
			config.ModelAlias = *value
		}
	}
	config.ProviderModel = catalog.Models[config.ModelAlias]
	// PromptFn deliberately remains empty: custom prompt selection is disabled.
	for i, raw := range advisoryList(nested["user_contexts"]) {
		if i >= 10 {
			break
		}
		item := advisoryObject(raw)
		path, data := advisoryString(item["path"]), advisoryString(item["data"])
		if path != nil && data != nil {
			config.UserContexts = append(config.UserContexts, UserContext{
				Path: advisoryTruncate(*path, 150, "(truncated due to length)"),
				Data: advisoryTruncate(*data, 10_000, "(truncated due to length)"),
			})
		}
	}
	for i, raw := range advisoryList(nested["skills"]) {
		if i >= 50 {
			break
		}
		item := advisoryObject(raw)
		if name := advisoryString(item["name"]); name != nil {
			skill := Skill{Name: advisoryTruncate(*name, 64, "")}
			if description := advisoryString(item["description"]); description != nil {
				skill.Description = advisoryTruncate(*description, 1024, "")
			}
			config.Skills = append(config.Skills, skill)
		}
	}
	if overflow := advisoryString(nested["skills_overflow"]); overflow != nil {
		value := advisoryTruncate(*overflow, 500, "")
		config.SkillsOverflow = &value
	}
	return context, config
}

func advisoryObject(raw json.RawMessage) map[string]json.RawMessage {
	var value map[string]json.RawMessage
	_ = json.Unmarshal(raw, &value)
	return value
}

func advisoryList(raw json.RawMessage) []json.RawMessage {
	var value []json.RawMessage
	_ = json.Unmarshal(raw, &value)
	return value
}

func advisoryString(raw json.RawMessage) *string {
	// Unmarshalling null into a string succeeds; use a pointer to preserve None.
	var value *string
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return value
}

// Match Gleam string length/slice: limits count grapheme clusters, not bytes
// or code points. The suffix is appended outside the retained-prefix limit.
func advisoryTruncate(value string, limit int, suffix string) string {
	g := uniseg.NewGraphemes(value)
	count, end := 0, 0
	for g.Next() {
		if count == limit {
			return value[:end] + "... " + suffix
		}
		_, end = g.Positions()
		count++
	}
	return value
}
