//go:build linux || darwin

package setup

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

func seedXPCProxyPrint(s *UserService, e *execFixture, pid string) {
	r := launchdContractPrint(s, "xpcproxy", pid, []string{"OSLogRateLimit => 64"})
	e.inspectResult = &r
}

func TestLaunchdStartingPersistentTimeoutRetainsInputsAndEvidence(t *testing.T) {
	s, m, e := serviceTestFixture("darwin")
	e.onMutation = func(verb string) error {
		if verb == "bootstrap" {
			seedXPCProxyPrint(s, e, "42")
		}
		return nil
	}
	waits := 0
	var originalDeadline time.Time
	s.wait = func(ctx context.Context, d time.Duration) error {
		deadline, ok := ctx.Deadline()
		if !ok || d != 100*time.Millisecond || time.Until(deadline) > 5*time.Second {
			t.Fatal("starting observation extended activation budget")
		}
		if waits == 0 {
			originalDeadline = deadline
		} else if deadline != originalDeadline {
			t.Fatal("starting observation reset activation deadline")
		}
		waits++
		return nil
	}
	err := s.Install(context.Background())
	detail := requireLaunchdEvidence(t, err, "xpcproxy", "positive")
	var starting *launchdStarting
	if waits != 50 || detail.category != categoryActivationTimeout || detail.stage != stageActivation || errors.As(err, &starting) || !errors.Is(err, ErrRollback) || !errors.Is(err, errServiceInputsRetained) || s.lastPID != 42 {
		t.Fatal("persistent startup was ready/retried beyond budget or lost timeout/PID evidence")
	}
	for _, path := range []string{s.binary, s.unit, s.opts.ConfigPath} {
		if !m.files[path].Exists {
			t.Fatal("uncertain starting registration lost its launch inputs")
		}
	}
	for _, verb := range mutationCalls(e) {
		if verb == "bootout" || verb == "kickstart" {
			t.Fatal("starting condition granted control or restoration authority")
		}
	}
}

func TestLaunchdStartingOnlyWaitActiveMayRetry(t *testing.T) {
	for _, operation := range []string{"running", "stop", "restore"} {
		s, m, e := serviceTestFixture("darwin")
		seedManaged(s, m, e)
		seedXPCProxyPrint(s, e, "42")
		e.calls = nil
		var err error
		switch operation {
		case "running":
			var running bool
			running, err = s.Running(context.Background())
			if running {
				t.Fatal("xpcproxy was reported running")
			}
		case "stop":
			err = s.Stop(context.Background())
		case "restore":
			err = s.Restore(context.Background())
		}
		var starting *launchdStarting
		if !errors.As(err, &starting) || !errors.Is(err, ErrService) || s.lastPID != 42 || len(mutationCalls(e)) != 0 {
			t.Fatal("status/control caller accepted starting or lost the shutdown veto")
		}
	}
}

func TestLaunchdStartingHardFailuresNeverRetry(t *testing.T) {
	for _, test := range [][2]string{
		{"PRIVATE unknown", "42"}, {"forking", "42"}, {"spawning", "42"}, {"", ""},
		{"running", ""}, {"running", "0"}, {"running", "-42"}, {"running", "PRIVATE"},
		{"running", "999999999999999999999999"}, {"running", "+0"},
		{"waiting", "42"}, {"spawn scheduled", "42"},
		{"xpcproxy", ""}, {"xpcproxy", "0"}, {"xpcproxy", "-42"},
		{"xpcproxy", "PRIVATE"}, {"xpcproxy", "999999999999999999999999"},
		{"xpcproxy", "+42"}, {"xpcproxy", "042"}, {"xpcproxy", "000"},
	} {
		s, m, e := serviceTestFixture("darwin")
		seedManaged(s, m, e)
		r := launchdContractPrint(s, test[0], test[1], []string{"OSLogRateLimit => 64"})
		e.inspectResult = &r
		s.wait = func(context.Context, time.Duration) error { t.Fatal("hard state/PID failure was retried"); return nil }
		err := s.waitActive(context.Background())
		var starting *launchdStarting
		if err == nil || errors.As(err, &starting) || s.lastPID != 0 {
			t.Fatal("unsupported state/PID gained starting authority")
		}
	}
	for _, kind := range []string{"local-unit", "path", "program", "argv", "environment", "format", "command", "overflow", "incomplete", "not-started", "empty-pid"} {
		s, m, e := serviceTestFixture("darwin")
		seedManaged(s, m, e)
		r := launchdContractPrint(s, "xpcproxy", "42", []string{"OSLogRateLimit => 64"})
		switch kind {
		case "local-unit":
			delete(m.files, s.unit)
		case "path":
			r.Stdout = []byte(strings.Replace(string(r.Stdout), "path = "+s.unit, "path = /foreign", 1))
		case "program":
			r.Stdout = []byte(strings.Replace(string(r.Stdout), "program = "+s.binary, "program = /foreign", 1))
		case "argv":
			r.Stdout = []byte(strings.Replace(string(r.Stdout), "\t\tserve\n", "\t\tforeign\n", 1))
		case "environment":
			r = launchdContractPrint(s, "xpcproxy", "42", []string{"PRIVATE => PRIVATE"})
		case "format":
			r.Stdout = []byte("PRIVATE malformed native output")
		case "command":
			r.ExitCode = 1
		case "overflow":
			r.Overflow = true
		case "incomplete":
			r.Incomplete = true
		case "not-started":
			r.Started = false
		case "empty-pid":
			r.Stdout = []byte(strings.Replace(string(r.Stdout), "\tpid = 42\n", "\tpid = \n", 1))
		}
		e.inspectResult = &r
		s.wait = func(context.Context, time.Duration) error {
			t.Fatal("unverified starting response was retried")
			return nil
		}
		var starting *launchdStarting
		if err := s.waitActive(context.Background()); err == nil || errors.As(err, &starting) || s.lastPID != 0 {
			t.Fatal("unverified response recorded a PID or became retryable")
		}
	}
}

func TestLaunchdStartingCancellationRetainsVetoAndSnapshot(t *testing.T) {
	for _, when := range []string{"before-wait", "in-wait"} {
		s, m, e := serviceTestFixture("darwin")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		started, activationQueries, independentCleanup := false, 0, false
		e.onMutation = func(verb string) error {
			if verb == "bootstrap" {
				started = true
				seedXPCProxyPrint(s, e, "42")
			}
			return nil
		}
		s.opts.Exec = stopContractExecutor(func(commandCtx context.Context, c Command) (CommandResult, error) {
			r, err := e.Run(commandCtx, c)
			if started && c.Args[0] == "print" {
				activationQueries++
				if ctx.Err() != nil && commandCtx.Err() == nil {
					independentCleanup = true
				}
				if when == "before-wait" && activationQueries == 1 {
					cancel()
				}
			}
			return r, err
		})
		waits := 0
		s.wait = func(context.Context, time.Duration) error {
			waits++
			cancel()
			return ctx.Err()
		}
		err := s.Install(ctx)
		detail := requireLaunchdEvidence(t, err, "xpcproxy", "positive")
		wantWaits := 1
		if when == "before-wait" {
			wantWaits = 0
		}
		if !errors.Is(err, ErrCanceled) || !errors.Is(err, ErrRollback) || detail.category != categoryCanceled || waits != wantWaits || s.lastPID != 42 || !independentCleanup {
			t.Fatal("cancellation lost startup veto/evidence or cleanup context independence")
		}
		for _, path := range []string{s.binary, s.unit, s.opts.ConfigPath} {
			if !m.files[path].Exists {
				t.Fatal("canceled uncertain startup lost launch inputs")
			}
		}
	}
}

func TestLaunchdStartingThenAbsentHonorsLivePIDVeto(t *testing.T) {
	for _, gone := range []bool{false, true} {
		s, m, e := serviceTestFixture("darwin")
		e.onMutation = func(verb string) error {
			if verb == "bootstrap" {
				seedXPCProxyPrint(s, e, "42")
			}
			return nil
		}
		checks := 0
		s.processGone = func(pid int) (bool, error) {
			checks++
			if pid != 42 {
				t.Fatal("startup PID shutdown obligation was replaced")
			}
			return gone, nil
		}
		waits := 0
		s.wait = func(context.Context, time.Duration) error {
			waits++
			e.inspectResult = nil
			e.active, e.registered = false, false
			return nil
		}
		err := s.Install(context.Background())
		detail := requireLaunchdEvidence(t, err, "xpcproxy", "positive")
		if waits != 50 || checks == 0 || detail.category != categoryActivationTimeout || errors.Is(err, ErrRollback) == gone || errors.Is(err, errServiceInputsRetained) == gone || s.lastPID != 42 {
			t.Fatal("later registration absence bypassed startup PID veto")
		}
		for _, path := range []string{s.binary, s.unit, s.opts.ConfigPath} {
			if m.files[path].Exists == gone {
				t.Fatal("input rollback ignored whether the startup PID was gone")
			}
		}
	}
}

func TestLaunchdStartingPIDReplacementRequiresPriorGone(t *testing.T) {
	for _, next := range []string{"xpcproxy", "running"} {
		for _, prior := range []string{"alive", "inaccessible", "gone"} {
			s, _, _ := serviceTestFixture("darwin")
			_, err := s.inspectLaunchd(launchdContractPrint(s, "xpcproxy", "42", nil), nil, true)
			var starting *launchdStarting
			if !errors.As(err, &starting) || s.lastPID != 42 {
				t.Fatal("initial verified starting PID was not recorded as veto")
			}
			checks := 0
			s.processGone = func(pid int) (bool, error) {
				checks++
				if pid != 42 {
					t.Fatal("checked an unverified replacement PID")
				}
				if prior == "inaccessible" {
					return false, errors.New("PRIVATE process observation failure")
				}
				return prior == "gone", nil
			}
			state, err := s.inspectLaunchd(launchdContractPrint(s, next, "43", nil), nil, true)
			if checks != 1 {
				t.Fatal("PID replacement skipped prior shutdown proof")
			}
			if prior != "gone" {
				detail := requireLaunchdEvidence(t, err, next, "positive")
				if detail.category != categoryPriorProcess || s.lastPID != 42 || state == stateRunning || errors.As(err, &starting) {
					t.Fatal("new PID overwrote unresolved shutdown obligation or became ready/retryable")
				}
			} else if s.lastPID != 43 || (next == "running" && (err != nil || state != stateRunning)) || (next == "xpcproxy" && (!errors.As(err, &starting) || state == stateRunning)) {
				t.Fatal("confirmed-gone prior PID did not allow the new owned observation")
			}
		}
	}
	// Same PID does not need another process query and is still not readiness.
	s, _, _ := serviceTestFixture("darwin")
	s.lastPID = 42
	s.processGone = func(int) (bool, error) { t.Fatal("same PID needed replacement proof"); return false, nil }
	_, err := s.inspectLaunchd(launchdContractPrint(s, "xpcproxy", "42", nil), nil, true)
	var starting *launchdStarting
	if !errors.As(err, &starting) || s.lastPID != 42 {
		t.Fatal("same owned PID lost startup error/veto")
	}
}

func TestLaunchdStartingConflictDuringActivationRetainsInputs(t *testing.T) {
	for _, next := range []string{"xpcproxy", "running"} {
		s, m, e := serviceTestFixture("darwin")
		e.onMutation = func(verb string) error {
			if verb == "bootstrap" {
				seedXPCProxyPrint(s, e, "42")
			}
			return nil
		}
		s.processGone = func(int) (bool, error) { return false, nil }
		waits := 0
		s.wait = func(context.Context, time.Duration) error {
			waits++
			r := launchdContractPrint(s, next, "43", nil)
			e.inspectResult = &r
			return nil
		}
		err := s.Install(context.Background())
		detail := requireLaunchdEvidence(t, err, next, "positive")
		if waits != 1 || detail.category != categoryPriorProcess || s.lastPID != 42 || !errors.Is(err, ErrRollback) || !errors.Is(err, errServiceInputsRetained) {
			t.Fatal("activation retried a PID conflict or discarded its old veto")
		}
		for _, path := range []string{s.binary, s.unit, s.opts.ConfigPath} {
			if !m.files[path].Exists {
				t.Fatal("unresolved competing PID removed launch inputs")
			}
		}
	}
}

func TestLaunchdStartingWrappedPrivacyAndCanceledEntry(t *testing.T) {
	s, _, _ := serviceTestFixture("darwin")
	_, err := s.inspectLaunchd(launchdContractPrint(s, "xpcproxy", "42", nil), nil, true)
	original := err
	err = serviceAt(stageActivation, fmt.Errorf("PRIVATE argv/path/token: %w", errors.Join(err, ErrCanceled, ErrRollback)))
	requireLaunchdEvidence(t, err, "xpcproxy", "positive")
	// %#v of errors.Join may contain hexadecimal pointer addresses. Detect
	// the standalone decimal PID, not coincidental digits inside an address.
	rawPID := regexp.MustCompile(`\b42\b`)
	for _, candidate := range []error{original, err, errors.Join(err, ErrRollback)} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			text := fmt.Sprintf(format, candidate)
			if strings.Contains(text, "PRIVATE") || rawPID.MatchString(text) || strings.Contains(text, "/home/") {
				t.Fatal("starting diagnostics leaked raw process/private data")
			}
		}
	}
	if !errors.Is(err, ErrService) || !errors.Is(err, ErrCanceled) || !errors.Is(err, ErrRollback) {
		t.Fatal("starting error wrapping lost sentinel semantics")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.opts.Exec = stopContractExecutor(func(context.Context, Command) (CommandResult, error) {
		t.Fatal("canceled activation issued a query")
		return CommandResult{}, nil
	})
	if err := s.waitActive(ctx); !errors.Is(err, ErrCanceled) || s.lastPID != 42 {
		t.Fatal("canceled entry lost existing PID veto")
	}
}

func TestLaunchdObservedXPCProxyWaitsForSamePIDRunning(t *testing.T) {
	s, _, e := serviceTestFixture("darwin")
	e.onMutation = func(verb string) error {
		if verb == "bootstrap" {
			r := launchdContractPrint(s, "xpcproxy", "42", []string{"OSLogRateLimit => 64"})
			e.inspectResult = &r
		}
		return nil
	}
	waits := 0
	s.wait = func(ctx context.Context, d time.Duration) error {
		if ctx.Err() != nil || d != 100*time.Millisecond {
			t.Fatal("activation wait budget changed")
		}
		waits++
		r := launchdContractPrint(s, "running", "42", []string{"OSLogRateLimit => 64"})
		e.inspectResult = &r
		return nil
	}
	if err := s.Install(context.Background()); err != nil || waits != 1 || s.lastPID != 42 {
		t.Fatal("owned native xpcproxy observation did not wait for verified running")
	}
}
