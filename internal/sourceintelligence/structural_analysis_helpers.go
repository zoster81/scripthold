package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

type structuralAnalyzerScope struct {
	label  string
	parent SymbolParent
}

func scanAnalyzerLogicalLines(ctx context.Context, document *SourceDocument, profile ScannerProfile, maxNesting int) (ScanResult, []LogicalLine, error) {
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, profile, ScannerLimits{
		MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting,
	})
	if err != nil {
		return ScanResult{}, nil, err
	}
	return scan, BuildLogicalLines(scan.Tokens, LogicalLineProfile{}), nil
}

func applyStructuralScanDiagnostics(builder *SymbolBuilder, scan ScanResult, language string) {
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{
			Code: language + "-" + diagnostic.Code, Message: diagnostic.Message,
			Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true,
		})
	}
	if !scan.Complete || scan.DiagnosticsTruncated {
		builder.MarkIncomplete()
	}
}

func addStructuralSymbol(builder *SymbolBuilder, spec SymbolSpec) (NormalizedSymbol, bool) {
	symbol, err := builder.Add(spec)
	if err == nil {
		return symbol, true
	}
	if operation.KindOf(err) != operation.KindLimit {
		builder.MarkIncomplete()
	}
	return NormalizedSymbol{}, false
}

func logicalLineTokenEnd(tokens []Token) int {
	end := len(tokens)
	for end > 0 && (tokens[end-1].Kind == TokenEOF || tokens[end-1].Kind == TokenNewline) {
		end--
	}
	return end
}

func tokenIndexEqualFold(tokens []Token, value string, start int) int {
	for index := max(start, 0); index < len(tokens); index++ {
		if strings.EqualFold(tokens[index].Text, value) {
			return index
		}
	}
	return -1
}

func firstIdentifierToken(tokens []Token, start int) int {
	for index := max(start, 0); index < len(tokens); index++ {
		if tokens[index].Kind == TokenIdentifier {
			return index
		}
	}
	return -1
}

func parentFromStructuralScopes(scopes []structuralAnalyzerScope) *SymbolParent {
	for index := len(scopes) - 1; index >= 0; index-- {
		if scopes[index].parent.ID != "" {
			value := scopes[index].parent
			return &value
		}
	}
	return nil
}

func markUnclosedStructuralScopes(builder *SymbolBuilder, language string, scopes []structuralAnalyzerScope) {
	if len(scopes) == 0 {
		return
	}
	builder.MarkIncomplete()
	_ = builder.AddDiagnostic(DiagnosticSpec{
		Code: language + "-unclosed-scope", Message: language + " source ends with an unclosed structural scope",
		Severity: DiagnosticWarning, AffectsCoverage: true,
	})
}

func qualifiedNameTail(value string) string {
	for _, separator := range []string{"::", "."} {
		if index := strings.LastIndex(value, separator); index >= 0 {
			return value[index+len(separator):]
		}
	}
	return value
}

func popStructuralScope(scopes []structuralAnalyzerScope, label string, caseInsensitive bool) []structuralAnalyzerScope {
	if len(scopes) == 0 {
		return scopes
	}
	if label == "" {
		return scopes[:len(scopes)-1]
	}
	equal := func(left, right string) bool {
		if caseInsensitive {
			return strings.EqualFold(left, right)
		}
		return left == right
	}
	if equal(scopes[len(scopes)-1].label, label) {
		return scopes[:len(scopes)-1]
	}
	return scopes
}

func cleanLispAtom(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "'")
	value = strings.TrimPrefix(value, ":")
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimSpace(value)
	return value
}

func lispTokenName(token Token) (string, OffsetRange) {
	name := strings.TrimSpace(token.Text)
	start := token.StartOffset
	for len(name) > 0 && (name[0] == ':' || name[0] == '\'') {
		name = name[1:]
		start++
	}
	for len(name) > 0 && name[len(name)-1] == '.' {
		name = name[:len(name)-1]
	}
	return name, OffsetRange{Start: start, End: start + len(name)}
}

func quotedTokenValue(token Token) string {
	value := strings.TrimSpace(token.Text)
	if len(value) < 2 {
		return ""
	}
	if (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0] {
		return value[1 : len(value)-1]
	}
	return ""
}
