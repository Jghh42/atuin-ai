package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Jghh42/atuin-ai/internal/atuin"
	"github.com/Jghh42/atuin-ai/internal/config"
	"github.com/Jghh42/atuin-ai/internal/credentials"
	"github.com/Jghh42/atuin-ai/internal/provider"
	"github.com/Jghh42/atuin-ai/internal/turn"
)

// Runtime is the production composition seam. Phase 7 supplies its credential
// lifecycle and account-bound catalog; no isolated proof store is imported.
type Runtime struct {
	Credentials credentials.Source
	Catalog     CatalogSource
	Provider    provider.Provider
	// WriteTimeout bounds each frame AND flush, not upstream generation time.
	// Zero selects five seconds. Inject a short bound for slow-client tests.
	WriteTimeout time.Duration
	// PreflightTimeout bounds credential/catalog availability before headers.
	// Zero selects eight seconds; sources must honor context cancellation.
	PreflightTimeout time.Duration
}

func NewWithRuntime(cfg config.Config, logger *slog.Logger, runtime Runtime) (*Server, error) {
	return newWithRuntime(cfg, logger, runtime, nil)
}

func streamChat(w http.ResponseWriter, r *http.Request, runtime Runtime, req atuin.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	timeout := runtime.WriteTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Atuin-AI-Session-Id", req.SessionID)
	turn.Run(ctx, runtime.Provider, req, turn.SinkFunc(func(ctx context.Context, e turn.Event) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Fail closed on wrappers/transports that cannot bound a blocking write.
		if err := controller.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			cancel()
			return err
		}
		data, err := json.Marshal(e.Data)
		if err == nil {
			_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Name, data)
		}
		if err == nil {
			err = controller.Flush()
		}
		if err != nil {
			cancel()
		}
		return err
	}))
}
