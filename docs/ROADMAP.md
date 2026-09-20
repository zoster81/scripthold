# Scripthold Development Roadmap

This file tracks **current and future work only**. Completed changes belong in [`CHANGELOG.md`](../CHANGELOG.md); current behavior and invariants belong in [`ARCHITECTURE.md`](ARCHITECTURE.md), [`TOOLS.md`](../TOOLS.md), and the focused operational guides.

## Current state

- Current public release: **Scripthold 3.2.1**.
- Public 3.2.1 surface: **38 tools**, **3 guided prompts**, **168 registered text encodings**, and **101 active source-intelligence providers**.
- Completed work is summarized in [`CHANGELOG.md`](../CHANGELOG.md) and implemented history remains in Git.
- Two 3.x capability areas remain tracked. The next capability milestone is verified self-update; no release-scoped publication milestone is currently assigned.
- Publication, installation, and private deployment are separate actions.

## Planning rules

- At most one release-scoped milestone may be active at a time.
- Define the user-visible outcome, compatibility boundary, security impact, and completion gate before implementation.
- Preserve current filesystem, encoding, mutation, backup, execution, transport, and source-intelligence boundaries unless a milestone explicitly changes them.
- Keep public MCP concepts compact and difficult to misuse.
- Require an exact clean commit and the release procedure in [`PUBLISHING.md`](PUBLISHING.md) for public release authority.
- Do not accumulate implementation diaries or per-phase evidence in this file.

## 3.x milestones

### Verified self-update

Add a failure-safe update lifecycle: discover a release, select the correct platform asset, verify identity/checksum, stage it, retain a known-good binary, switch, verify, and roll back when required.

Installation state remains separate from the persistent user-file backup store. Interrupted or invalid updates must leave a usable installation or an explicit recoverable state.

### Source-intelligence completion

Improve analyzer accuracy, trustworthy missing relationships, detection/provider quality, scale, index/query usefulness, and regression corpora without adding mutation authority to Source Intelligence.

Unsupported relationships remain unsupported until analyzers can prove them. Persistent on-disk source indexing or external parser/compiler/LSP dependencies require separate architectural approval.

## Reserved 4.0 boundary

Version 4.0 is reserved for a deliberately reviewed major compatibility or capability boundary after the 3.x work. Its scope is not frozen.

Automatic project-wide semantic refactoring remains out of scope unless maintainers explicitly reopen that decision.

Research topics such as visual desktop interaction or MCP federation are not approved milestones or APIs. They require separate product, architecture, privacy, security, and UX review before implementation.
