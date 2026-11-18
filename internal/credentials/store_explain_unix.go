//go:build linux || darwin

package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ExplainStore repeats the store's ownership and permission checks with plain
// lstat calls to name the first failing path. It is advisory only: OpenStore
// remains the enforcing check. It returns ErrStore when it finds no cause.
func ExplainStore(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return ErrStore
	}
	uid := uint32(os.Geteuid())
	current := "/"
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		current = filepath.Join(current, part)
		var st unix.Stat_t
		if unix.Lstat(current, &st) != nil {
			return ErrStore // missing directories are created by OpenStore
		}
		last := i == len(parts)-1
		sticky := st.Uid == 0 && st.Mode&unix.S_ISVTX != 0
		switch {
		case st.Mode&unix.S_IFMT == unix.S_IFLNK:
			return storeProblem{fmt.Sprintf("%s is a symlink; point XDG_CONFIG_HOME at the real directory instead", current)}
		case st.Mode&unix.S_IFMT != unix.S_IFDIR:
			return storeProblem{fmt.Sprintf("%s is not a directory", current)}
		case st.Uid != uid && (last || st.Uid != 0):
			return storeProblem{fmt.Sprintf("%s is owned by another user", current)}
		case last && st.Mode&07777 != 0700:
			return storeProblem{fmt.Sprintf("%s must have mode 0700; run: chmod 700 %s", current, current)}
		case st.Mode&0022 != 0 && !sticky:
			return storeProblem{fmt.Sprintf("%s is writable by group or others; run: chmod go-w %s", current, current)}
		}
	}
	for _, name := range []string{"lock", "record.json", "intent.json", clientTokenFile} {
		file := filepath.Join(path, name)
		var st unix.Stat_t
		if unix.Lstat(file, &st) != nil {
			continue
		}
		if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&07777 != 0600 || st.Nlink != 1 || st.Uid != uid {
			return storeProblem{fmt.Sprintf("%s must be a regular 0600 file owned by you with no hard links", file)}
		}
	}
	return ErrStore
}
