package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchAtuinSurgicalAndIdempotent(t *testing.T) {
	for _, text := range []string{
		"",
		"# global\nfoo = 'retained'\n",
		"[ai]\nmodel = 'unknown-private-choice' # model exactly\nenabled = false # enabled comment\nendpoint = '''old\nmultiline''' # endpoint comment\napi_token = 'SYNTHETIC-SECRET' # token comment\nunknown = { foo = [1,2] }\n[other]\nx = true\n",
		"# intro\n[ai] # table\nmodel = \"\"\"model\nwith # text\"\"\"\n# tail\n",
		"[[other]]\nx=1\n[\"unrelated\"]\nz='keep'\n[ai]\nmodel='gpt-6-luna'\n",
		"[ai]\n  model = 'gpt-6.1-sol'\r\n  enabled  =false\r\n[other]\nx=[\n 1, # aggregate comment\n 2,\n]\n",
		"[ai]\nendpoint = 'old'", // no newline
	} {
		t.Run(strings.ReplaceAll(text, "\n", "/")[:min(30, len(text))], func(t *testing.T) {
			out, e := PatchAtuin([]byte(text), 19222)
			if e != nil {
				t.Fatal(e)
			}
			again, e := PatchAtuin(out, 19222)
			if e != nil || !bytes.Equal(out, again) {
				t.Fatal("not idempotent", e, string(out), string(again))
			}
			for _, snippet := range []string{"model = 'unknown-private-choice' # model exactly", "model = \"\"\"model\nwith # text\"\"\"", "unknown = { foo = [1,2] }", "# enabled comment", "# endpoint comment", "# token comment", "model='gpt-6-luna'", "[other]\nx=[\n 1, # aggregate comment\n 2,\n]"} {
				if strings.Contains(text, snippet) && !strings.Contains(string(out), snippet) {
					t.Fatal("unrelated bytes changed", snippet)
				}
			}
			if strings.Contains(string(out), "SYNTHETIC-SECRET") {
				t.Fatal("token retained")
			}
		})
	}
}
func TestPatchAtuinRefusals(t *testing.T) {
	for _, text := range []string{
		"[ai]\nenabled=true\nenabled=false\n",
		"[ai]\nmodel='one'\n[ai]\nmodel='two'\n",
		"[ai]\nendpoint_protocol='hub'\n",
		"ai = {model='x'}\n",
		"ai.enabled=false\n",
		"[\"ai\"]\nmodel='x'\n",
		"['a\u0069']\nmodel='x'\n",
		"[\"a\\u0069\"]\nmodel='x'\n",
		"[ai.nested]\nx=1\n",
		"[ai]\n'enabled'=false\n",
		"[ai]\nmodel.foo=1\n",
		"[[ai]]\nx=1\n",
		"[ai]\nmodel=\"unterminated\n",
		"[ai]\nenabled=not_toml\n",
		strings.Repeat(" ", MaxAtuinBytes+1),
	} {
		if _, e := PatchAtuin([]byte(text), 17391); e == nil {
			t.Fatalf("accepted unsupported %q", text)
		}
	}
	// OSS explicitly configured remains byte-for-byte, as do unknown model IDs.
	if _, e := PatchAtuin([]byte("[ai]\nendpoint_protocol='oss'\nmodel='future-model'\n"), 17391); e != nil {
		t.Fatal(e)
	}
}
func TestConfigureAtuinPrivateBackupNoopAndModes(t *testing.T) {
	root := privateRoot(t)
	path := filepath.Join(root, "config.toml")
	files := DiskFiles{}
	original := []byte("# kept\n[ai]\nmodel='future-model'\napi_token='SYNTHETIC-ONLY'\n")
	if e := os.WriteFile(path, original, 0640); e != nil {
		t.Fatal(e)
	}
	if e := ConfigureAtuin(files, path, 17392); e != nil {
		t.Fatal(e)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0640 {
		t.Fatal("mode changed")
	}
	names, _ := filepath.Glob(filepath.Join(root, ".atuin-ai-backup-*"))
	if len(names) != 1 {
		t.Fatal(names)
	}
	b, e := os.ReadFile(names[0])
	if e != nil || !bytes.Equal(b, original) {
		t.Fatal("bad backup")
	}
	st, _ = os.Stat(names[0])
	if st.Mode().Perm() != 0600 {
		t.Fatal("backup not private")
	}
	if e = ConfigureAtuin(files, path, 17392); e != nil {
		t.Fatal(e)
	}
	after, _ := filepath.Glob(filepath.Join(root, ".atuin-ai-backup-*"))
	if len(after) != 1 {
		t.Fatal("noop made backup")
	}
	newPath := filepath.Join(root, "new", "config.toml")
	if e = ConfigureAtuin(files, newPath, 17391); e != nil {
		t.Fatal(e)
	}
	st, _ = os.Stat(newPath)
	if st.Mode().Perm() != 0600 {
		t.Fatal("new file not private")
	}
}
func TestResolverMacLegacyAndAmbiguity(t *testing.T) {
	root := privateRoot(t)
	home := filepath.Join(root, "home")
	files := DiskFiles{}
	legacy := filepath.Join(home, ".config", "atuin", "config.toml")
	_, e := ResolveAtuin("", home, "", "", "", files)
	if e == nil {
		t.Fatal("unknown installed version guessed a new active path")
	}
	p, e := ResolveAtuin(legacy, home, "", "", "", files)
	if e != nil || p != legacy {
		t.Fatal("explicit creation path refused")
	}
	if e = ConfigureAtuin(files, legacy, 17391); e != nil {
		t.Fatal(e)
	}
	p, e = ResolveAtuin("", home, "", "", "", files)
	if e != nil || p != legacy {
		t.Fatal(p, e)
	}
	newer := filepath.Join(home, ".atuin", "config.toml")
	if e = ConfigureAtuin(files, newer, 17391); e != nil {
		t.Fatal(e)
	}
	if _, e = ResolveAtuin("", home, "", "", "", files); e == nil {
		t.Fatal("ambiguity guessed")
	}
	p, e = ResolveAtuin(newer, home, "", "", "", files)
	if e != nil || p != newer {
		t.Fatal(p, e)
	}
	// XDG must not hide an existing legacy config either.
	if e = ConfigureAtuin(files, filepath.Join(root, "xdg", "atuin", "config.toml"), 17391); e != nil {
		t.Fatal(e)
	}
	if _, e = ResolveAtuin("", home, filepath.Join(root, "xdg"), "", "", files); e == nil {
		t.Fatal("XDG shadowed legacy")
	}
	if _, e = ResolveAtuin("", home, "", "relative", "", files); e == nil {
		t.Fatal("relative override accepted")
	}
}
