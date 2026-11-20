package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestAssemblyBareMetalEQUOriginal(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_ASSEMBLY_BAREMETAL_APIC")
	if path == "" {
		t.Skip("set SCRIPTHOLD_REAL_ASSEMBLY_BAREMETAL_APIC to the qualified original APIC assembly source")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 1<<20 {
		t.Fatal("original assembly fixture exceeds 1 MiB")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := sourceDocumentForScanner(string(content))
	doc.Path = path
	result, err := (AssemblyAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("original assembly analysis incomplete: %+v", result.Analysis)
	}
	symbols := symbolsByQualifiedName(result.Analysis.Symbols)
	for name, expected := range map[string]struct {
		kind SymbolKind
		line int
	}{
		"APIC_VER":     {SymbolKindConstant, 83},
		"APIC_ID":      {SymbolKindConstant, 82},
		"os_apic_init": {SymbolKindLabel, 14},
		"os_apic_read": {SymbolKindLabel, 27},
	} {
		symbol, found := symbols[name]
		if !found || symbol.Kind != expected.kind || symbol.NameRange.Start.Line != expected.line {
			t.Errorf("%s got %+v found=%v, expected kind=%s line=%d", name, symbol, found, expected.kind, expected.line)
		}
	}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Name == "equ" || symbol.Name == "mov" {
			t.Errorf("assembly instruction/keyword became a declaration: %+v", symbol)
		}
	}
}

func TestAssemblyBareMetalEQUWithLineEndings(t *testing.T) {
	const source = "os_apic_init:\n  ret\nAPIC_ID equ 0x020\nAPIC_VER equ 0x030\nFake equ ; missing operand\n; Ghost equ 1\n"
	for _, tc := range []struct{ name, code string }{{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (AssemblyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.code), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("incomplete: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range map[string]SymbolKind{"os_apic_init": SymbolKindLabel, "APIC_ID": SymbolKindConstant, "APIC_VER": SymbolKindConstant} {
				sym, ok := byName[name]
				if !ok || sym.Kind != kind {
					t.Errorf("%s got %+v present=%v; want %s", name, sym, ok, kind)
				}
			}
			for _, name := range []string{"Fake", "Ghost", "mov", "equ"} {
				if sym, ok := byName[name]; ok {
					t.Errorf("invented assembly symbol %s: %+v", name, sym)
				}
			}
		})
	}
}
