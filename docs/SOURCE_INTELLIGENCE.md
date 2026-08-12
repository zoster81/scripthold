# Source Intelligence Foundation Design

## Status

**APPROVED — R25 PLANNED DESIGN BASELINE.** This document records the approved foundation for source-symbol extraction and indexing. R25 remains planned and is not activated automatically by completion of an earlier milestone; roadmap ordering or explicit maintainer reprioritization governs activation.

R25 establishes a language-neutral public model and provider architecture. Native Go parsing is the first reference implementation because the repository is Go and the standard library provides a high-quality parser without a new dependency. **Go is not the final product scope.** Broad multi-language coverage is a mandatory R27 outcome defined separately in [MULTILANGUAGE_CODE_INTELLIGENCE.md](MULTILANGUAGE_CODE_INTELLIGENCE.md).

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
- return structured declarations rather than regex matches;
- represent functions, methods, types/classes, interfaces/traits where applicable, fields/properties, constants, variables/globals, constructors, namespaces/modules/packages, and other provider-supported declaration kinds;
- preserve hierarchical ownership (`parent`, qualified name, nested symbols where appropriate);
- return source positions and bounded signatures without returning complete source files by default;
- operate on one file, a bounded list of files, or bounded directory roots;
- reuse Scripthold's secure deterministic traversal and allowed-root boundary;
- make language selection explicit and trustworthy rather than infer from a filename alone without verification;
- establish a provider interface that permits multiple parser technologies later while preserving one public schema;
- use Go's `go/parser`, `go/ast`, and `go/token` as the first production provider;
- report unsupported languages and unsupported semantic capabilities explicitly;
- remain bounded in files, bytes, symbols, parse diagnostics, retained signatures, output size, time, and memory;
- preserve stdio/Streamable HTTP equivalence;
- provide the stable foundation required by R27 for broad multi-language code intelligence.

## Non-goals

R25 will not:

- pretend grep/regular expressions are a parser fallback for unsupported languages;
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

If a future provider requires an external process, that provider belongs to an explicitly reviewed R27 execution/security design and cannot be smuggled into R25 as an implementation detail.

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

### Risk: regex fallback produces plausible but wrong symbols

Mitigation: unsupported languages remain unsupported until a reviewed parser/provider exists. Grep remains a separate lexical tool and is never mislabeled semantic parsing.

### Risk: parsing entire repositories consumes too much memory

Mitigation: secure bounded traversal, per-file parsing, bounded concurrency, symbol/signature caps, independent output limits, and no unbounded retained AST/project index.

### Risk: filename-based language selection conflicts with Scripthold's content-evidence philosophy

Mitigation: extension/path naming can only choose parser candidates. Successful provider parsing/validation is required before claiming coverage, and ambiguous cases remain explicit.

### Risk: syntax symbols are mistaken for semantic resolution

Mitigation: every provider declares capability level. R25 results describe declarations/source structure; references, implementations, resolved calls, and project semantics are reserved for providers that can establish them accurately, principally in R27.

## Completion gate

R25 is complete only when:

1. the language-neutral symbol schema and provider interface are documented and reviewed against the R27 target language families;
2. a read-only public symbol operation is implemented with strict bounds and explicit partial coverage;
3. Go's native AST provider implements the approved declaration/hierarchy/signature/range baseline without external execution;
4. language selection does not treat extension alone as authoritative proof;
5. encoding, allowed-root, traversal, cancellation, output, and transport invariants remain intact;
6. regex pseudo-parsing is not used as an unsupported-language fallback;
7. deterministic, malformed-input, fuzz/resource, mutation-negative, and connector smoke tests pass;
8. the implementation leaves a documented provider extension point suitable for R27 rather than requiring a public-schema rewrite for each new language.

## Relationship to R27

R25 proves the common model. R27 must then deliver broad, production-quality multi-language coverage and deeper semantic relationships. R25 must therefore be judged not only by how well it parses Go, but by whether its architecture can support the mandatory R27 language matrix without breaking the public source-symbol contract.
