package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/tools"
)

func privateFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, e := os.ReadDir(dir)
	if e != nil {
		t.Fatal(e)
	}
	out := map[string]string{}
	for _, entry := range entries {
		b, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			t.Fatal(e)
		}
		out[entry.Name()] = string(b)
	}
	return out
}
func toolFromWire(t *testing.T, body string) tools.Call {
	t.Helper()
	for _, frame := range strings.Split(body, "\n\n") {
		if strings.HasPrefix(frame, "event: tool_call\n") {
			var c tools.Call
			if json.Unmarshal([]byte(strings.TrimPrefix(frame, "event: tool_call\ndata: ")), &c) != nil {
				t.Fatal("invalid tool frame")
			}
			return c
		}
	}
	t.Fatal("missing protected tool frame")
	return tools.Call{}
}
func TestRuntimeProtectedRestartReadOnlyDiagnosticsAndAttendedRepair(t *testing.T) {
	f := &fakeNative{t: t, now: time.Now().UTC().Truncate(time.Second)}
	path := privatePath(t)
	r := runtime(t, path, f)
	if e := r.Manager.Login(context.Background(), func(credentials.DevicePrompt) error { return nil }); e != nil {
		t.Fatal(e)
	}
	reason := map[string]any{"type": "reasoning", "id": "private-reason-0", "summary": []any{map[string]any{"type": "summary_text", "text": "PRIVATE-RUNTIME-SUMMARY"}}, "encrypted_content": "PRIVATE-RUNTIME-STATE", "status": "completed"}
	message := func(id, text string) map[string]any {
		return map[string]any{"type": "message", "id": id, "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}}}
	}
	function := func(id, backend string) map[string]any {
		return map[string]any{"type": "function_call", "id": id, "call_id": backend, "name": "read_file", "arguments": `{"file_path":"owned-fixture","extra":18446744073709551615}`, "status": "completed"}
	}
	f.generate = func(n int, input []map[string]any) []any {
		if n >= 2 && n <= 3 {
			want := []map[string]any{reason, message("private-message-0", " \tvisible "), function("private-item-0", "private-backend-0"), {"type": "function_call_output", "call_id": "private-backend-0", "output": "RESULT-0"}}
			if n == 3 {
				want = append(want, message("private-message-1", "middle "), function("private-item-1", "private-backend-1"), map[string]any{"type": "function_call_output", "call_id": "private-backend-1", "output": "RESULT-1"})
			}
			if len(input) != len(want)+2 {
				t.Error("replay duplicated/omitted original output or actual results")
			} else {
				for j, v := range want {
					if !reflect.DeepEqual(input[j+2], v) {
						t.Error("original provider replay/order/argument/trailing-text bytes changed")
					}
				}
			}
		}
		switch n {
		case 1:
			return []any{reason, message("private-message-0", " \tvisible "), function("private-item-0", "private-backend-0")}
		case 2:
			return []any{message("private-message-1", "middle "), function("private-item-1", "private-backend-1")}
		case 3:
			return []any{reason, message("terminal-message", "terminal ")}
		case 4:
			return []any{reason, function("new-item", "new-backend")}
		}
		t.Error("unexpected generation/retry")
		return nil
	}
	base, stop := startServer(t, r)
	request := map[string]any{"session_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "config": map[string]any{"capabilities": []string{"client_v1_read_file"}}, "messages": []any{map[string]any{"role": "user", "content": "OWNED-USER-PROMPT"}}}
	post := func(base string) string {
		b, _ := json.Marshal(request)
		status, body := api(t, base, "/api/cli/chat", string(b))
		if status != 200 {
			t.Fatal("chat status changed")
		}
		for _, private := range []string{"PRIVATE-RUNTIME-SUMMARY", "PRIVATE-RUNTIME-STATE", "private-item-0", "private-backend-0"} {
			if strings.Contains(body, private) {
				t.Fatal("private state leaked through actual HTTP")
			}
		}
		return body
	}
	body := post(base)
	first := toolFromWire(t, body)
	if len(first.ID) != 90 || !strings.HasPrefix(first.ID, "aic1_") {
		t.Fatal("opaque handle missing")
	}
	stop()
	if r.Close() != nil {
		t.Fatal("close failed")
	}
	credentialBefore, e := os.ReadFile(filepath.Join(path, "record.json"))
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(path, "continuation-v1")
	files := privateFiles(t, dir)
	if len(files) != 2 {
		t.Fatal("protected publication did not produce key and one snapshot")
	}
	for _, data := range files {
		for _, marker := range []string{"PRIVATE-RUNTIME-STATE", "PRIVATE-RUNTIME-SUMMARY", "OWNED-USER-PROMPT", "private-backend-0"} {
			if strings.Contains(data, marker) {
				t.Fatal("private disk object retained plaintext/archive")
			}
		}
	}
	// An unknown owned child makes lifecycle construction invalid. Status and
	// diagnosis must neither inspect nor repair that continuation namespace.
	unknown := filepath.Join(dir, "owned-unknown-input")
	if os.WriteFile(unknown, []byte("OWNED_NO_STATE"), 0600) != nil {
		t.Fatal("owned fixture write")
	}
	before := privateFiles(t, dir)
	status, e := NewStatus(path, config.Default(), f)
	if e != nil || status.Continuation != nil {
		t.Fatal("status opened private namespace")
	}
	status.Close()
	diagnostic, e := NewDiagnostic(path, config.Default(), f)
	if e != nil || diagnostic.Continuation != nil {
		t.Fatal("diagnosis opened private namespace")
	}
	diagnostic.Close()
	if !reflect.DeepEqual(before, privateFiles(t, dir)) {
		t.Fatal("read-only command changed private state")
	}
	os.Remove(unknown)
	appendHistory := func(c tools.Call, text, result string) {
		messages := request["messages"].([]any)
		messages = append(messages, map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": text}, map[string]any{"type": "tool_use", "id": c.ID, "name": c.Name, "input": c.Input}}}, map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": c.ID, "content": result}}})
		request["messages"] = messages
	}
	appendHistory(first, "visible ", "RESULT-0")
	r = runtime(t, path, f)
	base, stop = startServer(t, r)
	body = post(base)
	second := toolFromWire(t, body)
	if len(second.ID) != 90 {
		t.Fatal("nonreasoning inherited executable was not protected")
	}
	stop()
	r.Close()
	os.Remove(filepath.Join(dir, first.ID[22:54]+".sealed"))
	appendHistory(second, "middle ", "RESULT-1")
	r = runtime(t, path, f)
	base, stop = startServer(t, r)
	body = post(base)
	if strings.Contains(body, "event: tool_call") || !strings.Contains(body, "event: done") {
		t.Fatal("terminal text failed after second restart")
	}
	stop()
	r.Close()
	credentialAfter, e := os.ReadFile(filepath.Join(path, "record.json"))
	if e != nil || string(credentialAfter) != string(credentialBefore) {
		t.Fatal("ordinary generation/restart changed credential JSON")
	}
	// /new cannot repair a corrupt global key. Even a genuinely new reasoned
	// executable turn may make one native request, but must release no tools.
	if os.WriteFile(filepath.Join(dir, "key.bundle"), []byte("owned-corrupt-key"), 0600) != nil {
		t.Fatal("owned key corruption")
	}
	r = runtime(t, path, f)
	base, stop = startServer(t, r)
	request["messages"] = []any{map[string]any{"role": "user", "content": "OWNED-NEW-TRANSCRIPT"}}
	request["session_id"] = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	body = post(base)
	if strings.Contains(body, "event: tool_call") || !strings.Contains(body, "event: error") {
		t.Fatal("new transcript silently repaired/bypassed corrupt key")
	}
	stop()
	if e := r.Manager.Logout(context.Background()); e != nil {
		t.Fatal("attended logout could not remove corrupt global state")
	}
	if len(privateFiles(t, dir)) != 0 {
		t.Fatal("attended cleanup retained key/snapshots")
	}
	r.Close()
}
func TestRuntimeCapabilityFailureUnwindsCredentialFlock(t *testing.T) {
	path := privatePath(t)
	if os.MkdirAll(filepath.Join(path, "continuation-v1"), 0700) != nil {
		t.Fatal("owned fixture")
	}
	if os.Chmod(filepath.Join(path, "continuation-v1"), 0755) != nil {
		t.Fatal("owned mode")
	}
	if _, e := New(path, config.Default(), nil); e != credentials.ErrStore {
		t.Fatal("unsafe child accepted")
	}
	store, e := credentials.OpenStore(path)
	if e != nil {
		t.Fatal("constructor leaked credential flock")
	}
	store.Close()
	status, e := NewStatus(path, config.Default(), nil)
	if e != nil {
		t.Fatal("status inspected unsafe continuation child")
	}
	status.Close()
}
