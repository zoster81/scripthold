package sourceintelligence

import (
	"context"
	"strings"
)

type CommonLispAnalyzer struct{}
type ClojureAnalyzer struct{}
type EmacsLispAnalyzer struct{}

func (CommonLispAnalyzer) ID() AnalyzerID   { return AnalyzerCommonLisp }
func (CommonLispAnalyzer) Language() string { return "common-lisp" }
func (ClojureAnalyzer) ID() AnalyzerID      { return AnalyzerClojure }
func (ClojureAnalyzer) Language() string    { return "clojure" }
func (EmacsLispAnalyzer) ID() AnalyzerID    { return AnalyzerEmacsLisp }
func (EmacsLispAnalyzer) Language() string  { return "emacs-lisp" }

func (CommonLispAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase9Lisp(ctx, document, options, "common-lisp", AnalyzerCommonLisp, CommonLispScannerProfile())
}

func (ClojureAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase9Lisp(ctx, document, options, "clojure", AnalyzerClojure, ClojureScannerProfile())
}

func (EmacsLispAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzePhase9Lisp(ctx, document, options, "emacs-lisp", AnalyzerEmacsLisp, EmacsLispScannerProfile())
}

func analyzePhase9Lisp(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID, profile ScannerProfile) (AnalyzerResult, error) {
	builder, err := newPhase9Builder(ctx, document, options, language, analyzer)
	if err != nil {
		return AnalyzerResult{}, err
	}
	scan, err := ScanSource(ctx, document, profile, ScannerLimits{MaxTokens: scannerTokenBudget(document.Text), MaxTokenBytes: 1024 * 1024, MaxNesting: max(2048, options.MaxNesting)})
	if err != nil {
		return AnalyzerResult{}, err
	}
	phase9ApplyScanDiagnostics(builder, scan, language)
	pairs := PairDelimiterTokens(scan.Tokens, profile.Delimiters)
	dependencies := []StructuralDependency{}
	var currentParent *SymbolParent
	packages := make(map[string]SymbolParent)
	for open := 0; open < len(scan.Tokens); open++ {
		if scan.Tokens[open].Text != "(" || scan.Tokens[open].Nesting != 1 {
			continue
		}
		close := pairs[open]
		if close <= open || close >= len(scan.Tokens) {
			builder.MarkIncomplete()
			continue
		}
		if phase9LispFormSuppressed(scan.Tokens, open, language) {
			open = close
			continue
		}
		form := phase9NextFormToken(scan.Tokens, open+1, close)
		if form < 0 {
			continue
		}
		head := strings.ToLower(scan.Tokens[form].Text)
		switch language {
		case "common-lisp":
			phase9ParseCommonLispForm(document, builder, scan.Tokens, form, close, head, &currentParent, packages, &dependencies)
		case "clojure":
			phase9ParseClojureForm(document, builder, scan.Tokens, form, close, head, &currentParent, &dependencies)
		case "emacs-lisp":
			phase9ParseEmacsLispForm(document, builder, scan.Tokens, form, close, head, currentParent, &dependencies)
		}
		open = close
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func phase9LispFormSuppressed(tokens []Token, open int, language string) bool {
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

func phase9NextFormToken(tokens []Token, start, end int) int {
	for index := start; index < end; index++ {
		if tokens[index].Kind == TokenNewline || tokens[index].Kind == TokenEOF {
			continue
		}
		return index
	}
	return -1
}

func phase9NextAtomToken(tokens []Token, start, end int) int {
	for index := start; index < end; index++ {
		if tokens[index].Kind == TokenIdentifier || tokens[index].Kind == TokenKeyword || tokens[index].Kind == TokenString {
			return index
		}
	}
	return -1
}

func phase9ParseCommonLispForm(document *SourceDocument, builder *SymbolBuilder, tokens []Token, form, close int, head string, currentParent **SymbolParent, packages map[string]SymbolParent, dependencies *[]StructuralDependency) {
	if head == "require" {
		phase9LispRequireDependency(document, tokens, form+1, close, dependencies)
		return
	}
	if head == "in-package" {
		nameIndex := phase9NextAtomToken(tokens, form+1, close)
		if nameIndex < 0 {
			return
		}
		name := phase9CleanAtom(tokens[nameIndex].Text)
		if parent, ok := packages[strings.ToLower(name)]; ok {
			value := parent
			*currentParent = &value
		}
		return
	}
	nameIndex := phase9NextAtomToken(tokens, form+1, close)
	if nameIndex < 0 {
		return
	}
	name, nameRange := phase9TokenName(tokens[nameIndex])
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
	case "defclass":
		kind = SymbolKindClass
	case "defstruct":
		kind = SymbolKindStruct
	case "defun", "defmacro":
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
	symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: kind, NativeKind: native, Name: name, Parent: parent,
		Declaration: declaration, NameRange: nameRange, Signature: &signature, Evidence: SymbolEvidenceStructural})
	if ok && head == "defpackage" {
		value := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		packages[strings.ToLower(name)] = value
	}
}

func phase9ParseClojureForm(document *SourceDocument, builder *SymbolBuilder, tokens []Token, form, close int, head string, currentParent **SymbolParent, dependencies *[]StructuralDependency) {
	if head == "require" {
		phase9LispRequireDependency(document, tokens, form+1, close, dependencies)
		return
	}
	nameIndex := phase9NextAtomToken(tokens, form+1, close)
	if nameIndex < 0 {
		return
	}
	name, nameRange := phase9TokenName(tokens[nameIndex])
	if name == "" {
		return
	}
	declaration := OffsetRange{Start: tokens[form-1].StartOffset, End: tokens[close].EndOffset}
	signature := OffsetRange{Start: declaration.Start, End: tokens[nameIndex].EndOffset}
	kind := SymbolKind("")
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
	symbol, ok := phase9AddSymbol(builder, SymbolSpec{Kind: kind, NativeKind: head, Name: name, Parent: parent,
		Declaration: declaration, NameRange: nameRange, Signature: &signature, Evidence: SymbolEvidenceStructural})
	if ok && head == "ns" {
		value := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		*currentParent = &value
		phase9ClojureNSDependencies(document, tokens, form+1, close, dependencies)
	}
}

func phase9ParseEmacsLispForm(document *SourceDocument, builder *SymbolBuilder, tokens []Token, form, close int, head string, parent *SymbolParent, dependencies *[]StructuralDependency) {
	if head == "require" {
		phase9LispRequireDependency(document, tokens, form+1, close, dependencies)
		return
	}
	nameIndex := phase9NextAtomToken(tokens, form+1, close)
	if nameIndex < 0 {
		return
	}
	name, nameRange := phase9TokenName(tokens[nameIndex])
	if name == "" {
		return
	}
	kind := SymbolKind("")
	switch head {
	case "defclass":
		kind = SymbolKindClass
	case "defun", "defmacro":
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
	phase9AddSymbol(builder, SymbolSpec{Kind: kind, NativeKind: head, Name: name, Parent: parent,
		Declaration: declaration, NameRange: nameRange, Signature: &signature, Evidence: SymbolEvidenceStructural})
}

func phase9LispRequireDependency(document *SourceDocument, tokens []Token, start, end int, dependencies *[]StructuralDependency) {
	nameIndex := phase9NextAtomToken(tokens, start, end)
	if nameIndex < 0 {
		return
	}
	value := ""
	if tokens[nameIndex].Kind == TokenString {
		value = phase9StringTokenValue(tokens[nameIndex])
	} else {
		value = phase9CleanAtom(tokens[nameIndex].Text)
	}
	if value != "" {
		phase9AddDependency(document, dependencies, StructuralDependencyImport, value, tokens[nameIndex].StartOffset, tokens[nameIndex].EndOffset)
	}
}

func phase9ClojureNSDependencies(document *SourceDocument, tokens []Token, start, end int, dependencies *[]StructuralDependency) {
	pairs := PairDelimiterTokens(tokens, nil)
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
			nameIndex := phase9NextAtomToken(tokens, cursor+1, close)
			if nameIndex >= 0 && tokens[nameIndex].Kind == TokenIdentifier && !strings.HasPrefix(tokens[nameIndex].Text, ":") {
				phase9AddDependency(document, dependencies, StructuralDependencyImport, tokens[nameIndex].Text, tokens[nameIndex].StartOffset, tokens[nameIndex].EndOffset)
			}
			cursor = close
		}
		return
	}
}
