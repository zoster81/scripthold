# Documentation Agent Guide

This guide applies to files under `docs/`. Follow the root [`AGENTS.md`](../AGENTS.md) first.

## Document responsibilities

- [`ARCHITECTURE.md`](ARCHITECTURE.md): current product architecture and security/durability boundaries. Keep it current; do not turn it into a milestone history.
- [`PROJECT_DIRECTION.md`](PROJECT_DIRECTION.md): stable product identity, scope, transport model, and upstream relationship.
- [`ROADMAP.md`](ROADMAP.md): current and future milestone state only.
- [`DEVELOPMENT_CHECKLIST.md`](DEVELOPMENT_CHECKLIST.md): reusable engineering and verification checks.
- [`PUBLISHING.md`](PUBLISHING.md): maintainer release and distribution procedure.
- [`HTTP_SECURITY.md`](HTTP_SECURITY.md): current Streamable HTTP threat model and deployment contract.
- [`DURABLE_TASKS.md`](DURABLE_TASKS.md): current durable task process, recovery, log, retention, and configuration contract.
- [`LANGUAGE_CAPABILITIES.md`](LANGUAGE_CAPABILITIES.md): generated source-intelligence capability projection; update its generator/registry rather than editing claims independently.
- [`MIGRATION_2.0.md`](MIGRATION_2.0.md) and [`MIGRATION_3.0.md`](MIGRATION_3.0.md): user migration guides for intentional compatibility breaks.

Detailed tool schemas, examples, limits, and current public behavior belong in [`TOOLS.md`](../TOOLS.md). Release-by-release user-visible changes belong in [`CHANGELOG.md`](../CHANGELOG.md). Implementation chronology belongs in Git, not in new phase diaries or completed-design archives.

## Documentation policy

Documentation should answer one of these questions:

1. How do I install, configure, use, secure, migrate, or contribute to Scripthold?
2. What is the current product/architecture contract?
3. What work is currently planned?
4. What changed in a public or unreleased version?

If a document mainly records how a completed phase was executed, task IDs, checkpoint evidence, historical design alternatives, local deployment state, or chat continuation instructions, it should not remain part of the maintained documentation set.

Prefer updating an existing current document over creating a new milestone-specific file. Link to one source of truth instead of copying the same explanation into multiple files.

## Portability

Public documentation must work from a normal clone. Do not include private workspace paths, connector instance names, local PIDs, active binary filenames, workstation hashes, launcher state, credentials, tunnel identifiers, or operator handoff state.

Use repository-relative links and portable placeholders such as `/path/to/project` or `C:\Path\To\AllowedProject`.

## Consistency

- Keep README, project direction, roadmap, architecture, tool reference, publishing notes, and migrations consistent.
- Keep current limitations explicit and separate from planned work.
- Do not imply filename-based encoding detection.
- Do not claim streaming, atomicity, sandboxing, platform support, or security properties beyond implementation and tests.
- When tool behavior changes, verify descriptions against `internal/toolcatalog/catalog.json` and `TOOLS.md`.
- Use English technical prose and stable headings suitable for direct links.

## Verification

For documentation-only changes, run at least:

```bash
go test ./internal/projectidentity ./internal/toolcatalog -count=1
node scripts/check-markdown-links.js
git diff --check
```

Run broader checks when documentation changes accompany code, metadata, workflow, packaging, or release behavior.
