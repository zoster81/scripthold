# Scripts Agent Guide

This guide applies to `scripts/`. Follow the repository root [`AGENTS.md`](../AGENTS.md) first.

## Scope

Scripts validate workflows, release versions, checksums, and the generated MCP Registry manifest. They are security- and release-sensitive even when small.

## Invariants

- Keep tool and dependency versions pinned where CI expects reproducibility.
- Verify downloaded archives before extraction or execution.
- Treat command-line arguments, checksums, JSON, paths, tags, and release assets as untrusted.
- Reject malformed, duplicate, missing, and unexpected entries deterministically.
- Keep fork repository and Registry identities consistent with project-identity tests.
- `internal/toolcatalog/catalog.json` is the authoritative tool metadata source.
- `server.template.json` remains release-neutral; generated `server.json` is disposable output and must not be hand-edited or committed.
- MCPB release artifacts are produced only by GitHub workflows. Local script development and tests may verify source logic, staging metadata, templates, and workflow contracts only when they do not create `.mcpb` bundles. Never run local MCPB packing, repacking, simulation, dry-run packaging, real-bundle validation/checksumming, or final MCPB-backed Registry generation. GitHub workflow failures are diagnosed from logs and source/configuration, then fixed and rerun on GitHub.
- Write generated files through a temporary file followed by replacement; remove temporary artifacts on failure.
- Tests must not publish releases, mutate Git history, require credentials, or depend on live release state.

## JavaScript style

The existing scripts use Node.js CommonJS and built-in modules. Preserve that style unless a deliberate repository-wide migration is approved. Keep functions small, validate before mutation, use stable error text where tests depend on it, and avoid new dependencies for functionality available in the standard library.

Any behavior change requires a matching `node:test` regression case. Cover valid input and at least malformed, missing, duplicate, and unexpected input relevant to the change.

## Shell style

Use `set -euo pipefail`, quote expansions, use temporary directories with cleanup traps, pin external tool versions, and verify checksums before extraction. Keep scripts compatible with the Linux CI environment unless documented otherwise.

## Verification

```bash
node --test scripts/generate-server-json.test.js scripts/prepare-mcpb-assets.test.js scripts/release-candidate-provenance.test.js scripts/run-fuzz.test.js scripts/verify-release-version.test.js
node scripts/run-fuzz.js --profile smoke
bash scripts/validate-workflows.sh
go test ./internal/projectidentity ./internal/toolcatalog -count=1
git diff --check
```

The workflow validator downloads pinned tools, so report when network access prevents running it rather than silently substituting floating versions.
