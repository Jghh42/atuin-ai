package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/atuin"
)

// Independent expectations from core 565877e97ed3ff4840481b8d47e12eb2f9854039:
// src/atuin_ai_core/domain/tools.gleam client_tools(), and the capability table
// in docs/specs/atuin-ai-server-api.md. Do not derive these from registry.
var expectedToolNames = []string{
	"suggest_command", "read_file", "edit_file", "write_file", "atuin_history",
	"atuin_output", "execute_shell_command", "load_skill",
}

func TestDefinitionsCapabilityMatrix(t *testing.T) {
	for mask := 0; mask < 256; mask++ {
		t.Run(fmt.Sprintf("%08b", mask), func(t *testing.T) {
			caps := atuin.Capabilities{}
			want := []string{"suggest_command"}
			for bit, name := range expectedToolNames[1:] {
				enabled := mask&(1<<bit) != 0
				caps["client_v1_"+name] = enabled
				if enabled {
					want = append(want, name)
				}
			}
			caps["client_invocations"] = mask&128 != 0
			// None of these markers may influence even an otherwise empty registry.
			caps["unknown"] = true
			caps["web_search"] = true
			caps["web_scrape"] = true
			for _, name := range expectedToolNames[1:] {
				caps["client_v2_"+name] = true
				caps[name] = true
				caps[" client_v1_"+name] = true
				caps["client_v1_"+name+" "] = true
				caps[strings.ToUpper("client_v1_"+name)] = true
			}
			for repeat := 0; repeat < 3; repeat++ {
				defs := Definitions(caps)
				got := make([]string, len(defs))
				for i, d := range defs {
					got[i] = d.Name
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("names = %v; want %v", got, want)
				}
			}
		})
	}
	if got := Definitions(nil); len(got) != 1 || got[0].Name != "suggest_command" {
		t.Fatalf("nil capabilities: %v", got)
	}
}

// Each line snapshots one capability mask (seven tool bits plus invocations).
// Combined with per-definition goldens, this pins every complete registry shape.
func TestCapabilityMatrixGolden(t *testing.T) {
	var snapshot strings.Builder
	for mask := 0; mask < 256; mask++ {
		caps := atuin.Capabilities{"client_invocations": mask&128 != 0}
		for bit, name := range expectedToolNames[1:] {
			caps["client_v1_"+name] = mask&(1<<bit) != 0
		}
		names := make([]string, 0, 8)
		for _, definition := range Definitions(caps) {
			names = append(names, definition.Name)
		}
		fmt.Fprintf(&snapshot, "%08b %s\n", mask, strings.Join(names, ","))
	}
	path := filepath.Join("..", "..", "testdata", "tools", "capabilities.golden")
	if os.Getenv("UPDATE_TOOL_GOLDENS") == "1" {
		if err := os.WriteFile(path, []byte(snapshot.String()), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.String() != string(want) {
		t.Fatalf("capability matrix differs from %s", path)
	}
}

func allToolCapabilities() atuin.Capabilities {
	caps := atuin.Capabilities{}
	for _, name := range expectedToolNames[1:] {
		caps["client_v1_"+name] = true
	}
	return caps
}

// Snapshots pin the complete model-facing definitions (including all eight
// schemas), not a provider projection. To deliberately refresh original prose:
// UPDATE_TOOL_GOLDENS=1 go test ./internal/tools -run TestDefinitionGoldens
func TestDefinitionGoldens(t *testing.T) {
	for _, d := range Definitions(allToolCapabilities()) {
		t.Run(d.Name, func(t *testing.T) {
			got, err := json.MarshalIndent(d, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join("..", "..", "testdata", "tools", d.Name+".json")
			if os.Getenv("UPDATE_TOOL_GOLDENS") == "1" {
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("definition differs from %s\ngot:\n%s\nwant:\n%s", path, got, want)
			}
		})
	}
}

func TestDefinitionsMutationIndependent(t *testing.T) {
	caps := allToolCapabilities()
	pristine := Definitions(caps)
	first := Definitions(caps)
	second := Definitions(caps)
	for i := range first {
		first[i].Name = "mutated"
		first[i].Description = "mutated"
		for j := range first[i].Parameters {
			first[i].Parameters[j] = 'x'
		}
		// Mutating one schema must not affect another in the same returned slice.
		for j := i + 1; j < len(first); j++ {
			if !bytes.Equal(first[j].Parameters, pristine[j].Parameters) {
				t.Fatalf("schema %d aliases schema %d", i, j)
			}
		}
	}
	first = append(first, Definition{Name: "extra"})
	if first[len(first)-1].Name != "extra" {
		t.Fatal("append failed")
	}
	if !reflect.DeepEqual(second, pristine) {
		t.Fatal("mutating one result affected an existing result")
	}
	if got := Definitions(caps); !reflect.DeepEqual(got, pristine) {
		t.Fatal("mutating returned data affected future results")
	}
}

// json_schema.gleam object() omits additionalProperties. Optional fields remain
// optional, not nullable; defaults are prose rather than validation keywords.
// This pins the narrow vocabulary consumed by the separate call validator.
func TestDefinitionSchemaVocabulary(t *testing.T) {
	var walk func(*testing.T, map[string]json.RawMessage)
	walk = func(t *testing.T, node map[string]json.RawMessage) {
		t.Helper()
		for key, value := range node {
			switch key {
			case "type", "description", "required", "enum", "minimum", "maximum", "minItems", "maxItems":
			case "properties":
				var props map[string]map[string]json.RawMessage
				if err := json.Unmarshal(value, &props); err != nil {
					t.Fatal(err)
				}
				for _, child := range props {
					walk(t, child)
				}
			case "items":
				var child map[string]json.RawMessage
				if err := json.Unmarshal(value, &child); err != nil {
					t.Fatal(err)
				}
				walk(t, child)
			default:
				t.Errorf("unexpected schema keyword %q", key)
			}
		}
	}
	for _, d := range Definitions(allToolCapabilities()) {
		t.Run(d.Name, func(t *testing.T) {
			var node map[string]json.RawMessage
			if err := json.Unmarshal(d.Parameters, &node); err != nil {
				t.Fatal(err)
			}
			if string(node["type"]) != `"object"` {
				t.Fatal("tool parameters must be an object")
			}
			walk(t, node)
		})
	}
}
