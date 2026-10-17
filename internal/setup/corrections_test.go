//go:build linux || darwin

package setup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/credentials"
)

func TestDiskWriteOutcomeSurvivesPostRenameFailures(t *testing.T) {
	for _, step := range []string{"after-rename.read", "after-rename.dirsync"} {
		for _, existing := range []bool{false, true} {
			t.Run(step+fmt.Sprint(existing), func(t *testing.T) {
				root := privateRoot(t)
				path := filepath.Join(root, "config.toml")
				plain := DiskFiles{}
				if existing {
					if e := os.WriteFile(path, []byte("old"), 0640); e != nil {
						t.Fatal(e)
					}
				}
				before, e := plain.Read(path, 100)
				if e != nil {
					t.Fatal(e)
				}
				calls := 0
				files := DiskFiles{fault: func(name string) error {
					if name == step {
						calls++
						if calls == 1 {
							return ErrFile
						}
					}
					return nil
				}}
				outcome, e := files.Replace(path, before, []byte("new"), 0600, true)
				if !errors.Is(e, ErrCommitUncertain) || !outcome.Applied || !outcome.After.Exists || outcome.After.ino == 0 || !bytes.Equal(outcome.After.Data, []byte("new")) {
					t.Fatal("lost applied identity")
				}
				if step == "after-rename.dirsync" && outcome.Durable {
					t.Fatal("uncertain directory reported durable")
				}
				current, e := plain.Read(path, 100)
				if e != nil || !same(current, outcome.After) {
					t.Fatal("staged identity not the actual committed inode")
				}
				if e = rollbackWrite(files, path, before, outcome); e != nil {
					t.Fatal("guarded rollback failed", e)
				}
				current, e = plain.Read(path, 100)
				if e != nil || current.Exists != existing || (existing && !bytes.Equal(current.Data, before.Data)) {
					t.Fatal("old state not restored")
				}
			})
		}
	}
}
func TestConfigureRollsBackAllAppliedWritesAndRefusesConcurrentTarget(t *testing.T) {
	for _, step := range []string{"after-rename.read", "after-rename.dirsync"} {
		for _, existing := range []bool{false, true} {
			t.Run(step+fmt.Sprint(existing), func(t *testing.T) {
				root := privateRoot(t)
				path := filepath.Join(root, "config.toml")
				original := []byte("# preserved\n[ai]\nmodel='unknown-model'\n")
				if existing {
					if e := os.WriteFile(path, original, 0600); e != nil {
						t.Fatal(e)
					}
				}
				first := true
				files := DiskFiles{fault: func(name string) error {
					if name == step && first {
						first = false
						return ErrFile
					}
					return nil
				}}
				err := ConfigureAtuin(files, path, 19222)
				if !errors.Is(err, ErrCommitUncertain) || errors.Is(err, ErrRollback) {
					t.Fatal("primary/rollback outcome incorrect", err)
				}
				current, e := (DiskFiles{}).Read(path, MaxAtuinBytes)
				if e != nil || current.Exists != existing || (existing && !bytes.Equal(current.Data, original)) {
					t.Fatal("config applied error was not undone")
				}
			})
		}
	}
	t.Run("concurrent inode untouched", func(t *testing.T) {
		root := privateRoot(t)
		path := filepath.Join(root, "config.toml")
		if e := os.WriteFile(path, []byte("# original\n"), 0600); e != nil {
			t.Fatal(e)
		}
		first := true
		files := DiskFiles{fault: func(name string) error {
			if name == "after-rename.read" && first {
				first = false
				replacement := filepath.Join(root, "concurrent")
				if e := os.WriteFile(replacement, []byte("# concurrent\n"), 0600); e != nil {
					t.Fatal(e)
				}
				if e := os.Rename(replacement, path); e != nil {
					t.Fatal(e)
				}
				return ErrFile
			}
			return nil
		}}
		if err := ConfigureAtuin(files, path, 17391); !errors.Is(err, ErrRollback) || !errors.Is(err, ErrCommitUncertain) {
			t.Fatal("concurrent rollback refusal hidden", err)
		}
		b, e := os.ReadFile(path)
		if e != nil || string(b) != "# concurrent\n" {
			t.Fatal("overwrote concurrent config")
		}
	})
	t.Run("persistent sync uncertainty", func(t *testing.T) {
		root := privateRoot(t)
		path := filepath.Join(root, "config.toml")
		if e := os.WriteFile(path, []byte("# original\n"), 0600); e != nil {
			t.Fatal(e)
		}
		files := DiskFiles{fault: func(step string) error {
			if step == "after-rename.dirsync" {
				return ErrFile
			}
			return nil
		}}
		err := ConfigureAtuin(files, path, 17391)
		if !errors.Is(err, ErrCommitUncertain) || !errors.Is(err, ErrRollback) {
			t.Fatal("persistent uncertainty hidden", err)
		}
	})
}
func TestNewConfigRollbackRemoveSyncUncertaintyBlocksRestoration(t *testing.T) {
	root := privateRoot(t)
	path := filepath.Join(root, "config.toml")
	failedRead := false
	files := DiskFiles{fault: func(step string) error {
		if step == "after-rename.read" && !failedRead {
			failedRead = true
			return ErrFile
		}
		if step == "remove.dirsync" {
			return ErrFile
		}
		return nil
	}}
	s, m, e := serviceTestFixture("linux")
	seedManaged(s, m, e)
	w, a, _, _ := fixture("y", "n", "y")
	a.ready = true
	w.Service = s
	opens := 0
	w.OpenAuth = func() (Auth, error) {
		opens++
		if opens == 1 {
			return nil, credentials.ErrLocked
		}
		return a, nil
	}
	var configErr error
	w.Configure = func() error { configErr = ConfigureAtuin(files, path, 17391); return configErr }
	if w.Run(context.Background()) == nil || !errors.Is(configErr, ErrCommitUncertain) || !errors.Is(configErr, ErrRollback) {
		t.Fatal("new-config removal durability uncertainty hidden", configErr)
	}
	if e.active {
		t.Fatal("prior service restored after uncertain remove sync")
	}
	for _, verb := range mutationCalls(e) {
		if verb == "start" || verb == "kickstart" || verb == "bootstrap" {
			t.Fatal("automatic restoration attempted")
		}
	}
	current, err := (DiskFiles{}).Read(path, MaxAtuinBytes)
	if err != nil || current.Exists {
		t.Fatal("rollback did not apply removal before sync failure")
	}
}
func TestServiceJournalsFailedButAppliedWrites(t *testing.T) {
	for _, osname := range []string{"linux", "darwin"} {
		for _, which := range []string{"binary", "config", "unit"} {
			t.Run(osname+which, func(t *testing.T) {
				s, m, e := serviceTestFixture(osname)
				target := s.binary
				if which == "config" {
					target = s.opts.ConfigPath
				}
				if which == "unit" {
					target = s.unit
				}
				applied := false
				m.afterApply = func(path string, out WriteOutcome) (WriteOutcome, error) {
					if path == target && !applied {
						applied = true
						out.Durable = false
						return out, ErrCommitUncertain
					}
					return out, nil
				}
				err := s.Install(context.Background())
				if !errors.Is(err, ErrCommitUncertain) || errors.Is(err, ErrRollback) || !applied {
					t.Fatal("lost applied write/error", err)
				}
				for _, path := range []string{s.binary, s.opts.ConfigPath, s.unit} {
					if m.files[path].Exists {
						t.Fatal("failed-applied file not removed", which)
					}
				}
				if e.active || e.enabled {
					t.Fatal("activated after write failure")
				}
			})
		}
	}
	for _, badRollback := range []bool{false, true} {
		t.Run("prior running "+fmt.Sprint(badRollback), func(t *testing.T) {
			s, m, e := serviceTestFixture("linux")
			_, _ = m.Replace(s.unit, Snapshot{}, s.content, 0600, false)
			_, _ = m.Replace(s.binary, Snapshot{}, []byte("SYNTHETIC-IDENTITY:old"), 0755, false)
			e.active = true
			e.enabled = true
			first := true
			m.afterApply = func(path string, out WriteOutcome) (WriteOutcome, error) {
				if path == s.binary && first {
					first = false
					m.rollbackFail = badRollback
					out.Durable = false
					return out, ErrCommitUncertain
				}
				return out, nil
			}
			err := s.Install(context.Background())
			if !errors.Is(err, ErrCommitUncertain) || errors.Is(err, ErrRollback) != badRollback {
				t.Fatal("wrong primary/secondary outcome", err)
			}
			if e.active == badRollback {
				t.Fatal("automatic restore ignored rollback safety")
			}
		})
	}
}
func TestAtuinActivePathEvidence(t *testing.T) {
	for _, kind := range []string{"xdg-absent-legacy-only", "new-only", "unknown-empty", "environment-only"} {
		t.Run(kind, func(t *testing.T) {
			root := privateRoot(t)
			home := filepath.Join(root, "home")
			xdg, configRoot := "", ""
			legacy := filepath.Join(home, ".config", "atuin", "config.toml")
			target := legacy
			switch kind {
			case "xdg-absent-legacy-only":
				xdg = filepath.Join(root, "xdg")
			case "new-only":
				target = filepath.Join(home, ".atuin", "config.toml")
			case "environment-only":
				configRoot = filepath.Join(root, "override")
				target = filepath.Join(configRoot, "config.toml")
			}
			if kind != "unknown-empty" {
				if e := ConfigureAtuin(DiskFiles{}, target, 17391); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := ResolveAtuin("", home, xdg, configRoot, "", DiskFiles{}); e == nil {
				t.Fatal("uniqueness mistaken for active path", kind)
			}
			if path, e := ResolveAtuin(target, home, xdg, configRoot, "", DiskFiles{}); e != nil || path != target {
				t.Fatal("explicit selection refused")
			}
		})
	}
	root := privateRoot(t)
	home := filepath.Join(root, "home")
	xdg := filepath.Join(root, "xdg")
	active := filepath.Join(xdg, "atuin", "config.toml")
	if e := ConfigureAtuin(DiskFiles{}, active, 17391); e != nil {
		t.Fatal(e)
	}
	if path, e := ResolveAtuin("", home, xdg, "", "", DiskFiles{}); e != nil || path != active {
		t.Fatal("known layout agreement refused")
	}
}
func TestTOMLUTF8TypesCommentsAndNaN(t *testing.T) {
	for _, prefix := range [][]byte{{0xff, 0xfe}, {0xfe, 0xff}} {
		// A comment before [ai] ensures removing UTF8 validation would let the
		// decoder's BOM stripping bypass it, rather than failing only table scanning.
		data := append(append([]byte(nil), prefix...), []byte("# synthetic\n[ai]\nenabled=false\n")...)
		if _, e := PatchAtuin(data, 17391); e == nil {
			t.Fatal("UTF16 BOM accepted before UTF8 check")
		}
	}
	for _, text := range []string{"[ai]\nenabled=[\n false, # preserve this comment\n]\n", "[ai]\nendpoint=[\n 'old', # preserve\n]\n", "[ai]\napi_token={secret='SYNTHETIC'}\n", "[ai]\nenabled='false'\n", "[ai]\nendpoint=true\n", "[ai]\napi_token=42\n"} {
		root := privateRoot(t)
		path := filepath.Join(root, "config.toml")
		if e := os.WriteFile(path, []byte(text), 0600); e != nil {
			t.Fatal(e)
		}
		if e := ConfigureAtuin(DiskFiles{}, path, 17391); !errors.Is(e, ErrTOML) {
			t.Fatal("invalid authorized type accepted")
		}
		b, _ := os.ReadFile(path)
		backups, _ := filepath.Glob(filepath.Join(root, ".atuin-ai-backup-*"))
		if string(b) != text || len(backups) != 0 {
			t.Fatal("refusal removed embedded comments or made a backup")
		}
	}
	original := "number=nan\nvalues=[nan, {x=nan}]\n[ai]\nmodel='unknown' # exact\n"
	patched, e := PatchAtuin([]byte(original), 17391)
	if e != nil || !strings.Contains(string(patched), "number=nan\nvalues=[nan, {x=nan}]\n") {
		t.Fatal("unchanged NaN comparison failed", e)
	}
	again, e := PatchAtuin(patched, 17391)
	if e != nil || !bytes.Equal(patched, again) {
		t.Fatal("NaN config not idempotent")
	}
}
