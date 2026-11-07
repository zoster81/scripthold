package sourceintelligence

import (
	"context"
	"strings"
	"unicode/utf8"
)

type CommonLispAnalyzer struct{}
type ClojureAnalyzer struct{}
type EmacsLispAnalyzer struct{}

var (
	commonLispAnalyzerScannerProfile = CommonLispScannerProfile()
	clojureAnalyzerScannerProfile    = ClojureScannerProfile()
	emacsLispAnalyzerScannerProfile  = EmacsLispScannerProfile()
)

func (CommonLispAnalyzer) ID() AnalyzerID   { return AnalyzerCommonLisp }
func (CommonLispAnalyzer) Language() string { return "common-lisp" }
func (ClojureAnalyzer) ID() AnalyzerID      { return AnalyzerClojure }
func (ClojureAnalyzer) Language() string    { return "clojure" }
func (EmacsLispAnalyzer) ID() AnalyzerID    { return AnalyzerEmacsLisp }
func (EmacsLispAnalyzer) Language() string  { return "emacs-lisp" }

func (CommonLispAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeLispSource(ctx, document, options, "common-lisp", AnalyzerCommonLisp, commonLispAnalyzerScannerProfile)
}

func (ClojureAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeLispSource(ctx, document, options, "clojure", AnalyzerClojure, clojureAnalyzerScannerProfile)
}

func (EmacsLispAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeLispSource(ctx, document, options, "emacs-lisp", AnalyzerEmacsLisp, emacsLispAnalyzerScannerProfile)
}

func analyzeLispSource(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID, profile ScannerProfile) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, language, analyzer)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scanDocument := document
	if language == "common-lisp" || language == "clojure" || language == "emacs-lisp" {
		clone := *document
		clone.Text = maskLispReaderCharacters(document.Text, language)
		scanDocument = &clone
	}
	scan, err := ScanSource(ctx, scanDocument, profile, ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: max(2048, options.MaxNesting)})
	if err != nil {
		return AnalyzerResult{}, err
	}
	applyStructuralScanDiagnostics(builder, scan, language)
	pairs := pairOpeningDelimiterTokens(scan.Tokens, profile.Delimiters)
	dependencies := []StructuralDependency{}
	var currentParent *SymbolParent
	packages := make(map[string]SymbolParent)
	fiveAMPackages := make(map[string]bool)
	fiveAMActive, fiveAMSuite := false, false
	for open := 0; open < len(scan.Tokens); open++ {
		if scan.Tokens[open].Text != "(" || scan.Tokens[open].Nesting != 1 {
			continue
		}
		close := pairs[open]
		if close <= open || close >= len(scan.Tokens) {
			builder.MarkIncomplete()
			continue
		}
		if lispFormSuppressed(scan.Tokens, open, language) {
			open = close
			continue
		}
		form := nextLispFormToken(scan.Tokens, open+1, close)
		if form < 0 {
			continue
		}
		head := strings.ToLower(scan.Tokens[form].Text)
		switch language {
		case "common-lisp":
			switch head {
			case "defpackage":
				name := commonLispFormName(scan.Tokens, form+1, close)
				if name != "" {
					fiveAMPackages[name] = commonLispDefpackageUsesFiveAM(scan.Tokens, pairs, form, close)
				}
			case "in-package":
				fiveAMActive = fiveAMPackages[commonLispFormName(scan.Tokens, form+1, close)]
				fiveAMSuite = false
			case "in-suite":
				fiveAMSuite = fiveAMActive && commonLispFormName(scan.Tokens, form+1, close) != ""
			}
			parseCommonLispForm(document, builder, scan.Tokens, form, close, head, fiveAMActive, fiveAMSuite, &currentParent, packages, &dependencies)
		case "clojure":
			parseClojureForm(document, builder, scan.Tokens, pairs, form, close, head, &currentParent, &dependencies)
		case "emacs-lisp":
			parseEmacsLispForm(document, builder, scan.Tokens, form, close, head, currentParent, &dependencies)
		}
		open = close
	}
	return AnalyzerResult{Analysis: builder.takeResult(), Dependencies: dependencies}, nil
}

func maskLispReaderCharacters(text, language string) string {
	var masked []byte
	inString := false
	escaped := false
	for at := 0; at < len(text); {
		if inString {
			if escaped {
				escaped = false
				at++
				continue
			}
			if text[at] == '\\' {
				escaped = true
				at++
				continue
			}
			if text[at] == '"' {
				inString = false
			}
			at++
			continue
		}
		if text[at] == ';' {
			for at < len(text) && text[at] != '\r' && text[at] != '\n' {
				at++
			}
			continue
		}
		if text[at] == '"' {
			inString = true
			at++
			continue
		}
		end := at
		switch language {
		case "common-lisp":
			if strings.HasPrefix(text[at:], "#\\") {
				end = commonLispCharacterEnd(text, at)
			}
		case "clojure":
			if text[at] == '\\' {
				end = clojureCharacterEnd(text, at)
			}
		case "emacs-lisp":
			if text[at] == '?' {
				end = emacsLispCharacterEnd(text, at)
			}
		}
		if end > at {
			if masked == nil {
				masked = []byte(text)
			}
			maskRangePreservingLines(masked, at, end)
			at = end
			continue
		}
		_, size := utf8.DecodeRuneInString(text[at:])
		if size <= 0 {
			size = 1
		}
		at += size
	}
	if masked == nil {
		return text
	}
	return string(masked)
}

func commonLispCharacterEnd(text string, at int) int {
	cursor := at + 2
	if at < 0 || cursor > len(text) || !strings.HasPrefix(text[at:], "#\\") {
		return at
	}
	if cursor >= len(text) {
		return cursor
	}
	if lispReaderCharacterNameByte(text[cursor]) {
		for cursor < len(text) && lispReaderCharacterNameByte(text[cursor]) {
			cursor++
		}
		return cursor
	}
	_, size := utf8.DecodeRuneInString(text[cursor:])
	if size <= 0 {
		size = 1
	}
	return min(len(text), cursor+size)
}

func clojureCharacterEnd(text string, at int) int {
	cursor := at + 1
	if cursor >= len(text) {
		return cursor
	}
	if lispReaderCharacterNameByte(text[cursor]) {
		for cursor < len(text) && lispReaderCharacterNameByte(text[cursor]) {
			cursor++
		}
		return cursor
	}
	_, size := utf8.DecodeRuneInString(text[cursor:])
	if size <= 0 {
		size = 1
	}
	return min(len(text), cursor+size)
}

func emacsLispCharacterEnd(text string, at int) int {
	cursor := at + 1
	if cursor >= len(text) {
		return cursor
	}
	if text[cursor] != '\\' {
		_, size := utf8.DecodeRuneInString(text[cursor:])
		if size <= 0 {
			size = 1
		}
		return min(len(text), cursor+size)
	}
	cursor++
	for cursor+2 < len(text) && (text[cursor] == 'C' || text[cursor] == 'M' || text[cursor] == 'S' || text[cursor] == 'H' || text[cursor] == 'A' || text[cursor] == 's') && text[cursor+1] == '-' {
		cursor += 2
		if cursor < len(text) && text[cursor] == '\\' {
			cursor++
		}
	}
	if cursor >= len(text) {
		return cursor
	}
	_, size := utf8.DecodeRuneInString(text[cursor:])
	if size <= 0 {
		size = 1
	}
	return min(len(text), cursor+size)
}

func lispReaderCharacterNameByte(value byte) bool {
	return value == '_' || value == '-' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func lispFormSuppressed(tokens []Token, open int, language string) bool {
	previous := open - 1
	for previous >= 0 && tokens[previous].Kind == TokenNewline {
		previous--
	}
	if previous < 0 {
		return false
	}
	if tokens[previous].Text == "'" || tokens[previous].Text == "`" {
		return true
	}
	if language == "clojure" && tokens[previous].Text == "_" {
		before := previous - 1
		for before >= 0 && tokens[before].Kind == TokenNewline {
			before--
		}
		return before >= 0 && tokens[before].Text == "#"
	}
	return false
}

func nextLispFormToken(tokens []Token, start, end int) int {
	for index := start; index < end; index++ {
		if tokens[index].Kind == TokenNewline || tokens[index].Kind == TokenEOF {
			continue
		}
		return index
	}
	return -1
}

func nextLispAtomToken(tokens []Token, start, end int) int {
	for index := start; index < end; index++ {
		if tokens[index].Kind == TokenIdentifier || tokens[index].Kind == TokenKeyword || tokens[index].Kind == TokenString {
			return index
		}
	}
	return -1
}

func commonLispFormName(tokens []Token, start, close int) string {
	index := nextLispFormToken(tokens, start, close)
	if index < 0 || (tokens[index].Kind != TokenIdentifier && tokens[index].Kind != TokenKeyword) {
		return ""
	}
	return strings.ToLower(cleanLispAtom(tokens[index].Text))
}

// Require an explicit direct (:use :fiveam) package clause. A similarly named
// macro in another package is not evidence of a FiveAM test definition.
func commonLispDefpackageUsesFiveAM(tokens []Token, pairs map[int]int, form, close int) bool {
	for index := form + 1; index < close; index++ {
		if tokens[index].Text != "(" || tokens[index].Nesting != 2 {
			continue
		}
		end := pairs[index]
		if end <= index || end >= close {
			continue
		}
		clause := nextLispFormToken(tokens, index+1, end)
		if clause >= 0 && strings.EqualFold(cleanLispAtom(tokens[clause].Text), "use") {
			for member := clause + 1; member < end; member++ {
				if (tokens[member].Kind == TokenIdentifier || tokens[member].Kind == TokenKeyword) &&
					strings.EqualFold(cleanLispAtom(tokens[member].Text), "fiveam") {
					return true
				}
			}
		}
		index = end
	}
	return false
}

func parseCommonLispForm(document *SourceDocument, builder *SymbolBuilder, tokens []Token, form, close int, head string, fiveAMActive, fiveAMSuite bool, currentParent **SymbolParent, packages map[string]SymbolParent, dependencies *[]StructuralDependency) {
	if head == "require" {
		addLispRequireDependency(document, tokens, form+1, close, dependencies)
		return
	}
	if head == "in-package" {
		nameIndex := nextLispAtomToken(tokens, form+1, close)
		if nameIndex < 0 {
			return
		}
		name := cleanLispAtom(tokens[nameIndex].Text)
		if parent, ok := packages[strings.ToLower(name)]; ok {
			value := parent
			*currentParent = &value
		}
		return
	}
	nameIndex := nextLispAtomToken(tokens, form+1, close)
	if nameIndex < 0 {
		return
	}
	name, nameRange := lispTokenName(tokens[nameIndex])
	if name == "" {
		return
	}
	declaration := OffsetRange{Start: tokens[form-1].StartOffset, End: tokens[close].EndOffset}
	signature := OffsetRange{Start: declaration.Start, End: tokens[min(close, nameIndex+1)].EndOffset}
	kind := SymbolKind("")
	native := head
	switch head {
	case "defpackage":
		kind = SymbolKindPackage
	case "defclass", "define-condition":
		kind = SymbolKindClass
	case "defstruct":
		kind = SymbolKindStruct
	case "defun", "defmacro":
		kind = SymbolKindFunction
	case "def-suite":
		if !fiveAMActive || nameIndex != nextLispFormToken(tokens, form+1, close) ||
			tokens[nameIndex].Kind != TokenIdentifier || strings.HasPrefix(tokens[nameIndex].Text, ":") {
			return
		}
		kind = SymbolKindSection
	case "test":
		if !fiveAMSuite || nameIndex != nextLispFormToken(tokens, form+1, close) ||
			tokens[nameIndex].Kind != TokenIdentifier || strings.HasPrefix(tokens[nameIndex].Text, ":") {
			return
		}
		kind = SymbolKindFunction
	case "defconstant":
		kind = SymbolKindConstant
	case "defparameter", "defvar":
		kind = SymbolKindVariable
	}
	if kind == "" {
		return
	}
	parent := *currentParent
	if head == "defpackage" {
		parent = nil
	}
	symbol, ok := addStructuralSymbol(builder, SymbolSpec{Kind: kind, NativeKind: native, Name: name, Parent: parent,
		Declaration: declaration, NameRange: nameRange, Signature: &signature, Evidence: SymbolEvidenceStructural})
	if ok && head == "defpackage" {
		value := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		packages[strings.ToLower(name)] = value
	}
}

func parseClojureForm(document *SourceDocument, builder *SymbolBuilder, tokens []Token, pairs map[int]int, form, close int, head string, currentParent **SymbolParent, dependencies *[]StructuralDependency) {
	if head == "require" {
		addLispRequireDependency(document, tokens, form+1, close, dependencies)
		return
	}
	nameIndex := clojureDeclarationNameToken(tokens, pairs, form+1, close)
	if nameIndex < 0 {
		return
	}
	name, nameRange := lispTokenName(tokens[nameIndex])
	if name == "" {
		return
	}
	declaration := OffsetRange{Start: tokens[form-1].StartOffset, End: tokens[close].EndOffset}
	signature := OffsetRange{Start: declaration.Start, End: tokens[nameIndex].EndOffset}
	kind := SymbolKind("")
	native := head
	switch head {
	case "ns":
		kind = SymbolKindNamespace
	case "defrecord":
		kind = SymbolKindRecord
	case "deftype":
		kind = SymbolKindType
	case "defprotocol":
		kind = SymbolKindInterface
	case "defn", "defn-", "defmacro":
		kind = SymbolKindFunction
	case "deftest", "clojure.test/deftest":
		if tokens[form].Text != head || tokens[nameIndex].Kind != TokenIdentifier {
			return
		}
		kind = SymbolKindFunction
		native = "deftest"
	case "def":
		kind = SymbolKindVariable
	}
	if kind == "" {
		return
	}
	parent := *currentParent
	if head == "ns" {
		parent = nil
	}
	symbol, ok := addStructuralSymbol(builder, SymbolSpec{Kind: kind, NativeKind: native, Name: name, Parent: parent,
		Declaration: declaration, NameRange: nameRange, Signature: &signature, Evidence: SymbolEvidenceStructural})
	if ok && head == "ns" {
		value := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		*currentParent = &value
		addClojureNamespaceDependencies(document, tokens, form+1, close, dependencies)
	}
}

// Metadata preceding a Clojure namespace or definition name belongs to the
// reader form, not to the declared symbol.
func clojureDeclarationNameToken(tokens []Token, pairs map[int]int, start, close int) int {
	cursor := start
	for cursor < close {
		if tokens[cursor].Kind == TokenNewline {
			cursor++
			continue
		}
		if tokens[cursor].Text != "^" {
			return nextLispAtomToken(tokens, cursor, close)
		}
		cursor++
		for cursor < close && tokens[cursor].Kind == TokenNewline {
			cursor++
		}
		if cursor >= close {
			return -1
		}
		switch tokens[cursor].Text {
		case "{", "[", "(":
			end, ok := pairs[cursor]
			if !ok || end >= close {
				return -1
			}
			cursor = end + 1
		default:
			cursor++
		}
	}
	return -1
}

func parseEmacsLispForm(document *SourceDocument, builder *SymbolBuilder, tokens []Token, form, close int, head string, parent *SymbolParent, dependencies *[]StructuralDependency) {
	if head == "require" {
		addLispRequireDependency(document, tokens, form+1, close, dependencies)
		return
	}
	nameIndex := nextLispAtomToken(tokens, form+1, close)
	if nameIndex < 0 {
		return
	}
	name, nameRange := lispTokenName(tokens[nameIndex])
	if name == "" {
		return
	}
	kind := SymbolKind("")
	switch head {
	case "defclass":
		kind = SymbolKindClass
	case "defun", "defmacro":
		kind = SymbolKindFunction
	case "ert-deftest":
		if tokens[form].Text != "ert-deftest" ||
			nameIndex != nextLispFormToken(tokens, form+1, close) ||
			tokens[nameIndex].Kind != TokenIdentifier ||
			strings.HasPrefix(name, ":") {
			return
		}
		kind = SymbolKindFunction
	case "defconst":
		kind = SymbolKindConstant
	case "defcustom", "defvar":
		kind = SymbolKindVariable
	}
	if kind == "" {
		return
	}
	declaration := OffsetRange{Start: tokens[form-1].StartOffset, End: tokens[close].EndOffset}
	signature := OffsetRange{Start: declaration.Start, End: tokens[nameIndex].EndOffset}
	addStructuralSymbol(builder, SymbolSpec{Kind: kind, NativeKind: head, Name: name, Parent: parent,
		Declaration: declaration, NameRange: nameRange, Signature: &signature, Evidence: SymbolEvidenceStructural})
}

func addLispRequireDependency(document *SourceDocument, tokens []Token, start, end int, dependencies *[]StructuralDependency) {
	nameIndex := nextLispAtomToken(tokens, start, end)
	if nameIndex < 0 {
		return
	}
	value := ""
	if tokens[nameIndex].Kind == TokenString {
		value = quotedTokenValue(tokens[nameIndex])
	} else {
		value = cleanLispAtom(tokens[nameIndex].Text)
	}
	if value != "" {
		addStructuralDependency(document, dependencies, StructuralDependencyImport, value, tokens[nameIndex].StartOffset, tokens[nameIndex].EndOffset)
	}
}

func addClojureNamespaceDependencies(document *SourceDocument, tokens []Token, start, end int, dependencies *[]StructuralDependency) {
	pairs := pairOpeningDelimiterTokens(tokens, nil)
	for index := start; index < end; index++ {
		if !strings.EqualFold(tokens[index].Text, ":require") {
			continue
		}
		requireDepth := tokens[index].Nesting
		for cursor := index + 1; cursor < end; cursor++ {
			if strings.HasPrefix(tokens[cursor].Text, ":") && tokens[cursor].Nesting <= requireDepth {
				break
			}
			if tokens[cursor].Text != "[" || tokens[cursor].Nesting <= requireDepth {
				continue
			}
			close := pairs[cursor]
			if close <= cursor || close > end {
				continue
			}
			nameIndex := nextLispAtomToken(tokens, cursor+1, close)
			if nameIndex >= 0 && tokens[nameIndex].Kind == TokenIdentifier && !strings.HasPrefix(tokens[nameIndex].Text, ":") {
				addStructuralDependency(document, dependencies, StructuralDependencyImport, tokens[nameIndex].Text, tokens[nameIndex].StartOffset, tokens[nameIndex].EndOffset)
			}
			cursor = close
		}
		return
	}
}
