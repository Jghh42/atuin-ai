//go:build linux || darwin

package setup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Synthetic shape with the specific native facts observed on the user's Mac:
// direct OSLogRateLimit=64, root spawn scheduled/no PID, nested active tasks.
// No real launchctl output, account, environment or user file is accessed.
func launchdContractPrint(s *UserService, state, pid string, metadata []string) CommandResult {
	var out strings.Builder
	fmt.Fprintf(&out, "gui/%d/%s = {\n\tpath = %s\n\tprogram = %s\n\tstate = %s\n", s.opts.UID, serviceLabel, s.unit, s.binary, state)
	if pid != "" {
		fmt.Fprintf(&out, "\tpid = %s\n", pid)
	}
	out.WriteString("\targuments = {\n")
	for _, arg := range s.arguments() {
		fmt.Fprintf(&out, "\t\t%s\n", arg)
	}
	fmt.Fprintf(&out, "\t}\n\tenvironment = {\n\t\tHOME => %s\n\t\tXDG_CONFIG_HOME => %s\n", s.opts.Home, s.opts.XDGConfig)
	for _, line := range metadata {
		fmt.Fprintf(&out, "\t\t%s\n", line)
	}
	out.WriteString("\t}\n\tlast exit code = 78: EX_CONFIG\n\ttasks = {\n\t\tstate = active\n\t}\n}\n")
	return CommandResult{Started: true, ExitCode: 0, Stdout: []byte(out.String())}
}

func TestLaunchdObservedMetadataAndOwnedTransition(t *testing.T) {
	for _, pid := range []string{"", "0"} {
		s, _, _ := serviceTestFixture("darwin")
		r := launchdContractPrint(s, "spawn scheduled", pid, []string{"OSLogRateLimit => 64", "XPC_SERVICE_NAME => " + serviceLabel})
		if state, err := s.inspectLaunchd(r, nil, true); state != stateInactive || err != nil || s.lastPID != 0 {
			t.Fatal("owned scheduled registration was refused or treated as a live process")
		}
		if _, err := s.inspectLaunchd(r, nil, false); err == nil {
			t.Fatal("loaded fields alone conferred local file ownership")
		}
	}
	for _, metadata := range [][]string{nil, {"OSLogRateLimit => 64"}, {"XPC_SERVICE_NAME => " + serviceLabel}} {
		s, _, _ := serviceTestFixture("darwin")
		if state, err := s.inspectLaunchd(launchdContractPrint(s, "running", "42", metadata), nil, true); state != stateRunning || err != nil {
			t.Fatal("optional exact native metadata refused")
		}
	}
}

func TestLaunchdMetadataAndStateRefuseContradictions(t *testing.T) {
	for _, metadata := range [][]string{
		{"OSLogRateLimit => 63"}, {"OSLogRateLimit => 064"}, {"OSLogRateLimit => -64"},
		{"OSLogRateLimit => 64", "OSLogRateLimit => 64"},
		{"XPC_SERVICE_NAME => " + serviceLabel, "XPC_SERVICE_NAME => " + serviceLabel},
		{"XPC_SERVICE_NAME => foreign"}, {"PRIVATE_ENV => PRIVATE-VALUE"},
		{"HOME => /foreign"}, {"XDG_CONFIG_HOME => /foreign"},
	} {
		s, _, _ := serviceTestFixture("darwin")
		_, err := s.inspectLaunchd(launchdContractPrint(s, "spawn scheduled", "", metadata), nil, true)
		var detail *serviceFailure
		if !errors.As(err, &detail) || detail.category != categoryEnvironment || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("unexpected or duplicate environment was accepted or echoed")
		}
	}
	for _, fields := range [][2]string{{"spawn scheduled", "42"}, {"spawn scheduled", "-1"}, {"spawn scheduled", "bad"}, {"unknown", ""}, {"running", "0"}} {
		s, _, _ := serviceTestFixture("darwin")
		state, err := s.inspectLaunchd(launchdContractPrint(s, fields[0], fields[1], []string{"OSLogRateLimit => 64"}), nil, true)
		if err == nil || state == stateRunning || state == stateInactive {
			t.Fatal("contradictory PID/state granted lifecycle authority")
		}
	}
	for _, field := range []string{"path", "program", "arguments"} {
		s, _, _ := serviceTestFixture("darwin")
		r := launchdContractPrint(s, "spawn scheduled", "", []string{"OSLogRateLimit => 64"})
		data := string(r.Stdout)
		switch field {
		case "path":
			data = strings.Replace(data, "path = "+s.unit, "path = /foreign", 1)
		case "program":
			data = strings.Replace(data, "program = "+s.binary, "program = /foreign", 1)
		case "arguments":
			data = strings.Replace(data, "\t\tserve\n", "\t\tforeign\n", 1)
		}
		r.Stdout = []byte(data)
		if _, err := s.inspectLaunchd(r, nil, true); err == nil {
			t.Fatal("foreign scheduled registration accepted")
		}
	}
}

func TestLaunchdScheduledActivationUsesExistingBudgetAndSafeRollback(t *testing.T) {
	for _, becomesRunning := range []bool{true, false} {
		s, m, e := serviceTestFixture("darwin")
		e.onMutation = func(verb string) error {
			if verb == "bootstrap" {
				e.active = false
				r := launchdContractPrint(s, "spawn scheduled", "", []string{"OSLogRateLimit => 64"})
				e.inspectResult = &r
			} else if verb == "bootout" {
				e.inspectResult = nil
			}
			return nil
		}
		waits := 0
		s.wait = func(ctx context.Context, d time.Duration) error {
			if d != 100*time.Millisecond || ctx.Err() != nil {
				t.Fatal("activation polling budget changed")
			}
			waits++
			if becomesRunning && waits == 2 {
				e.active = true
				r := launchdContractPrint(s, "running", "42", []string{"OSLogRateLimit => 64"})
				e.inspectResult = &r
			}
			return nil
		}
		err := s.Install(context.Background())
		if becomesRunning {
			if err != nil || waits != 2 || !e.active {
				t.Fatal("scheduled job did not reach verified running state")
			}
		} else {
			var detail *serviceFailure
			if !errors.As(err, &detail) || detail.stage != stageActivation || detail.category != categoryActivationTimeout || errors.Is(err, ErrRollback) || waits != 50 {
				t.Fatal("scheduled job was marked healthy or exceeded its polling budget")
			}
			if e.registered || m.files[s.binary].Exists || m.files[s.unit].Exists || m.files[s.opts.ConfigPath].Exists {
				t.Fatal("verified stopped job did not safely roll back")
			}
		}
	}
}

func TestLaunchdPostBootstrapUnknownRetainsInputsWithoutTouchingForeign(t *testing.T) {
	for _, kind := range []string{"format", "metadata", "state", "overflow", "foreign-replacement"} {
		t.Run(kind, func(t *testing.T) {
			s, m, e := serviceTestFixture("darwin")
			var replacement Snapshot
			e.onMutation = func(verb string) error {
				if verb != "bootstrap" {
					return nil
				}
				r := launchdContractPrint(s, "spawn scheduled", "", nil)
				switch kind {
				case "format":
					r.Stdout = []byte("PRIVATE malformed native response")
				case "metadata":
					r = launchdContractPrint(s, "spawn scheduled", "", []string{"PRIVATE_ENV => PRIVATE"})
				case "state":
					r = launchdContractPrint(s, "unknown", "", nil)
				case "overflow":
					r.Overflow = true
				case "foreign-replacement":
					e.foreignPath = "/foreign"
					_, _ = m.Replace(s.binary, m.files[s.binary], []byte("PRIVATE external replacement"), 0755, false)
					replacement = m.files[s.binary]
					r.Stdout = []byte(strings.Replace(string(r.Stdout), "path = "+s.unit, "path = /foreign", 1))
				}
				e.inspectResult = &r
				return nil // Bootstrap succeeded; readback is inconclusive.
			}
			err := s.Install(context.Background())
			if !errors.Is(err, ErrService) || !errors.Is(err, ErrRollback) || !errors.Is(err, errServiceInputsRetained) || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("unknown registration lost primary failure/uncertainty/privacy")
			}
			for _, path := range []string{s.binary, s.unit, s.opts.ConfigPath} {
				if !m.files[path].Exists {
					t.Fatal("possibly registered job lost a journaled launch input")
				}
			}
			if kind == "foreign-replacement" && !same(replacement, m.files[s.binary]) {
				t.Fatal("retention rewrote an external replacement")
			}
			for _, verb := range mutationCalls(e) {
				if verb == "bootout" || verb == "kickstart" {
					t.Fatal("unverified registration was controlled")
				}
			}
		})
	}
}

func TestServiceUncertainStopRetainsInputs(t *testing.T) {
	for _, osname := range []string{"darwin", "linux"} {
		s, m, e := serviceTestFixture(osname)
		e.brokenActivation = true
		e.onMutation = func(verb string) error {
			if verb == "bootout" || verb == "stop" {
				return errors.New("PRIVATE stop applied but completion failed")
			}
			return nil
		}
		if osname == "linux" {
			// A failed start can still leave a running process. Force a primary
			// command failure after activation, so cleanup must stop it safely.
			e.brokenActivation = false
			e.onMutation = func(verb string) error {
				if verb == "start" || verb == "stop" {
					return errors.New("PRIVATE applied command failure")
				}
				return nil
			}
		}
		err := s.Install(context.Background())
		if !errors.Is(err, ErrRollback) || !errors.Is(err, errServiceInputsRetained) {
			t.Fatal("uncertain stop was treated as safe to remove inputs")
		}
		for _, path := range []string{s.binary, s.unit, s.opts.ConfigPath} {
			if !m.files[path].Exists {
				t.Fatal("uncertain stop removed a journaled launch input")
			}
		}
	}
}

func TestServiceFailureStages(t *testing.T) {
	for _, kind := range []string{"preflight", "source", "config", "inspect", "binary-write", "config-write", "unit-write", "bootstrap"} {
		t.Run(kind, func(t *testing.T) {
			s, m, e := serviceTestFixture("darwin")
			wantStage, wantCategory := stagePreflight, categoryUnsafeInput
			switch kind {
			case "preflight":
				s.opts.Source = "/downloads//synthetic-server"
			case "source":
				source := m.files[s.opts.Source]
				source.Data = []byte("PRIVATE invalid executable")
				m.files[s.opts.Source] = source
				wantStage, wantCategory = stageSource, categoryIdentity
			case "config":
				m.files[s.opts.ConfigPath] = Snapshot{Exists: true, Data: []byte("PRIVATE")}
				wantStage, wantCategory = stageConfig, categoryChanged
			case "inspect":
				e.inspectResult = &CommandResult{ExitCode: -1}
				wantStage, wantCategory = stageInspect, categoryNotStarted
			case "binary-write":
				m.fail = s.binary
				wantStage, wantCategory = stageWriteBinary, categoryWrite
			case "config-write":
				m.fail = s.opts.ConfigPath
				wantStage, wantCategory = stageWriteConfig, categoryWrite
			case "unit-write":
				m.fail = s.unit
				wantStage, wantCategory = stageWriteUnit, categoryWrite
			case "bootstrap":
				e.failVerb = "bootstrap"
				wantStage, wantCategory = stageBootstrap, categoryCommandExit
			}
			err := s.Install(context.Background())
			var detail *serviceFailure
			if !errors.As(err, &detail) || detail.stage != wantStage || detail.category != wantCategory || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("install failure lost its fixed stage/category or privacy")
			}
		})
	}
}

func TestServiceCanceledUnknownRegistrationRetainsInputs(t *testing.T) {
	s, m, e := serviceTestFixture("darwin")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.onMutation = func(verb string) error {
		if verb == "bootstrap" {
			e.inspectResult = &CommandResult{Started: true, ExitCode: 0, Stdout: []byte("PRIVATE malformed registration")}
			cancel()
			return context.Canceled
		}
		return nil
	}
	err := s.Install(ctx)
	if !errors.Is(err, ErrCanceled) || !errors.Is(err, ErrRollback) || !errors.Is(err, errServiceInputsRetained) {
		t.Fatal("canceled primary or independent cleanup uncertainty was lost")
	}
	for _, path := range []string{s.binary, s.unit, s.opts.ConfigPath} {
		if !m.files[path].Exists {
			t.Fatal("canceled unknown registration lost its launch input")
		}
	}
}

func TestServiceAbsentRegistrationDoesNotDiscardInputsUnderKnownLivePID(t *testing.T) {
	for _, gone := range []bool{false, true} {
		s, m, e := serviceTestFixture("darwin")
		s.processGone = func(int) (bool, error) { return gone, nil }
		e.onMutation = func(verb string) error {
			if verb == "bootstrap" {
				s.lastPID = 42
				e.active, e.registered = false, false
				return errors.New("PRIVATE applied start failure")
			}
			return nil
		}
		err := s.Install(context.Background())
		if err == nil || errors.Is(err, errServiceInputsRetained) == gone || errors.Is(err, ErrRollback) == gone {
			t.Fatal("registration absence bypassed known PID safety")
		}
		for _, path := range []string{s.binary, s.unit, s.opts.ConfigPath} {
			if m.files[path].Exists == gone {
				t.Fatal("launch inputs were removed under a known live PID or retained after safe absence")
			}
		}
	}
}

type stopContractExecutor func(context.Context, Command) (CommandResult, error)

func (f stopContractExecutor) Run(ctx context.Context, command Command) (CommandResult, error) {
	return f(ctx, command)
}

func TestLaunchdBootoutMustConfirmRegistrationAbsentBeforeRollback(t *testing.T) {
	for _, state := range []string{"spawn scheduled", "waiting"} {
		t.Run(state, func(t *testing.T) {
			s, m, e := serviceTestFixture("darwin")
			e.onMutation = func(verb string) error {
				if verb == "bootstrap" || verb == "bootout" {
					e.registered, e.active = true, false
					r := launchdContractPrint(s, state, "", []string{"OSLogRateLimit => 64"})
					e.inspectResult = &r
				}
				return nil // Even successful bootout does not prove deregistration.
			}
			err := s.Install(context.Background())
			if !errors.Is(err, ErrRollback) || !errors.Is(err, errServiceInputsRetained) || !e.registered {
				t.Fatal("still-registered nonrunning job was reported safely stopped")
			}
			for _, path := range []string{s.binary, s.unit, s.opts.ConfigPath} {
				if !m.files[path].Exists {
					t.Fatal("still-registered job lost its launch inputs")
				}
			}
			for _, verb := range mutationCalls(e) {
				if verb == "kickstart" {
					t.Fatal("uncertain stop authorized restoration")
				}
			}
		})
	}
}

func TestServiceCleanupStopAbsenceStillChecksPreviouslyObservedPID(t *testing.T) {
	for _, gone := range []bool{false, true} {
		t.Run(fmt.Sprint(gone), func(t *testing.T) {
			s, m, e := serviceTestFixture("darwin")
			started, inspections, processChecks := false, 0, 0
			e.onMutation = func(verb string) error {
				if verb == "bootstrap" {
					started = true
					return ErrService // Applied; first cleanup inspection sees PID42.
				}
				return nil
			}
			s.opts.Exec = stopContractExecutor(func(ctx context.Context, c Command) (CommandResult, error) {
				if started && c.Args[0] == "print" {
					inspections++
					if inspections >= 2 {
						e.active, e.registered = false, false // Stop's own observation.
					}
				}
				return e.Run(ctx, c)
			})
			s.processGone = func(pid int) (bool, error) {
				processChecks++
				if pid != 42 {
					t.Fatal("previous verified PID was lost")
				}
				return gone, nil
			}
			err := s.Install(context.Background())
			if err == nil || processChecks == 0 || errors.Is(err, ErrRollback) == gone || errors.Is(err, errServiceInputsRetained) == gone {
				t.Fatal("absence during Stop bypassed verified-PID shutdown proof")
			}
			for _, path := range []string{s.binary, s.unit, s.opts.ConfigPath} {
				if m.files[path].Exists == gone {
					t.Fatal("rollback did not honor verified-PID shutdown result")
				}
			}
		})
	}
}

func TestServiceStopEarlySuccessAlwaysChecksKnownPID(t *testing.T) {
	for _, osname := range []string{"darwin", "linux"} {
		for _, state := range []string{"absent", "inactive"} {
			if osname == "darwin" && state == "inactive" {
				continue // Darwin requires bootout, covered separately above.
			}
			for _, why := range []string{"alive", "inaccessible", "gone", "no-known-pid"} {
				t.Run(osname+"/"+state+"/"+why, func(t *testing.T) {
					s, m, e := serviceTestFixture(osname)
					if state == "inactive" {
						seedManaged(s, m, e)
					}
					e.active = false
					e.registered = state != "absent"
					s.lastPID = 42
					if why == "no-known-pid" {
						s.lastPID = 0
					}
					checks := 0
					s.processGone = func(pid int) (bool, error) {
						checks++
						if pid != 42 {
							t.Fatal("checked an unverified PID")
						}
						if why == "inaccessible" {
							return false, ErrService
						}
						return why == "gone", nil
					}
					err := s.Stop(context.Background())
					wantSuccess := why == "gone" || why == "no-known-pid"
					if (err == nil) != wantSuccess || (checks == 0) != (why == "no-known-pid") {
						t.Fatal("early stopped state bypassed known PID safety or rejected valid absence")
					}
				})
			}
		}
	}
}

func TestServiceFixedFailureDetailsAndPrivacy(t *testing.T) {
	private := errors.New("PRIVATE argv=/private env=PRIVATE token=PRIVATE")
	for _, category := range []serviceCategory{categoryEnvironment, categoryFormat, categoryState, categoryRollback, 255} {
		err := serviceAt(stageActivation, serviceCause(category, errors.Join(private, ErrRollback, ErrCanceled)))
		for _, format := range []string{"%v", "%+v", "%#v"} {
			if strings.Contains(fmt.Sprintf(format, err), "PRIVATE") {
				t.Fatal("private cause escaped fixed diagnostics")
			}
		}
		if !errors.Is(err, ErrService) || !errors.Is(err, ErrRollback) || !errors.Is(err, ErrCanceled) {
			t.Fatal("diagnostic wrapping lost sentinel semantics")
		}
		w, auth, service, _ := fixture("y", "n")
		auth.ready = true
		service.installErr = errors.Join(err, errServiceInputsRetained)
		if w.Run(context.Background()) == nil {
			t.Fatal("service failure was hidden")
		}
		output := w.TTY.(*terminalFixture).String()
		if !strings.Contains(output, "stage=verify_active category=") || !strings.Contains(output, "not rolled back") || strings.Contains(output, "PRIVATE") {
			t.Fatal("wizard lost safe failure details or exposed private cause")
		}
	}
	if serviceStage(255).String() != "unknown" || serviceCategory(255).String() != "unavailable" {
		t.Fatal("unknown diagnostic enum was not closed")
	}
	for _, input := range []struct {
		r        CommandResult
		category serviceCategory
	}{
		{CommandResult{Canceled: true}, categoryCanceled},
		{CommandResult{Overflow: true}, categoryOverflow},
		{CommandResult{}, categoryNotStarted},
		{CommandResult{Started: true, Incomplete: true}, categoryIncomplete},
		{CommandResult{Started: true, ExitCode: 78}, categoryCommandExit},
	} {
		var detail *serviceFailure
		if !errors.As(commandFailure(input.r), &detail) || detail.category != input.category {
			t.Fatal("command failure distinctions were collapsed")
		}
	}
}
