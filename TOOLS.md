# Scripthold Tool Reference

Scripthold's authoritative 30-tool catalog and 3 guided prompts are transport-independent. Stdio and Streamable HTTP expose the same schemas, annotations, process-wide allowed directories, limits, execution policy, typed errors, and prompt workflows; modern HTTP requests are stateless while retained legacy HTTP sessions remain stateful. Transport setup and security differ, but tool behavior does not; see [README.md](README.md), [docs/PROJECT_DIRECTION.md](docs/PROJECT_DIRECTION.md), [docs/HTTP_SECURITY.md](docs/HTTP_SECURITY.md), and [docs/DURABLE_TASKS.md](docs/DURABLE_TASKS.md).

## Guided Prompts

- `audit_encodings(path)`: read-only project encoding, BOM, ambiguity, and line-ending audit.
- `fix_mojibake(path)`: evidence-driven diagnosis and approval-gated repair of garbled legacy text.
- `migrate_to_utf8(path, pattern?)`: search, batch dry-run, approval, backup-enabled conversion, and final verification workflow.

These prompt concepts and the implementation approaches reviewed are credited to the [original project](docs/PROJECT_DIRECTION.md#reciprocal-feature-exchange); the resulting code is reworked for this fork's tool names, mutation guarantees, limits, and dual-transport server rather than mechanically synchronized. Prompt instructions guide clients; they do not add per-agent filesystem ACLs or bypass tool authorization.

## Error Handling

Reusable domain failures carry transport-independent typed categories for invalid input or paths, access denial, symlink escapes, missing files, permissions, encoding, conflicts, cancellation, limits, and filesystem failures. Every failed MCP tool call preserves human-readable text and adds a stable machine-readable code at `_meta.errorCode`. Per-file failures from `read_multiple_files`, batch `convert_encoding`, and `grep_text_files.skippedFiles` use the same `errorCode` vocabulary. Encoding-related per-file failures may additionally expose `encodingErrorCode` as an additive refinement: `ENCODING_AMBIGUOUS`, `ENCODING_MALFORMED`, `ENCODING_UNSUPPORTED`, `ENCODING_BOM_CONFLICT`, `ENCODING_UNREPRESENTABLE`, or `ENCODING_OTHER`. This refinement does not replace or reinterpret the stable top-level code.

Stable codes are `INVALID_INPUT`, `INVALID_PATH`, `ACCESS_DENIED`, `SYMLINK_ESCAPE`, `NOT_FOUND`, `PERMISSION`, `ENCODING`, `ENCODING_AMBIGUOUS`, `CONFLICT`, `PARTIAL_COMMIT`, `CANCELLED`, `LIMIT`, `IO_ERROR`, `INTERNAL_ERROR`, and the fallback `OPERATION_FAILED`. `PARTIAL_COMMIT` is reserved for a package apply whose final state includes at least one committed or unclassifiable target. Successful results omit error codes. See [docs/MIGRATION_2.0.md](docs/MIGRATION_2.0.md).

## File Operations

Mutating file tools share a durable filesystem layer. Replacement data is staged in the destination directory, synced before commit, and installed with platform-specific atomic operations. Existing-file snapshots detect practical concurrent modifications; initially missing destinations use no-replace commits. On Unix, containing directories are synced after namespace changes; on Windows, replacement and no-replace moves use write-through flags. These protections reduce but do not eliminate every path-based TOCTOU window.

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

The supplied line endings are written exactly as provided. Encoding failures and invalid BOM policies are rejected before filesystem mutation. The result is staged and synced before an atomic commit; existing targets are checked for concurrent changes, and new targets use a no-replace commit so a concurrently created file is not overwritten.

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
  "bomType": "utf-16-le"
}
```

### edit_file

Edit one existing text file through one shared encoding/BOM-aware preparation pipeline. Editing and diff generation require full-document state, so the source, prepared result, and any supplied patch are rejected when they exceed `MCP_MAX_FILE_BYTES`. Supply either `edits` or one strict single-file unified `patch`, never both.

`action` selects one of three modes:

- omitted or `direct`: preserve the historical behavior; prepare and optionally commit in one request;
- `preview`: prepare the exact encoded result without changing the file, optionally bind `backupPolicy: "required"`, retain the policy and exact mutation in a bounded process-local cache, and return a 256-bit `previewId`, expiry, diff, encoding metadata, and target/result fingerprints;
- `apply`: accept only `previewId`, atomically consume it, revalidate path, file identity, target fingerprint, and prepared-result fingerprint, durably capture the approved pre-state first when the retained policy is `required`, then commit the exact retained bytes through the durable mutation layer.

Every apply attempt is terminal, including conflict, cancellation, encoding-independent write failure, or successful no-op. Unknown, expired, evicted, replayed, or restart-invalidated identifiers return `CONFLICT`. Preview identifiers are never listed or written to ordinary logs. They are process-wide capabilities because every connection already shares the same roots and authorization policy; possession authorizes only the exact prepared mutation.

The preview cache is bounded independently by `MCP_MAX_EDIT_PREVIEWS` (default `128`), `MCP_MAX_EDIT_PREVIEW_BYTES` (default `67108864` dynamic retained bytes), and `MCP_EDIT_PREVIEW_TTL_SECONDS` (default `900`). Expired entries are removed before deterministic FIFO eviction. Cache removal closes retained file-identity handles, and process restart invalidates all previews. Omitted `backupPolicy` preserves the no-persistent-backup behavior. `backupPolicy: "required"` is accepted only by preview and requires a configured store; no backup is created until apply, and a logical no-op creates none.

**Parameters:**
- `action` (optional): `direct` (default), `preview`, or `apply`
- `previewId` (required only for `apply`): 64 hexadecimal characters; `apply` accepts no other fields
- `path` (required for `direct` and `preview`): Existing target file
- `edits` (conditionally required): Operations with `oldText`, `newText`, and optional `similarity` from `0.50` to `1.0`
- `patch` (conditionally required): One strict unified diff for the target; multi-file patches, creation/deletion, unordered or overlapping hunks, and no-newline markers are rejected
- `dryRun` (direct only): Return the prepared diff without writing
- `encoding` (direct/preview only): File encoding, auto-detected when omitted
- `forceWritable` (direct/preview only): Clear a read-only flag during commit when explicitly authorized
- `backupPolicy` (preview only): Omit for no persistent backup, or set exactly `required`; apply cannot override or weaken the retained policy

**Preparation and commit guarantees:**
- exact, whitespace-flexible, or opt-in bounded fuzzy matching with one unique best candidate;
- strict single-file unified-patch parsing and exact context validation;
- preservation of original encoding, BOM state, and consistent CRLF/LF style;
- byte-identical logical no-ops across all 168 registered encodings;
- rejection of unrepresentable output before mutation;
- one exact prepared byte sequence shared by preview and apply;
- target and result SHA-256 `content-v1` fingerprints;
- stable open-file identity retained through approval, including same-content path-replacement detection;
- when required, exact-byte pre-state capture with a durable checksummed manifest before permission changes or target replacement;
- failed capture prevents mutation; a backup committed before a later write failure remains valid and its `backupId` remains in structured output;
- synced same-directory staging, atomic replacement, path revalidation, snapshot conflict detection, and post-commit fingerprint verification.

**Direct example:**
```json
{
  "path": "/path/to/file.go",
  "edits": [
    {
      "oldText": "func oldName()",
      "newText": "func newName()"
    }
  ]
}
```

**Preview example:**
```json
{
  "action": "preview",
  "path": "/path/to/file.go",
  "patch": "--- /path/to/file.go\n+++ /path/to/file.go\n@@ -1 +1 @@\n-old\n+new\n",
  "backupPolicy": "required"
}
```

**Preview response:**
```json
{
  "action": "preview",
  "diff": "--- /path/to/file.go\n+++ /path/to/file.go\n@@ -1 +1 @@\n-old\n+new\n",
  "previewId": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "createdAt": "2026-08-03T21:00:00Z",
  "expiresAt": "2026-08-03T21:15:00Z",
  "targetPath": "/path/to/file.go",
  "targetFingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "resultFingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "encoding": "utf-8",
  "lineEndingStyle": "lf",
  "backupPolicy": "required",
  "changed": true,
  "applied": false
}
```

**Apply example:**
```json
{
  "action": "apply",
  "previewId": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
```

A successful apply omits `previewId` so the consumed capability is not re-emitted. `applied: true` means the prepared action completed successfully; `changed` distinguishes a committed byte change from a successful logical no-op. `backupPolicy` repeats the retained policy and `backupId` appears only after a durable manifest commit. If target mutation then fails, the MCP call is an error but structured output still carries that `backupId` with `applied: false`. `readOnlyCleared` appears only when the approved commit cleared that flag.

### patch_package

Run a strict versioned package of coordinated edits to existing regular files. The four actions are `inspect`, `dryRun`, `apply`, and `verify`. The workflow preserves the package's declared order and encoding/BOM/line-ending behavior, but it does **not** claim atomic multi-file commit or automatic rollback.

The input and every nested manifest object reject unknown JSON fields. `formatVersion` must be `patch-package-v1`, `fingerprintAlgorithm` must be `sha256`, and `fingerprintMode` must be `content-v1`. The manifest may omit `backupPolicy` or set it exactly to `required`. Targets must use unique normalized paths resolving to distinct filesystem objects; duplicate spellings, symlink aliases, junction aliases, and hard links are rejected. Creation, deletion, movement, renaming, and `/dev/null` patches are unsupported.

Each target declares:

- one existing regular-file `path` inside an allowed root;
- the exact current `expectedFingerprint` from `fingerprint_paths` or another `content-v1` result;
- optional `expectedResultFingerprint`; it is checked during `dryRun` when supplied and is required by `verify`;
- exactly one of `edits` or one strict single-file unified `patch`;
- optional `encoding` and `forceWritable`, with the same semantics as `edit_file`.

**Actions:**

- `inspect`: validates structure, limits, paths, file types, aliases, edit shapes, fuzzy thresholds, patch structure, and declared algorithms without reading target contents.
- `dryRun`: obtains a coherent package-wide pre-state, retains one stable file identity per target, prepares the exact result bytes through the shared edit pipeline, verifies final unchanged source state, and returns diffs plus aggregate pre/post fingerprints. When `backupPolicy` is `required`, it also validates package capture authority and conservatively preflights the aggregate source bytes, manifest count, pin count, and per-target version quotas without creating persistent state. It stores the exact prepared package and policy in a bounded process-local cache and returns a 256-bit `previewId`.
- `apply`: accepts only `previewId`. The capability is atomically consumed before validation, so replay, conflict, cancellation, staging failure, capture failure, commit failure, and success are all terminal. Every target is revalidated and every changed result is durably staged. When required, the store atomically reserves the complete conservative package budget, captures changed pre-states in manifest order, and the handler verifies every backup ID, path, source operation, and fingerprint. All targets are revalidated again after capture; only then can the first manifest-order commit begin. A failed capture creates no target mutation, although any already durable backup prefix remains valid and is returned. Final coherent fingerprint verification and existing `PARTIAL_COMMIT` semantics remain unchanged; no modified manifest is accepted and no automatic rollback is attempted.
- `verify`: requires `expectedResultFingerprint` for every target, reads current fingerprints, and returns ordered per-target matches plus expected and actual aggregate fingerprints. A mismatch returns `CONFLICT` with structured results.

Package preview identifiers are process-wide capabilities because every connection already shares the same roots and authorization policy. They are never listed or written to normal logs. Restart invalidates them. Expired entries are removed before deterministic FIFO eviction, and every removal closes retained file identities.

**Partial-commit contract:**

All changed outputs are staged before the first commit, but filesystem replacement remains per file. If a failure occurs after a target may have been replaced, the server stops and re-fingerprints every target with a bounded best-effort classification:

- `committed`: current bytes match the prepared result;
- `unchanged`: current bytes match the approved pre-state;
- `unknown`: neither state can be proven or the target could not be inspected conclusively.

When at least one target is committed or unknown, the MCP error code is `PARTIAL_COMMIT`. The structured response includes `failedIndex`, `failedPath`, the underlying `failureCode`, bounded `failureMessage`, counts, per-target states, actual fingerprints where available, and every durable per-target `backupId` produced by a required policy. Backups improve recovery evidence but do not make replacement transactional, trigger rollback, or change classification semantics.

**Limits:**

- `MCP_MAX_BATCH_FILES` bounds target count;
- `MCP_MAX_FILE_BYTES` bounds every source, patch, prepared file, apply revalidation snapshot, and post-commit verification pass; each target accepts at most 1000 edit operations;
- `MCP_MAX_PATCH_PACKAGE_BYTES` bounds semantic manifest JSON (default `16777216`);
- `MCP_MAX_PATCH_PACKAGE_PREPARED_BYTES` bounds aggregate retained preparation state during one dry run (default `67108864`);
- `MCP_MAX_PATCH_PACKAGE_PREVIEWS` bounds live package capabilities per process (default `16`);
- `MCP_MAX_PATCH_PACKAGE_PREVIEW_BYTES` bounds bytes retained by all live package capabilities (default `134217728`);
- `MCP_PATCH_PACKAGE_PREVIEW_TTL_SECONDS` bounds capability lifetime (default `900`);
- `MCP_MAX_OUTPUT_BYTES` bounds combined structured and text output, including worst-case partial-state diagnostics and backup identifiers;
- configured backup-store total-byte, object-byte, manifest, per-target-version, and pinned limits bound required package admission. Dry run and apply charge every changed source at full size conservatively even when verified deduplication later lowers committed object bytes.

**Dry-run example:**

```json
{
  "action": "dryRun",
  "manifest": {
    "formatVersion": "patch-package-v1",
    "label": "Rename two helpers",
    "fingerprintAlgorithm": "sha256",
    "fingerprintMode": "content-v1",
    "backupPolicy": "required",
    "targets": [
      {
        "path": "/project/a.go",
        "expectedFingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "edits": [
          {
            "oldText": "func oldA()",
            "newText": "func newA()"
          }
        ]
      },
      {
        "path": "/project/b.go",
        "expectedFingerprint": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
        "patch": "--- a/b.go\n+++ b/b.go\n@@ -1 +1 @@\n-old\n+new\n"
      }
    ]
  }
}
```

The successful dry-run response includes:

```json
{
  "action": "dryRun",
  "formatVersion": "patch-package-v1",
  "aggregateMode": "patch-package-aggregate-v1",
  "aggregateBeforeFingerprint": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
  "aggregateAfterFingerprint": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
  "previewId": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "createdAt": "2026-08-04T00:00:00Z",
  "expiresAt": "2026-08-04T00:15:00Z",
  "targetCount": 2,
  "changedCount": 2,
  "backupPolicy": "required",
  "backupCount": 0,
  "results": [
    {
      "index": 0,
      "path": "/project/a.go",
      "state": "prepared",
      "actualFingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "resultFingerprint": "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
      "diff": "...",
      "encoding": "utf-8",
      "lineEndingStyle": "lf",
      "changed": true
    }
  ]
}
```

**Apply example:**

```json
{
  "action": "apply",
  "previewId": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
```

A successful apply omits the consumed identifier and reports `applied: true`, `committedCount`, `unchangedCount`, `actualAggregateFingerprint`, and per-target `committed` or `unchanged` states. With required policy it also reports `backupPolicy: "required"`, `backupCount`, and one `backupId` for every changed target; logical no-ops receive none.

**Verify example:**

```json
{
  "action": "verify",
  "manifest": {
    "formatVersion": "patch-package-v1",
    "fingerprintAlgorithm": "sha256",
    "fingerprintMode": "content-v1",
    "targets": [
      {
        "path": "/project/a.go",
        "expectedFingerprint": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "expectedResultFingerprint": "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
        "edits": [
          {
            "oldText": "func oldA()",
            "newText": "func newA()"
          }
        ]
      }
    ]
  }
}
```

The aggregate fingerprint binds manifest order, Unicode-NFC slash-normalized declared paths, and ordered per-target `content-v1` fingerprints. It is reproducible package evidence, not a multi-file atomicity guarantee.

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

### create_directory

Create a directory recursively (like `mkdir -p`). Succeeds if already exists.

**Parameters:**
- `path` (required): Path to directory to create

### move_file

Move or rename files and directories with a platform-native no-replace operation. A destination created concurrently is not overwritten. Namespace changes are synced where the platform provides a directory-sync mechanism.

**Parameters:**
- `source` (required): Path to move
- `destination` (required): Destination path

### copy_file

Copy a regular file through exclusive same-directory staging, preserving source permissions and modification time where the platform supports them. The staged data is synced and installed atomically without replacing an existing or concurrently created destination. Does not copy directories.

**Parameters:**
- `source` (required): Source file path
- `destination` (required): Destination path

### delete_file

Delete a file after path revalidation and an optimistic metadata snapshot check, then sync the containing directory where supported. Does not delete directories.

**Parameters:**
- `path` (required): Path to delete

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

Review, restore, and explicitly garbage-collect the optional persistent backup store without returning object bytes or internal store paths. `status`, `list`, `inspect`, and `audit` remain read-only. `restorePreview`/`restoreApply` are restricted to the selected manifest's original currently authorized target. `gcDryRun`/`gcApply` manage only internal backup records and never expose their target paths. The tool is always registered so stdio and Streamable HTTP expose the same schema. When `MCP_BACKUP_STORE_DIR` is unset, `action=status` returns `enabled: false`; the other actions fail with `INVALID_INPUT`.

Actions form a strict union:

- `status`: accepts only `action`. It runs a quick structural verification and returns redacted format versions, health, generation, aggregate counts, configured limits, and bounded path-free issues.
- `list`: accepts `cursor`, `limit`, `targetPath`, and `pinned`. Results are newest-first and include only targets authorized by the current process roots. The opaque keyset cursor is authenticated and bound to the exact filters, current allowed/protected-root policy snapshot, and store generation; target visibility is revalidated on every page. Tampered or filter-swapped cursors fail with `INVALID_INPUT`, while a changed store generation fails with `CONFLICT`. `limit` defaults to 50 and cannot exceed 100.
- `inspect`: requires `backupId` and accepts no other action fields. It validates the strict manifest, confirms that its target is currently authorized, and fully hashes the referenced object before returning metadata such as digest, byte count, pre-state fingerprint, mode, modification time, label, pinned state, and manifest checksum. Object bytes are never returned.
- `audit`: accepts `auditMode=quick|full`, `maxObjects`, and `maxBytes`. Quick mode validates structure, references, object sizes, recovery residue, orphans, and index consistency. Full mode additionally hashes every referenced object under the requested limits, which cannot exceed the configured store limits. Audit reports issues but never repairs, quarantines, or deletes data.
- `restorePreview`: requires `backupId` and accepts no other action fields. It authorizes only the immutable manifest's original target, fully verifies the manifest and object, captures the exact current regular-file fingerprint or missing state, and preflights quota for the mandatory safety backup when the target exists. It returns current/result fingerprints, exact object size, an optional bounded diff when both states are safely decodable, and a 256-bit expiring `previewId`. It creates no object, manifest, or target mutation.
- `restoreApply`: accepts only `previewId`. The capability is atomically consumed before validation, so success, failure, cancellation, stale state, and replay are terminal. Apply revalidates current authorization, source manifest/object identity and digest, target identity/fingerprint or missing state, and the prepared result. Exact source bytes are durably staged. An existing target is then captured as a mandatory durable `sourceOperation=restore` safety backup before any permission change or replacement; a missing target receives no safety backup and is installed with no-replace. Final bytes are fingerprinted before success. Operational errors preserve `safetyBackupId` plus `restored`, `unchanged`, `missing`, or `unknown` evidence without automatic rollback.
- `gcDryRun`: accepts only `action`. It performs an authoritative bounded scan and returns a deterministic generation-bound plan behind a 256-bit expiring `previewId`. Pinned manifests and active restore sources are never selected. For each target, at least one manifest is preserved; additional unpinned manifests become candidates when they are older than `MCP_BACKUP_RETENTION_DAYS` or exceed `MCP_BACKUP_MAX_VERSIONS_PER_TARGET`. Objects are selected only when all candidate manifests are removed and no live reference remains; pre-existing orphan objects are also eligible. Candidate backup IDs, object digests, reasons, counts, and reclaimable unique bytes are returned, but target paths are omitted. Active capture reservations reject planning.
- `gcApply`: accepts only `previewId`, consumes it before validation, and reconstructs the complete plan at the original policy timestamp. Any generation, pin, manifest, object, active-restore, reservation, or reference-count change fails with `CONFLICT` before deletion. Apply moves selected manifests into typed trash first, rescans live references, fully verifies each now-unreferenced object, then moves those objects into trash. The derived index is refreshed after every durable partial outcome. Trash deletion is best effort; structured output preserves removed counts, reclaimed bytes, cleanup failures, remaining trash, and the resulting generation. Startup deletes only recognized valid GC trash and leaves unknown entries for review. There is no background GC, implicit quota-triggered GC, mutable pinning, automatic rollback, or secure-deletion guarantee.

Every result and retained restore/GC plan is bounded. `MCP_BACKUP_PLAN_TTL_SECONDS` controls both capability lifetimes; the process enforces separate fixed caps of 64 live restore capabilities and 64 live GC capabilities, with 16 MiB retained state for each cache. Restore also limits optional per-state diff input to 1 MiB. `MCP_MAX_OUTPUT_BYTES` bounds responses. Backup target paths are exposed only by authorized list/inspect/restore actions; GC plans never expose them. The store itself remains inaccessible through ordinary filesystem tools. Restore never deletes or consumes the source backup, and GC never mutates public target files.

**Status example:**
```json
{
  "action": "status"
}
```

**List example:**
```json
{
  "action": "list",
  "limit": 25,
  "targetPath": "/project/config.json",
  "pinned": false
}
```

**Inspect example:**
```json
{
  "action": "inspect",
  "backupId": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
```

**Full audit example:**
```json
{
  "action": "audit",
  "auditMode": "full",
  "maxObjects": 1000,
  "maxBytes": 1073741824
}
```

**Restore preview example:**
```json
{
  "action": "restorePreview",
  "backupId": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
```

A successful preview returns a nested `restore` object with `previewId`, `targetPath`, `targetExisted`, `currentFingerprint` when present, `resultFingerprint`, `objectBytes`, `objectVerified: true`, optional `diff`, and `state: "prepared"`.

**Restore apply example:**
```json
{
  "action": "restoreApply",
  "previewId": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
}
```

A successful existing-target apply returns `state: "restored"`, `applied: true`, `actualFingerprint`, and `safetyBackupId`. A successful missing-target restore omits `safetyBackupId`. Any later error retains the safety identifier and classified actual state in structured output.

**GC dry-run example:**
```json
{
  "action": "gcDryRun"
}
```

A successful dry run returns a nested `gc` object with `previewId`, `plannedAt`, `generation`, policy values, manifest/object counts, reclaimable bytes, bounded candidate metadata, and `state: "prepared"`. It does not change manifests, objects, the derived index, or public targets.

**GC apply example:**
```json
{
  "action": "gcApply",
  "previewId": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
}
```

A successful apply returns `state: "applied"` or `"no_op"`, the previous and resulting generations, removed counts, reclaimed bytes, and any trash residue. A partial operational error remains an MCP error while preserving durable progress in structured output.

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

Convert one file or a bounded batch through the selected decoder and target encoder. `dryRun` performs a complete preflight without writing and reports unrepresentable runes with Unicode code point plus 1-based line/column locations. Single-file failures remain explicit MCP errors. Batch results preserve input order, report per-file success or stable `errorCode` plus optional `encodingErrorCode`, and may succeed partially. `errorCount` counts all failed files while the compatibility `errors` summary retains only a deterministic bounded prefix of at most 64 entries under its byte budget; `errorsTruncated` and `errorsOmitted` expose any omitted summaries. Actual changed output uses synced same-directory staging, byte-identical no-op suppression, optional transactional backup, path revalidation, and concurrent-source-change rejection.

**Parameters:**
- `path` (conditionally required): One file to convert
- `paths` (conditionally required): Bounded array of files; mutually exclusive with `path`
- `from` (optional): Source encoding (auto-detected if omitted)
- `to` (required): Target encoding
- `backup` (optional): Transactionally create or replace a `.bak` backup before committing the conversion (default: false). The backup is staged and synced first; if target commit fails, a previous backup is restored or a newly created backup is removed. If restoration itself fails, the previous backup remains in a recovery staging file whose path is included in the error.
- `bom` (optional): BOM policy — `auto` (default), `always`, `never`, or `preserve`
- `dryRun` (optional): Preview per-file changes and unsupported-character locations without mutation

**BOM policy:**
- `auto`: UTF-8 and legacy targets have no BOM; UTF-16 LE/BE targets receive their canonical BOM
- `always`: Require the target encoding's canonical BOM; fails before mutation for encodings without BOM support
- `never`: Write no BOM
- `preserve`: Preserve source BOM presence using the canonical BOM of the target encoding

**Example:**
```json
{
  "path": "/path/to/multilingual.data",
  "from": "utf-16-le",
  "to": "utf-8",
  "backup": true,
  "bom": "auto"
}
```

**Response:**
```json
{
  "message": "Successfully converted /path/to/multilingual.data from utf-16-le to utf-8 (BOM: auto) (backup: /path/to/multilingual.data.bak)",
  "sourceEncoding": "utf-16-le",
  "targetEncoding": "utf-8",
  "backupPath": "/path/to/multilingual.data.bak",
  "bomPolicy": "auto",
  "hasBOM": false,
  "changed": true
}
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

Detect, strip, or add Unicode BOM (Byte Order Mark). Detection reads at most four prefix bytes. Strip and add stream the unchanged body into synced same-directory staging instead of loading the file. UTF-8 BOM breaks PHP/shell scripts. UTF-16 BOMs remain the authoritative and most interoperable encoding signal, although structurally clear BOMless UTF-16 may also be detected. Snapshot verification and path revalidation ensure cancellation or detected concurrent changes leave the original file unchanged.

**Parameters:**
- `path` (required): Path to the file
- `action` (required): `"detect"`, `"strip"`, or `"add"`
- `encoding` (required for "add"): BOM encoding — `utf-8`, `utf-16-le`, `utf-16-be`, `utf-32-le`, `utf-32-be`

**Example (detect):**
```json
{
  "path": "/path/to/file.php",
  "action": "detect"
}
```

**Response:**
```json
{
  "message": "BOM detected: utf-8 (3 bytes)",
  "hasBOM": true,
  "bomType": "utf-8",
  "bomBytes": 3,
  "changed": false
}
```

**Example (strip):**
```json
{
  "path": "/path/to/file.php",
  "action": "strip"
}
```

**Response:**
```json
{
  "message": "Stripped utf-8 BOM (3 bytes) from /path/to/file.php",
  "hasBOM": false,
  "bomType": "utf-8",
  "bomBytes": 3,
  "changed": true
}
```

**Example (add):**
```json
{
  "path": "/path/to/file.txt",
  "action": "add",
  "encoding": "utf-16-le"
}
```

**Response:**
```json
{
  "message": "Added utf-16-le BOM (2 bytes) to /path/to/file.txt",
  "hasBOM": true,
  "bomType": "utf-16-le",
  "bomBytes": 2,
  "changed": true
}
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

Durably enqueue one task. `idempotencyKey` is mandatory; an identical retry returns the original task, while a different request using the same key fails.

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
| `shell` | string | no | Windows: `powershell`, `pwsh`, `cmd`; Unix: `sh`, `bash`, `pwsh`. |
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

Accepts `taskId` and returns the latest state, timestamps, terminal result, worker liveness, and the bounded immutable lifecycle history. States are `queued`, `starting`, `running`, `succeeded`, `failed`, `timed_out`, `cancelled`, and `interrupted`.

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
