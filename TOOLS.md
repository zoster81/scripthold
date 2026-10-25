# Scripthold Tool Reference

This reference explains what each Scripthold tool does, when to use it, its main inputs, and the important safety or limit behavior. Start with the short purpose and examples in each section; use the detailed rules when you need exact API behavior or troubleshooting information.

The current public release, Scripthold `3.3.6`, exposes 43 tools and 3 guided prompts. The source tree may document additional unreleased tools below. Stdio and Streamable HTTP expose the same tool behavior; only connection setup and transport security differ. See [README.md](README.md) for setup, [docs/HTTP_SECURITY.md](docs/HTTP_SECURITY.md) for HTTP deployment, and [docs/DURABLE_TASKS.md](docs/DURABLE_TASKS.md) for long-running execution.

## Find the right tool

Use this index to start with a task, then open the tool section for its inputs, examples, limits, and errors. New to the server? Begin with the [quick start](README.md#quick-start) or the [Documentation Guide](docs/README.md).

| What you need to do | Reference |
|---|---|
| Read, write, or preview edits to a file | [File operations](#file-operations) |
| Browse directories or prepare multi-file filesystem changes | [Directory operations](#directory-operations) |
| Inspect, create, or edit Markdown with structural awareness | [Markdown document tools](#markdown-document-tools) |
| Navigate declarations and project relationships in source code | [Source Intelligence](#source-intelligence) |
| Compare fingerprints or verify workspace state | [Workspace state](#workspace-state) |
| Inspect, restore, or delete protected backups | [Backups](#backups) |
| Search text across authorized files | [Text search](#text-search) |
| Detect or change text encodings, BOMs, and line endings | [Encoding tools](#encoding-tools) |
| Check server roots, updates, or deferred read-only results | [Service and deferred operations](#service-and-deferred-operations) |
| Run and monitor durable local tasks | [Durable task execution](#durable-task-execution) |
| Use guided workflows, interpret errors, or check codecs | [Guided prompts](#guided-prompts), [error handling](#error-handling), [supported encodings](#supported-encodings) |

## Guided Prompts

- `audit_encodings(path)`: read-only project encoding, BOM, ambiguity, and line-ending audit.
- `fix_mojibake(path)`: evidence-driven diagnosis and approval-gated repair of garbled legacy text.
- `migrate_to_utf8(path, pattern?)`: search, batch dry-run, approval, backup-enabled conversion, and final verification workflow.

These prompt concepts and the implementation approaches reviewed are credited to the [original project](docs/PROJECT_DIRECTION.md#relationship-to-upstream); the resulting code is reworked for this fork's tool names, mutation guarantees, limits, and dual-transport server rather than mechanically synchronized. Prompt instructions guide clients; they do not add per-agent filesystem ACLs or bypass tool authorization.

## Error Handling

When a tool call fails, clients receive a readable message and a stable error code at `_meta.errorCode`. Use the code for automated handling and the message for diagnosis.

- Tools with structured output also include `errorCode` and `message` there, while preserving any existing structured failure evidence.
- Per-file failures from `read_multiple_files`, batch `convert_encoding`, and `grep_text_files.skippedFiles` use the same error-code vocabulary.
- An encoding-related per-file failure may also include `encodingErrorCode` with one of `ENCODING_AMBIGUOUS`, `ENCODING_MALFORMED`, `ENCODING_UNSUPPORTED`, `ENCODING_BOM_CONFLICT`, `ENCODING_UNREPRESENTABLE`, or `ENCODING_OTHER`. This adds detail; it does not replace the main code.

**Stable error codes:** `INVALID_INPUT`, `INVALID_PATH`, `ACCESS_DENIED`, `SYMLINK_ESCAPE`, `NOT_FOUND`, `PERMISSION`, `ENCODING`, `ENCODING_AMBIGUOUS`, `CONFLICT`, `PARTIAL_COMMIT`, `UNSUPPORTED`, `CANCELLED`, `LIMIT`, `IO_ERROR`, `INTERNAL_ERROR`, and the fallback `OPERATION_FAILED`. Successful calls do not include an error code.

**Important:** `PARTIAL_COMMIT` means a write failed after a target or package may already have changed. Where supported, the response includes fingerprints and state evidence to distinguish committed, unchanged, partial, or unknown outcomes. Do not assume an automatic rollback.

For the original error-code migration contract, see [Migration 2.0](docs/MIGRATION_2.0.md).

## File Operations

Use these tools to read, create, edit, copy, move, or remove files while keeping changes inside authorized workspace roots. Mutating tools are designed to detect common concurrent changes and avoid silently overwriting a destination that appeared after preparation.

Writes stage replacement bytes in the destination directory and synchronize them before commit. The exact safeguards depend on the operation and operating system:

- **Concurrent changes:** existing-file snapshots detect practical conflicts; a missing destination uses a no-replace commit so an intervening creator is not overwritten.
- **Recursive packages:** preserve exact operation scope, stable object/volume identity, package-wide staging, mandatory persistent backup before irreversible deletion of regular files, and native same-volume moves without replacement.
- **Unix:** synchronize containing directories after namespace changes.
- **Windows:** first use write-through `MoveFileExW`. After a retryable existing-file replacement failure **and a fresh state check**, a DELETE-gated `FileRenameInfoEx` POSIX replacement may install the already-synchronized staged file.
- **Limit:** these measures reduce, but cannot eliminate, every path-based time-of-check/time-of-use (TOCTOU) race.

### read_text_file

Read file contents through the shared incremental decoder with automatic encoding detection and optional partial reading. UTF-8 files pass through unchanged; other registered encodings convert to UTF-8. Empty files are treated as assumed UTF-8; non-empty ambiguous input fails with `ENCODING_AMBIGUOUS` until `encoding` is supplied. A Unicode transport BOM is removed from returned content and reported separately through `hasBOM` and `bomType`. `MCP_MAX_LINE_BYTES`, `MCP_MAX_DECODED_CHARACTERS`, and `MCP_MAX_OUTPUT_BYTES` bound the result.

**Parameters:**
- `path` (required): Path to the file
- `encoding` (optional): Encoding name (auto-detects if omitted)
- `offset` (optional): Start reading from this line number (1-indexed)
- `limit` (optional): Maximum number of lines to read
- `maxCharacters` (optional): Truncate content at this character count to prevent token overflow
- `lineNumbers` (optional): Prefix each returned line with its absolute 1-based line number (default: false)

**Example:**
```json
{
  "path": "/path/to/file.pas",
  "offset": 100,
  "limit": 50
}
```

**Response:**
```json
{
  "content": "line 100\nline 101\n...",
  "totalLines": 500,
  "fileSizeBytes": 15234,
  "startLine": 100,
  "endLine": 149,
  "truncated": false,
  "detectedEncoding": "utf-16-le",
  "encodingConfidence": 100,
  "hasBOM": true,
  "bomType": "utf-16-le"
}
```

### read_multiple_files

Read several files in one request using the same incremental encoding and BOM-aware decoding as `read_text_file`. Results remain in input order, even when an individual file fails or the request is cancelled.

- **Bounds and concurrency:** `MCP_MAX_BATCH_FILES` limits the number of inputs; `MCP_MAX_OUTPUT_BYTES` caps aggregate decoded output. Parallel reads are used only when their worst-case combined output fits that budget.
- **Per-file errors:** failures do not prevent other files from being read. Every requested path receives a stable result; `errorCount` counts all failures, and failed entries include `errorCode` and, when relevant, `encodingErrorCode`.
- **Bounded error summary:** the compatibility `errors` array retains at most 64 errors, subject to a separate byte budget derived from the output limit. Successfully decoded content takes priority over this duplicate summary.
- **Honest truncation:** `errorsTruncated` and `errorsOmitted` reveal when error details were left out.

**Parameters:**
- `paths` (required): Array of file paths to read
- `encoding` (optional): Encoding for all files (auto-detected per file if omitted)

**Example:**
```json
{
  "paths": ["/path/to/file1.pas", "/path/to/file2.pas"],
  "encoding": "cp1251"
}
```

**Response:**
```json
{
  "results": [
    {
      "path": "/path/to/file1.pas",
      "content": "program Hello;...",
      "detectedEncoding": "utf-16-le",
      "encodingConfidence": 100,
      "hasBOM": true,
      "bomType": "utf-16-le"
    },
    {
      "path": "/path/to/file2.pas",
      "content": "unit Utils;..."
    }
  ],
  "successCount": 2,
  "errorCount": 0
}
```

**Per-file failure example:**
```json
{
  "path": "/path/to/missing.pas",
  "error": "file not found: /path/to/missing.pas",
  "errorCode": "NOT_FOUND"
}
```

### write_whole_file

Replace the complete target file contents with the supplied UTF-8 `content`, using the selected target encoding through the shared document encoder. The explicit `write_whole_file` name is intentional: the historical `write_file` name could be mistaken for an incremental edit or append operation, while this tool discards any existing text not present in `content`. Use `edit_file` when only part of an existing document should change.

The file is written with exactly the supplied line endings. Invalid encoding or BOM policy fails before mutation.

- **Durable replacement:** replacement bytes are staged and synchronized before the final atomic commit. Existing targets are checked for concurrent changes; a newly created target uses a no-replace commit to prevent overwriting a file created in the meantime.
- **Bounded comparison:** the prepared and final states have fingerprints. For a sufficiently small existing target, the prior content fingerprint is also retained; larger targets use the historical metadata-only concurrency snapshot instead of an unbounded read.
- **Uncertain outcomes:** after a possible commit failure, the handler checks only bounded post-state evidence and reports `state: unchanged|committed|unknown`, `changed`, `applied: false`, and `actualFingerprint` when available. A committed or unclassifiable post-error state returns `PARTIAL_COMMIT` rather than claiming that nothing changed.

**Parameters:**
- `path` (required): Path to the file
- `content` (required): UTF-8 content to write
- `encoding` (optional): Target encoding. New files use the configured default (`utf-8` by default); existing files preserve a confidently detected encoding. Set `MCP_DEFAULT_ENCODING` or pass `encoding` explicitly for legacy formats such as `cp1251`.
- `bom` (optional): BOM policy — `auto` (default), `always`, `never`, or `preserve`; `none` is accepted as a compatibility alias for `never`

**BOM policy:**
- `auto`: UTF-8 and legacy encodings have no BOM; UTF-16 LE/BE receive their canonical BOM
- `always`: Require the target encoding's canonical BOM; fails for encodings without BOM support
- `never`: Write no BOM (`none` is accepted as an alias and canonicalized to `never`)
- `preserve`: Preserve the existing file's BOM presence, using the canonical BOM for the selected target encoding; a new file has no BOM

**Example:**
```json
{
  "path": "/path/to/multilingual.data",
  "content": "title = \"città\"\r\n",
  "encoding": "utf-16-le",
  "bom": "auto"
}
```

**Response:**
```json
{
  "message": "Successfully replaced complete file contents at /path/to/multilingual.data with 36 bytes (encoding: utf-16-le, BOM: auto)",
  "encoding": "utf-16-le",
  "bomPolicy": "auto",
  "hasBOM": true,
  "bomType": "utf-16-le",
  "targetFingerprint": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "resultFingerprint": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
  "actualFingerprint": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
  "state": "committed",
  "changed": true,
  "applied": true
}
```

### edit_file

Prepare one exact edit of an existing text file **without persistent mutation**. `edit_file` accepts only `action: "preview"`; mutation is performed separately by `edit_file_apply` using the returned `previewId`. Supply either `edits` or one strict single-file unified `patch`, never both.

The preview uses the existing encoding/BOM/line-ending-aware edit pipeline, retains the exact prepared bytes plus stable file identity in a bounded process-local cache, and returns a 256-bit `previewId`, diff, target/result fingerprints, encoding metadata, effective backup policy, creation/expiry timestamps, and `changed`. Preparation performs no target write, permission change, persistent backup capture, adjacent temp creation, or `.bak` creation.

**Parameters:**
- `action` (required): exactly `preview`
- `path` (required): existing regular target file
- `edits` (conditionally required): operations with `oldText`, `newText`, and optional `similarity` from `0.50` to `1.0`
- `patch` (conditionally required): one strict unified diff for the target
- `encoding` (optional): explicit file encoding; otherwise conservative auto-detection applies
- `forceWritable` (optional): approval-bound permission intent retained for apply; preview itself never changes permissions
- `backupPolicy` (optional): omit to inherit `MCP_BACKUP_DEFAULT_POLICY`, set `required` for a normal persistent backup, or set `pinned` for an immutable protected backup; callers cannot weaken an operator default of `required`

A logical no-op still returns a capability so approval evidence is explicit, but it needs no backup store and later apply performs no backup or write. A changed preview whose effective backup policy is `required` or `pinned` fails before capability creation if the persistent backup store cannot admit the required pre-state.

The edit preview cache is bounded by `MCP_MAX_EDIT_PREVIEWS` (default `128`), `MCP_MAX_EDIT_PREVIEW_BYTES` (default `67108864`), and `MCP_EDIT_PREVIEW_TTL_SECONDS` (default `900`). Expiry, deterministic FIFO eviction, and process restart invalidate capabilities and close retained file identities.

```json
{
  "action": "preview",
  "path": "/project/file.go",
  "edits": [
    {"oldText": "func oldName()", "newText": "func newName()"}
  ],
  "backupPolicy": "required"
}
```

### edit_file_apply

Apply one previously prepared `edit_file` capability. The **complete input schema is only** `previewId`; path, edits, patch, encoding, `forceWritable`, backup policy, content, and all other overrides are rejected as unknown fields.

Applying an `edit_file` preview is **one-shot**: the capability is consumed before validation, so success, conflict, cancellation, write failure, and replay are all terminal outcomes.

- **Revalidation:** check current authorization, target path and stable file identity, the approved pre-state fingerprint, and the retained result fingerprint.
- **Backup policy:** when a changed edit requires `required` or `pinned`, durably capture and verify the exact approved original **before** changing permissions or replacing the target, then revalidate again. A pinned immutable backup is excluded from automatic per-target retention and ordinary garbage collection.
- **Commit:** persist the approved bytes through synchronized same-directory replacement, then verify the final fingerprint.
- **No-op and failure:** a no-op returns `applied: false` without changing bytes/metadata or creating a backup. A backup remains valid if the later target write fails; **no automatic rollback is promised**.

Apply reports actual target evidence rather than reusing the preview prediction: `state` is `unchanged`, `committed`, or `unknown`, and `actualFingerprint` records the observed post-state when classification succeeds. If an error occurs before mutation and the approved fingerprint remains present, `changed` is false. If replacement committed but a later durability/verification step fails, the response is `applied: false`, `state: committed`, `changed: true`, and `PARTIAL_COMMIT`. An unclassifiable post-error state also fails conservatively with `PARTIAL_COMMIT` rather than claiming that no change occurred.

```json
{
  "previewId": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
```
### patch_package

Review or prepare a strict versioned package of coordinated edits to existing regular files **without persistent mutation**. `patch_package` accepts only `inspect`, `dryRun`, and `verify`; package application is performed separately by `patch_package_apply`.

The input and every nested manifest object reject unknown JSON fields. `formatVersion` must be `patch-package-v1`, `fingerprintAlgorithm` must be `sha256`, and `fingerprintMode` must be `content-v1`. Targets must resolve to distinct authorized filesystem objects; duplicate spellings, symlink/junction aliases, hard links, creation, deletion, movement, and `/dev/null` patches are rejected.

**Read-only actions:**
- `inspect`: validate package structure, bounds, authorization, aliases, edit/patch shapes, and declared algorithms without reading target contents.
- `dryRun`: capture a coherent package pre-state, retain stable identities, prepare exact result bytes, verify the final unchanged source state, and return ordered diffs plus aggregate pre/post fingerprints and a one-shot 256-bit `previewId`. Omitted `manifest.backupPolicy` inherits `MCP_BACKUP_DEFAULT_POLICY`; `required` creates normal persistent backups and `pinned` creates protected backups that automatic retention/GC never selects. If changed targets require persistent backups, dry-run performs read-only quota/admission preflight and fails before capability creation when admission is impossible. It creates no backup object, manifest, target-adjacent staging file, or target mutation.
- `verify`: require `expectedResultFingerprint` for every target, read current fingerprints, and return ordered per-target and aggregate matches. A mismatch returns `CONFLICT`.

Each target declares `path`, exact `expectedFingerprint`, optional/verify-required `expectedResultFingerprint`, exactly one of `edits` or `patch`, and optional `encoding`/`forceWritable` with the same preparation semantics as `edit_file`.

Package capability bounds remain `MCP_MAX_PATCH_PACKAGE_BYTES`, `MCP_MAX_PATCH_PACKAGE_PREPARED_BYTES`, `MCP_MAX_PATCH_PACKAGE_PREVIEWS`, `MCP_MAX_PATCH_PACKAGE_PREVIEW_BYTES`, and `MCP_PATCH_PACKAGE_PREVIEW_TTL_SECONDS`; `MCP_MAX_BATCH_FILES`, `MCP_MAX_FILE_BYTES`, `MCP_MAX_OUTPUT_BYTES`, and backup-store quotas also apply.

```json
{
  "action": "dryRun",
  "manifest": {
    "formatVersion": "patch-package-v1",
    "fingerprintAlgorithm": "sha256",
    "fingerprintMode": "content-v1",
    "backupPolicy": "required",
    "targets": [
      {
        "path": "/project/a.go",
        "expectedFingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "edits": [{"oldText": "old", "newText": "new"}]
      }
    ]
  }
}
```

### patch_package_apply

Apply one prepared patch-package capability. The complete input is only `previewId`; manifest, paths, patches, content, encoding, permissions, and backup overrides are rejected.

Apply atomically consumes the capability, revalidates every target identity/fingerprint, then—when the effective policy is `required` or `pinned`—durably captures and verifies all changed pre-states **before any target-adjacent staging is created**. `pinned` marks each new immutable backup protected from automatic per-target retention and ordinary GC. Every target is revalidated after backup capture; only then are changed result bytes staged and manifest-order commits allowed to begin. No-op targets receive no backup and no write.

Multi-file replacement is deliberately not transactional. If a later file fails, already committed files are not rolled back automatically. Structured `PARTIAL_COMMIT` evidence classifies targets as `committed`, `unchanged`, or `unknown`, reports the failed target and actual fingerprints when available, and retains every durable `backupId` to support explicit recovery.

```json
{
  "previewId": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
```
## Directory Operations

The recursive tools `tree`, `search_files`, `grep_text_files`, and `fingerprint_paths` use one deterministic, cancellation-aware secure walker. Every traversed entry is resolved before it is exposed to the tool. `tree`, `search_files`, and `grep_text_files` skip links or reparse points that resolve outside allowed directories; `fingerprint_paths` fails closed because silently omitting a required entry would produce misleading state evidence. Directory links encountered below the requested root are not followed. Nested `.gitignore` files are respected by default; each must be a bounded regular file inside an allowed root, and callers may opt out explicitly with `respectGitignore: false`.

### list_directory

List files and directories with optional pattern filtering.

**Parameters:**
- `path` (required): Path to directory
- `pattern` (optional): Glob pattern like `*.pas` or `*.dfm` (default: `*`)
- `sortBy` (optional): `name` (default), `mtime`, or `size`
- `reverse` (optional): Reverse the selected deterministic order (default: false)

**Example:**
```json
{
  "path": "/path/to/project",
  "pattern": "*.pas"
}
```

**Response:**
```json
{
  "files": ["main.pas", "utils.pas", "forms.pas"]
}
```

### tree

Compact indented tree view optimized for AI/LLM consumption. It returns entries in deterministic lexical traversal order and skips links or reparse points that resolve outside the allowed directories.

**Parameters:**
- `path` (required): Root directory
- `maxDepth` (optional): Maximum recursion depth (0 = unlimited)
- `maxFiles` (optional): Maximum entries to return (default: 1000)
- `dirsOnly` (optional): Only show directories, not files
- `exclude` (optional): Array of patterns to exclude
- `showEncoding` (optional): Detect and display encoding per file (useful for auditing legacy codebases)
- `respectGitignore` (optional): Apply nested `.gitignore` rules (default: true)

**Example:**
```json
{
  "path": "/path/to/project",
  "maxDepth": 3,
  "exclude": ["node_modules", ".git"]
}
```

**Example with encoding:**
```json
{
  "path": "/path/to/legacy-project",
  "showEncoding": true,
  "exclude": [".git"]
}
```

**Response (with showEncoding):**
```json
{
  "tree": "src/\n  main.pas  [windows-1251]\n  utils.pas  [windows-1251]\nREADME.md  [utf-8]",
  "fileCount": 3,
  "dirCount": 1,
  "truncated": false
}
```

**Response:**
```json
{
  "tree": "src/\n  handler/\n    read.go\n    write.go\n  server.go\nREADME.md",
  "fileCount": 4,
  "dirCount": 2,
  "truncated": false
}
```

### get_file_info

Get metadata about a file or directory (size, timestamps, permissions).

**Parameters:**
- `path` (required): Path to file or directory

### filesystem_package

Prepare a bounded package of coordinated filesystem operations **without persistent mutation**. The complete top-level input is `formatVersion: "filesystem-package-v1"` plus a non-empty ordered `operations` array. Unknown top-level and operation-specific fields are rejected.

The v1 operation set is closed:

- `mkdir`: `type`, `path`; creates exactly one missing directory during apply and never behaves like `mkdir -p`.
- `createFile`: `type`, `path`, `contentBase64`; creates an exact raw-byte file from strict standard Base64. The bytes are not decoded as text and receive no encoding/BOM/line-ending transformation.
- `copyFile`: `type`, `source`, `destination`; copies one existing real regular file.
- `copyDirectory`: `type`, `source`, `destination`; copies one complete real directory tree, including hidden entries and `.git`, with no `.gitignore` filtering.
- `move`: `type`, `source`, `destination`; moves one existing real file or directory only when source and destination are provably on the same filesystem volume and the platform provides native no-replace rename semantics.
- `deleteFile`: `type`, `path`; deletes one existing real regular file only after its exact prepared pre-state has been durably captured in the persistent backup store.
- `deleteDirectory`: `type`, `path`; deletes exactly the complete prepared real directory tree, children before parents. Every regular file in the tree must be durably captured first; a newly appeared descendant causes a conflict rather than being swept into the deletion.

All destinations must be missing at preview and remain no-replace. A later operation may use an earlier `mkdir` only as its immediate destination parent; no other operation may consume content or paths created, copied, moved, or deleted earlier in the same package. Duplicate canonical paths, hard-link aliases across operations, recursive overlaps, link/reparse entries, special files, nested foreign filesystems, and ambiguous dependency graphs fail closed.

Preview canonicalizes and authorizes every operand against current allowed/protected roots, binds existing objects and parents to stable identity, records missing-destination ancestry, captures exact recursive scopes and content/state fingerprints, checks move volume identity, computes source/staging/output bounds, and performs side-effect-free persistent-backup admission preflight when deletion would lose regular-file bytes. It creates no destination, staging file, backup object, or backup manifest.

A successful preview returns a 256-bit process-local `previewId`, creation/expiry timestamps, ordered operation summaries, exact file/directory/byte counts, required backup count, and expected result fingerprints where applicable. The cache is bounded and one-shot; expiry, eviction, restart, replay, or an unowned token fails with `CONFLICT`.

```json
{
  "formatVersion": "filesystem-package-v1",
  "operations": [
    {"type": "mkdir", "path": "/project/generated"},
    {"type": "createFile", "path": "/project/generated/header.bin", "contentBase64": "AAEC/w=="},
    {"type": "copyFile", "source": "/project/a.txt", "destination": "/project/a.copy.txt"},
    {"type": "copyDirectory", "source": "/project/assets", "destination": "/project/assets.copy"},
    {"type": "move", "source": "/project/old.dat", "destination": "/project/new.dat"},
    {"type": "deleteFile", "path": "/project/obsolete.txt"},
    {"type": "deleteDirectory", "path": "/project/obsolete-tree"}
  ]
}
```

Dedicated bounds are `MCP_MAX_FILESYSTEM_PACKAGE_OPERATIONS`, `MCP_MAX_FILESYSTEM_PACKAGE_BYTES`, `MCP_MAX_FILESYSTEM_RECURSIVE_ENTRIES`, `MCP_MAX_FILESYSTEM_RECURSIVE_DEPTH`, `MCP_MAX_FILESYSTEM_AGGREGATE_BYTES`, `MCP_MAX_FILESYSTEM_STAGING_BYTES`, `MCP_MAX_FILESYSTEM_PACKAGE_PREVIEWS`, `MCP_MAX_FILESYSTEM_PACKAGE_PREVIEW_BYTES`, and `MCP_FILESYSTEM_PACKAGE_PREVIEW_TTL_SECONDS`. Global `MCP_MAX_FILE_BYTES`, `MCP_MAX_FINGERPRINT_ENTRY_DETAILS`, and `MCP_MAX_OUTPUT_BYTES` also apply. Path strings have a fixed v1 structural bound of 32768 UTF-8 bytes.

### filesystem_package_apply

Apply one prepared `filesystem_package` capability. The **complete input schema is only** `previewId`; the manifest and every operation field are rejected if resubmitted at apply time.

Apply consumes the capability before revalidation. It then revalidates authorization, canonical path evidence, source/object/parent identities, missing destinations, exact recursive scopes, fingerprints, and same-volume move evidence. Required destructive pre-states are captured as one persistent backup batch and verified before any target mutation, followed by another complete revalidation. All feasible `createFile`, `copyFile`, and `copyDirectory` content is staged and synced before the first target commit, then the package is revalidated again.

Operations commit strictly in manifest order with no-replace semantics. `move` uses native same-volume no-replace rename only; cross-filesystem emulation is `UNSUPPORTED`. Deletes remove only the exact prepared entries and never broaden scope after preview. Each committed operation is verified before the next begins.

The package is intentionally not transactionally rolled back. A failure before any target mutation returns the underlying stable error and current per-operation state. A failure after a target may have advanced returns `PARTIAL_COMMIT` with bounded structured evidence classifying operations as `committed`, `unchanged`, `partially_committed`, or `unknown`, plus the failed index, durable `backupIds`, and bounded cleanup-residue diagnostics where relevant.

```json
{"previewId":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
```

### search_files

Recursively search for files and directories matching a glob pattern through the secure walker. Results are selected with bounded top-K retention, so `maxResults` bounds memory even when sorting globally by modification time or size. Entries resolving outside allowed directories are skipped, and nested `.gitignore` files are respected by default.

**Parameters:**
- `path` (required): Root directory to search from
- `pattern` (required): Glob pattern (`*.txt` for current dir, `**/*.txt` for recursive)
- `excludePatterns` (optional): Array of patterns to exclude
- `respectGitignore` (optional): Apply nested `.gitignore` rules (default: true)
- `maxResults` (optional): Maximum number of retained results (default: 10000)
- `sortBy` (optional): `name`, `mtime`, or `size`; when omitted, preserve the historical deterministic traversal order
- `reverse` (optional): Reverse the selected deterministic order; with omitted `sortBy`, this selects reverse name order

**Example:**
```json
{
  "path": "/path/to/project",
  "pattern": "**/*.go",
  "excludePatterns": ["vendor", "node_modules"]
}
```

**Response:**
```json
{
  "files": [
    "/path/to/project/main.go",
    "/path/to/project/src/utils.go"
  ]
}
```

## Markdown document tools

### markdown_read

Use `markdown_read` to understand the structure of one Markdown document without changing it. It is designed for questions such as: "What sections are in this file?", "Which heading does this anchor point to?", "Where is this fenced code block?", or "Is this managed table of contents stale?" The returned `targetId` values can be passed directly to `markdown_edit`, so an agent does not need to guess from line numbers or repeat a text search.

Marksplice `v1.2.0` is the sole authority for Markdown structure, relationships, fragment resolution and continuity, TOC generation/final-state synchronization, construction, and canonical Markdown generation. Scripthold handles file authorization, encoding/BOM/line endings, limits, and MCP errors. Markdown is not analyzed by Source Intelligence.

`action` is one of `inspect`, `query`, `get`, `resolve`, `validate`, or `generate`. `path` is always required; `encoding` is optional and uses the normal deterministic Scripthold encoding pipeline when omitted.

- `inspect` requires a positive `limit` and returns bounded nodes, sections, relationships, heading anchors, fenced blocks, alerts, footnote references, and front-matter facts.
- `query` requires `query: "nodes" | "sections" | "relationships"` plus a positive `limit`. Node queries accept `kinds`; section queries accept `levels` and optional `within` source range.
- `get` requires snapshot-bound `targetId`; `includeSource` optionally returns the exact authored source range proven by Marksplice.
- `resolve` requires a non-empty `fragment` and fails closed unless Marksplice resolves it uniquely.
- `validate` requires exactly one of `fragment` or heading `targetId`; the latter reports managed-TOC recognition/staleness.
- `generate` requires `generate: "toc" | "canonical_markdown"`. Generated output is read-only derived output and is never used as an implicit edit path.

Every successful response includes the source fingerprint and Scripthold physical metadata. `targetId` is opaque and valid only for the exact source snapshot; stale or unknown targets are rejected rather than rebound heuristically. Result-producing collections and encoded responses are bounded by the request/configured limits.

### markdown_workspace

Use `markdown_workspace` for read-only questions that span multiple Markdown documents inside one authorized root. Discovery is explicit: `scan` walks `.md`/`.markdown` files under the selected root, while `follow` starts from explicit relative Markdown entry files and follows only the local relationships recognized by Marksplice. Scripthold keeps filesystem authorization and per-file deterministic decoding; Marksplice v1.2.0 owns discovery, relationship resolution, graph construction, and workspace validation.

`action` is `inspect`, `query`, `validate`, or `repairPreview`. Every request requires absolute `root`, a `discovery` object, and positive `limit`. `discovery.mode: "follow"` also requires non-empty relative `entries`; `scan` accepts no entries. There is intentionally no workspace-wide `encoding` override, so mixed-encoding documentation trees continue through per-file content/BOM detection.

Optional `maxDocuments`, `maxRelationships`, `maxBytes`, and `maxDepth` can only narrow operator ceilings. Server defaults are 1,000 documents and 25,000 relationships; hard ceilings are 10,000 and 250,000. Aggregate decoded bytes remain bounded by `MCP_MAX_FILESYSTEM_AGGREGATE_BYTES`, traversal depth by `MCP_MAX_FILESYSTEM_RECURSIVE_DEPTH`, each decoded file by the normal file/decoded-text limits, and the final response by `MCP_MAX_OUTPUT_BYTES`.

- `inspect` returns the deterministic document inventory and total document count.
- `query` accepts `edges`, `outgoing`, `backlinks`, `reachable`, or `related`. All except `edges` require a caller-visible workspace document key. Results expose document keys and Marksplice relationship facts; Marksplice `NodeID` values are never public workspace identities.
- `validate` accepts optional `roots` for orphan analysis and optional `managedTocs` entries of `{document, fragment}`. Each managed fragment must resolve uniquely to a heading through Marksplice. Validation returns ordinary workspace diagnostics plus stale/unrecognized generated-index diagnostics; when Marksplice proves one or more managed TOCs stale, `totalRepairs` and bounded `repairDocuments` project its conservative repair plan without exposing `ChangeSet` internals.
- `repairPreview` requires non-empty `managedTocs`, uses the same Marksplice validation authority, and may accept `backupPolicy: "required" | "pinned"`. If there are no repairs it returns no token. If the complete repair plan exceeds `limit` or the full review response exceeds the output budget, it fails without leaving a preview. A successful `repairPreview` returns one expiring `previewId` plus every target's document/path, source/result fingerprints, encoding/BOM/EOL facts, diff, and changed state; it never writes a file. Pass only that `previewId` to `markdown_apply`, whose `workspace` result reports per-document committed/unchanged/unknown state, backup IDs, counts, and any partial commit without automatic rollback.

Ordinary returned lists are deterministically cut at `limit` and set `truncated` when more results exist; an approvable `repairPreview` is never truncated. `workspace_budget_exceeded` reports Marksplice workspace budget exhaustion; invalid graph/workspace input fails closed with `invalid_workspace`. `markdown_workspace` never writes files directly: only `repairPreview` creates bounded process-local apply state, while `inspect`, `query`, and `validate` remain pure read-only observations.

```json
{
  "action": "repairPreview",
  "root": "/project/docs",
  "discovery": {"mode": "scan"},
  "managedTocs": [{"document": "README.md", "fragment": "#contents"}],
  "limit": 100,
  "backupPolicy": "required"
}
```

### markdown_create

Use `markdown_create` to **prepare and review a complete new Markdown document** at an authorized path that does not exist yet. Marksplice builds the semantic document; Scripthold binds the preview to the missing destination, its existing parent directory, the generated physical bytes, and the selected encoding/BOM policy. Previewing never creates the file.

A `markdown_create` request requires `path`; it may also supply `frontMatter`, an ordered `blocks` array, `encoding`, and `bom: "auto" | "always" | "never"`. With no explicit encoding, the configured new-file default applies. Marksplice generates canonical LF Markdown for a new document.

Supported typed construction includes:

- **Document blocks:** YAML/TOML front matter, headings, paragraphs, thematic breaks, nested blockquotes and GitHub alerts, fenced code, math blocks, and GFM tables with alignment.
- **Collections and definitions:** ordered/unordered and task lists; reference definitions; single-line or reviewed canonical multiline footnote definitions, including deferred definitions.
- **Inline content:** recursive text, code, emphasis/strong/strikethrough, links/images, autolinks, reference forms, footnote references, and math.

`rawMarkdown` is supported only where Marksplice itself accepts and reparses that construction fragment.

A successful preview returns `previewId`, canonical Markdown, result fingerprint, encoding/BOM/EOL metadata, and physical size. The generated physical result is bounded by the normal file/output limits. The immediate parent must already exist as an authorized real directory, and the target must still be absent. Creation has no persistent backup because there is no approved pre-state.

After review, pass only the returned `previewId` to `markdown_apply`. Apply reauthorizes the same destination, verifies that the parent is still the same directory and the target is still missing, then uses the filesystem no-replace primitive. If another actor creates the destination first, Scripthold reports a conflict and does not overwrite it.

```json
{
  "path": "/project/docs/guide.md",
  "frontMatter": {
    "format": "yaml",
    "fields": [{"key": "title", "value": "Guide"}]
  },
  "blocks": [
    {
      "type": "heading",
      "level": 1,
      "content": [{"type": "text", "text": "Guide"}]
    },
    {
      "type": "paragraph",
      "content": [{"type": "text", "text": "Generated through Marksplice."}]
    }
  ]
}
```

### markdown_edit

Use `markdown_edit` to **preview changes to an existing Markdown document** without writing it. First inspect the document with `markdown_read`, select the returned snapshot-bound `targetId`, and review the proposed result before sending its `previewId` to `markdown_apply`.

Typical changes include:

- **Document structure:** rename headings, change heading levels, edit paragraphs or list items, move sections or list subtrees, toggle tasks, and adjust table alignment.
- **Text and references:** update inline formatting, links, images, autolinks, reference or footnote definitions, and individual parser-proven reference occurrences.
- **Special content:** update front matter, fenced code, math, raw HTML comments/anchors, blockquotes or GitHub alerts; remove supported thematic breaks.
- **Generated indexes:** synchronize one explicitly managed table of contents with the final combined document.

A request accepts **1–64 compatible operations** against one source snapshot. Unsupported shapes, interfering changes, stale targets, and unsafe relationship changes are rejected. The operation-specific fields and restrictions follow; **this summary is not a substitute for those closed forms**.

#### Exact operation contracts

##### Document constructors, tables and headings

Sixty-one closed operation forms are available. Four targetless document-level constructors use `action: "create"` and no `targetId`: `front_matter` requires `format: "yaml" | "toml"`; `front_matter_field` requires non-empty `key` and `value`; `reference_definition` requires non-empty `label` and `destination` with optional non-empty `title`; and `footnote_definition` requires non-empty `label` and logical-LF `body`.

Marksplice owns insertion position, canonical syntax, line endings, uniqueness, candidate reparsing, and failure on unsupported layouts; independently prepared constructors that contend for the same insertion point remain subject to normal `ComposeChanges` rejection. Whole front-matter removal uses targetless `action: "remove"`, `subject: "front_matter"`, and no operation-specific fields; Marksplice owns recognition of the leading envelope, its separator bytes, candidate reparsing, and surviving semantics. Table alignment uses `action: "set"`, `subject: "table"`, and a snapshot-bound table `targetId`.

`part: "column_alignment"` requires zero-based `column` plus `alignment: "default" | "left" | "right" | "center"`; `part: "alignments"` requires one alignment value per table column and updates the delimiter row atomically. `markdown_read` table targets expose the current `alignments` vector so callers never need to infer it from Markdown source. Marksplice owns table shape, column-count validation, authored delimiter trivia, no-op behavior, candidate reparsing, and stale-source checks. Heading rename uses `action: "rename"`, `subject: "heading"`, a snapshot-bound `targetId` returned by `markdown_read`, and the new `text`.

Rename participates in the same final composed fragment-continuity proof as other edits, so a rename that would silently break or retarget an existing resolved local fragment fails closed unless the request explicitly retargets that relationship in the same composed result. Heading level change uses `action: "set"`, `subject: "heading"`, the same kind of `targetId`, and `level` from 1 to 6.

##### Tasks and inline formatting

Task checkbox state uses `action: "set"`, `subject: "task"`, the snapshot-bound task `targetId`, and required boolean `checked`; Marksplice changes only the checkbox state byte, and setting an already-satisfied task is a source-bound no-op that preserves authored marker style. Simple inline replacement uses `action: "replace"`, `subject: "code_span" | "emphasis" | "strong" | "strikethrough"`, the matching snapshot-bound `targetId`, and `text`; Marksplice replaces only the source-proven inline content, preserves the authored delimiters, and rejects unsupported source shapes or unsafe delimiter-conflicting replacements.

##### Links and images

Direct link replacement uses `action: "replace"`, an `inline_link` target with `part: "destination" | "label" | "title"`, an `image` target with `part: "destination" | "alt" | "title"`, or an `autolink` target without `part`, plus non-empty `text`. Existing-title replacement requires a title to exist and preserves its authored delimiters. Title insertion uses `action: "add"`, `subject: "inline_link" | "image"`, `part: "title"`, and non-empty `text`; it requires no current title and inserts canonical double-quoted title syntax.

Title removal uses `action: "remove"` with the same subjects and `part: "title"`, accepts no `text`, requires an existing title, and removes only the owned title syntax while preserving separator whitespace. Marksplice owns all state and replacement validation and preserves unrelated source.

##### Fenced code

Fenced-code body replacement uses `action: "replace"`, `subject: "fenced_code"`, `part: "body"`, a fenced-block `targetId`, and non-empty `text`; info mutation uses `action: "set"`, the same subject, `part: "info"`, and required `text`, where an empty string clears the info string.

`markdown_read inspect` returns `fencedBlocks` targets for every top-level fenced block, including empty bodies; when a normal `fenced_code` node target is also available, it is accepted for the same block. Marksplice preserves fence shape, trivia, and existing line endings, validates info strings, and rejects unsafe body replacements.

##### Reference definitions and occurrences

Direct reference-definition mutation uses the snapshot-bound `reference_definition` target returned by `markdown_read`.

Rename uses `action: "rename"`, the definition `targetId`, and non-empty `text`; Marksplice atomically updates the definition label plus every parser-proven bound reference link/image, promotes collapsed and shortcut references to full form when needed to preserve visible labels, and rejects invalid or colliding normalized labels. Destination replacement uses `action: "replace"`, `part: "destination"`, and non-empty `text`. Existing-title replacement uses `action: "replace"`, `part: "title"`, and non-empty `text`, and requires a current title.

Title insertion uses `action: "add"`, `part: "title"`, and non-empty `text`, requires no current title, and inserts canonical double-quoted title syntax. Title removal uses `action: "remove"` with `part: "title"`, accepts no `text`, and requires an existing title. Complete definition removal uses `action: "remove"`, `subject: "reference_definition"`, the definition `targetId`, and no `part` or content fields; Marksplice removes only an unused definition's owned physical line and rejects removal while any parser-proven full, collapsed, shortcut, or image reference still uses it.

Marksplice preserves authored destination/title wrappers and unrelated bytes and owns all state/replacement validation. Reference-occurrence retargeting uses `action: "retarget"`, `subject: "reference_occurrence"`, the snapshot-bound relationship `targetId` returned by `markdown_read` relationship queries, and non-empty `text` naming an existing reference definition. The opaque target is bound to the exact source fingerprint and Marksplice relationship facts, so a stale target cannot silently select a different occurrence after the document changes.

Scripthold resolves that target only to Marksplice's parser-proven source offset; Marksplice then proves the occurrence kind, resolves the requested uniquely normalized definition, preserves the visible label, promotes collapsed or shortcut syntax to full form when needed, validates the candidate, and owns no-op behavior. Relationship `sourceOffset` remains diagnostic metadata and is never accepted directly as edit authority.

##### Footnotes and front matter

Footnote-definition rename uses `action: "rename"`, `subject: "footnote_definition"`, the snapshot-bound definition `targetId`, and non-empty `text`; Marksplice atomically updates the definition label and every parser-proven bound footnote reference, keeps an identical authored label as a source-bound no-op, rejects an exact collision with another definition and labels containing `[` or `]`, and leaves non-bound textual occurrences unchanged.

Footnote-definition body replacement uses `action: "replace"`, `subject: "footnote_definition"`, the snapshot-bound definition `targetId`, and non-empty `text`; LF in `text` represents logical body line breaks, while Marksplice preserves the definition's source-proven EOL and continuation indentation, keeps an identical logical body as a source-bound no-op, and rejects empty or CR-containing replacements, unsupported source layout, wrong target kinds, or stale source.

Footnote-definition removal uses `action: "remove"`, `subject: "footnote_definition"`, the snapshot-bound definition `targetId`, and no content fields; Marksplice removes the complete owned definition container and preserves all bytes outside that range, including external `[^label]` occurrences. Those preserved occurrences may become unresolved after reparsing when they were bound to the removed definition; unrelated footnote definitions/references and pre-existing unresolved usages must survive unchanged.

Front-matter field replacement uses `action: "replace"`, `subject: "front_matter_field"`, the snapshot-bound field `targetId`, non-empty single-line `text`, and no `part`; YAML and TOML share the same Marksplice authority, which preserves the original envelope, line endings, quote/style syntax, comments, spacing, and unrelated source, while duplicate keys remain non-targetable and invalid replacements fail closed.

Front-matter field rename uses `action: "rename"` with the same subject/target shape plus non-empty `text` for the new canonical key; Marksplice preserves the value wrapper and line trivia, treats an identical key as a source-bound no-op, and rejects invalid or colliding keys. Front-matter field removal uses `action: "remove"`, the same subject and snapshot-bound `targetId`, and no content fields; Marksplice removes the complete promoted field physical line plus its EOL while preserving and reparsing the remaining YAML/TOML envelope and Markdown body.

##### HTML, math, blockquotes and alerts

Raw-HTML replacement uses `action: "replace"`, `subject: "html_comment" | "html_anchor"`, the matching snapshot-bound target, non-empty single-line `text`, and no `part`; Marksplice replaces only the promoted comment payload or quoted anchor id/name value, preserves wrappers/attribute style and surrounding source, keeps identical values as source-bound no-ops, and rejects unsafe comment terminators, quote-breaking anchor values, multiline content, or wrong target kinds. Math-expression replacement uses `action: "replace"`, `subject: "math_expression"`, the matching snapshot-bound target, non-empty `text`, and no `part`.

Inline dollar, dollar-backtick, and block-dollar forms use the `math_expression` target returned by `markdown_read query`; exact-info fenced `math` blocks reuse the existing `markdown_read inspect.fencedBlocks` target instead of creating a duplicate identity. Marksplice preserves the exact delimiter/container form, permits multiline payloads where the fenced-math contract permits them, and rejects unsafe delimiters, invalid multiline replacements for dollar forms, empty fenced replacements, wrong kinds, and stale source.

Blockquote content replacement uses `action: "replace"`, `subject: "blockquote"`, the snapshot-bound top-level blockquote `targetId`, non-empty `text`, and no `part`; Marksplice replaces the complete inner content only for non-alert blockquotes whose owned physical content lines share one source-proven marker prefix and EOL style, preserves that authored container shape, keeps identical content as a source-bound no-op, and rejects alerts, lazy-continuation/segmented ownership, mixed-EOL source, empty replacement, wrong kinds, and stale source.

Alert-kind mutation uses `action: "set"`, `subject: "alert"`, the underlying snapshot-bound blockquote `targetId`, and `text: "note" | "tip" | "important" | "warning" | "caution"`; Scripthold maps only that closed transport enum to Marksplice `AlertKind`, while Marksplice changes only the reviewed `[!KIND]` marker and preserves blockquote prefix, body, spacing, and line endings.

Alert-body replacement uses `action: "replace"`, `subject: "alert"`, the same underlying snapshot-bound blockquote `targetId`, non-empty `text`, and no `part`; Marksplice preserves the reviewed `[!KIND]` marker line and kind, re-emits multiline body content with the source-proven blockquote marker prefix and EOL style, keeps identical body content as a source-bound no-op, and rejects ordinary blockquotes, marker-only non-alert shapes, lazy-continuation or mixed-prefix/mixed-EOL bodies, empty replacement, wrong kinds, and stale source.

Thematic-break removal uses `action: "remove"`, `subject: "thematic_break"`, and only the snapshot-bound `targetId`; Marksplice removes exactly the promoted top-level physical line and reparses the candidate, rejecting nested/wrong-kind targets or removals that would merge neighboring text into a different paragraph structure. Blockquote removal uses `action: "remove"`, `subject: "blockquote"`, and only the snapshot-bound top-level blockquote `targetId`; Marksplice removes the complete owned container including its physical line endings.

Recognized GitHub alerts use the same underlying blockquote identity, so the same operation removes the alert container without a separate alert target namespace. This operation removes exactly the Marksplice-owned container, which can include CommonMark lazy-continuation lines that do not repeat a `>` marker; Scripthold does not infer blockquote ownership from line prefixes or add a competing Markdown rule.

##### Paragraphs, lists and sections

Paragraph insertion uses `action: "insert"`, `subject: "paragraph"`, an existing paragraph `targetId`, `position: "before" | "after"`, and `markdown` containing exactly one paragraph; the target is the structural anchor, so callers do not need to calculate line numbers or separators. Paragraph replacement uses `action: "replace"`, `subject: "paragraph"`, a paragraph `targetId`, and `markdown` containing exactly one paragraph.

Direct list-item replacement uses `action: "replace"`, `subject: "list_item"`, a list-item `targetId`, and `markdown` containing only the item's direct content; Marksplice preserves the existing list marker or number, indentation, child items, and surrounding bytes. Complete subtree replacement uses the same action/subject plus `part: "subtree"`; its `markdown` must be one complete compatible list-item subtree, and Marksplice preserves the target's external sibling shape and semantic parent while allowing the replacement subtree itself to change content and descendants.

List-item removal uses `action: "remove"`, `subject: "list_item"`, and only a list-item `targetId`; it removes the complete supported subtree and preserves surrounding authored bytes. List-item insertion uses `action: "insert"`, `subject: "list_item"`, a list-item anchor or parent `targetId`, `position: "before" | "after" | "child"`, and `markdown` containing one complete compatible list-item subtree. `after` inserts after the anchor's complete subtree, not only its root line. `child` appends a validated child subtree under the target parent, after any existing child subtrees.

Marksplice owns indentation, parent/child identity, list-shape compatibility, and fragment validation. Task fragments are valid list-item fragments. List-item movement uses `action: "move"`, `subject: "list_item"`, the source list-item `targetId`, a second list-item `anchorTargetId`, and `position: "before" | "after"`; Marksplice moves the complete supported source subtree, preserves authored numbering and descendants, and may reparent it to the anchor's immediate semantic parent. Adjacent already-satisfied moves are snapshot-bound no-ops, while overlapping ancestor/descendant pairs and incomplete supported subtrees fail closed.

Task items also have a list-item identity, so they can be removed, inserted beside, or moved through that identity; their separate `task` target is not interchangeable with it. Paragraph removal uses `action: "remove"`, `subject: "paragraph"`, and only the paragraph `targetId`. Section insertion uses `action: "insert"`, `subject: "section"`, an existing section `targetId`, `position: "before" | "after" | "child"`, and `markdown` containing exactly one valid section subtree. `before` and `after` require a sibling root at the anchor section's level; `after` means after the anchor's complete subtree, including nested subsections.

`child` appends one direct child subtree after the parent's existing descendants and requires its root heading at exactly the parent level plus one; an `h6` section cannot accept a child. Nested subsections inside the inserted subtree are allowed when Marksplice proves the fragment. Section replacement uses `action: "replace"`, `subject: "section"`, the section `targetId`, `part: "body" | "subtree"`, and `markdown`. `body` changes only the direct body and preserves the section heading plus nested subsections; structure-changing headings are rejected.

`subtree` replaces the complete promoted section, so its `markdown` must itself be a valid section subtree beginning with the replacement heading. Section removal uses `action: "remove"`, `subject: "section"`, and the section `targetId` returned by a `markdown_read` sections query; it removes that heading, its body, and every nested subsection in the section subtree.

Section movement uses `action: "move"`, `subject: "section"`, the source section `targetId`, a second section `anchorTargetId`, and `position: "before" | "after"`; source and anchor roots must be at the same heading level, the complete source subtree moves intact, and moving beside a same-level anchor under another parent may reparent that subtree. The separate `headingTargetId` is not interchangeable with the section `targetId`. Inline Markdown such as emphasis or links is allowed for paragraph insertion/replacement, while invalid paragraph or section fragments are rejected by Marksplice.

#### Composition and safety limits

The sixty-one forms do not accept each other's fields. `path` is required; `encoding` is optional; `backupPolicy` may be `required` or `pinned` and otherwise inherits the operator default.

For local-fragment safety, Scripthold validates the final composed change through Marksplice `LocalFragmentContinuity`. Previously resolved surviving relationships must remain on the same semantic target or be explicitly retargeted by the edit; silent target changes and missing, ambiguous, invalid, or unknown outcomes fail closed with `invalid_structure`. This final-state proof allows unrelated and compensating multi-operation edits without introducing Scripthold fragment parsing, slug rules, or cross-snapshot node identity.

Managed TOC synchronization uses `action: "sync"`, `subject: "toc"`, and only the snapshot-bound heading `targetId` already accepted by `markdown_read validate`. At most one `sync/toc` may appear in a preview and it may be combined with other operations. Marksplice `ComposeChangesAndSyncTOC` first proves the ordinary same-snapshot changes, then derives the managed TOC from their final candidate; direct managed-body edits or ambiguous/unprovable target correspondence fail closed. The successful result remains one source-bound preview with a one-shot `previewId` and diff. Preview does **not** write the target or create a persistent backup.

Markdown semantic text is canonical UTF-8. The current mutation bridge also preserves authorized UTF-16 LE/BE and UTF-32 LE/BE compatibility files, with or without BOM, after proving that the decoded source round-trips to the exact original bytes; existing mixed line endings are preserved. Other encodings fail safely until unchanged-byte stability is proven.

#### Workflow example

**Typical workflow:** call `markdown_read` to obtain the exact semantic `targetId` required by the operation, call `markdown_edit` to review the combined result, then pass the returned `previewId` to `markdown_apply`. Fenced-block mutations should use the `targetId` from `inspect.fencedBlocks` when complete coverage is required, because empty fenced blocks intentionally have no generic `fenced_code` node target.

```json
{
  "path": "/project/README.md",
  "operations": [
    {
      "action": "rename",
      "subject": "heading",
      "targetId": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      "text": "Installation"
    },
    {
      "action": "set",
      "subject": "heading",
      "targetId": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
      "level": 3
    },
    {
      "action": "replace",
      "subject": "paragraph",
      "targetId": "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
      "markdown": "Updated **important** guidance with [details](details.md)."
    },
    {
      "action": "insert",
      "subject": "paragraph",
      "targetId": "1111111111111111111111111111111111111111111111111111111111111111",
      "position": "after",
      "markdown": "A new paragraph placed after the selected paragraph."
    },
    {
      "action": "remove",
      "subject": "section",
      "targetId": "2222222222222222222222222222222222222222222222222222222222222222"
    }
  ],
  "backupPolicy": "required"
}
```

### markdown_apply

Use `markdown_apply` after a `markdown_edit` or `markdown_create` preview has been reviewed and approved. Its **only input is `previewId`**: callers cannot change the target, Markdown content, encoding, or backup policy during apply. This keeps the applied result identical to the reviewed preview.

For an edit preview, Scripthold checks that the target is still authorized, is still the same file, and still contains the exact approved source; it rechecks encoding/BOM facts and required backup policy before replacement. For a create preview, it verifies the same authorized path, unchanged parent-directory identity, target absence, and prepared result bytes before a no-replace creation. Creation has no persistent backup because no prior file existed.

The preview token is one-shot. Success, cancellation, conflict, write failure, or replay all consume it, so an old approval cannot be reused after circumstances change. The result reports what was actually observed after apply: `unchanged`, `committed`, or `unknown`, plus the actual fingerprint when available.

```json
{"previewId":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
```

## Source Intelligence

### source_symbols

Navigate bounded source declarations without reading every complete source file. The tool is read-only and exposes four strict operation variants under one schema: `outline`, `digest`, `find`, and fingerprint-bound `show`. All variants reject unknown or operation-illegal fields.

Scripthold has **100 active Source Intelligence providers**. The generated [Language Capabilities](docs/LANGUAGE_CAPABILITIES.md) matrix shows what each provider can actually analyze; its contents are checked against the native registry.

- **How analysis works:** Go uses the standard-library AST. Other providers use bounded native scanners, structural recognizers, or offset-preserving adapters. Composite providers retain host-file coordinates and analyze only supported embedded regions.
- **What coverage means:** Shared file extensions and language dialects may be ambiguous. Indexing is bounded and process-local; enabling a provider does not imply that every project or semantic relationship is supported. Ambiguous, partial, truncated, and unsupported results are reported rather than guessed.
- **What is not required:** No external compiler, language server, network lookup, persistent code index, or complete-source cache.

**Common navigation parameters:**
- `paths` (required for `outline`, `digest`, `find`): one or more authorized files/directories; directory traversal reuses the secure `.gitignore`-aware walker.
- `language` / `encoding` (optional): explicit routing/decoding evidence; otherwise conservative detection applies.
- `includes` / `excludes`, `respectGitignore`, `maxFiles`: bounded traversal filters.
- `kinds` (optional for `outline`/`find`): normalized symbol-kind filter without hiding unsupported file coverage.
- `includeSignatures` (optional for `outline`/`find`): retain bounded declaration signatures.

**Operations:**
- `outline`: returns deterministic path/source-ordered normalized declarations and per-file detection/fingerprint/coverage evidence.
- `digest`: returns compact declaration counts plus structural dependencies, relations, and composite regions; it does not return source bodies.
- `find`: requires `query`; `match` is `exact`, `prefix`, or `qualified`. Multiple retained matches set `ambiguous: true` rather than inventing a unique target.
- `show`: requires one `path`, `symbolId`, `sourceFingerprint`, `language`, and `encoding`. The file is re-read and re-analyzed; a stale fingerprint returns `CONFLICT`. `maxBytes` bounds the selected declaration text.

Public source coordinates use 1-based Unicode-scalar, half-open ranges (`unicode-scalar-1-based-half-open`) independent of the original file encoding. Symbol IDs and source fingerprints are deterministic lowercase SHA-256 values. Per-file analysis failures are independent; aggregate `coverageComplete` becomes false for skipped, unsupported, ambiguous, partial, or truncated coverage.

Dedicated limits use the `MCP_SOURCE_MAX_*` family: input paths, files, aggregate/file bytes, symbols, signature/show bytes, diagnostics, detector probes, nesting, concurrency, request seconds, and output bytes. Corresponding global file/batch/output ceilings remain effective and cannot be bypassed.

```json
{
  "operation": "find",
  "paths": ["/project/src"],
  "query": "Work",
  "match": "exact",
  "includeSignatures": true,
  "maxSymbols": 50
}
```

A typical follow-up uses the returned per-file `sourceFingerprint`, symbol `id`, language, and encoding:

```json
{
  "operation": "show",
  "path": "/project/src/main.go",
  "symbolId": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "sourceFingerprint": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
  "language": "go",
  "encoding": "utf-8",
  "maxBytes": 65536
}
```

### source_query

`source_query` is the compact read-only contract for structural source search, supported project relations, fingerprint-verified task-context assembly, and bounded process-local incremental index generations over normalized analyzer facts. Only structural search is exposed; decoded textual search is provided by `grep_text_files`. Index generation/fingerprint binding adds no persistent source-index storage.

The single-tool surface was selected instead of three near-duplicate `source_search`, `source_relations`, and `source_context` tools because all three share authorization, traversal, language/encoding filters, index binding, evidence, and resource policy, while three complete schemas exceeded the fixed connector-catalog budget. The public request remains strict: unknown top-level fields are rejected by the MCP schema, selector/index objects reject unknown nested fields, and the handler rejects known fields that are illegal for the selected operation.

**Common parameters:**
- `operation` (required): `search`, `relations`, or `context`.
- `paths` (required): authorized source files/directories. The configured `MCP_SOURCE_MAX_INPUT_PATHS` ceiling applies.
- `language`, `encoding`, `includes`, `excludes`, `respectGitignore`, `maxFiles` (optional): use the established source-intelligence selection/traversal policy.
- `index` (optional): bind the query to a retained process-local index `generation`, deterministic lowercase SHA-256 `fingerprint`, or both. `stalePolicy` is `reject` or `allow`; omitted/`reject` fails with `CONFLICT` for stale or unavailable evidence, while `allow` may select an exact retained historical metadata generation and reports `index.staleness: "stale"`.

Selectors are fingerprint-bound and use `kind: "path" | "symbol" | "position"`. Every selector carries `path` plus the current lowercase SHA-256 `sourceFingerprint`; a `symbol` selector additionally requires lowercase SHA-256 `symbolId`, while a `position` selector requires positive 1-based `line` and `column`. Invalid combinations are rejected rather than guessed.

**`search`:** requires non-empty `query` and `mode: "structural"`; optional `match` is `exact`, `prefix`, or `contains`. Search operates over normalized symbols, dependencies, and resolved structural relationships without rescanning raw source. Unsupported modes are rejected as invalid input; use `grep_text_files` for decoded text search. `kinds`, `evidence`, and `maxResults` are search-specific. Evidence strength uses only analyzer-proven `structural`, `scope-resolved`, and `project-resolved` grades; a result may never claim a stronger level than the native analyzer proves.

**`relations`:** requires `relation`, whose vocabulary is `dependencies`, `dependents`, `references`, `definitions`, `inheritance`, `implementations`, `trace`, `impact`, or `cycles`. The supported relations include dependency/dependent adjacency, resolved structural references/definitions, inheritance, proven implementations, bounded dependency trace/impact, and dependency cycles. Requests for unavailable relation kinds are rejected as invalid input. `trace` requires both `subject` and `target`; `cycles` accepts neither selector and does not accept `maxDepth`; the other relation kinds require `subject` and reject `target`. Relation queries may use `evidence`, `maxResults`, `maxNodes`, `maxEdges`, and—where meaningful—`maxDepth`.

Relationship **resolution state is separate from evidence strength**. The frozen states are `resolved`, `ambiguous`, `unresolved`, and `external`. This prevents an ambiguous structural fact from being represented as though `ambiguous` were a weaker evidence level. Indexed results expose generation, deterministic index fingerprint, and staleness as `current` or retained historical `stale`; `not-indexed` remains part of the frozen vocabulary for compatibility with non-indexed/earlier paths.

**`context`:** returns source excerpts and related declarations for verified targets, within a strict output budget.

- **Inputs:** one to 32 fingerprint-bound `targets` and positive `budgetBytes`. Optional `bodyPolicy` is `prefer` or `signatures-only`; `maxItems` and `maxDepth` impose further limits.
- **Selection:** planning uses one coherent project generation, prioritizing `target`, `enclosing`, `direct-dependency`, `reverse-or-type-relation`, then `deeper-relation`. Deeper relations have signatures only. Explicit symbol and position targets are all-or-error under byte/item pressure. With `prefer`, a complete declaration body is returned when it fits; otherwise the planner uses its bounded signature.
- **Ambiguity:** a position at a resolved project reference selects its definition candidates without guessing between ambiguous matches. Planning does not invent caller/callee relations from names, and does not use embeddings or persistent storage.
- **Freshness:** before returning any text, including with `stalePolicy=allow`, the handler reopens authorized current source and checks the authoritative fingerprint. Any difference from the planned snapshot returns `CONFLICT`.
- **Accounting:** `usedBytes` is the actual UTF-8 length of returned `ContextItem.text`. Coverage and truncation distinguish item, depth, budget, and representation limits.

These configurable ceilings are fail-closed: invalid, non-positive, overflowing, or above-hard-maximum environment values fall back only as defined by the current configuration policy. Client-provided limits can only lower the effective configured ceiling. Existing source input/file/byte/request/output limits continue to apply as well.

| Limit / environment | Default | Hard maximum |
|---|---:|---:|
| `MaxResults` / `MCP_SOURCE_MAX_RESULTS` | 10,000 | 100,000 |
| `MaxGraphNodes` / `MCP_SOURCE_MAX_GRAPH_NODES` | 5,000 | 50,000 |
| `MaxGraphEdges` / `MCP_SOURCE_MAX_GRAPH_EDGES` | 20,000 | 200,000 |
| `MaxGraphDepth` / `MCP_SOURCE_MAX_GRAPH_DEPTH` | 8 | 64 |
| `MaxContextBytes` / `MCP_SOURCE_MAX_CONTEXT_BYTES` | 1 MiB | 8 MiB |
| `MaxContextItems` / `MCP_SOURCE_MAX_CONTEXT_ITEMS` | 256 | 4,096 |
| `MaxIndexProjects` / `MCP_SOURCE_MAX_INDEX_PROJECTS` | 4 | 16 |
| `MaxIndexGenerations` / `MCP_SOURCE_MAX_INDEX_GENERATIONS` | 2 | 4 |

Query results retain the `unicode-scalar-1-based-half-open` source-coordinate contract where ranges are returned, explicit generation-bound coverage/truncation, evidence/resolution state, and coherent index generation/fingerprint/staleness. The process-local index reuses only path/content/analyzer-configuration-identical parsed facts, conservatively rebuilds project relationship metadata for each changed generation, bounds retained scopes/generations, and never stores complete source bodies. File-content or probe-to-analysis drift fails closed; `context` still reopens and fingerprint-verifies current source before returning text. No persistent index, source mutation, external parser/compiler/LSP process, or network activity is introduced.

## Workspace state

### fingerprint_paths

Stream one deterministic SHA-256 content fingerprint for explicit regular files and directory roots. The canonical `content-v1` record format includes the input root index, Unicode-NFC slash-separated relative path, entry type, byte length, and file-content SHA-256. Absolute root names, modification times, ownership, and platform-specific permission bits are excluded, so identical trees copied to different locations produce the same result when the ordered inputs and filtering options match. If one root contains distinct filesystem names that collapse to the same NFC canonical path, the request fails with `INVALID_INPUT` instead of producing an ambiguous record stream.

Directory roots use deterministic lexical traversal, exclude `.git` directories in every mode, and respect nested `.gitignore` rules by default. Set `respectGitignore: false` to include otherwise ignored working-tree files; `.git` internals remain excluded. The initial `content-v1` mode includes real directories and regular files only: in-root symlinks, junctions, and other reparse-point entries are neither followed nor included, while entries resolving outside allowed roots fail with `SYMLINK_ESCAPE`. File bytes are streamed in two complete passes; success requires the aggregate state to match, so concurrent content, directory, or filtering changes fail with `CONFLICT`.

`MCP_MAX_BATCH_FILES` bounds root count, `MCP_MAX_FINGERPRINT_ENTRIES` bounds total inspected files plus directories, `MCP_MAX_FINGERPRINT_ENTRY_DETAILS` bounds optional entry records, and `MCP_MAX_OUTPUT_BYTES` bounds the encoded result. Entry-detail truncation does not change the aggregate fingerprint.

**Parameters:**
- `paths` (required): Ordered array of explicit regular files or directory roots
- `respectGitignore` (optional): Apply nested `.gitignore` rules to directory roots (default: true)
- `includeEntries` (optional): Return bounded per-entry records (default: false)
- `maxEntryDetails` (optional): Requested entry-detail limit; requires `includeEntries: true` and cannot exceed the server limit

**Example:**
```json
{
  "paths": ["/path/to/project", "/path/to/explicit.lock"],
  "includeEntries": true,
  "maxEntryDetails": 100
}
```

**Response:**
```json
{
  "algorithm": "sha256",
  "mode": "content-v1",
  "fingerprint": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "rootCount": 2,
  "fileCount": 18,
  "directoryCount": 6,
  "totalBytes": 48291,
  "entries": [
    {
      "rootIndex": 0,
      "path": ".",
      "type": "directory",
      "size": 0
    },
    {
      "rootIndex": 0,
      "path": "README.md",
      "type": "file",
      "size": 4120,
      "sha256": "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
    }
  ],
  "entriesTruncated": true
}
```

### verify_state

Run an ordered batch of read-only verification checks through one strict schema. The tool does not require `MCP_ENABLE_EXECUTION`, never accepts a command string, and never invokes a shell. A failed expectation is a normal result with `passed: false`; an operational problem such as an inaccessible path, ambiguous encoding, missing Git executable, cancellation, or a limit violation is reported in that check's `errorCode` and increments `errorCount`.

Each item in `checks` must contain `type` plus exactly one matching object:

- `json`: validates the syntax of one decoded JSON file. `path` is required and `encoding` is optional. The raw and decoded document are bounded by `MCP_MAX_FILE_BYTES`.
- `text`: validates one decoded text file. `path` is required; optional expectations are `encoding`, `bom=any|none|present|utf-8|utf-16-le|utf-16-be|utf-32-le|utf-32-be`, `lineEndings=any|lf|crlf|mixed|none`, and `trailingWhitespace=any|none|present`. Trailing-space diagnostics are capped at 1000 records.
- `gitDiff`: runs a fixed direct `git diff --check` invocation in `repositoryRoot`, with literal pathspecs, fsmonitor disabled, no external diff/textconv helpers, and optional `paths` placed after `--`. Absolute or escaping paths are rejected. `timeoutSeconds` defaults to 30 and cannot exceed 60.
- `fingerprint`: compares the shared `content-v1` aggregate for ordered `paths` with `expectedFingerprint`; optional `respectGitignore` has the same default-on behavior as `fingerprint_paths`.

Git receives closed stdin, bounded stdout/stderr, cancellation and process-tree termination, a filtered environment, disabled prompts, optional locks and lazy fetches, disabled system/global configuration, fixed locale, and no external diff or text-conversion helpers. The repository root is path- and identity-revalidated immediately before launch. This reduces but cannot eliminate the final path-based process-launch race.

`MCP_MAX_BATCH_FILES` bounds the number of checks and each nested path list, `MCP_MAX_LINE_BYTES` bounds decoded lines, `MCP_MAX_FINGERPRINT_ENTRIES` bounds fingerprint traversal, and `MCP_MAX_OUTPUT_BYTES` bounds the complete structured and text response.

**Example:**

```json
{
  "checks": [
    {
      "type": "json",
      "json": {
        "path": "/project/config.json",
        "encoding": "utf-8"
      }
    },
    {
      "type": "text",
      "text": {
        "path": "/project/legacy.data",
        "encoding": "utf-16-le",
        "bom": "utf-16-le",
        "lineEndings": "crlf",
        "trailingWhitespace": "none"
      }
    },
    {
      "type": "gitDiff",
      "gitDiff": {
        "repositoryRoot": "/project",
        "paths": ["README.md", "src/main.go"],
        "timeoutSeconds": 30
      }
    },
    {
      "type": "fingerprint",
      "fingerprint": {
        "paths": ["/project/src"],
        "expectedFingerprint": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
      }
    }
  ]
}
```

**Response summary:**

```json
{
  "passed": false,
  "checkCount": 4,
  "passedCount": 2,
  "failedCount": 1,
  "errorCount": 1,
  "results": [
    {
      "index": 0,
      "type": "json",
      "passed": true,
      "encoding": "utf-8"
    },
    {
      "index": 2,
      "type": "gitDiff",
      "passed": false,
      "exitCode": 2,
      "stdout": "README.md:12: trailing whitespace."
    },
    {
      "index": 3,
      "type": "fingerprint",
      "passed": false,
      "errorCode": "SYMLINK_ESCAPE",
      "error": "..."
    }
  ]
}
```

## Backups

### backup_store

Read and review the optional persistent backup store, or prepare restore/GC capabilities, **without persistent mutation**. `backup_store` is read-only; restore and GC application are separate tools. The tool remains registered when the store is disabled so operators can inspect policy state consistently.

**Actions:**
- `status`: accepts only `action`; reports enabled/health/generation/counts/limits/issues plus the effective operator `defaultPolicy`. When `MCP_BACKUP_STORE_DIR` is unset it returns `enabled: false`; an operator default of `required` is still visible and changed mutation previews then fail closed until a store is configured.
- `list`: newest-first authorized records with optional `cursor`, `limit`, `targetPath`, and `pinned`. Cursors remain authenticated, filter/policy/generation-bound, and visibility is revalidated on every page.
- `history`: requires an authorized `targetPath` and accepts the same paging/filter fields as `list`; it returns only versions for that target.
- `inspect`: requires `backupId`; authorizes the manifest target and fully verifies the immutable object before returning metadata. Object bytes and internal store paths are never exposed.
- `compare`: requires `backupId` and optionally `otherBackupId`. Without the second ID it compares the verified backup with the current authorized target. With two IDs it requires versions of the same authorized target and verifies both objects. Fingerprints/equality are always returned; a unified diff is added only when both sides are within the bounded text-diff limit and safely decodable. Binary/oversized content still receives verified fingerprint evidence without a fabricated diff.
- `audit`: `quick|full` bounded structural/object verification; never repairs or deletes data.
- `restorePreview`: requires `backupId`; authorizes only the immutable manifest's original target, verifies the source object, binds current missing/existing identity and fingerprint, and read-only preflights the mandatory safety backup for an existing target. It returns a 256-bit expiring `previewId`, fingerprints, object size, verification state, and optional bounded diff. No staging file, backup object, manifest, permission change, or target mutation is created.
- `gcDryRun`: creates a deterministic generation-bound, 256-bit expiring GC capability from an authoritative read-only plan. Pinned manifests, active restore sources, and referenced objects remain protected; no record/object is moved or deleted.

`MCP_BACKUP_DEFAULT_POLICY=disabled|required` sets the default for approval-bound content mutations: `edit_file`, `markdown_edit`, `patch_package`, `manage_bom`, and `convert_encoding`. The default is `disabled`.

- **Request override:** callers may request `required` to retain a persistent backup or `pinned` to protect an immutable backup from ordinary automatic deletion. They cannot weaken an operator-configured `required` policy. No-op mutations create no backup.
- **Per-target retention:** `MCP_BACKUP_MAX_VERSIONS_PER_TARGET` defaults to `64`. This is a **post-capture retention target**, not a reason to reject a new backup: after a newer unpinned backup is durable, the oldest eligible unpinned version is rotated as needed.
- **Separate contracts:** restore of an existing target still requires its own mandatory safety backup. Garbage collection does not capture public file content.

Restore/GC capabilities use `MCP_BACKUP_PLAN_TTL_SECONDS`; each cache has fixed bounded entry/state limits. `MCP_MAX_OUTPUT_BYTES` bounds responses. Backup target paths appear only where current-root authorization permits them; GC candidate output remains path-free.

```json
{"action":"history","targetPath":"/project/config.json","limit":25}
```

```json
{"action":"compare","backupId":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
```

### backup_restore_apply

Apply one `restorePreview` capability. The complete input is only `previewId` and every apply attempt consumes it before revalidation. Apply revalidates current authorization, immutable source manifest/object identity and digest, target identity/fingerprint or missing state, and prepared result bytes.

For an existing target, the mandatory `sourceOperation=restore` safety backup is durably captured and verified **before target-adjacent restore staging is created**; the target is revalidated again after capture. A missing target receives no safety backup and is installed with no-replace semantics. Final bytes are fingerprint-verified. Errors preserve `safetyBackupId` and bounded actual-state evidence; there is no automatic transactional rollback promise.

```json
{"previewId":"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"}
```

### backup_gc_apply

Apply one `gcDryRun` capability. The complete input is only `previewId`; the token is consumed before revalidation. The store reconstructs the plan at the original policy timestamp and rejects any generation, pin, manifest, object, active-restore, reservation, or reference-count drift before deletion. Selected manifests are moved to typed trash before verified now-unreferenced objects. The derived index is refreshed after durable partial outcomes; trash cleanup is best effort and reported. GC never selects pinned backups, never mutates public targets, and does not claim secure deletion or automatic rollback.

```json
{"previewId":"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"}
```

### backup_delete

Explicitly delete one selected backup by exact `backupId`. This is a dedicated destructive tool rather than a `backup_store` action, so the review surface remains truthfully read-only. The target recorded by the manifest must still be authorized by current roots. The selected manifest may be pinned: pinned backups are protected from automatic per-target retention and ordinary GC, but this explicit operation is the intentional removal authority.

Deletion is manifest-first and reference-safe. If the object is still referenced by another live backup or retained by an active restore, the object is preserved. An active restore using the selected manifest causes `CONFLICT` before deletion. Typed trash and derived-index recovery follow the same crash-consistency rules as GC.

```json
{"backupId":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
```
## Text search

### grep_text_files

Search decoded text incrementally using one regex `pattern` or a `patterns` array combined with OR semantics. Automatic detection uses the conservative registry-aware trust policy; ambiguous non-empty input requires an explicit `encoding`, so callers can still search explicit-only or otherwise ambiguous files by naming their codec. Directory inputs use the `.gitignore`-aware secure walker. Content mode preserves deterministic traversal order and bounded context queues; path/count modes scan each selected file without letting an early high-match file hide later files. `offset + maxMatches` is bounded by `MCP_MAX_MATCHES`, and retained match output remains within `MCP_MAX_OUTPUT_BYTES`.

Every search mode reports its coverage, so a partial result cannot silently masquerade as complete.

- **Counters:** `filesSearched` counts the deterministic candidate set. `filesScanned` counts candidates processed without an individual failure before stopping; `filesSkipped` counts encountered per-file failures.
- **Completeness:** `coverageComplete` is false if a file was skipped or the scan stopped before all candidates were processed, including output truncation.
- **Failure details:** `skippedFiles` retains a deterministic prefix of up to 64 entries with `path`, `error`, stable `errorCode`, and optional `encodingErrorCode`. Its separate byte cap derives from the overall output budget; match, path, and count results take priority over duplicate error details.
- **Further omissions:** `skippedFilesTruncated` and `skippedFilesOmitted` reveal omitted failures. Encoding and I/O errors on one file do not discard other valid results, but cancellation and output-limit errors terminate the request.

**Parameters:**
- `pattern` (conditionally required): One regular expression
- `patterns` (conditionally required): Array of regular expressions combined with OR semantics; may be used with `pattern`
- `paths` (required): Array of file or directory paths to search
- `caseSensitive` (optional): Case-sensitive matching (default: true)
- `contextBefore` (optional): Number of lines to show before each match
- `contextAfter` (optional): Number of lines to show after each match
- `maxMatches` (optional): Maximum total matches to return (default: 1000)
- `include` / `exclude` (optional): Backward-compatible single glob filters
- `includes` / `excludes` (optional): Arrays of glob filters
- `encoding` (optional): File encoding (auto-detected if omitted)
- `outputMode` (optional): `content` (default), `files_with_matches`, or `count`
- `matchesOnly` (optional): In content mode return only the regex substring in `text`
- `offset` (optional): Zero-based result offset for paging
- `respectGitignore` (optional): Apply nested `.gitignore` rules for directory inputs (default: true)

**Example:**
```json
{
  "pattern": "func\\s+\\w+",
  "paths": ["/path/to/project"],
  "include": "*.go",
  "contextBefore": 1,
  "contextAfter": 2,
  "maxMatches": 100
}
```

**Response:**
```json
{
  "matches": [
    {
      "path": "/path/to/project/main.go",
      "line": 15,
      "column": 1,
      "text": "func main() {",
      "before": ["package main"],
      "after": ["    fmt.Println(\"Hello\")", "}"],
      "encoding": "utf-8"
    }
  ],
  "totalMatches": 1,
  "filesSearched": 5,
  "filesScanned": 5,
  "filesMatched": 1,
  "filesSkipped": 0,
  "coverageComplete": true,
  "truncated": false
}
```

## Encoding Tools

### detect_encoding

Detect a file's encoding and report its confidence. Scripthold uses **file bytes and decoded-text evidence**, never a filename or extension. Uncertain non-empty input is reported as `ambiguous: true` rather than assigned a guessed encoding.

- **Unicode:** a BOM is authoritative. Without one, the Unicode candidate must have strong structural evidence. Even syntactically valid UTF-8 is rejected as binary if it contains too many control characters.
- **Empty input:** returns assumed `utf-8` with confidence `0` and `assumed: true`.
- **Legacy encodings:** a non-empty candidate must meet evidence thresholds, registry closure, strict decoding, decoded-text quality, and known-confusion checks.
- **Stateful and similar encodings:** HZ-GB-2312, ISO-2022-JP, and ISO-2022-KR additionally require verified syntax, detector agreement, and non-ASCII decoded evidence. GB18030 four-byte sequences exclude GBK but do not distinguish the generic GB18030 name from the exact GB18030:2022 revision.
- **Modes:** `sample`, `chunked`, and `full` use the same trust rules, including syntax state spanning chunk boundaries.

**Parameters:**
- `path` (required): Path to the file
- `mode` (optional): Detection mode
  - `sample` (default): Read begin/middle/end samples - fast, good for most files
  - `chunked`: Stream all chunks, preserving UTF-16 code-unit state and aggregating legacy evidence - thorough but slower
  - `full`: Read entire file - most accurate but uses more memory

**Example:**
```json
{
  "path": "/path/to/file.pas",
  "mode": "chunked"
}
```

**Response:**
```json
{
  "encoding": "windows-1251",
  "confidence": 95,
  "hasBOM": false
}
```

### convert_encoding

Prepare exact encoding-conversion bytes for one file or a bounded batch **without persistent mutation**. `convert_encoding` requires `dryRun: true`; application is performed separately by `convert_encoding_apply` using the returned capability. Every target must be fully representable and prepared successfully before a capability is returned.

**Parameters:**
- `path` or `paths` (exactly one form): one file or a bounded ordered batch
- `from` (optional): explicit source encoding; otherwise conservative detection applies
- `to` (required): target encoding
- `bom` (optional): `auto` (default), `always`, `never`, or `preserve`; `none` is accepted as a compatibility alias for `never`
- `dryRun` (required): exactly `true`
- `backup` (optional): bind creation/replacement of the adjacent `.bak` file to the later apply; preview itself never creates it
- `backupPolicy` (optional): omit to inherit `MCP_BACKUP_DEFAULT_POLICY`, set `required` for normal persistent capture, or `pinned` for protected persistent capture

Preview retains the **exact converted bytes** plus target identities/fingerprints in one global bounded capability cache shared with BOM mutations. The cache is controlled by `MCP_MAX_BYTE_MUTATION_PREVIEWS` (default `32`), `MCP_MAX_BYTE_MUTATION_PREVIEW_BYTES` (default `268435456`), and `MCP_BYTE_MUTATION_PREVIEW_TTL_SECONDS` (default `900`). Capability kind is bound, so a BOM token cannot be used by encoding apply and vice versa. Expiry, eviction, restart, and replay invalidate the token.

A changed preview with effective persistent policy `required` or `pinned` performs only read-only backup admission preflight. A no-op requires no store. Unsupported characters, ambiguous/invalid input, oversized results, aliases, and batch conflicts fail before capability creation and before any `.bak`, temp file, backup manifest, or target write exists.

```json
{
  "path": "/project/data.txt",
  "from": "utf-8",
  "to": "utf-16-le",
  "bom": "auto",
  "backup": true,
  "dryRun": true,
  "backupPolicy": "required"
}
```

### convert_encoding_apply

Apply one prepared conversion capability. The complete input is only `previewId`. Apply consumes it first, revalidates all target identities/fingerprints and retained result fingerprints, and—when required—durably captures/verifies every changed persistent pre-state before the first file write. Targets are revalidated after backup capture.

Each changed file is then replaced with the exact retained bytes; `backup: true` creates/replaces that target's adjacent `.bak` through the existing transactional single-file backup path. A logical no-op creates neither persistent nor adjacent backup and performs no write. Every committed target is fingerprint-verified.

Batch replacement remains sequential rather than transactionally atomic. If a later target fails, prior committed targets are not automatically rolled back; structured output reports `partialCommit`, `committedCount`, per-target errors/fingerprints, persistent `backupId`s, and adjacent backup paths where applicable.

```json
{"previewId":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
```
### detect_line_endings

Detect line ending style (CRLF/LF/mixed) through the shared incremental decoder, and find lines with inconsistent endings. This works across all 168 registered encodings. Uniform files require one pass; mixed files use a second digest-verified pass that retains only minority line numbers. `MCP_MAX_LINE_BYTES` bounds each decoded line and `MCP_MAX_OUTPUT_BYTES` bounds the returned list. Ambiguous non-empty input requires an explicit encoding.

**Parameters:**
- `path` (required): Path to the file to analyze
- `encoding` (optional): File encoding; auto-detected if omitted. Use an explicit value for ambiguous legacy encodings.

**Example:**
```json
{
  "path": "/path/to/extensionless-file",
  "encoding": "utf-16-le"
}
```

**Response:**
```json
{
  "style": "mixed",
  "totalLines": 150,
  "inconsistentLines": [45, 78, 123]
}
```

**Style values:**
- `crlf`: All lines use Windows line endings (\\r\\n)
- `lf`: All lines use Unix line endings (\\n)
- `mixed`: File has both CRLF and LF endings - `inconsistentLines` lists lines with minority style
- `none`: File has no line endings (single line or empty)

**Detection note:** encoding is inferred from bytes and decoded-content evidence, not from file extensions. UTF-16 LE/BE is auto-detected from a BOM or from conservative structural validation; use an explicit encoding when BOMless evidence is short, malformed, or endian-ambiguous.

### change_line_endings

Stream line-ending conversion to LF or CRLF while preserving the original encoding, BOM state, and every byte not belonging to a line-ending sequence. Shared path, encoding, BOM, and mode validation runs before the specialized conversion; an explicit encoding that conflicts with a Unicode BOM fails before mutation. The bounded transformer preserves CRLF pairs and standalone CR across chunk boundaries, handles UTF-16 LE/BE code units separately, and stages output directly on disk. Changed output uses synced atomic replacement with concurrent-modification detection. Use after `detect_line_endings` to fix mixed or wrong line endings. No-op if the file already uses the target style and preserves the file modification time.

**Parameters:**
- `path` (required): Path to the file
- `style` (required): Target line ending style (`"lf"` or `"crlf"`)
- `encoding` (optional): File encoding; auto-detected if omitted. Use an explicit value for ambiguous legacy encodings.

**Example:**
```json
{
  "path": "/path/to/extensionless-file",
  "style": "lf",
  "encoding": "utf-16-le"
}
```

**Response:**
```json
{
  "message": "Converted /path/to/extensionless-file from crlf to lf (3 lines changed)",
  "originalStyle": "crlf",
  "newStyle": "lf",
  "linesChanged": 3
}
```

### manage_bom

Detect BOM state or prepare an exact BOM mutation **without persistent mutation**. Mutating add/strip operations return a preview capability and are applied separately by `manage_bom_apply`.

**Actions:**
- `detect`: requires `path` and performs a bounded prefix read only.
- `addPreview`: requires `path` and BOM-capable `encoding` (`utf-8`, UTF-16 LE/BE, or UTF-32 LE/BE); prepares exact bytes and returns a capability.
- `stripPreview`: requires `path`; removes a detected BOM in the retained result. When no BOM exists it prepares an explicit no-op capability.

`backupPolicy` may be omitted to inherit `MCP_BACKUP_DEFAULT_POLICY`, set to `required`, or set to `pinned` for `addPreview`/`stripPreview`; `detect` accepts no mutation-policy fields. Preview retains exact bytes, stable identity, pre/result fingerprints, BOM metadata, and a 256-bit token in the shared byte-mutation cache described under `convert_encoding`. It creates no staging file, persistent backup, permission change, or target mutation. A required no-op needs no backup store.

```json
{"path":"/project/file.php","action":"stripPreview"}
```

### manage_bom_apply

Apply one prepared BOM capability. The complete input is only `previewId`. The token is kind-bound and consumed before revalidation; stale identity, same-content path replacement, target fingerprint drift, expiry, replay, or a token from another mutation class returns `CONFLICT`.

For a changed result with effective policy `required` or `pinned`, the exact approved pre-state is durably captured and verified before replacement and the target is revalidated again. A `pinned` pre-state is protected from automatic per-target retention and ordinary GC. The exact retained bytes are then atomically replaced and final fingerprint is verified. A no-op reports `applied: false`, creates no backup, and performs no write. No automatic rollback is promised after a durable backup if a later write fails.

```json
{"previewId":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
```
### list_encodings

Returns all 168 currently supported encodings with a stable canonical name, aliases, description, read/write capability, automatic-detection eligibility, explicit-only status, Unicode classification, BOM capability, and whether the `auto` BOM policy emits a BOM. Direct compatibility aliases plus applicable pinned IANA/WHATWG aliases are normalized only when they resolve to an already registered codec. Explicit codec support is intentionally broader than automatic detection: ambiguous legacy families remain explicit-only unless independent structural evidence satisfies the hardened trust path; HZ-GB-2312, ISO-2022-JP, and ISO-2022-KR require verified stateful evidence.

## Service and deferred operations

### list_allowed_directories

Returns directories the server is allowed to access. If empty, add paths as args in config.

### check_for_updates

Checks the latest GitHub release of the `zoster81/scripthold` fork and returns the current version, latest version, and an update message when applicable.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `force` | boolean | no | When `true`, bypasses the cached result and performs a fresh request |

Without `force`, successful results and offline failures are cached for 30 minutes to avoid repeated GitHub API calls. Cache reads are bounded and replacement uses a temporary file plus atomic rename; newly created Unix directories and files request `0700` and `0600` modes. Oversized, malformed, future-dated, or foreign-source entries are ignored. Semantic-version comparison honors prerelease precedence, so a stable release supersedes an equal-numbered `-internal-*` build. A background update check also runs once when the MCP server initializes.

The checker is notification-only: it never downloads, replaces, installs, or restarts the MCP server. It requires at least one published GitHub Release in the fork; if the fork has no release, the GitHub endpoint returns no latest version and the checker remains silent.

### deferred_operation

Observe or cancel work represented by a high-entropy `operationId`, and retrieve completed results in bounded UTF-8 segments. The same compact compatibility surface serves two reliability paths:

- **Oversized completed responses:** every tool is checked against a conservative encoded-response safety budget before the transport boundary. An otherwise completed response that is too large is retained in a bounded process-local response store and the original call returns a compact handle instead of attempting one oversized response. This path never re-executes the tool.
- **Durable long-running read-only operations:** when `MCP_DEFERRED_STORE_DIR` is configured, eligible calls are durably admitted before expensive execution begins and are owned by an independent helper process rather than the frontend request. The initial eligible set is `fingerprint_paths`, `grep_text_files`, `search_files`, `tree`, and `source_symbols`. `source_query` remains synchronous because its returned process-local index binding must stay reusable by subsequent frontend requests.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `operation` | `get` or `cancel` | yes | Observe/read a handle, or request cooperative cancellation of a durable running operation. |
| `operationId` | string | yes | Non-enumerable 256-bit-random handle returned by the original call. |
| `offset` | integer | no | UTF-8 byte offset returned as `nextOffset` by the preceding completed-result segment; defaults to `0`. |
| `limitBytes` | integer | no | Requested result segment size; values above the configured chunk ceiling are clamped. |

`get` reports the operation state plus `started`, `exposed`, `resultAvailable`, `originalIsError`, stable `errorCode`, result byte counts, and `pollAfterMs` while durable work remains active. For a completed result, continue with `offset=nextOffset` until `complete=true`; the retained bytes encode the finalized MCP tool result and structured output, preserving tool-level `isError` semantics rather than turning a completed tool error into an infrastructure failure.

Durable operation state lives in a separate owner-only store with bounded queue, concurrency, runtime, retention, terminal count, result bytes, and aggregate bytes. Current allowed roots are revalidated before recovered execution and before result retrieval. A pre-start stale operation may be relaunched after recovery; once the durable `started` marker exists, executor loss becomes `interrupted` and the invocation is never replayed automatically. `cancel` is meaningful only for durable operations; oversized-response handles are already complete and have no background execution to cancel.

The global `MCP_CALL_MAX_SYNC_SECONDS` cooperative deadline applies to **all public tools**, including tools that cannot run independently.

When the durable deferred-operation store is configured, clients on protocol `2026-07-28` or later may opt into `io.modelcontextprotocol/tasks`. Eligible slow `tools/call` requests then expose their existing durable operation through the native task methods:

- `tasks/get` observes progress and results; `tasks/cancel` requests cooperative cancellation.
- `tasks/update` accepts client input responses for a known task, but the current deferred engine **never enters `input_required`**. It validates the required `inputResponses` field, ignores keys with no outstanding response request, and does not mutate execution state.
- `deferred_operation` remains available for legacy sessions and for oversized **already completed** responses. Those oversized responses are not native tasks.

## Durable Task Execution

Scripthold executes shell commands and supported scripts as persistent asynchronous tasks. The MCP request records work and returns immediately; an independent supervisor, worker, and per-task executor own the queue and process lifecycle. See [Durable task execution](docs/DURABLE_TASKS.md) for the persistence, recovery, security, and retention contract.

Execution is disabled by default. `MCP_ENABLE_RUN_SCRIPT=1` authorizes `task_run` with `kind=script`, `MCP_ENABLE_SHELL=1` authorizes `kind=shell`, and `MCP_ENABLE_EXECUTION=1` authorizes both. Streamable HTTP additionally requires `MCP_HTTP_ENABLE_EXECUTION=1`.

### task_run

Durably enqueue one task. `idempotencyKey` is mandatory; an identical retry returns the original task, while a different request using the same key fails. Admission observes request cancellation while waiting for the shared task-store control lock and rechecks cancellation after lock acquisition, so a request already cancelled before durable admission does not later create a queued task.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `kind` | `shell` or `script` | yes | Selects the distinct authorization and validation path. |
| `idempotencyKey` | string | yes | Stable caller-generated key for retry safety. |
| `name` | string | no | Human-readable task name; duplicates are allowed. |
| `description` | string | no | Bounded operator-facing description. |
| `tags` | string[] | no | Bounded exact metadata tags. |
| `lockKeys` | string[] | no | Tasks sharing any key do not overlap. |
| `cwd` | string | no | Allowed working directory; defaults to the script parent or first allowed root. |
| `command` | string | shell only | Unrestricted command text. |
| `shell` | string | no | Logical shell name validated before admission. Windows: `powershell`, `pwsh`, `cmd`; Unix: `sh`, `bash`, `pwsh`. Executable filenames/paths such as `powershell.exe` are not shell identifiers. |
| `path` | string | script only | Allowed regular `.ps1`, `.bat`, `.cmd`, `.py`, `.js`, `.mjs`, `.cjs`, `.sh`, `.exe`, or `.com` file. |
| `args` | string[] | no | Direct script arguments without concatenation. |
| `maxRuntimeSeconds` | integer | no | Zero/omitted is unlimited unless an operator ceiling applies. Queue time is excluded. |

```json
{
  "kind": "shell",
  "idempotencyKey": "build-main-20260809-01",
  "name": "Build main",
  "description": "Release verification build",
  "tags": ["build", "release"],
  "lockKeys": ["workspace-main"],
  "cwd": "D:\\Dev\\project",
  "command": "go test ./...",
  "maxRuntimeSeconds": 0
}
```

### task_list

Returns newest-first bounded metadata with optional `statuses`, `kinds`, and `tags` filters plus `cursor`/`limit` paging. It never returns command text, arguments, paths, environment values, or output.

### task_get

Accepts `taskId` and returns the current state, timestamps, terminal result, worker availability, and bounded lifecycle history.

- **States:** `queued`, `starting`, `running`, `succeeded`, `failed`, `timed_out`, `cancelled`, and `interrupted`. Only a terminal state with `finishedAt` establishes completion; output can appear before that state is recorded.
- **Worker availability:** `workerOnline` describes the scheduler heartbeat, not an individual executor. A value of `false` while a task is `starting` or `running` does not prove failure.
- **Start and exit failures:** if a supported shell is unavailable, the terminal code is `TASK_START_FAILED` with a bounded, safely classified cause; private task paths and unclassified OS errors are withheld. A direct process that exits nonzero reports `PROCESS_EXIT_NONZERO` and its exit code.
- **Unclosed output streams:** if descendants keep stdout/stderr open after the direct process exits past the bounded drain delay, `PROCESS_OUTPUT_DRAIN_TIMEOUT` is reported. When available, the direct exit code is preserved rather than replaced with `PROCESS_WAIT_FAILED`.

### task_logs

Accepts `taskId`, independent `stdoutCursor` and `stderrCursor`, and optional `limitBytes`. Each stream returns data, the next absolute cursor, retained end offset, dropped byte count, and whether the requested cursor crossed an evicted middle region. Logs retain a fixed head and rolling tail.

### task_cancel

Accepts `taskId` and an optional bounded `reason`. Cancelling queued work prevents launch. Cancelling running work terminates its complete process tree. Repeated calls are idempotent and terminal tasks remain unchanged.

## Supported Encodings

Scripthold supports **168 read/write encodings**. Call `list_encodings` for the authoritative runtime inventory of canonical names, aliases, and capability metadata.

- **Implementation:** repository-pinned `golang.org/x/text v0.41.0` plus checked-in, deterministic pure-Go mappings and state machines derived and verified against pinned GNU libiconv evidence. Normal builds and execution need no libiconv, GCC, or network access.
- **Detection:** automatic encoding identification is deliberately more conservative than the set of codecs available when explicitly selected.
- **UTF-32:** LE and BE enforce valid Unicode scalar values, authoritative BOMs, cautious BOMless detection, and 32-bit code-unit-aware line-ending conversion. Generic `utf-32` is rejected because its byte order is ambiguous.

- **Unicode:** `utf-8`, `utf-16-le`, `utf-16-be`, `utf-32-le`, `utf-32-be`
- **Existing IBM/DOS/EBCDIC (`x/text`):** `ibm037`, `ibm437`, `ibm850`, `ibm852`, `ibm855`, `ibm858`, `ibm860`, `ibm862`, `ibm863`, `ibm865`, `ibm866`, `ibm1047`, `ibm1140`
- **Additional IBM/DOS/EBCDIC (generated):** `ibm1025`, `ibm1026`, `ibm1046`, `ibm1097`, `ibm1112`, `ibm1122`, `ibm1123`, `ibm1124`, `ibm1125`, `ibm1129`, `ibm1130`, `ibm1131`, `ibm1132`, `ibm1133`, `ibm1137`, `ibm1141`, `ibm1142`, `ibm1143`, `ibm1144`, `ibm1145`, `ibm1146`, `ibm1147`, `ibm1148`, `ibm1149`, `ibm1153`, `ibm1154`, `ibm1155`, `ibm1156`, `ibm1157`, `ibm1158`, `ibm1162`, `ibm1163`, `ibm1164`, `ibm1165`, `ibm1166`, `ibm12712`, `ibm16804`, `ibm273`, `ibm277`, `ibm278`, `ibm280`, `ibm282`, `ibm284`, `ibm285`, `ibm297`, `ibm423`, `ibm424`, `ibm425`, `ibm4971`, `ibm500`, `ibm737`, `ibm775`, `ibm853`, `ibm856`, `ibm857`, `ibm861`, `ibm864`, `ibm869`, `ibm870`, `ibm871`, `ibm875`, `ibm880`, `ibm905`, `ibm922`, `ibm924`
- **ISO-8859:** `iso-8859-1`, `iso-8859-2`, `iso-8859-3`, `iso-8859-4`, `iso-8859-5`, `iso-8859-6`, `iso-8859-6-e`, `iso-8859-6-i`, `iso-8859-7`, `iso-8859-8`, `iso-8859-8-e`, `iso-8859-8-i`, `iso-8859-9`, `iso-8859-10`, `iso-8859-13`, `iso-8859-14`, `iso-8859-15`, `iso-8859-16`
- **Windows:** `windows-874`, `windows-1250`, `windows-1251`, `windows-1252`, `windows-1253`, `windows-1254`, `windows-1255`, `windows-1256`, `windows-1257`, `windows-1258`
- **Existing Mac/KOI8/other single-byte (`x/text`):** `macintosh`, `x-mac-cyrillic`, `koi8-r`, `koi8-u`, `x-user-defined`
- **Additional regional/platform single-byte (generated):** `atarist`, `gb-1988-80`, `georgian-academy`, `georgian-ps`, `hp-roman8`, `jis-c6220-1969-ro`, `jis-x0201`, `koi8-t`, `mac-arabic`, `mac-central-europe`, `mac-croatian`, `mac-greek`, `mac-hebrew`, `mac-iceland`, `mac-romania`, `mac-thai`, `mac-turkish`, `mac-ukraine`, `mulelao-1`, `nextstep`, `pt154`, `riscos-latin1`, `rk1048`, `tds565`, `viscii`
- **East Asian and stateful multibyte:** `gbk`, `gb18030`, `gb18030-2022`, `hz-gb-2312`, `big5`, `big5-2003`, `big5-hkscs-1999`, `big5-hkscs-2001`, `big5-hkscs-2004`, `big5-hkscs-2008`, `euc-cn`, `euc-jp`, `euc-jisx0213`, `euc-tw`, `iso-2022-jp`, `iso-2022-jp-1`, `iso-2022-jp-2`, `iso-2022-jp-3`, `iso-2022-jp-ms`, `iso-2022-cn`, `iso-2022-cn-ext`, `iso-2022-kr`, `shift_jis`, `shift_jisx0213`, `euc-kr`, `johab`, `tcvn`

Generated single-byte mappings require exact one-byte decode and byte-identical reverse mapping. Generated multibyte/stateful implementations use bounded pure-Go tables/state machines; `gb18030-2022` applies 2,087 checked-in decode overrides and 2,087 encode overrides from exhaustive pinned-oracle comparison. Compatibility aliases never replace an established canonical codec silently.

The registry may consult pinned IANA/WHATWG indexes only to map an otherwise unknown label back to an already registered codec. Detector trust additionally requires sufficient evidence, strict decoding, decoded-text quality, binary rejection, known-confusion handling, and any required deterministic stateful signatures. `write_whole_file` therefore fails with `ENCODING_AMBIGUOUS` and leaves bytes unchanged when encoding is omitted for a non-empty existing file whose encoding cannot be trusted. `list_encodings` is the authoritative runtime inventory; the current text/encoding boundary is summarized in [Architecture](docs/ARCHITECTURE.md).
