//go:build linux || darwin

package setup

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Fixed messages only: never expose terminal contents or arbitrary OS errors.
var errTerminalReadiness = errors.New("init terminal readiness failed; completed independent steps are retained")
var errTerminalRead = errors.New("init terminal read failed; completed independent steps are retained")

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
		ready, e := t.waitReadable(fd)
		if errors.Is(e, unix.EINTR) {
			continue
		}
		if ctx.Err() != nil {
			return "", ErrCanceled
		}
		if e != nil {
			return "", e
		}
		if !ready {
			continue
		}
		var b [1]byte
		n, e := unix.Read(fd, b[:])
		if errors.Is(e, unix.EAGAIN) || errors.Is(e, unix.EINTR) {
			continue
		}
		if ctx.Err() != nil {
			return "", ErrCanceled
		}
		if e != nil {
			return "", errTerminalRead
		}
		if n == 0 || b[0] == 3 || b[0] == 4 {
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
