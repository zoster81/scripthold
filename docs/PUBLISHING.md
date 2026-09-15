# Scripthold Publishing Procedure

This document is the maintainer procedure for publishing semantic releases from `zoster81/scripthold`. Product scope belongs in [PROJECT_DIRECTION.md](PROJECT_DIRECTION.md), milestone state in [ROADMAP.md](ROADMAP.md), and reusable engineering checks in [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md).

## Release state sources

This procedure is intentionally version-neutral. Current public release details belong in [CHANGELOG.md](../CHANGELOG.md), GitHub Releases, and the MCP Registry rather than being copied into a maintainer runbook after every publication.

The Registry identity is `io.github.zoster81/scripthold`. Historical releases remain immutable. Publication and deployment are separate: a successful GitHub Release or Registry publication does not imply that any private or production runtime was upgraded.

## Release ownership boundaries

A public release is created only from an exact clean commit that has passed the complete **full-tier** push-event `Test Suite` `Release candidate` gate. Pull-request evidence tiers are review acceleration only and never publication authority.

Local maintainers may prepare source, run tests, build ordinary release-candidate binaries, and run deterministic GoReleaser snapshots. They must not create or simulate GitHub-owned MCPB release outputs.

The following actions remain explicit maintainer decisions and are never incidental side effects of validation:

- commit and push;
- annotated release tag creation;
- GitHub Release publication;
- deployment, active rollback, or runtime restart;
- toolchain upgrades.

## Release flow

1. Complete the release-scoped roadmap work and its documented verification gate.
2. Choose a semantic version that has not already become a consumable release. Published versions are immutable; do not reuse a version after GitHub assets or a Registry record exist.
3. Promote the changelog entry to a dated heading:

   ```text
   ## X.Y.Z - YYYY-MM-DD
   ```

4. Verify that the tag has a matching dated changelog release:

   ```bash
   node scripts/verify-release-version.js vX.Y.Z
   ```

5. Verify the exact clean commit locally with the applicable release-candidate checks. `goreleaser check`, release-script tests, workflow validation, secret scanning, six-target compilation, native/container smoke, and deterministic snapshot comparison belong here when required by the release scope.
6. Push `main` and require the `Test Suite` workflow's full-tier `Release candidate` job to succeed on that exact push-event SHA. Push events are always classified as full qualification; a narrower pull-request result cannot satisfy this release gate.
7. Create and push an **annotated** tag on that same commit:

   ```bash
   git tag -a vX.Y.Z -m "Scripthold X.Y.Z"
   git push origin vX.Y.Z
   ```

8. `.github/workflows/release.yml` verifies that the tag:
   - is annotated;
   - matches the dated changelog entry;
   - resolves to the exact current `origin/main` commit at publication time;
   - has a successful push-event `Test Suite` run for that exact SHA;
   - has a successful `Release candidate` job in that run.
9. GoReleaser publishes the normal release assets: six raw binaries, six platform archives, and `checksums.txt`, with the documentation/support files configured by `.goreleaser.yml`.
10. GitHub then runs the MCPB and Registry workflows in order. Those workflows verify the immutable release/tag inputs and published checksums before producing their GitHub-only outputs.

The tag-triggered Release workflow attests the already-completed exact-commit Test Suite gate instead of rerunning the full expensive race/static/vulnerability/fuzz/cross-build/container matrix before GoReleaser. That attested push gate already includes Gitleaks and `goreleaser check` in addition to the full native/platform/security evidence.

## Core release and discovery syndication

The **core release** is the product publication authority: exact-SHA Test Suite qualification, annotated tag, GoReleaser assets, GitHub-only MCPB assets, and MCP Registry publication. OpenSSF Scorecard and third-party discovery directories are supporting trust/discovery evidence; their outage or indexing delay must not retroactively make an already-valid core release appear failed.

If a future Registry manifest references an OCI image, that immutable image and its required MCP server annotation must be published and verified **before** the Registry manifest that names it. Do not publish a Registry package that points at an image tag or digest that does not yet exist.

The **discovery/syndication** layer follows successful core publication and reports each destination independently:

- OpenSSF Scorecard publishes signed results and SARIF through its standalone workflow; it is intentionally not a binary-release gate.
- Glama ownership is declared by the tracked `glama.json`. Claiming is an account-side GitHub action, not a release-workflow API: after adding or changing `glama.json`, rerun Glama's **Claim ownership** flow to make Glama pick up the latest metadata. Do not scrape the Glama web UI as a publication or freshness API.
- Smithery publication requires the Actions secret `SMITHERY_API_KEY`. Its stdio release API accepts one MCPB bundle per release, while Scripthold currently publishes six platform-specific MCPB bundles. Until Smithery documents multi-platform variants or Scripthold has one portable bundle with equivalent compatibility semantics, automation must not arbitrarily select one platform and call that a complete Scripthold release.
- MCP.Directory is expected to discover Official MCP Registry entries. Use its normal listing/claim process when required; do not invent a write API or automate its web form.
- PulseMCP ingests from the Official MCP Registry, crawling, and manual submissions. Its publisher API is partner-gated; do not treat it as a public write endpoint.
- Docker MCP Catalog is currently excluded for the local-server distribution because Scripthold is GPL-3.0 and Docker's contributor policy does not currently consider GPL suitable for that local-server consumption model. This is a distribution-policy exclusion, not a reason to change Scripthold's license.

A future Smithery publisher may run only after the MCPB assets are already immutable on GitHub: submit the intended bundle, poll the documented release-status endpoint to a terminal state, verify the public release/listing, and report failure as syndication failure without changing the core release result.

### MCP Registry transport posture

Scripthold supports both stdio and authenticated Streamable HTTP at runtime. The Registry package metadata must describe how a published package is actually launched, not every transport the executable is capable of supporting.

The current six platform MCPB packages are therefore correctly declared as `stdio`. Do not add a `remote` localhost URL: Registry `remotes` are for genuinely reachable remote services. Do not change MCPB metadata to `streamable-http` unless the bundle consumer can actually launch that transport with its full security contract.

A self-hosted OCI package is a possible future way to expose Streamable HTTP through Registry package metadata, but it is not publishable until all of the following are represented and tested together: immutable image identity and MCP annotation, `linux/amd64` plus `linux/arm64`, non-root execution, read-only filesystem, explicit authorized-workspace mount, bearer-token secret handling, listener/port mapping, Host policy, TLS certificate/key or trusted-proxy boundary, and a client URL/trust model consistent with [HTTP Security](HTTP_SECURITY.md). Binding `0.0.0.0` without TLS merely to make a container reachable is not acceptable.

Because the existing Registry record already truthfully describes the six published MCPB packages, runtime HTTP support alone is not a reason to mint a patch release. A patch release becomes appropriate only when new versioned package metadata is complete, secure, testable, and ready to publish without rewriting historical Registry versions.

## GitHub-only MCPB boundary

MCPB artifacts are produced **only by GitHub release workflows**.

Local maintainers, agents, and release-candidate procedures must never:

- create, pack, repack, simulate, or dry-run a real `.mcpb` bundle;
- generate or independently checksum real MCPB bundles;
- run real-bundle MCPB validation locally;
- generate or validate the final MCPB-backed `server.json` Registry manifest locally.

Local work is limited to source code, workflow definitions, templates, staging metadata, and non-artifact-producing unit tests. If a GitHub MCPB or Registry workflow fails, diagnose the GitHub logs and repository source/configuration, fix the repository, and rerun the workflow on GitHub. Do not reproduce the artifact-producing step locally.

The GitHub sequence is:

1. `.github/workflows/publish-mcpb-assets.yml` checks out the immutable release tag, verifies all 12 normal GoReleaser assets against `checksums.txt`, validates the pinned MCPB toolchain, produces the six OS/architecture-specific bundles, and uploads them with `mcpb-checksums.txt`.
2. `.github/workflows/publish-registry.yml` downloads the already-published MCPB assets, verifies their GitHub-produced checksum manifest, projects the tagged authoritative tool catalog into the release manifest, validates it, and publishes through GitHub OIDC.

This boundary applies even during troubleshooting.

## Deterministic local GoReleaser verification

When the release scope requires reproducibility evidence, use two physically independent source/output roots for the same exact source state and require:

- `goreleaser check` in each source copy;
- `goreleaser release --snapshot --clean` without publication;
- the expected six raw binaries, six platform archives, and `checksums.txt`;
- byte identity for corresponding artifacts and the checksum manifest;
- exact SHA-256 coverage of the 12 logical release assets;
- matching target/VCS metadata in raw binaries;
- deterministic archive entry order, timestamps, modes, owner/group metadata where configured, and gzip header normalization.

Use `dist/artifacts.json` as the structured mapping between GoReleaser's logical asset names and physical build paths. Do not infer release identity from recursive basenames.

## Public launcher and documentation gate

Release preparation verifies the tracked PowerShell examples as **public, sanitized references**. They must remain free of real credentials, machine-specific paths, private process state, and tunnel identifiers.

The examples cover:

- standalone stdio;
- standalone authenticated HTTP;
- OpenAI Secure MCP Tunnel to a dedicated stdio child plus independent local HTTP;
- the reverse tunnel-to-HTTP plus independent local stdio topology.

Behavioral requirements for HTTP security belong in [HTTP_SECURITY.md](HTTP_SECURITY.md), durable tasks in [DURABLE_TASKS.md](DURABLE_TASKS.md), and user-facing setup in the root [README.md](../README.md). Do not duplicate those contracts here.

## Registry and release metadata

`server.template.json` is release-neutral. It contains the fork identity, repository/homepage metadata, package filename patterns, checksum placeholders, and an intentionally empty `tools` array.

`internal/toolcatalog/catalog.json` is authoritative for tool names, titles, descriptions, and annotations. The GitHub Registry workflow combines the tagged template/catalog with verified published MCPB metadata; generated `server.json` is disposable release output and is never hand-edited or committed.

The Scripthold Registry identity is `io.github.zoster81/scripthold`. Historical `2.0.0` remains under the pre-rebrand identity and is not rewritten.

## Validation toolchain

Repository workflows pin their release-validation dependencies. Local validation should use the repository/workspace-approved matching versions rather than floating `latest` tools.

The relevant categories are:

- Go toolchain, race/CGO compiler, vet, the pinned `golangci-lint` policy, Staticcheck, and govulncheck;
- Node.js for release-script tests;
- actionlint and ShellCheck for workflow/shell validation;
- GoReleaser for normal release assets;
- Gitleaks for secret scanning;
- GitHub CLI for exact run/release/tag inspection;
- the GitHub-only MCPB and MCP Publisher pins used by the publication workflows.

Tool versions and hashes should be read from the workflow/configuration that actually consumes them, not duplicated into release prose unless a version is itself part of the public compatibility contract.

## Post-publication verification

After GitHub publication completes:

1. verify the Release is neither draft nor unintended prerelease;
2. verify the expected normal GoReleaser asset names are present;
3. download `checksums.txt` plus the 12 normal raw/archive assets and independently verify every SHA-256 entry;
4. execute `--version` on a representative compatible published binary where infrastructure permits;
5. confirm the GitHub MCPB job and Registry job both completed successfully and that the expected MCPB asset names/checksum manifest are attached, **without downloading or validating the real MCPB bundles locally**;
6. verify the Registry reports the intended semantic version and inspect its package types/transports;
7. verify discovery destinations independently where a stable supported read API or public listing exists; do not replace missing APIs with form scraping;
8. report a concise status summary covering GitHub Release, Registry version/package types/transports, Smithery, Glama, MCP.Directory, PulseMCP, and OpenSSF Scorecard publication status. Distinguish `not configured`, `not listed`, `stale`, `unverifiable through a supported API`, and actual publication failures;
9. record publication evidence in durable public history only where useful; keep workstation/runtime state private.

## Release verification checklist

Before tagging:

- [ ] release-scoped roadmap gate complete;
- [ ] dated changelog entry present and `verify-release-version.js` passes;
- [ ] branch, HEAD, `origin/main`, and working tree verified;
- [ ] focused/full tests, race, vet, static and vulnerability checks pass as required;
- [ ] catalog/runtime/documentation identity checks pass;
- [ ] Node release-script tests pass;
- [ ] workflow/shell validation passes;
- [ ] six supported target builds pass;
- [ ] required native/container runtime smoke passes without weakening hardening;
- [ ] deterministic GoReleaser snapshot gate passes when required;
- [ ] Gitleaks and `git diff --check` pass;
- [ ] no credentials, private paths, local process state, or generated release output are tracked;
- [ ] the exact pushed commit's **full-tier** `Release candidate` job is `success`; no pull-request tier result is substituted for it.

After tagging:

- [ ] Release workflow verifies the immutable annotated tag and exact Test Suite-gated commit;
- [ ] GoReleaser publication succeeds;
- [ ] GitHub-only MCPB publication succeeds;
- [ ] GitHub-only Registry publication succeeds;
- [ ] normal GoReleaser assets independently match `checksums.txt`;
- [ ] semantic tag, changelog release, embedded binary version, and Registry version agree;
- [ ] Registry package types and transports match the actually published launch semantics;
- [ ] Scorecard and discovery/syndication destinations are reported separately from core release success.

Deployment, active rollback, restoration, launcher changes, and runtime restarts are **separate operator actions** and are not implied by this checklist.

## Project lineage

Scripthold remains an independent GPL-3.0 downstream fork of the [original `mcp-file-tools` project](https://github.com/dimitar-grigorov/mcp-file-tools). Upstream synchronization is not part of the publication procedure; see [PROJECT_DIRECTION.md](PROJECT_DIRECTION.md) for the maintenance boundary.
