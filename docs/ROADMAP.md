# Scripthold Development Roadmap

This is the authoritative product roadmap for Scripthold, currently maintained in `zoster81/scripthold`.

Version `2.1.1` is the current public Scripthold release with 30 tools and 3 guided prompts. It is published on GitHub and in the MCP Registry, deployed on Windows amd64, and verified through an active rollback to `2.0.0` followed by restoration to `2.1.1`. Version `2.0.0` remains the historical 23-tool rollback baseline. R21 is complete. **R22 is `ACTIVE` and targets Scripthold `2.2.0` with global portable encoding coverage, full UTF-32 text support, conservative detection hardening, and corpus-backed verification.**

Product identity and the fork's independent relationship to upstream are defined in [PROJECT_DIRECTION.md](PROJECT_DIRECTION.md). Current milestone status and completion gates live in this document. Reusable engineering checks live in [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md), contributor workflow in [`CONTRIBUTING.md`](../CONTRIBUTING.md), scoped agent guidance in [`AGENTS.md`](../AGENTS.md), and completed R1-R6 engineering outcomes in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md).

## Operating rules

- Only one milestone may be `ACTIVE` at a time.
- Complete milestones in order unless maintainers explicitly reprioritize them.
- Keep changes atomic and limited to the active milestone.
- Use content bytes and structural evidence for encoding detection. File extensions must not select or bias an encoding.
- Treat domain-specific files, including MQL sources, as ordinary test fixtures rather than special encoding profiles.
- Preserve stdio support while adding Streamable HTTP.
- Keep both `task_run` execution kinds disabled by default on every transport.
- Build internal commit binaries as needed; create later public releases only after their dated changelog, verification, asset, Registry, and deployment gates pass.
- Every milestone must pass [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md) before it is marked complete.

## Milestone overview

| Milestone | Status | Outcome |
|---|---|---|
| R7 | COMPLETE | Replaced the historical roadmap with a clear operating plan and removed domain-specific MQL emphasis. |
| R8 | COMPLETE | Generic, conservative, extension-independent encoding detection, including BOMless UTF-16. |
| R9 | COMPLETE | Real bounded-memory streaming for large-file read, grep, conversion, line-ending, and BOM paths. |
| R10 | COMPLETE | Resolve public API inconsistencies and compatibility debt before the 2.0 boundary. |
| R11 | COMPLETE | Separate transport bootstrap from the shared MCP server and tool policies. |
| R12 | COMPLETE | Approve the Streamable HTTP threat model and security design. |
| R13 | COMPLETE | Implement and verify native MCP Streamable HTTP while preserving stdio. |
| R14 | COMPLETE | Completed hardening, publication, dual-transport deployment, active rollback, restoration, and final handoff for 2.0.0. |
| R15 | COMPLETE | Added attributed agent-ergonomics and project-aware workflows while preserving transport, memory, mutation, and security guarantees. |
| R16 | COMPLETE | Added verified change workflows through deterministic fingerprints, one-shot edit preview/apply, declared patch packages, and structured verification. |
| R17 | COMPLETE | Approved the bounded persistent-backup lifecycle, security boundary, quotas, restore safety, explicit GC, and non-rollback decisions. |
| R18 | COMPLETE | Implemented and verified the approved persistent-backup subsystem in phased, failure-injected, cross-platform increments. |
| R19 | COMPLETE | Added bounded deterministic mutation-free offline diagnostics for an existing persistent backup store. |
| R20 | COMPLETE | Stable SDK `v1.7.0`, stdio version gating, same-endpoint dual-generation HTTP, structured unsupported-version handling, and the full compatibility/conformance gate are verified in source. |
| R21 | COMPLETE | Delivered durable asynchronous task execution, exact-commit release gating, and the published, deployed, rollback-verified `2.1.1` release. |
| R22 | ACTIVE | Expand to global portable encoding coverage, promote UTF-32 to the full text pipeline, harden ambiguity/detection semantics, and complete the `2.2.0` release gate. |

---

# R22 — Global encoding coverage and Scripthold 2.2.0

## Goal

Deliver the broadest practical cross-platform text-encoding support that can be implemented and verified without weakening Scripthold's content-based detection, bounded-memory streaming, durable mutation, filesystem confinement, transport equivalence, or typed-error guarantees. The authoritative implementation and test contract is [GLOBAL_ENCODING_COVERAGE.md](GLOBAL_ENCODING_COVERAGE.md).

## Approved scope

- [x] Normalize the real-world encoding corpus with immutable provenance, byte sizes, SHA-256 values, and UTF-8 oracles where practical; pinned fixtures may be sourced from `arthenica/libiconv` and `oe-mirrors/uchardet`.
- [x] Replace the current minimal registry with one authoritative capability registry covering canonical names, aliases, detector labels, decoder/encoder support, BOM behavior, validation, and auto-detection eligibility.
- [x] Expose every applicable portable encoding already implemented by the repository-pinned `golang.org/x/text` before adding custom codec code.
- [x] Promote UTF-32 LE/BE from BOM-management-only handling to the complete text-operation pipeline with strict scalar validation and code-unit-aware line-ending support.
- [x] Add additional portable single-byte mappings from pinned libiconv-compatible sources using deterministic pure-Go tables and exhaustive byte-level tests.
- [x] Add missing multibyte/stateful families one at a time under focused TDD, streaming state machines, differential oracles, malformed-input tests, and chunk-boundary coverage. Phase 6 adds 21 explicit-only codecs and raises the verified source registry from 147 to 168.
- [x] Harden detection with detector-to-registry closure, short-input evidence floors, confusion matrices, binary rejection, malformed-input rejection, and deterministic `sample`/`chunked`/`full` semantics. Phase 7 adds strict/text-quality trust gates, stateful HZ/ISO-2022 signatures, GB18030 revision-safe ambiguity, and fail-closed existing-file writes without changing the 168-codec registry count.
- [x] Make grep and batch partial encoding failures bounded and visible rather than implying complete coverage when files were skipped. Phase 8 adds deterministic grep coverage metadata, bounded skipped/error summaries with explicit omitted counts, additive encoding subcodes, strict streaming UTF-8 validation, and terminal grep cancellation while preserving valid partial results.
- [x] Exercise every supported encoding through the applicable read, batch, grep, write, edit, patch, convert, BOM, line-ending, verification, and prompt workflows. Phase 9 adds a registry-driven all-168-codec public-operation matrix, byte-identical edit no-op verification, applicable JSON/BOM checks, complete trusted-corpus public detection, and explicit contracts for all three encoding workflow prompts.
- [x] Complete Phase 10 adversarial, fuzz, resource, cancellation, concurrency, failure-injection, race, static-analysis, and vulnerability verification for the expanded encoding surface. Representative R22 classes now pass malformed/no-mutation, decoded-limit, cancellation, deterministic batch/grep concurrency, 225,000 fixed-count fuzz executions, and bounded 1/16/64 MiB allocation gates; GB18030:2022 no longer allocates per sequence.
- [ ] Complete Phase 11 six-target build, native/container smoke, deterministic packaging, MCPB, Registry-manifest, and final release-candidate verification before release preparation. The source-only preflight is already green: `goreleaser check`, all four public PowerShell launcher parses, actionlint 1.7.12, ShellCheck 0.11.0, and the suspicious-untracked-residue audit pass without producing release artifacts.
- [ ] Prepare and publish `2.2.0` only from the exact clean commit that satisfies the completed R22 and normal publishing gates.

## Design decisions

- The production runtime remains pure Go. libiconv and uchardet are approved pinned corpus/oracle/reference sources, not runtime DLL/shared-library/subprocess dependencies.
- Explicit codec support may be broader than automatic detection. Ambiguous encodings remain explicit-only where reliable content-based discrimination cannot be justified.
- Malformed input must fail deterministically; silent replacement characters or lossy transliteration are not acceptable default decoding behavior.
- Unicode UTF-8, UTF-16, and UTF-32 are the supported Unicode transformation-format families. R22 does not invent a proprietary UTF-64 format or expose machine-dependent internal character representations as portable file encodings.
- The final supported-encoding count is derived from the verified authoritative registry after implementation; aliases do not count as separate encodings and planning documentation must not promise an unverified total.

## Completion gate

R22 is complete only when the detailed gates in [GLOBAL_ENCODING_COVERAGE.md](GLOBAL_ENCODING_COVERAGE.md) pass, the final registry/documentation count is generated or verified without drift, every trusted detection result resolves to a supported codec, malformed and ambiguous data fail safely, UTF-32 passes the full public operation matrix, grep exposes skipped encoding failures, the corpus is reproducible from pinned provenance, and the complete repository plus release-candidate verification matrix is green.

Publication, Registry upload, deployment, active rollback, and restoration remain separately governed final release operations under [PUBLISHING.md](PUBLISHING.md).

---

# R21 — Durable tasks and Scripthold 2.1.x release consolidation

## Goal

Maintain the `2.1.x` line with the [durable task subsystem](DURABLE_TASKS.md) required for long-running shell/script work, without weakening existing compatibility, security, backup, transport, or release invariants. Releases must be built from an exact verified commit and published through the immutable release procedure in [PUBLISHING.md](PUBLISHING.md).

## Completion record

- [x] Complete and verify R15–R20 source implementation and the 30-tool/3-prompt durable-task surface.
- [x] Verify the complete release candidate across race tests, static/vulnerability analysis, deterministic fuzzing, six cross-builds, native/container smoke tests, release metadata, documentation, and secret checks.
- [x] Consolidate Build/Test into one exact-commit `CI` release-candidate gate and make tag publication attest that successful commit instead of rerunning the same expensive checks.
- [x] Publish `v2.1.1` with six raw binaries, six platform archives, six GitHub-produced MCPB bundles, and both checksum manifests; publish `io.github.zoster81/scripthold` version `2.1.1` to the MCP Registry.
- [x] Deploy the published Windows amd64 `2.1.1` binary, verify tunnel-owned stdio plus authenticated legacy and stateless modern HTTP, perform an active rollback to the published `2.0.0` baseline, then restore and reverify `2.1.1`.

## Publication record

Release `2.1.1` was published on 2026-08-10 and is the current Scripthold release. The GitHub Release contains six raw binaries, six platform archives, six OS/architecture-specific MCPB bundles, `checksums.txt`, and `mcpb-checksums.txt`. All GoReleaser and MCPB SHA-256 entries were independently verified, and `io.github.zoster81/scripthold` version `2.1.1` is active in the MCP Registry. The published Windows amd64 runtime has been deployed and verified across tunnel-owned stdio, authenticated legacy HTTP, stateless MCP `2026-07-28` HTTP, active rollback to `2.0.0`, and restoration to `2.1.1`.

Version `2.0.0` remains the historical 23-tool rollback baseline. Detailed release changes belong in [CHANGELOG.md](../CHANGELOG.md), while this roadmap records product status and completion gates rather than operational incident chronology.

## Non-goals

- Beyond the approved five-tool task family, no new MCP tool, prompt, backup action, protocol extension, transport, authentication mode, or repair capability belongs in R21.
- No cleanup may rename persistent `mcp-file-tools:*` domain-separation constants or rewrite historical `2.0.0` asset/Registry evidence.
- No public release or Registry publication occurs from a dirty worktree or an unpublished commit.

## Completion status

R21 is `COMPLETE`. Future `2.1.x` maintenance uses the same exact-commit release-candidate gate, immutable publication procedure, artifact verification, and controlled deployment/rollback discipline.

---

# R7 — Roadmap and documentation reset

## Goal

Create one pragmatic development sequence, make the current limitations explicit, and remove the incorrect impression that MQL filenames receive special encoding behavior.

## Checklist

- [x] Publish a concise, contributor-facing R1-R6 engineering history.
- [x] Create this authoritative R7-R14 roadmap.
- [x] Create one reusable development and verification checklist.
- [x] Separate public roadmap, contributor guidance, and engineering history from private operator state.
- [x] State consistently that internal builds continue until the next public release, `2.0.0`.
- [x] Move Claude Code plugin verification out of active development and into the final 2.0 release gate.
- [x] Remove MQL-specific product claims from README, tool documentation, plugin documentation, runtime instructions, and tool catalog descriptions.
- [x] Replace MQL-specific examples with neutral filenames and content-based guidance.
- [x] Rename MQL acceptance tests and fixture directories to generic encoding acceptance names.
- [x] Keep coverage for UTF-16, UTF-8, multilingual text, BOMs, and CRLF/LF behavior after the rename.
- [x] Document that current BOMless UTF-16 auto-detection is incomplete and explicit encoding may still be required.
- [x] Correct documentation that currently implies large files are streamed when the shared document path still uses `os.ReadFile`.
- [x] Link README and publishing notes to this roadmap and the development checklist.
- [x] Add a root `AGENTS.md`, scoped subsystem guides, and a portable `CONTRIBUTING.md`.
- [x] Add a regression test that rejects private operator paths and connector identifiers in tracked text.
- [x] Run catalog, documentation, Go, Node, formatting, link, and diff verification.

## Completion record

Completed on 2026-07-26. Public planning, contributor checks, scoped agent guidance, and engineering history are separated by responsibility; private operator state is excluded from tracked text; public descriptions are extension-independent; generic UTF-16/UTF-8 acceptance fixtures pass under `.data`, extensionless, and `.random` filenames; and documentation/catalog drift checks are green.

---

# R8 — Generic encoding detection

## Goal

Infer encodings from byte structure and decoded-content evidence, independently of filenames and extensions. Prefer an explicit ambiguous result over a confident false classification.

## Design requirements

- BOM detection remains authoritative for UTF-8, UTF-16, and UTF-32 signatures.
- Extension, basename, directory, or language-specific content must not influence the selected encoding.
- BOMless UTF-16 LE/BE detection must combine multiple independent signals rather than rely only on alternating NUL bytes.
- Malformed Unicode input must be rejected or left ambiguous rather than silently repaired.
- Binary classification must occur after candidate decoding as well as on raw structural evidence.
- Detection results must remain deterministic for the same byte sequence.

## Checklist

- [x] Add structural BOMless UTF-16 LE and BE candidate detection.
- [x] Measure NUL distribution on even and odd byte positions.
- [x] Require even byte length for UTF-16 candidates.
- [x] Validate UTF-16 code units and surrogate pairs.
- [x] Reject isolated high/low surrogates and truncated pairs.
- [x] Measure printable, whitespace, replacement, control, and NUL rune ratios after decoding.
- [x] Verify decode/encode round-trip consistency for candidates.
- [x] Define conservative confidence thresholds and a minimum evidence size.
- [x] Avoid forcing a candidate when evidence is insufficient.
- [x] Integrate the same decision logic into sample, chunked, and full modes.
- [x] Verify candidate decisions across chunk boundaries.
- [x] Add fixtures with `.txt`, `.dat`, no extension, random extensions, and identical content under different names.
- [x] Add Latin, Cyrillic, Greek, Hebrew, Arabic, CJK, emoji, and mixed-script fixtures.
- [x] Add empty, BOM-only, very short, odd-length, truncated, and malformed UTF-16 cases.
- [x] Add executable, image, archive, random-byte, sparse-NUL, and binary-structure false-positive tests.
- [x] Fuzz detection and Unicode validation.
- [x] Document confidence semantics and ambiguous results.

## Completion gate

The same byte sequence must produce the same result under any filename. BOMless UTF-16 must be recognized only when structural and decoded-text evidence agree, and binary false-positive tests must pass.

## Completion record

Completed on 2026-07-26. Detection now uses one conservative UTF-16 LE/BE classifier with code-unit and surrogate validation, decoded-text metrics, NUL-byte parity, round-trip checks, deterministic confidence, and explicit ambiguity. Sample, chunked, and full modes share the same decision semantics; chunked analysis preserves surrogate state across 128 KiB boundaries. Tests cover identical bytes under unrelated filenames, multilingual scripts and emoji, malformed and short input, BOM-only files, legacy encodings, executable/image/archive/random data, and public read/grep integration. A bounded fuzz smoke test and the applicable verification ladder passed; the race detector was unavailable and no standalone deployment build was performed.

---

# R9 — Bounded-memory streaming pipeline

## Goal

Make large-file behavior match the documented memory guarantees. Streaming operations must not load complete source files, while operations that inherently require full-document state must reject oversized input before allocation.

## Architecture requirements

- Detection, BOM handling, decoding, line framing, and consuming operations must be separable streaming stages.
- Multibyte sequences, UTF-16 code units, CRLF pairs, and regex context may span chunks.
- Mutation output must be staged and synced before atomic commit without first materializing the complete target as `[]byte`.
- Memory bounds must account for concurrent workers, decoded expansion, result buffers, and exceptionally long lines.

## Checklist

- [x] Define explicit per-operation memory and line-length limits.
- [x] Add a shared incremental decoder for all registered encodings.
- [x] Add chunk-boundary tests for multibyte text, surrogate pairs, BOMs, CRLF, and lone CR/LF.
- [x] Stream `read_text_file` while preserving offset, limit, total line count, and character truncation semantics.
- [x] Bound aggregate memory in `read_multiple_files`, not only worker count.
- [x] Stream `grep_text_files` with a bounded previous-line ring buffer and bounded following context.
- [x] Preserve deterministic file and match order with streaming workers.
- [x] Stream `convert_encoding` from decoder to staged encoder output.
- [x] Preserve exact CRLF, LF, CR, and mixed line-ending sequences during conversion.
- [x] Add writer/reader-based mutation staging APIs.
- [x] Stream `detect_line_endings` and `change_line_endings`.
- [x] Stream `manage_bom` prefix inspection and staged copy.
- [x] Define and enforce an explicit full-document size limit for `edit_file` and unified diff generation.
- [x] Remove or make private `DetectSample` after all byte-slice consumers migrate.
- [x] Remove inaccurate streaming comments and warnings from configuration and documentation.
- [x] Test cancellation, read failures, write failures, disk-full simulation, cleanup, and concurrent source changes mid-stream.
- [x] Benchmark memory and throughput on representative small, medium, and large files.

## Completion gate

Every operation documented as streaming must have a verified bounded-memory path. Large-file tests must demonstrate that memory does not scale linearly with complete input size except for explicitly bounded full-document operations such as editing.

## Completion record

Completed on 2026-07-26. A shared read session now separates random-access detection from one sequential SHA-256 pass; incremental decoders support all 24 registered encodings; bounded line framing rejects decoded lines above 16 MiB; and `MCP_MEMORY_THRESHOLD` is enforced as the default hard budget for single-read output, aggregate batch output, retained grep state, inconsistent-line results, and full-document editing. Read, batch, grep, encoding conversion, line-ending detection/conversion, and BOM mutation now use bounded streams or disk staging while preserving BOM, encoding, ordering, cancellation, no-op, backup, and concurrent-modification behavior. Benchmarks on 1, 16, and 64 MiB inputs showed constant allocation footprints for line scanning and line-ending transformation; the applicable Go, static-analysis, vulnerability, manual MCP, documentation, and repository gates passed. The race detector and release-adjacent multi-platform checks remain deferred to R14.

---

# R10 — Public API and compatibility cleanup

## Goal

Use the major-version boundary to resolve inconsistent schemas, defaults, deprecated tools, and unsupported promises before the 2.x API is stabilized.

## Checklist

- [x] Remove the deprecated `directory_tree` tool and retain `tree` as the single recursive tree API.
- [x] Use UTF-8 as the international default for newly created files; retain `MCP_DEFAULT_ENCODING` and explicit legacy encodings such as `cp1251` as overrides.
- [x] Define behavior for empty and ambiguous files across every text tool.
- [x] Keep UTF-32 as BOM-management only rather than an incomplete registered text encoding.
- [x] Normalize public output JSON to camelCase, including `hasBOM`.
- [x] Define one stable public error-code vocabulary for single-tool `_meta` and batch items.
- [x] Define configurable limits for file size, decoded characters, line length, batch size, matches, output, and sessions.
- [x] Review every tool input/output schema and preserve fields not explicitly listed as breaking changes.
- [x] Remove obsolete `directory_tree` code and full API types; retain the documented stringified-array repair for current MCP client interoperability.
- [x] Produce a 1.8-to-2.0 migration table before implementation is finalized.
- [x] Update catalog, docs, manual tests, and schema compatibility tests together.

## Completion gate

All intentional breaking changes are explicit, tested, and listed in the migration guide. No deprecated or internally inconsistent public API remains accidentally carried into 2.x.

## Completion record

Completed on 2026-07-26. The public catalog now contains 23 tools after removing `directory_tree`; `tree` is the sole recursive tree API. Output fields use camelCase, single-tool errors expose `_meta.errorCode`, and batch errors share the same stable vocabulary. Empty files are explicitly assumed UTF-8, ambiguous non-empty content requires an encoding override, and UTF-32 remains BOM-management only. Separate `MCP_MAX_*` limits cover file input, decoded characters, line length, batch size, matches, output, and future HTTP sessions while `MCP_MEMORY_THRESHOLD` remains a deprecated file/output fallback. The complete change set and migration table are protected by schema, catalog, configuration, encoding-policy, manual MCP, and regression tests.

---

# R11 — Transport-independent server architecture

## Goal

Run one shared MCP server implementation through stdio and Streamable HTTP without duplicating tool registration, policies, roots, or error behavior.

## Architecture decision

- Allowed directories are process-wide policy. Every connection or future HTTP session attached to one server process shares the same configured roots, tool catalog, limits, execution flags, and error behavior.
- Sessions isolate protocol lifecycle, requests, cancellation, and concurrency; they are not per-agent filesystem identities, ACLs, or sandboxes.
- Prompt instructions may restrict an agent to writing in one project while reading shared projects, documentation, or libraries, but the server does not enforce those per-agent conventions.
- Technical isolation requires separate server processes with narrower roots and, where concurrent Git writes are possible, separate checkouts or worktrees.
- Startup directories are authoritative and immutable for the process. Dynamic MCP client roots remain a stdio-only compatibility path when no startup directories are configured. Future HTTP sessions will not mutate process roots.
- R11 does not add an HTTP listener, authentication, session manager, or network policy; those remain ordered work for R12 and R13.

## Checklist

- [x] Separate configuration loading from CLI parsing.
- [x] Separate server construction from transport startup.
- [x] Keep one authoritative tool catalog and registration path.
- [x] Define a transport-neutral server lifecycle abstraction.
- [x] Define explicit CLI/config transport selection.
- [x] Preserve stdio as a supported transport.
- [x] Keep allowed-directory policy authoritative and transport-independent.
- [x] Keep execution feature flags and authorization identical across transports.
- [x] Make logging, cancellation, graceful shutdown, and update checks lifecycle-aware.
- [x] Add equivalence tests for tools/list metadata and representative tool calls across transport adapters.

## Completion gate

The stdio executable uses the new architecture without behavior regression, and a second transport can be attached without duplicating handlers or weakening policy boundaries.

## Completion record

Completed on 2026-07-27. Configuration loading, CLI parsing, server construction, and transport startup are separate. `BuildServer` owns one shared 23-tool registration and process-wide policy; the stdio runner is lifecycle-aware and responds to process cancellation; explicit `--transport=stdio` and `MCP_TRANSPORT=stdio` preserved stdio as the sole implemented transport at the R11 boundary. Multiple connections to one server were verified to expose equivalent tool catalogs and the same configured roots, while provided configuration controls handler behavior. Dynamic client roots are limited to roots-capable stdio clients started without configured directories, and empty updates remove stale dynamic access. The complete race-detector suite and Gitleaks scans of Git history plus the working tree passed. R12 and R13 subsequently added the reviewed security design and native HTTP implementation on top of this architecture.

---

# R12 — Streamable HTTP security design

## Goal

Approve a concrete threat model and secure defaults before exposing filesystem and optional execution tools over an HTTP listener.

The approved design is [`HTTP_SECURITY.md`](HTTP_SECURITY.md). It is the source of truth for R13 HTTP configuration, trust boundaries, middleware order, accepted risks, security tests, and release blockers.

## Checklist

- [x] Document assets, trust boundaries, actors, and supported deployment models.
- [x] Preserve the R11 process-wide root model: all HTTP sessions share startup roots, client roots cannot mutate them, and per-agent isolation uses separate processes when required.
- [x] Bind to loopback by default.
- [x] Require explicit configuration for non-loopback binding.
- [x] Define token authentication and reverse-proxy integration.
- [x] Use constant-time credential comparison where applicable.
- [x] Define secure token loading without command-line secret exposure.
- [x] Validate `Host` and `Origin` and address DNS rebinding.
- [x] Disable CORS by default.
- [x] Define TLS expectations for direct and reverse-proxy deployments.
- [x] Set request body, header, connection, and concurrency limits.
- [x] Set read-header, request, idle, session, and shutdown timeouts without breaking SSE.
- [x] Define session identifiers, expiry, cleanup, and hijacking protections.
- [x] Define CSRF and browser-origin protections.
- [x] Prevent sensitive headers, tokens, file contents, commands, and session identifiers from entering logs.
- [x] Keep `run_script` and `shell` disabled by default.
- [x] Require a distinct explicit opt-in for execution over HTTP.
- [x] Define rate limiting and denial-of-service behavior.
- [x] Define trusted-proxy handling without accepting spoofed forwarding headers.
- [x] Define health and readiness endpoints that expose no sensitive data.
- [x] Define security tests and release-blocking findings.

## Completion gate

A reviewed security design exists, every identified threat has a mitigation or explicit accepted risk, and implementation tests are specified before HTTP code is merged.

## Completion record

Completed on 2026-07-27. [`HTTP_SECURITY.md`](HTTP_SECURITY.md) defines a fail-closed stateful Streamable HTTP profile with loopback binding, mandatory bearer authentication on every MCP request, exact Host and all-method Origin validation, no CORS, explicit non-loopback/TLS/proxy rules, bounded bodies, headers, requests, sessions, rate state, and timeouts, redacted logging, minimal health endpoints, and a second execution opt-in. It preserves R11 process-wide roots, disables HTTP client roots, keeps the initial event store unset, records SDK integration constraints, and lists required negative tests plus release-blocking findings before R13 implementation.

---

# R13 — Native MCP Streamable HTTP

## Goal

Implement the MCP Streamable HTTP transport according to R11 and R12 while preserving stdio behavior.

R13 must implement [`HTTP_SECURITY.md`](HTTP_SECURITY.md) without broadening its accepted risks or adding an unauthenticated compatibility mode.

## Checklist

- [x] Implement the Streamable HTTP endpoint using the shared server and the approved security middleware order.
- [x] Support required JSON-RPC request and streaming response behavior.
- [x] Implement session creation, lookup, expiration, and cleanup.
- [x] Propagate disconnect and request cancellation into tool contexts.
- [x] Implement graceful shutdown for listeners and active sessions.
- [x] Add health and readiness endpoints.
- [x] Apply authentication, Host/Origin, per-request and aggregate size, timeout, rate, session, and concurrency policies from R12.
- [x] Reject malformed content types, methods, JSON, protocol messages, query credentials, and event replay deterministically.
- [x] Keep stdio startup and protocol output unchanged.
- [x] Test simultaneous clients and concurrent sessions.
- [x] Test disconnect, timeout, cancellation, POST-paused idle expiry, SSE-only expiry, cleanup, and shutdown races.
- [x] Test oversized known-length/chunked payloads, oversized headers, saturation, bounded limiter state, and resource cleanup.
- [x] Compare complete tool metadata and representative tool results across stdio/direct and HTTP adapters.
- [x] Verify allowed-directory and dual execution policies on both transports, including shared process roots across simultaneous HTTP sessions.
- [x] Run native HTTP end-to-end tests and the existing stdio manual harness; the OpenAI Secure MCP Tunnel deployment remains stdio and was not changed or restarted.

## Completion gate

All 23 retained tools operate through native Streamable HTTP and stdio with equivalent schemas and policy boundaries, and the R12 security suite passes.

## Completion record

Completed on 2026-07-27. The executable now selects `stdio` or stateful `streamable-http` while constructing the 23 tools once through `BuildServer`. Native HTTP is loopback-bound and bearer-authenticated by default, validates exact Host and all-method Origin values, emits no CORS allow headers, disables HTTP client roots and event replay, supports optional TLS and bounded trusted-proxy handling, exposes minimal health/readiness routes, and coordinates graceful shutdown. Per-request and aggregate body budgets, non-SSE concurrency, live sessions, bounded per-peer rate state, idle cleanup, header limits, and cancellation are enforced before or around the pinned SDK handler. HTTP execution requires its own opt-in in addition to the existing tool authorization, and token-source variables are removed from the process environment after startup snapshotting.

Tests verified multiple simultaneous HTTP clients, unique sessions, DELETE and idle cleanup, SDK-aligned POST-paused and SSE-only expiry, cancellation propagation, authentication on every method, Host/Origin and proxy rejection, known-length and chunked `413` behavior, aggregate/concurrency `429` behavior, oversized-header `431`, log and TLS-path redaction, immutable process roots after an HTTP roots notification, and equivalence with the direct adapter for all tool metadata, CP1251 reads, allowed directories, and representative typed errors. Focused and complete Go tests, `go vet`, Staticcheck, govulncheck, the full race detector, the stdio manual harness, Node release tests, documentation/catalog checks, and Gitleaks content/history scans passed.

---

# R14 — Hardening and 2.0.0 release

## Goal

Finish platform, container, CI, documentation, packaging, migration, and release verification for the first public 2.x release.

## Final deployment verification

The controlled active rollback and restoration were completed on 2026-08-03.

## Checklist

- [x] Align Docker builder Go version with `go.mod`.
- [x] Pin the container bases and CI/release action versions.
- [x] Run the container as a non-root user.
- [x] Define mounts, temporary storage, allowed roots, healthcheck, and shutdown behavior.
- [x] Cover all six Windows/Linux/macOS amd64/arm64 targets in CI.
- [x] Ensure documentation and catalog changes trigger consistency tests.
- [x] Add HTTP tests to supported CI platforms.
- [x] Run race detector, vet, Staticcheck, govulncheck, actionlint, ShellCheck, and Gitleaks on the R14 working tree.
- [x] Add bounded fuzzing for detection, decoder chunking, line framing, HTTP parsing, proxy chains, and JSON-RPC inputs.
- [x] Run deterministic load, resource-accounting, cancellation, and session-cleanup soak tests, including 102,400 admitted/rejected requests and repeated race-detector cycles.
- [x] Build and runtime-smoke the Linux/amd64 container locally with UID/GID 10001, a read-only root filesystem, dropped capabilities, `no-new-privileges`, bounded temporary storage, SDK-driven stdio MCP, direct-TLS HTTP, negative security responses, health/readiness, and graceful shutdown.
- [x] Pass the native binary MCP smoke gate on Windows, Linux, and macOS GitHub runners.
- [x] Pass the Ubuntu container gate for non-root execution, hardened stdio, direct-TLS HTTP, security responses, health/readiness, and graceful shutdown.
- [x] Remove the stale GoReleaser Registry TODO and keep Registry publication in the checksum-verified workflow.
- [x] Normalize archive entry metadata to commit-derived values and verify that two independent GoReleaser snapshots produce identical checksums for all 6 raw binaries and 6 platform archives.
- [x] Generate internal prerelease manifests from both the direct six-target build and the reproducible GoReleaser snapshot checksums, then pass `mcp-publisher 1.7.9 validate` without login or publication.
- [x] Verify the known-good R10 rollback binary offline: exact hash/version, 23-tool stdio startup, and byte-identical two-reference launcher reversal/restore.
- [x] Update README, TOOLS, catalog, tunnel/HTTP examples, Smithery metadata, container docs, and publishing notes.
- [x] Finish the 1.8-to-2.0 migration guide.
- [x] Remove the optional Claude Code downloader plugin and marketplace metadata rather than carry a second network installer and cache trust boundary into 2.0.
- [x] Run the complete release checklist in [PUBLISHING.md](PUBLISHING.md).
- [x] Create and push `v2.0.0` only after all prior gates pass.
- [x] Verify release binaries, archives, checksums, signatures where available, and MCP Registry publication.
- [x] Deploy the published 2.0.0 runtime and execute live stdio plus authenticated Streamable HTTP smoke tests.
- [x] Execute the controlled active rollback, restore 2.0.0, and record the final handoff.

## Publication record

Published on 2026-08-02. Fork tag `v2.0.0` resolves to commit `1530fbb1eab529a1ef7236b4b3df8ab84a9a0d1d`. The tag workflow passed the complete Linux, Windows, and macOS test matrix, produced six raw binaries, six deterministic platform archives, and `checksums.txt`, and published the historical `io.github.zoster81/mcp-file-tools` version `2.0.0` to the MCP Registry through GitHub OIDC. All 12 published binary/archive checksums were independently verified against the release checksum file. No separate signature assets were emitted by the configured release pipeline.

Operator deployment completed on 2026-08-02. The published Windows amd64 `2.0.0` binary now runs through both the stdio tunnel path and the native loopback Streamable HTTP path. Live verification confirmed the embedded version, HTTP health/readiness, unauthenticated `401`, authenticated session initialization, and the complete 23-tool catalog. On 2026-08-03, a controlled active rollback to the retained R10 build verified the complete 23-tool stdio catalog while the later HTTP transport was intentionally absent. The published `2.0.0` runtime was then restored and reverified over stdio and authenticated Streamable HTTP, including the complete tool catalog and expected health, readiness, and authentication responses.

## Completion gate

`v2.0.0` is reproducible, verified across supported targets, includes secure native MCP Streamable HTTP and stdio, has complete migration documentation, and passes deployment plus rollback verification.

---

# R15 — Agent ergonomics and project-aware workflows

## Status

Completed on 2026-08-03. The implementation is included in the current `2.1.1` release.

## Goal

Reduce unnecessary tool calls and token usage for common repository and encoding workflows while preserving the fork's bounded-memory pipeline, durable mutation semantics, stable public errors, process-wide root model, and stdio/HTTP equivalence.

## Provenance and accepted scope

The [original project](PROJECT_DIRECTION.md#reciprocal-feature-exchange) is the explicit source for this R15 feature set and the implementation approaches reviewed during design. The fork accepts this work as part of a reciprocal exchange of useful functionality and techniques, while reworking the code against its own APIs, security model, bounded-memory pipeline, durable mutation guarantees, and dual-transport architecture rather than synchronizing it mechanically. See [PROJECT_DIRECTION.md](PROJECT_DIRECTION.md#reciprocal-feature-exchange).

## Implementation checklist

- [x] Add optional absolute line numbers to paged `read_text_file` results without changing default output.
- [x] Add grep output modes for content, matching paths, and per-file counts.
- [x] Add grep paging, pattern arrays, plural include/exclude filters, and matches-only output under existing limits.
- [x] Add `.gitignore`-aware traversal with explicit opt-out, nested rules, negation, and secure regular-file validation.
- [x] Add deterministic bounded sorting for directory/search results by name, modification time, or size.
- [x] Add batch encoding conversion, dry-run previews, per-file partial results, and machine-readable unsupported-character locations.
- [x] Add transport-independent MCP prompts for encoding audits, mojibake diagnosis, and controlled UTF-8 migration.
- [x] Add strict single-file unified-diff edit input with bounded hunk parsing and exact context validation.
- [x] Add opt-in fuzzy edit matching with explicit thresholds, deterministic complexity bounds, unique-best-match requirements, and safe ambiguity failure.
- [x] Complete repository-wide tests, race/static/security checks, documentation consistency, and final diff review.

## Design constraints

- Do not copy or mechanically synchronize another repository's implementation; design against this fork's current APIs and invariants.
- Preserve the existing 23-tool contract unless a deliberate versioned API decision justifies a change.
- Keep all read/search additions bounded by `MCP_MAX_*` limits and deterministic ordering.
- Route mutations through the shared encoding-aware document and durable filesystem layers.
- Keep prompts and tool metadata identical over stdio and Streamable HTTP.
- Do not weaken HTTP authentication, Host/Origin validation, session limits, logging redaction, or dual execution authorization.
- Do not add per-session filesystem ACLs or let HTTP clients mutate process roots.

## Completion gate

Accepted R15 features demonstrate measurable call/token reduction, pass normal and adversarial tests on supported platforms, remain bounded under large inputs, preserve exact mutation guarantees, and expose equivalent schemas and behavior through both transports.

## Completion record

Completed on 2026-08-03. The existing 23-tool catalog gained backward-compatible optional fields for line-numbered reads, richer paged grep, `.gitignore`-aware traversal, explicit bounded sorting, batch conversion previews, strict unified patches, and ambiguity-safe fuzzy edits. Three shared MCP prompts now guide encoding audits, mojibake diagnosis, and controlled UTF-8 migration over both stdio and Streamable HTTP. The original project is credited for both the feature concepts and implementation approaches reviewed, while the resulting code was reworked for the fork's secure walker, bounded-memory pipeline, durable mutation layer, stable schemas, process-wide roots, and dual-transport security model.

Normal, compatibility, malformed-input, symlink/reparse, `.gitignore`, paging, starvation, backup-collision, partial-batch, unsupported-character, patch, fuzzy-ambiguity, concurrency, and HTTP equivalence tests passed. A deterministic fixture reduced retained grep JSON from 2,963 bytes in content mode to 379 bytes in path-only mode, while batch conversion consolidates multiple files into one MCP request. Complete Go tests, the race detector, vet, Staticcheck, govulncheck, manual MCP checks, release-script tests, documentation/catalog identity checks, link validation, and Gitleaks passed. R15 has not yet been packaged into a public release or deployed to an operator runtime.

---

# R16 — Verified change workflows

## Status

Completed on 2026-08-04. Deterministic fingerprints, bounded one-shot `edit_file` preview/apply, complete `patch-package-v1` inspect/dry-run/apply/verify, and typed `verify_state` checks are implemented, verified, and included in the current `2.1.1` release.

## Goal

Make agent-driven mutations explicitly approvable, state-bound, reproducible, and verifiable without weakening the fork's existing path security, encoding preservation, durable single-file mutation guarantees, bounded-memory behavior, stable errors, or transport equivalence.

The approved design baseline is [VERIFIED_CHANGE_WORKFLOWS.md](VERIFIED_CHANGE_WORKFLOWS.md). That document is authoritative for R16 scope, security boundaries, sequencing, non-goals, and the deferred persistent-backup gate.

## Approved implementation scope

- [x] Add deterministic streamed fingerprints for explicit files and directory roots through the secure walker.
- [x] Add bounded one-shot preview/apply for existing `edit_file` operations, with cryptographically unguessable expiring identifiers, replay prevention, and target/result fingerprint validation.
- [x] Add a versioned declared patch-package format with strict inspect and non-mutating dry-run actions for bounded edits to existing regular files.
- [x] Add one-shot patch-package apply and verify, preflight and stage every target before the first commit, and report committed, unchanged, or unknown final states with `PARTIAL_COMMIT` without claiming multi-file atomicity or retained rollback.
- [x] Add selected structured verification checks with typed arguments, bounded diagnostics, fixed direct Git invocation, filtered environment, and no arbitrary shell command.
- [x] Keep schemas, limits, error metadata, prompts, and behavior equivalent over stdio and stateful Streamable HTTP.
- [x] Complete focused TDD, adversarial path/cache/replay/partial-commit tests, full regression, race, static-analysis, vulnerability, manual MCP, catalog, documentation, and repository checks.

## Separately governed follow-on work

Persistent backup storage and user-managed change review remain outside R16. R17 approved their lifecycle design, and R18 implemented it in separate phases rather than introducing an incidental unbounded `.bak` scheme or weakening the completed R16 contracts.

At the completed R18 boundary:

- omitted-policy edit/package workflows and direct edit create no persistent backup;
- edit preview may retain `backupPolicy: "required"`, causing apply to capture the approved pre-state before mutation;
- a patch-package manifest may retain the same exact policy, causing dry run to preflight and apply to capture every changed pre-state before the first commit;
- original-target restore requires a durable safety backup when the target exists;
- GC remains explicit, generation-bound, pin-aware, manifest-first, and reference-counted;
- package backups provide recovery evidence but no retained automatic rollback or multi-file atomicity;
- existing operation-specific `.bak` behavior remains unchanged.

## Design constraints

- Preserve backward-compatible direct edit behavior unless a separately approved API decision changes it.
- Use a process-local, bounded, expiring preview cache; process restart invalidates previews.
- Apply must consume the exact prepared operation rather than accepting resubmitted edit instructions.
- Fingerprints exclude unstable platform metadata by default and stream file content without full-file allocation.
- Patch packages initially edit existing regular files only; creation, deletion, move, rename, and `/dev/null` forms are rejected.
- Structured verification starts with filesystem- and repository-adjacent checks rather than a generic build or command runner.
- Never use a shell for structured verification.
- Do not claim complete atomicity across multiple files.
- Do not add per-session roots, HTTP root mutation, or weaker path validation.

## Implementation order

1. Fingerprint schema, shared primitive, and tests.
2. Bounded preview storage and `edit_file` preview/apply.
3. Patch-package manifest, inspect, and dry-run.
4. Patch-package apply, partial-commit reporting, and verify.
5. Initial structured verification checks.
6. Full R16 verification and documentation alignment.
7. Complete and approve a separate persistent-backup lifecycle design before implementation. **Completed as R17; implementation completed as R18.**

## Completion gate

R16 is complete only when the requirements and tests in [VERIFIED_CHANGE_WORKFLOWS.md](VERIFIED_CHANGE_WORKFLOWS.md) pass, all caches and outputs are bounded, stale or replayed approvals fail closed, package partial state is reported accurately, structured checks accept no arbitrary command strings, and no persistent backup system has been introduced without its separate approved design.

---

# R17 — Persistent backup lifecycle design

## Status

Completed on 2026-08-04. The approved contract is [PERSISTENT_BACKUP_LIFECYCLE.md](PERSISTENT_BACKUP_LIFECYCLE.md).

## Goal

Define a safe, bounded, crash-consistent persistent backup subsystem that can protect future approval-bound mutations and support user-reviewed restore and garbage collection without weakening allowed-root security, durable mutation semantics, transport equivalence, or current compatibility.

## Design checklist

- [x] Separate the store from public allowed directories and identify it as a new operator-configured internal authority.
- [x] Define one-writer process locking and reject overlapping, aliased, linked, or reparse-backed store paths.
- [x] Define immutable SHA-256 content-addressed objects and immutable manifests as the durable source of truth.
- [x] Keep the index derived and rebuildable rather than a single authoritative mutable database.
- [x] Define conservative total-byte, object, manifest, per-target, pin, retention, plan, output, and time limits.
- [x] Require quota reservation and durable object-plus-manifest capture before a protected target mutation begins.
- [x] Keep backup policy explicit, disabled by default, and bound into edit or package approval capabilities.
- [x] Define restore preview/apply with exact-byte verification, stale-target rejection, and mandatory safety backup of an existing current target.
- [x] Define deterministic garbage-collection dry-run/apply with generation checks, manifest-first removal, reference counting, and no background deletion.
- [x] Define startup recovery, structural corruption behavior, quick/full audit, orphan and trash handling, and explicit secure-deletion limitations.
- [x] Define security, crash-injection, quota, restore, GC, concurrency, cross-platform, transport-equivalence, fuzz, build, and release verification requirements.
- [x] Review and explicitly approve all ten lifecycle decisions.
- [x] Create a separate phased implementation milestone with focused TDD and no incidental `.bak` migration.

## Completion record

Maintainers accepted all ten decisions on 2026-08-04: a dedicated non-overlapping store, one writer, disabled-by-default approval-bound backups, immutable objects and manifests with a derived index, the documented quotas and retention defaults, original-target restore with mandatory safety backup, explicit GC dry-run/apply, deferred application-managed encryption and secure deletion guarantees, unchanged adjacent `.bak` behavior, and no automatic patch-package rollback. The design was completed before runtime implementation began.

---

# R18 — Persistent backup implementation

## Status

Completed on 2026-08-05. The approved [persistent backup lifecycle](PERSISTENT_BACKUP_LIFECYCLE.md) is implemented and verified. R18 brought its source boundary to 27 tools; R21 expands the current `2.1.1` release to 30 tools, while `2.0.0` remains the historical 23-tool rollback baseline.

## Goal

Implement durable exact-byte capture, bounded metadata management, approval-bound mutation protection, safe restore, and explicit garbage collection without weakening the existing root, encoding, memory, mutation, transport, or error contracts.

## Implementation phases

- [x] Phase 1 — Add disabled-by-default configuration, approved defaults and hard maxima, a dedicated canonical non-overlapping store root, owner-only permissions, a platform-native lifetime writer lock, immutable `backup-store-v1` descriptor, empty versioned layout, fail-closed structural validation, and protected-root denial for ordinary tools.
- [x] Phase 2 — Add immutable SHA-256 object capture, immutable checksummed manifests, quota reservations, derived index rebuild, bounded startup recovery, and quick/full internal audit primitives.
- [x] Phase 3 — Add the read-only `backup_store` status/list/inspect/audit surface with bounded cursors, redacted metadata, catalog/schema tests, and stdio/HTTP equivalence.
- [x] Phase 4 — Bind `backupPolicy: "required"` into `edit_file` preview/apply and durably capture the approved pre-state before mutation.
- [x] Phase 5 — Add package-wide reservation and all-target backup capture before the first `patch_package` commit while preserving explicit `PARTIAL_COMMIT` semantics and no automatic rollback.
- [x] Phase 6 — Add original-target restore preview/apply with exact object verification, stale-target rejection, and mandatory safety backup of an existing target.
- [x] Phase 7 — Add generation-bound GC dry-run/apply, immutable pin-at-creation semantics, manifest-first removal, reference-counted object deletion, trash recovery, and no background deletion.
- [x] Complete failure injection, fuzzing, race, static-analysis, vulnerability, documentation, transport-equivalence, six-target build, native runtime smoke, and release gates for the full subsystem.

## Phase 1 behavior

`MCP_BACKUP_STORE_DIR` remains unset by default. When configured in phase 1, startup validates or creates an empty internal store, restricts it to the process identity, acquires one exclusive lifetime lock, validates the immutable descriptor and expected empty layout, and excludes the store from every ordinary filesystem root. Relative, filesystem-root, overlapping, aliased, symlinked, junction-backed, reparse-backed, special-file, unexpectedly populated, malformed, unsupported, permissively owned, or concurrently locked stores fail startup without exposing their paths.

Phase 1 does not capture target bytes, write backup manifests, reserve quota, expose a management tool, change edit or package schemas, restore targets, garbage-collect data, migrate `.bak` files, add encryption keys, or claim rollback.

## Phase 2 behavior

Phase 2 adds an internal-only exact-byte capture transaction. It conservatively reserves total bytes, one manifest, per-target unpinned versions, and optional immutable pin capacity; retains stable target identity; streams and revalidates the target; installs or fully verifies a content-addressed SHA-256 object; commits a strict checksummed `backup-manifest-v1` record only after the object is durable; and then rebuilds the disposable `backup-index-v1` projection. The persisted index stays compact by recording only generation and aggregate counts, while ordered manifest, object, and target details are rebuilt in memory from authoritative files. Identical content is deduplicated only after full verification. Durable orphan objects remain accounted for when a later manifest step fails, while a manifest committed before an index error remains authoritative and is returned with the error.

Startup now performs bounded directory scans, rejects malformed manifests, missing referenced objects, links, reparse points, hard-linked internal files, inconsistent metadata, and structural permission failures, and rebuilds a missing, corrupt, stale, or tampered derived index. Capture revalidates the retained store-root identity and internal layout before staging and before durable installation. Quick internal audit validates structure, references, sizes, recovery residue, orphans, and index consistency; full audit additionally hashes every referenced object under explicit object and byte limits. Audit is read-only and never deletes or repairs uncertain data.

Phase 2 remains unreachable directly from MCP clients. It did not add `backup_store` or change the then-current 26-tool source catalog, and it still does not automatically capture any mutation, bind `backupPolicy` into previews, restore files, garbage-collect data, mutate pin state, or change adjacent `.bak` behavior.

## Phase 3 behavior

Phase 3 adds one always-registered read-only `backup_store` tool and brings the unreleased source catalog to 27 tools over both transports. `status` reports whether the operator configured a store and, when enabled, returns redacted format, health, generation, aggregate counts, configured limits, and bounded path-free issues. `list` returns newest-first manifest metadata with exact target/pinned filters, a maximum page size of 100, current-root visibility filtering, and an authenticated keyset cursor bound to filters, the allowed/protected-root policy snapshot, and store generation; target visibility is revalidated on every page. `inspect` requires one backup ID, revalidates current target authorization before hashing, and fully hashes the referenced object before returning metadata. `audit` exposes bounded quick or full read-only integrity scans and never repairs or deletes data.

The strict action union rejects unknown fields and cross-action parameters. Every output is bounded by `MCP_MAX_OUTPUT_BYTES`. Store IDs, object bytes, internal store paths, temporary paths, and capability secrets are never returned. With no configured store, only `status` succeeds and reports `enabled: false`; list, inspect, and audit fail explicitly. At the phase-3 boundary no mutation path created backups or introduced restore, garbage collection, pin mutation, or rollback.

## Phase 4 behavior

Phase 4 additively extends `edit_file` preview with the exact value `backupPolicy: "required"`; omitted policy remains the default and direct editing rejects the field. The policy is retained in the one-shot capability, while apply continues to accept only `previewId`. Preview creates no persistent state. Apply consumes the token, repeats path, stable-identity, target-fingerprint, prepared-result, cancellation, and output-limit checks, then calls the process-owned store to capture the exact authorized pre-state before any permission change or target replacement.

A changed edit returns `backupId` only after the strict manifest is durable and verified against the approved path, source operation, and target fingerprint. Capture or quota failure prevents mutation. A durable manifest remains authoritative if the derived index refresh fails, and apply may continue; a later commit failure remains an MCP error but preserves `backupId` in structured output with `applied: false`. Logical no-ops, omitted policy, and direct editing create no persistent backup. Tests cover UTF-16 BOM/CRLF, CP1251 exact bytes, read-only metadata, output limits, cancellation, quota rejection, post-capture target change, post-backup write failure, index degradation, replay, and concurrent apply over both transports.

## Phase 5 behavior

Phase 5 extends the strict `patch-package-v1` manifest with exact optional `backupPolicy: "required"`. Inspect validates syntax without store access. Dry run requires package batch authority only for required policy, retains the policy in the one-shot capability, computes the changed target set, and performs a side-effect-free conservative aggregate admission check over full source bytes, manifests, pin capacity, and per-target versions. Omitted policy and all-no-op packages preserve prior behavior.

Apply consumes the capability, preflights output including every possible backup ID, revalidates and stages all changed outputs, atomically reserves the complete package backup budget, then captures changed pre-states in manifest order. Every durable result is checked against its ID shape, normalized target, `patch_package` source operation, and approved fingerprint. All targets are revalidated after capture; no target commit can begin unless every changed target has a verified durable manifest. Incomplete capture returns the durable prefix and cleans staging without server mutation. Complete manifests remain authoritative across derived-index errors. Later per-file failures preserve all backup IDs and the existing committed/unchanged/unknown `PARTIAL_COMMIT` contract; no rollback is attempted.

Tests cover conservative dedup admission, duplicate targets, reservation release, concurrent overcommit, ordered manifests, quota/output failure, no-op exclusion, incomplete durable prefixes, post-capture target changes, derived-index degradation, first-commit barrier, per-target ID inspection, MCP structured errors, replay, partial commit, and HTTP/direct cross-session equivalence.

## Phase 6 behavior

Phase 6 extends the existing `backup_store` tool without adding a 28th tool. Because restore can mutate an authorized target, catalog annotations now mark the tool non-read-only and destructive; `status`, `list`, `inspect`, and `audit` themselves remain read-only. `restorePreview` accepts only `backupId`, authorizes only the immutable manifest's original target, retains open manifest/object identities, fully verifies the object, captures the exact existing fingerprint or missing state, preflights the mandatory safety backup, and returns a bounded expiring 256-bit capability. Preview never creates an object, manifest, or target mutation.

`restoreApply` accepts only `previewId` and consumes it before validation. It revalidates current roots, target resolution, retained identities, current fingerprint or missing state, manifest checksum, object identity, size, permissions, and SHA-256. Exact object bytes are streamed into target-adjacent synced staging and the staged digest must match the manifest. When the target exists, its current state is durably captured with `sourceOperation=restore` and verified before any permission change or replacement. The target is then revalidated again. Existing targets use optimistic replacement; missing targets use no-replace creation. Original mode and modification time are restored, final bytes are fingerprinted, and the source backup is never consumed.

Restore plans are bounded to 64 live capabilities, 16 MiB retained metadata/diffs, the configured `MCP_BACKUP_PLAN_TTL_SECONDS`, and a 1 MiB per-state diff input. Diffs are omitted for oversized, binary, or ambiguously decoded states while exact-byte restore remains available. Replay, expired plans, changed authorization, stale targets, appearing missing targets, changed source objects, quota changes, cancellation, and output-limit failure all fail closed. Existing-target failures preserve every durable `safetyBackupId`; post-replacement errors classify the actual target as restored, unchanged, missing, or unknown without automatic rollback. Tests cover exact text/binary bytes, read-only targets, missing-target races, safety-backup admission and post-capture changes, source corruption, concurrent apply, structured MCP output, catalog drift, manual harness, and HTTP/direct cross-session equivalence.

## Phase 7 behavior

Phase 7 extends the same `backup_store` tool with strict `gcDryRun` and `gcApply`, preserving the 27-tool catalog. Dry run accepts no policy fields, performs an authoritative bounded scan at one fixed UTC instant, rejects active single or package capture reservations, and retains one exact deterministic plan behind a separate 64-entry/16 MiB one-shot capability cache using `MCP_BACKUP_PLAN_TTL_SECONDS`. Pinned manifests and active restore sources are excluded. At least one manifest per target is retained; additional unpinned manifests are selected oldest-first when older than `MCP_BACKUP_RETENTION_DAYS` or needed to reduce unpinned versions to `MCP_BACKUP_MAX_VERSIONS_PER_TARGET`. Objects are digest-ordered and selected only when post-plan references are zero, including pre-existing orphans. MCP output omits target paths.

Apply consumes only `previewId`, holds the store transaction boundary, blocks new reservation admission, reconstructs the complete plan at the original policy instant, and rejects changed generation, pin state, manifests, objects, active restore references, reservations, or reference counts before deletion. Selected manifests move with no-replace into typed trash first. Live references are rescanned, then only zero-reference objects that pass full SHA-256 verification move into typed trash. Trash removal is best effort; partial errors preserve removed counts, reclaimed bytes, residue, and resulting generation. The derived index is rebuilt after every durable partial outcome, including cancellation through an independent recovery context. Startup deletes only recognized valid typed GC trash and preserves unknown or uncertain entries. GC never touches public target files, never runs automatically, never mutates pin state, never guarantees secure deletion, and never attempts rollback.

Tests cover retention and version-limit policy, one-version floor, immutable pins, shared-object reference counts, orphan reclamation, deterministic planning, stale generation, active reservations, active restore references, replay, bounded capability eviction/expiry, output limits, manifest-first and object-phase failure injection, cleanup residue, startup recovery, partial structured MCP errors, manual harness, and HTTP/direct cross-session capability use. The phase also fixes leaked empty target reservation accounting after pinned capture.

## Completion gate

R18 is complete only when every phase is implemented and reviewed, live manifests can never reference missing durable objects by intentional ordering, required mutations cannot start before durable backup capture, restore always preserves an existing current state, GC cannot remove a referenced object, all state and outputs remain bounded, ordinary tools cannot access the store, and the complete cross-platform and release verification matrix passes.

## Completion record

Completed on 2026-08-05. A full lifecycle regression verifies approval-bound edit capture, original-target restore with a durable safety backup, generation-bound GC, full audit, and repeated store reopen while public target bytes remain unchanged by GC. The complete package suite and race detector passed in deterministic shards; `go mod tidy -diff`, module verification, vet, Staticcheck, govulncheck, workflow linting, manual MCP checks, Node release tests, and all five persistent-format fuzz targets passed. Six Windows/Linux/macOS amd64/arm64 binaries and backup-store test executables compiled, the generated six-package Registry manifest passed MCP Publisher 1.7.9 validation, and native Windows stdio smoke exposed all 27 tools. A temporary Linux/amd64 container passed UID 10001, read-only-root, dropped-capability, `no-new-privileges`, bounded-tmpfs, stdio MCP, direct-TLS HTTP readiness, `401`/`403`/`405`, no-CORS, and clean shutdown checks.

---

# R19 — Offline backup store diagnostics

## Status

Completed on 2026-08-06. The authoritative contract is [OFFLINE_BACKUP_DIAGNOSTICS.md](OFFLINE_BACKUP_DIAGNOSTICS.md). The milestone provides the approved diagnostic-only design, mutation-free existing-store opener, bounded fail-soft scanner, strict offline JSON CLI, and complete verification gate. Repair behavior and MCP/server runtime behavior remain unchanged and unavailable.

## Goal

Provide deterministic bounded path-free diagnosis of an already existing persistent backup store when normal server startup rejects it, without creating, rebuilding, repairing, cleaning, renaming, or deleting any store state.

## Design constraints

- Diagnostics run offline and outside MCP transports.
- The store must already exist and must expose an existing owner-only single-link lock file.
- The command acquires the same exclusive writer boundary without create semantics and rejects an active server.
- The diagnostic path never calls normal mutating startup helpers such as descriptor/layout creation, index persistence, or GC-trash recovery.
- Quick mode remains metadata-only; full mode hashes referenced object bytes under explicit bounds.
- Output is one versioned deterministic JSON document containing fixed issue codes and no store paths, target paths, labels, raw manifests, temporary names, or object bytes.
- Any skipped, truncated, or limited check prevents a healthy result.
- Normal stdio, Streamable HTTP, MCP schemas, backup behavior, restore, and GC remain unchanged.
- Repair, quarantine, salvage, clone, migration, or other mutation requires a separate later design decision.

## Implementation phases

- [x] Phase 1 — Define the command boundary, existing-only lock semantics, output schema, bounds, compatibility, non-goals, tests, and completion gate.
- [x] Phase 2 — Add a non-creating existing-store opener with retained root/lock identity and mutation-negative tests.
- [x] Phase 3 — Add fail-soft bounded diagnostic scanning without weakening normal fail-closed startup.
- [x] Phase 4 — Add strict `backup-store diagnose` CLI parsing, deterministic JSON, exit codes, cancellation, and compatibility tests.
- [x] Phase 5 — Complete failure injection, race, fuzz, six-target compilation, native CLI smoke, documentation, and security gates.
- [x] Phase 6 — Review diagnostic evidence before deciding whether any separate recovery or repair design is justified. **No repair, quarantine, salvage, clone, or migration capability is justified or authorized without future operator evidence and a separate milestone.**

## Completion gate

R19 is complete only when healthy and corrupt existing stores can be diagnosed on Windows, Linux, and macOS without any filesystem mutation; active stores are rejected by the same exclusive lock boundary; reports remain deterministic, bounded, and path-free; limited scans never claim health; normal server behavior remains unchanged; and all focused, regression, race, static-analysis, vulnerability, six-target, native smoke, documentation, and security checks pass.

## Completion record

Completed on 2026-08-06. `backup-store diagnose` requires an explicit existing store, acquires the pre-existing owner-only single-link lock without create flags, validates root/lock/descriptor/layout identity, and emits one bounded deterministic `backup-diagnostic-v1` JSON document. Quick mode performs metadata validation; full mode additionally hashes referenced objects. Missing or stale derived index state is reported as maintenance without being rebuilt, while descriptor, layout, manifest, object, permission, link, limit, cancellation, and concurrent-change evidence fail closed. Mutation-negative tests prove that missing descriptor/index state, incomplete layout, staging, trash, orphan data, bytes, modes, modification times, and namespace remain unchanged.

Verification passed on the definitive source tree: `go mod tidy -diff`, `go mod verify`, the complete Go suite, complete race detector, vet, Staticcheck, govulncheck with no vulnerabilities, six bounded fuzz campaigns, Node release tests 7/7, manual 27-tool MCP harness, GoReleaser configuration, actionlint/ShellCheck workflow checks, Windows/Linux/macOS amd64/arm64 command and test compilation, and a native Windows CLI smoke proving exit `0` for a healthy store and exit `2` for a missing rebuildable index without recreating it. Project-identity/catalog tests, strict UTF-8/no-BOM/LF/trailing-whitespace checks, Markdown links, Gitleaks content/history scans, and clean diff checks passed. No repair, quarantine, salvage, clone, or migration capability was added.

---

# R20 — MCP 2026-07-28 adoption readiness

## Status

Completed in source on 2026-08-08. The authoritative contract is [MCP_2026_07_28_ADOPTION.md](MCP_2026_07_28_ADOPTION.md). Stable SDK `v1.7.0` provides modern stdio discovery where roots policy permits it and same-endpoint stateless `2026-07-28` HTTP beside the retained stateful legacy handler. Stateless traffic emits no session ID and consumes no legacy session capacity; legacy initialization, SSE, expiry, and DELETE remain intact. Unsupported singleton versions return the protocol-defined structured error without entering legacy session admission. The R20 boundary exposed 27 tools plus three prompts with shared process roots, backup store, and security policies; R21 expands the current `2.1.1` release to 30 tools. Compatibility, conformance, independent-client, fuzz, native, container, race, static-analysis, vulnerability, and six-target gates are complete. The verified R20 implementation is included in `2.1.1` and remains absent from the `2.0.0` rollback binary.

## Goal

Adopt final MCP protocol version `2026-07-28` through an official stable Go SDK while preserving supported legacy protocol behavior, the existing 27-tool source catalog, process-wide filesystem authority, and every stdio and HTTP security boundary.

## Design constraints

- The main dependency graph must not use a pre-release or pseudo-version SDK.
- Source SDK `v1.7.0` is authoritative for R20 implementation; publication and deployment remain separately governed until later approval.
- Stdio may offer `2026-07-28` when startup roots are configured or client roots are disabled; any session that still depends on deprecated client roots remains on the legacy compatibility path.
- HTTP keeps one endpoint and one outer security/admission pipeline, with separate stateful legacy and stateless new-protocol SDK handlers.
- Authentication, Host, Origin, proxy trust, rate, body budgets, concurrency, timeouts, logging, execution gates, readiness, and shutdown stay common across versions.
- Stateless requests create no session, emit no session ID, use no event store, and consume no legacy session capacity.
- `Mcp-Method` and `Mcp-Name` remain untrusted routing hints and never become authorization inputs.
- R20 adds no Apps, Tasks, MRTR workflow, OAuth server, application-selected positive cache lifetime, tracing export, tool, prompt, resource, or schema.
- Publication and deployment remain governed by the release gates in [PUBLISHING.md](PUBLISHING.md).

## Implementation phases

- [x] Phase 1 — Define the compatibility boundary, stable-SDK adoption gate, transport architecture, roots deprecation strategy, tests, risks, and completion gate.
- [x] Phase 2 — Qualified official stable Go SDK `v1.7.0` through a reversible temporary module change, completed API/security review, and recorded the required stdio middleware-gate design correction.
- [x] Phase 3 — Updated to stable SDK `v1.7.0`; enabled modern stdio discovery for configured-root and roots-disabled sessions; retained deterministic legacy initialization and roots notifications through a pre-discovery middleware gate when startup directories are absent.
- [x] Phase 4 — Implemented same-endpoint dual-generation HTTP behind the existing hardened middleware, with exact version routing, stateful legacy sessions, stateless `2026-07-28`, strict malformed-header rejection, cancellation propagation, and no stateless session admission.
- [x] Phase 5 — Completed conformance, downgrade, interoperability, security failure-injection, fuzz, native, and container gates while repeating race, static-analysis, vulnerability, and six-target verification.
- [x] Phase 6 — Aligned protocol, security, public status, publishing, and completion documentation without changing the published runtime.

## Completion gate

R20 is complete only when an official stable Go SDK supports final `2026-07-28`; stdio and HTTP accept the new version without relying on deprecated roots or protocol sessions; supported legacy clients retain current behavior; stateful and stateless HTTP share every security and resource-control boundary; tool catalogs, prompts, schemas, and representative results remain equivalent; malformed, unsupported, and downgrade cases fail deterministically; and the complete focused, regression, race, static, vulnerability, conformance, six-target, native, container, documentation, and security verification matrix passes.

## Completion record

Completed in source on 2026-08-08. Official conformance verifies the structured unsupported-version path and HTTP `400`; independent header validation passes 13/13, TypeScript SDK interoperability preserves legacy `2025-11-25`, the complete serial and race suites plus vet/Staticcheck/govulncheck pass, five HTTP/JSON-RPC fuzz targets pass, native and hardened-container smoke pass, and Windows/Linux/macOS amd64/arm64 command plus affected-test compilation succeeds. The verified R20 work is included in the current `2.1.1` release.
