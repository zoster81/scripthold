# Markdown / Document Intelligence (R30)

## Status and authority

Scripthold can inspect Markdown structure and make controlled, source-preserving edits. The four Markdown-specific MCP tools and the shared 4.0 confirmation tool support a read–preview–apply workflow (3.3.6 still uses `markdown_apply`):

| Task | Tool |
|---|---|
| Inspect headings, sections, links, and other elements in one document | `markdown_read` |
| Inspect relationships and validation findings across documents | `markdown_workspace` |
| Prepare a new structured document without writing it yet | `markdown_create` |
| Preview changes to an existing document | `markdown_edit` |
| Confirm an approved preview using its `previewId` | `confirm_verified_operation` (4.0) |

Markdown semantics come from [Marksplice v1.2.0](https://github.com/zoster81/marksplice), not from an independent Scripthold parser or a text-replacement fallback. Marksplice handles CommonMark/GFM parsing, structural targets, change planning, document links, graph semantics, and conservative validation. Scripthold controls authorized file access, encoding and BOM handling, resource limits, preview approval, backups, durable writes, cancellation, and conflict checks.

The current implementation covers its approved Markdown capability boundary. The following features remain unavailable:

- **Deferred:** knowledge overlays and alias/tag/logical-reference queries; table cell/row/column content or structural edits beyond supported alignment changes; changes through unproven legacy or stateful encodings outside the verified UTF-8/UTF-16/UTF-32 bridge.
- **Outside this integration:** HTML rendering and source maps, caller-defined extension recognizers, host-invented semantic repair, and background indexing outside authorized roots.

Unsupported operations fail safely rather than being emulated with regular expressions or guessed document identities. For the exact public inputs and examples, see the [Markdown tools](../TOOLS.md#markdown_read).

## Navigate this guide

- [Safety and data-preservation rules](#boundary-invariants)
- [Marksplice-to-tool capability mapping](#marksplice-v120-public-capability-mapping)
- [Public request schemas](#public-schema-model)
- [Encoding and byte preservation](#encoding-and-physical-byte-preservation)
- [Preview and apply workflow](#previewapply-lifecycle)
- [Limits and unavailable features](#out-of-scope)

## Boundary invariants

1. Markdown is not a Source Intelligence language/provider. `.md` and `.markdown` content is handled only by the R30 Markdown tools.
2. Marksplice receives one immutable BOM-free UTF-8 byte snapshot containing the source's exact decoded line endings.
3. Marksplice failures are fail-closed. Scripthold never retries a rejected operation through regexes, generic text replacement, or hand-written Markdown rules.
4. Existing-document mutations are prepared by Marksplice `ChangeSet` values and are revalidated against the exact approved snapshot before physical mutation.
5. `confirm_verified_operation` accepts only `previewId`. Mutation parameters cannot be supplied or changed at apply time.
6. Unrelated authored source and line endings must remain unchanged. If Scripthold cannot prove that the encoding path can reproduce the untouched physical representation safely, mutation is rejected rather than silently re-encoding the document.
7. Canonical Markdown generation is an explicit export/generation path and is never substituted for source-preserving editing.
8. Marksplice `NodeID` values are snapshot-local implementation identities. They are never exposed as durable public identities.
9. Every query that can allocate an unbounded result requires a positive caller-visible limit and remains bounded by Scripthold's configured output/file/workspace limits.
10. Workspace discovery never broadens filesystem authority. Marksplice `workspacefs` receives only an `fs.FS` backed by paths already authorized by Scripthold.
11. If a Markdown capability or relationship-safety proof requires semantic authority that the pinned Marksplice API does not expose, R30 leaves that capability or unsafe context unavailable rather than implementing a host-side heuristic, parser rule, identity scheme, or rewrite fallback. The capability remains planned until Marksplice can prove it.

## Marksplice v1.2.0 public capability mapping

The mapping below covers the public non-rendering v1.2.0 API. Optional third-party extension recognizers are intentionally not exposed by R30 because they would add caller-defined semantic code outside Scripthold's reviewed Markdown authority boundary.

### `markdown_read`

`markdown_read` is read-only. Its action vocabulary is `inspect`, `query`, `get`, `resolve`, `validate`, and `generate`.

| Marksplice capability | R30 mapping |
|---|---|
| `Parse` | Every single-document read/edit preparation |
| `Nodes`, `Node` | `inspect`, `query`, `get` |
| Typed node accessors for paragraph, heading, list item, task, table/cell/row, fenced code/block, code span, emphasis, strong, strikethrough, link, image, autolink, reference definition, thematic break, blockquote, alert, footnote definition/reference, front matter/field, HTML comment/anchor, math expression | `get` typed detail and `inspect` summaries |
| `SourceRange` and segmented payload/content range accessors | `get` exact authored source for a proven semantic range |
| `QueryNodes` | `query` with required positive `limit` |
| `Sections`, `Section`, `SectionChildHeadingIDs`, `QuerySections` | `inspect`, `query`, `get` section navigation |
| `HeadingAnchors`, `HeadingAnchor`, `ResolveFragment`, `ValidateFragment` | `resolve` and `validate` |
| `LinkRelationships` | `inspect`, `resolve`, `validate` |
| Table alignment/ownership/row/cell helpers | `get` table detail |
| List parent/child/subtree helpers | `get` list detail |
| `FrontMatter` and promoted simple fields | `inspect`, `get` |
| `Alerts`, alert body ranges | `inspect`, `get` |
| `FencedBlocks`, fenced payload ranges | `inspect`, `get` |
| Footnote definitions/references and body ranges | `inspect`, `get`, `validate` |
| Math expressions and payload ranges | `inspect`, `get` |
| `GenerateTOC`, `TOCStale` | `generate` (`toc`) and `validate` |
| `CanonicalMarkdown`, `RenderCanonicalMarkdown` | `generate` (`canonical_markdown`) only; never an edit path |

`ParseWithOptions`, extension registration, `ExtensionNodes`, and extension recognizer APIs are not exposed in R30.

### `markdown_edit`

`markdown_edit` always prepares a preview. It never writes a file directly. A request contains one authorized document target and one or more declarative operations evaluated against the same immutable source snapshot. Independent prepared changes are combined with `Document.ComposeChanges`; overlap or semantic interaction rejected by Marksplice rejects the entire preview.

The table below is the R30 target mapping, not a claim that every Marksplice preparation API is registered publicly. The exact currently exposed operation forms are authoritative in [`TOOLS.md`](../TOOLS.md) and the runtime schema. Table content/row/column editing remains planned; alignment-only table mutation is exposed because it changes only Marksplice-owned delimiter syntax. Existing-document targetless construction remains limited to the closed `create/*` edit forms described below, while complete new-document construction is exposed separately through `markdown_create`.

Existing-document operation mapping:

| Operation family | Marksplice preparation authority |
|---|---|
| Heading | `PrepareRenameHeading`, `PrepareSetHeadingLevel` |
| Section | `PrepareReplaceSection`, `PrepareReplaceSectionBody`, `PrepareInsertSectionBefore`, `PrepareInsertSectionAfter`, `PrepareAppendSectionChild`, `PrepareMoveSectionBefore`, `PrepareMoveSectionAfter`, `PrepareRemoveSection` |
| Paragraph | `PrepareReplaceParagraph`, `PrepareInsertParagraphBefore`, `PrepareInsertParagraphAfter`, `PrepareRemoveParagraph` |
| List item / subtree | `PrepareReplaceListItem`, `PrepareReplaceListItemSubtree`, `PrepareInsertListItemBefore`, `PrepareInsertListItemAfter`, `PrepareAppendListItemChild`, `PrepareAppendFirstListItemChild`, `PrepareMoveListItemBefore`, `PrepareMoveListItemAfter`, `PrepareRemoveListItem` |
| Task | `PrepareSetTaskChecked` |
| Table cell / row / column | `PrepareReplaceTableCell`, `PrepareReplaceTableRow`, `PrepareInsertTableRowBefore`, `PrepareInsertTableRowAfter`, `PrepareAppendTableRow`, `PrepareMoveTableRowBefore`, `PrepareMoveTableRowAfter`, `PrepareRemoveTableRow`, `PrepareInsertTableColumn`, `PrepareMoveTableColumn`, `PrepareRemoveTableColumn`, `PrepareSetTableColumnAlignment`, `PrepareSetTableAlignments` |
| Fenced block / fenced code | `PrepareReplaceFencedCode`, `PrepareSetFencedBlockInfo` |
| Inline code / emphasis | `PrepareReplaceCodeSpan`, `PrepareReplaceEmphasis`, `PrepareReplaceStrong`, `PrepareReplaceStrikethrough` |
| Blockquote / alert | `PrepareReplaceBlockquoteContent`, `PrepareRemoveBlockquote`, `PrepareSetAlertKind`, `PrepareReplaceAlertBody` |
| Inline link | destination/label/title replacement plus add/remove title preparations |
| Inline image | destination/alt/title replacement plus add/remove title preparations |
| Autolink | `PrepareReplaceAutoLink` |
| Reference definition / occurrence | destination/title replacement, add/remove title, `PrepareRenameReferenceDefinition`, append definitions, remove definition, `PrepareRetargetReferenceOccurrence` |
| Footnote | `PrepareRenameFootnote`, simple/multiline body replacement, append definition, remove definition |
| Front matter | add/remove envelope, append/rename/remove field, replace field value |
| Raw HTML structure inside Markdown | `PrepareReplaceHTMLAnchor`, `PrepareReplaceHTMLComment` |
| Math | `PrepareReplaceMathExpression` |
| Thematic break | `PrepareRemoveThematicBreak` |
| Managed TOC | `PrepareSyncTOC`, `ComposeChangesAndSyncTOC` |
| Local-fragment continuity | `LocalFragmentContinuity` on the final composed `ChangeSet` |
| Multi-operation atomic preparation | `ComposeChanges`; `ComposeChangesAndSyncTOC` when one managed TOC sync is present |

### `markdown_create`

`markdown_create` prepares a complete new document through the reviewed Marksplice `DocumentBuilder` API and returns a one-shot preview consumed by `confirm_verified_operation`. It requires an authorized missing destination whose immediate parent already exists as a real authorized directory. Scripthold captures the parent identity and missing-target snapshot, generates and encodes the exact result bytes, and performs no write during preview.

The public construction model exposes:

- YAML/TOML front matter;
- headings and paragraphs;
- thematic breaks;
- blockquotes, nested blockquotes, and GitHub alerts;
- ordered/unordered lists and task lists, including reviewed nested forms;
- fenced code;
- reference definitions and deferred reference definitions;
- single-line and reviewed canonical multiline footnote definitions, including deferred definitions;
- math blocks;
- GFM tables with optional alignments;
- typed inline text, code, emphasis, strong, strikethrough, links/images with optional titles, autolinks, direct/reference/forward/collapsed/shortcut references, footnote references, and math inline forms.

Typed construction is the default. An advanced `rawMarkdown` construction fragment is accepted only where the corresponding Marksplice API already accepts raw GFM and reparses/proves the requested structure. R30 never performs its own Markdown escaping or validation as a fallback. Creation uses Marksplice canonical LF Markdown, optional new-file encoding and `auto`/`always`/`never` BOM policy, normal file/output limits, and filesystem no-replace commit. It creates no persistent backup because there is no approved pre-state.

### `markdown_workspace`

`markdown_workspace` is registered as a read-only multi-document surface with `inspect`, `query`, `validate`, and `repairPreview`. Discovery is explicit through Marksplice `workspacefs.Scan` or `workspacefs.Follow`, and every returned collection is caller-bounded. `validate` may designate managed TOCs by workspace document key plus uniquely resolved heading fragment and project Marksplice's conservative repair plan. `repairPreview` binds that same plan to complete per-target physical evidence and an expiring one-shot capability; it still performs no persistent mutation. Only `confirm_verified_operation` may consume the capability and write the prepared multi-file result.

Workspace cardinality has dedicated Scripthold policy rather than borrowing batch, match, or Source Intelligence limits. The server defaults are 1,000 documents and 25,000 relationships; operators may raise them only up to hard ceilings of 10,000 documents and 250,000 relationships through `MCP_MARKDOWN_WORKSPACE_MAX_DOCUMENTS` and `MCP_MARKDOWN_WORKSPACE_MAX_RELATIONSHIPS`. Request-level `maxDocuments` and `maxRelationships` may only narrow the configured ceilings. Per-document bytes remain bounded by `MCP_MAX_FILE_BYTES`, aggregate decoded workspace bytes by `MCP_MAX_FILESYSTEM_AGGREGATE_BYTES`, traversal depth by `MCP_MAX_FILESYSTEM_RECURSIVE_DEPTH`, and serialized output by `MCP_MAX_OUTPUT_BYTES`.

| Marksplice capability | R30 mapping |
|---|---|
| `workspacefs.Scan` | Authorized recursive Markdown workspace load |
| `workspacefs.Follow` | Authorized entry-based relationship-following workspace load |
| `workspacefs.Options` / `Limits` | Explicit bounded `maxDocuments`, `maxBytes`, `maxDepth`, `maxRelationships` constrained by Scripthold limits |
| `Workspace.Documents` | Internal graph input; compact document inventory in `inspect` |
| `Workspace.BuildGraph` / `BuildDocumentGraph` | `query` relationship graph |
| `DocumentGraph` keys/edges/outgoing/backlinks/reachable/related | `query` graph operations |
| `Workspace.Validate` / `ValidateWorkspace` | `validate` |
| Workspace diagnostics | `validate` structured diagnostics: missing/ambiguous/invalid fragment, missing document, unresolved reference, orphan document, plus stale/unrecognized generated indexes for explicitly managed TOCs |
| `WorkspaceRepairPlan` / `WorkspaceRepair.Change` | Repair count/document projection from `validate`; bounded source-bound `repairPreview` capabilities whose internal `ChangeSet` values are never exposed |
| `BuildKnowledgeIndex` | Deferred; not exposed by the initial read-only workspace registration |
| Knowledge aliases/tags/references/related/reachable queries | Deferred; not exposed by the initial read-only workspace registration |

R30 does not invent repair heuristics. Workspace repair planning is limited to repairs produced by Marksplice's conservative repair plan, currently explicitly managed TOC synchronization. The caller names each managed TOC by document key and fragment; Scripthold accepts it only when Marksplice resolves that fragment uniquely to a heading. `repairPreview` must expose every affected target and rejects rather than truncates an approvable plan; zero repairs return no token. `markdown_workspace` never writes files directly: the returned capability is consumed only by `confirm_verified_operation`, which revalidates all targets and reports committed/unchanged/unknown state without automatic rollback.

### `confirm_verified_operation`

Input schema is intentionally closed:

```json
{
  "previewId": "<64 lowercase hexadecimal characters>"
}
```

Unknown fields are rejected. A preview token is one-shot, process-local, bounded, expires, and becomes invalid after process restart. Applying a preview performs cancellation checks, path reauthorization, target-type checks, source fingerprint revalidation, exact decode/BOM revalidation, Marksplice `ChangeSet.Apply` source-snapshot verification, backup policy enforcement, staging, durable replacement, and truthful post-write state reporting.

## Public schema model

### Common document input

Single-document operations use:

- `path`: required authorized file path;
- `encoding`: optional explicit source encoding; when omitted Scripthold uses its normal deterministic detection/ambiguity rules;
- `limit`: required for result-producing queries where Marksplice requires a positive limit.

No Markdown tool accepts an arbitrary filesystem path returned by Marksplice. Paths always re-enter Scripthold authorization.

### Targeting

Most `markdown_edit` operations require an opaque `targetId` returned by `markdown_read` for the **same document snapshot**. Section and list-item movement also requires an `anchorTargetId` from that snapshot. If the document changes, obtain new targets before preparing another edit; stale identifiers are rejected.

A small set of document-level operations does not use a target: creating a front-matter envelope, front-matter field, reference definition, or footnote definition, and removing the entire recognized front-matter envelope. These forms are explicitly validated and do not accept `targetId`.

Scripthold derives target IDs from the source fingerprint and the target's public kind/range identity, not from serialized Marksplice `NodeID` values. Source offsets, line numbers, byte ranges, and occurrence counts are read-only diagnostic information, **not accepted edit selectors**. Even reference-occurrence retargeting requires a relationship `targetId`, not a caller-provided source offset.

For exact operation-specific fields, see the [`markdown_edit` reference](../TOOLS.md#markdown_edit).

### Edit request shape

The public contract for `markdown_edit` is in the [Tool Reference](../TOOLS.md#markdown_edit), including the 61 supported action/subject forms, allowed fields, limits, and request examples. This section explains the **Marksplice guarantees behind those forms**, rather than maintaining another list of parameters. Each request prepares 1–64 compatible operations against one immutable document snapshot; previewing does not write the file.

#### Sections, paragraphs, lists, and tasks

Markdown structural changes use parser-proven boundaries, not line-prefix guesses or arbitrary offsets. A paragraph is inserted relative to an existing paragraph target, and a section is inserted relative to an existing section target. A section's `targetId` is distinct from its heading's `headingTargetId`.

- **Section insertion and movement:** sibling sections must have the anchor's heading level. A direct child must be exactly one level deeper (`h6` cannot have a child). `after` and `child` operate after the entire existing subtree, not merely the heading. Moving a same-level subtree beside another section may also change its parent.
- **Section replacement:** replacing only the direct body must preserve the existing hierarchy; replacing a whole subtree requires one valid complete section. Marksplice rejects fragments that cannot preserve the requested structural meaning.
- **List-item changes:** only the complete parser-proven subtree may move or be removed; direct-content edits leave descendants intact. Insertion, replacement, and movement validate marker style, indentation, parent/child relations, fragment completeness, and end-of-file ownership. Movement preserves authored numbering and descendants, can reparent beside a compatible anchor, and rejects ancestor/descendant overlap.
- **Tasks:** a task has both a list-item identity for structural operations and a distinct task identity for its checkbox. Checkbox updates change only the proven state byte. Identical states and already-satisfied adjacent moves may be source-bound no-ops.

Inline content inside paragraphs and list items can include Markdown markup when Marksplice proves the replacement shape. Replacement of code spans, emphasis, strong emphasis, or strikethrough keeps source-proven delimiters; unsafe delimiter conflicts and unsupported source shapes fail closed.

#### Links, definitions, and footnotes

For direct links, images, and autolinks, Marksplice edits only the requested source-owned component. It preserves surrounding wrappers, link/image labels, delimiters, and unrelated source. Existing-title replacement requires an existing title; adding a title requires none, uses a canonical quoted form, and removal preserves separator whitespace. See the [tool contracts](../TOOLS.md#markdown_edit) for the different `part` values.

Reference-definition rename updates the definition and **every parser-proven bound reference link or image** together. When necessary, collapsed or shortcut uses are expanded to keep the visible label unchanged. Colliding or invalid labels are rejected; removing a definition is allowed only when no parsed reference still uses it.

Retargeting one reference occurrence uses the snapshot-bound relationship `targetId` from `markdown_read`. Scripthold proves it refers to the same parsed occurrence and lets Marksplice resolve the new, uniquely identified definition. A diagnostic `sourceOffset` never grants edit authority; ambiguous references are not guessed.

Footnote rename likewise updates bound references but leaves unrelated textual occurrences unchanged. Body replacement preserves source-proven line endings and continuation indentation; an LF in the requested text represents a logical line break. Removing a complete footnote definition preserves external `[^label]` text, even when the remaining reference becomes unresolved after reparsing. Empty or malformed changes, collisions, unsupported layouts, and stale targets fail closed.

#### Front matter and document-level construction

Four document-level constructors create a front-matter envelope, front-matter field, reference definition, or footnote definition without a `targetId`. Removing a recognized leading front-matter envelope is also targetless. Marksplice controls insertion positions, canonical syntax, separator ownership, uniqueness, and candidate reparsing. Independently prepared constructors targeting the same insertion point can conflict during `ComposeChanges`; Scripthold does not silently replay them in sequence.

Editing YAML/TOML fields preserves the original envelope, quoting, comments, spaces, and line endings. Duplicate keys are intentionally not targetable. A field's rename must be a valid noncolliding canonical key; field removal deletes only its source-owned physical line. Existing-document construction and all edits remain bound to the original source snapshot.

#### Fenced code, HTML, math, and containers

`markdown_read inspect.fencedBlocks` supplies targets even for **empty** fenced blocks; these may not have the separate generic `fenced_code` node. Marksplice preserves the authored fence and line endings while validating body and info-string changes. A compatible non-empty generic target may resolve to the same block.

Raw HTML changes touch only parser-proven comment content or quoted anchor values, preserving wrapper and attribute style. Unsafe comment terminators, quote-breaking values, or multiline replacements are rejected. Math changes preserve the original dollar, dollar-backtick, block-dollar, or fenced-`math` form; a fenced math block shares its underlying fenced-block identity rather than introducing a second target namespace.

Non-alert blockquote body replacement requires one source-proven marker prefix and line-ending style. GitHub alerts have a separately validated kind and body, while sharing the underlying blockquote identity. Marksplice rejects lazy-continuation or mixed-prefix layouts when a replacement would be unsafe. Removing a quote/alert deletes its **complete parser-owned container**, which can include CommonMark continuation lines without visible `>` prefixes. Removing a thematic break is permitted only if reparsing proves that neighboring text does not merge into a different paragraph.

#### Composed changes and local relationships

`ComposeChanges` checks the **combined final candidate**. Nonoverlapping byte ranges are not sufficient proof that two edits are structurally compatible. Marksplice's `LocalFragmentContinuity` checks that previously resolved surviving local-fragment links still point to the same semantic targets or are explicitly retargeted; silent changes, ambiguity, and missing/invalid outcomes fail closed.

An explicitly managed TOC can be synchronized alongside other changes (at most one `sync/toc` plus 63 other operations). `ComposeChangesAndSyncTOC` derives the TOC from the **final** candidate instead of editing its body independently. Every operation still resolves against the same immutable source snapshot, so stale or conflicting targets are never rebound heuristically.

### Read output

Read output is semantic-first and bounded. Depending on action it contains:

- source fingerprint and encoding/BOM/EOL metadata owned by Scripthold;
- compact structural summaries and typed details owned by Marksplice;
- opaque `targetId` values;
- exact source excerpts only when explicitly requested and within output limits;
- relationship/fragment status;
- generation output for TOC or canonical Markdown when explicitly requested.

### Preview output

A successful mutation preview contains:

- `previewId`, creation/expiry metadata;
- requested operations;
- resolved semantic targets without leaking internal paths or Marksplice identities;
- derived changes explicitly identified as derived;
- warnings/suggestions that are advisory rather than silently applied;
- preserved pre-existing issues;
- affected public paths;
- before/result fingerprints;
- encoding/BOM/EOL metadata;
- bounded unified diff when representable;
- `changed`/no-op state;
- backup policy.

A no-op preview remains safe to apply and produces truthful unchanged state without manufacturing a backup or write.

## Encoding and physical byte preservation

R30 treats Markdown as canonical UTF-8 text. Other text encodings are compatibility/transcoding paths for authorized legacy or external files, not alternative Markdown semantics.

The host performs the physical encoding bridge:

1. read exact authorized bytes;
2. detect/validate encoding and transport BOM using Scripthold encoding infrastructure;
3. decode to BOM-free UTF-8 without normalizing EOLs;
4. parse/prepare/apply in Marksplice against those exact UTF-8 bytes;
5. re-encode using the approved original encoding/BOM policy only when the host can prove the physical representation is safe;
6. reject mutation if the source encoding path is ambiguous, invalid, non-round-trippable, or otherwise cannot satisfy unrelated-byte preservation.

At minimum, the original decoded snapshot must round-trip byte-identically through the selected encoder before an edit preview can be approved. This is a necessary guard, not permission to normalize stateful encodings. The current mutation bridge is proven only for UTF-8 plus fixed-width Unicode UTF-16 LE/BE and UTF-32 LE/BE, preserving BOM presence and decoded line endings; other registered encodings remain read-only for R30 mutation until unchanged-byte stability is proven. Read-only Markdown operations remain available when mutation must fail closed.

Mixed EOL documents are preserved because Marksplice receives the exact decoded EOL bytes and owns the replacement spans. Scripthold must not globally normalize LF/CRLF around a Marksplice change.

## Error model

R30 maps errors by typed/sentinel identity, never by parsing Marksplice diagnostic strings.

Structured Markdown error codes include:

- `target_not_found`
- `ambiguous_target`
- `unsupported_operation`
- `unsupported_target_kind`
- `invalid_structure`
- `invalid_construction`
- `invalid_query`
- `invalid_workspace`
- `workspace_budget_exceeded`
- `encoding_ambiguous`
- `encoding_not_byte_preserving`
- `source_conflict`
- `ambiguous_derived_changes`

Underlying MCP error families continue to use Scripthold's common invalid-input, encoding, limit, authorization, cancellation, conflict, and filesystem classifications.

Relevant Marksplice mapping:

- `ErrNodeNotFound` -> `target_not_found`;
- `ErrInvalidTargetKind` -> `unsupported_target_kind`;
- `ErrInvalidReplacement` -> `invalid_structure`;
- `ErrInvalidConstruction` -> `invalid_construction`;
- `ErrInvalidQuery` -> `invalid_query`;
- `ErrSourceConflict` -> `source_conflict`;
- `ErrInvalidGraph`, `ErrInvalidWorkspace`, `ErrInvalidKnowledge` -> `invalid_workspace`;
- `workspacefs.ErrBudgetExceeded` -> `workspace_budget_exceeded`.

Marksplice refusal is terminal for that requested semantic operation.

## Preview/apply lifecycle

A preview retains only bounded process-local state required for safe application: approved public path(s), exact physical fingerprints, decode/BOM metadata, prepared Marksplice changes, expected result fingerprints, file modes, backup policy, and bounded presentation data.

The apply lifecycle is:

1. validate and atomically claim the one-shot token;
2. cancellation check;
3. reauthorize every target and reject path/symlink/type changes;
4. reread exact physical source and compare the approved fingerprint;
5. decode using the approved encoding/BOM facts and require the exact approved UTF-8 snapshot;
6. call `ChangeSet.Apply` again so Marksplice independently verifies its source binding;
7. re-encode and verify the approved result fingerprint/byte-preservation constraints;
8. create required safety backup(s);
9. stage replacement(s);
10. revalidate immediately before replacement and perform the host durable mutation path;
11. report actual state, including partial failure if a multi-document physical commit cannot be completed atomically by the host.

Preview replay, expired tokens, evicted tokens, restart-invalidated tokens, concurrent double-apply, or source changes are conflicts.

## Test strategy

R30 is implemented incrementally with focused failing tests before each behavior where practical.

### Authority and regression tests

- Source Intelligence no longer registers, detects, routes, advertises, or tests a Markdown analyzer/provider.
- `.md`/`.markdown` remain ordinary authorized files for Markdown tools but never produce Source Intelligence Markdown symbols.
- no R30 package contains a Markdown regex/parser/fallback transformation path.
- `go.mod` resolves exactly the released Marksplice v1.2.0 baseline.

### Read and targeting tests

- ATX/Setext headings, sections, paragraphs, nested lists/tasks, tables, fenced blocks, front matter, references, footnotes, alerts, math, raw-HTML anchors/comments, links/images/autolinks;
- duplicate semantic names produce `ambiguous_target` rather than first-match selection;
- `targetId` resolves only against the exact source snapshot;
- required positive query limits and global output limits are enforced;
- malformed/non-standard source is readable where Marksplice exposes it, without invented mutation authority.

### Edit/preview tests

- every exposed edit operation delegates to the corresponding Marksplice `Prepare*`/builder authority;
- independent multi-edits compose; overlap/semantic interaction fails closed;
- Marksplice rejection leaves physical files byte-identical;
- exact authored delimiter/trivia/spacing and unrelated source bytes remain unchanged;
- mixed LF/CRLF is not normalized globally;
- no-op previews are truthful;
- canonical generation is never used as an edit fallback.

### Encoding tests

- UTF-8 with/without BOM;
- UTF-16 LE/BE with BOM and exact EOL preservation;
- representative legacy encodings that satisfy byte-stability proof;
- invalid byte sequences, ambiguous detection, BOM/encoding conflict, unrepresentable output, non-round-trippable/stateful cases all fail closed without mutation.

### Apply lifecycle tests

- apply schema accepts only `previewId` and rejects unknown/mutation fields;
- stale source, path replacement, authorization change, expiry, eviction, restart, replay, concurrent double apply;
- cancellation before destructive work;
- read-only targets/force-writable policy where explicitly supported;
- backup-required flow verifies backup bytes match approved pre-state;
- staging/write failure reports truthful state and never reuses the token;
- output-limit failure cannot make a consumed preview replayable.

### Workspace tests

- scan/follow only inside authorized roots;
- traversal, encoded traversal/separators, absolute/scheme/protocol-relative/backslash targets never expand authority;
- cycles, depth/document/byte/relationship budgets;
- graph outgoing/backlink/reachable/related queries;
- missing/ambiguous/invalid fragments, missing documents, unresolved references, orphan documents;
- managed TOC stale/unrecognized diagnostics and Marksplice-only repair preview;
- optional knowledge aliases/tags/logical references never discover new documents.

### Fuzz/race/regression

- fuzz selector/target IDs and closed request decoders;
- race tests for concurrent read and one-shot apply claims;
- full repository regression, vet, lint/static analysis, vulnerability scan, documentation checks, formatting, and Git cleanliness/diff review before promotion.

## Out of scope

- HTML rendering and HTML source maps;
- arbitrary third-party Marksplice extension recognizers;
- semantic inference beyond Marksplice's reviewed authority;
- automatic repair of ambiguous links or arbitrary prose;
- direct mutation by `markdown_read`, `markdown_workspace`, or `markdown_edit`;
- durable public use of Marksplice `NodeID` values;
- background indexing or hidden workspace discovery beyond caller-authorized roots.
