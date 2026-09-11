package sourceintelligence

import "sort"

func testSourceDocument(path, text string) *SourceDocument {
	return &SourceDocument{Path: path, Text: text, Encoding: "utf-8", lineStarts: buildLineStarts(text)}
}

func sourceDocumentForScanner(text string) *SourceDocument {
	return testSourceDocument("scanner.fixture", text)
}

func testAnalyzeOptions(signatures bool, maxSymbols int) AnalyzeOptions {
	return AnalyzeOptions{IncludeSignatures: signatures, MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: maxSymbols, MaxSignatureBytes: 8192, MaxDiagnostics: 64}}
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

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func containsSortedString(values []string, target string) bool {
	index := sort.SearchStrings(values, target)
	return index < len(values) && values[index] == target
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]int, len(got))
	for _, value := range got {
		seen[value]++
	}
	for _, value := range want {
		if seen[value] == 0 {
			return false
		}
		seen[value]--
	}
	return true
}

func hasAnalysisDiagnostic(diagnostics []AnalysisDiagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func dependencyValues(values []StructuralDependency) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = value.Value
	}
	return result
}

func hasStructuralRelation(values []StructuralRelation, kind, source, target string) bool {
	for _, value := range values {
		if value.Kind == kind && value.Source == source && value.Target == target && value.Evidence == SymbolEvidenceStructural {
			return true
		}
	}
	return false
}
