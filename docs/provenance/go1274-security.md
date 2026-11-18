# Current Go 1.27.4 security pin (2026-11-18)

The exact minimum, Make, CI, packaging and consumer toolchain pin is
**Go 1.27.4**. It replaces the [Go 1.27.3 pin](go1273-security.md). Earlier
releases and receipts remain Go 1.27.2 or 1.27.3 records and are not rebuilt or
relabeled. No library module or development-tool pin changed.

## Advisory

`make vuln` (govulncheck v1.9.0) reported
[GO-2026-6859](https://vuln.go.dev/ID/GO-2026-6859.json) (CVE-2026-97054,
published 2026-11-11), **"HTTP/1 server crash due to MaxBytesReader header race
in net/http"**. It affects `net/http` before 1.26.11 and in 1.27.0-0 through
versions before 1.27.4, and is reachable from this server's HTTP handlers.
References: [CL 862185](https://go.dev/cl/862185),
[issue 82563](https://go.dev/issue/82563) and the
[announcement](https://groups.google.com/g/golang-announce/c/iY8TY0wWgbM).
With Go 1.27.4, govulncheck reports no vulnerabilities.

## Official release

[Official download metadata](https://go.dev/dl/?mode=json) fetched on
2026-11-18 lists `go1.27.4` as `stable: true` with these records:

| Official archive | Bytes | SHA256 |
| --- | ---: | --- |
| `go1.27.4.linux-amd64.tar.gz` | 70680823 | `7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840` |
| `go1.27.4.darwin-arm64.tar.gz` | 68231011 | `91a44a2b8a716600ad94a4f143e156116620db8828086a6e3602291be9bd0bb4` |
| `go1.27.4.src.tar.gz` | 35153471 | `73cfb8571aec0c01177687db87163d46aa20e9779672772891aa17654d9a6099` |

The sumdb-verified `GOTOOLCHAIN=go1.27.4` download reported
`go version go1.27.4 linux/amd64`; its `bin/go` SHA256 was
`de0ae3e082ce7599eae4fb69fa2ef1e43b8f31ffd3fce62cdf81930aa9005050`.

## Legal and Unicode applicability

Compared byte for byte with Go 1.27.3, these are unchanged:

- all four selected Unicode 17 tables (`src/unicode/tables.go` and the
  `idna`, `bidi` and `norm` `tables17.0.0.go` files);
- Go `LICENSE` and `PATENTS`;
- `src/vendor/modules.txt`, so every stdlib-vendor legal grant.

Only the toolchain label changed in `LICENSES/Unicode-data-NOTICE.txt`
(new SHA256 `bac1a90cfa29496da47003f83e19b658484eb1f401123ce364b12cdf3ed81e3d`)
and `NOTICE`. The packager and artifact contract now key the table hashes under
`go1.27.4/`; the hashes themselves are unchanged.

## Verification

- `make check` passes, including govulncheck with no findings.
- A local two-path development package built with Go 1.27.4 passed
  `tests/packaging_test.py --artifacts`: 32 contract mutations, legal and
  checksum inventories, a raw-asset install, and the relocated Linux binary's
  version, authentication, lock and signal smoke test.
- Native macOS execution and the resource, compatibility and release gates were
  not rerun locally; CI runs them.
