//go:build linux || darwin

package setup

import (
	"errors"
	"strconv"
	"strings"
)

// Diagnostic vocabulary only. These enums confer NO lifecycle authority and
// must never be used to expand accepted launchd states or PID representations.
type launchdStateKind uint8

const (
	launchdStateMissing launchdStateKind = iota
	launchdStateEmpty
	launchdStateRunning
	launchdStateNotRunning
	launchdStateWaiting
	launchdStateScheduled
	launchdStateXPCProxy
	launchdStateSpawning
	launchdStateForking
	launchdStateSpawned
	launchdStateExited
	launchdStateOther
)

func (k launchdStateKind) String() string {
	names := [...]string{"missing", "empty", "running", "not_running", "waiting", "spawn_scheduled", "xpcproxy", "spawning", "forking", "spawned", "exited", "other_nonempty"}
	if int(k) >= len(names) {
		return "unavailable"
	}
	return names[k]
}

func classifyLaunchdState(value string, present bool) launchdStateKind {
	if !present {
		return launchdStateMissing
	}
	switch value {
	case "":
		return launchdStateEmpty
	case "running":
		return launchdStateRunning
	case "not running":
		return launchdStateNotRunning
	case "waiting":
		return launchdStateWaiting
	case "spawn scheduled":
		return launchdStateScheduled
	case "xpcproxy":
		return launchdStateXPCProxy
	case "spawning":
		return launchdStateSpawning
	case "forking":
		return launchdStateForking
	case "spawned":
		return launchdStateSpawned
	case "exited":
		return launchdStateExited
	default:
		return launchdStateOther
	}
}

type launchdPIDClass uint8

const (
	launchdPIDMissing launchdPIDClass = iota
	launchdPIDEmpty
	launchdPIDZero
	launchdPIDPositive
	launchdPIDNegative
	launchdPIDMalformed
	launchdPIDOverflow
	launchdPIDNoncanonical
)

func (p launchdPIDClass) String() string {
	names := [...]string{"missing", "empty", "zero", "positive", "negative", "malformed", "overflow", "noncanonical"}
	if int(p) >= len(names) {
		return "unavailable"
	}
	return names[p]
}

func classifyLaunchdPID(value string, present bool) launchdPIDClass {
	if !present {
		return launchdPIDMissing
	}
	if value == "" {
		return launchdPIDEmpty
	}
	pid, err := strconv.Atoi(value)
	if errors.Is(err, strconv.ErrRange) {
		return launchdPIDOverflow
	}
	if err != nil {
		return launchdPIDMalformed
	}
	if strconv.Itoa(pid) != value {
		return launchdPIDNoncanonical
	}
	if pid == 0 {
		return launchdPIDZero
	}
	if pid < 0 {
		return launchdPIDNegative
	}
	return launchdPIDPositive
}

type launchdStateSnapshot struct {
	state launchdStateKind
	pid   launchdPIDClass
}

// Only waitActive may retry this strictly owned, observed native starting
// condition. It remains an error to every status/control/restoration caller.
// The embedded diagnostic snapshot is immutable and confers no ready state.
type launchdStarting struct{ *serviceFailure }

func (e *launchdStarting) Unwrap() error { return e.serviceFailure }

// Called ONLY at a state/PID rejection after the existing parser and ownership
// checks passed. Store closed enum values, never raw output or field strings.
func launchdStateFailure(data []byte, fields map[string]string) *serviceFailure {
	state, statePresent := fields["state"]
	pid, pidPresent := fields["pid"]
	// launchFields intentionally trims lines before parsing scalars, so a bare
	// "pid =" / "state =" is absent from its map. Distinguish explicit empty
	// root fields for diagnostics WITHOUT changing that parser or its decisions.
	depth := 1
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		switch {
		case line == "}":
			depth--
		case strings.HasSuffix(line, " = {"):
			depth++
		case depth == 1:
			if line == "state =" && !statePresent {
				statePresent = true
			}
			if line == "pid =" && !pidPresent {
				pidPresent = true
			}
		}
	}
	return &serviceFailure{
		category: categoryState, cause: ErrService, hasSnapshot: true,
		snapshot: launchdStateSnapshot{classifyLaunchdState(state, statePresent), classifyLaunchdPID(pid, pidPresent)},
	}
}
