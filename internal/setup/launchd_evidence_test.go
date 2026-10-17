//go:build linux || darwin

package setup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func requireLaunchdEvidence(t *testing.T, err error, state, pid string) *serviceFailure {
	t.Helper()
	var detail *serviceFailure
	if !errors.Is(err, ErrService) || !errors.As(err, &detail) || !detail.hasSnapshot || detail.snapshot.state.String() != state || detail.snapshot.pid.String() != pid {
		t.Fatal("rejected root state/PID did not retain its closed evidence")
	}
	return detail
}

func TestLaunchdFailureEvidenceRootClassesWithoutAcceptanceChanges(t *testing.T) {
	cases := []struct {
		state, pid, stateKind, pidClass string
	}{
		{"running", "", "running", "missing"},
		{"running", "0", "running", "zero"},
		{"running", "-42", "running", "negative"},
		{"running", "PRIVATE-PID", "running", "malformed"},
		{"running", "999999999999999999999999", "running", "overflow"},
		{"running", "-0", "running", "noncanonical"},
		{"running", "000", "running", "noncanonical"},
		{"waiting", "42", "waiting", "positive"},
		{"not running", "42", "not_running", "positive"},
		{"spawn scheduled", "42", "spawn_scheduled", "positive"},
		{"xpcproxy", "42", "xpcproxy", "positive"},
		{"spawning", "", "spawning", "missing"},
		{"forking", "42", "forking", "positive"},
		{"spawned", "", "spawned", "missing"},
		{"exited", "", "exited", "missing"},
		{"PRIVATE-STATE /private/path token=PRIVATE", "PRIVATE-PID", "other_nonempty", "malformed"},
		{"", "", "empty", "missing"},
	}
	for _, test := range cases {
		s, _, _ := serviceTestFixture("darwin")
		r := launchdContractPrint(s, test.state, test.pid, []string{"OSLogRateLimit => 64"})
		state, err := s.inspectLaunchd(r, nil, true)
		if state == stateRunning || state == stateInactive || s.lastPID != 0 {
			t.Fatal("diagnostic classification changed rejected-state/PID authority")
		}
		requireLaunchdEvidence(t, err, test.stateKind, test.pidClass)
		if strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("private rejected value escaped diagnostics")
		}
	}
	for _, field := range []string{"pid", "state"} {
		s, _, _ := serviceTestFixture("darwin")
		r := launchdContractPrint(s, "running", "", nil)
		wantState, wantPID := "running", "empty"
		if field == "pid" {
			r.Stdout = []byte(strings.Replace(string(r.Stdout), "\targuments = {", "\tpid = \n\targuments = {", 1))
		} else {
			r.Stdout = []byte(strings.Replace(string(r.Stdout), "\tstate = running\n", "", 1))
			wantState, wantPID = "missing", "missing"
		}
		_, err := s.inspectLaunchd(r, nil, true)
		requireLaunchdEvidence(t, err, wantState, wantPID)
	}
	// Existing Atoi acceptance of positive noncanonical PID text stays intact.
	for _, pid := range []string{"42", "+42", "042"} {
		s, _, _ := serviceTestFixture("darwin")
		state, err := s.inspectLaunchd(launchdContractPrint(s, "running", pid, nil), nil, true)
		if err != nil || state != stateRunning || s.lastPID != 42 {
			t.Fatal("diagnostic PID classes narrowed existing positive-PID acceptance")
		}
	}
	for _, state := range []string{"waiting", "not running", "spawn scheduled"} {
		s, _, _ := serviceTestFixture("darwin")
		if got, err := s.inspectLaunchd(launchdContractPrint(s, state, "", nil), nil, true); err != nil || got != stateInactive {
			t.Fatal("diagnostic capture changed accepted nonrunning state")
		}
	}
}

func TestLaunchdFailureEvidenceOnlyAfterValidatedOwnership(t *testing.T) {
	for _, kind := range []string{"format", "registration", "environment", "command"} {
		s, _, _ := serviceTestFixture("darwin")
		r := launchdContractPrint(s, "running", "", nil)
		switch kind {
		case "format":
			r.Stdout = []byte("PRIVATE invalid native output")
		case "registration":
			r.Stdout = []byte(strings.Replace(string(r.Stdout), "program = "+s.binary, "program = /PRIVATE", 1))
		case "environment":
			r = launchdContractPrint(s, "running", "", []string{"PRIVATE => PRIVATE"})
		case "command":
			r.ExitCode = 1
		}
		_, err := s.inspectLaunchd(r, nil, true)
		var detail *serviceFailure
		if err == nil || (errors.As(err, &detail) && detail.hasSnapshot) {
			t.Fatal("unvalidated response acquired state evidence")
		}
		w, auth, service, _ := fixture("y", "n")
		auth.ready = true
		service.installErr = serviceAt(stageActivation, err)
		_ = w.Run(context.Background())
		if strings.Contains(w.TTY.(*terminalFixture).String(), "Service state evidence:") {
			t.Fatal("wizard invented evidence for an earlier failure")
		}
	}
}

func TestLaunchdFailureEvidenceSurvivesCleanupAndWrapping(t *testing.T) {
	for _, unknownCleanup := range []bool{false, true} {
		s, _, e := serviceTestFixture("darwin")
		started, queries := false, 0
		e.onMutation = func(verb string) error {
			if verb == "bootstrap" {
				started = true
			}
			return nil
		}
		s.opts.Exec = stopContractExecutor(func(ctx context.Context, c Command) (CommandResult, error) {
			if started && c.Args[0] == "print" {
				queries++
				if queries == 1 {
					return launchdContractPrint(s, "forking", "42", nil), nil
				}
				if unknownCleanup {
					return launchdContractPrint(s, "running", "", nil), nil
				}
				e.active, e.registered = false, false
			}
			return e.Run(ctx, c)
		})
		err := s.Install(context.Background())
		detail := requireLaunchdEvidence(t, err, "forking", "positive")
		if detail.stage != stageActivation || errors.Is(err, ErrRollback) != unknownCleanup {
			t.Fatal("primary activation failure or cleanup result changed")
		}
		wrapped := serviceAt(stageActivation, fmt.Errorf("PRIVATE wrapping: %w", errors.Join(err, ErrCanceled)))
		requireLaunchdEvidence(t, wrapped, "forking", "positive")
		if !errors.Is(wrapped, ErrCanceled) || !errors.Is(wrapped, ErrService) || errors.Is(wrapped, ErrRollback) != unknownCleanup {
			t.Fatal("state evidence wrapping lost sentinel semantics")
		}
	}
	// Snapshot fields are values: a wrapper does not alias mutable raw data or
	// the inner error's enum fields, even under an internal test mutation.
	s, _, _ := serviceTestFixture("darwin")
	r := launchdContractPrint(s, "running", "PRIVATE-PID", nil)
	_, err := s.inspectLaunchd(r, nil, true)
	inner := requireLaunchdEvidence(t, err, "running", "malformed")
	wrapped := serviceAt(stageActivation, errors.Join(err, ErrRollback, ErrCanceled))
	inner.snapshot = launchdStateSnapshot{launchdStateExited, launchdPIDZero}
	requireLaunchdEvidence(t, wrapped, "running", "malformed")
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, wrapped), "PRIVATE") {
			t.Fatal("error formatting exposed raw rejected state/PID")
		}
	}
}

func TestLaunchdFailureEvidenceClosedVocabularyAndPrivateWizard(t *testing.T) {
	if launchdStateKind(255).String() != "unavailable" || launchdPIDClass(255).String() != "unavailable" {
		t.Fatal("out-of-range evidence enum was not closed")
	}
	if classifyLaunchdState("PRIVATE", true) != launchdStateOther || classifyLaunchdPID("+42", true) != launchdPIDNoncanonical {
		t.Fatal("diagnostic-only classifier lost unknown/noncanonical distinction")
	}
	s, _, _ := serviceTestFixture("darwin")
	r := launchdContractPrint(s, "PRIVATE token=/private/provider", "PRIVATE-PID", nil)
	_, err := s.inspectLaunchd(r, nil, true)
	err = serviceAt(stageActivation, fmt.Errorf("PRIVATE raw wrapper: %w", errors.Join(err, ErrRollback, ErrCanceled)))
	for _, candidate := range []error{err, errors.Join(err, ErrRollback)} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			if strings.Contains(fmt.Sprintf(format, candidate), "PRIVATE") {
				t.Fatal("wrapped/joined diagnostic exposed private data")
			}
		}
	}
	w, auth, service, _ := fixture("y", "n")
	auth.ready = true
	service.installErr = err
	_ = w.Run(context.Background())
	output := w.TTY.(*terminalFixture).String()
	if !strings.Contains(output, "state_kind=other_nonempty pid_class=malformed") || strings.Contains(output, "PRIVATE") || strings.Contains(output, "/private") {
		t.Fatal("wizard lost fixed evidence or exposed private rejected values")
	}
}

func TestLaunchdFailureEvidenceExactReadback(t *testing.T) {
	s, _, _ := serviceTestFixture("darwin")
	r := launchdContractPrint(s, "running", "", []string{"OSLogRateLimit => 64"})
	_, err := s.inspectLaunchd(r, nil, true)
	if err == nil {
		t.Fatal("running without a PID was accepted")
	}
	w, auth, service, _ := fixture("y", "n")
	auth.ready = true
	service.installErr = serviceAt(stageActivation, err)
	if w.Run(context.Background()) == nil {
		t.Fatal("service failure was hidden")
	}
	output := w.TTY.(*terminalFixture).String()
	if !strings.Contains(output, "state_kind=running pid_class=missing") {
		t.Fatal("exact rejected state/PID evidence was lost")
	}
}
