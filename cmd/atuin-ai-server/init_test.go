package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/setup"
)

func TestInitConfigurationRejectsUTF16BOMBeforeDecoder(t *testing.T) {
	for _, prefix := range [][]byte{{0xff, 0xfe}, {0xfe, 0xff}} {
		data := append(append([]byte(nil), prefix...), []byte("port=17391\n")...)
		if _, err := decodeInitConfiguration(setup.Snapshot{Exists: true, Data: data}); err == nil {
			t.Fatal("UTF16 BOM accepted")
		}
	}
	cfg, err := decodeInitConfiguration(setup.Snapshot{Exists: true, Data: []byte("port=19333\ndefault_model='gpt-6-luna'\n")})
	if err != nil || cfg.Port != 19333 || cfg.DefaultModel != "gpt-6-luna" {
		t.Fatal("valid immutable snapshot refused")
	}
	if _, err = decodeInitConfiguration(setup.Snapshot{Exists: true, Data: []byte("private_value='SYNTHETIC-PRIVATE'\n")}); err == nil || strings.Contains(err.Error(), "SYNTHETIC") {
		t.Fatal("unsafe parser diagnostic")
	}
}
func TestInitDispatchExplicitPathsAndSafeArguments(t *testing.T) {
	calls := 0
	deps := commandDeps{initialize: func(_ context.Context, server, atuin string) error {
		calls++
		if server != "server path" || atuin != "atuin path" {
			t.Fatal("paths changed")
		}
		return nil
	}}
	if e := runWith(context.Background(), []string{"init", "--config", "server path", "--atuin-config", "atuin path"}, io.Discard, deps); e != nil || calls != 1 {
		t.Fatal(e, calls)
	}
	for _, args := range [][]string{{"init", "--yes"}, {"init", "--config"}, {"init", "--token", "PRIVATE"}, {"init", "extra"}, {"install"}} {
		err := runWith(context.Background(), args, io.Discard, deps)
		if err == nil || strings.Contains(err.Error(), "PRIVATE") || calls != 1 {
			t.Fatal("unsafe argument dispatch", err, calls)
		}
	}
	sentinel := errors.New("synthetic result")
	deps.initialize = func(context.Context, string, string) error { return sentinel }
	if e := runWith(context.Background(), []string{"init"}, io.Discard, deps); !errors.Is(e, sentinel) {
		t.Fatal("result lost")
	}
}
