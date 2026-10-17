//go:build linux || darwin

package setup

import (
	"errors"
	"fmt"
)

// Closed enums are the entire public-facing diagnostic vocabulary. Causes may
// contain private OS/command details; only Unwrap exposes them to internal logic.
type serviceStage uint8

const (
	stageUnknown serviceStage = iota
	stagePreflight
	stageSource
	stageBinary
	stageConfig
	stageInspect
	stageStop
	stageSourceRecheck
	stageWriteBinary
	stageWriteConfig
	stageWriteUnit
	stageVerifyWrites
	stageBootstrap
	stageActivation
	stageRollback
)

func (s serviceStage) String() string {
	names := [...]string{"unknown", "preflight", "source_snapshot", "installed_binary", "config_snapshot", "inspect_before", "stop_prior", "source_recheck", "write_binary", "write_config", "write_unit", "verify_writes", "bootstrap", "verify_active", "rollback"}
	if int(s) >= len(names) {
		return names[stageUnknown]
	}
	return names[s]
}

type serviceCategory uint8

const (
	categoryUnavailable serviceCategory = iota
	categoryUnsafeInput
	categoryIdentity
	categoryChanged
	categoryNotStarted
	categoryCanceled
	categoryOverflow
	categoryIncomplete
	categoryCommandExit
	categoryFormat
	categoryRegistration
	categoryEnvironment
	categoryState
	categoryActivationTimeout
	categoryWrite
	categoryRollback
)

func (c serviceCategory) String() string {
	names := [...]string{"unavailable", "unsafe_input", "identity_mismatch", "snapshot_changed", "command_not_started", "canceled", "overflow", "incomplete", "command_exit", "unknown_format", "registration_mismatch", "environment_mismatch", "unknown_state", "activation_timeout", "write_failed", "rollback_uncertain"}
	if int(c) >= len(names) {
		return names[0]
	}
	return names[c]
}

type serviceFailure struct {
	stage       serviceStage
	category    serviceCategory
	cause       error
	hasSnapshot bool
	snapshot    launchdStateSnapshot
}

func (e *serviceFailure) Error() string {
	return fmt.Sprintf("user service failed (stage=%s category=%s)", e.stage, e.category)
}
func (e *serviceFailure) Unwrap() error { return e.cause }
func (e *serviceFailure) Format(s fmt.State, _ rune) {
	_, _ = s.Write([]byte(e.Error()))
}

func serviceCause(category serviceCategory, cause error) error {
	return &serviceFailure{category: category, cause: errors.Join(ErrService, cause)}
}
func serviceAt(stage serviceStage, err error) error {
	category := categoryUnavailable
	if errors.Is(err, ErrCanceled) {
		category = categoryCanceled
	}
	wrapped := &serviceFailure{stage: stage, category: category, cause: err}
	var failure *serviceFailure
	if errors.As(err, &failure) {
		wrapped.category = failure.category
		// Copy by value from the primary error. Later cleanup inspections must
		// never replace the snapshot from the exact failing activation query.
		wrapped.hasSnapshot = failure.hasSnapshot
		wrapped.snapshot = failure.snapshot
	}
	return wrapped
}
func commandFailure(r CommandResult) error {
	switch {
	case r.Canceled:
		return serviceCause(categoryCanceled, ErrCanceled)
	case r.Overflow:
		return serviceCause(categoryOverflow, ErrService)
	case !r.Started:
		return serviceCause(categoryNotStarted, ErrService)
	case r.Incomplete || r.ExitCode < 0:
		return serviceCause(categoryIncomplete, ErrService)
	default:
		return serviceCause(categoryCommandExit, ErrService)
	}
}

var errServiceInputsRetained = errors.New("service launch inputs not rolled back because registration or stop is unverified")
