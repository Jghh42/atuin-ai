//go:build linux || darwin

package setup

import "golang.org/x/sys/unix"

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
