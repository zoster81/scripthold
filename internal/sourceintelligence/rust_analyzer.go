package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// RustAnalyzer performs bounded declaration-level Rust analysis without macro expansion,
// cfg evaluation, cargo metadata, compiler execution, or trait/type resolution.
type RustAnalyzer struct{}

func (RustAnalyzer) ID() AnalyzerID   { return AnalyzerRust }
func (RustAnalyzer) Language() string { return "rust" }
func (RustAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_rust_source", document.Path, err)
	}
	masked, rawDiagnostics, err := maskRustRawStrings(ctx, document.Text)
	if err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_rust_source", document.Path, err)
	}
	scanDocument := document
	if masked != document.Text {
		clone := *document
		clone.Text = masked
		clone.lineStarts = buildLineStarts(masked)
		scanDocument = &clone
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: "rust", Analyzer: string(AnalyzerRust), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, scanDocument, RustScannerProfile(), ScannerLimits{
		MaxTokens: scannerTokenBudget(scanDocument.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting,
	})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range append(rawDiagnostics, scan.Diagnostics...) {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{
			Code: "rust-" + diagnostic.Code, Message: diagnostic.Message,
			Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true,
		})
	}
	if !scan.Complete || len(rawDiagnostics) > 0 {
		builder.MarkIncomplete()
	}
	parser := &rustParser{
		ctx: ctx, document: document, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil), builder: builder,
	}
	parser.parseScope(0, len(scan.Tokens), nil, rustScopeNormal)
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_rust_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type rustScopeMode uint8

const (
	rustScopeNormal rustScopeMode = iota
	rustScopeStruct
	rustScopeTrait
	rustScopeImpl
)

type rustParser struct {
	ctx          context.Context
	document     *SourceDocument
	tokens       []Token
	pairs        map[int]int
	builder      *SymbolBuilder
	dependencies []StructuralDependency
	relations    []StructuralRelation
	stopped      bool
}

func (parser *rustParser) parseScope(start, end int, parent *SymbolParent, mode rustScopeMode) {
	for index := start; index < end && !parser.stopped; {
		if parser.ctx.Err() != nil {
			return
		}
		index = parser.skipTrivia(index, end)
		if index >= end || parser.tokens[index].Kind == TokenEOF {
			return
		}
		if next, ok := parser.skipMacro(index, end); ok {
			index = next
			continue
		}
		declarationStart := index
		index = parser.skipAttributes(index, end)
		index = parser.skipTrivia(index, end)
		semantic := parser.skipVisibilityAndQualifiers(index, end)
		if semantic >= end {
			return
		}
		if mode == rustScopeStruct {
			if next, ok := parser.parseStructField(declarationStart, semantic, end, parent); ok {
				index = next
				continue
			}
		}
		switch parser.tokens[semantic].Text {
		case "mod":
			index = parser.parseModule(declarationStart, semantic, end, parent)
		case "use":
			index = parser.parseUse(semantic, end)
		case "struct":
			index = parser.parseStruct(declarationStart, semantic, end, parent)
		case "enum":
			index = parser.parseEnum(declarationStart, semantic, end, parent)
		case "trait":
			index = parser.parseTrait(declarationStart, semantic, end, parent)
		case "impl":
			index = parser.parseImpl(declarationStart, semantic, end, parent)
		case "fn":
			index = parser.parseFunction(declarationStart, semantic, end, parent, mode != rustScopeNormal)
		case "const", "static":
			index = parser.parseValue(declarationStart, semantic, end, parent)
		case "type":
			index = parser.parseTypeAlias(declarationStart, semantic, end, parent)
		default:
			index = parser.skipUnknown(index, end)
		}
	}
}

func (parser *rustParser) token(index int, text string) bool {
	return index >= 0 && index < len(parser.tokens) && parser.tokens[index].Text == text
}

func (parser *rustParser) skipTrivia(index, end int) int {
	for index < end {
		switch parser.tokens[index].Kind {
		case TokenNewline, TokenDirective:
			index++
		default:
			return index
		}
	}
	return end
}

func (parser *rustParser) skipAttributes(index, end int) int {
	for index < end && parser.token(index, "#") {
		cursor := index + 1
		if parser.token(cursor, "!") {
			cursor++
		}
		if !parser.token(cursor, "[") {
			return index
		}
		close := parser.pairs[cursor]
		if close <= cursor || close >= end {
			parser.builder.MarkIncomplete()
			return index
		}
		index = parser.skipTrivia(close+1, end)
	}
	return index
}

func (parser *rustParser) skipVisibilityAndQualifiers(index, end int) int {
	for index < end {
		switch parser.tokens[index].Text {
		case "pub":
			index++
			if parser.token(index, "(") {
				if close := parser.pairs[index]; close > index && close < end {
					index = close + 1
				}
			}
		case "async", "const", "unsafe", "extern":
			// const is a qualifier only when followed later by fn; parseValue handles
			// ordinary const declarations before this helper is called via lookahead.
			if parser.tokens[index].Text == "const" && !parser.hasFollowingFn(index+1, end) {
				return index
			}
			index++
			if parser.tokens[index-1].Text == "extern" && index < end && parser.tokens[index].Kind == TokenString {
				index++
			}
		default:
			return index
		}
	}
	return end
}

func (parser *rustParser) hasFollowingFn(start, end int) bool {
	for index := start; index < end && index < start+6; index++ {
		if parser.token(index, "fn") {
			return true
		}
		if parser.tokens[index].Text == ";" || parser.tokens[index].Kind == TokenNewline {
			return false
		}
	}
	return false
}

func (parser *rustParser) skipMacro(start, end int) (int, bool) {
	cursor := start
	if parser.token(cursor, "macro_rules") {
		cursor++
		if !parser.token(cursor, "!") {
			return start, false
		}
		cursor++
		if cursor < end && parser.tokens[cursor].Kind == TokenIdentifier {
			cursor++
		}
	} else {
		if parser.tokens[cursor].Kind != TokenIdentifier && parser.tokens[cursor].Kind != TokenKeyword {
			return start, false
		}
		cursor++
		if !parser.token(cursor, "!") {
			return start, false
		}
		cursor++
	}
	cursor = parser.skipTrivia(cursor, end)
	if cursor >= end || (parser.tokens[cursor].Text != "(" && parser.tokens[cursor].Text != "[" && parser.tokens[cursor].Text != "{") {
		return start, false
	}
	close := parser.pairs[cursor]
	if close <= cursor || close >= end {
		parser.builder.MarkIncomplete()
		return end, true
	}
	next := close + 1
	if parser.token(next, ";") {
		next++
	}
	return next, true
}

func (parser *rustParser) parseModule(declarationStart, keyword, end int, parent *SymbolParent) int {
	nameIndex := parser.nextIdentifier(keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	base := parser.tokens[keyword].Nesting
	for index := nameIndex + 1; index < end; index++ {
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == base+1 {
			close := parser.pairs[index]
			if close <= index {
				parser.builder.MarkIncomplete()
				return end
			}
			symbol, added := parser.add(SymbolSpec{
				Kind: SymbolKindModule, NativeKind: "inline-module", Name: parser.tokens[nameIndex].Text, Parent: parent,
				Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[close].EndOffset},
				NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
				Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[index].StartOffset},
				Body:        &OffsetRange{Start: parser.tokens[index].StartOffset, End: parser.tokens[close].EndOffset},
				Visibility:  parser.visibility(declarationStart, keyword), Evidence: SymbolEvidenceStructural,
			})
			if added {
				moduleParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
				parser.parseScope(index+1, close, moduleParent, rustScopeNormal)
			}
			return close + 1
		}
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == base {
			parser.add(SymbolSpec{
				Kind: SymbolKindModule, NativeKind: "external-module", Name: parser.tokens[nameIndex].Text, Parent: parent,
				Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[index].EndOffset},
				NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
				Visibility:  parser.visibility(declarationStart, keyword), Evidence: SymbolEvidenceStructural,
			})
			rangeValue, err := parser.document.RangeFromUTF8Offsets(parser.tokens[nameIndex].StartOffset, parser.tokens[nameIndex].EndOffset)
			if err == nil {
				parser.dependencies = append(parser.dependencies, StructuralDependency{
					Kind: StructuralDependencyImport, Value: parser.tokens[nameIndex].Text, Range: rangeValue, Evidence: SymbolEvidenceStructural,
				})
			}
			return index + 1
		}
	}
	parser.builder.MarkIncomplete()
	return end
}

func (parser *rustParser) parseUse(keyword, end int) int {
	semicolon := parser.findSemicolon(keyword+1, end, parser.tokens[keyword].Nesting)
	if semicolon < 0 {
		parser.builder.MarkIncomplete()
		return end
	}
	first := parser.skipTrivia(keyword+1, semicolon)
	last := parser.previousCode(semicolon-1, first)
	if first < semicolon && last >= first {
		value := tokenRangeText(parser.tokens, first, last+1)
		if value != "" {
			rangeValue, err := parser.document.RangeFromUTF8Offsets(parser.tokens[first].StartOffset, parser.tokens[last].EndOffset)
			if err == nil {
				parser.dependencies = append(parser.dependencies, StructuralDependency{
					Kind: StructuralDependencyImport, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural,
				})
			}
		}
	}
	return semicolon + 1
}

func (parser *rustParser) parseStruct(declarationStart, keyword, end int, parent *SymbolParent) int {
	nameIndex := parser.nextIdentifier(keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	base := parser.tokens[keyword].Nesting
	open := parser.findToken(keyword+1, end, "{", base+1)
	semi := parser.findSemicolon(keyword+1, end, base)
	if open < 0 || (semi >= 0 && semi < open) {
		terminator := semi
		if terminator < 0 {
			terminator = parser.statementEnd(keyword, end)
		}
		parser.add(SymbolSpec{
			Kind: SymbolKindStruct, NativeKind: "struct", Name: parser.tokens[nameIndex].Text, Parent: parent,
			Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[terminator].EndOffset},
			NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
			Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[terminator].EndOffset},
			Visibility:  parser.visibility(declarationStart, keyword), Evidence: SymbolEvidenceStructural,
		})
		return min(terminator+1, end)
	}
	close := parser.pairs[open]
	if close <= open {
		parser.builder.MarkIncomplete()
		return end
	}
	symbol, added := parser.add(SymbolSpec{
		Kind: SymbolKindStruct, NativeKind: "struct", Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[open].StartOffset},
		Body:        &OffsetRange{Start: parser.tokens[open].StartOffset, End: parser.tokens[close].EndOffset},
		Visibility:  parser.visibility(declarationStart, keyword), Evidence: SymbolEvidenceStructural,
	})
	if added {
		structParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		parser.parseScope(open+1, close, structParent, rustScopeStruct)
	}
	return close + 1
}

func (parser *rustParser) parseStructField(declarationStart, semantic, end int, parent *SymbolParent) (int, bool) {
	nameIndex := semantic
	if parser.tokens[nameIndex].Kind != TokenIdentifier {
		return declarationStart + 1, false
	}
	base := parser.tokens[nameIndex].Nesting
	colon := parser.findToken(nameIndex+1, end, ":", base)
	if colon < 0 {
		return declarationStart + 1, false
	}
	terminator := parser.findFieldEnd(colon+1, end, base)
	next := terminator + 1
	if terminator < 0 {
		terminator = parser.previousCode(end-1, colon+1)
		if terminator < colon+1 {
			return declarationStart + 1, false
		}
		next = end
	}
	parser.add(SymbolSpec{
		Kind: SymbolKindField, NativeKind: "field", Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[terminator].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Visibility:  parser.visibility(declarationStart, semantic), Evidence: SymbolEvidenceStructural,
	})
	return next, true
}

func (parser *rustParser) parseEnum(declarationStart, keyword, end int, parent *SymbolParent) int {
	nameIndex := parser.nextIdentifier(keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	open := parser.findToken(keyword+1, end, "{", parser.tokens[keyword].Nesting+1)
	if open < 0 {
		return parser.skipUnknown(declarationStart, end)
	}
	close := parser.pairs[open]
	if close <= open {
		parser.builder.MarkIncomplete()
		return end
	}
	parser.add(SymbolSpec{
		Kind: SymbolKindEnum, NativeKind: "enum", Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[open].StartOffset},
		Body:        &OffsetRange{Start: parser.tokens[open].StartOffset, End: parser.tokens[close].EndOffset},
		Visibility:  parser.visibility(declarationStart, keyword), Evidence: SymbolEvidenceStructural,
	})
	return close + 1
}

func (parser *rustParser) parseTrait(declarationStart, keyword, end int, parent *SymbolParent) int {
	nameIndex := parser.nextIdentifier(keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	open := parser.findToken(keyword+1, end, "{", parser.tokens[keyword].Nesting+1)
	if open < 0 {
		return parser.skipUnknown(declarationStart, end)
	}
	close := parser.pairs[open]
	if close <= open {
		parser.builder.MarkIncomplete()
		return end
	}
	symbol, added := parser.add(SymbolSpec{
		Kind: SymbolKindTrait, NativeKind: "trait", Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[open].StartOffset},
		Body:        &OffsetRange{Start: parser.tokens[open].StartOffset, End: parser.tokens[close].EndOffset},
		Visibility:  parser.visibility(declarationStart, keyword), Evidence: SymbolEvidenceStructural,
	})
	if added {
		traitParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		parser.parseScope(open+1, close, traitParent, rustScopeTrait)
	}
	return close + 1
}

func (parser *rustParser) parseImpl(declarationStart, keyword, end int, parent *SymbolParent) int {
	base := parser.tokens[keyword].Nesting
	open := parser.findToken(keyword+1, end, "{", base+1)
	if open < 0 {
		parser.builder.MarkIncomplete()
		return parser.skipUnknown(declarationStart, end)
	}
	close := parser.pairs[open]
	if close <= open {
		parser.builder.MarkIncomplete()
		return end
	}
	headerStart := parser.skipGenericPrefix(keyword+1, open)
	forIndex := parser.findTextTopLevel(headerStart, open, "for")
	traitName := ""
	targetName := ""
	if forIndex >= 0 {
		traitName = tokenRangeText(parser.tokens, headerStart, forIndex)
		targetName = tokenRangeText(parser.tokens, forIndex+1, open)
	} else {
		targetName = tokenRangeText(parser.tokens, headerStart, open)
	}
	targetName = strings.TrimSpace(targetName)
	traitName = strings.TrimSpace(traitName)
	if targetName == "" {
		parser.builder.MarkIncomplete()
		return close + 1
	}
	nativeKind := "inherent-impl"
	if traitName != "" {
		nativeKind = "trait-impl"
	}
	symbol, added := parser.add(SymbolSpec{
		Kind: SymbolKindImplementation, NativeKind: nativeKind, Name: targetName, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[headerStart].StartOffset, End: parser.tokens[open-1].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[open].StartOffset},
		Body:        &OffsetRange{Start: parser.tokens[open].StartOffset, End: parser.tokens[close].EndOffset},
		Evidence:    SymbolEvidenceStructural, Disambiguator: nativeKind + ":" + traitName + ":" + targetName,
	})
	if traitName != "" {
		rangeValue, err := parser.document.RangeFromUTF8Offsets(parser.tokens[headerStart].StartOffset, parser.tokens[open-1].EndOffset)
		if err == nil {
			parser.relations = append(parser.relations, StructuralRelation{
				Kind: "implements", Source: targetName, Target: traitName, Range: rangeValue, Evidence: SymbolEvidenceStructural,
			})
		}
	}
	if added {
		implParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		parser.parseScope(open+1, close, implParent, rustScopeImpl)
	}
	return close + 1
}

func (parser *rustParser) skipGenericPrefix(start, end int) int {
	index := parser.skipTrivia(start, end)
	if index >= end || !parser.token(index, "<") {
		return index
	}
	depth := 0
	for ; index < end; index++ {
		switch parser.tokens[index].Text {
		case "<":
			depth++
		case ">":
			depth--
			if depth == 0 {
				return parser.skipTrivia(index+1, end)
			}
		}
	}
	return start
}

func (parser *rustParser) findTextTopLevel(start, end int, text string) int {
	angle := 0
	for index := start; index < end; index++ {
		switch parser.tokens[index].Text {
		case "<":
			angle++
		case ">":
			if angle > 0 {
				angle--
			}
		default:
			if angle == 0 && parser.tokens[index].Text == text {
				return index
			}
		}
	}
	return -1
}

func (parser *rustParser) parseFunction(declarationStart, keyword, end int, parent *SymbolParent, member bool) int {
	nameIndex := parser.nextIdentifier(keyword+1, end)
	if nameIndex < 0 {
		return keyword + 1
	}
	base := parser.tokens[keyword].Nesting
	paren := -1
	for index := nameIndex + 1; index < end; index++ {
		if parser.tokens[index].Text == "(" && parser.tokens[index].Nesting == base+1 {
			paren = index
			break
		}
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == base {
			return index + 1
		}
	}
	if paren < 0 {
		return nameIndex + 1
	}
	closeParen := parser.pairs[paren]
	if closeParen <= paren {
		parser.builder.MarkIncomplete()
		return paren + 1
	}
	bodyOpen := -1
	semicolon := -1
	for index := closeParen + 1; index < end; index++ {
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == base+1 {
			bodyOpen = index
			break
		}
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == base {
			semicolon = index
			break
		}
	}
	kind := SymbolKindFunction
	nativeKind := "function-declaration"
	if member {
		kind = SymbolKindMethod
		nativeKind = "method-declaration"
	}
	declarationEnd := parser.tokens[closeParen].EndOffset
	signatureEnd := declarationEnd
	next := closeParen + 1
	var body *OffsetRange
	if bodyOpen >= 0 {
		close := parser.pairs[bodyOpen]
		if close <= bodyOpen {
			parser.builder.MarkIncomplete()
			return end
		}
		declarationEnd = parser.tokens[close].EndOffset
		signatureEnd = parser.tokens[bodyOpen].StartOffset
		value := OffsetRange{Start: parser.tokens[bodyOpen].StartOffset, End: parser.tokens[close].EndOffset}
		body = &value
		next = close + 1
		if member {
			nativeKind = "method-definition"
		} else {
			nativeKind = "function-definition"
		}
	} else if semicolon >= 0 {
		declarationEnd = parser.tokens[semicolon].EndOffset
		signatureEnd = parser.tokens[semicolon].StartOffset
		next = semicolon + 1
	}
	parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: signatureEnd}, Body: body,
		Visibility: parser.visibility(declarationStart, keyword), Evidence: SymbolEvidenceStructural,
		Disambiguator: tokenRangeText(parser.tokens, nameIndex+1, closeParen+1) + ":" + nativeKind,
	})
	return next
}

func (parser *rustParser) parseValue(declarationStart, keyword, end int, parent *SymbolParent) int {
	semicolon := parser.findSemicolon(keyword+1, end, parser.tokens[keyword].Nesting)
	if semicolon < 0 {
		parser.builder.MarkIncomplete()
		return parser.skipUnknown(declarationStart, end)
	}
	nameIndex := parser.nextIdentifier(keyword+1, semicolon)
	if nameIndex < 0 {
		return semicolon + 1
	}
	kind := SymbolKindConstant
	nativeKind := parser.tokens[keyword].Text
	if parser.token(keyword, "static") {
		kind = SymbolKindVariable
	}
	parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[semicolon].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Visibility:  parser.visibility(declarationStart, keyword), Evidence: SymbolEvidenceStructural,
	})
	return semicolon + 1
}

func (parser *rustParser) parseTypeAlias(declarationStart, keyword, end int, parent *SymbolParent) int {
	semicolon := parser.findSemicolon(keyword+1, end, parser.tokens[keyword].Nesting)
	if semicolon < 0 {
		return parser.skipUnknown(declarationStart, end)
	}
	nameIndex := parser.nextIdentifier(keyword+1, semicolon)
	if nameIndex < 0 {
		return semicolon + 1
	}
	parser.add(SymbolSpec{
		Kind: SymbolKindAlias, NativeKind: "associated-type", Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[semicolon].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[semicolon].EndOffset},
		Visibility:  parser.visibility(declarationStart, keyword), Evidence: SymbolEvidenceStructural,
	})
	return semicolon + 1
}

func (parser *rustParser) statementEnd(start, end int) int {
	base := parser.tokens[start].Nesting
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == base {
			return index
		}
		if parser.tokens[index].Kind == TokenNewline && parser.tokens[index].Nesting == base {
			return index
		}
		if parser.tokens[index].Kind == TokenEOF {
			return index
		}
	}
	return end - 1
}

func (parser *rustParser) skipUnknown(start, end int) int {
	if next, ok := parser.skipMacro(start, end); ok {
		return next
	}
	terminator := parser.statementEnd(start, end)
	return min(terminator+1, end)
}

func (parser *rustParser) findSemicolon(start, end, nesting int) int {
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == nesting {
			return index
		}
	}
	return -1
}

func (parser *rustParser) findFieldEnd(start, end, nesting int) int {
	for index := start; index < end; index++ {
		if (parser.tokens[index].Text == "," || parser.tokens[index].Text == ";") && parser.tokens[index].Nesting == nesting {
			return index
		}
	}
	return -1
}

func (parser *rustParser) findToken(start, end int, text string, nesting int) int {
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == text && parser.tokens[index].Nesting == nesting {
			return index
		}
	}
	return -1
}

func (parser *rustParser) nextIdentifier(index, end int) int {
	for index < end {
		if parser.tokens[index].Kind == TokenIdentifier {
			return index
		}
		if parser.tokens[index].Text == ";" || parser.tokens[index].Kind == TokenEOF {
			return -1
		}
		index++
	}
	return -1
}

func (parser *rustParser) previousCode(index, start int) int {
	for index >= start {
		if parser.tokens[index].Kind != TokenNewline && parser.tokens[index].Kind != TokenDirective {
			return index
		}
		index--
	}
	return -1
}

func (parser *rustParser) visibility(start, end int) Visibility {
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == "pub" {
			return VisibilityPublic
		}
	}
	return ""
}

func (parser *rustParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
	symbol, err := parser.builder.Add(spec)
	if operation.KindOf(err) == operation.KindLimit {
		parser.stopped = true
		return NormalizedSymbol{}, false
	}
	if err != nil {
		parser.builder.MarkIncomplete()
		return NormalizedSymbol{}, false
	}
	return symbol, true
}

// maskRustRawStrings masks r###"..."###, br###"..."### and cr###"..."### while
// preserving every decoded UTF-8 byte offset and physical newline.
func maskRustRawStrings(ctx context.Context, text string) (string, []ScannerDiagnostic, error) {
	masked := []byte(text)
	changed := false
	var diagnostics []ScannerDiagnostic
	for index := 0; index < len(text); {
		if index&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return "", nil, err
			}
		}
		if strings.HasPrefix(text[index:], "//") {
			next := strings.IndexAny(text[index+2:], "\r\n")
			if next < 0 {
				break
			}
			index += next + 2
			continue
		}
		if strings.HasPrefix(text[index:], "/*") {
			depth := 1
			cursor := index + 2
			for cursor < len(text) && depth > 0 {
				if strings.HasPrefix(text[cursor:], "/*") {
					depth++
					cursor += 2
				} else if strings.HasPrefix(text[cursor:], "*/") {
					depth--
					cursor += 2
				} else {
					cursor++
				}
			}
			index = cursor
			continue
		}
		if text[index] == '"' {
			index++
			for index < len(text) {
				if text[index] == '\\' {
					index += min(2, len(text)-index)
					continue
				}
				if text[index] == '"' {
					index++
					break
				}
				index++
			}
			continue
		}
		openEnd, hashes, ok := rustRawStringOpening(text, index)
		if !ok {
			index++
			continue
		}
		closing := `"` + strings.Repeat("#", hashes)
		relative := strings.Index(text[openEnd:], closing)
		end := len(text)
		if relative >= 0 {
			end = openEnd + relative + len(closing)
		} else {
			diagnostics = append(diagnostics, ScannerDiagnostic{
				Code: "unterminated-raw-string", Message: "Rust raw string literal is not terminated",
				StartOffset: index, EndOffset: len(text),
			})
		}
		for cursor := index; cursor < end; cursor++ {
			if masked[cursor] != '\r' && masked[cursor] != '\n' {
				masked[cursor] = ' '
			}
		}
		changed = true
		index = end
	}
	if !changed {
		return text, diagnostics, nil
	}
	return string(masked), diagnostics, nil
}

func rustRawStringOpening(text string, start int) (contentStart, hashes int, ok bool) {
	if start > 0 {
		previous := text[start-1]
		if previous == '_' || previous >= '0' && previous <= '9' || previous >= 'A' && previous <= 'Z' || previous >= 'a' && previous <= 'z' {
			return 0, 0, false
		}
	}
	index := start
	if strings.HasPrefix(text[index:], "br") || strings.HasPrefix(text[index:], "cr") {
		index += 2
	} else if strings.HasPrefix(text[index:], "r") {
		index++
	} else {
		return 0, 0, false
	}
	for index < len(text) && text[index] == '#' {
		hashes++
		if hashes > 255 {
			return 0, 0, false
		}
		index++
	}
	if index >= len(text) || text[index] != '"' {
		return 0, 0, false
	}
	return index + 1, hashes, true
}
