package sourceintelligence

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// semanticOracleExpectation is an independently specified declaration, not a
// snapshot copied from an analyzer's output.
type semanticOracleExpectation struct {
	QualifiedName string     `json:"qualifiedName"`
	Kind          SymbolKind `json:"kind"`
	Line          int        `json:"line"`
	Parent        string     `json:"parent,omitempty"`
	NativeKind    string     `json:"nativeKind,omitempty"`
}

type semanticOracleCase struct {
	ID               string                      `json:"id"`
	Language         string                      `json:"language"`
	Path             string                      `json:"path"`
	Source           string                      `json:"source"`
	Present          []semanticOracleExpectation `json:"present"`
	Forbidden        []string                    `json:"forbidden"`
	CoverageComplete bool                        `json:"coverageComplete"`
}

type semanticOracleMatrix struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Purpose       string               `json:"purpose"`
	Cases         []semanticOracleCase `json:"cases"`
}

func loadSemanticOracleMatrix(t *testing.T) semanticOracleMatrix {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "semantic_oracle_matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var matrix semanticOracleMatrix
	if err := decoder.Decode(&matrix); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("oracle matrix must contain exactly one JSON value: %v", err)
	}
	if matrix.SchemaVersion != 1 || len(matrix.Cases) == 0 || matrix.Purpose == "" {
		t.Fatalf("invalid semantic oracle manifest: version=%d cases=%d", matrix.SchemaVersion, len(matrix.Cases))
	}
	return matrix
}

func TestSemanticOracleMatrix(t *testing.T) {
	matrix := loadSemanticOracleMatrix(t)
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	seenCases := map[string]bool{}
	coveredLanguages := map[string]bool{}
	for _, item := range matrix.Cases {
		if item.ID == "" || seenCases[item.ID] {
			t.Fatalf("missing or duplicate semantic oracle ID %q", item.ID)
		}
		seenCases[item.ID] = true
		if item.Language == "" || item.Source == "" || item.Path == "" ||
			filepath.IsAbs(item.Path) || filepath.Base(item.Path) != item.Path ||
			strings.Contains(item.Path, ":") || len(item.Present) == 0 || len(item.Forbidden) == 0 {
			t.Fatalf("invalid oracle %q: a portable sample must contain positive and negative assertions", item.ID)
		}
		descriptor, ok := registry.Resolve(item.Language)
		if !ok || descriptor.ID != item.Language {
			t.Fatalf("oracle %q has no canonical provider %q", item.ID, item.Language)
		}
		analyzer, ok := AnalyzerFor(descriptor)
		if !ok {
			t.Fatalf("oracle %q has no active analyzer", item.ID)
		}
		coveredLanguages[item.Language] = true
		t.Run(item.ID, func(t *testing.T) {
			document := testSourceDocument(item.Path, item.Source)
			analysis, err := analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			assertAnalyzerResultCoordinates(t, analysis)
			if analysis.Analysis.Truncated || analysis.Analysis.CoverageComplete != item.CoverageComplete {
				t.Errorf("coverage or truncation misreported: complete=%v truncated=%v, want complete=%v; diagnostics=%v",
					analysis.Analysis.CoverageComplete, analysis.Analysis.Truncated, item.CoverageComplete, analysis.Analysis.Diagnostics)
			}
			symbols := map[string][]NormalizedSymbol{}
			for _, sym := range analysis.Analysis.Symbols {
				symbols[sym.QualifiedName] = append(symbols[sym.QualifiedName], sym)
			}
			seenExpected := map[string]bool{}
			for _, expected := range item.Present {
				if expected.QualifiedName == "" || expected.Kind == "" || expected.Line <= 0 || seenExpected[expected.QualifiedName] {
					t.Fatalf("invalid or repeated positive oracle %+v", expected)
				}
				seenExpected[expected.QualifiedName] = true
				matches := symbols[expected.QualifiedName]
				if len(matches) != 1 {
					t.Errorf("expected exactly one declaration %q (%s), got %d: %v", expected.QualifiedName, expected.Kind, len(matches), matches)
					continue
				}
				symbol := matches[0]
				if symbol.Kind != expected.Kind || symbol.DeclarationRange.Start.Line != expected.Line {
					t.Errorf("%q got kind=%q line=%d, want kind=%q line=%d", expected.QualifiedName, symbol.Kind,
						symbol.DeclarationRange.Start.Line, expected.Kind, expected.Line)
				}
				if expected.Parent != "" && symbol.ParentQualifiedName != expected.Parent {
					t.Errorf("%q has parent %q, want %q", expected.QualifiedName, symbol.ParentQualifiedName, expected.Parent)
				}
				if expected.NativeKind != "" && symbol.NativeKind != expected.NativeKind {
					t.Errorf("%q has native kind %q, want %q", expected.QualifiedName, symbol.NativeKind, expected.NativeKind)
				}
				if symbol.NameRange.Start.Line != expected.Line {
					t.Errorf("%q name range starts at line %d, expected %d", expected.QualifiedName,
						symbol.NameRange.Start.Line, expected.Line)
				}
			}
			seenForbidden := map[string]bool{}
			for _, name := range item.Forbidden {
				if name == "" || seenForbidden[name] || seenExpected[name] {
					t.Fatalf("invalid, duplicate or contradictory forbidden oracle %q", name)
				}
				seenForbidden[name] = true
				if found := symbols[name]; len(found) > 0 {
					t.Errorf("false structural declaration %q: %v", name, found)
				}
			}
		})
	}
	t.Logf("Positive and negative semantic oracle cases: %d; distinct providers with these checks: %d of %d. The remainder are NOT certified by this matrix.",
		len(matrix.Cases), len(coveredLanguages), len(registry.CapabilityRows()))
}

// TestSemanticOracleCRLFParity guards declaration identity and hierarchy
// independently of source newline encoding. UTF-8 byte offsets may differ.
func TestSemanticOracleCRLFParity(t *testing.T) {
	matrix := loadSemanticOracleMatrix(t)
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	type identity struct {
		QualifiedName string
		Kind          SymbolKind
		Line          int
		Parent        string
	}
	for _, item := range matrix.Cases {
		if !strings.Contains(item.Source, "\n") || strings.Contains(item.Source, "\r\n") {
			continue
		}
		t.Run(item.ID, func(t *testing.T) {
			descriptor, ok := registry.Resolve(item.Language)
			if !ok {
				t.Fatal("canonical provider missing")
			}
			analyzer, ok := AnalyzerFor(descriptor)
			if !ok {
				t.Fatal("provider analyzer missing")
			}
			var signatures [2][]identity
			var coverage [2]bool
			sources := [2]string{item.Source, strings.ReplaceAll(item.Source, "\n", "\r\n")}
			for variant, source := range sources {
				result, err := analyzer.Analyze(context.Background(), testSourceDocument(item.Path, source), testAnalyzeOptions(true, 256))
				if err != nil {
					t.Fatalf("variant %d: %v", variant, err)
				}
				coverage[variant] = result.Analysis.CoverageComplete
				for _, sym := range result.Analysis.Symbols {
					signatures[variant] = append(signatures[variant], identity{
						QualifiedName: sym.QualifiedName, Kind: sym.Kind,
						Line: sym.DeclarationRange.Start.Line, Parent: sym.ParentQualifiedName,
					})
				}
			}
			if !reflect.DeepEqual(signatures[0], signatures[1]) || coverage[0] != coverage[1] {
				t.Fatalf("LF/CRLF semantic drift; LF=%v coverage=%v, CRLF=%v coverage=%v",
					signatures[0], coverage[0], signatures[1], coverage[1])
			}
		})
	}
}
