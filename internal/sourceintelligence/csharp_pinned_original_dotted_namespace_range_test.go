package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCSharpPinnedWholeSerilogDottedNamespaceNameRange(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CSHARP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned complete C# originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "serilog-LogEventLevel.cs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 1521 {
		t.Fatalf("upstream Serilog original changed: %d bytes", len(b))
	}
	for _, tc := range []struct{ name, src string }{
		{"git-lf", string(b)}, {"worktree-crlf", strings.ReplaceAll(string(b), "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CSharpAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("LogEventLevel.cs", tc.src), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Symbols) != 8 {
				t.Fatalf("complete original namespace/enum and six enum-value symbols expected: count=%d complete=%v truncated=%v",
					len(result.Analysis.Symbols), result.Analysis.CoverageComplete, result.Analysis.Truncated)
			}
			found := symbolsByQualifiedName(result.Analysis.Symbols)
			ns, ok := found["Serilog.Events"]
			if !ok || ns.Kind != SymbolKindNamespace || ns.NativeKind != "namespace" ||
				ns.DeclarationRange.Start.Line != 15 || ns.NameRange.Start.Line != 15 ||
				ns.NameRange.Start.Column != 11 || ns.NameRange.End.Column != 25 {
				t.Errorf("complete authored namespace nameRange must select Serilog.Events on line15 col11-25: %+v exists=%v", ns, ok)
			}
			enum, ok := found["Serilog.Events.LogEventLevel"]
			if !ok || enum.Kind != SymbolKindEnum || enum.NameRange.Start.Line != 20 ||
				enum.ParentQualifiedName != "Serilog.Events" {
				t.Errorf("following genuine enum owner and source line wrong: %+v exists=%v", enum, ok)
			}
			for name, line := range map[string]int{
				"Verbose": 26, "Debug": 32, "Information": 38,
				"Warning": 43, "Error": 49, "Fatal": 55,
			} {
				symbol, present := found["Serilog.Events.LogEventLevel."+name]
				if !present || symbol.Kind != SymbolKindConstant || symbol.NativeKind != "enum-value" ||
					symbol.ParentQualifiedName != "Serilog.Events.LogEventLevel" || symbol.NameRange.Start.Line != line ||
					symbol.ParentID != enum.ID {
					t.Errorf("original enum member %s line %d: %+v exists=%v", name, line, symbol, present)
				}
			}
		})
	}
}

func TestCSharpPinnedIndependentWholeDotnetNamespaceControl(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CSHARP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned complete C# originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "dotnet-IComparable.cs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 3813 {
		t.Fatalf("upstream dotnet original changed: %d bytes", len(b))
	}
	result, err := (CSharpAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("IComparable.cs", string(b)), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 5 {
		t.Fatalf("independent complete five supported declarations: count=%d complete=%v", len(result.Analysis.Symbols), result.Analysis.CoverageComplete)
	}
	namespaceCount, interfaces, methods := 0, 0, 0
	for _, s := range result.Analysis.Symbols {
		switch s.Kind {
		case SymbolKindNamespace:
			namespaceCount++
			if s.Name != "System" || s.NameRange.Start.Line != 4 ||
				s.NameRange.Start.Column != 11 || s.NameRange.End.Column != 17 {
				t.Errorf("simple namespace regressed: %+v", s)
			}
		case SymbolKindInterface:
			interfaces++
			if s.Name != "IComparable" || s.ParentQualifiedName != "System" ||
				(s.NameRange.Start.Line != 11 && s.NameRange.Start.Line != 44) {
				t.Errorf("interface original: %+v", s)
			}
		case SymbolKindMethod:
			methods++
			if s.Name != "CompareTo" || s.ParentQualifiedName != "System.IComparable" ||
				(s.NameRange.Start.Line != 39 && s.NameRange.Start.Line != 71) {
				t.Errorf("authored overload: %+v", s)
			}
		default:
			t.Errorf("original unexpected symbol: %+v", s)
		}
	}
	if namespaceCount != 1 || interfaces != 2 || methods != 2 {
		t.Errorf("all original declarations mismatch: ns=%d interfaces=%d methods=%d", namespaceCount, interfaces, methods)
	}
}

func TestCSharpDottedNamespaceRangeFileAndBlockScope(t *testing.T) {
	for _, tc := range []struct {
		name, src, qualified string
		lastColumn           int
	}{
		{"file-scoped", "namespace App.Models;\npublic class Box {}\n", "App.Models", 21},
		{"block-scoped", "namespace App.Models { public class Box {} }\n", "App.Models", 21},
		{"single-identifier", "namespace System; public class Box {}\n", "System", 17},
		{"three-segments", "namespace One.Two.Three;\nclass Box {}\n", "One.Two.Three", 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CSharpAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("namespace.cs", tc.src), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			ns, ok := byName[tc.qualified]
			if !ok || ns.Kind != SymbolKindNamespace ||
				ns.NameRange.Start.Line != 1 || ns.NameRange.Start.Column != 11 ||
				ns.NameRange.End.Column != tc.lastColumn {
				t.Errorf("namespace full source name range mismatch: %+v exists=%v", ns, ok)
			}
			if _, ok := byName[tc.qualified+".Box"]; !ok {
				t.Errorf("following owned class missing: %+v", result.Analysis.Symbols)
			}
		})
	}
}
