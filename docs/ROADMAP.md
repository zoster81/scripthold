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

**Current qualification boundary:** Ruby has passed the development-handler structural acceptance gate on independently annotated complete scopes from eight real projects (18 original source files, 156 supported declarations), with regression tests and explicit non-declaration cases. This qualifies **those selected scopes**, not every Ruby grammar construct or an updated installed server. Dynamic metaprogramming, some nested/endless body shapes, and unresolved runtime relationships remain outside claimed semantic coverage; final cross-platform, race, promotion, and standalone-MCP checks remain deferred. The registry/factory inventory is complete for all 100 providers; detailed shared-boundary tracing and independent per-provider semantic qualification remain open. Do not reopen Ruby unless new independent evidence identifies a relevant defect.

After Ruby, prioritize **shared implementations across the full provider registry**, not an alphabetical language list or a fixed C-then-BASIC queue. First inventory consumers of source-document decoding, scanner profiles, logical-line construction, symbol building, scope/range and conditional handling, and handler/index/coverage orchestration. Reproduce any proposed common defect in representative direct consumers before changing shared code. Compare a minimal shared repair with isolated fixes, reject regressions and unsupported generalizations, and requalify all affected consumers. Select the next family by demonstrated severity, potential reuse, corpus coverage, and remaining verification cost.

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

**Current inventory evidence:** all 100 registered providers resolve to a factory and an `Analyze` implementation. Static Go-AST inspection and direct call-site checks identify 18 `analyze*` entry points shared directly by 47 provider implementations, 13 registered composite-template hosts, and at least five explicit cross-analyzer delegations (including Flow to TypeScript and C++/CLI to C++). The original-project qualification ledger contains at least eight accepted independent projects for each active provider; acquisition qualification is **not** structural correctness certification. These call-graph counts are a verified starting subset, not exhaustive transitive dispatch or scanner/lexer sharing.

A real QBasic source oracle exposed a P0 false declaration: `DEF SEG` was indexed as function `SEG`. The shared seven-dialect `DEF` recognizer has a focused regression and development fix retaining `DEF FN...` functions. QBasic now has a selected-file comparison across **eight independent original projects**, with each selected original file read completely and **192 structural symbols** reported by the current Go MCP handlers in total. The reviewed declarations, negative cases, coverage status, and representative owners/positions match the selected oracles; the whole-language semantic inventory, detailed project relationships, and source-query workflow are **not yet certified**. A connected server still reports false `SEG` symbols in another original QBasic project and omits two names from a three-name `DIM SHARED`; current Go handlers reproduce neither error. This is a connected-versus-development difference, not proof that the installed server was updated.

**Classic BASIC selected-scope gate:** Eight independent original-project files have now been read completely and compared with the development Go MCP `source_symbols` handler (625 reported structural symbols in those files). Selected expected declarations, kinds, owners, source lines, and negative cases were checked, including a new fix for type-first QB64 `DIM AS` forms that previously indexed `AS` as a variable. This does **not** yet certify exhaustive symbol inventories, complete cross-file relationships, `source_query`, or deployment of the fix to an installed connector. The shared QBasic/FreeBasic/VB family still requires separate project qualification per provider; a shared test alone is insufficient.

**Shared brace-parser early qualification:** One independently selected, completely read original file has been checked with the development Go handler for each of Apex, D, Dart, Groovy, and Solidity. Selected declarations, positions and forbidden false symbols passed after a Groovy-only fix for semicolon-free class fields; the other four matched without new parser changes. The connected installed server showed differences from development for several samples. These are **first-project samples, not the eight-independent-project acceptance gate for any of the five providers**; remaining complete oracles, cross-file navigation and query workflows are still open.

Continue auditing shared scanners, logical lines, conditional masking, symbol building, and source-query indexing for **all remaining providers**, including SQL/data/config, hardware, scientific/functional, BEAM, .NET, systems, and language-specific analyzers. Some are related by syntax but **do not share a parser**; do not merge their implementation or infer coverage from a family name. Use the native analyzer registry and capability matrix to reconcile every active provider and track ungrouped providers explicitly. A repair in a common component never replaces **at least eight independently qualified original projects per language where available**, including positive and negative cases, project navigation and honest unsupported coverage.

**Priority:** P0 — fabricated or misclassified symbols, incorrect ownership, or wrong source bodies; P1 — missing declarations, incomplete hierarchy/outline/digest, or wrong paths/relationships; P2 — misleading completeness, ambiguity, encoding, freshness, or nondeterminism; P3 — unnecessary output, latency, or memory cost. Address P0/P1 first; run independent project checks in parallel only where they do not share mutable qualification inputs.

**Repeatable method:** Follow the [project-first Source Intelligence qualification workflow](DEVELOPMENT_CHECKLIST.md#source-intelligence-project-first-qualification). Independently label original source, compare the connected MCP runtime, development analyzer and an actual local handler test when run, and check grep/read under identical conditions. Record which boundaries remain untested.

**Completion rule:** Verified selected scopes must contain no unexplained false declarations, misclassifications, ownership errors, or unjustified omissions. Unsupported semantics and partial coverage must remain explicit. The paired comparison must show how much **correct, useful information** each method returns and at what measured cost; fewer calls by themselves are not success. Avoid new public tools, breaking schemas, persistent indexes, and external language servers without separate evidence-based approval.

**Published baseline:** The [4.0.1 changelog](../CHANGELOG.md#401---2026-10-26), [tool reference](../TOOLS.md#source_symbols), and generated [language capability matrix](LANGUAGE_CAPABILITIES.md) define released behavior. Cross-platform CI does not certify semantic correctness for every language. Releases follow [Publishing](PUBLISHING.md).
