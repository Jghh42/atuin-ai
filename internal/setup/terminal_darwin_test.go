package setup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func syntheticPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	master := os.NewFile(uintptr(fd), "synthetic PTY master")
	// Test-only Python provides the ioctl's 128-byte output buffer. The fd is
	// our newly allocated PTY, never the developer/runner controlling terminal.
	// Missing Python/ioctl support is a failure, not a skipped native test.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "-I", "-B", "-c", `
import fcntl, os
fcntl.ioctl(3, 0x20007454, 0)  # TIOCPTYGRANT
fcntl.ioctl(3, 0x20007452, 0)  # TIOCPTYUNLK
name = fcntl.ioctl(3, 0x40807453, bytes(128))  # TIOCPTYGNAME
print(os.fsdecode(name.split(b'\0', 1)[0]))
`)
	cmd.ExtraFiles = []*os.File{master}
	name, err := cmd.Output()
	if err != nil {
		master.Close()
		t.Fatal("synthetic Darwin PTY ioctl helper failed", err)
	}
	path := strings.TrimSpace(string(name))
	if !strings.HasPrefix(path, "/dev/ttys") || strings.ContainsAny(path, "\r\n\x00") {
		master.Close()
		t.Fatal("synthetic Darwin PTY returned an invalid slave name")
	}
	slave, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		t.Fatal(err)
	}
	return master, slave
}

func TestDeviceTerminalDarwinSelectBoundaries(t *testing.T) {
	for _, fd := range []int{-1, unix.FD_SETSIZE, unix.FD_SETSIZE + 1} {
		terminal := &DeviceTerminal{selectRead: func(int, *unix.FdSet, *unix.Timeval) (int, error) {
			t.Fatal("out-of-range descriptor reached select")
			return 0, nil
		}}
		if _, err := terminal.waitReadable(fd); err != errTerminalReadiness {
			t.Fatal("descriptor bounds were not refused")
		}
	}
	for _, fd := range []int{0, unix.FD_SETSIZE - 1} {
		calls := 0
		terminal := &DeviceTerminal{selectRead: func(n int, read *unix.FdSet, timeout *unix.Timeval) (int, error) {
			calls++
			if n != fd+1 || !read.IsSet(fd) || timeout.Sec != 0 || timeout.Usec != 100000 {
				t.Fatal("incorrect or reused select arguments")
			}
			read.Zero()
			timeout.Usec = 0
			return 0, nil
		}}
		for range 2 {
			if ready, err := terminal.waitReadable(fd); ready || err != nil {
				t.Fatal("timeout interpreted as input")
			}
		}
		if calls != 2 {
			t.Fatal("missing repeated select")
		}
	}
	terminal := &DeviceTerminal{selectRead: func(int, *unix.FdSet, *unix.Timeval) (int, error) {
		return 0, unix.EBADF
	}}
	if _, err := terminal.waitReadable(0); err != errTerminalReadiness {
		t.Fatal("select failure leaked or became cancellation")
	}
}

func TestDeviceTerminalDarwinInterruptedWait(t *testing.T) {
	master, slave := syntheticPTY(t)
	defer master.Close()
	defer slave.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	terminal := &DeviceTerminal{file: slave, selectRead: func(n int, read *unix.FdSet, timeout *unix.Timeval) (int, error) {
		calls++
		if calls == 1 {
			return 0, unix.EINTR
		}
		cancel()
		return 1, nil // Cancellation must win even when readiness is reported.
	}}
	if answer, err := terminal.Answer(ctx); answer != "" || !errors.Is(err, ErrCanceled) || calls != 2 {
		t.Fatal("interrupted wait did not retry or consumed input after cancellation")
	}
	calls = 0
	if _, err := terminal.Answer(ctx); !errors.Is(err, ErrCanceled) || calls != 0 {
		t.Fatal("canceled context entered select")
	}

	// Have a complete line ready, then cancel precisely before its newline.
	// The newline must remain unread rather than authorize the answer.
	if _, err := master.Write([]byte("Y\n")); err != nil {
		t.Fatal(err)
	}
	ctx, cancelLine := context.WithCancel(context.Background())
	defer cancelLine()
	calls = 0
	terminal.selectRead = func(n int, read *unix.FdSet, timeout *unix.Timeval) (int, error) {
		ready, err := unix.Select(n, read, nil, nil, timeout)
		if ready > 0 && err == nil {
			calls++
			if calls == 2 {
				cancelLine()
			}
		}
		return ready, err
	}
	if answer, err := terminal.Answer(ctx); answer != "" || !errors.Is(err, ErrCanceled) || calls != 2 {
		t.Fatal("canceled newline authorized an answer")
	}
	terminal.selectRead = nil
	remaining, cancelRemaining := context.WithTimeout(context.Background(), time.Second)
	defer cancelRemaining()
	if answer, err := terminal.Answer(remaining); answer != "" || err != nil {
		t.Fatal("newline was consumed after cancellation")
	}
}
