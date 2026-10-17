package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"github.com/Jghh42/atuin-ai/internal/application"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/setup"
)

func runInit(ctx context.Context, args []string, deps commandDeps) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	serverPath := flags.String("config", "", "server TOML path")
	atuinPath := flags.String("atuin-config", "", "explicit Atuin TOML path")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return errors.New("usage: atuin-ai-server init [--config PATH] [--atuin-config PATH]")
	}
	if deps.initialize != nil {
		return deps.initialize(ctx, *serverPath, *atuinPath)
	}
	tty, e := setup.OpenTerminal()
	if e != nil {
		return e
	}
	defer tty.Close()
	if os.Geteuid() == 0 {
		return errors.New("init is a user-only workflow; do not run as root or with sudo; no changes made")
	}
	// No app state or configuration is read until a controlling TTY is open.
	files := setup.DiskFiles{}
	path := *serverPath
	if path == "" {
		path = os.Getenv("CHAT_CONFIG")
	}
	explicit := path != ""
	if !explicit {
		dir, err := os.UserConfigDir()
		if err != nil {
			return errors.New("cannot locate server configuration")
		}
		path = filepath.Join(dir, "atuin-ai", "config.toml")
	}
	path, e = filepath.Abs(path)
	if e != nil {
		return errors.New("cannot resolve server configuration path")
	}
	snapshot, e := files.Read(path, 64<<10)
	if e != nil || (explicit && !snapshot.Exists) {
		return errors.New("cannot load server configuration; explicit paths must exist")
	}
	// Decode and validate the same bounded immutable snapshot that service setup
	// later compares before writing. Existing normal serve/auth behavior is untouched.
	cfg, e := decodeInitConfiguration(snapshot)
	if e != nil {
		return e
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return errors.New("cannot locate user home")
	}
	executable, e := os.Executable()
	if e != nil {
		return errors.New("cannot locate executable")
	}
	accountHome := ""
	if account, err := user.LookupId(strconv.Itoa(os.Geteuid())); err == nil {
		accountHome = account.HomeDir
	}
	service := setup.NewService(setup.ServiceOptions{OS: runtime.GOOS, Home: home, AccountHome: accountHome, UID: os.Geteuid(), ConfigPath: path, Config: snapshot, Port: cfg.Port, Source: executable, XDGConfig: os.Getenv("XDG_CONFIG_HOME"), RuntimeDir: os.Getenv("XDG_RUNTIME_DIR"), Files: files, Exec: setup.NativeExecutor{}})
	// Path discovery is read-only and delayed until its step. Ambiguity/config
	// refusal must not discard successful independent authentication/service work.
	target := *atuinPath
	if target != "" {
		target, e = filepath.Abs(target)
		if e != nil {
			return errors.New("cannot resolve Atuin configuration path")
		}
	}
	wizard := setup.Wizard{
		TTY: tty,
		OpenAuth: func() (setup.Auth, error) {
			r, err := openRuntime(cfg, deps, false)
			if err != nil {
				return nil, err
			}
			return &initAuth{r}, nil
		},
		Service: service,
		PrepareConfig: func() (string, error) {
			chosen, err := setup.ResolveAtuin(target, home, os.Getenv("XDG_CONFIG_HOME"), os.Getenv("ATUIN_CONFIG_DIR"), os.Getenv("ATUIN_HOME"), files)
			if err != nil {
				return "", err
			}
			target = chosen
			return fmt.Sprintf("Atuin configuration: %s\nOnly [ai] enabled=true, endpoint=http://127.0.0.1:%d, api_token=empty; model and all other settings retained.", target, cfg.Port), nil
		},
		Configure: func() error { return setup.ConfigureAtuin(files, target, cfg.Port) },
	}
	return wizard.Run(ctx)
}

func decodeInitConfiguration(snapshot setup.Snapshot) (config.Config, error) {
	cfg := config.Default()
	if !snapshot.Exists {
		return cfg, nil
	}
	if len(snapshot.Data) > 64<<10 || !utf8.Valid(snapshot.Data) {
		return config.Config{}, errors.New("cannot load server configuration; bounded UTF-8 required")
	}
	meta, err := toml.Decode(string(snapshot.Data), &cfg)
	if err != nil {
		return config.Config{}, errors.New("cannot load server configuration")
	}
	for _, key := range meta.Keys() {
		if len(key) != 1 || (key[0] != "port" && key[0] != "default_model") {
			return config.Config{}, errors.New("cannot load server configuration; only port/default_model supported")
		}
	}
	if cfg.Validate() != nil {
		return config.Config{}, errors.New("cannot load server configuration")
	}
	return cfg, nil
}

type initAuth struct{ *application.Runtime }

func (a *initAuth) Status() credentials.Status { return a.Manager.Status() }
func (a *initAuth) Login(ctx context.Context, display func(credentials.DevicePrompt) error) error {
	return a.Manager.Login(ctx, display)
}
