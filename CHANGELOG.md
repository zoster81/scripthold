# Changelog

This changelog records user-visible Scripthold changes. Detailed implementation history remains available in Git.

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
