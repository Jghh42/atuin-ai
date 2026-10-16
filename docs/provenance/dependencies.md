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

Staticcheck and govulncheck are pinned development commands in `Makefile`,
not linked runtime dependencies. Go itself and its standard library carry
their upstream BSD-style license in the installed toolchain's `LICENSE`.
Project-owned material is licensed under Apache-2.0 as recorded in ADR 0003;
this does not change dependency license terms.
