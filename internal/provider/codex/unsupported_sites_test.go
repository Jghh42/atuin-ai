package codex

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/provider"
)

func TestPreparedUnsupportedSitesKeepRawClosedCompatibility(t *testing.T) {
	namespaced := fn("f", "backend", `{"file_path":"synthetic"}`)
	namespaced["namespace"] = map[string]any{"name": "PRIVATE-NAMESPACE"}
	refusal := msg("not public")
	refusal["content"] = []any{map[string]any{"type": "PRIVATE-CONTENT-TYPE", "text": "PRIVATE-CONTENT"}}
	for _, row := range []struct {
		wire     string
		category provider.Category
	}{
		{frame("response.created", map[string]any{"response": map[string]any{"id": "r", "status": "in_progress", "output": []any{msg("not public")}}}), provider.UnsupportedLifecycleOutput},
		{complete(refusal), provider.UnsupportedContent},
		{frame("response.custom_tool.delta", map[string]any{"delta": "PRIVATE-CUSTOM"}), provider.UnsupportedChannel},
		{complete(map[string]any{"type": "PRIVATE-UNKNOWN-TYPE", "id": "private-item"}), provider.UnsupportedItem},
		{complete(namespaced), provider.UnsupportedNamespace},
	} {
		events, _, e := parsedPrepared(row.wire)
		if e == nil {
			t.Fatal("unsupported site accepted")
		}
		assertNoCalls(t, events)
		f, ok := provider.FailureOf(e)
		if !ok || f.Category() != row.category {
			t.Fatalf("new closed site category missing: got=%s want=%s found=%v", f.Category(), row.category, ok)
		}
		_, e = parseStream(strings.NewReader(row.wire), func(provider.Event) error { return nil })
		f, _ = provider.FailureOf(streamFailure(e))
		if f.Category() != provider.UnsupportedOutput {
			t.Fatal("raw compatibility classification changed")
		}
	}
}
func TestPreparedOutputContainerBoundsBeforeCallbacks(t *testing.T) {
	var items []map[string]any
	for index := 0; index < 129; index++ {
		m := msg("public")
		m["id"] = fmt.Sprintf("msg-%d", index)
		items = append(items, m)
	}
	events, _, e := parsedPrepared(complete(items...))
	if e == nil || len(events) != 0 {
		t.Fatal("oversized output decoded/emitted before container bound")
	}
	m := msg("")
	parts := make([]any, 129)
	for j := range parts {
		parts[j] = map[string]any{"type": "output_text", "text": "public"}
	}
	m["content"] = parts
	events, _, e = parsedPrepared(complete(m))
	if e == nil || len(events) != 0 {
		t.Fatal("oversized content decoded/emitted before container bound")
	}
}
