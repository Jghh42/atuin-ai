# Native init: systemd v258 serialization contract

This narrow follow-up uses official public **systemd v258** source, not host
supervisor execution or an invented shell-like ExecStart fixture. No systemd
library/code is linked and no new SDK/module/command interface is introduced.
The public source files were fetched without user curl configuration, credentials
or inherited environment, into ignored `dist/init-fix-diags/systemd-v258/`.
These are source-derived synthetic fixtures, **not native Linux execution proof**.

## Empty arrays are not missing scalar properties

- [systemctl-show.c:1417–1434](https://github.com/systemd/systemd/blob/v258/src/systemctl/systemctl-show.c#L1417-L1434):
  the specialized EnvironmentFiles struct-array printer emits one line per entry,
  and **no line at all for an empty array**. `--all` does not change that loop.
- [systemctl-show.c:1515–1574](https://github.com/systemd/systemd/blob/v258/src/systemctl/systemctl-show.c#L1515-L1574):
  the ExecStart struct-array printer similarly emits no line when empty, as with
  an unloaded/not-found service. A loaded managed service MUST have its expected
  nonempty ExecStart; omission is not accepted for loaded state.
- Ordinary scalar/STRV printing differs; requested scalar fields remain required.
  [bus-print-properties.c:250–280](https://github.com/systemd/systemd/blob/v258/src/shared/bus-print-properties.c#L250-L280)
  emits requested empty string-array fields with SHOW_EMPTY, but the specialized
  loops above bypass that behavior.

Only those TWO documented omissions are normalized: EnvironmentFiles to empty,
and ExecStart to empty ONLY for LoadState=not-found. The entire command must have
completed successfully with no cancellation, overflow, incomplete transport or
truncated final line. Missing required scalars/other fields, unknown properties,
loaded ExecStart omission and nonempty environment-file overrides still refuse.
No transport error is interpreted as an empty array or an absent service.

## Executable, loaded argv and runtime argv are different representations

- [load-fragment.c:920–1011](https://github.com/systemd/systemd/blob/v258/src/core/load-fragment.c#L920-L1011)
  recognizes `@` and resolves the executable with unit_path_printf separately
  from argv. With `@`, argv[0] must be supplied explicitly.
- [load-fragment.c:1023–1045](https://github.com/systemd/systemd/blob/v258/src/core/load-fragment.c#L1023-L1045)
  C-unquotes each argument and expands unit specifiers, but retains dollar escapes
  in LOADED argv.
- [exec-invoke.c:6319–6327](https://github.com/systemd/systemd/blob/v258/src/core/exec-invoke.c#L6319-L6327)
  calls replace_env_argv only at execution. Its
  [env-util.c:726–755](https://github.com/systemd/systemd/blob/v258/src/basic/env-util.c#L726-L755)
  dollar-pair handling and [argv wrapper](https://github.com/systemd/systemd/blob/v258/src/basic/env-util.c#L892-L972)
  collapse each `$$` to a literal `$`, including in the explicit argv[0].

The generated Linux unit is therefore marker **v3** and renders:

```text
ExecStart=@"/home/a$literal/.local/bin/atuin-ai-server" "/home/a$$literal/.local/bin/atuin-ai-server" serve --config "/home/a$$literal/config$${CHOICE}.toml"
```

The first token is the literal executable path with unit/C quoting but NO dollar
expansion escaping. The explicit argv[0] and config argument escape dollars.
Expected loaded argv retains doubled dollars; expected runtime/proc argv contains
single literal dollars. Existing exact-byte recognition refuses older/modified
units for manual migration. macOS's marker/format is unchanged.

The same ExecStart printer above uses `strv_join(info.argv, " ")` and prints path
and joined argv **raw**, without C/shell quoting. Inspection compares that native
flattened loaded representation to the known, exact owned-unit template; it does
not unquote a foreign executable path into an apparent match or pretend to recover
arbitrary argv boundaries. Exact unit bytes, no drop-ins/reload-needed state and
all effective-context guards remain required. Actual running argv boundaries,
UID and executable inode are independently checked through proc metadata.
Ambiguous field delimiters/unsupported representations refuse. Spaces/percent/
dollars are supported in executable paths; quotes/backslashes are supported in
config arguments, with real raw-show positive fixtures. Executable paths themselves
must additionally pass systemd's [string_is_safe check](https://github.com/systemd/systemd/blob/v258/src/basic/string-util.c#L1078-L1091),
called by load-fragment.c:985: ASCII quotes and backslashes are NOT legal there,
even when C-unquoted. Init refuses those executable paths BEFORE any filesystem
or supervisor operation rather than inventing a successful loader fixture. macOS
XML paths are unaffected.

## Environment is a different printer/escaping context

Environment is a string array, printed through shell_maybe_quote(...,0), not the
raw ExecStart struct path/argv printer:

- [bus-print-properties.c:250–280](https://github.com/systemd/systemd/blob/v258/src/shared/bus-print-properties.c#L250-L280)
- [escape.c:552–599](https://github.com/systemd/systemd/blob/v258/src/basic/escape.c#L552-L599),
  [escape.h:6–22](https://github.com/systemd/systemd/blob/v258/src/basic/escape.h#L6-L22)
- [test-escape.c:158–180](https://github.com/systemd/systemd/blob/v258/src/test/test-escape.c#L158-L180)

Environment assignment values do not perform dollar expansion. Their unit
encoding keeps `$` literal; their SHOW representation may quote/escape it as
`\$`. The bounded string-array decoder accepts literal escaped dollar/backtick,
without any expansion. The synthetic quoter is tested against official flags0
snapshot vectors. This is not a general shell or arbitrary command decoder.

## Exact reviewed public file hashes

All URLs above are official v258 paths; hashes pin the exact reviewed bytes even
if a remote tag later moves. No downloaded source is executed.

| Source (`src/…`) | SHA256 |
| --- | --- |
| systemctl/systemctl-show.c | `ab098e4719f297add086ffd2ccc5fa4f8f6acf5c6f8a8bb767dce58b3953bc57` |
| core/load-fragment.c | `edf4e42df6936a866d07617cacb8b046cd552391c8cae8d3935c36b33fd5e3f4` |
| core/exec-invoke.c | `d1e6085b06af938fc9981bf854286a0c1ea4f9f184ba4b3cbf629927da61f850` |
| basic/env-util.c | `851963467e9a350cf7695afdaa7cd10442b4b338a10ad7923a469c1375965038` |
| shared/bus-print-properties.c | `03df977d247169791b8439e4da4447bfd8cc10e7b59c16ecc33503ce55e79e1c` |
| basic/escape.c | `a52bed201c08de0859aea2669ccce830820c7b2e564efb73d99a15f6762120fc` |
| basic/escape.h | `58cfcd44131121db2b8386bf33d223645f2b5cd782107509c0069317a113de45` |
| test/test-escape.c | `c27430aac95dbd16f803264411fced7cd87504383e77896ff295c503eb19ffe9` |
| basic/string-util.c | `36b2f8d2b98e6daf9300b4204a2dd843f025e5d5656c4ba3eadc6baa95335a35` |
