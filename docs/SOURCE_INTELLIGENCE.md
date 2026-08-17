# Source Intelligence Foundation Design

## Status

**COMPLETE — R25.** The implementation and completion verification finished on 2026-08-13. Repository delivery was finalized at `1c558a8cf37c634f6ef4b8a7e3af2d6879c33526`, whose exact push-event CI and aggregate release-candidate gate passed. This document is the completed contract and verification record for the native language-neutral source-navigation foundation. R26 subsequently completed as a separate milestone, and R27 completed on 2026-08-16. Neither later status change alters the frozen R25 contract.

R25 establishes a language-neutral public model and provider architecture implemented natively in Scripthold. Go's standard-library parser is the first reference implementation because it is already available without adding a parser dependency, but **Go is not the final product scope**. R25 must also prove that the shared model is not Go-shaped by exercising several structurally different language families before completion. Broad multi-language coverage is a mandatory R27 outcome defined separately in [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md).

The approved implementation direction is intentionally dependency-light and native: source intelligence must be built from Scripthold-owned Go code and existing standard-library facilities rather than by embedding Tree-sitter, Babel, OpenRewrite, language-server runtimes, compiler frontends, downloaded grammars, or equivalent third-party parsing engines. External projects may be studied for algorithms, workflows, feature ideas, and acceptance criteria, but are not implementation dependencies.

## Problem

The current Scripthold surface is strong at filesystem navigation, grep, fingerprints, verified mutation, encoding preservation, and durable execution, but it has no structured understanding of source-code symbols.

An agent that asks questions such as:

- "What functions and types are defined in this file?"
- "Show me the class/module hierarchy."
- "Where is this method declared?"
- "Give me the signatures in this package without reading every file in full."

must currently rely on text search or load substantial source text. Grep can find lexical patterns but cannot reliably distinguish declarations, ownership, nested scopes, overloaded names, comments/strings, or language grammar.

R25 introduces **symbol extraction / source indexing** as a bounded read-only capability. AST parsing is an implementation technique; the public product concept is a language-neutral symbol model.

## Goals

R25 will:

- expose a read-only source-symbol operation suitable for AI/agent navigation;
- return structured declarations rather than unqualified raw lexical matches;
- represent functions, methods, types/classes, interfaces/traits where applicable, fields/properties, constants, variables/globals, constructors, namespaces/modules/packages, and other provider-supported declaration kinds;
- preserve hierarchical ownership (`parent`, qualified name, nested symbols where appropriate);
- return source positions and bounded signatures without returning complete source files by default;
- operate on one file, a bounded list of files, or bounded directory roots;
- reuse Scripthold's secure deterministic traversal and allowed-root boundary;
- make language selection explicit and trustworthy through an evidence cascade rather than treating a filename alone as authoritative;
- establish a provider interface that permits multiple native analysis strategies later while preserving one public schema, including lexical scanners, token-aware recognizers, structural parsers, composite-document segmenters, and project resolvers;
- use Go's `go/parser`, `go/ast`, and `go/token` as the first production provider while also proving the common architecture against a brace-oriented language, a Basic-family language, and an indentation-oriented language before R25 completion;
- report unsupported languages and unsupported semantic capabilities explicitly;
- remain bounded in files, bytes, symbols, parse diagnostics, retained signatures, output size, time, and memory;
- preserve stdio/Streamable HTTP equivalence;
- provide the stable foundation required by R27 for broad multi-language code intelligence.

## Non-goals

R25 will not:

- promote raw grep/regular-expression matches to syntactic or semantic facts without the validation required by the advertised evidence level;
- promise broad multi-language coverage by itself;
- implement project-wide reference finding, implementations, call graphs, or dependency graphs as the primary R25 deliverable;
- run arbitrary compilers, build systems, shell commands, or language servers from a read-only symbol request;
- mutate source files;
- automatically refactor/rename symbols;
- claim semantic type resolution where the provider performed syntax-only parsing;
- download parser grammars, dependencies, language servers, or indexes at request time;
- trust file extensions as proof of source encoding or language when content/provider validation contradicts them;
- retain an unbounded daemon-wide project index in the first implementation.

## Public capability

The conceptual public operation is `source_symbols`. The final name may change during the activation compatibility review, but the capability is approved.

A representative request model is:

```json
{
  "paths": ["/project"],
  "language": "go",
  "kinds": ["function", "method", "type"],
  "includeSignatures": true,
  "maxSymbols": 2000
}
```

The exact schema must remain strict, bounded, and language-neutral.

Expected request dimensions include:

- `paths`: one or more authorized files/directories;
- optional `language`: explicit canonical provider language when known;
- optional `kinds`: declaration kinds to retain;
- optional include/exclude path filters consistent with secure traversal;
- optional `includeSignatures`;
- optional nesting/detail controls;
- bounded `maxSymbols`/result limit;
- explicit `.gitignore` policy for directory traversal consistent with existing recursive tools.

Unknown fields and unsupported values are rejected.

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

R27 may refine language detection using project manifests/shebangs/provider evidence, but it must preserve explicit uncertainty rather than guess silently.

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

R25 must define a narrow internal provider interface rather than embedding Go-specific behavior into the handler.

Conceptually a provider is responsible for:

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

The first provider will use the Go standard library:

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

The initial implementation should prefer request-scoped or bounded cacheable parsing. If a process-local cache is introduced, it must define:

- cache key/fingerprint;
- memory and entry bounds;
- invalidation on content change;
- allowed-root changes where applicable;
- restart semantics;
- no persistence of source contents unless separately approved.

A durable incremental index belongs primarily to R27.

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

R25 must publish explicit hard limits before activation.

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

The following decisions were explicitly approved before R25 activation and are part of the handoff contract. An implementation session must not silently reopen them. A maintainer may revise them explicitly, in which case this document and the roadmap must be updated before implementation diverges.

1. **Native implementation.** R25/R27 source intelligence is implemented in Scripthold-owned Go code. Do not add Tree-sitter, Babel, OpenRewrite, parser-runtime bindings, downloaded grammars, language servers, compiler frontends, or equivalent third-party parsing engines as the source-intelligence foundation.
2. **External projects are design references only.** Their useful ideas include compact outline/digest/show workflows, structural search, evidence-qualified relations, bounded context assembly, incremental indexing, language registries, composite-document delegation, and capability matrices. Their code/runtime dependencies are not imported merely to obtain those features.
3. **Regex and grep are allowed primitives.** Raw textual matches remain lexical evidence. Regex over validated code spans, token streams, or scope-aware recognizers may contribute to structural results when tests establish that capability. The implementation must never label evidence more strongly than it proves.
4. **No universal compiler is required.** The shared engine should recognize the source structure required for navigation and indexing rather than implement complete evaluation/type semantics for every language.
5. **Shared infrastructure before language proliferation.** Reusable decoding/position mapping, language detection, scanning, tokenization, scope tracking, declaration normalization, and diagnostics must be centralized. New languages compose these primitives instead of cloning complete scanners.
6. **Small common IR.** The public model normalizes genuinely common concepts but preserves `nativeKind`, language identity, precise ranges, provider/analyzer identity, diagnostics, and evidence so language-specific meaning is not erased.
7. **Composite documents are first-class.** One physical file may contain a host format plus multiple language regions. Region mapping must preserve positions in the original decoded document. Classic ASP is the initial composite canary.
8. **Language detection is evidence-based and economical.** Explicit language, exact filename, compound suffix, extension candidates, shebang/interpreter, internal directives, content markers, project/path evidence, and finally bounded analyzer probes form an ordered cascade. Extensions narrow candidates but are not proof.
9. **Ambiguity is a valid result.** Detection must return exact/probable/ambiguous/unknown-equivalent evidence instead of silently choosing a language when available evidence does not justify the choice.
10. **R25 must prove non-Go neutrality.** The completion canaries are Go, C#, VB.NET, Python, plus Classic ASP segmentation/delegation. These cover standard-library AST integration, brace/OOP syntax, Basic-family line/end scopes, indentation scopes, and mixed-language documents. R25 is not broad R27 language completion.
11. **R25 remains syntax/navigation focused.** Project-wide semantic references, implementations, call graphs, blast radius, persistent indexing, and broad language expansion belong to R27. R25 may expose only the local structural facts required to make its public symbol/navigation model coherent.
12. **Compact agent workflow.** Prefer one coherent `source_symbols`-class tool with operations equivalent to `outline`, `digest`, `find`, and `show` rather than multiplying near-duplicate MCP tools. Final schema names remain subject to the Phase 1 compatibility review, but tool-count reduction is the default.
13. **No source duplication in future indexes.** R27 should index fingerprints, symbols, ranges, dependencies, relations, diagnostics, and generations; exact source bodies should normally be re-read through Scripthold and fingerprint-verified on demand.
14. **Encoding remains Scripthold-authoritative.** Language detection must never be confused with encoding detection. Source bytes are decoded through the existing content/BOM trust path before native analyzers consume text unless an explicitly reviewed analyzer requires raw bytes.

## Evidence model

R25 must define evidence strength independently from the internal implementation technique. The final names may be refined during Phase 1, but the distinctions are mandatory:

- **textual** — a byte/text occurrence, including ordinary grep/regex;
- **lexical** — a token/pattern recognized after enough lexical processing to exclude irrelevant text such as comments/strings where promised;
- **structural** — declaration, scope, import, inheritance, or call-site syntax established by a language recognizer/parser;
- **scope-resolved** — an identifier related to a declaration through proven local/enclosing scope rules;
- **project-resolved** — a target selected through project symbol/import/dependency evidence without claiming complete compiler semantics;
- **semantic** — a relation established with sufficient language semantics/type/dispatch information to justify that label.

R25 primarily emits structural declaration evidence. R27 may add the stronger levels. A unique name match in a repository is never automatically semantic.

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
    -> R25 query operations
    -> R27 project resolver/index/graphs later
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

The implementation may choose package names after inspecting repository conventions. Public/provider abstractions must not expose implementation types from `go/ast` or any future language analyzer.

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

## Public source-navigation model

Phase 1 must freeze a compact model capable of powering four agent-oriented operations without duplicating full file text:

- `outline`: deterministic symbols/signatures/ranges and hierarchy for one or more paths;
- `digest`: bounded module/file summary containing language, declarations, imports/dependencies where already structurally known, counts, coverage and estimated source footprint without full bodies;
- `find`: exact/prefix/name/qualified-name lookup over the current request scope, not an R27 persistent project index;
- `show`: return the exact bounded source region for a selected symbol using current source/fingerprint evidence rather than embedding all bodies in the outline.

The final public tool may express these as an `operation` field on `source_symbols` or another smaller coherent schema chosen in Phase 1. Do not create four public tools without a demonstrated schema/security reason.

The normalized symbol representation must cover at least:

- deterministic request-local `id` or equivalent disambiguator;
- `path` and canonical `language`;
- normalized `kind` and bounded language-specific `nativeKind`;
- `name`, `qualifiedName`, and parent identity/name where reliable;
- `declarationRange` and `nameRange`;
- optional `signatureRange` and `bodyRange` when reliably established;
- bounded signature text when requested;
- visibility/modifiers where syntactically reliable;
- evidence/analyzer identity and bounded diagnostics;
- explicit completeness/truncation state.

Public byte offsets are forbidden unless their coordinate domain is explicitly defined and valid after Scripthold decoding. Line/column positions must remain correct for UTF-8, UTF-16, UTF-32, and supported legacy encodings that reach an analyzer.

## Frozen R25 Phase 1 public contract

Phase 1 was completed on 2026-08-13 after explicit review against Go, C#, VB.NET, Python, Classic ASP, C/C++, JavaScript/TypeScript, Rust, Pascal/Delphi, MQL4/MQL5, Razor, and Vue constructs. The review found no need for a Go-shaped public field. R25 implementation must preserve the contract below unless the maintainer explicitly reopens Phase 1 and updates both the tests and this document.

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

The registry is the canonical capability source; `TestR25AnalyzerRegistryCoverageIsMechanicallyConsistent` verifies that every registered `SourceAnalysis` capability resolves to the matching analyzer and that metadata-only future entries cannot activate R27 behavior.

| Language/form | R25 strategy | Declaration/navigation coverage | Strongest R25 evidence |
|---|---|---|---|
| Go | Standard-library `go/parser` / `go/ast` / `go/token` | Packages, imports, grouped constants/variables, named types/aliases, structs/interfaces/members, generics, functions and receiver-associated methods | `structural` |
| C# | Shared scanner + focused brace/OOP recognizer | Namespaces, classes/structs/interfaces/records/enums, nested/generic types, constructors/destructors, methods, properties/indexers, events, fields/constants, using directives, attributes/modifiers, partial/extension and expression-bodied syntax | `structural` |
| VB.NET | Shared case-insensitive scanner + logical statements + explicit-`End` scopes | Namespace/module/type declarations, `Sub`/`Function`/constructors, properties/events including custom events, fields/constants, escaped identifiers, declaration modifiers, `Declare` callables, continuation/colon statements, `Imports`, `Inherits`/`Implements` | `structural` |
| Python | Shared scanner + indentation ownership | Classes, functions/async functions, methods/nested definitions, decorators, multiline signatures and imports | `structural` |
| Classic ASP | Host/embedded segmenter + bounded VBScript-family delegation | Host/directive/server/expression regions, server-side script blocks, include dependencies and VBScript-like declarations with host coordinates; unsupported JScript remains explicit | `structural` |

R25 intentionally does not claim project-wide type binding, references, implementations, dispatch/call resolution, or semantic compilation for these canaries. Those capabilities remain R27 work.

### Implementation progress

- Phase 0: **COMPLETE** — activation/context/module mapping and pre-change baselines passed on 2026-08-13.
- Phase 1: **COMPLETE** — contract, evidence model, cross-family review and limits are frozen above; focused RED tests compile and fail only because `Config.Source` and `source_symbols` implementation are intentionally absent at this stage.
- Phase 2: **COMPLETE** — shared decoded-file streaming now reuses one digesting `ReadSession`, `SourceDocument` maps UTF-8/internal offsets to 1-based Unicode-scalar half-open ranges across UTF-8/16/32/legacy encodings and CR/LF/CRLF mixtures, and bounded source slices/fingerprints/cancellation are covered by focused tests; full handler regressions remained green after extracting the shared decoder.
- Phase 3: **COMPLETE** — the validated native registry routes the five R25 canaries while representing future families as inactive metadata only; the evidence detector covers explicit names/aliases, basenames, compound suffixes, shebangs, directives, content disagreement, ambiguity classes, project hints, bounded probes, spoofed inputs, deterministic ordering and cancellation without consulting encoding state.
- Phase 4: **COMPLETE** — one profile-driven state-machine scanner now covers shared identifiers/keywords, delimiter tracking, line/block/nestable comments, C# raw/verbatim/interpolated strings, VB.NET strings/continuations, Python triple/raw/f-string families, logical lines, indentation, directives, bounded tokens/nesting, diagnostics, determinism and cancellation; focused tests, full package tests and a short real fuzz run are green.
- Phase 5: **COMPLETE** — one common flat symbol builder now owns normalized/native kinds, deterministic collision-resistant IDs, lexical and explicit parent ownership, qualified names, decoded-source declaration/name/signature/body ranges, visibility/modifiers, structural-evidence ceilings, bounded signatures/diagnostics/symbols, coverage/truncation state, deterministic ordering, and brace/explicit-`End`/indent scope adapters; rejected work is transactional and focused plus full-package tests are green.
- Phase 6: **COMPLETE** — the Go reference analyzer uses only standard-library AST/token packages, recovers partial parser output with bounded diagnostics, normalizes grouped declarations/types/members/generics/functions/receiver methods through the common builder, and passes focused plus full-package regressions.
- Phase 7: **COMPLETE** — the native C# brace/OOP canary reuses the shared scanner and covers namespace/type/member/generic/nesting/partial/extension/expression-body cases while excluding declaration-like comments/strings; focused and full-package tests are green.
- Phase 8: **COMPLETE** — the VB.NET canary reuses case-insensitive scanner/logical-line primitives, explicit `End` ownership, continuation/colon statements, declaration members and structural `Imports`/`Inherits`/`Implements`; focused and full-package tests are green.
- Phase 9: **COMPLETE** — the Python canary uses shared indentation tokens for class/function/method/nested ownership, decorators, async/multiline declarations and import facts while keeping triple strings opaque; focused and full-package tests are green.
- Phase 10: **COMPLETE** — Classic ASP now segments host/directive/server/expression/server-script regions, preserves physical host coordinates, delegates supported VBScript-like regions, reports JScript as unsupported, and extracts include facts; focused and full-package tests are green.
- Phase 11: **COMPLETE** — request-scoped `outline`/`digest`/`find`/fingerprint-bound `show` orchestration composes existing authorization, walker, decoding, detection, analyzers and ordered concurrency; direct handler tests plus full handler/source-intelligence regressions are green.
- Phase 12: **COMPLETE** — the hand-authored conformance corpus covers all five canaries across UTF-8/UTF-16/UTF-32/legacy encodings, Unicode, false-positive negatives, malformed/partial inputs, deterministic repetition, generated-source limits, and mechanically checked registry/analyzer consistency; full source-intelligence regressions are green.
- Phase 13: **COMPLETE** — local Windows/amd64 benchmarks cover every canary, a 5,000-function generated Go source, `outline`/`digest`/`find`/`show`, and 80 mixed small files; serial versus four-worker output is identical. Measured evidence did not justify adding a cache, so the simpler request-scoped design and frozen defaults remain.
- Phase 14: **COMPLETE** — the authoritative catalog exposes one strict read-only `source_symbols` `oneOf` schema, runtime registration and documentation agree on the 35-tool pre-release `3.0.0` surface present at R25 completion, direct MCP and HTTP-equivalence R25 contracts are green, and a temporary source stdio server launched through `go run` passed external MCP discovery plus a real `source_symbols outline` call without touching the active R24 runtime.
- Phase 15: **COMPLETE** — the complete R25 diff and public model were reviewed after all focused gates; `go mod verify`, full normal and race suites, `go vet`, Staticcheck, govulncheck, deterministic scanner fuzzing, source MCP smoke, catalog/schema/transport checks, and compile-only builds for Windows/Linux/macOS on amd64/arm64 all passed. The explicit R27 compatibility review found no public-schema redesign requirement: the frozen normalized/native kind split, hierarchy, decoded ranges, composite `regionId`, evidence ladder, dependency/relation records and request-scoped navigation model remain extensible for the approved R27 catalog. R26 and R27 were out of scope for R25 implementation.

### Completion verification record

The final local completion gate on 2026-08-13 preserved the strict 35-tool pre-release `3.0.0` catalog and the four-branch `source_symbols` input contract. The complete normal suite and complete CGO race suite passed; Staticcheck reported no findings and govulncheck reported no known vulnerabilities. The final post-hardening scanner fuzz gate completed 3,013 executions, and compile-only `CGO_ENABLED=0` builds succeeded for Windows, Linux, and macOS on both amd64 and arm64 without producing a deployment candidate.

MCP acceptance covered direct/server construction, Streamable HTTP equivalence, and an external stdio process launched temporarily from source. The stdio smoke discovered the 35-tool catalog and executed a real `source_symbols outline` request against a temporary Go source file. No release, tag, commit, push, candidate activation, launcher change, tunnel restart, or active R24 runtime change was part of R25 completion.

### Post-completion real-world hardening

Before R25 delivery, a separate real-world acceptance pass raised the quality bar to at least eight independent public source origins for every R25 canary. The private, non-vendored acceptance corpus pinned 41 public upstreams: 8 Go, 8 C#, 8 VB.NET, 8 Python, and 9 Classic ASP origins. It contained 8,945 selected source files totaling 62,238,585 source bytes. The external projects remained read-only test inputs and were never runtime/build dependencies.

Every selected public source was automatically language-detected and analyzed twice on the final R25 hardening tree. The gate retained 125,607 symbols and verified deterministic output, unique per-file symbol IDs, valid parent IDs, source-bounded declaration/name ranges, signature/body containment, and stable ordering. Go reached 504/504 complete files, C# 3,550/3,550, Python 1,933/1,933, VB.NET 2,942 complete files plus five truthful partial fixtures out of 2,947, and Classic ASP 10 complete pages plus one deliberately unsupported server-side JScript page out of 11. The VB.NET partials were one physically inconsistent inactive conditional-compilation branch and four intentionally malformed compiler/analyzer fixtures; no supported valid source remained unexpectedly partial.

The real-source pass drove focused RED-to-GREEN regressions for corroborated language-detection precedence, VB.NET multiline strings, escaped identifiers, declaration modifiers, bodyless and `Declare` callables, custom events and multiline-lambda `End` pairing; C# attributed declarations, indexers, destructors and multiline interpolation expressions; Python relative-import levels; and the public `find.maxSymbols` contract. `find.maxSymbols` now limits retained matches without prematurely truncating the bounded per-file declaration analysis required to discover later symbols. Three conservatively ambiguous decodes were verified explicitly: two UTF-8 files and one Windows-1252 VB.NET source.

Public runtime acceptance on the final local R25 candidate exercised recursive traversal with `find(maxSymbols=1)` on new C#, VB.NET, Python and Classic ASP upstreams, a 36-file Go `digest`, and fingerprint-bound `find -> show` round trips on new Go, C#, VB.NET, Python and Classic ASP sources. The complete repository gate then passed normal and CGO race suites, `go vet`, Staticcheck, govulncheck, 3,013 scanner fuzz executions, all standalone Node release-script tests, Gitleaks, six CGO0 cross-builds, and corrected current-source external stdio MCP smoke. These checks supplement rather than replace the tracked conformance corpus and normal repository tests.

### Repository delivery verification

The first pushed delivery run exposed three portability assumptions in test/smoke harnesses rather than production source-intelligence behavior: the R25 contract test compared non-canonical temporary paths on Windows/macOS; the container stdio smoke passed a host path where the child process saw the bind at `/data`; and the native macOS stdio smoke constructed a source path through `/var` after the server had canonicalized the same root to `/private/var`. The fixes were confined to test code in `a6d3804a3e176a870c60e5126b6fd0e99d7433fb` and `1c558a8cf37c634f6ef4b8a7e3af2d6879c33526`; no production source-symbol implementation changed.

The exact final push-event CI run for `1c558a8cf37c634f6ef4b8a7e3af2d6879c33526` passed Windows, Ubuntu and macOS native tests/smoke, container smoke, deterministic fuzz, static/workflow analysis, module verification, all six Windows/Linux/macOS amd64/arm64 builds, and the aggregate `Release candidate` job. No tag or public release was created, and the test-only delivery hardening did not require another runtime candidate build.

## Sequential TDD implementation plan

Every phase is mandatory and sequential. A new implementation chat must read the required project context, inspect the current repository state, identify the first phase whose exit criterion is not yet satisfied, and continue from there. Do not skip a failing earlier phase to implement a later feature. Each phase follows focused TDD where practical: add/reproduce the focused failing test, prove it fails for the intended reason, implement the smallest coherent behavior, rerun focused tests, then run directly affected regressions.

### Phase 0 — activation, context, and baseline

When the maintainer explicitly activates R25:

1. read the root and every applicable scoped `AGENTS.md`, this document, `MULTILANGUAGE_CODE_INTELLIGENCE.md`, `ROADMAP.md`, `DEVELOPMENT_CHECKLIST.md`, encoding/security/traversal source-of-truth documents, and the private operator backlog;
2. confirm R24 remains complete, R26/R27 are not accidentally activated, and update only R25 to `ACTIVE` if activation was explicitly authorized;
3. record branch, `HEAD`, `origin/main`, working-tree state and unrelated user changes before editing;
4. inspect current handler/server/catalog patterns, configuration limits, secure walker, encoding APIs, concurrency primitives and tests;
5. establish baseline focused tests for encoding/traversal/catalog behavior that R25 will reuse;
6. do not change release/tag/runtime/launcher/deployment state as part of R25 source implementation.

Exit criterion: repository/module map and clean preservation plan are documented in-session; applicable baseline tests pass or pre-existing failures are explicitly isolated before R25 code changes.

### Phase 1 — freeze public contract and resource limits

Before implementation, add failing tests that define:

- the final `source_symbols`-class tool name and compact operation schema;
- `outline`, `digest`, `find`, and `show` behavior or the explicitly approved equivalent;
- strict unknown-field rejection and operation-specific field legality;
- normalized symbol/range/evidence/coverage schemas;
- per-file diagnostics and partial-coverage representation;
- language detection result states and candidate/evidence output bounds;
- read-only MCP annotations and stdio/HTTP schema equivalence;
- hard/default limits for input paths, files, aggregate source bytes, per-file bytes, symbols, signature/body return bytes, diagnostics, detector probes, nesting, concurrency, time/cancellation and aggregate output.

Review the proposed public model explicitly against Go, C#, VB.NET, Python, Classic ASP, C/C++, JavaScript/TypeScript, Rust, Pascal/Delphi, MQL4/MQL5, Razor and Vue constructs before freezing it. Do not implement analyzers until this review shows that no public field assumes Go-specific concepts.

Exit criterion: the public contract, evidence vocabulary and limits are test-defined; tests fail only because the implementation is absent.

### Phase 2 — `SourceDocument` and coordinate correctness

Build the shared decoded source abstraction on the existing encoding subsystem. It must:

- preserve the authoritative detected/explicit encoding and BOM evidence;
- expose decoded text under current file-size/decoded-size bounds;
- construct deterministic line-start mappings without unbounded duplicate storage;
- map analyzer offsets/ranges to the public line/column contract;
- distinguish decoded UTF-8/internal offsets from original-file byte offsets;
- support CRLF, LF, CR and mixed endings without changing source;
- expose bounded slices for signatures/body retrieval;
- honor cancellation during large inputs.

Focused tests must cover UTF-8, UTF-8 BOM, UTF-16 LE/BE, UTF-32 LE/BE, representative legacy encodings, Unicode identifiers, combining/multibyte text, very long lines, mixed endings, malformed/ambiguous encoding and range boundaries.

Exit criterion: analyzers can consume one safe decoded coordinate space and return ranges that remain correct independent of original encoding.

### Phase 3 — language registry and detector

Implement one registry as the canonical routing/capability table. Add the initial R25 canaries plus enough future R27 entries to prove that the registry model supports broad families without implementing them yet. Registry validation tests must reject duplicate canonical IDs/aliases, conflicting exact basenames, accidental duplicate extensions without explicit ambiguity, invalid analyzer references and inconsistent capability claims.

Implement the ordered detector cascade. Detector tests must cover:

- explicit language and aliases;
- exact basename and compound suffix;
- extensionless shebangs;
- misleading extension/content disagreement;
- `.h`, `.m`, `.inc`, `.bas` ambiguity classes;
- Classic ASP/page directives;
- malformed/spoofed shebang/directive inputs;
- project/path hints as non-authoritative evidence;
- ambiguous and unknown outputs;
- deterministic evidence ordering;
- bounded content probing and cancellation.

Exit criterion: language routing is deterministic, ambiguity-safe, inexpensive on unambiguous files, and independent from encoding selection.

### Phase 4 — shared scanner/lexer primitives

Build the native scanner core before the non-Go structural analyzers. The core must support profile-controlled:

- identifiers/keywords and case-sensitive/case-insensitive matching;
- punctuation/operators and balanced delimiter tracking;
- line/block/nestable comments where enabled;
- normal/raw/verbatim/triple/interpolated string families required by the initial canaries;
- escape handling;
- physical versus logical lines and continuation rules;
- optional indentation tokens;
- optional directive/preprocessor spans;
- bounded token/text retention and cancellation.

Do not turn the scanner profile into a giant language-specific switch. Shared behavior belongs in reusable primitives; language-specific behavior stays behind profile/analyzer interfaces.

Test comments/strings containing declaration-looking text, unterminated literals/comments, pathological nesting, huge logical lines, Unicode identifiers where permitted, repeated scanning determinism, cancellation and fuzz boundaries.

Exit criterion: C#, VB.NET and Python analyzers can share the scanner infrastructure without copying its core lexical logic.

### Phase 5 — common symbol builder, scopes, and diagnostics

Implement reusable language-neutral construction for:

- normalized/native kinds;
- parent/child scopes;
- qualified names;
- deterministic IDs/disambiguators suitable for request-local overloaded symbols;
- declaration/name/signature/body ranges;
- visibility/modifiers;
- bounded diagnostics;
- structural evidence and `coverageComplete` logic.

Add generic scope-stack tests for braces, explicit `End` scopes and indentation adapters without yet claiming language-specific correctness. Ensure overloads and nested symbols do not collide merely by name.

Exit criterion: language analyzers emit through one normalized builder and cannot bypass coverage/evidence rules.

### Phase 6 — Go reference analyzer

Implement the Go analyzer using only standard-library `go/parser`, `go/ast` and `go/token` behind the common analyzer interface. Cover package, imports where structurally useful, grouped const/var, named types/aliases, structs/interfaces/members, functions, methods/receivers, generics, signatures and reliable ranges. Preserve recoverable parser diagnostics and partial coverage.

Do not expose `go/ast` types through shared/public APIs. Do not add type checking, `go list`, module download or compiler execution merely for declarations.

Exit criterion: Go passes the existing R25 quality bar and proves standard-library AST integration through the same IR used by native scanners.

### Phase 7 — C# brace/OOP canary

Implement a native C# structural analyzer using the shared scanner plus focused token parsing. R25 requires declaration/navigation coverage for representative:

- namespaces, classes, structs, interfaces, records and enums;
- methods, constructors, properties, events and fields/constants;
- generics and nested types;
- attributes/modifiers without promoting attributes to declarations;
- using directives where included in `digest` structural metadata;
- partial and extension-method syntax represented without project-wide semantic resolution;
- expression-bodied members sufficiently to obtain declaration/body ranges where reliable.

Do not attempt full C# compilation/type binding in R25. Tests must aggressively include declaration-like text in comments, strings, interpolated strings and malformed/incomplete editing states.

Exit criterion: the common model handles a modern brace/OOP language without Go-specific special cases.

### Phase 8 — VB.NET Basic-family canary

Implement native VB.NET declaration analysis through a case-insensitive scanner, logical-line builder and explicit-scope recognizer. Cover representative:

- `Namespace`, `Module`, `Class`, `Structure`, `Interface`, `Enum`;
- `Sub`, `Function`, constructors, `Property`, `Event`, fields/constants;
- `Inherits`/`Implements` structural declarations;
- generics and modifiers;
- line continuation and colon-separated statements;
- `'` comments and string handling;
- explicit `End ...` scopes and malformed/missing endings.

Structure the Basic-family primitives so later VB6, VBA, VBScript, QBasic/QuickBASIC, classic BASIC and other profiles can reuse them without pretending the dialects are identical.

Exit criterion: line-oriented/case-insensitive/end-delimited syntax works through shared infrastructure and proves the provider abstraction is not brace-centric.

### Phase 9 — Python indentation canary

Implement native Python structural analysis using the shared scanner/indentation support. Cover representative:

- modules/imports for structural digest metadata;
- functions, async functions, classes and methods;
- decorators associated with the following declaration;
- nested definitions;
- multiline signatures and annotations;
- indentation/dedent scope ownership;
- strings including triple-quoted strings so embedded declaration-looking text cannot leak;
- incomplete/malformed source with bounded recovery diagnostics.

R25 does not claim dynamic dispatch/reference resolution.

Exit criterion: indentation-defined ownership works without contaminating the common IR with brace/end assumptions.

### Phase 10 — Classic ASP composite canary

Implement the first `CompositeSegmenter` for Classic ASP. The segmenter must preserve original document coordinates and identify at least:

- host HTML/text regions;
- ASP directives such as language selection;
- `<% ... %>` server-script regions;
- `<%= ... %>` expression regions;
- server-side `<script ... runat="server">` regions with language evidence where present;
- include/dependency directives where reliably recognizable.

Delegate VBScript-like embedded code through a narrowly scoped Basic-family profile sufficient to prove segmentation/delegation architecture; if JScript or another scripting language is declared but not implemented in R25, report that embedded region explicitly as unsupported rather than misparse it. Do not create a monolithic ASP grammar.

Tests must prove that region-relative analysis maps back to exact host-file line/column ranges across mixed HTML/script, CRLF and non-UTF-8 source.

Exit criterion: one physical file can truthfully expose multiple language regions without losing position or coverage evidence.

### Phase 11 — compact `outline` / `digest` / `find` / `show` orchestration

Wire authorized file/list/directory requests through secure traversal, encoding, detection, segmentation and analyzers. Implement the four compact navigation behaviors under one coherent public tool unless Phase 1 explicitly approved another surface.

Requirements include:

- deterministic path then source-position ordering;
- bounded concurrency using existing project primitives;
- independent per-file failures and aggregate `coverageComplete`;
- language/kind filters without hiding unsupported coverage;
- `digest` that avoids returning full source bodies;
- request-scoped `find` with deterministic exact/prefix/qualified lookup and explicit ambiguity;
- `show` that re-reads/currently verifies the target source evidence before returning the selected bounded source region, rather than trusting a stale copied body;
- strict output/result limits and cancellation;
- no workspace mutation, cache files, external process or network activity.

Exit criterion: an MCP client can navigate the R25 canary languages materially more efficiently than by reading every complete source file.

### Phase 12 — correctness corpus and adversarial quality gate

Create a maintainable per-language conformance corpus and reusable harness. For every R25 language/form, include:

- positive declaration variants;
- negative declaration-looking comments/strings;
- multiline signatures and nesting;
- malformed/truncated editing states;
- ambiguous detection fixtures;
- scope/shadow/overload cases relevant to that language;
- Unicode identifiers/content;
- applicable UTF-8/UTF-16/UTF-32/legacy encodings;
- LF/CRLF and relevant mixed-line-ending cases;
- large/generated sources within limits;
- adversarial long literals/comments/nesting;
- deterministic repeated-output checks;
- cancellation and resource-limit failures.

Track quality with tests that make false positives and range errors visible. Where practical, use hand-authored expected symbol tables/ranges rather than self-generated golden output from the analyzer under test. Fuzz scanner/recognizer boundaries and malformed inputs.

Exit criterion: each canary satisfies a documented production declaration/navigation bar, not merely hello-world parsing.

### Phase 13 — performance, memory, and concurrency gate

Benchmark representative workloads before finalizing defaults:

- many small files;
- fewer large generated files;
- mixed R25 languages;
- unambiguous versus ambiguous language detection;
- cold parse and repeated request/cache behavior if a bounded cache exists;
- outline/digest/find/show operations.

Verify that retained memory is bounded by configured per-file/aggregate/result limits rather than total repository source size. If a cache is introduced, fingerprint content, bound count/bytes, invalidate on source/provider/config change, and make restart semantics explicit. Race-test shared registry/cache state.

Exit criterion: defaults/hard ceilings are evidence-driven, documented and covered by tests; no unbounded AST/token/source retention exists.

### Phase 14 — MCP/catalog/documentation integration

Only after internal behavior is stable:

- add/update the authoritative tool catalog entry and registration/schema;
- update README/TOOLS references and examples without duplicating the detailed design;
- document the R25 supported-language/capability matrix generated or mechanically checked against the registry;
- update roadmap status/completion text only according to actual milestone state;
- verify no duplicate near-equivalent source-intelligence tools were introduced;
- run source MCP smoke over stdio and HTTP and connector-level read-only acceptance.

Exit criterion: discovery, catalog, docs and runtime schema agree exactly and the canary capability matrix is truthful.

### Phase 15 — full completion and R27 handoff gate

Review the complete diff and confirm no unrelated files changed. Run the affected focused tests first, then the repository gates required by the applicable guides, including formatting, `go mod verify`, full normal tests, race where configured toolchain permits it, vet, Staticcheck, govulncheck, source server smoke, documentation/link/catalog checks, relevant fuzz tests, `git diff --check`, final Git status, and supported-platform compile/runtime checks required by changed code.

Before declaring R25 complete, perform an explicit architecture review against the full R27 language/capability matrix in `MULTILANGUAGE_CODE_INTELLIGENCE.md`. Confirm that adding VB6/VBA/VBScript, C/C++, Java/JavaScript/TypeScript, Rust, Pascal/Delphi, MQL4/MQL5, .NET-related formats and composite web formats does not require breaking the R25 public model.

R25 completion does not authorize R27 implementation, commit, push, release, candidate build, launcher change, tunnel restart or deployment. Those remain separately authorized actions.

Exit criterion: every R25 completion-gate item and repository verification gate has passed with recorded evidence, and the R27 compatibility review finds no public-schema redesign requirement.

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

R25 proves the common model across structurally different canaries. R27 must then deliver the approved broad native language/source-format catalog, project relationships, structural search, bounded context, and incremental indexing. R25 must therefore be judged not only by its canary accuracy, but by whether its architecture can support the complete R27 capability matrix without breaking the public source-symbol contract.
