# Scripthold Repository Agent Guide

## Scope and precedence

This file applies to the entire repository. A nested `AGENTS.md` adds or overrides instructions only for files below its directory. Read this file, then the nearest scoped guide before editing.

Do not copy private workstation state, local process details, credentials, or operator-specific paths into tracked files. Public content must be reproducible by an external contributor from a normal clone.

## Operator communication

- Communicate with the operator in Italian unless explicitly requested otherwise.
- Use simple, direct, understandable language for progress updates and completion reports. Start with what was done, what changed, what remains, and whether the requested work is actually complete.
- Explain unavoidable technical terms in plain language when they first matter. Do not make hashes, task IDs, counters, or implementation details the main explanation; present them only as supporting evidence after the practical conclusion.
- When something fails or blocks progress, state the practical consequence first, then the technical cause and verification evidence.

## Sources of truth

- Product identity, scope, transports, and upstream relationship: [`docs/PROJECT_DIRECTION.md`](docs/PROJECT_DIRECTION.md)
- Current architecture and security/durability boundaries: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)
- Current/future milestone state: [`docs/ROADMAP.md`](docs/ROADMAP.md)
- Reusable engineering checks: [`docs/DEVELOPMENT_CHECKLIST.md`](docs/DEVELOPMENT_CHECKLIST.md)
- Contributor workflow: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Tool behavior, schemas, limits, and examples: [`TOOLS.md`](TOOLS.md)
- Release procedure: [`docs/PUBLISHING.md`](docs/PUBLISHING.md)
- Streamable HTTP security contract: [`docs/HTTP_SECURITY.md`](docs/HTTP_SECURITY.md)
- Durable task execution contract: [`docs/DURABLE_TASKS.md`](docs/DURABLE_TASKS.md)
- Source Intelligence capability projection: [`docs/LANGUAGE_CAPABILITIES.md`](docs/LANGUAGE_CAPABILITIES.md), rendered from the native registry
- Intentional compatibility changes: [`docs/MIGRATION_2.0.md`](docs/MIGRATION_2.0.md) and [`docs/MIGRATION_3.0.md`](docs/MIGRATION_3.0.md)
- Authoritative MCP tool metadata: [`internal/toolcatalog/catalog.json`](internal/toolcatalog/catalog.json)

Link to these sources instead of duplicating their content. Release history belongs in `CHANGELOG.md`, implementation history in Git, and current milestone state only in `docs/ROADMAP.md`. Do not create phase diaries, checkpoint documents, or completed-design archives when current behavior can be documented in an existing source of truth.

## Repository map

- `cmd/scripthold`: CLI entry point and transport bootstrap.
- `filetoolsserver`: MCP server construction, roots, and tool registration.
- `filetoolsserver/handler`: MCP adapters and shared text-document behavior.
- `internal/encoding`: encoding registry and content-based detection.
- `internal/security`: path normalization, resolution, and allowed-root enforcement.
- `internal/filesystem`: secure traversal and durable mutation primitives.
- `internal/filesystempackage`: transport-independent filesystem-package manifest, planner, one-shot capability, revalidation, apply orchestration, and partial-state classification.
- `internal/backupstore`: dedicated internal backup-store authority, format, locking, integrity, recovery, restore, and garbage-collection primitives.
- `internal/httptransport`: secured native Streamable HTTP listener, admission, sessions, and lifecycle.
- `internal/diagnostics`: process-wide redacted server/access diagnostics plus bounded optional file retention and multi-process ownership.
- `internal/operation`: transport-independent error categories.
- `internal/concurrency`: bounded deterministic worker coordination.
- `internal/textstream`: incremental decoding consumers, bounded line framing, and streaming line-ending transforms.
- `internal/toolcatalog`: embedded tool metadata and drift checks.
- `scripts`: release metadata and workflow validation utilities.

## Working method

For non-trivial changes:

1. Restate the intended behavior and identify compatibility, security, encoding, filesystem, concurrency, and platform edge cases.
2. Define the smallest coherent design and focused tests before implementation.
3. Review at least two concrete failure or regression risks and their mitigations.
4. Implement the smallest correct change, review the complete diff, and report exactly what was verified.

Use focused TDD when practical: reproduce, confirm the expected failure, implement, rerun focused tests, then run the relevant regression suite.

## Project invariants

- Encoding detection is derived from bytes and decoded-content evidence, never filenames or extensions.
- Unicode BOM evidence is authoritative. Ambiguous data must not be classified with unjustified confidence.
- New files default to UTF-8; existing files preserve a confidently detected encoding unless explicitly overridden.
- Streaming text operations must bound line, result, context, and aggregate output memory independently of complete source size.
- Full-document operations such as editing must reject inputs above their configured hard limit before reading or diffing them.
- Preserve encoding, BOM policy, and line endings exactly where a tool promises preservation.
- All filesystem access must remain inside validated allowed roots after symlink, junction, and reparse-point resolution.
- Missing paths must be validated through their nearest existing ancestor.
- Mutations must preserve the durable staging, snapshot, no-replace, rollback, cleanup, and platform-sync guarantees in `internal/filesystem`.
- Public error schemas and tool metadata remain stable unless a roadmap milestone explicitly changes them.
- `task_run` is the only public execution tool; the synchronous `run_script` and `shell` tools are removed. Its `kind=script` and `kind=shell` requests remain disabled by default and retain distinct authorization gates.
- Durable execution state belongs to the private task store and independent supervisor/worker/executor topology. Do not make task lifetime depend on an MCP frontend, transport connection, or request context.
- Allowed directories are process-wide policy shared by every connection; do not introduce per-session filesystem ACLs or let future HTTP sessions mutate startup roots without an explicit roadmap decision.
- Dynamic MCP client roots are a stdio-only compatibility path when no startup directories are configured.
- Native HTTP must follow `docs/HTTP_SECURITY.md`; do not weaken authentication, Host/Origin checks, session limits, logging redaction, or the dual execution opt-in.
- A configured backup store is a separate process-wide internal authority: it must not overlap public roots, must remain inaccessible to ordinary tools, and must preserve the owner-only, one-writer, immutable-record, no-background-GC, pin protection, manifest-before-object deletion, and no-automatic-rollback boundaries summarized in `docs/ARCHITECTURE.md`. Synchronous per-target FIFO retention after a newer manifest is durable is the only approved automatic deletion path.
- Preserve stdio behavior while transport work is in progress.

## Verification commands

Start with the narrowest applicable tests, then expand as needed:

```bash
go test ./path/to/affected/package -count=1
go mod verify
go test ./... -count=1
go vet ./...
golangci-lint run ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
node --test scripts/check-markdown-links.test.js scripts/test-ci-policy.test.js
node --test scripts/generate-server-json.test.js scripts/prepare-mcpb-assets.test.js scripts/release-candidate-provenance.test.js scripts/run-fuzz.test.js scripts/verify-release-version.test.js
node scripts/check-markdown-links.js
```

Run `go test -race ./...` only where a working CGO compiler is available. Run `bash scripts/validate-workflows.sh` when workflows or shell scripts change. Use the full release checks from `docs/PUBLISHING.md` only for release-related work.

Always run `gofmt` on changed Go files and `git diff --check`. Review final `git status` for unexpected files.

## Change rules

- Inspect surrounding code and tests before editing.
- Preserve existing formatting, encoding, BOM state, and line endings.
- Prefer shared internal primitives over handler-local copies.
- Keep changes atomic and avoid unrelated refactors.
- Treat paths, file contents, environment variables, process output, and network data as untrusted.
- Never add credentials, real tunnel identifiers, workstation paths, PIDs, private binary hashes, or operator handoff state to tracked files.
- Do not manually edit generated release output such as `server.json`; update its source template/catalog or generator instead.
- MCPB release artifacts are GitHub-only outputs. Never create, pack, repack, simulate, dry-run, checksum, or validate real `.mcpb` bundles locally, and never generate the final MCPB-backed Registry manifest locally. If the GitHub MCPB/Registry workflow fails, diagnose logs and source/configuration, fix the repository, and rerun the workflow on GitHub.
- When tool metadata changes, update the catalog, runtime behavior, README links, TOOLS reference, tests, and release projection together.
- Do not change dependencies, public schemas, release versions, workflows, or packaging incidentally.

## Scoped guides

Additional instructions exist in:

- [`docs/AGENTS.md`](docs/AGENTS.md)
- [`filetoolsserver/handler/AGENTS.md`](filetoolsserver/handler/AGENTS.md)
- [`internal/encoding/AGENTS.md`](internal/encoding/AGENTS.md)
- [`internal/filesystem/AGENTS.md`](internal/filesystem/AGENTS.md)
- [`internal/backupstore/AGENTS.md`](internal/backupstore/AGENTS.md)
- [`internal/httptransport/AGENTS.md`](internal/httptransport/AGENTS.md)
- [`internal/security/AGENTS.md`](internal/security/AGENTS.md)
- [`scripts/AGENTS.md`](scripts/AGENTS.md)

Do not add another scoped guide unless that subtree has distinct commands, invariants, generated artifacts, or security constraints that cannot be stated clearly here.

## Completion report

State files changed, behavior affected, tests executed and their results, checks not performed, remaining risks, and repository status. Distinguish review, modification, compilation, testing, build, publication, and deployment; do not imply a step occurred when it did not.
