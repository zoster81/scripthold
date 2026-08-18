# R28 Engine Hygiene

## Status

R28 source implementation is **COMPLETE** as of 2026-08-18. Its intended release line remains Scripthold 3.1; release, publication, deployment, launcher, and active-runtime changes remain separate maintainer actions.

This document is the completed implementation contract and verification record for R28. The public milestone state remains defined by [ROADMAP.md](ROADMAP.md); completed R23-R27 contracts remain authoritative compatibility history.

## Outcome

Reduce repository complexity that can be demonstrated from current code and tests without gratuitously changing product behavior.

R28 may:

- remove dead or test-only internal helpers;
- consolidate implementations that already share the same invariants and externally observable behavior;
- retain compatibility entry points while moving their implementation onto current primitives;
- reorganize source files when historical phase names obscure current responsibility;
- simplify unused state or redundant branches proven unnecessary by tests and call-site analysis;
- improve performance only when a benchmark or profile demonstrates a real bottleneck and verifies the improvement.

R28 does not authorize a public MCP redesign, broad compatibility removal, dependency replacement, semantic source refactoring, or speculative performance work.

## Compatibility boundary

Public MCP tool names, schemas, effects, messages, error categories, preview/apply authority, encoding/BOM/EOL behavior, durable mutation guarantees, backup-store behavior, transport behavior, and task execution behavior remain unchanged unless a separate compatibility change is explicitly approved.

Exported Go compatibility entry points are not deleted merely because they are no longer registered as MCP tools. They may be reduced to adapters over current implementations when equivalence is demonstrated.

The following are explicitly **not** generic cleanup targets:

- `MCP_MEMORY_THRESHOLD`, which remains the documented migration fallback for specific limits;
- legacy text encodings, which remain a core product capability;
- MCP legacy-protocol or dynamic-roots compatibility paths whose behavior is still part of the adopted protocol contract;
- platform-specific filesystem branches required by Windows, Linux, or macOS semantics;
- security, durability, encoding, and backup boundaries that only look structurally similar.

## Security and durability boundary

Every refactor must preserve the existing allowed-root, reparse/symlink, TOCTOU, fingerprint, one-shot capability, staged replacement, backup, cancellation, output-limit, and partial-state guarantees of the affected subsystem.

Do not merge implementations across a security or durability boundary unless their preconditions, commit authority, failure semantics, and tests are genuinely identical.

## Evidence rules

A cleanup candidate requires at least one of these forms of repository evidence:

1. an unexported symbol has no production callers and only forwards to a current implementation;
2. two implementations execute the same contract and focused equivalence tests can bind their outputs;
3. state or a branch is unreachable under the current validated input contract;
4. historical file organization materially obscures current domain ownership and can be changed without code behavior changes;
5. a benchmark or profile demonstrates a performance bottleneck.

Textual labels such as `legacy`, `deprecated`, `phase`, `compatibility`, or `TODO` are discovery signals, not sufficient evidence by themselves.

## Execution plan

### Phase 0 — Activation and baseline

Status: **COMPLETE**

- Read the active roadmap, completed R23-R27 contracts, root/scoped agent guidance, and engineering checklist.
- Verify branch, `HEAD`, `origin/main`, divergence, and working-tree state before R28 edits.
- Activate R28 in the public roadmap.
- Establish focused regression baselines before the first code change.

### Phase 1 — Compatibility-bridge consolidation

Status: **COMPLETE**

Each pre-R23 handler bridge was reviewed independently.

Decisions:

- `HandleBackupStore`: keep the exported compatibility entry point and legacy input validation. Validated status/list/inspect/audit plus restore-preview/GC-dry-run actions now route through the R23 read surface; apply dispatch remains separate. Regression coverage binds structured status/list/inspect/audit equivalence while preserving the legacy disabled-store response.
- `HandleEditFile`: retain the small compatibility dispatcher over current preview/apply plus the intentionally preserved direct-edit path. Removing the exported bridge would change package compatibility without eliminating duplicate machinery.
- `HandlePatchPackage`: retain the small compatibility dispatcher over current inspect/verify/dry-run/apply primitives for the same reason.
- `HandleConvertEncoding`: retain the legacy direct and batch implementation. It shares lower-level encoding/BOM helpers with current code, but its streaming conversion, partial batch result, direct mutation, and legacy backup/error semantics are not equivalent to the R23 capability-bound preview/apply contract.
- `HandleManageBom`: retain the legacy direct add/strip path. BOM detection is already shared through `detectBOMPrefix`, while legacy direct mutation is intentionally distinct from the R23 preview capability, fingerprint, backup-policy, and apply authority model.

Deprecated bridges therefore remain where they are deliberate package compatibility surface rather than unexplained duplicate implementations.

### Phase 2 — Internal dead and test-only paths

Status: **COMPLETE**

- Removed `handler.searchSingleFile`; focused tests now exercise `searchSingleFileWithBudget` directly.
- Removed `normalizeAllowedDirectories` and `readTextDocument`, two unexported production wrappers referenced only by same-package tests; those tests now call the current implementations directly.
- Removed the production-only `detectorLabelHasDisposition` test predicate and kept the registry-disposition assertion local to its test.
- Retained exported compatibility helpers such as `encoding.LineEndingBytes`; R28 does not infer removability from test-heavy usage when package compatibility may be involved.
- Re-ran a bounded declaration/reference scan across tracked Go packages after the removals; it found no remaining unexported production functions whose only non-declaration references are in same-package tests.

No subsystem boundary or public API was changed by this phase.

### Phase 3 — Historical source organization

Status: **COMPLETE**

The source-intelligence production tree was reviewed by responsibility and reorganized in bounded groups. Before the moves, tracked filename-reference scans found no code, documentation, script, build, or generated-tool dependency on the historical production filenames.

Completed organization:

- generic token, scanner-profile, composite-scanner, logical-line, and scanner-runtime files now use responsibility names instead of R27 phase labels;
- .NET analyzer files now identify their F#/C++/CLI, composite, and JScript.NET/CIL/PowerShell ownership;
- the former phase-7, phase-8, phase-9, phase-10, phase-11, and phase-16 production groups now use analyzer/profile/helper names based on the language families or domains they implement;
- each bounded move group was followed by the complete `internal/sourceintelligence` package tests; the opening and final groups also ran the applicable vet/Staticcheck checks;
- the resulting production directory has no remaining `phase`/`r27`-named Go files. Historical phase names remain in tests where they identify the qualification milestone and therefore improve evidence traceability rather than obscure production ownership.

No Go source content or behavior was changed by these moves.

### Phase 4 — Measured performance work

Status: **COMPLETE**

Representative Windows amd64 benchmarks were run three times for source analyzers, large generated Go analysis, bounded text reads, large source queries, and many-small-file symbol queries. The runs were stable within the expected short-benchmark variance: the large generated Go analyzer remained about 13.3-13.6 ms/op at roughly 5.9-6.0 MB/s; bounded reads scaled from about 46-47 MB/s at 1 MiB to about 205 MB/s at 64 MiB; and the large source-query workload remained about 31.8-36.5 ms/op at roughly 6.8-7.8 MB/s.

A CPU profile of the large generated Go workload showed distributed cost dominated by runtime allocation/GC work plus the expected `SymbolBuilder.Add`/`normalizeSymbol` path and hashing. It did not isolate a new R28 regression or a single hot path whose replacement was justified without changing ownership, caching, or correctness assumptions. Allocation counts remain useful future performance evidence, but they are not by themselves authorization for a speculative algorithm change.

Decision: make no performance code change in R28. The benchmark/profile gate provides a measured baseline and avoids claiming an improvement that was not implemented or demonstrated. The temporary CPU-profile artifact was removed after inspection.

### Phase 5 — Final hygiene and verification

Status: **COMPLETE**

Final evidence on Windows amd64 with the verified workspace toolchain:

- the touched-scope compatibility re-scan found only explained residuals: the deliberate deprecated `HandleBackupStore` package bridge, the exported `encoding.LineEndingBytes` compatibility helper, and supported legacy text encodings; the post-removal test-only-helper scan found zero remaining unexported production helpers referenced only by same-package tests;
- complete diff review found no unrelated path changes, all 31 source-intelligence production renames matched their `HEAD` source blobs byte-for-byte, no production `phase`/`r27` Go filenames remain, `gofmt` was clean, local Markdown links and modified-text control-character checks passed, and `git diff --check` passed;
- focused backup-store, grep, encoding/BOM, helper-removal, and source-intelligence gates passed before the final repository run;
- `go test ./... -count=1 -timeout=15m` passed across the complete repository. An earlier isolated handler run timed out while Windows was blocked in `FlushFileBuffers`; the exact `x-mac-cyrillic` subtest then passed in under two seconds and the final complete repository run passed with the handler package completing in about 307 seconds, so the evidence supports a transient durability-I/O delay rather than a deterministic R28 regression;
- `go mod verify`, `go vet ./...`, golangci-lint 2.12.2, Staticcheck 0.7.0, govulncheck 1.7.0, the source-server smoke, project-identity/catalog tests, and Gitleaks history/current-working-tree scans passed; govulncheck reported no vulnerabilities and golangci-lint reported zero issues;
- the CGO race gate passed for `internal/sourceintelligence` and `filetoolsserver/handler`, with the full handler race package completing in about 217 seconds;
- no platform-specific implementation path was changed by R28. Product builds, cross-builds, release actions, launcher changes, deployment, and runtime restart were not performed as part of source completion.

R28 therefore completes without a public MCP behavior change and without claiming an unmeasured performance improvement.

## Initial audit decisions

The opening audit intentionally rejected several tempting but unsafe cleanup shortcuts:

- Do not delete `MCP_MEMORY_THRESHOLD`; handler guidance requires it as the migration fallback.
- Do not delete all exported pre-R23 bridges; not being registered as MCP tools does not make exported Go entry points irrelevant to compatibility.
- Do not treat every occurrence of `legacy` in the encoding subsystem as debt.
- Do not collapse modern and legacy MCP protocol paths without a dedicated compatibility decision.
- Do not mass-rename every `phase*` file solely to remove historical names.
- Do not claim performance improvements without measurements.

These decisions are part of the R28 safety boundary and should be revisited only with new repository evidence or explicit maintainer approval.
