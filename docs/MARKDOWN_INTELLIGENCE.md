# Markdown / Document Intelligence (R30)

## Status and authority

R30 introduces Markdown and documentation intelligence through four MCP tools:

- `markdown_read`
- `markdown_edit`
- `markdown_workspace`
- `markdown_apply`

The semantic authority is `github.com/zoster81/marksplice v1.1.1`. Scripthold does not implement a competing Markdown parser, Markdown regular-expression fallback, or Markdown string-replacement engine.

Marksplice owns Markdown parsing, GFM/CommonMark semantics, structural identification, source-preserving change planning, structured construction, document relationships, graph semantics, and conservative workspace validation/repair. Scripthold owns filesystem authorization, encoding/BOM handling, physical reads/writes, resource limits, preview/apply lifecycle, backups, durable mutation, cancellation, and conflict revalidation.

HTML rendering and Markdown-to-HTML source maps are intentionally outside R30.

## Boundary invariants

1. Markdown is not a Source Intelligence language/provider. `.md` and `.markdown` content is handled only by the R30 Markdown tools.
2. Marksplice receives one immutable BOM-free UTF-8 byte snapshot containing the source's exact decoded line endings.
3. Marksplice failures are fail-closed. Scripthold never retries a rejected operation through regexes, generic text replacement, or hand-written Markdown rules.
4. Existing-document mutations are prepared by Marksplice `ChangeSet` values and are revalidated against the exact approved snapshot before physical mutation.
5. `markdown_apply` accepts only `previewId`. Mutation parameters cannot be supplied or changed at apply time.
6. Unrelated authored source and line endings must remain unchanged. If Scripthold cannot prove that the encoding path can reproduce the untouched physical representation safely, mutation is rejected rather than silently re-encoding the document.
7. Canonical Markdown generation is an explicit export/generation path and is never substituted for source-preserving editing.
8. Marksplice `NodeID` values are snapshot-local implementation identities. They are never exposed as durable public identities.
9. Every query that can allocate an unbounded result requires a positive caller-visible limit and remains bounded by Scripthold's configured output/file/workspace limits.
10. Workspace discovery never broadens filesystem authority. Marksplice `workspacefs` receives only an `fs.FS` backed by paths already authorized by Scripthold.

## Marksplice v1.1.1 public capability mapping

The mapping below covers the public non-rendering v1.1.1 API. Optional third-party extension recognizers are intentionally not exposed by R30 because they would add caller-defined semantic code outside Scripthold's reviewed Markdown authority boundary.

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
| Managed TOC | `PrepareSyncTOC` |
| Multi-operation atomic preparation | `ComposeChanges` |

New-document construction maps the reviewed `DocumentBuilder` API into typed construction blocks and inline values:

- YAML/TOML front matter;
- headings and paragraphs;
- thematic breaks;
- blockquotes, nested blockquotes, and GitHub alerts;
- ordered/unordered lists and task lists, including reviewed nested forms;
- fenced code;
- reference definitions and deferred reference definitions;
- footnote definitions and deferred footnote definitions;
- math blocks;
- GFM tables with optional alignments;
- typed inline text, code, emphasis, strong, strikethrough, links/images with optional titles, autolinks, direct/reference/forward/collapsed/shortcut references, footnote references, and math inline forms.

Typed construction is the default. An advanced `rawMarkdown` construction/edit fragment is accepted only where the corresponding Marksplice API already accepts raw GFM and reparses/proves the requested structure. R30 never performs its own Markdown escaping or validation as a fallback.

### `markdown_workspace`

`markdown_workspace` is the multi-document surface. Its action vocabulary is `inspect`, `query`, `validate`, and `repair`.

| Marksplice capability | R30 mapping |
|---|---|
| `workspacefs.Scan` | Authorized recursive Markdown workspace load |
| `workspacefs.Follow` | Authorized entry-based relationship-following workspace load |
| `workspacefs.Options` / `Limits` | Explicit bounded `maxDocuments`, `maxBytes`, `maxDepth`, `maxRelationships` constrained by Scripthold limits |
| `Workspace.Documents` | Internal graph input; compact document inventory in `inspect` |
| `Workspace.BuildGraph` / `BuildDocumentGraph` | `inspect`, `query` relationship graph |
| `DocumentGraph` keys/edges/outgoing/backlinks/reachable/related | `query` graph operations |
| `Workspace.Validate` / `ValidateWorkspace` | `validate` |
| Workspace diagnostics | `validate` structured diagnostics: missing/ambiguous/invalid fragment, missing document, unresolved reference, orphan document, stale/unrecognized generated index |
| `WorkspaceRepairPlan` / `WorkspaceRepair.Change` | `repair` preview; only Marksplice-proven repairs are eligible |
| `BuildKnowledgeIndex` | Optional caller-provided syntax-independent aliases/tags/logical references over the already-authorized graph |
| Knowledge aliases/tags/references/related/reachable queries | `query` knowledge operations |

R30 does not invent repair heuristics. Workspace repair authority is limited to repairs produced by Marksplice's conservative repair plan, currently including explicitly managed TOC synchronization. A repair action produces preview state; it never writes directly.

### `markdown_apply`

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

A target is exactly one of:

1. `targetId`: opaque snapshot-bound identity returned by `markdown_read`;
2. a semantic selector whose uniqueness can be proven by Marksplice-backed structure and typed metadata;
3. a structural path made from proven section/list/table relationships;
4. a source offset only for Marksplice APIs whose public contract specifically identifies an occurrence by source offset, such as reference retargeting.

Ambiguous selectors fail with candidates. R30 never selects the first duplicate implicitly.

`targetId` is derived from the source snapshot fingerprint plus the target's public kind/range identity, not from a serialized Marksplice `NodeID`. It is therefore stable only for that exact source snapshot and cannot be used after the file changes.

Line numbers, byte ranges, and occurrence counts are diagnostic output, not general mutation authority.

### Edit request shape

`markdown_edit` uses one document plus an ordered `operations` array. Every operation contains a closed `action`/`subject` combination and only the fields defined for that combination. Examples of action classes are `create`, `insert`, `replace`, `remove`, `move`, `rename`, `set`, and `sync`.

All operations are resolved against the same immutable snapshot. R30 does not mutate an in-memory document after operation 1 and then reinterpret operation 2 against changed coordinates. Marksplice `ComposeChanges` is the final authority on whether independently prepared operations can coexist.

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

The host performs the physical encoding bridge:

1. read exact authorized bytes;
2. detect/validate encoding and transport BOM using Scripthold encoding infrastructure;
3. decode to BOM-free UTF-8 without normalizing EOLs;
4. parse/prepare/apply in Marksplice against those exact UTF-8 bytes;
5. re-encode using the approved original encoding/BOM policy only when the host can prove the physical representation is safe;
6. reject mutation if the source encoding path is ambiguous, invalid, non-round-trippable, or otherwise cannot satisfy unrelated-byte preservation.

At minimum, the original decoded snapshot must round-trip byte-identically through the selected encoder before an edit preview can be approved. This is a necessary guard, not permission to normalize stateful encodings. The implementation may further restrict mutation to encoding profiles for which unchanged source segments remain physically stable. Read-only Markdown operations remain available when mutation must fail closed.

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
- `go.mod` resolves exactly the non-retracted Marksplice v1.1.1 baseline.

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
