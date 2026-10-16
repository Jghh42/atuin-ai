// codex-proof is an opt-in native Codex compatibility probe, not the server CLI.
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

	"github.com/Jghh42/atuin-ai/internal/codexproof"
)

func run(ctx context.Context, args []string, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, "Use login, status, prove, refresh or logout with --approve-live.")
		return 2
	}
	flags := flag.NewFlagSet("codex-proof", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	approved := flags.Bool("approve-live", false, "approve isolated live Codex probe")
	page := flags.Bool("device-page", false, "privately display device code on IPv4 loopback port1455")
	model := flags.String("model", "gpt-6.1-sol", "model to validate; default from the pinned Pi catalog, not a claim of access")
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || !*approved {
		fmt.Fprintln(out, "Explicit --approve-live is required; invalid arguments are rejected.")
		return 2
	}
	var err error
	switch args[0] {
	case "login":
		var display func(codexproof.DevicePrompt) error
		if *page {
			var stop func()
			display, stop, err = startDevicePage(ctx)
			if err != nil {
				fmt.Fprintln(out, "Cannot open private device-code display.")
				return 1
			}
			defer stop()
			fmt.Fprintln(out, "Open http://127.0.0.1:1455/ through the existing SSH tunnel to see the private device code. This is a display page, not an OAuth callback.")
		} else {
			// This controlled probe never emits a device code to process output.
			// Production terminal delivery needs a real TTY check and review.
			fmt.Fprintln(out, "Login requires --device-page for private code delivery.")
			return 2
		}
		err = codexproof.Login(ctx, display)
	case "status":
		err = codexproof.WithRecord(ctx, func(r codexproof.Record) error {
			return json.NewEncoder(out).Encode(struct {
				LoggedIn   bool      `json:"logged_in"`
				Generation uint64    `json:"generation"`
				ExpiresAt  time.Time `json:"expires_at"`
			}{r.AccessToken != "", r.Generation, r.ExpiresAt})
		})
	case "prove":
		err = codexproof.WithRecord(ctx, func(r codexproof.Record) error {
			if r.AccessToken == "" || !time.Now().Before(r.ExpiresAt) {
				return errors.New("no current credential")
			}
			report, e := codexproof.ProveInference(ctx, r.AccessToken, r.AccountID, *model)
			// ProveInference returns only locally constructed safe diagnostics,
			// never provider bodies, identifiers, headers or transport errors.
			if e != nil {
				fmt.Fprintln(out, "Inference diagnostic:", e.Error())
			}
			if writeErr := json.NewEncoder(out).Encode(struct {
				Inference  codexproof.InferenceReport `json:"inference"`
				Generation uint64                     `json:"credential_generation"`
			}{report, r.Generation}); writeErr != nil {
				return errors.New("proof report write failed")
			}
			return e
		})
	case "refresh":
		err = codexproof.Refresh(ctx)
	case "logout":
		err = codexproof.Logout(ctx)
		if err == nil {
			fmt.Fprintln(out, "Local credentials cleared; remote revocation is not confirmed.")
		}
	default:
		fmt.Fprintln(out, "Unknown operation.")
		return 2
	}
	if err != nil {
		fmt.Fprintln(out, "Codex proof operation failed; no credentials or provider response bodies are printed.")
		return 1
	}
	fmt.Fprintln(out, "Codex proof operation completed.")
	return 0
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stdout))
}
