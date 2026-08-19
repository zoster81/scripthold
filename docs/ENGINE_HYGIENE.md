# R28 Engine Hygiene

## Status

R28 is **COMPLETE** and shipped in Scripthold `3.1.0` on 2026-08-18. Release qualification, GitHub publication, GitHub-only MCPB publication, and MCP Registry publication are complete. Deployment, launcher, and active-runtime changes remain separate maintainer actions.

This document is the completed implementation contract and verification record for R28. The public milestone state remains defined by [ROADMAP.md](ROADMAP.md); completed R23-R27 contracts remain authoritative compatibility history.

## Outcome

R28 reduced demonstrated implementation debt without redesigning the public MCP surface. It consolidated only behavior proven equivalent, removed dead/test-only helpers supported by call-site evidence, reorganized historical production filenames by current responsibility, and used benchmarks/profiling to decide whether performance work was justified. No speculative performance optimization was introduced.
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

## Completed work

R28 reviewed cleanup candidates against current callers, compatibility contracts, and focused tests rather than treating age or naming as evidence of debt.

- The deprecated `HandleBackupStore` read bridge was consolidated onto current R23 read primitives where behavior was proven equivalent; non-equivalent edit/package/encoding/BOM compatibility paths were deliberately retained.
- Four unexported production helpers whose only purpose was test forwarding were removed after call-site review.
- Thirty-one source-intelligence production files were renamed by current responsibility without changing their source contents. The later verification-architecture cleanup also removed historical milestone/phase naming from permanent Go test architecture.
- Representative benchmarks and CPU profiling established a baseline but did not identify a bottleneck that justified speculative performance code changes.
- Focused, complete normal, affected race, vet, lint, Staticcheck, vulnerability, secret, documentation, and diff checks passed. R28 shipped in `3.1.0` without a public MCP behavior change.

## Retained cleanup decisions

The opening audit intentionally rejected several tempting but unsafe cleanup shortcuts:

- Do not delete `MCP_MEMORY_THRESHOLD`; handler guidance requires it as the migration fallback.
- Do not delete all exported pre-R23 bridges; not being registered as MCP tools does not make exported Go entry points irrelevant to compatibility.
- Do not treat every occurrence of `legacy` in the encoding subsystem as debt.
- Do not collapse modern and legacy MCP protocol paths without a dedicated compatibility decision.
- Do not mass-rename every `phase*` file solely to remove historical names.
- Do not claim performance improvements without measurements.

These decisions are part of the R28 safety boundary and should be revisited only with new repository evidence or explicit maintainer approval.
