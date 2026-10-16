package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestExplicitApprovalRequired(t *testing.T) {
	for _, args := range [][]string{nil, {"login"}, {"refresh"}, {"logout"}, {"login", "--approve-live", "--callback-port", "0"}, {"login", "--approve-live", "unexpected"}, {"unknown", "--approve-live"}} {
		var out bytes.Buffer
		if code := run(context.Background(), args, &out); code != 2 {
			t.Fatal("unsafe command accepted")
		}
		if strings.Contains(out.String(), "authorize?") || strings.Contains(out.String(), "state=") {
			t.Fatal("authorization URL leaked")
		}
	}
}
