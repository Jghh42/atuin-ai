// Package config loads the server's deliberately small, strict TOML configuration.
// Listener host selection and environment variable handling belong to the caller;
// no configuration field can change the loopback-only listener policy.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Fixed resource limits are not operator-configurable.
const (
	MaxRequestBodyBytes       = 4 << 20
	MaxConcurrentChats        = 4
	MaxProviderEventBytes     = 1 << 20
	MaxCallArgumentsBytes     = 256 << 10
	MaxCallsPerResponse       = 32
	UpstreamInactivityTimeout = 60 * time.Second
	GenerationTimeout         = 300 * time.Second
)

// Config is process-wide configuration. Models retain their TOML array order.
// An empty DefaultModel selects the first model's alias.
type Config struct {
	Port         int     `toml:"port"`
	DefaultModel string  `toml:"default_model"`
	Models       []Model `toml:"models"`
}

// Model maps a public alias to a private upstream model ID.
type Model struct {
	Alias       string `toml:"alias"`
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Model       string `toml:"model"`
}

// Default returns independent defaults on each call. The upstream model is a
// placeholder, not a claim of availability or verified OAuth compatibility.
func Default() Config {
	return Config{
		Port:         8080,
		DefaultModel: "gpt-5",
		Models: []Model{{
			Alias: "gpt-5", Name: "GPT-5",
			Description: "Placeholder; verify model access and compatibility before use",
			Model:       "gpt-5",
		}},
	}
}

// Load reads a TOML file, rejects unknown keys at every level, applies defaults,
// and validates the result. An empty path returns Default; a nonempty missing
// path is an error. The caller resolves CHAT_CONFIG; Load never reads it.
// Omitted models use the placeholder catalog; explicitly empty models are invalid.
// Omitted default_model selects the first resulting model, even in a custom list.
func Load(path string) (Config, error) {
	if path == "" {
		return Default(), nil
	}
	c := Config{Port: 8080}
	meta, err := toml.DecodeFile(path, &c)
	if err != nil {
		return Config{}, fmt.Errorf("load config %q: %w", path, err)
	}
	// The decoder matches struct fields case-insensitively. Require canonical
	// spellings before applying defaults, so Models and models cannot alias or
	// bypass the explicit-empty catalog check.
	for _, key := range meta.Keys() {
		valid := false
		if len(key) == 1 {
			valid = key[0] == "port" || key[0] == "default_model" || key[0] == "models"
		} else if len(key) == 2 && key[0] == "models" {
			switch key[1] {
			case "alias", "name", "description", "model":
				valid = true
			}
		}
		if !valid {
			return Config{}, fmt.Errorf("config contains a noncanonical or unknown key: %s", key.String())
		}
	}
	if keys := meta.Undecoded(); len(keys) > 0 {
		names := make([]string, len(keys))
		for i, key := range keys {
			names[i] = key.String()
		}
		return Config{}, fmt.Errorf("config %q: unknown keys: %s", path, strings.Join(names, ", "))
	}
	if !meta.IsDefined("models") {
		c.Models = Default().Models
	}
	if c.DefaultModel == "" && len(c.Models) > 0 {
		c.DefaultModel = c.Models[0].Alias
	}
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("config %q: %w", path, err)
	}
	return c, nil
}

// Validate checks the port, required model fields, unique aliases, and default
// membership. Description may be empty. It does not contact the provider or
// modify the config; an empty DefaultModel means the first configured alias.
func (c Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if len(c.Models) == 0 {
		return fmt.Errorf("at least one model is required")
	}
	aliases := make(map[string]bool, len(c.Models))
	for i, m := range c.Models {
		if strings.TrimSpace(m.Alias) == "" || strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.Model) == "" {
			return fmt.Errorf("models[%d]: alias, name, and model must be nonempty", i)
		}
		if aliases[m.Alias] {
			return fmt.Errorf("models[%d]: duplicate alias %q", i, m.Alias)
		}
		aliases[m.Alias] = true
	}
	if c.DefaultModel != "" && !aliases[c.DefaultModel] {
		return fmt.Errorf("default_model %q is not a configured alias", c.DefaultModel)
	}
	return nil
}
