package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCSharpPinnedOriginalWholeSerilogNamespaceNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CSHARP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole C# originals not configured")
	}
	path := filepath.Join(root, "serilog-LogEventLevel.cs")
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		Language: "csharp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64, IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated ||
		len(outlined.Symbols) != 8 {
		t.Fatalf("whole original C# detailed outline: err=%v failed=%+v symbols=%+v", err, failed, outlined.Symbols)
	}
	var nsID, enumID string
	memberLines := map[string]int{
		"Verbose": 26, "Debug": 32, "Information": 38,
		"Warning": 43, "Error": 49, "Fatal": 55,
	}
	seenMembers := make(map[string]bool, len(memberLines))
	for _, s := range outlined.Symbols {
		switch s.Name {
		case "Serilog.Events":
			if string(s.Kind) != "namespace" || s.NativeKind != "namespace" ||
				s.NameRange.Start.Line != 15 || s.NameRange.Start.Column != 11 ||
				s.NameRange.End.Line != 15 || s.NameRange.End.Column != 25 {
				t.Errorf("whole authored namespace range wrong: %+v", s)
			}
			nsID = s.ID
		case "LogEventLevel":
			if string(s.Kind) != "enum" || s.ParentQualifiedName != "Serilog.Events" ||
				s.NameRange.Start.Line != 20 || s.ParentID != nsID {
				t.Errorf("real enum owner changed: %+v", s)
			}
			enumID = s.ID
		case "Verbose", "Debug", "Information", "Warning", "Error", "Fatal":
			if string(s.Kind) != "constant" || s.NativeKind != "enum-value" ||
				s.ParentQualifiedName != "Serilog.Events.LogEventLevel" || s.ParentID != enumID ||
				s.QualifiedName != "Serilog.Events.LogEventLevel."+s.Name ||
				s.NameRange.Start.Line != memberLines[s.Name] || seenMembers[s.Name] {
				t.Errorf("original enum member changed: %+v", s)
			}
			seenMembers[s.Name] = true
		default:
			t.Errorf("unplanned whole-original symbol: %+v", s)
		}
	}
	if nsID == "" || enumID == "" || len(seenMembers) != len(memberLines) ||
		len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatal("authored namespace or source fingerprint missing")
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "csharp", Encoding: "utf-8",
		Query: "Serilog.Events", Match: "exact", MaxFiles: 1, MaxSymbols: 8,
	})
	if err != nil || failed != nil || len(found.Symbols) != 1 || found.Symbols[0].NameRange.Start.Column != 11 {
		t.Fatalf("namespace find name range: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: nsID, Language: "csharp", Encoding: "utf-8",
		SourceFingerprint: outlined.Files[0].SourceFingerprint, MaxBytes: 2048,
	})
	if err != nil || failed != nil || shown.Show == nil ||
		!strings.HasPrefix(strings.TrimSpace(shown.Show.Text), "namespace Serilog.Events;") {
		t.Fatalf("source-backed exact namespace show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "csharp", Encoding: "utf-8",
		Query: "Serilog.Events", Match: "exact", Mode: "structural", MaxResults: 8,
	})
	if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) != 1 {
		t.Fatalf("source structural namespace search: err=%v failed=%+v search=%+v", err, failed, searched.Search)
	}
}

func TestCSharpPinnedIndependentWholeDotnetOriginalNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CSHARP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole C# originals not configured")
	}
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{filepath.Join(root, "dotnet-IComparable.cs")},
		Presentation: "detailed", Language: "csharp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated ||
		len(outlined.Symbols) != 5 {
		t.Fatalf("whole independent dotnet original changed: err=%v failed=%+v symbols=%+v", err, failed, outlined.Symbols)
	}
	for _, s := range outlined.Symbols {
		if s.Name == "System" && (s.NameRange.Start.Line != 4 ||
			s.NameRange.Start.Column != 11 || s.NameRange.End.Column != 17) {
			t.Errorf("simple C# namespace must remain source-faithful: %+v", s)
		}
	}
}
