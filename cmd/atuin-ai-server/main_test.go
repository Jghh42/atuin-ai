package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPrecedenceAndDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("CHAT_CONFIG", "")
	cfg, err := loadConfiguration("")
	if err != nil || cfg.Port != 8080 {
		t.Fatalf("default: %+v %v", cfg, err)
	}
	defaultPath := filepath.Join(dir, "atuin-ai", "config.toml")
	if err := os.MkdirAll(filepath.Dir(defaultPath), 0700); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(defaultPath, "port = 8101\n")
	cfg, err = loadConfiguration("")
	if err != nil || cfg.Port != 8101 {
		t.Fatalf("user config: %+v %v", cfg, err)
	}
	envPath := filepath.Join(dir, "env.toml")
	write(envPath, "port = 8102\n")
	t.Setenv("CHAT_CONFIG", envPath)
	cfg, err = loadConfiguration("")
	if err != nil || cfg.Port != 8102 {
		t.Fatalf("env config: %+v %v", cfg, err)
	}
	flagPath := filepath.Join(dir, "flag.toml")
	write(flagPath, "port = 8103\n")
	cfg, err = loadConfiguration(flagPath)
	if err != nil || cfg.Port != 8103 {
		t.Fatalf("flag config: %+v %v", cfg, err)
	}
	if _, err = loadConfiguration(filepath.Join(dir, "absent")); err == nil {
		t.Fatal("missing explicit file accepted")
	}
	write(flagPath, `secret_unknown_key = "do-not-log-this-secret"`)
	if _, err = loadConfiguration(flagPath); err == nil || strings.Contains(err.Error(), "do-not-log") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestCommandArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"auth"}, {"serve", "--host", "0.0.0.0"}, {"serve", "--port", "8080"}, {"serve", "extra"}} {
		if err := run(context.Background(), args, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
