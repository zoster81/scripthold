package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

type phase9Scope struct {
	label  string
	parent SymbolParent
}

func newPhase9Builder(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID) (*SymbolBuilder, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return nil, operation.New(operation.KindInvalidInput, "source document is required")
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: language, Analyzer: string(analyzer), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return nil, err
	}
	return builder, nil
}

func phase9ScanLogicalLines(ctx context.Context, document *SourceDocument, profile ScannerProfile, maxNesting int) (ScanResult, []LogicalLine, error) {
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

func phase9ApplyScanDiagnostics(builder *SymbolBuilder, scan ScanResult, language string) {
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

func phase9AddDependency(document *SourceDocument, dependencies *[]StructuralDependency, kind StructuralDependencyKind, value string, start, end int) {
	value = strings.TrimSpace(value)
	if value == "" || start < 0 || end <= start || end > len(document.Text) {
		return
	}
	rangeValue, err := document.RangeFromUTF8Offsets(start, end)
	if err != nil {
		return
	}
	*dependencies = appendUniqueDependencies(*dependencies, []StructuralDependency{{
		Kind: kind, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural,
	}})
}

func phase9AddSymbol(builder *SymbolBuilder, spec SymbolSpec) (NormalizedSymbol, bool) {
	symbol, err := builder.Add(spec)
	if err == nil {
		return symbol, true
	}
	if operation.KindOf(err) != operation.KindLimit {
		builder.MarkIncomplete()
	}
	return NormalizedSymbol{}, false
}

func phase9LineEndToken(tokens []Token) int {
	end := len(tokens)
	for end > 0 && (tokens[end-1].Kind == TokenEOF || tokens[end-1].Kind == TokenNewline) {
		end--
	}
	return end
}

func phase9TokenIndexFold(tokens []Token, value string, start int) int {
	for index := max(start, 0); index < len(tokens); index++ {
		if strings.EqualFold(tokens[index].Text, value) {
			return index
		}
	}
	return -1
}

func phase9FirstIdentifier(tokens []Token, start int) int {
	for index := max(start, 0); index < len(tokens); index++ {
		if tokens[index].Kind == TokenIdentifier {
			return index
		}
	}
	return -1
}

func phase9ParentFromScopes(scopes []phase9Scope) *SymbolParent {
	for index := len(scopes) - 1; index >= 0; index-- {
		if scopes[index].parent.ID != "" {
			value := scopes[index].parent
			return &value
		}
	}
	return nil
}

func phase9MarkUnclosedScopes(builder *SymbolBuilder, language string, scopes []phase9Scope) {
	if len(scopes) == 0 {
		return
	}
	builder.MarkIncomplete()
	_ = builder.AddDiagnostic(DiagnosticSpec{
		Code: language + "-unclosed-scope", Message: language + " source ends with an unclosed structural scope",
		Severity: DiagnosticWarning, AffectsCoverage: true,
	})
}

func phase9QualifiedTail(value string) string {
	for _, separator := range []string{"::", "."} {
		if index := strings.LastIndex(value, separator); index >= 0 {
			return value[index+len(separator):]
		}
	}
	return value
}

func phase9PopScope(scopes []phase9Scope, label string, caseInsensitive bool) []phase9Scope {
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

func phase9CleanAtom(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "'")
	value = strings.TrimPrefix(value, ":")
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimSpace(value)
	return value
}

func phase9TokenName(token Token) (string, OffsetRange) {
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

func phase9StringTokenValue(token Token) string {
	value := strings.TrimSpace(token.Text)
	if len(value) < 2 {
		return ""
	}
	if (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0] {
		return value[1 : len(value)-1]
	}
	return ""
}
