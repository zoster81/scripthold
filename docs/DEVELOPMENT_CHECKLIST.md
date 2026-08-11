# Development Checklist

Use this portable checklist for every active milestone in [ROADMAP.md](ROADMAP.md). Repository-wide and scoped agent instructions live in [`AGENTS.md`](../AGENTS.md) files; contributor workflow lives in [`CONTRIBUTING.md`](../CONTRIBUTING.md).

Apply only the checks relevant to the change, but record omissions explicitly.

## 1. Requirements and edge cases

- [ ] Restate the intended behavior and observable outcome.
- [ ] Identify affected packages, handlers, schemas, metadata, documentation, workflows, and packaging.
- [ ] Identify compatibility constraints and intentional breaking changes.
- [ ] Define valid, empty, missing, malformed, ambiguous, oversized, and unsupported inputs.
- [ ] Define encoding, BOM, and LF/CRLF behavior where relevant.
- [ ] Define filesystem, permission, read-only, missing-path, and partial-failure behavior.
- [ ] Review path traversal, symlink, junction, reparse-point, and allowed-root risks.
- [ ] Review cancellation, timeout, concurrency, race, TOCTOU, and cleanup behavior.
- [ ] For prepared operations, define capability-token lifetime, entropy, replay, ownership, eviction, restart, and stale-target behavior.
- [ ] For multi-file operations, define preflight, staging, commit order, partial-commit reporting, crash behavior, and any explicit non-atomicity.
- [ ] Review Windows, Linux, macOS, long-path, and cross-platform implications.
- [ ] Identify likely regressions and public behavior that must remain unchanged.

## 2. Architecture and test strategy

- [ ] Define the smallest coherent component and file set before editing.
- [ ] Keep transport, MCP adapters, domain logic, and filesystem primitives separated.
- [ ] Reuse shared internal primitives instead of adding local copies.
- [ ] Define data flow, ownership, memory bounds, and cleanup responsibilities.
- [ ] Define deterministic fingerprint fields, canonical path representation, metadata exclusions, and cross-platform stability.
- [ ] Keep persistent backup retention, quota, registry, garbage collection, and restore policy behind an explicit approved design gate.
- [ ] Define typed errors and public error mapping.
- [ ] Define time and space complexity when relevant.
- [ ] Select focused failing tests before implementation when practical.
- [ ] Include normal behavior, edge cases, invalid input, and regression coverage.
- [ ] Include encoding/BOM and LF/CRLF cases where relevant.
- [ ] Include filesystem failure and concurrent-modification cases where relevant.
- [ ] Include cancellation, timeout, saturation, deterministic ordering, and race cases where relevant.
- [ ] For durable tasks, test idempotency conflict, queue and concurrency bounds, logical locks, frontend/worker/supervisor failure, reboot-style stale recovery, at-most-once behavior, process-tree cancellation, cursor gaps, retention, durability-limit mismatch, and allowed-directory changes between restarts.
- [ ] Include platform-specific and cross-build coverage where relevant.
- [ ] Include security-negative tests, not only successful paths.
- [ ] For structured execution, verify fixed direct invocation, argument construction, working-directory confinement, environment filtering, timeout, and bounded diagnostics without a shell.

## 3. Devil's advocate review

- [ ] Identify at least two concrete implementation risks.
- [ ] Review allowed-root escape and path-based race windows.
- [ ] Review data loss, non-atomic writes, rollback, cleanup, and recovery artifacts.
- [ ] Review unbounded memory, output, lines, requests, sessions, worker queues, preview caches, manifests, and retained recovery artifacts.
- [ ] Review persistent task state, immutable transition count, task directory scans, head/tail logs, terminal retention, heartbeat staleness, PID reuse, supervisor duplication, and orphan helper behavior.
- [ ] Review nondeterministic ordering and cancellation behavior.
- [ ] Review capability guessing, disclosure, replay, concurrent consumption, expiration, and cache-exhaustion risks.
- [ ] Review multi-file partial commits, misleading atomicity claims, and recovery evidence after failure.
- [ ] Review encoding corruption, malformed Unicode, and binary false positives.
- [ ] Review dependency, platform, API, metadata, and documentation drift.
- [ ] Revise the design before implementation if mitigations are insufficient.

## 4. Repository safety before editing

- [ ] Read the root and nearest scoped `AGENTS.md` files.
- [ ] Read the active roadmap milestone and relevant implementation/tests.
- [ ] Verify branch, `HEAD`, remote tracking, and working-tree status.
- [ ] Preserve unrelated contributor changes.
- [ ] Inspect each target file's surrounding context.
- [ ] Check size, encoding, BOM, and line endings when relevant.
- [ ] Prefer targeted edits and no-replace moves over full rewrites.
- [ ] Do not use destructive Git commands or rewrite history as part of ordinary development.
- [ ] Keep private operator state, credentials, local paths, PIDs, and deployment details out of tracked files.

## 5. TDD and implementation

- [ ] Reproduce the issue or missing behavior with a focused test.
- [ ] Confirm the test fails for the expected reason.
- [ ] Implement the smallest correct production change.
- [ ] Keep public schemas stable unless the active milestone explicitly changes them.
- [ ] Preserve formatting, encoding, BOM state, and line endings.
- [ ] Use explicit error handling, rollback, and cleanup.
- [ ] Add comments only for genuinely non-obvious constraints.
- [ ] Avoid unrelated refactors and dependency changes.
- [ ] Rerun the focused test and confirm it passes.
- [ ] Review the changed code before broader verification.

## 6. Verification ladder

Run checks from focused to broad and record exact outcomes.

### Focused checks

- [ ] affected package tests;
- [ ] affected handler or integration tests;
- [ ] metadata or script tests;
- [ ] platform-specific focused tests.

### Go baseline

- [ ] `gofmt` on changed Go files;
- [ ] `go mod verify`;
- [ ] `go test ./... -count=1`;
- [ ] `go vet ./...`;
- [ ] Staticcheck at the repository-pinned version;
- [ ] govulncheck at the repository-pinned version;
- [ ] race detector where a working CGO compiler is available;
- [ ] coverage review for affected packages when risk justifies it.

### Build and platform checks

- [ ] native build for the development platform when production code changes;
- [ ] Windows amd64/arm64 cross-builds when platform or release behavior changes;
- [ ] Linux amd64/arm64 cross-builds when platform or release behavior changes;
- [ ] macOS amd64/arm64 cross-builds when platform or release behavior changes;
- [ ] runtime execution on available platforms when behavior is platform-specific.
- [ ] exact-binary durable-task smoke with frontend restart, worker kill, supervisor kill/restart/adoption, offline queue recovery, parallel tasks, shared lock keys, logs, and cancellation.

### Repository and documentation checks

- [ ] Node release-script tests;
- [ ] JSON and YAML parsing;
- [ ] PowerShell parsing when PowerShell files change;
- [ ] actionlint and ShellCheck when workflows or shell scripts change;
- [ ] Markdown local-link validation;
- [ ] catalog/runtime/documentation drift tests;
- [ ] private-operator-state regression test;
- [ ] `git diff --check`;
- [ ] complete diff review;
- [ ] final `git status` with no unexpected files.

### Security and release-adjacent checks

- [ ] Gitleaks for tracked content and history when available and relevant;
- [ ] GoReleaser configuration checks when packaging changes;
- [ ] GitHub MCPB/Registry workflow definitions, packaging-script unit tests, templates, and metadata validation when catalog or packaging changes; never create, pack, simulate, dry-run, checksum, or validate real `.mcpb` bundles locally, and never generate the final MCPB-backed Registry manifest locally;
- [ ] no credentials, tokens, private keys, cookies, real tunnel identifiers, or workstation state added.

## 7. Documentation and metadata

- [ ] Update only documents whose behavior, status, or promises changed.
- [ ] Use repository-relative links and portable placeholders.
- [ ] Keep roadmap status, README, publishing notes, and current limitations consistent.
- [ ] Do not imply encoding detection based on filenames or extensions.
- [ ] Do not claim streaming, sandboxing, atomicity, or platform support beyond verified behavior.
- [ ] Update `CHANGELOG.md` for user-visible or architectural changes.
- [ ] Keep `internal/toolcatalog/catalog.json`, runtime registration, README, `TOOLS.md`, and release projection synchronized.
- [ ] Keep generated release output out of version control.

## 8. Commit and review gate

- [ ] Stage only files belonging to the change.
- [ ] Review the staged diff and staged file list.
- [ ] Use a concise English commit message.
- [ ] Verify commit contents and working tree afterward.
- [ ] In the pull request or handoff, state behavior, compatibility, security considerations, tests, omissions, remaining risks, and repository status.

## 9. Internal build gate

Use this section only for an explicitly requested internal build.

- [ ] Build from the exact verified clean commit.
- [ ] Embed an unambiguous commit-derived version.
- [ ] Use a new versioned filename rather than overwriting a known-good artifact.
- [ ] Record size and SHA-256.
- [ ] Verify `--version` and Go VCS/module metadata.
- [ ] Preserve a known-good rollback artifact.
- [ ] Keep local launcher, credentials, process state, and deployment evidence outside the repository.

## 10. Public release and deployment gate

Use [PUBLISHING.md](PUBLISHING.md) for the full release procedure. Release `2.1.1` is the current published, deployed, and rollback-verified Scripthold release; `2.0.0` remains the historical rollback baseline. Future releases should apply the same checks to their active roadmap scope. MCPB artifacts are a GitHub-only release output: local release preparation must never produce or simulate `.mcpb` bundles. If the GitHub MCPB/Registry workflow fails, diagnose its logs and source/configuration, fix the repository, and rerun the workflow on GitHub.

- [x] R7-R13, the 2.0 release scope, and the migration guide are complete;
- [x] the HTTP security design and transport test suite pass;
- [x] the optional plugin retention decision is complete: the fork-owned downloader plugin is removed for 2.0;
- [x] release tag, dated changelog entry, embedded binary version, and Registry version match;
- [x] all supported platform assets and checksums are verified;
- [x] release, Registry publication, and live stdio plus authenticated Streamable HTTP smoke tests succeed;
- [x] the controlled active rollback succeeds and the published runtime is restored before R14 is closed;
- [x] the `2.1.1` GitHub Release, checksum manifests, six MCPB bundles, and `io.github.zoster81/scripthold` Registry publication are independently verified;
- [x] the published `2.1.1` Windows runtime passes tunnel-owned stdio plus authenticated legacy/modern HTTP smoke tests, the active `2.0.0` rollback check succeeds, and `2.1.1` is restored and reverified.
