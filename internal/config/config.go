// Package config loads the optional, deliberately small TOML configuration.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	MaxRequestBodyBytes       = 4 << 20
	MaxConcurrentChats        = 4
	MaxProviderEventBytes     = 1 << 20
	MaxCallArgumentsBytes     = 256 << 10
	MaxCallsPerResponse       = 32
	UpstreamInactivityTimeout = 60 * time.Second
	GenerationTimeout         = 300 * time.Second
)

// ErrLegacyModels is a fixed, safe migration diagnostic for command output.
var ErrLegacyModels = errors.New("remove [[models]]/models and alias mappings: models are now discovered after application login; optionally set default_model to an eligible upstream ID")

type Config struct {
	Port         int    `toml:"port"`
	DefaultModel string `toml:"default_model"`
}

func Default() Config { return Config{Port: 17391} }
func Load(path string) (Config, error) {
	c := Default()
	if path == "" {
		return c, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	defer file.Close()
	return load(file)
}

const maxConfigBytes = 64 << 10

func load(reader io.Reader) (Config, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxConfigBytes+1))
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	if len(data) > maxConfigBytes {
		return Config{}, errors.New("configuration exceeds 64 KiB")
	}
	// Both decodes operate on one immutable read, so replacement of the source
	// file cannot bypass the key/migration checks between parses.
	text := string(data)
	c := Default()
	var raw map[string]any
	meta, err := toml.Decode(text, &raw)
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	for _, k := range meta.Keys() {
		if strings.EqualFold(k[0], "models") {
			return Config{}, ErrLegacyModels
		}
		if len(k) != 1 || (k[0] != "port" && k[0] != "default_model") {
			return Config{}, fmt.Errorf("unknown configuration key: %s", k.String())
		}
	}
	if _, err = toml.Decode(text, &c); err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if len(c.DefaultModel) > 128 || strings.TrimSpace(c.DefaultModel) != c.DefaultModel || strings.ContainsAny(c.DefaultModel, "\r\n\t") {
		return fmt.Errorf("default_model must be a bounded upstream model ID")
	}
	return nil
}
