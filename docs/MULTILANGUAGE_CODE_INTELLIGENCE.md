# Broad Multi-Language Code Intelligence Design

## Status

**APPROVED — R27 ACTIVE DESIGN BASELINE.** This document records the mandatory broad multi-language outcome for R27. R26 is complete; R27 was explicitly activated on 2026-08-14, Phases 0-9 are complete, and Phase 10 is the first incomplete phase. R27 is not a Go enhancement milestone and must not be declared complete with Go-only or narrowly Go-centric coverage.

R27 builds on the language-neutral native analyzer architecture required by [SOURCE_INTELLIGENCE.md](SOURCE_INTELLIGENCE.md). The implementation foundation is fixed to Scripthold-owned Go scanners, recognizers, structural parsers, composite segmenters and project resolvers plus standard-library facilities where available. Public schema details remain subject to the staged contract review below, but external parser engines, downloaded grammars, compiler frontends and language-server runtimes are not implementation-time alternatives within the approved R27 plan.

## Phase 0 activation audit conclusions

The 2026-08-14 activation audit confirmed that the completed R25 foundation can be extended without a breaking redesign of the existing `source_symbols` public contract. The four R25 operations (`outline`, `digest`, `find`, `show`), decoded Unicode-scalar coordinates, fingerprint-bound `show`, request/file/output bounds, explicit ambiguity handling and read-only annotations remain compatibility constraints for R27.

The internal R25 model requires deliberate extension before broad R27 coverage:

- the language registry needs a richer mechanically checked per-language/dialect capability model beyond the current source-analysis/composite/case-sensitivity flags;
- analyzer dispatch must scale beyond a small fixed canary switch while remaining native, deterministic and explicit about provider identity/version;
- scanner/recognizer infrastructure needs profile-driven support for additional scope, directive, string/comment, template/composite and legacy line models rather than one global language switch;
- relation IR must add explicit resolution/evidence state for references, definitions, implementations, overrides, calls and project relationships without overstating lexical or structural matches;
- incremental indexing must be a separate bounded internal authority keyed by source fingerprints/coherent generations rather than hidden mutable state inside the request-scoped R25 handler;
- the R25 tracked conformance corpus and private real-world canary corpus remain regression evidence, but R27 requires representative malformed/negative/encoding/resource fixtures and mechanically checked capability coverage for every production provider.

No release, launcher, deployment or active-runtime state is part of R27 activation. Phase 1 preserved the established R25 `source_symbols` surface and froze the additive `source_query` contract; later R27 phases must implement that contract without silently widening its evidence claims or compatibility boundary.

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
- dependency/call graph aggregation;
- stable cache/index fingerprints.

The provider/result schema must state capability level explicitly so clients know whether an answer is syntax-derived, semantically resolved, partial, or unsupported.

## Completion breadth requirement

R27 cannot satisfy its goal by adding declaration-only providers for many languages while keeping all advanced intelligence Go-only.

Before R27 can be complete:

- all mandatory languages must satisfy the declaration capability quality bar;
- each mandatory language must expose every reliable structural relationship supported by its provider rather than discarding it for schema simplicity;
- project-wide resolved references/definitions must work for a **meaningful cross-ecosystem subset**, not just Go, with each result labeled by its actual evidence level;
- that resolved subset must include languages from several distinct ecosystems, including at minimum one C/C++-family language, one JVM/.NET-family language, one JavaScript/TypeScript-family language, one dynamic language, Rust or Go, and Pascal/Delphi where the native resolver can establish useful relationships accurately;
- semantic labels are reserved for cases where the native analyzer truly establishes the required semantics; otherwise use the strongest justified structural/scope-resolved/project-resolved evidence plus an explicit resolution state rather than regex or name-only approximation.

The final R27 activation plan must publish a concrete per-language capability matrix before implementation is considered complete.

## Public operations

R27 extends R25 without creating one MCP tool per graph/query verb. Phase 1 froze the compact read-only surface as:

- `source_symbols` — retain/expand R25 `outline`, `digest`, `find`, and `show` navigation without a breaking redesign;
- `source_query` — one additive strict read-only contract with `search`, `relations`, and `context` operations sharing source selection, fingerprint/index binding, evidence vocabulary, and resource policy;
- an index/status/refresh operation only if a later incremental-index lifecycle cannot be expressed cleanly through `source_query` without ambiguity.

The single `source_query` contract is the explicitly approved smaller equivalent to the originally sketched three-tool surface. Three complete near-duplicate schemas exceeded the fixed connector-catalog budget, while the consolidated contract remained usable and preserved stricter operation-specific validation in the typed handler. Unknown top-level fields and unknown selector/index fields are rejected; known fields that are illegal for the selected operation are rejected as invalid input. Source intelligence remains read-only.

Phase 1 freezes the request/result vocabulary and bounds, not working project semantics: until later R27 phases implement the native engine, otherwise-valid `source_query` requests return `UNSUPPORTED`. Phase 1 creates no persistent index and does not claim project-wide search, relations, or context results are already available.

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

External language servers, compiler services and parser processes are **outside the approved R27 implementation plan**. R27 must first implement its declared capabilities through the native architecture above and must report unsupported/partial semantics honestly where full compiler-equivalent resolution is unavailable.

A future milestone may reconsider a fixed external semantic-provider boundary only through a separate explicit maintainer decision and security design. Such a decision must not be introduced incidentally while implementing R27 and must not use `task_run` or arbitrary request-controlled execution as a shortcut.

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

Any optional persistent index storage is a separately reviewable internal trust boundary within R27. External semantic providers are outside the approved R27 plan unless a later milestone explicitly changes that decision.

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
12. persistent index storage, if enabled, is a separate protected internal authority and must be reviewed before activation;
13. public tools remain compact: preserve `source_symbols` and use the Phase 1-frozen additive `source_query` `search`/`relations`/`context` surface rather than one tool per relation verb;
14. embeddings, model dependencies, automated source rewriting/refactoring, and external parser/compiler/LSP processes are outside R27's approved core scope.

## R27 capability matrix contract

Before language expansion begins, every registry entry must expose a mechanically checkable capability row containing at least:

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

A single `supported: true` flag is insufficient. Documentation must be generated from or mechanically checked against this registry data so implementation, tool output and docs cannot drift independently. Phase 2 established [LANGUAGE_CAPABILITIES.md](LANGUAGE_CAPABILITIES.md) as the deterministic checked-in projection of the native registry; `go run ./internal/sourceintelligence/cmd/capability-matrix` renders the canonical Markdown to stdout, and tests compare the checked-in file byte-for-byte.

## R27 sequential implementation plan

Every phase is mandatory and sequential. A future chat starts by reading the project guides, this document, `SOURCE_INTELLIGENCE.md`, the current roadmap and private operator backlog, then identifies the first incomplete phase from repository evidence. It must not select an easier later language wave while an earlier phase has unresolved correctness failures.

### Phase 0 — activation and completed-foundation audit

When R27 is explicitly activated:

1. confirm R25 and R26 are complete with their final verification evidence and that R27 has been explicitly activated;
2. record branch, `HEAD`, `origin/main`, working tree and unrelated changes;
3. inspect the actual R25 public schema, registry, detector, scanners, analyzers, composite support, limits, cache behavior and corpus rather than coding from this document alone;
4. run focused R25 regression baselines before modifying shared source-intelligence primitives;
5. compare the shipped R25 model to the complete R27 target catalog and record any internal-only extension needed; a breaking public redesign is a blocker requiring explicit maintainer review;
6. do not change release/runtime/launcher/deployment state as part of activation.

Exit criterion: the completed R25 source-intelligence base and R26 milestone boundary are understood, all deviations from the planned R25 handoff are understood, and R27 alone is the active source-intelligence milestone.

### Phase 1 — freeze R27 public relations/search/context contract — COMPLETE

Completed on 2026-08-14 using TDD: focused RED tests first proved the R27 public contract and new limits were absent, then the smallest contract-only implementation was added. The frozen surface is the explicitly approved smaller equivalent: one additive `source_query` tool with `search`, `relations`, and `context` operations while the existing `source_symbols` contract remains unchanged.

Phase 1 froze:

- search modes `textual`, `lexical`, and `structural`, with explicit evidence filtering;
- relation kinds for dependencies/dependents, references/definitions, inheritance/implementations/overrides, callers/callees, trace, impact and cycles;
- the ordered evidence ladder `textual` / `lexical` / `structural` / `scope-resolved` / `project-resolved` / `semantic`, with separate `resolved` / `ambiguous` / `unresolved` / `external` resolution state;
- graph node/edge/depth/file/result limits and bounded context bytes/items;
- optional index generation/fingerprint binding, `reject`/`allow` stale policy, and result staleness vocabulary `current` / `stale` / `not-indexed`;
- fingerprint-bound path/symbol/position selectors;
- context-budget behavior and deterministic priority vocabulary;
- strict top-level and nested selector/index unknown-field rejection plus operation-specific known-field validation;
- read-only MCP annotations and direct/Streamable-HTTP contract equivalence.

The contract is intentionally compact because three complete near-duplicate public schemas exceeded the fixed connector definition budget. Local JSON-Schema definition reuse and typed runtime validation preserve the same operation-specific legality and configured ceilings without raising that budget. The generic compact MCP output schema remains the repository-wide connector policy; the typed structured-result model and this contract document freeze relationship evidence, resolution, index, coverage, and context vocabulary.

Valid Phase 1 `source_query` requests deliberately return `UNSUPPORTED` until later native analysis/index phases implement them. No persistent index, relation engine, context assembler, release, launcher, deployment, or active-runtime change is part of Phase 1.

Exit criterion: **met** — clients can distinguish every planned relationship/evidence/resolution state without language-specific hidden fields, and the public contract is transport-equivalent and resource-bounded.

### Phase 2 — expand shared native scanner/recognizer infrastructure — COMPLETE

Completed on 2026-08-14. The shared native scanner/recognizer foundation now provides reusable profile-driven primitives for:

- language-specific identifier policies while preserving R25's Unicode identifier behavior as the default;
- configurable line-start directives and balanced single- or multi-byte delimiters with top-only deterministic pairing;
- Basic-style logical-line assembly, statement separators and continuation reuse; Python now reuses the same line builder for indentation-aware logical lines;
- analyzer-proven keyword-scope pairing that fails closed on crossed or malformed closes rather than re-parenting source;
- bounded free/fixed-form physical-line models with decoded Unicode-scalar column handling, continuation fields and fixed/colon label recognition;
- Lisp/S-expression balanced forms without exposing declaration-like text in strings/comments;
- bounded queued shell-style heredocs, quoted delimiters, tab-stripping variants, cancellation and complete malformed-pending diagnostics;
- offset-preserving composite/template segmentation and masking that retains exact decoded UTF-8 byte offsets and CR/LF positions;
- reusable delimiter pairing adopted by C#, logical-line assembly adopted by VB.NET and Python, proving the new primitives replace R25 duplication rather than sitting unused beside it.

Phase 2 also expanded the registry to cover the approved R27 target catalog as explicit canonical capability rows without activating unsupported analyzers. `Razor` and `Blazor`, Objective-C and Objective-C++, MQL4/MQL5, VB-family dialects and other required distinctions remain separate registry identities. At the Phase 2 checkpoint only the five completed R25 providers claimed production declaration capabilities; later provider waves activate rows only after their implementation and tests pass, while all remaining planned rows stay explicitly `unimplemented` with analyzer strategy/version and known limitations recorded. [LANGUAGE_CAPABILITIES.md](LANGUAGE_CAPABILITIES.md) is rendered from this registry and byte-for-byte tested against it.

Scanner/profile validation is fail-closed and bounded, malformed/adversarial cases and fuzz seeds cover delimiters, heredocs, S-expressions, line models and composites, and no external parser/compiler/LSP runtime, network access, persistent index or public schema change was introduced.

Exit criterion: **met** — later language waves can be built primarily from analyzers/profiles and shared bounded primitives rather than scanner forks, and every approved registry entry has mechanically checked capability metadata before language expansion begins.

### Phase 3 — mandatory declaration wave: C/C++ and JVM — COMPLETE

Completed on 2026-08-14 using TDD over two native structural parser families built on the Phase 2 scanner foundation. C and C++ now provide bounded declarations versus definitions, structs/classes/unions/enums, namespaces, fields/globals/constants, functions/methods, constructors/destructors/operators, templates, overload identity, literal includes and structural inheritance. C++ additionally handles raw-string masking, unambiguous same-file qualified out-of-class definitions and function-pointer declarators without promoting pointer variables to functions. `.h` remains ambiguous unless distinctive C++ content independently corroborates the C++ candidate.

Preprocessor macro bodies remain opaque directives; literal includes are reported structurally, while conditional preprocessing marks coverage incomplete because macro state is not evaluated. Macro expansion, compile-database/project type resolution and semantic relationships remain explicitly unsupported rather than inferred.

Java and Kotlin now provide bounded packages/imports, classes/interfaces/enums plus Java records and Kotlin data/sealed/enum-class forms, constructors, methods, fields/properties/constants, nested types, Kotlin type aliases/import aliases, Java annotations, generics-aware declaration headers and structural extends/implements/permits/supertype facts. Classpath/build-model/type resolution and semantic relationships remain outside this phase.

Phase 3 conformance covers malformed/incomplete input, declaration false-positive resistance in comments and advanced string forms, C++ raw strings, Java text blocks, Kotlin triple strings, conditional-preprocessor truthfulness, Unicode/source ranges across UTF-16LE, UTF-32LE, UTF-16BE and Windows-1252 fixtures, deterministic repeated analysis, cancellation, strict symbol limits over generated 1,200-declaration inputs, public `source_symbols` mixed-language routing/find behavior, static analysis and analyzer fuzzing. The capability matrix is regenerated from the authoritative registry, so only implemented providers claim production declaration capabilities.

Exit criterion: **met** — C, C++, Java and Kotlin satisfy the Phase 3 production declaration/navigation gate and add truthful structural dependency/type-relationship facts without claiming later project or semantic resolution.

### Phase 4 — mandatory declaration wave: JavaScript/TypeScript and Rust — COMPLETE

Completed on 2026-08-14 using TDD over one shared ECMAScript/TypeScript structural family with separate analyzer identities and one independent Rust structural analyzer, all built on the Phase 2 scanner/builder/relationship foundation. JavaScript/JSX now covers literal ES-module imports, exports/re-exports, literal CommonJS `require`, functions, classes, constructors, methods, fields/private fields, directly named arrow functions and structural `extends` relationships. Template literals remain declaration-opaque, JSX markup remains inside its containing expression, and a conservative offset-preserving regex prepass prevents valid regular-expression contents from corrupting shared delimiter state without reclassifying division or `/=`.

TypeScript/TSX extends that provider family with interfaces, type aliases, enums, namespaces/modules, properties, generic declarations, overload declaration-versus-definition identity, structural `extends`/`implements` relationships and TSX-safe expression handling. TypeScript retains its own `typescript-native` analyzer identity rather than being reported as JavaScript support. Type checking, dynamic property/prototype behavior, non-literal module loading, JSX/TSX component semantics and project/semantic resolution remain explicitly unsupported.

Rust now covers inline/external modules, `use`, structs and fields, enums, traits, trait and inherent `impl` blocks, functions/methods, associated constants/types, generics/lifetimes in signatures and structural trait-implementation relationships. Rust raw strings are masked offset-preservingly, nested comments and attributes are handled, and macro definitions/invocations remain opaque rather than expanded. `cfg` evaluation, macro expansion, Cargo/project/type/trait resolution and semantic relationships remain explicitly unsupported.

Phase 4 conformance covers declaration false-positive resistance in regex/template/JSX/TSX/raw-string/macro bodies, malformed input, ASI, nested comments/attributes, Unicode and decoded-source ranges across UTF-16LE/UTF-32LE/UTF-16BE fixtures, deterministic repeated analysis, cancellation, strict symbol limits over generated 1,200-declaration inputs per language, public mixed-language `source_symbols` routing/find behavior, vet/Staticcheck and analyzer fuzzing. The capability matrix is regenerated from the authoritative registry and does not activate later providers.

Exit criterion: **met** — JavaScript/TypeScript dynamic uncertainty and Rust trait/impl structure fit the common model with truthful structural evidence, while stronger project/type/semantic claims remain withheld.

### Phase 5 — mandatory declaration wave: PHP/Ruby/Swift/Pascal-Delphi — COMPLETE

Completed on 2026-08-14 with five distinct native analyzer identities (`php-native`, `ruby-native`, `swift-native`, `pascal-native`, and `delphi-native`) over the shared decoded-source scanner/builder model. PHP covers namespaces/use, literal includes, classes/interfaces/traits/enums, functions/methods/properties/constants, constructors/destructors and structural extends/implements/trait-use evidence. Heredoc/nowdoc bodies are masked offset-preservingly and only literal include/require targets are reported; dynamic includes, autoloading and runtime metaprogramming remain unresolved.

Ruby covers modules/classes, reopen declarations as distinct source identities, instance/singleton/class methods, constructors, constants, literal require/require_relative dependencies and structural inheritance/mixin relationships. Explicit `end` scopes include anonymous control blocks so nested control flow cannot close declaration scopes accidentally. DSL/metaprogramming and dynamic dispatch remain intentionally unexpanded.

Swift covers imports, classes/structs/enums/protocols, extensions, functions/methods/properties/initializers, aliases/associated types and structural inheritance/conformance evidence. Extension members are attached structurally to the extended type without fabricating a second declaration; macro expansion, conditional compilation and compiler/type/package resolution remain outside this phase.

Pascal and Delphi use separate provider identities over shared case-insensitive line-oriented primitives. Coverage includes programs/units/packages, `uses`, interface/implementation sections, classes/records/interfaces/enums/aliases, fields/properties/constants/variables, procedures/functions/methods/constructors/destructors, forwards, overload modifiers, visibility, nested routines, Delphi generic qualified implementations and class/record helpers with structural relations. Compiler directives are lexically opaque and are not evaluated. Ambiguous `.pas`/`.inc` routing remains fail-closed unless independent evidence disambiguates it.

Phase 5 conformance covers false-positive resistance across strings/heredocs/metaprogramming/directives, malformed input, deterministic repeated analysis, cancellation, public mixed-language `source_symbols` routing/find behavior, strict 1,200-declaration resource-limit fixtures per provider, PHP UTF-16LE, Ruby Windows-1252, Swift UTF-16BE, Pascal IBM850+CRLF and Delphi Windows-1252+CRLF source decoding, vet/Staticcheck and analyzer fuzzing. Detector marker hardening also prevents line-oriented C#/VB.NET/Java/Kotlin evidence from crossing physical newlines into Pascal/Delphi section syntax.

Exit criterion: **met** — all original fourteen mandatory language families now have production declaration/navigation coverage through native analyzers, while stronger project/type/semantic/index claims remain withheld.

### Phase 6 — Basic, .NET and Classic ASP breadth — COMPLETE

Completed on 2026-08-14 with sixteen new production analyzer identities while retaining and extending the existing Classic ASP provider, bringing the mechanically generated capability matrix to 33 active providers. VB6, VBA, VBScript, QBasic, classic line-numbered BASIC, FreeBASIC, and PureBasic share bounded case-insensitive scanner/line primitives but retain distinct analyzer identities and dialect policies; ambiguous `.bas` routing remains fail-closed unless explicit or project evidence identifies the dialect. VB6/VBA module metadata, literal library/include dependencies, type/member/routine structure, QBasic line-number handling, and FreeBASIC/PureBasic dialect-specific container/dependency forms are structural only.

F#, C++/CLI, JScript.NET, CIL/MSIL, and PowerShell have separate Phase 6 providers. F# reports namespace/module/type/member/open/inheritance structure without compiler inference; C++/CLI reuses only proven C++ structure through offset-preserving CLR-modifier masking and reprojects fresh provider IDs; JScript.NET preserves package hierarchy and reconstructs literal import and inheritance evidence directly in host coordinates; CIL reports assembly/module/type/member structure; PowerShell masks here-strings and reports classes/functions/filters/fields/methods and literal `using module` dependencies without executing profiles or dynamic commands.

Classic ASP now delegates both VBScript and JScript server regions while retaining unsupported-language degradation. ASP.NET Web Forms delegates structurally declared C#/VB server-code regions; Razor and Blazor preserve host coordinates and analyze only balanced `@functions`/`@code` member regions; XAML reports quote-safe structural `x:Class`, `x:Name`, and XML namespace evidence. Generated partial classes, binding/render semantics, page lifecycle, runtime scripting, project/type resolution, semantic relations beyond proven structural edges, and incremental indexing remain outside this phase.

TDD/conformance covered legacy Windows-1252 and IBM850 CRLF inputs plus UTF-16 LE/BE Unicode fixtures, deterministic repeated analysis, malformed/cancellation/opaque-region boundaries, conservative `.bas` ambiguity, all 17 Phase 6 new-or-extended analyzers under generated 1,200-declaration symbol limits, and the public `source_symbols` path for eleven auto-routed formats plus six explicit shared-extension/syntax dialects. The terminal Phase 6 fuzz campaign executed 29,686 cases, retained 128 new interesting inputs (156 total), and completed without panic or invariant failure.

The original acceptance scope follows for auditability:

Extend the R25 Basic/composite foundation to:

- VB6;
- VBA;
- VBScript;
- QuickBASIC/QBasic;
- classic line-numbered BASIC families;
- FreeBASIC;
- PureBasic;
- F#;
- C++/CLI;
- legacy JScript.NET;
- CIL/MSIL;
- PowerShell;
- Classic ASP with VBScript and supported declared scripting regions;
- ASP.NET Web Forms, Razor/Blazor and XAML as source/composite formats where applicable.

Reuse shared Basic primitives but maintain dialect-specific keywords, declarations, file metadata, scoping and semantics. Do not claim one Basic dialect merely because another parses. Composite .NET formats must preserve host coordinates and delegate embedded C#/VB/etc. regions.

Exit criterion: the explicitly requested Basic/.NET/ASP scope has truthful per-dialect capability rows and representative real-world/legacy tests.

### Phase 7 — MQL4/MQL5 and C-like specialty languages — COMPLETE

Completed on 2026-08-15 with twelve distinct production analyzer identities, raising the mechanically generated capability matrix to 45 active providers. MQL4 and MQL5 remain separate providers while sharing only declaration-safe C++ lexical/structural machinery: functions, classes, methods, globals/inputs, event handlers, literal `#include`/`#import` dependencies and structural inheritance are retained under fresh MQL provider identities, while macro expansion, conditional-preprocessor evaluation, imported-binary semantics and trading runtime state remain explicitly unresolved. Shared `.mqh` routing remains fail-closed between MQL4 and MQL5 unless explicit/project evidence identifies the dialect.

Objective-C and Objective-C++ have distinct providers for `@interface`, `@protocol`, `@implementation`, `@property`, class/instance methods, initializer constructors, imports and structural inheritance/protocol conformance. Objective-C++ masks Objective-C declaration regions offset-preservingly before delegating the remaining compatible source to the C++ structural analyzer, then reprojects all retained facts under the Objective-C++ provider identity. `.m` remains ambiguous with MATLAB/Octave from extension alone and is selected as Objective-C only when independent source evidence corroborates it.

Dart, D, Zig, Nim, Solidity, Apex and AL use bounded native structural recognizers tailored to their declaration models; Arduino reuses only proven C++ declaration structure under its own provider identity. Coverage includes language-appropriate modules/imports, types/containers, functions/methods, fields/constants, constructors and reliable structural inheritance/mixin/implements/extension facts. Solidity additionally preserves contract/interface/library/event/modifier structure; Apex preserves trigger declarations; AL preserves namespace/object/procedure/extension structure. No provider in this phase claims compiler/build/project/type resolution, runtime evaluation, semantic relations or incremental indexing.

Phase 7 conformance covers deterministic decoded-source analysis across Windows-1252 CRLF, UTF-16 LE/BE and UTF-32LE fixtures; declaration-like comment/string negatives; malformed input and cancellation; conservative `.m` and `.mqh` ambiguity; generated 1,200-declaration resource bounds for all twelve providers; public `source_symbols` routing for distinctive extensions plus explicit shared-header dialects; and a terminal 24,880-execution fuzz campaign with 58 new interesting inputs and no panic/invariant failure. The shared scanner was also hardened so a longer block-comment opener wins over a shared line-comment prefix, preventing Nim `#[ ... ]#` contents from leaking declarations through the `#` line-comment rule.

The original acceptance scope follows for auditability:

Implement MQL4 and MQL5 as distinct analyzers sharing only proven C-like lexical/structural primitives. Cover functions, classes, structs, interfaces where supported by the dialect, enums, methods, constructors/destructors, globals/inputs, event handlers, `#include`, `#import`, macros and conditional preprocessing as structurally reliable.

Extend appropriate shared infrastructure to Objective-C/Objective-C++, Dart, D, Zig, Nim, Solidity, Apex, AL and Arduino source conventions. Objective-C `.m` detection must remain distinguishable from MATLAB/Octave through content/project evidence rather than extension alone.

Exit criterion: **met** — trading and specialty C-like languages reach the declared production structural bar without being mislabeled as C++ dialects.

### Phase 8 — dynamic, BEAM and scripting breadth — COMPLETE

Completed on 2026-08-15 with eleven distinct production analyzer identities for Perl, Lua, Luau, Elixir, Erlang, Gleam, Groovy, POSIX shell, Bash, Tcl and AutoHotkey, raising the mechanically generated capability matrix to 56 active providers. The implementation uses language-appropriate native scanners and structural recognizers rather than forcing one brace model: Perl package/substructure, Lua/Luau functions and Luau types, Elixir module/function blocks, Erlang module/record/function-clause structure, Gleam types/functions/constants, Groovy brace/OOP structure, shell/Bash functions, Tcl namespace/proc commands, and AutoHotkey classes/functions are normalized under distinct provider identities.

Structural dependency coverage includes literal/static Perl `use`/`require`, Lua/Luau `require`, Elixir alias/import/require/use, Erlang include/import/behaviour attributes, Gleam imports, Groovy imports, POSIX `.` and Bash `source`, Tcl source/package requirements and AutoHotkey includes. Dynamic targets, runtime dispatch, metaprogramming, macro/eval behavior, project/type resolution, semantic relationships and incremental indexing remain explicit limitations rather than inferred facts. POSIX shell and Bash stay separate providers: `.sh`/shell shebang routing remains Bash-compatible while the stricter POSIX `shell` provider is available through explicit selection.

Phase 8 conformance covers deterministic decoded-source analysis across Windows-1252/CRLF, UTF-16 LE/BE and UTF-32LE fixtures; language-specific opaque regions including Perl POD/data/heredocs, Lua long brackets, Elixir/Groovy multiline strings, Groovy slashy forms, shell here-documents/backticks and AutoHotkey escaped strings; malformed/incomplete coverage diagnostics; cancellation; generated 1,200-declaration resource bounds for every provider; conservative content markers and ambiguity behavior; static-versus-dynamic dependency targets; and public `source_symbols` routing across all auto-detectable Phase 8 formats plus explicit POSIX shell selection. A terminal fuzz gate completed 913,587 executions across all eleven analyzers, adding 514 new interesting inputs beyond the seed corpus with no panic or invariant failure. Shared Phase 7 brace-engine regressions remained green after the minimal Groovy masking/package extension.

The original acceptance scope follows for auditability:

Implement/complete Perl, Lua, Luau, Elixir, Erlang, Gleam, Groovy, POSIX shell/Bash, Tcl and AutoHotkey. Use native token/structural recognizers appropriate to each language; do not force brace-oriented machinery where it does not fit.

Dynamic dispatch and runtime metaprogramming limitations must remain explicit. Shell here-documents, quoting and function syntax require negative tests to prevent declaration leakage from strings/data blocks.

Exit criterion: **met** — these languages have production declaration/navigation coverage and structurally reliable imports/modules where the language exposes them, with dynamic/runtime limitations kept explicit.

### Phase 9 — scientific, legacy and functional breadth — COMPLETE

Completed on 2026-08-15 with twelve distinct production analyzer identities for Fortran, COBOL, Ada, MATLAB, Octave, Julia, R, Haskell, OCaml, Common Lisp, Clojure and Emacs Lisp, raising the mechanically generated capability matrix from 56 to 68 active providers. The implementation deliberately uses three native structural families instead of one universal parser: fixed/free source-line models for Fortran and COBOL, language-specific token/line recognizers for Ada and the scientific/ML-family languages, and balanced S-expression recognizers for the Lisp family.

Fortran supports free source plus fixed-form extensions with decoded Unicode-scalar column handling and continuation ranges preserved in original coordinates; COBOL distinguishes fixed and free source, indicator/inline comments, `PROGRAM-ID`, procedure sections and static `COPY` targets. Ada exposes packages, types, procedures/functions and static `with` dependencies. MATLAB and Octave retain separate provider identities and share only compatible line/scope primitives; generic `.m` remains ambiguous with Objective-C and between the two numerical dialects until distinctive evidence such as `classdef`, `endfunction` or Objective-C constructs corroborates one candidate. Julia covers modules, struct/type forms, functions, compact functions and macros structurally; R recognizes assigned functions and only dependency forms whose package target is statically justified. Haskell covers modules/imports, data/newtype/type/class forms, signatures and top-level bindings; OCaml distinguishes module/type/class, value and function bindings. Common Lisp, Clojure and Emacs Lisp use reader-aware balanced forms, suppress quoted/discarded declaration-like data where proven, and retain only structurally static dependencies; Clojure namespace parsing preserves every static `:require` vector.

Conformance covers fixed-form Fortran IBM850/CRLF, COBOL and Ada Windows-1252/CRLF, UTF-16 LE/BE and UTF-32LE modern-language fixtures, deterministic repeated analysis, malformed input with explicit incomplete coverage, cancellation, generated 1,200-declaration limits for all twelve providers, false-positive negatives for comments/quotes/reader forms, static-versus-dynamic dependency boundaries, conservative content detection and public heterogeneous `source_symbols` routing. The terminal fuzz gate executed 723,229 cases across all twelve analyzers, adding 470 new interesting inputs beyond the seed corpus with no panic or invariant failure.

The original acceptance scope follows for auditability:

Implement/complete Fortran, COBOL, Ada, MATLAB/Octave, Julia, R, Haskell, OCaml, Common Lisp, Clojure and Emacs Lisp.

Where fixed-format or line-oriented syntax makes regex/token recognizers the best implementation, use them after appropriate lexical/fixed-column validation and report structural capability only to the level proven by the recognizer. MATLAB `.m` remains detector-ambiguous with Objective-C until content/project evidence resolves it.

Legacy encodings and line-ending conventions are part of the quality gate for languages/ecosystems where real repositories commonly require them.

Exit criterion: **met** — legacy/scientific/functional entries have explicit, tested declaration/structure capability rather than text-only placeholders unless the format genuinely has no declaration concept.

### Phase 10 — data, infrastructure, hardware and document formats

Implement structural capabilities for SQL with dialect profiles, PL/SQL, GraphQL, HCL/Terraform, Nix, Protocol Buffers, VHDL, Verilog/SystemVerilog, Assembly dialects, HTML, XML, CSS/SCSS/Sass/Less, JSON, YAML, TOML, Markdown, OpenAPI and Ansible-oriented YAML.

These formats do not need fictitious programming-language semantics. Their capability rows should expose meaningful entities such as schemas/types/operations/modules/signals/entities/labels/selectors/sections/keys/resources depending on the format, plus dependencies/includes where structurally meaningful.

Exit criterion: the complete approved non-general-purpose catalog is routable and provides useful truthful structural navigation.

### Phase 11 — composite/template breadth

Extend `CompositeSegmenter` support to Vue, Svelte, Astro, PHP/HTML, JSP, Jinja, Twig, Blade, EJS, ASP.NET Web Forms and Razor/Blazor combinations not already complete.

Requirements:

- preserve original decoded-document coordinates;
- handle compound suffixes and nested/template language hints;
- use offset-preserving masking or direct region mapping rather than concatenating fragments and losing positions;
- delegate embedded script/style/server regions to registered analyzers;
- report unsupported embedded languages/regions explicitly;
- expose host/import/include relationships without pretending template expressions are project-resolved semantics.

Exit criterion: mixed-source files participate in outline/search/index queries without corrupting ranges or hiding unsupported regions.

### Phase 12 — project symbol tables and dependency resolution

Build project-wide resolution on normalized R25/R27 facts, not raw source rescanning for each query. Use deterministic structures such as:

- exact qualified-name hash maps;
- name-to-candidate indexes;
- per-file/module/scope symbol maps;
- forward and reverse dependency adjacency lists;
- exported/public-surface maps where the language permits reliable extraction.

Resolve in ordered stages appropriate to the language: local/enclosing scope, same file/type/module, explicit imports/aliases, dependency-constrained exported candidates, then broader project candidates. Preserve the stage/evidence that produced each edge. A unique broad-project candidate remains `project-resolved`/inferred unless stronger language rules justify semantic status.

Exit criterion: useful cross-file definitions/references work across the required ecosystem subset with deterministic ambiguity rather than name-guess certainty.

### Phase 13 — relations, graph algorithms, structural search and impact

Implement bounded graph/query features on normalized relations:

- dependencies/dependents through forward/reverse adjacency;
- callers/callees with syntactic/resolution evidence preserved;
- implementations/inheritance/overrides where justified;
- shortest trace/path through bounded BFS;
- impact/blast-radius through bounded forward/reverse traversal;
- cycles through Tarjan strongly connected components in `O(V + E)`;
- structural search over normalized symbols/tokens/relationships, complementing existing text grep rather than replacing it.

Every graph query must bound files, nodes, edges, depth, retained diagnostics and output. Cycles/recursion must never produce unbounded traversal.

Exit criterion: graph operations are deterministic, evidence-aware and available across multiple language ecosystems rather than one favored language.

### Phase 14 — bounded `source_query` context operation

Implement deterministic task-context assembly around selected symbols. The baseline priority should prefer:

1. selected target signatures/bodies;
2. enclosing type/module context;
3. direct imports/dependencies needed to understand the target;
4. directly resolved callees/dependencies with bodies while budget permits;
5. signatures for remaining direct relations;
6. callers/implementations/type relations;
7. deeper relations as signatures only.

Each retained item has a deterministic priority and estimated/actual output cost. Degrade from body to signature instead of silently dropping all context when the budget tightens. Re-read exact current source through authorized Scripthold paths and verify fingerprints before returning body ranges.

Exit criterion: agents can obtain task-focused context without embeddings and without loading entire projects.

### Phase 15 — incremental index generation

Introduce a deterministic incremental index only after source facts and relation semantics are stable. The baseline index stores metadata rather than duplicate complete source:

- canonical root/project identity;
- file content fingerprint;
- analyzer/configuration version fingerprint;
- project manifest/config fingerprints where resolution depends on them;
- language/dialect;
- symbols/ranges;
- imports/exports and normalized relationships;
- diagnostics/capability coverage;
- generation identity.

Reparse changed files by content fingerprint and invalidate affected dependents according to known dependency evidence. When the impact set cannot be proven narrowly, invalidate a larger scope explicitly rather than risk stale authority. Queries bind to one coherent generation; generation swap must not mix old/new edges.

Start process-local unless persistent storage is explicitly justified. Persistent storage requires its own protected-root, format, permissions, quota, corruption/recovery, cleanup and privacy review before implementation.

Exit criterion: warm incremental updates avoid unnecessary full rebuilds without sacrificing correctness or coherent-generation guarantees.

### Phase 16 — complete capability/corpus matrix

Run the reusable conformance harness across every approved target-catalog row. Programming languages must have representative positive/negative/malformed/declaration/scope/range tests; composite formats require region/delegation tests; document/config formats require meaningful structural tests. Add mixed-language repositories, ambiguous extensions, Unicode, UTF-8/16/32 and relevant legacy encodings, LF/CRLF, large/generated sources, cancellation and resource-limit cases.

The checked-in capability matrix must state any partial/unsupported advanced feature explicitly. Do not count a language as production-supported because a detector recognizes its extension.

Exit criterion: all target entries have truthful tested capability rows and every original mandatory language satisfies its stronger acceptance bar.

### Phase 17 — scale, race, security and connector gate

Benchmark and test:

- many small files;
- large generated files;
- mixed-language monorepos;
- cold source analysis/index;
- warm incremental refresh;
- exact/prefix symbol lookup;
- relation queries and bounded graph traversal;
- context assembly;
- highly ambiguous detection workloads.

Verify bounded CPU/memory/output behavior, cancellation, duplicate refresh suppression, concurrent read queries, generation swap, source changes during indexing, race-safe registry/index access, no hidden network/external execution, allowed-root enforcement and no source-content logging/leakage.

Run connector-level workflows demonstrating outline -> find/show -> relations -> context over heterogeneous projects without full-project source reads.

Exit criterion: the broad feature set remains bounded and useful at repository scale and does not weaken Scripthold's existing security/encoding guarantees.

### Phase 18 — documentation, completion and future handoff

Review the full diff and capability matrix. Update tool catalog, README/TOOLS, roadmap/history and language support documentation from authoritative registry data. Run all applicable focused and repository-wide gates, static/vulnerability checks, race/platform tests, fuzzing, documentation links/catalog drift checks, source smoke, `git diff --check` and final Git status.

R27 is not complete until the broad catalog, original mandatory-language gates, cross-ecosystem resolved relations, structural search/graphs, bounded context, and incremental-index requirements above are all satisfied or the maintainer has explicitly revised this document. R27 completion itself does not authorize commit, push, release, candidate build, runtime migration or deployment.

Exit criterion: repository evidence satisfies every R27 completion gate and no undocumented capability or language claim exceeds tested behavior.

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

### Risk: broad syntax coverage is mistaken for semantic intelligence

Mitigation: recognizing syntax/declarations does not automatically establish resolved references, implementations, or calls. Every relationship retains its actual evidence level and stronger project/semantic claims require the corresponding native resolver evidence.

### Risk: coverage pressure reintroduces external parser/runtime dependencies

Mitigation: R27's approved plan is native. Unsupported or partial capabilities remain explicit rather than being filled by an incidental language server, compiler process, downloaded grammar, or request-controlled execution path. Changing that boundary requires a later explicit architecture decision.

### Risk: persistent indexes leak source or become stale authority

Mitigation: content/provider/config fingerprints drive invalidation, storage is bounded/protected/versioned, source snippets are avoided unless explicitly approved, and queries bind to coherent index generations.

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
8. structural search, dependencies/dependents, callers/callees, implementations/inheritance, trace, impact and cycle queries are bounded and evidence-aware where the capability matrix claims them;
9. incremental invalidation is content-fingerprint-based and coherent-generation safe, and any persistent index has a separately reviewed protected storage boundary without duplicating complete source by default;
10. no external parser/compiler/language-server process or downloaded grammar/runtime was introduced outside an explicit later architecture decision;
11. encoding, decoded-coordinate mapping, allowed-root, cancellation, concurrency, transport, logging and resource invariants remain intact across heterogeneous and composite repositories;
12. complete language/composite corpora, malformed/negative tests, encoding/range tests, fuzz/resource/race tests, cross-language integration tests, scale benchmarks, connector smoke, static/vulnerability checks and supported-platform build/runtime gates pass as applicable.

## Relationship to future work

R27 establishes broad code intelligence, not automated code transformation. A later refactoring milestone may combine semantic identity from R27 with R23/R24 preview/apply mutation capabilities, but it must preserve exact-change approval, filesystem security, encoding behavior, and backup/partial-state guarantees rather than allowing semantic tools to mutate source directly.
