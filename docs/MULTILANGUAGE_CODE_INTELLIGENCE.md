# Broad Multi-Language Code Intelligence Contract

## Status

R27 is **COMPLETE**. This document defines the final broad native source-intelligence contract completed in 2026-08 and built on the R25 `source_symbols` foundation.

R27 uses Scripthold-owned Go scanners, recognizers, structural parsers, composite segmenters, and project resolvers plus standard-library facilities where available. Ordinary source-intelligence requests do not depend on external parser engines, downloaded grammars, compiler frontends, language-server runtimes, project execution, or hidden network activity.

The final public surface preserves `source_symbols` and adds the strict read-only `source_query` `search`, `relations`, and `context` operations. The provider catalog has 101 active analyzers across 103 registry rows; capability claims remain provider-specific, evidence-qualified, bounded, deterministic, and fail closed where analysis cannot prove a relationship.
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

R27 retains a minimum release gate while targeting a substantially broader native language catalog. The minimum families that may not be dropped are:

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

Those fourteen are a minimum quality gate, **not the architectural or product horizon**. R25's registry/scanner/IR design and R27's implementation plan must also accommodate the following approved target catalog, with support and capability reported per canonical language/dialect rather than by vague family labels:

- mainstream/system: C, C++, Objective-C/Objective-C++, C#, Java, Kotlin, Scala, Go, Rust, Swift, Dart, D, Zig and Nim;
- web/dynamic: JavaScript, TypeScript/TSX, Flow, Python, PHP, Ruby, Perl, Lua, Luau, Elixir, Erlang, Gleam and Groovy;
- .NET-relevant source: C#, VB.NET, F#, C++/CLI, legacy JScript.NET and CIL/MSIL, plus PowerShell as a major .NET-adjacent automation language;
- Basic family: VB.NET, VB6, VBA, VBScript, QuickBASIC/QBasic, classic line-numbered BASIC families, FreeBASIC and PureBasic;
- scientific/legacy: Fortran, COBOL, Ada, Pascal/Object Pascal/Delphi, MATLAB/Octave, Julia and R;
- functional/Lisp: Haskell, OCaml, Common Lisp, Clojure and Emacs Lisp;
- shell/automation: POSIX shell/Bash, PowerShell, Tcl and AutoHotkey;
- trading: **MQL4 and MQL5 as distinct dialects/languages**, sharing infrastructure only where their syntax actually permits it;
- hardware/low-level: Assembly with explicit dialect profiles, VHDL, Verilog/SystemVerilog and Arduino source conventions;
- data/infra/DSL: SQL with explicit dialect profiles where needed, PL/SQL, GraphQL, HCL/Terraform, Nix, Protocol Buffers, Solidity, Apex and AL;
- document/config source formats: HTML, XML, CSS, SCSS, Sass, Less, JSON, YAML, TOML, Markdown, OpenAPI and Ansible-oriented YAML where distinguishable;
- composite/template formats: Classic ASP, ASP.NET Web Forms, Razor/Blazor, Vue, Svelte, Astro, PHP/HTML, JSP, Jinja, Twig, Blade and EJS.

A composite/source format is not falsely promoted to a standalone programming language merely for registry simplicity. One physical document may expose a host format and several embedded language regions. Similarly, CLR implementations of an existing language such as IronPython or IronRuby reuse the Python/Ruby language model rather than inventing a duplicate language solely because the runtime differs.

A provider may share native infrastructure across closely related languages, but support must still be reported per canonical language/dialect. TypeScript cannot be claimed solely because JavaScript parses; Delphi/Object Pascal cannot be claimed from minimal ISO Pascal; VB6/VBA/VBScript cannot be collapsed into VB.NET; MQL4 cannot be claimed solely because MQL5 recognizes similar C-like constructs.

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
- no raw lexical/regex match marketed as a stronger syntactic or semantic capability than the analyzer actually proves.

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

Semantic capability may require substantially richer native parsing, project modeling, scope/type information, or dispatch analysis. It must be exposed only when Scripthold can establish it accurately; otherwise the strongest justified evidence remains structural, scope-resolved, or project-resolved while the orthogonal resolution state reports resolved, ambiguous, unresolved, or external as applicable.

### Index capability

- incremental invalidation;
- project-wide symbol lookup;
- project-wide reference lookup;
- aggregation of relation types the provider/project model actually supports;
- stable generation/source fingerprints.

The provider/result schema must state capability level explicitly so clients know whether an answer is syntax-derived, semantically resolved, partial, or unsupported.

## Breadth and evidence requirement

R27 completion required more than declaration-only breadth. All mandatory languages meet the declaration quality bar; providers expose reliable structural relationships rather than hiding them for schema simplicity; project-resolved references/definitions cover a meaningful cross-ecosystem subset; and stronger semantic labels remain reserved for facts the native analyzers can actually prove.

The mechanically checked per-provider capability matrix in [LANGUAGE_CAPABILITIES.md](LANGUAGE_CAPABILITIES.md) is the public record of those claims. Unsupported or weaker relationships remain explicit rather than being inferred from name similarity.

## Public operations

R27 preserves a compact read-only surface rather than creating one MCP tool per query verb:

- `source_symbols` retains the R25 `outline`, `digest`, `find`, and fingerprint-bound `show` navigation contract;
- `source_query` provides strict `search`, `relations`, and `context` operations sharing source selection, generation/fingerprint binding, evidence vocabulary, and resource policy.

Unknown fields and fields illegal for the selected operation are rejected. Unsupported relationships return explicit `UNSUPPORTED` behavior rather than guessed results. No separate public index-management tool or persistent on-disk index was introduced.

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

R27 remains provider/analyzer-agnostic at the public API layer but the approved implementation foundation is **native Scripthold Go code**. R27 extends the scanner, recognizer, structural-parser, composite-segmentation and project-resolution primitives established by R25 instead of embedding third-party parser engines or grammar runtimes.

Allowed native implementation strategies include:

- reuse of Go standard-library parsers where the target language is already supported there;
- shared Scripthold scanners/lexers configured by language-family profiles;
- token-aware declaration/scope recognizers;
- dedicated Scripthold structural parsers for languages whose constructs cannot meet the quality bar with simpler recognizers;
- regex/grep as bounded lexical or recognizer primitives when their evidence level is truthful;
- composite-document segmenters that delegate embedded regions to other native analyzers;
- Scripthold-owned scope, import/export, dependency and project resolvers layered over normalized structural facts.

Do not introduce Tree-sitter, Babel, OpenRewrite, language-server runtimes, compiler frontends, downloaded grammars, or analogous parsing engines merely to accelerate R27 coverage. External projects may inform algorithms and tests but are not dependencies. If a future maintainer wants to change this native-only foundation, that is a new explicit architecture decision requiring documentation and roadmap revision before implementation.

Quality is evaluated per language and per capability, not by the number of language names a registry can accept.

## External process / language-server boundary

External language servers, compiler services and parser processes are **outside the R27 contract**. Unsupported or partial semantics remain explicit where the native analyzers cannot establish compiler-equivalent resolution.

Introducing an external semantic-provider boundary would require a separate explicit architecture/security decision. It must not be introduced incidentally or use `task_run` or arbitrary request-controlled execution as a shortcut.

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

## Call-site boundary

Some providers expose syntactic call-site evidence, but that capability is not equivalent to a resolved call graph. A call target cannot be labeled resolved merely because one same-named function exists in the project, and dynamic/virtual dispatch remains explicit uncertainty.

Public `callers`/`callees` relations remain `UNSUPPORTED` unless the relevant analyzer/project model can prove the required target identity. Any future resolved call-graph capability must remain bounded in depth, nodes, edges, files, and output and must preserve evidence/resolution state rather than upgrading lexical or structural matches.

## Incremental indexing

R27 implements bounded **process-local** coherent index generations. File content fingerprints are the primary invalidation signal; provider/analyzer configuration also participates in generation identity. Unchanged parsed facts may be reused, while changed generations conservatively rebuild project relationships before publication when narrower dependency invalidation cannot be proven safely.

Queries bind to one coherent generation and stale-generation policy is explicit. Complete source bodies are not retained in the index; context materialization reopens and fingerprint-verifies current authorized source. Modification time alone is never authoritative source identity.

R27 introduced no persistent on-disk source index. Any future persistence would create a new protected storage authority with its own ownership, format, corruption, quota, redaction, and cleanup design and must not reuse the backup store casually.

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
- concurrent parses/queries;
- index memory;
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

R27 retains only bounded process-local index generations. Persistent index storage and external semantic providers remain outside this contract and require separate architecture/security decisions.

## Frozen R27 implementation decisions

R27 inherits all frozen R25 source-intelligence decisions unless this document explicitly strengthens them. In particular:

1. source intelligence remains implemented in native Scripthold Go code;
2. the R25 `SourceDocument`, registry, detector, scanner, analyzer, evidence, composite-region and normalized-symbol abstractions are extended rather than replaced by a second architecture;
3. regex/grep remain allowed at truthful textual/lexical/recognizer evidence levels;
4. broad coverage must not be achieved by weakening capability labels;
5. programming-language support is reported per canonical language/dialect; composite/source formats are reported as host/embedded capabilities rather than fake standalone semantics;
6. all approved target-catalog entries must have an explicit capability row before R27 completion; programming languages must reach production declaration/navigation coverage unless this document explicitly marks the concept not applicable, composite formats must reach reliable segmentation/delegation, and document/config formats must expose the structural capabilities meaningful to that format;
7. the original fourteen mandatory families remain non-negotiable high-quality gates and cannot be displaced by adding many easier languages;
8. MQL4 and MQL5 are separate supported dialects; VB.NET, VB6, VBA and VBScript are separate supported dialects; Pascal/Object Pascal/Delphi coverage must include Delphi-specific constructs; C/C++/Objective-C, Java/JavaScript, and similarly named families are never conflated;
9. relationship evidence uses the ordered R25 ladder textual, lexical, structural, scope-resolved, project-resolved, semantic; resolution is a separate state machine with resolved, ambiguous, unresolved, and external states where required;
10. one unique project-wide name match is not semantic resolution;
11. exact source bodies are not duplicated into the index by default; ranges and fingerprints point back to authoritative source;
12. persistent on-disk source indexing is not part of R27; introducing it later requires a separate protected-storage architecture decision;
13. public tools remain compact: preserve `source_symbols` plus the additive `source_query` `search`/`relations`/`context` surface rather than one tool per relation verb;
14. embeddings, model dependencies, automated source rewriting/refactoring, and external parser/compiler/LSP processes are outside R27's approved core scope.

## R27 capability matrix contract

Every registry entry exposes a mechanically checkable capability row containing at least:

- canonical language/source-format ID and family;
- detection evidence supported;
- scanner/lexer profile;
- composite host/embedded behavior where applicable;
- declarations/hierarchy/signatures/ranges;
- imports/includes/uses/dependencies;
- inheritance/implements/traits/protocol relationships;
- syntactic calls;
- scope-resolved references where available;
- project-resolved references/definitions where available;
- implementations/overrides where available;
- caller/callee resolution level;
- incremental-index support;
- legacy/source encodings exercised by tests where relevant;
- analyzer strategy and version;
- known unsupported/partial constructs.

A single `supported: true` flag is insufficient. [LANGUAGE_CAPABILITIES.md](LANGUAGE_CAPABILITIES.md) is the deterministic checked-in projection of the native registry; `go run ./internal/sourceintelligence/cmd/capability-matrix` renders the canonical Markdown to stdout, and tests compare the checked-in file byte-for-byte so implementation, tool output, and documentation cannot drift independently.

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

## Benchmark and scale evidence

R27 qualification includes representative many-small-file, large-generated-file, mixed-language, cold/warm/incremental, symbol/reference, graph, and context workloads. Performance claims are evidence-driven; correctness and bounded resource behavior take precedence over cache or invalidation optimization.

## Devil's advocate findings

### Risk: broad language count sacrifices correctness

Mitigation: support is counted per language only after a production-quality fixture/malformed/resource test bar. Capability levels are explicit and providers may remain syntax-only rather than fake semantic results.

### Risk: R27 quietly remains Go-centric

Mitigation: the mandatory language list and completion breadth requirements are normative. Go cannot be the only language with semantic relationships, and Pascal/Delphi plus multiple distinct ecosystems are explicitly required.

### Risk: broad syntax coverage is mistaken for semantic intelligence

Mitigation: recognizing syntax/declarations does not automatically establish resolved references, implementations, or calls. Every relationship retains its actual evidence level and stronger project/semantic claims require the corresponding native resolver evidence.

### Risk: coverage pressure reintroduces external parser/runtime dependencies

Mitigation: R27's approved plan is native. Unsupported or partial capabilities remain explicit rather than being filled by an incidental language server, compiler process, downloaded grammar, or request-controlled execution path. Changing that boundary requires a later explicit architecture decision.

### Risk: index generations leak source or become stale authority

Mitigation: process-local generations are content/provider/config fingerprint-bound, bounded, and coherent; complete source bodies are not retained, and context reopens fingerprint-verified authorized source. Persistent index storage was not introduced.

### Risk: dynamic languages produce misleading reference graphs

Mitigation: dynamic/ambiguous dispatch remains explicit. Name similarity or lexical occurrence is not promoted to semantic resolution.

## Completion gate

R27 is complete only when:

1. every approved target-catalog entry has a mechanically checked capability row; programming languages reach the declared production declaration/navigation bar, composite formats reach reliable segmentation/delegation, and document/config formats expose their meaningful structural entities;
2. the original fourteen mandatory families satisfy their stronger production-quality acceptance bar and cannot be substituted by easier breadth elsewhere;
3. Pascal/Object Pascal/Delphi coverage includes the language-specific baseline above and representative legacy-source tests;
4. MQL4/MQL5, VB.NET/VB6/VBA/VBScript, the approved .NET-relevant formats, Classic ASP and other explicitly requested dialects are separately identified and tested rather than hidden behind family labels;
5. the published capability matrix states detection, declarations, structural relationships, scope/project resolution, semantic relationships where genuinely proven, calls, dependencies, composite support, encoding coverage and incremental indexing support;
6. project-wide resolved references/definitions and graph relationships work across the required distinct language ecosystems and retain truthful structural/scope-resolved/project-resolved/semantic evidence plus explicit resolved/ambiguous/unresolved/external state instead of Go-only or name-only certainty;
7. the frozen `source_query` search/relations/context operations and retained `source_symbols` workflow satisfy the compact public-surface, bounds, determinism and read-only requirements;
8. structural search and supported relations (including dependencies/dependents, implementations/inheritance, trace, impact, and cycles) are bounded and evidence-aware; `callers`, `callees`, `overrides`, or other unproven relations remain explicitly `UNSUPPORTED`;
9. incremental invalidation is content-fingerprint-based and coherent-generation safe; R27 introduces no persistent index and retains no complete source bodies in process-local generations;
10. no external parser/compiler/language-server process or downloaded grammar/runtime was introduced outside an explicit later architecture decision;
11. encoding, decoded-coordinate mapping, allowed-root, cancellation, concurrency, transport, logging and resource invariants remain intact across heterogeneous and composite repositories;
12. complete language/composite corpora, malformed/negative tests, encoding/range tests, fuzz/resource/race tests, cross-language integration tests, scale benchmarks, connector smoke, static/vulnerability checks and supported-platform build/runtime gates pass as applicable.

## Future boundary

R27 establishes broad code intelligence, not automatic code transformation. Future source-intelligence work may improve analysis only where evidence remains truthful and bounded. Source changes continue through explicit verified mutation capabilities; automatic semantic refactoring/project-wide transformation remains outside the approved roadmap unless maintainers deliberately reopen that architectural decision.
