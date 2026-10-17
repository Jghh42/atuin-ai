package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/tests/testutil"
)

const atuinSHA = "4ca8b77c8a526b8c294eb864a2b74c95c685c983"

func pinnedBinary(t *testing.T) string {
	t.Helper()
	manifest := os.Getenv("ATUIN_E2E_MANIFEST")
	if manifest == "" {
		t.Skip("BLOCKED real-CLI gate: set ATUIN_E2E_MANIFEST to build-atuin.py artifact; default checks never provision network/toolchains")
	}
	b, e := os.ReadFile(manifest)
	if e != nil {
		t.Fatal(e)
	}
	var m struct {
		SourceSHA  string            `json:"source_sha"`
		Rust       string            `json:"rust"`
		Binary     string            `json:"binary"`
		SHA256     string            `json:"binary_sha256"`
		Recipe     string            `json:"recipe_sha256"`
		Support    string            `json:"support_sha256"`
		Archive    string            `json:"source_archive_sha256"`
		Lock       string            `json:"cargo_lock_sha256"`
		Fresh      bool              `json:"fresh_inputs"`
		Unchanged  bool              `json:"source_worktree_unchanged"`
		Components map[string]string `json:"toolchain_components"`
		Flags      map[string]string `json:"flags"`
		Command    []string          `json:"command"`
	}
	if e = json.Unmarshal(b, &m); e != nil {
		t.Fatal(e)
	}
	if m.SourceSHA != atuinSHA || m.Rust != "1.98.0" || !filepath.IsAbs(m.Binary) {
		t.Fatal("wrong pinned source/toolchain/artifact")
	}
	hashFile := func(path string) string {
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(b)
		return hex.EncodeToString(sum[:])
	}
	expectedComponents := map[string]string{"rustc": "0e37cb339f447fc44d6d781073bacacebfdc5612f2600e4c7e84c266f5f3aced", "cargo": "2f512d170d3dd23e16ababcda32ee2e6d5172d861a7af1f504e0b1e270cafab9", "rust-std": "f5022e6c95a5ad23cca2513dc8281200f585fa188de6370aa37b128a43f876a3"}
	expectedFlags := map[string]string{"CARGO_BUILD_JOBS": "2", "CARGO_INCREMENTAL": "0", "CARGO_PROFILE_RELEASE_DEBUG": "0", "CARGO_PROFILE_RELEASE_STRIP": "symbols", "CARGO_PROFILE_RELEASE_LTO": "false"}
	if !m.Fresh || !m.Unchanged || m.Recipe != hashFile("build-atuin.py") || m.Support != hashFile("../build_support.py") || m.Archive != "eec76d918575cfa6e22bdeae3492c7db11c06d1c208e285ac408bf67bd39d709" || m.Lock != "83997196519a5da77b5f126a4f479b920184aefb4bdc80a8754ef775d09de74d" || !reflect.DeepEqual(m.Components, expectedComponents) || !reflect.DeepEqual(m.Flags, expectedFlags) || !reflect.DeepEqual(m.Command, []string{"cargo", "build", "--locked", "--release", "-p", "atuin"}) {
		t.Fatal("stale/inconsistent fresh source/toolchain/recipe proof; rebuild")
	}
	info, e := os.Lstat(m.Binary)
	if e != nil || !info.Mode().IsRegular() {
		t.Fatal("artifact must be a regular non-symlink file")
	}
	b, e = os.ReadFile(m.Binary)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != m.SHA256 {
		t.Fatal("binary hash differs from build manifest")
	}
	return m.Binary
}

type step map[string]any

func wait(s string) step { return step{"wait": s} }

// Text can render before the FSM processes done. Wait for the source's idle
// completion banner after this unique answer before sending a slash/exit key.
func settled(s string) step { return wait(`(?s)` + regexp.QuoteMeta(s) + `.*Responded in`) }
func send(s string) step    { return step{"send": s} }
func call(id, name, input string) testutil.Call {
	return testutil.Call{ID: id, Name: name, Input: json.RawMessage(input)}
}
func runPTY(t *testing.T, binary, root, endpoint, prompt string, steps []step, extra ...map[string]any) {
	t.Helper()
	plan := map[string]any{"binary": binary, "root": root, "endpoint": endpoint, "prompt": prompt, "steps": steps}
	for _, opts := range extra {
		for k, v := range opts {
			plan[k] = v
		}
	}
	b, e := json.Marshal(plan)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	cmd := exec.CommandContext(ctx, "python3", "-I", "confinement.py")
	// Give Python's finally/atexit handlers a bounded chance to reap its
	// separate-session PTY and daemon children before hard termination.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 7 * time.Second
	cmd.Stdin = bytes.NewReader(b)
	// Python itself also gets a minimal environment, not the test runner's secrets.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8", "HOME=" + filepath.Join(root, "home"), "TMPDIR=" + root}
	if manifest := os.Getenv("ATUIN_TEST_BWRAP_MANIFEST"); manifest != "" {
		cmd.Env = append(cmd.Env, "ATUIN_TEST_BWRAP_MANIFEST="+manifest)
	}
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("PTY driver: %v\n%s", e, out)
	}
	t.Log(strings.TrimSpace(string(out)))
}
func assertGenerations(t *testing.T, n *testutil.Native, count int) {
	t.Helper()
	client, upstream, _, _, _ := n.Captures()
	if len(client) != count || len(upstream) != count {
		t.Fatalf("one generation per request: client=%d upstream=%d want=%d", len(client), len(upstream), count)
	}
	sessions := n.Sessions()
	if len(sessions) != count {
		t.Fatal("missing HTTP session witnesses")
	}
	for i, r := range client {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("OSS CLI sent client authentication")
		}
		if sessions[i] == "" || sessions[i] != sessions[0] {
			t.Fatal("session response identity changed")
		}
		if i > 0 {
			var request map[string]any
			if json.Unmarshal(r.Body, &request) != nil || request["session_id"] != sessions[0] {
				t.Fatal("CLI continuation omitted server-issued session")
			}
		}
	}
}
func assertResult(t *testing.T, n *testutil.Native, id, contains string) {
	t.Helper()
	_, up, _, _, _ := n.Captures()
	if len(up) < 2 {
		t.Fatal("CLI did not continue with tool results")
	}
	var req struct {
		Input []map[string]any `json:"input"`
	}
	if e := json.Unmarshal(up[len(up)-1].Body, &req); e != nil {
		t.Fatal(e)
	}
	var foundCall, foundResult bool
	var callCount, resultCount int
	var observed string
	for _, v := range req.Input {
		if v["call_id"] == id {
			if v["type"] == "function_call" {
				foundCall = true
				callCount++
			}
			if v["type"] == "function_call_output" {
				s, _ := v["output"].(string)
				observed = s
				resultCount++
				foundResult = strings.Contains(s, contains)
			}
		}
	}
	if !foundCall || !foundResult || callCount != 1 || resultCount != 1 {
		if len(observed) > 1500 {
			observed = observed[:1500]
		}
		t.Fatalf("full-history/call correlation missing for %s (call=%v result=%v); synthetic result: %q", id, foundCall, foundResult, observed)
	}
}
func assertClientErrorResult(t *testing.T, n *testutil.Native, id string) {
	t.Helper()
	requests, _, _, _, _ := n.Captures()
	var body struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if len(requests) < 2 || json.Unmarshal(requests[len(requests)-1].Body, &body) != nil {
		t.Fatal("missing client error continuation")
	}
	for _, m := range body.Messages {
		var blocks []map[string]any
		if json.Unmarshal(m.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			if b["type"] == "tool_result" && b["tool_use_id"] == id {
				if b["is_error"] != true {
					t.Fatal("actual CLI did not mark failed/denied tool result as error")
				}
				return
			}
		}
	}
	t.Fatal("actual CLI omitted failed result")
}
func TestPinnedCLI(t *testing.T) {
	binary := pinnedBinary(t)
	t.Run("text-model-default-and-selection", func(t *testing.T) {
		n := testutil.New(t, []testutil.Turn{{Text: "SYNTHETIC_FIRST"}, {Text: "SYNTHETIC_SECOND"}}, true)
		parent := t.TempDir()
		root := filepath.Join(parent, "private-fixture")
		if e := os.Mkdir(root, 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.MkdirAll(filepath.Join(parent, ".atuin", "skills", "parent"), 0700); e != nil {
			t.Fatal(e)
		}
		poisoned := []byte("SYNTHETIC_ANCESTOR_CONTEXT !`touch ANCESTOR_EXECUTED`\n")
		for _, p := range []string{filepath.Join(parent, "TERMINAL.md"), filepath.Join(parent, ".atuin", "TERMINAL.md")} {
			if e := os.WriteFile(p, poisoned, 0600); e != nil {
				t.Fatal(e)
			}
		}
		if e := os.WriteFile(filepath.Join(parent, ".atuin", "skills", "parent", "SKILL.md"), []byte("---\nname: SYNTHETIC_PARENT_SKILL\ndescription: never discover\n---\nfixture\n"), 0600); e != nil {
			t.Fatal(e)
		}
		git := exec.Command("git", "init", "--quiet", parent)
		git.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + parent, "GIT_CONFIG_NOSYSTEM=1"}
		if out, e := git.CombinedOutput(); e != nil {
			t.Fatalf("synthetic ancestor git fixture: %v %s", e, out)
		}
		if e := os.Chmod(parent, 0777); e != nil {
			t.Fatal(e)
		}
		runPTY(t, binary, root, n.URL, "synthetic first", []step{settled("SYNTHETIC_FIRST"), send("/model\r"), wait("Luna"), send("\x1b[B"), send("\r"), send("synthetic second\r"), settled("SYNTHETIC_SECOND"), send("\x1b")})
		assertGenerations(t, n, 2)
		client, up, models, _, _ := n.Captures()
		for _, capture := range append(append([]testutil.Request(nil), client...), up...) {
			for _, forbidden := range []string{"SYNTHETIC_ANCESTOR_CONTEXT", "SYNTHETIC_PARENT_SKILL", parent} {
				if bytes.Contains(capture.Body, []byte(forbidden)) {
					t.Fatal("external ancestor context/project/skill leaked into request")
				}
			}
		}
		if _, e := os.Lstat(filepath.Join(root, "sandbox", "ANCESTOR_EXECUTED")); !os.IsNotExist(e) {
			t.Fatal("external context embedded command executed")
		}
		if models != 0 {
			t.Fatalf("catalog fetches=%d", models)
		}
		for i, r := range up {
			var body map[string]any
			_ = json.Unmarshal(r.Body, &body)
			if body["model"] != []string{"gpt-6.1-sol", "gpt-6-luna"}[i] {
				t.Fatal("wrong selected/default model")
			}
		}
		var a, b map[string]any
		_ = json.Unmarshal(client[0].Body, &a)
		_ = json.Unmarshal(client[1].Body, &b)
		selected, ok := b["config"].(map[string]any)
		if !ok || selected["model"] != "gpt-6-luna" {
			t.Fatal("actual /model picker choice was not sent by CLI")
		}
		sessions := n.Sessions()
		if len(sessions) != 2 || sessions[0] == "" || b["session_id"] != sessions[0] || sessions[0] != sessions[1] {
			t.Fatal("CLI did not reuse server-issued session identity")
		}
		if !bytes.Contains(up[1].Body, []byte("synthetic first")) || !bytes.Contains(up[1].Body, []byte("SYNTHETIC_FIRST")) {
			t.Fatal("full text history missing")
		}
	})
	t.Run("session-resume-new-process", func(t *testing.T) {
		n := testutil.New(t, []testutil.Turn{{Text: "SYNTHETIC_BEFORE_RESTART"}, {Text: "SYNTHETIC_AFTER_RESTART"}}, true)
		root := t.TempDir()
		runPTY(t, binary, root, n.URL, "synthetic before restart", []step{settled("SYNTHETIC_BEFORE_RESTART"), send("\x1b")})
		runPTY(t, binary, root, n.URL, "synthetic after restart", []step{settled("SYNTHETIC_AFTER_RESTART"), send("\x1b")})
		assertGenerations(t, n, 2)
		_, up, _, _, _ := n.Captures()
		for _, text := range []string{"synthetic before restart", "SYNTHETIC_BEFORE_RESTART", "synthetic after restart"} {
			if !bytes.Contains(up[1].Body, []byte(text)) {
				t.Fatal("new actual CLI process did not replay persisted history")
			}
		}
	})
	t.Run("suggestion-insert-not-execute", func(t *testing.T) {
		n := testutil.New(t, []testutil.Turn{{Calls: []testutil.Call{call("suggest", "suggest_command", `{"command":"printf SYNTHETIC_SUGGESTION","description":"synthetic only","confidence":"high","danger":"low"}`)}}}, true)
		runPTY(t, binary, t.TempDir(), n.URL, "synthetic suggestion", []step{wait("printf SYNTHETIC_SUGGESTION"), send("\t")}, map[string]any{"expected_result": "__atuin_ai_insert__:printf SYNTHETIC_SUGGESTION"})
		assertGenerations(t, n, 1)
	})
	cases := []struct {
		name          string
		calls         []testutil.Call
		result        string
		deny          bool
		file, content string
	}{
		{"read_file", []testutil.Call{call("read", "read_file", `{"file_path":"fixture.txt","offset":0,"limit":10}`)}, "SYNTHETIC_OLD", false, "", ""},
		{"write_file", []testutil.Call{call("write", "write_file", `{"file_path":"new.txt","content":"SYNTHETIC_NEW"}`)}, "", false, "new.txt", "SYNTHETIC_NEW"},
		{"execute_shell_command", []testutil.Call{call("shell", "execute_shell_command", `{"command":"printf SYNTHETIC_SHELL","shell":"bash","description":"synthetic only"}`)}, "SYNTHETIC_SHELL", false, "", ""},
		{"atuin_history", []testutil.Call{call("history", "atuin_history", `{"query":"SYNTHETIC_HISTORY","filter_modes":["global"],"limit":2}`)}, "SYNTHETIC_HISTORY", false, "", ""},
		{"atuin_output", []testutil.Call{call("output", "atuin_output", `{"history_id":"SYNTHETIC_HISTORY_ID","ranges":[[0,2]]}`)}, "SYNTHETIC_CAPTURED_OUTPUT", false, "", ""},
		{"load_skill", []testutil.Call{call("skill", "load_skill", `{"name":"synthetic"}`)}, "SYNTHETIC_SKILL_BODY", false, "", ""},
		{"permission-denial", []testutil.Call{call("deny", "write_file", `{"file_path":"denied.txt","content":"NEVER"}`)}, "denied", true, "denied.txt", ""},
		{"execution-failure", []testutil.Call{call("failure", "execute_shell_command", `{"command":"exit 7","shell":"bash","description":"synthetic failure"}`)}, "Exit code: 7", false, "", ""},
		{"parallel-tools", []testutil.Call{call("parallel-a", "read_file", `{"file_path":"fixture.txt"}`), call("parallel-b", "load_skill", `{"name":"synthetic"}`)}, "SYNTHETIC_OLD", false, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := testutil.New(t, []testutil.Turn{{Calls: c.calls}, {Text: "SYNTHETIC_FINISHED"}}, true)
			root := t.TempDir()
			steps := []step{}
			// load_skill has no permission target; the real CLI loads from its registry.
			if c.name != "load_skill" {
				steps = append(steps, wait("Allow"))
				if c.deny {
					steps = append(steps, send("\x1b[A"))
				}
				steps = append(steps, send("\r"))
			}
			steps = append(steps, settled("SYNTHETIC_FINISHED"), send("\x1b"))
			opts := map[string]any{}
			if c.name == "atuin_history" {
				opts["seed_history"] = true
			}
			if c.name == "atuin_output" {
				opts["daemon"] = true
				n.HistoryIDFile = filepath.Join(root, "history-id.txt")
			}
			runPTY(t, binary, root, n.URL, "synthetic "+c.name, steps, opts)
			assertGenerations(t, n, 2)
			for _, v := range c.calls {
				expected := c.result
				if c.name == "parallel-tools" && v.Name == "load_skill" {
					expected = "SYNTHETIC_SKILL_BODY"
				}
				assertResult(t, n, v.ID, expected)
				if c.deny || c.name == "execution-failure" {
					assertClientErrorResult(t, n, v.ID)
				}
			}
			if c.name == "atuin_history" {
				id, e := os.ReadFile(filepath.Join(root, "history-id.txt"))
				if e != nil {
					t.Fatal(e)
				}
				assertResult(t, n, "history", "History ID: "+strings.TrimSpace(string(id)))
			}
			if c.file != "" {
				b, e := os.ReadFile(filepath.Join(root, "sandbox", c.file))
				if c.deny {
					if !os.IsNotExist(e) {
						t.Fatal("denied tool wrote file")
					}
				} else if e != nil || string(b) != c.content {
					t.Fatal("CLI write did not match fixture")
				}
			}
		})
	}
	t.Run("edit_file-after-read", func(t *testing.T) {
		n := testutil.New(t, []testutil.Turn{{Calls: []testutil.Call{call("read", "read_file", `{"file_path":"fixture.txt"}`)}}, {Calls: []testutil.Call{call("edit", "edit_file", `{"file_path":"fixture.txt","old_string":"SYNTHETIC_OLD","new_string":"SYNTHETIC_EDITED"}`)}}, {Text: "SYNTHETIC_FINISHED"}}, true)
		root := t.TempDir()
		runPTY(t, binary, root, n.URL, "synthetic edit", []step{wait("Allow"), send("\r"), wait("Allow this file for this session"), send("\r"), settled("SYNTHETIC_FINISHED"), send("\x1b")})
		assertGenerations(t, n, 3)
		assertResult(t, n, "edit", "")
		assertResult(t, n, "read", "SYNTHETIC_OLD")
		b, e := os.ReadFile(filepath.Join(root, "sandbox", "fixture.txt"))
		if e != nil || string(b) != "SYNTHETIC_EDITED\n" {
			t.Fatal("edit did not execute correctly")
		}
	})
	t.Run("buffered-tool-until-completion", func(t *testing.T) {
		n := testutil.New(t, []testutil.Turn{{Text: "SYNTHETIC_BUFFER_WITNESS", Calls: []testutil.Call{call("buffer", "write_file", `{"file_path":"buffered.txt","content":"SYNTHETIC_BUFFERED"}`)}, Hold: true}, {Text: "SYNTHETIC_FINISHED"}}, true)
		root := t.TempDir()
		runPTY(t, binary, root, n.URL, "synthetic buffer", []step{wait("SYNTHETIC_BUFFER_WITNESS"), {"absent": "buffered.txt", "release": true}, step{"wait": "Allow", "fresh": true}, send("\r"), settled("SYNTHETIC_FINISHED"), send("\x1b")})
		assertGenerations(t, n, 2)
		assertResult(t, n, "buffer", "")
		n.AssertBuffered(t)
		b, e := os.ReadFile(filepath.Join(root, "sandbox", "buffered.txt"))
		if e != nil || string(b) != "SYNTHETIC_BUFFERED" {
			t.Fatal("buffered tool did not execute after successful completion and permission")
		}
	})
	t.Run("provider-error-after-text", func(t *testing.T) {
		n := testutil.New(t, []testutil.Turn{{Text: "SYNTHETIC_PARTIAL", Calls: []testutil.Call{call("never", "write_file", `{"file_path":"never.txt","content":"NEVER"}`)}, Fail: true}}, true)
		runPTY(t, binary, t.TempDir(), n.URL, "synthetic error", []step{wait("SYNTHETIC_PARTIAL"), wait("Retry"), {"absent": "never.txt"}, send("\x1b")})
		assertGenerations(t, n, 1)
	})
	t.Run("mixed-batch-never-executes", func(t *testing.T) {
		n := testutil.New(t, []testutil.Turn{{Calls: []testutil.Call{call("mixed-write", "write_file", `{"file_path":"never.txt","content":"NEVER"}`), call("mixed-suggest", "suggest_command", `{"command":"printf synthetic","description":"synthetic","confidence":"high","danger":"low"}`)}}}, true)
		runPTY(t, binary, t.TempDir(), n.URL, "synthetic invalid batch", []step{wait("Retry"), {"absent": "never.txt"}, send("\x1b")})
		assertGenerations(t, n, 1)
	})
	t.Run("disconnect-cancels-upstream", func(t *testing.T) {
		n := testutil.New(t, []testutil.Turn{{Text: "SYNTHETIC_CANCEL_WITNESS", Cancel: true}}, true)
		runPTY(t, binary, t.TempDir(), n.URL, "synthetic cancel", []step{wait("SYNTHETIC_CANCEL_WITNESS"), send("\x03")})
		n.AwaitCancel()
		assertGenerations(t, n, 1)
	})
	for _, state := range []string{"missing", "expired", "revoked"} {
		t.Run("oauth-"+state, func(t *testing.T) {
			n := testutil.New(t, []testutil.Turn{{Text: "SYNTHETIC_AUTH_OK"}}, state != "missing")
			if state != "missing" {
				n.Expire(state == "revoked")
			}
			steps := []step{wait("Retry"), send("\x1b")}
			count := 0
			if state == "expired" {
				steps = []step{settled("SYNTHETIC_AUTH_OK"), send("\x1b")}
				count = 1
			}
			root := t.TempDir()
			runPTY(t, binary, root, n.URL, "synthetic oauth", steps)
			client, up, _, _, rot := n.Captures()
			if len(client) != 1 || len(up) != count {
				t.Fatal("credential preflight generated unexpectedly")
			}
			if state != "missing" && rot != 1 {
				t.Fatal("expected exactly one refresh")
			}
			if state == "revoked" {
				n.Login()
				runPTY(t, binary, root, n.URL, "synthetic after explicit reauth", []step{settled("SYNTHETIC_AUTH_OK"), send("\x1b")})
				client, up, _, exchanges, rot := n.Captures()
				if len(client) != 2 || len(up) != 1 || exchanges != 2 || rot != 1 {
					t.Fatal("explicit synthetic reauth did not restore real CLI inference")
				}
			}
		})
	}
}
