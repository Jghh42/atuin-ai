package setup

import (
	"os"

	"golang.org/x/sys/unix"
)

type DeviceTerminal struct {
	file *os.File
	// Private synchronization seam; production always uses the platform syscall.
	poll func([]unix.PollFd, int) (int, error)
}

func isTerminal(fd int) bool { _, e := unix.IoctlGetTermios(fd, unix.TCGETS); return e == nil }

func (t *DeviceTerminal) waitReadable(fd int) (bool, error) {
	if fd < 0 || int64(fd) > 1<<31-1 {
		return false, errTerminalReadiness
	}
	p := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	poll := t.poll
	if poll == nil {
		poll = unix.Poll
	}
	_, err := poll(p, 100)
	if err == unix.EINTR {
		return false, unix.EINTR
	}
	if err != nil {
		return false, errTerminalReadiness
	}
	if p[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
		return false, ErrCanceled
	}
	return p[0].Revents&unix.POLLIN != 0, nil
}
