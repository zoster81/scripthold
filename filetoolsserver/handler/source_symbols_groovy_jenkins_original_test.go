package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Full original Jenkinsfiles are opt-in external fixtures, never checked into Git.
func TestGroovyOriginalJenkinsMCPDeclarationIdentity(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_GROOVY_JENKINS_ROOT")
	if root == "" {
		t.Skip("full original Jenkinsfile corpus not configured")
	}
	for _, tc := range []struct {
		file          string
		positiveQuery string
		wantSymbols   int
		wantName      string
		wantLine      int
	}{
		{"packaging-Jenkinsfile", "jobProperties", 2, "jobProperties", 1},
		{"jenkins-design-Jenkinsfile", "stage", 0, "", 0},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			raw, err := os.ReadFile(path)
			if err != nil || len(raw) == 0 {
				t.Fatalf("original Jenkinsfile missing or empty: %v", err)
			}
			h := NewHandler([]string{root})
			ctx := context.Background()
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "groovy", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || !outline.CoverageComplete || outline.Truncated ||
				len(outline.Symbols) != tc.wantSymbols || len(outline.Files) != 1 {
				t.Fatalf("original outline identities: err=%v failed=%+v output=%+v", err, failed, outline)
			}
			fp := outline.Files[0].SourceFingerprint
			if fp == "" {
				t.Fatal("original fingerprint missing")
			}
			if tc.wantSymbols == 2 {
				want := map[string]int{"jobProperties": 1, "nodeWithTimeout": 73}
				for _, sym := range outline.Symbols {
					line, ok := want[sym.Name]
					if !ok || sym.NameRange.Start.Line != line ||
						sym.ParentQualifiedName != "" || sym.QualifiedName != sym.Name {
						t.Errorf("incorrect actual Groovy definition: %+v", sym)
					}
				}
			}
			failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "groovy",
				Encoding: "utf-8", MaxFiles: 1,
			})
			if err != nil || failed != nil || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint != fp {
				t.Fatalf("original digest/fingerprint mismatch: err=%v failed=%+v", err, failed)
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "groovy", Encoding: "utf-8",
				Query: tc.positiveQuery, Match: "exact", MaxFiles: 1, MaxSymbols: 16,
				FileDetails: "relevant",
			})
			wantFound := 0
			if tc.wantName != "" {
				wantFound = 1
			}
			if err != nil || failed != nil || len(found.Symbols) != wantFound {
				t.Fatalf("original exact find: err=%v failed=%+v found=%+v", err, failed, found)
			}
			if wantFound != 0 {
				sym := found.Symbols[0]
				if sym.Name != tc.wantName || sym.NameRange.Start.Line != tc.wantLine ||
					sym.Kind != "variable" || sym.ParentQualifiedName != "" {
					t.Fatalf("source-authored variable identity missing: %+v", sym)
				}
				failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "groovy", Encoding: "utf-8",
					SymbolID: sym.ID, SourceFingerprint: fp, MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, "def jobProperties = [") {
					t.Fatalf("verified source excerpt missing: err=%v failed=%+v", err, failed)
				}
			}
			failed, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "groovy", Query: tc.positiveQuery, Match: "exact",
				MaxFiles: 1, MaxResults: 16,
			})
			if err != nil || failed != nil || searched.Search == nil ||
				!searched.Coverage.CoverageComplete ||
				len(searched.Search.Matches) != wantFound {
				t.Fatalf("original structural search mismatch: err=%v failed=%+v result=%+v",
					err, failed, searched)
			}
		})
	}
}
