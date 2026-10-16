// Package siwcproof implements an explicitly opted-in, single-user OAuth proof harness.
package siwcproof

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

var errSafe = errors.New("SIWC proof operation failed")

// Record contains secrets. Never log or serialize it outside the private store.
type Record struct {
	HostID            string    `json:"host_id"`
	ClientID          string    `json:"client_id,omitempty"`
	Subject           string    `json:"subject,omitempty"`
	Scopes            []string  `json:"scopes,omitempty"`
	AccessToken       string    `json:"access_token,omitempty"`
	RefreshToken      string    `json:"refresh_token,omitempty"`
	IDToken           string    `json:"id_token,omitempty"`
	ExpiresAt         time.Time `json:"expires_at,omitempty"`
	EarliestRefreshAt time.Time `json:"earliest_refresh_at,omitempty"`
	Generation        uint64    `json:"generation"`
}

// String and GoString prevent accidental token disclosure through fmt logging.
func (Record) String() string   { return "[private SIWC record]" }
func (Record) GoString() string { return "[private SIWC record]" }

type store struct {
	dir  string
	lock *os.File
}

func stateDir() (string, error) {
	h, e := os.UserHomeDir()
	if e != nil {
		return "", errSafe
	}
	return filepath.Join(h, ".atuin-ai-siwc-proof"), nil
}
func openStore() (*store, error) {
	d, e := stateDir()
	if e != nil {
		return nil, errSafe
	}
	return openStoreAt(d)
}
func openStoreAt(d string) (*store, error) {
	// Reject symlinks in every existing component before making private state.
	d, e := filepath.Abs(d)
	if e != nil {
		return nil, errSafe
	}
	var paths []string
	for p := d; p != "/"; p = filepath.Dir(p) {
		paths = append(paths, p)
	}
	for i := len(paths) - 1; i >= 0; i-- {
		p := paths[i]
		st, e := os.Lstat(p)
		if os.IsNotExist(e) {
			if os.Mkdir(p, 0700) != nil {
				return nil, errSafe
			}
			st, e = os.Lstat(p)
		}
		if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return nil, errSafe
		}
		uid := st.Sys().(*syscall.Stat_t).Uid
		// Same-UID processes are trusted. Permit root's sticky temporary
		// directory for isolated tests, but reject other writable ancestry.
		stickyRoot := uid == 0 && st.Mode()&os.ModeSticky != 0
		if (uid != 0 && uid != uint32(os.Geteuid())) || (st.Mode().Perm()&0022 != 0 && !stickyRoot) {
			return nil, errSafe
		}
	}
	st, e := os.Lstat(d)
	if e != nil || st.Mode().Perm() != 0700 {
		return nil, errSafe
	}
	f, e := privateOpen(filepath.Join(d, "lock"), syscall.O_CREAT|syscall.O_RDWR)
	if e != nil {
		return nil, errSafe
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return nil, errSafe
	}
	return &store{d, f}, nil
}
func privateOpen(p string, flags int) (*os.File, error) {
	fd, e := syscall.Open(p, flags|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return nil, errSafe
	}
	f := os.NewFile(uintptr(fd), p)
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Sys().(*syscall.Stat_t).Nlink != 1 || st.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		f.Close()
		return nil, errSafe
	}
	return f, nil
}
func (s *store) close() { _ = syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN); _ = s.lock.Close() }
func (s *store) load() (Record, error) {
	var r Record
	f, e := privateOpen(filepath.Join(s.dir, "record.json"), syscall.O_RDONLY)
	if e != nil {
		if _, se := os.Lstat(filepath.Join(s.dir, "record.json")); os.IsNotExist(se) {
			return r, nil
		}
		return r, errSafe
	}
	defer f.Close()
	b, e := readLimited(f)
	if e != nil || json.Unmarshal(b, &r) != nil {
		return Record{}, errSafe
	}
	return r, nil
}
func (s *store) save(r Record) error {
	p := filepath.Join(s.dir, "record.json")
	if st, e := os.Lstat(p); e == nil {
		if !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Sys().(*syscall.Stat_t).Nlink != 1 || st.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
			return errSafe
		}
	} else if !os.IsNotExist(e) {
		return errSafe
	}
	b, e := json.Marshal(r)
	if e != nil {
		return errSafe
	}
	f, e := os.CreateTemp(s.dir, ".record-*")
	if e != nil {
		return errSafe
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return errSafe
	}
	if os.Rename(f.Name(), p) != nil {
		return errSafe
	}
	d, e := os.Open(s.dir)
	if e != nil {
		return errSafe
	}
	defer d.Close()
	if d.Sync() != nil {
		return errSafe
	}
	return nil
}

// LoadRecord returns secrets from the private store under an exclusive lock.
func LoadRecord() (Record, error) {
	s, e := openStore()
	if e != nil {
		return Record{}, errSafe
	}
	defer s.close()
	return s.load()
}

// WithRecord holds the lifetime lock while fn uses the current tokens.
func WithRecord(ctx context.Context, fn func(Record) error) error {
	if ctx.Err() != nil {
		return errSafe
	}
	s, e := openStore()
	if e != nil {
		return errSafe
	}
	defer s.close()
	r, e := s.load()
	if e != nil {
		return errSafe
	}
	return fn(r)
}
