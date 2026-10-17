# v0.1.0 experimental release — frozen preparation handoff

## Status and authority

**Planned/unpublished at this documentation checkpoint.** The user authorized
an initial experimental release and volunteered a Linux laptop installer test
after publication. That supersedes earlier blanket publication prohibitions,
not historical evidence or safety/artifact gates. This is a personal single-user,
loopback-only early 0.x release, **not production completeness**.

This writer prepares documentation plus the separately authorized narrow offline
resource-harness correction. No commit, push, main/public tag, draft, upload,
publication, serving/auth/config/TTY runtime change, credential/provider access or
native service operation is performed. Disposable private Git fixtures with
synthetic identities/test tags are permitted solely for offline validation. Root review and independent validation must precede further action;
root's subsequent explicit approval is required before creating the draft. Do not
silently broaden scope if any gate fails. No force/amend/reset, tag replacement,
PR/branch-protection change or historical artifact/log modification is permitted.

This plan freezes the intended acceptance sequence, not an unreviewed source
identity. The approved final source SHA, source inventory, tag object, CI run IDs
and asset/report hashes must be recorded externally after the final commit; a
source document cannot attest its own eventual commit hash. Any implementation
or documentation change invalidates that candidate's freeze and needs a new
review/verification/CI checkpoint. Preserve prior receipts unchanged.

## Evidence and limitations

Baseline: `e27e1897b4469bf3cc190e630199283af82c1825` (e27). Read-only GitHub
checks confirmed all four runs have that exact head SHA and conclusion `success`:

| Family | Baseline receipt |
| --- | --- |
| Go checks, including native synthetic macOS terminal regressions | [39057015760](https://github.com/Jghh42/atuin-ai/actions/runs/39057015760) |
| Pinned CLI20/oracle14 compatibility and aggregate | [39057015793](https://github.com/Jghh42/atuin-ai/actions/runs/39057015793) |
| Two-target/two-path DEVELOPMENT packaging/consumer gates | [39057015776](https://github.com/Jghh42/atuin-ai/actions/runs/39057015776) |
| Full offline resource execution/cleanup aggregate | [39057015808](https://github.com/Jghh42/atuin-ai/actions/runs/39057015808) |

The user subsequently reported native macOS LaunchAgent service active, init
complete, background activity enabled, and Atuin's `test` reaching ready using
`gpt-6.1-sol`. This is real user-reported Mac setup/Atuin evidence, not another
synthetic fixture. It does not certify every checklist case, both-model inference,
production refresh/restart/logout lifecycle or byte identity with a new build.
**Real Linux systemd-user execution remains untested**, explicitly disclosed for
this experimental release. The offered laptop attempt is post-publication work,
not a pre-recorded pass or permission for an agent to operate host services.

Historical [native-init independent acceptance](init-status.md) passed exact
258-file source inventory
`e74200ad62df2dbc5e6b16ed4834130cd6b6b1d0d1d8bae25ed37584998927d4`:
CLI20/oracle14, DEVELOPMENT packaging36+32, shipping/wizard smoke and full resources
446 generations/0 catalogs. It remains immutable prior-input evidence, not proof
of this docs commit or fresh v0.1.0 artifacts. Earlier failure logs and superseded
packages retain their original identities. Neither CI cross-builds nor injected
supervisor tests are native LaunchAgent/systemd execution proof.

## Release-mode resource acceptance correction (pending review)

The original runner rejected genuine release-mode artifacts after independent
verification because it required DEVELOPMENT mode exactly. Root separately
authorized the narrow offline harness correction: `--release` (or
`make resource-release`) explicitly requires
`CLEAN TAGGED; publication requires separate approval`; the default `make resource`
still requires `DEVELOPMENT; RELEASE BLOCKED` exactly. Wrong/unknown modes refuse.
The complete independent `packaging_contract.verify()` consumer always runs
**before** mode selection, including actual source inventory/HEAD, clean/tag
identity, eight assets, legal/build metadata and checksums. No producer is called
by the resource driver and no tag/publication action is added.

Requested/verified mode is recorded in outer provenance and confined report/input
evidence. `--release` is outer-only, not an inside bypass. The closed eight-public-
file snapshot, sealed binary, namespace/privacy/ownership checks, workload,
thresholds, timeout budgets and cleanup remain unchanged. Fast mode tests and an
opt-in real-consumer suite (`tests/resources/release_mode_test.py`) cover a
disposable clean-tagged fixture, wrong opt-in, missing private test tag, dirty
source and rehashed manifest tampering. Existing CI still packages DEVELOPMENT
artifacts and uses the unchanged default selection; no workflow permissions change.

Root independent review/validation must clear the correction before final freeze.
Do not patch/relabel artifacts, substitute DEVELOPMENT results for release-byte
evidence, skip the full suite, or change thresholds/counts/timeouts. The eventual
actual clean v0.1.0 package must still pass full release-mode resources.

## Frozen build and asset contract

- Final source must be reviewed, committed, clean, equal to approved remote main,
  and have all four exact-HEAD CI families successful. Baseline e27 results are
  not CI for the new documentation or any subsequently authorized harness change.
- Existing `scripts/package.py` release mode requires a **preexisting exact-HEAD
  tag**. Use an annotated immutable `v0.1.0` tag only after root authorizes that
  freeze. Never move, delete/recreate or replace it. An unexpected existing local
  or remote tag/release is a stop condition, not permission to overwrite.
- Build truly fresh v0.1.0 packages with exact Go **1.27.2**, `CGO_ENABLED=0`,
  Linux amd64 **GOAMD64=v1** and macOS arm64. The existing packager internally
  builds both targets from two distinct absolute source/build paths with separate
  fresh build caches. Use fresh owned nonsymlink output, ignored if in the checkout;
  never reuse or rename earlier development artifacts.
- Retain manifests, full source/build/legal inventories, normalized metadata and
  SHA256SUMS. Raw binaries, archives and legal bundle must satisfy the existing
  independent eight-file consumer contract. This is same-host two-path
  reproducibility, **not independent producer attestation**.
- macOS has Go's initial ARM64 ad-hoc signature, not Developer ID/notarization.
  No Gatekeeper bypass, paid signing promise or unsupported architecture assets.

The complete eight uploaded assets are:

```text
atuin-ai-server-v0.1.0-linux-amd64
atuin-ai-server-v0.1.0-linux-amd64.tar.gz
atuin-ai-server-v0.1.0-darwin-arm64
atuin-ai-server-v0.1.0-darwin-arm64.tar.gz
atuin-ai-server-v0.1.0-legal.txt
install.sh
manifest.json
SHA256SUMS
```

SHA256SUMS covers the other seven files exactly once; the manifest describes six
nonrecursive assets. GitHub's automatic source archives are separate and are not
installer inputs. The generated installer must be byte-identical to the reviewed
source template with the one `v0.1.0` pin, never a development/UNPUBLISHED pin.

## Root-owned execution sequence (not executed by this writer)

1. **Review the authorized correction and docs.** Independently review the
   explicit opt-in after independent consumer verification, unchanged default
   selection/confinement and negative/real-fixture coverage. No serving/auth/
   config/catalog/TTY changes or new dependencies are authorized. Run independent
   source/check acceptance before delivering the final combined candidate.
2. **Deliver the reviewed source normally.** Commit/push only under root's
   explicit authority, after checks. Record exact local/upstream/remote equality
   and clean status. Require all four new exact-HEAD workflow families, including
   native macOS terminal tests, compatibility aggregate and full resource cleanup
   aggregate. Preserve exact first-failure logs before proposing changes; no blind
   rerun, force push, gate suppression or publication if any job fails.
3. **Freeze/tag locally after approval.** Recheck tag/release absence (root's
   earlier empty listing is historical), source identity and all-four success.
   Create the approved annotated exact-HEAD tag needed by the release packager.
   Do not push a public tag or create a release yet. Freeze source/toolchain and
   retain the annotated tag object's identity outside source.
4. **Fresh release package and independent acceptance.** These are later root
   commands, using approved fresh absolute output directories, not commands
   executed for the final/public release during this preparation task:

   ```sh
   make check
   make build
   make package VERSION=v0.1.0 OUT="$RELEASE_OUT"
   python3 -B -I tests/packaging_test.py --artifacts "$RELEASE_OUT"
   make resource-release ARTIFACTS="$RELEASE_OUT" OUT="$RESOURCE_OUT"
   python3 -B -I tests/packaging_test.py --artifacts "$RELEASE_OUT"
   ```

   Require **36 default consumer/installer tests +32 artifact mutations** before
   and after resources, independent contract/source/legal verification, relocated
   Linux shipping/installer smoke, and actual packaged init detached no-TTY
   zero-state/private-PTY missing-login No smoke. Use isolated synthetic HOME/bin
   and fake providers only; no actual login/provider or host service operation.
   Require complete full resources **446 generations, 0 catalogs**, not `--case`
   partial output: unchanged RSS/retention/signals/real 60s/300s timeout gates,
   bounded ownership/isolation, all owned wrappers/server PIDs gone, samplers and
   fixture handlers joined, and scratch/namespace cleanup provenance. Recheck
   exact release asset hashes, source cleanliness/tag identity and independent
   consumer after the suite. No byte-identity claim against prior Mac artifacts.
5. **External frozen receipt and root approval.** Record final SHA/inventory/tag
   object, four exact-head success URLs, all eight asset sizes/hashes, manifest/
   checksum hashes, independent 36+32/pre-post/smoke results, full resource report
   and cleanup provenance. Independent validator/root must clear these actual
   release artifacts before public tag/draft/upload. Do not edit source to add
   own-hash receipts after freeze. If any gate fails, stop and retain evidence;
   never move the frozen tag to accommodate a fix.
6. **Draft only under subsequent explicit root approval.** Push only the approved
   annotated tag, verify its peeled remote commit, then create a draft with
   `--verify-tag`, exact target and reviewed notes. Upload **all eight assets**
   without `--clobber`; compare complete draft inventory/bytes/checksums before
   publication. Never expose a partial release as Latest. The intended latest
   channel requires a complete normal GitHub release (not a GitHub prerelease),
   still clearly titled/described as **experimental v0.1.0**. If root chooses a
   GitHub prerelease instead, stop and revise channel expectations: it cannot
   serve the normal latest endpoint. Do not change repository protection or
   assume GitHub server-side immutability is enabled; policy forbids replacement.
7. **Publish only the cleared complete draft.** Root makes the final publication/
   Latest decision after all preceding gates. Corrections afterward require a
   new version/tag/assets, never in-place replacement. No announcement that an
   endpoint works until read-back verification below succeeds.
8. **Post-publication read-back.** Download the public versioned eight assets via
   bounded HTTPS/HTTPS redirects with `curl -q` first and curl >=8.4. Compare them
   with the frozen hashes and independent contract, not just remote self-described
   checksums. Verify the public Latest bootstrap's redirect chain and exact script
   bytes/pin, and that all its binary/legal/checksum URLs stay on `v0.1.0` without
   another latest lookup. Run the reviewed pinned checksum-first outer bootstrap
   and installer in isolated synthetic HOME/owned absolute bin paths, preserving
   adjacent legal notices; check `version` and non-authenticating/decline smoke
   only. Record canonical public asset URLs and observed outcomes. No real
   credentials, user config, providers, native services or private E2E harness.

## User-facing handoff and proposed release notes

**Draft wording only — not a publication receipt:**

> Experimental v0.1.0: a small personal loopback-only Atuin AI server, native Go
> Codex device login and Responses streaming, with optional attended user-service
> and Atuin setup. Linux amd64 (GOAMD64=v1) and Apple Silicon macOS arm64, CGO=0.
> Native macOS LaunchAgent/init and Atuin use were user-confirmed on the e27
> delivery; release-stamped builds have separate verification. Real Linux
> systemd-user execution is not yet tested. No production-maturity, both-model
> lifecycle, independent build attestation or Apple notarization claim.

Link the actual [Releases page](https://github.com/Jghh42/atuin-ai/releases),
[pinned checksum-first installer](../packaging.md#pinned-installer-after-publication)
and [attended init safety/removal](../init.md) after confirming publication.
No Go SDK or sudo is needed. **curl >=8.4 is required before downloading the
bootstrap**, with `-q` first on every invocation. The installer verifies raw
binary/legal hashes and installs `atuin-ai-server` plus
`atuin-ai-server.licenses-v0.1.0.txt`; retain the legal file beside the binary.
It never authenticates, edits Atuin/server config or profiles, or enables autostart.

`init` is a separate terminal-only command; existing native application login is
retained, every consent defaults No, and service/config changes are independent.
Server configuration remains only `port` and optional `default_model`; no auth,
model catalog or serving behavior changes are part of this release preparation.
Any fresh native/Linux or provider attempt needs fresh explicit consent. For the
user's offered Linux laptop test, request safe version/install/init/service
outcomes only; unsupported user manager means foreground/manual setup, not sudo,
linger changes, ownership-check relaxation or raw log/config/credential sharing.
