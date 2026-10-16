# Scripthold Publishing Procedure

This document is the maintainer procedure for publishing semantic releases from `zoster81/scripthold`. Product scope belongs in [PROJECT_DIRECTION.md](PROJECT_DIRECTION.md), milestone state in [ROADMAP.md](ROADMAP.md), and reusable engineering checks in [DEVELOPMENT_CHECKLIST.md](DEVELOPMENT_CHECKLIST.md).

## Release state sources

This procedure is intentionally version-neutral. Current public release details belong in [CHANGELOG.md](../CHANGELOG.md), GitHub Releases, and the MCP Registry rather than being copied into a maintainer runbook after every publication.

The Registry identity is `io.github.zoster81/scripthold`. Historical releases must not be changed or reused. That maintainer policy does not imply GitHub's optional release-immutability enforcement is enabled. Publication and deployment are separate: a successful GitHub Release or Registry publication does not imply that any private or production runtime was upgraded.

## Release ownership boundaries

A public release is created only from an exact clean commit that has passed the complete **full-tier** push-event `Test Suite` `Release candidate` gate. Pull-request evidence tiers are review acceleration only and never publication authority.

Local maintainers may prepare source, run tests, build ordinary release-candidate binaries, and run deterministic GoReleaser snapshots. They must not create or simulate GitHub-owned MCPB release outputs.

The following actions remain explicit maintainer decisions and are never incidental side effects of validation:

- commit and push;
- annotated release tag creation;
- GitHub Release publication;
- deployment, active rollback, or runtime restart;
- toolchain upgrades.

## Repository governance and OpenSSF Best Practices

The OpenSSF Scorecard is a diagnostic, not a release gate or a reason to generate artificial activity. Protect the supply chain by requiring genuine pull requests, working CI, independent review when a reviewer is available, and verifiable release attestations. The [contributor review policy](../CONTRIBUTING.md#review-ownership-and-merge-safety) and `.github/CODEOWNERS` describe the human responsibilities; **only GitHub repository rules can enforce them**.

The following setup is an **operator checklist**, not a claim that these settings are currently enabled. Configure it from GitHub using repository administrator access; the project workflows cannot change repository rules or register a Best Practices account on the maintainer's behalf.

### Stage 1: require PRs and real CI without locking out a solo maintainer

1. Open [the repository rulesets](https://github.com/zoster81/scripthold/settings/rules) and edit the active `Main integrity` branch ruleset for `main`. Keep deletion and non-fast-forward protection enabled, with no broad bypass exceptions.
2. Enable **Require a pull request before merging**. While operating as a solo maintainer, use **zero required approving reviews** initially; this still requires a PR but does not pretend to provide independent human review.
3. Enable **Require status checks to pass before merging** and select the existing GitHub Actions checks **`Release candidate`** (Test Suite) and **`Analyze (Go)`** (CodeQL), using the real check names shown on a recent PR. Where GitHub offers a check-source selector, use the GitHub Actions app. Require the branch to be up to date before merging. Do not require individual matrix jobs that are legitimately skipped by the repository's evidence-tier policy.
4. Test the ruleset against a small, legitimate documentation PR: verify both required checks complete and the PR cannot merge while either is pending or failing; verify that an approved result can actually merge. External contributors' first-time Actions runs may require maintainer approval before checks can start. Do not use `pull_request_target` with untrusted code or grant extra token permissions to make a pending check pass.
5. Revisit existing release procedures after activating the ruleset: publication still requires a successful **full-tier push-event** `Release candidate` on the exact merged commit. A green PR check alone is not sufficient.

### Stage 2: require independent human review

1. Arrange an **independent trusted reviewer** with explicit repository write access who can actually inspect security, code and release workflow changes. Agree on availability and emergency maintenance coverage before making the next step mandatory.
2. Update CODEOWNERS to add the authorized reviewer where responsibility genuinely applies, including `.github/CODEOWNERS` and workflow/release-security files. Do not invent reviewers or use alternate identities to manufacture approvals.
3. In the same ruleset enable **one required approving review**, **dismiss stale approvals when new changes are pushed**, **require approval of the most recent reviewable push**, and optionally **require review from code owners** only after an eligible, distinct code owner can approve a maintainer-authored change. Require conversations to be resolved where supported.
4. Verify with a real PR that self-approval is rejected, the other reviewer can approve, new changes invalidate stale approval, and failed or pending CI prevents merging. Keep bypass permissions narrowly scoped and auditable; do not disable security controls just to unblock routine updates.

If a second reviewer cannot yet be arranged, keep Stage 1 enabled and explicitly report that independent review is **not** enforced. Introducing a mandatory one-person review rule before an eligible reviewer exists would prevent legitimate maintenance.

### OpenSSF Best Practices and provenance

- With the maintainer's own account, register the public project at [OpenSSF Best Practices](https://www.bestpractices.dev/), supply the canonical GitHub URL and answer the criteria with links to the actual policy, tests, branch rules and release evidence. Claim only requirements that are demonstrably met; pursue the passing badge through its real questionnaire. Account registration and attestation of organizational practices are human responsibilities.
- In [GitHub repository security settings](https://github.com/zoster81/scripthold/settings/security_analysis), verify that Dependabot alerts/security updates, private vulnerability reporting, code scanning, secret scanning and push protection are enabled **where available**. Record unavailable features or plan limits honestly; do not claim they are enabled without checking. Protect the maintainer account with passkeys or two-factor authentication and securely retained recovery methods.
- Continue generating real GitHub OIDC/Sigstore attestations for **future** release assets through the existing Release workflow. Verify the signature/subject/checksum relationship independently. For future releases, additionally publish the original signed DSSE envelope as an in-toto JSONL companion only after independent verification of the Sigstore bundle, exact source and every release checksum. The JSONL reuses the original signature and requires the accompanying Sigstore bundle for certificate/transparency evidence; it is not a renamed bundle or a new attestation. Never retrofit unearned attestations to historic builds or publish a new release merely to affect the historical signed-release score.
- Monitor [the public Scorecard results](https://securityscorecards.dev/viewer/?uri=github.com/zoster81/scripthold) for subsequent, correctly attributed scans. Historical repository age, contributor diversity and reviewed changesets take time; a successful CI run is not automatically the same as a PR checked **before** merge. Avoid artificial commits, fake reviewers, misleading badges or relaxed scans to change the number.

## Release flow

1. Complete the release-scoped roadmap work and its documented verification gate.
2. Choose a semantic version that has not already become a consumable release. Treat every published version as permanent; do not reuse one after GitHub assets or a Registry record exist.
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
   - has a successful `Release candidate` job in that run;
   - has a successful OpenSSF Scorecard `push` run on the default `main` branch for that exact SHA.
9. GoReleaser publishes the normal release assets: six raw binaries, six platform archives, and `checksums.txt`, with the documentation/support files configured by `.goreleaser.yml`.
10. The same release job generates signed Sigstore SLSA build provenance for every normal asset listed by `checksums.txt`, persists the attestation through GitHub's attestation service, checks every original release asset against its checksum and independently verifies the attestation's signature, builder, source and subject with GitHub CLI. Only on success does it attach both the original `.sigstore.json` bundle and the same signed DSSE envelope as `.intoto.jsonl` to the GitHub Release. Consumers must retain the bundle for Sigstore trust evidence; JSONL alone is not independently sufficient.
11. GitHub then runs the MCPB and Registry workflows in order. Those workflows verify the release tag, published assets, and checksums before producing their GitHub-only outputs.

**GitHub release immutability is not yet supported by this multi-job publication flow.** GoReleaser currently publishes the release before the separate attestation and MCPB jobs attach all of their assets. Do not enable GitHub release-immutability enforcement until the pipeline creates a draft, attaches and verifies every normal/provenance/MCPB asset through authenticated access, and publishes only after all required files are present. Draft assets cannot be consumed through the existing unauthenticated public release-download URLs, so a draft-first migration requires an explicit cross-job transfer and failure-recovery design. Do not treat the existing no-mutation policy as a GitHub-enforced lock.

The tag-triggered Release workflow attests the already-completed exact-commit Test Suite and OpenSSF Scorecard push evidence instead of rerunning the full expensive race/static/vulnerability/fuzz/cross-build/container matrix before GoReleaser. The Test Suite push gate already includes Gitleaks and `goreleaser check` in addition to the full native/platform/security evidence. Scorecard itself remains on its supported default-branch execution path; release tags do not invoke the Scorecard action directly.

## Future immutable-release pipeline: prevention requirements (not active)

The current GitHub release, GitHub-only MCPB, and Registry workflows remain the supported publication path. Do not enable GitHub's release-immutability setting or replace these workflows based on a green build alone. Any replacement must retain the release format and trust evidence that users already receive, while eliminating the need to add assets after public publication. Historical CI failures involving temporary-path aliases, dependency/version mismatches, and incorrect package types inform the following **design requirements**, not changes to the working runtime.

### Preserve release authority and output semantics

- Require an annotated release tag on the exact current `main` commit, a successful **full-tier push-event** Test Suite `Release candidate` for that SHA, and a successful Scorecard **main-branch push** run for the same SHA. Scorecard does not support release-tag execution. A passing PR, a skipped required job, a different commit, or a rerun selected only by a mutable name is insufficient. Independently resolve the annotated Git tag to its commit; GitHub Release `target_commitish` is not source evidence because GitHub ignores that field when the tag already exists.
- Keep GoReleaser, Node, the MCPB CLI, publishing tools, and external Actions pinned and compatible. Update every `github/codeql-action` component together; mixed versions can fail even when the individual action references are valid. Do not combine speculative toolchain upgrades with the pipeline migration.
- Qualify the exact GoReleaser version with publication skipped, not snapshot semantics. Preserve the six original binaries, six platform archives, checksum list, embedded version, archive contents, timestamps/modes, release notes from GoReleaser, and existing title/prerelease behavior. Archive/checksum paths differ from raw binary build paths; validate each type inside its producer, then transmit **no producer-local path** to another job.
- Preserve the genuine Sigstore/SLSA attestation for the **12 normal binary/archive subjects**. Publish its verified original bundle and signed in-toto envelope without fabricating subjects or signatures for later MCPB bundles.

### Keep host paths and file identities separate

The filesystem of one GitHub job is not the filesystem of another. In particular, macOS may expose the same trusted temporary directory as both `/var/folders/...` and `/private/var/folders/...`, and Windows may expose a directory through a short 8.3 alias such as `RUNNER~1`. The future producer-to-consumer manifest must carry only exact allowlisted ASCII asset names, sizes and SHA-256 hashes, plus authenticated repository, source commit, workflow run, producer attempt, and numeric GitHub artifact IDs. Never transfer `/tmp`, `/var/tmp`, `$RUNNER_TEMP`, Windows/UNC paths, or GoReleaser build paths as asset authority. Never repair an untrusted name using `basename`, case conversion, Unicode normalization, or separator replacement; reject a noncanonical input.

Within each Linux staging job, use fresh, private, single-owner destinations and verified ordinary-file identities. Resolve and authorize the intended directory boundary before access, prevent symlink/hardlink and rename races, verify actual streamed bytes, and fail without overwriting existing files when an input changes. Do not reject a legitimate platform-provided temporary-directory parent solely because its *spelling* includes a system alias; distinguish trusted ancestor resolution from an untrusted leaf. Executable modes and MCPB layout must come from the verified release/packaging contract, not from permissions assumed to survive GitHub artifact transfer.

### Transfer complete typed inventories before publication

The planned design separates an unprivileged normal release producer, a GitHub-only MCPB producer, and one narrowly privileged finalizer. Each stage has its own exact, independently validated allowlist:

- **15 normal files:** six raw binaries, six platform archives, `checksums.txt`, and two authentic provenance companions.
- **7 MCPB files:** six real, GitHub-produced platform-specific `.mcpb` packages and `mcpb-checksums.txt`.
- **22 public release files:** only the verified combination of both inventories. Release notes and GoReleaser internal metadata are authenticated transfer inputs, **not extra public assets**.

When using the pinned raw-file GitHub artifact actions, set `upload-artifact` to `archive: false`, `if-no-files-found: error`, and `overwrite: false`. A raw upload uses the **physical filename**, not its configured artifact `name`; names must be deterministic, attempt-specific, and mapped back to the original allowlisted asset by trusted metadata. For `download-artifact`, select authenticated numeric `artifact-ids`, use a fresh private destination, `skip-decompress: true`, and `digest-mismatch: error`. This matters especially for original `.zip` release files, which must remain byte-identical files rather than being automatically extracted. Independently check that **every** requested ID and all 22 byte streams arrived: action success or server-reported digest alone is not sufficient.

The builder and packaging jobs must not receive `contents: write`. The finalizer alone may create an unpublished GitHub **draft**, resume only exactly matching pre-existing draft assets, verify the remote 22-file set against original size/digest and source identity, then perform the single public-publish transition. A partial upload, conflicting asset, stale attempt, duplicate job, or failed remote verification must leave the release unpublished; never overwrite a published release. Disable or repurpose any independent manual post-publication upload/repair entrypoints before enabling immutability. Publish Registry metadata through the existing separate OIDC job **after** the verified public release, with idempotent error handling; a Registry outage must not trigger mutation of an immutable GitHub release.

A lost response to the final publish request is **not** permission to publish again blindly. Fetch the release and complete remote asset list through authenticated GitHub APIs. The read-only `verifyPublishedAssetSet` contract may recognize an already published **immutable** release only when the same exact 22 names, sizes, upload states, IDs, and SHA-256 digests match the staged manifest. The existing `verifyDraftAssetSet` retains its draft-only requirement for the prepublish gate. If the API still reports a draft, partial inventory, mutable public release, or conflicting data, stop and re-establish the current remote state before taking any action; never repair a public release by overwriting assets. Neither validator performs remote writes or substitutes for independent tag/run/source verification.

Pin the **publisher and packaging code revision** as well as the input asset commit. Reusable workflows and manual dispatch must not silently run mutable `main` scripts against a different tag without an explicit, verified code-identity contract. Serialize competing publication entrypoints for the same tag.

The offline `validatePublicationAuthoritySnapshot` check binds the complete 22-file staging manifest to an annotated Git tag object, the exact current `main` commit, the checked-out publisher commit, successful exact-commit **main-push** Test Suite and Scorecard runs, and a completed `Release candidate` job from the selected Test Suite run **and attempt**. It accepts authenticated, repository-scoped API responses as inputs; it does **not** authenticate how those responses were fetched, lock concurrent publishers, or grant publication permission. A future finalizer must fetch this evidence independently, recheck authoritative refs and draft assets immediately before each state-changing request, and fail closed if any evidence changes. GitHub REST job metadata (including `run_attempt`) is required; a projection that omits fields is not suitable as publication authority.

The offline `planReleaseFinalizerStep` offers **non-executable proposals**, never authorization to mutate GitHub. It first verifies the same publication evidence, then distinguishes unknown/ambiguous release lookup (`inspect-absence` or halt), a verified draft with missing assets (one canonical `upload-candidate`), a complete draft (`publish-candidate`), and a fully checked immutable release (`complete`). After a lost publish response, only a freshly checked published immutable release is recognized as complete; an apparently unchanged draft must halt rather than blindly retry. Uncertain uploads may advance only if a fresh remote listing independently contains the exact asset. None of these proposals acquires a lock or prevents another writer: the future workflow must implement one same-tag concurrency boundary, authenticate all API responses, and refresh source, release and assets before **every** remote write.

For that same-tag boundary, use the **same canonical tag-derived concurrency group across tag-triggered and manual finalizer entrypoints**; a group based on the workflow name or manual-dispatch branch can accidentally isolate competing writers. Set `cancel-in-progress: false` so an executing publisher is never interrupted. GitHub supports `concurrency.queue: max` (up to 100 pending runs), but the currently pinned `actionlint` v1.7.12 does **not** accept that key. Do not add it until a coordinated, pinned validator upgrade passes workflow and CI qualification. Without `queue: max`, GitHub replaces an older pending run when a newer one queues, even when `cancel-in-progress` is false; operators must not assume every retry executes or that dispatch order is preserved. Treat any canceled or superseded attempt as requiring a fresh authority/inventory check, not implicit publication success.

For release-asset inventory, the authenticated read-only `readReleaseObservation` adapter accepts an already established positive numeric release ID and a canonical tag. It uses pinned-version GitHub REST `GET` requests only: read the release by ID, fetch `/repos/zoster81/scripthold/releases/{release_id}/assets?per_page=100&page=1`, then read the release by ID again to reject intervening identity or draft-state changes. It refuses redirects, unexpected origins, non-JSON or oversized responses (512 KiB per response), authentication failures, and asset pages larger than the 22-file contract. The separate manifest validator handles missing, renamed, or mismatched assets. The caller supplies the secret token; the adapter never creates, uploads, publishes or discovers a missing draft. `normalizeReleaseAssetPage` additionally rejects any pagination `Link` header, mismatched release ID or query, non-200 response, or more than 22 entries. A 100-item page suffices for the fixed 22-file allowlist; silently concatenating, truncating, or ignoring pages is forbidden. A 404 produces only the untrusted `inspect-absence` planning state, **not** permission to create. A future workflow must independently discover drafts (the release-by-tag API documents published releases only), authorize same-tag serialization and recheck state immediately before any mutation. None of this reader or planner code is invoked by the currently working production release workflows.

### Required evidence before approving this migration

Treat the existing CI and release scripts as a coupled system. Update release/MCPB/Registry workflows, source/provenance validators, `scripts/distribution-trust.test.js`, `scripts/test-ci-policy.test.js`, architecture tests, and `testarchitecture/evidence-map.json` together. Replace policy assertions that explicitly require *late* `gh release upload` or broad write permissions with **stronger** assertions for the draft-only finalizer, read-only producers, source binding, and absence of post-publication mutation. Never delete or skip a failing security gate merely to make a changed workflow green.

Before the first review push, require negative tests for missing/duplicate/wrong-type assets, 15+7 inventory mismatches, path aliases (including Windows 8.3 and macOS temporary roots), symlink/replacement races, wrong run or attempt, incomplete download, missing IDs, raw ZIP byte preservation, malicious names, token misuse, mixed workflow-code revisions, and every draft/retry/finalizer failure boundary. Run applicable Node/Go, actionlint/ShellCheck, static/vulnerability, provenance, and complete-diff checks. A subsequent **authorized, non-release** GitHub workflow test must prove the exact pinned raw upload/download behavior across jobs using harmless synthetic files; it must not create a public tag, release, or local MCPB package. Only after the actual cross-platform full-tier CI and this integration evidence pass may a maintainer authorize a genuine immutable release.

See [Development Checklist](DEVELOPMENT_CHECKLIST.md) for the reusable review and verification gate. These are **future requirements**, not claims that the draft-first pipeline, raw cross-job transfer, or immutable publication already works.

## Core release and discovery syndication

The **core release** is the product publication authority: exact-SHA Test Suite qualification, annotated tag, GoReleaser assets, signed Sigstore build provenance for those normal assets, GitHub-only MCPB assets, and MCP Registry publication. OpenSSF Scorecard and third-party discovery directories are supporting trust/discovery evidence; their outage or indexing delay must not retroactively make an already-valid core release appear failed.

If a future Registry manifest references an OCI image, that immutable image and its required MCP server annotation must be published and verified **before** the Registry manifest that names it. Do not publish a Registry package that points at an image tag or digest that does not yet exist.

The **discovery/syndication** layer follows successful core publication and reports each destination independently:

- OpenSSF Scorecard publishes signed results and SARIF through its standalone workflow on default-branch `main` pushes and its schedule; manual dispatch remains available but is experimental upstream. The Scorecard action does not support release-tag pushes. Before publication, the Release workflow requires successful exact-commit Scorecard evidence from the supported `main` push. The release then publishes signed Sigstore provenance for subsequent Scorecard scans. External result indexing remains separate from the core release result.
- Glama ownership is declared by the tracked `glama.json`. Claiming is an account-side GitHub action, not a release-workflow API: after adding or changing `glama.json`, rerun Glama's **Claim ownership** flow to make Glama pick up the latest metadata. Do not scrape the Glama web UI as a publication or freshness API.
- Smithery publication requires the Actions secret `SMITHERY_API_KEY`. Its stdio release API accepts one MCPB bundle per release, while Scripthold currently publishes six platform-specific MCPB bundles. Until Smithery documents multi-platform variants or Scripthold has one portable bundle with equivalent compatibility semantics, automation must not arbitrarily select one platform and call that a complete Scripthold release.
- MCP.Directory is expected to discover Official MCP Registry entries. Use its normal listing/claim process when required; do not invent a write API or automate its web form.
- PulseMCP ingests from the Official MCP Registry, crawling, and manual submissions. Its publisher API is partner-gated; do not treat it as a public write endpoint.
- Docker MCP Catalog is currently excluded for the local-server distribution because Scripthold is GPL-3.0 and Docker's contributor policy does not currently consider GPL suitable for that local-server consumption model. This is a distribution-policy exclusion, not a reason to change Scripthold's license.

A future Smithery publisher may run only after the MCPB assets are complete and checksum-verified on GitHub: submit the intended bundle, poll the documented release-status endpoint to a terminal state, verify the public release/listing, and report failure as syndication failure without changing the core release result.

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

1. `.github/workflows/publish-mcpb-assets.yml` checks out the annotated release tag, verifies all 12 normal GoReleaser assets against `checksums.txt`, validates the pinned MCPB toolchain, produces the six OS/architecture-specific bundles, and uploads them with `mcpb-checksums.txt`.
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
5. verify that new releases contain the original `.sigstore.json` bundle and the matching `.intoto.jsonl` signed envelope, and that GitHub attestation verification succeeds for independently downloaded normal release assets;
6. confirm the GitHub MCPB job and Registry job both completed successfully and that the expected MCPB asset names/checksum manifest are attached, **without downloading or validating the real MCPB bundles locally**;
7. verify the Registry reports the intended semantic version and inspect its package types/transports;
8. verify discovery destinations independently where a stable supported read API or public listing exists; do not replace missing APIs with form scraping;
9. report a concise status summary covering GitHub Release, signed provenance, Registry version/package types/transports, Smithery, Glama, MCP.Directory, PulseMCP, and OpenSSF Scorecard publication status. Distinguish `not configured`, `not listed`, `stale`, `unverifiable through a supported API`, and actual publication failures;
10. record publication evidence in durable public history only where useful; keep workstation/runtime state private.

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
- [ ] the exact pushed commit's **full-tier** `Release candidate` job is `success`; no pull-request tier result is substituted for it;
- [ ] the exact pushed commit has a successful OpenSSF Scorecard `push` run on the default `main` branch.

After tagging:

- [ ] Release workflow verifies the annotated tag and exact Test Suite-gated commit;
- [ ] GoReleaser publication succeeds;
- [ ] signed Sigstore SLSA build provenance is generated for the normal release assets, persisted as a GitHub attestation, cryptographically verified against the release assets and source, and attached as the original `.sigstore.json` bundle alongside its unchanged `.intoto.jsonl` DSSE envelope;
- [ ] GitHub-only MCPB publication succeeds;
- [ ] GitHub-only Registry publication succeeds;
- [ ] normal GoReleaser assets independently match `checksums.txt`;
- [ ] semantic tag, changelog release, embedded binary version, and Registry version agree;
- [ ] Registry package types and transports match the actually published launch semantics;
- [ ] Scorecard and discovery/syndication destinations are reported separately from core release success.

Deployment, active rollback, restoration, launcher changes, and runtime restarts are **separate operator actions** and are not implied by this checklist.

## Project lineage

Scripthold remains an independent GPL-3.0 downstream fork of the [original `mcp-file-tools` project](https://github.com/dimitar-grigorov/mcp-file-tools). Upstream synchronization is not part of the publication procedure; see [PROJECT_DIRECTION.md](PROJECT_DIRECTION.md) for the maintenance boundary.
