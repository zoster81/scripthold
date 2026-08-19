# Source Intelligence Foundation Contract

## Status

R25 is **COMPLETE**. This document defines the final native language-neutral `source_symbols` foundation completed in 2026-08 and preserved by later source-intelligence work.

## Scope

R25 established a bounded read-only symbol/navigation model, decoded Unicode-scalar coordinates, evidence-qualified language detection, native provider/scanner/composite architecture, and five production canaries: Go, C#, VB.NET, Python, and Classic ASP. R27 later expanded the same architecture to the broad provider catalog and `source_query` without replacing the R25 public contract.

Source intelligence remains dependency-light and native: ordinary requests do not execute project code or require external parser/compiler/LSP runtimes or downloaded grammars. Raw lexical matches are not promoted to structural or semantic facts without the evidence promised by the provider. Requests remain inside allowed roots, bounded by configured file/byte/symbol/output/time limits, cancellation-aware, read-only, and transport-equivalent across stdio and Streamable HTTP.

The public R25 tool is `source_symbols`. Its final strict operation schema and coordinate/evidence/resource contracts are defined below and in the authoritative tool catalog; conceptual pre-implementation request sketches are intentionally omitted from this completed contract.
## Language-neutral symbol model

Every returned symbol must use one shared representation independent of provider technology.

The baseline fields are:

- `path` — authorized source path;
- `language` — canonical language/provider language identifier;
- `kind` — normalized Scripthold symbol kind;
- `name` — source-level declaration name;
- `qualifiedName` — provider-derived stable hierarchical name where reliably available;
- `parent` or `parentQualifiedName` — enclosing declaration/module where applicable;
- `signature` — bounded declaration signature when requested and reliably reconstructable;
- `visibility` — normalized visibility when the language expresses it and the provider can determine it syntactically;
- `start` — 1-based line/column start;
- `end` — 1-based line/column end when reliably available;
- optional provider-specific normalized modifiers represented through an approved bounded extension rather than arbitrary raw AST dumps.

Result-level fields must include:

- provider/language used;
- files considered and parsed;
- files skipped/failed with bounded stable error evidence;
- symbol count;
- truncation state;
- `coverageComplete` or equivalent explicit completeness evidence.

A partial result must never be presented as complete when files were skipped, limits stopped traversal, or parse failures prevented coverage.

## Normalized symbol kinds

The common model must cover at least these concepts when supported by a language:

- `package` / `module` / `namespace`;
- `type` / `class` / `struct` / `enum`;
- `interface` / `trait` / protocol-like declaration;
- `function`;
- `method`;
- `constructor`;
- `field` / `property`;
- `constant`;
- `variable` / `global`;
- type aliases;
- language-specific declaration kinds only through a documented extensible mapping.

The public API must not force unlike constructs into a misleading kind merely to make every provider identical. Common kinds should be normalized where semantics are genuinely comparable; provider-specific distinctions may be additive and explicit.

## Hierarchy and source outline

A principal R25 use case is a compact source outline.

The model must make it possible to reconstruct relationships such as:

```text
package/module
  -> type/class
      -> field/property
      -> constructor
      -> method
  -> function
  -> constant/global
```

Providers may emit a flat ordered symbol list with parent identifiers or an explicitly bounded nested representation. The implementation should prefer a representation that is deterministic, easy for MCP clients to consume, and does not duplicate large symbol payloads.

Source order must be deterministic where the language/provider exposes it.

## Language identification

Language selection must not be an unreviewed filename-extension heuristic.

The initial contract is:

- an explicit `language` request selects a registered provider but still requires that provider to parse/validate the source;
- without explicit language, a bounded provider-selection layer may use path naming as a candidate hint only, then require parser/provider validation before reporting a language as successfully parsed;
- ambiguous or unsupported cases are reported explicitly;
- a file extension may narrow candidate providers for efficiency but is not authoritative evidence comparable to successful parsing.

R27 later extended language detection with project/path/provider evidence while preserving explicit uncertainty rather than guessing silently.

## Encoding contract

Source intelligence must integrate with Scripthold's content-based encoding model.

Requirements:

- source bytes remain subject to normal allowed-root and file-size limits;
- BOM evidence remains authoritative;
- non-empty ambiguous encodings must not be silently decoded under an arbitrary legacy codec;
- providers consume text only after an approved decode path unless the language parser itself requires exact raw-byte semantics;
- line/column positions must refer unambiguously to the source document as exposed to the user;
- any byte-offset fields require explicit provider guarantees and cannot be assumed portable across decoded encodings;
- signatures returned to the MCP client are UTF-8 transport text;
- malformed/unrepresentable source produces bounded typed parse/encoding evidence rather than mutation.

The Go provider may use the native parser's byte-oriented API internally, but the public position contract must remain consistent with the shared source model.

## Provider architecture

R25 uses a narrow internal provider interface rather than embedding Go-specific behavior into the handler.

A provider is responsible for:

- canonical language identity and supported source forms;
- bounded parsing of one source unit;
- normalized declaration extraction;
- position mapping;
- signature generation;
- provider-specific syntax diagnostics;
- declaration capabilities it can and cannot supply.

The orchestration layer is responsible for:

- path authorization;
- deterministic traversal;
- file/byte/output limits;
- encoding policy;
- cancellation;
- concurrency bounds;
- result ordering;
- partial-failure evidence;
- MCP schema and typed error mapping.

Provider code must not independently bypass these shared boundaries.

## Go reference provider

The Go reference provider uses the standard library:

- `go/parser`;
- `go/ast`;
- `go/token`.

It must extract, at minimum:

- package name;
- top-level `const` and `var` declarations;
- named types and aliases;
- struct/interface declarations and relevant members;
- functions;
- methods and receiver association;
- signatures suitable for compact navigation;
- source ranges.

The provider should recover useful declarations from syntactically incomplete source where the standard parser can do so safely, but parse errors must be visible in bounded diagnostics. A partially parsed file cannot silently be labeled fully covered.

R25 does not require type checking, package loading, module download, compiler execution, or `go list` merely to produce syntax-level symbols.

## Indexing scope

The word "index" in R25 means efficient bounded aggregation of parsed symbol records over requested paths. It does not automatically imply a permanent background database.

R25 remains request-scoped and does not depend on a hidden persistent symbol database. R27 later added bounded process-local coherent generations keyed by source/configuration fingerprints without changing the R25 `source_symbols` contract or retaining complete source bodies.

## Ordering and determinism

For identical source bytes, provider version, request, and traversal policy, results should be deterministic.

Expected ordering is deterministic path order followed by source position and stable kind/name tie-breaking. Provider-specific map iteration or parser data structures must not leak nondeterministic ordering into MCP output.

## Error and partial-coverage model

A multi-file symbol request should preserve useful results from valid files while making incomplete coverage explicit.

Expected per-file conditions include:

- parsed successfully;
- parsed with recoverable syntax diagnostics;
- unsupported language;
- ambiguous encoding;
- malformed source/encoding;
- inaccessible or unsafe path;
- file too large;
- cancellation/aggregate limit.

Cancellation and global output/resource exhaustion should remain terminal where continuing would make response bounds unreliable. Ordinary per-file parse failures may be retained as bounded skipped/error records according to the final schema.

## Complexity and bounds

R25 operations are bounded by the final source-specific limits below and by the applicable global file/output ceilings.

Expected asymptotics:

- traversal: `O(entries)` under secure walker bounds;
- parsing: approximately `O(source bytes)` for ordinary parser providers;
- symbol extraction: `O(AST/declaration nodes)`;
- retained memory: bounded per file plus bounded aggregate symbol/signature/result state;
- output: independently bounded by `MCP_MAX_OUTPUT_BYTES` or a more specific lower cap.

No request may retain complete ASTs for an unbounded number of files simultaneously. Large path sets require sequential or bounded-concurrency processing.

## Security boundary

`source_symbols` is read-only.

It must not:

- write caches into the user workspace;
- execute project code, generators, compilers, package managers, build scripts, or language servers implicitly;
- follow escaping links;
- read outside allowed roots;
- expose private backup/task-store roots;
- download dependencies or grammars;
- log complete source text or unbounded signatures.

External parser/compiler/language-server processes are outside the approved R25/R27 source-intelligence plan. Introducing such a provider requires a later explicit architecture decision and cannot be smuggled into R25 as an implementation detail.

## Frozen implementation decisions

The following decisions remain compatibility and architecture constraints. They may be revised only through an explicit maintainer decision that updates this contract and the roadmap before implementation diverges.

1. **Native implementation.** R25/R27 source intelligence is implemented in Scripthold-owned Go code. Do not add Tree-sitter, Babel, OpenRewrite, parser-runtime bindings, downloaded grammars, language servers, compiler frontends, or equivalent third-party parsing engines as the source-intelligence foundation.
2. **External projects are design references only.** Their useful ideas include compact outline/digest/show workflows, structural search, evidence-qualified relations, bounded context assembly, incremental indexing, language registries, composite-document delegation, and capability matrices. Their code/runtime dependencies are not imported merely to obtain those features.
3. **Regex and grep are allowed primitives.** Raw textual matches remain lexical evidence. Regex over validated code spans, token streams, or scope-aware recognizers may contribute to structural results when tests establish that capability. The implementation must never label evidence more strongly than it proves.
4. **No universal compiler is required.** The shared engine should recognize the source structure required for navigation and indexing rather than implement complete evaluation/type semantics for every language.
5. **Shared infrastructure before language proliferation.** Reusable decoding/position mapping, language detection, scanning, tokenization, scope tracking, declaration normalization, and diagnostics must be centralized. New languages compose these primitives instead of cloning complete scanners.
6. **Small common IR.** The public model normalizes genuinely common concepts but preserves `nativeKind`, language identity, precise ranges, provider/analyzer identity, diagnostics, and evidence so language-specific meaning is not erased.
7. **Composite documents are first-class.** One physical file may contain a host format plus multiple language regions. Region mapping must preserve positions in the original decoded document. Classic ASP is the initial composite canary.
8. **Language detection is evidence-based and economical.** Explicit language, exact filename, compound suffix, extension candidates, shebang/interpreter, internal directives, content markers, project/path evidence, and finally bounded analyzer probes form an ordered cascade. Extensions narrow candidates but are not proof.
9. **Ambiguity is a valid result.** Detection must return exact/probable/ambiguous/unknown-equivalent evidence instead of silently choosing a language when available evidence does not justify the choice.
10. **R25 proved non-Go neutrality.** The completion canaries are Go, C#, VB.NET, Python, plus Classic ASP segmentation/delegation. These cover standard-library AST integration, brace/OOP syntax, Basic-family line/end scopes, indentation scopes, and mixed-language documents; broad catalog completion arrived in R27.
11. **R25 remains syntax/navigation focused.** Project-wide semantic references, implementations, call graphs, blast radius, persistent indexing, and broad language expansion belong to R27. R25 may expose only the local structural facts required to make its public symbol/navigation model coherent.
12. **Compact agent workflow.** One coherent `source_symbols` tool owns `outline`, `digest`, `find`, and `show` rather than multiplying near-duplicate MCP tools.
13. **No source duplication in indexes.** R27's later process-local generations retain fingerprints, symbols, ranges, dependencies, relations, diagnostics, and generation evidence; exact source bodies are re-read through Scripthold and fingerprint-verified on demand.
14. **Encoding remains Scripthold-authoritative.** Language detection must never be confused with encoding detection. Source bytes are decoded through the existing content/BOM trust path before native analyzers consume text unless an explicitly reviewed analyzer requires raw bytes.

## Evidence model

Evidence strength is independent from the internal implementation technique. The final vocabulary is:

- **textual** — a byte/text occurrence, including ordinary grep/regex;
- **lexical** — a token/pattern recognized after enough lexical processing to exclude irrelevant text such as comments/strings where promised;
- **structural** — declaration, scope, import, inheritance, or call-site syntax established by a language recognizer/parser;
- **scope-resolved** — an identifier related to a declaration through proven local/enclosing scope rules;
- **project-resolved** — a target selected through project symbol/import/dependency evidence without claiming complete compiler semantics;
- **semantic** — a relation established with sufficient language semantics/type/dispatch information to justify that label.

R25 primarily emits structural declaration evidence. R27 later added stronger levels only where project/analyzer evidence proves them. A unique name match in a repository is never automatically semantic.

## Native architecture

The target dependency flow is:

```text
raw source bytes
    -> existing Scripthold encoding/BOM authority
    -> SourceDocument + position map
    -> LanguageDetector + LanguageRegistry
    -> optional CompositeSegmenter
    -> native scanner/lexer
    -> language/family recognizer or structural parser
    -> normalized symbols/scopes/diagnostics
    -> `source_symbols` query operations
    -> R27 project resolver/query/index layers
```

The intended internal responsibilities are:

- `SourceDocument`: decoded source, encoding/BOM metadata, deterministic line map, decoded-coordinate mapping, and bounded access helpers;
- `LanguageRegistry`: canonical IDs, aliases, families, exact basenames, extensions/compound extensions, interpreters, detector hints, analyzer strategy, composite strategy, and declared capabilities;
- `LanguageDetector`: ordered evidence collection, candidate narrowing, ambiguity handling, and bounded analyzer probes only where necessary;
- shared scanner/lexer primitives: identifiers, keywords, punctuation, delimiters, comments, strings, escapes, newline significance, line continuation, optional indentation and preprocessing hooks;
- native analyzers: `ScannerRecognizer`, `TokenStructuralParser`, and language-specific/full structural parsers only where the simpler forms cannot meet the declared accuracy contract;
- `CompositeSegmenter`: host/embedded region extraction with original-document position preservation and delegation to registered analyzers;
- normalizer: common symbol kinds plus language-native distinctions, hierarchy, signatures, visibility/modifiers, diagnostics, evidence and coverage;
- orchestrator: authorized traversal, encoding, limits, cancellation, deterministic ordering, concurrency, partial errors and MCP mapping.

Public/provider abstractions do not expose implementation types from `go/ast` or language-specific analyzers.

## Language detection contract

Detection and parsing are separate responsibilities. The detector should perform the cheapest reliable checks first:

1. explicit canonical/alias language requested by the caller;
2. exact basename rules;
3. compound suffix rules;
4. ordinary extension to a deterministic candidate set;
5. shebang/interpreter for applicable files, especially extensionless sources;
6. language/modeline/directive evidence from bounded decoded content;
7. distinctive content markers;
8. bounded project/path evidence that can disambiguate but never grant filesystem authority;
9. parser/recognizer probes only for the remaining ambiguous candidates.

Ambiguous extensions such as `.h`, `.m`, `.inc`, `.bas`, and composite/template suffixes require dedicated tests. A path hint is evidence, not source-language proof. The registry is the single source of truth for language aliases/candidate routing; supported-language documentation and detection-table tests should be derived from it rather than maintained as separate lists.

## Scanner and recognizer policy

The shared scanner must be incremental/bounded and cancellation-aware. It should solve expensive lexical edge cases once: strings, comments, escapes, delimiters, line continuations, logical lines, optional indentation, and optional preprocessing spans. Language analyzers compose scanner profiles and focused recognizers instead of copying scanners wholesale.

Regular expressions are appropriate for bounded, well-defined tasks such as directives, labels, candidate discovery, fixed-format constructs, and recognizers operating on already validated code/logical-line spans. Go's standard `regexp` engine is acceptable under existing resource limits. Regex is not a license to match arbitrary source text and report semantic certainty.

A language analyzer should use the least complex strategy that passes its quality contract:

- scanner/recognizer for simple, line-oriented, fixed-format, legacy, or DSL constructs;
- token structural parser for nested declarations/scopes where token relationships matter;
- dedicated structural grammar/parser only for constructs that cannot be recognized reliably through shared primitives.

## Final R25 public contract

The final schema was reviewed across structurally different language families and requires no Go-specific public field. It remains authoritative unless maintainers explicitly revise the compatibility contract.

### Tool and operation shape

The public tool name is `source_symbols`. It remains one read-only tool with four strict `oneOf` operation branches, each with `additionalProperties: false`:

- `outline`: required `operation`, `paths`; optional `language`, `encoding`, `kinds`, `includes`, `excludes`, `respectGitignore`, `includeSignatures`, `maxSymbols`, `maxFiles`;
- `digest`: required `operation`, `paths`; optional `language`, `encoding`, `includes`, `excludes`, `respectGitignore`, `maxFiles`;
- `find`: required `operation`, `paths`, `query`; optional `match`, `language`, `encoding`, `kinds`, `includes`, `excludes`, `respectGitignore`, `includeSignatures`, `maxSymbols`, `maxFiles`; `match` is `exact`, `prefix`, or `qualified`;
- `show`: required `operation`, `path`, `symbolId`, `sourceFingerprint`, `language`, `encoding`; optional `maxBytes`.

`outline`, `digest`, and `find` are request-scoped over authorized paths. `show` is deliberately stateless: it re-reads the authoritative source, verifies the supplied content fingerprint, decodes with the selected canonical encoding, re-analyzes the file, and resolves the deterministic `symbolId`. A stale fingerprint is a conflict; R25 must not depend on a hidden process/session symbol cache. `show` returns one complete bounded selected region or a limit error rather than silently presenting a truncated body as exact source.

Path arrays accept at most 256 input paths. `kinds` accepts at most 32 entries. Include/exclude lists are independently bounded to 64 entries. `find.query` is bounded to 512 decoded Unicode scalar values. Explicit language and encoding names are canonicalized/validated rather than accepted as arbitrary analyzer identifiers.

### Coordinates, identity, symbols, and coverage

The public coordinate system identifier is `unicode-scalar-1-based-half-open`: line and column are 1-based in the decoded source document, columns count Unicode scalar values, and ranges use an exclusive end. Tabs count as one source scalar; no grapheme-width or display-cell promise is made. Raw original-file byte offsets are not public R25 coordinates.

A normalized symbol is a flat deterministic record with request-local hierarchy references rather than a recursively duplicated payload. It contains at least deterministic 64-lowercase-hex `id`, authorized `path`, canonical `language`, normalized `kind`, bounded `nativeKind`, `name`, optional `qualifiedName`, optional `parentId`/`parentQualifiedName`, optional composite `regionId`, `declarationRange`, `nameRange`, optional reliable `signatureRange`/`bodyRange`, optional bounded `signature`, optional visibility/modifiers, evidence, and analyzer identity. The ID must include enough normalized identity/range information to avoid collisions between overloads, nested declarations, and repeated names; it must not be a name-only hash.

Composite hosts keep the physical host `path` and host-document coordinates while using `regionId` and canonical embedded-language identity where applicable. This is sufficient for Classic ASP now and for later Razor/Vue-style segmentation without changing the public range model.

Every request reports deterministic file summaries, files considered/parsed/skipped, symbol count, truncation state, and aggregate `coverageComplete`. Each file summary carries source fingerprint, selected encoding, language/detection evidence, analyzer, bounded diagnostics, status, and file-level completeness. Ordinary per-file failures may coexist with useful results, but any skipped/failed/limited coverage forces the relevant completeness flag false.

`outline` and `find` return normalized symbol records and never return complete source bodies implicitly. `digest` returns bounded per-file structural summaries, declaration counts and structurally proven dependency/import facts without full bodies. `show` is the only R25 operation that returns the selected exact source region.

### Evidence and language detection

Symbol facts use the frozen evidence vocabulary `textual`, `lexical`, `structural`, `scope-resolved`, `project-resolved`, and `semantic`; R25 declarations are primarily `structural`. Stronger labels remain unavailable unless the analyzer actually proves them.

Language detection is a separate evidence system with result states `exact`, `probable`, `ambiguous`, and `unknown`. It does not expose fabricated percentage confidence. Candidate lists contain at most 16 entries and evidence lists at most 32 entries, with explicit truncation/omission evidence if those fixed output caps are reached. The ordered evidence kinds remain explicit request, exact basename, compound suffix, extension, shebang/interpreter, internal directive/modeline, content marker, project/path hint, and bounded analyzer probe. Explicit `language` is strong selection evidence but does not excuse analyzer validation of the source.

### Source-analysis resource limits

R25 adds one small `Config.Source` limit group rather than a second general budgeting subsystem. Where a source-specific budget overlaps an existing server-wide file/output budget, the effective value is the lower of the two. Source limits can therefore tighten general policy but can never bypass it.

| Limit / environment | Default | Hard maximum |
|---|---:|---:|
| `MaxInputPaths` / `MCP_SOURCE_MAX_INPUT_PATHS` | 32 | 256 |
| `MaxFiles` / `MCP_SOURCE_MAX_FILES` | 256 | 4,096 |
| `MaxAggregateBytes` / `MCP_SOURCE_MAX_AGGREGATE_BYTES` | 64 MiB | 512 MiB |
| `MaxFileBytes` / `MCP_SOURCE_MAX_FILE_BYTES` | 8 MiB | 64 MiB |
| `MaxSymbols` / `MCP_SOURCE_MAX_SYMBOLS` | 10,000 | 100,000 |
| `MaxSignatureBytes` / `MCP_SOURCE_MAX_SIGNATURE_BYTES` | 8 KiB | 64 KiB |
| `MaxShowBytes` / `MCP_SOURCE_MAX_SHOW_BYTES` | 1 MiB | 8 MiB |
| `MaxDiagnostics` / `MCP_SOURCE_MAX_DIAGNOSTICS` | 256 | 4,096 |
| `MaxDetectorProbes` / `MCP_SOURCE_MAX_DETECTOR_PROBES` | 4 | 16 |
| `MaxNesting` / `MCP_SOURCE_MAX_NESTING` | 256 | 2,048 |
| `MaxConcurrency` / `MCP_SOURCE_MAX_CONCURRENCY` | 4 | 32 |
| `MaxRequestSeconds` / `MCP_SOURCE_MAX_REQUEST_SECONDS` | 30 s | 300 s |
| `MaxOutputBytes` / `MCP_SOURCE_MAX_OUTPUT_BYTES` | 16 MiB | 64 MiB |

The existing bounded-environment policy applies: invalid, non-positive, overflowing, or above-hard-maximum values fall back to the documented default rather than weakening a compiled ceiling. Client request limits may only lower the configured effective ceiling. Cancellation may terminate earlier than `MaxRequestSeconds`.

### Cross-family compatibility review

- Go receivers, grouped declarations and generics fit normalized kinds plus hierarchy/ranges without exposing `go/ast` concepts.
- C# namespaces, records, constructors, properties/events, nested/generic types, partial and extension syntax fit `nativeKind`, modifiers and parent identity without type binding.
- VB.NET modules, case-insensitive names, `Sub`/`Function`/`New`, explicit `End` scopes and line continuations fit the same model; matching semantics are analyzer/language aware rather than globally case-sensitive.
- Python modules/classes/functions/async/decorators and indentation-defined nested ownership fit parent/range evidence without brace assumptions.
- Classic ASP requires multiple language regions in one physical file, which is covered by `regionId` plus host-document coordinates.
- C/C++ overloads, operators/destructors and templates require collision-safe IDs and `nativeKind`, not semantic type resolution.
- JavaScript/TypeScript functions/classes/interfaces/type aliases/namespaces and overload-like declarations fit the extensible normalized/native kind split.
- Rust modules, traits, impl-associated items and methods fit hierarchy/native kinds without forcing implementation blocks into a universal semantic type.
- Pascal/Delphi units, classes/records, procedures/functions, constructors/destructors/properties and forward/implementation forms fit ranges, modifiers and native kinds.
- MQL4 and MQL5 use separate canonical language IDs even though their declaration surface is C-like.
- Razor and Vue require the same first-class composite-region mechanism as Classic ASP, not a fake single-language parse.

### R25 supported-language capability matrix

The registry is the canonical capability source. Independent provider/capability conformance and drift tests verify that registered source-analysis capabilities resolve to matching analyzers and cannot become active through metadata alone.

| Language/form | R25 strategy | Declaration/navigation coverage | Strongest R25 evidence |
|---|---|---|---|
| Go | Standard-library `go/parser` / `go/ast` / `go/token` | Packages, imports, grouped constants/variables, named types/aliases, structs/interfaces/members, generics, functions and receiver-associated methods | `structural` |
| C# | Shared scanner + focused brace/OOP recognizer | Namespaces, classes/structs/interfaces/records/enums, nested/generic types, constructors/destructors, methods, properties/indexers, events, fields/constants, using directives, attributes/modifiers, partial/extension and expression-bodied syntax | `structural` |
| VB.NET | Shared case-insensitive scanner + logical statements + explicit-`End` scopes | Namespace/module/type declarations, `Sub`/`Function`/constructors, properties/events including custom events, fields/constants, escaped identifiers, declaration modifiers, `Declare` callables, continuation/colon statements, `Imports`, `Inherits`/`Implements` | `structural` |
| Python | Shared scanner + indentation ownership | Classes, functions/async functions, methods/nested definitions, decorators, multiline signatures and imports | `structural` |
| Classic ASP | Host/embedded segmenter + bounded VBScript-family delegation | Host/directive/server/expression regions, server-side script blocks, include dependencies and VBScript-like declarations with host coordinates; unsupported JScript remains explicit | `structural` |

R25 intentionally does not claim project-wide type binding, references, implementations, dispatch/call resolution, or semantic compilation for these canaries. R27 later added only the project/query capabilities supported by stronger evidence while preserving these R25 limits.

## Completion record

R25 completed on 2026-08-13 with the final `source_symbols` contract, five production canaries (Go, C#, VB.NET, Python, and Classic ASP), decoded-coordinate/encoding conformance, deterministic and malformed-input coverage, bounded resource tests, direct/HTTP/stdio MCP acceptance, complete normal/race/static/vulnerability checks, and supported-target compilation. A separate non-vendored real-world corpus then exercised at least eight independent public origins per canary and drove focused hardening without changing the public schema.

R27 subsequently extended this foundation to the broad provider catalog, project relations, structural search, bounded context, and process-local index generations while preserving the R25 `source_symbols` contract. Detailed release history belongs in `CHANGELOG.md` and `ROADMAP_HISTORY.md` rather than in this contract.
## Required tests

### Shared schema/orchestration

- strict unknown-field rejection;
- empty/missing paths;
- deterministic file/symbol ordering;
- symbol-kind filters;
- signature inclusion/exclusion;
- result truncation and `coverageComplete` behavior;
- output/file/symbol/byte limits;
- cancellation;
- stdio/HTTP equivalence;
- read-only MCP annotations and mutation-negative tests.

### Path and encoding security

- escaping symlinks/junctions/reparse points;
- aliases and missing/inaccessible paths;
- UTF-8/BOM cases;
- representative UTF-16/legacy cases where the provider path supports decoded text;
- ambiguous and malformed encodings;
- very long lines and bounded position/signature handling;
- same content under misleading/unrelated filenames to prove that extension alone is not authoritative.

### Go provider

- packages;
- functions and methods;
- pointer/value receivers;
- structs and embedded fields;
- interfaces;
- aliases and named types;
- grouped const/var declarations;
- generic type/function parameters;
- nested/anonymous syntax where relevant to the normalized model;
- build tags/comments that should not become symbols;
- comments/strings containing declaration-like text;
- malformed/truncated source with bounded diagnostics;
- large generated source under configured limits;
- deterministic signatures and ranges.

### Resource and quality

- bounded allocation as source count/size grows;
- fuzz provider parsing/normalization boundaries where practical;
- race tests for caches/concurrency if introduced;
- no source mutation before/after requests;
- no external command/network activity;
- connector smoke demonstrating materially less full-file reading for source navigation.

## Devil's advocate findings

### Risk: the common schema becomes Go-shaped

Mitigation: normalized concepts are language-neutral and provider-specific capabilities are explicit. R27's required language families are documented before R25 implementation so the provider interface must be reviewed against non-Go constructs such as classes, namespaces, traits, properties, overloads, modules, and Pascal units.

### Risk: lexical/regex recognizers produce plausible but overclaimed symbols

Mitigation: textual and lexical matches remain labeled at their actual evidence level. Regex may participate in a structural recognizer only after the language-specific lexical/scope validation required by that analyzer and only when corpus tests demonstrate the advertised declaration accuracy. Unsupported constructs remain explicit.

### Risk: parsing entire repositories consumes too much memory

Mitigation: secure bounded traversal, per-file parsing, bounded concurrency, symbol/signature caps, independent output limits, and no unbounded retained AST/project index.

### Risk: filename-based language selection conflicts with Scripthold's content-evidence philosophy

Mitigation: extension/path naming can only choose parser candidates. Successful provider parsing/validation is required before claiming coverage, and ambiguous cases remain explicit.

### Risk: syntax symbols are mistaken for semantic resolution

Mitigation: every provider declares capability level. R25 results describe declarations/source structure; references, implementations, resolved calls, and project semantics are reserved for providers that can establish them accurately, principally in R27.

## Completion gate

R25 is complete only when:

1. the language-neutral symbol schema, evidence model, registry, detector, shared scanner and analyzer interfaces are documented and reviewed against the complete R27 target catalog;
2. one compact read-only `source_symbols`-class capability implements the approved outline/digest/find/show-equivalent workflow with strict bounds and explicit partial coverage;
3. Go, C#, VB.NET and Python implement the approved production declaration/navigation canary bar through the same common model, and Classic ASP proves host/embedded segmentation/delegation with original-document range mapping;
4. language selection follows the approved evidence cascade and does not treat extension alone as authoritative proof;
5. encoding, coordinate mapping, allowed-root, traversal, cancellation, output, concurrency and transport invariants remain intact;
6. regex/grep use is evidence-qualified: raw lexical matches are never promoted to structural or semantic facts without the validation promised by the analyzer;
7. deterministic, malformed-input, negative comment/string, range, encoding, fuzz/resource, mutation-negative, performance and connector smoke tests pass for every R25 canary;
8. the implementation leaves the documented native registry/scanner/composite/provider extension points required by R27 without a public-schema rewrite for the approved broad catalog.

## Relationship to R27

R25 proved the common model across structurally different canaries. R27 subsequently delivered the broad native language/source-format catalog, supported project relationships, structural search, bounded context, and incremental indexing without breaking the `source_symbols` contract.
