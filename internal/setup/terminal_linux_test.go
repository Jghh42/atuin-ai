package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func syntheticPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		master.Close()
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		t.Fatal(err)
	}
	return master, slave
}
func TestDeviceTerminalActualAnswerPTYBoundaries(t *testing.T) {
	for _, kind := range []string{"canonical-EOF", "hangup", "cancel-during-poll", "oversized", "exact-limit"} {
		t.Run(kind, func(t *testing.T) {
			master, slave := syntheticPTY(t)
			ctx, cancel := context.WithCancel(context.Background())
			entered := make(chan struct{})
			var once sync.Once
			terminal := &DeviceTerminal{file: slave, poll: func(p []unix.PollFd, timeout int) (int, error) {
				once.Do(func() { close(entered) })
				return unix.Poll(p, timeout)
			}}
			type result struct {
				answer string
				err    error
			}
			results := make(chan result, 1)
			done := make(chan struct{})
			go func() { defer close(done); answer, err := terminal.Answer(ctx); results <- result{answer, err} }()
			defer func() { cancel(); master.Close(); slave.Close(); <-done }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("Answer never entered real poll")
			}
			switch kind {
			case "canonical-EOF":
				if _, err := master.Write([]byte{4}); err != nil {
					t.Fatal(err)
				}
			case "hangup":
				if err := master.Close(); err != nil {
					t.Fatal(err)
				}
			case "cancel-during-poll":
				cancel()
			case "oversized":
				if _, err := master.Write([]byte(strings.Repeat("x", 129) + "\n")); err != nil {
					t.Fatal(err)
				}
			case "exact-limit":
				if _, err := master.Write([]byte(strings.Repeat("x", 128) + "\n")); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case r := <-results:
				if kind == "exact-limit" {
					if r.err != nil || len(r.answer) != 128 {
						t.Fatal("bounded valid line refused")
					}
				} else if !errors.Is(r.err, ErrCanceled) || r.answer != "" {
					t.Fatal("TTY boundary did not cancel safely", kind)
				}
			case <-time.After(time.Second):
				t.Fatal("real PTY Answer did not terminate")
			}
		})
	}
}
