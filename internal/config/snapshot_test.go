package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type replacingReader struct {
	io.Reader
	replace func()
}

func (r *replacingReader) Read(p []byte) (int, error) {
	if r.replace != nil {
		replace := r.replace
		r.replace = nil
		replace()
	}
	return r.Reader.Read(p)
}
func TestConfigurationUsesOneBoundedSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	replacement := `[[models]]
alias="obsolete"`
	reader := &replacingReader{Reader: strings.NewReader("port=17392"), replace: func() {
		if err := os.WriteFile(path, []byte(replacement), 0600); err != nil {
			t.Fatal(err)
		}
	}}
	c, err := load(reader)
	if err != nil || c.Port != 17392 {
		t.Fatal("configuration snapshot changed between parses", c, err)
	}
	if _, err := Load(path); err != ErrLegacyModels {
		t.Fatal("fixture replacement not observed by next load", err)
	}
	if _, err := load(strings.NewReader(strings.Repeat(" ", maxConfigBytes+1))); err == nil {
		t.Fatal("unbounded config")
	}
}
