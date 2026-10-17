# Unicode table/data redistribution notices (Phase 9a review fix)

The original 27-test checkpoint omitted a separate Unicode grant needed by
uniseg's generated tables. Its MIT license alone is insufficient. The omission
also required checking the shipping Go stdlib/vendor data, not merely adding a
current moving Unicode license page. Earlier artifacts at source `e76a85...`
are superseded and must not be handed to the user as legal-complete bundles.

## Unicode 15.0.0 — uniseg v0.4.7

The verified module's generated headers explicitly name Unicode 15.0.0 sources,
generated September 5, 2023, and refer to Unicode's license agreement:

| Module file | Unicode 15 source below https://www.unicode.org/Public/15.0.0/ | SHA256 of module file |
| --- | --- | --- |
| `graphemeproperties.go` | `ucd/auxiliary/GraphemeBreakProperty.txt`, `ucd/emoji/emoji-data.txt` | `321483625d9a4aa92e4a5730df51c59d5b49725cd20e3879749488e5628b3da8` |
| `wordproperties.go` | `ucd/auxiliary/WordBreakProperty.txt`, same emoji data | `10136e6c3c8ea619276428d59278e312aa4ce33b2e50d98f3a618f162b3a5864` |
| `sentenceproperties.go` | `ucd/auxiliary/SentenceBreakProperty.txt`, same emoji data | `272502600803b8e99721cb9d76002a6546ea2350056bf66bb7860c39a2032bd9` |
| `lineproperties.go` | `ucd/LineBreak.txt`, same emoji data | `d73432ab989195312fdaa4f4dcea34c7bfb478160b98ad26fbc31df94f898b45` |
| `eastasianwidth.go` | `ucd/EastAsianWidth.txt`, same emoji data | `1cd2368ddedc56182d2f64576f2a713a2ad6234265d940575285f61006527c5b` |
| `emojipresentation.go` | same emoji data | `c4e1921aafedd787a29a0a6e329b85c6833eb1cfad0cfaf00450783f99e33b9c` |

The versioned [UCD ReadMe](https://www.unicode.org/Public/15.0.0/ucd/ReadMe.txt)
and [GraphemeBreakProperty](https://www.unicode.org/Public/15.0.0/ucd/auxiliary/GraphemeBreakProperty.txt)
headers say **© 2022 Unicode®, Inc.** The versioned
[emoji data](https://www.unicode.org/Public/15.0.0/ucd/emoji/emoji-data.txt) agrees.
Fetched research copies SHA256 respectively:
`53672c0d0b5185e3cf04c8e970d544c3af81ae7c8eeba0b9cf6d355aa954ae1f`,
`5a0f8748575432f8ff95e1dd5bfaa27bda1a844809e17d6939ee912bba6568a1`,
`29071dba22c72c27783a73016afb8ffaeb025866740791f9c2d0b55cc45a3470`.
Data files themselves are not added to the source or shipping bundle.

The full historical **UNICODE, INC. LICENSE AGREEMENT — DATA FILES AND SOFTWARE**
section is retained verbatim in `LICENSES/Unicode-15-data-license.txt`:
Copyright © 1991–2022 Unicode, Inc.; SHA256
`68f5b9f5ea36881a0942ba02f558e9e1faf76cc09cb165ad801744c61b738844`.
Authoritative source: official unicode-org/icu **release-72-1**, exact commit
[`ff3514f257ea10afe7e710e9f946f68d256704b1`, `icu4c/LICENSE`](https://github.com/unicode-org/icu/blob/ff3514f257ea10afe7e710e9f946f68d256704b1/icu4c/LICENSE).
Whole upstream LICENSE SHA256:
`af3e84c401f1a35e8d32d6eb1a33fe587c3981aa5cd206033d9527c1855b57a2`.
The retained bytes equal its COMPLETE first Unicode license section, before the
separator introducing unrelated ICU third-party licenses. No license year or
terms were guessed or rewritten. ICU code/runtime and unrelated grants are not
linked/imported; this pinned official project preserves Unicode's own grant.
Uniseg's MIT grant remains included alongside the Unicode data grant.

## Unicode 17.0.0 — actual Go 1.27.2 stdlib/vendor selection

Both shipping target dependency graphs select `unicode`, x/net IDNA and x/text
bidi/normalization packages. Inspecting actual selected GoFiles (not just vendor
directory contents) established these Unicode 17 tables:

| Go source path | SHA256 |
| --- | --- |
| `src/unicode/tables.go` | `ac0e5d5d1b58ed4b02d7cb4a0085bdefbdb4f7aae6fadddbc9477f8463da8bd8` |
| `src/vendor/golang.org/x/net/idna/tables17.0.0.go` | `c1a0162f154cb63bab4de125817927c1d1d8a19326ad7c6adbb69d9d249b9cf3` |
| `src/vendor/golang.org/x/text/unicode/bidi/tables17.0.0.go` | `a49eafd4eb11f7c8ae810ee5d69ca59a0cddd97dc644afbcb3e40563617088fc` |
| `src/vendor/golang.org/x/text/unicode/norm/tables17.0.0.go` | `26784025eb1881a4452f101a7ef34b33c8ca02bd0603bfec41d2c823494ff7cb` |

Their Version/UnicodeVersion constants are **17.0.0**. The vendor directories
also retain `tables15.0.0.go` alternatives, but `!go1.27` excludes those; the
selected `go1.27` files are Unicode17. The [Unicode17 UCD ReadMe](https://www.unicode.org/Public/17.0.0/ucd/ReadMe.txt)
says **© 2025 Unicode®, Inc.**, SHA256
`9fe1a90bd32659d7953616283dc2bffaa165518aae9ace026040c42c559ba606`.
No CLDR language tables are selected in the shipping graph.

`LICENSES/Unicode-17-data-license.txt` retains the complete **UNICODE LICENSE V3**
grant, Copyright © 2016–2025 Unicode, Inc., SPDX Unicode-3.0; SHA256
`68af68845f9302fdc7f4d903cbdb6f1d376cd97e9c753bff087c35917e7d8831`.
Authoritative source: official unicode-org/icu **release-78.1**, exact commit
[`049e0d6a420629ac7db77256987d083a563287b5`, `LICENSE`](https://github.com/unicode-org/icu/blob/049e0d6a420629ac7db77256987d083a563287b5/LICENSE).
Whole upstream LICENSE SHA256:
`e55522d81edc687a341a4411e0776e54ca654e90147f354a90458aaced4116af`.
Again the retained bytes equal the entire first Unicode grant section, not ICU's
unrelated notices. Existing Go LICENSE/PATENTS/vendor BSD grants are also kept;
none are treated as substitutes for the separate Unicode grant.

## Completeness / verification boundary

`LICENSES/Unicode-data-NOTICE.txt` attributes all six uniseg and four Go tables,
original data copyrights, versions and immutable grant sources. SHA256:
`99d6e72fa8572a8cf43eecc2b68b1063faa283bba603adddd9f715112e527485`.
All three Unicode legal files MUST be present in BOTH human archives and the
installer's concatenated legal asset. Packager checks exact reviewed grant/
notice hashes, the six generated module table selections and all four selected
Go tables/version constants on BOTH targets, recording table hashes. The
independent consumer contract pins the ten table hashes and exact full legal
inventory; mutations missing a grant, changing legal bytes or truncating the
installer legal bundle must fail even after outer hashes are recomputed.
Go/vendor/MIT attribution remains distinct from Unicode data terms. Toolchain/
module updates require a fresh applicability/license/source review, not a new
copyright year guessed from today's web page. Research used only public legal/
source URLs, never accounts, retained probes or native provider endpoints.
