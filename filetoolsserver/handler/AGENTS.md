# Handler Subsystem Agent Guide

This guide applies to `filetoolsserver/handler/`. Follow the repository root [`AGENTS.md`](../../AGENTS.md) first.

## Responsibilities

Handlers are MCP adapters. Keep encoding, filesystem, security, concurrency, execution, and typed-error logic in the corresponding `internal` package whenever it is reusable or policy-bearing.

Preserve the flow:

1. validate and normalize input;
2. validate the requested path against allowed roots;
3. prepare through shared domain primitives;
4. honor context cancellation before mutation or process start;
5. revalidate path and expected state at the commit boundary;
6. map typed errors and return stable MCP output.

## Public behavior

- Treat input/output structs and JSON field names as public API.
- Preserve existing messages and error codes unless an explicit compatibility milestone changes them.
- Keep streaming text operations on the shared decoded-stream and `internal/textstream` primitives; reserve `textDocument` for explicitly bounded full-document operations such as editing.
- Preserve encoding, BOM, and line-ending semantics promised by each tool.
- Do not add filename- or extension-based encoding behavior.
- `task_run` is the only public execution entry point. Keep `kind=script` and `kind=shell` authorization distinct even though they share durable task infrastructure; both remain disabled by default.
- Bound decoded lines, batches, matches, paging, sorting retention, fuzzy comparison work, patch input/hunks, context, output, worker coordination, and full-document memory through the specific `MCP_MAX_*` limits or explicit fixed safety caps; retain `MCP_MEMORY_THRESHOLD` only as the documented migration fallback.
- Keep fuzzy edits opt-in and ambiguity-safe; keep unified patches single-file, exact-context, ordered, and non-overlapping.
- Batch mutations must preserve input order and expose partial success explicitly; dry runs must not create backups or alter source bytes.

## Tool metadata

`internal/toolcatalog/catalog.json` is authoritative for MCP names, titles, descriptions, and annotations. When adding or changing a tool:

- update catalog metadata;
- update registration and schemas;
- add focused handler tests;
- update README and `TOOLS.md` links/sections;
- run catalog and server drift tests;
- update release projection tests when applicable.

Do not register handler-local descriptions that diverge from the catalog.

## Tests

Prefer table-driven tests and `t.TempDir()`. Cover successful behavior plus invalid input, path denial, missing files, permissions, encoding failures, BOM conflicts, cancellation, concurrent modification, cleanup, and platform-specific behavior.

For mutation handlers, verify both returned metadata and bytes on disk. For read/search handlers, verify deterministic ordering, truncation, limits, and stable partial errors.

Keep fixtures generic and content-based. A test filename must not imply special encoding semantics.

## Verification

```bash
go test ./filetoolsserver/handler -count=1
go test ./filetoolsserver ./internal/toolcatalog -count=1
go run test_server.go
go test ./... -count=1
git diff --check
```

Run affected internal package tests first when a handler delegates to changed domain logic.
