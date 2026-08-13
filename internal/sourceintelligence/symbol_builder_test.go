package sourceintelligence

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestSymbolBuilderNormalizesRangesHierarchySignatureAndModifiers(t *testing.T) {
	text := "class Café<T> {\n    public void Work(int value) { }\n}\n"
	document := sourceDocumentForScanner(text)
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Language:          "csharp",
		Analyzer:          string(AnalyzerCSharp),
		IncludeSignatures: true,
		MaxEvidence:       SymbolEvidenceStructural,
		Limits:            SymbolBuilderLimits{MaxSymbols: 32, MaxSignatureBytes: 1024, MaxDiagnostics: 8},
	})

	classStart := strings.Index(text, "class")
	classNameStart := strings.Index(text, "Café")
	classEnd := strings.LastIndex(text, "}") + 1
	classSymbol, err := builder.Add(SymbolSpec{
		Kind:        SymbolKindClass,
		NativeKind:  "class",
		Name:        "Café",
		Declaration: OffsetRange{Start: classStart, End: classEnd},
		NameRange:   OffsetRange{Start: classNameStart, End: classNameStart + len("Café")},
		Signature:   offsetRangePointer(OffsetRange{Start: classStart, End: strings.Index(text, " {")}),
		Visibility:  VisibilityInternal,
		Modifiers:   []string{"partial", "public", "partial"},
		Evidence:    SymbolEvidenceStructural,
	})
	if err != nil {
		t.Fatal(err)
	}
	if classSymbol.QualifiedName != "Café" || classSymbol.ParentID != "" {
		t.Fatalf("class hierarchy = %+v", classSymbol)
	}
	if classSymbol.Signature != "class Café<T>" {
		t.Fatalf("class signature = %q", classSymbol.Signature)
	}
	if !reflect.DeepEqual(classSymbol.Modifiers, []string{"partial", "public"}) {
		t.Fatalf("class modifiers = %v", classSymbol.Modifiers)
	}
	if classSymbol.DeclarationRange.Start != (Position{Line: 1, Column: 1}) || classSymbol.NameRange.Start != (Position{Line: 1, Column: 7}) {
		t.Fatalf("class ranges = declaration=%+v name=%+v", classSymbol.DeclarationRange, classSymbol.NameRange)
	}
	assertLowerHex64(t, "class id", classSymbol.ID)

	builder.Scopes().Push(ScopeFrame{
		SymbolID:      classSymbol.ID,
		QualifiedName: classSymbol.QualifiedName,
		Kind:          string(classSymbol.Kind),
		Boundary:      ScopeBoundary{Style: ScopeBoundaryBrace, Value: 1},
	})
	methodStart := strings.Index(text, "public void")
	methodNameStart := strings.Index(text, "Work")
	methodEnd := strings.Index(text, "}\n}") + 1
	methodSymbol, err := builder.Add(SymbolSpec{
		Kind:        SymbolKindMethod,
		NativeKind:  "method",
		Name:        "Work",
		Declaration: OffsetRange{Start: methodStart, End: methodEnd},
		NameRange:   OffsetRange{Start: methodNameStart, End: methodNameStart + len("Work")},
		Signature:   offsetRangePointer(OffsetRange{Start: methodStart, End: strings.Index(text, ") {") + 1}),
		Body:        offsetRangePointer(OffsetRange{Start: strings.Index(text, ") {") + 2, End: methodEnd}),
		Visibility:  VisibilityPublic,
		Modifiers:   []string{"public"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if methodSymbol.ParentID != classSymbol.ID || methodSymbol.ParentQualifiedName != "Café" || methodSymbol.QualifiedName != "Café.Work" {
		t.Fatalf("method hierarchy = %+v", methodSymbol)
	}
	if methodSymbol.Evidence != SymbolEvidenceStructural || methodSymbol.SignatureRange == nil || methodSymbol.BodyRange == nil {
		t.Fatalf("method evidence/ranges = %+v", methodSymbol)
	}
	assertLowerHex64(t, "method id", methodSymbol.ID)

	result := builder.Result()
	if !result.CoverageComplete || result.Truncated || len(result.Diagnostics) != 0 || len(result.Symbols) != 2 {
		t.Fatalf("builder result = %+v", result)
	}
}

func TestSymbolBuilderDeterministicIDsDistinguishOverloadsNestedAndDisambiguators(t *testing.T) {
	text := "func Work(a int)\nfunc Work(a string)\n"
	build := func() []NormalizedSymbol {
		builder := NewSymbolBuilder(sourceDocumentForScanner(text), SymbolBuilderOptions{
			Language:    "test",
			Analyzer:    "test-analyzer",
			MaxEvidence: SymbolEvidenceStructural,
			Limits:      SymbolBuilderLimits{MaxSymbols: 16, MaxSignatureBytes: 1024, MaxDiagnostics: 8},
		})
		firstName := strings.Index(text, "Work")
		secondDecl := strings.LastIndex(text, "func")
		secondName := strings.LastIndex(text, "Work")
		for _, spec := range []SymbolSpec{
			{Kind: SymbolKindFunction, NativeKind: "function", Name: "Work", Declaration: OffsetRange{0, strings.Index(text, "\n")}, NameRange: OffsetRange{firstName, firstName + 4}},
			{Kind: SymbolKindFunction, NativeKind: "function", Name: "Work", Declaration: OffsetRange{secondDecl, len(text) - 1}, NameRange: OffsetRange{secondName, secondName + 4}},
			{Kind: SymbolKindFunction, NativeKind: "function", Name: "Work", Declaration: OffsetRange{secondDecl, len(text) - 1}, NameRange: OffsetRange{secondName, secondName + 4}, Disambiguator: "synthetic-second-view"},
		} {
			if _, err := builder.Add(spec); err != nil {
				t.Fatal(err)
			}
		}
		return builder.Result().Symbols
	}
	first := build()
	second := build()
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("builder output is nondeterministic:\nfirst=%+v\nsecond=%+v", first, second)
	}
	seen := map[string]struct{}{}
	for _, symbol := range first {
		if _, duplicate := seen[symbol.ID]; duplicate {
			t.Fatalf("symbol ID collision: %+v", first)
		}
		seen[symbol.ID] = struct{}{}
	}
}

func TestSymbolBuilderParentOverrideSupportsNonLexicalOwnership(t *testing.T) {
	text := "type Box struct{}\nfunc (b Box) Get() int { return 1 }\n"
	builder := NewSymbolBuilder(sourceDocumentForScanner(text), SymbolBuilderOptions{
		Language: "go", Analyzer: string(AnalyzerGo), MaxEvidence: SymbolEvidenceStructural,
		Limits: SymbolBuilderLimits{MaxSymbols: 16, MaxSignatureBytes: 1024, MaxDiagnostics: 8},
	})
	typeName := strings.Index(text, "Box")
	typeSymbol, err := builder.Add(SymbolSpec{
		Kind: SymbolKindStruct, NativeKind: "struct", Name: "Box",
		Declaration: OffsetRange{0, strings.Index(text, "\n")}, NameRange: OffsetRange{typeName, typeName + 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	methodStart := strings.Index(text, "func")
	methodName := strings.Index(text, "Get")
	method, err := builder.Add(SymbolSpec{
		Kind: SymbolKindMethod, NativeKind: "method", Name: "Get",
		Declaration: OffsetRange{methodStart, len(text) - 1}, NameRange: OffsetRange{methodName, methodName + 3},
		Parent: &SymbolParent{ID: typeSymbol.ID, QualifiedName: typeSymbol.QualifiedName},
	})
	if err != nil {
		t.Fatal(err)
	}
	if method.ParentID != typeSymbol.ID || method.QualifiedName != "Box.Get" {
		t.Fatalf("non-lexical parent = %+v", method)
	}
}

func TestScopeStackBraceExplicitEndAndIndentAdapters(t *testing.T) {
	stack := NewScopeStack()
	class := ScopeFrame{SymbolID: "class", QualifiedName: "Outer", Kind: "class", Boundary: ScopeBoundary{Style: ScopeBoundaryBrace, Value: 1}}
	method := ScopeFrame{SymbolID: "method", QualifiedName: "Outer.Work", Kind: "method", Boundary: ScopeBoundary{Style: ScopeBoundaryBrace, Value: 2}}
	stack.Push(class)
	stack.Push(method)
	if current, ok := stack.Current(); !ok || current.SymbolID != "method" {
		t.Fatalf("brace current = %+v, %v", current, ok)
	}
	if popped := stack.CloseBrace(1); !reflect.DeepEqual(scopeIDs(popped), []string{"method"}) {
		t.Fatalf("brace depth 1 popped %v", scopeIDs(popped))
	}
	if popped := stack.CloseBrace(0); !reflect.DeepEqual(scopeIDs(popped), []string{"class"}) {
		t.Fatalf("brace depth 0 popped %v", scopeIDs(popped))
	}

	stack.Push(ScopeFrame{SymbolID: "ns", QualifiedName: "N", Kind: "namespace", Boundary: ScopeBoundary{Style: ScopeBoundaryExplicitEnd, Label: "namespace"}})
	stack.Push(ScopeFrame{SymbolID: "vbclass", QualifiedName: "N.C", Kind: "class", Boundary: ScopeBoundary{Style: ScopeBoundaryExplicitEnd, Label: "class"}})
	if _, ok := stack.CloseExplicit("namespace"); ok {
		t.Fatal("explicit-end adapter closed a non-top matching ancestor")
	}
	if closed, ok := stack.CloseExplicit("CLASS"); !ok || closed.SymbolID != "vbclass" {
		t.Fatalf("CloseExplicit(class) = %+v, %v", closed, ok)
	}
	if closed, ok := stack.CloseExplicit("namespace"); !ok || closed.SymbolID != "ns" {
		t.Fatalf("CloseExplicit(namespace) = %+v, %v", closed, ok)
	}

	stack.Push(ScopeFrame{SymbolID: "pyclass", QualifiedName: "C", Kind: "class", Boundary: ScopeBoundary{Style: ScopeBoundaryIndent, Value: 4}})
	stack.Push(ScopeFrame{SymbolID: "pyfunc", QualifiedName: "C.work", Kind: "function", Boundary: ScopeBoundary{Style: ScopeBoundaryIndent, Value: 8}})
	if popped := stack.Dedent(4); !reflect.DeepEqual(scopeIDs(popped), []string{"pyfunc"}) {
		t.Fatalf("Dedent(4) popped %v", scopeIDs(popped))
	}
	if popped := stack.Dedent(0); !reflect.DeepEqual(scopeIDs(popped), []string{"pyclass"}) {
		t.Fatalf("Dedent(0) popped %v", scopeIDs(popped))
	}
}

func TestSymbolBuilderEnforcesEvidenceAndBoundsWithoutDiscardingPartialResult(t *testing.T) {
	text := "class A {}\nclass B {}\n"
	builder := NewSymbolBuilder(sourceDocumentForScanner(text), SymbolBuilderOptions{
		Language: "csharp", Analyzer: string(AnalyzerCSharp), IncludeSignatures: true, MaxEvidence: SymbolEvidenceStructural,
		Limits: SymbolBuilderLimits{MaxSymbols: 1, MaxSignatureBytes: 5, MaxDiagnostics: 2},
	})
	nameA := strings.Index(text, "A")
	first, err := builder.Add(SymbolSpec{
		Kind: SymbolKindClass, NativeKind: "class", Name: "A",
		Declaration: OffsetRange{0, strings.Index(text, "\n")}, NameRange: OffsetRange{nameA, nameA + 1},
		Signature: offsetRangePointer(OffsetRange{0, strings.Index(text, "\n")}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Signature != "" || first.SignatureRange == nil {
		t.Fatalf("oversized signature should keep range but omit text: %+v", first)
	}
	if builder.Result().CoverageComplete {
		t.Fatal("signature truncation did not lower coverage")
	}

	nameB := strings.LastIndex(text, "B")
	_, err = builder.Add(SymbolSpec{
		Kind: SymbolKindClass, NativeKind: "class", Name: "B",
		Declaration: OffsetRange{strings.LastIndex(text, "class"), len(text) - 1}, NameRange: OffsetRange{nameB, nameB + 1},
	})
	if operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("symbol limit error = %v, kind=%v", err, operation.KindOf(err))
	}
	partial := builder.Result()
	if len(partial.Symbols) != 1 || !partial.Truncated || partial.CoverageComplete {
		t.Fatalf("partial result after symbol limit = %+v", partial)
	}

	semanticBuilder := NewSymbolBuilder(sourceDocumentForScanner(text), SymbolBuilderOptions{
		Language: "csharp", Analyzer: string(AnalyzerCSharp), MaxEvidence: SymbolEvidenceStructural,
		Limits: SymbolBuilderLimits{MaxSymbols: 4, MaxSignatureBytes: 64, MaxDiagnostics: 2},
	})
	_, err = semanticBuilder.Add(SymbolSpec{
		Kind: SymbolKindClass, NativeKind: "class", Name: "A", Evidence: SymbolEvidenceSemantic,
		Declaration: OffsetRange{0, strings.Index(text, "\n")}, NameRange: OffsetRange{nameA, nameA + 1},
	})
	if operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("semantic evidence error = %v, kind=%v", err, operation.KindOf(err))
	}
}

func TestSymbolBuilderRejectsInvalidWorkWithoutPartialSideEffects(t *testing.T) {
	text := "class A {}\n"
	name := strings.Index(text, "A")
	builder := NewSymbolBuilder(sourceDocumentForScanner(text), SymbolBuilderOptions{
		Language: "csharp", Analyzer: string(AnalyzerCSharp), IncludeSignatures: true, MaxEvidence: SymbolEvidenceStructural,
		Limits: SymbolBuilderLimits{MaxSymbols: 4, MaxSignatureBytes: 5, MaxDiagnostics: 2},
	})
	spec := SymbolSpec{
		Kind: SymbolKindClass, NativeKind: "class", Name: "A",
		Declaration: OffsetRange{0, len(text) - 1}, NameRange: OffsetRange{name, name + 1},
		Signature: offsetRangePointer(OffsetRange{0, len(text) - 1}),
	}
	if _, err := builder.Add(spec); err != nil {
		t.Fatal(err)
	}
	before := builder.Result()
	if len(before.Diagnostics) != 1 || !before.Truncated || before.CoverageComplete {
		t.Fatalf("unexpected baseline after bounded signature: %+v", before)
	}
	if _, err := builder.Add(spec); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("duplicate symbol error=%v kind=%v", err, operation.KindOf(err))
	}
	after := builder.Result()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected duplicate mutated result:\nbefore=%+v\nafter=%+v", before, after)
	}

	clean := NewSymbolBuilder(sourceDocumentForScanner(text), SymbolBuilderOptions{
		Language: "csharp", Analyzer: string(AnalyzerCSharp), MaxEvidence: SymbolEvidenceStructural,
		Limits: SymbolBuilderLimits{MaxSymbols: 4, MaxSignatureBytes: 64, MaxDiagnostics: 1},
	})
	if err := clean.AddDiagnostic(DiagnosticSpec{Code: "bad", Message: "bad", Severity: DiagnosticSeverity("invalid"), AffectsCoverage: true}); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("invalid diagnostic error=%v kind=%v", err, operation.KindOf(err))
	}
	if result := clean.Result(); !result.CoverageComplete || len(result.Diagnostics) != 0 || result.DiagnosticsTruncated {
		t.Fatalf("invalid diagnostic mutated clean result: %+v", result)
	}
	if err := clean.AddDiagnostic(DiagnosticSpec{Code: "valid", Message: "valid", Severity: DiagnosticWarning}); err != nil {
		t.Fatal(err)
	}
	if err := clean.AddDiagnostic(DiagnosticSpec{Code: "bad", Message: "bad", Severity: DiagnosticSeverity("invalid")}); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("invalid diagnostic beyond cap error=%v kind=%v", err, operation.KindOf(err))
	}
	if result := clean.Result(); len(result.Diagnostics) != 1 || result.DiagnosticsTruncated {
		t.Fatalf("invalid diagnostic beyond cap was treated as truncation: %+v", result)
	}
}

func TestSymbolBuilderDiagnosticsAreBoundedAndCoverageAware(t *testing.T) {
	builder := NewSymbolBuilder(sourceDocumentForScanner("abc\n"), SymbolBuilderOptions{
		Language: "test", Analyzer: "test", MaxEvidence: SymbolEvidenceStructural,
		Limits: SymbolBuilderLimits{MaxSymbols: 4, MaxSignatureBytes: 64, MaxDiagnostics: 2},
	})
	for index := 0; index < 5; index++ {
		builder.AddDiagnostic(DiagnosticSpec{
			Code: "parse", Message: "diagnostic", Severity: DiagnosticWarning,
			Range: offsetRangePointer(OffsetRange{0, 1}), AffectsCoverage: true,
		})
	}
	result := builder.Result()
	if len(result.Diagnostics) != 2 || !result.DiagnosticsTruncated || result.CoverageComplete {
		t.Fatalf("bounded diagnostics result = %+v", result)
	}
}

func TestSymbolBuilderRejectsInvalidRangesKindsAndOptions(t *testing.T) {
	document := sourceDocumentForScanner("abc\n")
	invalidOptions := []SymbolBuilderOptions{
		{},
		{Language: "test", Analyzer: "a", Limits: SymbolBuilderLimits{MaxSymbols: 1, MaxSignatureBytes: 1, MaxDiagnostics: 1}},
		{Language: "test", Analyzer: "a", MaxEvidence: SymbolEvidenceStructural, Limits: SymbolBuilderLimits{}},
	}
	for _, options := range invalidOptions {
		builder := NewSymbolBuilder(document, options)
		_, err := builder.Add(SymbolSpec{Kind: SymbolKindFunction, Name: "x", Declaration: OffsetRange{0, 1}, NameRange: OffsetRange{0, 1}})
		if operation.KindOf(err) != operation.KindInvalidInput {
			t.Fatalf("invalid options %+v error=%v kind=%v", options, err, operation.KindOf(err))
		}
	}

	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Language: "test", Analyzer: "a", MaxEvidence: SymbolEvidenceStructural,
		Limits: SymbolBuilderLimits{MaxSymbols: 4, MaxSignatureBytes: 64, MaxDiagnostics: 2},
	})
	for _, spec := range []SymbolSpec{
		{Kind: SymbolKind("not-normalized"), Name: "x", Declaration: OffsetRange{0, 1}, NameRange: OffsetRange{0, 1}},
		{Kind: SymbolKindFunction, Name: "x", Declaration: OffsetRange{2, 1}, NameRange: OffsetRange{0, 1}},
		{Kind: SymbolKindFunction, Name: "x", Declaration: OffsetRange{0, 1}, NameRange: OffsetRange{2, 3}},
		{Kind: SymbolKindFunction, Name: "", Declaration: OffsetRange{0, 1}, NameRange: OffsetRange{0, 1}},
	} {
		if _, err := builder.Add(spec); operation.KindOf(err) != operation.KindInvalidInput {
			t.Fatalf("invalid spec %+v error=%v kind=%v", spec, err, operation.KindOf(err))
		}
	}
}

func TestSymbolBuilderResultOrderingIsBySourceThenID(t *testing.T) {
	text := "aaa bbb ccc"
	builder := NewSymbolBuilder(sourceDocumentForScanner(text), SymbolBuilderOptions{
		Language: "test", Analyzer: "a", MaxEvidence: SymbolEvidenceStructural,
		Limits: SymbolBuilderLimits{MaxSymbols: 8, MaxSignatureBytes: 64, MaxDiagnostics: 2},
	})
	for _, spec := range []SymbolSpec{
		{Kind: SymbolKindVariable, Name: "ccc", Declaration: OffsetRange{8, 11}, NameRange: OffsetRange{8, 11}},
		{Kind: SymbolKindVariable, Name: "aaa", Declaration: OffsetRange{0, 3}, NameRange: OffsetRange{0, 3}},
		{Kind: SymbolKindVariable, Name: "bbb", Declaration: OffsetRange{4, 7}, NameRange: OffsetRange{4, 7}},
	} {
		if _, err := builder.Add(spec); err != nil {
			t.Fatal(err)
		}
	}
	result := builder.Result()
	names := make([]string, len(result.Symbols))
	for index, symbol := range result.Symbols {
		names[index] = symbol.Name
	}
	if !reflect.DeepEqual(names, []string{"aaa", "bbb", "ccc"}) {
		t.Fatalf("source ordering = %v", names)
	}
}

func TestSymbolBuilderContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	builder := NewSymbolBuilder(sourceDocumentForScanner("abc"), SymbolBuilderOptions{
		Context: ctx, Language: "test", Analyzer: "a", MaxEvidence: SymbolEvidenceStructural,
		Limits: SymbolBuilderLimits{MaxSymbols: 4, MaxSignatureBytes: 64, MaxDiagnostics: 2},
	})
	_, err := builder.Add(SymbolSpec{Kind: SymbolKindVariable, Name: "abc", Declaration: OffsetRange{0, 3}, NameRange: OffsetRange{0, 3}})
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancelled builder error=%v kind=%v", err, operation.KindOf(err))
	}
}

func offsetRangePointer(value OffsetRange) *OffsetRange { return &value }

func scopeIDs(frames []ScopeFrame) []string {
	result := make([]string, len(frames))
	for index, frame := range frames {
		result[index] = frame.SymbolID
	}
	return result
}

func assertLowerHex64(t *testing.T, label, value string) {
	t.Helper()
	if len(value) != 64 {
		t.Fatalf("%s length=%d value=%q", label, len(value), value)
	}
	for _, current := range value {
		if !strings.ContainsRune("0123456789abcdef", current) {
			t.Fatalf("%s = %q, want lower hex", label, value)
		}
	}
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
