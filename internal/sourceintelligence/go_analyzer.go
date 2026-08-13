package sourceintelligence

import (
	"context"
	"go/ast"
	"go/parser"
	goscanner "go/scanner"
	"go/token"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/operation"
)

// GoAnalyzer extracts syntax-level Go declarations with only the Go standard library.
type GoAnalyzer struct{}

func (GoAnalyzer) ID() AnalyzerID   { return AnalyzerGo }
func (GoAnalyzer) Language() string { return "go" }

func (GoAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return AnalyzerResult{}, operation.New(operation.KindInvalidInput, "source document is required")
	}
	if err := ctx.Err(); err != nil {
		return AnalyzerResult{}, operation.Wrap(operation.KindCancelled, "analyze_go_source", document.Path, err)
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: "go", Analyzer: string(AnalyzerGo), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return AnalyzerResult{}, err
	}
	fileSet := token.NewFileSet()
	file, parseErr := parser.ParseFile(fileSet, document.Path, document.Text, parser.AllErrors|parser.ParseComments)
	analysis := &goDocumentAnalysis{
		ctx: ctx, document: document, fileSet: fileSet, file: file, builder: builder,
		typeParents: make(map[string]SymbolParent), dependencyCap: options.Limits.MaxSymbols,
	}
	analysis.addParseDiagnostics(parseErr)
	if err := analysis.checkContext(); err != nil {
		return AnalyzerResult{}, err
	}
	if file == nil {
		return analysis.result(), nil
	}
	analysis.emitPackage()
	analysis.emitDependencies()
	analysis.emitTypes()
	if !analysis.symbolsStopped {
		analysis.emitValuesAndFunctions()
	}
	if err := analysis.checkContext(); err != nil {
		return AnalyzerResult{}, err
	}
	return analysis.result(), nil
}

type goDocumentAnalysis struct {
	ctx             context.Context
	document        *SourceDocument
	fileSet         *token.FileSet
	file            *ast.File
	builder         *SymbolBuilder
	packageParent   SymbolParent
	typeParents     map[string]SymbolParent
	dependencies    []StructuralDependency
	dependencyCap   int
	dependencyLimit bool
	symbolsStopped  bool
}

func (analysis *goDocumentAnalysis) result() AnalyzerResult {
	return AnalyzerResult{Analysis: analysis.builder.Result(), Dependencies: append([]StructuralDependency(nil), analysis.dependencies...)}
}

func (analysis *goDocumentAnalysis) checkContext() error {
	if err := analysis.ctx.Err(); err != nil {
		return operation.Wrap(operation.KindCancelled, "analyze_go_source", analysis.document.Path, err)
	}
	return nil
}

func (analysis *goDocumentAnalysis) emitPackage() {
	if analysis.file.Name == nil || !analysis.file.Package.IsValid() {
		analysis.builder.MarkIncomplete()
		_ = analysis.builder.AddDiagnostic(DiagnosticSpec{Code: "go-package", Message: "parsed Go file has no valid package declaration", Severity: DiagnosticError, AffectsCoverage: true})
		return
	}
	declaration, ok := analysis.offsetRange(analysis.file.Package, analysis.file.Name.End())
	if !ok {
		analysis.recordInvalidASTRange("package declaration", nil)
		return
	}
	nameRange, ok := analysis.nodeRange(analysis.file.Name)
	if !ok {
		analysis.recordInvalidASTRange("package name", &declaration)
		return
	}
	symbol, added := analysis.add(SymbolSpec{Kind: SymbolKindPackage, NativeKind: "package", Name: analysis.file.Name.Name, Declaration: declaration, NameRange: nameRange, Signature: &declaration, Visibility: VisibilityPackage, Evidence: SymbolEvidenceStructural})
	if added {
		analysis.packageParent = SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
	}
}

func (analysis *goDocumentAnalysis) emitDependencies() {
	for _, spec := range analysis.file.Imports {
		if analysis.checkContext() != nil {
			return
		}
		if spec == nil || spec.Path == nil {
			continue
		}
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			analysis.builder.MarkIncomplete()
			_ = analysis.builder.AddDiagnostic(DiagnosticSpec{Code: "go-import", Message: "import path string could not be decoded", Severity: DiagnosticError, Range: analysis.nodeRangePointer(spec.Path), AffectsCoverage: true})
			continue
		}
		if len(analysis.dependencies) >= analysis.dependencyCap {
			if !analysis.dependencyLimit {
				analysis.dependencyLimit = true
				analysis.builder.MarkTruncated()
				_ = analysis.builder.AddDiagnostic(DiagnosticSpec{Code: "dependency-limit", Message: "structural dependency retention limit reached", Severity: DiagnosticWarning, AffectsCoverage: true})
			}
			continue
		}
		offsets, ok := analysis.nodeRange(spec)
		if !ok {
			analysis.recordInvalidASTRange("import declaration", nil)
			continue
		}
		publicRange, err := analysis.document.RangeFromUTF8Offsets(offsets.Start, offsets.End)
		if err != nil {
			analysis.recordInvalidASTRange("import declaration", &offsets)
			continue
		}
		alias := ""
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		analysis.dependencies = append(analysis.dependencies, StructuralDependency{Kind: StructuralDependencyImport, Value: value, Alias: alias, Range: publicRange, Evidence: SymbolEvidenceStructural})
	}
}

func (analysis *goDocumentAnalysis) emitTypes() {
	for _, declaration := range analysis.file.Decls {
		if analysis.checkContext() != nil || analysis.symbolsStopped {
			return
		}
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, rawSpec := range general.Specs {
			if analysis.symbolsStopped {
				return
			}
			typeSpec, ok := rawSpec.(*ast.TypeSpec)
			if !ok || typeSpec.Name == nil || typeSpec.Name.Name == "_" {
				continue
			}
			typeSymbol, added := analysis.emitTypeSpec(general, typeSpec)
			if !added {
				continue
			}
			analysis.typeParents[typeSpec.Name.Name] = SymbolParent{ID: typeSymbol.ID, QualifiedName: typeSymbol.QualifiedName}
			analysis.emitTypeMembers(typeSpec, typeSymbol)
		}
	}
}

func (analysis *goDocumentAnalysis) emitTypeSpec(general *ast.GenDecl, spec *ast.TypeSpec) (NormalizedSymbol, bool) {
	kind := SymbolKindType
	nativeKind := "named-type"
	if spec.Assign.IsValid() {
		kind = SymbolKindAlias
		nativeKind = "alias"
	} else {
		switch spec.Type.(type) {
		case *ast.StructType:
			kind, nativeKind = SymbolKindStruct, "struct"
		case *ast.InterfaceType:
			kind, nativeKind = SymbolKindInterface, "interface"
		}
	}
	start := spec.Pos()
	if len(general.Specs) == 1 && !general.Lparen.IsValid() {
		start = general.Pos()
	}
	declaration, ok := analysis.offsetRange(start, spec.End())
	if !ok {
		analysis.recordInvalidASTRange("type declaration", nil)
		return NormalizedSymbol{}, false
	}
	nameRange, ok := analysis.nodeRange(spec.Name)
	if !ok {
		analysis.recordInvalidASTRange("type name", &declaration)
		return NormalizedSymbol{}, false
	}
	signature := declaration
	var body *OffsetRange
	switch concrete := spec.Type.(type) {
	case *ast.StructType:
		if concrete.Fields != nil {
			if rangeValue, valid := analysis.offsetRange(concrete.Fields.Opening, concrete.Fields.End()); valid {
				body = &rangeValue
			}
			if opening, valid := analysis.offset(concrete.Fields.Opening); valid {
				signature.End = trimTrailingSourceWhitespace(analysis.document.Text, signature.Start, opening)
			}
		}
	case *ast.InterfaceType:
		if concrete.Methods != nil {
			if rangeValue, valid := analysis.offsetRange(concrete.Methods.Opening, concrete.Methods.End()); valid {
				body = &rangeValue
			}
			if opening, valid := analysis.offset(concrete.Methods.Opening); valid {
				signature.End = trimTrailingSourceWhitespace(analysis.document.Text, signature.Start, opening)
			}
		}
	}
	return analysis.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: spec.Name.Name, Parent: &analysis.packageParent, Declaration: declaration, NameRange: nameRange, Signature: &signature, Body: body, Visibility: goVisibility(spec.Name.Name), Evidence: SymbolEvidenceStructural})
}

func (analysis *goDocumentAnalysis) emitTypeMembers(spec *ast.TypeSpec, owner NormalizedSymbol) {
	parent := &SymbolParent{ID: owner.ID, QualifiedName: owner.QualifiedName}
	switch concrete := spec.Type.(type) {
	case *ast.StructType:
		if concrete.Fields == nil {
			return
		}
		for _, field := range concrete.Fields.List {
			analysis.emitStructField(field, parent)
			if analysis.symbolsStopped {
				return
			}
		}
	case *ast.InterfaceType:
		if concrete.Methods == nil {
			return
		}
		for _, field := range concrete.Methods.List {
			analysis.emitInterfaceField(field, parent)
			if analysis.symbolsStopped {
				return
			}
		}
	}
}

func (analysis *goDocumentAnalysis) emitStructField(field *ast.Field, parent *SymbolParent) {
	if field == nil {
		return
	}
	declaration, ok := analysis.nodeRange(field)
	if !ok {
		analysis.recordInvalidASTRange("struct field", nil)
		return
	}
	if len(field.Names) == 0 {
		identifier := embeddedIdentifier(field.Type)
		if identifier == nil || identifier.Name == "_" {
			return
		}
		nameRange, valid := analysis.nodeRange(identifier)
		if !valid {
			analysis.recordInvalidASTRange("embedded field name", &declaration)
			return
		}
		analysis.add(SymbolSpec{Kind: SymbolKindField, NativeKind: "embedded-field", Name: identifier.Name, Parent: parent, Declaration: declaration, NameRange: nameRange, Signature: &declaration, Visibility: goVisibility(identifier.Name), Modifiers: []string{"embedded"}, Evidence: SymbolEvidenceStructural})
		return
	}
	for _, name := range field.Names {
		if name == nil || name.Name == "_" {
			continue
		}
		nameRange, valid := analysis.nodeRange(name)
		if !valid {
			analysis.recordInvalidASTRange("field name", &declaration)
			continue
		}
		analysis.add(SymbolSpec{Kind: SymbolKindField, NativeKind: "field", Name: name.Name, Parent: parent, Declaration: declaration, NameRange: nameRange, Signature: &declaration, Visibility: goVisibility(name.Name), Evidence: SymbolEvidenceStructural})
		if analysis.symbolsStopped {
			return
		}
	}
}

func (analysis *goDocumentAnalysis) emitInterfaceField(field *ast.Field, parent *SymbolParent) {
	if field == nil {
		return
	}
	declaration, ok := analysis.nodeRange(field)
	if !ok {
		analysis.recordInvalidASTRange("interface member", nil)
		return
	}
	if len(field.Names) == 0 {
		identifier := embeddedIdentifier(field.Type)
		if identifier == nil || identifier.Name == "_" {
			return
		}
		nameRange, valid := analysis.nodeRange(identifier)
		if !valid {
			analysis.recordInvalidASTRange("embedded interface name", &declaration)
			return
		}
		analysis.add(SymbolSpec{Kind: SymbolKindField, NativeKind: "embedded-interface", Name: identifier.Name, Parent: parent, Declaration: declaration, NameRange: nameRange, Signature: &declaration, Visibility: goVisibility(identifier.Name), Modifiers: []string{"embedded"}, Evidence: SymbolEvidenceStructural})
		return
	}
	for _, name := range field.Names {
		if name == nil || name.Name == "_" {
			continue
		}
		nameRange, valid := analysis.nodeRange(name)
		if !valid {
			analysis.recordInvalidASTRange("interface member name", &declaration)
			continue
		}
		kind, nativeKind := SymbolKindField, "interface-member"
		if _, ok := field.Type.(*ast.FuncType); ok {
			kind, nativeKind = SymbolKindMethod, "interface-method"
		}
		analysis.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: name.Name, Parent: parent, Declaration: declaration, NameRange: nameRange, Signature: &declaration, Visibility: goVisibility(name.Name), Evidence: SymbolEvidenceStructural})
		if analysis.symbolsStopped {
			return
		}
	}
}

func (analysis *goDocumentAnalysis) emitValuesAndFunctions() {
	for _, declaration := range analysis.file.Decls {
		if analysis.checkContext() != nil || analysis.symbolsStopped {
			return
		}
		switch concrete := declaration.(type) {
		case *ast.GenDecl:
			if concrete.Tok == token.CONST || concrete.Tok == token.VAR {
				analysis.emitValueDecl(concrete)
			}
		case *ast.FuncDecl:
			analysis.emitFuncDecl(concrete)
		}
	}
}

func (analysis *goDocumentAnalysis) emitValueDecl(declaration *ast.GenDecl) {
	kind, nativeKind := SymbolKindVariable, "var"
	if declaration.Tok == token.CONST {
		kind, nativeKind = SymbolKindConstant, "const"
	}
	for _, rawSpec := range declaration.Specs {
		valueSpec, ok := rawSpec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		declarationRange, valid := analysis.nodeRange(valueSpec)
		if !valid {
			analysis.recordInvalidASTRange(nativeKind+" declaration", nil)
			continue
		}
		for _, name := range valueSpec.Names {
			if name == nil || name.Name == "_" {
				continue
			}
			nameRange, valid := analysis.nodeRange(name)
			if !valid {
				analysis.recordInvalidASTRange(nativeKind+" name", &declarationRange)
				continue
			}
			analysis.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: name.Name, Parent: &analysis.packageParent, Declaration: declarationRange, NameRange: nameRange, Signature: &declarationRange, Visibility: goVisibility(name.Name), Evidence: SymbolEvidenceStructural})
			if analysis.symbolsStopped {
				return
			}
		}
	}
}

func (analysis *goDocumentAnalysis) emitFuncDecl(declaration *ast.FuncDecl) {
	if declaration == nil || declaration.Name == nil || declaration.Name.Name == "_" {
		return
	}
	declarationRange, ok := analysis.nodeRange(declaration)
	if !ok {
		analysis.recordInvalidASTRange("function declaration", nil)
		return
	}
	nameRange, ok := analysis.nodeRange(declaration.Name)
	if !ok {
		analysis.recordInvalidASTRange("function name", &declarationRange)
		return
	}
	signature := declarationRange
	var body *OffsetRange
	if declaration.Body != nil {
		if bodyRange, valid := analysis.nodeRange(declaration.Body); valid {
			body = &bodyRange
		}
		if opening, valid := analysis.offset(declaration.Body.Lbrace); valid {
			signature.End = trimTrailingSourceWhitespace(analysis.document.Text, signature.Start, opening)
		}
	}
	kind, nativeKind := SymbolKindFunction, "function"
	parent := analysis.packageParent
	var modifiers []string
	if declaration.Recv != nil && len(declaration.Recv.List) > 0 {
		kind, nativeKind = SymbolKindMethod, "method"
		receiverName, pointer := receiverTypeName(declaration.Recv.List[0].Type)
		if receiverName != "" {
			if known, exists := analysis.typeParents[receiverName]; exists {
				parent = known
			} else {
				qualified := receiverName
				if analysis.packageParent.QualifiedName != "" {
					qualified = analysis.packageParent.QualifiedName + "." + receiverName
				}
				parent = SymbolParent{QualifiedName: qualified}
			}
		}
		if pointer {
			modifiers = append(modifiers, "pointer-receiver")
		} else {
			modifiers = append(modifiers, "value-receiver")
		}
	}
	analysis.add(SymbolSpec{Kind: kind, NativeKind: nativeKind, Name: declaration.Name.Name, Parent: &parent, Declaration: declarationRange, NameRange: nameRange, Signature: &signature, Body: body, Visibility: goVisibility(declaration.Name.Name), Modifiers: modifiers, Evidence: SymbolEvidenceStructural})
}

func (analysis *goDocumentAnalysis) add(spec SymbolSpec) (NormalizedSymbol, bool) {
	if analysis.symbolsStopped || analysis.checkContext() != nil {
		return NormalizedSymbol{}, false
	}
	symbol, err := analysis.builder.Add(spec)
	if operation.KindOf(err) == operation.KindLimit {
		analysis.symbolsStopped = true
		return NormalizedSymbol{}, false
	}
	if err != nil {
		analysis.builder.MarkIncomplete()
		_ = analysis.builder.AddDiagnostic(DiagnosticSpec{Code: "go-normalize", Message: err.Error(), Severity: DiagnosticError, AffectsCoverage: true})
		return NormalizedSymbol{}, false
	}
	return symbol, true
}

func (analysis *goDocumentAnalysis) addParseDiagnostics(parseErr error) {
	if parseErr == nil {
		return
	}
	analysis.builder.MarkIncomplete()
	add := func(position token.Position, message string) {
		var rangeValue *OffsetRange
		if position.Offset >= 0 && position.Offset <= len(analysis.document.Text) {
			offset := position.Offset
			if offset == len(analysis.document.Text) || utf8.RuneStart(analysis.document.Text[offset]) {
				rangeValue = &OffsetRange{Start: offset, End: offset}
			}
		}
		_ = analysis.builder.AddDiagnostic(DiagnosticSpec{Code: "go-parse", Message: message, Severity: DiagnosticError, Range: rangeValue, AffectsCoverage: true})
	}
	switch concrete := parseErr.(type) {
	case goscanner.ErrorList:
		for _, item := range concrete {
			if item != nil {
				add(item.Pos, item.Msg)
			}
		}
	case *goscanner.Error:
		add(concrete.Pos, concrete.Msg)
	default:
		add(token.Position{}, parseErr.Error())
	}
}

func (analysis *goDocumentAnalysis) recordInvalidASTRange(label string, rangeValue *OffsetRange) {
	analysis.builder.MarkIncomplete()
	_ = analysis.builder.AddDiagnostic(DiagnosticSpec{Code: "go-range", Message: "parser produced an unusable " + label + " range", Severity: DiagnosticWarning, Range: rangeValue, AffectsCoverage: true})
}

func (analysis *goDocumentAnalysis) offset(position token.Pos) (int, bool) {
	if !position.IsValid() {
		return 0, false
	}
	resolved := analysis.fileSet.PositionFor(position, false)
	if resolved.Offset < 0 || resolved.Offset > len(analysis.document.Text) {
		return 0, false
	}
	if resolved.Offset < len(analysis.document.Text) && !utf8.RuneStart(analysis.document.Text[resolved.Offset]) {
		return 0, false
	}
	return resolved.Offset, true
}

func (analysis *goDocumentAnalysis) offsetRange(start, end token.Pos) (OffsetRange, bool) {
	startOffset, startOK := analysis.offset(start)
	endOffset, endOK := analysis.offset(end)
	if !startOK || !endOK || endOffset < startOffset {
		return OffsetRange{}, false
	}
	return OffsetRange{Start: startOffset, End: endOffset}, true
}

func (analysis *goDocumentAnalysis) nodeRange(node ast.Node) (OffsetRange, bool) {
	if node == nil {
		return OffsetRange{}, false
	}
	return analysis.offsetRange(node.Pos(), node.End())
}

func (analysis *goDocumentAnalysis) nodeRangePointer(node ast.Node) *OffsetRange {
	value, ok := analysis.nodeRange(node)
	if !ok {
		return nil
	}
	return &value
}

func trimTrailingSourceWhitespace(text string, start, end int) int {
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	for end > start {
		r, size := utf8.DecodeLastRuneInString(text[start:end])
		if !unicode.IsSpace(r) {
			break
		}
		end -= size
	}
	return end
}

func goVisibility(name string) Visibility {
	if ast.IsExported(name) {
		return VisibilityPublic
	}
	return VisibilityPackage
}

func embeddedIdentifier(expression ast.Expr) *ast.Ident {
	switch concrete := expression.(type) {
	case *ast.Ident:
		return concrete
	case *ast.SelectorExpr:
		return concrete.Sel
	case *ast.StarExpr:
		return embeddedIdentifier(concrete.X)
	case *ast.IndexExpr:
		return embeddedIdentifier(concrete.X)
	case *ast.IndexListExpr:
		return embeddedIdentifier(concrete.X)
	case *ast.ParenExpr:
		return embeddedIdentifier(concrete.X)
	default:
		return nil
	}
}

func receiverTypeName(expression ast.Expr) (string, bool) {
	switch concrete := expression.(type) {
	case *ast.Ident:
		return concrete.Name, false
	case *ast.StarExpr:
		name, _ := receiverTypeName(concrete.X)
		return name, true
	case *ast.IndexExpr:
		return receiverTypeName(concrete.X)
	case *ast.IndexListExpr:
		return receiverTypeName(concrete.X)
	case *ast.ParenExpr:
		return receiverTypeName(concrete.X)
	default:
		return "", false
	}
}
