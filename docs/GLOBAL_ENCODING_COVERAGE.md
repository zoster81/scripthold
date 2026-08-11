# Global Encoding Coverage — R22 / Scripthold 2.2.0

## Status

**COMPLETE.** R22 shipped in Scripthold `2.2.0` on 2026-08-11. This document is the stable implementation, security, provenance, and verification contract for the resulting encoding subsystem; milestone chronology belongs in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md) and release notes in [CHANGELOG.md](../CHANGELOG.md).

## Final scope

Scripthold `2.2.0` exposes **168 canonical read/write encodings** through one authoritative capability registry. Explicit codec support is deliberately broader than automatic detection.

The completed work includes:

- the complete applicable repository-pinned `golang.org/x/text v0.40.0` codec surface;
- full UTF-32 LE/BE text-pipeline support;
- 88 additional deterministic pure-Go fixed single-byte mappings derived from pinned GNU libiconv evidence;
- 21 additional multibyte/stateful or residual exact codecs derived and verified against the same pinned source;
- conservative detector trust gates and explicit ambiguity for unsafe guesses;
- bounded partial-failure evidence for grep and batch operations;
- registry-driven public-operation verification across all 168 codecs;
- adversarial, fuzz, resource, cancellation, concurrency, race, static/vulnerability, cross-platform, native, container, and deterministic packaging gates.

`list_encodings` is the runtime authority for canonical names, aliases, and capability metadata. Documentation must not maintain a second hand-counted codec inventory.

## Sources and runtime boundary

R22 uses three complementary sources:

- repository-pinned `golang.org/x/text` as the primary pure-Go codec source;
- [`arthenica/libiconv`](https://github.com/arthenica/libiconv) as a pinned source of portable charset definitions, mappings, fixtures, and differential-test oracles;
- [`oe-mirrors/uchardet`](https://github.com/oe-mirrors/uchardet) as a pinned source of real-world detection fixtures, detector nomenclature, and differential evidence.

The production runtime remains pure Go. Ordinary builds and runtime execution do **not** depend on libiconv, uchardet, GCC, CGO, subprocesses, or network access for charset support.

Checked-in external fixtures and generated mapping evidence are bound to immutable upstream revisions and record the applicable source path/URL, byte size, SHA-256, declared charset, and licensing/provenance metadata. Network access is a maintainer-only refresh activity; CI and normal tests consume checked-in data.

The generated R22 mapping source is pinned to GNU libiconv revision `9d19c66d0a1768cffcf497b2db70bf4018b578d7`. Generation tools are verification/oracle infrastructure, not production dependencies.

## Capability model

Every public registry entry has one stable canonical name and explicit capability metadata covering:

- declared aliases and accepted detector labels;
- decoder and encoder construction;
- BOM/Unicode semantics;
- structural validation requirements;
- automatic-detection eligibility;
- explicit-only status where evidence is insufficient for safe detection.

The public states are:

1. **Supported** — explicit read/write/convert/edit behavior is implemented and verified.
2. **Auto-detectable** — content evidence may select the encoding under the conservative detector policy.
3. **Explicit-only** — the codec is supported, but automatic selection is intentionally disabled or restricted.
4. **Excluded** — non-portable, host-dependent, insecure, lossy, or insufficiently verifiable representations are not exposed as file encodings.

Aliases never count as separate encodings. A detector label cannot become a trusted result unless it resolves to an auto-detectable registry entry or is rejected as ambiguous/unsupported before a text consumer opens the file.

## Unicode contract

UTF-8, UTF-16 LE/BE, and UTF-32 LE/BE are supported Unicode transformation formats.

UTF-32 validation rejects:

- byte lengths not divisible by four;
- truncated code units;
- surrogate code points;
- scalars above `U+10FFFF`;
- BOM/endianness conflicts.

UTF-32 BOMs are authoritative. BOMless LE/BE detection is conservative and requires aligned structural evidence, valid scalars, and decoded-text quality. Generic `utf-32` remains rejected because a byte-order-unspecified file encoding is not deterministic.

Line-ending transformation is code-unit aware for UTF-32 rather than reusing byte-oriented or UTF-16-specific logic.

Scripthold does not invent a proprietary UTF-64 format and does not expose machine-dependent internal representations such as native-width `wchar_t` or host-endian `UCS-*-INTERNAL` as portable file encodings.

## Additional single-byte mappings

The generated single-byte set is selected mechanically from pinned libiconv-compatible definitions.

A mapping is admitted only when:

- each defined source byte consumes exactly one byte;
- decoding produces a valid Unicode scalar;
- encoding that scalar returns the identical source byte;
- the mapping does not replace an existing `x/text`-owned canonical behavior;
- the generated output is deterministic and reviewable.

The inventory identified 130 fixed one-byte/bijective candidates across the evaluated definition sets. Existing `x/text` ownership and compatibility aliases account for 42 of them; the remaining **88** became additional pure-Go registry entries.

Every generated single-byte codec is exhaustively verified across all 256 byte positions, including strict rejection of undefined bytes and byte-identical reverse mapping for every defined scalar.

## Additional multibyte and stateful codecs

R22 adds these 21 canonical codecs beyond the pre-existing/x-text surface:

- `big5-2003`;
- `big5-hkscs:1999`, `big5-hkscs:2001`, `big5-hkscs:2004`, `big5-hkscs:2008`;
- `euc-cn`, `euc-jisx0213`, `euc-tw`;
- `gb18030-2022`;
- `ibm1162`, `ibm1163`;
- `iso-2022-cn`, `iso-2022-cn-ext`;
- `iso-2022-jp-1`, `iso-2022-jp-2`, `iso-2022-jp-3`, `iso-2022-jp-ms`;
- `iso-2022-kr`;
- `johab`;
- `shift_jisx0213`;
- `tcvn`.

Direct multibyte codecs use compact generated prefix-free mappings plus canonical reverse mappings. ISO-2022 variants use bounded pure-Go state machines over generated raw character-set tables. TCVN uses generated standalone/composition mappings with bounded lookahead.

IBM1162 and IBM1163 were admitted only after exact-byte round-trip verification for every defined byte. CP1161 remains excluded because its canonical reverse mapping is not byte-exact for every defined source byte.

### GB18030:2022

`gb18030-2022` is a strict wrapper around the pinned `x/text` GB18030 implementation plus generated differential overrides.

The maintainer oracle compares:

- all 1,611,540 grammatical two/four-byte sequences; and
- all 1,112,064 non-surrogate Unicode scalar values

against the pinned GNU libiconv GB18030:2022 converter. The checked-in result contains **2,087 decode overrides** and **2,087 encode overrides**, with source-header SHA-256 provenance.

Runtime lookup avoids per-sequence heap allocation; bounded-output reads remain approximately constant-memory as source size grows.

## Compatibility aliases

Compatibility spellings do not inflate the canonical count or silently replace established behavior. In particular, existing ownership is retained for unversioned Big5-HKSCS, CP932/943, CP936, CP949, CP950, GB2312/csgb2312, and HZ aliases where documented by the registry.

Raw graphic character sets that do not provide the ASCII/control behavior required by the text-file contract are not exposed as complete file encodings.

## Detection policy

Automatic detection remains byte/content based and filename-independent.

The trust pipeline is:

1. authoritative BOM inspection;
2. strict structural validation for Unicode candidates;
3. detector candidate collection and registry canonicalization;
4. candidate-specific syntax validation;
5. strict decoding;
6. decoded-text quality and binary rejection;
7. confusion/competitor comparison;
8. deterministic trusted result or explicit ambiguity.

A probabilistic legacy guess does not become trusted merely because one decoder can map every byte. Short non-ASCII inputs require an evidence floor, and known confusion pairs are exercised explicitly.

HZ-GB-2312, ISO-2022-JP, and ISO-2022-KR require verified escape/shift syntax, strong raw-detector agreement, strict decoding, and decoded non-ASCII evidence. Signature-like ASCII remains ambiguous.

GB18030 four-byte grammar can distinguish GBK, but syntax alone cannot safely choose between generic GB18030 and the exact GB18030:2022 public codec when the bytes are revision-equivalent. Revision-specific ambiguity remains explicit when evidence cannot distinguish them.

`sample`, `chunked`, and `full` modes share candidate semantics. Stateful detector evidence survives arbitrary chunk boundaries; bounded probes do not grow with complete source size.

The pinned 61-fixture detection corpus yields:

- 29 exact trusted canonical results;
- 1 GB18030 payload safely classified as the byte-exact/text-equivalent narrower GBK subset;
- 31 explicit ambiguities;
- **zero trusted text mismatches**.

A trusted BOMless result must resolve to an `autoDetectable` registry descriptor and strict-decode successfully.

## Strict decoding and encoding

Malformed input is never silently repaired as ordinary behavior. Invalid byte sequences, truncated multibyte/stateful sequences, illegal state transitions, malformed Unicode scalars, or unrepresentable output produce deterministic encoding errors before mutation.

For single-byte mappings, every defined byte must round-trip exactly. For multibyte/stateful formats with multiple equivalent byte spellings, tests distinguish Unicode semantic equivalence from canonical encoder output rather than requiring a historical byte representation that the format does not guarantee.

Cancellation remains typed as `CANCELLED` through conversion/stream layers rather than being reclassified as a generic encoding or I/O failure.

## Public operation contract

Every registered codec is exercised through every applicable public encoding-aware path:

- `list_encodings`;
- `detect_encoding` where auto-detectable;
- `read_text_file` and `read_multiple_files`;
- `grep_text_files`;
- `write_whole_file`;
- `edit_file`, including preview/apply and byte-identical no-op behavior;
- `patch_package` where text preparation applies;
- `convert_encoding`, including dry run and unrepresentable-rune diagnostics;
- `detect_line_endings` and `change_line_endings`;
- `manage_bom` where the encoding defines BOM semantics;
- `verify_state` text/JSON checks;
- the three encoding workflow prompts.

BOM and line-ending preservation promises remain operation-specific and unchanged by codec expansion.

## Grep and batch partial failures

Partial coverage is explicit rather than silently presented as complete.

For `grep_text_files`:

- `filesSearched` is the deterministic candidate count;
- `filesScanned` counts candidates processed without per-file failure before processing stopped;
- `filesSkipped` counts encountered per-file failures;
- `coverageComplete` is false when any candidate is skipped or scanning stops early;
- `skippedFiles` retains a deterministic bounded prefix of path/error metadata;
- truncation and omitted counts state when additional failures are not retained.

Valid matches from readable files remain available. Cancellation and output-limit failures are terminal rather than ordinary skips.

Batch read/conversion results preserve deterministic per-file ordering and exact total error counts while duplicate error-summary text remains bounded. A conversion never mutates a file whose decoding/encoding preflight failed.

Encoding failures may refine the stable top-level error with `encodingErrorCode` values such as ambiguous, malformed, unsupported, BOM-conflicting, or unrepresentable input.

## Corpus contract

Checked-in real-world corpora are regression evidence, not scratch state.

Each managed fixture records the actual local bytes and immutable provenance. A replaced fixture must update its source reference, size, and SHA-256 together; stale metadata is a test failure. Where practical, fixtures also carry independently produced UTF-8 oracles or oracle digests.

Maintainer refresh tooling may download only from approved pinned sources and must verify bytes before writing deterministic metadata. Normal tests remain offline.

Filename/extension independence is tested by placing identical bytes under unrelated names where appropriate.

## Verification contract

Codec and detector changes require focused TDD and the relevant subset of:

- exhaustive single-byte maps;
- real fixture decode/encode and independent UTF-8 oracle checks;
- valid/invalid multibyte and stateful boundary cases;
- truncation at every relevant byte position;
- arbitrary chunk boundaries, including one-byte reads;
- BOM present/absent/conflicting cases;
- LF, CRLF, lone CR, mixed, and no-ending cases;
- same bytes under unrelated filenames;
- short-input confusion and binary negatives;
- mutation no-op/preservation and failure-before-mutation checks;
- source-change, cancellation, disk/write failure, and durable-staging injection;
- bounded large-input allocation/throughput checks;
- detector/decoder/encoder/BOM/line fuzz targets;
- complete repository tests, race detector, vet, Staticcheck, govulncheck, release-script tests, workflow checks, and secret scanning when release-adjacent.

R22 completion evidence included 225,000 fixed-count encoding/text fuzz executions, representative 1/16/64 MiB bounded-output allocation gates, six Windows/Linux/macOS amd64/arm64 builds, 114 target-specific test-binary compilations, native Windows runtime smoke, hardened Linux container stdio/direct-TLS HTTP smoke, and two independent byte-identical GoReleaser snapshots of the final clean release source.

## Release boundary and completion record

The local release-candidate procedure may build ordinary binaries/archives and deterministic GoReleaser snapshots, but real MCPB bundles, `mcpb-checksums.txt`, the final MCPB-backed Registry manifest, and Registry publication are **GitHub-only** outputs and are never produced or simulated locally.

R22 completed on 2026-08-11. The exact release commit passed the full push-event `CI` `Release candidate` gate before annotated tag `v2.2.0` was created. GoReleaser publication, GitHub-only MCPB packaging, and MCP Registry publication then completed successfully.

Operational release details belong in [PUBLISHING.md](PUBLISHING.md); milestone history belongs in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md).

## Non-goals

- filename- or extension-based encoding selection;
- permissive best-effort decoding that hides malformed bytes;
- lossy transliteration as ordinary conversion behavior;
- machine-dependent internal character representations;
- a proprietary UTF-64 format;
- native runtime dependencies added solely to increase charset count;
- weakening filesystem confinement, durable mutation, transport security, or bounded-memory guarantees.
