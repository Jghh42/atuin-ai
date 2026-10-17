// Package httpserver implements the loopback-only Atuin HTTP boundary.
package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/catalog"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
)

// Server owns admission control and downstream HTTP writes.
type Server struct {
	http *http.Server
	log  *slog.Logger
}

// New remains safely unavailable without explicit runtime dependencies. The
// executable composes the production lifecycle via NewWithRuntime.
func New(cfg config.Config, logger *slog.Logger) (*Server, error) {
	return NewWithCatalog(cfg, logger, catalog.New(nil, cfg.DefaultModel))
}

type CatalogSource interface {
	Get(context.Context) (catalog.Snapshot, error)
}

// NewWithCatalog injects the same snapshot source for listing and chat resolution.
func NewWithCatalog(cfg config.Config, logger *slog.Logger, source CatalogSource) (*Server, error) {
	return newWithCatalog(cfg, logger, source, nil)
}

// newWithCatalog retains the private decoder observer for boundary tests.
func newWithCatalog(cfg config.Config, logger *slog.Logger, source CatalogSource, decoded func(atuin.Request)) (*Server, error) {
	return newWithRuntime(cfg, logger, Runtime{Catalog: source}, decoded)
}

func newWithRuntime(cfg config.Config, logger *slog.Logger, runtime Runtime, decoded func(atuin.Request)) (*Server, error) {
	source := runtime.Catalog
	if source == nil {
		return nil, errors.New("catalog source required")
	}
	return buildServer(cfg, logger, source, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timeout := runtime.PreflightTimeout
		if timeout <= 0 {
			timeout = 8 * time.Second
		}
		preflight, stop := context.WithTimeout(r.Context(), timeout)
		defer stop()
		if runtime.Credentials != nil {
			cred, err := runtime.Credentials.Current(preflight)
			if err != nil || cred.AccessToken == "" || cred.Identity.Account == "" || cred.Identity.Provider == "" || preflight.Err() != nil {
				writeError(w, 503, "upstream_unavailable", "Upstream login required")
				return
			}
		}
		snapshot, err := source.Get(preflight)
		if err != nil || preflight.Err() != nil {
			writeError(w, 503, "models_unavailable", "Valid application login required for bundled models; check default_model override")
			return
		}
		response := snapshot.Response()
		models := atuin.ModelCatalog{Default: response.Default, Models: make(map[string]string)}
		for _, m := range response.Models {
			models.Models[m.Alias] = m.Alias
		}
		decoder := atuin.Decoder{Catalog: models}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, 400, "invalid_request", "Unable to read request body")
			return
		}
		request, err := decoder.Decode(body)
		if err != nil {
			var protocolError *atuin.Error
			if errors.As(err, &protocolError) {
				status := http.StatusBadRequest
				if protocolError.Code == "internal_error" {
					status = http.StatusInternalServerError
				}
				writeError(w, status, protocolError.Code, protocolError.Message)
				return
			}
			writeError(w, 500, "internal_error", "Unable to decode chat request")
			return
		}
		if decoded != nil {
			decoded(request)
		}
		if runtime.Provider == nil || runtime.Credentials == nil || preflight.Err() != nil {
			writeError(w, http.StatusServiceUnavailable, "upstream_unavailable", "Upstream login required")
			return
		}
		// Synthetic catalog sources may omit identity; production Cache never does.
		if id := snapshot.Identity(); id.Provider != "" {
			r = r.WithContext(credentials.BindIdentity(r.Context(), id))
		}
		streamChat(w, r, runtime, request)
	}))
}

// newServer accepts a chat implementation for deterministic boundary tests.
func newServer(cfg config.Config, logger *slog.Logger, chat http.Handler) (*Server, error) {
	return buildServer(cfg, logger, catalog.New(nil, cfg.DefaultModel), chat)
}

func buildServer(cfg config.Config, logger *slog.Logger, source CatalogSource, chat http.Handler) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	slots := make(chan struct{}, config.MaxConcurrentChats)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Match literally: no redirecting path cleanup or implicit HEAD route.
		switch r.URL.Path {
		case "/api/cli/models":
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				writeError(w, 405, "method_not_allowed", "Method not allowed")
				return
			}
			snapshot, err := source.Get(r.Context())
			if err != nil {
				writeError(w, 503, "models_unavailable", "Valid application login required for bundled models; check default_model override")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(snapshot.Response())
		case "/api/cli/chat":
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", "POST")
				writeError(w, 405, "method_not_allowed", "Method not allowed")
				return
			}
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				w.Header().Set("Retry-After", "1")
				writeError(w, 503, "busy", "Too many concurrent chat requests")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodyBytes)
			defer r.Body.Close()
			body, err := io.ReadAll(r.Body)
			if err != nil {
				var limit *http.MaxBytesError
				if errors.As(err, &limit) {
					writeError(w, 413, "request_too_large", "Request body exceeds 4 MiB")
				} else {
					writeError(w, 400, "invalid_request", "Unable to read request body")
				}
				return
			}
			// The runner bounds generation separately from the downstream context,
			// so an upstream timeout can still produce a terminal error.
			r.Body = io.NopCloser(bytes.NewReader(body))
			chat.ServeHTTP(w, r)
		default:
			writeError(w, 404, "not_found", "Route not found")
		}
	})
	return &Server{http: &http.Server{
		Addr:              net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Port)),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      config.GenerationTimeout + 10*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		// Do not allow net/http diagnostics to include untrusted request details.
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}, log: logger}, nil
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}{code, message})
}

// Listen always uses IPv4 loopback; callers cannot configure a host.
func (s *Server) Listen() (net.Listener, error) { return net.Listen("tcp4", s.http.Addr) }

// Serve rejects externally supplied non-loopback listeners as defense in depth.
// Cancellation drains active handlers for at most five seconds, then cancels
// them and closes their connections.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || addr.IP.To4() == nil || !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		_ = listener.Close()
		return errors.New("server requires an IPv4 127.0.0.1 listener")
	}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.http.BaseContext = func(net.Listener) context.Context { return base }
	finished := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-ctx.Done():
			deadline, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			if err := s.http.Shutdown(deadline); err != nil {
				cancel()
				_ = s.http.Close()
			}
		case <-finished:
		}
	}()
	s.log.Info("listening", "address", listener.Addr().String())
	err := s.http.Serve(&boundedListener{Listener: listener, slots: make(chan struct{}, maxConnections)})
	close(finished)
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
