package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestNativeProcessInspectorSyntheticProcOnly(t *testing.T) {
	for _, kind := range []string{"valid", "foreign-uid", "foreign-executable", "foreign-argv", "stable-symlink", "malformed-stat", "missing-pid"} {
		t.Run(kind, func(t *testing.T) {
			root := privateRoot(t)
			procRoot := filepath.Join(root, "proc")
			pid := 42
			dir := filepath.Join(procRoot, strconv.Itoa(pid))
			if e := os.MkdirAll(dir, 0700); e != nil {
				t.Fatal(e)
			}
			binary := filepath.Join(root, "stable")
			if e := os.WriteFile(binary, []byte("SYNTHETIC-EXECUTABLE"), 0755); e != nil {
				t.Fatal(e)
			}
			uid := os.Geteuid()
			statusUID := uid
			if kind == "foreign-uid" {
				statusUID = uid + 1
			}
			status := fmt.Sprintf("Name:\tsynthetic\nUid:\t%d\t%d\t%d\t%d\n", statusUID, statusUID, statusUID, statusUID)
			if e := os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0600); e != nil {
				t.Fatal(e)
			}
			fields := []string{"S"}
			for len(fields) < 20 {
				fields = append(fields, "0")
			}
			fields[19] = "123456"
			stat := "42 (synthetic with spaces) " + strings.Join(fields, " ") + "\n"
			if kind == "malformed-stat" {
				stat = "invalid"
			}
			if e := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0600); e != nil {
				t.Fatal(e)
			}
			args := []string{binary, "serve", "--config", filepath.Join(root, "config.toml")}
			actualArgs := append([]string(nil), args...)
			if kind == "foreign-argv" {
				actualArgs[1] = "foreign"
			}
			if e := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(strings.Join(actualArgs, "\x00")+"\x00"), 0600); e != nil {
				t.Fatal(e)
			}
			target := binary
			if kind == "foreign-executable" {
				target = filepath.Join(root, "foreign")
				if e := os.WriteFile(target, []byte("SYNTHETIC-EXECUTABLE"), 0755); e != nil {
					t.Fatal(e)
				}
			}
			if e := os.Symlink(target, filepath.Join(dir, "exe")); e != nil {
				t.Fatal(e)
			}
			// Deliberately unreadable/nonexistent environment: verification MUST NOT use it.
			if e := os.Symlink(filepath.Join(root, "missing-environment"), filepath.Join(dir, "environ")); e != nil {
				t.Fatal(e)
			}
			if kind == "stable-symlink" {
				real := filepath.Join(root, "real")
				if e := os.Rename(binary, real); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(real, binary); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "missing-pid" {
				pid = 999
			}
			err := (processInspector{root: procRoot}).verify(context.Background(), pid, binary, args, uid)
			if (err == nil) != (kind == "valid") {
				t.Fatal("process identity decision wrong", kind, err)
			}
		})
	}
}
