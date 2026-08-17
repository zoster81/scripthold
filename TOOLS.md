# Scripthold Tool Reference

The current completed-R27 Unreleased next-major source tree exposes an authoritative 36-tool catalog and 3 guided prompts; the public Scripthold 2.2.0 release exposes 30 tools. Both catalogs are transport-independent within their respective version. Stdio and Streamable HTTP expose the same schemas, annotations, process-wide allowed directories, limits, execution policy, typed errors, and prompt workflows; modern HTTP requests are stateless while retained legacy HTTP sessions remain stateful. Transport setup and security differ, but tool behavior does not; see [README.md](README.md), [docs/PROJECT_DIRECTION.md](docs/PROJECT_DIRECTION.md), [docs/HTTP_SECURITY.md](docs/HTTP_SECURITY.md), and [docs/DURABLE_TASKS.md](docs/DURABLE_TASKS.md).

## Guided Prompts

- `audit_encodings(path)`: read-only project encoding, BOM, ambiguity, and line-ending audit.
- `fix_mojibake(path)`: evidence-driven diagnosis and approval-gated repair of garbled legacy text.
- `migrate_to_utf8(path, pattern?)`: search, batch dry-run, approval, backup-enabled conversion, and final verification workflow.

These prompt concepts and the implementation approaches reviewed are credited to the [original project](docs/PROJECT_DIRECTION.md#reciprocal-feature-exchange); the resulting code is reworked for this fork's tool names, mutation guarantees, limits, and dual-transport server rather than mechanically synchronized. Prompt instructions guide clients; they do not add per-agent filesystem ACLs or bypass tool authorization.

## Error Handling

Reusable domain failures carry transport-independent typed categories for invalid input or paths, access denial, symlink escapes, missing files, permissions, encoding, conflicts, cancellation, limits, and filesystem failures. Every failed MCP tool call preserves human-readable text and a stable machine-readable code at `_meta.errorCode`. For tools with a structured output contract, the same `errorCode` and `message` are also merged into structured output so clients that do not expose MCP metadata/text still receive actionable diagnostics; existing structured failure evidence is retained alongside the common envelope. The intentionally content-only `check_update` tool keeps its content-only response contract. Per-file failures from `read_multiple_files`, batch `convert_encoding`, and `grep_text_files.skippedFiles` use the same `errorCode` vocabulary. Encoding-related per-file failures may additionally expose `encodingErrorCode` as an additive refinement: `ENCODING_AMBIGUOUS`, `ENCODING_MALFORMED`, `ENCODING_UNSUPPORTED`, `ENCODING_BOM_CONFLICT`, `ENCODING_UNREPRESENTABLE`, or `ENCODING_OTHER`. This refinement does not replace or reinterpret the stable top-level code.

Stable codes are `INVALID_INPUT`, `INVALID_PATH`, `ACCESS_DENIED`, `SYMLINK_ESCAPE`, `NOT_FOUND`, `PERMISSION`, `ENCODING`, `ENCODING_AMBIGUOUS`, `CONFLICT`, `PARTIAL_COMMIT`, `UNSUPPORTED`, `CANCELLED`, `LIMIT`, `IO_ERROR`, `INTERNAL_ERROR`, and the fallback `OPERATION_FAILED`. `PARTIAL_COMMIT` means a mutating operation failed after the target or package may have advanced; accompanying structured state/fingerprint evidence distinguishes committed, unchanged, partial, or unknown outcomes where supported. Successful results omit error codes. See [docs/MIGRATION_2.0.md](docs/MIGRATION_2.0.md).

## File Operations

Mutating file tools share a durable filesystem layer. Replacement data is staged in the destination directory, synced before commit, and installed with platform-specific atomic operations. Existing-file snapshots detect practical concurrent modifications; initially missing destinations use no-replace commits. R24 adds exact recursive scope evidence, stable object and volume identity, package-wide staging, mandatory persistent backup before irreversible regular-file deletion, and native same-volume no-replace move. On Unix, containing directories are synced after namespace changes; on Windows, replacement and no-replace moves use write-through flags. These protections reduce but do not eliminate every path-based TOCTOU window.

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

Read multiple files through the same incremental encoding/BOM-aware pipeline used by `read_text_file`. `MCP_MAX_BATCH_FILES` limits input count and `MCP_MAX_OUTPUT_BYTES` bounds aggregate decoded output. The ordered coordinator preserves input order, using parallelism only when the aggregate worst-case output fits the budget. Individual file failures do not stop the operation, and cancellation still produces one stable result for every requested path. `errorCount` counts every failed item; each failed result retains its stable `errorCode` and optional `encodingErrorCode`. The compatibility `errors` summary keeps only a deterministic bounded prefix (at most 64 entries and subject to a fixed byte budget derived from the output limit); successful decoded content has priority over this duplicate summary, so retained error summaries also yield to the remaining aggregate output budget. `errorsTruncated` and `errorsOmitted` state explicitly when further summaries were omitted.

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

The supplied line endings are written exactly as provided. Encoding failures and invalid BOM policies are rejected before filesystem mutation. The result is staged and synced before an atomic commit; existing targets are checked for concurrent changes, and new targets use a no-replace commit so a concurrently created file is not overwritten. The prepared result and final target are fingerprinted; an existing pre-state fingerprint is additionally retained when the target already fits the configured bounded file-read limit, while larger existing targets keep the historical metadata-only concurrency snapshot rather than forcing a new unbounded read. If replacement reports an error after a possible commit, the handler re-reads only bounded evidence and reports `state: unchanged|committed|unknown`, `changed`, `applied: false`, and `actualFingerprint` when available; a committed or unclassifiable post-error state returns `PARTIAL_COMMIT` rather than an empty or misleading result.

**Parameters:**
- `path` (required): Path to the file
- `content` (required): UTF-8 content to write
- `encoding` (optional): Target encoding. New files use the configured default (`utf-8` by default); existing files preserve a confidently detected encoding. Set `MCP_DEFAULT_ENCODING` or pass `encoding` explicitly for legacy formats such as `cp1251`.
- `bom` (optional): BOM policy — `auto` (default), `always`, `never`, or `preserve`

**BOM policy:**
- `auto`: UTF-8 and legacy encodings have no BOM; UTF-16 LE/BE receive their canonical BOM
- `always`: Require the target encoding's canonical BOM; fails for encodings without BOM support
- `never`: Write no BOM
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

Prepare one exact edit of an existing text file **without persistent mutation**. `edit_file` is read-only in R23 and accepts only `action: "preview"`; the historical omitted-action/`direct` mutation and in-tool `apply` form are removed. Supply either `edits` or one strict single-file unified `patch`, never both.

The preview uses the existing encoding/BOM/line-ending-aware edit pipeline, retains the exact prepared bytes plus stable file identity in a bounded process-local cache, and returns a 256-bit `previewId`, diff, target/result fingerprints, encoding metadata, effective backup policy, creation/expiry timestamps, and `changed`. Preparation performs no target write, permission change, persistent backup capture, adjacent temp creation, or `.bak` creation.

**Parameters:**
- `action` (required): exactly `preview`
- `path` (required): existing regular target file
- `edits` (conditionally required): operations with `oldText`, `newText`, and optional `similarity` from `0.50` to `1.0`
- `patch` (conditionally required): one strict unified diff for the target
- `encoding` (optional): explicit file encoding; otherwise conservative auto-detection applies
- `forceWritable` (optional): approval-bound permission intent retained for apply; preview itself never changes permissions
- `backupPolicy` (optional): omit to inherit `MCP_BACKUP_DEFAULT_POLICY`, or set exactly `required`; callers cannot weaken an operator default of `required`

A logical no-op still returns a capability so approval evidence is explicit, but it needs no backup store and later apply performs no backup or write. A changed preview whose effective backup policy is `required` fails before capability creation if the persistent backup store cannot admit the required pre-state.

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

The capability is consumed before revalidation, so success, conflict, cancellation, write failure, and replay are terminal. Apply revalidates authorization, path, stable file identity, approved pre-state fingerprint, and retained result fingerprint. For a changed edit with effective policy `required`, the exact approved pre-state is durably captured and verified before permission changes or target replacement, then the target is revalidated again. The exact retained bytes are committed through synced same-directory replacement and verified by final fingerprint. A no-op returns `applied: false`, creates no backup, and leaves metadata/bytes unchanged. A durable backup remains valid if a later target write fails; no automatic rollback is promised.

Apply reports actual target evidence rather than reusing the preview prediction: `state` is `unchanged`, `committed`, or `unknown`, and `actualFingerprint` records the observed post-state when classification succeeds. If an error occurs before mutation and the approved fingerprint remains present, `changed` is false. If replacement committed but a later durability/verification step fails, the response is `applied: false`, `state: committed`, `changed: true`, and `PARTIAL_COMMIT`. An unclassifiable post-error state also fails conservatively with `PARTIAL_COMMIT` rather than claiming that no change occurred.

```json
{
  "previewId": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
```
### patch_package

Review or prepare a strict versioned package of coordinated edits to existing regular files **without persistent mutation**. R23 `patch_package` accepts only `inspect`, `dryRun`, and `verify`; package application moved to `patch_package_apply`.

The input and every nested manifest object reject unknown JSON fields. `formatVersion` must be `patch-package-v1`, `fingerprintAlgorithm` must be `sha256`, and `fingerprintMode` must be `content-v1`. Targets must resolve to distinct authorized filesystem objects; duplicate spellings, symlink/junction aliases, hard links, creation, deletion, movement, and `/dev/null` patches are rejected.

**Read-only actions:**
- `inspect`: validate package structure, bounds, authorization, aliases, edit/patch shapes, and declared algorithms without reading target contents.
- `dryRun`: capture a coherent package pre-state, retain stable identities, prepare exact result bytes, verify the final unchanged source state, and return ordered diffs plus aggregate pre/post fingerprints and a one-shot 256-bit `previewId`. Omitted `manifest.backupPolicy` inherits `MCP_BACKUP_DEFAULT_POLICY`; `required` may be supplied explicitly but cannot be weakened by apply. If changed targets require persistent backups, dry-run performs read-only quota/admission preflight and fails before capability creation when admission is impossible. It creates no backup object, manifest, target-adjacent staging file, or target mutation.
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

Apply atomically consumes the capability, revalidates every target identity/fingerprint, then—when the effective policy is `required`—durably captures and verifies all changed pre-states **before any target-adjacent staging is created**. Every target is revalidated after backup capture; only then are changed result bytes staged and manifest-order commits allowed to begin. No-op targets receive no backup and no write.

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

### source_symbols

Navigate bounded source declarations without reading every complete source file. The tool is read-only and exposes four strict operation variants under one schema: `outline`, `digest`, `find`, and fingerprint-bound `show`. All variants reject unknown or operation-illegal fields.

The completed R27 production surface contains 101 active approved target-catalog providers across 103 total registry rows; only auxiliary Dockerfile and Make metadata remain intentionally inactive and are not counted as production support. The authoritative per-provider capability projection is [docs/LANGUAGE_CAPABILITIES.md](docs/LANGUAGE_CAPABILITIES.md); it is generated from the native registry and mechanically checked for drift. Go uses the standard-library AST; the remaining programming-language providers use bounded native scanners, structural recognizers, or offset-preserving adapters, including the Phase 16 Scala and Flow providers. Composite providers preserve host coordinates and delegate only explicitly supported embedded regions. Dialect and shared-extension ambiguity remains fail-closed unless independent evidence resolves it. Phase 15 adds bounded process-local incremental indexing for every active source-analysis provider, while project/semantic relationship claims remain capability-specific and are never inferred merely from provider activation. Unsupported, ambiguous, partial, or truncated coverage is reported explicitly rather than guessed. No external parser/compiler/LSP process, network lookup, persistent code index, or complete-source cache is used.

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

`source_query` is the frozen **R27 Phase 1** compact read-only contract for source search, project relations, and task-context assembly. R27 Phases 13-15 implement bounded structural search, supported project relations, fingerprint-verified task context, and a bounded process-local incremental index over normalized analyzer facts; Phase 16 completes the approved provider catalog and Phase 17 verifies scale, concurrency, security, benchmark, and heterogeneous connector workflows without changing this public schema. `textual`/`lexical` search remain `UNSUPPORTED` because existing `grep_text_files` remains the decoded text-search surface; Phase 15 activates coherent `index` generation/fingerprint binding without adding persistent storage.

The single-tool surface was selected instead of three near-duplicate `source_search`, `source_relations`, and `source_context` tools because all three share authorization, traversal, language/encoding filters, index binding, evidence, and resource policy, while three complete schemas exceeded the fixed connector-catalog budget. The public request remains strict: unknown top-level fields are rejected by the MCP schema, selector/index objects reject unknown nested fields, and the handler rejects known fields that are illegal for the selected operation.

**Common parameters:**
- `operation` (required): `search`, `relations`, or `context`.
- `paths` (required): authorized source files/directories. The configured `MCP_SOURCE_MAX_INPUT_PATHS` ceiling applies.
- `language`, `encoding`, `includes`, `excludes`, `respectGitignore`, `maxFiles` (optional): use the established source-intelligence selection/traversal policy.
- `index` (optional): bind the query to a retained process-local index `generation`, deterministic lowercase SHA-256 `fingerprint`, or both. `stalePolicy` is `reject` or `allow`; omitted/`reject` fails with `CONFLICT` for stale or unavailable evidence, while `allow` may select an exact retained historical metadata generation and reports `index.staleness: "stale"`.

Selectors are fingerprint-bound and use `kind: "path" | "symbol" | "position"`. Every selector carries `path` plus the current lowercase SHA-256 `sourceFingerprint`; a `symbol` selector additionally requires lowercase SHA-256 `symbolId`, while a `position` selector requires positive 1-based `line` and `column`. Invalid combinations are rejected rather than guessed.

**`search`:** requires non-empty `query` and `mode: "textual" | "lexical" | "structural"`; optional `match` is `exact`, `prefix`, or `contains`. Phase 13 implements `structural` over normalized symbols, dependencies, and resolved structural relationships without rescanning raw source. `textual` and `lexical` remain explicitly `UNSUPPORTED`. `kinds`, `evidence`, and `maxResults` are search-specific. Evidence strength uses the frozen ordered vocabulary `textual`, `lexical`, `structural`, `scope-resolved`, `project-resolved`, and `semantic`; a result may never claim a stronger level than the native analyzer proves.

**`relations`:** requires `relation`, whose frozen vocabulary is `dependencies`, `dependents`, `references`, `definitions`, `inheritance`, `implementations`, `overrides`, `callers`, `callees`, `trace`, `impact`, or `cycles`. Phase 13 implements dependency/dependent adjacency, resolved structural references/definitions, inheritance, proven implementations, bounded dependency trace/impact, and dependency cycles. `callers`, `callees`, and `overrides` return `UNSUPPORTED` until analyzers provide trustworthy corresponding facts rather than name guesses. `trace` requires both `subject` and `target`; `cycles` accepts neither selector and does not accept `maxDepth`; the other relation kinds require `subject` and reject `target`. Relation queries may use `evidence`, `maxResults`, `maxNodes`, `maxEdges`, and—where meaningful—`maxDepth`.

Relationship **resolution state is separate from evidence strength**. The frozen states are `resolved`, `ambiguous`, `unresolved`, and `external`. This prevents an ambiguous structural fact from being represented as though `ambiguous` were a weaker point on the textual-to-semantic evidence ladder. Indexed results expose generation, deterministic index fingerprint, and staleness as `current` or retained historical `stale`; `not-indexed` remains part of the frozen vocabulary for compatibility with non-indexed/earlier paths.

**`context`:** requires one to 32 fingerprint-bound `targets` and positive `budgetBytes`; optional `bodyPolicy` is `prefer` or `signatures-only`, with `maxItems` and `maxDepth` as additional bounds. Phase 14's deterministic planner now runs against the coherent Phase 15 project generation; embeddings and persistent storage remain absent. Explicit symbol/position targets are all-or-error under byte/item pressure; `prefer` retains a complete declaration body when it fits and otherwise degrades to the bounded declaration signature. The frozen priority vocabulary is `target`, `enclosing`, `direct-dependency`, `direct-related-body`, `direct-related-signature`, `reverse-or-type-relation`, and `deeper-relation`; deeper relations are always signatures. Position selectors that hit resolved project references target their resolved definition candidates and preserve ambiguity rather than guessing. Before any text is returned—even when `stalePolicy=allow` selected historical metadata—the handler reopens the authorized current source, compares the authoritative fingerprint with the planned fingerprint, and slices only that verified decoded snapshot; source drift returns `CONFLICT`. `usedBytes` counts the actual UTF-8 bytes in returned decoded `ContextItem.text`, and coverage/truncation reports budget, item, depth, or representation degradation honestly. Direct caller/callee body/signature candidates remain absent until analyzers expose trustworthy call facts rather than name matches.

R27 adds these configurable ceilings; invalid, non-positive, overflowing, or above-hard-maximum environment values fall back to their defaults under the same fail-closed configuration policy as R25. Client-provided limits can only lower the effective configured ceiling. Existing source input/file/byte/request/output limits continue to apply as well.

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

Phase 15 query results retain the existing `unicode-scalar-1-based-half-open` source-coordinate contract where ranges are returned, explicit generation-bound coverage/truncation, evidence/resolution state, and coherent index generation/fingerprint/staleness. The process-local index reuses only path/content/analyzer-configuration-identical parsed facts, conservatively rebuilds project relationship metadata for each changed generation, bounds retained scopes/generations, and never stores complete source bodies. File-content or probe-to-analysis drift fails closed; `context` still reopens and fingerprint-verifies current source before returning text. No persistent index, source mutation, external parser/compiler/LSP process, or network activity is introduced.

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

### backup_store

Read and review the optional persistent backup store, or prepare restore/GC capabilities, **without persistent mutation**. R23 `backup_store` is read-only; restore and GC application are separate tools. The tool remains registered when the store is disabled so operators can inspect policy state consistently.

**Actions:**
- `status`: accepts only `action`; reports enabled/health/generation/counts/limits/issues plus the effective operator `defaultPolicy`. When `MCP_BACKUP_STORE_DIR` is unset it returns `enabled: false`; an operator default of `required` is still visible and changed mutation previews then fail closed until a store is configured.
- `list`: newest-first authorized records with optional `cursor`, `limit`, `targetPath`, and `pinned`. Cursors remain authenticated, filter/policy/generation-bound, and visibility is revalidated on every page.
- `history`: requires an authorized `targetPath` and accepts the same paging/filter fields as `list`; it returns only versions for that target.
- `inspect`: requires `backupId`; authorizes the manifest target and fully verifies the immutable object before returning metadata. Object bytes and internal store paths are never exposed.
- `compare`: requires `backupId` and optionally `otherBackupId`. Without the second ID it compares the verified backup with the current authorized target. With two IDs it requires versions of the same authorized target and verifies both objects. Fingerprints/equality are always returned; a unified diff is added only when both sides are within the bounded text-diff limit and safely decodable. Binary/oversized content still receives verified fingerprint evidence without a fabricated diff.
- `audit`: `quick|full` bounded structural/object verification; never repairs or deletes data.
- `restorePreview`: requires `backupId`; authorizes only the immutable manifest's original target, verifies the source object, binds current missing/existing identity and fingerprint, and read-only preflights the mandatory safety backup for an existing target. It returns a 256-bit expiring `previewId`, fingerprints, object size, verification state, and optional bounded diff. No staging file, backup object, manifest, permission change, or target mutation is created.
- `gcDryRun`: creates a deterministic generation-bound, 256-bit expiring GC capability from an authoritative read-only plan. Pinned manifests, active restore sources, and referenced objects remain protected; no record/object is moved or deleted.

`MCP_BACKUP_DEFAULT_POLICY=disabled|required` controls the operator default for eligible approval-bound content mutations (`edit_file`, `patch_package`, `manage_bom`, and `convert_encoding`). The default is `disabled`. A request may explicitly strengthen the policy to `required`; it cannot weaken a configured `required`. No-op mutations create no persistent backup. Restore keeps its independent mandatory safety-backup rule for an existing target, and GC never captures public file content.

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

Apply one `gcDryRun` capability. The complete input is only `previewId`; the token is consumed before revalidation. The store reconstructs the plan at the original policy timestamp and rejects any generation, pin, manifest, object, active-restore, reservation, or reference-count drift before deletion. Selected manifests are moved to typed trash before verified now-unreferenced objects. The derived index is refreshed after durable partial outcomes; trash cleanup is best effort and reported. GC never mutates public targets and does not claim secure deletion or automatic rollback.

```json
{"previewId":"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"}
```
### grep_text_files

Search decoded text incrementally using one regex `pattern` or a `patterns` array combined with OR semantics. Automatic detection uses the conservative registry-aware trust policy; ambiguous non-empty input requires an explicit `encoding`, so callers can still search explicit-only or otherwise ambiguous files by naming their codec. Directory inputs use the `.gitignore`-aware secure walker. Content mode preserves deterministic traversal order and bounded context queues; path/count modes scan each selected file without letting an early high-match file hide later files. `offset + maxMatches` is bounded by `MCP_MAX_MATCHES`, and retained match output remains within `MCP_MAX_OUTPUT_BYTES`.

Partial coverage is explicit in every output mode. `filesSearched` remains the deterministic count of candidate files selected for the request, `filesScanned` counts candidates processed without a per-file failure before the scan stopped, and `filesSkipped` counts encountered per-file failures. `coverageComplete` is false whenever any file was skipped or the scan stopped before all candidates were processed, including result truncation. `skippedFiles` retains a deterministic bounded prefix of per-file `path`, `error`, stable `errorCode`, and optional `encodingErrorCode`; at most 64 details are retained, a separate byte cap is derived from the output budget, and retained match/path/count results take priority when only residual output budget remains. `skippedFilesTruncated` and `skippedFilesOmitted` make additional failures explicit. Encoding/I/O skips preserve valid results from other files; cancellation and output-limit failures remain terminal.

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

Detect the encoding of a file with confidence percentage. Detection is based on BOMs and content, never on filename or extension. Unicode BOMs remain authoritative; BOMless Unicode candidates require strict structural evidence, and control-heavy syntactically valid UTF-8 is rejected as binary rather than trusted as text. Empty files return assumed UTF-8 with confidence 0 and `assumed: true`. Non-empty legacy input must pass an evidence floor, registry closure, strict decoding, decoded-text quality, and known-confusion checks; otherwise the tool returns `ambiguous: true` instead of a forced encoding. HZ-GB-2312, ISO-2022-JP, and ISO-2022-KR require verified stateful syntax plus detector agreement and non-ASCII decoded evidence. GB18030 four-byte syntax excludes GBK but does not guess between generic GB18030 and the exact GB18030:2022 revision. `sample`, `chunked`, and `full` share the same trust semantics, including state that crosses chunk boundaries.

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

Prepare exact encoding-conversion bytes for one file or a bounded batch **without persistent mutation**. R23 `convert_encoding` requires `dryRun: true`; the historical write form (`dryRun: false` or omitted) is removed. Every target must be fully representable and prepared successfully before a capability is returned.

**Parameters:**
- `path` or `paths` (exactly one form): one file or a bounded ordered batch
- `from` (optional): explicit source encoding; otherwise conservative detection applies
- `to` (required): target encoding
- `bom` (optional): `auto` (default), `always`, `never`, or `preserve`
- `dryRun` (required): exactly `true`
- `backup` (optional): bind creation/replacement of the adjacent `.bak` file to the later apply; preview itself never creates it
- `backupPolicy` (optional): omit to inherit `MCP_BACKUP_DEFAULT_POLICY`, or set exactly `required` for persistent-store pre-state capture

Preview retains the **exact converted bytes** plus target identities/fingerprints in one global bounded capability cache shared with BOM mutations. The cache is controlled by `MCP_MAX_BYTE_MUTATION_PREVIEWS` (default `32`), `MCP_MAX_BYTE_MUTATION_PREVIEW_BYTES` (default `268435456`), and `MCP_BYTE_MUTATION_PREVIEW_TTL_SECONDS` (default `900`). Capability kind is bound, so a BOM token cannot be used by encoding apply and vice versa. Expiry, eviction, restart, and replay invalidate the token.

A changed preview with effective persistent policy `required` performs only read-only backup admission preflight. A no-op requires no store. Unsupported characters, ambiguous/invalid input, oversized results, aliases, and batch conflicts fail before capability creation and before any `.bak`, temp file, backup manifest, or target write exists.

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

Detect BOM state or prepare an exact BOM mutation **without persistent mutation**. R23 removes direct `add`/`strip` writes from this tool.

**Actions:**
- `detect`: requires `path` and performs a bounded prefix read only.
- `addPreview`: requires `path` and BOM-capable `encoding` (`utf-8`, UTF-16 LE/BE, or UTF-32 LE/BE); prepares exact bytes and returns a capability.
- `stripPreview`: requires `path`; removes a detected BOM in the retained result. When no BOM exists it prepares an explicit no-op capability.

`backupPolicy` may be omitted to inherit `MCP_BACKUP_DEFAULT_POLICY` or set to `required` for `addPreview`/`stripPreview`; `detect` accepts no mutation-policy fields. Preview retains exact bytes, stable identity, pre/result fingerprints, BOM metadata, and a 256-bit token in the shared byte-mutation cache described under `convert_encoding`. It creates no staging file, persistent backup, permission change, or target mutation. A required no-op needs no backup store.

```json
{"path":"/project/file.php","action":"stripPreview"}
```

### manage_bom_apply

Apply one prepared BOM capability. The complete input is only `previewId`. The token is kind-bound and consumed before revalidation; stale identity, same-content path replacement, target fingerprint drift, expiry, replay, or a token from another mutation class returns `CONFLICT`.

For a changed result with effective policy `required`, the exact approved pre-state is durably captured and verified before replacement and the target is revalidated again. The exact retained bytes are then atomically replaced and final fingerprint is verified. A no-op reports `applied: false`, creates no backup, and performs no write. No automatic rollback is promised after a durable backup if a later write fails.

```json
{"previewId":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
```
### list_encodings

Returns all 168 currently supported encodings with a stable canonical name, aliases, description, read/write capability, automatic-detection eligibility, explicit-only status, Unicode classification, BOM capability, and whether the `auto` BOM policy emits a BOM. Direct compatibility aliases plus applicable pinned IANA/WHATWG aliases are normalized only when they resolve to an already registered codec. Explicit codec support is intentionally broader than automatic detection: ambiguous legacy families remain explicit-only unless independent structural evidence satisfies the hardened trust path; HZ-GB-2312, ISO-2022-JP, and ISO-2022-KR require verified stateful evidence.

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

Accepts `taskId` and returns the latest state, timestamps, terminal result, worker liveness, and the bounded immutable lifecycle history. States are `queued`, `starting`, `running`, `succeeded`, `failed`, `timed_out`, `cancelled`, and `interrupted`. `workerOnline` reports only the scheduler worker heartbeat; it is independent of the per-task executor, so `false` during `starting` or `running` is not by itself a task failure. A terminal state plus `finishedAt` is authoritative; final-looking stdout/stderr can be visible before that transition and is not process-exit evidence. A supported shell whose runtime is unavailable remains a `TASK_START_FAILED` terminal result, but its bounded message preserves only an explicitly safe preparation/start cause; internal task-store paths and unclassified OS errors remain private. A direct process that exits non-zero reports `PROCESS_EXIT_NONZERO` with its exit code. If the direct process exits but inherited stdout/stderr handles held by descendants do not close before the bounded drain delay, the distinct terminal code is `PROCESS_OUTPUT_DRAIN_TIMEOUT`; when available, the direct process exit code is preserved instead of being replaced by a generic `PROCESS_WAIT_FAILED`.

### task_logs

Accepts `taskId`, independent `stdoutCursor` and `stderrCursor`, and optional `limitBytes`. Each stream returns data, the next absolute cursor, retained end offset, dropped byte count, and whether the requested cursor crossed an evicted middle region. Logs retain a fixed head and rolling tail.

### task_cancel

Accepts `taskId` and an optional bounded `reason`. Cancelling queued work prevents launch. Cancelling running work terminates its complete process tree. Repeated calls are idempotent and terminal tasks remain unchanged.

## Supported Encodings

Scripthold exposes **168 read/write encodings**. `list_encodings` is the authoritative runtime inventory for canonical names, compatibility aliases, and capability metadata. The implementation combines the applicable repository-pinned `golang.org/x/text v0.40.0` surface with checked-in deterministic pure-Go mappings/state machines derived and verified from pinned GNU libiconv evidence; ordinary builds and execution require no libiconv, GCC, or network access. Automatic detection remains intentionally narrower than explicit codec support. UTF-32 LE/BE use strict scalar validation, authoritative BOM handling, conservative BOMless detection, and 32-bit code-unit-aware line-ending conversion; generic `utf-32` is intentionally rejected because byte order would be ambiguous.

- **Unicode:** `utf-8`, `utf-16-le`, `utf-16-be`, `utf-32-le`, `utf-32-be`
- **Existing IBM/DOS/EBCDIC (`x/text`):** `ibm037`, `ibm437`, `ibm850`, `ibm852`, `ibm855`, `ibm858`, `ibm860`, `ibm862`, `ibm863`, `ibm865`, `ibm866`, `ibm1047`, `ibm1140`
- **Additional IBM/DOS/EBCDIC (generated):** `ibm1025`, `ibm1026`, `ibm1046`, `ibm1097`, `ibm1112`, `ibm1122`, `ibm1123`, `ibm1124`, `ibm1125`, `ibm1129`, `ibm1130`, `ibm1131`, `ibm1132`, `ibm1133`, `ibm1137`, `ibm1141`, `ibm1142`, `ibm1143`, `ibm1144`, `ibm1145`, `ibm1146`, `ibm1147`, `ibm1148`, `ibm1149`, `ibm1153`, `ibm1154`, `ibm1155`, `ibm1156`, `ibm1157`, `ibm1158`, `ibm1162`, `ibm1163`, `ibm1164`, `ibm1165`, `ibm1166`, `ibm12712`, `ibm16804`, `ibm273`, `ibm277`, `ibm278`, `ibm280`, `ibm282`, `ibm284`, `ibm285`, `ibm297`, `ibm423`, `ibm424`, `ibm425`, `ibm4971`, `ibm500`, `ibm737`, `ibm775`, `ibm853`, `ibm856`, `ibm857`, `ibm861`, `ibm864`, `ibm869`, `ibm870`, `ibm871`, `ibm875`, `ibm880`, `ibm905`, `ibm922`, `ibm924`
- **ISO-8859:** `iso-8859-1`, `iso-8859-2`, `iso-8859-3`, `iso-8859-4`, `iso-8859-5`, `iso-8859-6`, `iso-8859-6-e`, `iso-8859-6-i`, `iso-8859-7`, `iso-8859-8`, `iso-8859-8-e`, `iso-8859-8-i`, `iso-8859-9`, `iso-8859-10`, `iso-8859-13`, `iso-8859-14`, `iso-8859-15`, `iso-8859-16`
- **Windows:** `windows-874`, `windows-1250`, `windows-1251`, `windows-1252`, `windows-1253`, `windows-1254`, `windows-1255`, `windows-1256`, `windows-1257`, `windows-1258`
- **Existing Mac/KOI8/other single-byte (`x/text`):** `macintosh`, `x-mac-cyrillic`, `koi8-r`, `koi8-u`, `x-user-defined`
- **Additional regional/platform single-byte (generated):** `atarist`, `gb-1988-80`, `georgian-academy`, `georgian-ps`, `hp-roman8`, `jis-c6220-1969-ro`, `jis-x0201`, `koi8-t`, `mac-arabic`, `mac-central-europe`, `mac-croatian`, `mac-greek`, `mac-hebrew`, `mac-iceland`, `mac-romania`, `mac-thai`, `mac-turkish`, `mac-ukraine`, `mulelao-1`, `nextstep`, `pt154`, `riscos-latin1`, `rk1048`, `tds565`, `viscii`
- **East Asian and stateful multibyte:** `gbk`, `gb18030`, `gb18030-2022`, `hz-gb-2312`, `big5`, `big5-2003`, `big5-hkscs-1999`, `big5-hkscs-2001`, `big5-hkscs-2004`, `big5-hkscs-2008`, `euc-cn`, `euc-jp`, `euc-jisx0213`, `euc-tw`, `iso-2022-jp`, `iso-2022-jp-1`, `iso-2022-jp-2`, `iso-2022-jp-3`, `iso-2022-jp-ms`, `iso-2022-cn`, `iso-2022-cn-ext`, `iso-2022-kr`, `shift_jis`, `shift_jisx0213`, `euc-kr`, `johab`, `tcvn`

Generated single-byte mappings require exact one-byte decode and byte-identical reverse mapping. Generated multibyte/stateful implementations use bounded pure-Go tables/state machines; `gb18030-2022` applies 2,087 checked-in decode overrides and 2,087 encode overrides from exhaustive pinned-oracle comparison. Compatibility aliases never replace an established canonical codec silently.

The registry may consult pinned IANA/WHATWG indexes only to map an otherwise unknown label back to an already registered codec. Detector trust additionally requires sufficient evidence, strict decoding, decoded-text quality, binary rejection, known-confusion handling, and any required deterministic stateful signatures. `write_whole_file` therefore fails with `ENCODING_AMBIGUOUS` and leaves bytes unchanged when encoding is omitted for a non-empty existing file whose encoding cannot be trusted. The completed provenance, detector, and oracle contract is documented in [Global Encoding Coverage](docs/GLOBAL_ENCODING_COVERAGE.md).
