package handler

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// A bounded exhaustive 786-file structural screen of the pinned original Redis
// C checkout. Only the explicitly annotated headers have semantic inventories.
func TestSourceSymbolsOriginalRedisWholeProjectBatchedScreen(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	project := filepath.Join(root, "real-projects", "c-redis-redis")
	h := NewHandler([]string{root})
	var originals []string
	err := filepath.WalkDir(project, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.Type().IsRegular() && (filepath.Ext(path) == ".c" || filepath.Ext(path) == ".h") {
			originals = append(originals, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(originals)
	const originalFiles = 786 // Independently pinned checkout: 474 .c and 312 .h.
	if len(originals) != originalFiles {
		t.Fatalf("original Redis C source set changed: %d != %d", len(originals), originalFiles)
	}
	// The upstream regex test intentionally embeds mixed 8-bit fixture bytes
	// in quoted strings. UTF-8 is not valid, and no single language encoding
	// describes those strings. Latin-1 maps each byte without loss; C grammar
	// outside those literals is ASCII. Keep the encoding decision source-local.
	mixedEncoding := filepath.Join(project, "deps", "tre", "tests", "retest.c")
	utf8Sources := make([]string, 0, originalFiles-1)
	foundMixed := false
	for _, path := range originals {
		if path == mixedEncoding {
			foundMixed = true
		} else {
			utf8Sources = append(utf8Sources, path)
		}
	}
	if !foundMixed {
		t.Fatal("original TRE encoding fixture missing")
	}
	fingerprints := make(map[string]string, originalFiles)
	const batchSize = 32 // Default MCP_SOURCE_MAX_INPUT_PATHS; 25 UTF-8 batches plus the mixed fixture.
	for start := 0; start < len(utf8Sources); start += batchSize {
		end := min(start+batchSize, len(utf8Sources))
		fail, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Presentation: "compact", Paths: utf8Sources[start:end],
			Language: "c", Encoding: "utf-8", MaxFiles: batchSize,
		})
		if err != nil || fail != nil || !digest.CoverageComplete || digest.Truncated ||
			digest.FilesConsidered != end-start || digest.FilesParsed != end-start ||
			digest.FilesSkipped != 0 || len(digest.OverviewFiles) != end-start {
			t.Fatalf("Redis C digest batch [%d:%d] incomplete: err=%v failed=%+v digest=%+v", start, end, err, fail, digest)
		}
		expected := make(map[string]bool, end-start)
		for _, path := range utf8Sources[start:end] {
			expected[path] = true
		}
		for _, file := range digest.OverviewFiles {
			if !expected[file.Path] || fingerprints[file.Path] != "" ||
				!file.CoverageComplete || file.Status != "parsed" ||
				file.Language != "c" || file.SourceFingerprint == "" {
				t.Errorf("unexpected/duplicate/partial original Redis source: %+v", file)
			}
			fingerprints[file.Path] = file.SourceFingerprint
			delete(expected, file.Path)
		}
		for omitted := range expected {
			t.Errorf("eligible C source omitted from batch: %s", omitted)
		}
	}
	fail, special, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "digest", Presentation: "compact", Paths: []string{mixedEncoding},
		Language: "c", Encoding: "iso-8859-1", MaxFiles: 1,
	})
	if err != nil || fail != nil || !special.CoverageComplete || special.FilesParsed != 1 ||
		len(special.OverviewFiles) != 1 || special.OverviewFiles[0].Path != mixedEncoding ||
		!special.OverviewFiles[0].CoverageComplete || special.OverviewFiles[0].SourceFingerprint == "" {
		t.Fatalf("mixed-byte original fixture not separately parsed: %v %+v %+v", err, fail, special)
	}
	fingerprints[mixedEncoding] = special.OverviewFiles[0].SourceFingerprint
	if len(fingerprints) != originalFiles {
		t.Fatalf("only %d/%d original Redis files indexed", len(fingerprints), originalFiles)
	}

	// The complete declaration sets below are independently read from original
	// headers, including the anonymous aggregate's correctly owned fields.
	for _, tc := range []struct{ file, oracle string }{
		{"crc64.h", `
crc64_init function 6 -
crc64 function 7 -
crc64Test function 10 -
`},
		{"sha256.h", `
BYTE alias 20 -
WORD alias 21 -
SHA256_CTX alias 28 -
SHA256_CTX.data field 24 SHA256_CTX
SHA256_CTX.datalen field 25 SHA256_CTX
SHA256_CTX.bitlen field 26 SHA256_CTX
SHA256_CTX.state field 27 SHA256_CTX
sha256_init function 31 -
sha256_update function 32 -
sha256_final function 33 -
`},
		{"zipmap.h", `
zipmapNew function 18 -
zipmapSet function 19 -
zipmapDel function 20 -
zipmapRewind function 21 -
zipmapNext function 22 -
zipmapGet function 23 -
zipmapExists function 24 -
zipmapLen function 25 -
zipmapBlobLen function 26 -
zipmapRepr function 27 -
zipmapValidateIntegrity function 28 -
zipmapTest function 31 -
`},
	} {
		path := filepath.Join(project, "src", tc.file)
		fail, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Presentation: "detailed", Paths: []string{path},
			Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 50,
		})
		if err != nil || fail != nil || !outline.CoverageComplete || len(outline.Files) != 1 {
			t.Fatalf("Redis %s original outline failed: %v %+v %+v", tc.file, err, fail, outline)
		}
		type target struct {
			kind, owner string
			line        int
		}
		want := map[string]target{}
		for _, line := range strings.Split(strings.TrimSpace(tc.oracle), "\n") {
			parts := strings.Fields(line)
			if len(parts) != 4 {
				t.Fatalf("invalid independent oracle record %q", line)
			}
			number, err := strconv.Atoi(parts[2])
			if err != nil {
				t.Fatal(err)
			}
			owner := parts[3]
			if owner == "-" {
				owner = ""
			}
			want[parts[0]] = target{parts[1], owner, number}
		}
		if len(outline.Symbols) != len(want) {
			t.Errorf("%s symbols=%d expected=%d", tc.file, len(outline.Symbols), len(want))
		}
		for _, symbol := range outline.Symbols {
			expected, ok := want[symbol.QualifiedName]
			if !ok || string(symbol.Kind) != expected.kind ||
				symbol.ParentQualifiedName != expected.owner ||
				symbol.NameRange.Start.Line != expected.line || symbol.ID == "" {
				t.Errorf("%s incorrect or fabricated declaration: %+v want=%+v found=%v", tc.file, symbol, expected, ok)
			}
			delete(want, symbol.QualifiedName)
		}
		for missing := range want {
			t.Errorf("%s missing declaration %s", tc.file, missing)
		}
	}

	// The original project is larger than maxFiles. Search every bounded batch
	// and reconcile results, rather than silently dropping later files.
	for _, question := range []struct {
		name, header, source   string
		headerLine, sourceLine int
	}{
		{"crc64_init", "crc64.h", "crc64.c", 6, 141},
		{"sha256_init", "sha256.h", "sha256.c", 31, 90},
	} {
		seen := map[string]int{}
		for start := 0; start < len(utf8Sources); start += batchSize {
			end := min(start+batchSize, len(utf8Sources))
			fail, result, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Match: "exact", Query: question.name,
				Paths: utf8Sources[start:end], Language: "c", Encoding: "utf-8",
				MaxFiles: batchSize, MaxResults: 20,
			})
			if err != nil || fail != nil || !result.Coverage.CoverageComplete ||
				result.Coverage.FilesParsed != end-start || result.Search == nil {
				t.Fatalf("%s batch [%d:%d] search incomplete: %v %+v %+v", question.name, start, end, err, fail, result)
			}
			for _, match := range result.Search.Matches {
				if string(match.Kind) != "function" || match.SourceFingerprint != fingerprints[match.Path] {
					t.Errorf("untrusted structural match: %+v", match)
				}
				seen[match.Path] = match.Range.Start.Line
			}
		}
		fail, extra, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Mode: "structural", Match: "exact", Query: question.name,
			Paths: []string{mixedEncoding}, Language: "c", Encoding: "iso-8859-1",
			MaxFiles: 1, MaxResults: 20,
		})
		if err != nil || fail != nil || !extra.Coverage.CoverageComplete ||
			extra.Coverage.FilesParsed != 1 || extra.Search == nil {
			t.Fatalf("%s mixed-byte source search incomplete: %v %+v %+v", question.name, err, fail, extra)
		}
		for _, match := range extra.Search.Matches {
			seen[match.Path] = match.Range.Start.Line
		}
		expected := map[string]int{
			filepath.Join(project, "src", question.header): question.headerLine,
			filepath.Join(project, "src", question.source): question.sourceLine,
		}
		if len(seen) != len(expected) {
			t.Errorf("%s across Redis project: matches=%v expected=%v", question.name, seen, expected)
		}
		for p, line := range expected {
			if seen[p] != line {
				t.Errorf("%s missing/misplaced at %s:%d got %d", question.name, p, line, seen[p])
			}
		}
	}
	t.Logf("Redis C complete file selection: %d original C/header files in %d bounded batches; 3 annotated headers, 2 project-wide searches", originalFiles, (len(utf8Sources)+batchSize-1)/batchSize+1)
}
