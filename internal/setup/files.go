//go:build linux || darwin

package setup

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const maxFileBytes = 64 << 20

var ErrFile = errors.New("unsafe, changed or unavailable setup file; use manual configuration")
var ErrCommitUncertain = errors.New("setup write was applied but verification or durability is uncertain")

// WriteOutcome survives errors AFTER rename. After is the staged inode/content
// identity, not a possibly failed reread of the destination. Noop is not Applied.
type WriteOutcome struct {
	After   Snapshot
	Applied bool
	Durable bool
}

type Snapshot struct {
	Data   []byte
	Exists bool
	Mode   os.FileMode
	dev    uint64
	ino    uint64
}

type Files interface {
	Read(string, int) (Snapshot, error)
	Replace(string, Snapshot, []byte, os.FileMode, bool) (WriteOutcome, error)
	Remove(string, Snapshot) error
}

// DiskFiles performs descriptor-relative nofollow walks, anchored atomic writes
// and inode/content compare-before-commit. Same-UID hostile processes/root and
// filesystems without local rename/fsync semantics are outside the boundary.
type DiskFiles struct{ fault func(string) error }

func (d DiskFiles) step(name string) error {
	if d.fault != nil {
		return d.fault(name)
	}
	return nil
}

func validPath(p string) bool {
	if !utf8.ValidString(p) || !filepath.IsAbs(p) || filepath.Clean(p) != p || p == "/" {
		return false
	}
	for _, r := range p {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func directory(path string, create bool) (*os.File, error) {
	if !validPath(path) && path != "/" {
		return nil, ErrFile
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrFile
	}
	if path == "/" {
		return os.NewFile(uintptr(fd), "setup directory"), nil
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(err, unix.ENOENT) && create {
			if err = unix.Mkdirat(fd, part, 0700); err == nil {
				err = unix.Fsync(fd)
				if err == nil {
					next, err = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
				}
			}
		}
		unix.Close(fd)
		if err != nil {
			return nil, ErrFile
		}
		fd = next
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || (st.Uid != 0 && st.Uid != uint32(os.Geteuid())) || (st.Mode&0022 != 0 && !(st.Uid == 0 && st.Mode&unix.S_ISVTX != 0)) {
			unix.Close(fd)
			return nil, ErrFile
		}
	}
	return os.NewFile(uintptr(fd), "setup directory"), nil
}

func readAt(dir *os.File, name string, limit int) (Snapshot, error) {
	fd, e := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(e, unix.ENOENT) {
		return Snapshot{}, nil
	}
	if e != nil {
		return Snapshot{}, ErrFile
	}
	f := os.NewFile(uintptr(fd), "setup file")
	defer f.Close()
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 || st.Mode&07000 != 0 || st.Size > int64(limit) {
		return Snapshot{}, ErrFile
	}
	data, e := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if e != nil || len(data) > limit {
		return Snapshot{}, ErrFile
	}
	var after unix.Stat_t
	if unix.Fstat(fd, &after) != nil || after.Size != st.Size || after.Mtim != st.Mtim || after.Ctim != st.Ctim {
		return Snapshot{}, ErrFile
	}
	return Snapshot{Data: data, Exists: true, Mode: os.FileMode(st.Mode & 0777), dev: uint64(st.Dev), ino: st.Ino}, nil
}
func (DiskFiles) Read(path string, limit int) (Snapshot, error) {
	if !validPath(path) || limit < 1 || limit > maxFileBytes {
		return Snapshot{}, ErrFile
	}
	// Missing ancestry is absence, but every existing ancestor must still be safe.
	dir, e := directory(filepath.Dir(path), false)
	if e != nil {
		// Walk deepest existing prefix to distinguish missing from unsafe ancestry.
		p := filepath.Dir(path)
		for p != "/" {
			_, err := os.Lstat(p)
			if !os.IsNotExist(err) {
				break
			}
			p = filepath.Dir(p)
		}
		d, err := directory(p, false)
		if err != nil {
			return Snapshot{}, ErrFile
		}
		d.Close()
		if p == filepath.Dir(path) {
			return Snapshot{}, ErrFile
		}
		return Snapshot{}, nil
	}
	defer dir.Close()
	return readAt(dir, filepath.Base(path), limit)
}
func same(a, b Snapshot) bool {
	return a.Exists == b.Exists && (!a.Exists || (a.dev == b.dev && a.ino == b.ino && a.Mode == b.Mode && sha256.Sum256(a.Data) == sha256.Sum256(b.Data)))
}

func stage(dir *os.File, data []byte, mode os.FileMode, prefix string) (string, error) {
	// os.CreateTemp is not used: all operations stay on the anchored descriptor.
	for i := 0; i < 8; i++ {
		nonce, err := randomName()
		if err != nil {
			return "", err
		}
		name := prefix + nonce
		fd, e := unix.Openat(int(dir.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
		if errors.Is(e, unix.EEXIST) {
			continue
		}
		if e != nil {
			return "", ErrFile
		}
		f := os.NewFile(uintptr(fd), "setup staging")
		_, e = io.Copy(f, bytes.NewReader(data))
		if e == nil {
			e = f.Chmod(mode)
		}
		if e == nil {
			e = f.Sync()
		}
		ce := f.Close()
		if e != nil || ce != nil {
			unix.Unlinkat(int(dir.Fd()), name, 0)
			return "", ErrFile
		}
		return name, nil
	}
	return "", ErrFile
}
func (d DiskFiles) Replace(path string, old Snapshot, data []byte, mode os.FileMode, backup bool) (WriteOutcome, error) {
	if !validPath(path) || len(data) > maxFileBytes || mode&^0777 != 0 || mode&0022 != 0 {
		return WriteOutcome{}, ErrFile
	}
	dir, e := directory(filepath.Dir(path), true)
	if e != nil {
		return WriteOutcome{}, e
	}
	defer dir.Close()
	current, e := readAt(dir, filepath.Base(path), maxFileBytes)
	if e != nil || !same(current, old) {
		return WriteOutcome{}, ErrFile
	}
	if old.Exists && bytes.Equal(old.Data, data) {
		return WriteOutcome{After: old, Durable: true}, nil
	}
	if old.Exists {
		mode = old.Mode
	}
	name, e := stage(dir, data, mode, ".atuin-ai-stage-")
	if e != nil {
		return WriteOutcome{}, e
	}
	defer unix.Unlinkat(int(dir.Fd()), name, 0)
	// Capture the committed identity before rename; post-rename errors MUST NOT
	// erase the evidence needed by the journal's compare-guarded rollback.
	staged, e := readAt(dir, name, maxFileBytes)
	if e != nil || !staged.Exists || !bytes.Equal(staged.Data, data) {
		return WriteOutcome{}, ErrFile
	}
	if old.Exists && backup {
		_, e = stage(dir, old.Data, 0600, ".atuin-ai-backup-")
		if e != nil || dir.Sync() != nil {
			return WriteOutcome{}, ErrFile
		}
	}
	current, e = readAt(dir, filepath.Base(path), maxFileBytes)
	if e != nil || !same(current, old) {
		return WriteOutcome{}, ErrFile
	}
	if unix.Renameat(int(dir.Fd()), name, int(dir.Fd()), filepath.Base(path)) != nil {
		return WriteOutcome{}, ErrFile
	}
	outcome := WriteOutcome{After: staged, Applied: true}
	verifyErr := d.step("after-rename.read")
	if verifyErr == nil {
		current, verifyErr = readAt(dir, filepath.Base(path), maxFileBytes)
		if verifyErr == nil && !same(current, staged) {
			verifyErr = ErrFile
		}
	}
	syncErr := d.step("after-rename.dirsync")
	if syncErr == nil {
		syncErr = dir.Sync()
	}
	outcome.Durable = syncErr == nil
	if verifyErr != nil || syncErr != nil {
		return outcome, ErrCommitUncertain
	}
	return outcome, nil
}

// rollbackWrite uses a best-effort inode/content guard: detected replacements
// refuse, but comparison followed by rename/unlink is NOT an atomic CAS. Hostile
// same-UID/root processes and nonlocal filesystems are outside the boundary.
// Restoration must itself verify durability before automatic service restore.
func rollbackWrite(files Files, path string, before Snapshot, write WriteOutcome) error {
	if !write.Applied {
		return nil
	}
	if before.Exists {
		outcome, err := files.Replace(path, write.After, before.Data, before.Mode, false)
		if err != nil || !outcome.Durable || !outcome.After.Exists || outcome.After.Mode != before.Mode || !bytes.Equal(outcome.After.Data, before.Data) {
			return errors.Join(ErrRollback, err)
		}
		return nil
	}
	if err := files.Remove(path, write.After); err != nil {
		return errors.Join(ErrRollback, err)
	}
	return nil
}
func (d DiskFiles) Remove(path string, old Snapshot) error {
	if !validPath(path) || !old.Exists {
		return ErrFile
	}
	dir, e := directory(filepath.Dir(path), false)
	if e != nil {
		return e
	}
	defer dir.Close()
	current, e := readAt(dir, filepath.Base(path), maxFileBytes)
	if e != nil || !same(current, old) {
		return ErrFile
	}
	if unix.Unlinkat(int(dir.Fd()), filepath.Base(path), 0) != nil {
		return ErrFile
	}
	if d.step("remove.dirsync") != nil || dir.Sync() != nil {
		return ErrCommitUncertain
	}
	return nil
}
