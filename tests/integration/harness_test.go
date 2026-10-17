package integration

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestOfflineHarnessIntegrityAndWatchdog(t *testing.T) {
	python, e := exec.LookPath("python3")
	if e != nil {
		t.Skip("Python stdlib harness regressions unavailable; external gates still require Python/bwrap")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	args := []string{"-B", "-I", "../harness_test.py", "Integrity"}
	if runtime.GOOS == "linux" {
		args = append(args, "Watchdog")
	} else {
		t.Log("Linux pidfd/watchdog regression unavailable here; external Linux compatibility gate remains required")
	}
	cmd := exec.CommandContext(ctx, python, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "LANG=C.UTF-8"}
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("offline harness regression: %v\n%s", e, out)
	}
}
