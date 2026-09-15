# HTTP Transport Agent Guide

This guide applies to `internal/httptransport/`. Follow the repository root [`AGENTS.md`](../../AGENTS.md) first.

## Source of truth

[`docs/HTTP_SECURITY.md`](../../docs/HTTP_SECURITY.md) defines the approved threat model, configuration contract, middleware order, accepted risks, required negative tests, and release blockers. Do not broaden that model or add compatibility bypasses without explicit review and synchronized documentation.

## Invariants

- Bind to loopback by default and fail closed on insecure non-loopback combinations.
- Authenticate every MCP `GET`, `POST`, and `DELETE` request. A session identifier never replaces authentication.
- Validate exact Host and all-method Origin before authentication; emit no CORS allow headers.
- Keep startup roots process-wide and immutable. HTTP clients cannot supply or mutate roots.
- Keep the SDK event store disabled until a separately reviewed durable-replay design exists.
- Bound headers, per-request bodies, aggregate in-flight body bytes, concurrent non-SSE requests, sessions, rate-limiter state, and timeouts.
- Match the pinned SDK timeout semantics: pause idle expiry for active POST requests, but allow an SSE-only session to expire unless the client sends keepalive traffic.
- Keep access logs category-only and path-redacted. Never log tokens, query strings, bodies, tool arguments, complete session IDs, commands, or file contents.
- Clear HTTP credential variables after configuration is snapshotted so child processes cannot inherit them.
- HTTP execution requires both the legacy tool authorization and `MCP_HTTP_ENABLE_EXECUTION=1`.
- Use the shared `BuildServer`; do not duplicate tool registration or handler policy.

## Tests

Start with:

```bash
go test ./internal/httptransport -count=1
go test ./cmd/scripthold ./filetoolsserver ./filetoolsserver/handler -count=1
```

Security-sensitive changes require negative tests for authentication, Host/Origin, proxy trust, body and aggregate limits, concurrency, session admission/cleanup, cancellation, logging redaction, and shutdown. Run the complete race suite before completion.