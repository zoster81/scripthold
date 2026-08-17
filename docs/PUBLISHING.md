# Scripthold Publishing Procedure

This document is the maintainer procedure for publishing semantic releases from `zoster81/scripthold`. Product scope belongs in [PROJECT_DIRECTION.md](PROJECT_DIRECTION.md), milestone state in [ROADMAP.md](ROADMAP.md), and reusable engineering checks in [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md).

## Current release state

- Current public release: **`2.2.0`**.
- MCP Registry identity: **`io.github.zoster81/scripthold`**.
- Release surface: 30 tools, 3 guided prompts, and 168 registered text encodings.
- `2.0.0` remains the historical pre-rebrand rollback release with its original asset and Registry identity.
- Publication and deployment are separate. A successful GitHub Release or Registry publication does not imply that any private or production runtime was upgraded.

## Release ownership boundaries

A public release is created only from an exact clean commit that has passed the complete push-event `Test Suite` `Release candidate` gate.

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
6. Push `main` and require the `Test Suite` workflow's `Release candidate` job to succeed on that exact push-event SHA.
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

The tag-triggered Release workflow attests the already-completed exact-commit Test Suite gate instead of rerunning the full expensive race/static/vulnerability/fuzz/cross-build/container matrix before GoReleaser.

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
6. verify the Registry reports the intended semantic version;
7. record publication evidence in durable public history only where useful; keep workstation/runtime state private.

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
- [ ] the exact pushed commit's `Release candidate` job is `success`.

After tagging:

- [ ] Release workflow verifies the immutable annotated tag and exact Test Suite-gated commit;
- [ ] GoReleaser publication succeeds;
- [ ] GitHub-only MCPB publication succeeds;
- [ ] GitHub-only Registry publication succeeds;
- [ ] normal GoReleaser assets independently match `checksums.txt`;
- [ ] semantic tag, changelog release, embedded binary version, and Registry version agree.

Deployment, active rollback, restoration, launcher changes, and runtime restarts are **separate operator actions** and are not implied by this checklist.

## Project lineage

Scripthold remains an independent GPL-3.0 downstream fork of the [original `mcp-file-tools` project](https://github.com/dimitar-grigorov/mcp-file-tools). Upstream synchronization is not part of the publication procedure; see [PROJECT_DIRECTION.md](PROJECT_DIRECTION.md) for the maintenance boundary.
