package codex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/tools"
	"github.com/Jghh42/atuin-ai/internal/turn"
)

type beginFailure struct{ credentials.Storage }

func (beginFailure) Begin(context.Context, credentials.Record) error { return credentials.ErrStore }
func TestPreparedNativeFailedBeginJoinsCallbacksAndRevokesPendingPublish(t *testing.T) {
	for _, operation := range []string{"login", "logout", "refresh"} {
		t.Run(operation, func(t *testing.T) {
			f := newPreparedFixture(t, func(f *preparedFixture, _ map[string]any) string {
				r := reasonOutput()
				r["id"] = fmt.Sprintf("reason-%d", f.posts)
				m := msg("visible")
				m["id"] = fmt.Sprintf("message-%d", f.posts)
				return complete(r, m, fn(fmt.Sprintf("item-%d", f.posts), fmt.Sprintf("backend-%d", f.posts), `{"file_path":"synthetic"}`))
			}, func(s credentials.Storage) credentials.Storage { return beginFailure{s} })
			request := initialRequest()
			first := runPrepared(t, f.client, request)
			if len(first.calls) != 1 {
				t.Fatal("initial protected state missing")
			}
			request = appendPublicHistory(request, first, false, false)
			key := filepath.Join(f.path, "continuation-v1", "key.bundle")
			before, e := os.ReadFile(key)
			if e != nil {
				t.Fatal(e)
			}
			pending, e := f.client.(provider.PreparedProvider).Prepare(context.Background(), provider.Request{Conversation: request})
			if e != nil {
				t.Fatal(e)
			}
			defer pending.Close()
			var held []tools.Call
			if _, e = pending.Stream(context.Background(), func(event provider.Event) error { held = append(held, event.Calls...); return nil }); e != nil {
				t.Fatal(e)
			}
			validated, e := tools.ValidateBatch(held, request.Config.Capabilities)
			if e != nil {
				t.Fatal(e)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			turnDone := make(chan struct{})
			var toolFrames int
			go func() {
				turn.Run(context.Background(), f.client, request, turn.SinkFunc(func(_ context.Context, event turn.Event) error {
					if event.Name == "text" {
						close(entered)
						<-release
					}
					if event.Name == "tool_call" {
						toolFrames++
					}
					return nil
				}))
				close(turnDone)
			}()
			<-entered
			if operation == "refresh" {
				f.now = f.now.Add(12*time.Hour - credentials.AccessSkew)
			}
			lifeDone := make(chan error, 1)
			go func() {
				switch operation {
				case "login":
					lifeDone <- f.manager.Login(context.Background(), func(credentials.DevicePrompt) error { t.Error("OAuth/display after failed Begin"); return nil })
				case "logout":
					lifeDone <- f.manager.Logout(context.Background())
				case "refresh":
					_, e := f.manager.Current(context.Background())
					lifeDone <- e
				}
			}()
			<-pending.(*preparedGeneration).lease.Context().Done()
			select {
			case <-lifeDone:
				t.Fatal("unusable publication returned before native/sink cleanup joined")
			default:
			}
			emitted := false
			publishDone := make(chan error, 1)
			go func() {
				publishDone <- pending.Publish(context.Background(), validated, func(context.Context, []tools.Call) error { emitted = true; return nil })
			}()
			select {
			case e := <-publishDone:
				if e == nil || emitted {
					t.Fatal("revoked pending generation published")
				}
			case <-time.After(time.Second):
				t.Fatal("pending Publish deadlocked with Suspend")
			}
			close(release)
			select {
			case e := <-lifeDone:
				if e != credentials.ErrStore {
					t.Fatal("failed Begin result changed", e)
				}
			case <-time.After(time.Second):
				t.Fatal("native cleanup join did not finish")
			}
			<-turnDone
			if toolFrames != 0 {
				t.Fatal("private generation released tools after unusable publication")
			}
			after, e := os.ReadFile(key)
			if e != nil || string(after) != string(before) {
				t.Fatal("failed Begin purged/changed durable key")
			}
		})
	}
}
