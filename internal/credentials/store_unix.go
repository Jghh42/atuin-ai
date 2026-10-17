//go:build linux || darwin

package credentials

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Storage is held exclusively for the manager lifetime. Begin must durably
// override any old record before external token-rotation side effects. Commit
// must durably save the replacement before clearing that override.
type Storage interface {
	Load() (Record, error)
	Begin(context.Context, Record) error
	Commit(context.Context, Record) error
	Close() error
}

// DiskStore anchors every operation to a validated directory descriptor. It
// never resolves paths again after acquiring that capability. Same-UID hostile
// processes and a hostile root are outside the local-filesystem trust boundary.
type DiskStore struct {
	dir      *os.File
	lock     *os.File
	readOnly bool
	// fault is a deterministic test seam, invoked before each mutating syscall.
	fault func(string) error
}

func (s *DiskStore) String() string   { return "[private credential store]" }
func (s *DiskStore) GoString() string { return "[private credential store]" }

// OpenStore rejects symlinks and unsafe ancestry while walking descriptors.
// Root-owned sticky directories (e.g. /tmp) may contain isolated test roots;
// the final application directory itself must be euid-owned, exactly 0700.
func OpenStore(path string) (*DiskStore, error) {
	return openStore(path, false)
}

// OpenDiagnosticStore takes the SAME validated lifetime lock, but only opens
// existing directories/files: no mkdir, lock creation, orphan cleanup, or sync.
// Load remains strict, including pending-intent precedence. This is not a token
// import path; callers use only this application's normal state directory.
func OpenDiagnosticStore(path string) (*DiskStore, error) {
	return openStore(path, true)
}

func openStore(path string, diagnostic bool) (*DiskStore, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return nil, ErrStore
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrStore
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(e, unix.ENOENT) && !diagnostic {
			if e = unix.Mkdirat(fd, part, 0700); e == nil {
				// Make newly created ancestry durable before relying on child state.
				if unix.Fsync(fd) != nil {
					unix.Close(fd)
					return nil, ErrStore
				}
				next, e = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
		unix.Close(fd)
		if e != nil {
			return nil, ErrStore
		}
		fd = next
		var st unix.Stat_t
		e = unix.Fstat(fd, &st)
		uid := uint32(os.Geteuid())
		sticky := st.Uid == 0 && st.Mode&unix.S_ISVTX != 0
		safe := e == nil && (st.Uid == 0 || st.Uid == uid) && (st.Mode&0022 == 0 || sticky)
		if i == len(parts)-1 {
			safe = safe && st.Uid == uid && st.Mode&07777 == 0700
		}
		if !safe {
			unix.Close(fd)
			return nil, ErrStore
		}
	}
	s := &DiskStore{dir: os.NewFile(uintptr(fd), "credential directory"), readOnly: diagnostic}
	flags := unix.O_RDWR | unix.O_CREAT
	if diagnostic {
		flags = unix.O_RDONLY
	}
	lock, e := s.open("lock", flags, 0600)
	if e != nil {
		s.dir.Close()
		return nil, ErrStore
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lock.Close()
		s.dir.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrLocked
		}
		return nil, ErrStore
	}
	s.lock = lock
	if !diagnostic && s.dir.Sync() != nil {
		s.Close()
		return nil, ErrStore
	}
	if s.checkOrphans(!diagnostic) != nil {
		s.Close()
		return nil, ErrStore
	}
	return s, nil
}
func (s *DiskStore) open(name string, flags int, mode uint32) (*os.File, error) {
	fd, e := unix.Openat(int(s.dir.Fd()), name, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, mode)
	if e != nil {
		return nil, e
	}
	var st unix.Stat_t
	e = unix.Fstat(fd, &st)
	if e != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&07777 != 0600 || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) {
		unix.Close(fd)
		return nil, ErrStore
	}
	return os.NewFile(uintptr(fd), "private credential file"), nil
}
func (s *DiskStore) Close() error {
	// Never unlink or replace the lifetime lock inode.
	if s.lock != nil {
		_ = unix.Flock(int(s.lock.Fd()), unix.LOCK_UN)
		_ = s.lock.Close()
		s.lock = nil
	}
	if s.dir != nil {
		_ = s.dir.Close()
		s.dir = nil
	}
	return nil
}
func (s *DiskStore) checkOrphans(clean bool) error {
	names, e := s.dir.Readdirnames(-1)
	if e != nil {
		return ErrStore
	}
	changed := false
	for _, n := range names {
		if !strings.HasPrefix(n, ".record-") {
			continue
		}
		f, e := s.open(n, unix.O_RDONLY, 0)
		if e != nil {
			return ErrStore
		}
		f.Close()
		if !clean {
			continue
		}
		if unix.Unlinkat(int(s.dir.Fd()), n, 0) != nil {
			return ErrStore
		}
		changed = true
	}
	if changed && s.dir.Sync() != nil {
		return ErrStore
	}
	return nil
}
func (s *DiskStore) read(name string) (Record, bool, error) {
	f, e := s.open(name, unix.O_RDONLY, 0)
	if errors.Is(e, unix.ENOENT) {
		return Record{}, false, nil
	}
	if e != nil {
		return Record{}, false, ErrStore
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, maxRecordBytes+1))
	var r Record
	if e != nil || len(b) > maxRecordBytes || decodeJSON(b, &r, true) != nil || r.validate() != nil {
		return Record{}, false, ErrStore
	}
	return r, true, nil
}
func (s *DiskStore) Load() (Record, error) {
	r, ok, e := s.read("record.json")
	if e != nil {
		return Record{}, ErrStore
	}
	if !ok {
		r = emptyRecord()
	}
	intent, exists, e := s.read("intent.json")
	if e != nil {
		return Record{}, ErrStore
	}
	if exists {
		if intent.State != "unusable" || intent.Generation < r.Generation || intent.AuthEpoch < r.AuthEpoch {
			return Record{}, ErrStore
		}
		return intent, nil
	}
	return r, nil
}
func (s *DiskStore) step(ctx context.Context, name string) error {
	if ctx.Err() != nil {
		return ErrStore
	}
	if s.fault != nil && s.fault(name) != nil {
		return ErrStore
	}
	return nil
}
func (s *DiskStore) atomic(ctx context.Context, name string, r Record) error {
	if r.validate() != nil {
		return ErrStore
	}
	// Refuse unsafe existing destinations, even though rename never follows one.
	f, e := s.open(name, unix.O_RDONLY, 0)
	if e == nil {
		f.Close()
	} else if !errors.Is(e, unix.ENOENT) {
		return ErrStore
	}
	b, e := json.Marshal(r)
	if e != nil || len(b) > maxRecordBytes {
		return ErrStore
	}
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return ErrStore
	}
	temp := ".record-" + hex.EncodeToString(nonce[:])
	f, e = s.open(temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, 0600)
	if e != nil {
		return ErrStore
	}
	defer unix.Unlinkat(int(s.dir.Fd()), temp, 0)
	if s.step(ctx, name+".write") != nil {
		f.Close()
		return ErrStore
	}
	// io.Copy handles short writes; the entire byte record must complete.
	_, e = io.Copy(f, strings.NewReader(string(b)))
	if e == nil {
		e = s.step(ctx, name+".fsync")
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil || s.step(ctx, name+".rename") != nil {
		return ErrStore
	}
	if unix.Renameat(int(s.dir.Fd()), temp, int(s.dir.Fd()), name) != nil {
		return ErrStore
	}
	if s.step(ctx, name+".dirsync") != nil || s.dir.Sync() != nil {
		return ErrStore
	}
	return nil
}
func (s *DiskStore) Begin(ctx context.Context, r Record) error {
	if s.readOnly {
		return ErrStore
	}
	old, e := s.Load()
	// Gaps are permitted after an in-memory reservation whose Begin failed
	// before durability; counters must never go backwards or wrap. Each epoch
	// advance must still be backed by at least one generation advance.
	if e != nil || r.Generation <= old.Generation || r.AuthEpoch < old.AuthEpoch || r.AuthEpoch-old.AuthEpoch > r.Generation-old.Generation {
		return ErrStore
	}
	if r.State != "unusable" {
		return ErrStore
	}
	return s.atomic(ctx, "intent.json", r)
}
func (s *DiskStore) Commit(ctx context.Context, r Record) error {
	if s.readOnly {
		return ErrStore
	}
	intent, ok, e := s.read("intent.json")
	if e != nil || !ok || intent.State != "unusable" || r.Generation != intent.Generation || r.AuthEpoch != intent.AuthEpoch || (r.State != "active" && r.State != "logged_out") {
		return ErrStore
	}
	if s.atomic(ctx, "record.json", r) != nil {
		return ErrStore
	}
	if s.step(ctx, "intent.unlink") != nil {
		return ErrStore
	}
	if unix.Unlinkat(int(s.dir.Fd()), "intent.json", 0) != nil {
		return ErrStore
	}
	// The replacement is already durable. Failure here blocks this manager;
	// restart may use that durable replacement, never the old refresh token.
	if s.step(ctx, "intent.dirsync") != nil || s.dir.Sync() != nil {
		return ErrStore
	}
	return nil
}
