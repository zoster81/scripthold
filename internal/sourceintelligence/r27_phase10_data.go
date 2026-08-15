package sourceintelligence

import (
	"context"
	"fmt"
	"regexp"
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

type phase10SQLDialectProfile struct {
	name    string
	pattern *regexp.Regexp
}

var phase10SQLDialectProfiles = []phase10SQLDialectProfile{
	{name: "common", pattern: regexp.MustCompile(`(?im)\bcreate[ \t]+(?:(?:or[ \t]+replace)[ \t]+)?(schema|table|view|function|procedure|type)[ \t]+([A-Za-z_][A-Za-z0-9_$]*(?:\.[A-Za-z_][A-Za-z0-9_$]*)*)`)},
	{name: "postgresql", pattern: regexp.MustCompile(`(?im)\bcreate[ \t]+(materialized[ \t]+view)[ \t]+([A-Za-z_][A-Za-z0-9_$]*(?:\.[A-Za-z_][A-Za-z0-9_$]*)*)`)},
	{name: "sqlserver", pattern: regexp.MustCompile(`(?im)\bcreate[ \t]+or[ \t]+alter[ \t]+(view|function|procedure)[ \t]+([A-Za-z_][A-Za-z0-9_$]*(?:\.[A-Za-z_][A-Za-z0-9_$]*)*)`)},
}

func (SQLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase10Builder(ctx, document, options, "sql", AnalyzerSQL)
	if err != nil {
		return AnalyzerResult{}, err
	}
	parents := map[string]SymbolParent{}
	source := phase10MaskComments(document.Text, []string{"--"}, "/*", "*/")
	source = phase10MaskStrings(source, true, false, false)
	seen := map[string]struct{}{}
	for _, profile := range phase10SQLDialectProfiles {
		for _, match := range profile.pattern.FindAllStringSubmatchIndex(source, -1) {
			if err := ctx.Err(); err != nil {
				return AnalyzerResult{}, err
			}
			kindWord := strings.ToLower(strings.Join(strings.Fields(document.Text[match[2]:match[3]]), " "))
			fullName := document.Text[match[4]:match[5]]
			key := fmt.Sprintf("%d:%d:%s", match[0], match[5], strings.ToLower(fullName))
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			kind := SymbolKindType
			switch kindWord {
			case "schema":
				kind = SymbolKindSchema
			case "function", "procedure":
				kind = SymbolKindFunction
			}
			name := fullName
			var parent *SymbolParent
			if dot := strings.LastIndex(fullName, "."); dot >= 0 {
				if value, ok := parents[strings.ToLower(fullName[:dot])]; ok {
					v := value
					parent = &v
					name = fullName[dot+1:]
				}
			}
			declEnd := match[1]
			if semi := strings.Index(document.Text[declEnd:], ";"); semi >= 0 {
				declEnd += semi + 1
			}
			nameStart := match[4]
			if name != fullName {
				nameStart = match[5] - len(name)
			}
			native := kindWord
			if profile.name != "common" {
				native = profile.name + "-" + strings.ReplaceAll(kindWord, " ", "-")
			}
			symbol, ok := phase10AddSymbol(builder, kind, native, name, parent, OffsetRange{Start: match[0], End: declEnd}, OffsetRange{Start: nameStart, End: match[5]})
			if ok && kind == SymbolKindSchema {
				parents[strings.ToLower(fullName)] = SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
			}
		}
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

var (
	phase10PLSQLPackage = regexp.MustCompile(`(?im)\bcreate[ \t]+(?:or[ \t]+replace[ \t]+)?package(?:[ \t]+body)?[ \t]+([A-Za-z_][A-Za-z0-9_$#]*)[ \t]+(?:as|is)\b`)
	phase10PLSQLRoutine = regexp.MustCompile(`(?im)^[ \t]*(procedure|function)[ \t]+([A-Za-z_][A-Za-z0-9_$#]*)\b`)
)

func (PLSQLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase10Builder(ctx, document, options, "plsql", AnalyzerPLSQL)
	if err != nil {
		return AnalyzerResult{}, err
	}
	source := phase10MaskComments(document.Text, []string{"--"}, "/*", "*/")
	source = phase10MaskStrings(source, true, false, false)
	packageMatch := phase10PLSQLPackage.FindStringSubmatchIndex(source)
	var parent *SymbolParent
	if packageMatch != nil {
		name := document.Text[packageMatch[2]:packageMatch[3]]
		declEnd := packageMatch[1]
		symbol, ok := phase10AddSymbol(builder, SymbolKindPackage, "package", name, nil, OffsetRange{Start: packageMatch[0], End: declEnd}, OffsetRange{Start: packageMatch[2], End: packageMatch[3]})
		if ok {
			parent = phase10Parent(symbol)
		}
	}
	for _, match := range phase10PLSQLRoutine.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		kindWord := strings.ToLower(document.Text[match[2]:match[3]])
		name := document.Text[match[4]:match[5]]
		end := match[1]
		if semi := strings.Index(document.Text[end:], ";"); semi >= 0 {
			end += semi + 1
		}
		phase10AddSymbol(builder, SymbolKindFunction, kindWord, name, parent, OffsetRange{Start: match[0], End: end}, OffsetRange{Start: match[4], End: match[5]})
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

var (
	phase10GraphQLSchema    = regexp.MustCompile(`(?m)\bschema[ \t]*\{`)
	phase10GraphQLContainer = regexp.MustCompile(`(?m)\b(type|interface|input|enum)[ \t]+([A-Za-z_][A-Za-z0-9_]*)[^\{]*\{`)
	phase10GraphQLOperation = regexp.MustCompile(`(?m)\b(query|mutation|subscription)[ \t]+([A-Za-z_][A-Za-z0-9_]*)\b`)
	phase10GraphQLField     = regexp.MustCompile(`(?m)([A-Za-z_][A-Za-z0-9_]*)[ \t]*(?:\([^)]*\)[ \t]*)?:`)
)

func (GraphQLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase10Builder(ctx, document, options, "graphql", AnalyzerGraphQL)
	if err != nil {
		return AnalyzerResult{}, err
	}
	source := phase10MaskComments(document.Text, []string{"#"}, "", "")
	source = phase10MaskStrings(source, false, true, true)
	if match := phase10GraphQLSchema.FindStringIndex(source); match != nil {
		nameStart := match[0]
		phase10AddSymbol(builder, SymbolKindSchema, "schema", "schema", nil, OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: nameStart, End: nameStart + len("schema")})
	}
	for _, match := range phase10GraphQLContainer.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		open := match[1] - 1
		close := phase10MatchingBrace(source, open)
		if close < 0 {
			phase10Diagnostic(builder, "graphql-unclosed-block", "GraphQL declaration has an unclosed block", match[0], match[1])
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
		symbol, ok := phase10AddSymbol(builder, kind, word, name, nil, OffsetRange{Start: match[0], End: close + 1}, OffsetRange{Start: match[4], End: match[5]})
		if !ok || word == "enum" {
			continue
		}
		parent := phase10Parent(symbol)
		bodyStart := open + 1
		body := source[bodyStart:close]
		for _, field := range phase10GraphQLField.FindAllStringSubmatchIndex(body, -1) {
			fieldName := document.Text[bodyStart+field[2] : bodyStart+field[3]]
			start := bodyStart + field[0]
			end := bodyStart + field[1]
			phase10AddSymbol(builder, SymbolKindField, "field", fieldName, parent, OffsetRange{Start: start, End: end}, OffsetRange{Start: bodyStart + field[2], End: bodyStart + field[3]})
		}
	}
	for _, match := range phase10GraphQLOperation.FindAllStringSubmatchIndex(source, -1) {
		word := strings.ToLower(document.Text[match[2]:match[3]])
		name := document.Text[match[4]:match[5]]
		phase10AddSymbol(builder, SymbolKindOperation, word, name, nil, OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: match[4], End: match[5]})
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

var phase10HCLBlock = regexp.MustCompile(`(?m)^([ \t]*)(variable|resource|data|module|output)[ \t]+"([^"]+)"(?:[ \t]+"([^"]+)")?[ \t]*\{`)
var phase10HCLGenericBlock = regexp.MustCompile(`(?m)^([ \t]*)([A-Za-z_][A-Za-z0-9_-]*)(?:[ \t]+"([^"]+)")?[ \t]*\{`)
var phase10HCLSource = regexp.MustCompile(`(?m)^[ \t]*source[ \t]*=[ \t]*"([^"]+)"`)

func (TerraformAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase10Builder(ctx, document, options, "terraform", AnalyzerTerraform)
	if err != nil {
		return AnalyzerResult{}, err
	}
	dependencies := []StructuralDependency{}
	source := phase10MaskComments(document.Text, []string{"#", "//"}, "/*", "*/")
	source = phase10MaskHeredocs(source)
	for _, match := range phase10HCLBlock.FindAllStringSubmatchIndex(source, -1) {
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
		close := phase10MatchingBrace(source, open)
		if close < 0 {
			phase10Diagnostic(builder, "terraform-unclosed-block", "Terraform block has an unclosed brace", match[0], match[1])
			continue
		}
		name := first
		kind := SymbolKindVariable
		native := word
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
		phase10AddSymbol(builder, kind, native, name, nil, OffsetRange{Start: match[0], End: close + 1}, OffsetRange{Start: nameStart, End: nameEnd})
		if word == "module" {
			bodyStart := open + 1
			if sourceMatch := phase10HCLSource.FindStringSubmatchIndex(source[bodyStart:close]); sourceMatch != nil {
				value := document.Text[bodyStart+sourceMatch[2] : bodyStart+sourceMatch[3]]
				phase10AddDependency(document, &dependencies, StructuralDependencyImport, value, bodyStart+sourceMatch[2], bodyStart+sourceMatch[3])
			}
		}
	}
	for _, match := range phase10HCLGenericBlock.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		word := document.Text[match[4]:match[5]]
		switch word {
		case "variable", "resource", "data", "module", "output":
			continue
		}
		open := match[1] - 1
		close := phase10MatchingBrace(source, open)
		if close < 0 {
			phase10Diagnostic(builder, "hcl-unclosed-block", "HCL block has an unclosed brace", match[0], match[1])
			continue
		}
		name := word
		nameStart, nameEnd := match[4], match[5]
		if match[6] >= 0 {
			label := document.Text[match[6]:match[7]]
			name += "." + label
			nameEnd = match[7]
		}
		phase10AddSymbol(builder, SymbolKindSection, "hcl-block", name, nil, OffsetRange{Start: match[0], End: close + 1}, OffsetRange{Start: nameStart, End: nameEnd})
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

var phase10NixBinding = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_'-]*)[ \t]*=`)

func (NixAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase10Builder(ctx, document, options, "nix", AnalyzerNix)
	if err != nil {
		return AnalyzerResult{}, err
	}
	inLet := false
	for _, line := range phase10Lines(document.Text) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		trimmed := strings.TrimSpace(phase10StripLineComment(line.text, "#"))
		if trimmed == "let" || strings.HasPrefix(trimmed, "let ") {
			inLet = true
			continue
		}
		if inLet && (trimmed == "in" || strings.HasPrefix(trimmed, "in ")) {
			inLet = false
			continue
		}
		if !inLet {
			continue
		}
		match := phase10NixBinding.FindStringSubmatchIndex(trimmed)
		if match == nil {
			continue
		}
		name := trimmed[match[2]:match[3]]
		trimOffset := strings.Index(line.text, trimmed)
		start := line.start + trimOffset
		phase10AddSymbol(builder, SymbolKindVariable, "binding", name, nil, OffsetRange{Start: start, End: line.end}, OffsetRange{Start: start + match[2], End: start + match[3]})
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

var (
	phase10ProtoPackage = regexp.MustCompile(`(?m)^[ \t]*package[ \t]+([A-Za-z_][A-Za-z0-9_.]*)[ \t]*;`)
	phase10ProtoImport  = regexp.MustCompile(`(?m)^[ \t]*import(?:[ \t]+(?:public|weak))?[ \t]+"([^"]+)"[ \t]*;`)
	phase10ProtoBlock   = regexp.MustCompile(`(?m)^[ \t]*(message|enum|service)[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*\{`)
	phase10ProtoField   = regexp.MustCompile(`(?m)^[ \t]*(?:optional[ \t]+|required[ \t]+|repeated[ \t]+)?[A-Za-z_.][A-Za-z0-9_.<> ,]*[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*=[ \t]*[0-9]+`)
	phase10ProtoRPC     = regexp.MustCompile(`(?m)^[ \t]*rpc[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*\(`)
)

func (ProtoAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newPhase10Builder(ctx, document, options, "proto", AnalyzerProto)
	if err != nil {
		return AnalyzerResult{}, err
	}
	dependencies := []StructuralDependency{}
	source := phase10MaskComments(document.Text, []string{"//"}, "/*", "*/")
	var packageParent *SymbolParent
	if match := phase10ProtoPackage.FindStringSubmatchIndex(source); match != nil {
		name := document.Text[match[2]:match[3]]
		symbol, ok := phase10AddSymbol(builder, SymbolKindPackage, "package", name, nil, OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: match[2], End: match[3]})
		if ok {
			packageParent = phase10Parent(symbol)
		}
	}
	for _, match := range phase10ProtoImport.FindAllStringSubmatchIndex(source, -1) {
		value := document.Text[match[2]:match[3]]
		phase10AddDependency(document, &dependencies, StructuralDependencyImport, value, match[2], match[3])
	}
	for _, match := range phase10ProtoBlock.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		word := document.Text[match[2]:match[3]]
		name := document.Text[match[4]:match[5]]
		open := match[1] - 1
		close := phase10MatchingBrace(source, open)
		if close < 0 {
			phase10Diagnostic(builder, "proto-unclosed-block", "Protocol Buffers declaration has an unclosed block", match[0], match[1])
			continue
		}
		kind := SymbolKindType
		if word == "enum" {
			kind = SymbolKindEnum
		} else if word == "service" {
			kind = SymbolKindInterface
		}
		symbol, ok := phase10AddSymbol(builder, kind, word, name, packageParent, OffsetRange{Start: match[0], End: close + 1}, OffsetRange{Start: match[4], End: match[5]})
		if !ok {
			continue
		}
		parent := phase10Parent(symbol)
		bodyStart := open + 1
		body := source[bodyStart:close]
		if word == "service" {
			for _, rpc := range phase10ProtoRPC.FindAllStringSubmatchIndex(body, -1) {
				name := document.Text[bodyStart+rpc[2] : bodyStart+rpc[3]]
				phase10AddSymbol(builder, SymbolKindOperation, "rpc", name, parent, OffsetRange{Start: bodyStart + rpc[0], End: bodyStart + rpc[1]}, OffsetRange{Start: bodyStart + rpc[2], End: bodyStart + rpc[3]})
			}
		} else if word == "message" {
			for _, field := range phase10ProtoField.FindAllStringSubmatchIndex(body, -1) {
				name := document.Text[bodyStart+field[2] : bodyStart+field[3]]
				phase10AddSymbol(builder, SymbolKindField, "field", name, parent, OffsetRange{Start: bodyStart + field[0], End: bodyStart + field[1]}, OffsetRange{Start: bodyStart + field[2], End: bodyStart + field[3]})
			}
		}
	}
	return AnalyzerResult{Analysis: builder.Result(), Dependencies: dependencies}, nil
}

func phase10MatchingBrace(text string, open int) int {
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
