// Package safety provides a conservative command-suggestion danger backstop.
// It is a heuristic, NOT a shell parser, sandbox, permission check, or proof of
// safety. A missing warning never authorizes execution. The CLI owns permissions.
package safety

import (
	"regexp"
	"strings"
)

// Warning describes a heuristic match using fixed text, never command content.
type Warning struct{ Category, Message string }

type rule struct {
	expression *regexp.Regexp
	warning    Warning
}

// These independently written patterns cover the pinned core's categories but
// deliberately have no keyword prefilter: that filter hides chmod/pipe/fork
// warnings in the reference. Matching is conservative and can flag quoted text.
var rules = []rule{
	{regexp.MustCompile(`(?i)(^|[^a-z0-9_])(?:sudo|doas)(?:\s|$)`), Warning{"privileged_execution", "Privileged execution can change protected system state."}},
	{regexp.MustCompile(`(?i)(^|[^a-z0-9_])rm(?:\s+[^\s;&|]+)*\s+(?:-[a-z]*r[a-z]*|--recursive)(?:\s|$)`), Warning{"destructive_delete", "Recursive deletion can permanently remove files."}},
	{regexp.MustCompile(`(?i)(^|[^a-z0-9_])(?:dd|shred)(?:\s|$)[^\n;&|]*(?:of\s*=\s*["']?/dev/|/dev/(?:sd|hd|vd|nvme|mmcblk|disk|rdisk))`), Warning{"disk_overwrite", "Writing a disk device can destroy data."}},
	{regexp.MustCompile(`(?i)(^|[^a-z0-9_])(?:mkfs(?:\.[a-z0-9_]+)?|wipefs|sgdisk|fdisk|parted|format)(?:\s|$)`), Warning{"filesystem_destroy", "Formatting or partition changes can destroy filesystems."}},
	{regexp.MustCompile(`(?i)(^|[^a-z0-9_])shred(?:\s|$)`), Warning{"destructive_delete", "Secure deletion can irreversibly destroy data."}},
	{regexp.MustCompile(`(?i)(^|[^a-z0-9_])chmod(?:\s+[^\s;&|]+)*\s+(?:0?777|a\+rwx)(?:\s|$)`), Warning{"permission_risk", "World-writable permissions can expose files or directories."}},
	{regexp.MustCompile(`(?i)(^|[^a-z0-9_])(?:curl|wget)(?:\s|$)[^\n;]*\|\s*(?:(?:sudo|env)\s+)*(?:/[^\s|;]+/)?(?:sh|bash|zsh|fish|nu)(?:\s|$)`), Warning{"pipe_to_shell", "Running downloaded code can execute unreviewed instructions."}},
	{regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`), Warning{"fork_bomb", "A fork bomb can exhaust machine resources."}},
	{regexp.MustCompile(`(?i)(?:^|[\s;&|])[0-9]*>\s*["']?/dev/(?:sd[a-z]|hd[a-z]|vd[a-z]|nvme[0-9]|mmcblk[0-9]|r?disk[0-9])`), Warning{"disk_overwrite", "Redirecting output to a disk device can destroy data."}},
}

// Check scans non-comment lines in stable rule order. It intentionally does not
// skip an entire script just because its first line is a comment. Shell quoting,
// aliases, expansion, encodings, and other languages may evade these patterns.
func Check(command string) []Warning {
	var executable strings.Builder
	for _, line := range strings.Split(command, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		executable.WriteString(line)
		executable.WriteByte('\n')
	}
	text := executable.String()
	var warnings []Warning
	seen := make(map[string]bool)
	for _, r := range rules {
		if !seen[r.warning.Category] && r.expression.MatchString(text) {
			warnings = append(warnings, r.warning)
			seen[r.warning.Category] = true
		}
	}
	return warnings
}

// Escalate raises matched suggestions to high danger and appends fixed warning
// text. It never lowers danger or removes model-supplied notes. Callers validate
// the risk enum before calling. It is idempotent for its own warning messages.
func Escalate(command, danger, notes string) (string, string) {
	warnings := Check(command)
	if len(warnings) == 0 {
		return danger, notes
	}
	for _, w := range warnings {
		if strings.Contains(notes, w.Message) {
			continue
		}
		if notes != "" {
			notes += " "
		}
		notes += w.Message
	}
	return "high", notes
}
