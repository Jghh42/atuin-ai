package tools

// Schema constraints follow src/atuin_ai_core/domain/tools.gleam at core
// 565877e97ed3ff4840481b8d47e12eb2f9854039 and docs/specs/atuin-ai-server-api.md
// (Tool-call input schemas). Prose is original. Like src/atuin_ai_core/json_schema.gleam
// object(), these objects omit additionalProperties and allow unknown fields.
// Keep this vocabulary narrow: object, string, integer, boolean, array, required,
// enum, minimum/maximum, minItems/maxItems, and description. No strict-mode rules,
// client-only fields, nullable optionals, or client runtime defaults are added.
// TestDefinitionGoldens pins all eight schemas and their model-facing prose.
const (
	suggestCommandSchema = `{
 "type":"object",
 "properties":{
  "command":{"type":"string","description":"Shell command offered for the user to run."},
  "description":{"type":"string","description":"Short explanation of the command."},
  "confidence":{"type":"string","enum":["high","med","low"],"description":"Estimated confidence that the command fits the request."},
  "confidence_notes":{"type":"string","description":"Reason for the confidence rating."},
  "danger":{"type":"string","enum":["high","med","low"],"description":"Risk: high for destructive or sudo commands, med for file changes, low for read-only work."},
  "danger_notes":{"type":"string","description":"Specific risks the user should consider."}
 },
 "required":["command","description","confidence","danger"]
}`
	readFileSchema = `{
 "type":"object",
 "properties":{
  "file_path":{"type":"string","description":"Absolute path or path relative to the working directory."},
  "limit":{"type":"integer","minimum":1,"maximum":1000,"description":"Line count to read; defaults to 100. Unused for directories."},
  "offset":{"type":"integer","description":"Lines to skip; defaults to zero. Unused for directories."}
 },
 "required":["file_path"]
}`
	editFileSchema = `{
 "type":"object",
 "properties":{
  "file_path":{"type":"string","description":"Absolute or working-directory-relative path of the previously read file."},
  "old_string":{"type":"string","description":"Exact existing text, including all whitespace."},
  "new_string":{"type":"string","description":"Replacement text."},
  "replace_all":{"type":"boolean","description":"Replace every match when true; otherwise require one match. Defaults to false."}
 },
 "required":["file_path","old_string","new_string"]
}`
	writeFileSchema = `{
 "type":"object",
 "properties":{
  "file_path":{"type":"string","description":"Absolute path or path relative to the working directory."},
  "content":{"type":"string","description":"Entire contents of the resulting file."},
  "overwrite":{"type":"boolean","description":"Allow replacing an existing file. Defaults to false."}
 },
 "required":["file_path","content"]
}`
	atuinHistorySchema = `{
 "type":"object",
 "properties":{
  "query":{"type":"string","description":"History search text; an empty string requests recent commands."},
  "limit":{"type":"integer","minimum":1,"maximum":50,"description":"Result count; defaults to 10."},
  "filter_modes":{"type":"array","items":{"type":"string","enum":["workspace","directory","session","host","global"]},"description":"Search scopes in preference order; the first permitted scope is used."}
 },
 "required":["query","filter_modes"]
}`
	atuinOutputSchema = `{
 "type":"object",
 "properties":{
  "history_id":{"type":"string","description":"History entry identifier from history results or turn context."},
  "ranges":{"type":"array","items":{"type":"array","items":{"type":"integer"},"minItems":2,"maxItems":2},"maxItems":10,"description":"Inclusive zero-based [start, end] line pairs; negative endpoints count backward. Advertised default: [[0,1000]], but this CLI fetches all output when omitted or empty; send explicit ranges to bound retrieval."}
 },
 "required":["history_id"]
}`
	executeShellCommandSchema = `{
 "type":"object",
 "properties":{
  "command":{"type":"string","description":"Shell command to run on the user's machine."},
  "shell":{"type":"string","enum":["bash","fish","zsh","nu"],"description":"Interpreter for the command."},
  "timeout":{"type":"integer","minimum":1,"maximum":600,"description":"Execution time limit in seconds; defaults to 30."},
  "description":{"type":"string","description":"Purpose of running this command."}
 },
 "required":["command","shell","description"]
}`
	loadSkillSchema = `{
 "type":"object",
 "properties":{
  "name":{"type":"string","description":"Skill identifier from the available-skills list."}
 },
 "required":["name"]
}`
)
