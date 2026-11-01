package codex

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jghh42/atuin-ai/internal/provider"
)

type checkpointBody struct {
	io.ReadCloser
	once   sync.Once
	closed chan struct{}
}

func (b *checkpointBody) Close() error {
	e := b.ReadCloser.Close()
	b.once.Do(func() { close(b.closed) })
	return e
}
func TestReviewLargeDoneCancellationClosesNativeWithoutTools(t *testing.T) {
	for _, kind := range []string{"reasoning", "message"} {
		for _, deadline := range []bool{false, true} {
			f := newPreparedFixture(t, func(*preparedFixture, map[string]any) string { return "" })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if deadline {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 100*time.Millisecond)
				defer stop()
			}
			closed, writerDone := make(chan struct{}), make(chan struct{})
			transport := integrationTransport(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != ResponsesURL {
					t.Fatal("unexpected synthetic destination")
				}
				rd, wr := io.Pipe()
				go func() {
					defer close(writerDone)
					defer wr.Close()
					for j := 0; j < 16; j++ {
						r := reasonOutput()
						if kind == "reasoning" {
							r["encrypted_content"] = strings.Repeat("z", 900<<10)
						} else {
							r = schemaMessage("")
							r["content"] = []any{}
							r["name"] = strings.Repeat("n", 300<<10)
							r["call_id"] = strings.Repeat("c", 300<<10)
							r["arguments"] = strings.Repeat("a", 256<<10)
						}
						r["id"] = strings.Repeat("x", j+1)
						if _, e := io.WriteString(wr, frame("response.output_item.done", map[string]any{"sequence_number": j, "output_index": j, "item": r})); e != nil {
							return
						}
					}
					if !deadline {
						cancel()
					}
					<-request.Context().Done()
				}()
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: &checkpointBody{ReadCloser: rd, closed: closed}}, nil
			})
			client := NewPrepared(RequestClient{Transport: transport, Now: func() time.Time { return f.now }}, f.manager, f.store)
			generation, e := client.(provider.PreparedProvider).Prepare(ctx, provider.Request{Conversation: initialRequest()})
			if e != nil {
				t.Fatal(e)
			}
			calls := 0
			_, e = generation.Stream(ctx, func(event provider.Event) error { calls += len(event.Calls); return nil })
			generation.Close()
			if e == nil || calls != 0 {
				t.Fatal("canceled/expired native checkpoint stream published tools")
			}
			select {
			case <-closed:
			default:
				t.Fatal("native body not closed before Stream return")
			}
			select {
			case <-writerDone:
			case <-time.After(time.Second):
				t.Fatal("owned synthetic stream writer not joined")
			}
		}
	}
}
