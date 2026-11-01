//go:build linux || darwin

package credentials

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// ContinuationDirectory is a fixed-child capability, not an arbitrary fd/path
// API. It remains subordinate to DiskStore's lifetime flock. The caller must
// close it before DiskStore and serialize all operations including Close.
type ContinuationDirectory struct{ dir *os.File }
type ContinuationEntry struct{ payload *continuationEntryPayload }
type continuationEntryPayload struct {
	name  string
	bytes int64
}

func (e ContinuationEntry) Name() string {
	if e.payload == nil {
		return ""
	}
	return e.payload.name
}
func (e ContinuationEntry) Bytes() int64 {
	if e.payload == nil {
		return 0
	}
	return e.payload.bytes
}

func (ContinuationDirectory) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "[private continuation directory]")
}
func (ContinuationDirectory) MarshalJSON() ([]byte, error) {
	return []byte(`"[private continuation directory]"`), nil
}
func (*ContinuationDirectory) LogValue() slog.Value {
	return slog.StringValue("[private continuation directory]")
}
func (ContinuationEntry) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "[private continuation entry]")
}
func (ContinuationEntry) MarshalJSON() ([]byte, error) {
	return []byte(`"[private continuation entry]"`), nil
}
func (*ContinuationEntry) LogValue() slog.Value {
	return slog.StringValue("[private continuation entry]")
}

func (s *DiskStore) OpenContinuationDirectory() (*ContinuationDirectory, error) {
	if s == nil || s.readOnly || s.dir == nil || s.lock == nil {
		return nil, ErrStore
	}
	root := int(s.dir.Fd())
	fd, e := unix.Openat(root, "continuation-v1", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(e, unix.ENOENT) {
		if unix.Mkdirat(root, "continuation-v1", 0700) != nil || s.dir.Sync() != nil {
			return nil, ErrStore
		}
		fd, e = unix.Openat(root, "continuation-v1", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	}
	if e != nil {
		return nil, ErrStore
	}
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&07777 != 0700 || st.Uid != uint32(os.Geteuid()) {
		unix.Close(fd)
		return nil, ErrStore
	}
	return &ContinuationDirectory{os.NewFile(uintptr(fd), "private continuation directory")}, nil
}
func lowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func continuationName(n string) bool {
	return n == "key.bundle" || strings.HasSuffix(n, ".sealed") && lowerHex(strings.TrimSuffix(n, ".sealed"), 32) || strings.HasPrefix(n, ".pending-") && lowerHex(strings.TrimPrefix(n, ".pending-"), 32)
}
func (d *ContinuationDirectory) open(n string, flags int) (*os.File, error) {
	if d == nil || d.dir == nil || !continuationName(n) {
		return nil, ErrStore
	}
	fd, e := unix.Openat(int(d.dir.Fd()), n, flags|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if e != nil {
		return nil, e
	}
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&07777 != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		unix.Close(fd)
		return nil, ErrStore
	}
	return os.NewFile(uintptr(fd), "private continuation file"), nil
}

// Scan checks at most 132 entries; unknown or unsafe entries fail without delete.
// Actual file bytes, including pending crash residue, are always accounted.
func (d *ContinuationDirectory) Scan() ([]ContinuationEntry, error) {
	if d == nil || d.dir == nil {
		return nil, ErrStore
	}
	fd, e := unix.Openat(int(d.dir.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrStore
	}
	f := os.NewFile(uintptr(fd), "private continuation scan")
	defer f.Close()
	entries := make([]ContinuationEntry, 0, 132)
	for {
		names, e := f.Readdirnames(16)
		if e != nil && e != io.EOF {
			return nil, ErrStore
		}
		for _, n := range names {
			if len(entries) == 132 || !continuationName(n) {
				return nil, ErrStore
			}
			v, e := d.open(n, unix.O_RDONLY)
			if e != nil {
				return nil, ErrStore
			}
			st, e := v.Stat()
			ce := v.Close()
			if e != nil || ce != nil || st.Size() < 0 {
				return nil, ErrStore
			}
			entries = append(entries, ContinuationEntry{payload: &continuationEntryPayload{name: n, bytes: st.Size()}})
		}
		if e == io.EOF {
			return entries, nil
		}
	}
}
func (d *ContinuationDirectory) Read(n string, max int) ([]byte, bool, error) {
	if max < 0 || max > 2<<20 {
		return nil, false, ErrStore
	}
	f, e := d.open(n, unix.O_RDONLY)
	if errors.Is(e, unix.ENOENT) {
		return nil, false, nil
	}
	if e != nil {
		return nil, false, ErrStore
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || st.Size() > int64(max) {
		return nil, false, ErrStore
	}
	b, e := io.ReadAll(io.LimitReader(f, int64(max)+1))
	if e != nil || len(b) > max {
		clear(b)
		return nil, false, ErrStore
	}
	return b, true, nil
}

// Atomic writes one bounded immutable record (or key replacement). step is a
// synchronous test/fault seam. On success absolutely no filesystem operation
// follows the FINAL dirsync; the caller must separately authorize publication.
// Failed writes leave bounded, safely-owned pending residue for accounting.
func (d *ContinuationDirectory) Atomic(ctx context.Context, n string, b []byte, step func(string) error) error {
	if !continuationName(n) || strings.HasPrefix(n, ".pending-") || len(b) > 2<<20 {
		return ErrStore
	}
	check := func(label string) error {
		if ctx.Err() != nil {
			return ErrStore
		}
		if step != nil && step(label) != nil {
			return ErrStore
		}
		return nil
	}
	existing, e := d.open(n, unix.O_RDONLY)
	if e == nil {
		existing.Close()
		if n != "key.bundle" {
			return ErrStore
		}
	} else if !errors.Is(e, unix.ENOENT) {
		return ErrStore
	}
	var random [16]byte
	if _, e := rand.Read(random[:]); e != nil {
		return ErrStore
	}
	pending := ".pending-" + hex.EncodeToString(random[:])
	if check("create") != nil {
		return ErrStore
	}
	f, e := d.open(pending, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL)
	if e != nil {
		return ErrStore
	}
	if check("write") != nil {
		f.Close()
		return ErrStore
	}
	for len(b) > 0 {
		n, e := f.Write(b)
		if e != nil || n == 0 {
			f.Close()
			return ErrStore
		}
		b = b[n:]
	}
	if check("fsync") != nil || f.Sync() != nil {
		f.Close()
		return ErrStore
	}
	ce := f.Close()
	if ce != nil || check("close") != nil || check("rename") != nil {
		return ErrStore
	}
	if unix.Renameat(int(d.dir.Fd()), pending, int(d.dir.Fd()), n) != nil {
		return ErrStore
	}
	if check("dirsync") != nil || d.dir.Sync() != nil {
		return ErrStore
	}
	if step != nil && step("after-dirsync") != nil {
		return ErrStore
	}
	return nil
}
func (d *ContinuationDirectory) Remove(n string) error {
	f, e := d.open(n, unix.O_RDONLY)
	if errors.Is(e, unix.ENOENT) {
		return nil
	}
	if e != nil {
		return ErrStore
	}
	if f.Close() != nil {
		return ErrStore
	}
	if unix.Unlinkat(int(d.dir.Fd()), n, 0) != nil {
		return ErrStore
	}
	return nil
}
func (d *ContinuationDirectory) Sync() error {
	if d == nil || d.dir == nil || d.dir.Sync() != nil {
		return ErrStore
	}
	return nil
}
func (d *ContinuationDirectory) Close() error {
	if d == nil || d.dir == nil {
		return nil
	}
	e := d.dir.Close()
	d.dir = nil
	if e != nil {
		return ErrStore
	}
	return nil
}
