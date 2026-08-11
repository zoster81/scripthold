# Real-world line-ending fixtures

This directory contains pinned, unmodified upstream files used by integration tests for `detect_line_endings` and `change_line_endings`.

## Coverage

This fixture set is the **24-encoding real-world baseline that predates the R22 registry expansion**. It is intentionally not the complete current encoding inventory.

The baseline covers:

- UTF-8, UTF-16 LE, and UTF-16 BE;
- Windows-1250 through Windows-1258 and Windows-874;
- ISO-8859-1, ISO-8859-2, ISO-8859-5, ISO-8859-7, ISO-8859-9, and ISO-8859-15;
- KOI8-R, KOI8-U, and IBM866;
- GBK and GB18030.

The source documents contain real non-ASCII text in the relevant languages. The pinned upstream files use LF line endings and are never modified in place by the test suite. Write tests copy each fixture to `t.TempDir()`, convert the real document to CRLF, verify it, convert it back to LF, and require a byte-identical round trip.

The complete current registry is verified separately by registry-driven operation matrices and generated/oracle-backed tests; see [`docs/GLOBAL_ENCODING_COVERAGE.md`](../../../../docs/GLOBAL_ENCODING_COVERAGE.md). `list_encodings` remains authoritative for the runtime inventory.

## Provenance and integrity

`manifest.json` records, for each fixture:

- canonical encoding;
- exact upstream project and immutable revision;
- source path and URL;
- license file;
- SHA-256 digest and byte length;
- BOM state;
- original line-ending style and counts.

The integration tests validate the manifest, hashes, decoding, baseline encoding coverage, line-ending detection, conversion, and byte-identical round trips.

## Sources

- Chromium encoding browser-test corpus, pinned to the revision recorded in `manifest.json`. See `LICENSE.chromium`.
- uchardet language and encoding corpus, pinned to the revision recorded in `manifest.json`. See `COPYING.uchardet`.

The `*.fixture` files are marked `-text` in `.gitattributes` so Git cannot normalize their bytes or line endings during checkout.
