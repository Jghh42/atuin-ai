package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Jghh42/atuin-ai/internal/siwcproof"
)

func run(ctx context.Context, args []string, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, "Use login, prove, status, refresh, or logout with --approve-live.")
		return 2
	}
	fs := flag.NewFlagSet("siwc-proof", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	approved := fs.Bool("approve-live", false, "explicitly approve live OAuth operations")
	port := fs.Int("callback-port", 1455, "loopback callback port (1455 only)")
	model := fs.String("model", "", "visible account model for inference proof")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || !*approved || *port != 1455 {
		fmt.Fprintln(out, "Explicit --approve-live is required; callback port must be 1455.")
		return 2
	}
	var err error
	switch args[0] {
	case "login":
		fmt.Fprintln(out, "Open http://127.0.0.1:1455/ in your browser once ready.")
		err = siwcproof.Login(ctx, *port)
	case "prove":
		err = siwcproof.WithRecord(ctx, func(record siwcproof.Record) error {
			if record.AccessToken == "" || !time.Now().Before(record.ExpiresAt) {
				return errors.New("no valid proof credential")
			}
			catalog, e := siwcproof.ReadCatalog(ctx, record.AccessToken)
			if e != nil {
				return e
			}
			selected, e := siwcproof.SelectModel(catalog, *model)
			if e != nil {
				return e
			}
			report, e := siwcproof.ProveInference(ctx, record.AccessToken, selected)
			if writeErr := json.NewEncoder(out).Encode(struct {
				Catalog              siwcproof.CatalogProof   `json:"catalog"`
				Inference            siwcproof.InferenceProof `json:"inference"`
				CredentialGeneration uint64                   `json:"credential_generation"`
			}{catalog, report, record.Generation}); writeErr != nil {
				return errors.New("proof report write failed")
			}
			return e
		})
	case "status":
		err = siwcproof.WithRecord(ctx, func(record siwcproof.Record) error {
			ready := record.ExpiresAt.Add(-5 * time.Minute)
			if record.EarliestRefreshAt.After(ready) {
				ready = record.EarliestRefreshAt
			}
			return json.NewEncoder(out).Encode(struct {
				LoggedIn       bool      `json:"logged_in"`
				Generation     uint64    `json:"generation"`
				ExpiresAt      time.Time `json:"expires_at"`
				RefreshReadyAt time.Time `json:"refresh_ready_at"`
			}{record.AccessToken != "", record.Generation, record.ExpiresAt, ready})
		})
	case "refresh":
		err = siwcproof.Refresh(ctx)
	case "logout":
		err = siwcproof.Logout(ctx)
	default:
		fmt.Fprintln(out, "Unknown operation.")
		return 2
	}
	if err != nil {
		fmt.Fprintln(out, "SIWC proof operation failed.")
		return 1
	}
	fmt.Fprintln(out, "SIWC proof operation completed.")
	return 0
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stdout))
}
