# Scripthold Publishing Procedure

Use this procedure when preparing a public Scripthold release. It covers the required commit-level verification, protected GitHub publication, MCPB and Registry handoff, and post-publication checks. **Publishing a release does not update or restart an installed server.** The release must come from an exact clean commit that passes the full push-event qualification described below.

For product scope, planned work, and ordinary development checks, see [Project Direction](PROJECT_DIRECTION.md), [Roadmap](ROADMAP.md), and the [Development Checklist](DEVELOPMENT_CHECKLIST.md).

## Find the release step

- **Before publishing:** [release ownership and required checks](#release-ownership-boundaries), [release flow](#release-flow).
- **Validating artifacts and metadata:** [immutable pipeline rules](#immutable-release-pipeline-prevention-requirements), [Registry and release metadata](#registry-and-release-metadata), [validation toolchain](#validation-toolchain).
- **After publishing:** [post-publication verification](#post-publication-verification) and the [release checklist](#release-verification-checklist).
- **Repository governance:** [OpenSSF and branch protection](#repository-governance-and-openssf-best-practices).

## Release state sources

This procedure is intentionally version-neutral. Current public release details belong in [CHANGELOG.md](../CHANGELOG.md), GitHub Releases, and the MCP Registry rather than being copied into a maintainer runbook after every publication.

The Registry identity is `io.github.zoster81/scripthold`. Historical releases must not be changed or reused. The active draft-first release workflow requires GitHub release immutability to be enabled and checks that setting before publication. Publication and deployment are separate: a successful GitHub Release or Registry publication does not imply that any private or production runtime was upgraded.

## Release ownership boundaries

A public release is created only from an exact clean commit that has passed the complete **full-tier** push-event `Test Suite` `Release candidate` gate. Pull-request evidence tiers are review acceleration only and never publication authority.

Local maintainers may prepare source, run tests, build ordinary release-candidate binaries, and run deterministic GoReleaser snapshots. They must not create or simulate GitHub-owned MCPB release outputs.

The following actions remain explicit maintainer decisions and are never incidental side effects of validation:

- commit and push;
- annotated release tag creation;
- GitHub Release publication;
- deployment, active rollback, or runtime restart;
- toolchain upgrades.

## Linux publishing runner and temporary storage

The Release, MCPB packaging, Registry publication, and OpenSSF Scorecard workflows use the explicit GitHub-hosted `ubuntu-26.04` image, rather than the floating `ubuntu-latest` label. The release, MCPB, and Registry jobs check that the runner-provided temporary directory is writable and backed by an accepted disk filesystem (`ext4`, `xfs`, `btrfs`, or `zfs`). They then set `TMPDIR` and `GOTMPDIR` for subsequent steps through `GITHUB_ENV`. An unexpected or RAM-backed runner temporary filesystem causes a fail-closed error before heavy build or packaging work.

This preflight avoids the Ubuntu 26.04 RAM-backed `/tmp` quota for tools that respect these environment variables. It does not remount `/tmp`, control tools that ignore the variables, or replace exact-commit, tag, checksum, provenance, MCPB, or Registry release verification. The full publication chain must still be qualified through an authorized genuine release; passing non-publishing GoReleaser builds or synthetic artifact-transfer tests is not that evidence.

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
- Generate genuine GitHub OIDC/Sigstore SLSA attestations for every release's normal assets and verify the signature, subject and checksum relationship independently. Publish the original unchanged signed DSSE envelope as an in-toto JSONL companion only after verifying the Sigstore bundle, exact source and all release checksums. The JSONL reuses the original signature and requires its Sigstore bundle for certificate/transparency evidence; it is not a new signature or a renamed bundle. Never retrofit unearned attestations to historical builds.
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
9. GoReleaser builds the six raw binaries, six platform archives and `checksums.txt` without publishing a GitHub Release. The tagged producer authenticates the original bytes and stages all 13 originals for the private raw-artifact transfer.
10. The producer generates genuine signed Sigstore SLSA provenance for the 12 binary/archive subjects, retains the original `.sigstore.json` bundle and the same signed DSSE `.intoto.jsonl` envelope, and sends both companions with the 13 files. The separate GitHub-only MCPB consumer independently verifies the signed 12-subject evidence before preparing six packages and `mcpb-checksums.txt`; JSONL alone is not a substitute for the original Sigstore bundle.
11. The unprivileged `verify-complete` job and the independently protected `finalize` job each authenticate the resulting 22 original files at their own runner boundaries. After the maintainer's environment approval, the finalizer alone may create or safely resume one unpublished draft and make one immutable public transition after verifying all 22 assets. Only after confirmed immutable publication may the Registry workflow run its read-only prerequisite and OIDC publisher.

**The protected single-writer draft-first pipeline is the current publication path.** Each future release still requires independent exact-commit CI, protected approval, verification of all 22 original assets, immutable GitHub publication, and Registry readback. Do not reuse a published version or alter consumed tags or historic incomplete drafts. The original v3.3.3 publication evidence belongs in the [Changelog](../CHANGELOG.md#333---2026-10-23), not in this version-neutral runbook.

The tag-triggered Release workflow attests the already-completed exact-commit Test Suite and OpenSSF Scorecard push evidence instead of rerunning the full expensive race/static/vulnerability/fuzz/cross-build/container matrix before GoReleaser. The Test Suite push gate already includes Gitleaks and `goreleaser check` in addition to the full native/platform/security evidence. Scorecard itself remains on its supported default-branch execution path; release tags do not invoke the Scorecard action directly.

## Immutable-release pipeline: prevention requirements

The active release pipeline prepares all 22 original assets privately, verifies each trust boundary, and publishes through one protected, draft-first GitHub Release writer. Former competing post-publication asset writers are retired. The requirements below prevent post-publication mutation, unsafe temporary-path aliases, dependency/version mismatches, and incorrect MCPB package types.

### Preserve release authority and output semantics

- Require an annotated release tag on the exact current `main` commit, a successful **full-tier push-event** Test Suite `Release candidate` for that SHA, and a successful Scorecard **main-branch push** run for the same SHA. Scorecard does not support release-tag execution. A passing PR, a skipped required job, a different commit, or a rerun selected only by a mutable name is insufficient. Independently resolve the annotated Git tag to its commit; GitHub Release `target_commitish` is not source evidence because GitHub ignores that field when the tag already exists.
- Keep GoReleaser, Node, the MCPB CLI, publishing tools, and external Actions pinned and compatible. Update every `github/codeql-action` component together; mixed versions can fail even when the individual action references are valid. Do not combine speculative toolchain upgrades with the pipeline migration.
- Qualify the exact GoReleaser version with publication skipped, not snapshot semantics. Preserve the six original binaries, six platform archives, checksum list, embedded version, archive contents, timestamps/modes, release notes from GoReleaser, and existing title/prerelease behavior. GoReleaser v2.18.1 writes the exact generated release-note bytes to `dist/CHANGELOG.md` in the tagged producer; transmit those bytes separately as size- and SHA-256-checked, run-bound private metadata. They are **not** a 23rd release asset. Never regenerate the notes in a consumer job, substitute GitHub-generated notes, or infer trust from a producer-local file path. Archive/checksum paths differ from raw binary build paths; validate each type inside its producer, then transmit **no producer-local path** to another job.
- Preserve the genuine Sigstore/SLSA attestation for the **12 normal binary/archive subjects**. Publish its verified original bundle and signed in-toto envelope without fabricating subjects or signatures for later MCPB bundles.
- Keep the original tagged Release workflow's **certificate signer identity** as a separate trust boundary from the signed statement's declared source and builder fields. GitHub CLI [verifies the signing workflow identity](https://cli.github.com/manual/gh_attestation_verify); when a reusable workflow generates the attestation, that reusable workflow is the signer, not its caller. A PR, manual-dispatch, or reusable-workflow signing rehearsal cannot silently stand in for `.github/workflows/release.yml@refs/tags/<tag>` under the current exact `--cert-identity` policy. Any signer-workflow migration requires explicit approval, a reviewed identity policy, and real hosted cryptographic verification; do not widen the accepted certificate identity to make a nonpublishing proof pass.

### Keep host paths and file identities separate

The filesystem of one GitHub job is not the filesystem of another. In particular, macOS may expose the same trusted temporary directory as both `/var/folders/...` and `/private/var/folders/...`, and Windows may expose a directory through a short 8.3 alias such as `RUNNER~1`. The isolated, non-publishing raw-artifact transfer probe is pinned to `ubuntu-26.04` for all three jobs on pull requests and manual runs, with no 24.04 fallback.

Qualify the 22-file cross-job transport on a real GitHub-hosted Ubuntu 26.04 runner rather than inferring compatibility from local Fedora CoreOS/Alpine testing or from `ubuntu-latest`. When changing production workflows, independently qualify their GoReleaser build, security checks, and filesystem/scratch-space behavior on 26.04. Ubuntu 26.04 may mount `/tmp` as a quota-limited RAM-backed filesystem: for large GoReleaser builds, measure scratch capacity and place large build temporaries on an independently checked, owner-controlled, disk-backed workspace when necessary.

Preserve path ownership, absolute-path confinement and portable original byte identities; do not replace a capacity error with an unverified temporary-directory alias. The producer-to-consumer manifest must carry only exact allowlisted ASCII asset names, sizes and SHA-256 hashes, plus authenticated repository, source commit, workflow run, producer attempt, and numeric GitHub artifact IDs. Never transfer `/tmp`, `/var/tmp`, `$RUNNER_TEMP`, Windows/UNC paths, or GoReleaser build paths as asset authority.

Never repair an untrusted name using `basename`, case conversion, Unicode normalization, or separator replacement; reject a noncanonical input.

The non-publishing transfer probe also includes a Linux-only Go check that takes the **15 normal synthetic files actually downloaded by GitHub Actions** and copies them into a separate private directory through `releasearchive.StageRawNormalHandoff`, verifying the full inventory and unchanged original ZIP bytes. Its independent test oracle supplies the expected content without treating a producer manifest as its own proof. This is a genuine physical cross-job staging check on the hosted runner, but **not** a test of authentic GoReleaser provenance, genuine MCPB packaging, or final release publication.

Within each Linux staging job, use fresh, private, single-owner destinations and verified ordinary-file identities. Resolve and authorize the intended directory boundary before access, prevent symlink/hardlink and rename races, verify actual streamed bytes, and fail without overwriting existing files when an input changes. Do not reject a legitimate platform-provided temporary-directory parent solely because its *spelling* includes a system alias; distinguish trusted ancestor resolution from an untrusted leaf. Executable modes and MCPB layout must come from the verified release/packaging contract, not from permissions assumed to survive GitHub artifact transfer.

### Transfer complete typed inventories before publication

Use the links below to inspect one release trust boundary at a time:

- [Asset inventory and writer authority](#asset-inventory-and-writer-authority)
- [Source identity and immutable publication](#source-identity-and-immutable-publication)
- [Original-byte transfer and verification](#original-byte-transfer-and-verification)
- [Optional Go producer staging](#optional-go-producer-staging)
- [Active provenance and consumer checks](#active-provenance-and-consumer-checks)
- [Finalizer observation and recovery](#finalizer-observation-and-recovery)
- [Registry publication prerequisite](#registry-publication-prerequisite)

#### Asset inventory and writer authority

The active release flow separates the unprivileged normal-asset producer, the GitHub-only MCPB producer, and the narrowly privileged finalizer. Each stage has its own independently verified asset allowlist:

- **15 normal files:** six raw binaries, six platform archives, `checksums.txt`, and two authentic provenance companions.
- **7 MCPB files:** six real, GitHub-produced platform-specific `.mcpb` packages and `mcpb-checksums.txt`.
- **22 public release files:** only the verified combination of both inventories. Release notes and GoReleaser internal metadata are authenticated transfer inputs, **not extra public assets**.

When using the pinned raw-file GitHub artifact actions, set `upload-artifact` to `archive: false`, `if-no-files-found: error`, and `overwrite: false`. A raw upload uses the **physical filename**, not its configured artifact `name`; names must be deterministic, attempt-specific, and mapped back to the original allowlisted asset by trusted metadata. For `download-artifact`, select authenticated numeric `artifact-ids`, use a fresh private destination, `skip-decompress: true`, and `digest-mismatch: error`. This matters especially for original `.zip` release files, which must remain byte-identical files rather than being automatically extracted. Independently check that **every** requested ID and all 22 byte streams arrived: action success or server-reported digest alone is not sufficient.

The builder and packaging jobs must not receive `contents: write`. The finalizer alone may create an unpublished GitHub **draft**, resume only exactly matching pre-existing draft assets, verify the remote 22-file set against original size/digest and source identity, then perform the single public-publish transition. A partial upload, conflicting asset, stale attempt, duplicate job, or failed remote verification must leave the release unpublished; never overwrite a published release. Disable or repurpose any independent manual post-publication upload/repair entrypoints before enabling immutability. Publish Registry metadata through the existing separate OIDC job **after** the verified public release, with idempotent error handling; a Registry outage must not trigger mutation of an immutable GitHub release.

Verify the 12 genuine Sigstore signatures once in the mandatory successful MCPB consumer, before packaging. The dependent finalizer checks the verified producer jobs and all 22 original byte digests; it must not repeat 12 expensive cryptographic CLI verifications that the successful upstream job has already performed. Neither stage accepts unsigned or modified assets.

The merged **read-only `verify-complete` job** checks the draft-writer input contract after authenticating and separating all 22 original bytes. It requires exactly 15 normal and seven MCPB originals with matching names and sizes in private, real directories, rejecting unexpected entries. It also validates GitHub's built-in event, repository, tag ref, workflow ref, commit, **`verify-complete` job identity** and run/attempt against the same fixed tagged-source rules used by the protected writer, whose required job identity remains `finalize`.

A successful verifier is only upstream evidence: **no local files or write permission cross into the separate protected `finalize` job**. The protected job now independently downloads the same 22 original artifact IDs into its own fresh owner-private staging directory, rechecks the producing jobs and run/attempt, remeasures the original bytes and verifies source/CI authority using the shared **read-only** script. This necessary repeated verification occurs at a distinct runner trust boundary, not twice in one job.

The protected writer must refresh publication and environment permissions before each remote mutation. Its release-mutation calls are connected behind environment approval and immutable-policy checks, but no authentic tagged execution or public transition has occurred.

The protected draft writer also binds the release **title, complete GoReleaser notes and prerelease flag** to the authenticated source metadata. Creation and each resumed draft upload reject a missing or altered field, including changes between the two authenticated release reads and the pre-upload recheck. Notes metadata must have the original producer's canonical size/SHA-256/base64 representation; arbitrary free-form notes are not an upload input. These checks do not authenticate the producing GitHub job by themselves, and the writer is called only from the protected `finalize` step; GitHub immutable-release enforcement is still disabled and no genuine tagged publication has been qualified.

The protected public transition is deliberately separate from preparing a draft. The protected `publishCompleteDraft` function requires an existing unpublished draft containing all 22 authenticated original asset names, sizes and GitHub-reported digests; it rechecks the exact tagged source, successful main-push release authority, original title/notes/prerelease state and repository immutability policy immediately before making **one** non-retryable draft-to-public `PATCH`.

It uses a separate administrator-read token only to inspect immutable-release enforcement, not to enable it. Supplying the same credential for both release writes and administration reads is rejected **before any GitHub API request**; configure independently scoped credentials and never give administrative write permissions to the publisher. A successful transition additionally requires GitHub to report `immutable: true` and an independent readback of the published 22-asset inventory.

Unknown responses, changed policy, incomplete assets and any ambiguity stop; an uncertain `PATCH` never authorizes an automatic retry. The protected `finalizeCompleteRelease` coordinator now checks both distinct credentials, the complete private 15+7-file source inventory and the enabled immutable-release setting **before the first draft creation or upload**.

It then prepares exactly one fully authenticated 22-asset draft and passes that draft's verified numeric release ID to the one-shot publication function; changed draft identity, ambiguous uploads and uncertain publication stop without automatic retry. This coordinator additionally rejects a supplied GitHub Actions context unless it describes a tag-triggered `push` of the exact first-party `release.yml` workflow, repository, tagged source commit, `finalize` job and positive run/attempt identifiers, **before any GitHub request**.

Those environment fields are merely defensive validation, not proof of the runner's identity or protected environment: the activated workflow must supply them from trusted GitHub contexts and enforce the environment approval and per-tag lock independently. Before any future draft write, the coordinator also uses the **separate Administration:read credential** to independently retrieve the protected finalizer environment and its complete deployment-ref policy.

It fails closed unless GitHub confirms the exact required maintainer, no administrator approval bypass, and one tag-only `v*` rule; it checks the same policy again after preparing the draft and before the public transition. Missing permissions, changed protection, unexpected reviewers or branch rules halt publication. These API reads are point-in-time defense in depth, **not evidence that a tagged job obtained GitHub approval**. Any change to this coordinator still requires fresh protected tagged-run qualification.

The workflow, not the coordinator alone, must still enforce the concurrency lock, environment approval and repository write authority.

Before every immutable promotion, the maintainer must independently enable and verify GitHub's repository-level **immutable releases** setting using administrative access. The authenticated `GET /repos/zoster81/scripthold/immutable-releases` response must explicitly report `enabled: true`; a `403`, missing setting, or false value is **not** publication authority. Do not assume that the release job's standard `GITHUB_TOKEN` can read this administrative endpoint. Keep the finalizer's fail-closed publication barrier in place; never re-enable competing public asset writers or waive authenticated 22-file verification. A complete draft alone is not yet an immutable public release.

The protected finalizer's `readImmutableReleasePolicy` reader checks this exact administrative GitHub endpoint with a **separately supplied administrator token**, rejects missing, false, unauthorized, redirected, malformed, or oversized responses, and returns only a positive `enabled: true` observation. [GitHub's REST permission contract](https://docs.github.com/en/rest/repos/repos#check-if-immutable-releases-are-enabled-for-a-repository) requires repository **Administration: read** for this GET; a repository-scoped GitHub App installation token or fine-grained personal access token with only that permission can supply the separate read authority. Do not give the finalizer an administration-write credential or expose this token to producer jobs. A disabled setting may appear as a 404 or an explicit false response; both block publication. The reader neither changes the setting nor grants durable publication authority. The protected finalizer must recheck the setting before the public transition and must not treat the ordinary workflow token as administrative proof.

A lost response to the final publish request is **not** permission to publish again blindly. Fetch the release and complete remote asset list through authenticated GitHub APIs. The read-only `verifyPublishedAssetSet` contract may recognize an already published **immutable** release only when the same exact 22 names, sizes, upload states, IDs, and SHA-256 digests match the staged manifest. The existing `verifyDraftAssetSet` retains its draft-only requirement for the prepublish gate. If the API still reports a draft, partial inventory, mutable public release, or conflicting data, stop and re-establish the current remote state before taking any action; never repair a public release by overwriting assets. Neither validator performs remote writes or substitutes for independent tag/run/source verification.

The protected finalizer's `readVerifiedImmutablePublication` reader retrieves a release, its complete bounded asset list, and the release again through authenticated GitHub GET requests. It applies the same exact 22-file immutable-publication contract and rejects changed or incomplete observations. This is **postpublication read-only evidence**, not a replacement for original-byte verification, trusted producer/run provenance, the administrative immutability precondition, or a safe single-writer publication operation. The protected finalizer invokes these readers in the current tagged release workflow; readback remains a point-in-time observation.

#### Source identity and immutable publication

Pin the **publisher and packaging code revision** as well as the input asset commit. The merged reusable MCPB consumer rejects any mismatch between the requested version, the triggering release tag, both checked-out source commits and GitHub's tagged event commit **before executing tag-owned validation code or consuming signed files**. Both of its checkouts now disable persisted Git credentials. This source-qualified guard does not establish genuine signed-15 consumer or six-package hosted qualification. Registry manual dispatch independently authenticates the immutable release before running tag-owned code; its initial policy verifier intentionally remains workflow-owned. Reusable workflows and manual dispatch must not silently run mutable `main` scripts against a different tag without an explicit, verified code-identity contract. Serialize competing publication entrypoints for the same tag.

The offline `validatePublicationAuthoritySnapshot` check binds the complete 22-file staging manifest to an annotated Git tag object, the exact current `main` commit, the checked-out publisher commit, successful exact-commit **main-push** Test Suite and Scorecard runs, and a completed `Release candidate` job from the selected Test Suite run **and attempt**. It accepts authenticated, repository-scoped API responses as inputs; it does **not** authenticate how those responses were fetched, lock concurrent publishers, or grant publication permission. The protected finalizer must fetch this evidence independently, recheck authoritative refs and draft assets immediately before each state-changing request, and fail closed if any evidence changes. GitHub REST job metadata (including `run_attempt`) is required; a projection that omits fields is not suitable as publication authority.

The read-only `readPublicationAuthority` helper now fetches those inputs itself through fixed, authenticated GitHub REST GET endpoints. It reads the annotated tag reference and Git tag object, current `main` reference, separately filtered exact-commit Test Suite and Scorecard main-push runs, and jobs for the selected Test Suite run **and attempt**, then passes them to the same strict offline validator.

Every JSON response is size-limited; any result with an incomplete page, more than 100 items, extra `Link` pagination, an incongruent run/job, or mismatched source fails closed. The protected finalizer job retains a read-only `GITHUB_TOKEN` with **Actions: read** and **Contents: read**; an independently scoped, short-lived GitHub App token provides **Contents: write** only for draft/release mutations. Producer jobs have no release-write authority.

The returned verdict is evidence from one series of network reads, **not a lock, a durable permit, a cryptographic proof of a protected tag, or permission to publish**. A publisher must bind its checkout to the exact commit, serialize tag operations and refresh this evidence immediately before each mutation; a changed source or competing publisher requires aborting, not proceeding on a cached result. The current draft-first workflow applies this fresh-evidence boundary to every attempted publication.

#### Original-byte transfer and verification

The offline `assembleRawReleaseStaging` producer-evidence adapter accepts two independently validated 15/7-file stage manifests and their explicit numeric run, producer attempt, repository and upload identities. It checks each original filename, size and SHA-256 against its producer manifest and the supplied GitHub Actions REST artifact observation, requires the exact common run/repository and consistent attempt within each stage, and returns only the canonical 22-file manifest plus the pinned raw-download ID plan. The producer records cannot contain host paths or extra authority fields. The caller must independently authenticate the REST fetches, verify the source workflow and producer job/attempt, open a private no-follow download destination, check all 22 downloaded byte streams, and reacquire publication authority before any later write. This adapter does **not** perform network I/O, acquire a concurrency lock, authorize uploads, or publish releases. The active tagged pipeline uses this evidence boundary before the private 22-file transfer.

The separate read-only `assembleRawNormalHandoff` and `verifyRawNormalHandoffPayloads` gates support the prerequisite transfer of **exactly 15 normal files** into the GitHub-only MCPB build job. They reuse the same authenticated artifact ID, source/run/attempt, exact filename, size, SHA-256 and raw multi-download validation rules; the payload gate rejects incomplete or unsafe flat-directory metadata before opening any source, then streams and hashes all 15 originals.

The shared stream verifier for these transfers, archived staging inputs and prepared MCPB binaries rejects 100 consecutive zero-length byte chunks while allowing intermittent empty chunks; it cannot interrupt an asynchronous read that never resolves. For both the 15-file and complete 22-file gates, the expected size and SHA-256 are pinned before asynchronous source opening; the opener receives a separate metadata copy and cannot change the byte-verification authority or returned inventory.

The caller must provide stable no-follow, single-owner file handles from an isolated destination; these helpers cannot establish filesystem ownership or prove that a later consumer sees the same bytes. The normal-only manifest never satisfies the **unchanged 22-file finalizer publication gate**. The merged GitHub workflow connects the corresponding normal handoff and final 22-file verification boundaries, but a synthetic transfer probe still cannot qualify genuine signed-15 ingestion or GitHub-built MCPB packages.

The merged MCPB consumer's read-only `verifyPreparedMcpbBinaryPayloads` gate checks the **six server binary byte streams after preparation** against the SHA-256 and byte lengths from the independently authenticated normal producer inventory. Its six exact platform stage/binary names are derived from the canonical normal release allowlist, not from a packaging directory listing. Expected values are detached before the first asynchronous file open so a caller cannot rewrite later expected digests. The GitHub packaging job must supply stable no-follow, single-owner handles to the actual prepared `server/` copies, then separately verify the genuine packaged MCPB results; this helper neither opens files itself nor creates, validates, or publishes a package. The helper does not publish an MCPB or replace the separate GitHub-only packaging step.

**Optional Go consumer staging (not wired into release publication):** `releasearchive.StageRawNormalHandoff` accepts 15 physical downloads plus their canonical original names, sizes, and SHA-256 values. It requires separately trusted source evidence, checks the complete private download directory, makes exclusive verified copies in a different private directory, and rejects invalid, duplicate, relative, or overlapping paths. See [the implementation](../internal/releasearchive/normal_handoff.go).

The caller remains responsible for authenticating the GitHub source jobs and canonical 15-name allowlist. The separate read-only JavaScript `planRawNormalPrivateStage` helper derives the exact `{downloadName, original: {name, size, sha256}}` records expected by Go from the independently validated 15-file artifact-ID batch; it neither downloads nor opens bytes and is not itself proof of file integrity.

The accompanying optional Go helpers have distinct responsibilities: `DecodeRawNormalHandoffPlan` strictly parses exactly 15 records within a 16 KiB limit, rejecting malformed or duplicate JSON fields, names, sizes, and digests; `BindRawNormalHandoffPlan` requires a separately authenticated complete asset inventory before staging. Neither parsing nor binding authenticates GitHub jobs, artifact IDs, or downloaded bytes. The caller must establish those authorities independently.

These Linux-only helpers are not an active release path. Copy/verification failures can leave an explicitly **non-publishable partial stage** requiring identity-safe cleanup, and same-user file races remain an external constraint. A GitHub-hosted Ubuntu 26.04 synthetic 15-file transfer, including an unchanged ZIP, qualified the isolated staging behavior—not genuine Sigstore signing or MCPB packaging.

The merged producer and consumer's read-only `bindRawUploadJobOutputs` adapter turns the reviewed producer job's exact ordered `upload-artifact` step outputs (`ids`, `digests`, `attempt`) into the canonical 15-normal or 7-MCPB source input. It rejects missing, extra, repeated, nondecimal, unsafe or oversized numeric IDs; noncanonical or mismatched original SHA-256 digests; stale/noncanonical attempts; wrong stage inventory; and oversized output vectors. File-to-ID positions follow the fixed release asset allowlist, never caller-supplied asset enumeration order.

It copies the resulting metadata so later caller mutation cannot change the binding. This parser does **not** authenticate the origin of these claimed job outputs, prove that the selected job uploaded those IDs, verify signatures, or open/download bytes. The trusted tagged workflow passes outputs directly from its fixed pinned upload steps, authenticates that producer job and attempt independently, rechecks each numeric artifact ID using GitHub REST, and verifies every received original byte.

The active tagged producer and consumer use this adapter; it cannot authenticate its input without independent job evidence.

The read-only `readRawNormalStaging` and `readRawCompleteStaging` adapters obtain each producer's GitHub Actions raw artifact metadata through the fixed, authenticated `GET /repos/zoster81/scripthold/actions/artifacts/{id}` endpoint. They accept only complete 15- or 15+7-item producer inputs with positive numeric repository/run/attempt/artifact IDs, exact source/tag/filename/digest and unique IDs, rejecting inconsistent inventories before issuing even one GET. Every bounded nonredirecting JSON response is reduced to its identity and workflow fields, then revalidated by the shared stage and final raw transfer contracts. They never discover an artifact by mutable name or latest run; they neither download file bytes nor authorize any release mutation. Every caller must independently establish that the supplying workflow jobs and attempts are trustworthy, keep the download destination private, verify every original byte and regain authority immediately before a write. The authenticated REST read is an observation, not an atomic GitHub snapshot or a release lock.

The read-only `verifyNormalChecksumClosure` check compares the **original `checksums.txt` bytes** against the checksum-file size and SHA-256 in the independently authenticated 15-file normal inventory. It uses the existing strict checksum parser and requires exactly the 12 canonical binary/archive subjects, each agreeing with the corresponding independently pinned original digest. Changed bytes, missing or duplicate entries, foreign names, malformed records and oversized input fail closed. This check neither signs nor verifies Sigstore provenance, and the normal producer inventory is not an independent authority merely because it contains digests; the tagged GitHub producer must authenticate the source/run/attempt and all original bytes separately. The active production chain uses this check before staging the signed normal originals.

For a nonpublishing normal-producer rehearsal, maintainers can use the separately triggered `Ubuntu 26 normal release rehearsal (nonpublishing)` workflow to test an **existing annotated tag** on GitHub-hosted Ubuntu 26.04. Run it manually from `main` with the tag as input; it independently checks the remote annotated tag object and source commit, builds all six binaries and six archives with the production-pinned GoReleaser in `release --clean --skip=publish,announce` mode, and checks the exact 12 normal checksum subjects against the original output bytes. It also checks a disk-backed build scratch area. The job has only `contents: read`, persists no checkout credentials, uploads nothing, and performs no signing, MCPB packaging, Registry operation, tag creation, release publication or active workflow replacement. A completed rehearsal proves only **13 local normal files** on that hosted runner, not the 15-file signed normal inventory, 22-file finalizer, or actual immutable release. Always use the remote tag identity: a similarly named local tag may point to different history.

The merged normal producer's `measureNormalSourcePayloads` helper prepares the producer-side original inventory from an independently enumerated, owner-controlled flat directory of **exactly 15 files**, before any cross-job upload. It rejects unexpected names, duplicates, non-regular files, multiple links, empty or oversized files (with tighter checksum and provenance limits), then streams and hashes each original and requires all **12** checksum entries to match the measured binary/archive bytes.

It binds the claimed tag and source commit to the resulting normal-stage manifest, without including any producer-local path. The byte-stream reader tolerates intermittent empty chunks but rejects 100 consecutive zero-length chunks without progress, matching the Go release-archive reader's resource boundary; this does not interrupt a source whose asynchronous read never returns.

The calling job must protect the directory and file handles against symlink/replacement races and upload the same verified bytes; this helper cannot establish those operating-system guarantees, authenticate GitHub workflow identity, verify a Sigstore signature, or authorize publication. The active tagged GoReleaser producer invokes this helper before verifying the authentic provenance pair and handing its files to downstream jobs.

A complete 15-file original inventory is not proof of signed release content: a replaced binary, rewritten `checksums.txt`, and new asset hashes can all agree with each other. The future tagged producer preserves the unchanged signed DSSE envelope using `projectBundle` and checks `checkNormalProvenancePair` against the **same measured** originals before private raw upload. These are structural checks, not signature verification. The mandatory successful MCPB consumer authenticates the producing job and verifies the 12 signatures with genuine GitHub CLI against the 15 **downloaded** originals exactly once, before packaging. The finalizer later checks the exact received bytes instead of repeating that signature verification.

The merged normal producer's pure `planOriginalGoReleaserPaths` selector chooses the **13 public normal GoReleaser outputs** (six binaries, six archives and `checksums.txt`) from the pinned producer's larger `artifacts.json` inventory. GoReleaser also records intermediate binaries and metadata, which must never be mistaken for release assets. The selector demands exact canonical names, producer types, target platforms and local layouts: binaries originate in target-specific build subdirectories, while archives and checksums are flat under `dist`.

It rejects missing, duplicated, case-aliased and substituted output paths and returns only a local, untrusted-to-other-jobs source mapping. The checksums are checked against the exact 12 public subjects, but this pure selector does **not** open files, establish binary sizes, authenticate producer identity or signatures, or create staging files. A future trusted Linux producer must independently open and authorize each physical original, verify its exact size and bytes, and use the existing safe Go staging primitives before any attempt-scoped raw upload.

#### Optional Go producer staging

**Optional Go producer staging (not wired into release publication):** the `releasearchive` source-binding and rooted-copy helpers can verify and privately stage the 15 normal originals, including the genuine Sigstore bundle and signed JSONL kept outside GoReleaser's `dist` tree. The sidecar variant accepts independently trusted source roots and preserves exact signed bytes without rewriting or resigning them. Source and staging trees must remain disjoint where required, and a failed late copy leaves a **non-publishable partial stage** to clean up safely. See [the implementation](../internal/releasearchive/producer_handoff.go).

These helpers do not authenticate the Sigstore signer, GitHub workflow/run/attempt, upload artifact IDs, or publisher authority. The active GoReleaser, provenance, MCPB, and Registry workflows do not invoke them; existing exact-run, original-byte, cryptographic, and publication gates remain mandatory.

#### Active provenance and consumer checks

The merged Linux MCPB consumer's `verifyStagedNormalAttestations` helper is used for a **separate, owner-private 15-file download stage with original filenames**, after raw transfer has restored the original names.

It measures each original with a no-follow file descriptor, checks the 12 checksummed subjects and unchanged Sigstore bundle/in-toto JSONL together, then invokes the existing genuine `gh attestation verify` implementation on every one of the 12 original subject paths with fixed repository, release workflow certificate identity, source tag/commit, and hosted-runner restriction. A failed external signature verification rejects the entire result; this verifier has **no injectable production signature-verification callback**.

It measures all 15 originals again after the CLI checks and reports only the count of cryptographically verified subjects. The active tagged consumer verifies these attestations before GitHub-only MCPB packaging. The caller must independently authenticate the GitHub source job/run/attempt and numeric artifact IDs, provide an owner-controlled private stage, and prevent same-user concurrent writers: measuring original bytes before and after separate pathname-based CLI calls is **not an atomic filesystem snapshot or an uploader identity proof**.

Windows test fixtures exercise only the structural preflight; successful verification against genuine signed release evidence was qualified during the v3.3.3 hosted Linux consumer. Its directory inventory check now reads at most 16 entries for the required 15 names, avoiding an unbounded whole-directory listing; this is a resource bound, not an atomic snapshot of a directory being concurrently modified.

The merged normal producer's `checkNormalProvenancePair` helper checks the **two original provenance companions** against the complete 15-file normal inventory and original 12-subject checksum file before raw handoff. It rejects missing, modified, oversized, or mismatched Sigstore bundle/in-toto JSONL bytes; reconstructs the exact unchanged signed DSSE envelope; and reuses the existing strict source-commit, tag, 12-subject and `.github/workflows/release.yml` builder-identity checks. This is structural consistency only: synthetic DSSE signatures can satisfy it. The **GitHub MCPB consumer** performs real GitHub CLI attestation verification of every downloaded subject after authenticating the producer job/run/attempt and original bytes. The producer does not redundantly verify its own newly signed files. The `checkNormalProvenancePair` helper neither registers attestations nor creates signed artifacts; the active tagged workflow uses these boundaries.

The separate read-only `readJobVerifiedRawNormalStaging` and `readJobVerifiedRawCompleteStaging` entrypoints add an independent GitHub source-job check **before** those raw artifact reads. Their `sourcePolicy` must come from reviewed, trusted workflow code (numeric workflow ID, exact workflow file path, event and distinct expected job names), **never from a producer manifest or job output**.

They read the exact workflow run and its 1 or 2 numbered-attempt job lists from authenticated, nonredirecting and bounded REST endpoints, require successful job IDs bound to the expected source SHA, run and attempt, reject incomplete/paginated lists, then reread the run to detect an intervening rerun. The normal producer can be successful while the larger workflow is still in progress; a completed failed workflow is rejected.

The job checks do **not** establish that a particular job uploaded each artifact: GitHub's artifact-by-ID response identifies the workflow run but contains no uploader job ID. The validated producer inventory and trusted workflow policy are copied before asynchronous REST reads, so later caller mutations cannot change which run, job or artifact IDs are checked. The trusted workflow still needs a separate secure job-to-upload-output contract, original byte verification and a fresh publication-authority check.

These entrypoints do not mutate GitHub, are not used by active release workflows, and cannot authorize publishing or prove an atomic snapshot.

#### Finalizer observation and recovery

The offline `planReleaseFinalizerStep` offers **non-executable proposals**, never authorization to mutate GitHub. It first verifies the same publication evidence, then distinguishes unknown/ambiguous release lookup (`inspect-absence` or halt), a verified draft with missing assets (one canonical `upload-candidate`), a complete draft (`publish-candidate`), and a fully checked immutable release (`complete`). After a lost publish response, only a freshly checked published immutable release is recognized as complete; an apparently unchanged draft must halt rather than blindly retry. Uncertain uploads may advance only if a fresh remote listing independently contains the exact asset. None of these proposals acquires a lock or prevents another writer. The protected workflow must enforce same-tag concurrency, authenticate API responses, and refresh the source, release, and asset inventory before **every** remote write.

For that same-tag boundary, use the **same canonical tag-derived concurrency group across tag-triggered and manual finalizer entrypoints**; a group based on the workflow name or manual-dispatch branch can accidentally isolate competing writers. Set `cancel-in-progress: false` so an executing publisher is never interrupted. GitHub supports `concurrency.queue: max` (up to 100 pending runs), but the currently pinned `actionlint` v1.7.12 does **not** accept that key. Do not add it until a coordinated, pinned validator upgrade passes workflow and CI qualification. Without `queue: max`, GitHub replaces an older pending run when a newer one queues, even when `cancel-in-progress` is false; operators must not assume every retry executes or that dispatch order is preserved. Treat any canceled or superseded attempt as requiring a fresh authority/inventory check, not implicit publication success.

The read-only `resolveFinalizerEvent` check accepts only a canonical `push` release-tag ref or a `workflow_dispatch` started from `main` with explicit `inputs.tag`; it rejects other events, branch/tag confusion and foreign repositories. Both forms resolve to the same `scripthold-readonly-preflight-<tag>` group. The read-only `inspectFinalizerPreflight` checks that group and exact manifest tag, reads authenticated source/CI authority, discovers an existing release through the bounded read-only listing, and rereads authority. The second check compares the actual annotated tag **object SHA** (not only the peeled commit), selected Test Suite run **and attempt**, Scorecard run and source identity. Changes or transport errors halt the preflight. The returned group is only a **naming contract**, not an acquired lock, and a discovered absence does **not** authorize release creation. This preflight never sends any mutating HTTP request.

The new `.github/workflows/release-finalizer-preflight.yml` is an intentionally **read-only** GitHub Actions entrypoint. It accepts version-tag pushes and manual requests, uses a native workflow-level `scripthold-readonly-preflight-<tag>` concurrency group across both entrypoints, and sets `cancel-in-progress: false`. After credential-free checkout and pinned Node setup, `release-finalizer-entrypoint.js` rejects invalid tags, other repositories, branch pushes, and manual requests not started from `main`.

Only the subsequent `release-finalizer-source.js` step receives the scoped read-only `GITHUB_TOKEN`; it fetches the annotated tag reference, Git tag object, and current `main` from fixed GitHub REST GET endpoints, requires each to identify the workflow checkout SHA, and rereads the tag reference to catch replacement. GitHub's source reads are separate requests, not an atomic snapshot.

The job has `contents: read` only: it does **not** call the complete manifest/CI/draft `inspectFinalizerPreflight`, download release assets, create drafts, upload, publish, or use a Registry credential. A pull-request YAML check is not evidence that the workflow has executed on GitHub; manual dispatch requires the workflow to be present on the default branch.

This read-only preflight group serializes **only runs that actually use it**; it is not the publication lock. The merged tagged `release.yml` uses the separate canonical `scripthold-immutable-release-<tag>` group, with its two former public asset writers retired together. Never add a second release writer or permit a manual repair path to bypass the publication lock; the read-only preflight must not acquire it. Recheck authenticated source, release and asset evidence after entering writer concurrency and immediately before **each** remote write. An in-process mutex, unprotected Git ref or polling active runs is not an atomic cross-run lock. GitHub does not promise FIFO ordering, and the default single pending run may be superseded. Preserve those recovery semantics rather than silently assuming that all dispatched attempts execute.

For release-asset inventory, the authenticated read-only `readReleaseObservation` adapter accepts an already established positive numeric release ID and a canonical tag. It uses pinned-version GitHub REST `GET` requests only: read the release by ID, fetch `/repos/zoster81/scripthold/releases/{release_id}/assets?per_page=100&page=1`, then read the release by ID again to reject intervening identity or draft-state changes.

It refuses redirects, unexpected origins, non-JSON or oversized responses (512 KiB per response), authentication failures, and asset pages larger than the 22-file contract. The separate manifest validator handles missing, renamed, or mismatched assets. The caller supplies the secret token; the adapter never creates, uploads, publishes or discovers a missing draft. `normalizeReleaseAssetPage` additionally rejects any pagination `Link` header, mismatched release ID or query, non-200 response, or more than 22 entries.

A 100-item page suffices for the fixed 22-file allowlist; silently concatenating, truncating, or ignoring pages is forbidden. A 404 produces only the untrusted `inspect-absence` planning state, **not** permission to create.

The read-only `discoverReleaseByTag` helper can now scan up to 20 numbered pages of 30 releases each using fixed repository-scoped authenticated REST `GET /releases?per_page=30&page=N` URLs, with bounded UTF-8 response sizes and strict validation of all supplied pagination links. It rejects invalid/mixed identities, repeated numeric IDs, duplicate exact tags, incomplete scans, foreign next links and changes between a matching list entry and the subsequent independent by-ID read. Finding one valid candidate is a recovery observation, never permission to modify it.

Finding no tag returns `not-found-unproven`, **not** proof that no draft exists: GitHub documents draft listing as available only to users with repository push access, and multi-page reads are not atomic. The finalizer must check effective authorization, serialize same-tag writers, and recheck source, list, release and asset state before every mutation. In particular, do not use the published-release-by-tag endpoint to infer draft absence.

The protected finalizer invokes the required reader and recovery checks only inside the tagged Release job. Any uncertain-write recovery requires fresh evidence of the exact source, run/attempt, and remote draft state; a previous successful publication does not qualify it.

#### Registry publication prerequisite

**Registry read-only prerequisite:** the merged `publish-registry.yml` rejects either `workflow_call` or manual Registry publication unless GitHub exposes the exact tagged source commit as an already published immutable release. Before Registry OIDC, it uses the ordinary read-only workflow token to independently read the release by tag and stable ID, require an unpaginated complete set of all 22 original asset names and distinct numeric IDs, and check positive sizes, SHA-256 digest metadata and uploaded states.

The first preflight executes only workflow-owned trusted code and authenticates the immutable release **before executing any script from the tag checkout**. The Registry manifest generator then runs from that exact tagged source, so a later manual rerun cannot silently substitute `main`'s generator. Both checkouts disable persisted Git credentials; neither can replace the independently verified release inventory. This does not mint a release token, write a GitHub Release, or replace the existing MCPB download/checksum validation.

Every Registry handoff must pass the same independently verified immutable-release prerequisite; a previous successful run is not sufficient evidence for a future version.

### Controlled publication and recovery

Each release requires a newly qualified exact `main` commit, successful full-tier push-event Test Suite and Scorecard, an unused annotated tag, enabled immutability, and protected maintainer approval. The tagged workflow's unprivileged `verify-complete` job checks the 22 originals before the protected `finalize` job obtains separate Administration:read and Contents:write GitHub App tokens. The coordinator rechecks the tagged source, original GoReleaser notes and private files, verifies all 22 draft assets, then publishes once. Only independently verified immutable publication authorizes the Registry OIDC publisher.

The initial end-to-end publication evidence is recorded under [3.3.3 in the Changelog](../CHANGELOG.md#333---2026-10-23). Maintainer approval of a protected deployment is a publication checkpoint, **not independent human code review**. Historical success does not authorize a new tag or mean an installed server has been upgraded.

On missing evidence, changed source, partial draft or uncertain GitHub mutation, stop without blind retries, re-tagging, overwriting immutable assets or bypassing protected controls. Recovery of an unpublished draft is permitted only after authenticating the **same unchanged source**, run/attempt and remote draft state and proving that any action is safe and idempotent. A failed consumed tag cannot be moved to a later main commit; qualify a new commit and unused release version instead. Preserve historical incomplete drafts for explicit, separately reviewed handling.

### Required evidence for publication workflow changes

Treat the existing CI and release scripts as a coupled system. Update release/MCPB/Registry workflows, source/provenance validators, `scripts/distribution-trust.test.js`, `scripts/test-ci-policy.test.js`, architecture tests, and `testarchitecture/evidence-map.json` together. Replace policy assertions that explicitly require *late* `gh release upload` or broad write permissions with **stronger** assertions for the draft-only finalizer, read-only producers, source binding, and absence of post-publication mutation. Never delete or skip a failing security gate merely to make a changed workflow green.

Retain the already qualified negative tests for missing/duplicate/wrong-type assets, 15+7 inventory mismatches, path aliases (including Windows 8.3 and macOS temporary roots), symlink/replacement races, wrong run or attempt, incomplete download, missing IDs, raw ZIP byte preservation, malicious names, token misuse, mixed workflow-code revisions, and every draft/retry/finalizer failure boundary. Run applicable Node/Go, actionlint/ShellCheck, static/vulnerability, provenance, and complete-diff checks. The GitHub-hosted synthetic raw upload/download proof establishes transport behavior without creating any public release or local MCPB package; retain that evidence while action versions and inputs remain unchanged. Any changed implementation must earn fresh hosted publication evidence for its own exact source.

See [Development Checklist](DEVELOPMENT_CHECKLIST.md) for the reusable review and verification gate. These remain mandatory for every workflow change; synthetic transfer tests alone do not prove genuine signed production, MCPB packaging or immutable publication.

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
