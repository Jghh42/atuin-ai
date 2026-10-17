//go:build linux || darwin

package setup

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// These are source-derived v258 wire fixtures, not live service evidence. See
// docs/provenance/systemd-init.md for exact upstream refs, lines and source hashes.
func TestSystemdV258EmptyStructArraysAndFreshInstall(t *testing.T) {
	s, m, e := serviceTestFixture("linux")
	r, err := s.command(context.Background(), "inspect")
	if err != nil || !r.success() {
		t.Fatal("synthetic completed show unavailable")
	}
	if bytes.Contains(r.Stdout, []byte("EnvironmentFiles=")) || bytes.Contains(r.Stdout, []byte("ExecStart=")) {
		t.Fatal("fixture invented empty struct-array lines")
	}
	if !bytes.Contains(r.Stdout, []byte("FragmentPath=\n")) {
		t.Fatal("required empty scalar missing from fixture")
	}
	if state, err := s.inspect(context.Background(), false); err != nil || state != stateAbsent {
		t.Fatal("normal uninstalled service rejected", err)
	}
	if err := s.Install(context.Background()); err != nil {
		t.Fatal("fresh native-format installation rejected", err)
	}
	if !e.active || !e.enabled || !m.files[s.binary].Exists || !m.files[s.unit].Exists || !m.files[s.opts.ConfigPath].Exists {
		t.Fatal("fresh install did not actually mutate fixture/activate")
	}
	r, err = s.command(context.Background(), "inspect")
	if err != nil || bytes.Contains(r.Stdout, []byte("EnvironmentFiles=")) || !bytes.Contains(r.Stdout, []byte("ExecStart={ path="+s.binary+" ; argv[]="+s.binary+" serve --config "+s.opts.ConfigPath+" ;")) {
		t.Fatal("loaded fixture does not use v258 raw ExecStart representation")
	}
	s.opts.Config = m.files[s.opts.ConfigPath]
	m.writes = nil
	if err := s.Install(context.Background()); err != nil || len(m.writes) != 0 {
		t.Fatal("native-format idempotence failed", err)
	}
}
func removeProperty(data []byte, key string) []byte {
	var out strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" && !strings.HasPrefix(line, key+"=") {
			out.WriteString(line + "\n")
		}
	}
	return []byte(out.String())
}
func TestSystemdV258OnlyDocumentedOmissionsAllowed(t *testing.T) {
	for _, loaded := range []bool{false, true} {
		for _, key := range strings.Split(systemdProperties, ",") {
			if key == "EnvironmentFiles" || (!loaded && key == "ExecStart") {
				continue
			}
			t.Run(key+map[bool]string{true: " loaded", false: " absent"}[loaded], func(t *testing.T) {
				s, m, e := serviceTestFixture("linux")
				if loaded {
					_, _ = m.Replace(s.unit, Snapshot{}, s.content, 0600, false)
				}
				r, err := s.command(context.Background(), "inspect")
				if err != nil {
					t.Fatal(err)
				}
				r.Stdout = removeProperty(r.Stdout, key)
				e.inspectResult = &r
				e.inspectErr = nil
				m.writes = nil
				e.calls = nil
				if err := s.Install(context.Background()); err == nil || len(m.writes) != 0 || len(mutationCalls(e)) != 0 {
					t.Fatal("missing required property authorized mutation", key)
				}
			})
		}
	}
	for _, kind := range []string{"exit-error", "canceled", "overflow", "incomplete", "truncated-line", "contradictory-exec", "contradictory-env-file"} {
		t.Run(kind, func(t *testing.T) {
			s, m, e := serviceTestFixture("linux")
			r, err := s.command(context.Background(), "inspect")
			if err != nil {
				t.Fatal(err)
			}
			e.inspectErr = nil
			switch kind {
			case "exit-error":
				r.ExitCode = 1
				e.inspectErr = ErrService
			case "canceled":
				r.Canceled = true
				e.inspectErr = context.Canceled
			case "overflow":
				r.Overflow = true
			case "incomplete":
				r.Incomplete = true
			case "truncated-line":
				r.Stdout = bytes.TrimSuffix(r.Stdout, []byte("\n"))
			case "contradictory-exec":
				r.Stdout = append(r.Stdout, []byte("ExecStart={ path=/foreign ; argv[]=/foreign ; ignore_errors=no }\n")...)
			case "contradictory-env-file":
				r.Stdout = append(r.Stdout, []byte("EnvironmentFiles=/foreign (ignore_errors=no)\n")...)
			}
			e.inspectResult = &r
			e.calls = nil
			if s.Install(context.Background()) == nil || len(m.writes) != 0 || len(mutationCalls(e)) != 0 {
				t.Fatal("unknown/truncated show converted omitted arrays into absence")
			}
		})
	}
	s, m, e := serviceTestFixture("linux")
	_, _ = m.Replace(s.unit, Snapshot{}, s.content, 0600, false)
	e.overrides = map[string]string{"EnvironmentFiles": "/foreign/env (ignore_errors=no)"}
	m.writes = nil
	if s.Install(context.Background()) == nil || len(m.writes) != 0 || len(mutationCalls(e)) != 0 {
		t.Fatal("nonempty environment-file array defaulted away")
	}
}
func serviceFixturePaths(home, cfg string) (*UserService, *memoryFiles, *execFixture) {
	original, m, e := serviceTestFixture("linux")
	o := original.opts
	o.Home = home
	o.AccountHome = home
	o.ConfigPath = cfg
	o.XDGConfig = ""
	s := NewService(o)
	s.identify = original.identify
	s.wait = original.wait
	s.processGone = original.processGone
	s.processCheck = func(_ context.Context, pid int, binary string, args []string, uid int) error {
		if pid != 42 || binary != s.binary || !equalStrings(args, s.arguments()) || uid != s.opts.UID || !m.files[binary].Exists {
			return ErrService
		}
		return nil
	}
	e.unitPath = func() string {
		if m.files[s.unit].Exists {
			return s.unit
		}
		return ""
	}
	e.program = s.binary
	e.arguments = s.arguments()
	e.environment = []string{"HOME=" + s.opts.Home, "XDG_CONFIG_HOME=" + s.opts.XDGConfig}
	return s, m, e
}
func TestSystemdV258ExecutableLoadedAndRuntimeDollarForms(t *testing.T) {
	home := "/home/synthetic$seat"
	cfg := home + "/config${CHOICE}$$.toml"
	s, m, e := serviceFixturePaths(home, cfg)
	// load-fragment.c:975-1011 resolves executable separately. @ supplies argv0
	// explicitly, so argv0 and config get $$ while executable path gets literal $.
	expected := `ExecStart=@"/home/synthetic$seat/.local/bin/atuin-ai-server" "/home/synthetic$$seat/.local/bin/atuin-ai-server" serve --config "/home/synthetic$$seat/config$${CHOICE}$$$$.toml"`
	if !strings.Contains(string(s.content), expected+"\n") {
		t.Fatal("executable/argument render forms conflated")
	}
	if err := s.Install(context.Background()); err != nil {
		t.Fatal("literal-dollar install failed", err)
	}
	if !e.active || !e.enabled {
		t.Fatal("dollar-path fixture never activated")
	}
	r, err := s.command(context.Background(), "inspect")
	if err != nil {
		t.Fatal(err)
	}
	show := `ExecStart={ path=/home/synthetic$seat/.local/bin/atuin-ai-server ; argv[]=/home/synthetic$$seat/.local/bin/atuin-ai-server serve --config /home/synthetic$$seat/config$${CHOICE}$$$$.toml ; ignore_errors=no ;`
	if !bytes.Contains(r.Stdout, []byte(show)) {
		t.Fatal("fixture quotes or prematurely expands loaded argv")
	}
	if !bytes.Contains(r.Stdout, []byte(`Environment="HOME=/home/synthetic\$seat" "XDG_CONFIG_HOME=/home/synthetic\$seat/.config"`)) {
		t.Fatal("fixture omits native environment shell quoting")
	}
	gotRuntime := false
	s.processCheck = func(_ context.Context, pid int, binary string, args []string, uid int) error {
		gotRuntime = pid == 42 && binary == s.binary && uid == s.opts.UID && equalStrings(args, []string{s.binary, "serve", "--config", cfg})
		if !gotRuntime {
			return ErrService
		}
		return nil
	}
	if running, err := s.Running(context.Background()); err != nil || !running || !gotRuntime {
		t.Fatal("runtime argv was not checked independently from loaded argv")
	}
	// The old pre-expanded mock argv must fail against the loaded representation.
	r.Stdout = bytes.Replace(r.Stdout, []byte("argv[]="+strings.Join(s.loadedArguments(), " ")), []byte("argv[]="+strings.Join(s.arguments(), " ")), 1)
	e.inspectResult = &r
	e.inspectErr = nil
	m.writes = nil
	e.calls = nil
	if s.Install(context.Background()) == nil || len(m.writes) != 0 || len(mutationCalls(e)) != 0 {
		t.Fatal("pre-expanded loaded argv accepted")
	}
}
func TestSystemdV258UnsafeExecutableRejectedBeforeMutation(t *testing.T) {
	// load-fragment.c calls string_is_safe(path); v258 string-util.c:1078-1091
	// rejects quotes/backslashes in executable paths, even when C-unquoted.
	for _, home := range []string{`/home/double"quote`, "/home/single'quote", `/home/back\slash`} {
		t.Run(home, func(t *testing.T) {
			s, m, e := serviceFixturePaths(home, home+"/config.toml")
			if s.Install(context.Background()) == nil || len(m.writes) != 0 || len(e.calls) != 0 {
				t.Fatal("unsupported executable path reached writes/supervisor")
			}
		})
	}
}
func TestSystemdV258RawPathsAndEnvironmentQuoting(t *testing.T) {
	for _, home := range []string{"/home/space name", "/home/percent%n", "/home/$literal${HOME}$$ space"} {
		t.Run(home, func(t *testing.T) {
			s, _, e := serviceFixturePaths(home, home+"/server config%q\"\\.toml")
			if err := s.Install(context.Background()); err != nil || !e.active {
				t.Fatal("source-format path not preserved", err)
			}
			r, err := s.command(context.Background(), "inspect")
			if err != nil || !bytes.Contains(r.Stdout, []byte("ExecStart={ path="+s.binary+" ; argv[]="+strings.Join(s.loadedArguments(), " ")+" ;")) {
				t.Fatal("ExecStart fixture invented path/argv escaping")
			}
		})
	}
	// Snapshot vectors are from v258 test-escape.c:163-180 (flags0).
	for _, v := range [][2]string{{`\`, `"\\"`}, {`"`, `"\""`}, {"foo bar", `"foo bar"`}, {`foo$bar`, `"foo\$bar"`}, {`foo "bar" waldo`, `"foo \"bar\" waldo"`}} {
		if showEnvironmentWord(v[0]) != v[1] {
			t.Fatal("fixture differs from upstream shell_maybe_quote vector")
		}
		words, err := systemdWords(v[1])
		if err != nil || len(words) != 1 || words[0] != v[0] {
			t.Fatal("native Environment string-array decoding failed")
		}
	}
}
