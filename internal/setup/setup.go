// Package setup implements the attended, opt-in native setup workflow. It never
// validates saved credentials by refreshing, discovering models or inferring.
package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Jghh42/atuin-ai/internal/credentials"
)

var ErrCanceled = errors.New("init canceled; completed independent steps are retained")
var ErrSetup = errors.New("init incomplete; see terminal instructions (no private details logged)")

type Auth interface {
	Status() credentials.Status
	Login(context.Context, func(credentials.DevicePrompt) error) error
	Close() error
}

type Terminal interface {
	io.Writer
	// Answer returns a bounded line, or an error on EOF/cancellation. No default yes.
	Answer(context.Context) (string, error)
}

type Service interface {
	Preview() string
	// Running recognizes ONLY this application's exact owned user-service file.
	Running(context.Context) (bool, error)
	Stop(context.Context) error
	Restore(context.Context) error
	Install(context.Context) error
}

type Wizard struct {
	TTY           Terminal
	OpenAuth      func() (Auth, error)
	Service       Service
	Configure     func() error
	ConfigPreview string
	PrepareConfig func() (string, error)
}

func ask(ctx context.Context, t Terminal, text string) (bool, error) {
	if ctx.Err() != nil {
		return false, ErrCanceled
	}
	if _, e := fmt.Fprint(t, text+" [y/N] "); e != nil {
		return false, ErrCanceled
	}
	a, e := t.Answer(ctx)
	if e != nil || ctx.Err() != nil {
		return false, ErrCanceled
	}
	return strings.EqualFold(strings.TrimSpace(a), "y") || strings.EqualFold(strings.TrimSpace(a), "yes"), nil
}

// Run requires an already opened controlling terminal BEFORE OpenAuth is called.
func (w Wizard) Run(ctx context.Context) (result error) {
	if w.TTY == nil || w.OpenAuth == nil {
		return errors.New("init requires an interactive controlling terminal; no changes made")
	}
	if ctx.Err() != nil {
		return ErrCanceled
	}
	t := w.TTY
	stopped := false
	safeRestore := true
	var auth Auth
	defer func() {
		if auth != nil {
			if auth.Close() != nil {
				safeRestore = false
				if result == nil {
					result = ErrSetup
				}
				fmt.Fprintln(t, "Credential shutdown failed; service not restarted. Stop manually before retrying.")
			}
		}
		if stopped && safeRestore { // independent restoration budget, including after Ctrl-C
			restore, cancel := context.WithTimeout(context.Background(), supervisorTimeout)
			defer cancel()
			if w.Service.Restore(restore) != nil {
				fmt.Fprintln(t, "Previously running service could not be restored; start it manually.")
				if result == nil {
					result = ErrSetup
				}
			}
		}
	}()
	var e error
	auth, e = w.OpenAuth()
	if errors.Is(e, credentials.ErrLocked) {
		// Do not probe services for ordinary ready authentication or non-lock failures.
		running, probeErr := w.Service.Running(ctx)
		if probeErr != nil || !running {
			fmt.Fprintln(t, "Application store is locked. Stop the server manually, then retry; never remove the lock file.")
			return ErrSetup
		}
		yes, err := ask(ctx, t, "Managed user service owns the application lifetime. Stop it temporarily for init?")
		if err != nil {
			return err
		}
		if !yes {
			return nil
		}
		// Stop may take effect even when its command reports cancellation/failure.
		// Record the reconciliation obligation BEFORE dispatch, not after success.
		stopped = true
		if w.Service.Stop(ctx) != nil {
			fmt.Fprintln(t, "Managed-service stop did not complete; no authentication attempted. Prior state will be reconciled only when safe.")
			if ctx.Err() != nil {
				return ErrCanceled
			}
			return ErrSetup
		}
		auth, e = w.OpenAuth()
	}
	if e != nil {
		fmt.Fprintln(t, "Cannot open application state; investigate permissions/corruption or stop a foreground server. Do not remove locks or edit credentials.")
		return ErrSetup
	}
	if !auth.Status().LoggedIn {
		yes, err := ask(ctx, t, "No usable local application login. Log in now?")
		if err != nil {
			return err
		}
		if !yes {
			fmt.Fprintln(t, "Login declined; no service or Atuin configuration changes.")
			return nil
		}
		if auth.Login(ctx, func(p credentials.DevicePrompt) error {
			_, err := fmt.Fprintf(t, "Open %s and enter code: %s\n", credentials.ActivationURL, p.UserCode)
			return err
		}) != nil {
			fmt.Fprintln(t, "Login did not complete; no service or Atuin configuration changes.")
			if ctx.Err() != nil {
				return ErrCanceled
			}
			return ErrSetup
		}
		if !auth.Status().LoggedIn {
			return ErrSetup
		}
	} else {
		fmt.Fprintln(t, "Existing local login retained (upstream acceptance not checked).")
	}
	// Close is a barrier, not a defer after supervisor startup.
	if auth.Close() != nil {
		auth = nil
		safeRestore = false
		fmt.Fprintln(t, "Credential shutdown failed; do not start service until investigated.")
		return ErrSetup
	}
	auth = nil
	fmt.Fprintln(t, w.Service.Preview())
	yes, err := ask(ctx, t, "Install and start user service?")
	if err != nil {
		return err
	}
	partial := false
	if yes {
		if installErr := w.Service.Install(ctx); installErr != nil {
			if errors.Is(installErr, ErrRollback) {
				safeRestore = false
				fmt.Fprintln(t, "Service rollback/restoration failed; do not start until investigated manually.")
			}
			fmt.Fprintln(t, "Service setup failed or unsupported; use foreground serve. Existing managed state was rolled back where safe.")
			if ctx.Err() != nil {
				if errors.Is(installErr, ErrRollback) {
					return errors.Join(ErrCanceled, ErrRollback)
				}
				return ErrCanceled
			}
			partial = true
		} else {
			stopped = false
			fmt.Fprintln(t, "User service started and verified active.")
		}
	}
	if w.PrepareConfig != nil {
		preview, err := w.PrepareConfig()
		if err != nil {
			fmt.Fprintln(t, "Atuin path unavailable or ambiguous; select --atuin-config PATH. Authentication and valid service retained; init partially complete.")
			return ErrSetup
		}
		w.ConfigPreview = preview
	}
	fmt.Fprintln(t, w.ConfigPreview)
	yes, err = ask(ctx, t, "Configure Atuin to use the local endpoint?")
	if err != nil {
		return err
	}
	if yes {
		if configErr := w.Configure(); configErr == nil {
			fmt.Fprintln(t, "Atuin configuration ready; only ai.enabled, ai.endpoint and ai.api_token authorized. ai.model retained.")
		} else {
			if errors.Is(configErr, ErrRollback) {
				safeRestore = false
				fmt.Fprintln(t, "Atuin rollback is uncertain; automatic prior-service restoration is blocked. Investigate the private backup locally.")
			}
			fmt.Fprintln(t, "Atuin configuration failed; any applied change was restored only when comparison and durability checks succeeded. Check the private backup if present. Select an unambiguous --atuin-config path or edit [ai] manually: enabled=true, endpoint=the displayed localhost URL, api_token=empty. Retain model and use OSS protocol. Authentication and valid service setup are retained.")
			partial = true
		}
	}
	if partial {
		fmt.Fprintln(t, "Init partially complete; independent successful steps retained.")
		return ErrSetup
	}
	fmt.Fprintln(t, "Init complete. No upstream acceptance test was performed.")
	return nil
}
