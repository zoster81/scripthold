# Scripthold Development Roadmap

This file tracks **current and future work only**. Completed changes belong in [`CHANGELOG.md`](../CHANGELOG.md); current behavior and invariants belong in [`ARCHITECTURE.md`](ARCHITECTURE.md), [`TOOLS.md`](../TOOLS.md), and the focused operational guides.

## Current state

- Public releases: see [GitHub Releases](https://github.com/zoster81/scripthold/releases/latest); **4.0.1** was released through the protected immutable release process on 2026-10-26; post-release semantic fixes remain unreleased.
- Current public capabilities: **36 tools**, **3 guided prompts**, **168 registered text encodings**, and **100 active source-intelligence providers**.
- Completed release changes belong in the [Changelog](../CHANGELOG.md), and the [4.0 migration guide](MIGRATION_4.0.md) covers the unified confirmation tool. Publishing and installing are separate; an active server does not change until it is explicitly updated.

## Planning rules

- At most one release-scoped milestone may be active at a time.
- Define the user-visible outcome, compatibility boundary, security impact, and completion gate before implementation.
- Preserve current filesystem, encoding, mutation, backup, execution, transport, and source-intelligence boundaries unless a milestone explicitly changes them.
- Keep public MCP concepts compact and difficult to misuse.
- Require an exact clean commit and the release procedure in [`PUBLISHING.md`](PUBLISHING.md) for public release authority.
- Do not accumulate implementation diaries or per-phase evidence in this file.

## Active milestone — Source Intelligence structural reliability

**Purpose:** Give an LLM a concise, dependable first map of an unfamiliar project, then let it locate and inspect only the source relevant to its task. Source Intelligence is a set of **read-only evidence and navigation tools**, not an agent, compiler or semantic decision engine. The **LLM** interprets the user request, selects queries, evaluates relevant files and decides what to inspect or provisionally exclude. **Scripthold** returns bounded source facts, locations, supported relations, exact excerpts and honest limitations; it neither chooses the edit site nor judges relevance to the user's task. The agent remains free to use `source_*`, `grep_text_files`, or direct reads; spontaneous tool selection is not a qualification target.

**Released baseline (4.0.1):** `source_symbols` provides `digest`, `outline`, `find`, and fingerprint-verified `show`; `source_query` provides supported structural search, relationships, and context. `outline` defaults to `presentation: "tree"`; callers needing the historical top-level `files` and `symbols` request `presentation: "detailed"`. The separate filesystem `tree` tool describes paths, not source-symbol ownership. See the [tool reference](../TOOLS.md#source_symbols) and [capability matrix](LANGUAGE_CAPABILITIES.md) for the actual contracts. Post-release parser fixes in Git are not proof that an already running connector has been updated.

### Current original-project qualification

**Progress in the development tree (not the installed 4.0.1 release):**

| Gate | Verified | Remaining | Meaning |
|---|---:|---:|---|
| Initial task-oriented navigation | **100 / 100 providers** | 0 providers | At least two independent original-project maintenance scenarios per qualified provider (201 selected provider/project cases, including a third Classic ASP case). |
| Midpoint independent-project audit | **100 / 100 additional cases** | 0 cases | Two *further* independent original projects for each of the first 50 providers; all 50 pairs passed bounded source navigation. |
| Second-half independent-project audit | **84 / 100 additional cases** | 16 cases | Two further independent original projects per completed provider 51–92; Nix navigation has explicitly partial coverage. |
| Retrospective selected-file declaration audit | **22 / 22 project cases** | 0 cases | 77 original files and 1,899 independently inventoried supported declarations (see table below); not a complete language grammar. |
| Standalone MCP / cross-platform promotion | Deferred | Not assessed | Requires a separate authorized verification stage after the source-level qualification. |

**What the gates establish.** The initial 100-provider campaign and the first 50 providers' additional-project audit are complete. The second-half audit has verified two *further* independent original projects for each provider 51–92, distinct from the initial project origins. Completed providers (canonical registry IDs): `python`, `go`, `csharp`, `php`, `swift`, `sql`, `graphql`, `terraform`, `haskell`, `toml`, `scala`, `r`, `zig`, `rust`, `erlang`, `fsharp`, `vbnet`, `gleam`, `al`, `blazor`, `razor`, `twig`, `jinja`, `blade`, `ejs`, `arduino`, `openapi`, `ansible-yaml`, `ocaml`, `autohotkey`, `julia`, `nim`, `nix`, `xaml`, `proto`, `plsql`, `cil`, `jscript-net`, `flow`, `aspnet-webforms`, `classic-asp`, and `cpp-cli`. Providers 93–100 still require two additional original projects each.

### Qualification method and evidence limits

**Project-navigation checks** begin with a realistic maintenance question and complete reads of the selected original sources *before* querying the development handler. Immutable upstream revisions, independent grep/source oracles, plausible neighboring and unrelated files, bounded digest/find/query responses, fingerprint-bound excerpts and truthful coverage are checked together. A PASS certifies only the tested navigation task, not an entire grammar, hidden dependencies, program execution or an LLM's choice to call the tool. The [project-first checklist](DEVELOPMENT_CHECKLIST.md#source-intelligence-project-first-qualification) defines the reusable acceptance criteria. Local original snapshots remain outside tracked files; provider-specific Go tests and Git history retain implementation and verification evidence.

**Retrospective selected-file declaration inventories** are a separate, stricter gate: 22 project cases, 77 fully read original files and 1,899 independently annotated declarations within the stated supported structural syntax. These inventories compare the complete detailed outline against original-source names, native kinds, owners, lines, duplicate/conditional alternatives and negative non-declarations. They are **not** whole-repository or whole-language certification.

| Independently inventoried source scopes | Selected files | Supported declarations |
|---|---:|---:|
| zoxide / ripgrep (TOML) | 7 | 201 |
| Oracle Sample Schemas / Logger (SQL) | 5 | 20 |
| Hasura / Kubespray (Terraform) | 9 | 94 |
| GraphQL Go / Hasura (GraphQL) | 5 | 65 |
| MediatR / Humanizer (C#) | 7 | 148 |
| attrs / FastAPI (Python) | 4 | 132 |
| optparse-applicative / Aeson (Haskell) | 8 | 333 |
| CodeIgniter / WordPress (PHP) | 8 | 101 |
| fsnotify / Gorilla mux (Go) | 8 | 443 |
| Swift compiler / Exercism (Swift) | 8 | 51 |
| Scala XML / os-lib (Scala) | 8 | 311 |
| **Total** | **77** | **1,899** |

Provider-level repairs already completed in the development tree are documented in the [Unreleased changelog](../CHANGELOG.md#unreleased). In particular, conditional branches, nested and local declarations, multiline syntax, embedded languages and owner relationships are projected only where specifically supported; a selected inventory does not certify all such forms. The [language capability matrix](LANGUAGE_CAPABILITIES.md) describes registered features, not proof of exhaustive syntax recognition.

### Shared parser families for targeted repairs

Prioritize **shared implementations across the full provider registry**, not an alphabetical language list or a fixed C-then-BASIC queue. Compare the reach of common decoder, scanner, logical-line, symbol-builder, conditional, and handler/index code before choosing where to work. A shared repair requires independent defects and regression checks in its affected consumers; similar syntax alone is not sufficient evidence. Choose the next family by demonstrated severity, reuse potential, available original projects, and verification cost.

The source tree currently contains these **confirmed shared implementation seams** (examples, not a complete grouping of all 100 providers):

| Shared implementation | Direct consumers or composition |
|---|---|
| `analyzeCFamily` | C and C++; C-like relatives also need individual dependency tracing |
| `analyzeBasicDialect` | VB6, VBA, VBScript, QBasic, classic BASIC, FreeBasic, PureBasic |
| `analyzeECMAScriptDialect` | JavaScript, TypeScript; Flow also composes the JavaScript-family analysis |
| `analyzeJVMFamilyScope` | Java, Kotlin; JSP reuses Java member parsing in embedded regions |
| `analyzePascalFamily` | Pascal, Delphi |
| `analyzeLispSource` | Common Lisp, Clojure, Emacs Lisp |
| `analyzeLuaFamily` | Lua, Luau |
| `analyzeShellFamily` | Shell, Bash |
| `analyzeMATLABLike` | MATLAB, Octave |
| `analyzeHDLSource` | Verilog, SystemVerilog; VHDL is separate |
| `analyzeObjectiveCFamily` | Objective-C, Objective-C++ (with additional C++ composition) |
| `analyzeMQL` | MQL4, MQL5 (with C-family-related components) |
| `analyzeMarkupDocument` / `analyzeCSSDocument` | HTML/XML and CSS/SCSS/Less, respectively; Sass remains separate |
| `analyzeBraceLanguage` | Apex, D, Dart, Groovy, Solidity (dialect policies still differ) |
| `analyzeScriptStyleHost` | Astro, Svelte, Vue (composite hosts with distinct regions) |
| `analyzeRazorFamily` / `analyzeTemplateHost` | Razor/Blazor and Jinja/Twig, respectively |
| Composite-host and .NET member-region helpers | Vue, Svelte, Astro, PHP/HTML, JSP, EJS, Blade, Jinja/Twig and ASP.NET/Razor/Blazor integrations have distinct embedded-language boundaries |

The shared implementations above identify useful regression boundaries, not a complete dependency graph. Confirm affected callers and provider composition against the current source before applying a family-wide change.

### Source Intelligence concise response contract — on-demand qualification

All seven opt-in `presentation: "concise"` projections are implemented in the development tree: `source_symbols` (`digest`, `outline`, `find`, `show`) and `source_query` (`search`, `relations`, `context`). Legacy defaults and public formats are unchanged. The installed 4.0.1 server does not automatically include these unreleased changes. For inputs, exact output behavior, limits and examples, use the [source response contract](../TOOLS.md#source-intelligence-response-contract-development-opt-in).

Selected public-MCP client checks have passed on two independent original projects each for Java, Go and Python, including safe source drilldown; this is **not cross-provider certification**. Further seven-format comparisons are **deferred during normal navigation qualification** and required only for a response-affecting change, demonstrated usability issue, compatibility decision, or proposed default migration. Preserve correct evidence, honest partial/unsupported states, full safe selectors, and historical-client compatibility; follow the [on-demand response checklist](DEVELOPMENT_CHECKLIST.md#source-intelligence-response-contract-qualification-on-demand).

### Known limitations and verification boundaries

These caveats describe **current development-tree behavior and unresolved work**, not a list of completed language campaigns. Resolved parser changes belong in the [Unreleased changelog](../CHANGELOG.md#unreleased); the [capability matrix](LANGUAGE_CAPABILITIES.md) and [tool reference](../TOOLS.md#source_symbols) describe supported navigation.

- **Selected scope is not full grammar coverage.** Even a completely inventoried selected file does not prove unrelated files, every language construct, full ownership semantics, inferred relations, or runtime correctness. In GraphQL, directive definitions, enum value members and fragment definitions are not projected as declarations, and anonymous operations, arguments and query selections are likewise excluded from the selected supported-declaration inventory. A successful `coverageComplete` result means the requested files were processed within the analyzer's declared scope, not that all declarations, call sites, or dependencies were found. Some larger original Lua and D files, MQL4 server scripts, and complex VHDL forms remain partially supported. A shell script with no function declarations can legitimately return no symbols. Keep partial or excluded files distinct from passing maintenance scenarios.
- **Document/configuration providers have narrower indexes.** Nix returns supported `let` bindings but does not index root or result attribute sets and reports incomplete coverage. Protocol Buffers indexes named messages, enums, services, fields and RPCs within its supported syntax; enum values and `extend` declarations are not indexed, even when selected-file scanning reports complete. OpenAPI operation navigation currently requires an explicit `operationId`; endpoints without one are not indexed as operations. Ansible `import_playbook` references are not followed or treated as tasks. These tools do not evaluate Nix expressions, resolve OpenAPI references, or execute imported playbooks.
- **Large Zig source files may remain partially indexed.** In the original zmx project, `src/main.zig` reports incomplete selected-file coverage. The qualified navigation task uses the narrower configuration, POSIX helper, test-entry, and unrelated-label scope, with an independent direct read of the main entry point; it does not claim complete indexing of the larger file.
- **Some component forms remain conservatively partial.** Vue 2 Options API methods and computed properties (for example `sortgroupcity`) remain an unverified current-HEAD gap. Svelte 5 direct object-destructured `$props` bindings (for example `copyCode`) are now indexed in the development source, including aliases, defaults and rest bindings; nested/computed patterns are not fully inferred. The selected Vue/Svelte tasks do not prove complete component APIs or browser behavior.
- **Some language constructs are intentionally conservative.** Scala XML's selected 87-declaration audit covers package, type and type-member declarations, not function-local declarations (including local classes/objects and nested functions), macro expansion or Scala compiler semantics. Swift enum cases and directly nested local function declarations are now indexed, but local declarations inside deeper control-flow or closure scopes, arbitrary function-local variables, conditional compilation semantics and full Swift grammar are not certified; `coverageComplete` must not be interpreted as compiler validation. Nested or macro-generated Common Lisp/Clojure/Emacs Lisp forms, advanced D and Lua syntax, complex VHDL types, ambiguous Ruby method bodies, preprocessor directives, generic/type inference, and dynamic dispatch are not universally resolved. Do not manufacture symbols, ownership, callable bodies, or project-wide dependencies where analysis lacks evidence.
- **Structural links are not execution semantics.** Adjacent Go/TypeScript/R/PHP code, XML/MSBuild imports, GraphQL resolvers, stylesheet consumers, HTML code-behind, and configuration overrides may be inspected directly, but providers do not automatically establish every cross-file semantic relationship. No original-project execution, compilation, database query, cloud operation, UI rendering, numerical simulation, or runtime correctness is certified by this milestone.
- **Development and installed servers differ.** An installed 4.0.1 connector may predate Git changes. Confirm discrepancies against current development handlers rather than treating an older result as a current-HEAD defect. Fingerprint-bound `show` can correctly refuse oversized source regions with `LIMIT`; narrow the requested declaration or use an authorized targeted source read instead of silently raising resource bounds.

### Next work

1. **Complete the independent-project audit:** Continue with **provider 93**, then providers 94–100: two further independent original projects per provider, **16 cases remaining**. Use the [project-first checklist](DEVELOPMENT_CHECKLIST.md#source-intelligence-project-first-qualification), maintain private source provenance and compare navigation against independent original-source evidence. Do not confuse partial navigation with complete syntax coverage.
2. **Repair proven navigation defects:** Reproduce misleading or missing edit targets on development HEAD before changing a parser. Prioritize fabricated symbols or links (P0), missed significant declarations and misleading completeness (P1), then inefficient but correct navigation (P2); do not chase unsupported compiler semantics without a demonstrated need. The Vue 2 case above remains a candidate, not a confirmed new development-HEAD regression.
3. **Keep response and release gates separate:** Recheck concise output only when behavior or compatibility changes, using the [on-demand checklist](DEVELOPMENT_CHECKLIST.md#source-intelligence-response-contract-qualification-on-demand). Standalone MCP, transport, cross-platform and release qualification require separate authorization. Do not execute original third-party projects.

The published contract is defined by the [4.0.1 changelog](../CHANGELOG.md#401---2026-10-26), [tool reference](../TOOLS.md#source_symbols), and generated [language capability matrix](LANGUAGE_CAPABILITIES.md). For release authority and cross-platform checks use [Publishing](PUBLISHING.md); source navigation tests alone are not release certification.
