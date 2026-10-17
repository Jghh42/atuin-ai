//go:build linux || darwin

package setup

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const supervisorTimeout = 15 * time.Second
const serviceLabel = "io.github.jghh42.atuin-ai"

var ErrService = errors.New("user service unavailable or unsafe; use foreground serve, never sudo")
var ErrRollback = errors.New("setup rollback/restoration failed or is uncertain; investigate manually before starting")

type ServiceOptions struct {
	OS, Home, AccountHome, ConfigPath, Source, XDGConfig, RuntimeDir string
	UID                                                              int
	Config                                                           Snapshot
	Port                                                             int
	Files                                                            Files
	Exec                                                             Executor
}
type UserService struct {
	identify     func([]byte) bool
	wait         func(context.Context, time.Duration) error
	processCheck func(context.Context, int, string, []string, int) error
	processGone  func(int) (bool, error)
	lastPID      int
	opts         ServiceOptions
	binary, unit string
	content      []byte
	env          []string
}

func NewService(o ServiceOptions) *UserService {
	// Pin defaults explicitly; the user manager may carry a different inherited
	// XDG_CONFIG_HOME. Server state and unit location must use THIS captured value.
	if o.XDGConfig == "" {
		o.XDGConfig = filepath.Join(o.Home, ".config")
	}
	if o.OS == "linux" && o.RuntimeDir == "" {
		o.RuntimeDir = "/run/user/" + strconv.Itoa(o.UID)
	}
	s := &UserService{opts: o, binary: filepath.Join(o.Home, ".local", "bin", "atuin-ai-server"), identify: serverBinary, processCheck: verifyNativeProcess, processGone: managedProcessGone}
	s.wait = func(ctx context.Context, d time.Duration) error {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
	s.unit = filepath.Join(o.XDGConfig, "systemd", "user", "atuin-ai-server.service")
	if o.OS == "darwin" {
		s.unit = filepath.Join(o.Home, "Library", "LaunchAgents", serviceLabel+".plist")
	}
	s.env = []string{"HOME=" + o.Home, "XDG_CONFIG_HOME=" + o.XDGConfig, "PATH=/usr/bin:/bin", "LC_ALL=C"}
	if o.OS == "linux" {
		s.env = append(s.env, "XDG_RUNTIME_DIR="+o.RuntimeDir, "DBUS_SESSION_BUS_ADDRESS=unix:path="+filepath.Join(o.RuntimeDir, "bus"))
	}
	s.content = s.render()
	return s
}
func (s *UserService) Preview() string {
	semantics := "systemd USER service starts with your user manager (normally login; linger is NOT enabled)"
	if s.opts.OS == "darwin" {
		semantics = "LaunchAgent starts at GUI login, NOT machine boot"
	}
	foreground := fmt.Sprintf("run current executable %q with serve", s.opts.Source)
	if s.opts.Config.Exists {
		foreground += fmt.Sprintf(" --config %q", s.opts.ConfigPath)
	} else {
		foreground += " (uses the missing default config's built-in defaults)"
	}
	return fmt.Sprintf("Optional %s.\nStable executable: %q (atomic self-copy if needed; matching bytes unchanged).\nService: %q\nServer config: %q\nForeground alternative: %s", semantics, s.binary, s.unit, s.opts.ConfigPath, foreground)
}
func persistent(p string) bool {
	if !validPath(p) {
		return false
	}
	for _, root := range []string{"/tmp", "/var/tmp", "/private/tmp", "/private/var/tmp", "/var/folders", "/private/var/folders", "/run", "/dev", "/proc"} {
		if p == root || strings.HasPrefix(p, root+"/") {
			return false
		}
	}
	return true
}
func (s *UserService) preflight() error {
	o := s.opts
	if o.UID <= 0 || o.UID != os.Geteuid() || o.Files == nil || o.Exec == nil || (o.OS != "linux" && o.OS != "darwin") || !persistent(o.Home) || o.Home != o.AccountHome || !persistent(o.ConfigPath) || !persistent(s.unit) || !validPath(o.Source) || !persistent(o.XDGConfig) {
		return ErrService
	}
	if o.OS == "linux" && o.RuntimeDir != "/run/user/"+strconv.Itoa(o.UID) {
		return ErrService
	}
	// v258 load-fragment.c:985 calls string_is_safe (string-util.c:1078):
	// C-quoting cannot make quotes/backslashes legal in the executable path.
	// Reject BEFORE any filesystem/supervisor mutation; config argv may use them.
	if o.OS == "linux" && strings.ContainsAny(s.binary, "\"'\\") {
		return ErrService
	}
	return nil
}
func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func systemdEnvironmentQuote(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}
func systemdArgumentQuote(s string) string {
	return systemdEnvironmentQuote(strings.ReplaceAll(s, "$", "$$"))
}
func (s *UserService) render() []byte {
	o := s.opts
	if o.OS == "darwin" {
		env := "<key>HOME</key><string>" + xmlText(o.Home) + "</string><key>XDG_CONFIG_HOME</key><string>" + xmlText(o.XDGConfig) + "</string>"
		return []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- atuin-ai init managed v2 -->\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\"><dict>\n<key>Label</key><string>" + serviceLabel + "</string>\n<key>ProgramArguments</key><array><string>" + xmlText(s.binary) + "</string><string>serve</string><string>--config</string><string>" + xmlText(o.ConfigPath) + "</string></array>\n<key>EnvironmentVariables</key><dict>" + env + "</dict>\n<key>RunAtLoad</key><true/><key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>\n<key>ThrottleInterval</key><integer>10</integer><key>Umask</key><integer>63</integer>\n</dict></plist>\n")
	}
	env := "Environment=" + systemdEnvironmentQuote("HOME="+o.Home) + "\nEnvironment=" + systemdEnvironmentQuote("XDG_CONFIG_HOME="+o.XDGConfig) + "\n"
	// @ separates the executable path (no dollar expansion) from explicitly
	// supplied argv[0]. Both argv[0] and --config then use $$ for runtime literals.
	// See v258 load-fragment.c:975-1011 and exec-invoke.c:6319-6327.
	return []byte("# atuin-ai init managed v3\n[Unit]\nDescription=Atuin AI loopback server\nStartLimitIntervalSec=60\nStartLimitBurst=3\n\n[Service]\nType=simple\nExecStart=@" + systemdEnvironmentQuote(s.binary) + " " + systemdArgumentQuote(s.binary) + " serve --config " + systemdArgumentQuote(o.ConfigPath) + "\n" + env + "Restart=on-failure\nRestartSec=10\nUMask=0077\n\n[Install]\nWantedBy=default.target\n")
}
func (s *UserService) recognized() (Snapshot, error) {
	if s.preflight() != nil {
		return Snapshot{}, ErrService
	}
	old, err := s.opts.Files.Read(s.unit, MaxAtuinBytes)
	if err != nil || (old.Exists && !bytes.Equal(old.Data, s.content)) {
		return Snapshot{}, ErrService
	}
	return old, nil
}
func (s *UserService) observed(ctx context.Context) (serviceState, error) {
	old, err := s.recognized()
	if err != nil {
		return stateUnknown, err
	}
	return s.inspect(ctx, old.Exists)
}
func (s *UserService) Running(ctx context.Context) (bool, error) {
	state, err := s.observed(ctx)
	return state == stateRunning, err
}
func (s *UserService) Stop(ctx context.Context) error {
	state, err := s.observed(ctx)
	if err != nil {
		return err
	}
	if state == stateAbsent {
		return nil
	}
	if state == stateInactive && s.opts.OS == "linux" {
		return nil
	}
	if s.mutate(ctx, "stop") != nil {
		return ErrService
	}
	state, err = s.observed(ctx)
	if err != nil || state == stateRunning {
		return ErrService
	}
	if s.lastPID > 0 {
		gone, e := s.processGone(s.lastPID)
		if e != nil || !gone {
			return ErrService
		}
	}
	return nil
}
func (s *UserService) Restore(ctx context.Context) error {
	// Also used after a stop command that applied THEN failed/canceled. Reobserve
	// under the caller's independent cleanup context before any restorative action.
	state, err := s.observed(ctx)
	if err != nil {
		return err
	}
	if state == stateRunning {
		return nil
	}
	// A deregistered job can still be shutting down. Do not bootstrap another
	// process until the previously verified PID is definitively gone.
	if s.lastPID > 0 {
		gone, e := s.processGone(s.lastPID)
		if e != nil || !gone {
			return ErrService
		}
	}
	old, err := s.recognized()
	if err != nil || !old.Exists {
		return ErrService
	}
	verb := "start"
	if s.opts.OS == "darwin" && state == stateInactive {
		verb = "kickstart"
	}
	if s.mutate(ctx, verb) != nil {
		return ErrService
	}
	return s.waitActive(ctx)
}
func (s *UserService) waitActive(ctx context.Context) error {
	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for i := 0; i < 50; i++ {
		if deadline.Err() != nil {
			return ErrService
		}
		state, err := s.observed(deadline)
		if err != nil {
			return err
		}
		if state == stateRunning {
			return nil
		}
		if s.wait(deadline, 100*time.Millisecond) != nil {
			return ErrService
		}
	}
	return ErrService
}
func serverBinary(data []byte) bool {
	info, err := buildinfo.Read(bytes.NewReader(data))
	return err == nil && info.Path == "github.com/Jghh42/atuin-ai/cmd/atuin-ai-server" && info.Main.Path == "github.com/Jghh42/atuin-ai"
}

func (s *UserService) Install(ctx context.Context) (result error) {
	unit, err := s.recognized()
	if err != nil {
		return err
	}
	source, err := s.opts.Files.Read(s.opts.Source, maxFileBytes)
	if err != nil || !source.Exists || source.Mode&0111 == 0 || !s.identify(source.Data) {
		return ErrService
	}
	binary, err := s.opts.Files.Read(s.binary, maxFileBytes)
	if err != nil || (binary.Exists && (binary.Mode&0111 == 0 || (!bytes.Equal(binary.Data, source.Data) && !s.identify(binary.Data)))) {
		return ErrService
	}
	cfg, err := s.opts.Files.Read(s.opts.ConfigPath, MaxAtuinBytes)
	if err != nil || !same(cfg, s.opts.Config) {
		return ErrService
	}
	prior, err := s.inspect(ctx, unit.Exists)
	if err != nil {
		return err
	}
	wasActivated := prior == stateRunning || (s.opts.OS == "darwin" && prior == stateInactive)
	wasEnabled := false
	if s.opts.OS == "linux" && unit.Exists {
		r, e := s.command(ctx, "is-enabled")
		switch {
		case e == nil && r.success() && strings.TrimSpace(string(r.Stdout)) == "enabled":
			wasEnabled = true
		case r.complete() && r.ExitCode == 1 && strings.TrimSpace(string(r.Stdout)) == "disabled":
		default:
			return ErrService
		}
	}
	type change struct {
		path   string
		before Snapshot
		write  WriteOutcome
	}
	changes := []change{}
	startAttempted, enableAttempted, reloadAttempted := false, false, false
	defer func() {
		if result == nil {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), supervisorTimeout)
		defer cancel()
		failed := false
		// An attempted bootstrap/start does NOT confer ownership. Reinspect before
		// bootout/stop/disable; unknown or foreign registration is never touched.
		if startAttempted || enableAttempted {
			state, e := s.observed(cleanup)
			if e != nil {
				failed = true
			} else {
				if startAttempted && (state == stateRunning || (s.opts.OS == "darwin" && state == stateInactive)) {
					if s.Stop(cleanup) != nil {
						failed = true
					}
				}
				if enableAttempted && !wasEnabled {
					if s.mutate(cleanup, "disable") != nil {
						failed = true
					}
				}
			}
		}
		for i := len(changes) - 1; i >= 0; i-- {
			c := changes[i]
			if rollbackWrite(s.opts.Files, c.path, c.before, c.write) != nil {
				failed = true
			}
		}
		if s.opts.OS == "linux" && reloadAttempted {
			if s.mutate(cleanup, "daemon-reload") != nil {
				failed = true
			}
		}
		if wasActivated && !failed {
			if s.Restore(cleanup) != nil {
				failed = true
			}
		}
		if failed {
			result = errors.Join(result, ErrRollback)
		}
	}()
	// The defer already owns reconciliation BEFORE a potentially applied Stop.
	if wasActivated {
		if s.Stop(ctx) != nil {
			if ctx.Err() != nil {
				return ErrCanceled
			}
			return ErrService
		}
	}
	replace := func(path string, old Snapshot, data []byte, mode os.FileMode) error {
		if old.Exists && bytes.Equal(old.Data, data) {
			return nil
		}
		outcome, e := s.opts.Files.Replace(path, old, data, mode, false)
		if outcome.Applied {
			changes = append(changes, change{path, old, outcome})
		}
		if e == nil && !outcome.Durable {
			e = ErrCommitUncertain
		}
		return e
	}
	sourceNow, err := s.opts.Files.Read(s.opts.Source, maxFileBytes)
	if err != nil || !same(source, sourceNow) {
		return ErrService
	}
	if err = replace(s.binary, binary, source.Data, 0755); err != nil {
		return errors.Join(ErrService, err)
	}
	if !cfg.Exists {
		if err = replace(s.opts.ConfigPath, cfg, []byte(fmt.Sprintf("port = %d\n", s.opts.Port)), 0600); err != nil {
			return errors.Join(ErrService, err)
		}
	}
	if err = replace(s.unit, unit, s.content, 0600); err != nil {
		return errors.Join(ErrService, err)
	}
	expected := func(path string, old Snapshot) Snapshot {
		for i := len(changes) - 1; i >= 0; i-- {
			if changes[i].path == path {
				return changes[i].write.After
			}
		}
		return old
	}
	for _, input := range []struct {
		path string
		old  Snapshot
	}{{s.binary, binary}, {s.unit, unit}, {s.opts.ConfigPath, cfg}} {
		current, e := s.opts.Files.Read(input.path, maxFileBytes)
		if e != nil || !same(current, expected(input.path, input.old)) {
			return ErrService
		}
	}
	if ctx.Err() != nil {
		return ErrCanceled
	}
	if s.opts.OS == "linux" {
		reloadAttempted = true
		if s.mutate(ctx, "daemon-reload") != nil {
			return ErrService
		}
		// A previously absent unit may acquire global drop-ins when it is loaded.
		// Verify the EFFECTIVE loaded unit before enable/start, not only our file.
		if state, e := s.observed(ctx); e != nil || state != stateInactive {
			return ErrService
		}
		enableAttempted = true
		if s.mutate(ctx, "enable") != nil {
			return ErrService
		}
	}
	startAttempted = true
	if s.mutate(ctx, "start") != nil {
		if ctx.Err() != nil {
			return ErrCanceled
		}
		return ErrService
	}
	return s.waitActive(ctx)
}
