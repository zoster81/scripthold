package sourceintelligence

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

var _ SourceAnalyzer = GoAnalyzer{}

var goAnalyzerTestOptions = AnalyzeOptions{
	IncludeSignatures: true,
	Limits: SymbolBuilderLimits{
		MaxSymbols:        512,
		MaxSignatureBytes: 16 * 1024,
		MaxDiagnostics:    64,
	},
}

func TestGoAnalyzerDeclarationsDependenciesRangesAndSignatures(t *testing.T) {
	text := `//go:build linux
// package fakecomment
package sample

import (
    ctx "context"
    _ "embed"
    "io"
)

const (
    Alpha = 1
    beta = 2
)

var (
    Global int
    pairA, pairB = 1, 2
)

type Alias = string

type Box[T any] struct {
    Value T
    io.Reader
    hidden int
}

type Service interface {
    Run(ctx.Context) error
    io.Closer
}

func Map[T any, R comparable](in T) R {
    var zero R
    return zero
}

func (b *Box[T]) Get() T { return b.Value }
func (b Box[T]) Size() int { return 1 }

func outer() {
    text := "func FakeString() {}"
    _ = text
    _ = func() { /* func FakeLiteral() {} */ }
}
`
	document := sourceDocumentForScanner(text)
	document.Path = "sample.go"
	analyzer := GoAnalyzer{}
	if analyzer.ID() != AnalyzerGo || analyzer.Language() != "go" {
		t.Fatalf("Go analyzer identity = %q/%q", analyzer.ID(), analyzer.Language())
	}

	result, err := analyzer.Analyze(context.Background(), document, goAnalyzerTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("unexpected Go coverage: %+v", result.Analysis)
	}

	byQualified := symbolsByQualifiedName(result.Analysis.Symbols)
	expectedKinds := map[string]SymbolKind{
		"sample":                SymbolKindPackage,
		"sample.Alpha":          SymbolKindConstant,
		"sample.beta":           SymbolKindConstant,
		"sample.Global":         SymbolKindVariable,
		"sample.pairA":          SymbolKindVariable,
		"sample.pairB":          SymbolKindVariable,
		"sample.Alias":          SymbolKindAlias,
		"sample.Box":            SymbolKindStruct,
		"sample.Box.Value":      SymbolKindField,
		"sample.Box.Reader":     SymbolKindField,
		"sample.Box.hidden":     SymbolKindField,
		"sample.Service":        SymbolKindInterface,
		"sample.Service.Run":    SymbolKindMethod,
		"sample.Service.Closer": SymbolKindField,
		"sample.Map":            SymbolKindFunction,
		"sample.Box.Get":        SymbolKindMethod,
		"sample.Box.Size":       SymbolKindMethod,
		"sample.outer":          SymbolKindFunction,
	}
	for qualifiedName, kind := range expectedKinds {
		symbol, ok := byQualified[qualifiedName]
		if !ok {
			t.Fatalf("missing %s; symbols=%v", qualifiedName, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		if symbol.Kind != kind || symbol.Language != "go" || symbol.Analyzer != string(AnalyzerGo) || symbol.Evidence != SymbolEvidenceStructural {
			t.Fatalf("%s = %+v, want kind=%s Go structural", qualifiedName, symbol, kind)
		}
		assertLowerHex64(t, qualifiedName+" id", symbol.ID)
	}
	for _, fake := range []string{"sample.fakecomment", "sample.FakeString", "sample.FakeLiteral"} {
		if _, exists := byQualified[fake]; exists {
			t.Fatalf("declaration-looking comment/string/literal leaked as %s", fake)
		}
	}

	box := byQualified["sample.Box"]
	for _, childName := range []string{"sample.Box.Value", "sample.Box.Reader", "sample.Box.hidden", "sample.Box.Get", "sample.Box.Size"} {
		child := byQualified[childName]
		if child.ParentQualifiedName != "sample.Box" || child.ParentID != box.ID {
			t.Fatalf("%s parent = %q/%q, want Box %q", childName, child.ParentQualifiedName, child.ParentID, box.ID)
		}
	}
	service := byQualified["sample.Service"]
	for _, childName := range []string{"sample.Service.Run", "sample.Service.Closer"} {
		child := byQualified[childName]
		if child.ParentQualifiedName != "sample.Service" || child.ParentID != service.ID {
			t.Fatalf("%s parent = %q/%q, want Service %q", childName, child.ParentQualifiedName, child.ParentID, service.ID)
		}
	}

	if !containsString(byQualified["sample.Box.Get"].Modifiers, "pointer-receiver") {
		t.Fatalf("pointer receiver modifiers = %v", byQualified["sample.Box.Get"].Modifiers)
	}
	if !containsString(byQualified["sample.Box.Size"].Modifiers, "value-receiver") {
		t.Fatalf("value receiver modifiers = %v", byQualified["sample.Box.Size"].Modifiers)
	}
	if byQualified["sample.Alpha"].Visibility != VisibilityPublic || byQualified["sample.beta"].Visibility != VisibilityPackage {
		t.Fatalf("Go visibility normalization Alpha=%q beta=%q", byQualified["sample.Alpha"].Visibility, byQualified["sample.beta"].Visibility)
	}

	for qualifiedName, want := range map[string]string{
		"sample.Box":      "type Box[T any] struct",
		"sample.Map":      "func Map[T any, R comparable](in T) R",
		"sample.Box.Get":  "func (b *Box[T]) Get() T",
		"sample.Box.Size": "func (b Box[T]) Size() int",
	} {
		if got := byQualified[qualifiedName].Signature; got != want {
			t.Fatalf("%s signature = %q, want %q", qualifiedName, got, want)
		}
		if byQualified[qualifiedName].SignatureRange == nil {
			t.Fatalf("%s is missing signature range", qualifiedName)
		}
	}
	get := byQualified["sample.Box.Get"]
	if get.NameRange.Start.Line != 39 || get.DeclarationRange.Start.Line != 39 || get.BodyRange == nil {
		t.Fatalf("Get ranges = name=%+v declaration=%+v body=%+v", get.NameRange, get.DeclarationRange, get.BodyRange)
	}

	if got, want := result.Dependencies, []StructuralDependency{
		{Kind: StructuralDependencyImport, Value: "context", Alias: "ctx", Evidence: SymbolEvidenceStructural},
		{Kind: StructuralDependencyImport, Value: "embed", Alias: "_", Evidence: SymbolEvidenceStructural},
		{Kind: StructuralDependencyImport, Value: "io", Evidence: SymbolEvidenceStructural},
	}; len(got) != len(want) {
		t.Fatalf("dependencies = %+v, want %d imports", got, len(want))
	} else {
		for index := range want {
			if got[index].Kind != want[index].Kind || got[index].Value != want[index].Value || got[index].Alias != want[index].Alias || got[index].Evidence != want[index].Evidence {
				t.Fatalf("dependency[%d] = %+v, want %+v", index, got[index], want[index])
			}
			if got[index].Range.Start.Line < 1 || got[index].Range.End.Line < got[index].Range.Start.Line {
				t.Fatalf("dependency[%d] range = %+v", index, got[index].Range)
			}
		}
	}

	second, err := analyzer.Analyze(context.Background(), document, goAnalyzerTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, second) {
		t.Fatalf("Go analyzer is nondeterministic:\nfirst=%+v\nsecond=%+v", result, second)
	}
}

func TestGoAnalyzerGroupedTypesNamedTypesAndInterfaceMembers(t *testing.T) {
	text := `package grouped

type (
    Number int
    Text = string
    Generic[T any] []T
)

type Constraint interface {
    ~int | ~string
    String() string
}
`
	result := analyzeGoText(t, text, goAnalyzerTestOptions)
	byQualified := symbolsByQualifiedName(result.Analysis.Symbols)
	for name, kind := range map[string]SymbolKind{
		"grouped.Number":            SymbolKindType,
		"grouped.Text":              SymbolKindAlias,
		"grouped.Generic":           SymbolKindType,
		"grouped.Constraint":        SymbolKindInterface,
		"grouped.Constraint.String": SymbolKindMethod,
	} {
		if symbol, ok := byQualified[name]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v, %v; want %s", name, symbol, ok, kind)
		}
	}
	if strings.Contains(strings.Join(sortedSymbolQualifiedNames(result.Analysis.Symbols), "\n"), "~int") {
		t.Fatal("type constraint term became a symbol")
	}
	if signature := byQualified["grouped.Generic"].Signature; signature != "Generic[T any] []T" {
		t.Fatalf("grouped generic signature = %q", signature)
	}
}

func TestGoAnalyzerRecoverableParseErrorsKeepPartialSymbolsAndBoundDiagnostics(t *testing.T) {
	text := `package broken

type Box struct { Value int }
func Good() int { return 1 }
func Broken(
`
	options := goAnalyzerTestOptions
	options.Limits.MaxDiagnostics = 2
	result := analyzeGoText(t, text, options)
	if result.Analysis.CoverageComplete {
		t.Fatalf("malformed Go source reported complete: %+v", result.Analysis)
	}
	if len(result.Analysis.Diagnostics) == 0 || len(result.Analysis.Diagnostics) > 2 {
		t.Fatalf("parse diagnostics = %+v", result.Analysis.Diagnostics)
	}
	byQualified := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"broken", "broken.Box", "broken.Good"} {
		if _, ok := byQualified[name]; !ok {
			t.Fatalf("recoverable parse lost %s: %v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestGoAnalyzerSymbolAndDependencyLimitsReturnUsefulPartialCoverage(t *testing.T) {
	var source strings.Builder
	source.WriteString("package generated\nimport (\n")
	for index := 0; index < 40; index++ {
		fmt.Fprintf(&source, "_ \"example.com/dependency/%d\"\n", index)
	}
	source.WriteString(")\n")
	for index := 0; index < 200; index++ {
		fmt.Fprintf(&source, "func F%03d() {}\n", index)
	}
	options := goAnalyzerTestOptions
	options.Limits.MaxSymbols = 32
	options.Limits.MaxDiagnostics = 8
	result := analyzeGoText(t, source.String(), options)
	if !result.Analysis.Truncated || result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 32 {
		t.Fatalf("bounded symbol result = %+v", result.Analysis)
	}
	if len(result.Dependencies) != 32 {
		t.Fatalf("bounded dependencies = %d, want 32", len(result.Dependencies))
	}
	if !hasAnalysisDiagnostic(result.Analysis.Diagnostics, "symbol-limit") || !hasAnalysisDiagnostic(result.Analysis.Diagnostics, "dependency-limit") {
		t.Fatalf("limit diagnostics = %+v", result.Analysis.Diagnostics)
	}
}

func TestGoAnalyzerSignatureExclusionAndCancellation(t *testing.T) {
	text := "package sample\nfunc Work[T any](value T) T { return value }\n"
	options := goAnalyzerTestOptions
	options.IncludeSignatures = false
	result := analyzeGoText(t, text, options)
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Signature != "" {
			t.Fatalf("signature exclusion leaked %q on %s", symbol.Signature, symbol.QualifiedName)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (GoAnalyzer{}).Analyze(ctx, sourceDocumentForScanner(text), goAnalyzerTestOptions)
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancel error=%v kind=%v", err, operation.KindOf(err))
	}
}

func analyzeGoText(t *testing.T, text string, options AnalyzeOptions) AnalyzerResult {
	t.Helper()
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.go"
	result, err := (GoAnalyzer{}).Analyze(context.Background(), document, options)
	if err != nil {
		t.Fatalf("Go Analyze: %v", err)
	}
	return result
}

func symbolsByQualifiedName(symbols []NormalizedSymbol) map[string]NormalizedSymbol {
	result := make(map[string]NormalizedSymbol, len(symbols))
	for _, symbol := range symbols {
		result[symbol.QualifiedName] = symbol
	}
	return result
}

func sortedSymbolQualifiedNames(symbols []NormalizedSymbol) []string {
	result := make([]string, len(symbols))
	for index, symbol := range symbols {
		result[index] = symbol.QualifiedName
	}
	return sortedStrings(result)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasAnalysisDiagnostic(diagnostics []AnalysisDiagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
