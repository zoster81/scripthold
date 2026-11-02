# Changelog

This changelog records user-visible Scripthold changes. Detailed implementation history remains available in Git.

## Unreleased

### Added

- SQL source navigation now finds named `CREATE INDEX` and `CREATE UNIQUE INDEX` objects, including common `IF NOT EXISTS` and `USING` forms, as `resource` declarations with `nativeKind: index`. Index headers must identify a table and column list; unsupported forms report incomplete coverage instead of inventing indexes. SQL execution and database schema resolution remain unavailable.
- Assembly source navigation now finds MASM/Motorola `NAME EQU value` constants, named MASM/NASM `DB`/`DW` data, and named Motorola 68HC11 `FCB`/`FCC`/`FDB`/`RMB` data declarations outside recognized macro/structure definitions. `FCB`/`FDB` operands must be present, `RMB` requires one storage-size expression, and `FCC` strings must use matching delimiters; this is structural navigation, not assembler expression evaluation or macro expansion.

### Fixed

- Source navigation's shared scanner now recognizes line and block comments immediately adjacent to operator tokens (for example, `||/* comment */`), instead of absorbing the comment opener and misinterpreting an apostrophe within the comment as an unterminated literal. A real FFmpeg checkasm file and LF/CRLF regressions for C, C++, Java, JavaScript, and Lua verify the boundary; this does not add full preprocessing or resolve dynamic macros.
- C and C++ source navigation now preserves a single balanced anonymous `typedef struct { ... } Name;` or `typedef union { ... } Name;` as a type alias with correctly owned members, instead of inventing global variables from the fields and alias. Original Redis SHA-256 declarations and LF/CRLF regressions verify the correction. Unsupported aggregate declarators and arbitrary macro expansion remain unresolved.
- C and C++ source navigation now classifies a balanced anonymous `typedef enum { ... } Name;` as a type alias rather than a variable. This corrects `CURLHcode` in curl's original public header, retains following declarations, and is covered by LF/CRLF and real-handler regressions. Multiple typedef declarators, expanded macros, and arbitrary enum expressions are not inferred.
- C and C++ source navigation now recognizes uppercase return-type export macros ending in `API` without an underscore, including declarations split across lines (such as `XMLPARSEAPI(XML_Parser)` in an original Expat header). Unsupported multi-argument wrapper forms are consumed without fabricating function or variable declarations; LF/CRLF and whole-project handler regression tests cover the behavior. This does not execute macros or resolve compiler conditions.
- C and C++ source navigation now treats balanced `extern "C" { ... }` linkage blocks as transparent declaration scopes, including inside conditional preprocessor branches. This prevents a fabricated `CJSON_PUBLIC` function in the original cJSON utility header while preserving all 14 real declarations; LF/CRLF and real-handler regressions cover the change without expanding macros.
- Groovy source navigation now recognizes semicolon-free class fields with visibility/final modifiers, initialized fields and bounded nested generic types without mistaking initializer calls for methods. Source-derived LF/CRLF regressions and eight independent original project files cover this behavior; arbitrary dynamic or multiline Groovy declarations remain outside this narrow recognition.
- Groovy source navigation no longer indexes semicolon-terminated `package` declarations as fabricated variables; real package symbols and class members remain discoverable, without changing the other brace-language dialects.
- Solidity source navigation now preserves contract and library fields initialized by calls (such as `address(0)` or `uint16(...)`) instead of fabricating methods or omitting the real field name. The narrowly scoped fix preserves genuine Solidity functions and existing Apex initialization handling; original project source is inspected, not executed.
- Apex source navigation now recognizes auto-properties with accessor visibility modifiers, including `{ get; private set; }`, without inventing `get` or `set` fields. The property keeps its real name and source location; unsupported accessor bodies remain conservatively scoped.
- Dart source navigation now recognizes typed and inferred getters with block or expression bodies as properties, preserving their names and source ranges rather than indexing calls inside the getter body as methods. This corrects false methods and missing getters observed in real Dart extension sources; ordinary methods and other brace-language dialects keep their existing behavior.
- D source navigation now identifies simple `alias Name = target;` declarations as aliases instead of fabricating functions from `typeof(...)` or template targets. Unsupported alias forms are not guessed and are reported with incomplete structural coverage. This also makes supported D aliases discoverable through structural search without executing source code.
- Groovy source navigation now preserves semicolon-free primitive and simple generic class fields (such as `boolean enabled` and `List<String> names`) without mistaking the next method for part of the field. This is bounded structural recognition, not evaluation of arbitrary Groovy expressions or complete generic type resolution.
- Groovy source navigation now recognizes simple semicolon-free class fields without merging them into the following method declaration. This restores missing fields and the correct method source position; complex Groovy syntax remains conservatively scoped and the other brace-language dialects retain their existing parsing rules.
- Classic BASIC and QBasic source navigation now recognizes type-first `DIM AS Type name1, name2` declarations, including QB64's `_UNSIGNED` modifier. Previously these declarations could fabricate a variable named `AS` and omit the first real name. The fix reuses the shared BASIC parser while preserving variable ownership and source positions; other dialects retain their existing declaration rules. No source program is executed.
- BASIC-family source navigation no longer reports `DEF SEG` segment-register directives as `DEF FN` functions. The common seven-dialect parser now requires an explicit `FN`-prefixed function name after `DEF`, while retaining genuine classic BASIC and QBasic `DEF FN...` declarations. This fixes false symbols found in an original QBasic game converter without executing the program or changing the public API.
- Ruby source navigation now recognizes explicit Ruby 3 endless methods (`def name = expression`) without consuming a following method's scope or closing `end`. Ordinary methods and bracket setters keep their original ownership; unsupported expression semantics are not evaluated.
- Ruby source navigation now names constructors of explicitly qualified classes by their final class constant, rather than repeating the full qualified class name. Simple method bodies with default parameter values are retained when their closing `end` is unambiguous, even if later sibling Ruby declarations use inline syntax; uncertain later bodies still fall back to signatures.
- Ruby source navigation now returns full, fingerprint-verified source for simple methods with unambiguous `end` boundaries via `source_symbols show` and body-preferred `source_query context`. Nested or uncertain method bodies remain signature-only rather than claiming guessed code.
- Ruby source navigation now recognizes inline `private def`, `protected def`, and `public def` declarations with their method-specific visibility, preventing misattributed later declarations when nested scopes close.
- Ruby source navigation now recognizes explicit left-shift and bracket method operators (`<<`, `[]`, and `[]=`), preserving class ownership of subsequent methods. Dynamic methods created through Ruby DSL calls remain unresolved; no Ruby code is executed.
- Python source navigation now includes direct module assignments and annotations as `variable` declarations and direct class attributes as `field` declarations, preserving ownership and source positions. Method-local variables and dotted assignments remain excluded; conditional and dynamic assignments are not resolved or executed.
- JavaScript/Flow source detection now uses real CommonJS/ES module export syntax to corroborate JavaScript-family files and recognizes paired generated `flow-typed` declaration headers as Flow, so these sources do not become ambiguous from coincidental Ruby, Python, F#, or other language markers. Comments, quoted strings, and backtick templates do not supply false JavaScript module evidence; a single or embedded fake Flow header is insufficient. Ordinary Flow `@flow` directives continue to take precedence.
- Twig and Jinja source navigation no longer reports runtime-computed HTML `id` fragments (including mixed literal and template expressions) as fixed entities. Literal HTML IDs and template block/macro declarations remain discoverable at their original positions; templates are not executed or rendered.
- PowerShell source navigation now correctly names scope-qualified functions such as `function global:prompt` using the declared function name, and no longer treats hashtable keys such as `Function = @(` or `Filter = @(` as function declarations. This is structural navigation only: it does not execute scripts or resolve dynamic scopes.
- OpenAPI source navigation now avoids reporting Arazzo workflow step `operationId` references as API operation declarations. An explicitly identified Arazzo root document returns incomplete OpenAPI coverage rather than fabricated operations; genuine OpenAPI documents and rootless OpenAPI fragments remain supported. No workflow interpretation or reference resolution is performed.
- C++ and Objective-C++ source navigation now identifies pointer variables constructed with a parenthesized `new` expression (such as `std::mutex *hookMutex(new std::mutex);`) as variables instead of function declarations. This keeps the original identifier, parent, and source position, while preserving ordinary function declarations and existing literal-initializer handling; it does not evaluate C++ expressions.
- Less source navigation now preserves selectors containing variable interpolation (such as `.@{prefix}-alert`), mixin declarations with semicolon-separated parameters, and selectors following `//` comments. The shared CSS/SCSS/Less scanner also uses the correct source segment offset when evaluating selector names, preventing missed declarations after earlier statements. These are structural discoveries; Less interpolation and mixins are not evaluated or expanded.
- Elixir source navigation recognizes `defprotocol` as an interface-like declaration and preserves ownership and source positions of protocol functions, without expanding protocol implementations.
- Erlang source navigation recognizes single-quoted module atoms (such as `-module('chat').`) and correctly associates functions with their declared module, while preserving module and function source positions.
- D source navigation now preserves annotated and template-parameterized class and struct declarations, including those inside visibility groups, while excluding imported names and unexpanded mixin invocations from invented declarations. Unexpanded mixins explicitly leave coverage incomplete; D source is not compiled or evaluated.
- Common Lisp source navigation now recognizes standard `define-condition` definitions as class-like declarations, preserving their original names, offsets, and package ownership. This is structural navigation and does not evaluate Lisp forms or expand macros.
- COBOL source navigation now recognizes PROCEDURE DIVISION declarations with USING parameters, including headers continued across lines, so named paragraphs in free-format programs remain navigable. Existing fixed-format paragraph and section handling is preserved; argument/control-flow semantics are not evaluated.
- CIL source navigation now retains methods whose declaration headers span multiple lines, including the return type and method name on later lines; it bounds header collection and preserves class ownership and source positions without interpreting IL instructions.
- C source navigation now recognizes named function definitions and declarations following conservative unexpanded `_PUBLIC(type)`, `_API(type)`, or `_EXPORT(type)` return-type wrappers. It preserves source locations in projects such as cJSON, rejects malformed wrapper arguments rather than inventing functions, and does not evaluate or expand macros.
- Bash and POSIX shell source navigation now finds functions declared inside command-level brace groups, including protected installation scripts, while keeping function-local declarations and quoted text out of global navigation.
- AutoHotkey source navigation now recognizes classes whose opening brace is on the next line, including nested classes and their members, without interpreting unrelated later braces as the class body. Existing same-line declarations remain unchanged.
- Ansible YAML source navigation now distinguishes real named plays from standalone task lists and tasks inside unnamed plays by checking for a sibling `hosts` field. It preserves task names and source locations, including `pre_tasks`, without interpreting Ansible execution semantics.
- Dart source navigation now distinguishes top-level `final` values and fields initialized by function calls from actual callable declarations. This preserves names used in projects such as Spotube without altering the other brace-language analyzers. Original Hiddify annotation handling is protected by a separate regression test.
- Kotlin source navigation now recognizes named `object` declarations, including `data object` and nested named objects, and preserves their member ownership. Anonymous object expressions are not promoted into declarations. The existing JVM-family parser is reused and Java behavior remains unchanged.
- Swift source navigation now recognizes `package` access modifiers introduced in modern Swift, preserving package-scoped nested types, methods, and constructors. This fixes missing Vapor testing helpers without adding a parallel parser or changing the public API.
- C# source navigation no longer misclassifies fields initialized with constructor calls or lambdas as methods or properties; expression-bodied properties and real methods keep their expected kinds. Verified against the original Polly resilience pipeline sources.
- C# source navigation now preserves generic method names when declarations have line breaks before a method's type-parameter or argument list. It reuses the existing structural-token scanner, avoids indexing generic parameter names as methods, and retains original declaration positions.
- C++ source navigation retains explicitly written single-level qualifiers on definitions even when their type or namespace is declared in another file. The existing project search can then retrieve declarations and definitions sharing that written name; matching text alone does not prove class ownership or a semantic link. The public API is unchanged.
- C++ source navigation now recognizes classes, structs, and unions with common unexpanded uppercase `_API` or `_EXPORT` decorations, preserving declared names, constructors, member ownership, inheritance, and source positions without mistaking a standalone class name for an export macro.
- Rust source navigation now keeps multiline quoted string literals opaque, preserving real declarations after macro strings while avoiding false symbols from embedded text. Unterminated strings still report incomplete coverage.
- Flow and TypeScript source navigation no longer confuses object-type braces inside generic class constraints with the class body, preserving real class methods and source ownership across LF and CRLF input.
- TypeScript source navigation no longer reports class or interface generic constraints as inheritance relationships. Actual `extends` and `implements` clauses remain indexed, including when generic parameters span multiple lines.
- SQL source navigation recognizes multiline `CREATE TABLE` declarations when a keyword or table name starts on the next line. Existing comment/string exclusions and incomplete-coverage diagnostics for unsupported headers remain intact.
- Kotlin source navigation now names extension properties after their declared member (rather than the receiver type) and excludes getter-local variables from class property lists. Class declarations and subsequent members remain discoverable.
- COBOL source navigation now recognizes historical mixed-letter-and-digit sequence columns, sequence-only records, and named comment indicators in fixed-format files, avoiding false free-format classification and spurious unterminated-string warnings.
- MATLAB source navigation now recognizes enumeration blocks inside classes when balancing structural scopes. Valid enumeration classes report complete coverage without inventing declaration symbols for enum values; incomplete classes still report diagnostics.
- Assembly source navigation excludes labels, MASM `PROC` declarations, and `EQU` constants inside recognized unexpanded macro and structure definitions rather than inventing globals. A shared bounded line scan now handles labels, procedures, `EQU`, `DB`, and `DW` while preserving declarations outside those blocks.
- VHDL source navigation preserves architecture declarations beyond nested subprogram `end;` statements and reports missing outer architecture closures, including when a nested function ends at the end of the file, instead of treating an inner closure as complete.
- COBOL source navigation recognizes paragraph declarations in free-form and fixed-format `PROCEDURE DIVISION` code, retaining section ownership and source locations without treating standalone executable statements as paragraphs.
- SQL source navigation no longer treats unrecognized `CREATE TABLE` headers as confirmed table declarations; it keeps recognized definitions available and explicitly reports incomplete coverage for unsupported headers.
- D source navigation no longer mistakes assertions inside `unittest` blocks or `static assert(...)` compile-time checks for function declarations; surrounding real functions and types remain discoverable.
- Source Intelligence preserves distinct type-suffixed BASIC variables and retains CIL class ownership across nested method bodies, so later declarations remain discoverable.
- PHP/HTML structural analysis no longer indexes a runtime-generated HTML `id` containing embedded PHP as a fixed identifier; neighboring literal IDs remain discoverable.
- C++/CLI source analysis recognizes managed classes declared with trailing `sealed` or `abstract` modifiers and excludes `[assembly:...]` attributes from source declarations, including when an attribute line omits its trailing semicolon.
- PureBasic source navigation recognizes procedures with explicit return types (for example `Procedure.i` and `Procedure.s`) while preserving their declared names and source positions.
- Dart source navigation correctly identifies named constructors (including factory and const forms) as constructors rather than ordinary methods; surrounding methods remain unchanged.
- Objective-C++ source navigation no longer mistakes the Objective-C `Class` return type for a lowercase C++ `class` declaration; C++ classes and functions remain discoverable.
- HTML source navigation excludes apparent tags inside script, style, and other raw-text elements, preventing runtime-generated IDs from appearing as static declarations; unclosed raw-text elements report incomplete coverage.

These read-only parser fixes do not change MCP inputs, filesystem authorization, or execution privileges.

## 4.0.1 - 2026-10-26

This release improves read-only source navigation and response size, without changing filesystem mutation or authorization behavior. Structural navigation can now return a compact project map, an outline tree, and an authenticated continuation cursor so clients can inspect specific declarations instead of transferring entire files. The relative usefulness of these workflows versus grep and targeted reads remains task-dependent; the release does not claim complete semantic accuracy for every supported language.

**MCP catalog refresh:** After updating to 4.0.1, reconnect or refresh tool discovery so LLM clients receive the current tool descriptions and response schema. `source_symbols` with `operation: "outline"` defaults to `presentation: "tree"` (`outlineFiles`). The optional `presentation: "detailed"` continues to expose the earlier top-level `files` and `symbols` layout when explicitly requested. No separate migration is needed for agents using the refreshed catalog. Other operations and read-only authorization boundaries remain unchanged.

### Added

- `source_query relations` now explicitly labels results `evidenceScope: "indexed-relations-only"`. Complete file scanning does not imply complete semantic caller/reference coverage; an empty result must not be interpreted as proof of no usages.
- Optional `source_symbols digest` `presentation: "compact"` provides a bounded multi-file project overview with declaration counts, limited top-level public entry-point names, source provenance and explicit coverage. Existing digest responses remain unchanged by default; full import and relation details remain available in the historical form.

### Fixed

- `source_query` and `source_symbols` now retain at most `maxFiles` selected file paths during directory traversal rather than collecting every matching path first. Selection remains lexicographically deterministic, filters and path authorization are unchanged, and omitted-file coverage remains explicit.
- A filtered `source_symbols outline` now applies `maxSymbols` to matching declarations after bounded analysis, instead of losing matches behind earlier declarations of other kinds. Full-analysis and result-truncation coverage remain explicit.

### Changed

- Clarified the `source_symbols` tool description with explicit operation-specific inputs and grep/read alternatives. This does not change accepted requests, validation, or runtime behavior.
- `source_symbols outline` now returns a compact hierarchical `tree` view by default. Refreshed MCP tool discovery exposes the current response shape to LLM clients; `presentation: "detailed"` is an optional full-layout view, and `presentation: "compact"` remains available for a flat list.
- Source Intelligence structural search now includes analyzer-proven declaration `name`, `kind`, and `qualifiedName` when available, reducing the need for separate symbol lookups. Non-declaration matches retain their existing shape without invented identity; requests, source freshness checks, and search limits are unchanged.
- Source query responses now distinguish omitted files, incomplete selected-file analysis, and truncated search/relationship/context output with explicit coverage booleans, while retaining the existing aggregate flags.
- Source context planning now avoids broad reverse-dependency declarations for symbol and position selectors while retaining verified type relationships and explicit file-level context; outgoing type references are grouped with bounded request-local indexes rather than repeatedly scanning the full relation array.
- Repeated project context dependencies now reuse request-local immutable root-symbol lists rather than rebuilding symbol maps and arrays for every incoming edge. No public request changes or persistent indexes are introduced.
- Structural source search now retains only the best `maxResults` matches in a bounded heap rather than collecting and sorting every match. Global match ordering and truncation semantics are preserved, with no new persistent index.
- `source_query` structural search now supports optional `cursor` input and `search.nextCursor` output for bounded continuation. Tokens are authenticated, time-limited, and bound to search filters and the current indexed generation; old clients can omit cursors.
- Invalid, expired, or cross-query source search cursors now fail before source-file collection and index refresh, after preserving allowed-path authorization. Valid cursors still revalidate the current index generation before returning results.

## 4.0.0 - 2026-10-25

### Changed

- Consolidated eight approval-bound mutation apply tools into `confirm_verified_operation(previewId, note?)`. Preview commands and identifiers remain unchanged; one-shot application still uses each operation's original validation, backup, and partial-state safeguards. The public MCP catalog changes from 43 to 36 tools.
- Added `operationKind` to confirmation results and an optional descriptive `note` limited to 256 UTF-8 bytes. Confirmation accepts no mutation overrides, and invalid requests do not consume prepared previews.
- Removed the eight prior public apply tool names without aliases. Clients must refresh tool discovery and migrate according to [Migration to 4.0](docs/MIGRATION_4.0.md). Outstanding process-local previews cannot survive a server restart.
- Updated the immutable SHA-pinned CodeQL and GitHub App Token actions, and improved CI policy checks to accept qualified action upgrades while preserving pinning, shared versions, and protected publication ordering.

## 3.3.6 - 2026-10-25

### Fixed

- Corrected Windows standalone self-update helper dispatch by staging the verified helper as a `.exe` file. Update and explicit recovery retain the existing transaction, integrity, and locking requirements. Linux and macOS continue to use their native executable naming.

## 3.3.5 - 2026-10-24

### Fixed

- Increased the bounded GitHub release-list response allowance to handle larger repositories during protected release finalization. Individual release and other API responses retain their existing limits, and publication authentication and artifact verification remain unchanged.

## 3.3.4 - 2026-10-24

### Changed

- Clarified all 43 MCP tool descriptions to make tool selection more direct, reducing their combined length by 582 characters (18.80%) without changing tool names, schemas, or behavior.

- Made the documentation easier to navigate with a shorter README, a task-based index, a grouped tool reference, and a separate self-update guide. The tool reference now explains Markdown editing commands, while the Markdown Intelligence guide focuses on preserving source content. Release instructions describe the current process rather than past releases. These documentation changes do not affect tool behavior or release artifacts.
- Removed unsupported Source Intelligence options so `source_query` accepts only implemented search modes, relationships, and evidence levels. Invalid requests are rejected before indexing. Removed unused capability settings and inactive Dockerfile/Make provider placeholders. All 100 active providers and existing read-only analysis features remain available.
- Improved pre-merge checks for documentation changes so the full Source Intelligence test package runs before merging. Removed tests that required exact wording, fixed tool counts, or a fixed number of CI jobs. Publication and security checks remain in place, including checks that GitHub Actions use pinned references and release jobs run on approved runners.

## 3.3.3 - 2026-10-23

### Security

- Before publication, release checks now reject artifacts that cannot be verified against their signed Sigstore SLSA provenance. The original in-toto DSSE JSONL companion is published unchanged alongside the release bundle. No new signatures are generated, and existing security checks remain required.
- Secured temporary backup index files before replacing the index. This closes a Windows-only permissions gap that could occur during concurrent backups.
- Updated the required Go version and pinned container builder to Go 1.27.2 to include standard-library security fixes. Container checks now require the builder's Go patch version to match `go.mod`.

### Changed

- Protected release draft creation now verifies the new draft using its authenticated GitHub ID instead of waiting for it to appear in release listings. It does not retry draft creation or relax artifact checks.
- MCPB packaging now checks prepared server binaries against authenticated artifacts from the original GitHub build before comparing their contents with the expected digests. Unverifiable inputs are still rejected.
- Pinned every CodeQL step to the same verified v4.38.3 action commit. Dependabot now groups CodeQL action updates to avoid mixed workflow versions. Also updated the Scorecard artifact-upload action.
- Updated CI tooling to qualified Go analysis versions and Node.js 26.11.1, including a golangci-lint release compatible with Go 1.27.2.
- Updated the Go system dependency `golang.org/x/sys` to v0.49.0.
- Updated `golang.org/x/text` to v0.43.0 and its required dependencies. All CI and publication jobs now use the same verified `actions/setup-node` v7.1.0 commit, enforced by a regression test.
- Replaced multiple GitHub Release writers with one protected publication step. It first creates a draft, transfers the original build files with authenticated evidence, verifies their Sigstore provenance, and packages MCPB assets on GitHub. Publication requires all 22 release assets to be verified; the MCP Registry is updated only afterward. For v3.3.3, the signed handoff contained 15 original files, and GitHub produced six MCPB packages plus a checksum. The release was published as immutable, followed by a successful OIDC-authenticated Registry update. An independent check confirmed all 22 published asset names, sizes, and SHA-256 digests against the original GitHub Actions artifacts, as well as the Registry digests for the six MCPB packages.

## 3.3.0 - 2026-09-28

### Added

- Added `scripthold update recover` to resume eligible pending standalone updates without network access. It invokes a fixed recovery helper without accepting a transaction authority from the caller, and distinguishes failures before startup from launcher cleanup failures after the helper starts.
- Added `scripthold update` for adopted standalone installations. It verifies the latest immutable official release and update candidate, prepares the update in a locally controlled area, and starts a detached helper. If the installation is already current, no change is made. If launcher cleanup later fails, the command still reports that the helper started.
- Added `scripthold update --adopt` for supported standalone binaries. Adoption requires the installation to be idle, validates it before creating local update state, and stops on uncertain conditions. It does not download or switch releases.
- Added read-only `scripthold update status` to show standalone update state, verified installed and candidate versions, cached release availability, and whether safe recovery is possible. It does not modify the installation.
- Added `markdown_create` to preview a complete new Markdown document before writing it. It uses Marksplice to build validated content, follows a defined new-file encoding and BOM policy, ties the preview to its parent directory, and creates the file only through a one-time `markdown_apply`. An existing file is never silently replaced.
- Added read-only `markdown_workspace` to discover, inspect, and validate links between Markdown documents within authorized directories. Marksplice's `workspacefs` uses explicit `scan` and `follow` discovery modes and limits the number of documents and relationships examined.
- Added `repairPreview` to plan managed table-of-contents repairs across Markdown files without changing them. A one-time `markdown_apply` first checks that files are still valid targets, applies the configured backups, then writes changes in a defined order. If only some changes succeed, the result reports that partial state.
- Added Marksplice-backed removal of a recognized front-matter block at the start of a document, without requiring the caller to specify a target offset.
- Added Markdown table alignment editing through Marksplice. Users can change one column or all columns together, and table reads expose the current alignment settings.
- Added four validated Marksplice constructors for existing documents: front-matter blocks, front-matter fields, link reference definitions, and footnote definitions. Callers do not supply raw insertion offsets.
- Added safe retargeting of a single reference link or image using a relationship confirmed by the Markdown parser. Full, collapsed, and shortcut references are supported, without granting unrestricted offset editing.
- Introduced R30 Markdown Intelligence: `markdown_read` inspects existing documents, `markdown_edit` previews changes tied to a specific file version, and the one-time `markdown_apply` checks the file again, follows backup policy, and reports the actual result. A preview can combine up to **64 compatible changes**; overlapping or conflicting edits are rejected before writing. Supported operations include headings, task checkboxes, inline formatting, links and images, front matter, HTML comments and anchors, math, blockquotes and GitHub alerts, tables of contents, code fences, reference links, footnotes, paragraphs, nested lists, and complete sections. Edits can insert, replace, remove, move, or retarget supported elements where the parser can establish safe boundaries. See [`TOOLS.md`](TOOLS.md) for the exact available operations and [`docs/MARKDOWN_INTELLIGENCE.md`](docs/MARKDOWN_INTELLIGENCE.md) for source-preservation guarantees.

- Markdown changes now support UTF-16 LE/BE and UTF-32 LE/BE while preserving existing BOM settings and line endings. Unsupported legacy encodings are still rejected.

### Changed

- Upgraded to Marksplice 1.2.0. Markdown previews now check that local fragment links still resolve after all proposed edits. They accept links that remain valid or are explicitly retargeted, and reject missing, ambiguous, invalid, unknown, or silently changed targets. Managed tables of contents can be updated alongside other edits using the final preview.
- The `markdown_create` footnote `body` field now supports validated multiline definitions, including deferred definitions, without changing its public schema.
- Unified the internal preparation flow for single-file edits and patch packages. Both now share checks for output bytes, file identity, fingerprints, backups, and write approval, followed by ordered application and accurate failure reporting. Public schemas are unchanged.
- Reduced processing time and memory allocations in Source Intelligence scanners and builders, including composite templates, shell scripts, HDL, and MATLAB/Octave. Public schemas and supported capabilities are unchanged.

### Fixed

- Included Marksplice 1.2.0 fixes for consistent Markdown parsing and regeneration, including raw link destinations.
- After upstream rejected release-tag runs, OpenSSF Scorecard was kept on its supported default-branch workflow. Release verification now requires a successful Scorecard result for the exact commit pushed to `main`.

## 3.2.1 - 2026-09-16

### Added

- Added OpenSSF Scorecard results to GitHub code scanning through SARIF reports, and updated Glama ownership metadata.
- Added signed Sigstore SLSA provenance for standard GoReleaser builds. The provenance bundle is attached to each GitHub Release so users can verify where the assets came from.

### Changed

- Pinned external GitHub Actions to immutable commit hashes and added checks to catch unintended changes in publication and trust metadata.
- Reduced permissions granted to release jobs. The reusable MCPB publisher now has read-only access by default, and container base images use immutable digests. CodeQL checks pull requests and pushed commits, and the security policy links to private vulnerability reporting.
- Configured OpenSSF Scorecard to run on `main` pushes and release-tag pushes, as well as on a schedule or by manual request. A later release changed this trigger policy; see 3.3.0.
- Updated README badges to link to release, download, security, MCP Registry, and Glama evidence.
- Removed an outdated Smithery tool count and clarified the difference between official releases and third-party directory listings.
- Recovery diagnostics for deferred operations now identify the failure category without exposing file paths or producing unbounded output.

### Fixed

- Unreadable deferred-operation records are preserved and reported without preventing recovery of valid records. Repeated reports of the same failure are suppressed until its status changes.

## 3.2.0 - 2026-09-15

### Added

- Added server diagnostics with sensitive details redacted and output limits enforced. Optional log rotation and retention keep these diagnostics separate from HTTP access logs and durable-task output.
- Added protected persistent backups using `backupPolicy: "pinned"`. Removing one with `backup_delete` requires its exact backup ID.
- Added `deferred_operation` to check progress, cancel durable read-only work, and retrieve completed results that are too large for a normal response. Clients supporting native MCP tasks can monitor the same work through that interface.

### Changed

- Updated the Go minimum to 1.27.1 and refreshed validation and release tools: Node.js 26.8.2, golangci-lint 2.13.2, govulncheck 1.8.0, GoReleaser 2.18.1, Gitleaks 8.30.1, and CodeQL Action 4.38.0.
- Updated the MCP Go SDK to v1.8.0 and its supporting dependencies.
- Improved connector reliability by limiting synchronous call time, retaining oversized responses for later retrieval, and allowing eligible long-running read-only calls to continue as durable tasks.
- Reduced duplicate memory use in Source Intelligence. Its public schemas and supported features did not change.
- Backup histories no longer stop accepting new backups when full. After a new backup is safely stored, the oldest eligible backup is removed according to retention rules. The default target is 64 retained versions.

### Fixed

- Improved Source Intelligence handling of real-world code in Haskell, Julia, PHP/PHP-HTML, Blade, JavaScript/TypeScript, Lua/Luau, JSP, Erlang, F#, BASIC-family languages, Gleam, Jinja/Twig, and Terraform/HCL. Malformed Zig declarations are also handled more safely.
- Fixed races when observing deferred-operation states. Cancellation now also interrupts waits for durable storage locks.
- Fixed backup targets becoming permanently blocked when their history reached its retention limit.
- `backup_delete` now refuses to delete a backup if its response would exceed the output limit. The check happens before the destructive operation.
- Accepted `none`, regardless of letter case, as an alias for BOM policy `never`. Returned metadata still uses `never`.

## 3.1.6 - 2026-08-19

- Fixed integer-size risks identified by CodeQL, including platform-dependent configuration values and bounded backup-list cursors.

## 3.1.5 - 2026-08-19

- Organized repository checks by risk and type of change. Unknown changes trigger broader checks; fuzz targets share one risk-based manifest, releases require evidence for an exact commit, and local validation profiles complement CI.
- Fixed byte-offset handling for Unicode text in Classic ASP, ASP.NET, and Razor source scanning.

## 3.1.0 - 2026-08-18

- Simplified internal engine code without changing public MCP tools. Removed obsolete test-only paths, combined equivalent implementations, and reorganized source files while keeping necessary compatibility behavior.

## 3.0.0 - 2026-08-17

### Added

- Introduced read-only Source Intelligence with 101 active providers, `source_symbols`, and structural `source_query`. Project relationships and context are bounded, ambiguous languages are rejected, and incremental indexes remain local to the process.
- Added offline backup recovery into a separate destination while preserving the evidence needed to verify recovered data.
- Added `filesystem_package` and `filesystem_package_apply` to preview and apply bounded groups of file creation, copying, moving, and deletion operations.
- Added dedicated apply tools for edits, patches, restore/garbage collection, encoding, and BOM changes. Preparation remains read-only until a one-time `previewId` is approved and applied.
- Added `golangci-lint` and CodeQL checks to the release-quality pipeline.

### Changed

- Replaced separate public create, copy, move, and delete tools with the safer filesystem-package workflow.
- Separated read-only previews from tools that write files. Apply requests accept only the identifier of an approved preview.
- Improved persistent backup protection for approved file changes and destructive filesystem operations.

See [`docs/MIGRATION_3.0.md`](docs/MIGRATION_3.0.md) for caller migration details.

## 2.2.0 - 2026-08-11

- Expanded text encoding support to 168 canonical encodings for reading and writing.
- Added full UTF-32 little-endian and big-endian text support, with strict GB18030:2022 validation.
- Automatic encoding detection now rejects ambiguous, malformed, binary-like, or weakly supported input instead of guessing.
- Grep and batch operations now report incomplete coverage and encoding failures explicitly, with bounded output.

## 2.1.1 - 2026-08-10

- Combined release checks into one gate that verifies the exact commit before tagging.
- Renamed the whole-document replacement tool from `write_file` to `write_whole_file`, making its destructive behavior clear.
- Fixed a race when resolving missing file paths and corrected how durable tasks retain their allowed roots.
- MCP Registry publication now uses verified, platform-specific MCPB release assets.

## 2.1.0 - 2026-08-09

### Added

- Added durable background tasks with persistent queues, idempotent requests, logical locks, bounded logs, cancellation, and recovery. Supervisor, worker, and executor responsibilities are separated.
- Added reproducible path fingerprints, one-time edit and patch approvals, structured verification, improved grep/search, batch conversion, and prompts for encoding workflows.
- Added optional persistent backups with bounded inspection, capture, restore, audit, and explicit garbage collection.
- Added support for MCP protocol version `2026-07-28` while preserving compatible legacy stdio and HTTP behavior.

### Changed

- Replaced the public synchronous `run_script` and `shell` tools with durable tasks.
- Improved backup and file-changing operations to detect conflicts, validate approval capabilities, and report partial results accurately.

## 2.0.0 - 2026-08-02

### Added

- Added authenticated Streamable HTTP alongside stdio. HTTP starts with loopback-only defaults and supports bearer tokens, Host/Origin validation, request limits, TLS/proxy controls, health/readiness endpoints, and graceful shutdown.
- Added consistent typed errors, configurable resource limits, one authoritative tool catalog, and release verification across platforms.
- Added bounded streaming text and encoding operations plus shared primitives for durable file changes.
- Added conservative UTF-16 detection without relying on file extensions or a BOM, while reducing false positives from malformed or binary data.

### Changed

- New files use UTF-8 by default. Existing files keep their confidently detected encoding unless the user explicitly converts them.
- Dynamic MCP roots are available only to compatible stdio clients started without configured directories. HTTP allowed roots remain fixed by startup configuration.
- Improved reproducibility of release and container builds and required non-root container execution.

See [`docs/MIGRATION_2.0.md`](docs/MIGRATION_2.0.md) for the intentional 1.8-to-2.0 compatibility changes.

## 1.8.0 - 2026-07-25

- Established Scripthold's own repository identity and release/update process.
- Added shared file operations that preserve encoding and BOM details, consistent errors, predictable directory traversal, durable changes, and initial optional execution tools.
- Strengthened Windows junction/reparse-point path checks and protection against concurrent file changes.

## Earlier fork history

Earlier development details are preserved in Git history. The changelog intentionally does not duplicate commit-by-commit or phase-by-phase engineering records.
