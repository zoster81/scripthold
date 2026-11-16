package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Mini-redis Get and Set each declare an inherent impl whose methods share
// names with struct fields. Source positions and kinds disambiguate the two.
func TestRustOriginalMiniRedisCommandNavigation(t *testing.T) {
	type expected struct {
		line   int
		name   string
		kind   SymbolKind
		parent string
	}
	check := func(t *testing.T, source string, wants []expected) {
		t.Helper()
		result, err := (RustAnalyzer{}).Analyze(context.Background(),
			sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
			t.Fatalf("original valid Rust unexpectedly incomplete: %+v", result.Analysis)
		}
		if len(result.Analysis.Symbols) != len(wants) {
			t.Fatalf("Rust declarations=%d want=%d: %v",
				len(result.Analysis.Symbols), len(wants),
				sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		used := make([]bool, len(wants))
		for _, sym := range result.Analysis.Symbols {
			if sym.ID == "" || sym.NameRange.Start.Line < 1 {
				t.Fatalf("Rust returned an unsafe source target: %+v", sym)
			}
			if strings.Contains(sym.Name, "Phantom") || sym.Name == "parse" ||
				sym.Name == "response" || sym.Name == "duration_from_ms_str" {
				t.Fatalf("comment or local code invented a declaration: %+v", sym)
			}
			found := false
			for i, want := range wants {
				if !used[i] && sym.NameRange.Start.Line == want.line &&
					sym.Name == want.name && sym.Kind == want.kind &&
					sym.ParentQualifiedName == want.parent {
					used[i], found = true, true
					break
				}
			}
			if !found {
				t.Errorf("unexpected Rust declaration at line %d: %s kind=%s parent=%s",
					sym.NameRange.Start.Line, sym.Name, sym.Kind, sym.ParentQualifiedName)
			}
		}
		for i, wasUsed := range used {
			if !wasUsed {
				t.Errorf("missing original declaration: %+v", wants[i])
			}
		}
	}
	const portable = "// pub struct Phantom;\npub struct Get {\n    key: String,\n}\nimpl Get {\n    pub fn key(&self) -> &str {\n        &self.key\n    }\n}\n"
	portableWants := []expected{
		{2, "Get", SymbolKindStruct, ""},
		{3, "key", SymbolKindField, "Get"},
		{5, "Get", SymbolKindImplementation, ""},
		{6, "key", SymbolKindMethod, "Get"},
	}
	for _, tc := range []struct{ name, eol string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			check(t, strings.ReplaceAll(portable, "\n", tc.eol), portableWants)
		})
	}
	root := os.Getenv("SCRIPTHOLD_REAL_RUST_MINI_REDIS_ROOT")
	if root == "" {
		return
	}
	for _, tc := range []struct {
		name, anchor string
		wants        []expected
	}{
		{
			name: "get.rs", anchor: "pub struct Get {",
			wants: []expected{
				{12, "Get", SymbolKindStruct, ""},
				{14, "key", SymbolKindField, "Get"},
				{17, "Get", SymbolKindImplementation, ""},
				{19, "new", SymbolKindMethod, "Get"},
				{26, "key", SymbolKindMethod, "Get"},
				{50, "parse_frames", SymbolKindMethod, "Get"},
				{64, "apply", SymbolKindMethod, "Get"},
				{87, "into_frame", SymbolKindMethod, "Get"},
			},
		},
		{
			name: "set.rs", anchor: "pub struct Set {",
			wants: []expected{
				{21, "Set", SymbolKindStruct, ""},
				{23, "key", SymbolKindField, "Set"},
				{26, "value", SymbolKindField, "Set"},
				{29, "expire", SymbolKindField, "Set"},
				{32, "Set", SymbolKindImplementation, ""},
				{37, "new", SymbolKindMethod, "Set"},
				{46, "key", SymbolKindMethod, "Set"},
				{51, "value", SymbolKindMethod, "Set"},
				{56, "expire", SymbolKindMethod, "Set"},
				{80, "parse_frames", SymbolKindMethod, "Set"},
				{128, "apply", SymbolKindMethod, "Set"},
				{144, "into_frame", SymbolKindMethod, "Set"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, "src", "cmd", tc.name)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original Rust source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if strings.Count(source, tc.anchor) != 1 {
				t.Fatal("original mini-redis source anchor changed")
			}
			check(t, source, tc.wants)
		})
	}
}
