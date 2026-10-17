//go:build linux || darwin

package setup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Command struct {
	Path      string
	Args, Env []string
}

// CommandResult deliberately keeps transport/cancellation/overflow distinct
// from a completed nonzero exit. Raw output is private and never a diagnostic.
type CommandResult struct {
	Stdout, Stderr                          []byte
	ExitCode                                int
	Started, Canceled, Overflow, Incomplete bool
}

func (CommandResult) String() string   { return "[private supervisor result]" }
func (CommandResult) GoString() string { return "[private supervisor result]" }
func (CommandResult) MarshalJSON() ([]byte, error) {
	return []byte(`"[private supervisor result]"`), nil
}
func (CommandResult) Format(s fmt.State, _ rune) {
	_, _ = s.Write([]byte("[private supervisor result]"))
}
func (r CommandResult) complete() bool {
	return r.Started && !r.Canceled && !r.Overflow && !r.Incomplete && r.ExitCode >= 0
}
func (r CommandResult) success() bool { return r.complete() && r.ExitCode == 0 }

type Executor interface {
	Run(context.Context, Command) (CommandResult, error)
}
type NativeExecutor struct{}
type captureBudget struct {
	sync.Mutex
	remaining int
	overflow  bool
}
type commandCapture struct {
	bytes.Buffer
	budget *captureBudget
}

func (b *commandCapture) Write(p []byte) (int, error) {
	n := len(p)
	b.budget.Lock()
	defer b.budget.Unlock()
	if len(p) > b.budget.remaining {
		p = p[:b.budget.remaining]
		b.budget.overflow = true
	}
	b.budget.remaining -= len(p)
	_, _ = b.Buffer.Write(p)
	return n, nil
}

// Fixed executables/verbs, no shell, minimal environment. The new process group
// alone is canceled; stdout AND stderr SHARE the unchanged total16KiB cap.
func (NativeExecutor) Run(ctx context.Context, c Command) (CommandResult, error) {
	r := CommandResult{ExitCode: -1}
	if !allowedCommand(c) {
		return r, ErrService
	}
	ctx, cancel := context.WithTimeout(ctx, supervisorTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Env = c.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	budget := &captureBudget{remaining: 16 << 10}
	out := commandCapture{budget: budget}
	diagnostic := commandCapture{budget: budget}
	cmd.Stdout = &out
	cmd.Stderr = &diagnostic
	err := cmd.Run()
	r.Stdout = out.Bytes()
	r.Stderr = diagnostic.Bytes()
	r.Started = cmd.Process != nil
	r.Canceled = ctx.Err() != nil
	r.Overflow = budget.overflow
	if cmd.ProcessState != nil {
		r.ExitCode = cmd.ProcessState.ExitCode()
	}
	var exitError *exec.ExitError
	r.Incomplete = err != nil && !errors.As(err, &exitError)
	if r.Canceled {
		return r, ErrCanceled
	}
	if r.Overflow || err != nil {
		return r, ErrService
	}
	return r, nil
}

const systemdProperties = "LoadState,FragmentPath,DropInPaths,NeedDaemonReload,Type,ExecStart,Environment,EnvironmentFiles,PassEnvironment,UnsetEnvironment,MainPID,ExecMainPID,ActiveState,SubState,RemainAfterExit,RootDirectory"

func allowedCommand(c Command) bool {
	a := c.Args
	if c.Path == "/usr/bin/systemctl" {
		if len(a) == 3 && a[0] == "--user" && a[2] == "atuin-ai-server.service" {
			switch a[1] {
			case "is-enabled", "stop", "start", "enable", "disable":
				return true
			}
		}
		if len(a) == 2 && a[0] == "--user" && a[1] == "daemon-reload" {
			return true
		}
		return len(a) == 5 && a[0] == "--user" && a[1] == "show" && a[2] == "--property="+systemdProperties && a[3] == "--no-pager" && a[4] == "atuin-ai-server.service"
	}
	if c.Path != "/bin/launchctl" || len(a) < 2 {
		return false
	}
	switch a[0] {
	case "print", "bootout", "kickstart":
		return len(a) == 2 && validDomain(a[1], true)
	case "bootstrap":
		return len(a) == 3 && validDomain(a[1], false) && validPath(a[2]) && filepathBase(a[2]) == serviceLabel+".plist"
	}
	return false
}
func filepathBase(p string) string { i := strings.LastIndexByte(p, '/'); return p[i+1:] }
func validDomain(s string, label bool) bool {
	if label {
		suffix := "/" + serviceLabel
		if !strings.HasSuffix(s, suffix) {
			return false
		}
		s = strings.TrimSuffix(s, suffix)
	}
	if !strings.HasPrefix(s, "gui/") {
		return false
	}
	uid, err := strconv.Atoi(strings.TrimPrefix(s, "gui/"))
	return err == nil && uid > 0 && uid == os.Geteuid() && s == "gui/"+strconv.Itoa(uid)
}
