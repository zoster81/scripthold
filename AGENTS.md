# Scripthold Repository Agent Guide

## Scope and precedence

This file applies to the entire repository. A nested `AGENTS.md` adds or overrides instructions only for files below its directory. Read this file, then the nearest scoped guide before editing.

Do not copy private workstation state, local process details, credentials, or operator-specific paths into tracked files. Public content must be reproducible by an external contributor from a normal clone.

## Sources of truth

- Product identity, fork scope, transports, and upstream relationship: [`docs/PROJECT_DIRECTION.md`](docs/PROJECT_DIRECTION.md)
- Current/future milestone state and gates: [`docs/ROADMAP.md`](docs/ROADMAP.md)
- Completed milestone history: [`docs/ROADMAP_HISTORY.md`](docs/ROADMAP_HISTORY.md)
- Reusable engineering checks: [`docs/DEVELOPMENT_CHECKLIST.md`](docs/DEVELOPMENT_CHECKLIST.md)
- Contributor workflow: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Tool behavior and examples: [`TOOLS.md`](TOOLS.md)
- Release procedure: [`docs/PUBLISHING.md`](docs/PUBLISHING.md)
- Streamable HTTP security design: [`docs/HTTP_SECURITY.md`](docs/HTTP_SECURITY.md)
- R16 verified-change design: [`docs/VERIFIED_CHANGE_WORKFLOWS.md`](docs/VERIFIED_CHANGE_WORKFLOWS.md)
- Approved R17 persistent-backup lifecycle design and R18 implementation contract: [`docs/PERSISTENT_BACKUP_LIFECYCLE.md`](docs/PERSISTENT_BACKUP_LIFECYCLE.md)
- R19 offline backup-store diagnostics design: [`docs/OFFLINE_BACKUP_DIAGNOSTICS.md`](docs/OFFLINE_BACKUP_DIAGNOSTICS.md)
- R20 MCP `2026-07-28` adoption design: [`docs/MCP_2026_07_28_ADOPTION.md`](docs/MCP_2026_07_28_ADOPTION.md)
- Completed R22 global encoding coverage and Scripthold 2.2.0 contract: [`docs/GLOBAL_ENCODING_COVERAGE.md`](docs/GLOBAL_ENCODING_COVERAGE.md)
- Completed R23 MCP mutation-surface and backup-UX contract: [`docs/MCP_MUTATION_SURFACE.md`](docs/MCP_MUTATION_SURFACE.md)
- Completed R24 safe-filesystem operations contract and verification record: [`docs/SAFE_FILESYSTEM_OPERATIONS.md`](docs/SAFE_FILESYSTEM_OPERATIONS.md)
- Completed R25 source-intelligence contract and verification record: [`docs/SOURCE_INTELLIGENCE.md`](docs/SOURCE_INTELLIGENCE.md)
- Completed R26 backup-recovery contract and verification record: [`docs/BACKUP_RECOVERY.md`](docs/BACKUP_RECOVERY.md)
- Completed R27 broad multi-language code-intelligence contract and verification record: [`docs/MULTILANGUAGE_CODE_INTELLIGENCE.md`](docs/MULTILANGUAGE_CODE_INTELLIGENCE.md)
- Completed R28 engine-hygiene contract and verification record: [`docs/ENGINE_HYGIENE.md`](docs/ENGINE_HYGIENE.md)
- Mechanically verified R27 language capability projection: [`docs/LANGUAGE_CAPABILITIES.md`](docs/LANGUAGE_CAPABILITIES.md), rendered from the native registry
- Authoritative MCP tool metadata: [`internal/toolcatalog/catalog.json`](internal/toolcatalog/catalog.json)

Link to these documents instead of duplicating their detailed content. R28 is complete and no later release-scoped milestone is active by default. Before later milestone work, read the completed R25-R28 contracts together with the roadmap and explicitly activate the intended milestone; completed requirements remain authoritative history unless maintainers deliberately revise them.

## Repository map

- `cmd/scripthold`: CLI entry point and transport bootstrap.
- `filetoolsserver`: MCP server construction, roots, and tool registration.
- `filetoolsserver/handler`: MCP adapters and shared text-document behavior.
- `internal/encoding`: encoding registry and content-based detection.
- `internal/security`: path normalization, resolution, and allowed-root enforcement.
- `internal/filesystem`: secure traversal and durable mutation primitives.
- `internal/filesystempackage`: transport-independent R24 filesystem-package manifest, planner, one-shot capability, revalidation, apply orchestration, and partial-state classification.
- `internal/backupstore`: dedicated internal backup-store authority, format, locking, integrity, recovery, restore, and garbage-collection primitives.
- `internal/httptransport`: secured native Streamable HTTP listener, admission, sessions, and lifecycle.
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
- A configured backup store is a separate process-wide internal authority: it must not overlap public roots, must remain inaccessible to ordinary tools, and must preserve the owner-only, one-writer, immutable-format, no-background-GC, and no-automatic-rollback decisions in `docs/PERSISTENT_BACKUP_LIFECYCLE.md`.
- Preserve stdio behavior while transport work is in progress.

## Verification commands

Start with the narrowest applicable tests, then expand as needed:

```bash
go test ./path/to/affected/package -count=1
go mod verify
go test ./... -count=1
go vet ./...
golangci-lint run ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
node --test scripts/generate-server-json.test.js scripts/prepare-mcpb-assets.test.js scripts/verify-release-version.test.js
go run test_server.go
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
