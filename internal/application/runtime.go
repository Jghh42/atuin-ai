// Package application composes one lifetime credential manager, one account-
// bound catalog and the native Responses provider. No proof state is imported.
package application

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/continuation"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/httpserver"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
)

type Runtime struct {
	Manager      *credentials.Manager
	Continuation *continuation.Store
	Catalog      *catalog.Cache
	HTTP         httpserver.Runtime
	store        *credentials.DiskStore
	transport    *http.Transport
	discovery    codex.Discovery
	defaultModel string
}

func StateDirectory() (string, error) {
	dir, e := os.UserConfigDir()
	if e != nil {
		return "", credentials.ErrStore
	}
	return filepath.Join(dir, "atuin-ai", "credentials"), nil
}

// New accepts only a transport injection, never upstream URL configuration.
func New(path string, cfg config.Config, transport http.RoundTripper) (*Runtime, error) {
	return NewWithOptions(path, cfg, Options{Transport: transport})
}

// Options provides deterministic test seams, not upstream endpoint settings.
type Options struct {
	Transport http.RoundTripper
	Now       func() time.Time
	Wait      func(context.Context, time.Duration) error
}

func NewWithOptions(path string, cfg config.Config, opts Options) (*Runtime, error) {
	return newWithOptions(path, cfg, opts, false)
}

// NewDiagnostic opens only existing application state without filesystem cleanup
// or creation. Its manager's diagnostic accessor never rotates/saves credentials.
func NewDiagnostic(path string, cfg config.Config, transport http.RoundTripper) (*Runtime, error) {
	return newWithOptions(path, cfg, Options{Transport: transport}, true)
}

// NewStatus preserves ordinary credential status behavior without ever opening,
// creating, inspecting, pruning or exporting continuation state.
func NewStatus(path string, cfg config.Config, transport http.RoundTripper) (*Runtime, error) {
	return newWithOptions(path, cfg, Options{Transport: transport}, false, true)
}

func newWithOptions(path string, cfg config.Config, opts Options, diagnostic bool, statusOnly ...bool) (*Runtime, error) {
	transport := opts.Transport
	var owned *http.Transport
	if transport == nil {
		owned = credentials.NewTransport()
		transport = owned
	}
	open := credentials.OpenStore
	if diagnostic {
		open = credentials.OpenDiagnosticStore
	}
	store, e := open(path)
	if e != nil {
		if owned != nil {
			owned.CloseIdleConnections()
		}
		return nil, e
	}
	var private *continuation.Store
	var dependent credentials.LifecycleDependent
	if !diagnostic && !(len(statusOnly) > 0 && statusOnly[0]) {
		dir, err := store.OpenContinuationDirectory()
		if err != nil {
			store.Close()
			if owned != nil {
				owned.CloseIdleConnections()
			}
			return nil, credentials.ErrStore
		}
		private, err = continuation.NewStoreWithClock(dir, opts.Now)
		if err != nil {
			dir.Close()
			store.Close()
			if owned != nil {
				owned.CloseIdleConnections()
			}
			return nil, credentials.ErrStore
		}
		dependent = private
	}
	var cache *catalog.Cache
	manager, e := credentials.NewManager(store, credentials.ManagerOptions{OAuth: credentials.OAuthClient{Transport: transport, Now: opts.Now, Wait: opts.Wait}, Invalidate: func() {
		if cache != nil {
			cache.Invalidate()
		}
	}, Dependent: dependent})
	if e != nil {
		if private != nil {
			private.Close()
		}
		store.Close()
		if owned != nil {
			owned.CloseIdleConnections()
		}
		return nil, e
	}
	// Every serving-capable runtime requires the local client token; the HTTP
	// server refuses to start without one.
	var token string
	if private != nil {
		if token, e = store.ClientToken(); e != nil {
			_ = manager.Close()
			if owned != nil {
				owned.CloseIdleConnections()
			}
			return nil, credentials.ErrStore
		}
	}
	// Bind once, before exposing either source. No dynamic cache/source rebind.
	discovery := codex.Discovery{Transport: transport}
	cache = catalog.New(manager, cfg.DefaultModel)
	native := codex.RequestClient{Credentials: manager, Transport: transport, Now: opts.Now}
	var generation provider.Provider = native
	if private != nil {
		generation = codex.NewPrepared(native, manager, private)
	}
	return &Runtime{Manager: manager, Continuation: private, Catalog: cache, HTTP: httpserver.Runtime{Credentials: manager, Catalog: cache, Provider: generation, ClientToken: token}, store: store, transport: owned, discovery: discovery, defaultModel: cfg.DefaultModel}, nil
}

// ClientToken returns the local Atuin client bearer token, creating it on first
// use. Diagnostic runtimes are read-only and never create it.
func (r *Runtime) ClientToken() (string, error) {
	if r.HTTP.ClientToken != "" {
		return r.HTTP.ClientToken, nil
	}
	return r.store.ClientToken()
}

// RotateClientToken replaces the token. Callers hold the exclusive store lock,
// so no running server can still be using the old token.
func (r *Runtime) RotateClientToken() (string, error) {
	token, e := r.store.RotateClientToken()
	if e == nil && r.HTTP.ClientToken != "" {
		r.HTTP.ClientToken = token
	}
	return token, e
}
func (r *Runtime) Close() error {
	e := r.Manager.Close()
	if r.transport != nil {
		r.transport.CloseIdleConnections()
	}
	return e
}
