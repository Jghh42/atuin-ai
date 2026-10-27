package codex

import (
	"context"
	"net"
	"net/url"

	"github.com/Jghh42/atuin-ai/internal/provider"
)

func failure(cause error, phase provider.Phase, category provider.Category) error {
	return provider.WithFailure(cause, provider.NewFailure(phase, category, 0))
}

// Inspect before discarding the original error; never retain it or render it.
// Timeout wins over other network failures anywhere within the total budget.
func isTimeout(err error) (timeout bool) {
	provider.WalkErrors(err, func(e error) bool {
		if e == context.DeadlineExceeded {
			timeout = true
			return true
		}
		// Do not invoke recursive net.OpError.Timeout (which may traverse causes).
		switch e.(type) {
		case *net.OpError, *url.Error:
			return false
		}
		if n, ok := e.(net.Error); ok {
			timeout = n.Timeout()
		}
		return timeout
	})
	return timeout
}
func httpFailure(status int) error {
	category := provider.HTTPRejected
	switch {
	case status == 401:
		category = provider.HTTPUnauthorized
	case status == 403:
		category = provider.HTTPForbidden
	case status == 429:
		category = provider.HTTPRateLimited
	case status >= 500 && status <= 599:
		category = provider.HTTPServerError
	}
	return provider.WithFailure(ErrResponse, provider.NewFailure(provider.PhaseHTTP, category, status))
}

func streamFailure(err error) error {
	// No generic traversal/stringification and no wrapping sink errors.
	switch err {
	case ErrStream:
		return failure(ErrStream, provider.PhaseStream, provider.StreamMalformed)
	case ErrRefusal:
		return failure(ErrRefusal, provider.PhaseStream, provider.Refusal)
	case ErrUnsupported:
		return failure(ErrUnsupported, provider.PhaseStream, provider.UnsupportedOutput)
	default:
		return err
	}
}
