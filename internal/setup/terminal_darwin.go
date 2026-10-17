package setup

import (
	"os"

	"golang.org/x/sys/unix"
)

type DeviceTerminal struct {
	file *os.File
	// Private synchronization seam; production always uses the platform syscall.
	selectRead func(int, *unix.FdSet, *unix.Timeval) (int, error)
}

func isTerminal(fd int) bool { _, e := unix.IoctlGetTermios(fd, unix.TIOCGETA); return e == nil }

func (t *DeviceTerminal) waitReadable(fd int) (bool, error) {
	// Darwin poll uses kqueue, which does not support the /dev/tty alias.
	// select dispatches through the controlling terminal's select operation.
	// Check BEFORE FdSet.Set: x/sys does not bounds-check its bitset index.
	if fd < 0 || fd >= unix.FD_SETSIZE {
		return false, errTerminalReadiness
	}
	var read unix.FdSet
	read.Set(fd)
	timeout := unix.Timeval{Usec: 100000}
	selectRead := t.selectRead
	if selectRead == nil {
		selectRead = func(n int, r *unix.FdSet, timeout *unix.Timeval) (int, error) {
			return unix.Select(n, r, nil, nil, timeout)
		}
	}
	n, err := selectRead(fd+1, &read, &timeout)
	if err == unix.EINTR {
		return false, unix.EINTR
	}
	if err != nil {
		return false, errTerminalReadiness
	}
	return n > 0 && read.IsSet(fd), nil
}
