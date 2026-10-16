package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptionalConfig(t *testing.T) {
	c, err := Load("")
	if err != nil || c.Port != 17391 || c.DefaultModel != "" {
		t.Fatal(c, err)
	}
	for _, text := range []string{"", "port=1234\ndefault_model='gpt-6.1-sol'"} {
		p := filepath.Join(t.TempDir(), "c.toml")
		if err := os.WriteFile(p, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p); err != nil {
			t.Fatal(err)
		}
	}
}
func TestRejectConfig(t *testing.T) {
	for _, text := range []string{"port=0", "port=65536", "PORT=12", "host='0.0.0.0'", "port='12'", "default_model=1", "models=[]", "[[models]]\nalias='x'", "Models=[]"} {
		t.Run(text, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "c.toml")
			if err := os.WriteFile(p, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(p)
			if err == nil {
				t.Fatal("accepted")
			}
			if strings.Contains(strings.ToLower(text), "models") && !strings.Contains(err.Error(), "remove") {
				t.Fatal(err)
			}
		})
	}
}
func TestValidation(t *testing.T) {
	for _, c := range []Config{{Port: 0}, {Port: 65536}, {Port: 17391, DefaultModel: " bad"}, {Port: 17391, DefaultModel: strings.Repeat("x", 129)}} {
		if c.Validate() == nil {
			t.Fatal(c)
		}
	}
}
