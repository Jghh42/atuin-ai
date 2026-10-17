//go:build linux || darwin

package setup

import (
	"context"
	"strconv"
	"strings"
	"unicode"
)

type serviceState uint8

const (
	stateUnknown serviceState = iota
	stateAbsent
	stateInactive
	stateRunning
	stateForeign
)

func (s *UserService) command(ctx context.Context, verb string) (CommandResult, error) {
	c := Command{Env: s.env}
	if s.opts.OS == "linux" {
		c.Path = "/usr/bin/systemctl"
		c.Args = []string{"--user", verb}
		if verb == "inspect" {
			c.Args = []string{"--user", "show", "--property=" + systemdProperties, "--no-pager", "atuin-ai-server.service"}
		} else if verb != "daemon-reload" {
			c.Args = append(c.Args, "atuin-ai-server.service")
		}
	} else {
		c.Path = "/bin/launchctl"
		domain := "gui/" + strconv.Itoa(s.opts.UID)
		switch verb {
		case "inspect":
			c.Args = []string{"print", domain + "/" + serviceLabel}
		case "stop":
			c.Args = []string{"bootout", domain + "/" + serviceLabel}
		case "kickstart":
			c.Args = []string{"kickstart", domain + "/" + serviceLabel}
		case "start":
			c.Args = []string{"bootstrap", domain, s.unit}
		default:
			return CommandResult{}, ErrService
		}
	}
	return s.opts.Exec.Run(ctx, c)
}
func (s *UserService) mutate(ctx context.Context, verb string) error {
	r, err := s.command(ctx, verb)
	if err != nil || !r.success() {
		return commandFailure(r)
	}
	return nil
}
func (s *UserService) inspect(ctx context.Context, local bool) (serviceState, error) {
	r, err := s.command(ctx, "inspect")
	if !r.complete() {
		return stateUnknown, commandFailure(r)
	}
	if s.opts.OS == "darwin" {
		return s.inspectLaunchd(r, err, local)
	}
	return s.inspectSystemd(ctx, r, err, local)
}

// show output is restricted to the fixed requested properties. Duplicate or
// unsupported representations fail closed, never become absence or ownership.
func properties(data []byte) (map[string]string, error) {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return nil, ErrService
	}
	result := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return nil, ErrService
		}
		if _, exists := result[key]; exists {
			return nil, ErrService
		}
		result[key] = value
	}
	return result, nil
}
func (s *UserService) inspectSystemd(ctx context.Context, r CommandResult, commandErr error, local bool) (serviceState, error) {
	// Only a successfully completed response authorizes documented omission
	// normalization. An exit/transport/cancellation/truncation error is NOT empty.
	if commandErr != nil || !r.complete() || !r.success() {
		return stateUnknown, ErrService
	}
	p, err := properties(r.Stdout)
	if err != nil {
		return stateUnknown, err
	}
	for _, name := range strings.Split(systemdProperties, ",") {
		if _, ok := p[name]; !ok {
			// v258 systemctl-show.c:1417-1434,1515-1574: these specialized
			// struct-array printers emit no line for zero entries, even with --all.
			if name == "EnvironmentFiles" || (name == "ExecStart" && p["LoadState"] == "not-found") {
				p[name] = ""
				continue
			}
			return stateUnknown, ErrService
		}
	}
	if len(p) != len(strings.Split(systemdProperties, ",")) {
		return stateUnknown, ErrService
	}
	if p["LoadState"] == "not-found" && p["FragmentPath"] == "" && p["DropInPaths"] == "" && p["ActiveState"] == "inactive" && p["SubState"] == "dead" && p["MainPID"] == "0" && p["ExecMainPID"] == "0" && p["ExecStart"] == "" && p["EnvironmentFiles"] == "" {
		return stateAbsent, nil
	}
	if !local || p["LoadState"] != "loaded" || p["FragmentPath"] != s.unit || p["DropInPaths"] != "" || p["NeedDaemonReload"] != "no" || p["Type"] != "simple" || p["RemainAfterExit"] != "no" || p["RootDirectory"] != "" || p["EnvironmentFiles"] != "" || p["PassEnvironment"] != "" || p["UnsetEnvironment"] != "" {
		return stateForeign, ErrService
	}
	if !systemdExecMatches(p["ExecStart"], s.binary, s.loadedArguments()) {
		return stateForeign, ErrService
	}
	env, err := systemdWords(p["Environment"])
	if err != nil || !s.environmentMatches(env) {
		return stateForeign, ErrService
	}
	for _, key := range []string{"MainPID", "ExecMainPID"} {
		n, e := strconv.Atoi(p[key])
		if e != nil || n < 0 || strconv.Itoa(n) != p[key] {
			return stateUnknown, ErrService
		}
	}
	switch {
	case p["ActiveState"] == "inactive" && p["SubState"] == "dead" && p["MainPID"] == "0":
		return stateInactive, nil
	case p["ActiveState"] == "failed" && p["SubState"] == "failed" && p["MainPID"] == "0":
		return stateInactive, nil
	case p["ActiveState"] == "active" && p["SubState"] == "running":
		pid, err := strconv.Atoi(p["MainPID"])
		if err != nil || pid <= 0 || p["ExecMainPID"] != p["MainPID"] {
			return stateForeign, ErrService
		}
		if s.processCheck(ctx, pid, s.binary, s.arguments(), s.opts.UID) != nil {
			return stateForeign, ErrService
		}
		s.lastPID = pid
		return stateRunning, nil
	default:
		return stateUnknown, ErrService
	}
}
func (s *UserService) arguments() []string {
	return []string{s.binary, "serve", "--config", s.opts.ConfigPath}
}

// v258 load-fragment.c retains $$ in loaded argv; exec-invoke.c expands it
// only at execution. The @ renderer supplies an independently escaped argv[0].
func (s *UserService) loadedArguments() []string {
	args := s.arguments()
	for i := range args {
		args[i] = strings.ReplaceAll(args[i], "$", "$$")
	}
	return args
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func (s *UserService) environmentMatches(values []string) bool {
	if len(values) != 2 {
		return false
	}
	m := map[string]string{}
	for _, entry := range values {
		key, val, ok := strings.Cut(entry, "=")
		if !ok {
			return false
		}
		if _, exists := m[key]; exists {
			return false
		}
		m[key] = val
	}
	return m["HOME"] == s.opts.Home && m["XDG_CONFIG_HOME"] == s.opts.XDGConfig
}

// v258 systemctl-show.c:1528-1557 prints executable path and strv_join(argv,
// " ") RAW, not quoted/escaped. Compare its native flattened representation to
// our exact owned unit template; never C-unquote a foreign executable path into
// an apparent match. The running process's actual argv boundaries are checked
// independently by processCheck. Ambiguous field delimiters still refuse.
func systemdExecMatches(value, binary string, loaded []string) bool {
	if !strings.HasPrefix(value, "{ ") || !strings.HasSuffix(value, " }") {
		return false
	}
	fields := strings.Split(value[2:len(value)-2], " ; ")
	m := map[string]string{}
	for _, field := range fields {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			return false
		}
		if _, exists := m[k]; exists {
			return false
		}
		m[k] = v
	}
	return m["path"] == binary && m["argv[]"] == strings.Join(loaded, " ") && m["ignore_errors"] == "no"
}

// Decode the shell_maybe_quote(...,0) string-array representation used by
// Environment (bus-print-properties.c:250-280), NOT raw ExecStart structs.
// Escaped $/backtick are literal; this parser never expands environment values.
func systemdWords(value string) ([]string, error) {
	var words []string
	var word strings.Builder
	quote := byte(0)
	started := false
	flush := func() {
		if started {
			words = append(words, word.String())
			word.Reset()
			started = false
		}
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == '\\' {
			i++
			if i >= len(value) {
				return nil, ErrService
			}
			started = true
			switch value[i] {
			case '\\', '"', '\'', '$', '`':
				word.WriteByte(value[i])
			case 's':
				word.WriteByte(' ')
			case 'x':
				if i+2 >= len(value) {
					return nil, ErrService
				}
				n, err := strconv.ParseUint(value[i+1:i+3], 16, 8)
				if err != nil || n < 32 || n == 127 {
					return nil, ErrService
				}
				word.WriteByte(byte(n))
				i += 2
			default:
				return nil, ErrService
			}
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				if c < 32 || c == 127 {
					return nil, ErrService
				}
				word.WriteByte(c)
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			started = true
			continue
		}
		if c == ' ' || c == '\t' {
			flush()
			continue
		}
		if c < 32 || c == 127 {
			return nil, ErrService
		}
		started = true
		word.WriteByte(c)
	}
	if quote != 0 {
		return nil, ErrService
	}
	flush()
	for _, w := range words {
		for _, r := range w {
			if unicode.IsControl(r) {
				return nil, ErrService
			}
		}
	}
	return words, nil
}

func (s *UserService) inspectLaunchd(r CommandResult, commandErr error, local bool) (serviceState, error) {
	// A normal completed ESRCH-style exit with an EXACT target-specific diagnostic
	// is distinct from missing GUI/tools, timeout, cancellation, overflow or a
	// partial print. None of those indeterminate cases authorizes bootstrap/bootout.
	missing := "Could not find service \"" + serviceLabel + "\" in domain for user gui: " + strconv.Itoa(s.opts.UID)
	diagnostic := strings.TrimSpace(string(r.Stderr))
	diagnostic = strings.TrimPrefix(diagnostic, "Bad request.\n")
	if commandErr != nil && r.ExitCode == 113 && len(r.Stdout) == 0 && diagnostic == missing {
		return stateAbsent, nil
	}
	if commandErr != nil || !r.success() {
		return stateUnknown, commandFailure(r)
	}
	fields, blocks, err := launchFields(r.Stdout, "gui/"+strconv.Itoa(s.opts.UID)+"/"+serviceLabel)
	if err != nil {
		return stateUnknown, serviceCause(categoryFormat, err)
	}
	if !local || fields["path"] != s.unit || fields["program"] != s.binary || !equalStrings(blocks["arguments"], s.arguments()) {
		return stateForeign, serviceCause(categoryRegistration, ErrService)
	}
	env := []string{}
	seen := map[string]bool{}
	for _, line := range blocks["environment"] {
		key, val, ok := strings.Cut(line, " => ")
		if !ok || seen[key] {
			return stateForeign, serviceCause(categoryEnvironment, ErrService)
		}
		seen[key] = true
		if key == "XPC_SERVICE_NAME" && val == serviceLabel {
			continue
		}
		// Native launchd adds this exact logging metadata even though our
		// plist does not configure it. Only the observed value is accepted.
		if key == "OSLogRateLimit" && val == "64" {
			continue
		}
		env = append(env, key+"="+val)
	}
	if !s.environmentMatches(env) {
		return stateForeign, serviceCause(categoryEnvironment, ErrService)
	}
	if fields["state"] == "running" {
		pid, err := strconv.Atoi(fields["pid"])
		if err != nil || pid <= 0 {
			return stateForeign, serviceCause(categoryState, ErrService)
		}
		s.lastPID = pid
		return stateRunning, nil
	}
	// This is an owned registration, not a live process. In particular,
	// spawn scheduled must be polled within waitActive's existing budget.
	if (fields["state"] == "not running" || fields["state"] == "waiting" || fields["state"] == "spawn scheduled") && (fields["pid"] == "" || fields["pid"] == "0") {
		return stateInactive, nil
	}
	return stateUnknown, serviceCause(categoryState, ErrService)
}
func launchFields(data []byte, target string) (map[string]string, map[string][]string, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != target+" = {" {
		return nil, nil, ErrService
	}
	scalars := map[string]string{}
	blocks := map[string][]string{}
	depth := 1
	block := ""
	for _, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if line == "}" {
			depth--
			if depth < 0 {
				return nil, nil, ErrService
			}
			if depth == 1 {
				block = ""
			}
			continue
		}
		if depth == 0 {
			return nil, nil, ErrService
		}
		if strings.HasSuffix(line, " = {") {
			if depth == 1 {
				key := strings.TrimSuffix(line, " = {")
				if _, ok := blocks[key]; ok {
					return nil, nil, ErrService
				}
				block = key
				blocks[block] = []string{}
			}
			depth++
			continue
		}
		if depth == 1 {
			key, val, ok := strings.Cut(line, " = ")
			if ok {
				if _, exists := scalars[key]; exists {
					return nil, nil, ErrService
				}
				scalars[key] = val
			}
		} else if depth == 2 && (block == "arguments" || block == "environment") {
			blocks[block] = append(blocks[block], line)
		}
	}
	if depth != 0 {
		return nil, nil, ErrService
	}
	return scalars, blocks, nil
}
