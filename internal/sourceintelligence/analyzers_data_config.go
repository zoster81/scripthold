package sourceintelligence

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type SQLAnalyzer struct{}
type PLSQLAnalyzer struct{}
type GraphQLAnalyzer struct{}
type TerraformAnalyzer struct{}
type NixAnalyzer struct{}
type ProtoAnalyzer struct{}

func (SQLAnalyzer) ID() AnalyzerID         { return AnalyzerSQL }
func (SQLAnalyzer) Language() string       { return "sql" }
func (PLSQLAnalyzer) ID() AnalyzerID       { return AnalyzerPLSQL }
func (PLSQLAnalyzer) Language() string     { return "plsql" }
func (GraphQLAnalyzer) ID() AnalyzerID     { return AnalyzerGraphQL }
func (GraphQLAnalyzer) Language() string   { return "graphql" }
func (TerraformAnalyzer) ID() AnalyzerID   { return AnalyzerTerraform }
func (TerraformAnalyzer) Language() string { return "terraform" }
func (NixAnalyzer) ID() AnalyzerID         { return AnalyzerNix }
func (NixAnalyzer) Language() string       { return "nix" }
func (ProtoAnalyzer) ID() AnalyzerID       { return AnalyzerProto }
func (ProtoAnalyzer) Language() string     { return "proto" }

type sqlDialectProfile struct {
	name    string
	pattern *regexp.Regexp
}

type sqlDeclarationCandidate struct {
	profile    string
	kindWord   string
	fullName   string
	parentName string
	leafName   string
	start      int
	headerEnd  int
	nameStart  int
	nameEnd    int
	leafStart  int
	leafEnd    int
}

const sqlIdentifierPattern = "(?:[A-Za-z_][A-Za-z0-9_$]*|\"(?:[^\"]|\"\")*\"|`(?:[^`]|``)*`|\\[(?:[^\\]]|\\]\\])*\\])"
const sqlQualifiedNamePattern = sqlIdentifierPattern + "(?:[ \\t]*\\.[ \\t]*" + sqlIdentifierPattern + ")*"

var (
	sqlIdentifier       = regexp.MustCompile(sqlIdentifierPattern)
	sqlSimpleIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*$`)
	sqlReference        = regexp.MustCompile(`(?im)\breferences[ \t\r\n]+(` + sqlQualifiedNamePattern + `)`)
	sqlDialectProfiles  = []sqlDialectProfile{
		{name: "common", pattern: regexp.MustCompile(`(?im)\bcreate[ \t\r\n]+(?:(?:or[ \t]+replace)[ \t\r\n]+)?(schema|table|view|function|procedure|type)[ \t\r\n]+(?:if[ \t]+not[ \t]+exists[ \t\r\n]+)?(` + sqlQualifiedNamePattern + `)`)},
		{name: "common", pattern: regexp.MustCompile(`(?im)\bcreate[ \t\r\n]+(?:unique[ \t\r\n]+)?(index)[ \t\r\n]+(?:if[ \t]+not[ \t]+exists[ \t\r\n]+)?(` + sqlQualifiedNamePattern + `)`)},
		{name: "postgresql", pattern: regexp.MustCompile(`(?im)\bcreate[ \t]+(materialized[ \t]+view)[ \t]+(?:if[ \t]+not[ \t]+exists[ \t]+)?(` + sqlQualifiedNamePattern + `)`)},
		{name: "sqlserver", pattern: regexp.MustCompile(`(?im)\bcreate[ \t]+or[ \t]+alter[ \t]+(view|function|procedure)[ \t]+(` + sqlQualifiedNamePattern + `)`)},
		{name: "oracle", pattern: regexp.MustCompile(`(?im)\bcreate[ \t\r\n]+(?:or[ \t\r\n]+replace[ \t\r\n]+)?(?:force|noforce)[ \t\r\n]+(view)[ \t\r\n]+(` + sqlQualifiedNamePattern + `)`)},
	}
)

func (SQLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "sql", AnalyzerSQL)
	if err != nil {
		return AnalyzerResult{}, err
	}
	source := maskSourceComments(document.Text, []string{"--"}, "/*", "*/")
	source = maskSourceStrings(source, true, false, false)
	candidates, unsupportedHeaders, unsupportedIndexes := sqlDeclarations(document, source)
	for _, header := range unsupportedHeaders {
		builder.MarkIncomplete()
		_ = builder.AddDiagnostic(DiagnosticSpec{
			Code: "sql-unrecognized-table-header", Message: "SQL table declaration has an unrecognized header",
			Severity: DiagnosticWarning, Range: &header, AffectsCoverage: true,
		})
	}
	for _, header := range unsupportedIndexes {
		builder.MarkIncomplete()
		_ = builder.AddDiagnostic(DiagnosticSpec{
			Code: "sql-unrecognized-index-header", Message: "SQL index declaration has an unrecognized header",
			Severity: DiagnosticWarning, Range: &header, AffectsCoverage: true,
		})
	}
	parents := map[string]SymbolParent{}
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		kind := SymbolKindType
		switch candidate.kindWord {
		case "schema":
			kind = SymbolKindSchema
		case "function", "procedure":
			kind = SymbolKindFunction
		case "index":
			kind = SymbolKindResource
		}
		name := candidate.fullName
		nameStart := candidate.nameStart
		nameEnd := candidate.nameEnd
		var parent *SymbolParent
		if candidate.parentName != "" {
			if value, ok := parents[strings.ToLower(candidate.parentName)]; ok {
				v := value
				parent = &v
				name = candidate.leafName
				nameStart = candidate.leafStart
				nameEnd = candidate.leafEnd
			}
		}
		declEnd := candidate.headerEnd
		if semi := strings.Index(source[declEnd:], ";"); semi >= 0 {
			declEnd += semi + 1
		}
		native := candidate.kindWord
		if candidate.profile != "common" {
			native = candidate.profile + "-" + strings.ReplaceAll(candidate.kindWord, " ", "-")
		}
		symbol, ok := addDocumentDataHardwareSymbol(builder, kind, native, name, parent, OffsetRange{Start: candidate.start, End: declEnd}, OffsetRange{Start: nameStart, End: nameEnd})
		if ok && kind == SymbolKindSchema {
			parents[strings.ToLower(candidate.fullName)] = SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
		}
	}
	var dependencies structuralDependencyAccumulator
	for _, match := range sqlReference.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		fullName, _, _, _, _, ok := sqlCanonicalName(document.Text[match[2]:match[3]])
		if !ok {
			continue
		}
		dependencies.add(document, StructuralDependencyReference, fullName, match[2], match[3])
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies.items}, nil
}

func sqlDeclarations(document *SourceDocument, source string) ([]sqlDeclarationCandidate, []OffsetRange, []OffsetRange) {
	seen := map[string]struct{}{}
	var candidates []sqlDeclarationCandidate
	var unsupportedHeaders []OffsetRange
	var unsupportedIndexes []OffsetRange
	for _, profile := range sqlDialectProfiles {
		for _, match := range profile.pattern.FindAllStringSubmatchIndex(source, -1) {
			if !sqlDeclarationBoundary(source, match[0]) {
				continue
			}
			kindWord := strings.ToLower(strings.Join(strings.Fields(document.Text[match[2]:match[3]]), " "))
			if kindWord == "table" && !sqlSupportedTableHeader(source, match[1]) {
				unsupportedHeaders = append(unsupportedHeaders, OffsetRange{Start: match[0], End: match[1]})
				continue
			}
			if kindWord == "index" && !sqlSupportedIndexHeader(source, match[1]) {
				unsupportedIndexes = append(unsupportedIndexes, OffsetRange{Start: match[0], End: match[1]})
				continue
			}
			fullName, parentName, leafName, leafStart, leafEnd, ok := sqlCanonicalName(document.Text[match[4]:match[5]])
			if !ok {
				continue
			}
			key := fmt.Sprintf("%d:%d:%s:%s", match[0], match[5], kindWord, strings.ToLower(fullName))
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, sqlDeclarationCandidate{
				profile: profile.name, kindWord: kindWord, fullName: fullName, parentName: parentName, leafName: leafName,
				start: match[0], headerEnd: match[1], nameStart: match[4], nameEnd: match[5], leafStart: match[4] + leafStart, leafEnd: match[4] + leafEnd,
			})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].start != candidates[j].start {
			return candidates[i].start < candidates[j].start
		}
		if candidates[i].headerEnd != candidates[j].headerEnd {
			return candidates[i].headerEnd < candidates[j].headerEnd
		}
		if candidates[i].kindWord != candidates[j].kindWord {
			return candidates[i].kindWord < candidates[j].kindWord
		}
		return candidates[i].fullName < candidates[j].fullName
	})
	return candidates, unsupportedHeaders, unsupportedIndexes
}

var sqlIndexBody = regexp.MustCompile(`(?is)^[ \t\r\n]+on[ \t\r\n]+(?:only[ \t\r\n]+)?` + sqlQualifiedNamePattern + `[ \t\r\n]*(?:using[ \t\r\n]+[A-Za-z_][A-Za-z_0-9]*[ \t\r\n]*)?\(`)

func sqlSupportedIndexHeader(source string, headerEnd int) bool {
	match := sqlIndexBody.FindStringIndex(source[headerEnd:])
	if match == nil {
		return false
	}
	open := headerEnd + match[1] - 1
	depth := 0
	for index := open; index < len(source); index++ {
		switch source[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return strings.TrimSpace(source[open+1:index]) != ""
			}
		case ';':
			return false
		}
	}
	return false
}

// Recognize common SQL table-header shapes without claiming to validate each
// dialect's full DDL grammar. Unknown forms remain explicitly incomplete.
func sqlSupportedTableHeader(source string, headerEnd int) bool {
	rest := strings.TrimLeft(source[headerEnd:], " \t\r\n")
	if len(rest) == 0 {
		return false
	}
	if rest[0] == '(' {
		return true
	}
	end := 0
	for end < len(rest) {
		value := rest[end]
		if value < 'A' || value > 'Z' {
			if value < 'a' || value > 'z' {
				break
			}
		}
		end++
	}
	switch strings.ToLower(rest[:end]) {
	case "as", "like", "of", "partition", "using", "options", "cluster", "with":
		return true
	default:
		return false
	}
}

func sqlDeclarationBoundary(source string, start int) bool {
	if start < 0 || start > len(source) {
		return false
	}
	for index := start - 1; index >= 0; index-- {
		switch source[index] {
		case ' ', '\t':
			continue
		case '\r', '\n', ';':
			return true
		default:
			return false
		}
	}
	return true
}

func sqlCanonicalName(raw string) (fullName, parentName, leafName string, leafStart, leafEnd int, ok bool) {
	matches := sqlIdentifier.FindAllStringIndex(raw, -1)
	if len(matches) == 0 {
		return "", "", "", 0, 0, false
	}
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		parts = append(parts, sqlCanonicalIdentifier(raw[match[0]:match[1]]))
	}
	last := matches[len(matches)-1]
	leafName = parts[len(parts)-1]
	fullName = strings.Join(parts, ".")
	if len(parts) > 1 {
		parentName = strings.Join(parts[:len(parts)-1], ".")
	}
	return fullName, parentName, leafName, last[0], last[1], true
}

func sqlCanonicalIdentifier(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 {
		return raw
	}
	value := raw
	switch {
	case raw[0] == '"' && raw[len(raw)-1] == '"':
		value = strings.ReplaceAll(raw[1:len(raw)-1], `""`, `"`)
	case raw[0] == '`' && raw[len(raw)-1] == '`':
		value = strings.ReplaceAll(raw[1:len(raw)-1], "``", "`")
	case raw[0] == '[' && raw[len(raw)-1] == ']':
		value = strings.ReplaceAll(raw[1:len(raw)-1], "]]", "]")
	}
	if sqlSimpleIdentifier.MatchString(value) {
		return value
	}
	return raw
}

var (
	plsqlPackage           = regexp.MustCompile(`(?im)\bcreate[ \t]+(?:or[ \t]+replace[ \t]+)?package(?:[ \t]+body)?[ \t]+([A-Za-z_][A-Za-z0-9_$#]*)[ \t]+(?:as|is)\b`)
	plsqlStandaloneRoutine = regexp.MustCompile(`(?im)^[ \t]*create[ \t]+(?:or[ \t]+replace[ \t]+)?(procedure|function)[ \t]+([A-Za-z_][A-Za-z0-9_$#]*)\b`)
	plsqlRoutine           = regexp.MustCompile(`(?im)^[ \t]*(procedure|function)[ \t]+([A-Za-z_][A-Za-z0-9_$#]*)\b`)
	plsqlBatchEnd          = regexp.MustCompile(`(?m)^[ \t]*/[ \t]*\r?$`)
)

type plsqlPackageScope struct {
	start, end int
	parent     *SymbolParent
}

func (PLSQLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "plsql", AnalyzerPLSQL)
	if err != nil {
		return AnalyzerResult{}, err
	}
	source := maskSourceComments(document.Text, []string{"--"}, "/*", "*/")
	source = maskSourceStrings(source, true, false, false)
	packages := plsqlPackage.FindAllStringSubmatchIndex(source, -1)
	batchEnds := plsqlBatchEnd.FindAllStringIndex(source, -1)
	scopes := make([]plsqlPackageScope, 0, len(packages))
	parents := make(map[string]SymbolParent)
	batch := 0
	for i, match := range packages {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		for batch < len(batchEnds) && batchEnds[batch][0] < match[0] {
			batch++
		}
		end := len(source)
		if i+1 < len(packages) {
			end = packages[i+1][0]
		}
		if batch < len(batchEnds) && batchEnds[batch][0] < end {
			end = batchEnds[batch][0]
		}
		name := document.Text[match[2]:match[3]]
		key := strings.ToLower(name)
		parent, exists := parents[key]
		if !exists {
			symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindPackage, "package", name, nil,
				OffsetRange{Start: match[0], End: match[1]},
				OffsetRange{Start: match[2], End: match[3]})
			if ok {
				parent = symbol
				parents[key] = parent
				exists = true
			}
		}
		scope := plsqlPackageScope{start: match[0], end: end}
		if exists {
			scope.parent = &parent
		}
		scopes = append(scopes, scope)
	}
	for _, match := range plsqlStandaloneRoutine.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		kindWord := strings.ToLower(document.Text[match[2]:match[3]])
		name := document.Text[match[4]:match[5]]
		addDocumentDataHardwareSymbol(builder, SymbolKindFunction, kindWord, name, nil,
			OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: match[4], End: match[5]})
	}
	scopeIndex := 0
	for _, match := range plsqlRoutine.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		for scopeIndex+1 < len(scopes) && scopes[scopeIndex+1].start < match[0] {
			scopeIndex++
		}
		var parent *SymbolParent
		if len(scopes) > 0 && scopes[scopeIndex].start < match[0] &&
			match[0] < scopes[scopeIndex].end {
			parent = scopes[scopeIndex].parent
		}
		kindWord := strings.ToLower(document.Text[match[2]:match[3]])
		name := document.Text[match[4]:match[5]]
		end := match[1]
		if semi := strings.Index(document.Text[end:], ";"); semi >= 0 {
			end += semi + 1
		}
		addDocumentDataHardwareSymbol(builder, SymbolKindFunction, kindWord, name, parent,
			OffsetRange{Start: match[0], End: end}, OffsetRange{Start: match[4], End: match[5]})
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

var (
	graphqlSchema    = regexp.MustCompile(`(?m)\bschema[ \t]*\{`)
	graphqlContainer = regexp.MustCompile(`(?m)\b(type|interface|input|enum)[ \t]+([A-Za-z_][A-Za-z0-9_]*)[^\{]*\{`)
	graphqlOperation = regexp.MustCompile(`(?m)\b(query|mutation|subscription)[ \t]+([A-Za-z_][A-Za-z0-9_]*)\b`)
	graphqlNamedLeaf = regexp.MustCompile(`(?m)^[ \t]*(union|scalar)[ \t]+([A-Za-z_][A-Za-z0-9_]*)\b`)
	graphqlField     = regexp.MustCompile(`(?m)([A-Za-z_][A-Za-z0-9_]*)[ \t]*(?:\([^)]*\)[ \t]*)?:`)
)

func (GraphQLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "graphql", AnalyzerGraphQL)
	if err != nil {
		return AnalyzerResult{}, err
	}
	source := maskGraphQLSource(document.Text)
	if match := graphqlSchema.FindStringIndex(source); match != nil {
		nameStart := match[0]
		addDocumentDataHardwareSymbol(builder, SymbolKindSchema, "schema", "schema", nil, OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: nameStart, End: nameStart + len("schema")})
	}
	for _, match := range graphqlContainer.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		open := match[1] - 1
		close := matchingBraceOffset(source, open)
		if close < 0 {
			addDocumentDataHardwareDiagnostic(builder, "graphql-unclosed-block", "GraphQL declaration has an unclosed block", match[0], match[1])
			continue
		}
		word := strings.ToLower(document.Text[match[2]:match[3]])
		name := document.Text[match[4]:match[5]]
		kind := SymbolKindType
		if word == "interface" {
			kind = SymbolKindInterface
		} else if word == "enum" {
			kind = SymbolKindEnum
		}
		symbol, ok := addDocumentDataHardwareSymbol(builder, kind, word, name, nil, OffsetRange{Start: match[0], End: close + 1}, OffsetRange{Start: match[4], End: match[5]})
		if !ok || word == "enum" {
			continue
		}
		parent := &symbol
		bodyStart := open + 1
		body := source[bodyStart:close]
		for _, field := range graphqlField.FindAllStringSubmatchIndex(body, -1) {
			fieldName := document.Text[bodyStart+field[2] : bodyStart+field[3]]
			start := bodyStart + field[0]
			end := bodyStart + field[1]
			addDocumentDataHardwareSymbol(builder, SymbolKindField, "field", fieldName, parent, OffsetRange{Start: start, End: end}, OffsetRange{Start: bodyStart + field[2], End: bodyStart + field[3]})
		}
	}
	for _, match := range graphqlNamedLeaf.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		word := document.Text[match[2]:match[3]]
		name := document.Text[match[4]:match[5]]
		addDocumentDataHardwareSymbol(builder, SymbolKindType, word, name, nil,
			OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: match[4], End: match[5]})
	}
	for _, match := range graphqlOperation.FindAllStringSubmatchIndex(source, -1) {
		word := strings.ToLower(document.Text[match[2]:match[3]])
		name := document.Text[match[4]:match[5]]
		addDocumentDataHardwareSymbol(builder, SymbolKindOperation, word, name, nil, OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: match[4], End: match[5]})
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

var hclBlock = regexp.MustCompile(`(?m)^([ \t]*)(variable|resource|data|module|output)[ \t]+"([^"]+)"(?:[ \t]+"([^"]+)")?[ \t]*\{`)
var hclGenericBlock = regexp.MustCompile(`(?m)^([ \t]*)([A-Za-z_][A-Za-z0-9_-]*)(?:[ \t]+"([^"]+)")?[ \t]*\{`)
var hclSource = regexp.MustCompile(`(?m)^[ \t]*source[ \t]*=[ \t]*"([^"]+)"`)

type hclBlockCandidate struct {
	start     int
	open      int
	close     int
	kind      SymbolKind
	native    string
	name      string
	nameRange OffsetRange
	word      string
}

type hclScope struct {
	close  int
	parent *SymbolParent
}

func (TerraformAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "terraform", AnalyzerTerraform)
	if err != nil {
		return AnalyzerResult{}, err
	}
	dependencies := structuralDependencyAccumulator{items: []StructuralDependency{}}
	source := maskSourceComments(document.Text, []string{"#", "//"}, "/*", "*/")
	source = maskSourceHeredocs(source)
	candidates := make([]hclBlockCandidate, 0)
	for _, match := range hclBlock.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		word := document.Text[match[4]:match[5]]
		first := document.Text[match[6]:match[7]]
		second := ""
		if match[8] >= 0 {
			second = document.Text[match[8]:match[9]]
		}
		open := match[1] - 1
		close := matchingBraceOffset(source, open)
		if close < 0 {
			addDocumentDataHardwareDiagnostic(builder, "terraform-unclosed-block", "Terraform block has an unclosed brace", match[0], match[1])
			continue
		}
		name := first
		kind := SymbolKindVariable
		nameStart, nameEnd := match[6], match[7]
		switch word {
		case "resource", "data":
			kind = SymbolKindResource
			name = first + "." + second
			nameStart, nameEnd = match[6], match[9]
		case "module":
			kind = SymbolKindModule
		case "variable", "output":
			kind = SymbolKindVariable
		}
		candidates = append(candidates, hclBlockCandidate{
			start: match[0], open: open, close: close, kind: kind, native: word, name: name,
			nameRange: OffsetRange{Start: nameStart, End: nameEnd}, word: word,
		})
	}
	for _, match := range hclGenericBlock.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		word := document.Text[match[4]:match[5]]
		switch word {
		case "variable", "resource", "data", "module", "output":
			continue
		}
		open := match[1] - 1
		close := matchingBraceOffset(source, open)
		if close < 0 {
			addDocumentDataHardwareDiagnostic(builder, "hcl-unclosed-block", "HCL block has an unclosed brace", match[0], match[1])
			continue
		}
		name := word
		nameStart, nameEnd := match[4], match[5]
		if match[6] >= 0 {
			label := document.Text[match[6]:match[7]]
			name += "." + label
			nameEnd = match[7]
		}
		candidates = append(candidates, hclBlockCandidate{
			start: match[0], open: open, close: close, kind: SymbolKindSection, native: "hcl-block", name: name,
			nameRange: OffsetRange{Start: nameStart, End: nameEnd}, word: word,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].start != candidates[j].start {
			return candidates[i].start < candidates[j].start
		}
		return candidates[i].close > candidates[j].close
	})
	maxNesting := options.MaxNesting
	if maxNesting <= 0 {
		maxNesting = 2048
	}
	scopes := make([]hclScope, 0, min(8, maxNesting))
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		for len(scopes) > 0 && candidate.start >= scopes[len(scopes)-1].close {
			scopes = scopes[:len(scopes)-1]
		}
		if len(scopes) >= maxNesting {
			addDocumentDataHardwareDiagnostic(builder, "terraform-block-nesting-limit", "Terraform/HCL structural block nesting exceeds the configured limit", candidate.start, candidate.open+1)
			break
		}
		var parent *SymbolParent
		if len(scopes) > 0 {
			parent = scopes[len(scopes)-1].parent
		}
		symbol, added := addDocumentDataHardwareSymbol(builder, candidate.kind, candidate.native, candidate.name, parent, OffsetRange{Start: candidate.start, End: candidate.close + 1}, candidate.nameRange)
		if candidate.word == "module" {
			bodyStart := candidate.open + 1
			if sourceMatch := hclSource.FindStringSubmatchIndex(source[bodyStart:candidate.close]); sourceMatch != nil {
				value := document.Text[bodyStart+sourceMatch[2] : bodyStart+sourceMatch[3]]
				dependencies.add(document, StructuralDependencyImport, value, bodyStart+sourceMatch[2], bodyStart+sourceMatch[3])
			}
		}
		if added {
			scopes = append(scopes, hclScope{close: candidate.close, parent: &symbol})
		}
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies.items}, nil
}

var nixBinding = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_'-]*)[ \t]*=`)

func (NixAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "nix", AnalyzerNix)
	if err != nil {
		return AnalyzerResult{}, err
	}
	inLet := false
	rootSeen := false
	for _, line := range sourceTextLines(document.Text) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		trimmed := strings.TrimSpace(stripLineComment(line.text, "#"))
		if !rootSeen && trimmed != "" {
			rootSeen = true
			if trimmed != "let" && !strings.HasPrefix(trimmed, "let ") {
				addDocumentDataHardwareDiagnostic(builder, "nix-unmodeled-root-structure",
					"Nix root expressions outside a let binding are not indexed", line.start, line.end)
			}
		}
		if trimmed == "let" || strings.HasPrefix(trimmed, "let ") {
			inLet = true
			continue
		}
		if inLet && (trimmed == "in" || strings.HasPrefix(trimmed, "in ")) {
			// Only let bindings are indexed; attributes returned by the Nix
			// expression must not silently count as completely navigable.
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "in"))
			if value == "" || strings.Contains(value, "{") {
				addDocumentDataHardwareDiagnostic(builder, "nix-unmodeled-result-structure",
					"Nix result structure is not indexed by the let-binding analyzer", line.start, line.end)
			}
			inLet = false
			continue
		}
		if !inLet {
			continue
		}
		match := nixBinding.FindStringSubmatchIndex(trimmed)
		if match == nil {
			continue
		}
		name := trimmed[match[2]:match[3]]
		trimOffset := strings.Index(line.text, trimmed)
		start := line.start + trimOffset
		addDocumentDataHardwareSymbol(builder, SymbolKindVariable, "binding", name, nil, OffsetRange{Start: start, End: line.end}, OffsetRange{Start: start + match[2], End: start + match[3]})
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

var (
	protoPackage   = regexp.MustCompile(`(?m)^[ \t]*package[ \t]+([A-Za-z_][A-Za-z0-9_.]*)[ \t]*;`)
	protoImport    = regexp.MustCompile(`(?m)^[ \t]*import(?:[ \t]+(?:public|weak))?[ \t]+"([^"]+)"[ \t]*;`)
	protoBlock     = regexp.MustCompile(`(?m)^[ \t]*(message|enum|service)[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*\{`)
	protoField     = regexp.MustCompile(`(?m)^[ \t]*(?:optional[ \t]+|required[ \t]+|repeated[ \t]+)?[A-Za-z_.][A-Za-z0-9_.<> ,]*[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*=[ \t]*[0-9]+`)
	protoEnumValue = regexp.MustCompile(`(?m)^[ \t]*([A-Za-z_][A-Za-z0-9_]*)[ \t]*=[ \t]*-?[0-9]+[ \t]*;`)
	protoRPC       = regexp.MustCompile(`(?m)^[ \t]*rpc[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*\(`)
)

func (ProtoAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "proto", AnalyzerProto)
	if err != nil {
		return AnalyzerResult{}, err
	}
	dependencies := structuralDependencyAccumulator{items: []StructuralDependency{}}
	source := maskSourceComments(document.Text, []string{"//"}, "/*", "*/")
	var packageParent *SymbolParent
	if match := protoPackage.FindStringSubmatchIndex(source); match != nil {
		name := document.Text[match[2]:match[3]]
		symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindPackage, "package", name, nil, OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: match[2], End: match[3]})
		if ok {
			packageParent = &symbol
		}
	}
	for _, match := range protoImport.FindAllStringSubmatchIndex(source, -1) {
		value := document.Text[match[2]:match[3]]
		dependencies.add(document, StructuralDependencyImport, value, match[2], match[3])
	}
	// Collect all brace-bounded declarations first, so fields in nested
	// messages cannot be duplicated under an enclosing message.
	type protoScope struct {
		match       []int
		word        string
		open, close int
		parent      int
		children    []OffsetRange
		owner       SymbolParent
	}
	var scopes []protoScope
	var stack []int
	for _, match := range protoBlock.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		open := match[1] - 1
		close := matchingBraceOffset(source, open)
		if close < 0 {
			addDocumentDataHardwareDiagnostic(builder, "proto-unclosed-block", "Protocol Buffers declaration has an unclosed block", match[0], match[1])
			break // Do not promote descendants of a malformed outer declaration.
		}
		for len(stack) > 0 && match[0] >= scopes[stack[len(stack)-1]].close {
			stack = stack[:len(stack)-1]
		}
		parent := -1
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
			scopes[parent].children = append(scopes[parent].children, OffsetRange{Start: match[0], End: close + 1})
		}
		scopes = append(scopes, protoScope{match: match, word: document.Text[match[2]:match[3]], open: open, close: close, parent: parent})
		stack = append(stack, len(scopes)-1)
	}
	for index := range scopes {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		scope := &scopes[index]
		match := scope.match
		name := document.Text[match[4]:match[5]]
		parent := packageParent
		if scope.parent >= 0 {
			parent = &scopes[scope.parent].owner
		}
		kind := SymbolKindType
		if scope.word == "enum" {
			kind = SymbolKindEnum
		} else if scope.word == "service" {
			kind = SymbolKindInterface
		}
		owner, ok := addDocumentDataHardwareSymbol(builder, kind, scope.word, name, parent, OffsetRange{Start: match[0], End: scope.close + 1}, OffsetRange{Start: match[4], End: match[5]})
		if !ok {
			break
		}
		scope.owner = owner
		bodyStart := scope.open + 1
		body := source[bodyStart:scope.close]
		var kindMatches [][]int
		var memberKind SymbolKind
		var native string
		switch scope.word {
		case "message":
			kindMatches = protoField.FindAllStringSubmatchIndex(body, -1)
			memberKind, native = SymbolKindField, "field"
		case "service":
			kindMatches = protoRPC.FindAllStringSubmatchIndex(body, -1)
			memberKind, native = SymbolKindOperation, "rpc"
		case "enum":
			kindMatches = protoEnumValue.FindAllStringSubmatchIndex(body, -1)
			memberKind, native = SymbolKindConstant, "enum-value"
		default:
			continue
		}
		for _, member := range kindMatches {
			memberStart := bodyStart + member[0]
			// Direct children are ordered and contain their own nested scopes.
			child := sort.Search(len(scope.children), func(i int) bool {
				return scope.children[i].Start > memberStart
			})
			if child > 0 && memberStart < scope.children[child-1].End {
				continue
			}
			memberName := document.Text[bodyStart+member[2] : bodyStart+member[3]]
			addDocumentDataHardwareSymbol(builder, memberKind, native, memberName, &scope.owner, OffsetRange{Start: memberStart, End: bodyStart + member[1]}, OffsetRange{Start: bodyStart + member[2], End: bodyStart + member[3]})
		}
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies.items}, nil
}

func matchingBraceOffset(text string, open int) int {
	if open < 0 || open >= len(text) || text[open] != '{' {
		return -1
	}
	depth := 0
	quote := byte(0)
	escaped := false
	for i := open; i < len(text); i++ {
		value := text[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if value == '\\' {
				escaped = true
				continue
			}
			if value == quote {
				quote = 0
			}
			continue
		}
		if value == '\'' || value == '"' {
			quote = value
			continue
		}
		switch value {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
