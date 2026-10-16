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
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/httpserver"
	"github.com/Jghh42/atuin-ai/internal/provider/codex"
)

type Runtime struct {
	Manager   *credentials.Manager
	Catalog   *catalog.Cache
	HTTP      httpserver.Runtime
	transport *http.Transport
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
	transport := opts.Transport
	var owned *http.Transport
	if transport == nil {
		owned = credentials.NewTransport()
		transport = owned
	}
	store, e := credentials.OpenStore(path)
	if e != nil {
		if owned != nil {
			owned.CloseIdleConnections()
		}
		return nil, e
	}
	var cache *catalog.Cache
	manager, e := credentials.NewManager(store, credentials.ManagerOptions{OAuth: credentials.OAuthClient{Transport: transport, Now: opts.Now, Wait: opts.Wait}, Invalidate: func() { cache.Invalidate() }})
	if e != nil {
		store.Close()
		if owned != nil {
			owned.CloseIdleConnections()
		}
		return nil, e
	}
	// Bind once, before exposing either source. No dynamic cache/source rebind.
	cache = catalog.New(manager, codex.Discovery{Transport: transport}, cfg.DefaultModel, opts.Now)
	native := codex.RequestClient{Credentials: manager, Transport: transport, Now: opts.Now}
	return &Runtime{Manager: manager, Catalog: cache, HTTP: httpserver.Runtime{Credentials: manager, Catalog: cache, Provider: native}, transport: owned}, nil
}
func (r *Runtime) Close() error {
	e := r.Manager.Close()
	if r.transport != nil {
		r.transport.CloseIdleConnections()
	}
	return e
}
