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

**Milestone acceptance:** Use the [project-first qualification checklist](DEVELOPMENT_CHECKLIST.md#source-intelligence-project-first-qualification) for the complete repeatable workflow. The active gate requires:

1. **Independent original-project evidence.** Start from a real maintenance question and source-first oracle on each of two different projects per provider. The historical corpus is a source of candidates, not current PASS evidence.
2. **Useful, bounded navigation.** Verify compact source maps, targeted declarations, exact fingerprint-bound excerpts, directly evidenced neighbors/tests and at least one plausible distractor against independent grep/read results. Scripthold supplies evidence; the LLM decides relevance.
3. **Truthful coverage and corrections.** Keep missing, partial, unsupported and misleading findings visible. Fix confirmed development-HEAD defects with targeted failing tests, a minimal shared-safe change and affected-family regressions. Selected navigation PASS is not proof of complete grammar or correct future edits.
4. **Separate final qualification.** Standalone MCP, transport, cross-platform and release checks remain deferred until explicitly authorized; do not execute third-party original projects.

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

The shared implementations below identify useful regression boundaries, not a complete dependency graph. Confirm affected callers and provider composition against the current source before applying a family-wide change.

### Current original-project qualification

**Progress in the development tree (not the installed 4.0.1 release):**

| Gate | Verified | Remaining | Meaning |
|---|---:|---:|---|
| Initial task-oriented navigation | **50 / 100 providers** | 50 providers | Two independent original-project maintenance scenarios per qualified provider (100 selected provider/project cases). |
| Midpoint independent-project audit | **60 / 100 additional cases** | 40 cases | Two *further* independent projects for each of the first 50 providers. So far, 30 providers have passed both additional cases. |
| Standalone MCP / cross-platform promotion | Deferred | Not assessed | Requires a separate authorized verification stage after the source-level qualification. |

**The original 50-provider milestone is frozen.** Ordinary qualification pauses before provider 51 until all 100 additional audit cases have been reviewed. Additional cases must use independent original project origins, not recycled examples from the first two projects. An acquired project, a successful symbol lookup, a complete bounded scan and a navigation PASS are different evidence levels. Pending does not mean failed.

**Providers with initial two-project navigation PASS** (canonical registry IDs, grouped only for readability):

- `vba`, `vb6`, `vbscript`, `qbasic`, `classic-basic`, `freebasic`, `purebasic`, `pascal`, `delphi`, `shell`
- `bash`, `typescript`, `javascript`, `flow`, `java`, `kotlin`, `jsp`, `lua`, `luau`, `matlab`
- `octave`, `verilog`, `systemverilog`, `vhdl`, `objective-c`, `objective-cpp`, `solidity`, `apex`, `groovy`, `dart`
- `d`, `c`, `cpp`, `ruby`, `clojure`, `emacs-lisp`, `common-lisp`, `mql4`, `mql5`, `html`
- `xml`, `css`, `scss`, `less`, `sass`, `powershell`, `vue`, `fortran`, `svelte`, `astro`

**Additional audit completed for both new projects:** `vba`, `vb6`, `vbscript`, `qbasic`, `classic-basic`, `freebasic`, `purebasic`, `pascal`, `delphi`, `shell`, `bash`, `typescript`, `javascript`, `flow`, `java`, `kotlin`, `astro`, `jsp`, `lua`, `luau`, `matlab`, `octave`, `verilog`, `systemverilog`, `vhdl`, `objective-c`, `objective-cpp`, `solidity`, `apex`, `groovy`. These 60 PASS results were checked against independently read source and the current development Go MCP handlers. The exact selected projects, revisions and individual results are retained in the private qualification evidence; testable behavior is represented by the original-project handler tests under `filetoolsserver/handler`. Do not copy private workspace state or handoff details into tracked documentation.

### Audit method and evidence boundary

For each new project, select a realistic maintenance question **before** querying `source_*`. Independently inspect source and relevant tests or configuration, identify the likely primary edit area and a plausible unrelated alternative, then check a bounded compact `source_symbols digest`, targeted `find` / structural `source_query`, and fingerprint-bound `show`. Compare the answers with `grep_text_files` and direct reads. Record correct and misleading findings, omissions, partial/truncated coverage and measured costs only when actually collected. A provider's reported symbol count or `coverageComplete: true` is **not** an independent declaration inventory or a guarantee about unindexed callers.

Mark an additional case **navigation PASS** only when the supported tools provide useful, source-verifiable evidence for the selected edit region, adjacent implementation/tests and a distinguishable distractor. Keep `not tested`, `screening only`, `FAIL` and `blocked/unsupported` separate. A qualified scenario does not certify the whole project's behavior, complete grammar, every dependency, or the LLM's eventual edit. Never execute third-party original projects. The reusable [project-first qualification checklist](DEVELOPMENT_CHECKLIST.md#source-intelligence-project-first-qualification) is the procedural authority; do not maintain a parallel test diary here.

**Important remaining limitations:**

- **Mixed template files:** The current development branch now excludes false HTML/JSP IDs arising from URL query parameters and nested template fragments inside quoted attributes; focused RED/GREEN tests and independent JSP project tests protect the corrected behavior. Dynamic attributes, script execution and complete template semantics remain outside the structural navigation guarantee.
- **Lua/Luau scope boundaries:** Selected cross-file navigation on additional original Lua projects passes, but other advanced Lua file regions reported incomplete analysis and were not credited as fully covered. Additional Luau benchmark-statistics and Markdown-renderer tasks pass in explicitly selected Luau files; adjacent shell scripts were checked through direct source reads, not parsed as Luau. A task-scoped PASS does not certify every file or construct.
- **MATLAB selected scopes:** Additional project navigation can locate dataset-classification flags, model-selection helpers and separately authored threshold scripts, and distinguish exact/approximate coordinate converters from unrelated UTM functions. It does not certify MATLAB execution, script-generated values, numerical correctness or the complete project.
- **HDL module/interface declarations:** Development analyzers now locate comma-continued Verilog/SystemVerilog `#(...)` parameters and supported packed struct types inside modules and interfaces. Recognized struct fields are no longer mislabeled as enclosing signals. Verilog numeric apostrophes do not turn adjacent comments into quoted text. Original Verilator and UART project cases, earlier original HDL projects, LF/CRLF and allocation-budget tests cover these boundaries. Source navigation still does not perform preprocessing, elaboration or simulation.
- **VHDL architecture types:** The development analyzer now locates complete, single-line `type ... is range ...;` declarations in an architecture's declarative prefix, with exact architecture ownership and offsets. It does not assign process/subprogram-local types to the enclosing architecture or claim full coverage of package types, complex type declarations, or declarations after local subprograms. Independent original FPGA Webserver and GCVideo tasks, earlier VHDL originals, and LF/CRLF negative tests cover the selected navigation.
- **Objective-C selector keywords:** Development indexing preserves selector labels such as `and:` even when the shared C++ scanner classifies the same spelling as a keyword. This prevents misleading argument-name selectors in Objective-C and Objective-C++, including single-line and multiline declarations. Independent original retain-cycle-filter and Unity WebView tasks, exact lookup, fingerprint-bound excerpts, structural search and negative tests cover selected scopes; the tools do not perform runtime callback resolution or app execution.
- **Solidity contract ownership:** Current development handlers preserve Aave V3 named-import contract/interface owners and correctly attach flash-loan fee controls to their contracts; an older installed connector can instead return unowned functions despite reporting complete selected-file coverage. Original Aave V3 and AirCash maintenance scenarios now have task-scoped tests for bounded maps, named declarations, exact excerpts, structural search and alternatives. Neither contract deployment nor financial behavior is verified.
- **Groovy annotated types:** The development parser now skips package and import headers before identifying real class declarations. Without this boundary an annotation with array arguments, such as `@SuppressWarnings([...])`, could be falsely indexed as a callable and hide the following class despite complete reported coverage. Independent protobuf Gradle plugin and AspectJX original-project tasks and LF/CRLF regressions cover the corrected selected scopes; Gradle builds and plugin execution were not performed.
- **Shell scripts without declarations:** A complete scan may legitimately return zero function symbols for a command-only script; that is not an executable-command map.
- **Macro and dynamic semantics:** Preprocessor expansion, dynamic dispatch, compiler/type resolution, and complete transitive call graphs are not navigation guarantees. Historical C/C++ file inventories include explicitly partial results; do not reinterpret these as full-project semantic certification.
- **Development versus installed runtime:** Post-release fixes on the current branch do not update an already running connector. Confirm a reported discrepancy on development HEAD before classifying it as a product defect. For example, development Go handlers locate explicit Verilog TDMA period parameters in an additional original project even where an older installed connector omits them. A fingerprint-bound `show` of a module larger than the configured byte limit correctly returns `LIMIT`; use targeted parameter declarations and direct source reads instead of silently raising that bound.

### Next work

1. **Finish the midpoint audit (active):** Continue with Dart and the remaining untested providers using independent original projects. Validate source-first navigation against current development HEAD, and correct misleading declarations through focused RED/GREEN tests and affected-family regression. If a candidate offers no meaningful navigable source, choose another independent accepted project and preserve the replacement rationale in private evidence rather than forcing a PASS.
2. **Resume provider qualification at 51 / 100** only after the first 50 providers have completed their two additional independent-project cases. Select the next language or shared analyzer using observed defects, impact and evidence availability, not a permanently fixed alphabetical queue. Reuse historical corpus inventories rather than recrawling entire projects without need.
3. **Escalate defects by user impact:** P0 misleading or invented symbols, owners, excerpts or relationships; P1 missing important edit targets, incorrect locations or deceptive completeness; P2 overly large first responses or other bounded-efficiency gaps; P3 compiler-exact semantics without demonstrated navigation benefit. Protect encoding, path authorization, fingerprints and public schema compatibility for every fix.
4. **Qualify the standalone server separately when authorized:** After source and handler evidence is sufficient, run the deferred transport and cross-platform promotion/release checks. Ordinary repository work must not restart a deployed server or execute third-party projects.

The published contract is defined by the [4.0.1 changelog](../CHANGELOG.md#401---2026-10-26), [tool reference](../TOOLS.md#source_symbols), and generated [language capability matrix](LANGUAGE_CAPABILITIES.md). For release authority and cross-platform checks use [Publishing](PUBLISHING.md); source navigation tests alone are not release certification.
