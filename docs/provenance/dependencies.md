# Phase 1 dependency notices

The only non-standard-library runtime dependency is
[`github.com/BurntSushi/toml` v1.5.0](https://github.com/BurntSushi/toml/tree/v1.5.0).
It is used without source modification, linked through Go modules rather than
vendored. `go.sum` records module integrity hashes.

The upstream `COPYING` file is copied verbatim to
[`LICENSES/BurntSushi-TOML-MIT.txt`](../../LICENSES/BurntSushi-TOML-MIT.txt):
MIT License; Copyright (c) 2013 TOML authors.
SHA-256: `d21cb1c60785d6d3a84a7059323ccafc45c645b1bbda281c76a62d66ad2d7dc3`.
Include `NOTICE` and relevant license texts with redistributed binaries.

Staticcheck and govulncheck are pinned development commands in `Makefile`,
not linked runtime dependencies. Go itself and its standard library carry
their upstream BSD-style license in the installed toolchain's `LICENSE`.
Project licensing remains pending owner confirmation as recorded in ADR 0003.
