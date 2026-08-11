# Global Encoding Coverage — R22 / Scripthold 2.2.0

## Status

`ACTIVE` design and implementation contract for R22. The current public release remains `2.1.1`; the behavior described here is a development target until the corresponding implementation and verification gates are complete.

## Goal

Expand Scripthold from its current 24 registered read/write encodings to the broadest practical, portable, deterministic text-encoding coverage that can be implemented and verified across Windows, Linux, and macOS without weakening existing bounded-memory, filesystem, mutation, transport, or error guarantees.

R22 is the release-scoped milestone for Scripthold `2.2.0`.

## Sources and runtime boundary

R22 uses three complementary sources:

- the repository-pinned `golang.org/x/text` implementation as the primary pure-Go codec source;
- [`arthenica/libiconv`](https://github.com/arthenica/libiconv) as a pinned source of portable charset definitions, mappings, fixtures, and differential-test oracles;
- [`oe-mirrors/uchardet`](https://github.com/oe-mirrors/uchardet) as a pinned source of real-world detection fixtures, detector nomenclature, and differential evidence.

The production runtime remains pure Go. R22 must not introduce a libiconv or uchardet shared-library, DLL, subprocess, or CGO runtime dependency merely to increase charset count. External repositories are test/provenance/oracle sources unless a separately reviewed implementation decision says otherwise.

All downloaded fixtures must be bound to an immutable upstream revision and record source repository, revision, source path or URL, byte size, SHA-256, declared charset, and licensing/provenance information. CI and normal tests must remain offline; network access is a maintainer-only corpus refresh operation.

## Coverage model

Every public encoding entry must have one stable canonical name, documented aliases, a decoder, an encoder where the source format is writable, and explicit capability metadata.

R22 distinguishes four capability states:

1. **Supported** — explicit read/write/convert/edit operations are implemented and verified.
2. **Auto-detectable** — the detector may select the encoding only when evidence satisfies its conservative confidence and validation policy.
3. **Explicit-only** — the codec is supported, but automatic selection is intentionally disabled because reliable discrimination cannot be guaranteed.
4. **Excluded** — non-portable, process-internal, lossy, non-standard, or insufficiently verifiable representations are not exposed as file encodings.

Aliases never count as independent encodings. A charset must not be described as supported merely because the detector can name it or an external library recognizes it.

## Unicode scope

R22 promotes UTF-32 LE and UTF-32 BE from BOM-management-only behavior to full text encodings across the normal read/write/search/edit/convert/verification pipeline. Generic BOM-aware UTF-32 behavior may be exposed only with deterministic documented semantics.

UTF-32 validation must reject code points above `U+10FFFF`, surrogate code points, truncated 32-bit units, malformed byte lengths, and BOM/endianness conflicts. BOMless UTF-32 automatic detection must remain conservative; explicit LE/BE selection remains available when evidence is insufficient.

**Current R22 source status:** phases 1–10 are implemented. UTF-32 LE/BE are full registry entries backed by the pinned `x/text` codec with strict pre-decode scalar validation, authoritative BOM handling, conservative aligned BOMless detection, and 32-bit line-ending transformation. Phase 5 adds 88 deterministic pure-Go fixed single-byte mappings generated from pinned GNU libiconv revision `9d19c66d0a1768cffcf497b2db70bf4018b578d7`; phase 6 adds 21 multibyte/stateful or residual exact codecs derived and verified from the same pinned source, bringing the source-tree read/write registry to 168 canonical encodings. Phase 7 hardens automatic detection without changing that count: probabilistic legacy results must pass evidence, registry closure, strict decoding, decoded-text quality, binary rejection, and known-confusion checks; HZ-GB-2312, ISO-2022-JP, and ISO-2022-KR may be trusted only through verified stateful evidence. Phase 8 makes grep and batch partial encoding failures explicit and bounded, adds additive per-file encoding failure subcodes, and makes incremental UTF-8 decoding strict. Phase 9 adds a registry-driven public-operation gate over all 168 codecs. Phase 10 adds representative malformed/cancellation/limit/concurrency tests, 225,000 fixed-count fuzz executions, and 1/16/64 MiB bounded-output benchmarks across UTF-32, generated single-byte, direct multibyte, ISO-2022, TCVN, and GB18030:2022 classes. It also fixes cancellation propagation through conversion streams and removes GB18030:2022 per-sequence heap allocation, keeping representative large-read allocations approximately constant at 150–155 KiB/op. The phase-5 mappings and phase-6 additions remain explicit-only except for ISO-2022-KR, whose independent stateful signature justifies promotion. Generic `utf-32` remains rejected because byte order is not deterministic without an explicit protocol rule.

Unicode defines UTF-8, UTF-16, and UTF-32 transformation formats. R22 does not invent a proprietary `UTF-64` encoding. Machine-internal representations such as native-width `wchar_t`, `UCS-*-INTERNAL`, or other host-endian/alignment-dependent formats are excluded from the portable public file-encoding contract.

## Registry architecture

`internal/encoding` must become the single source of truth for encoding capabilities. The registry must centralize:

- canonical public name;
- aliases and detector labels;
- decoder and encoder constructors;
- Unicode/BOM behavior;
- structural validation requirements;
- auto-detection eligibility;
- explicit-only status where applicable;
- documentation metadata.

The detector, stream decoder/encoder, BOM logic, handlers, tests, and `list_encodings` output must consume that shared registry instead of maintaining independent charset assumptions.

A build/test invariant must prove that every detector label that can become a trusted result either canonicalizes to a supported registry entry or is deliberately rejected/ambiguous before a text consumer attempts to open it. `detected but unsupported` is not an acceptable steady-state outcome for an in-scope R22 charset.

## Codec implementation order

### Phase A — existing pure-Go coverage

First expose every applicable portable codec already available from the repository-pinned `golang.org/x/text`, including its single-byte charmaps and Japanese, Korean, Simplified Chinese, Traditional Chinese, UTF-16, and UTF-32 families.

This phase includes exhaustive alias normalization and tests proving that public canonical names remain stable.

### Phase B — additional single-byte mappings

For portable libiconv-compatible single-byte encodings not already provided by `x/text`, prefer generated static Go tables derived from pinned authoritative mappings. Generation output must be deterministic and reviewable; generated tables must not depend on the network at build or test time.

Phase 5 applies that rule mechanically. The maintainer generator reads the pinned libiconv public/system compatibility definition sets, verifies the exact upstream revision, and probes selected converter headers across all 256 possible bytes. A mapping is eligible only when every defined input consumes exactly one byte and its Unicode scalar encodes back to the identical source byte. Stateful/buffered, multibyte, non-bijective, and existing `x/text`-owned mappings are excluded from this phase. The resulting 88 selected codecs are checked in as pure-Go tables with source-definition and source-header SHA-256 provenance; libiconv and GCC are generation-time oracles only and are not runtime or normal-test dependencies.

The inventory found 130 pinned fixed one-byte/bijective mappings across the evaluated public, AIX, DOS, extra, and z/OS definition sets. Of those, 24 are byte-identical to existing `x/text` charmaps and 18 resolve through existing public names/aliases whose `x/text` semantics remain authoritative; the remaining 88 are the phase-5 additions. This prevents alias spelling from inflating the codec count and avoids silently replacing already published mappings.

Every single-byte codec receives exhaustive 256-byte decode coverage and representable-rune encode coverage. Undefined byte values follow explicit strict semantics rather than silently substituting replacement characters. The phase-5 regression suite therefore verifies 22,528 byte positions across the 88 new mappings, including exact reverse-byte round trip for every defined scalar.

### Phase C — additional multibyte/stateful encodings

Implement missing multibyte or stateful families one at a time. Each implementation must provide an incremental streaming decoder and encoder with bounded state across arbitrary chunk boundaries. Examples may include EUC-TW, ISO-2022 variants, Johab, Big5 variants, or other portable formats found in the approved source repositories, but inclusion requires successful oracle and adversarial verification.

Phase 6 applies that rule to 21 canonical additions: `big5-2003`, four versioned Big5-HKSCS variants, `euc-cn`, `euc-jisx0213`, `euc-tw`, `gb18030-2022`, `ibm1162`, `ibm1163`, `iso-2022-cn`, `iso-2022-cn-ext`, `iso-2022-jp-1`, `iso-2022-jp-2`, `iso-2022-jp-3`, `iso-2022-jp-ms`, `iso-2022-kr`, `johab`, `shift_jisx0213`, and `tcvn`. Direct multibyte codecs use compact generated prefix-free decode tables plus canonical reverse mappings. ISO-2022 variants use bounded pure-Go state machines over generated raw character-set tables instead of serializing the full cross-product of reachable states. TCVN uses generated standalone/composition mappings with at most one byte of lookahead. IBM1162 and IBM1163 were admitted only after the generator proved exact byte round-trip for every defined byte; CP1161 remains excluded because four defined source bytes are not byte-exact under its canonical reverse mapping.

`gb18030-2022` is implemented as a strict wrapper over the pinned `x/text v0.40.0` GB18030 codec with generated differential overrides rather than assuming which standard revision the base library implements. The maintainer oracle exhaustively compares all 1,611,540 grammatical two/four-byte sequences and all 1,112,064 non-surrogate Unicode scalar values against GNU libiconv's pinned GB18030:2022 converter. The checked-in result contains 2,087 decode overrides and 2,087 encode overrides plus the converter-header SHA-256. Runtime and normal tests require no libiconv, GCC, CGO, subprocess, or network access.

Compatibility aliases do not inflate the count or silently change existing semantics: unversioned `big5-hkscs` remains owned by the existing `big5` compatibility mapping; CP932/CP943 remain `shift_jis`; CP936 remains `gbk`; CP949 remains `euc-kr`; CP950 remains `big5`; `gb2312`/`csgb2312` remain existing `gbk` aliases; and `hz` remains `hz-gb-2312`. Raw graphic character sets such as JIS X0208/JIS X0212/ISO-IR-165 are not exposed as complete file encodings because they do not provide the ASCII/control behavior required by the text-file contract. Legacy Unicode-compatible formats remain governed by Phase D.

No family advances to the shared public registry until its focused test matrix is green. All 21 phase-6 additions satisfied that gate. Phase 7 keeps them explicit-only except for ISO-2022-KR, whose escape-driven syntax, raw-detector agreement, strict decode, and decoded non-ASCII evidence provide an independent trust signal.

### Phase D — legacy Unicode-compatible formats

Evaluate portable legacy Unicode representations such as UCS-2/UCS-4 or UTF-7 individually. Formats whose semantics are ambiguous, insecure for auto-detection, host-dependent, or not safely round-trippable may be explicit-only or excluded. UTF-7, if supported, must never be automatically selected without separately justified structural evidence.

## Detection policy

Automatic detection remains byte/content based and filename-independent.

The decision pipeline is:

1. authoritative BOM inspection;
2. strict structural validation for Unicode candidates;
3. detector candidate collection and label canonicalization;
4. candidate-specific syntax validation;
5. decoded-text quality and binary rejection;
6. candidate comparison/confusion handling;
7. deterministic confidence result or explicit ambiguity.

Short non-ASCII inputs require an evidence floor. A probabilistic single-byte guess must not become trusted merely because one decoder can map every byte. The R22 regression suite includes known confusion pairs and families such as CP437/MacRoman, CP850/MacRoman, ISO-8859-1/Windows-1252, Windows-1251/MacCyrillic, Windows-1253/ISO-8859-7, CP949/EUC-KR, CP932/Shift-JIS, CP950/Big5, CP936/GBK, and collisions discovered from the expanded corpus. Phase 7 intentionally returns ambiguity for plausible competing legacy decoders instead of maximizing the number of guesses.

Stateful syntax is necessary but not sufficient. HZ-GB-2312, ISO-2022-JP, and ISO-2022-KR require a recognized escape/shift signature, strong raw-detector agreement, strict decode of the selected canonical codec, and a minimum decoded non-ASCII evidence floor. Signature-like ASCII examples remain ambiguous. GB18030 four-byte grammar excludes GBK, but because both generic GB18030 and exact GB18030:2022 are public codecs, byte syntax alone does not guess the revision; explicit selection is required when revision-specific evidence cannot be established.

`sample`, `chunked`, and `full` modes share candidate semantics and produce equivalent decisions for inputs that fit completely in all modes. Chunked state survives arbitrary boundaries for Unicode and escape-driven formats; phase-7 tests place an ISO-2022 escape sequence across the configured chunk boundary. A bounded stateful probe retains at most 64 KiB once a signature begins; full payload validation remains streaming.

The pinned 61-fixture detection corpus now yields 29 exact trusted canonical results, one GB18030 payload safely classified as the byte-exact/text-equivalent narrower GBK subset, and 31 explicit ambiguities with **zero trusted text mismatches** under a permanent corpus invariant. A trusted non-BOM result must resolve to an `autoDetectable` registry descriptor and strict-decode successfully. Unicode BOMs remain authoritative even when the following payload is malformed; text operations then reject the malformed payload through the strict decoder.

## Strict decoding and encoding

R22 must not silently repair malformed input. Invalid byte sequences, truncated multibyte sequences, illegal state transitions, invalid Unicode scalar values, or unrepresentable output must produce deterministic encoding errors before mutation.

For single-byte mappings, exact byte round-trip is required for every defined byte value. For multibyte or stateful formats that permit multiple equivalent byte representations, tests must distinguish Unicode semantic equivalence from canonical encoder output rather than incorrectly requiring one historical byte spelling.

## Public operation matrix

Every supported encoding must be exercised through all applicable encoding-aware paths:

- `list_encodings`;
- `detect_encoding` when auto-detectable;
- explicit `read_text_file`;
- `read_multiple_files`;
- `grep_text_files`;
- `write_whole_file`;
- `edit_file`, including preview/apply and byte-identical no-op behavior;
- `patch_package` where text preparation applies;
- `convert_encoding`, including dry-run and unsupported-rune diagnostics;
- `detect_line_endings`;
- `change_line_endings`;
- `manage_bom` where BOM semantics exist;
- `verify_state` JSON/text checks;
- prompt workflows that audit, diagnose mojibake, or migrate encodings.

BOM and line-ending preservation guarantees remain unchanged. UTF-32 requires code-unit-aware line-ending transformation rather than reuse of ASCII-byte or UTF-16-specific logic.

## Grep and batch partial-result policy

Phase 8 makes partial coverage explicit without changing successful match or batch-result ordering. `grep_text_files.filesSearched` remains the deterministic candidate-file count; `filesScanned` counts candidates processed without a per-file failure before processing stopped, `filesSkipped` counts encountered per-file failures, and `coverageComplete` is false whenever any file was skipped or processing stopped before every candidate was scanned. Valid matches from other files remain available. Per-file `skippedFiles` details are retained in deterministic order under a fixed 64-entry cap and a separate byte budget derived from the output limit. Retained valid grep results take priority over those diagnostic details when only residual budget remains; `skippedFilesTruncated` and `skippedFilesOmitted` state explicitly when additional failures were omitted.

Per-file failures retain the stable 2.x `errorCode` vocabulary and may add `encodingErrorCode` to distinguish `ENCODING_AMBIGUOUS`, `ENCODING_MALFORMED`, `ENCODING_UNSUPPORTED`, `ENCODING_BOM_CONFLICT`, `ENCODING_UNREPRESENTABLE`, and other encoding failures. Grep treats cancellation and output-limit failures as terminal rather than presenting them as ordinary skips. Supplying an explicit encoding continues to bypass auto-detection ambiguity when that codec is supported.

`read_multiple_files` and batch `convert_encoding` continue to return one deterministic per-file result for every admitted input and preserve successful results when other files fail. Their total `errorCount` remains exact, while the compatibility `errors` summary retains only a deterministic bounded prefix under the same fixed-count/byte-budget policy and exposes `errorsTruncated` plus `errorsOmitted`. For batch reads, decoded content takes priority and the duplicate summary yields to the remaining aggregate output budget. Single-file operations continue to fail explicitly. A batch conversion never mutates a file whose decoding or encoding preflight failed.

## Corpus contract

`filetoolsserver/handler/testdata/internet-corpus` becomes a regression corpus rather than scratch evidence once its manifest is normalized.

For each fixture the manifest must record the actual local bytes and their true provenance. Replaced or enriched fixtures must not retain stale source URLs, sizes, or hashes from earlier files. Where practical, each fixture should also have a separately produced UTF-8 oracle or an oracle digest.

Maintainer corpus refresh tooling may download additional fixtures from the approved repositories only at pinned revisions, verify the retrieved bytes, and write deterministic metadata. Normal unit/integration tests must consume only checked-in data.

Filename and extension must never influence expected detection. Tests should copy identical bytes under unrelated names where needed to prove this invariant.

## Test strategy

### Focused TDD

For each new codec or detector rule:

1. add the smallest focused failing fixture/test;
2. confirm the expected failure mode;
3. implement the smallest codec/registry/detection change;
4. rerun the focused test;
5. run the complete affected encoding and handler suites before proceeding.

### Codec tests

- exhaustive 256-byte maps for every single-byte encoding;
- decode/encode round-trip for real fixtures;
- independent UTF-8 oracle comparison where available;
- every valid and invalid boundary sequence for multibyte/stateful formats;
- truncated input at every possible byte position;
- invalid escape/state transitions;
- unsupported output rune diagnostics;
- BOM present/absent/conflicting forms;
- LF, CRLF, lone CR, mixed endings, and no-ending files;
- arbitrary chunk boundaries, including one-byte reads.

### Detection tests

- manifest-declared real-world corpus;
- same bytes under unrelated filenames/extensions;
- short-input ambiguity;
- confusion matrices for overlapping legacy charsets;
- valid multilingual text and mixed scripts;
- executable, image, archive, compressed, random, sparse-NUL, and structured binary negatives;
- malformed Unicode and malformed stateful encodings;
- `sample`/`chunked`/`full` consistency;
- deterministic repeatability;
- detector-result-to-registry closure.

### Mutation and filesystem tests

- exact preservation of source encoding, BOM, and line endings;
- byte-identical no-op behavior;
- read-only files and explicit writable override;
- staged-write failures and disk-full injection;
- source changes between preparation and commit;
- cancellation at decode, encode, stage, and commit boundaries;
- backup creation/rollback behavior where requested;
- symlink, junction, reparse-point, hard-link, and allowed-root constraints unchanged.

### Resource tests

- bounded decoded lines and outputs;
- aggregate batch and grep budgets;
- exceptionally long lines;
- large single-byte, multibyte, stateful, UTF-16, and UTF-32 files;
- allocation/throughput benchmarks proving streaming working memory remains bounded independently of complete source size except for already documented full-document operations.

### Fuzzing

Add or extend fuzz targets for:

- charset detection and canonicalization;
- every new structural validator;
- incremental decoders and encoders;
- stateful escape machines;
- UTF-32 scalar validation;
- BOM handling;
- line framing and line-ending transforms.

Fuzz smoke used in CI must remain deterministic and bounded according to the repository release policy.

## R22 implementation sequence

1. Normalize the existing internet-corpus manifest and provenance.
2. Redesign the registry around one capability descriptor and detector-label closure tests.
3. Expose the complete applicable `x/text` codec set.
4. Promote UTF-32 LE/BE to the full text pipeline.
5. Add additional portable single-byte libiconv mappings with generated pure-Go tables. **Implemented: 88 new explicit-only mappings; registry total 147.**
6. Add missing multibyte/stateful families one at a time under focused TDD. **Implemented: 21 explicit-only additions; registry total 168.**
7. Harden global detection, short-input ambiguity, and confusion handling. **Implemented: conservative trust gating with zero trusted text mismatches on the 61-fixture pinned detection corpus.**
8. Make grep/batch partial encoding failures explicit and bounded. **Implemented: deterministic coverage metadata, bounded skipped/error summaries with omitted counts, stable encoding subcodes, and strict streaming UTF-8 validation.**
9. Run the full public-operation matrix for every supported encoding. **Implemented: registry-driven public handler coverage across all 168 codecs, trusted corpus/BOM detection, applicable JSON/BOM semantics, and all three encoding workflow prompts.**
10. Complete adversarial, fuzz, memory, concurrency, and failure-injection verification. **Implemented: representative R22 class matrices, cancellation/limit/malformed no-mutation gates, deterministic concurrency checks, 225,000 fixed-count fuzz executions, shared durable-layer failure injection, and bounded 1/16/64 MiB allocation verification.**
11. Complete full repository, six-target, native/container, packaging, and deterministic release-candidate verification. **Source-only preflight complete: GoReleaser configuration, public PowerShell launcher parsing, actionlint 1.7.12, ShellCheck 0.11.0, and untracked-residue checks are green; artifact-producing six-target/container/packaging/MCPB/Registry gates remain pending.**
12. Only after all prior gates are green, prepare and publish Scripthold `2.2.0` through the normal exact-commit release procedure.

## Completion gate

R22 is complete only when all of the following are true:

- the final supported-encoding inventory is generated or verified from the authoritative registry, with no documentation count drift;
- every supported encoding has a verified decoder and encoder where writable;
- every trusted auto-detection result resolves to a supported registry entry;
- ambiguous data fails explicitly rather than using an unjustified fallback;
- malformed input never silently substitutes replacement text;
- UTF-32 LE/BE participates in the complete supported text-operation matrix;
- directory grep/search results expose skipped encoding failures rather than implying complete scans;
- corpus provenance, hashes, and oracles are internally consistent and reproducible from pinned sources;
- public operation tests pass for every supported encoding;
- focused fuzzing, resource, cancellation, failure-injection, and race tests pass;
- `go mod verify`, the complete Go suite, vet, Staticcheck, govulncheck, Node release tests, catalog/runtime/documentation checks, Gitleaks, and `git diff --check` pass;
- Windows, Linux, and macOS amd64/arm64 builds pass, with the applicable native and hardened-container runtime smokes;
- GoReleaser output, six MCPB bundles, and the generated Registry manifest pass the existing deterministic release-candidate gates;
- the release candidate is an exact clean commit with a dated `2.2.0` changelog entry before tagging.

Publication, Registry upload, deployment, active rollback, and restoration remain separate final release operations governed by `PUBLISHING.md`; they must not be inferred from source completion alone.

## Non-goals

- filename- or extension-based encoding selection;
- arbitrary best-effort decoding that hides malformed bytes;
- lossy transliteration as a normal encoding conversion;
- machine-dependent internal character representations;
- a proprietary UTF-64 format;
- adding native runtime dependencies solely to increase the charset count;
- weakening filesystem confinement, durable mutation, transport security, or bounded-memory guarantees.
