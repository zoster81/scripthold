package sourceintelligence

import (
	"context"
	"regexp"
	"strings"
)

type HTMLAnalyzer struct{}
type XMLAnalyzer struct{}
type CSSAnalyzer struct{}
type SCSSAnalyzer struct{}
type SassAnalyzer struct{}
type LessAnalyzer struct{}
type JSONAnalyzer struct{}
type YAMLAnalyzer struct{}
type TOMLAnalyzer struct{}
type MarkdownAnalyzer struct{}
type OpenAPIAnalyzer struct{}
type AnsibleYAMLAnalyzer struct{}

func (HTMLAnalyzer) ID() AnalyzerID          { return AnalyzerHTML }
func (HTMLAnalyzer) Language() string        { return "html" }
func (XMLAnalyzer) ID() AnalyzerID           { return AnalyzerXML }
func (XMLAnalyzer) Language() string         { return "xml" }
func (CSSAnalyzer) ID() AnalyzerID           { return AnalyzerCSS }
func (CSSAnalyzer) Language() string         { return "css" }
func (SCSSAnalyzer) ID() AnalyzerID          { return AnalyzerSCSS }
func (SCSSAnalyzer) Language() string        { return "scss" }
func (SassAnalyzer) ID() AnalyzerID          { return AnalyzerSass }
func (SassAnalyzer) Language() string        { return "sass" }
func (LessAnalyzer) ID() AnalyzerID          { return AnalyzerLess }
func (LessAnalyzer) Language() string        { return "less" }
func (JSONAnalyzer) ID() AnalyzerID          { return AnalyzerJSON }
func (JSONAnalyzer) Language() string        { return "json" }
func (YAMLAnalyzer) ID() AnalyzerID          { return AnalyzerYAML }
func (YAMLAnalyzer) Language() string        { return "yaml" }
func (TOMLAnalyzer) ID() AnalyzerID          { return AnalyzerTOML }
func (TOMLAnalyzer) Language() string        { return "toml" }
func (MarkdownAnalyzer) ID() AnalyzerID      { return AnalyzerMarkdown }
func (MarkdownAnalyzer) Language() string    { return "markdown" }
func (OpenAPIAnalyzer) ID() AnalyzerID       { return AnalyzerOpenAPI }
func (OpenAPIAnalyzer) Language() string     { return "openapi" }
func (AnsibleYAMLAnalyzer) ID() AnalyzerID   { return AnalyzerAnsibleYAML }
func (AnsibleYAMLAnalyzer) Language() string { return "ansible-yaml" }

var (
	markupTag      = regexp.MustCompile(`(?is)<([A-Za-z_][A-Za-z0-9_.:-]*)\b[^>]*>`)
	markupDoubleID = regexp.MustCompile(`(?i)\bid[ \t\r\n]*=[ \t\r\n]*"([^"]+)"`)
	markupSingleID = regexp.MustCompile(`(?i)\bid[ \t\r\n]*=[ \t\r\n]*'([^']+)'`)
	markupBareID   = regexp.MustCompile(`(?i)\bid[ \t\r\n]*=[ \t\r\n]*([^\s>"']+)`)
)

func (HTMLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeMarkupDocument(ctx, document, options, "html", AnalyzerHTML)
}

func (XMLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeMarkupDocument(ctx, document, options, "xml", AnalyzerXML)
}

func analyzeMarkupDocument(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, language, analyzer)
	if err != nil {
		return AnalyzerResult{}, err
	}
	regions := [][2]string{{"<!--", "-->"}}
	if language == "xml" {
		regions = append(regions, [2]string{"<![CDATA[", "]]>"})
	}
	source := maskDelimitedSourceRegions(document.Text, regions)
	for _, tag := range markupTag.FindAllStringSubmatchIndex(source, -1) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		raw := document.Text[tag[0]:tag[1]]
		var id []int
		for _, pattern := range []*regexp.Regexp{markupDoubleID, markupSingleID, markupBareID} {
			match := pattern.FindStringSubmatchIndex(raw)
			if match != nil {
				id = match
				break
			}
		}
		if id == nil {
			continue
		}
		name := raw[id[2]:id[3]]
		addDocumentDataHardwareSymbol(builder, SymbolKindEntity, "id-element", name, nil,
			OffsetRange{Start: tag[0], End: tag[1]}, OffsetRange{Start: tag[0] + id[2], End: tag[0] + id[3]})
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

var (
	scssVariable = regexp.MustCompile(`(?m)^[ \t]*\$([A-Za-z_-][A-Za-z0-9_-]*)[ \t]*:`)
	lessVariable = regexp.MustCompile(`(?m)^[ \t]*@([A-Za-z_-][A-Za-z0-9_-]*)[ \t]*:`)
)

func (CSSAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeCSSDocument(ctx, document, options, "css", AnalyzerCSS, false, false)
}
func (SCSSAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeCSSDocument(ctx, document, options, "scss", AnalyzerSCSS, true, false)
}
func (LessAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	return analyzeCSSDocument(ctx, document, options, "less", AnalyzerLess, false, true)
}

func analyzeCSSDocument(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID, scss, less bool) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, language, analyzer)
	if err != nil {
		return AnalyzerResult{}, err
	}
	var lineCommentMarkers []string
	if scss {
		lineCommentMarkers = []string{"//"}
	}
	structural := maskSourceComments(document.Text, lineCommentMarkers, "/*", "*/")
	if scss {
		for _, match := range scssVariable.FindAllStringSubmatchIndex(structural, -1) {
			name := document.Text[match[2]:match[3]]
			addDocumentDataHardwareSymbol(builder, SymbolKindVariable, "variable", name, nil, OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: match[2], End: match[3]})
		}
	}
	if less {
		for _, match := range lessVariable.FindAllStringSubmatchIndex(structural, -1) {
			name := document.Text[match[2]:match[3]]
			addDocumentDataHardwareSymbol(builder, SymbolKindVariable, "variable", name, nil, OffsetRange{Start: match[0], End: match[1]}, OffsetRange{Start: match[2], End: match[3]})
		}
	}
	selectorSource := maskSourceStrings(structural, true, true, false)
	cssSelectors(ctx, builder, document, selectorSource)
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

func cssSelectors(ctx context.Context, builder *SymbolBuilder, document *SourceDocument, source string) {
	if len(source) != len(document.Text) {
		builder.MarkIncomplete()
		return
	}
	start := 0
	for start < len(source) {
		open := strings.IndexByte(source[start:], '{')
		if open < 0 {
			return
		}
		open += start
		segmentStart := open - 1
		for segmentStart >= 0 && source[segmentStart] != '}' && source[segmentStart] != ';' && source[segmentStart] != '{' {
			segmentStart--
		}
		segmentStart++
		raw := strings.TrimSpace(source[segmentStart:open])
		if raw != "" && !strings.HasPrefix(raw, "@") && !strings.Contains(raw, ": ") {
			for _, span := range cssSelectorSpans(source[segmentStart:open]) {
				selector := source[span.Start:span.End]
				if strings.Contains(selector, "\n") && strings.Contains(selector, ":") {
					continue
				}
				nameStart := segmentStart + span.Start
				nameEnd := segmentStart + span.End
				name := document.Text[nameStart:nameEnd]
				addDocumentDataHardwareSymbol(builder, SymbolKindSelector, "selector", name, nil, OffsetRange{Start: nameStart, End: open}, OffsetRange{Start: nameStart, End: nameEnd})
			}
		}
		if err := ctx.Err(); err != nil {
			builder.MarkIncomplete()
			return
		}
		start = open + 1
	}
}

func cssSelectorSpans(segment string) []OffsetRange {
	spans := make([]OffsetRange, 0, 4)
	start := 0
	parenDepth := 0
	bracketDepth := 0
	appendSpan := func(end int) {
		left, right := start, end
		for left < right && cssWhitespace(segment[left]) {
			left++
		}
		for right > left && cssWhitespace(segment[right-1]) {
			right--
		}
		if left < right {
			spans = append(spans, OffsetRange{Start: left, End: right})
		}
	}
	for index := 0; index < len(segment); index++ {
		switch segment[index] {
		case '\\':
			if index+1 < len(segment) {
				index++
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case ',':
			if parenDepth == 0 && bracketDepth == 0 {
				appendSpan(index)
				start = index + 1
			}
		}
	}
	appendSpan(len(segment))
	return spans
}

func cssWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n', '\f':
		return true
	default:
		return false
	}
}

func (SassAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "sass", AnalyzerSass)
	if err != nil {
		return AnalyzerResult{}, err
	}
	for _, line := range sourceTextLines(document.Text) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		trimmed := strings.TrimSpace(stripLineComment(line.text, "//"))
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "$") {
			if colon := strings.Index(trimmed, ":"); colon > 1 {
				name := strings.TrimSpace(trimmed[1:colon])
				nameStart := line.start + strings.Index(line.text, name)
				addDocumentDataHardwareSymbol(builder, SymbolKindVariable, "variable", name, nil, OffsetRange{Start: line.start, End: line.end}, OffsetRange{Start: nameStart, End: nameStart + len(name)})
			}
			continue
		}
		if strings.Contains(trimmed, ": ") || strings.HasPrefix(trimmed, "@") {
			continue
		}
		if strings.HasPrefix(trimmed, ".") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "&") {
			nameStart := line.start + strings.Index(line.text, trimmed)
			addDocumentDataHardwareSymbol(builder, SymbolKindSelector, "selector", trimmed, nil, OffsetRange{Start: line.start, End: line.end}, OffsetRange{Start: nameStart, End: nameStart + len(trimmed)})
		}
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

func (JSONAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "json", AnalyzerJSON)
	if err != nil {
		return AnalyzerResult{}, err
	}
	parser := jsonStructuralParser{ctx: ctx, document: document, builder: builder}
	index := skipJSONSpace(document.Text, 0)
	if index >= len(document.Text) || document.Text[index] != '{' {
		addDocumentDataHardwareDiagnostic(builder, "json-root", "JSON structural navigation requires an object root", index, min(index+1, len(document.Text)))
		return AnalyzerResult{Analysis: builder.Result()}, nil
	}
	if _, ok := parser.object(index, nil); !ok {
		addDocumentDataHardwareDiagnostic(builder, "json-malformed", "JSON object is malformed or truncated", index, len(document.Text))
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

type jsonStructuralParser struct {
	ctx      context.Context
	document *SourceDocument
	builder  *SymbolBuilder
}

func (p *jsonStructuralParser) object(index int, parent *SymbolParent) (int, bool) {
	text := p.document.Text
	if index >= len(text) || text[index] != '{' {
		return index, false
	}
	index++
	for {
		if err := p.ctx.Err(); err != nil {
			p.builder.MarkIncomplete()
			return index, false
		}
		index = skipJSONSpace(text, index)
		if index >= len(text) {
			return index, false
		}
		if text[index] == '}' {
			return index + 1, true
		}
		key, keyStart, keyEnd, next, ok := jsonString(text, index)
		if !ok {
			return index, false
		}
		index = skipJSONSpace(text, next)
		if index >= len(text) || text[index] != ':' {
			return index, false
		}
		index = skipJSONSpace(text, index+1)
		declEnd := jsonValueEnd(text, index)
		if declEnd <= index {
			return index, false
		}
		symbol, added := addDocumentDataHardwareSymbol(p.builder, SymbolKindKey, "key", key, parent, OffsetRange{Start: keyStart, End: declEnd}, OffsetRange{Start: keyStart + 1, End: keyEnd - 1})
		var childParent *SymbolParent
		if added {
			childParent = parentFromNormalizedSymbol(symbol)
		}
		if text[index] == '{' {
			var childOK bool
			index, childOK = p.object(index, childParent)
			if !childOK {
				return index, false
			}
		} else {
			index = declEnd
		}
		index = skipJSONSpace(text, index)
		if index < len(text) && text[index] == ',' {
			index++
			continue
		}
		if index < len(text) && text[index] == '}' {
			return index + 1, true
		}
		return index, false
	}
}

func jsonString(text string, index int) (string, int, int, int, bool) {
	if index >= len(text) || text[index] != '"' {
		return "", index, index, index, false
	}
	start := index
	index++
	var value strings.Builder
	for index < len(text) {
		if text[index] == '\\' {
			if index+1 >= len(text) {
				return "", start, index, index, false
			}
			value.WriteByte(text[index+1])
			index += 2
			continue
		}
		if text[index] == '"' {
			return value.String(), start, index + 1, index + 1, true
		}
		value.WriteByte(text[index])
		index++
	}
	return "", start, index, index, false
}

func skipJSONSpace(text string, index int) int {
	for index < len(text) && (text[index] == ' ' || text[index] == '\t' || text[index] == '\r' || text[index] == '\n') {
		index++
	}
	return index
}

func jsonValueEnd(text string, index int) int {
	if index >= len(text) {
		return index
	}
	if text[index] == '"' {
		_, _, _, next, ok := jsonString(text, index)
		if ok {
			return next
		}
		return index
	}
	if text[index] == '{' || text[index] == '[' {
		open, close := text[index], byte('}')
		if open == '[' {
			close = ']'
		}
		depth := 0
		quote := false
		escaped := false
		for i := index; i < len(text); i++ {
			if quote {
				if escaped {
					escaped = false
					continue
				}
				if text[i] == '\\' {
					escaped = true
				} else if text[i] == '"' {
					quote = false
				}
				continue
			}
			if text[i] == '"' {
				quote = true
				continue
			}
			if text[i] == open {
				depth++
			} else if text[i] == close {
				depth--
				if depth == 0 {
					return i + 1
				}
			}
		}
		return index
	}
	end := index
	for end < len(text) && text[end] != ',' && text[end] != '}' && text[end] != ']' && text[end] != '\r' && text[end] != '\n' {
		end++
	}
	return end
}

var yamlKey = regexp.MustCompile(`^([A-Za-z0-9_.-]+)[ \t]*:(?:[ \t]*(.*))?$`)

type yamlScope struct {
	indent int
	parent SymbolParent
}

func (YAMLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "yaml", AnalyzerYAML)
	if err != nil {
		return AnalyzerResult{}, err
	}
	var scopes []yamlScope
	for _, line := range sourceTextLines(document.Text) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		leading := line.text[:len(line.text)-len(strings.TrimLeft(line.text, " \t"))]
		if strings.Contains(leading, "\t") {
			addDocumentDataHardwareDiagnostic(builder, "yaml-tab-indentation", "YAML indentation contains a tab and cannot be proven structurally", line.start, line.end)
			continue
		}
		trimmed := strings.TrimSpace(stripLineComment(line.text, "#"))
		if trimmed == "" || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "---") {
			continue
		}
		for len(scopes) > 0 && line.indent <= scopes[len(scopes)-1].indent {
			scopes = scopes[:len(scopes)-1]
		}
		match := yamlKey.FindStringSubmatchIndex(trimmed)
		if match == nil {
			continue
		}
		name := trimmed[match[2]:match[3]]
		var parent *SymbolParent
		if len(scopes) > 0 {
			v := scopes[len(scopes)-1].parent
			parent = &v
		}
		trimStart := line.start + strings.Index(line.text, trimmed)
		nameStart := trimStart + match[2]
		symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindKey, "key", name, parent, OffsetRange{Start: trimStart, End: line.end}, OffsetRange{Start: nameStart, End: nameStart + len(name)})
		value := ""
		if match[4] >= 0 {
			value = strings.TrimSpace(trimmed[match[4]:match[5]])
		}
		if ok && value == "" {
			scopes = append(scopes, yamlScope{indent: line.indent, parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
		}
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

var (
	tomlSection    = regexp.MustCompile(`^\[([^\[\]]+)\]$`)
	tomlArrayTable = regexp.MustCompile(`^\[\[([^\[\]]+)\]\]$`)
	tomlKey        = regexp.MustCompile(`^([A-Za-z0-9_.-]+)[ \t]*=`)
)

func (TOMLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "toml", AnalyzerTOML)
	if err != nil {
		return AnalyzerResult{}, err
	}
	sections := map[string]SymbolParent{}
	var current *SymbolParent
	multilineArrayDepth := 0
	addSection := func(line sourceTextLine, trimmed string, match []int, nativeKind string) {
		full := strings.TrimSpace(trimmed[match[2]:match[3]])
		name := full
		var parent *SymbolParent
		if dot := strings.LastIndex(full, "."); dot >= 0 {
			if p, ok := sections[full[:dot]]; ok {
				v := p
				parent = &v
				name = full[dot+1:]
			}
		}
		trimStart := line.start + strings.Index(line.text, trimmed)
		nameOffset := strings.LastIndex(trimmed, name)
		symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindSection, nativeKind, name, parent, OffsetRange{Start: trimStart, End: line.end}, OffsetRange{Start: trimStart + nameOffset, End: trimStart + nameOffset + len(name)})
		if ok {
			value := SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
			sections[full] = value
			current = &value
		}
	}
	for _, line := range sourceTextLines(document.Text) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		trimmed := strings.TrimSpace(stripLineComment(line.text, "#"))
		if trimmed == "" {
			continue
		}
		if multilineArrayDepth > 0 {
			multilineArrayDepth += tomlBracketDelta(trimmed)
			if multilineArrayDepth < 0 {
				multilineArrayDepth = 0
			}
			continue
		}
		if match := tomlArrayTable.FindStringSubmatchIndex(trimmed); match != nil {
			addSection(line, trimmed, match, "array-table")
			continue
		}
		if match := tomlSection.FindStringSubmatchIndex(trimmed); match != nil {
			addSection(line, trimmed, match, "section")
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			addDocumentDataHardwareDiagnostic(builder, "toml-malformed-section", "TOML section header is malformed", line.start, line.end)
			continue
		}
		if match := tomlKey.FindStringSubmatchIndex(trimmed); match != nil {
			name := trimmed[match[2]:match[3]]
			trimStart := line.start + strings.Index(line.text, trimmed)
			addDocumentDataHardwareSymbol(builder, SymbolKindKey, "key", name, current, OffsetRange{Start: trimStart, End: line.end}, OffsetRange{Start: trimStart + match[2], End: trimStart + match[3]})
			if equals := strings.IndexByte(trimmed, '='); equals >= 0 {
				value := strings.TrimSpace(trimmed[equals+1:])
				if strings.HasPrefix(value, "[") {
					multilineArrayDepth = tomlBracketDelta(value)
					if multilineArrayDepth < 0 {
						multilineArrayDepth = 0
					}
				}
			}
		}
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

func tomlBracketDelta(text string) int {
	depth := 0
	quote := byte(0)
	escaped := false
	for index := 0; index < len(text); index++ {
		value := text[index]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if quote == '"' && value == '\\' {
				escaped = true
				continue
			}
			if value == quote {
				quote = 0
			}
			continue
		}
		if value == '"' || value == '\'' {
			quote = value
			continue
		}
		switch value {
		case '[':
			depth++
		case ']':
			depth--
		}
	}
	return depth
}

var markdownHeading = regexp.MustCompile(`^(#{1,6})[ \t]+(.+?)[ \t]*#*[ \t]*$`)

func (MarkdownAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "markdown", AnalyzerMarkdown)
	if err != nil {
		return AnalyzerResult{}, err
	}
	type headingScope struct {
		level  int
		parent SymbolParent
	}
	var scopes []headingScope
	inFence := false
	fenceChar := byte(0)
	for _, line := range sourceTextLines(document.Text) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		trimmed := strings.TrimSpace(line.text)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			char := trimmed[0]
			if !inFence {
				inFence, fenceChar = true, char
			} else if char == fenceChar {
				inFence, fenceChar = false, 0
			}
			continue
		}
		if inFence {
			continue
		}
		match := markdownHeading.FindStringSubmatchIndex(line.trimmed)
		if match == nil {
			continue
		}
		level := match[3] - match[2]
		name := strings.TrimSpace(line.trimmed[match[4]:match[5]])
		for len(scopes) > 0 && scopes[len(scopes)-1].level >= level {
			scopes = scopes[:len(scopes)-1]
		}
		var parent *SymbolParent
		if len(scopes) > 0 {
			v := scopes[len(scopes)-1].parent
			parent = &v
		}
		trimStart := line.start + strings.Index(line.text, line.trimmed)
		nameOffset := strings.Index(line.trimmed, name)
		symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindSection, "heading", name, parent, OffsetRange{Start: trimStart, End: line.end}, OffsetRange{Start: trimStart + nameOffset, End: trimStart + nameOffset + len(name)})
		if ok {
			scopes = append(scopes, headingScope{level: level, parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
		}
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

func (OpenAPIAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "openapi", AnalyzerOpenAPI)
	if err != nil {
		return AnalyzerResult{}, err
	}
	lines := sourceTextLines(document.Text)
	inSchemas := false
	schemasIndent := -1
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		trimmed := strings.TrimSpace(stripLineComment(line.text, "#"))
		if trimmed == "" {
			continue
		}
		if trimmed == "schemas:" {
			inSchemas, schemasIndent = true, line.indent
			continue
		}
		if inSchemas && line.indent <= schemasIndent {
			inSchemas = false
		}
		if inSchemas && line.indent > schemasIndent && strings.HasSuffix(trimmed, ":") && !strings.Contains(strings.TrimSuffix(trimmed, ":"), " ") {
			name := strings.TrimSuffix(trimmed, ":")
			trimStart := line.start + strings.Index(line.text, trimmed)
			addDocumentDataHardwareSymbol(builder, SymbolKindType, "schema", name, nil, OffsetRange{Start: trimStart, End: line.end}, OffsetRange{Start: trimStart, End: trimStart + len(name)})
			continue
		}
		if strings.HasPrefix(trimmed, "operationId:") {
			name := unquoteSourceValue(strings.TrimSpace(strings.TrimPrefix(trimmed, "operationId:")))
			if name != "" {
				trimStart := line.start + strings.Index(line.text, trimmed)
				nameOffset := strings.Index(trimmed, name)
				addDocumentDataHardwareSymbol(builder, SymbolKindOperation, "operation", name, nil, OffsetRange{Start: trimStart, End: line.end}, OffsetRange{Start: trimStart + nameOffset, End: trimStart + nameOffset + len(name)})
			}
		}
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

func (AnsibleYAMLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "ansible-yaml", AnalyzerAnsibleYAML)
	if err != nil {
		return AnalyzerResult{}, err
	}
	var play *SymbolParent
	tasksIndent := -1
	for _, line := range sourceTextLines(document.Text) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		trimmed := strings.TrimSpace(stripLineComment(line.text, "#"))
		if trimmed == "tasks:" && play != nil {
			tasksIndent = line.indent
			continue
		}
		if !strings.HasPrefix(trimmed, "- name:") {
			continue
		}
		name := unquoteSourceValue(strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:")))
		if name == "" {
			continue
		}
		trimStart := line.start + strings.Index(line.text, trimmed)
		nameOffset := strings.Index(trimmed, name)
		if play == nil || tasksIndent < 0 || line.indent <= tasksIndent {
			symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindSection, "play", name, nil, OffsetRange{Start: trimStart, End: line.end}, OffsetRange{Start: trimStart + nameOffset, End: trimStart + nameOffset + len(name)})
			if ok {
				play = parentFromNormalizedSymbol(symbol)
				tasksIndent = -1
			}
			continue
		}
		addDocumentDataHardwareSymbol(builder, SymbolKindOperation, "task", name, play, OffsetRange{Start: trimStart, End: line.end}, OffsetRange{Start: trimStart + nameOffset, End: trimStart + nameOffset + len(name)})
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}
