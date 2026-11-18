//go:build linux || darwin

package credentials

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// ClientToken returns the local client bearer token, durably creating it on
// first use. The lifetime store lock serializes creation and rotation.
func (s *DiskStore) ClientToken() (string, error) {
	token, ok, e := s.readClientToken()
	if e != nil || ok {
		return token, e
	}
	if s.readOnly {
		return "", ErrStore
	}
	return s.writeClientToken()
}

// RotateClientToken durably replaces the token. A running server keeps the
// token it loaded at startup, so rotation requires the exclusive store lock.
func (s *DiskStore) RotateClientToken() (string, error) {
	if s.readOnly {
		return "", ErrStore
	}
	return s.writeClientToken()
}

func (s *DiskStore) writeClientToken() (string, error) {
	token, e := newClientToken()
	if e != nil {
		return "", e
	}
	ctx, cancel := context.WithTimeout(context.Background(), persistenceTimeout)
	defer cancel()
	if s.replace(ctx, clientTokenFile, []byte(token+"\n")) != nil {
		return "", ErrStore
	}
	return token, nil
}

func (s *DiskStore) readClientToken() (string, bool, error) {
	f, e := s.open(clientTokenFile, unix.O_RDONLY, 0)
	if errors.Is(e, unix.ENOENT) {
		return "", false, nil
	}
	if e != nil {
		return "", false, ErrStore
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 128))
	token := strings.TrimSuffix(string(b), "\n")
	if e != nil || !ValidClientToken(token) {
		return "", false, ErrStore
	}
	return token, true, nil
}

// ReadClientToken reads an existing token WITHOUT the store lock, so the token
// of a running server can be displayed. Writers replace it by atomic rename.
// It never creates directories or files.
func ReadClientToken(path string) (string, error) {
	fd, e := openDirectory(path, false)
	if errors.Is(e, os.ErrNotExist) {
		return "", ErrNoClientToken
	}
	if e != nil {
		return "", ErrStore
	}
	s := &DiskStore{dir: os.NewFile(uintptr(fd), "credential directory"), readOnly: true}
	defer s.Close()
	token, ok, e := s.readClientToken()
	if e != nil {
		return "", e
	}
	if !ok {
		return "", ErrNoClientToken
	}
	return token, nil
}
