# Development Checklist

Use this checklist for non-trivial Scripthold changes. Current milestone state lives in [ROADMAP.md](ROADMAP.md), repository/scoped instructions in [`AGENTS.md`](../AGENTS.md), contributor workflow in [`CONTRIBUTING.md`](../CONTRIBUTING.md), and public release procedure in [PUBLISHING.md](PUBLISHING.md).

Apply only the checks relevant to the change, but report skipped checks explicitly.

## Find a check

Use [requirements](#1-requirements-and-edge-cases) and [design review](#2-architecture-and-test-strategy) before editing; follow [implementation](#5-tdd-and-implementation) and [verification](#6-verification-ladder) while working; then use the [documentation](#7-documentation-and-metadata), [review](#8-commit-and-review-gate), and [release](#10-public-release-gate) gates only when relevant.

## 1. Requirements and edge cases

- [ ] Restate the intended behavior and observable outcome.
- [ ] Identify affected packages, handlers, schemas, metadata, documentation, workflows, and packaging.
- [ ] For MCP tools, verify that static annotations describe the complete public tool capability; do not mix read-only and mutating actions under misleading metadata.
- [ ] Identify compatibility constraints and any intentional breaking change.
- [ ] Define valid, empty, missing, malformed, ambiguous, oversized, and unsupported inputs.
- [ ] Define encoding, BOM, and LF/CRLF behavior where relevant.
- [ ] Define filesystem, permission, read-only, missing-path, and partial-failure behavior.
- [ ] Review path traversal, symlink, junction, reparse-point, hard-link, and allowed-root risks.
- [ ] Review cancellation, timeout, concurrency, race, TOCTOU, and cleanup behavior.
- [ ] For prepared operations, define capability lifetime, entropy, replay, ownership, eviction, restart, and stale-target behavior.
- [ ] For approval-bound mutations, ensure apply accepts only the prepared capability identifier unless an explicitly reviewed design proves additional fields cannot alter the approved operation.
- [ ] For multi-file operations, define preflight, staging, commit order, partial-state reporting, crash behavior, and any explicit non-atomicity.
- [ ] Review Windows, Linux, macOS, long-path, and cross-platform implications.
- [ ] Identify likely regressions and public behavior that must remain unchanged.

## 2. Architecture and test strategy

- [ ] Generate multiple credible candidate designs before editing, including the smallest obvious solution and materially different alternatives.
- [ ] Keep transport, MCP adapters, domain logic, and filesystem primitives separated.
- [ ] Reuse shared internal primitives instead of adding local copies.
- [ ] Define data flow, ownership, memory bounds, and cleanup responsibilities.
- [ ] Define deterministic fingerprints, canonical path representation, and metadata exclusions where relevant.
- [ ] Keep persistent backup retention, restore, and garbage-collection policy behind the approved backup contract.
- [ ] Define typed errors and public error mapping.
- [ ] State meaningful time/space complexity where relevant.
- [ ] For each candidate, record affected components, data flow, compatibility impact, meaningful complexity/trade-offs, and the focused tests that would prove or disprove it.
- [ ] Include normal, edge, invalid-input, regression, filesystem-failure, encoding/BOM, line-ending, cancellation, concurrency, and platform cases as applicable.
- [ ] Include security-negative tests, not only successful paths.
- [ ] For durable tasks, cover idempotency conflict, queue/concurrency bounds, logical locks, frontend/worker/supervisor failure, stale recovery, at-most-once behavior, process-tree cancellation, cursor gaps, retention, and allowed-directory changes between restarts.
- [ ] For cancellation tests, check prompt execution-context cancellation separately from durable terminal-state persistence; hold the control lock deterministically to test that distinction instead of assuming both phases complete within one short deadline.
- [ ] For structured execution, verify fixed invocation, argument construction, working-directory confinement, environment filtering, timeout, cancellation, and bounded diagnostics without a shell.

## 3. Devil's advocate review

- [ ] Challenge every phase-2 candidate with concrete implementation or operational risks; compare all candidates against the same repository evidence and constraints.
- [ ] Review allowed-root escape and path-based race windows.
- [ ] Review data loss, non-atomic writes, rollback, cleanup, and recovery artifacts.
- [ ] Review unbounded memory, output, lines, requests, sessions, queues, caches, manifests, and retained recovery state.
- [ ] Review nondeterministic ordering, cancellation, replay, and concurrent consumption.
- [ ] Review multi-file partial commits and misleading atomicity claims.
- [ ] Review encoding corruption, malformed Unicode, and binary false positives.
- [ ] Review dependency, platform, API, metadata, and documentation drift.
- [ ] Review whether any supposedly read-only MCP tool can reach filesystem, backup-store, task-store, or other persistent mutation paths.
- [ ] Eliminate dominated candidates explicitly and identify whether criticism requires a new or revised candidate.
- [ ] If the design space changes, return to phase 2, add or revise candidates, then run phase 3 against the complete candidate set again.
- [ ] Repeat the `phase 2 -> phase 3 -> phase 2` loop until no materially better candidate remains and the preferred design is justified by repository evidence.
- [ ] Reopen phase 1 only if the review reveals a new or changed requirement/edge case; do not substitute random trial-and-error for design convergence.

## 4. Repository safety before editing

- [ ] Read the root and nearest scoped `AGENTS.md` files.
- [ ] Read the relevant roadmap/design/source-of-truth documents.
- [ ] Verify branch, `HEAD`, remote tracking, and working-tree status.
- [ ] Preserve unrelated contributor changes.
- [ ] Inspect surrounding implementation and tests before editing.
- [ ] Check target-file encoding, BOM, and line endings when relevant.
- [ ] Prefer targeted or atomic edits and preserve unrelated content.
- [ ] Do not use destructive Git commands or rewrite history during ordinary development.
- [ ] Keep private operator state, credentials, local paths, PIDs, and runtime/deployment state out of tracked files.

## 5. TDD and implementation

- [ ] Reproduce the issue or missing behavior with a focused test when practical.
- [ ] Confirm the test fails for the expected reason.
- [ ] Implement the smallest correct production change.
- [ ] Keep public schemas stable unless an approved milestone explicitly changes them.
- [ ] Preserve formatting, encoding, BOM state, and line endings.
- [ ] Use explicit error handling, rollback, and cleanup.
- [ ] Add comments only for genuinely non-obvious constraints.
- [ ] Avoid unrelated refactors and dependency changes.
- [ ] Rerun the focused test and confirm it passes.
- [ ] Review the changed code before broader verification.

## Source Intelligence project-first qualification

This reusable method checks whether `source_symbols` and `source_query` give a trustworthy picture of an unfamiliar *real project*, not just one selected declaration. Full source reading is a **test-oracle activity**: the product must still help its users avoid reading every file.

1. **Select a bounded, qualified original project.** Verify its identity and source revision against the qualification ledger. Record every selected source file, language, encoding/BOM, relevant imports, exclusions and byte limits. Do not mistake files for independent projects or process binaries, data, and licenses as source. Never build or execute third-party project code.
2. **Read original project sources independently of tool results.** The qualification unit is the **entire original project for one language/dialect**, not an isolated file. Enumerate all relevant source files in the checkout, including project-owned tests; classify vendored/embedded dependencies, generated/binary/non-source files and exclusions explicitly. For a large project, process all eligible files in bounded batches and preserve one aggregate project report. If any required source file remains unexamined, mark the project **incomplete**, never fully qualified. Individual complete files/classes may still support defect reproduction, but are not substitutes for project-wide qualification. Before trusting an output, create a manually judged oracle for each *fully qualified* scope: all real declarations, names, kinds, parent owners, declaration lines, overloads, relationships where supported, and forbidden examples (comments, strings, assignments, calls). A source file that was merely read is not automatically an exhaustively annotated file. Treat grep matches as candidate evidence, not verified declarations.
3. **Run the installed/connected MCP baseline.** Apply `source_symbols` `digest`, `outline` (tree, compact, detailed), `find`, fingerprint-verified `show`, and applicable `source_query` `search`, `context`, and supported `relations` to the *same explicit source-file set*. Record inputs, selected language, encoding, identities/fingerprints, statuses, `coverageComplete`, truncation, and actual runtime/version when known. Treat an older connected server as a **historical RED comparison baseline**, not as proof that the current source is still defective. A connected-MCP discrepancy is not an executed RED test on development HEAD; confirm the missing behavior with a failing local Go regression before attributing it to current code. Never assume the running server contains the current Git changes.
4. **Run the development version at the appropriate boundary.** For a parser reproducer, call `internal/sourceintelligence` on the original bytes using `OpenSourceDocument`; label the result `local-analyzer`. For an MCP contract check during the structural campaign, call the current local `source_symbols`/`source_query` **Go handlers** through focused integration tests with identical inputs and limits; label these results `local-MCP-handler`. An analyzer call is not a handler call. **Defer any standalone local MCP server or transport test until final qualification**; it is not an iteration prerequisite. Do not restart or update the installed server to make comparisons agree without explicit authorization.
5. **Compare against truth, not counts alone.** Match symbols by file, kind, qualified name, declaration location, owner and overload/definition identity. Examine false positives, omissions, wrong kinds/parents, misplaced ranges, wrong bodies, inconsistent digests/trees and unjustified completeness. Run `grep_text_files` and targeted `read_text_file` for **the same questions**; preserve cases in which grep provides better evidence. For a fully and independently annotated scope, count true positives, false positives, and false negatives only after matching name, kind, location, and owner (and overload identity where relevant). Report precision as TP/(TP+FP) and recall as TP/(TP+FN) only when the denominators are nonzero; state the actual number of examined files, declarations, and projects. A 100% score on selected files is not an estimate of the whole language or project. When macros or generated declarations introduce namespaces or owners, distinguish source-written symbol accuracy from compiler-expanded semantic identity: a perfect written-source oracle does not prove the final expanded qualified names. Report correct useful facts, false/missing facts, tool calls and actual response sizes; distinguish serialized character length from wire bytes, and do not claim timing/memory without measurements.
6. **Classify, reproduce, repair.** First distinguish a wrong oracle, unsupported syntax, encoding ambiguity, stale connected binary and a genuine current-HEAD defect. For a genuine defect, derive a minimal portable positive/negative test from the original source; observe RED before editing; compare at least the minimal solution and a credible alternative against false-symbol, performance, compatibility and resource risks; implement the smallest safe fix; demonstrate GREEN and targeted neighbor/handler tests. Do not add project-specific whitelists or delete failing evidence.
7. **Recheck the real inputs and report precisely.** Repeat the source-versus-local comparison after the fix and validate input fingerprints. Store a machine-readable **private** project/revision report containing the original source inventory, explicitly fully annotated scope(s), `connected-MCP`, `local-analyzer`, `local-MCP-handler`, and `grep+read` evidence where actually exercised, all discrepancies, test outcomes and remaining unsupported cases. A scanned project or `coverageComplete: true` does not certify every symbol. Mark unperformed boundaries as `not tested`; only a real handler/server test qualifies local MCP behavior. Never commit private source checkouts, workstation paths, runtime identifiers, or reports.
8. **Prioritize proven shared foundations without skipping individual languages.** Reconcile the native registry against actual analyzer and scanner call sites; classify direct shared parsers separately from composed/embedded analyzers and merely similar syntax. Before changing a shared primitive, identify its consumers, demonstrate a real problem with independent counterexamples, and compare a common repair with provider-local alternatives. Recheck every directly affected family after the fix. Independently qualify **at least eight real original projects per provider where available**; a common implementation, a successful family test, or eight files from one project never replace those per-language oracles. Preserve honest unsupported syntax and coverage in the private report and the current roadmap. Do not assume C/C++ or BASIC exhaust the families or dictate a permanent test order.

**Run qualification per dialect and entire project.** Process the active language providers in documented dialect/family groups; order groups by directly shared parser/scanner boundaries and observed defect severity, not only alphabetically. Within each provider, choose eight independently qualified original project identities from the pinned ledger; finish a project's complete eligible source inventory and applicable project-wide operations before advancing it to PASS. For a project exceeding input/output/file limits, split into deterministic batches that together cover all eligible files, reconcile duplicate/missing paths across batches, and aggregate real coverage/failure evidence. A language filter alone does not restrict a directory to source files: apply precise include/exclude patterns and verify the selected source file set against the original checkout. Record test-vs-production and vendored inclusions, and treat stale source revisions, unparsed files, truncation, unexplained false/missing declarations, or incomplete relations as **open project qualifications**, never green by omission. Representative single-file tests remain regression witnesses only. Preserve full-project source-query questions, cross-file results, grep/read comparisons and exact revision in the private project report; do not execute third-party sources. No arbitrary promise that a huge checkout can be fully qualified in one tool call.

**Report qualification stages separately.** An acquired project is only a source candidate. A **read-and-sampled** project has complete reads for the declared file/class scope and checked positive/forbidden *examples*, but has no exhaustive inventory claim. A **fully annotated selected scope** has an independently enumerated expected inventory (including nested declarations, owners, source locations, and relevant negatives) and verified analyzer/handler results. A **project-navigation-qualified** scope additionally verifies applicable `digest`, outline presentations, `find`/`show`, structural search, context, supported relations, and matched grep/read questions. State the exact count of independent projects at each stage; eight sampled projects do not meet an eight-project full-qualification gate. Handler-returned symbol totals and `coverageComplete: true` are observations, not independent ground truth.

See [ROADMAP.md](ROADMAP.md#active-milestone--source-intelligence-structural-reliability) for milestone-specific acceptance. Do **not** test spontaneous LLM tool choice or require source tooling to beat grep.

## 6. Verification ladder

Run checks from focused to broad and record exact outcomes. During implementation, stop at the narrowest level that covers the changed component and its integration boundaries unless evidence or change scope justifies expansion. Do not treat the full repository, full race, fuzz, or release-adjacent gates as mandatory after every isolated edit.

Use three practical local levels:

1. **Focused iteration** — changed package(s), directly affected handler/integration/schema/catalog tests, relevant documentation/policy checks, formatting, diff review, and targeted race/fuzz only when the touched behavior warrants them.
2. **Pre-push/promotion** — full normal regression plus repository-wide static/vulnerability checks and the canonical bounded fuzz smoke. Use this before pushing a meaningful batch, after cross-cutting changes, or when focused evidence leaves uncertainty. For a coherent large documentation or tool-catalog batch, run `go test -timeout=30m -count=1 ./...`, every `scripts/*.test.js` Node test, Markdown link validation, and complete diff review locally before the first push. Do this once per stable batch, not after each wording edit; repeat affected checks if the qualified inputs change.
3. **Release/full qualification** — exact-commit cross-platform race/build/smoke/security/release evidence required by CI and publication policy. Do not reproduce this entire tier locally after every incremental commit.

Independent focused checks may run in parallel when they do not mutate shared inputs or compete for the same scarce resource. Prefer one Go invocation over several competing full-package invocations because the Go tool already parallelizes package work; avoid concurrent CPU-heavy full test/race/fuzz jobs that oversubscribe the machine or make timing-sensitive tests flaky.

**Local long-running Go tests:** Default to `go test -timeout=30m` **per package** for qualification and for encoding/durability tests that perform many real filesystem synchronizations, instead of arbitrarily short ad-hoc limits. The private workspace validation runner supplies this flag automatically; selected short tests can use explicit lower limits. For a standalone 30-minute Go test launched as a durable task, reserve outer execution headroom (normally 1980 seconds) for termination/reporting, and inspect the terminal result before claiming success. This is not a change to CI job ceilings, server task execution defaults, or the independent investigation of stuck tests.

GitHub CI applies fail-closed evidence tiers to pull requests. Documentation-only changes run repository policy, all `internal/sourceintelligence` package tests (including generated capability/documentation drift), local Markdown-link validation, release/script policy tests, and secret scanning. Go-only changes add the normal Linux regression suite, evidence-map-derived Windows/macOS platform tests and race-sensitive package tests, module verification, static/vulnerability analysis, and the canonical risk-based fuzz smoke. Changes to workflows, scripts, test architecture, build/release/configuration metadata, fixtures, or any unclassified path require full qualification. Pushes to `main`/`master`, manual runs, classifier bootstrap, and classification failures also require full qualification.

The complete exact-commit gate retains broad normal regression across Linux/Windows/macOS, broad race coverage on all three native runners, native binary smoke, six-target cross-builds, hardened container smoke, workflow validation, Gitleaks, GoReleaser configuration validation, static/vulnerability analysis, and canonical fuzz evidence. The `Release candidate` aggregator must fail when any evidence required by the selected tier is missing or unsuccessful.

### Focused checks

- [ ] affected package tests;
- [ ] affected handler/integration tests;
- [ ] metadata or script tests;
- [ ] platform-specific focused tests.

### Go verification

For focused iteration:

- [ ] `gofmt` on changed Go files;
- [ ] affected package and integration tests with `-count=1`;
- [ ] `go mod verify` when Go module/dependency integrity is relevant;
- [ ] targeted vet/static/race/fuzz checks when the touched code or failure class warrants them.

For pre-push/promotion or cross-cutting changes:

- [ ] `go test ./... -count=1`;
- [ ] `go vet ./...`;
- [ ] `golangci-lint run ./...` with the repository-pinned policy;
- [ ] Staticcheck at the repository-pinned version;
- [ ] govulncheck at the repository-pinned version;
- [ ] canonical bounded fuzz smoke when applicable;
- [ ] race coverage at the scope justified by the change; full repository race belongs to full/promotion evidence rather than routine isolated iteration;
- [ ] coverage/benchmark review when the risk justifies it.

### Build and platform checks

- [ ] native build when production code changes;
- [ ] Windows amd64/arm64 cross-builds when platform or release behavior changes;
- [ ] Linux amd64/arm64 cross-builds when platform or release behavior changes;
- [ ] macOS amd64/arm64 cross-builds when platform or release behavior changes;
- [ ] runtime execution on available platforms when behavior is platform-specific;
- [ ] exact-binary durable-task smoke when task lifecycle or release behavior changes.

### Repository and documentation checks

- [ ] Node release-script and CI-policy tests where release or workflow metadata is relevant;
- [ ] JSON/YAML parsing for changed structured files;
- [ ] PowerShell parsing for changed `.ps1` files;
- [ ] actionlint/ShellCheck for workflow or shell changes;
- [ ] Markdown local-link validation;
- [ ] catalog/runtime/documentation drift tests;
- [ ] private-operator-state regression test;
- [ ] scan modified text for unexpected control characters when generated or scripted edits are involved;
- [ ] `git diff --check`;
- [ ] complete diff review;
- [ ] final `git status` with no unexpected files.

### Security and release-adjacent checks

- [ ] Gitleaks for tracked content/history when relevant;
- [ ] GoReleaser configuration checks when packaging changes;
- [ ] GitHub MCPB/Registry workflow definitions, templates, and non-artifact-producing script tests when packaging/catalog behavior changes;
- [ ] never create, pack, simulate, dry-run, checksum, or validate real `.mcpb` bundles locally, and never generate the final MCPB-backed Registry manifest locally;
- [ ] no credentials, tokens, private keys, cookies, real tunnel identifiers, or workstation state added.

### Immutable-release workflow qualification

Use this extra gate whenever the release, MCPB, provenance or Registry publication chain changes. [PUBLISHING.md](PUBLISHING.md#immutable-release-pipeline-prevention-requirements) defines the active failure-prevention contract. Previous successful publication cannot qualify a changed workflow; require fresh, exact-revision evidence for any changed trust boundary.

- [ ] Start from historical CI failure **classes**, not just green current tests: temporary-directory aliases, Windows short names, permission/link races, wrong MCPB subject types, mixed pinned Action versions, and incomplete/late publication.
- [ ] Compare the current GoReleaser/MCPB/Registry output contract with any proposed flow change. Preserve real archive/binary paths, version metadata, original release notes, genuine 12-subject signed provenance, six authentic GitHub-only MCPB packages, checksum semantics, and stable consumer URLs.
- [ ] Test signer and source identity independently of sidecar byte consistency: mutate both the Sigstore bundle and in-toto envelope together, recalculate their original asset sizes and digests, and require rejection for a wrong workflow path/ref, repository, builder, source commit, or subject. A mismatched companion alone does not prove those checks. Structural fixture validation never substitutes for genuine GitHub CLI cryptographic verification.
- [ ] Keep producer-local filesystem paths out of cross-job manifests. Test Windows drive/UNC/8.3 aliases, macOS `/var` versus `/private/var`, `/tmp` and `/var/tmp`, symlink and hardlink escapes, case/Unicode collisions, path length and malicious separators. Do not treat all legitimate system-alias ancestors as attacks.
- [ ] Use verified, owner-controlled, no-follow, exclusive staging and stable file identity before checking original bytes; exercise replacement/rename/permission/concurrent-writer failures and no-overwrite cleanup.
- [ ] Authenticate each stage by exact repository/commit/run/attempt, original name/size/SHA and GitHub numeric artifact ID. Independently validate both stage inventories and require the complete published set before transition.
- [ ] Set one-file `upload-artifact` options `archive: false`, `if-no-files-found: error`, `overwrite: false`; set `download-artifact` options `artifact-ids`, `skip-decompress: true`, `digest-mismatch: error`, and an isolated `path`. Check the physical staged basename and the post-download **original bytes**, including files that end in `.zip`.
- [ ] Exercise wrong/missing/duplicate IDs, stale attempts, mismatched digests, incomplete download, truncated or altered streams, and unexpected filenames even when the GitHub Action itself reports success.
- [ ] Keep `internal/releasearchive` in the Go-only CI targeted **Windows/macOS platform** and **Linux race** selections: its rooted filesystem stages are OS-dependent and include concurrent directory allocation. Preserve full-tier promotion checks separately.
- [ ] For authenticated ZIP verification, test non-progressing and invalid `ReaderAt` sources during both original digest hashing and ZIP payload reading; allow finite intermittent empty reads, retain CRC-32/SHA-256 validation, and do not claim interruption of a blocking read.
- [ ] For producer and staging-consumer JavaScript byte streams, reject 100 consecutive empty chunks while accepting intermittent empty chunks separated by real progress. This cannot interrupt a blocked asynchronous iterator; the hosting job still needs an execution timeout.
- [ ] Fuzz the bounded 16 KiB, 15-record raw-normal JSON decoder with malformed, duplicate, case-aliased and oversized inputs. Also test a reader returning repeated `(0, nil)` results, intermittent empty reads, and impossible byte counts; a byte-size limit alone does not bound non-progressing reads. Accepted **wire-format** objects must retain exact names, sizes and digests under ordinary JSON key-order normalization; never assume internal Go struct JSON field names match the strict incoming schema, and never treat successful decoding as GitHub source/asset authentication.
- [ ] Give build and packaging jobs read-only publication authority, and only the finalizer the minimum write authority needed for an unpublished draft. Bind **workflow helper revision** as well as tagged source SHA; reject mutable-main versus tag code drift and independent post-publication repair writes.
- [ ] Exercise draft state transitions: cancellation, partial upload, API timeout after success, duplicate/retried upload, conflicting existing asset, publish collision, and Registry failure after publication. Reject missing, forged, or changing draft title, original GoReleaser notes, and prerelease state both at creation and before every resumed upload; require canonical producer notes metadata before any provider call. Resume exact matches only; never overwrite an already-published asset.
- [ ] Update all coupled workflow, Node policy, `testarchitecture/evidence-map.json`, architectural tests, and maintainer documentation in one reviewed batch. Replace obsolete old-flow assertions with **stronger** negative/security tests, never remove them merely to pass.
- [ ] Pin related CodeQL components and compiler/analysis tooling as compatible units; keep the existing exact-SHA Test Suite and supported main-push Scorecard provenance gate. Workflow/script changes must receive the full evidence tier.
- [ ] Before authorizing a public tag/release, require normal static/Node/Go workflow qualification, actionlint/ShellCheck, source-format/diff review, and **real GitHub cross-job raw-transfer evidence** from a harmless non-release synthetic test. Report missing proof; never simulate real MCPB packaging locally.

## 7. Documentation and metadata

- [ ] Update only documents whose behavior, status, contract, or navigation changed.
- [ ] For every affected public document or tool section, explain first what it does, when or why to use it, and representative applications before low-level mechanics.
- [ ] Prefer plain English, short direct sentences, and concrete examples; define unavoidable technical terms when they first matter.
- [ ] Keep schemas, limits, security constraints, compatibility rules, and failure behavior exact, but place them after the reader understands the normal workflow where practical.
- [ ] Review the whole affected public section for jargon, implementation-first wording, duplicated details, and unclear purpose rather than patching only one sentence.
- [ ] Keep each fact in its designated source of truth and link instead of duplicating detailed procedures or history.
- [ ] Use repository-relative links and portable placeholders.
- [ ] Keep README, roadmap, project direction, publishing notes, tool reference, and subsystem contracts consistent.
- [ ] Do not imply filename- or extension-based encoding detection.
- [ ] Do not claim streaming, sandboxing, atomicity, deployment, or platform verification beyond evidence.
- [ ] Update `CHANGELOG.md` for user-visible behavior, compatibility, security, packaging, or architecture changes.
- [ ] Keep `internal/toolcatalog/catalog.json`, runtime registration, README tool links, `TOOLS.md`, and release projection synchronized.
- [ ] Keep generated release output out of version control.

## 8. Commit and review gate

- [ ] Stage only files belonging to the change.
- [ ] Review the staged diff and staged file list.
- [ ] Use a concise English commit message.
- [ ] Verify commit contents and working tree afterward.
- [ ] In the review/handoff, distinguish code reviewed, code modified, tests executed, builds performed, publication, deployment, and checks not performed.

## 9. Internal build gate

Use this section only for an explicitly requested internal build.

- [ ] Build from the exact verified clean commit.
- [ ] Embed an unambiguous commit-derived version.
- [ ] Use a new versioned filename rather than overwriting a known-good artifact.
- [ ] Record size and SHA-256 privately where operationally required.
- [ ] Verify `--version` and Go VCS/module metadata.
- [ ] Preserve a known-good rollback artifact.
- [ ] Keep launcher, credentials, process state, and deployment evidence outside the repository.

## 10. Public release gate

Use [PUBLISHING.md](PUBLISHING.md) as the authoritative release procedure. The active pipeline is protected, draft-first, single-writer and GitHub-enforced immutable. This checklist describes the release gate; the qualification section above applies when the publication implementation changes.

- [ ] release-scoped milestone complete;
- [ ] exact clean commit and dated changelog entry verified;
- [ ] complete **full-tier** push-event `Test Suite` (`.github/workflows/test.yml`) `Release candidate` gate passes on the exact pushed SHA; pull-request tier evidence is never publication authority;
- [ ] OpenSSF Scorecard has a successful supported `push` run on the default `main` branch for that same exact SHA;
- [ ] annotated tag resolves to that same commit;
- [ ] GoReleaser produces the 13 authentic normal originals, without publishing an incomplete GitHub Release;
- [ ] signed Sigstore SLSA provenance and its unchanged DSSE companion are authenticated for all 12 binary/archive subjects;
- [ ] GitHub-only MCPB packaging produces six authentic packages and `mcpb-checksums.txt`;
- [ ] unprivileged and protected jobs independently verify all 22 original artifacts by run, attempt, name, size and SHA-256;
- [ ] protected environment approval and separate short-lived, least-privileged publication/administration tokens are verified;
- [ ] one complete draft is published once, then independently read back as `draft: false`, `immutable: true` with all 22 assets;
- [ ] only afterward, Registry OIDC publication succeeds and its six MCPB URLs and SHA-256 digests match GitHub;
- [ ] tag, changelog, embedded version, and Registry version agree.

Deployment, active rollback, restoration, private-launcher changes, and runtime restarts are separate operator actions and require their own authorization and verification.
