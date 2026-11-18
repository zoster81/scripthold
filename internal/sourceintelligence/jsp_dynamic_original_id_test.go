package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSPPinnedWholeOriginalStaticAndDynamicIDs(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JSP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete immutable JSP originals not configured")
	}
	for _, tc := range []struct {
		file   string
		names  []string
		absent []string
	}{
		{"bookstore-index.jsp", []string{"tags", "cart", "customer", "header", "cart", "form"}, nil},
		{"exam-system-exam.jsp", []string{"pDAO", "remainingTime", "myform"}, []string{"c1", "c2", "c3", "c4"}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("full pinned upstream source must be bounded regular file: %v %v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), "<jsp:useBean id=") {
				t.Fatal("unexpected immutable JSP source")
			}
			result, err := (JSPAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, string(raw)), testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Errorf("complete original JSP file unexpectedly incomplete: %+v", result.Analysis)
			}
			if len(result.Analysis.Symbols) != len(tc.names) {
				t.Errorf("whole original static IDs: got %d want %d", len(result.Analysis.Symbols), len(tc.names))
			}
			for i, sym := range result.Analysis.Symbols {
				if i >= len(tc.names) {
					t.Errorf("false stable original host ID: %+v", sym)
					continue
				}
				if sym.Name != tc.names[i] || sym.Kind != SymbolKindEntity ||
					sym.NativeKind != "id-element" {
					t.Errorf("authored static source ID at %d wrong: %+v", i, sym)
				}
			}
			for _, excluded := range tc.absent {
				for _, sym := range result.Analysis.Symbols {
					if sym.Name == excluded {
						t.Errorf("dynamic expression ID must never be indexed as stable %q: %+v", excluded, sym)
					}
				}
			}
		})
	}
}

func TestJSPDynamicIDExpressionsAreNotStaticSourceDeclarations(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(strings.ReplaceAll(eol, "\r", "CR"), func(t *testing.T) {
			source := strings.Join([]string{
				"<main id=\"stable\" data-name=\"<%=v%>\"></main>",
				"<input id=\"c1<%=i%>\" name=\"ans<%=i%>\">",
				"<input id='prefix<%= i %>suffix'>",
				"<input id=\"prefix${user.id}\">",
				"<input id=\"#{bean.id}\">",
				"<jsp:useBean id=\"bean\" class=\"TestBean\"/>",
				"<div id=\"other\"></div>", "",
			}, eol)
			result, err := (JSPAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("dynamic.jsp", source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Errorf("all complete authored JSP source must be analyzed: %+v", result.Analysis)
			}
			want := []string{"stable", "bean", "other"}
			if len(result.Analysis.Symbols) != len(want) {
				t.Errorf("dynamic id names must not become stable symbols, got %d want %d", len(result.Analysis.Symbols), len(want))
			}
			for i, symbol := range result.Analysis.Symbols {
				if i < len(want) && symbol.Name != want[i] {
					t.Errorf("JSP host ID at %d got %q want %q", i, symbol.Name, want[i])
				}
			}
		})
	}
}
