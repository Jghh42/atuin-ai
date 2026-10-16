package codex

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/provider"
)

var (
	ErrStream      = errors.New("invalid Codex response stream")
	ErrRefusal     = errors.New("codex response refused")
	ErrUnsupported = errors.New("unsupported Codex response output")
)

// Stream implements one generation. It deliberately ignores MIME: the private
// backend has returned nonstandard Content-Type in the pinned live proof.
// Recognizable, bounded SSE and an explicit completed snapshot are still required.
func (c RequestClient) Stream(ctx context.Context, req provider.Request, sink func(provider.Event) error) (provider.Usage, error) {
	return c.stream(ctx, req, sink, config.GenerationTimeout, config.UpstreamInactivityTimeout)
}

var _ provider.Provider = RequestClient{}

func (c RequestClient) stream(parent context.Context, req provider.Request, sink func(provider.Event) error, generation, inactivity time.Duration) (provider.Usage, error) {
	if sink == nil {
		return provider.Usage{}, ErrRequest
	}
	deadline, stop := context.WithTimeout(parent, generation)
	defer stop()
	ctx, cancel := context.WithCancelCause(deadline)
	defer cancel(nil)
	activity := make(chan struct{}, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		timer := time.NewTimer(inactivity)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-activity:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(inactivity)
			case <-timer.C:
				cancel(context.DeadlineExceeded)
				return
			}
		}
	}()
	defer func() { cancel(nil); <-finished }()
	response, err := c.Open(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return provider.Usage{}, context.Cause(ctx)
		}
		return provider.Usage{}, err
	}
	closed := make(chan struct{})
	closeOnCancel := context.AfterFunc(ctx, func() { _ = response.Body.Close(); close(closed) })
	defer func() {
		if !closeOnCancel() {
			<-closed
		} else {
			_ = response.Body.Close()
		}
	}()
	reader := activityReader{response.Body, func() {
		select {
		case activity <- struct{}{}:
		default:
		}
	}}
	usage, err := parseStream(&reader, func(event provider.Event) error {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		if err := sink(event); err != nil {
			cancel(err)
			return err
		}
		return nil
	})
	if ctx.Err() != nil {
		return provider.Usage{}, context.Cause(ctx)
	}
	return usage, err
}

type activityReader struct {
	io.Reader
	active func()
}

func (r *activityReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.active()
	}
	return n, err
}
