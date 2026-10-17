//go:build linux || darwin

package setup

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type memoryFiles struct {
	onRead       func(string)
	files        map[string]Snapshot
	serial       uint64
	writes       []string
	fail         string
	rollbackFail bool
	afterApply   func(string, WriteOutcome) (WriteOutcome, error)
}

func (m *memoryFiles) Read(p string, limit int) (Snapshot, error) {
	if m.onRead != nil {
		m.onRead(p)
	}
	s := m.files[p]
	s.Data = append([]byte(nil), s.Data...)
	if len(s.Data) > limit {
		return Snapshot{}, ErrFile
	}
	return s, nil
}
func (m *memoryFiles) Replace(p string, old Snapshot, b []byte, mode os.FileMode, _ bool) (WriteOutcome, error) {
	if !same(m.files[p], old) || p == m.fail || (m.rollbackFail && old.Exists) {
		return WriteOutcome{}, ErrFile
	}
	m.serial++
	s := Snapshot{Data: append([]byte(nil), b...), Exists: true, Mode: mode, dev: 1, ino: m.serial}
	m.files[p] = s
	m.writes = append(m.writes, p)
	outcome := WriteOutcome{After: s, Applied: true, Durable: true}
	if m.afterApply != nil {
		return m.afterApply(p, outcome)
	}
	return outcome, nil
}
func (m *memoryFiles) Remove(p string, s Snapshot) error {
	if m.rollbackFail || !same(m.files[p], s) {
		return ErrFile
	}
	delete(m.files, p)
	return nil
}

type execFixture struct {
	unitPath                    func() string
	program, foreignPath        string
	uid                         int
	arguments, environment      []string
	registered, active, enabled bool
	calls                       []Command
	failVerb                    string
	brokenActivation            bool
	overrides                   map[string]string
	inspectResult               *CommandResult
	inspectErr                  error
	onMutation                  func(string) error
}

// v258 bus-print-properties.c:263 uses shell_maybe_quote(...,0) for
// string arrays ONLY. Its executable struct printer does NOT quote argv/path.
// Fixtures contain valid UTF-8 and no controls (the production path guard).
func showEnvironmentWord(s string) string {
	if !strings.ContainsAny(s, " \t\"\\`$*?['()<>|&;!") {
		return s
	}
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "$", "\\$")
	return "\"" + s + "\""
}
func (e *execFixture) Run(ctx context.Context, c Command) (CommandResult, error) {
	e.calls = append(e.calls, c)
	r := CommandResult{Started: true, ExitCode: 0}
	if ctx.Err() != nil {
		return CommandResult{ExitCode: -1, Canceled: true}, ctx.Err()
	}
	if !allowedCommand(c) {
		return CommandResult{ExitCode: -1}, errors.New("unexpected allowlist rejection")
	}
	verb := c.Args[0]
	if c.Path == "/usr/bin/systemctl" {
		verb = c.Args[1]
	}
	if (verb == "show" || verb == "print") && e.inspectResult != nil {
		return *e.inspectResult, e.inspectErr
	}
	if verb == e.failVerb {
		r.ExitCode = 1
		r.Stderr = []byte("synthetic command failure")
		return r, ErrService
	}
	path := ""
	if e.unitPath != nil {
		path = e.unitPath()
	}
	if e.foreignPath != "" {
		path = e.foreignPath
	}
	switch verb {
	case "show":
		props := map[string]string{"LoadState": "loaded", "FragmentPath": path, "DropInPaths": "", "NeedDaemonReload": "no", "Type": "simple", "ExecStart": "", "Environment": "", "EnvironmentFiles": "", "PassEnvironment": "", "UnsetEnvironment": "", "MainPID": "0", "ExecMainPID": "0", "ActiveState": "inactive", "SubState": "dead", "RemainAfterExit": "no", "RootDirectory": ""}
		if path == "" {
			props["LoadState"] = "not-found"
		}
		if e.active {
			props["MainPID"] = "42"
			props["ExecMainPID"] = "42"
			props["ActiveState"] = "active"
			props["SubState"] = "running"
		}
		// Source-derived native serialization: ExecStart's path and joined argv
		// are printed RAW (not shell/C quoted). Empty struct arrays emit NO line.
		// The generated @ command supplies argv[0] explicitly. Unit loading
		// retains $$ in argv; replace_env_argv collapses it only at exec time.
		loaded := append([]string(nil), e.arguments...)
		for i := range loaded {
			loaded[i] = strings.ReplaceAll(loaded[i], "$", "$$")
		}
		props["ExecStart"] = "{ path=" + e.program + " ; argv[]=" + strings.Join(loaded, " ") + " ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0/0 }"
		words := []string{}
		for _, entry := range e.environment {
			words = append(words, showEnvironmentWord(entry))
		}
		props["Environment"] = strings.Join(words, " ")
		if path == "" {
			props["Environment"] = ""
			props["ExecStart"] = ""
		}
		for k, v := range e.overrides {
			props[k] = v
		}
		var out strings.Builder
		for _, k := range strings.Split(systemdProperties, ",") {
			if k == "EnvironmentFiles" && props[k] == "" {
				continue
			}
			if k == "ExecStart" && props[k] == "" {
				continue
			}
			out.WriteString(k + "=" + props[k] + "\n")
		}
		r.Stdout = []byte(out.String())
		return r, nil
	case "print":
		if !e.registered && !e.active {
			r.ExitCode = 113
			r.Stderr = []byte("Bad request.\nCould not find service \"" + serviceLabel + "\" in domain for user gui: " + strconv.Itoa(e.uid) + "\n")
			return r, ErrService
		}
		state := "waiting"
		pid := "0"
		if e.active {
			state = "running"
			pid = "42"
		}
		out := "gui/" + strconv.Itoa(e.uid) + "/" + serviceLabel + " = {\n path = " + path + "\n program = " + e.program + "\n state = " + state + "\n pid = " + pid + "\n arguments = {\n"
		for _, a := range e.arguments {
			out += "  " + a + "\n"
		}
		out += " }\n environment = {\n"
		for _, entry := range e.environment {
			k, v, _ := strings.Cut(entry, "=")
			out += "  " + k + " => " + v + "\n"
		}
		out += " }\n}\n"
		r.Stdout = []byte(out)
		return r, nil
	case "is-enabled":
		if e.enabled {
			r.Stdout = []byte("enabled\n")
			return r, nil
		}
		r.ExitCode = 1
		r.Stdout = []byte("disabled\n")
		return r, ErrService
	case "start", "bootstrap", "kickstart":
		e.registered = true
		if !e.brokenActivation {
			e.active = true
		}
	case "stop", "bootout":
		e.registered = false
		e.active = false
	case "enable":
		e.enabled = true
	case "disable":
		e.enabled = false
	}
	if e.onMutation != nil {
		if err := e.onMutation(verb); err != nil {
			r.ExitCode = 1
			r.Canceled = errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
			return r, err
		}
	}
	return r, nil
}
func serviceTestFixture(osname string) (*UserService, *memoryFiles, *execFixture) {
	m := &memoryFiles{files: map[string]Snapshot{}}
	e := &execFixture{}
	s := NewService(ServiceOptions{OS: osname, Home: "/home/synthetic", AccountHome: "/home/synthetic", UID: os.Geteuid(), ConfigPath: "/home/synthetic/config.toml", Port: 19333, Source: "/downloads/synthetic-server", RuntimeDir: "/run/user/" + strconv.Itoa(os.Geteuid()), Files: m, Exec: e})
	e.unitPath = func() string {
		if m.files[s.unit].Exists {
			return s.unit
		}
		return ""
	}
	e.program = s.binary
	e.uid = s.opts.UID
	e.arguments = s.arguments()
	e.environment = []string{"HOME=" + s.opts.Home, "XDG_CONFIG_HOME=" + s.opts.XDGConfig}
	s.processGone = func(pid int) (bool, error) {
		if pid != 42 {
			return false, ErrService
		}
		return !e.active, nil
	}
	s.processCheck = func(_ context.Context, pid int, binary string, args []string, uid int) error {
		if pid != 42 || binary != s.binary || !equalStrings(args, s.arguments()) || uid != s.opts.UID || !m.files[binary].Exists {
			return ErrService
		}
		return nil
	}
	s.identify = func(b []byte) bool { return bytes.HasPrefix(b, []byte("SYNTHETIC-IDENTITY:")) }
	s.wait = func(context.Context, time.Duration) error { return nil }
	_, _ = m.Replace(s.opts.Source, Snapshot{}, []byte("SYNTHETIC-IDENTITY:new"), 0755, false)
	m.writes = nil
	return s, m, e
}
func TestServiceInstallAndIdempotentBinary(t *testing.T) {
	for _, osname := range []string{"linux", "darwin"} {
		t.Run(osname, func(t *testing.T) {
			s, m, e := serviceTestFixture(osname)
			if err := s.Install(context.Background()); err != nil {
				t.Fatal(err)
			}
			if !e.active || (osname == "linux" && !e.enabled) {
				t.Fatal("not started/enabled")
			}
			running, err := s.Running(context.Background())
			if err != nil || !running {
				t.Fatal("status not verified")
			}
			stable := m.files[s.binary]
			unit := m.files[s.unit]
			// The config is resolved once; a subsequent run captures its existing snapshot.
			s.opts.Config = m.files[s.opts.ConfigPath]
			m.writes = nil
			if err = s.Install(context.Background()); err != nil {
				t.Fatal(err)
			}
			if !same(stable, m.files[s.binary]) || !same(unit, m.files[s.unit]) || len(m.writes) != 0 {
				t.Fatal("same-byte files replaced", m.writes)
			}
			for _, c := range e.calls {
				for _, env := range c.Env {
					if strings.HasPrefix(env, "OPENAI") || strings.HasPrefix(env, "CHAT_CONFIG") {
						t.Fatal("secret environment forwarded")
					}
				}
			}
		})
	}
}
func TestServiceNegativeControlsAndRollback(t *testing.T) {
	for _, osname := range []string{"linux", "darwin"} {
		t.Run(osname+" foreign unit", func(t *testing.T) {
			s, m, e := serviceTestFixture(osname)
			_, _ = m.Replace(s.unit, Snapshot{}, []byte("foreign"), 0600, false)
			m.writes = nil
			if s.Install(context.Background()) == nil || len(e.calls) != 0 || len(m.writes) != 0 {
				t.Fatal("foreign file changed or supervisor invoked")
			}
		})
		t.Run(osname+" invalid source", func(t *testing.T) {
			s, m, e := serviceTestFixture(osname)
			source := m.files[s.opts.Source]
			source.Data = []byte("not a server")
			m.files[s.opts.Source] = source
			if s.Install(context.Background()) == nil || len(e.calls) != 0 {
				t.Fatal("unvalidated source installed")
			}
		})
		t.Run(osname+" foreign binary", func(t *testing.T) {
			s, m, e := serviceTestFixture(osname)
			_, _ = m.Replace(s.binary, Snapshot{}, []byte("arbitrary executable"), 0755, false)
			if s.Install(context.Background()) == nil || len(e.calls) != 0 {
				t.Fatal("arbitrary executable overwritten")
			}
		})
		t.Run(osname+" write failure retains old", func(t *testing.T) {
			s, m, e := serviceTestFixture(osname)
			_, _ = m.Replace(s.binary, Snapshot{}, []byte("SYNTHETIC-IDENTITY:old"), 0755, false)
			before := append([]byte(nil), m.files[s.binary].Data...)
			m.fail = s.unit
			if s.Install(context.Background()) == nil {
				t.Fatal("failure hidden")
			}
			if !bytes.Equal(before, m.files[s.binary].Data) || e.active {
				t.Fatal("old binary not restored")
			}
			if m.files[s.opts.ConfigPath].Exists {
				t.Fatal("partial config retained")
			}
		})
		t.Run(osname+" startup failure rolls back activation", func(t *testing.T) {
			s, m, e := serviceTestFixture(osname)
			e.brokenActivation = true
			if s.Install(context.Background()) == nil {
				t.Fatal("claimed active based on files only")
			}
			if m.files[s.binary].Exists || m.files[s.unit].Exists || e.enabled {
				t.Fatal("partial state retained")
			}
		})
		t.Run(osname+" rollback failure reported", func(t *testing.T) {
			s, m, e := serviceTestFixture(osname)
			m.rollbackFail = true
			e.brokenActivation = true
			if !errors.Is(s.Install(context.Background()), ErrRollback) {
				t.Fatal("rollback failure hidden")
			}
		})
	}
	s, _, e := serviceTestFixture("linux")
	s.opts.Home = "/tmp/synthetic"
	if s.Install(context.Background()) == nil || len(e.calls) != 0 {
		t.Fatal("temporary HOME accepted")
	}
	s, _, e = serviceTestFixture("linux")
	s.opts.Home = "/home/another"
	if s.Install(context.Background()) == nil || len(e.calls) != 0 {
		t.Fatal("test HOME accepted")
	}
	s, _, e = serviceTestFixture("linux")
	s.opts.ConfigPath = "/tmp/capture/config.toml"
	if s.Install(context.Background()) == nil || len(e.calls) != 0 {
		t.Fatal("transient config accepted")
	}
	s, _, e = serviceTestFixture("linux")
	s.opts.UID = 0
	if s.Install(context.Background()) == nil || len(e.calls) != 0 {
		t.Fatal("root service accepted")
	}
	s, m, e := serviceTestFixture("linux")
	s.opts.Config = Snapshot{Exists: true, Data: []byte("changed")}
	if s.Install(context.Background()) == nil || len(e.calls) != 0 || len(m.writes) != 0 {
		t.Fatal("changed config accepted")
	}
}
func TestForeignSupervisorUnitOrRegisteredLabelRefused(t *testing.T) {
	for _, osname := range []string{"linux", "darwin"} {
		for _, local := range []bool{false, true} {
			t.Run(osname+strconv.FormatBool(local), func(t *testing.T) {
				s, m, e := serviceTestFixture(osname)
				if local {
					_, _ = m.Replace(s.unit, Snapshot{}, s.content, 0600, false)
				}
				m.writes = nil
				e.foreignPath = "/foreign/other-unit"
				e.registered = true
				e.active = true
				if s.Install(context.Background()) == nil || len(m.writes) != 0 {
					t.Fatal("foreign registered service changed")
				}
				for _, c := range e.calls {
					if strings.Contains(strings.Join(c.Args, " "), "stop") || strings.Contains(strings.Join(c.Args, " "), "bootout") {
						t.Fatal("stopped foreign service")
					}
				}
			})
		}
	}
}
func TestServiceRejectsConcurrentStartupInputs(t *testing.T) {
	for _, kind := range []string{"source", "config", "unit", "binary"} {
		t.Run(kind, func(t *testing.T) {
			s, m, e := serviceTestFixture("linux")
			var target string
			switch kind {
			case "source":
				target = s.opts.Source
			case "config":
				target = s.opts.ConfigPath
				_, _ = m.Replace(target, Snapshot{}, []byte("port=19333\n"), 0600, false)
				s.opts.Config = m.files[target]
			case "unit":
				target = s.unit
				_, _ = m.Replace(target, Snapshot{}, s.content, 0600, false)
			case "binary":
				target = s.binary
				_, _ = m.Replace(target, Snapshot{}, []byte("SYNTHETIC-IDENTITY:new"), 0755, false)
			}
			reads := 0
			m.onRead = func(path string) {
				if path == target {
					reads++
					if reads == 2 {
						changed := m.files[path]
						changed.Data = append(append([]byte(nil), changed.Data...), []byte("# concurrent edit")...)
						m.files[path] = changed
					}
				}
			}
			if s.Install(context.Background()) == nil || e.active {
				t.Fatal("concurrent startup input accepted", kind)
			}
		})
	}
}
func TestServiceRestoresPreviouslyRunningOnFailedReplacement(t *testing.T) {
	s, m, e := serviceTestFixture("linux")
	_, _ = m.Replace(s.unit, Snapshot{}, s.content, 0600, false)
	_, _ = m.Replace(s.binary, Snapshot{}, []byte("SYNTHETIC-IDENTITY:old"), 0755, false)
	e.active = true
	e.enabled = true
	m.fail = s.opts.ConfigPath
	if s.Install(context.Background()) == nil {
		t.Fatal("expected failure")
	}
	if !e.active || !e.enabled || string(m.files[s.binary].Data) != "SYNTHETIC-IDENTITY:old" {
		t.Fatal("prior activation not restored")
	}
}
func TestServiceEscapingAndCommandAllowlist(t *testing.T) {
	s, _, _ := serviceTestFixture("darwin")
	s.opts.Home = "/home/a & <b> \"c\""
	s.binary = s.opts.Home + "/.local/bin/atuin-ai-server"
	s.opts.ConfigPath = s.opts.Home + "/config.toml"
	content := s.render()
	// XML token roundtrip must parse and contain original unescaped path text.
	d := xml.NewDecoder(bytes.NewReader(content))
	seen := false
	for {
		tok, err := d.Token()
		if err != nil {
			if err.Error() != "EOF" {
				t.Fatal(err)
			}
			break
		}
		if text, ok := tok.(xml.CharData); ok && string(text) == s.binary {
			seen = true
		}
	}
	if !seen {
		t.Fatal("XML escaping changed executable path")
	}
	quoted := systemdArgumentQuote(`/home/a %n $HOME \\ "quoted"`)
	if quoted != `"/home/a %%n $$HOME \\\\ \"quoted\""` {
		t.Fatal("systemd escaping", quoted)
	}
	for _, path := range []string{"/home/a\nfile", "/home/a\rfile", "/home/a\tfile"} {
		if persistent(path) {
			t.Fatal("control-bearing service path accepted")
		}
	}
	for _, c := range []Command{
		{Path: "/bin/sh", Args: []string{"-c", "true"}},
		{Path: "systemctl", Args: []string{"--user", "start", "atuin-ai-server.service"}},
		{Path: "/usr/bin/systemctl", Args: []string{"start", "foreign.service"}},
		{Path: "/usr/bin/systemctl", Args: []string{"--user", "start", "foreign.service"}},
		{Path: "/bin/launchctl", Args: []string{"bootstrap", "system", "/tmp/io.github.jghh42.atuin-ai.plist"}},
		{Path: "/bin/launchctl", Args: []string{"print", "gui/0/" + serviceLabel}},
	} {
		if allowedCommand(c) {
			t.Fatal("unsafe supervisor command allowed", c.Args)
		}
	}
	if serverBinary([]byte("SYNTHETIC-IDENTITY: definitely not a real Go executable")) {
		t.Fatal("production identity validator accepts synthetic bytes")
	}
	budget := &captureBudget{remaining: 16 << 10}
	cap := commandCapture{budget: budget}
	data := bytes.Repeat([]byte("x"), 1<<20)
	n, err := cap.Write(data)
	if err != nil || n != len(data) || cap.Len() != 16<<10 || !budget.overflow {
		t.Fatal("output unbounded")
	}
}
