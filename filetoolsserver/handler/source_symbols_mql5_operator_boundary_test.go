package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceSymbolsMQL5OperatorOwnershipAndVerifiedShow(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "operators.mqh")
	source := strings.Join([]string{
		"class CJAVal {",
		"  long m_iv;",
		"public:",
		"  void operator=(const int a) { m_iv=a; }",
		"  bool operator==(const int a) { return m_iv==a; }",
		"  bool operator!=(const int a) { return m_iv!=a; }",
		"  long ToInt() const { return m_iv; }",
		"};", "",
	}, "\r\n")
	bytes := []byte{0xff, 0xfe}
	for _, c := range []byte(source) {
		bytes = append(bytes, c, 0)
	}
	if err := os.WriteFile(path, bytes, 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "compact", Paths: []string{path},
		Language: "mql5", MaxFiles: 1, MaxSymbols: 32,
	})
	if err != nil || failed != nil || !outline.CoverageComplete || outline.SymbolCount != 6 || len(outline.OutlineFiles) != 1 || outline.OutlineFiles[0].Encoding != "utf-16-le" {
		t.Fatalf("MQL5 outline: err=%v tool=%+v result=%+v", err, failed, outline)
	}
	found := map[string]*SourceOutlineNode{}
	for _, s := range outline.OutlineFiles[0].Symbols {
		if found[s.QualifiedName] != nil {
			t.Fatalf("duplicate %s", s.QualifiedName)
		}
		found[s.QualifiedName] = s
	}
	for name, kind := range map[string]string{
		"CJAVal": "class", "CJAVal.m_iv": "field", "CJAVal.operator=": "operator",
		"CJAVal.operator==": "operator", "CJAVal.operator!=": "operator", "CJAVal.ToInt": "method",
	} {
		s := found[name]
		if s == nil || string(s.Kind) != kind {
			t.Errorf("missing %s as %s: %+v", name, kind, s)
		}
	}
	toInt := found["CJAVal.ToInt"]
	if toInt == nil || toInt.Line != 7 {
		t.Fatalf("wrong method start: %+v", toInt)
	}
	failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "show", Path: path, Language: "mql5", Encoding: "utf-16-le",
		SymbolID: toInt.SymbolID, SourceFingerprint: outline.OutlineFiles[0].SourceFingerprint, MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil || !strings.HasPrefix(strings.TrimLeft(shown.Show.Text, " \t"), "long ToInt() const") {
		t.Fatalf("show includes unrelated prior operator: err=%v tool=%+v show=%+v", err, failed, shown.Show)
	}
	failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "digest", Presentation: "compact", Paths: []string{path}, Language: "mql5", MaxFiles: 1,
	})
	if err != nil || failed != nil || !digest.CoverageComplete || digest.SymbolCount != outline.SymbolCount {
		t.Fatalf("digest mismatch: err=%v tool=%+v digest=%+v", err, failed, digest)
	}
	failed, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "mql5", Query: "ToInt", Mode: "structural", Match: "exact", MaxResults: 4,
	})
	if err != nil || failed != nil || query.Search == nil || len(query.Search.Matches) != 1 || query.Search.Matches[0].Range.Start.Line != 7 {
		t.Fatalf("search mismatch: err=%v tool=%+v query=%+v", err, failed, query.Search)
	}
}
