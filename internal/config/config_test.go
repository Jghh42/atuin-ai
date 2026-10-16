package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func loadText(t *testing.T, text string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestDefaults(t *testing.T) {
	want := Default()
	if err := want.Validate(); err != nil {
		t.Fatal(err)
	}
	if want.Port != 8080 || want.DefaultModel != want.Models[0].Alias {
		t.Fatalf("bad defaults: %+v", want)
	}
	for _, text := range []string{"", "# empty config\n", "port = 8080"} {
		got, err := loadText(t, text)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: got %+v, %v", text, got, err)
		}
	}
	t.Setenv("CHAT_CONFIG", "/does/not/exist")
	got, err := Load("")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("empty path: %+v, %v", got, err)
	}
	got.Models[0].Name = "changed"
	if Default().Models[0].Name == "changed" {
		t.Fatal("defaults share backing memory")
	}
}

func TestOrderedModelsAndDefault(t *testing.T) {
	const models = `
[[models]]
alias = "second"
name = "Second"
model = "private-2"
[[models]]
alias = "first"
name = "First"
description = "Some description"
model = "private-1"
`
	for _, tc := range []struct{ prefix, want string }{
		{"", "second"}, {"default_model = \"first\"\n", "first"}, {"default_model = \"\"\n", "second"},
	} {
		c, err := loadText(t, "port = 1234\n"+tc.prefix+models)
		if err != nil {
			t.Fatal(err)
		}
		if c.Port != 1234 || c.DefaultModel != tc.want || len(c.Models) != 2 || c.Models[0].Alias != "second" || c.Models[1].Model != "private-1" {
			t.Fatalf("unexpected config: %+v", c)
		}
	}
}

func TestRejectInvalidTOMLAndUnknownKeys(t *testing.T) {
	for _, text := range []string{
		"port =", "port = \"8080\"", "port = 8080\nport = 8081", "models = {}",
		"host = '0.0.0.0'", "bind = '127.0.0.1'", "security = {}", "[listener]\nhost = 'localhost'",
		"[[models]]\nalias='x'\nname='X'\nmodel='upstream'\nbind='localhost'",
		"[[models]]\nalias='x'\nname='X'\nmodel='upstream'\n[models.security]\nauth=false",
		"port=0", "port=65536", "models=[]", "default_model='missing'",
	} {
		t.Run(text, func(t *testing.T) {
			if _, err := loadText(t, text); err == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
	_, err := loadText(t, "host='localhost'")
	if err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("unknown key not identified: %v", err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing file accepted")
	}
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("directory accepted")
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Config)
	}{
		{"negative port", func(c *Config) { c.Port = -1 }},
		{"zero port", func(c *Config) { c.Port = 0 }},
		{"large port", func(c *Config) { c.Port = 65536 }},
		{"nil models", func(c *Config) { c.Models = nil }},
		{"empty models", func(c *Config) { c.Models = []Model{} }},
		{"empty alias", func(c *Config) { c.Models[0].Alias = " " }},
		{"empty name", func(c *Config) { c.Models[0].Name = "\t" }},
		{"empty upstream", func(c *Config) { c.Models[0].Model = "" }},
		{"duplicate alias", func(c *Config) { c.Models = append(c.Models, c.Models[0]) }},
		{"unknown default", func(c *Config) { c.DefaultModel = "absent" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Default()
			tc.change(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
	for _, port := range []int{1, 65535} {
		c := Default()
		c.Port = port
		c.DefaultModel = ""
		c.Models[0].Description = ""
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		if c.DefaultModel != "" {
			t.Fatal("Validate mutated config")
		}
	}
}

func TestExample(t *testing.T) {
	if _, err := Load("../../config.example.toml"); err != nil {
		t.Fatal(err)
	}
}

func TestFixedLimits(t *testing.T) {
	if MaxRequestBodyBytes != 4*1024*1024 || MaxConcurrentChats != 4 || MaxProviderEventBytes != 1024*1024 || MaxCallArgumentsBytes != 256*1024 || MaxCallsPerResponse != 32 || UpstreamInactivityTimeout != 60*time.Second || GenerationTimeout != 300*time.Second {
		t.Fatal("resource policy changed")
	}
}
