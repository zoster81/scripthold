# Documentation Agent Guide

This guide applies to files under `docs/`. Follow the root [`AGENTS.md`](../AGENTS.md) first.

## Document responsibilities

- `PROJECT_DIRECTION.md`: stable product identity, transport scope, independent-fork boundary, and upstream relationship.
- `ROADMAP.md`: authoritative current/future milestone state, operating rules, and completion gates.
- `DEVELOPMENT_CHECKLIST.md`: reusable, portable engineering and verification checks.
- `ROADMAP_HISTORY.md`: concise completed milestone history for R1 onward, not an operator session log.
- `PUBLISHING.md`: maintainer release and distribution procedure.
- `MIGRATION_2.0.md`: authoritative intentional breaking changes and migration actions for 1.8 to 2.0.
- `MIGRATION_3.0.md`: Unreleased R23 next-major MCP-surface migration from mixed mutation actions to read-only preparation plus `previewId`-only apply tools.
- `HTTP_SECURITY.md`: approved Streamable HTTP threat model, secure defaults, implementation constraints, test matrix, and release blockers.
- `VERIFIED_CHANGE_WORKFLOWS.md`: approved R16 design baseline for fingerprints, preview/apply, patch packages, structured verification, and its relationship to the later backup subsystem.
- `PERSISTENT_BACKUP_LIFECYCLE.md`: approved R17 design and R18 implementation contract for the internal store boundary, content-addressed objects, manifests, quotas, restore, garbage collection, and crash recovery.
- `OFFLINE_BACKUP_DIAGNOSTICS.md`: R19 diagnostic-only design for inspecting an existing store without creation, repair, cleanup, or other filesystem mutation.
- `MCP_2026_07_28_ADOPTION.md`: R20 compatibility and security design for adopting MCP `2026-07-28` without losing legacy stdio or stateful HTTP behavior.
- `GLOBAL_ENCODING_COVERAGE.md`: completed R22 / 2.2.0 implementation and verification contract for global portable encoding coverage, full UTF-32 text support, detector hardening, corpus provenance, and release gates.
- `MCP_MUTATION_SURFACE.md`: completed R23 contract for truthful read-only/mutation tool boundaries, capability-bound preview/apply, connector-blocking reduction, and backup-history/default-policy UX.
- `SAFE_FILESYSTEM_OPERATIONS.md`: approved planned R24 baseline for typed preview/apply filesystem packages, recursive directory operations, backup-before-loss, and partial-state evidence.
- `SOURCE_INTELLIGENCE.md`: approved planned R25 baseline for language-neutral structured symbol extraction/indexing and the initial Go AST reference provider.
- `BACKUP_RECOVERY.md`: approved planned R26 baseline for offline evidence-preserving backup-store salvage/reconstruction without mutating the source store.
- `MULTILANGUAGE_CODE_INTELLIGENCE.md`: approved planned R27 baseline for broad mandatory multi-language source intelligence, semantic capability reporting, relationships, and incremental indexing.

Keep operational details in their proper source instead of duplicating them across documents. Planned baseline documents are binding requirements for their milestone unless maintainers explicitly revise them; do not replace them with a shorter roadmap summary.

## Portability rules

Documentation must be usable by an external contributor from a normal clone.

Do not include:

- private workspace or home-directory paths;
- connector instance names, local PIDs, active binary filenames, or workstation hashes;
- private handoff files or launcher state;
- credentials, real tunnel identifiers, or unsanitized configuration;
- instructions that depend on a specific contributor asking an agent to commit, push, or restart a service.

Use repository-relative links, environment variables, and obvious placeholders such as `/path/to/project` or `C:\Path\To\AllowedProject`.

Historical documents should record architectural outcomes, compatibility decisions, public releases, and reproducible validation—not ephemeral branch tracking, local deployment, or process state.

## Consistency

- Keep project direction, roadmap status, README, tool reference, and publishing notes consistent.
- Keep current limitations explicit and distinguish current behavior, completed history, and planned work.
- Keep Streamable HTTP implementation aligned with `HTTP_SECURITY.md`; changes to its trust model or accepted risks require explicit review.
- Do not imply that filename extensions influence encoding detection.
- Do not claim streaming, atomicity, sandboxing, or platform support beyond what tests and implementation establish.
- When tool behavior changes, verify links and descriptions against `internal/toolcatalog/catalog.json` and `TOOLS.md`.
- Use English technical prose and stable headings suitable for direct links.

## Verification

For documentation-only changes, run at least:

```bash
go test ./internal/projectidentity ./internal/toolcatalog -count=1
git diff --check
```

Also validate all modified Markdown links. Run broader tests when documentation changes accompany code, metadata, workflow, packaging, or release behavior.
