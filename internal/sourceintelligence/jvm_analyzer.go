package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// JavaAnalyzer performs bounded declaration-level Java analysis without a JVM,
// compiler frontend, classpath, or build-system dependency.
type JavaAnalyzer struct{}

func (JavaAnalyzer) ID() AnalyzerID   { return AnalyzerJava }
func (JavaAnalyzer) Language() string { return "java" }
func (JavaAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeJVMFamily(ctx, document, options, false)
}

// KotlinAnalyzer performs bounded declaration-level Kotlin analysis without a
// compiler frontend, classpath, Gradle, or Kotlin runtime dependency.
type KotlinAnalyzer struct{}

func (KotlinAnalyzer) ID() AnalyzerID   { return AnalyzerKotlin }
func (KotlinAnalyzer) Language() string { return "kotlin" }
func (KotlinAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeJVMFamily(ctx, document, options, true)
}

var javaModifiers = map[string]struct{}{
	"abstract": {}, "final": {}, "native": {}, "private": {}, "protected": {}, "public": {}, "sealed": {}, "static": {},
	"strictfp": {}, "synchronized": {}, "transient": {}, "volatile": {}, "non-sealed": {},
}

var kotlinModifiers = map[string]struct{}{
	"abstract": {}, "actual": {}, "annotation": {}, "companion": {}, "const": {}, "crossinline": {}, "data": {}, "enum": {},
	"expect": {}, "external": {}, "final": {}, "infix": {}, "inline": {}, "inner": {}, "internal": {}, "lateinit": {},
	"noinline": {}, "open": {}, "operator": {}, "out": {}, "override": {}, "private": {}, "protected": {}, "public": {},
	"reified": {}, "sealed": {}, "suspend": {}, "tailrec": {}, "value": {}, "vararg": {},
}

func analyzeJVMFamily(ctx context.Context, document *SourceDocument, options AnalyzeOptions, kotlin bool) (AnalyzerResult, error) {
	return analyzeJVMFamilyScope(ctx, document, options, kotlin, false)
}

// analyzeJVMFamilyScope also permits JSP declaration fragments to reuse the
// Java member grammar without changing Java/Kotlin compilation-unit behavior.
func analyzeJVMFamilyScope(ctx context.Context, document *SourceDocument, options AnalyzeOptions, kotlin, members bool) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_jvm_source", document.Path, err)
	}
	language := "java"
	analyzer := AnalyzerJava
	profile := JavaScannerProfile()
	if kotlin {
		language = "kotlin"
		analyzer = AnalyzerKotlin
		profile = KotlinScannerProfile()
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: language, Analyzer: string(analyzer), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scan, err := ScanSource(ctx, document, profile, ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: maxNesting})
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, diagnostic := range scan.Diagnostics {
		value := OffsetRange{Start: diagnostic.StartOffset, End: diagnostic.EndOffset}
		_ = builder.AddDiagnostic(DiagnosticSpec{Code: language + "-" + diagnostic.Code, Message: diagnostic.Message, Severity: DiagnosticWarning, Range: &value, AffectsCoverage: true})
	}
	if !scan.Complete {
		builder.MarkIncomplete()
	}
	parser := &jvmParser{
		ctx: ctx, document: document, tokens: scan.Tokens, pairs: PairDelimiterTokens(scan.Tokens, nil), builder: builder, kotlin: kotlin,
	}
	parser.parseScope(0, len(scan.Tokens), nil, members, "")
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_jvm_source", document.Path, err)
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: parser.dependencies, Relations: parser.relations}, nil
}

type jvmParser struct {
	ctx           context.Context
	document      *SourceDocument
	tokens        []Token
	pairs         map[int]int
	builder       *SymbolBuilder
	kotlin        bool
	dependencies  []StructuralDependency
	relations     []StructuralRelation
	packageParent *SymbolParent
	stopped       bool
}

func (parser *jvmParser) parseScope(start, end int, parent *SymbolParent, members bool, owner string) {
	currentParent := parent
	memberDepth := -1
	if parser.kotlin && members && start > 0 {
		memberDepth = parser.tokens[start-1].Nesting
	}
	for index := start; index < end && !parser.stopped; {
		if parser.ctx.Err() != nil {
			return
		}
		index = nextStructuralToken(parser.tokens, index, end)
		if index >= end || parser.tokens[index].Kind == TokenEOF {
			return
		}
		if memberDepth >= 0 && parser.tokens[index].Nesting > memberDepth {
			index++
			continue
		}
		if parser.token(index, "@") {
			if parser.kotlin {
				if next := parser.skipKotlinAnnotations(index, end); next > index {
					candidate := nextStructuralToken(parser.tokens, next, end)
					if candidate < end && parser.tokens[candidate].Nesting == parser.tokens[index].Nesting &&
						parser.kotlinAnnotatedPropertyAt(candidate, end) {
						index = parser.parseKotlinProperty(index, end, currentParent)
					} else if candidate < end && parser.tokens[candidate].Nesting == parser.tokens[index].Nesting &&
						parser.kotlinAnnotatedFunctionAt(candidate, end) {
						index = parser.parseKotlinFunction(index, end, currentParent, members)
					} else if candidate < end && parser.tokens[candidate].Nesting == parser.tokens[index].Nesting {
						if keyword, _, nativeKind, ok := parser.typeDeclarationAt(candidate, end); ok &&
							keyword-candidate < 12 {
							// Keep a validated type's annotations in the existing class excerpt.
							index = parser.parseType(index, keyword, nativeKind, end, currentParent)
						} else {
							index = next
						}
					} else {
						index = next
					}
					continue
				}
			} else if !members {
				if next := parser.skipJavaAnnotations(index, end); next > index {
					index = next
					continue
				}
			}
		}
		if !members && parser.token(index, "package") {
			next, packageParent := parser.parsePackage(index, end)
			if packageParent != nil {
				currentParent = packageParent
				parser.packageParent = packageParent
			}
			index = next
			continue
		}
		if !members && parser.token(index, "import") {
			index = parser.parseImport(index, end)
			continue
		}
		if keyword, declarationStart, nativeKind, ok := parser.typeDeclarationAt(index, end); ok {
			index = parser.parseType(declarationStart, keyword, nativeKind, end, currentParent)
			continue
		}
		if parser.kotlin {
			if parser.token(index, "typealias") {
				index = parser.parseKotlinTypeAlias(index, end, currentParent)
				continue
			}
			if members && parser.token(index, "constructor") {
				index = parser.parseKotlinSecondaryConstructor(index, end, currentParent, owner)
				continue
			}
			if parser.token(index, "fun") || parser.kotlinFunctionWithModifiers(index, end) {
				index = parser.parseKotlinFunction(index, end, currentParent, members)
				continue
			}
			if parser.token(index, "val") || parser.token(index, "var") || parser.kotlinPropertyWithModifiers(index, end) {
				index = parser.parseKotlinProperty(index, end, currentParent)
				continue
			}
		} else if members {
			if next, ok := parser.parseJavaMember(index, end, currentParent, owner); ok {
				index = next
				continue
			}
		}
		index++
	}
}

func (parser *jvmParser) token(index int, value string) bool {
	return index >= 0 && index < len(parser.tokens) && parser.tokens[index].Text == value
}

func (parser *jvmParser) parsePackage(start, end int) (int, *SymbolParent) {
	terminator := parser.statementTerminator(start+1, end)
	if terminator < 0 {
		parser.builder.MarkIncomplete()
		return end, nil
	}
	valueEnd := terminator
	if parser.tokens[terminator].Kind == TokenNewline {
		valueEnd = terminator
	}
	first := nextIdentifierToken(parser.tokens, start+1, valueEnd)
	last := previousIdentifierToken(parser.tokens, valueEnd-1, start+1)
	if parser.kotlin {
		// Kotlin's soft "internal" modifier is also valid as a package segment.
		// Validate each separator rather than truncating a partially parsed name.
		first, last = start+1, valueEnd-1
		expectSegment := true
		valid := first <= last
		for index := first; index <= last && valid; index++ {
			token := parser.tokens[index]
			if expectSegment {
				valid = token.Kind == TokenIdentifier ||
					(token.Kind == TokenKeyword && token.Text == "internal")
			} else {
				valid = token.Text == "."
			}
			expectSegment = !expectSegment
		}
		if !valid || expectSegment {
			parser.builder.MarkIncomplete()
			return terminator + 1, nil
		}
	}
	if first < 0 || last < first {
		return terminator + 1, nil
	}
	name := tokenRangeText(parser.tokens, first, last+1)
	declarationEnd := parser.tokens[terminator].EndOffset
	if parser.tokens[terminator].Kind == TokenNewline {
		declarationEnd = parser.tokens[last].EndOffset
	}
	symbol, added := parser.add(SymbolSpec{
		Kind: SymbolKindPackage, NativeKind: "package", Name: name,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: parser.tokens[first].StartOffset, End: parser.tokens[last].EndOffset}, Evidence: SymbolEvidenceStructural,
	})
	if !added {
		return terminator + 1, nil
	}
	return terminator + 1, &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
}

func (parser *jvmParser) parseImport(start, end int) int {
	terminator := parser.statementTerminator(start+1, end)
	if terminator < 0 {
		parser.builder.MarkIncomplete()
		return end
	}
	cursor := start + 1
	if !parser.kotlin && parser.token(cursor, "static") {
		cursor++
	}
	alias := ""
	valueEnd := terminator
	if parser.kotlin {
		for index := cursor; index < terminator; index++ {
			if parser.token(index, "as") {
				valueEnd = index
				if aliasIndex := nextIdentifierToken(parser.tokens, index+1, terminator); aliasIndex >= 0 {
					alias = parser.tokens[aliasIndex].Text
				}
				break
			}
		}
	}
	value := tokenRangeText(parser.tokens, cursor, valueEnd)
	if value != "" {
		last := previousStructuralToken(parser.tokens, valueEnd-1, cursor)
		if last >= cursor {
			rangeValue, err := parser.document.RangeFromUTF8Offsets(parser.tokens[start].StartOffset, parser.tokens[last].EndOffset)
			if err == nil {
				parser.dependencies = append(parser.dependencies, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Alias: alias, Range: rangeValue, Evidence: SymbolEvidenceStructural})
			}
		}
	}
	return terminator + 1
}

func (parser *jvmParser) statementTerminator(start, end int) int {
	depth := 0
	if start < len(parser.tokens) {
		depth = parser.tokens[start].Nesting
	}
	for index := start; index < end; index++ {
		if !parser.kotlin && parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
			return index
		}
		if parser.kotlin && parser.tokens[index].Kind == TokenNewline && parser.tokens[index].Nesting == depth {
			return index
		}
		if parser.tokens[index].Kind == TokenEOF {
			return index
		}
	}
	return -1
}

func (parser *jvmParser) typeDeclarationAt(start, end int) (keyword, declarationStart int, nativeKind string, ok bool) {
	cursor := start
	declarationStart = start
	modifiers := kotlinModifiers
	if !parser.kotlin {
		modifiers = javaModifiers
	}
	var prefixes []string
	for cursor < end {
		text := strings.ToLower(parser.tokens[cursor].Text)
		if parser.kotlin && text == "enum" && cursor+1 < end && parser.token(cursor+1, "class") {
			return cursor + 1, declarationStart, "enum-class", true
		}
		if parser.kotlin && text == "fun" && cursor+1 < end && parser.token(cursor+1, "interface") {
			return cursor + 1, declarationStart, "fun-interface", true
		}
		if _, modifier := modifiers[text]; modifier {
			prefixes = append(prefixes, text)
			cursor++
			continue
		}
		if !parser.kotlin && text == "class" {
			previous := previousStructuralToken(parser.tokens, cursor-1, 0)
			if previous >= 0 && parser.tokens[previous].Text == "." {
				return 0, start, "", false
			}
		}
		if text == "class" || text == "interface" ||
			(parser.kotlin && text == "object" && cursor+1 < end && parser.tokens[cursor+1].Kind == TokenIdentifier) ||
			(!parser.kotlin && (text == "enum" || text == "record")) {
			nativeKind = text
			if parser.kotlin && text == "class" {
				if containsLower(prefixes, "value") {
					nativeKind = "value-class"
				} else if containsLower(prefixes, "data") {
					nativeKind = "data-class"
				} else if containsLower(prefixes, "sealed") {
					nativeKind = "sealed-class"
				}
			}
			return cursor, declarationStart, nativeKind, true
		}
		return 0, start, "", false
	}
	return 0, start, "", false
}

func containsLower(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

// kotlinTypeHeaderContinues recognizes a narrowly bounded primary-constructor
// header continuation instead of attaching unrelated declarations after a class.
func (parser *jvmParser) kotlinTypeHeaderContinues(nameIndex, newline, end, depth int, continued bool) bool {
	next := nextStructuralToken(parser.tokens, newline+1, end)
	if next >= end {
		return false
	}
	source := parser.document.Text
	startOffset := parser.tokens[nameIndex].EndOffset
	stopOffset := parser.tokens[next].StartOffset
	if stopOffset < startOffset || stopOffset-startOffset > 4096 ||
		strings.Count(source[startOffset:stopOffset], "\n") > 8 {
		return false
	}
	if continued && parser.token(next, "{") && parser.tokens[next].Nesting == depth+1 {
		return true
	}
	if parser.tokens[next].Nesting != depth {
		return false
	}
	if parser.token(next, ":") || (continued && parser.token(next, "where")) {
		return true
	}
	cursor := next
	if parser.token(cursor, "@") {
		cursor = parser.skipKotlinAnnotations(cursor, end)
	}
	for count := 0; count < 4 && cursor < end; count++ {
		if _, modifier := kotlinModifiers[strings.ToLower(parser.tokens[cursor].Text)]; !modifier {
			break
		}
		cursor = nextStructuralToken(parser.tokens, cursor+1, end)
	}
	return parser.token(cursor, "constructor") &&
		parser.token(nextStructuralToken(parser.tokens, cursor+1, end), "(")
}

func (parser *jvmParser) parseType(start, keyword int, nativeKind string, end int, parent *SymbolParent) int {
	depth := parser.tokens[keyword].Nesting
	nameIndex := nextIdentifierToken(parser.tokens, keyword+1, end)
	if nameIndex < 0 {
		parser.builder.MarkIncomplete()
		return keyword + 1
	}
	open := -1
	lineEnd := -1
	continuedHeader := false
	for index := nameIndex + 1; index < end; index++ {
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == depth+1 {
			open = index
			break
		}
		if parser.kotlin && (parser.tokens[index].Kind == TokenNewline || parser.tokens[index].Kind == TokenEOF) && parser.tokens[index].Nesting == depth {
			if parser.tokens[index].Kind == TokenNewline &&
				parser.kotlinTypeHeaderContinues(nameIndex, index, end, depth, continuedHeader) {
				continuedHeader = true
				continue
			}
			lineEnd = index
			break
		}
		if !parser.kotlin && parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
			lineEnd = index
			break
		}
	}
	close := -1
	terminator := lineEnd
	if open >= 0 {
		close = parser.pairs[open]
		if close <= open || close >= end {
			parser.builder.MarkIncomplete()
			return end
		}
		terminator = close
	}
	if terminator < 0 {
		parser.builder.MarkIncomplete()
		return end
	}
	kind := SymbolKindClass
	switch nativeKind {
	case "interface", "fun-interface":
		kind = SymbolKindInterface
	case "enum", "enum-class":
		kind = SymbolKindEnum
	case "record":
		kind = SymbolKindRecord
	}
	modifiers := collectKnownModifiers(parser.tokens, start, keyword, parser.modifierSet())
	declarationEnd := parser.tokens[terminator].EndOffset
	signatureEnd := declarationEnd
	var body *OffsetRange
	if open >= 0 {
		signatureEnd = parser.tokens[open].StartOffset
		value := OffsetRange{Start: parser.tokens[open].StartOffset, End: parser.tokens[close].EndOffset}
		body = &value
	} else if parser.tokens[terminator].Kind == TokenNewline || parser.tokens[terminator].Kind == TokenEOF {
		previous := previousStructuralToken(parser.tokens, terminator-1, nameIndex)
		if previous >= nameIndex {
			declarationEnd = parser.tokens[previous].EndOffset
			signatureEnd = declarationEnd
		}
	}
	symbol, added := parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[start].StartOffset, End: signatureEnd}, Body: body,
		Visibility: visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
	})
	if !added {
		return terminator + 1
	}
	typeParent := &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
	if parser.kotlin {
		parser.collectKotlinTypeHeader(symbol.QualifiedName, nameIndex+1, chooseHeaderEnd(open, terminator), depth, typeParent, symbol.Name)
	} else {
		parser.collectJavaTypeRelations(symbol.QualifiedName, nameIndex+1, chooseHeaderEnd(open, terminator), depth)
	}
	if open >= 0 && kind != SymbolKindEnum {
		parser.parseScope(open+1, close, typeParent, true, symbol.Name)
	}
	return terminator + 1
}

func chooseHeaderEnd(open, terminator int) int {
	if open >= 0 {
		return open
	}
	return terminator
}

func (parser *jvmParser) modifierSet() map[string]struct{} {
	if parser.kotlin {
		return kotlinModifiers
	}
	return javaModifiers
}

func (parser *jvmParser) collectJavaTypeRelations(source string, start, end, nesting int) {
	clauses := map[string]string{"extends": "extends", "implements": "implements", "permits": "permits"}
	for index := start; index < end; {
		kind, marker := clauses[parser.tokens[index].Text]
		if !marker {
			index++
			continue
		}
		clauseStart := index + 1
		clauseEnd := end
		for cursor := clauseStart; cursor < end; cursor++ {
			if _, nextClause := clauses[parser.tokens[cursor].Text]; nextClause {
				clauseEnd = cursor
				break
			}
		}
		for _, part := range splitCommaTokenRangeAt(parser.tokens, clauseStart, clauseEnd, nesting) {
			target := normalizedTypeSpelling(parser.tokens, part[0], part[1], nil)
			parser.addRelation(kind, source, target, part[0], part[1])
		}
		index = clauseEnd
	}
}

func (parser *jvmParser) collectKotlinTypeHeader(source string, start, end, nesting int, parent *SymbolParent, owner string) {
	primaryOpen := -1
	colon := -1
	for index := start; index < end; index++ {
		if parser.tokens[index].Text == "(" && parser.tokens[index].Nesting == nesting+1 && primaryOpen < 0 {
			primaryOpen = index
			if close := parser.pairs[index]; close > index {
				parser.addKotlinPrimaryConstructor(parent, owner, index, close)
				parser.addKotlinPrimaryProperties(parent, index+1, close)
				index = close
			}
			continue
		}
		if parser.tokens[index].Text == ":" && parser.tokens[index].Nesting == nesting {
			colon = index
			break
		}
	}
	if colon < 0 {
		return
	}
	parts, balanced := splitKotlinSupertypeRangeAt(parser.tokens, colon+1, end, nesting)
	if !balanced {
		parser.builder.MarkIncomplete()
	}
	for _, part := range parts {
		targetEnd := parser.kotlinSupertypeTypeEnd(part[0], part[1], nesting)
		target := normalizedTypeSpelling(parser.tokens, part[0], targetEnd, nil)
		for strings.HasSuffix(target, "()") {
			target = strings.TrimSuffix(target, "()")
		}
		parser.addRelation("supertype", source, target, part[0], part[1])
	}
}

func (parser *jvmParser) kotlinSupertypeTypeEnd(start, end, nesting int) int {
	for index := start; index < end; index++ {
		if parser.tokens[index].Text != "(" || parser.tokens[index].Nesting != nesting+1 {
			continue
		}
		previous := previousStructuralToken(parser.tokens, index-1, start)
		if previous < start {
			continue
		}
		close := parser.pairs[index]
		if close <= index || close >= end {
			continue
		}
		if nextStructuralToken(parser.tokens, close+1, end) >= end {
			return index
		}
	}
	return end
}

func splitKotlinSupertypeRangeAt(tokens []Token, start, end, nesting int) ([][2]int, bool) {
	var result [][2]int
	partStart := start
	genericDepth := 0
	delegated := false
	for index := start; index < end; index++ {
		if tokens[index].Nesting != nesting {
			continue
		}
		text := tokens[index].Text
		if genericDepth == 0 && text == "by" {
			delegated = true
			continue
		}
		if !delegated && text != "" {
			if strings.Trim(text, "<") == "" {
				genericDepth += len(text)
				continue
			}
			if genericDepth > 0 && strings.Trim(text, ">") == "" {
				genericDepth -= min(genericDepth, len(text))
				continue
			}
		}
		if text == "," && genericDepth == 0 {
			if partStart < index {
				result = append(result, [2]int{partStart, index})
			}
			partStart = index + 1
			delegated = false
		}
	}
	if partStart < end {
		result = append(result, [2]int{partStart, end})
	}
	return result, genericDepth == 0
}

func (parser *jvmParser) addKotlinPrimaryConstructor(parent *SymbolParent, owner string, open, close int) {
	if parent == nil || owner == "" {
		return
	}
	start := open
	if previous := previousStructuralToken(parser.tokens, open-1, 0); previous >= 0 {
		start = previous
	}
	parser.add(SymbolSpec{
		Kind: SymbolKindConstructor, NativeKind: "primary-constructor", Name: owner, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[close].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[start].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[close].EndOffset}, Evidence: SymbolEvidenceStructural,
		Disambiguator: "primary:" + tokenRangeText(parser.tokens, open, close+1),
	})
}

func (parser *jvmParser) addKotlinPrimaryProperties(parent *SymbolParent, start, end int) {
	for _, part := range splitCommaTokenRangeAt(parser.tokens, start, end, parser.tokens[start-1].Nesting) {
		marker := -1
		for index := part[0]; index < part[1]; index++ {
			if parser.token(index, "val") || parser.token(index, "var") {
				marker = index
				break
			}
		}
		if marker < 0 {
			continue
		}
		nameIndex := nextIdentifierToken(parser.tokens, marker+1, part[1])
		if nameIndex < 0 {
			continue
		}
		declarationStart := marker
		if annotation := nextStructuralToken(parser.tokens, part[0], marker); annotation < marker &&
			parser.token(annotation, "@") && parser.skipKotlinAnnotations(annotation, part[1]) == marker {
			declarationStart = annotation
		}
		parser.add(SymbolSpec{
			Kind: SymbolKindProperty, NativeKind: parser.tokens[marker].Text + "-parameter", Name: parser.tokens[nameIndex].Text, Parent: parent,
			Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[part[1]-1].EndOffset},
			NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset}, Evidence: SymbolEvidenceStructural,
		})
	}
}

func (parser *jvmParser) addRelation(kind, source, target string, start, end int) {
	if target == "" || start < 0 || end <= start || end > len(parser.tokens) {
		return
	}
	rangeValue, err := parser.document.RangeFromUTF8Offsets(parser.tokens[start].StartOffset, parser.tokens[end-1].EndOffset)
	if err == nil {
		parser.relations = append(parser.relations, StructuralRelation{Kind: kind, Source: source, Target: target, Range: rangeValue, Evidence: SymbolEvidenceStructural})
	}
}

func (parser *jvmParser) skipKotlinAnnotations(start, end int) int {
	cursor := start
	for cursor < end {
		for cursor < end && parser.tokens[cursor].Kind == TokenNewline {
			cursor++
		}
		if cursor >= end || parser.tokens[cursor].Text != "@" {
			return cursor
		}
		annotationStart := cursor
		cursor++
		if cursor >= end || (parser.tokens[cursor].Kind != TokenIdentifier && parser.tokens[cursor].Kind != TokenKeyword) {
			return annotationStart
		}
		cursor++
		if cursor < end && parser.tokens[cursor].Text == ":" {
			cursor++
			if cursor >= end || (parser.tokens[cursor].Kind != TokenIdentifier && parser.tokens[cursor].Kind != TokenKeyword) {
				return annotationStart
			}
			cursor++
		}
		for cursor+1 < end && parser.tokens[cursor].Text == "." && (parser.tokens[cursor+1].Kind == TokenIdentifier || parser.tokens[cursor+1].Kind == TokenKeyword) {
			cursor += 2
		}
		if cursor < end && parser.tokens[cursor].Text == "(" {
			close := parser.pairs[cursor]
			if close <= cursor || close >= end {
				return annotationStart
			}
			cursor = close + 1
		}
	}
	return cursor
}

func (parser *jvmParser) skipJavaAnnotations(start, end int) int {
	cursor := start
	for cursor < end {
		for cursor < end && parser.tokens[cursor].Kind == TokenNewline {
			cursor++
		}
		if cursor >= end || parser.tokens[cursor].Text != "@" {
			return cursor
		}
		annotationStart := cursor
		cursor++
		if cursor >= end || parser.tokens[cursor].Kind != TokenIdentifier {
			return annotationStart
		}
		cursor++
		for cursor+1 < end && parser.tokens[cursor].Text == "." && parser.tokens[cursor+1].Kind == TokenIdentifier {
			cursor += 2
		}
		if cursor < end && parser.tokens[cursor].Text == "(" {
			close := parser.pairs[cursor]
			if close <= cursor || close >= end {
				return annotationStart
			}
			cursor = close + 1
		}
	}
	return cursor
}
func (parser *jvmParser) parseJavaMember(start, end int, parent *SymbolParent, owner string) (int, bool) {
	declarationStart := start
	semanticStart := parser.skipJavaAnnotations(start, end)
	if semanticStart >= end {
		return start + 1, false
	}
	depth := parser.tokens[semanticStart].Nesting
	terminator := -1
	for index := semanticStart; index < end; index++ {
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
			terminator = index
			break
		}
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == depth+1 {
			terminator = index
			break
		}
	}
	if terminator < 0 {
		return start + 1, false
	}
	paren := -1
	for index := semanticStart; index < terminator; index++ {
		if parser.tokens[index].Text == "=" && parser.tokens[index].Nesting == depth {
			break
		}
		if parser.tokens[index].Text == "(" && parser.tokens[index].Nesting == depth+1 {
			paren = index
			break
		}
	}
	if paren >= 0 {
		nameIndex := previousIdentifierToken(parser.tokens, paren-1, semanticStart)
		if nameIndex >= 0 {
			kind := SymbolKindMethod
			nativeKind := "method-declaration"
			if parser.tokens[nameIndex].Text == owner {
				kind = SymbolKindConstructor
				nativeKind = "constructor-declaration"
			}
			closeParen := parser.pairs[paren]
			if closeParen <= paren {
				parser.builder.MarkIncomplete()
				return terminator + 1, false
			}
			bodyOpen := -1
			for index := closeParen + 1; index < end; index++ {
				if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == depth+1 {
					bodyOpen = index
					break
				}
				if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
					terminator = index
					break
				}
			}
			declarationEnd := parser.tokens[terminator].EndOffset
			var body *OffsetRange
			next := terminator + 1
			if bodyOpen >= 0 {
				close := parser.pairs[bodyOpen]
				if close <= bodyOpen || close >= end {
					parser.builder.MarkIncomplete()
					return end, false
				}
				declarationEnd = parser.tokens[close].EndOffset
				value := OffsetRange{Start: parser.tokens[bodyOpen].StartOffset, End: parser.tokens[close].EndOffset}
				body = &value
				next = close + 1
				if kind == SymbolKindConstructor {
					nativeKind = "constructor-definition"
				} else {
					nativeKind = "method-definition"
				}
			}
			modifiers := collectKnownModifiers(parser.tokens, semanticStart, nameIndex, javaModifiers)
			_, added := parser.add(SymbolSpec{
				Kind: kind, NativeKind: nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
				Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: declarationEnd},
				NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
				Signature:   &OffsetRange{Start: parser.tokens[semanticStart].StartOffset, End: parser.tokens[bodyStartOrTerminator(bodyOpen, terminator)].StartOffset}, Body: body,
				Visibility: visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
				Disambiguator: tokenRangeText(parser.tokens, paren, closeParen+1),
			})
			return next, added
		}
	}
	limit := terminator
	for index := semanticStart; index < terminator; index++ {
		if parser.tokens[index].Text == "=" && parser.tokens[index].Nesting == depth {
			limit = index
			break
		}
	}
	nameIndex := previousIdentifierToken(parser.tokens, limit-1, semanticStart)
	if nameIndex < 0 {
		return terminator + 1, false
	}
	modifiers := collectKnownModifiers(parser.tokens, semanticStart, nameIndex, javaModifiers)
	kind := SymbolKindField
	nativeKind := "field"
	if containsLower(modifiers, "static") && containsLower(modifiers, "final") {
		kind = SymbolKindConstant
		nativeKind = "constant"
	}
	_, added := parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[declarationStart].StartOffset, End: parser.tokens[terminator].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Visibility:  visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
	})
	return terminator + 1, added
}

func (parser *jvmParser) kotlinFunctionWithModifiers(start, end int) bool {
	for index := start; index < end && index < start+12; index++ {
		if parser.token(index, "fun") {
			return true
		}
		if parser.tokens[index].Kind == TokenNewline || parser.tokens[index].Text == ";" || parser.tokens[index].Text == "{" {
			return false
		}
	}
	return false
}

func (parser *jvmParser) parseKotlinFunction(start, end int, parent *SymbolParent, members bool) int {
	funIndex := start
	for funIndex < end && !parser.token(funIndex, "fun") {
		funIndex++
	}
	if funIndex >= end {
		return start + 1
	}
	paren := -1
	for index := funIndex + 1; index < end; index++ {
		if parser.tokens[index].Text == "(" {
			paren = index
			break
		}
		if parser.tokens[index].Kind == TokenNewline {
			return index + 1
		}
	}
	if paren < 0 {
		return funIndex + 1
	}
	nameIndex := previousIdentifierToken(parser.tokens, paren-1, funIndex+1)
	closeParen := parser.pairs[paren]
	if nameIndex < 0 || closeParen <= paren {
		parser.builder.MarkIncomplete()
		return paren + 1
	}
	depth := parser.tokens[funIndex].Nesting
	bodyOpen, terminator := -1, -1
	for index := closeParen + 1; index < end; index++ {
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == depth+1 {
			bodyOpen = index
			break
		}
		if parser.tokens[index].Kind == TokenNewline && parser.tokens[index].Nesting == depth {
			terminator = index
			break
		}
	}
	declarationEnd := parser.tokens[closeParen].EndOffset
	next := closeParen + 1
	var body *OffsetRange
	signatureEnd := declarationEnd
	if bodyOpen >= 0 {
		close := parser.pairs[bodyOpen]
		if close <= bodyOpen || close >= end {
			parser.builder.MarkIncomplete()
			return end
		}
		declarationEnd = parser.tokens[close].EndOffset
		signatureEnd = parser.tokens[bodyOpen].StartOffset
		value := OffsetRange{Start: parser.tokens[bodyOpen].StartOffset, End: parser.tokens[close].EndOffset}
		body = &value
		next = close + 1
	} else if terminator >= 0 {
		previous := previousStructuralToken(parser.tokens, terminator-1, closeParen)
		if previous >= closeParen {
			declarationEnd = parser.tokens[previous].EndOffset
			signatureEnd = declarationEnd
		}
		next = terminator + 1
		if parser.token(previous, "=") && strings.TrimSpace(parser.document.Text[parser.tokens[previous].EndOffset:parser.tokens[terminator].StartOffset]) == "" {
			if expression, after, ok := parser.kotlinWrappedExpressionLine(start, terminator, end, depth); ok {
				declarationEnd = expression.End
				body = &expression
				next = after
			} else {
				// A wrapped expression we cannot bound safely needs direct inspection.
				parser.builder.MarkIncomplete()
			}
		}
	}
	kind := SymbolKindFunction
	if members {
		kind = SymbolKindMethod
	}
	modifiers := collectKnownModifiers(parser.tokens, start, funIndex, kotlinModifiers)
	parser.add(SymbolSpec{
		Kind: kind, NativeKind: string(kind), Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Signature:   &OffsetRange{Start: parser.tokens[start].StartOffset, End: signatureEnd}, Body: body,
		Visibility: visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
		Disambiguator: tokenRangeText(parser.tokens, paren, closeParen+1),
	})
	return next
}

// A trailing '=' can introduce a complete, single-physical-line Kotlin
// expression on the next indented line. Do not promote an unrelated peer or
// claim to parse a multi-line expression, chained call, or flow semantics.
func (parser *jvmParser) kotlinWrappedExpressionLine(start, newline, end, depth int) (OffsetRange, int, bool) {
	first := nextStructuralToken(parser.tokens, newline+1, end)
	if first >= end || parser.tokens[first].Kind == TokenEOF {
		return OffsetRange{}, 0, false
	}
	text := parser.document.Text
	if kotlinSourceIndent(text, parser.tokens[first].StartOffset) <= kotlinSourceIndent(text, parser.tokens[start].StartOffset) {
		return OffsetRange{}, 0, false
	}
	lineEnd := len(text)
	if at := strings.IndexByte(text[parser.tokens[first].StartOffset:], '\n'); at >= 0 {
		lineEnd = parser.tokens[first].StartOffset + at
	}
	for index := first; index < end && parser.tokens[index].StartOffset <= lineEnd; index++ {
		if parser.tokens[index].Kind != TokenNewline {
			continue
		}
		if parser.tokens[index].Nesting != depth {
			return OffsetRange{}, 0, false
		}
		last := previousStructuralToken(parser.tokens, index-1, first)
		if last < first {
			return OffsetRange{}, 0, false
		}
		switch parser.tokens[last].Text {
		case "=", ".", ",", "+", "-", "?", ":", "&&", "||", "?:", "->":
			return OffsetRange{}, 0, false
		}
		return OffsetRange{Start: parser.tokens[first].StartOffset, End: parser.tokens[last].EndOffset}, index + 1, true
	}
	return OffsetRange{}, 0, false
}

// kotlinSourceIndent reads only the leading whitespace of the token's physical line.
func kotlinSourceIndent(source string, offset int) int {
	lineStart := strings.LastIndexByte(source[:offset], '\n') + 1
	indent := 0
	for lineStart+indent < offset && (source[lineStart+indent] == ' ' || source[lineStart+indent] == '\t') {
		indent++
	}
	return indent
}

// kotlinMultilineGetterExpression accepts only a bounded next-line expression:
// a chain with explicit continuation prefixes or a balanced if/when block.
// Raw-line limits prevent scanning an unrelated tail of an oversized source.
func (parser *jvmParser) kotlinMultilineGetterExpression(getter, equals, newline, end int) (int, int, bool) {
	source := parser.document.Text
	if strings.TrimSpace(source[parser.tokens[equals].EndOffset:parser.tokens[newline].StartOffset]) != "" {
		return 0, 0, false
	}
	getterIndent := kotlinSourceIndent(source, parser.tokens[getter].StartOffset)
	lineStart := parser.tokens[newline].EndOffset
	firstLine, lastLineEnd := -1, -1
	control := false
	previousContinuation := false
	bounded := false
	for lines := 0; lineStart < len(source) && lines < 96; lines++ {
		lineEnd := len(source)
		if at := strings.IndexByte(source[lineStart:], '\n'); at >= 0 {
			lineEnd = lineStart + at
		}
		line := source[lineStart:lineEnd]
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			if indent <= getterIndent {
				bounded = true
				break
			}
			if lineEnd-parser.tokens[equals].EndOffset > 16384 ||
				strings.HasPrefix(trimmed, "fun ") || strings.HasPrefix(trimmed, "val ") ||
				strings.HasPrefix(trimmed, "var ") || strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "get(") || strings.HasPrefix(trimmed, "set(") {
				return 0, 0, false
			}
			if firstLine < 0 {
				firstLine = lineStart + indent
				control = strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "if(") ||
					strings.HasPrefix(trimmed, "when ") || strings.HasPrefix(trimmed, "when(")
			} else if !control && !previousContinuation &&
				!strings.HasPrefix(trimmed, ".") && !strings.HasPrefix(trimmed, "?:") &&
				!strings.HasPrefix(trimmed, "!!") && !strings.HasPrefix(trimmed, "::") &&
				!strings.HasPrefix(trimmed, "&&") && !strings.HasPrefix(trimmed, "||") {
				return 0, 0, false
			}
			previousContinuation = false
			for _, suffix := range []string{".", ",", "+", "-", "?:", "&&", "||", "->"} {
				if strings.HasSuffix(trimmed, suffix) {
					previousContinuation = true
					break
				}
			}
			lastLineEnd = lineEnd
		}
		if lineEnd == len(source) {
			bounded = true
			break
		}
		lineStart = lineEnd + 1
	}
	if (!bounded && lineStart < len(source)) || firstLine < 0 || lastLineEnd <= firstLine {
		return 0, 0, false
	}
	first, last := -1, -1
	hasBodyBraces := false
	for index := equals + 1; index < end && parser.tokens[index].StartOffset < lastLineEnd; index++ {
		token := parser.tokens[index]
		if token.Kind == TokenNewline || token.Kind == TokenDirective {
			continue
		}
		if token.Kind == TokenEOF || token.EndOffset > lastLineEnd ||
			strings.Contains(source[token.StartOffset:token.EndOffset], "\n") {
			return 0, 0, false
		}
		if first < 0 {
			first = index
		}
		last = index
	}
	if first < 0 || parser.tokens[first].StartOffset < firstLine || last < first {
		return 0, 0, false
	}
	switch parser.tokens[last].Text {
	case "=", ".", ",", "+", "-", "?", ":", "&&", "||", "?:", "->":
		return 0, 0, false
	}
	for index := first; index <= last; index++ {
		switch parser.tokens[index].Text {
		case "{", "(", "[":
			close := parser.pairs[index]
			if close <= index || close > last {
				return 0, 0, false
			}
			if parser.tokens[index].Text == "{" {
				hasBodyBraces = true
			}
		}
	}
	if control && !hasBodyBraces {
		return 0, 0, false
	}
	return parser.tokens[last].EndOffset, last + 1, true
}

// kotlinPropertyGetter bounds only an explicitly indented, parameterless getter
// with a single physical expression line or a balanced brace body. It does not
// infer Kotlin expression continuations or standalone get() calls as accessors.
func (parser *jvmParser) kotlinPropertyGetter(propertyStart, from, end, depth int) (int, int, bool, bool) {
	getter := nextStructuralToken(parser.tokens, from, end)
	if getter >= end || !parser.token(getter, "get") || parser.tokens[getter].Nesting != depth {
		return 0, 0, false, false
	}
	source := parser.document.Text
	if kotlinSourceIndent(source, parser.tokens[getter].StartOffset) <= kotlinSourceIndent(source, parser.tokens[propertyStart].StartOffset) {
		return 0, 0, false, false
	}
	open := nextStructuralToken(parser.tokens, getter+1, end)
	if !parser.token(open, "(") || parser.pairs[open] != open+1 {
		return 0, 0, true, false
	}
	bodyStart := nextStructuralToken(parser.tokens, open+2, end)
	if bodyStart >= end {
		return 0, 0, true, false
	}
	if parser.token(bodyStart, "{") && parser.tokens[bodyStart].Nesting == depth+1 {
		close := parser.pairs[bodyStart]
		if close <= bodyStart || close >= end {
			return 0, 0, true, false
		}
		return parser.tokens[close].EndOffset, close + 1, true, true
	}
	if !parser.token(bodyStart, "=") || parser.tokens[bodyStart].Nesting != depth {
		return 0, 0, true, false
	}
	if bodyStart+1 < end && parser.tokens[bodyStart+1].Kind == TokenNewline &&
		parser.tokens[bodyStart+1].Nesting == depth {
		last, after, ok := parser.kotlinMultilineGetterExpression(getter, bodyStart, bodyStart+1, end)
		return last, after, true, ok
	}
	for index := bodyStart + 1; index < end; index++ {
		if parser.tokens[index].Kind != TokenNewline {
			continue
		}
		if parser.tokens[index].Nesting != depth {
			return 0, 0, true, false
		}
		// Preserve literal RHS tokens: previousStructuralToken intentionally skips strings.
		last := index - 1
		for last > bodyStart && parser.tokens[last].Kind == TokenDirective {
			last--
		}
		if last <= bodyStart || strings.Contains(source[parser.tokens[last].StartOffset:parser.tokens[last].EndOffset], "\n") {
			return 0, 0, true, false
		}
		switch parser.tokens[last].Text {
		case "=", ".", ",", "+", "-", "?", ":", "&&", "||", "?:", "->":
			return 0, 0, true, false
		}
		following := nextStructuralToken(parser.tokens, index+1, end)
		if following < end && parser.tokens[following].Nesting == depth {
			followingIndent := kotlinSourceIndent(source, parser.tokens[following].StartOffset)
			if followingIndent > kotlinSourceIndent(source, parser.tokens[getter].StartOffset) {
				return 0, 0, true, false
			}
			if followingIndent > kotlinSourceIndent(source, parser.tokens[propertyStart].StartOffset) {
				// A continuation may align with the getter instead of being deeper.
				switch parser.tokens[following].Text {
				case ".", "?.", "?:", "?", "!!", "::", "+", "-", "&&", "||", "(", "[":
					return 0, 0, true, false
				}
			}
		}
		return parser.tokens[last].EndOffset, index + 1, true, true
	}
	return 0, 0, true, false
}

// kotlinPropertySetter keeps a directly indented, one-parameter brace setter
// within its owning property. Expression setters and malformed blocks remain partial.
func (parser *jvmParser) kotlinPropertySetter(propertyStart, from, end, depth int) (int, int, bool, bool) {
	setter := nextStructuralToken(parser.tokens, from, end)
	if setter >= end || !parser.token(setter, "set") || parser.tokens[setter].Nesting != depth {
		return 0, 0, false, false
	}
	if kotlinSourceIndent(parser.document.Text, parser.tokens[setter].StartOffset) <=
		kotlinSourceIndent(parser.document.Text, parser.tokens[propertyStart].StartOffset) {
		return 0, 0, false, false
	}
	open := nextStructuralToken(parser.tokens, setter+1, end)
	if !parser.token(open, "(") {
		return 0, 0, true, false
	}
	close := parser.pairs[open]
	if close <= open+1 || close >= end {
		return 0, 0, true, false
	}
	param := nextIdentifierToken(parser.tokens, open+1, close)
	if param != open+1 || nextStructuralToken(parser.tokens, param+1, close) != close {
		return 0, 0, true, false
	}
	body := nextStructuralToken(parser.tokens, close+1, end)
	if !parser.token(body, "{") || parser.tokens[body].Nesting != depth+1 {
		return 0, 0, true, false
	}
	closeBody := parser.pairs[body]
	if closeBody <= body || closeBody >= end {
		return 0, 0, true, false
	}
	return parser.tokens[closeBody].EndOffset, closeBody + 1, true, true
}

// kotlinAnnotatedFunctionAt accepts only declaration modifiers before fun.
// A class annotation or file annotation must never become a method prefix.
func (parser *jvmParser) kotlinAnnotatedFunctionAt(start, end int) bool {
	cursor := start
	for count := 0; cursor < end && count < 12; count++ {
		if parser.token(cursor, "fun") {
			return true
		}
		if _, ok := kotlinModifiers[strings.ToLower(parser.tokens[cursor].Text)]; !ok {
			return false
		}
		cursor = nextStructuralToken(parser.tokens, cursor+1, end)
	}
	return false
}

// kotlinAnnotatedPropertyAt accepts only property modifiers before val/var.
// It must not scan into a value class constructor and misattach class annotations.
func (parser *jvmParser) kotlinAnnotatedPropertyAt(start, end int) bool {
	cursor := start
	for count := 0; cursor < end && count < 12; count++ {
		if parser.token(cursor, "val") || parser.token(cursor, "var") {
			return true
		}
		if _, ok := kotlinModifiers[strings.ToLower(parser.tokens[cursor].Text)]; !ok {
			return false
		}
		cursor = nextStructuralToken(parser.tokens, cursor+1, end)
	}
	return false
}

func (parser *jvmParser) kotlinPropertyWithModifiers(start, end int) bool {
	for index := start; index < end && index < start+12; index++ {
		if parser.token(index, "val") || parser.token(index, "var") {
			return true
		}
		if parser.tokens[index].Kind == TokenNewline || parser.tokens[index].Text == ";" || parser.tokens[index].Text == "{" {
			return false
		}
	}
	return false
}

func (parser *jvmParser) parseKotlinProperty(start, end int, parent *SymbolParent) int {
	marker := start
	for marker < end && !parser.token(marker, "val") && !parser.token(marker, "var") {
		marker++
	}
	if marker >= end {
		return start + 1
	}
	nameIndex := nextIdentifierToken(parser.tokens, marker+1, end)
	if nameIndex < 0 {
		return marker + 1
	}
	depth := parser.tokens[marker].Nesting
	// Extension properties name the identifier after the receiver's final dot.
	// Stop at the type or initializer so dots inside values are not declarations.
	for index := marker + 1; index < end; index++ {
		token := parser.tokens[index]
		if token.Nesting == depth && (token.Kind == TokenNewline || token.Text == ":" || token.Text == "=" || token.Text == "by") {
			break
		}
		if token.Nesting == depth && token.Text == "." {
			if next := nextStructuralToken(parser.tokens, index+1, end); next < end && parser.tokens[next].Kind == TokenIdentifier {
				nameIndex = next
			}
		}
	}
	terminator := end
	for index := nameIndex + 1; index < end; index++ {
		if parser.tokens[index].Kind == TokenNewline && parser.tokens[index].Nesting == depth {
			terminator = index
			break
		}
		if parser.tokens[index].Text == ";" && parser.tokens[index].Nesting == depth {
			terminator = index
			break
		}
	}
	last := previousStructuralToken(parser.tokens, terminator-1, nameIndex)
	if last < nameIndex {
		last = nameIndex
	}
	declarationEnd := parser.tokens[last].EndOffset
	next := terminator + 1
	if terminator < end && parser.tokens[terminator].Kind == TokenNewline {
		var seenGet, seenSet bool
		for count := 0; count < 2; count++ {
			following := nextStructuralToken(parser.tokens, next, end)
			if following >= end || parser.tokens[following].Nesting != depth ||
				kotlinSourceIndent(parser.document.Text, parser.tokens[following].StartOffset) <=
					kotlinSourceIndent(parser.document.Text, parser.tokens[start].StartOffset) {
				break
			}
			var accessorEnd, after int
			var recognized, complete bool
			switch {
			case parser.token(following, "get"):
				if seenGet {
					parser.builder.MarkIncomplete()
					break
				}
				seenGet = true
				accessorEnd, after, recognized, complete = parser.kotlinPropertyGetter(start, next, end, depth)
			case parser.token(following, "set") && parser.token(marker, "var"):
				if seenSet {
					parser.builder.MarkIncomplete()
					break
				}
				seenSet = true
				accessorEnd, after, recognized, complete = parser.kotlinPropertySetter(start, next, end, depth)
			default:
				break
			}
			if !recognized {
				break
			}
			if !complete {
				parser.builder.MarkIncomplete()
				break
			}
			declarationEnd, next = accessorEnd, after
		}
		following := nextStructuralToken(parser.tokens, next, end)
		if following < end && parser.tokens[following].Nesting == depth &&
			(parser.token(following, "get") || parser.token(following, "set")) &&
			kotlinSourceIndent(parser.document.Text, parser.tokens[following].StartOffset) >
				kotlinSourceIndent(parser.document.Text, parser.tokens[start].StartOffset) {
			parser.builder.MarkIncomplete()
		}
	}
	kind := SymbolKindProperty
	nativeKind := parser.tokens[marker].Text
	modifiers := collectKnownModifiers(parser.tokens, start, marker, kotlinModifiers)
	if containsLower(modifiers, "const") {
		kind = SymbolKindConstant
		nativeKind = "const-val"
	}
	parser.add(SymbolSpec{
		Kind: kind, NativeKind: nativeKind, Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: declarationEnd},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset},
		Visibility:  visibilityFromModifiers(modifiers), Modifiers: modifiers, Evidence: SymbolEvidenceStructural,
	})
	return min(next, end)
}

func (parser *jvmParser) parseKotlinSecondaryConstructor(start, end int, parent *SymbolParent, owner string) int {
	paren := -1
	for index := start + 1; index < end; index++ {
		if parser.tokens[index].Text == "(" {
			paren = index
			break
		}
	}
	if paren < 0 {
		return start + 1
	}
	closeParen := parser.pairs[paren]
	if closeParen <= paren {
		parser.builder.MarkIncomplete()
		return paren + 1
	}
	depth := parser.tokens[start].Nesting
	bodyOpen, lineEnd := -1, -1
	for index := closeParen + 1; index < end; index++ {
		if parser.tokens[index].Text == "{" && parser.tokens[index].Nesting == depth+1 {
			bodyOpen = index
			break
		}
		if parser.tokens[index].Kind == TokenNewline && parser.tokens[index].Nesting == depth {
			lineEnd = index
			break
		}
	}
	declarationEnd := parser.tokens[closeParen].EndOffset
	next := closeParen + 1
	var body *OffsetRange
	if bodyOpen >= 0 {
		close := parser.pairs[bodyOpen]
		if close <= bodyOpen || close >= end {
			parser.builder.MarkIncomplete()
			return end
		}
		declarationEnd = parser.tokens[close].EndOffset
		value := OffsetRange{Start: parser.tokens[bodyOpen].StartOffset, End: parser.tokens[close].EndOffset}
		body = &value
		next = close + 1
	} else if lineEnd >= 0 {
		previous := previousStructuralToken(parser.tokens, lineEnd-1, closeParen)
		if previous >= closeParen {
			declarationEnd = parser.tokens[previous].EndOffset
		}
		next = lineEnd + 1
	}
	nameStart := parser.tokens[start].StartOffset
	nameEnd := parser.tokens[start].EndOffset
	parser.add(SymbolSpec{
		Kind: SymbolKindConstructor, NativeKind: "secondary-constructor", Name: owner, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: declarationEnd}, NameRange: OffsetRange{Start: nameStart, End: nameEnd},
		Signature: &OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[bodyStartOrTerminator(bodyOpen, max(lineEnd, closeParen))].StartOffset}, Body: body,
		Evidence: SymbolEvidenceStructural, Disambiguator: "secondary:" + tokenRangeText(parser.tokens, paren, closeParen+1),
	})
	return next
}

func (parser *jvmParser) parseKotlinTypeAlias(start, end int, parent *SymbolParent) int {
	nameIndex := nextIdentifierToken(parser.tokens, start+1, end)
	if nameIndex < 0 {
		return start + 1
	}
	terminator := parser.statementTerminator(nameIndex+1, end)
	if terminator < 0 {
		terminator = end - 1
	}
	last := previousStructuralToken(parser.tokens, terminator-1, nameIndex)
	if last < nameIndex {
		last = nameIndex
	}
	parser.add(SymbolSpec{
		Kind: SymbolKindAlias, NativeKind: "typealias", Name: parser.tokens[nameIndex].Text, Parent: parent,
		Declaration: OffsetRange{Start: parser.tokens[start].StartOffset, End: parser.tokens[last].EndOffset},
		NameRange:   OffsetRange{Start: parser.tokens[nameIndex].StartOffset, End: parser.tokens[nameIndex].EndOffset}, Evidence: SymbolEvidenceStructural,
	})
	return min(terminator+1, end)
}

func (parser *jvmParser) add(spec SymbolSpec) (NormalizedSymbol, bool) {
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
