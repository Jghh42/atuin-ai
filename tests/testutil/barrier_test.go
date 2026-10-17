package testutil

import "testing"

func TestBarrierRejectsPrematureExecutableFrames(t *testing.T) {
	n := &Native{heldIndex: 0, downstream: map[int][]byte{0: []byte("event: status\ndata: {}\n\nevent: tool_")}}
	n.downstream[0] = append(n.downstream[0], []byte("call\ndata: {}\n\n")...)
	if n.checkRelease() == nil || n.barrierChecked {
		t.Fatal("permission wait could mask premature downstream tool frame")
	}
	n.downstream[0] = []byte("event: text\ndata: {}\n\n")
	if e := n.checkRelease(); e != nil {
		t.Fatal(e)
	}
}
