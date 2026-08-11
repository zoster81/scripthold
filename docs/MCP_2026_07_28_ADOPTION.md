# MCP 2026-07-28 Adoption Contract

## Status

**COMPLETE.** R20 adopted final MCP protocol version `2026-07-28` through official stable `github.com/modelcontextprotocol/go-sdk v1.7.0` while retaining supported legacy behavior. The implementation is part of the current Scripthold release line.

This document is the stable protocol-compatibility, transport-routing, and security contract. Historical implementation chronology belongs in [ROADMAP_HISTORY.md](ROADMAP_HISTORY.md); the outer HTTP threat model remains authoritative in [HTTP_SECURITY.md](HTTP_SECURITY.md).

## External references

- [MCP 2026-07-28 specification announcement](https://blog.modelcontextprotocol.io/posts/2026-07-28/)
- [MCP Go SDK releases](https://github.com/modelcontextprotocol/go-sdk/releases)
- [MCP Go SDK protocol guide](https://github.com/modelcontextprotocol/go-sdk/blob/main/docs/protocol.md)
- [SEP-2577 roots, sampling, and logging deprecation](https://modelcontextprotocol.io/seps/2577-deprecate-roots-sampling-and-logging)

## Compatibility goals

Scripthold must:

- support exact protocol version `2026-07-28` through an official stable Go SDK;
- preserve supported legacy clients and stateful legacy HTTP behavior;
- expose one shared tool/prompt catalog, schemas, limits, allowed-root policy, typed errors, and execution gates regardless of protocol generation;
- keep stdio and Streamable HTTP equivalent at the tool layer;
- add stateless HTTP without weakening authentication, Host, Origin, proxy, rate, concurrency, body, timeout, logging, readiness, execution, or shutdown controls;
- avoid relying on deprecated client roots for `2026-07-28`;
- reject malformed or unsupported version inputs deterministically without silently downgrading individual requests.

## Non-goals

The R20 boundary does not add:

- MCP Apps or protocol Tasks;
- application-managed OAuth/authorization-server behavior;
- Enterprise Managed Authorization;
- Multi Round-Trip Requests for tool confirmation or missing input;
- durable subscriptions or event replay;
- server-side protocol session state for `2026-07-28`;
- per-client filesystem roots/ACLs;
- new file-operation schemas solely because the protocol version changed.

## Stable SDK boundary

Production uses official stable Go SDK `v1.7.0`. The adopted public APIs provide:

- `StreamableHTTPOptions.Stateless` for modern stateless HTTP;
- bounded SDK request-body handling;
- request-cancellation propagation for stateless requests;
- `server/discover` and protocol-version support;
- standardized HTTP header validation;
- supported legacy negotiation without vendoring or forking the SDK.

Scripthold explicitly projects only capabilities it implements. It does not select a positive tool-list cache lifetime; SDK list/discovery output uses `ttlMs: 0`, so results are immediately stale.

Any future SDK upgrade is a dependency/security change and must requalify the compatibility and security behavior below rather than assuming semantic equivalence from a version number.

## Shared server architecture

All supported protocol generations use the same `*mcp.Server` created by `filetoolsserver.BuildServer`.

The following remain single process-wide authorities:

- tool and prompt registration;
- configured allowed roots;
- backup-store authority;
- durable-task/execution policy;
- resource limits;
- typed error behavior;
- lifecycle and cancellation plumbing.

Protocol routing must never create a second product-policy implementation or catalog.

## Stdio negotiation

Stdio remains the default transport.

### Startup roots configured

When startup allowed directories are configured:

- modern discovery may negotiate `2026-07-28`;
- filesystem authority remains entirely process-owned;
- client metadata cannot broaden roots;
- tool behavior is independent of the negotiated supported protocol generation.

### No startup roots

Legacy client roots remain a stdio-only compatibility path when no startup directories were configured.

Because `2026-07-28` deprecates roots, Scripthold must not negotiate modern protocol semantics while filesystem authority would depend on legacy client roots. In that case discovery is rejected before the SDK can mutate connection state, allowing normal legacy initialization and roots exchange instead.

Legacy roots notifications may update/clear only the process-wide dynamic root set admitted by the existing validation rules. The current working directory is never substituted implicitly.

### Legacy handshake compatibility

`MCP_STDIO_LEGACY_HANDSHAKE=1` is a narrow stdio-only compatibility override for intermediaries that probe `server/discover` and then initialize the same persistent child twice.

When enabled:

- discovery is rejected before SDK state changes;
- the first successful legacy initialization result may be reused only for an equivalent repeated initialization on that connection;
- a repeated initialization with different parameters remains an error;
- Streamable HTTP is unaffected.

Leave the override disabled for normal modern stdio clients.

## Streamable HTTP routing

One hardened `/mcp` outer pipeline routes to two internal SDK handlers:

- **legacy stateful handler** — preserves supported legacy initialization, `Mcp-Session-Id`, authenticated `POST`/`GET`/`DELETE`, SSE, idle expiry, explicit deletion, and bounded session admission;
- **`2026-07-28` stateless handler** — accepts authenticated stateless `POST`, creates no protocol session, emits no `Mcp-Session-Id`, uses no legacy session capacity, and propagates client disconnect cancellation.

The outer handler recognizes the protocol versions supported by the pinned SDK: `2026-07-28`, `2025-11-25`, `2025-06-18`, `2025-03-26`, and `2024-11-05`.

Routing rules:

- exact `2026-07-28` → stateless handler;
- exact supported legacy version → stateful handler;
- absent version header → legacy initialization route only;
- repeated, empty, comma-joined, or whitespace-variant version values → rejected before SDK dispatch;
- otherwise well-formed unsupported singleton → stateless SDK error lane only, so the protocol-defined structured unsupported-version error is returned without session admission.

The unsupported-version path returns JSON-RPC code `-32022` with HTTP `400` and structured requested/supported version data.

No routing middleware pre-reads or duplicates the JSON body. Per-request and aggregate body budgets wrap the single body stream before SDK decoding.

## Stateless HTTP method policy

For exact `2026-07-28`:

- `POST` is the MCP request method;
- protocol-level `GET` and `DELETE` return method rejection because there is no session stream or termination operation;
- `Last-Event-ID` is rejected because no event store is configured;
- any `Mcp-Session-Id` header is rejected rather than ignored;
- no legacy session capacity is acquired or released;
- normal non-SSE concurrency, rate, body, authentication, Host, Origin, proxy, logging, timeout, readiness, and shutdown controls still apply.

Health and readiness endpoints are protocol-independent.

## Common HTTP admission order

Protocol generation does not alter the security boundary. The common outer order is:

1. identify peer and trusted-proxy state;
2. enforce proxy/plaintext boundary rules;
3. validate Host;
4. validate Origin for every method;
5. apply peer rate limiting;
6. serve health/readiness where applicable;
7. reject shutdown/non-ready work;
8. validate MCP path and empty query;
9. authenticate bearer token;
10. validate transport method and protocol-version header shape;
11. apply concurrency and body-budget admission;
12. route to stateless or stateful SDK handler;
13. admit/locate a session only on the legacy stateful path;
14. emit category-only redacted access logging.

[HTTP_SECURITY.md](HTTP_SECURITY.md) is authoritative for the complete threat model, middleware requirements, limits, and accepted risks.

## Standardized header handling

`Mcp-Protocol-Version`, `Mcp-Method`, and `Mcp-Name` are untrusted network input.

- Singleton headers reject multiple values.
- Header bytes remain bounded by the HTTP server limit.
- Protocol routing requires exact version strings; aliases/prefixes/date normalization/OWS variants are not accepted.
- `Mcp-Method` and `Mcp-Name` never authorize tools, paths, roots, or execution.
- Header/body method consistency remains protocol validation, not authorization.
- Arbitrary header values are not logged.

Reverse proxies may route on standardized headers, but Scripthold validates the complete request independently.

## Discovery and capability projection

`server/discover` is supplied by the stable SDK.

Scripthold's capability projection:

- advertises only protocol versions accepted by the selected transport/configuration;
- preserves the same server identity, tools, prompts, and instructions as legacy initialization;
- omits capabilities not implemented by the server;
- does not advertise roots as a modern `2026-07-28` authority;
- preserves legacy roots capability only where the stdio compatibility path requires it;
- is deterministic for the same binary and startup configuration.

Client discovery metadata cannot change process roots or tool registration.

## Tool-list caching

The pinned SDK emits default cache fields `ttlMs: 0` and `cacheScope: "public"`. Scripthold does not choose a positive list-result cache lifetime.

A future positive cache policy would need separate review because dynamic legacy roots compatibility, prompt/tool availability, and execution policy may change independently of a stale cached catalog.

## Multi Round-Trip Requests

Current Scripthold tool handlers do not make server-to-client requests and advertise no MRTR-dependent capability.

Approval-bound edit, patch-package, restore, and garbage-collection workflows retain their explicit preview/capability/apply contracts. They are not implicitly translated into protocol MRTR flows.

## Roots deprecation strategy

- Startup roots are the preferred and authoritative configuration for every protocol version.
- HTTP never accepts client roots.
- `2026-07-28` never depends on roots.
- Legacy stdio roots remain a compatibility feature only while supported legacy clients require them.
- Removing roots support is a future compatibility decision requiring explicit migration evidence.

## Error and downgrade behavior

- Exact modern requests use stateless semantics.
- Supported legacy clients continue to initialize/use stateful sessions.
- Unsupported versions fail explicitly; Scripthold does not silently downgrade one request.
- A legacy session cannot switch protocol generation mid-session.
- Client-side fallback after failed discovery is owned by the client SDK; the stateless server path does not emulate legacy initialization.
- Protocol errors remain path-free and stack-free.

## Configuration impact

Normal HTTP dual routing and modern stdio discovery require no separate protocol-mode setting.

The only protocol-specific compatibility setting introduced after R20 is `MCP_STDIO_LEGACY_HANDSHAKE`, described above. It defaults off and is scoped to stdio.

Session limits apply only to stateful legacy HTTP sessions; process-wide tool/output/body/concurrency limits continue to apply according to their own contract.

## Required verification

Changes to this compatibility boundary require the relevant subset of:

### Stdio

- configured startup roots negotiate modern protocol with current clients;
- roots-dependent legacy fallback remains deterministic;
- legacy roots notifications remain stdio-only;
- equivalent repeated initialization is accepted only under the explicit compatibility override;
- different repeated initialization is rejected;
- representative tool behavior, errors, cancellation, backup, and output limits remain equivalent.

### HTTP

- exact modern/legacy/absent version routing;
- malformed, repeated, empty, OWS-variant, comma-joined, and unsupported headers;
- standardized header/body mismatch rejection;
- modern GET/DELETE/session-header/Last-Event-ID rejection;
- zero stateless session-capacity consumption, including when legacy capacity is full;
- legacy session admission, expiry, GET, DELETE, and shutdown behavior;
- modern disconnect cancellation;
- body-limit enforcement before SDK dispatch.

### Security and limits

Repeat the applicable HTTP negative matrix for both generations:

- bearer authentication;
- Host/Origin;
- trusted/untrusted proxies;
- per-request and aggregate body limits;
- concurrency and rate limits;
- timeouts/cancellation;
- logging redaction;
- execution dual opt-in;
- readiness/shutdown;
- malformed JSON/content type;
- no CORS headers or sensitive-path/token/body/session/header leakage.

### Interoperability and release gates

- Go module verification and dependency review;
- official protocol conformance applicable to Scripthold;
- independent legacy client interoperability when practical;
- bounded fuzzing for protocol/header/routing boundaries;
- complete Go/race/vet/static/vulnerability gates;
- native and hardened-container smoke;
- six supported OS/architecture compilation targets.

## Security review findings

### Dual routing must not fork security policy

Two SDK handlers could create bypasses if each owned different middleware. The mitigation is one outer authentication/admission pipeline with protocol routing only after shared controls, without rereading the body.

### Stateless traffic must never consume legacy session state

Doing so would produce incorrect cleanup and an artificial denial-of-service boundary. The modern path never creates/acquires/releases an application session record, and tests assert legacy counts do not change.

### Deprecated roots must not become modern authority

Modern negotiation is unavailable when stdio filesystem authority depends on legacy client roots. HTTP never accepts roots. Startup configuration remains authoritative.

### Standardized headers are not authorization

Clients control method/name/version headers. Tool/filesystem/execution authority remains derived from authenticated decoded requests plus process policy, with SDK header/body consistency checks as protocol validation only.

### Legacy behavior must remain regression-tested

New stateless routing is selected only by exact modern version evidence. Supported legacy stateful sessions remain on the same endpoint and are exercised in the compatibility gate.

## Completion record

R20 completed on 2026-08-08. Stable SDK `v1.7.0`, modern stdio negotiation, roots-safe legacy fallback, same-endpoint stateful/stateless HTTP routing, structured unsupported-version behavior, official conformance, independent legacy interoperability, fuzzing, native/hardened-container smoke, complete race/static/vulnerability checks, and all six supported build targets were verified.

Later milestones expanded the tool catalog without changing this protocol/security contract. Current tool counts belong to the authoritative runtime catalog and README rather than to the historical R20 boundary.
