//go:build linux || darwin

package setup

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

type DeviceTerminal struct {
	file *os.File
	// Private synchronization seam: tests still call real unix.Poll on a PTY.
	poll func([]unix.PollFd, int) (int, error)
}

// OpenTerminal is deliberately independent of stdin/stdout/stderr. Refusal
// happens before config, state, directory creation or supervisor invocation.
func OpenTerminal() (*DeviceTerminal, error) {
	fd, e := unix.Open("/dev/tty", unix.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, errors.New("init requires an interactive controlling terminal; no changes made")
	}
	if !isTerminal(fd) {
		unix.Close(fd)
		return nil, errors.New("init requires an interactive controlling terminal; no changes made")
	}
	return &DeviceTerminal{file: os.NewFile(uintptr(fd), "controlling terminal")}, nil
}
func (t *DeviceTerminal) Close() error                { return t.file.Close() }
func (t *DeviceTerminal) Write(b []byte) (int, error) { return t.file.Write(b) }
func (t *DeviceTerminal) Answer(ctx context.Context) (string, error) {
	var line []byte
	fd := int(t.file.Fd())
	for {
		if ctx.Err() != nil {
			return "", ErrCanceled
		}
		p := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		poll := t.poll
		if poll == nil {
			poll = unix.Poll
		}
		_, e := poll(p, 100)
		if errors.Is(e, unix.EINTR) {
			continue
		}
		if e != nil {
			return "", ErrCanceled
		}
		if p[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
			return "", ErrCanceled
		}
		if p[0].Revents&unix.POLLIN == 0 {
			continue
		}
		var b [1]byte
		n, e := unix.Read(fd, b[:])
		if errors.Is(e, unix.EAGAIN) || errors.Is(e, unix.EINTR) {
			continue
		}
		if e != nil || n == 0 || b[0] == 3 || b[0] == 4 {
			return "", ErrCanceled
		}
		if b[0] == '\n' {
			return string(line), nil
		}
		if len(line) >= 128 {
			return "", ErrCanceled
		}
		line = append(line, b[0])
	}
}
