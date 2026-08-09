# Contributing to Scripthold

Thank you for contributing to Scripthold, currently maintained in `zoster81/scripthold`.

## Before you start

Read the root [`AGENTS.md`](AGENTS.md) and any scoped `AGENTS.md` that applies to the files you plan to change. Product identity, transport scope, and the independent upstream boundary are defined in [`docs/PROJECT_DIRECTION.md`](docs/PROJECT_DIRECTION.md); current priorities and compatibility gates are tracked in [`docs/ROADMAP.md`](docs/ROADMAP.md).

Prerequisites:

- Go version declared by `go.mod`;
- Git;
- Node.js 18 or later for release-script tests;
- Bash, `curl`, `tar`, and `sha256sum` for workflow linting;
- a working C compiler only when running the Go race detector locally.

Do not add credentials, real tunnel identifiers, private launcher copies, workstation-specific paths, process IDs, or local deployment state to the repository.

## Development workflow

1. Start from a clean branch based on the current target branch.
2. Inspect the affected implementation, tests, public schemas, metadata, and documentation.
3. Add or select a focused test that demonstrates the required behavior.
4. Implement the smallest coherent change.
5. Run focused tests, then the applicable repository checks.
6. Review the complete diff for unrelated changes and generated artifacts.
7. Describe behavior, compatibility impact, tests, and remaining risks in the pull request.

Keep pull requests focused. Separate unrelated refactors, dependency updates, generated metadata changes, and behavior changes whenever practical.

## Common commands

```bash
# Focused package test
go test ./internal/encoding -count=1

# Baseline verification
gofmt -w path/to/changed.go
go mod verify
go test ./... -count=1
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

# Integration and release-script tests
go run test_server.go
node --test scripts/generate-server-json.test.js scripts/verify-release-version.test.js

# Requires Bash and network access to download pinned tools
bash scripts/validate-workflows.sh

# Requires CGO and a C compiler
go test -race ./...
```

Run only checks relevant to the change during iteration, but complete the applicable gates in [`docs/DEVELOPMENT_CHECKLIST.md`](docs/DEVELOPMENT_CHECKLIST.md) before requesting review.

## Testing expectations

Tests should cover the behavior being changed, not only implementation details. Include negative and regression cases where relevant:

- malformed, empty, missing, ambiguous, or oversized input;
- encoding, BOM, malformed Unicode, and LF/CRLF behavior;
- path traversal, symlink, junction, and reparse-point escapes;
- filesystem permission, cleanup, rollback, and concurrent-modification failures;
- cancellation, timeout, saturation, deterministic ordering, and bounded resource use;
- Windows, Linux, and macOS differences.

Prefer temporary directories and synthetic fixtures. Tests must not depend on a contributor's workstation layout or private services.

## Documentation and metadata

Use repository-relative links and portable placeholders. Public documentation must be useful from a normal clone.

`internal/toolcatalog/catalog.json` is the authoritative source for tool names, titles, descriptions, and annotations. When public tool behavior changes, keep the catalog, registration, README, `TOOLS.md`, tests, and release projection consistent.

Do not hand-edit generated `server.json`. Update `server.template.json`, the tool catalog, or `scripts/generate-server-json.js`, then run its tests.

Update `CHANGELOG.md` for user-visible behavior, compatibility, security, packaging, or architecture changes.

## Security-sensitive changes

Changes under `internal/security`, `internal/filesystem`, execution handlers, allowed-root handling, or native HTTP transport require explicit negative tests. HTTP work must follow [`docs/HTTP_SECURITY.md`](docs/HTTP_SECURITY.md); preserve its fail-closed defaults and document any unavoidable TOCTOU, SDK, proxy, or platform limitation.

Never weaken path validation to make a test pass. Fix the abstraction or test setup instead.

## Commit and pull-request guidance

Use concise English commit messages. Do not include build products, credentials, local caches, private launchers, or temporary recovery files.

A pull request should state:

- the problem and intended behavior;
- affected public behavior or compatibility;
- security and failure-mode considerations;
- tests and checks executed;
- checks not performed and why;
- remaining risks or follow-up work.

Release tags, published assets, and Registry publication follow [`docs/PUBLISHING.md`](docs/PUBLISHING.md) and are maintainer-controlled.
