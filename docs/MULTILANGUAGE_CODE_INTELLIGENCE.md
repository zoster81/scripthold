# Broad Multi-Language Code Intelligence Design

## Status

**APPROVED — R27 PLANNED DESIGN BASELINE.** This document records the mandatory broad multi-language outcome for R27. R27 is not a Go enhancement milestone and must not be declared complete with Go-only or narrowly Go-centric coverage.

R27 builds on the language-neutral symbol/provider architecture required by [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md). Exact parser libraries, grammar versions, language-server integrations, and public tool names remain implementation-time design choices subject to dependency/security review, but the language breadth, capability transparency, resource bounds, and correctness requirements below are approved.

## Product outcome

R27 turns Scripthold from a filesystem/text-aware MCP server with source outlines into a **broad source-code intelligence layer** useful across heterogeneous real-world repositories.

An agent should be able to ask, without loading entire projects into context:

- what symbols exist and where they are declared;
- where a symbol is referenced;
- what implements or overrides an interface/type/member where the language model can prove it;
- what a file/module/package depends on;
- what calls a function/method and what it calls where reliable analysis is available;
- how declarations are organized across modules/classes/types/namespaces;
- what changed in the source index after files are edited;
- which parts of an answer are complete, partial, syntactic-only, or semantic.

The design must favor trustworthy, explicit capability levels over plausible but incorrect cross-language guesses.

## Mandatory language breadth

R27 must provide production-quality source-symbol coverage for a broad minimum language set. The baseline required families are:

1. **C**;
2. **C++**;
3. **C#**;
4. **Java**;
5. **Kotlin**;
6. **JavaScript**;
7. **TypeScript**;
8. **Python**;
9. **Rust**;
10. **Go**;
11. **PHP**;
12. **Ruby**;
13. **Swift**;
14. **Pascal / Object Pascal / Delphi**.

A provider may share infrastructure across closely related languages, but support must be reported per canonical language rather than hiding gaps behind one family label. For example, TypeScript support cannot be claimed solely because JavaScript parses, and Delphi/Object Pascal features cannot be claimed solely from a minimal ISO Pascal grammar.

Additional languages are encouraged when a reliable bounded provider is available. Candidate future additions may include shell languages, PowerShell, Lua, Scala, Objective-C, Dart, SQL dialects, or others, but these do not replace the mandatory baseline above.

## Minimum quality bar per mandatory language

Every mandatory language must have a documented production provider that passes:

- representative real-world source fixtures;
- malformed/incomplete-source tests;
- comments/string false-positive negatives;
- language-specific declaration/hierarchy/signature tests;
- Unicode/source-encoding tests applicable to that ecosystem;
- large-file/resource limits;
- deterministic ordering;
- cancellation;
- cross-platform tests where provider behavior depends on platform;
- explicit capability reporting;
- no regex pseudo-parser fallback marketed as language support.

A language is not counted as supported merely because a grammar can parse one hello-world file.

## Capability levels

R27 must distinguish what each provider can actually prove. A single boolean `supported` is insufficient.

The common capability vocabulary should cover at least:

### Declaration capability

- symbols/declarations;
- hierarchy/ownership;
- signatures;
- source ranges;
- visibility/modifiers where reliable.

This capability is mandatory for **all** baseline languages.

### Structural relationship capability

- imports/includes/uses/requires/module dependencies;
- type inheritance/extension declarations;
- interface/trait/protocol declarations;
- syntactically identifiable call sites;
- local/member relationships that do not require project-wide type resolution.

Providers should expose these where grammar evidence is reliable.

### Semantic relationship capability

- resolved references;
- definitions;
- implementations;
- overrides;
- resolved call graph edges;
- type relationships requiring name/type resolution.

Semantic capability may require a richer parser, compiler frontend, project model, or language-server integration. It must be exposed only when the provider can establish it accurately.

### Index capability

- incremental invalidation;
- project-wide symbol lookup;
- project-wide reference lookup;
- dependency/call graph aggregation;
- stable cache/index fingerprints.

The provider/result schema must state capability level explicitly so clients know whether an answer is syntax-derived, semantically resolved, partial, or unsupported.

## Completion breadth requirement

R27 cannot satisfy its goal by adding declaration-only providers for many languages while keeping all advanced intelligence Go-only.

Before R27 can be complete:

- all mandatory languages must satisfy the declaration capability quality bar;
- each mandatory language must expose every reliable structural relationship supported by its provider rather than discarding it for schema simplicity;
- project-wide semantic references/definitions must work for a **meaningful cross-ecosystem subset**, not just Go;
- that semantic subset must include languages from several distinct ecosystems, including at minimum one C/C++-family language, one JVM/.NET-family language, one JavaScript/TypeScript-family language, one dynamic language, Rust or Go, and Pascal/Delphi where provider technology can establish references accurately;
- any mandatory language lacking a semantic feature must report it explicitly as unsupported/partial, with no regex approximation.

The final R27 activation plan must publish a concrete per-language capability matrix before implementation is considered complete.

## Public operations

Exact tool names are deferred, but the approved product surface conceptually includes read-only operations equivalent to:

- `source_symbols` — R25 declaration outline, retained and expanded;
- `source_find_symbol` — bounded project symbol lookup by exact/prefix/qualified identity;
- `source_references` — references to a selected symbol/definition under provider capability;
- `source_definitions` — resolve declaration/definition targets where supported;
- `source_implementations` — interface/trait/base-type implementation relationships where supported;
- `source_dependencies` — module/package/file dependency edges;
- `source_callers` / `source_callees` or one bounded call-relationship operation;
- project/index status or refresh operations only if an incremental index is introduced.

The final public design should avoid an explosion of near-duplicate tools if a smaller strict schema can express the same capabilities clearly. However, distinct read-only vs mutating boundaries from R23 remain mandatory; source intelligence itself is read-only.

## Common entity model

R27 must extend, not replace, R25's language-neutral source model.

Entities require stable request-local/project-local identifiers derived from deterministic evidence such as:

- language/provider;
- project/index fingerprint;
- normalized path;
- qualified name;
- declaration kind;
- source range;
- provider-specific disambiguator for overloads/generics where necessary.

Identifiers must not be claimed globally stable across arbitrary edits unless the algorithm can prove that property.

The common model must represent constructs found outside Go, including:

- namespaces and modules;
- classes/structs/records;
- interfaces/traits/protocols;
- enums/unions;
- functions/procedures;
- methods;
- constructors/destructors;
- properties/fields/events;
- operators;
- templates/generics/type parameters;
- extension methods/categories/extensions where applicable;
- nested/local declarations where the provider chooses to expose them;
- Pascal units, classes, records, interfaces, procedures/functions, properties, and implementation/interface-section relationships.

The schema must not erase overload identity or force unrelated language constructs into one ambiguous name.

## Provider technology policy

R27 is provider-agnostic at the public API layer.

Allowed implementation strategies may include:

- language-native parser/compiler libraries;
- mature reviewed parser libraries;
- Tree-sitter-class concrete syntax grammars;
- compiler frontends;
- language-server protocol integrations;
- carefully bounded combinations of syntax parser plus project resolver.

Provider selection must consider correctness, licensing, release maintenance, platform support, memory/CPU behavior, startup cost, security boundary, and offline reproducibility.

A provider cannot be selected merely because it supports many grammar names. Quality is evaluated per language.

## External process / language-server boundary

Some semantic features may require external language servers or compiler services. Such integrations are **not ordinary arbitrary `task_run` execution**.

Before any external provider is enabled, its design must specify:

- fixed executable discovery/configuration controlled by the operator, not arbitrary request command strings;
- allowed working directory and project-root confinement;
- sanitized environment;
- no implicit package installation/download;
- no shell interpolation;
- process lifetime and concurrency limits;
- request timeout/cancellation;
- bounded stdout/stderr/protocol messages;
- crash/restart behavior;
- filesystem authority no broader than Scripthold's configured roots where technically enforceable;
- network policy and whether provider networking must be disabled or separately opted in;
- version/protocol compatibility checks;
- redacted logging.

If these conditions cannot be met, the provider remains syntax-only or unsupported rather than obtaining unrestricted shell authority.

## Project model and root discovery

Different languages use different project/package structures. R27 must support heterogeneous repositories without allowing project metadata to broaden filesystem authority.

Project manifests may be used as parsing/resolution evidence, for example module/package/build metadata, but:

- every file read remains inside allowed roots;
- manifests do not grant new roots;
- project-root inference is bounded and deterministic;
- nested projects/monorepos are supported through explicit result evidence;
- package-manager lockfiles/configuration are read as data only;
- no dependency download is triggered automatically;
- missing external dependencies degrade capability explicitly rather than causing hidden network activity.

## Dependency graph

R27 must expose bounded dependency relationships at the most reliable level available:

- file includes/imports;
- module/package imports;
- project/package dependencies derived from source/manifests where safe;
- language-specific `uses`, `require`, `use`, `mod`, `namespace`, or equivalent constructs.

Edges must identify their evidence level: syntactic declaration, resolved local target, external/unresolved target, or provider-semantic resolution.

Graph responses require node/edge/depth/output limits and deterministic ordering.

## References and definitions

A reference result must distinguish at least:

- declaration/definition;
- read/reference;
- write/assignment where provider can establish it;
- call/reference where relevant;
- import/include relationship;
- unresolved/ambiguous candidate.

Project-wide searches must bind to an index/source fingerprint so callers can detect stale results after edits.

Textually matching identifiers in comments, strings, unrelated scopes, or shadowed names must not be returned as semantically resolved references.

When the provider offers only syntax-level candidate references, the result must say so explicitly.

## Implementations and inheritance

For languages with interfaces/traits/protocols/base types, R27 should expose:

- declared inheritance/extension edges;
- implements/conforms relationships;
- overrides where semantically supported;
- mixins/traits/extensions where the common model can represent them accurately.

Duck-typed/dynamic-language relationships must not be fabricated from method-name similarity alone.

## Call graph

Call relationships are useful but easy to overclaim.

R27 must distinguish:

- syntactic call-site target spelling;
- locally resolved target;
- semantically resolved target;
- dynamic/virtual/unknown dispatch.

A call graph edge cannot be labeled resolved merely because one same-named function exists in the project.

Call graph queries require bounded depth, nodes, edges, files, and output. Recursive cycles must be represented without unbounded traversal.

## Incremental indexing

R27 may introduce a process-local or optional persistent source index, but it must be deterministic and invalidation-safe.

The baseline index design must define:

- canonical project/root identity;
- file content fingerprint as the primary invalidation signal;
- provider version/configuration fingerprint;
- project manifest/configuration fingerprint where semantics depend on it;
- per-file parse artifacts;
- symbol/reference/relationship tables;
- bounded memory/disk quotas;
- eviction/retention;
- concurrency control;
- crash recovery if persistent;
- no source-content leakage outside approved storage;
- complete invalidation when provider semantics/configuration changes.

Modification time alone is insufficient as authoritative source identity.

### Incremental behavior

After a file changes, the index should reparse only affected files plus dependency dependents required by provider semantics, rather than rebuild the entire repository when avoidable.

However, optimization must not outrank correctness: if dependency impact cannot be bounded/proven, the provider may invalidate a larger scope explicitly.

## Index storage security

If persistence is introduced, the index becomes a new internal storage authority and requires a dedicated reviewed boundary before implementation.

It must define:

- operator configuration/default-disabled behavior unless otherwise approved;
- protected-root separation from ordinary filesystem tools;
- owner-only permissions;
- path/content redaction;
- versioned format;
- corruption handling;
- quota/cleanup policy;
- whether source snippets are stored (prefer not unless necessary and explicitly approved).

R27 must not casually reuse the backup store for code indexes; the authorities and lifecycle semantics are different.

## Encoding and legacy-source coverage

Broad language support must respect Scripthold's encoding mission.

Requirements:

- provider input goes through the established encoding/BOM trust path unless a provider has a documented raw-byte source contract;
- ambiguous non-empty encoding is not silently guessed;
- line/column mappings remain correct after decode;
- Unicode signatures/names are returned as UTF-8 MCP text;
- language-specific source encoding declarations may inform validation only through an explicitly reviewed provider rule;
- Pascal/Delphi and other legacy codebases must be tested with representative non-UTF-8 source where supported;
- no provider may assume every repository is UTF-8 solely because modern language tooling often does.

## Heterogeneous repository behavior

One request may span multiple supported languages.

The orchestrator must:

- select candidate providers deterministically;
- parse files with the appropriate provider;
- preserve global deterministic ordering;
- report per-language file/symbol/error counts;
- expose incomplete/unsupported files explicitly;
- avoid one provider failure aborting unrelated successful languages unless a global resource/cancellation failure requires termination.

A monorepo response should make it obvious which languages and capabilities were actually covered.

## Performance and resource contract

R27 must remain useful on large repositories without becoming an unbounded IDE daemon.

Hard limits are required for:

- files scanned/indexed;
- source bytes parsed;
- symbols;
- references;
- graph nodes/edges/depth;
- diagnostics;
- external-provider processes;
- concurrent parses/queries;
- index memory;
- optional index disk bytes;
- output bytes;
- per-request time;
- background work, if any.

Expected complexity should be documented per provider/capability. Result limits must not silently imply complete coverage.

## Cancellation and concurrency

All parsing/index/query operations must honor cancellation.

The design must cover:

- simultaneous read queries;
- source changes while indexing;
- duplicate refresh work;
- provider crashes/timeouts;
- index generation swap;
- query against stale generation;
- bounded worker pools;
- race-tested cache/index access.

Queries should either bind to one coherent index generation or return explicit stale/conflict evidence rather than mix generations invisibly.

## Error and coverage model

Every response must make quality/completeness visible.

Expected result metadata includes:

- languages requested/detected;
- providers used and capability levels;
- index/source generation/fingerprint;
- files considered/parsed/skipped;
- complete vs partial coverage;
- limits/truncation;
- syntax/semantic diagnostics count;
- unresolved edges/references count where meaningful.

A provider with missing project dependencies may still return syntax symbols while reporting semantic capability unavailable for that request.

## Security boundary

Source intelligence is read-only with respect to the workspace.

It must not:

- change source files;
- run project code;
- run build/test hooks implicitly;
- download dependencies or grammars at request time;
- expand allowed roots based on project manifests;
- expose backup/task/index internal roots;
- pass arbitrary request strings to a shell;
- log complete source, credentials, private paths, or unbounded provider diagnostics.

Any optional index storage or external semantic provider is a separately reviewable internal trust boundary within R27.

## Language-specific acceptance themes

The final R27 test matrix must include language-specific constructs, not just shared toy examples.

### C / C++

- functions, structs/classes/unions/enums, namespaces, methods, templates where supported;
- headers/includes;
- macros/preprocessor impact explicitly scoped;
- overloads;
- declarations vs definitions;
- compile-database/project context when semantic resolution requires it;
- no claim of fully resolved semantics without the required compilation context.

### C#

- namespaces, classes/structs/interfaces/records/enums;
- methods/constructors/properties/events;
- generics, partial types, extension methods;
- using directives and project references where supported.

### Java / Kotlin

- packages/imports;
- classes/interfaces/enums/records/data/sealed constructs as applicable;
- methods/constructors/properties;
- generics;
- nested types;
- inheritance/implementations;
- mixed Java/Kotlin project relationships where provider supports them.

### JavaScript / TypeScript

- ES modules/CommonJS evidence;
- functions/classes/methods/fields;
- imports/exports;
- arrow functions where symbol policy includes them;
- TypeScript interfaces/types/enums/generics/namespaces;
- JSX/TSX when declared supported;
- dynamic resolution uncertainty explicitly represented.

### Python

- modules/imports;
- classes/functions/methods;
- async definitions;
- decorators;
- nested definitions;
- assignments/type aliases when provider can distinguish them;
- dynamic dispatch/reference ambiguity explicitly represented.

### Rust

- modules/use;
- structs/enums/traits/impl blocks;
- functions/methods;
- generics/lifetimes where relevant to signatures;
- trait implementations;
- macros handled according to documented provider capability.

### Go

- packages/imports;
- types/interfaces/functions/methods;
- generics;
- references/type relationships at the semantic level promised by the provider;
- no special public-schema privilege compared with other languages.

### PHP

- namespaces/use;
- classes/interfaces/traits/enums;
- functions/methods/properties/constants;
- dynamic include/reference limitations explicit.

### Ruby

- modules/classes/methods/constants;
- mixins;
- singleton/class methods;
- reopen/dynamic metaprogramming limitations explicit.

### Swift

- modules/imports;
- classes/structs/enums/protocols/extensions;
- functions/methods/properties/initializers;
- protocol conformance/extension relationships where supported.

### Pascal / Object Pascal / Delphi

- programs, units, packages where applicable;
- `interface` vs `implementation` sections;
- classes, records, interfaces, enums/types;
- procedures/functions/methods/constructors/destructors;
- properties/fields/constants/variables;
- `uses` dependencies;
- forward declarations and implementation matching;
- overloaded methods and visibility sections;
- nested procedures/functions where supported;
- compiler directives/generics/class helpers/record helpers where provider capability permits;
- representative legacy encodings and CRLF source.

Pascal/Delphi is an explicit baseline family, not a best-effort optional add-on.

## Required cross-language tests

R27 must include repositories containing several languages simultaneously and verify:

- correct provider routing;
- same symbol schema across languages;
- no cross-provider identifier collision;
- deterministic aggregate ordering;
- per-language capability reporting;
- independent partial failures;
- output/resource bounds;
- source changes invalidating only appropriate index generations;
- connector use showing agents can navigate heterogeneous projects without reading all source files manually.

## Benchmark and scale gate

Before completion, the project must define representative repository-scale fixtures/benchmarks for:

- many small files;
- fewer large generated files;
- mixed-language monorepos;
- cold parse/index;
- warm incremental update;
- symbol lookup;
- reference query;
- bounded graph traversal.

Memory and latency targets should be evidence-driven during implementation, but regressions must be measured and capped before release.

## Devil's advocate findings

### Risk: broad language count sacrifices correctness

Mitigation: support is counted per language only after a production-quality fixture/malformed/resource test bar. Capability levels are explicit and providers may remain syntax-only rather than fake semantic results.

### Risk: R27 quietly remains Go-centric

Mitigation: the mandatory language list and completion breadth requirements are normative. Go cannot be the only language with semantic relationships, and Pascal/Delphi plus multiple distinct ecosystems are explicitly required.

### Risk: Tree-sitter/grammar availability is mistaken for semantic intelligence

Mitigation: grammar parsing establishes syntax/declarations, not automatic semantic resolution. References/implementations/calls are labeled by evidence level and may require richer providers.

### Risk: external language servers become unrestricted execution

Mitigation: any external provider gets a fixed reviewed process boundary with no request-controlled command strings, no shell interpolation, bounded resources, sanitized environment, and explicit network/dependency policy.

### Risk: persistent indexes leak source or become stale authority

Mitigation: content/provider/config fingerprints drive invalidation, storage is bounded/protected/versioned, source snippets are avoided unless explicitly approved, and queries bind to coherent index generations.

### Risk: dynamic languages produce misleading reference graphs

Mitigation: dynamic/ambiguous dispatch remains explicit. Name similarity or lexical occurrence is not promoted to semantic resolution.

## Completion gate

R27 is complete only when:

1. all mandatory baseline languages have production-quality declaration/symbol providers under the common R25 schema;
2. Pascal/Object Pascal/Delphi coverage includes the language-specific baseline above and representative legacy-source tests;
3. a published per-language capability matrix states declarations, structural relationships, semantic references/definitions, implementations, calls, dependencies, and incremental indexing support;
4. advanced semantic intelligence works across several distinct language ecosystems and is not Go-only;
5. unsupported semantic capabilities are explicit and never replaced by regex/name-guess heuristics;
6. heterogeneous repository queries provide deterministic bounded partial-coverage evidence;
7. any external provider/process and any persistent index have separately reviewed security/storage boundaries;
8. incremental invalidation is fingerprint-based and coherent-generation safe;
9. encoding, allowed-root, cancellation, transport, logging, and resource invariants remain intact;
10. language-specific fixtures, malformed-input tests, fuzz/resource/race tests, cross-language integration tests, scale benchmarks, connector smoke, static/vulnerability checks, and supported-platform build/runtime gates pass as applicable.

## Relationship to future work

R27 establishes broad code intelligence, not automated code transformation. A later refactoring milestone may combine semantic identity from R27 with R23/R24 preview/apply mutation capabilities, but it must preserve exact-change approval, filesystem security, encoding behavior, and backup/partial-state guarantees rather than allowing semantic tools to mutate source directly.
