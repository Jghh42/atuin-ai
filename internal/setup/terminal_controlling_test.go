//go:build linux || darwin

package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type terminalTestMessage struct {
	Phase  string
	Answer string
	Error  string
}

// This helper never runs init, opens application state, or invokes a supervisor.
// Its only terminal is a newly allocated synthetic controlling PTY. Stdout and
// stderr are pipes, so OpenTerminal must really use /dev/tty, not standard input.
func TestDeviceTerminalHelper(t *testing.T) {
	mode := os.Getenv("ATUIN_SETUP_TERMINAL_HELPER")
	if mode == "" {
		return
	}
	signal.Ignore(syscall.SIGHUP)
	terminal, err := OpenTerminal()
	encode := json.NewEncoder(os.Stdout)
	send := func(m terminalTestMessage) {
		if err := encode.Encode(m); err != nil {
			t.Fatal(err)
		}
	}
	if mode == "no-tty" {
		if terminal != nil || err == nil {
			t.Fatal("accepted a process without a controlling terminal")
		}
		send(terminalTestMessage{Phase: "refused"})
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	flags, err := unix.FcntlInt(terminal.file.Fd(), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_NONBLOCK == 0 {
		t.Fatal("controlling terminal lost nonblocking reads")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	control := os.NewFile(3, "synthetic cancellation control")
	defer control.Close()
	go func() {
		var b [1]byte
		if _, err := control.Read(b[:]); err == nil {
			cancel()
		}
	}()
	send(terminalTestMessage{Phase: "ready"})
	if mode == "wizard" {
		w, _, _, events := fixture()
		w.TTY = terminal
		if err := w.Run(ctx); err != nil {
			t.Fatal(err)
		}
		if !hasEvent(*events, "login") || hasEvent(*events, "install") || hasEvent(*events, "config") {
			t.Fatal("synthetic wizard consent did not remain independent")
		}
		send(terminalTestMessage{Phase: "result"})
		return
	}
	count := 1
	if mode == "sequential" {
		count = 3
	}
	for range count {
		answer, err := terminal.Answer(ctx)
		m := terminalTestMessage{Phase: "result", Answer: answer}
		switch {
		case err == nil:
		case errors.Is(err, ErrCanceled):
			m.Error = "canceled"
		case errors.Is(err, errTerminalRead):
			m.Error = "read"
		case errors.Is(err, errTerminalReadiness):
			m.Error = "readiness"
		default:
			t.Fatal("unexpected terminal error category")
		}
		send(m)
	}
}

func TestDeviceTerminalControllingTTY(t *testing.T) {
	for _, mode := range []string{"no-tty", "accept", "enter", "decline", "sequential", "cancel", "EOF", "hangup", "exact-limit", "oversized", "wizard"} {
		t.Run(mode, func(t *testing.T) {
			var master, slave *os.File
			if mode != "no-tty" {
				master, slave = syntheticPTY(t)
				defer master.Close()
				defer slave.Close()
			}
			controlRead, controlWrite, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer controlRead.Close()
			defer controlWrite.Close()
			cmd := exec.Command(os.Args[0], "-test.run=^TestDeviceTerminalHelper$", "-test.count=1")
			cmd.Env = []string{"ATUIN_SETUP_TERMINAL_HELPER=" + mode, "GORACE=atexit_sleep_ms=0"}
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			if slave != nil {
				cmd.Stdin = slave
				cmd.SysProcAttr.Setctty = true
				cmd.SysProcAttr.Ctty = 0
			}
			cmd.ExtraFiles = []*os.File{controlRead}
			var stderr strings.Builder
			cmd.Stderr = &stderr
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			defer func() {
				if !waited {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			}()
			finish := func() {
				t.Helper()
				done := make(chan error, 1)
				go func() { done <- cmd.Wait() }()
				select {
				case err := <-done:
					waited = true
					if err != nil {
						t.Fatal("synthetic terminal helper failed", err)
					}
				case <-time.After(5 * time.Second):
					_ = cmd.Process.Kill()
					<-done
					waited = true
					t.Fatal("synthetic terminal helper did not exit")
				}
			}
			controlRead.Close()
			if slave != nil {
				slave.Close()
			}
			messages := make(chan terminalTestMessage, 8)
			decodeDone := make(chan error, 1)
			go func() {
				decoder := json.NewDecoder(stdout)
				count := 2
				if mode == "no-tty" {
					count = 1
				} else if mode == "sequential" {
					count = 4
				}
				for range count {
					var m terminalTestMessage
					if err := decoder.Decode(&m); err != nil {
						decodeDone <- err
						return
					}
					messages <- m
				}
			}()
			await := func() terminalTestMessage {
				t.Helper()
				select {
				case m := <-messages:
					return m
				case err := <-decodeDone:
					t.Fatalf("terminal helper ended before result: %v", err)
				case <-time.After(5 * time.Second):
					t.Fatal("terminal helper did not finish")
				}
				return terminalTestMessage{}
			}
			if mode == "no-tty" {
				if m := await(); m.Phase != "refused" {
					t.Fatal("missing controlling-terminal refusal")
				}
				finish()
				return
			}
			if m := await(); m.Phase != "ready" {
				t.Fatal("helper did not open /dev/tty")
			}
			// In particular, Darwin must not return an immediate poll rejection.
			select {
			case m := <-messages:
				t.Fatalf("answered without input: category=%s", m.Error)
			case <-decodeDone:
				t.Fatal("helper exited without input")
			case <-time.After(250 * time.Millisecond):
			}
			input := ""
			expected := []string{""}
			switch mode {
			case "accept":
				input, expected = "Y\n", []string{"Y"}
			case "enter":
				input = "\n"
			case "decline":
				input, expected = "N\n", []string{"N"}
			case "sequential":
				input, expected = "Y\n\nN\n", []string{"Y", "", "N"}
			case "cancel":
				if _, err := controlWrite.Write([]byte{1}); err != nil {
					t.Fatal(err)
				}
			case "EOF":
				input = "\x04"
			case "hangup":
				master.Close()
			case "exact-limit":
				expected = []string{strings.Repeat("x", 128)}
				input = expected[0] + "\n"
			case "oversized":
				input = strings.Repeat("x", 129) + "\n"
			case "wizard":
				input = "y\nn\n\n"
			}
			if input != "" {
				if _, err := io.WriteString(master, input); err != nil {
					t.Fatal(err)
				}
			}
			for _, answer := range expected {
				m := await()
				if m.Phase != "result" {
					t.Fatal("missing answer result")
				}
				switch mode {
				case "cancel", "EOF", "oversized":
					if m.Error != "canceled" || m.Answer != "" {
						t.Fatal("input boundary did not cancel safely")
					}
				case "hangup":
					if (m.Error != "canceled" && m.Error != "read") || m.Answer != "" {
						t.Fatal("terminal hangup did not terminate safely")
					}
				default:
					if m.Error != "" || m.Answer != answer {
						t.Fatal("valid synthetic answer refused or mixed across prompts")
					}
				}
			}
			finish()
		})
	}
}

func TestDeviceTerminalSafeFailureCategories(t *testing.T) {
	for _, category := range []error{errTerminalReadiness, errTerminalRead, ErrCanceled} {
		terminal := &terminalFixture{err: fmt.Errorf("untrusted synthetic details: %w", category)}
		_, err := ask(context.Background(), terminal, "synthetic prompt")
		if err != category {
			t.Fatal("fixed terminal error category was lost or arbitrary details escaped")
		}
	}
}
