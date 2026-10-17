//go:build linux || darwin

package setup

import "golang.org/x/sys/unix"

// A conflicting positive PID that cannot replace the previous verified PID
// leaves an unresolved obligation not representable by lastPID alone. This
// invocation must not clear it through absence, old-PID exit or another install.
func (s *UserService) processCertainty() error {
	if s.pidUncertain {
		return serviceCause(categoryPriorProcess, ErrService)
	}
	return nil
}

// Central shutdown proof for Stop, restoration and rollback. Even an empty
// lastPID slot cannot waive a previously latched conflicting observation.
func (s *UserService) knownProcessGone() error {
	if err := s.processCertainty(); err != nil {
		return err
	}
	if s.lastPID > 0 {
		gone, err := s.processGone(s.lastPID)
		if err != nil || !gone {
			return serviceCause(categoryState, ErrService)
		}
	}
	return nil
}

// Signal zero observes only the previously VERIFIED owned PID; it sends no
// terminating signal. A live/reused/inaccessible PID never authorizes restart.
// No process discovery, name matching, process environment or unrelated kill.
func managedProcessGone(pid int) (bool, error) {
	if pid <= 0 {
		return false, ErrService
	}
	err := unix.Kill(pid, 0)
	if err == unix.ESRCH {
		return true, nil
	}
	if err == nil || err == unix.EPERM {
		return false, nil
	}
	return false, ErrService
}
