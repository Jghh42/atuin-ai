# Go 1.27.3 security pin (2026-10-27)

> Superseded by the [Go 1.27.4 pin](go1274-security.md).

The current exact minimum/Make/CI/package/consumer pin is **Go 1.27.3**.
Historical v0.1.0 and phase receipts remain Go 1.27.2 records; their tags/assets
are not rebuilt or relabeled. This is a debug-candidate toolchain update, not a
new public patch release. No library module or development-tool pin changed.

## Verified advisory and official release

The official [reviewed GO-2026-6809 advisory](https://vuln.go.dev/ID/GO-2026-6809.json)
(CVE-2026-111943, published 2026-10-21) is **“Distrust compromised intermediate
certificates in crypto/x509”**. `Certificate.Verify` is affected before 1.26.10
and in 1.27.0-0 through versions before 1.27.3. The advisory links
[CL 851047](https://go.dev/cl/851047), [issue 82345](https://go.dev/issue/82345)
and the [official announcement](https://groups.google.com/g/golang-announce/c/66pojueaQek).
Go normally uses system trust stores; the fix adds a denylist for known
compromised intermediate certificates in narrower cases where system trust may
not have been updated. We do not override trust roots, disable verification or
change TLS defaults. There is **no evidence this caused a colleague's generation
failures**; diagnostic categories are not a retrospective root-cause claim.

A bounded normal-TLS fetch of [official current download metadata](https://go.dev/dl/?mode=json)
on 2026-10-27 confirmed `go1.27.3`, `stable:true`, and these exact records:

| Official archive | Bytes | SHA256 |
| --- | ---: | --- |
| `go1.27.3.linux-amd64.tar.gz` | 70588678 | `65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40` |
| `go1.27.3.darwin-arm64.tar.gz` | 68196737 | `dd3908cf20959d798c1e06ebe5b72213f0505ce01373b40ce3110975a6cd99c2` |
| `go1.27.3.src.tar.gz` | 35134035 | `6e7749035cf360f06b2575d162ea79df9fda8ef671889e5c375f8e50777f26bb` |

The Linux official archive was actually downloaded with `curl -q`, HTTPS-only
redirects, default TLS verification, finite time/size limits; its size and full
archive SHA256 matched metadata before private local extraction. Its unmodified
`bin/go` reported `go version go1.27.3 linux/amd64`, SHA256
`5f5eb6d0b2fb6aa0bb5d5f8c5f1c944e0de955c925f0111af92a9d8f79680b5b`.
The normal sumdb-verified `GOTOOLCHAIN=go1.27.3` downloaded toolchain's `bin/go`
was byte-for-byte identical. No root install, global PATH replacement, custom
SDK source patch, version fallback or vulnerability suppression was used.
The Darwin/source archive records were verified as metadata, **not downloaded
or executed**. Native Darwin execution remains a separate CI/manual gate.

## Independent actual-source/legal comparison

An independent script (not importing the packager) queried both shipping target
selected dependency graphs using the actual Go 1.27.2 and 1.27.3 toolchains,
measured selected source/notice files, and compared legal/table bytes. Ignored
review evidence lives in `dist/generation-failure-diagnostics-v2/`:
`OFFICIAL-DOWNLOAD-PROVENANCE.json`, `TOOLCHAIN-LEGAL-COMPARISON.json`,
`toolchain-legal-measurement.log`, both extracted `go1.27.*-stdlib-source-notices.txt`
and the empty `stdlib-source-notices.diff`. The measurement ran before raising
go.mod's minimum so the old compiler could inspect that same source graph.

Measured unchanged bytes/pins:

- Go `LICENSE`: `911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad`.
- Go `PATENTS`: `96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc`.
- All eight vendored x/crypto, x/net, x/sys and x/text LICENSE/PATENTS files:
  unchanged corresponding BSD/PATENTS bytes above; no new/removed vendor grant.
- Combined notice-bearing comments from selected Go/assembly/header source
  files for BOTH shipping targets:
  `99df598e700d9967c194f5993b5b1095c0f007a0a15d2c2dc07e89fffce9b265`.
  No notice source was added/removed. The consumer's reviewed notice pin is
  therefore **unchanged**, not guessed or accepted merely from producer metadata.
- All four selected Unicode 17 tables: byte-identical to the four hashes in
  [Unicode provenance](unicode-data.md). Current table provenance keys now name
  exactly `go1.27.3/`; hashes are unchanged. Both complete Unicode grants and
  uniseg v0.4.7's Unicode 15 tables remain unchanged.
- `LICENSES/Unicode-data-NOTICE.txt`: only the verified toolchain version label
  changed from 1.27.2 to 1.27.3. Current hash
  `a17c6464748b6769c7a9af58ad2cbdbba53f241e869703a4157004b653e78c9e`
  is pinned independently in producer and consumer. Copyright years/grant terms
  are not changed. Root NOTICE's current table attribution names 1.27.3.

Among selected standard-library source files, only `crypto/x509/verify.go` and
`crypto/x509/root_darwin.go` changed. Their old/new complete source hashes are
in the comparison report. The actual new verify.go contains
`distrustedSPKIHashes` and its verification guard; no application root override
is necessary or introduced.

## Fuzz fix and exact-pin propagation

The actual `src/internal/fuzz/fuzz.go` is byte-identical in both SDKs: SHA256
`db5a9c6f3b26220eb9fb1c144caf012a555c6adef785cb9d185426e508d9d2aa`.
Lines 129–137 retain the parent-AND-child context error guard and issue 75804
comment documented in the [historical fuzz diagnosis](../history/fuzz-toolchain-diagnosis.md).
This is direct source/hash proof, not a passing rerun used to infer the fix.

`go.mod` remains the single exact Make/CI/fuzz/resource-helper toolchain source.
All five workflow files already derive the version from go.mod and disable
check-latest. Go/packaging/fuzz/resource workflows explicitly verify actual
GOVERSION and pin subsequent steps; compatibility invokes Make, whose exact
GOTOOLCHAIN/GOVERSION precondition supplies that check. They require no literal
version edit. The packager and independent consumer now reject any
version other than 1.27.3; shipping smoke expectations match it. Existing fuzz
budgets, seeds, parallelism and failure assertions remain untouched. No module,
govulncheck, Staticcheck or nilness tool upgrade, scanner skip, broad version
allowlist or check-result substitution is part of this change.
