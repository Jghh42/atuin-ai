package setup

import "golang.org/x/sys/unix"

func isTerminal(fd int) bool { _, e := unix.IoctlGetTermios(fd, unix.TCGETS); return e == nil }
