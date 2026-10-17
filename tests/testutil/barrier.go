package testutil

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
)

type recordingWriter struct {
	http.ResponseWriter
	n     *Native
	index int
}

func (w *recordingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *recordingWriter) Write(p []byte) (int, error) {
	w.n.mu.Lock()
	w.n.downstream[w.index] = append(w.n.downstream[w.index], p...)
	w.n.mu.Unlock()
	return w.ResponseWriter.Write(p)
}
func (n *Native) checkRelease() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.heldIndex < 0 {
		return fmt.Errorf("completion barrier was not established")
	}
	if bytes.Contains(n.downstream[n.heldIndex], []byte("event: tool_call\n")) {
		return fmt.Errorf("executable downstream frame observed BEFORE provider completion release")
	}
	n.barrierChecked = true
	return nil
}
func (n *Native) AssertBuffered(t *testing.T) {
	t.Helper()
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.barrierChecked {
		t.Fatal("downstream completion barrier not asserted")
	}
	if bytes.Count(n.downstream[n.heldIndex], []byte("event: tool_call\n")) != 1 {
		t.Fatal("expected exactly one downstream call after release")
	}
}
