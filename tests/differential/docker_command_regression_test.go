package differential

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDockerSuccessWarningsAreNotMachineOutput(t *testing.T) {
	root := t.TempDir()
	fake := filepath.Join(root, "docker")
	script := `#!/bin/sh
[ "$1" = '--host=unix:///var/run/docker.sock' ] || exit 91
printf 'WARNING: synthetic cold-image pull/cgroup diagnostic before stdout\n' >&2
case "$2" in
 create) printf '%s\n' 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' ;;
 envelope) cat >/dev/null; printf '%s\n' '{"status":200,"body":"SYNTHETIC_ONLY","session":"synthetic-session"}' ;;
 failure) printf 'SYNTHETIC_FAILURE_STDOUT\n'; printf 'SYNTHETIC_FAILURE_STDERR\n' >&2; exit 7 ;;
 generic) printf '80/tcp -> 127.0.0.1:12345\n' ;;
 *) exit 92 ;;
esac
printf 'WARNING: synthetic diagnostic after stdout\n' >&2
`
	if err := os.WriteFile(fake, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run := func(mode string) dockerOutput {
		out, err := executeDocker(ctx, fake, root, strings.NewReader("synthetic input"), mode)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.Stderr, "before stdout") || !strings.Contains(out.Stderr, "after stdout") {
			t.Fatal("success diagnostics lost")
		}
		return out
	}
	out := run("create")
	id, err := resourceID(out.Stdout)
	if err != nil || id != strings.Repeat("a", 64) {
		t.Fatal("stderr contaminated resource ID", err)
	}
	if _, err := resourceID(out.Stdout + out.Stderr); err == nil {
		t.Fatal("old combined streams would silently pass as ID")
	}
	response := run("envelope")
	result, err := decodeOracleEnvelope(response.Stdout)
	if err != nil || result.Status != 200 || result.Body != "SYNTHETIC_ONLY" || result.Session != "synthetic-session" {
		t.Fatal("stderr contaminated HTTP envelope", err)
	}
	if _, err := decodeOracleEnvelope(response.Stdout + response.Stderr); err == nil {
		t.Fatal("combined diagnostics accepted as JSON")
	}
	if _, err := decodeOracleEnvelope(response.Stdout + ` {}`); err == nil {
		t.Fatal("multiple envelopes accepted")
	}
	// Generic commands are NOT forced through the ID validator. In particular,
	// nonempty stdout from `docker port` still fails the unchanged no-port gate.
	if strings.TrimSpace(run("generic").Stdout) != "80/tcp -> 127.0.0.1:12345" {
		t.Fatal("generic output filtered")
	}
	failed, err := executeDocker(ctx, fake, root, nil, "failure")
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 {
		t.Fatal("command failure hidden", err)
	}
	if !strings.Contains(failed.Stdout, "SYNTHETIC_FAILURE_STDOUT") || !strings.Contains(failed.Stderr, "SYNTHETIC_FAILURE_STDERR") || !strings.Contains(err.Error(), "SYNTHETIC_FAILURE_STDOUT") || !strings.Contains(err.Error(), "SYNTHETIC_FAILURE_STDERR") {
		t.Fatal("failure diagnostics lost")
	}
}

func TestDockerCreateIDRejectsMalformedStdout(t *testing.T) {
	for _, v := range []string{"", strings.Repeat("a", 12), strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("a", 64) + "\n" + strings.Repeat("b", 64), "WARNING\n" + strings.Repeat("a", 64)} {
		if _, err := resourceID(v); err == nil {
			t.Fatalf("invalid ID stdout accepted: %q", v)
		}
	}
	if id, err := resourceID(" \n" + strings.Repeat("a", 64) + "\r\n"); err != nil || id != strings.Repeat("a", 64) {
		t.Fatal("normal output whitespace rejected")
	}
}
