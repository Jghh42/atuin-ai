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

func TestLaunchdPIDConflictCannotBeClearedByLaterAbsence(t *testing.T) {
	for _, nextState := range []string{"xpcproxy", "running"} {
		for _, prior := range []string{"alive", "inaccessible"} {
			for _, wasActive := range []bool{false, true} {
				for _, external := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/prior=%t/external=%t", nextState, prior, wasActive, external), func(t *testing.T) {
						s, m, e := serviceTestFixture("darwin")
						if wasActive {
							seedManaged(s, m, e)
						}
						phase, starts, waits := "prior-stop", 0, 0
						checked := []string{}
						installed := map[string]Snapshot{}
						e.onMutation = func(verb string) error {
							if verb == "bootstrap" {
								starts++
								if starts == 1 {
									phase = "starting"
									seedXPCProxyPrint(s, e, "42")
								} else {
									phase = "restored"
									e.inspectResult = nil // A wrongly permitted restoration.
								}
							}
							return nil
						}
						s.processGone = func(pid int) (bool, error) {
							if pid != 42 {
								t.Fatal("untracked new PID must not be mistaken for the sole known obligation")
							}
							checked = append(checked, phase)
							if phase == "conflict" {
								phase = "cleanup"
								if prior == "inaccessible" {
									return false, errors.New("PRIVATE process observation failure")
								}
								return false, nil
							}
							// The OLD PID is now gone, but the separately observed
							// conflicting new PID remains alive and untracked.
							return true, nil
						}
						s.opts.Exec = stopContractExecutor(func(ctx context.Context, c Command) (CommandResult, error) {
							if phase == "cleanup" && c.Args[0] == "print" {
								e.inspectResult = nil
								e.active, e.registered = false, false
							}
							return e.Run(ctx, c)
						})
						s.wait = func(context.Context, time.Duration) error {
							waits++
							phase = "conflict"
							for _, path := range []string{s.binary, s.unit, s.opts.ConfigPath} {
								installed[path] = m.files[path]
							}
							if external {
								_, _ = m.Replace(s.binary, m.files[s.binary], []byte("PRIVATE external replacement"), 0755, false)
								installed[s.binary] = m.files[s.binary]
							}
							r := launchdContractPrint(s, nextState, "43", nil)
							e.inspectResult = &r
							return nil
						}
						err := s.Install(context.Background())
						if !errors.Is(err, ErrRollback) || !errors.Is(err, errServiceInputsRetained) || s.lastPID != 42 || starts != 1 || waits != 1 {
							t.Fatal("later absence/old-PID exit erased an unresolved conflicting observation")
						}
						wantChecks := "conflict"
						if wasActive {
							wantChecks = "prior-stop,conflict"
						}
						if strings.Join(checked, ",") != wantChecks {
							t.Fatal("cleanup tried to clear conflict using only the older PID")
						}
						for path, expected := range installed {
							if !same(expected, m.files[path]) {
								t.Fatal("uncertain conflict removed/restored launch inputs or overwrote an external replacement")
							}
						}
					})
				}
			}
		}
	}
}

func TestLaunchdPIDConflictGoneBeforeReplacementDoesNotLatch(t *testing.T) {
	for _, state := range []string{"xpcproxy", "running"} {
		s, _, _ := serviceTestFixture("darwin")
		_, _ = s.inspectLaunchd(launchdContractPrint(s, "xpcproxy", "42", nil), nil, true)
		s.processGone = func(pid int) (bool, error) {
			if pid != 42 {
				t.Fatal("replacement tested the wrong prior PID")
			}
			return true, nil
		}
		observed, err := s.inspectLaunchd(launchdContractPrint(s, state, "43", nil), nil, true)
		var starting *launchdStarting
		if s.pidUncertain || s.lastPID != 43 || (state == "running" && (err != nil || observed != stateRunning)) || (state == "xpcproxy" && (!errors.As(err, &starting) || observed == stateRunning)) {
			t.Fatal("proved-gone prior process was confused with an unresolved conflict")
		}
	}
}

func TestLaunchdPIDConflictSharedGuardCannotBeWaivedByEmptySlot(t *testing.T) {
	for _, operation := range []string{"running", "stop", "restore", "activate", "install", "confirm-absent", "confirm-inactive"} {
		s, m, _ := serviceTestFixture("linux")
		s.pidUncertain = true // Native Linux does not create the latch; guard is shared.
		s.lastPID = 0
		s.opts.Exec = stopContractExecutor(func(context.Context, Command) (CommandResult, error) {
			t.Fatal("latched invocation issued a supervisor command")
			return CommandResult{}, nil
		})
		s.processGone = func(int) (bool, error) {
			t.Fatal("latched invocation tried to clear uncertainty via one PID")
			return true, nil
		}
		var err error
		switch operation {
		case "running":
			var running bool
			running, err = s.Running(context.Background())
			if running {
				t.Fatal("latched shared guard declared readiness")
			}
		case "stop":
			err = s.Stop(context.Background())
		case "restore":
			err = s.Restore(context.Background())
		case "activate":
			err = s.waitActive(context.Background())
		case "install":
			err = s.Install(context.Background())
		case "confirm-absent":
			err = s.confirmStopped(stateAbsent)
		case "confirm-inactive":
			err = s.confirmStopped(stateInactive)
		}
		var detail *serviceFailure
		if !errors.As(err, &detail) || detail.category != categoryPriorProcess || !s.pidUncertain || s.lastPID != 0 || len(m.writes) != 0 {
			t.Fatal("empty PID slot bypassed sticky process uncertainty")
		}
	}
}

func TestLaunchdPIDConflictStaysUncertainAcrossServiceMethods(t *testing.T) {
	for _, state := range []string{"absent", "running"} {
		for _, operation := range []string{"running", "stop", "restore", "activate", "install", "observe-same"} {
			t.Run(state+"/"+operation, func(t *testing.T) {
				s, m, e := serviceTestFixture("darwin")
				seedManaged(s, m, e)
				_, _ = s.inspectLaunchd(launchdContractPrint(s, "xpcproxy", "42", nil), nil, true)
				s.processGone = func(int) (bool, error) { return false, nil }
				if _, err := s.inspectLaunchd(launchdContractPrint(s, "running", "43", nil), nil, true); err == nil {
					t.Fatal("setup did not encounter a PID conflict")
				}
				checks := 0
				s.processGone = func(int) (bool, error) { checks++; return true, nil }
				e.active, e.registered = state == "running", state == "running"
				e.inspectResult = nil
				e.calls, m.writes = nil, nil
				var err error
				switch operation {
				case "running":
					var ready bool
					ready, err = s.Running(context.Background())
					if ready {
						t.Fatal("conflicted invocation became ready")
					}
				case "stop":
					err = s.Stop(context.Background())
				case "restore":
					err = s.Restore(context.Background())
				case "activate":
					err = s.waitActive(context.Background())
				case "install":
					err = s.Install(context.Background())
				case "observe-same":
					_, err = s.inspectLaunchd(launchdContractPrint(s, "running", "42", nil), nil, true)
				}
				var detail *serviceFailure
				if !errors.As(err, &detail) || detail.category != categoryPriorProcess || s.lastPID != 42 || checks != 0 || len(mutationCalls(e)) != 0 || len(m.writes) != 0 {
					t.Fatal("method cleared sticky uncertainty via absence/same PID/another install")
				}
			})
		}
	}
}
