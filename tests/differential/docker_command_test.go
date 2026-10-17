package differential

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

// Docker success stderr includes cold-image pull progress and host warnings.
// It is diagnostic output, NEVER a resource ID or an HTTP JSON envelope.
// The executable parameter is a unit-test seam; real callers use /usr/bin/docker.
type dockerOutput struct{ Stdout, Stderr string }

func executeDocker(ctx context.Context, executable, home string, input io.Reader, args ...string) (dockerOutput, error) {
	cmd := exec.CommandContext(ctx, executable, append([]string{"--host=unix:///var/run/docker.sock"}, args...)...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + home}
	cmd.Stdin = input
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	out := dockerOutput{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		return out, fmt.Errorf("docker %s: %w\nstdout:\n%s\nstderr:\n%s", args[0], err, out.Stdout, out.Stderr)
	}
	return out, nil
}

var fullResourceID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func resourceID(stdout string) (string, error) {
	id := strings.TrimSpace(stdout)
	if !fullResourceID.MatchString(id) {
		return "", fmt.Errorf("invalid Docker create stdout resource ID: %q", id)
	}
	return id, nil
}

type oracleEnvelope struct {
	Status  int    `json:"status"`
	Body    string `json:"body"`
	Session string `json:"session"`
}

func decodeOracleEnvelope(stdout string) (oracleEnvelope, error) {
	var result oracleEnvelope
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		return result, fmt.Errorf("invalid oracle helper stdout envelope: %w", err)
	}
	return result, nil
}
