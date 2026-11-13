package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two additional independent Classic ASP originals, distinct from the logger,
// anti-forgery, and session project origins already used. The originals are
// source snapshots: these tests never execute the applications.
func TestClassicASP91AdditionalOriginalProjectNavigation(t *testing.T) {
	base64Root := os.Getenv("SCRIPTHOLD_REAL_ASP91_BASE64_ROOT")
	browserRoot := os.Getenv("SCRIPTHOLD_REAL_ASP91_BROWSER_ROOT")
	if base64Root == "" && browserRoot == "" {
		t.Skip("both pinned additional Classic ASP projects are not configured")
	}
	if base64Root == "" || browserRoot == "" {
		t.Fatal("both independent original Classic ASP roots must be configured together")
	}
	cases := []struct {
		name, root, file, target, targetText, sibling, siblingText, oracle, callsite, unrelated string
		targetLine, siblingLine                                                                 int
		extra                                                                                   []struct {
			name string
			line int
		}
	}{
		{
			name: "Base64 conversion helper reused by public encode/decode functions",
			root: base64Root, file: "base64.inc.asp",
			target: "Stream_StringToBinary", targetLine: 24,
			targetText: "Private Function Stream_StringToBinary(Text)",
			sibling:    "Stream_BinaryToString", siblingLine: 40,
			siblingText: "Private Function Stream_BinaryToString(Binary)",
			oracle:      "BinaryStream.CharSet = \"us-ascii\"",
			callsite:    "oNode.nodeTypedValue = Stream_StringToBinary(sText)",
			unrelated:   "CreateObject",
			extra: []struct {
				name string
				line int
			}{{"Base64Encode", 2}, {"Base64Decode", 13}},
		},
		{
			name: "Folder extension deny-list versus unrelated byte-size formatter",
			root: browserRoot, file: "list.asp",
			target: "IsValidFile", targetLine: 177,
			targetText: "Function IsValidFile(FileName)",
			sibling:    "ByteConversion", siblingLine: 196,
			siblingText: "Function ByteConversion(NumberOfBytes)",
			oracle:      "DenyExt = \"asp, db, lnk\"",
			callsite:    "If ( IsValidFile (sFileName) ) Then",
			unrelated:   "MapPath",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tc.root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("pinned original must be a bounded regular file: info=%v err=%v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(raw)
			for _, excerpt := range []string{tc.targetText, tc.siblingText, tc.oracle, tc.callsite} {
				if !strings.Contains(text, excerpt) {
					t.Fatalf("independent original-source oracle absent: %q", excerpt)
				}
			}
			ctx := context.Background()
			h := NewHandler([]string{tc.root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "classic-asp",
				Presentation: "compact", MaxFiles: 1,
			})
			if err != nil || digest.Truncated || !digest.CoverageComplete ||
				digest.FilesParsed != 1 || len(digest.OverviewFiles) != 1 ||
				digest.OverviewFiles[0].SourceFingerprint == "" {
				t.Fatalf("selected original Classic ASP digest incomplete: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "classic-asp", MaxFiles: 1, MaxSymbols: 96,
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated ||
				outline.FilesParsed != 1 {
				t.Fatalf("selected original outline incomplete: err=%v outline=%+v", err, outline)
			}
			found := map[string]int{}
			realLocations := map[string]int{tc.target: tc.targetLine, tc.sibling: tc.siblingLine}
			for _, entry := range tc.extra {
				realLocations[entry.name] = entry.line
			}
			for _, sym := range outline.Symbols {
				if line, ok := realLocations[sym.Name]; ok {
					if sym.Path != path || sym.NameRange.Start.Line != line ||
						sym.ParentQualifiedName != "" || sym.ID == "" {
						t.Errorf("real supported declaration incorrectly located: %+v", sym)
					}
					found[sym.Name]++
				}
				if sym.Name == tc.unrelated {
					t.Errorf("runtime call was incorrectly indexed as declaration: %+v", sym)
				}
			}
			for name := range realLocations {
				if found[name] != 1 {
					t.Errorf("original declaration %q occurred %d times (want 1): %+v",
						name, found[name], outline.Symbols)
				}
			}
			if t.Failed() {
				return
			}
			_, targeted, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "classic-asp",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				FileDetails: "relevant",
			})
			if err != nil || targeted.Truncated || !targeted.CoverageComplete ||
				len(targeted.Symbols) != 1 || len(targeted.Files) != 1 {
				t.Fatalf("targeted Classic ASP source navigation unsafe: err=%v result=%+v", err, targeted)
			}
			target, source := targeted.Symbols[0], targeted.Files[0]
			if target.NameRange.Start.Line != tc.targetLine ||
				target.Path != path || target.ID == "" || source.Encoding == "" ||
				source.SourceFingerprint != digest.OverviewFiles[0].SourceFingerprint {
				t.Fatalf("original target source identity disagrees: target=%+v source=%+v", target, source)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "classic-asp",
				SymbolID: target.ID, SourceFingerprint: source.SourceFingerprint,
				Encoding: source.Encoding, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.targetText) {
				t.Fatalf("fingerprint-bound original excerpt missing: err=%v shown=%+v", err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "classic-asp", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 64,
			})
			linked := 0
			if query.Search != nil {
				for _, match := range query.Search.Matches {
					if match.SymbolID == target.ID && match.SourceFingerprint == source.SourceFingerprint &&
						match.Range.Start.Line == tc.targetLine {
						linked++
					}
				}
			}
			if err != nil || query.Search == nil || query.Coverage.ResultsTruncated ||
				!query.Coverage.CoverageComplete || linked != 1 {
				t.Fatalf("structural source search not linked to exact original: err=%v result=%+v", err, query)
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "classic-asp",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || !negative.CoverageComplete || len(negative.Symbols) != 0 {
				t.Fatalf("runtime-call non-declaration fabricated: err=%v symbols=%+v", err, negative.Symbols)
			}
			t.Logf("original task navigation PASS: indexed=%d verified=%d sourceComplete=%v",
				len(outline.Symbols), len(realLocations), outline.CoverageComplete)
		})
	}
}
