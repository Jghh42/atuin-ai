package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestInitIsolatedTerminalHelper(t *testing.T) {
	mode := os.Getenv("ATUIN_INIT_TEST_HELPER")
	if mode == "" {
		return
	}
	var output bytes.Buffer
	err := runWith(context.Background(), []string{"init"}, &output, commandDeps{})
	if mode == "no-tty" {
		if err == nil || !strings.Contains(err.Error(), "controlling terminal") {
			os.Exit(20)
		}
	} else if os.Geteuid() == 0 {
		if err == nil || !strings.Contains(err.Error(), "user-only workflow") {
			os.Exit(23)
		}
	} else if err != nil {
		os.Exit(21)
	}
	if output.Len() != 0 {
		os.Exit(22)
	}
	os.Exit(0)
}
func isolatedCommand(t *testing.T, mode, root string) *exec.Cmd {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestInitIsolatedTerminalHelper$")
	// An allowlisted entirely synthetic HOME/config environment, not os.Environ.
	cmd.Env = []string{"HOME=" + root, "XDG_CONFIG_HOME=" + root, "ATUIN_INIT_TEST_HELPER=" + mode, "PATH=/usr/bin:/bin"}
	return cmd
}
func TestInitNoControllingTTYHasNoStateSideEffects(t *testing.T) {
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	cmd := isolatedCommand(t, "no-tty", root)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("detached helper failed: %v %s", e, out)
	}
	names, e := os.ReadDir(root)
	if e != nil || len(names) != 0 {
		t.Fatal("noninteractive init created state")
	}
}
func TestInitRealPTYMissingLoginDecline(t *testing.T) {
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	master, e := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer master.Close()
	if e = unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); e != nil {
		t.Fatal(e)
	}
	number, e := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if e != nil {
		t.Fatal(e)
	}
	slave, e := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer slave.Close()
	cmd := isolatedCommand(t, "decline", root)
	cmd.Stdin = slave
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	if os.Geteuid() == 0 {
		if e = cmd.Wait(); e != nil {
			t.Fatal(e)
		}
		names, e := os.ReadDir(root)
		if e != nil || len(names) != 0 {
			t.Fatal("root setup mutated state")
		}
		return
	}
	result := make(chan error, 1)
	go func() {
		var transcript strings.Builder
		buf := make([]byte, 1024)
		for {
			n, e := master.Read(buf)
			transcript.Write(buf[:n])
			if strings.Contains(transcript.String(), "Log in now? [y/N]") {
				_, e = master.Write([]byte("n\n"))
				result <- e
				return
			}
			if e != nil {
				result <- e
				return
			}
			if transcript.Len() > 4096 {
				result <- fmt.Errorf("unbounded terminal output")
				return
			}
		}
	}()
	select {
	case e = <-result:
		if e != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatal(e)
		}
	case <-time.After(4 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("missing native terminal prompt")
	}
	if e = cmd.Wait(); e != nil {
		t.Fatal(e)
	}
	// Only the application's private empty store/lock may have been created.
	if _, e = os.Stat(filepath.Join(root, "atuin", "config.toml")); !os.IsNotExist(e) {
		t.Fatal("Atuin config changed after decline")
	}
	if _, e = os.Stat(filepath.Join(root, ".local", "bin", "atuin-ai-server")); !os.IsNotExist(e) {
		t.Fatal("stable binary created after decline")
	}
	state := filepath.Join(root, "atuin-ai", "credentials")
	names, e := os.ReadDir(state)
	if e != nil {
		t.Fatal(e)
	}
	if len(names) != 1 || names[0].Name() != "lock" {
		t.Fatal("login refusal wrote credentials")
	}
}
