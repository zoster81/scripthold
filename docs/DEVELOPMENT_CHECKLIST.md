# Development Checklist

Use this checklist for non-trivial Scripthold changes. Current milestone state lives in [ROADMAP.md](ROADMAP.md), repository/scoped instructions in [`AGENTS.md`](../AGENTS.md), contributor workflow in [`CONTRIBUTING.md`](../CONTRIBUTING.md), and public release procedure in [PUBLISHING.md](PUBLISHING.md).

Apply only the checks relevant to the change, but report skipped checks explicitly.

## 1. Requirements and edge cases

- [ ] Restate the intended behavior and observable outcome.
- [ ] Identify affected packages, handlers, schemas, metadata, documentation, workflows, and packaging.
- [ ] For MCP tools, verify that static annotations describe the complete public tool capability; do not mix read-only and mutating actions under misleading metadata.
- [ ] Identify compatibility constraints and any intentional breaking change.
- [ ] Define valid, empty, missing, malformed, ambiguous, oversized, and unsupported inputs.
- [ ] Define encoding, BOM, and LF/CRLF behavior where relevant.
- [ ] Define filesystem, permission, read-only, missing-path, and partial-failure behavior.
- [ ] Review path traversal, symlink, junction, reparse-point, hard-link, and allowed-root risks.
- [ ] Review cancellation, timeout, concurrency, race, TOCTOU, and cleanup behavior.
- [ ] For prepared operations, define capability lifetime, entropy, replay, ownership, eviction, restart, and stale-target behavior.
- [ ] For approval-bound mutations, ensure apply accepts only the prepared capability identifier unless an explicitly reviewed design proves additional fields cannot alter the approved operation.
- [ ] For multi-file operations, define preflight, staging, commit order, partial-state reporting, crash behavior, and any explicit non-atomicity.
- [ ] Review Windows, Linux, macOS, long-path, and cross-platform implications.
- [ ] Identify likely regressions and public behavior that must remain unchanged.

## 2. Architecture and test strategy

- [ ] Define the smallest coherent component and file set before editing.
- [ ] Keep transport, MCP adapters, domain logic, and filesystem primitives separated.
- [ ] Reuse shared internal primitives instead of adding local copies.
- [ ] Define data flow, ownership, memory bounds, and cleanup responsibilities.
- [ ] Define deterministic fingerprints, canonical path representation, and metadata exclusions where relevant.
- [ ] Keep persistent backup retention, restore, and garbage-collection policy behind the approved backup contract.
- [ ] Define typed errors and public error mapping.
- [ ] State meaningful time/space complexity where relevant.
- [ ] Select focused failing tests before implementation when practical.
- [ ] Include normal, edge, invalid-input, regression, filesystem-failure, encoding/BOM, line-ending, cancellation, concurrency, and platform cases as applicable.
- [ ] Include security-negative tests, not only successful paths.
- [ ] For durable tasks, cover idempotency conflict, queue/concurrency bounds, logical locks, frontend/worker/supervisor failure, stale recovery, at-most-once behavior, process-tree cancellation, cursor gaps, retention, and allowed-directory changes between restarts.
- [ ] For structured execution, verify fixed invocation, argument construction, working-directory confinement, environment filtering, timeout, cancellation, and bounded diagnostics without a shell.

## 3. Devil's advocate review

- [ ] Identify at least two concrete implementation or operational risks.
- [ ] Review allowed-root escape and path-based race windows.
- [ ] Review data loss, non-atomic writes, rollback, cleanup, and recovery artifacts.
- [ ] Review unbounded memory, output, lines, requests, sessions, queues, caches, manifests, and retained recovery state.
- [ ] Review nondeterministic ordering, cancellation, replay, and concurrent consumption.
- [ ] Review multi-file partial commits and misleading atomicity claims.
- [ ] Review encoding corruption, malformed Unicode, and binary false positives.
- [ ] Review dependency, platform, API, metadata, and documentation drift.
- [ ] Review whether any supposedly read-only MCP tool can reach filesystem, backup-store, task-store, or other persistent mutation paths.
- [ ] Revise the design before implementation if a mitigation is insufficient.

## 4. Repository safety before editing

- [ ] Read the root and nearest scoped `AGENTS.md` files.
- [ ] Read the relevant roadmap/design/source-of-truth documents.
- [ ] Verify branch, `HEAD`, remote tracking, and working-tree status.
- [ ] Preserve unrelated contributor changes.
- [ ] Inspect surrounding implementation and tests before editing.
- [ ] Check target-file encoding, BOM, and line endings when relevant.
- [ ] Prefer targeted or atomic edits and preserve unrelated content.
- [ ] Do not use destructive Git commands or rewrite history during ordinary development.
- [ ] Keep private operator state, credentials, local paths, PIDs, and runtime/deployment state out of tracked files.

## 5. TDD and implementation

- [ ] Reproduce the issue or missing behavior with a focused test when practical.
- [ ] Confirm the test fails for the expected reason.
- [ ] Implement the smallest correct production change.
- [ ] Keep public schemas stable unless an approved milestone explicitly changes them.
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
- [ ] affected handler/integration tests;
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
- [ ] coverage/benchmark review when the risk justifies it.

### Build and platform checks

- [ ] native build when production code changes;
- [ ] Windows amd64/arm64 cross-builds when platform or release behavior changes;
- [ ] Linux amd64/arm64 cross-builds when platform or release behavior changes;
- [ ] macOS amd64/arm64 cross-builds when platform or release behavior changes;
- [ ] runtime execution on available platforms when behavior is platform-specific;
- [ ] exact-binary durable-task smoke when task lifecycle or release behavior changes.

### Repository and documentation checks

- [ ] Node release-script tests where release metadata is relevant;
- [ ] JSON/YAML parsing for changed structured files;
- [ ] PowerShell parsing for changed `.ps1` files;
- [ ] actionlint/ShellCheck for workflow or shell changes;
- [ ] Markdown local-link validation;
- [ ] catalog/runtime/documentation drift tests;
- [ ] private-operator-state regression test;
- [ ] scan modified text for unexpected control characters when generated or scripted edits are involved;
- [ ] `git diff --check`;
- [ ] complete diff review;
- [ ] final `git status` with no unexpected files.

### Security and release-adjacent checks

- [ ] Gitleaks for tracked content/history when relevant;
- [ ] GoReleaser configuration checks when packaging changes;
- [ ] GitHub MCPB/Registry workflow definitions, templates, and non-artifact-producing script tests when packaging/catalog behavior changes;
- [ ] never create, pack, simulate, dry-run, checksum, or validate real `.mcpb` bundles locally, and never generate the final MCPB-backed Registry manifest locally;
- [ ] no credentials, tokens, private keys, cookies, real tunnel identifiers, or workstation state added.

## 7. Documentation and metadata

- [ ] Update only documents whose behavior, status, contract, or navigation changed.
- [ ] Keep each fact in its designated source of truth and link instead of duplicating detailed procedures or history.
- [ ] Use repository-relative links and portable placeholders.
- [ ] Keep README, roadmap, project direction, publishing notes, tool reference, and subsystem contracts consistent.
- [ ] Do not imply filename- or extension-based encoding detection.
- [ ] Do not claim streaming, sandboxing, atomicity, deployment, or platform verification beyond evidence.
- [ ] Update `CHANGELOG.md` for user-visible behavior, compatibility, security, packaging, or architecture changes.
- [ ] Keep `internal/toolcatalog/catalog.json`, runtime registration, README tool links, `TOOLS.md`, and release projection synchronized.
- [ ] Keep generated release output out of version control.

## 8. Commit and review gate

- [ ] Stage only files belonging to the change.
- [ ] Review the staged diff and staged file list.
- [ ] Use a concise English commit message.
- [ ] Verify commit contents and working tree afterward.
- [ ] In the review/handoff, distinguish code reviewed, code modified, tests executed, builds performed, publication, deployment, and checks not performed.

## 9. Internal build gate

Use this section only for an explicitly requested internal build.

- [ ] Build from the exact verified clean commit.
- [ ] Embed an unambiguous commit-derived version.
- [ ] Use a new versioned filename rather than overwriting a known-good artifact.
- [ ] Record size and SHA-256 privately where operationally required.
- [ ] Verify `--version` and Go VCS/module metadata.
- [ ] Preserve a known-good rollback artifact.
- [ ] Keep launcher, credentials, process state, and deployment evidence outside the repository.

## 10. Public release gate

Use [PUBLISHING.md](PUBLISHING.md) as the authoritative release procedure. This checklist intentionally does not duplicate historical release evidence or GitHub-only MCPB packaging steps.

- [ ] release-scoped milestone complete;
- [ ] exact clean commit and dated changelog entry verified;
- [ ] complete push-event `CI` `Release candidate` gate passes on the exact pushed SHA;
- [ ] annotated tag resolves to that same commit;
- [ ] normal GoReleaser publication succeeds;
- [ ] GitHub-only MCPB and Registry workflows succeed;
- [ ] normal published assets independently match `checksums.txt`;
- [ ] tag, changelog, embedded version, and Registry version agree.

Deployment, active rollback, restoration, private-launcher changes, and runtime restarts are separate operator actions and require their own authorization and verification.
