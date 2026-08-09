# Scripthold publishing notes

This document is the maintainer procedure for publishing Scripthold from `zoster81/scripthold`. Product direction, historical milestone evidence, and detailed security contracts live in their dedicated documents and are intentionally not repeated here.

## Current state

- Current public release: `2.1.0`, the first Scripthold-named semantic release and the first release assigned to the fork-owned Scripthold Registry identity.
- Rollback baseline: `2.0.0`, published before the Scripthold repository/asset/Registry rename and retained with its historical asset and Registry identity.
- Source catalog: 30 tools and 3 guided prompts over stdio and Streamable HTTP.
- Protocols: MCP `2026-07-28` where roots policy permits it, with retained legacy compatibility; HTTP uses stateless modern requests beside stateful legacy sessions behind one security pipeline.
- Registry: `io.github.zoster81/scripthold` version `2.1.0` is published and active; `2.0.0` remains under the historical `io.github.zoster81/mcp-file-tools` identity.
- Assets: Scripthold-named releases use `scripthold_<os>_<arch>` GoReleaser names and publish matching OS/architecture-specific `.mcpb` bundles plus a separate MCPB checksum manifest.
- Module: `github.com/zoster81/scripthold`.
- Release tags must match a dated `CHANGELOG.md` entry and the generated Registry version.

See [ROADMAP.md](ROADMAP.md) for the remaining 2.1.0 deployment/rollback gate, [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md) for reusable engineering checks, [HTTP_SECURITY.md](HTTP_SECURITY.md) for transport security, and [PROJECT_DIRECTION.md](PROJECT_DIRECTION.md) for lineage and maintenance policy.

## Fork release flow

Use this flow for `2.1.0` and later fork-owned semantic releases. Development commits may be tested or deployed internally, but public tags require a dated changelog entry and the full applicable release gate.

1. Ensure the release-scoped roadmap work is complete and `main` is clean, tested, and pushed to `origin`.
2. Choose a semantic version that has not been used by this fork.
3. Promote the `CHANGELOG.md` unreleased section to a dated `## X.Y.Z - YYYY-MM-DD` release heading.
4. Verify that the semantic tag is represented by that exact changelog release:

   ```bash
   node scripts/verify-release-version.js vX.Y.Z
   ```

5. Push `main` and wait for its GitHub Actions checks to pass.
6. Create and push the release tag:

   ```bash
   git tag -a vX.Y.Z -m "Scripthold X.Y.Z"
   git push origin vX.Y.Z
   ```

7. `.github/workflows/release.yml` revalidates the tag/metadata match, then reruns the cross-platform race, HTTP, catalog/documentation identity, native task lifecycle, manual server, static, vulnerability, fuzz, workflow, and release-script gates before GoReleaser. Test jobs remain read-only; the GoReleaser release job and MCPB asset workflow receive `contents: write`, while Registry publication receives `contents: read` plus `id-token: write`.
8. `.goreleaser.yml` publishes the release to `zoster81/scripthold` with:
   - reproducible `-trimpath` builds timestamped from the source commit;
   - archive binary and documentation entries with commit-derived timestamps plus fixed owner, group, and modes;
   - `.tar.gz` archives for Linux/macOS and `.zip` archives for Windows;
   - raw binaries for all six supported OS/architecture targets;
   - `checksums.txt`;
   - `README.md`, `TOOLS.md`, `CHANGELOG.md`, `LICENSE`;
   - `docs/DURABLE_TASKS.md`;
   - `examples/start-openai-tunnel-stdio-plus-local-http.ps1`;
   - `examples/start-openai-tunnel-http-plus-local-stdio.ps1`;
   - `examples/start-local-stdio.ps1`;
   - `examples/start-local-http.ps1`.
9. After GoReleaser publishes the normal binaries and platform archives, `.github/workflows/release.yml` invokes `.github/workflows/publish-mcpb-assets.yml`. That workflow verifies the immutable release tag and all 12 GoReleaser checksums, validates and deterministically packages each raw binary as one OS/architecture-specific MCPB bundle, and uploads the six `.mcpb` assets plus `mcpb-checksums.txt` to the same GitHub Release. `.github/workflows/publish-registry.yml` runs only after those release assets exist: it downloads and verifies the six published MCPB bundles, projects their hashes plus the tagged catalog into `server.json`, validates the manifest, and publishes through GitHub OIDC. Manual repair of an existing tag runs the MCPB asset workflow first and the Registry workflow second, without rebuilding or replacing the original GoReleaser assets.
10. Independently verify the published asset names and SHA-256 values before announcing the release.

## Public launcher examples

The tracked launchers cover standalone stdio, standalone HTTP, tunnel-owned stdio with independent local HTTP, and the reverse combined topology. Private credentials and machine-specific orchestration must remain outside the repository.

A private combined launcher must normalize process identity across the object shapes it uses: `Start-Process -PassThru` exposes the process identifier as `Id`, while CIM process discovery exposes `ProcessId`. Persist PID files and compare ownership through one normalization helper rather than assuming either property exists universally. Shutdown cleanup must also be idempotent: a child that exits after its parent is stopped is already in the desired terminal state, not a cleanup failure. Validate the complete owned process topology before destructive actions and never broaden cleanup to unrelated process trees.

`examples/start-openai-tunnel-stdio-plus-local-http.ps1` is the default OpenAI quick start. It must:

- remain in English;
- contain placeholders only;
- never contain a real Runtime API key, Tunnel ID, or bearer token;
- require the exact `tunnel_` plus 32 lowercase hexadecimal identifier format;
- configure `MCP_COMMAND` for one tunnel-owned stdio child and clear tunnel-side URL/header bindings;
- enable the stdio legacy-handshake compatibility flag so an equivalent repeated initialize is idempotent while a different repeat remains rejected;
- start a second, independent Scripthold process on authenticated loopback HTTP;
- use distinct backup stores for the stdio and HTTP processes;
- share one separate owner-only task store and keep its supervisor independent from both frontends;
- prevent the HTTP process from inheriting OpenAI control-plane credentials and prevent the tunnel process from inheriting the HTTP token;
- keep both `task_run` execution kinds disabled by default and require the additional HTTP gate only on the HTTP branch;
- validate the tunnel client, MCP binary, token file, and canonical allowed directory;
- run `tunnel-client doctor --explain` against the stdio command before starting the daemon;
- report runtime success only after tunnel `/readyz` succeeds and `/api/status` shows exactly one enabled `main` channel with `probe_status=ok`;
- stop only processes it started and restore all managed process-level environment variables when it exits.

`examples/start-openai-tunnel-http-plus-local-stdio.ps1` is the explicit reverse topology. It must keep HTTP bearer values out of argv, redirect background logs away from local MCP stdout, run the independent stdio process in the foreground, and start/reuse the same durable task supervisor used by the HTTP frontend.

`examples/start-local-stdio.ps1` is the standalone local stdio reference. It must reserve stdout for MCP JSON-RPC, select `--transport=stdio` explicitly, clear unrelated tunnel/HTTP credentials, keep execution disabled by default, and start/reuse the durable task supervisor without attaching it to MCP stdout.

`examples/start-local-http.ps1` is the standalone native HTTP reference. It must:

- bind to loopback by default and require explicit non-loopback opt-in;
- require TLS or an explicitly trusted proxy CIDR for non-loopback use;
- load the bearer token from a regular private file rather than a command-line argument;
- keep both execution authorization layers disabled by default;
- expose all durable task limits and start/reuse the independent supervisor;
- clear unrelated control-plane credentials from the server child environment;
- restore all managed process-level environment variables when it exits.

Real credentials belong in private copies outside the Git checkout.

## MCP Registry status

`server.template.json` is a release-neutral template owned by `zoster81/scripthold`. It contains the fork namespace, repository, homepage, package filenames, zeroed checksum placeholders, and an intentionally empty `tools` array; it is not published directly. The authoritative tool names, descriptions, titles, and annotations live in `internal/toolcatalog/catalog.json`, which is embedded by the Go runtime.

`.github/workflows/publish-mcpb-assets.yml` owns MCPB release packaging. It verifies an immutable `refs/tags/<version>` source checkout, the exact six raw binaries plus six platform archives, and all 12 GoReleaser SHA-256 values before `scripts/prepare-mcpb-assets.js` stages six binary MCPB packages from the tagged catalog and license. MCPB 2.1.2 validates each staged package; normalized ZIP metadata and a second byte-identical pack check enforce deterministic output before the six bundles and `mcpb-checksums.txt` are uploaded idempotently to GitHub Release. `.github/workflows/publish-registry.yml` has read-only release-content permission: it downloads and verifies those already-published MCPB assets, then `scripts/generate-server-json.js` projects the tagged catalog, exact version, MCPB URLs, OS/architecture selectors, and hashes into temporary `server.json` before GitHub OIDC publication.

The Scripthold Registry identity is `io.github.zoster81/scripthold`, and published version `2.1.0` is its first active release. The already-published `2.0.0` record remains under the historical `io.github.zoster81/mcp-file-tools` identity and is not rewritten by the repository rename. The production Registry reports `2.1.0` as active/latest with six MCPB packages; the publication manifest is generated from the authoritative 30-tool catalog.

## Project lineage

Scripthold remains an independent downstream fork of `dimitar-grigorov/mcp-file-tools` and preserves the original project's GPL-3.0 attribution. Upstream synchronization is not part of the release procedure; evaluate upstream ideas selectively under the maintenance policy in [PROJECT_DIRECTION.md](PROJECT_DIRECTION.md).

## Validation toolchain

The repository pins the release-validation toolchain instead of resolving
floating `latest` versions during CI:

- actionlint 1.7.12 and ShellCheck 0.11.0 for GitHub Actions workflows;
- actions/checkout 7.0.1, actions/setup-go 7.0.0, and actions/upload-artifact 7.0.1;
- Staticcheck v0.7.0 and govulncheck v1.6.0 for Go analysis;
- GoReleaser action 7.2.3 with GoReleaser v2.17.1 for release generation;
- MCP Publisher v1.8.1 for Registry validation/publication and MCPB 2.1.2 for native-bundle schema/format validation.

The workflow-linter archives and MCP Publisher archive are verified against
fixed SHA-256 values before extraction. Local release preparation should also
run Gitleaks and use Cosign to verify signed release assets when bundles are
available.

## Release verification checklist

Run this checklist after the applicable release-scoped milestone and completion gates in [ROADMAP.md](ROADMAP.md) pass. Use [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md) for internal milestones and builds.

- release-scoped roadmap work complete, with any intentionally post-publication deployment gate recorded explicitly;
- working tree clean;
- expected branch and HEAD verified;
- no credentials or real tunnel identifiers in tracked files or history;
- `go test -count=1 ./...` succeeds;
- `go vet ./...` succeeds;
- `go mod verify` succeeds;
- PowerShell example parses under Windows PowerShell 5.1;
- JSON, YAML, JavaScript, Markdown, actionlint, and ShellCheck checks succeed;
- README, tool reference, changelog, Smithery metadata, runtime catalog, roadmap, and generated Registry manifest describe the same fork-owned capabilities;
- runtime tool registration and annotations match `internal/toolcatalog/catalog.json`, every catalog tool is linked from README and documented in TOOLS.md, and `server.template.json` keeps `tools` empty;
- secure traversal tests cover Unix symlinks and Windows junction/reparse-point escapes;
- mutation tests cover synced staging, transactional backup rollback, cleanup, no-replace creation, concurrent-modification rejection, and platform cross-builds;
- typed-error tests cover standard and joined causes, security/path categories, encoding categories, mutation conflicts, cancellation, and stable batch error codes;
- ordered-concurrency tests cover bounded in-flight work, deterministic commit order, cancellation modes, early stop, saturation, and race detection;
- Staticcheck and govulncheck succeed at the versions pinned by CI;
- Gitleaks reports no tracked-history secrets;
- the container image is built from pinned bases, runs as UID/GID 10001, and passes stdio plus direct-TLS HTTP smoke tests with the documented mount, tmpfs, health, and shutdown model;
- all six Windows/Linux/macOS amd64/arm64 builds are generated, and representative binaries are runtime-executed where infrastructure permits;
- GoReleaser configuration targets `zoster81/scripthold`, passes `goreleaser check`, and produces identical checksums across two independent snapshots;
- six OS/architecture-specific MCPB bundles validate against the pinned MCPB schema/format, have recorded SHA-256 values, and a generated six-package manifest passes `mcp-publisher validate` without publication;
- `scripts/verify-release-version.js` confirms the release tag has a matching dated changelog entry before GoReleaser runs;
- release tag, changelog release, embedded binary version, and generated Registry version match;
- release assets and checksums are verified after publication;
- the known-good rollback binary and launcher reversal are verified offline before deployment, followed by an active rollback test during the controlled release cutover.
