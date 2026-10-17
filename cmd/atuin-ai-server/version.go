package main

import (
	"errors"
	"fmt"
	"io"
	"runtime"
)

// Set only by the deterministic packager. Ordinary go builds remain explicitly
// unidentified development builds; do not infer a clean source from build VCS.
var (
	version      = "development"
	revision     = "unknown"
	sourceSHA256 = "unknown"
	sourceDirty  = "unknown"
)

func printVersion(out io.Writer) error {
	_, err := fmt.Fprintf(out, "atuin-ai-server version=%s revision=%s source_sha256=%s dirty=%s target=%s/%s go=%s\n", version, revision, sourceSHA256, sourceDirty, runtime.GOOS, runtime.GOARCH, runtime.Version())
	if err != nil {
		return errors.New("cannot write version")
	}
	return nil
}
