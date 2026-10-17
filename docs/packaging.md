# Packaging and installer contract

**v0.1.0 is planned, not published at this preparation checkpoint.** The user
authorized an initial experimental release, subject to the [release handoff and
remaining gates](plans/release-v0.1.0.md). Historical development receipts remain
unchanged. The separately authorized offline resource-harness correction adds
explicit release selection after independent verification; review and full
acceptance on the eventual release bytes remain mandatory.
Minimum assets are **linux/amd64 (GOAMD64=v1)** and **darwin/arm64**, both pure Go
(`CGO_ENABLED=0`). Native macOS setup/Atuin use was user-confirmed on e27;
**real Linux systemd-user execution is untested**. Go's initial ARM64 ad-hoc
signature is not Developer ID signing/notarization. No Gatekeeper bypass, sudo,
or shell/profile edits are provided by packaging/installation.
The separate attended [`init`](init.md) command now offers explicitly optional
user-service autostart and Atuin configuration after local authentication.
Installer/bootstrap behavior remains noninteractive and never invokes init.
Native success does not establish byte identity with fresh version-stamped
release builds or production maturity.

## Build / examine locally

Requires Git, Python 3.9+ stdlib, and the exact official **Go 1.27.2** toolchain
pinned by `go.mod` (normal verified Go toolchain/module downloads are allowed).
Commands must run against trusted source/tooling on a local owned filesystem.

```sh
make packaging-test
# DEVELOPMENT ONLY: snapshots include nonignored untracked source and honest
# dirty/diff identity. RELEASE BLOCKED, even if the input happens to be clean.
make package-dev VERSION=v0.0.0-dev.phase9a OUT="$PWD/dist/my-new-run"
python3 -B -I tests/packaging_test.py --artifacts "$PWD/dist/my-new-run"
```

Output must be fresh (not already exist), nonsymlink, and ignored if inside the
repository. The packager never overwrites earlier evidence. Failed runs keep a
`FAILED` marker and must not be distributed. Build scratch is private 0700 and
removed after success/failure; output is 0700. Archives for each target contain
only the executable, LICENSE, NOTICE and applicable license/notice texts. Raw
binaries and a single verified legal-text bundle support installation **without
any archive extraction or execution of downloaded code**. Manifests/checksums
are separate human/verifier assets, not secrets or credentials.

Each package includes:

- `atuin-ai-server-TAG-{linux-amd64,darwin-arm64}` raw executables;
- corresponding `.tar.gz` archives for manual use;
- `atuin-ai-server-TAG-legal.txt` (project, linked module, protocol attribution,
  Go runtime/stdlib and bundled stdlib-vendor licenses/patents/source notices);
- `install.sh`, pinned internally to TAG; `SHA256SUMS`; `manifest.json`.

`version` and `--version` write reproducible version, revision, complete source
inventory SHA256, dirty flag, target and Go identity to stdout. They do not open
configuration/credentials/locks or network, even while another process holds the
store lock. Ordinary `make build` is explicitly unidentified `development` /
`unknown` source, not a release identity.

## Reproducibility and provenance boundary

The packager captures the entire tracked plus nonignored untracked file set
(including docs/tests/build helpers), rejecting private-looking paths, symlinks,
hardlinks and nonregular files. Ignored binaries/private state and Git internals
are excluded. Deleted tracked files are represented by dirty/diff evidence.
A sorted path/content/size/executable-mode inventory hashes the actual snapshot;
revision/tree, dirty/tracked-diff flags, diff/status hashes and untracked paths
are retained honestly. Assume-unchanged/skip-worktree index flags and sparse
checkout/index configurations are rejected (including development builds).
The complete full HEAD tree is independently compared with actual snapshot
blob bytes and normalized executable modes, not inferred from `git status`.
Missing tracked files and mode changes hidden by core.filemode=false make the
snapshot dirty. HEAD/snapshot equality and the full HEAD inventory hash are
recorded; release mode fails on any mismatch. File modes are normalized to
Git-style 0644/0755; parent
directories are private. Complete snapshot inventories reject extra files,
directories, symlinks, content/mode changes before and after each build. Original
source identity is rechecked afterward. Do not add private files to nonignored
source. Neither this nor Gitignore is a general secret detector.

Two fresh snapshots at **different absolute paths**, with separate fresh Go
build caches, build BOTH targets. Same-target raw binary, deterministic archive,
legal bundle and normalized `go version -m` output must match byte-for-byte.
Tar member order, type, ownership, names, modes, mtime and gzip header metadata
are normalized (epoch zero, no wall clock or gzip filename). No build path or
current timestamp is injected. Explicit stable linker variables, `-trimpath`,
`-buildvcs=false`, `-mod=readonly`, `-buildmode=exe`, CGO=0 and Linux GOAMD64=v1
are recorded, along with Python/zlib packaging versions. The build environment
is allowlisted; user GOFLAGS/GOENV/GOWORK,
experiments, proxies and credential settings are not inherited. Toolchain
version/executable hashes, module verification, actual target dependency graphs,
archive/legal inventories and SHA256s are in the manifest. `go mod verify`
checks downloaded modules against the pinned sums, and linked modules must be
exactly TOML, uniseg and x/sys. JWT is a separate proof dependency, not linked or
included as a shipping module/license. The root NOTICE retains historical proof
attribution without claiming linkage. Separate full Unicode data grants and
specific table attribution are mandatory: uniseg's six Unicode 15.0.0 tables,
and Go 1.27.2's selected Unicode 17.0.0 stdlib/vendor tables. Version-selected
table names/versions/hashes and reviewed grant hashes are completeness gates.
[Exact immutable license sources and applicability](provenance/unicode-data.md).

The consumer verifier does NOT trust the manifest's own file/member sets. It
requires the independently fixed eight-output contract, all seven checksum
rows exactly once (including the manifest), and the six nonrecursive manifest
assets. It checks both minimum targets, file types/modes/bytes/sizes, full source
identity against actual source and HEAD, every normalized archive member and
legal content against reviewed source/Go/Unicode pins, the exact installer legal
bundle and tag-pinned installer source. Actual `go version -m` must equal the
recorded metadata. Thirty-two mandatory rehashed mutation regressions run on
fresh real artifacts with `--artifacts`; missing artifacts fail, never skip.

**This is two-path local reproducibility/consistency, NOT independent producer
attestation.** The same toolchain/module cache/host/operator is used. SHA256SUMS
is same-source integrity, not a signature or authenticated provenance statement.
Trust in source/toolchain/host and normal HTTPS/GitHub remains required. Reusing a
compromised same-UID producer/cache cannot be detected by matching its own output.

## Installer behavior

Installer requirement: **curl 8.4.0 or newer**. The installer checks this before
any download/scratch and fails actionably on older/unparseable versions, without
automatically installing anything. Older Linux/macOS system curl may need a newer
trusted curl on PATH; manually installing the verified binary/archive is still
possible without a Go SDK or curl upgrade. Every curl invocation uses **`-q` as
its FIRST argument**, including version checks and outer bootstrap examples,
so user `.curlrc` cannot enable insecure TLS, auth/netrc or unwanted outputs.

The minimum is deliberate: [curl 8.4.0's pinned option documentation](https://github.com/curl/curl/blob/d755a5f7c009dd63a61b2c745180d8ba937cbfeb/docs/cmdline-opts/max-filesize.d)
confirms older `--max-filesize` does NOT limit unknown-length transfers. From
8.4.0 it aborts in flight. A post-download `wc` alone is NOT a disk/transfer cap.
Requiring the corrected client avoids a fragile POSIX head/FIFO/child-status
pipeline. Limits are 65536 checksum bytes, 67108864 binary bytes and 1048576 legal
bytes, enforced by curl during transfer AND checked afterward.

Tests require real curl >=8.4 and OpenSSL, in addition to Python. They use only
private synthetic HOME/rc/netrc/ephemeral CA and forced loopback HTTPS endpoints:
real trust failure despite `insecure` rc, positive HTTPS redirects, rejected HTTP
redirects, positive chunked bodies and unknown-length overflows at ALL THREE
limits (curl exit63 and observed scratch size <= cap). A no-`-q` negative witness
proves the synthetic rc would actually bypass trust/send auth/write output.
No operator HOME/rc/netrc, public release, account or provider is accessed. The
original 27 stub tests did NOT establish these real-client properties; their
"unsafe redirect" was only a fake failure and oversize checked completed files.

### Pinned installer (after publication)

**Do not run these planned download commands until the complete v0.1.0 release
is public.** Check the [Releases page](https://github.com/Jghh42/atuin-ai/releases)
first. Install trusted **curl 8.4.0+ before the outer bootstrap download**, not
only before the installer's own downloads. No sudo or unverified `curl | sh`.
The following POSIX example downloads a version-pinned installer and checksums
into owned scratch, checks the installer hash, then runs it. Review `install.sh`
before the final `sh` line if you have not already reviewed that release.

```sh
(
  set -eu
  umask 077
  curl -q --version | awk '
    NR == 1 {
      if ($1 != "curl" || $2 !~ /^[0-9]+[.][0-9]+[.][0-9]+$/) exit 1
      split($2, v, ".")
      if (!(v[1] > 8 || (v[1] == 8 && v[2] >= 4))) exit 1
      ok = 1
    }
    END { if (!ok) exit 1 }
  ' || { printf '%s\n' 'curl 8.4.0+ is required before downloading.' >&2; exit 1; }
  work=$(mktemp -d)
  trap 'rm -rf -- "$work"' 0
  cd "$work"
  base=https://github.com/Jghh42/atuin-ai/releases/download/v0.1.0
  for name in install.sh SHA256SUMS; do
    curl -q --proto '=https' --proto-redir '=https' --tlsv1.2 \
      --fail --show-error --silent --location --max-redirs 3 \
      --connect-timeout 15 --max-time 120 --max-filesize 65536 \
      --output "$name" "$base/$name"
  done
  expected=$(awk '
    $2 == "install.sh" { n++; h = $1; if (NF != 2) bad = 1 }
    END {
      if (bad || n != 1 || length(h) != 64 || h ~ /[^0-9a-f]/) exit 1
      print h
    }
  ' SHA256SUMS)
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum install.sh | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 install.sh | awk '{print $1}')
  else
    printf '%s\n' 'A SHA-256 tool is required.' >&2; exit 1
  fi
  [ "$actual" = "$expected" ] || { printf '%s\n' 'Installer checksum mismatch.' >&2; exit 1; }
  # Review the downloaded script before proceeding if needed.
  sh ./install.sh --version v0.1.0 --bin-dir "$HOME/.local/bin"
)
```

Both checksum and installer come from the same HTTPS release: this detects
corruption/mismatched files, not an independently signed publisher identity.
Check `"$HOME/.local/bin/atuin-ai-server" version` afterward. The installer checks
binary/legal hashes itself; keep the installed
`atuin-ai-server.licenses-v0.1.0.txt` beside the binary. Manual raw/archive users
must likewise verify hashes and retain legal notices. No Go SDK is needed.

 A source template without an
embedded release requires an explicit safe version. A generated latest bootstrap
contains ONE tag: every checksum, raw binary and legal download uses
`releases/download/TAG/...`, never resolves latest again. An explicit `--version`
overrides that pin consistently. Unsupported OS/architecture pairs fail before
download. Linux x86_64/amd64 and Darwin arm64/aarch64 aliases are recognized;
Darwin x86_64 and Linux arm64 are not silently misidentified as supported.

Normal TLS and HTTPS-only redirects are mandatory, with bounded redirection,
connection/total time and per-file sizes. Scratch is private and trap-cleaned.
Matching-release binary AND legal checksums are verified before staging. Bounded
binary-header checks reject truncated/wrong-format/wrong-architecture assets;
no downloaded executable is run. No remote manifest pathname is used in a file
operation, and there is no tar parser/extraction attack class. The legal bundle
is installed beside the binary as `atuin-ai-server.licenses-TAG.txt`.

Default destination is `~/.local/bin`; explicit bin directories must be absolute
and unambiguous (spaces supported). Symlink leaves/ancestors and nonregular
existing targets are refused. Use a trusted, owned destination and ancestors;
hostile same-UID/root processes or filesystem replacement are not in the threat
boundary. Existing permissions are never changed. New directories are 0700;
binary and public legal files are 0755/0644. Verified files stage IN the
destination filesystem; no existing binary is truncated or chmodded. The legal
file is renamed first, then the executable commits via one atomic rename. Any
precommit failure/catchable interruption retains the old executable and cleans
owned temporary files. A new version-specific legal file may remain on binary
rename failure; it is not executable state. Catchable signals are ignored during
the single commit rename. SIGKILL/power loss can leave an owned staging file;
atomic replacement means the old OR complete new executable, not a partial file.
This is not an fsync/power-loss durability guarantee. Other users' scratch is
never removed. PATH instructions are printed, not applied. Users may separately choose the
reviewed `atuin-ai-server init` workflow; it is never run by the installer. No auth login,
credentials, configuration, profiles, autostart, sudo, TLS weakening or Gatekeeper
bypass is performed.

### Immutable publication policy

Release mode (`make package VERSION=EXISTING_TAG OUT=NEW_DIR`) requires reviewed
**CLEAN** source and a pre-existing exact-HEAD tag. Review cannot be established
by code; the operator must obtain it. Development builds remain RELEASE BLOCKED
and must never be promoted by editing manifest/linker metadata. No script or CI
workflow creates tags/releases, uploads release assets or has contents-write
permission. Packaging CI uploads development workflow artifacts only and fails
on missing outputs or validation failures; this does not clear existing CI gates.

Before separately approved publication, agree an immutable tag and preserve all
reviewed hashes/manifests. Never move/reuse/delete-and-recreate published tags or
replace release assets/checksums in place. Corrections need a NEW version/tag.
`latest` may select a newer complete release; publish all versioned assets and
matching checksums before moving the channel, and never relabel a development
snapshot as clean. A release/version pin is integrity policy, not cryptographic
publisher authentication. The experimental release has [explicit bounded
acceptance gates](plans/release-v0.1.0.md), including full resource acceptance on
the actual release package. `make resource` remains DEVELOPMENT-only;
`make resource-release` explicitly requires independently verified CLEAN TAGGED
artifacts, with the same isolation, workload and cleanup gates. Broader
production lifecycle/both-model claims remain outside this initial release.

## Native macOS ARM64 handoff — historical development checklist

The user subsequently confirmed native LaunchAgent/init and Atuin use on e27;
[evidence and limits](plans/release-v0.1.0.md#evidence-and-limitations). The retained
checklist below is not a claim that native Mac execution is still wholly pending.
Any fresh live/native attempt needs fresh explicit consent. Use ONLY an independently
reviewed development artifact and its exact archive
SHA256/manifest supplied outside the source snapshot. Cross-compilation and
ad-hoc signing are **not native execution proof**. The user volunteered native
validation; no Go SDK is required (optional `go version -m` only).

1. In an owned scratch directory, `shasum -a 256 ARCHIVE` and compare the exact
   supplied digest; inspect `tar -tzf ARCHIVE`. Manually extract this verified,
   trusted archive, retain its legal texts, relocate/copy the binary and run
   `./atuin-ai-server version` / `--version`. Expect darwin/arm64, Go1.27.2,
   explicit development tag/revision/hash and honest dirty=true if applicable.
   Do not auto-bypass Gatekeeper. If macOS refuses execution, report only the
   safe outcome; no `xattr -d` or security-policy changes are prescribed.
2. Create an **absolute 0700 private root** using `root=$(mktemp -d)` then
   canonicalize it with `root=$(cd "$root" && pwd -P)` (macOS `/var` is often a
   symlink); create a 0700 `$root/home`. For EVERY command set `HOME="$root/home"`. On macOS
   `os.UserConfigDir` **ignores XDG_CONFIG_HOME**: expect
   `$HOME/Library/Application Support/atuin-ai/credentials`. Never use the real
   HOME or retained-probe generation 3. On Linux use a private XDG_CONFIG_HOME.
3. Run version with `CHAT_CONFIG` pointing to a missing file; expect success
   with no state created. Use `env -u CHAT_CONFIG HOME="$root/home" BINARY serve`
   for default startup (no credentials). Expect IPv4 loopback 17391, models
   JSON 503 `models_unavailable`, no login/browser/network activity. If that
   port is occupied, stop only your own test server or use a new private config.
4. Create config directories 0700 and new config files 0600 (`umask 077`).
   Confirm missing default means built-ins; explicit missing config fails.
   Check default config < `CHAT_CONFIG` < `serve --config PATH` precedence with
   three distinct unused ports, always under the private root. Only `port` and
   optional `default_model` are valid. Do not alter existing user files.
5. While serving, second `auth status` must fail on the lifetime lock, whereas
   version must still succeed. Never unlink the lock. Send SIGINT, confirm
   bounded clean exit; restart and repeat using SIGTERM. After shutdown, private
   test `auth status` reports logged out. Inspect ONLY modes/paths (0700 dirs,
   0600 credential records/lock), never private file contents. Repeat relocated
   execution to catch source-path dependence.
6. OPTIONAL, user-attended real login/catalog/text/tool continuation follows
   the [separate production handoff](plans/phase7-attended-login-handoff.md),
   only in the user's own terminal and private test HOME. Do NOT request,
   capture or send device codes, tokens, account IDs, provider bodies or store
   contents. Report safe pass/fail outcomes only; no probe-state import. No live
   calls are in automated packaging tests. Live gates remain NOT PASSED until
   separately evidenced, even if the binary smoke passes.

Remove only your own empty/synthetic scratch after stopping its process; do not
blindly delete a live-login store. Retain safe hashes/version/outcomes, not auth
transcripts or screenshots. Phase 9b profiling/load/memory/leak work is separate.
