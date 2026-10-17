package setup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/credentials"
)

type terminalFixture struct {
	bytes.Buffer
	answers []string
	err     error
}

func (t *terminalFixture) Answer(context.Context) (string, error) {
	if len(t.answers) == 0 {
		if t.err != nil {
			return "", t.err
		}
		return "", io.EOF
	}
	s := t.answers[0]
	t.answers = t.answers[1:]
	return s, nil
}

type authFixture struct {
	ready              bool
	loginErr, closeErr error
	events             *[]string
}

func (a *authFixture) Status() credentials.Status {
	*a.events = append(*a.events, "status")
	return credentials.Status{LoggedIn: a.ready}
}
func (a *authFixture) Login(_ context.Context, display func(credentials.DevicePrompt) error) error {
	*a.events = append(*a.events, "login")
	if a.loginErr != nil {
		return a.loginErr
	}
	if e := display(credentials.DevicePrompt{UserCode: "SYNTHETIC-ONLY"}); e != nil {
		return e
	}
	a.ready = true
	return nil
}
func (a *authFixture) Close() error { *a.events = append(*a.events, "close"); return a.closeErr }

type serviceFixture struct {
	events                                    *[]string
	running                                   bool
	probeErr, stopErr, restoreErr, installErr error
}

func (s *serviceFixture) Preview() string { return "synthetic user service preview" }
func (s *serviceFixture) Running(context.Context) (bool, error) {
	*s.events = append(*s.events, "probe")
	return s.running, s.probeErr
}
func (s *serviceFixture) Stop(context.Context) error {
	*s.events = append(*s.events, "stop")
	return s.stopErr
}
func (s *serviceFixture) Restore(context.Context) error {
	*s.events = append(*s.events, "restore")
	return s.restoreErr
}
func (s *serviceFixture) Install(context.Context) error {
	*s.events = append(*s.events, "install")
	return s.installErr
}
func fixture(answers ...string) (Wizard, *authFixture, *serviceFixture, *[]string) {
	events := []string{}
	a := &authFixture{events: &events}
	s := &serviceFixture{events: &events}
	w := Wizard{TTY: &terminalFixture{answers: answers}, OpenAuth: func() (Auth, error) { events = append(events, "open"); return a, nil }, Service: s, ConfigPreview: "synthetic config", Configure: func() error { events = append(events, "config"); return nil }}
	return w, a, s, &events
}
func hasEvent(events []string, event string) bool {
	for _, e := range events {
		if e == event {
			return true
		}
	}
	return false
}
func TestConsentCombinationsAndCloseBarrier(t *testing.T) {
	for _, ready := range []bool{false, true} {
		for _, service := range []bool{false, true} {
			for _, config := range []bool{false, true} {
				answer := func(b bool) string {
					if b {
						return "yes"
					}
					return ""
				}
				answers := []string{answer(service), answer(config)}
				if !ready {
					answers = append([]string{"y"}, answers...)
				}
				w, a, _, events := fixture(answers...)
				a.ready = ready
				if e := w.Run(context.Background()); e != nil {
					t.Fatal(e)
				}
				if hasEvent(*events, "install") != service || hasEvent(*events, "config") != config || hasEvent(*events, "login") == ready || hasEvent(*events, "probe") {
					t.Fatal(*events)
				}
				closeAt := -1
				for i, e := range *events {
					if e == "close" {
						closeAt = i
					}
					if (e == "install" || e == "config") && closeAt < 0 {
						t.Fatal("mutation before Close barrier", *events)
					}
				}
			}
		}
	}
}
func TestRefusalsCancellationAndPrimaryFailure(t *testing.T) {
	t.Run("no tty", func(t *testing.T) {
		w, _, _, events := fixture()
		w.TTY = nil
		if w.Run(context.Background()) == nil || len(*events) != 0 {
			t.Fatal(*events)
		}
	})
	for _, answer := range []string{"", "n", "arbitrary", "yes please"} {
		w, _, _, events := fixture(answer)
		if w.Run(context.Background()) != nil || hasEvent(*events, "login") || hasEvent(*events, "config") || hasEvent(*events, "install") {
			t.Fatal(*events)
		}
	}
	for _, ready := range []bool{false, true} {
		w, a, _, events := fixture()
		a.ready = ready
		if !errors.Is(w.Run(context.Background()), ErrCanceled) || hasEvent(*events, "install") || hasEvent(*events, "config") {
			t.Fatal(*events)
		}
	}
	w, _, _, events := fixture("y")
	w.OpenAuth = func() (Auth, error) { *events = append(*events, "open"); return nil, credentials.ErrStore }
	if w.Run(context.Background()) == nil || hasEvent(*events, "probe") {
		t.Fatal(*events)
	}
	w, a, _, events := fixture("y")
	a.loginErr = credentials.ErrDenied
	if w.Run(context.Background()) == nil || hasEvent(*events, "install") || hasEvent(*events, "config") {
		t.Fatal(*events)
	}
	w, a, _, events = fixture("y", "y")
	a.ready = true
	a.closeErr = credentials.ErrStore
	if w.Run(context.Background()) == nil || hasEvent(*events, "install") {
		t.Fatal(*events)
	}
	w, a, _, events = fixture("y", "y")
	a.ready = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(w.Run(ctx), ErrCanceled) || hasEvent(*events, "install") {
		t.Fatal(*events)
	}
}
func TestManagedLockStopAndRestore(t *testing.T) {
	for _, decline := range []bool{true, false} {
		answers := []string{"n"}
		if !decline {
			answers = []string{"y", "n"}
		}
		w, a, s, events := fixture(answers...)
		s.running = true
		calls := 0
		w.OpenAuth = func() (Auth, error) {
			calls++
			*events = append(*events, "open")
			if calls == 1 {
				return nil, credentials.ErrLocked
			}
			return a, nil
		}
		if e := w.Run(context.Background()); e != nil {
			t.Fatal(e)
		}
		if hasEvent(*events, "stop") == decline || hasEvent(*events, "restore") == decline {
			t.Fatal(*events)
		}
	}
	w, _, s, events := fixture("y")
	s.running = false
	w.OpenAuth = func() (Auth, error) { return nil, credentials.ErrLocked }
	if w.Run(context.Background()) == nil || hasEvent(*events, "stop") {
		t.Fatal(*events)
	}
	w, a, s, events := fixture("y")
	s.running = true
	s.restoreErr = ErrService
	calls := 0
	w.OpenAuth = func() (Auth, error) {
		calls++
		if calls == 1 {
			return nil, credentials.ErrLocked
		}
		return a, nil
	}
	// EOF after service stop must still restore, and preserve cancellation.
	if !errors.Is(w.Run(context.Background()), ErrCanceled) || !hasEvent(*events, "restore") {
		t.Fatal(*events)
	}
	if !strings.Contains(w.TTY.(*terminalFixture).String(), "could not be restored") {
		t.Fatal("missing secondary failure")
	}
}
func TestPartialSuccessAndUnsafeRestoration(t *testing.T) {
	w, a, s, events := fixture("y", "y")
	a.ready = true
	s.installErr = ErrService
	if w.Run(context.Background()) == nil || !hasEvent(*events, "config") {
		t.Fatal(*events)
	}
	w, a, _, events = fixture("n", "y")
	a.ready = true
	w.Configure = func() error { return ErrTOML }
	if w.Run(context.Background()) == nil || hasEvent(*events, "install") {
		t.Fatal(*events)
	}
	w, a, s, events = fixture("y", "y", "n")
	a.ready = true
	s.running = true
	s.installErr = ErrRollback
	calls := 0
	w.OpenAuth = func() (Auth, error) {
		calls++
		if calls == 1 {
			return nil, credentials.ErrLocked
		}
		return a, nil
	}
	if w.Run(context.Background()) == nil || hasEvent(*events, "restore") {
		t.Fatal(*events)
	}
}
