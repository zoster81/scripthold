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

**Purpose:** Help an agent understand unfamiliar source code without first reading every complete file. Source Intelligence must provide accurate, bounded declarations, ownership, hierarchy, project overviews, and targeted source access. The agent remains free to choose `source_*`, `grep_text_files`, or direct reads; spontaneous client tool selection is **not** a qualification target.

**Released baseline (4.0.1):** `source_symbols` provides `digest`, `outline`, `find`, and fingerprint-verified `show`; `source_query` provides supported structural search, relationships, and context. `outline` defaults to `presentation: "tree"`; callers needing the historical top-level `files` and `symbols` request `presentation: "detailed"`. The separate filesystem `tree` tool describes paths, not source-symbol ownership. See the [tool reference](../TOOLS.md#source_symbols) and [capability matrix](LANGUAGE_CAPABILITIES.md) for the actual contracts. Post-release parser fixes in Git are not proof that an already running connector has been updated.

**Remaining acceptance gates, in dependency order:**

1. **Reconcile project evidence.** Use the authoritative real-project qualification ledger for the 100 active source providers, targeting at least eight independent qualified projects per provider where available. Reconcile selected-question reports against original project identities and preserve excluded, missing, failed, and historically qualified cases separately. Do not execute third-party project code.
2. **Establish independent structural oracles.** Select representative real files, classes, modules, and cross-file questions. Derive complete expected declarations for each selected scope from original text, recording required **and forbidden** symbols, kinds, names, source positions, ownership, and supported relationships. A single correct declaration does not qualify a whole file or provider.
3. **Qualify symbols and hierarchies.** Check full selected-file and selected-class inventories, including nested members and variable/function distinctions. Verify `source_symbols outline` in tree, compact, and detailed presentations; cross-check counts, parent identifiers, qualified names, and stable ordering. A partial or truncated result must never appear exhaustive.
4. **Qualify project navigation.** Check `digest` file maps and entry points, filesystem `tree` where used, `find`/`source_query search`, supported dependencies and relations, and exact `show`/`context` source retrieval. Verify fingerprints and freshness. A complete file scan does **not** establish complete callers or references; unindexed relationships cannot prove absence.
5. **Compare with independent grep/read evidence.** Ask the same navigation question over the same eligible project scope using bounded `source_*` and `grep_text_files` plus targeted reads. Judge both against the independent oracle, **not** by treating every grep hit as a declaration. Record true findings, false positives, omissions, coverage/truncation, relevant structured information, tool calls, output bytes, and latency/memory only when measured. Preserve cases where grep/read is more accurate or efficient.
6. **Fix reproducible defects.** Separate development HEAD from the installed connector and documented unsupported syntax. For confirmed defects, capture a focused failing regression (including relevant original-source evidence), compare minimal implementation alternatives, fix without project-specific exceptions, then verify focused and adjacent behavior. Cover LF/CRLF, BOM/encoding, malformed/oversized inputs, ambiguity, and bounded resource use when applicable.
7. **Complete provider and regression qualification.** Repeat representative positive, negative, hierarchy, and project-navigation cases for each declared provider capability. Run risk-appropriate regression, static, race, safety, and cross-platform checks before proposing promotion. Report remaining limitations honestly; do not claim universal grammar correctness from sampled results.
8. **Final standalone MCP qualification (deferred).** Only after project-first structural and handler tests are sufficiently complete, run the separately authorized standalone local MCP server and transport checks. This is deliberately deferred; do not start or restart servers during ordinary source-analyzer development.

**Qualification scope:** A successful selected question or `coverageComplete: true` does not prove a complete language grammar, full project hierarchy, or all callers and dependencies. Ruby has 18 complete independently annotated selected files; Groovy and Solidity each now have one fully annotated selected original file, while the other project comparisons check representative expected and forbidden declarations. None proves that an installed connector contains post-release fixes, and final cross-platform, promotion, and standalone-MCP qualification remain deferred.

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

**Current inventory evidence:** all 100 registered providers resolve to a factory and an `Analyze` implementation. Static Go-AST inspection and direct call-site checks identify 18 `analyze*` entry points shared directly by 47 provider implementations, 13 registered composite-template hosts, and at least five explicit cross-analyzer delegations (including Flow to TypeScript and C++/CLI to C++). These call-graph counts are a verified starting subset, not exhaustive transitive dispatch or scanner/lexer sharing.

### Current original-project qualification

The original-project acquisition ledger records at least eight accepted independent projects for each of the 100 registered providers; this external qualification corpus is not part of the public release. **Acquiring a project, completely reading a selected file, checking sampled declarations, and qualifying a complete scope are different stages.** The table reports the actual development Go MCP-handler checks; symbol totals are *returned counts*, not independent proof that all real declarations were found.

| Provider | Independent projects with selected original files read | Verified development-handler scope | Still open |
|---|---:|---|---|
| Ruby | 8 (18 files) | Complete independently annotated selected scopes: 156 supported declarations | Dynamic Ruby forms, complete project relations, broader grammar |
| QBasic | 8 | Selected positive/negative declarations, owners and positions; 192 symbols returned | Exhaustive selected-file inventories and project navigation |
| Classic BASIC | 8 | Selected positive/negative declarations, owners and positions; 625 symbols returned | Exhaustive selected-file inventories and project navigation |
| Apex | 8 | Selected names, kinds, positions and forbidden symbols; 47 returned | Full inventories, all navigation modes and cross-file relations |
| D | 8 | Selected names, kinds, positions and forbidden symbols; 150 returned | Full inventories and relations; one source reports incomplete coverage for dynamic mixins |
| Dart | 8 | Selected names, kinds, positions and forbidden symbols; 94 returned | Full inventories, all navigation modes and cross-file relations |
| Groovy | 8 | One complete independently annotated selected-file inventory: 8 declarations validated through the development Go MCP handler, including hierarchy and selected navigation operations; other projects sampled | Seven more fully annotated original selected-file scopes; complete relations and project navigation |
| Solidity | 8 | One complete independently annotated selected-file inventory: 8 declarations validated through the development Go MCP handler; other projects sampled | Seven more fully annotated original selected-file scopes, all navigation modes and relations |

**Next focus:** Deepen the eight already-read Groovy and eight Solidity independent originals to fully annotated selected-file/class inventories, hierarchy and multi-file navigation, then apply the same qualification gate to **every** sampled provider. Keep comparing the connected runtime, development analyzer, actual Go handlers, and grep/read on the same questions. A fresh defect must be reproduced against current development code before changing a shared parser. Continue tracing common scanner, logical-line, conditional, symbol-builder and source-query/index boundaries across **all 100 providers**, including those not listed above. Languages with similar syntax do not necessarily share implementations; a family fix never replaces the individual provider gate.

**Priority:** P0 — fabricated or misclassified symbols, incorrect ownership, or wrong source bodies; P1 — missing declarations, incomplete hierarchy/outline/digest, or wrong paths/relationships; P2 — misleading completeness, ambiguity, encoding, freshness, or nondeterminism; P3 — unnecessary output, latency, or memory cost. Address P0/P1 first; run independent project checks in parallel only where they do not share mutable qualification inputs.

**Repeatable method:** Follow the [project-first Source Intelligence qualification workflow](DEVELOPMENT_CHECKLIST.md#source-intelligence-project-first-qualification). Independently label original source, compare the connected MCP runtime, development analyzer and an actual local handler test when run, and check grep/read under identical conditions. Record which boundaries remain untested.

**Completion rule:** Verified selected scopes must contain no unexplained false declarations, misclassifications, ownership errors, or unjustified omissions. Unsupported semantics and partial coverage must remain explicit. The paired comparison must show how much **correct, useful information** each method returns and at what measured cost; fewer calls by themselves are not success. Avoid new public tools, breaking schemas, persistent indexes, and external language servers without separate evidence-based approval.

**Published baseline:** The [4.0.1 changelog](../CHANGELOG.md#401---2026-10-26), [tool reference](../TOOLS.md#source_symbols), and generated [language capability matrix](LANGUAGE_CAPABILITIES.md) define released behavior. Cross-platform CI does not certify semantic correctness for every language. Releases follow [Publishing](PUBLISHING.md).
