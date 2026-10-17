//go:build linux || darwin

package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/credentials"
)

func mutationCalls(e *execFixture) []string {
	var verbs []string
	for _, c := range e.calls {
		verb := c.Args[0]
		if c.Path == "/usr/bin/systemctl" {
			verb = c.Args[1]
		}
		switch verb {
		case "stop", "bootout", "start", "bootstrap", "kickstart", "enable", "disable":
			verbs = append(verbs, verb)
		}
	}
	return verbs
}
func seedManaged(s *UserService, m *memoryFiles, e *execFixture) {
	_, _ = m.Replace(s.unit, Snapshot{}, s.content, 0600, false)
	_, _ = m.Replace(s.binary, Snapshot{}, []byte("SYNTHETIC-IDENTITY:old"), 0755, false)
	e.active = true
	e.registered = true
	e.enabled = true
	m.writes = nil
}

func TestIndeterminateLaunchdInspectionNeverAuthorizesMutation(t *testing.T) {
	for _, kind := range []string{"generic-exit", "timeout", "overflow", "incomplete", "not-started", "not-found-wrong-target", "not-found-partial-stdout"} {
		t.Run(kind, func(t *testing.T) {
			s, m, e := serviceTestFixture("darwin")
			r := CommandResult{Started: true, ExitCode: 1, Stderr: []byte("synthetic failure")}
			switch kind {
			case "timeout":
				r.Canceled = true
			case "overflow":
				r.Overflow = true
			case "incomplete":
				r.Incomplete = true
				r.ExitCode = 113
				r.Stderr = []byte("Could not find service \"" + serviceLabel + "\" in domain for user gui: " + fmt.Sprint(s.opts.UID))
			case "not-started":
				r.Started = false
				r.ExitCode = -1
			case "not-found-wrong-target":
				r.ExitCode = 113
				r.Stderr = []byte("Could not find service \"foreign\" in domain for user gui: 1000")
			case "not-found-partial-stdout":
				r.ExitCode = 113
				r.Stderr = []byte("Could not find service \"" + serviceLabel + "\" in domain for user gui: " + fmt.Sprint(s.opts.UID))
				r.Stdout = []byte("partial registered job")
			}
			e.inspectResult = &r
			e.inspectErr = ErrService
			e.foreignPath = "/foreign/unit"
			e.registered = true
			e.active = true
			if err := s.Install(context.Background()); err == nil || len(m.writes) != 0 || len(mutationCalls(e)) != 0 {
				t.Fatal("indeterminate observation mutated service/files", kind)
			}
		})
	}
}
func TestPartialSystemdNotFoundOutputIsIndeterminate(t *testing.T) {
	s, m, e := serviceTestFixture("linux")
	r := CommandResult{Started: true, ExitCode: 1, Stdout: []byte("LoadState=not-found\nFragmentPath=\nDropInPaths=\n")}
	e.inspectResult = &r
	e.inspectErr = ErrService
	if s.Install(context.Background()) == nil || len(m.writes) != 0 || len(mutationCalls(e)) != 0 {
		t.Fatal("partial failed show output authorized installation")
	}
}
func TestFailedBootstrapNeverBootsOutForeignOrUnknownRegistration(t *testing.T) {
	for _, kind := range []string{"foreign", "overflow", "timeout"} {
		t.Run(kind, func(t *testing.T) {
			s, m, e := serviceTestFixture("darwin")
			e.onMutation = func(verb string) error {
				if verb == "bootstrap" {
					e.foreignPath = "/foreign/unit"
					e.registered = true
					e.active = true
					if kind != "foreign" {
						r := CommandResult{Started: true, ExitCode: 0, Overflow: kind == "overflow", Canceled: kind == "timeout"}
						e.inspectResult = &r
						e.inspectErr = ErrService
					}
					return ErrService
				}
				return nil
			}
			err := s.Install(context.Background())
			if !errors.Is(err, ErrRollback) {
				t.Fatal("indeterminate registration not reported", err)
			}
			for _, verb := range mutationCalls(e) {
				if verb == "bootout" || verb == "kickstart" {
					t.Fatal("touched foreign registration")
				}
			}
			if !e.active || !e.registered {
				t.Fatal("foreign job stopped")
			}
			if m.files[s.unit].Exists {
				t.Fatal("own failed installation file not rolled back")
			}
		})
	}
}
func TestLinuxEffectiveConfigurationAndRuntimeIdentityRefusals(t *testing.T) {
	cases := []map[string]string{
		{"DropInPaths": "/foreign/override.conf"},
		{"NeedDaemonReload": "yes"},
		{"Type": "oneshot", "RemainAfterExit": "yes", "MainPID": "0", "ExecMainPID": "0", "SubState": "exited"},
		{"MainPID": "0", "ExecMainPID": "0"},
		{"MainPID": "42", "ExecMainPID": "43"},
		{"ExecStart": "{ path=/foreign/executable ; argv[]=/foreign/executable ; ignore_errors=no }"},
		{"Environment": "HOME=/foreign XDG_CONFIG_HOME=/foreign"},
		{"EnvironmentFiles": "/foreign/credentials.env (ignore_errors=no)"},
		{"PassEnvironment": "SECRET"},
		{"UnsetEnvironment": "HOME"},
	}
	for i, override := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			s, m, e := serviceTestFixture("linux")
			seedManaged(s, m, e)
			e.overrides = override
			if err := s.Install(context.Background()); err == nil || len(m.writes) != 0 || len(mutationCalls(e)) != 0 {
				t.Fatal("effective foreign/stale/no-process unit accepted", i)
			}
			if running, err := s.Running(context.Background()); err == nil || running {
				t.Fatal("unsafe running service offered for temporary stop")
			}
		})
	}
	for _, why := range []string{"wrong uid", "wrong inode", "wrong argv"} {
		t.Run(why, func(t *testing.T) {
			s, m, e := serviceTestFixture("linux")
			seedManaged(s, m, e)
			calls := 0
			s.processCheck = func(context.Context, int, string, []string, int) error { calls++; return ErrService }
			if s.Install(context.Background()) == nil || calls == 0 || len(mutationCalls(e)) != 0 || len(m.writes) != 0 {
				t.Fatal("process ownership not checked")
			}
		})
	}
}
func TestStopAppliedThenErrorReconcilesWithIndependentContext(t *testing.T) {
	for _, osname := range []string{"linux", "darwin"} {
		for _, canceled := range []bool{false, true} {
			t.Run(osname+fmt.Sprint(canceled), func(t *testing.T) {
				service, m, e := serviceTestFixture(osname)
				seedManaged(service, m, e)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				e.onMutation = func(verb string) error {
					if verb == "stop" || verb == "bootout" {
						if canceled {
							cancel()
							return context.Canceled
						}
						return ErrService
					}
					return nil
				}
				w, _, _, _ := fixture("y")
				w.Service = service
				opens := 0
				w.OpenAuth = func() (Auth, error) { opens++; return nil, credentials.ErrLocked }
				err := w.Run(ctx)
				if err == nil || (canceled && !errors.Is(err, ErrCanceled)) {
					t.Fatal("primary stop failure lost", err)
				}
				if opens != 1 || !e.active {
					t.Fatal("applied stop not reconciled/restored")
				}
			})
		}
	}
	t.Run("installation stop itself applies then cancels", func(t *testing.T) {
		s, m, e := serviceTestFixture("linux")
		seedManaged(s, m, e)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		e.onMutation = func(verb string) error {
			if verb == "stop" {
				cancel()
				return context.Canceled
			}
			return nil
		}
		err := s.Install(ctx)
		if !errors.Is(err, ErrCanceled) || !e.active {
			t.Fatal("install stop cancellation did not restore", err)
		}
	})
}
func TestDeregisteredButLivePriorPIDBlocksBootstrap(t *testing.T) {
	s, m, e := serviceTestFixture("darwin")
	seedManaged(s, m, e)
	s.processGone = func(int) (bool, error) { return false, nil }
	w, _, _, _ := fixture("y")
	w.Service = s
	w.OpenAuth = func() (Auth, error) { return nil, credentials.ErrLocked }
	if w.Run(context.Background()) == nil {
		t.Fatal("live prior process treated as stopped")
	}
	for _, verb := range mutationCalls(e) {
		if verb == "bootstrap" || verb == "kickstart" {
			t.Fatal("restarted before prior PID was gone")
		}
	}
}
func TestGlobalDropinOnFirstLoadRefusedBeforeEnableStart(t *testing.T) {
	s, m, e := serviceTestFixture("linux")
	e.onMutation = func(verb string) error {
		if verb == "daemon-reload" {
			e.overrides = map[string]string{"DropInPaths": "/foreign/global-dropin.conf"}
		}
		return nil
	}
	if s.Install(context.Background()) == nil || len(mutationCalls(e)) != 0 {
		t.Fatal("newly effective override reached enable/start")
	}
	if m.files[s.unit].Exists || m.files[s.binary].Exists || m.files[s.opts.ConfigPath].Exists {
		t.Fatal("own failed files not restored")
	}
}
func TestConfigRollbackUncertaintyBlocksPriorServiceRestore(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(fmt.Sprint(uncertain), func(t *testing.T) {
			s, m, e := serviceTestFixture("linux")
			seedManaged(s, m, e)
			w, a, _, _ := fixture("y", "n", "y")
			a.ready = true
			w.Service = s
			opens := 0
			w.OpenAuth = func() (Auth, error) {
				opens++
				if opens == 1 {
					return nil, credentials.ErrLocked
				}
				return a, nil
			}
			w.Configure = func() error {
				if uncertain {
					return errors.Join(ErrCommitUncertain, ErrRollback)
				}
				return ErrCommitUncertain
			}
			if w.Run(context.Background()) == nil || e.active == uncertain {
				t.Fatal("config rollback safety ignored")
			}
		})
	}
}
func TestSystemdContextSpecificEscapingAndPinnedDefaultXDG(t *testing.T) {
	value := `HOME=/home/a $literal %n "quote" \slash`
	environment := systemdEnvironmentQuote(value)
	argument := systemdArgumentQuote(value)
	if strings.Contains(environment, "$$") || !strings.Contains(argument, "$$literal") || !strings.Contains(environment, "%%n") {
		t.Fatal("environment and exec dollar rules conflated")
	}
	s, _, _ := serviceTestFixture("linux")
	if s.opts.XDGConfig != "/home/synthetic/.config" || !strings.Contains(string(s.content), `Environment="XDG_CONFIG_HOME=/home/synthetic/.config"`) {
		t.Fatal("default XDG not pinned")
	}
	for _, osname := range []string{"linux", "darwin"} {
		s, m, e2 := serviceTestFixture(osname)
		if err := s.Install(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, c := range e2.calls {
			found := false
			for _, v := range c.Env {
				if v == "XDG_CONFIG_HOME="+s.opts.XDGConfig {
					found = true
				}
			}
			if !found {
				t.Fatal("supervisor context inherited unspecified XDG")
			}
		}
		if !m.files[s.unit].Exists {
			t.Fatal("missing generated unit")
		}
	}
}
func TestObservedDollarPercentSpacePathsAreLiteral(t *testing.T) {
	original, m, e := serviceTestFixture("linux")
	o := original.opts
	o.Home = "/home/a $literal %n space"
	o.AccountHome = o.Home
	o.ConfigPath = o.Home + "/config space.toml"
	o.XDGConfig = ""
	s := NewService(o)
	e.unitPath = func() string { return s.unit }
	e.program = s.binary
	e.arguments = s.arguments()
	e.environment = []string{"HOME=" + s.opts.Home, "XDG_CONFIG_HOME=" + s.opts.XDGConfig}
	_, _ = m.Replace(s.unit, Snapshot{}, s.content, 0600, false)
	if state, err := s.inspect(context.Background(), true); err != nil || state != stateInactive {
		t.Fatal("literal environment/argument paths changed", err)
	}
	if !strings.Contains(string(s.content), "$$literal") || strings.Contains(string(s.content), `Environment="HOME=/home/a $$literal`) {
		t.Fatal("systemd environment dollar incorrectly escaped")
	}
}
func TestTypedCommandResultsArePrivateBoundedAndNotGenericAbsence(t *testing.T) {
	r := CommandResult{Started: true, ExitCode: 1, Stdout: []byte("SYNTHETIC-PRIVATE"), Stderr: []byte("SYNTHETIC-PRIVATE")}
	encoded, err := json.Marshal(r)
	if err != nil || bytes.Contains(encoded, []byte("SYNTHETIC")) || strings.Contains(fmt.Sprintf("%+v %#v %q", r, r, r), "SYNTHETIC") {
		t.Fatal("supervisor output escaped redaction")
	}
	budget := &captureBudget{remaining: 16 << 10}
	a := commandCapture{budget: budget}
	b := commandCapture{budget: budget}
	_, _ = a.Write(bytes.Repeat([]byte("a"), 12000))
	_, _ = b.Write(bytes.Repeat([]byte("b"), 12000))
	if a.Len()+b.Len() != 16<<10 || !budget.overflow {
		t.Fatal("stdout/stderr combined cap widened")
	}
	if r.success() || !r.complete() {
		t.Fatal("completed error result lost exit status")
	}
	r.Canceled = true
	if r.complete() {
		t.Fatal("canceled command considered definitive")
	}
}
