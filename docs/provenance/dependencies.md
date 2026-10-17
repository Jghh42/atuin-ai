# Dependency notices

The phase 1 configuration dependency is
[`github.com/BurntSushi/toml` v1.5.0](https://github.com/BurntSushi/toml/tree/v1.5.0).
It is used without source modification, linked through Go modules rather than
vendored. `go.sum` records module integrity hashes.

The upstream `COPYING` file is copied verbatim to
[`LICENSES/BurntSushi-TOML-MIT.txt`](../../LICENSES/BurntSushi-TOML-MIT.txt):
MIT License; Copyright (c) 2013 TOML authors.
SHA-256: `d21cb1c60785d6d3a84a7059323ccafc45c645b1bbda281c76a62d66ad2d7dc3`.
Include `NOTICE` and relevant license texts with redistributed binaries.

The isolated SIWC proof links
[`github.com/golang-jwt/jwt/v5` v5.3.1](https://github.com/golang-jwt/jwt/tree/v5.3.1)
without source modification. Its MIT text (only a trailing blank line removed)
is retained at
[`LICENSES/Golang-JWT-MIT.txt`](../../LICENSES/Golang-JWT-MIT.txt):
Copyright (c) 2012 Dave Grijalva; Copyright (c) 2021 golang-jwt maintainers.
SHA-256: `2ce2d18a4c90147990d4ea9b690bcb7a5e13239e8a31ef68bedd0f99f79e0cf6`.
This dependency is not linked by the main server or native Codex probe.
The Codex probe's Pi-derived protocol attribution is recorded separately in
[Pi provenance](pi-codex.md).

The Phase 2 decoder also links
[`github.com/rivo/uniseg` v0.4.7](https://github.com/rivo/uniseg/tree/v0.4.7)
without source modification for grapheme-cluster lengths and truncation,
matching the pinned Gleam string semantics (including combining marks/emoji).
Its verbatim MIT license is retained at
[`LICENSES/Uniseg-MIT.txt`](../../LICENSES/Uniseg-MIT.txt),
Copyright (c) 2019 Oliver Kuederle.
SHA-256: `a59885f5f0f3b3c07cf9444db5fe399b6f0791eac82055f7b85cc65500551039`.
Module integrity is pinned in `go.sum`; include the license with binaries.
**The MIT text is not sufficient for its generated Unicode data.** All six
Unicode15 table sources require the separately retained full historical Unicode
data/software grant and table attribution. The actual Go1.27.2 shipping graph
also selects Unicode17 stdlib/IDNA/bidi/normalization tables and requires its
Unicode License V3 grant. Both complete grants and specific table attribution
are bundled, independently pinned and checked; [immutable authoritative sources,
applicability and hashes](unicode-data.md). Earlier 9a artifacts without them
are superseded, despite the original tests passing.

Phase 7 links [`golang.org/x/sys` v0.49.0](https://pkg.go.dev/golang.org/x/sys@v0.49.0)
without modification for Linux/macOS descriptor-relative nofollow opens, metadata
checks, atomic rename/unlink and lifetime Unix flock. It is a direct module
pinned by `go.mod`/`go.sum`, not vendored. Unmodified upstream BSD license and
patent grant are retained at `LICENSES/Go-x-sys-BSD.txt` (Copyright 2009 The Go
Authors) and `LICENSES/Go-x-sys-PATENTS.txt`. SHA-256 respectively:
`911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad` and
`96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc`.
Include both with redistributed binaries. No encryption/keyring/Windows or
multireplica feature is added by this dependency.

Development commands are pinned exactly in `Makefile`, never linked as
application/runtime dependencies:

- Staticcheck **v0.8.2**, upstream release **2026.2.2**, the latest stable release
  verified on 2026-10-16. Its [maintained release notes](https://github.com/dominikh/go-tools/releases/tag/2026.2.2)
  explicitly support Go 1.27.2 export data. Module checksum:
  `h1:+jPbCzuYxYrcoOvCecYUFDj+XJQRmzIhJmlbpps21Zk=`; upstream tag commit
  `54455593555daf6bfcf3c480faaada7de3145357`.
- `golang.org/x/vuln/cmd/govulncheck` **v1.9.0**, current stable module verified
  from official Go module metadata/tagged [Go source](https://go.googlesource.com/vuln/+/refs/tags/v1.9.0/).
  Module checksum `h1:12VaqnI9kJfqwRoPptxoqsUhQtdfrSbdksT7X/KY/sc=`; upstream
  tag commit `e672cbe8fbc8c76e28fcb6db0efd9ce354e0dac5`. Its x/tools v0.52.0
  dependency supports Go 1.27 syntax/export v5, unlike the old scanner's v0.29.0.
- The `golang.org/x/tools/go/analysis/passes/nilness/cmd/nilness` command is pinned
  at **v0.52.0** (official [tag](https://go.googlesource.com/tools/+/refs/tags/v0.52.0/),
  checksum `h1:ryxg4U63oaUYk7uOLTF37n43f4qgtGvqV4Ioit1BaEs=`). Staticcheck 2026.2
  [retired SA5011 upstream](https://staticcheck.dev/changes/2026.2/#changed-checks)
  and recommends nilness as a similar alternative. We run that maintained
  analysis explicitly rather than quietly omitting nil-dereference checking;
  this is not a claim that the analyzers have identical diagnostics.

The old Staticcheck v0.7.0 export-data error and govulncheck v1.1.4 SSA panic
were preserved before choosing compatible released tools. No runtime SDK module
version, app dependency graph or go.sum is widened by these `go run ...@version`
commands. Toolchain selection/first failure evidence and validation are in the
[sourced diagnosis](../plans/fuzz-toolchain-diagnosis.md). Go itself and its standard library carry
their upstream BSD-style license in the installed toolchain's `LICENSE`.
Phase 9a packaging includes the exact toolchain LICENSE/PATENTS, stdlib-vendor
license/patent texts and notice-bearing comment blocks from selected stdlib
Go/assembly/header files (including Sun fdlibm and Moshier/Cephes notices).
Native `init` additionally links Go's `debug/buildinfo` (including object readers,
compression helpers and runtime/debug), `os/exec` and `os/user`; these are stdlib,
not new modules/SDKs. Its reviewed source-notice union is **335056 bytes**, SHA256
`99df598e700d9967c194f5993b5b1095c0f007a0a15d2c2dc07e89fffce9b265`
(formerly321753 / `4ebe0243d4d9032fdc65b2447ecd71160aa6cad402ad7c6e780a7e6090124400`).
The55 added source sections retain Go Authors BSD headers plus the full FreeBSD
ELF Polstra/O'Brien BSD-2-clause notice/disclaimer; no old sections changed or
were removed. The producer already includes those bytes automatically from BOTH
CGO0 target graphs; the independent consumer's exact reviewed hash/size pin is
updated, not relaxed. Go LICENSE/PATENTS, four stdlib-vendor pairs, Unicode table/
license hashes and the24-entry legal asset set are unchanged. This is a legal
inventory correction for the new linked stdlib graph, not a license change.
The shipping graph is checked for both targets against exactly TOML v1.5.0,
uniseg v0.4.7 and x/sys v0.49.0. Their retained license texts must equal the
verified module cache files (`COPYING`, `LICENSE.txt`, and `LICENSE`/`PATENTS`
respectively). JWT, development analyzers and external compatibility tooling
are NOT shipping linkage. Root NOTICE's historical proof/tool references do not
assert otherwise; JWT's proof-only license is not bundled.
[Packaging/provenance contract](../packaging.md) and each generated manifest
record complete legal/archive inventories and hashes; matching local hashes
are not independent producer attestation.
Project-owned material is licensed under Apache-2.0 as recorded in ADR 0003;
this does not change dependency license terms.
