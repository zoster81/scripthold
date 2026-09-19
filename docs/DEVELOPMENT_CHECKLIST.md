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

- [ ] Generate multiple credible candidate designs before editing, including the smallest obvious solution and materially different alternatives.
- [ ] Keep transport, MCP adapters, domain logic, and filesystem primitives separated.
- [ ] Reuse shared internal primitives instead of adding local copies.
- [ ] Define data flow, ownership, memory bounds, and cleanup responsibilities.
- [ ] Define deterministic fingerprints, canonical path representation, and metadata exclusions where relevant.
- [ ] Keep persistent backup retention, restore, and garbage-collection policy behind the approved backup contract.
- [ ] Define typed errors and public error mapping.
- [ ] State meaningful time/space complexity where relevant.
- [ ] For each candidate, record affected components, data flow, compatibility impact, meaningful complexity/trade-offs, and the focused tests that would prove or disprove it.
- [ ] Include normal, edge, invalid-input, regression, filesystem-failure, encoding/BOM, line-ending, cancellation, concurrency, and platform cases as applicable.
- [ ] Include security-negative tests, not only successful paths.
- [ ] For durable tasks, cover idempotency conflict, queue/concurrency bounds, logical locks, frontend/worker/supervisor failure, stale recovery, at-most-once behavior, process-tree cancellation, cursor gaps, retention, and allowed-directory changes between restarts.
- [ ] For structured execution, verify fixed invocation, argument construction, working-directory confinement, environment filtering, timeout, cancellation, and bounded diagnostics without a shell.

## 3. Devil's advocate review

- [ ] Challenge every phase-2 candidate with concrete implementation or operational risks; compare all candidates against the same repository evidence and constraints.
- [ ] Review allowed-root escape and path-based race windows.
- [ ] Review data loss, non-atomic writes, rollback, cleanup, and recovery artifacts.
- [ ] Review unbounded memory, output, lines, requests, sessions, queues, caches, manifests, and retained recovery state.
- [ ] Review nondeterministic ordering, cancellation, replay, and concurrent consumption.
- [ ] Review multi-file partial commits and misleading atomicity claims.
- [ ] Review encoding corruption, malformed Unicode, and binary false positives.
- [ ] Review dependency, platform, API, metadata, and documentation drift.
- [ ] Review whether any supposedly read-only MCP tool can reach filesystem, backup-store, task-store, or other persistent mutation paths.
- [ ] Eliminate dominated candidates explicitly and identify whether criticism requires a new or revised candidate.
- [ ] If the design space changes, return to phase 2, add or revise candidates, then run phase 3 against the complete candidate set again.
- [ ] Repeat the `phase 2 -> phase 3 -> phase 2` loop until no materially better candidate remains and the preferred design is justified by repository evidence.
- [ ] Reopen phase 1 only if the review reveals a new or changed requirement/edge case; do not substitute random trial-and-error for design convergence.

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

Run checks from focused to broad and record exact outcomes. During implementation, stop at the narrowest level that covers the changed component and its integration boundaries unless evidence or change scope justifies expansion. Do not treat the full repository, full race, fuzz, or release-adjacent gates as mandatory after every isolated edit.

Use three practical local levels:

1. **Focused iteration** — changed package(s), directly affected handler/integration/schema/catalog tests, relevant documentation/policy checks, formatting, diff review, and targeted race/fuzz only when the touched behavior warrants them.
2. **Pre-push/promotion** — full normal regression plus repository-wide static/vulnerability checks and the canonical bounded fuzz smoke. Use this before pushing a meaningful batch, after cross-cutting changes, or when focused evidence leaves uncertainty.
3. **Release/full qualification** — exact-commit cross-platform race/build/smoke/security/release evidence required by CI and publication policy. Do not reproduce this entire tier locally after every incremental commit.

Independent focused checks may run in parallel when they do not mutate shared inputs or compete for the same scarce resource. Prefer one Go invocation over several competing full-package invocations because the Go tool already parallelizes package work; avoid concurrent CPU-heavy full test/race/fuzz jobs that oversubscribe the machine or make timing-sensitive tests flaky.

GitHub CI applies fail-closed evidence tiers to pull requests. Documentation-only changes run repository policy, generated capability/documentation drift, local Markdown-link validation, release/script policy tests, and secret scanning. Go-only changes add the normal Linux regression suite, evidence-map-derived Windows/macOS platform tests and race-sensitive package tests, module verification, static/vulnerability analysis, and the canonical risk-based fuzz smoke. Changes to workflows, scripts, test architecture, build/release/configuration metadata, fixtures, or any unclassified path require full qualification. Pushes to `main`/`master`, manual runs, classifier bootstrap, and classification failures also require full qualification.

The complete exact-commit gate retains broad normal regression across Linux/Windows/macOS, broad race coverage on all three native runners, native binary smoke, six-target cross-builds, hardened container smoke, workflow validation, Gitleaks, GoReleaser configuration validation, static/vulnerability analysis, and canonical fuzz evidence. The `Release candidate` aggregator must fail when any evidence required by the selected tier is missing or unsuccessful.

### Focused checks

- [ ] affected package tests;
- [ ] affected handler/integration tests;
- [ ] metadata or script tests;
- [ ] platform-specific focused tests.

### Go verification

For focused iteration:

- [ ] `gofmt` on changed Go files;
- [ ] affected package and integration tests with `-count=1`;
- [ ] `go mod verify` when Go module/dependency integrity is relevant;
- [ ] targeted vet/static/race/fuzz checks when the touched code or failure class warrants them.

For pre-push/promotion or cross-cutting changes:

- [ ] `go test ./... -count=1`;
- [ ] `go vet ./...`;
- [ ] `golangci-lint run ./...` with the repository-pinned policy;
- [ ] Staticcheck at the repository-pinned version;
- [ ] govulncheck at the repository-pinned version;
- [ ] canonical bounded fuzz smoke when applicable;
- [ ] race coverage at the scope justified by the change; full repository race belongs to full/promotion evidence rather than routine isolated iteration;
- [ ] coverage/benchmark review when the risk justifies it.

### Build and platform checks

- [ ] native build when production code changes;
- [ ] Windows amd64/arm64 cross-builds when platform or release behavior changes;
- [ ] Linux amd64/arm64 cross-builds when platform or release behavior changes;
- [ ] macOS amd64/arm64 cross-builds when platform or release behavior changes;
- [ ] runtime execution on available platforms when behavior is platform-specific;
- [ ] exact-binary durable-task smoke when task lifecycle or release behavior changes.

### Repository and documentation checks

- [ ] Node release-script and CI-policy tests where release or workflow metadata is relevant;
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
- [ ] For every affected public document or tool section, explain first what it does, when or why to use it, and representative applications before low-level mechanics.
- [ ] Prefer plain English, short direct sentences, and concrete examples; define unavoidable technical terms when they first matter.
- [ ] Keep schemas, limits, security constraints, compatibility rules, and failure behavior exact, but place them after the reader understands the normal workflow where practical.
- [ ] Review the whole affected public section for jargon, implementation-first wording, duplicated details, and unclear purpose rather than patching only one sentence.
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
- [ ] complete **full-tier** push-event `Test Suite` (`.github/workflows/test.yml`) `Release candidate` gate passes on the exact pushed SHA; pull-request tier evidence is never publication authority;
- [ ] OpenSSF Scorecard has a successful supported `push` run on the default `main` branch for that same exact SHA;
- [ ] annotated tag resolves to that same commit;
- [ ] normal GoReleaser publication succeeds;
- [ ] signed Sigstore SLSA build provenance is generated and attached for the normal release assets;
- [ ] GitHub-only MCPB and Registry workflows succeed;
- [ ] normal published assets independently match `checksums.txt`;
- [ ] tag, changelog, embedded version, and Registry version agree.

Deployment, active rollback, restoration, private-launcher changes, and runtime restarts are separate operator actions and require their own authorization and verification.
