package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Kept separate from TestPinnedCLI's original twenty-case contract. The Python
// coordinator additionally denies egress, starts actual production server
// processes, and holds only done across two graceful process restarts.
func TestPinnedReasoningCLI(t *testing.T) {
	cli := pinnedBinary(t)
	server, helper := os.Getenv("ATUIN_REASONING_SERVER"), os.Getenv("ATUIN_REASONING_HELPER")
	for _, path := range []string{server, helper} {
		info, e := os.Lstat(path)
		if e != nil || !filepath.IsAbs(path) || !info.Mode().IsRegular() {
			t.Fatal("explicit regular absolute reasoning proof server/helper required")
		}
	}
	out := os.Getenv("ATUIN_REASONING_OUT")
	if out == "" {
		out = filepath.Join(t.TempDir(), "proof")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "python3", "-B", "-I", "../reasoning/run.py", "--server", server, "--helper", helper, "--cli", cli, "--out", out)
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8", "HOME=" + t.TempDir(), "PYTHONDONTWRITEBYTECODE=1"}
	if manifest := os.Getenv("ATUIN_TEST_BWRAP_MANIFEST"); manifest != "" {
		command.Env = append(command.Env, "ATUIN_TEST_BWRAP_MANIFEST="+manifest)
	}
	b, e := command.CombinedOutput()
	if e != nil {
		t.Fatalf("separate reasoning proof: %v\n%s", e, b)
	}
	t.Log(string(b))
}
