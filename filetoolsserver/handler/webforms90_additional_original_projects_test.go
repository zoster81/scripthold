package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Further independent upstream projects, distinct from the earlier DevExpress
// and Telerik WebForms navigation cases. Original applications are never run.
func TestWebForms90AdditionalOriginalProjectNavigation(t *testing.T) {
	easyRoot := os.Getenv("SCRIPTHOLD_REAL_WEBFORMS90_EASYQUERY_ROOT")
	expenseRoot := os.Getenv("SCRIPTHOLD_REAL_WEBFORMS90_EXPENSE_ROOT")
	if easyRoot == "" && expenseRoot == "" {
		t.Skip("two independently pinned additional WebForms projects not configured")
	}
	if easyRoot == "" || expenseRoot == "" {
		t.Fatal("both additional original WebForms projects must be configured")
	}
	cases := []struct {
		name, root, page, code, target, pageAnchor, related, codeClass, codeEvidence, negative string
		targetLine, relatedLine, codeClassLine, methodLine                                     int
		directive                                                                              string
	}{
		{
			name: "EasyQuery landing content versus linked Page_Load",
			root: easyRoot, page: "EasyQueryDefault.aspx", code: "Default.aspx.cs",
			target: "BodyContent", targetLine: 3,
			pageAnchor: "ID=\"BodyContent\"", directive: "CodeBehind=\"Default.aspx.cs\"",
			codeClass: "_Default", codeClassLine: 10, methodLine: 12,
			codeEvidence: "protected void Page_Load(", negative: "ApproveLink",
		},
		{
			name: "WebExpense approval link visibility versus role-based Page_Load",
			root: expenseRoot, page: "index.aspx", code: "index.aspx.cs",
			target: "ApproveLink", targetLine: 38, pageAnchor: "id=\"ApproveLink\"",
			directive: "CodeFile=\"index.aspx.cs\"",
			related:   "SubmitLink", relatedLine: 33,
			codeClass: "MainPage", codeClassLine: 40, methodLine: 101,
			codeEvidence: "ApproveLink.Visible = true;", negative: "RadEditor2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pagePath := filepath.Join(tc.root, tc.page)
			codePath := filepath.Join(tc.root, tc.code)
			for _, path := range []string{pagePath, codePath} {
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("original source missing, nonregular or oversized: %s info=%+v err=%v", path, info, err)
				}
			}
			pageBytes, err := os.ReadFile(pagePath)
			if err != nil {
				t.Fatal(err)
			}
			codeBytes, err := os.ReadFile(codePath)
			if err != nil {
				t.Fatal(err)
			}
			pageSource, codeSource := string(pageBytes), string(codeBytes)
			if !strings.Contains(pageSource, tc.pageAnchor) ||
				!strings.Contains(pageSource, tc.directive) ||
				!strings.Contains(codeSource, "class "+tc.codeClass) ||
				!strings.Contains(codeSource, "void Page_Load(") ||
				!strings.Contains(codeSource, tc.codeEvidence) {
				t.Fatal("original source differs from independently read page/code-behind oracle")
			}
			if tc.related != "" && !strings.Contains(pageSource, "id=\""+tc.related+"\"") {
				t.Fatalf("related original server control %s missing", tc.related)
			}
			ctx, h := context.Background(), NewHandler([]string{tc.root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{pagePath}, Language: "aspnet-webforms", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.Truncated ||
				digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("WebForms original page digest incomplete: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{pagePath},
				Language: "aspnet-webforms", MaxFiles: 1, MaxSymbols: 96,
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated ||
				outline.FilesParsed != 1 {
				t.Fatalf("WebForms original page outline incomplete: err=%v outline=%+v", err, outline)
			}
			targetCount, relatedCount := 0, 0
			for _, symbol := range outline.Symbols {
				if symbol.Name == tc.target && symbol.NameRange.Start.Line == tc.targetLine {
					targetCount++
				}
				if tc.related != "" && symbol.Name == tc.related &&
					symbol.NameRange.Start.Line == tc.relatedLine {
					relatedCount++
				}
				if symbol.Name == tc.negative {
					t.Errorf("irrelevant original WebForms control fabricated: %+v", symbol)
				}
			}
			if targetCount != 1 || tc.related != "" && relatedCount != 1 {
				t.Fatalf("original page edit region or neighbor missing: target=%d related=%d symbols=%+v",
					targetCount, relatedCount, outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{pagePath}, Language: "aspnet-webforms",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 48,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated ||
				len(found.Symbols) != 1 || len(found.Files) != 1 {
				t.Fatalf("exact original WebForms target unsafe: err=%v found=%+v", err, found)
			}
			sym, file := found.Symbols[0], found.Files[0]
			if sym.Path != pagePath || sym.NameRange.Start.Line != tc.targetLine ||
				sym.ID == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint ||
				file.Encoding == "" {
				t.Fatalf("original WebForms source identity wrong: symbol=%+v file=%+v", sym, file)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: pagePath, Language: "aspnet-webforms",
				SymbolID: sym.ID, SourceFingerprint: file.SourceFingerprint,
				Encoding: file.Encoding, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.pageAnchor) {
				t.Fatalf("original control fingerprint-bound excerpt missing: err=%v shown=%+v", err, shown.Show)
			}
			_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{pagePath},
				Language: "aspnet-webforms", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 48,
			})
			linked := 0
			if searched.Search != nil {
				for _, hit := range searched.Search.Matches {
					if hit.Name == tc.target && hit.SymbolID == sym.ID &&
						hit.SourceFingerprint == file.SourceFingerprint {
						linked++
					}
				}
			}
			if err != nil || searched.Search == nil || !searched.Coverage.CoverageComplete ||
				searched.Coverage.ResultsTruncated || linked != 1 {
				t.Fatalf("original markup structural search diverges: err=%v query=%+v", err, searched)
			}
			_, relations, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "relations", Relation: "dependencies", Paths: []string{pagePath},
				Language: "aspnet-webforms",
				Subject: &SourceSelectorInput{Kind: "path", Path: pagePath,
					SourceFingerprint: file.SourceFingerprint},
				MaxFiles: 1, MaxResults: 32, MaxNodes: 32, MaxEdges: 64, MaxDepth: 2,
			})
			depFound := false
			if relations.Relations != nil {
				for _, edge := range relations.Relations.Relations {
					if edge.Target.QualifiedName == tc.code &&
						edge.Source.SourceFingerprint == file.SourceFingerprint {
						depFound = true
					}
				}
			}
			if err != nil || relations.Relations == nil || !relations.Coverage.CoverageComplete || !depFound {
				t.Fatalf("original code-behind directive relationship lost: err=%v relations=%+v", err, relations)
			}
			_, codeOutline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{codePath},
				Language: "csharp", MaxFiles: 1, MaxSymbols: 96,
			})
			if err != nil || codeOutline.Truncated || codeOutline.FilesParsed != 1 ||
				!codeOutline.CoverageComplete {
				t.Fatalf("original C# code-behind outline failed: err=%v outline=%+v", err, codeOutline)
			}
			classFound, methodFound := false, false
			for _, cs := range codeOutline.Symbols {
				if cs.Name == tc.codeClass && cs.NameRange.Start.Line == tc.codeClassLine {
					classFound = true
				}
				if cs.Name == "Page_Load" && cs.NameRange.Start.Line == tc.methodLine &&
					strings.HasSuffix(cs.ParentQualifiedName, tc.codeClass) {
					methodFound = true
				}
			}
			if !classFound || !methodFound {
				t.Fatalf("actual original code-behind class or handler missing: class=%v method=%v symbols=%+v",
					classFound, methodFound, codeOutline.Symbols)
			}
			_, codeFound, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{codePath}, Language: "csharp",
				Query: "Page_Load", Match: "exact", MaxFiles: 1, MaxSymbols: 16,
				FileDetails: "relevant",
			})
			if err != nil || len(codeFound.Files) != 1 || len(codeFound.Symbols) != 1 {
				t.Fatalf("code-behind exact handler unavailable: err=%v found=%+v", err, codeFound)
			}
			method, csFile := codeFound.Symbols[0], codeFound.Files[0]
			if method.NameRange.Start.Line != tc.methodLine || csFile.SourceFingerprint == "" ||
				method.ID == "" {
				t.Fatalf("unsafe original code-behind handler selector: %+v %+v", method, csFile)
			}
			_, codeShown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: codePath, Language: "csharp",
				SymbolID: method.ID, SourceFingerprint: csFile.SourceFingerprint,
				Encoding: csFile.Encoding, MaxBytes: 8192,
			})
			if err != nil || codeShown.Show == nil ||
				!strings.Contains(codeShown.Show.Text, tc.codeEvidence) {
				t.Fatalf("original C# behavior not in fingerprint-bound method: err=%v shown=%+v",
					err, codeShown.Show)
			}
			_, unrelated, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{codePath}, Language: "csharp",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || len(unrelated.Symbols) != 0 {
				t.Fatalf("runtime control reference fabricated as C# declaration: err=%v result=%+v",
					err, unrelated.Symbols)
			}
			t.Logf("original maintenance navigation PASS: markup controls=%d, linked C# declarations=%d",
				len(outline.Symbols), len(codeOutline.Symbols))
		})
	}
}
