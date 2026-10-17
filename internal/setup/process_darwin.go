package setup

import "context"

// Linux process verification is never substituted on Darwin. launchd inspection
// verifies the fixed GUI UID domain, registration/argv/environment and live PID.
func verifyNativeProcess(context.Context, int, string, []string, int) error { return ErrService }
