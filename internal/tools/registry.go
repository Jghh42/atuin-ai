package tools

import "github.com/Jghh42/atuin-ai/internal/atuin"

// Definitions returns the terminal suggestion tool followed by exactly the
// advertised v1 local tools. Invocation markers, v2 markers, and unknown names
// do not enable tools. Every call owns its slice and schema bytes.
// Ordering and gates follow src/atuin_ai_core/domain/tools.gleam client_tools()
// at 565877e97ed3ff4840481b8d47e12eb2f9854039; see
// docs/specs/atuin-ai-server-api.md, Capability strings and local tools, and
// TestDefinitionsCapabilityMatrix / TestDefinitionsMutationIndependent.
func Definitions(caps atuin.Capabilities) []Definition {
	definitions := make([]Definition, 0, len(registry))
	for _, entry := range registry {
		if entry.capability == "" || caps.Has(entry.capability) {
			definitions = append(definitions, Definition{
				Name:        entry.name,
				Description: entry.description,
				Parameters:  []byte(entry.schema),
			})
		}
	}
	return definitions
}

// Strings keep the shared registry immutable; provider-specific projections and
// local execution policy belong outside this package.
var registry = [...]struct {
	capability, name, description, schema string
}{
	{"", "suggest_command", "Offer a command for the user to run, with an explanation and confidence and risk ratings. Use plain text instead when no command is being proposed.", suggestCommandSchema},
	{"client_v1_read_file", "read_file", "Inspect a local file or directory. File lines include one-based numbers and tabs. Read only unseen ranges unless an edit reports that the file changed; do not re-read merely to check freshness.", readFileSchema},
	{"client_v1_edit_file", "edit_file", "Replace exact text in a file already read with read_file. Require a unique match unless replace_all is true. Prefer targeted edits over rewriting; re-read after a changed-file error, not preemptively.", editFileSchema},
	{"client_v1_write_file", "write_file", "Write complete file contents, creating parent directories as needed. Existing files require overwrite=true; use edit_file for localized changes.", writeFileSchema},
	{"client_v1_atuin_history", "atuin_history", "Find past commands in Atuin history. Prefer the narrowest useful scope: workspace for a project, directory for a location, session for the current shell, host for this machine, or global for all machines.", atuinHistorySchema},
	{"client_v1_atuin_output", "atuin_output", "Retrieve captured command output by history ID. For the latest command, use the ID in turn context directly. Request focused inclusive line ranges, including negative endpoints for the tail; avoid fetching large outputs in full. Returned lines have one-based numbers and tabs.", atuinOutputSchema},
	{"client_v1_execute_shell_command", "execute_shell_command", "Run a shell command when its result is needed or the user prefers direct execution. Prefer specialized tools and read-only operations; avoid destructive work without clear intent. Suggest a command instead when execution is unnecessary.", executeShellCommandSchema},
	{"client_v1_load_skill", "load_skill", "Fetch a relevant skill's full instructions by its listed name. Skills contain user workflows and conventions; embedded shell substitutions are resolved in the returned content.", loadSkillSchema},
}
