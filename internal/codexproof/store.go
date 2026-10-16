// Package codexproof implements an isolated, opt-in Codex device-login proof.
package codexproof

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var errSafe = errors.New("codex proof operation failed")

// Record contains secrets. Never log or serialize it outside the private store.
type Record struct {
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Generation   uint64    `json:"generation"`
}

// String and GoString prevent accidental token disclosure through fmt logging.
func (Record) String() string   { return "[private Codex record]" }
func (Record) GoString() string { return "[private Codex record]" }

type store struct {
	dir  string
	lock *os.File
}

func stateDir() (string, error) {
	h, e := os.UserHomeDir()
	if e != nil {
		return "", errSafe
	}
	return filepath.Join(h, ".atuin-ai-codex-proof"), nil
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
	s := &store{d, f}
	if s.cleanOrphans() != nil {
		s.close()
		return nil, errSafe
	}
	return s, nil
}

// Remove only our own private, unlinked temporary records while holding the
// lifetime lock. Never follow symlinks or remove another file class.
func (s *store) cleanOrphans() error {
	entries, e := os.ReadDir(s.dir)
	if e != nil {
		return errSafe
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".record-") {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		f, e := privateOpen(path, syscall.O_RDONLY)
		if e != nil {
			return errSafe
		}
		_ = f.Close()
		if os.Remove(path) != nil {
			return errSafe
		}
	}
	return nil
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

// WithRecord holds the exclusive lifetime lock while fn uses the current secrets.
// The callback must not retain or log the record, or call another store operation.
func WithRecord(ctx context.Context, fn func(Record) error) error {
	if ctx.Err() != nil || fn == nil {
		return errSafe
	}
	s, e := openStore()
	if e != nil {
		return errSafe
	}
	defer s.close()
	r, e := s.load()
	if e != nil || ctx.Err() != nil {
		return errSafe
	}
	return fn(r)
}

// StatusInfo contains no account identifier or credentials.
type StatusInfo struct {
	LoggedIn   bool
	ExpiresAt  time.Time
	Generation uint64
}

// Status reports retained login state, not a claim that the server accepts it.
func Status() (StatusInfo, error) {
	s, e := openStore()
	if e != nil {
		return StatusInfo{}, errSafe
	}
	defer s.close()
	r, e := s.load()
	if e != nil {
		return StatusInfo{}, errSafe
	}
	return StatusInfo{LoggedIn: r.AccessToken != "" && r.RefreshToken != "", ExpiresAt: r.ExpiresAt, Generation: r.Generation}, nil
}

// Logout clears local credentials only. Remote revocation is unsupported.
func Logout(ctx context.Context) error {
	if ctx.Err() != nil {
		return errSafe
	}
	s, e := openStore()
	if e != nil {
		return errSafe
	}
	defer s.close()
	r, e := s.load()
	if e != nil || ctx.Err() != nil {
		return errSafe
	}
	return s.save(Record{Generation: r.Generation + 1})
}
