package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// PythonAnalyzer performs indentation-defined declaration analysis over the shared scanner.
type PythonAnalyzer struct{}

func (PythonAnalyzer) ID() AnalyzerID   { return AnalyzerPython }
func (PythonAnalyzer) Language() string { return "python" }

func (PythonAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_python_source", document.Path, err)
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: "python", Analyzer: string(AnalyzerPython), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxTokens := scannerTokenBudget(document.Text)
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, PythonScannerProfile(), ScannerLimits{MaxTokens: maxTokens, MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		rangeValue := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: "python-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &rangeValue, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	lines := buildPythonLogicalLines(scan.Tokens)
	parser := &pythonParser{ctx: ctx, document: document, lines: lines, builder: builder}
	parser.parseRange(0, len(lines), nil, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_python_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies}, nil
}

type pythonLogicalLine struct {
	indent      int
	tokens      []Token
	startOffset int
	endOffset   int
}

type pythonParser struct {
	ctx          context.Context
	document     *SourceDocument
	lines        []pythonLogicalLine
	builder      *SymbolBuilder
	dependencies []StructuralDependency
	stopped      bool
}

func buildPythonLogicalLines(tokens []Token) []pythonLogicalLine {
	lines := BuildLogicalLines(tokens, LogicalLineProfile{TrackIndentation: true, SkipDirectives: true})
	result := make([]pythonLogicalLine, 0, len(lines))
	for _, line := range lines {
		result = append(result, pythonLogicalLine{
			indent: line.Indent, tokens: line.Tokens, startOffset: line.StartOffset, endOffset: line.EndOffset,
		})
	}
	return result
}

func (parser *pythonParser) parseRange(start, end int, parent *SymbolParent, parentKind string) {
	for index := start; index < end && !parser.stopped; {
		if parser.ctx.Err() != nil {
			return
		}
		line := parser.lines[index]
		if len(line.tokens) == 0 {
			index++
			continue
		}
		if pythonTokenEqual(line.tokens[0], "import") || pythonTokenEqual(line.tokens[0], "from") {
			parser.parseImport(line)
			index++
			continue
		}

		decoratorStart := -1
		declarationIndex := index
		for declarationIndex < end && pythonIsDecorator(parser.lines[declarationIndex]) {
			if decoratorStart < 0 {
				decoratorStart = parser.lines[declarationIndex].startOffset
			}
			if declarationIndex+1 >= end || parser.lines[declarationIndex+1].indent != line.indent {
				break
			}
			declarationIndex++
		}
		if declarationIndex != index || pythonIsDecorator(line) {
			if declarationIndex >= end || pythonIsDecorator(parser.lines[declarationIndex]) {
				index = declarationIndex + 1
				continue
			}
			line = parser.lines[declarationIndex]
		}

		kind, nativeKind, nameIndex, async := pythonDeclaration(line, parentKind)
		if kind == "" {
			index++
			continue
		}
		scopeEnd := parser.scopeEnd(declarationIndex, end)
		declarationStart := line.startOffset
		modifiers := []string(nil)
		if decoratorStart >= 0 {
			declarationStart = decoratorStart
			modifiers = append(modifiers, "decorated")
		}
		if async {
			modifiers = append(modifiers, "async")
		}
		declarationEnd := line.endOffset
		var body *OffsetRange
		if scopeEnd > declarationIndex+1 {
			declarationEnd = parser.lines[scopeEnd-1].endOffset
			value := OffsetRange{Start: parser.lines[declarationIndex+1].startOffset, End: declarationEnd}
			body = &value
		} else if pythonLineEndsWithColon(line) {
			parser.builder.MarkIncomplete()
			_ = parser.builder.AddDiagnostic(DiagnosticSpec{Code: "python-empty-scope", Message: "declaration has no indented body", Severity: DiagnosticWarning, Range: &OffsetRange{Start: line.startOffset, End: line.endOffset}, AffectsCoverage: true})
		}
		nameToken := line.tokens[nameIndex]
		symbol, err := parser.builder.Add(SymbolSpec{
			Kind: kind, NativeKind: nativeKind, Name: nameToken.Text, Parent: parent,
			Declaration: OffsetRange{Start: declarationStart, End: declarationEnd},
			NameRange:   OffsetRange{Start: nameToken.StartOffset, End: nameToken.EndOffset},
			Signature:   &OffsetRange{Start: line.startOffset, End: line.endOffset}, Body: body,
			Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
		})
		if operation.KindOf(err) == operation.KindLimit {
			parser.stopped = true
			return
		}
		if err != nil {
			parser.builder.MarkIncomplete()
			index = scopeEnd
			continue
		}
		if scopeEnd > declarationIndex+1 {
			child := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
			parser.parseRange(declarationIndex+1, scopeEnd, child, nativeKind)
		}
		index = scopeEnd
	}
}

func (parser *pythonParser) scopeEnd(index, end int) int {
	indent := parser.lines[index].indent
	for cursor := index + 1; cursor < end; cursor++ {
		if parser.lines[cursor].indent <= indent {
			return cursor
		}
	}
	return end
}

func pythonDeclaration(line pythonLogicalLine, parentKind string) (SymbolKind, string, int, bool) {
	if len(line.tokens) < 2 {
		return "", "", -1, false
	}
	cursor := 0
	async := false
	if pythonTokenEqual(line.tokens[cursor], "async") {
		async = true
		cursor++
	}
	if cursor >= len(line.tokens) {
		return "", "", -1, false
	}
	switch {
	case pythonTokenEqual(line.tokens[cursor], "class"):
		name := pythonNextIdentifier(line.tokens, cursor+1)
		if name < 0 {
			return "", "", -1, false
		}
		return SymbolKindClass, "class", name, async
	case pythonTokenEqual(line.tokens[cursor], "def"):
		name := pythonNextIdentifier(line.tokens, cursor+1)
		if name < 0 {
			return "", "", -1, false
		}
		if parentKind == "class" {
			return SymbolKindMethod, "method", name, async
		}
		return SymbolKindFunction, "function", name, async
	default:
		return "", "", -1, false
	}
}

func (parser *pythonParser) parseImport(line pythonLogicalLine) {
	if len(line.tokens) < 2 {
		return
	}
	start := 1
	if pythonTokenEqual(line.tokens[0], "from") {
		for index := 1; index < len(line.tokens); index++ {
			if pythonTokenEqual(line.tokens[index], "import") {
				start = 1
				line.tokens = line.tokens[:index]
				break
			}
		}
	}
	value := pythonModuleName(line.tokens[start:])
	if value == "" {
		return
	}
	rangeValue, err := parser.document.RangeFromUTF8Offsets(line.startOffset, line.endOffset)
	if err == nil {
		parser.dependencies = append(parser.dependencies, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural})
	}
}

func pythonModuleName(tokens []Token) string {
	var parts []string
	for _, token := range tokens {
		if token.Kind == TokenIdentifier || token.Text == "." {
			parts = append(parts, token.Text)
			continue
		}
		if token.Text == "," || strings.EqualFold(token.Text, "as") {
			break
		}
	}
	return strings.Join(parts, "")
}

func pythonIsDecorator(line pythonLogicalLine) bool {
	return len(line.tokens) > 0 && line.tokens[0].Text == "@"
}

func pythonLineEndsWithColon(line pythonLogicalLine) bool {
	return len(line.tokens) > 0 && line.tokens[len(line.tokens)-1].Text == ":"
}

func pythonNextIdentifier(tokens []Token, start int) int {
	for index := start; index < len(tokens); index++ {
		if tokens[index].Kind == TokenIdentifier {
			return index
		}
	}
	return -1
}

func pythonTokenEqual(token Token, text string) bool {
	return token.Text == text
}
