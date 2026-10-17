package setup

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func verifyNativeProcess(ctx context.Context, pid int, binary string, args []string, uid int) error {
	return (processInspector{root: "/proc"}).verify(ctx, pid, binary, args, uid)
}

// root is a private synthetic-proc fixture seam; production always uses /proc.
// Never read environ, credentials, cwd, descriptors or unrelated processes.
type processInspector struct{ root string }

func procRead(dir *os.File, name string, limit int) ([]byte, error) {
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrService
	}
	f := os.NewFile(uintptr(fd), "managed process metadata")
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil || len(b) > limit {
		return nil, ErrService
	}
	return b, nil
}
func processStart(stat []byte) (string, error) {
	i := strings.LastIndex(string(stat), ") ")
	if i < 0 {
		return "", ErrService
	}
	fields := strings.Fields(string(stat[i+2:]))
	if len(fields) < 20 {
		return "", ErrService
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return "", ErrService
	}
	return fields[19], nil
}
func (p processInspector) verify(ctx context.Context, pid int, binary string, args []string, uid int) error {
	if ctx.Err() != nil || pid <= 0 || uid <= 0 || !validPath(binary) {
		return ErrService
	}
	path := filepath.Join(p.root, strconv.Itoa(pid))
	dir, err := directory(path, false)
	if err != nil {
		return ErrService
	}
	defer dir.Close()
	first, err := procRead(dir, "stat", 4096)
	if err != nil {
		return err
	}
	start, err := processStart(first)
	if err != nil {
		return err
	}
	status, err := procRead(dir, "status", 64<<10)
	if err != nil {
		return err
	}
	owned := false
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, "Uid:") {
			if owned {
				return ErrService
			}
			fields := strings.Fields(strings.TrimPrefix(line, "Uid:"))
			if len(fields) != 4 {
				return ErrService
			}
			for _, field := range fields {
				n, e := strconv.Atoi(field)
				if e != nil || n != uid {
					return ErrService
				}
			}
			owned = true
		}
	}
	if !owned {
		return ErrService
	}
	cmdline, err := procRead(dir, "cmdline", 16<<10)
	if err != nil || len(cmdline) == 0 || cmdline[len(cmdline)-1] != 0 {
		return ErrService
	}
	if !equalStrings(strings.Split(string(cmdline[:len(cmdline)-1]), "\x00"), args) {
		return ErrService
	}
	// The proc exe magic link is intentionally followed, but only for this exact
	// already UID-checked PID. Compare its inode to a nofollow anchored stable exe.
	exeFD, err := unix.Openat(int(dir.Fd()), "exe", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrService
	}
	defer unix.Close(exeFD)
	stableDir, err := directory(filepath.Dir(binary), false)
	if err != nil {
		return ErrService
	}
	defer stableDir.Close()
	stableFD, err := unix.Openat(int(stableDir.Fd()), filepath.Base(binary), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrService
	}
	defer unix.Close(stableFD)
	var running, stable unix.Stat_t
	if unix.Fstat(exeFD, &running) != nil || unix.Fstat(stableFD, &stable) != nil || stable.Mode&unix.S_IFMT != unix.S_IFREG || stable.Nlink != 1 || stable.Uid != uint32(uid) || stable.Mode&0111 == 0 || stable.Mode&0022 != 0 || stable.Mode&07000 != 0 || running.Dev != stable.Dev || running.Ino != stable.Ino {
		return ErrService
	}
	last, err := procRead(dir, "stat", 4096)
	if err != nil {
		return err
	}
	end, err := processStart(last)
	if err != nil || start != end || ctx.Err() != nil {
		return ErrService
	}
	return nil
}
