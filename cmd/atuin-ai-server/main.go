package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Jghh42/atuin-ai/internal/application"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/httpserver"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	out := io.Writer(os.Stderr)
	if len(os.Args) == 2 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		out = os.Stdout
	}
	if err := run(ctx, os.Args[1:], out); err != nil {
		// Configuration parsers and OS errors may include file contents or paths.
		// Only deliberately generic errors reach logs.
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("command failed", "reason", err.Error())
		os.Exit(1)
	}
}

type commandDeps struct {
	state     string
	transport http.RoundTripper
	display   func(credentials.DevicePrompt) error
}

func run(ctx context.Context, args []string, stderr io.Writer) error {
	return runWith(ctx, args, stderr, commandDeps{})
}
func runWith(ctx context.Context, args []string, stderr io.Writer, deps commandDeps) error {
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version") {
		if len(args) != 1 {
			return errors.New("version takes no arguments")
		}
		return printVersion(stderr)
	}
	if len(args) > 0 && args[0] == "auth" {
		return runAuth(ctx, args[1:], stderr, deps)
	}
	if len(args) == 0 || args[0] != "serve" {
		return errors.New("usage: atuin-ai-server serve [--config PATH] | auth login|status|logout|diagnose | version | --version")
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	// Do not echo arbitrary command line values into logs.
	flags.SetOutput(io.Discard)
	path := flags.String("config", "", "TOML configuration path (overrides CHAT_CONFIG)")
	if err := flags.Parse(args[1:]); err != nil {
		return errors.New("invalid serve arguments")
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected serve arguments")
	}
	cfg, err := loadConfiguration(*path)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(stderr, nil))
	runtime, err := openRuntime(cfg, deps, false)
	if err != nil {
		return err
	}
	defer runtime.Close()
	server, err := httpserver.NewWithRuntime(cfg, logger, runtime.HTTP)
	if err != nil {
		return errors.New("invalid server configuration")
	}
	listener, err := server.Listen()
	if err != nil {
		return errors.New("cannot open IPv4 loopback listener; check whether the configured port is already in use")
	}
	return server.Serve(ctx, listener)
}

func openRuntime(cfg config.Config, deps commandDeps, diagnostic bool) (*application.Runtime, error) {
	path := deps.state
	if path == "" {
		var err error
		path, err = application.StateDirectory()
		if err != nil {
			return nil, err
		}
	}
	if diagnostic {
		return application.NewDiagnostic(path, cfg, deps.transport)
	}
	return application.New(path, cfg, deps.transport)
}
func runAuth(ctx context.Context, args []string, out io.Writer, deps commandDeps) error {
	if len(args) != 1 || (args[0] != "login" && args[0] != "status" && args[0] != "logout" && args[0] != "diagnose") {
		return errors.New("usage: atuin-ai-server auth login|status|logout|diagnose; stop server before auth commands")
	}
	display := deps.display
	if args[0] == "login" && display == nil {
		// /dev/tty is the controlling terminal, not redirected stdout/stderr/logs.
		tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		if err != nil {
			return errors.New("login requires an interactive controlling terminal")
		}
		defer tty.Close()
		display = func(p credentials.DevicePrompt) error {
			_, e := fmt.Fprintf(tty, "Open %s and enter code: %s\n", credentials.ActivationURL, p.UserCode)
			return e
		}
	}
	cfg := config.Default()
	if args[0] == "login" || args[0] == "diagnose" {
		var err error
		cfg, err = loadConfiguration("")
		if err != nil {
			if args[0] == "diagnose" {
				return writeDiagnostic(out, codex.DiagnosticReport{Stage: codex.StageConfiguration, Result: codex.ResultConfigurationUnavailable})
			}
			return err
		}
	}
	runtime, err := openRuntime(cfg, deps, args[0] == "diagnose")
	if err != nil {
		if args[0] == "diagnose" {
			configured := cfg.DefaultModel != ""
			return writeDiagnostic(out, codex.DiagnosticReport{Stage: codex.StageCredentials, Result: codex.ResultCredentialUnavailable, DefaultOverrideConfigured: &configured})
		}
		return err
	}
	defer runtime.Close()
	switch args[0] {
	case "diagnose":
		return writeDiagnostic(out, runtime.Diagnose(ctx))
	case "status":
		status := runtime.Manager.Status()
		switch {
		case status.ReauthenticationRequired:
			_, err = fmt.Fprintln(out, "Reauthentication required (local session unusable)")
		case status.LoggedIn:
			_, err = fmt.Fprintf(out, "Logged in (local record; upstream acceptance not verified); generation=%d auth_epoch=%d expires_at=%s\n", status.Generation, status.AuthEpoch, status.ExpiresAt.Format(time.RFC3339))
		default:
			_, err = fmt.Fprintln(out, "Logged out")
		}
	case "logout":
		err = runtime.Manager.Logout(ctx)
		if err == nil {
			_, err = fmt.Fprintln(out, "Local credentials deleted; remote revocation is not supported")
		}
	case "login":
		err = runtime.Manager.Login(ctx, display)
		if err == nil {
			// Bundled membership does not verify upstream account acceptance.
			if _, e := runtime.Catalog.Get(ctx); e != nil {
				_, err = fmt.Fprintln(out, "Login saved successfully; bundled model catalog unavailable (credentials or default_model override)")
			} else {
				_, err = fmt.Fprintln(out, "Login saved successfully; bundled model catalog available; upstream acceptance not verified")
			}
		}
	}
	if err != nil { // Writer/OS errors must not leak paths or caller text.
		switch err {
		case credentials.ErrStore, credentials.ErrOAuth, credentials.ErrDenied, credentials.ErrExpired, credentials.ErrReauth, credentials.ErrUnavailable:
			return err
		default:
			return errors.New("authentication command failed")
		}
	}
	return nil
}

// Diagnostic failures are report outcomes, not arbitrary errors in command logs.
// Like the other auth commands, output goes to stderr. Writer failures stay safe.
func writeDiagnostic(out io.Writer, report codex.DiagnosticReport) error {
	if json.NewEncoder(out).Encode(report) != nil {
		return errors.New("cannot write diagnostic report")
	}
	return nil
}

func loadConfiguration(path string) (config.Config, error) {
	if path == "" {
		path = os.Getenv("CHAT_CONFIG")
	}
	explicit := path != ""
	if !explicit {
		dir, err := os.UserConfigDir()
		if err != nil {
			return config.Config{}, errors.New("cannot locate user configuration directory")
		}
		path = filepath.Join(dir, "atuin-ai", "config.toml")
	}
	cfg, err := config.Load(path)
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, config.ErrLegacyModels) {
		return config.Config{}, config.ErrLegacyModels
	}
	if !explicit && errors.Is(err, os.ErrNotExist) {
		return config.Default(), nil
	}
	return config.Config{}, errors.New("cannot load configuration; check TOML fields and values")
}
