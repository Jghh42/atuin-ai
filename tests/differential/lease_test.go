package differential

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"regexp"
	"syscall"
	"testing"
	"time"
)

type lease struct {
	run, id string
	writer  io.Writer
}

func startLease(t *testing.T) lease {
	t.Helper()
	var nonce [12]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		t.Fatal(e)
	}
	id := hex.EncodeToString(nonce[:])
	run := os.Getenv("ATUIN_COMPAT_RUN_LABEL")
	if run == "" {
		run = "atuin-phase8-local-" + id
	}
	if !regexp.MustCompile(`^atuin-phase8-[a-zA-Z0-9_.-]{1,100}$`).MatchString(run) {
		t.Fatal("invalid owned run label")
	}
	// Ownership/watchdog registration precedes ALL network/container creation.
	cmd := exec.Command("python3", "-I", "docker_watchdog.py", "watch", run, id)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // survive parent test-group interrupts
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	writer, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = writer.Close()
		select {
		case e := <-done:
			if e != nil {
				t.Errorf("independent lease cleanup: %v", e)
			}
		case <-time.After(35 * time.Second):
			_ = cmd.Process.Kill()
			t.Error("lease cleanup timeout")
		}
	})
	return lease{run: run, id: id, writer: writer}
}
func (l lease) create(t *testing.T, args ...string) string {
	t.Helper()
	if _, e := io.WriteString(l.writer, "begin\n"); e != nil {
		t.Fatal(e)
	}
	result := docker(t, args...)
	if _, e := io.WriteString(l.writer, "end\n"); e != nil {
		t.Fatal(e)
	}
	return result
}
func (l lease) labels() []string {
	return []string{"--label", "ai.atuin.phase8.run=" + l.run, "--label", "ai.atuin.phase8.lease=" + l.id}
}
