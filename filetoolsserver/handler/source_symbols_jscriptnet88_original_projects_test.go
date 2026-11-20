package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A source-first maintenance case on two additional, independent upstream
// projects. No third-party source is compiled or executed by this test.
func TestJScriptNet88AdditionalOriginalProjectNavigation(t *testing.T) {
	dzonny := os.Getenv("SCRIPTHOLD_REAL_JSNET88_DZONNY")
	admin := os.Getenv("SCRIPTHOLD_REAL_JSNET88_ADMIN")
	if dzonny == "" && admin == "" {
		t.Skip("additional original JScript.NET projects not configured")
	}
	if dzonny == "" || admin == "" {
		t.Fatal("both independent original JScript.NET projects must be configured together")
	}
	type scenario struct {
		name, root, target, neighbor, excerpt, forbidden string
		targetLine, neighborLine                         int
		paths                                            []string
		relevant, other                                  string
	}
	cases := []scenario{
		{
			name: "template greeting and keyboard wait entry point",
			root: filepath.Dir(dzonny), paths: []string{dzonny},
			target: "Main", targetLine: 5, neighbor: "Program", neighborLine: 4,
			excerpt: "static function Main(args: String[])", forbidden: "WriteLine",
			relevant: "Console.ReadKey();", other: "$safeprojectname$.Program.Main(",
		},
		{
			name: "shared temporary path generation for log and shortcut",
			root: admin,
			paths: []string{
				filepath.Join(admin, "utils.js"),
				filepath.Join(admin, "errorLog.js"),
				filepath.Join(admin, "package.js"),
				filepath.Join(admin, "parameters.js"),
				filepath.Join(admin, "setup.js"),
			},
			target: "GenerateRandomPath", targetLine: 13,
			neighbor: "DeleteFile", neighborLine: 21,
			excerpt: "function GenerateRandomPath(extension)", forbidden: "ReadToEnd",
			relevant: "GenerateRandomPath('.log')", other: "GenerateRandomPath('.lnk')",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range tc.paths {
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("selected original source must be a bounded regular file: %s, info=%v err=%v", path, info, err)
				}
			}
			// Independent source oracles were chosen from complete originals
			// before querying the local development handler.
			var targetFile, relevantFile, otherFile string
			for _, path := range tc.paths {
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(raw), tc.excerpt) {
					targetFile = path
				}
				if strings.Contains(string(raw), tc.relevant) {
					relevantFile = path
				}
				if strings.Contains(string(raw), tc.other) {
					otherFile = path
				}
			}
			if targetFile == "" || relevantFile == "" || otherFile == "" {
				t.Fatalf("original project-maintenance evidence is missing: target=%q related=%q other=%q", targetFile, relevantFile, otherFile)
			}
			if len(tc.paths) > 1 && (targetFile == relevantFile || targetFile == otherFile || relevantFile == otherFile) {
				t.Fatal("cross-file original call-site evidence was not independent")
			}

			ctx, h := context.Background(), NewHandler([]string{tc.root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Presentation: "compact", Paths: tc.paths,
				Language: "jscript-net", MaxFiles: len(tc.paths),
			})
			if err != nil || digest.FilesParsed != len(tc.paths) ||
				len(digest.OverviewFiles) != len(tc.paths) || digest.Truncated {
				t.Fatalf("bounded first-pass source map failed: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: tc.paths,
				Language: "jscript-net", MaxFiles: len(tc.paths), MaxSymbols: 128,
			})
			if err != nil || outline.FilesParsed != len(tc.paths) || outline.Truncated {
				t.Fatalf("bounded original source outline failed: err=%v outline=%+v", err, outline)
			}
			seenTarget, seenNeighbor := 0, false
			for _, sym := range outline.Symbols {
				if sym.Path == targetFile && sym.Name == tc.target && sym.NameRange.Start.Line == tc.targetLine {
					if tc.target == "Main" && sym.ParentQualifiedName != "Program" && !strings.HasSuffix(sym.ParentQualifiedName, ".Program") {
						t.Errorf("template entry point attributed to wrong class: %+v", sym)
					}
					if tc.target == "GenerateRandomPath" && sym.ParentQualifiedName != "" {
						t.Errorf("global utility was assigned a fabricated owner: %+v", sym)
					}
					seenTarget++
				}
				if sym.Path == targetFile && sym.Name == tc.neighbor && sym.NameRange.Start.Line == tc.neighborLine {
					seenNeighbor = true
				}
				if sym.Name == tc.forbidden {
					t.Errorf("call/reference wrongly indexed as a declaration: %+v", sym)
				}
			}
			if seenTarget != 1 || !seenNeighbor {
				t.Fatalf("original maintenance declaration or related type/function missing: target=%d neighbor=%v symbols=%+v", seenTarget, seenNeighbor, outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: tc.paths, Language: "jscript-net",
				Query: tc.target, Match: "exact", MaxFiles: len(tc.paths),
				MaxSymbols: 64, FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Symbols) != 1 || len(found.Files) != 1 {
				t.Fatalf("exact edit-target lookup failed: err=%v find=%+v", err, found)
			}
			sym, file := found.Symbols[0], found.Files[0]
			if sym.Name != tc.target || sym.Path != targetFile ||
				sym.NameRange.Start.Line != tc.targetLine ||
				sym.ID == "" || file.Encoding == "" || file.SourceFingerprint == "" {
				t.Fatalf("edit-target identity not safe: sym=%+v file=%+v", sym, file)
			}
			fingerprintMatches := false
			for _, overview := range digest.OverviewFiles {
				if overview.SourceFingerprint == file.SourceFingerprint && overview.Path == targetFile {
					fingerprintMatches = true
				}
			}
			if !fingerprintMatches {
				t.Fatal("first-pass source fingerprint disagrees with targeted find")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: targetFile, Language: "jscript-net",
				Encoding: file.Encoding, SymbolID: sym.ID,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("fingerprint-bound source excerpt failed: err=%v show=%+v", err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: tc.paths,
				Language: "jscript-net", Query: tc.target, Match: "exact",
				MaxFiles: len(tc.paths), MaxResults: 64,
			})
			if err != nil || query.Search == nil || query.Coverage.ResultsTruncated ||
				len(query.Search.Matches) != 1 ||
				query.Search.Matches[0].SymbolID != sym.ID ||
				query.Search.Matches[0].SourceFingerprint != file.SourceFingerprint {
				t.Fatalf("source_query does not link to exact original declaration: err=%v query=%+v", err, query)
			}
			_, neg, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: tc.paths, Language: "jscript-net",
				Query: tc.forbidden, Match: "exact", MaxFiles: len(tc.paths), MaxSymbols: 32,
			})
			if err != nil || len(neg.Symbols) != 0 {
				t.Fatalf("unrelated call/reference fabricated as declaration: err=%v symbols=%+v", err, neg.Symbols)
			}
			t.Logf("local handler original navigation passed: selectedFiles=%d indexedSymbols=%d coverageComplete=%v", len(tc.paths), len(outline.Symbols), outline.CoverageComplete)
		})
	}
}
