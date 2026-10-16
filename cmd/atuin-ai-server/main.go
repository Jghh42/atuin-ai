package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/httpserver"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stderr); err != nil {
		// Configuration parsers and OS errors may include file contents or paths.
		// Only deliberately generic errors reach logs.
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("command failed", "reason", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "serve" {
		return errors.New("usage: atuin-ai-server serve [--config PATH]")
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
	server, err := httpserver.New(cfg, logger)
	if err != nil {
		return errors.New("invalid server configuration")
	}
	listener, err := server.Listen()
	if err != nil {
		return errors.New("cannot open IPv4 loopback listener; check whether the configured port is already in use")
	}
	return server.Serve(ctx, listener)
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
