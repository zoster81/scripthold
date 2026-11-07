package sourceintelligence

import (
	"context"
	"encoding/xml"
	"io"
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
func (OpenAPIAnalyzer) ID() AnalyzerID       { return AnalyzerOpenAPI }
func (OpenAPIAnalyzer) Language() string     { return "openapi" }
func (AnsibleYAMLAnalyzer) ID() AnalyzerID   { return AnalyzerAnsibleYAML }
func (AnsibleYAMLAnalyzer) Language() string { return "ansible-yaml" }

var (
	markupTag               = regexp.MustCompile(`(?is)<([A-Za-z_][A-Za-z0-9_.:-]*)\b[^>]*>`)
	markupDoubleID          = regexp.MustCompile(`(?i)\bid[ \t\r\n]*=[ \t\r\n]*"([^"]+)"`)
	markupSingleID          = regexp.MustCompile(`(?i)\bid[ \t\r\n]*=[ \t\r\n]*'([^']+)'`)
	markupBareID            = regexp.MustCompile(`(?i)\bid[ \t\r\n]*=[ \t\r\n]*([^\s>"']+)`)
	xmlAndroidResourceRoot  = regexp.MustCompile(`(?is)^\s*(?:<\?xml\b[^>]*>\s*)?<resources\b[^>]*>`)
	xmlAndroidResourceClose = regexp.MustCompile(`(?i)</resources\s*>`)
	xmlResourceDoubleName   = regexp.MustCompile(`(?i)(?:^|[ \t\r\n])name[ \t\r\n]*=[ \t\r\n]*"([^"]+)"`)
	xmlResourceSingleName   = regexp.MustCompile(`(?i)(?:^|[ \t\r\n])name[ \t\r\n]*=[ \t\r\n]*'([^']+)'`)
	htmlRawTextClose        = map[string]*regexp.Regexp{
		"script":   regexp.MustCompile(`(?i)</script\s*>`),
		"style":    regexp.MustCompile(`(?i)</style\s*>`),
		"textarea": regexp.MustCompile(`(?i)</textarea\s*>`),
		"title":    regexp.MustCompile(`(?i)</title\s*>`),
		"xmp":      regexp.MustCompile(`(?i)</xmp\s*>`),
		"iframe":   regexp.MustCompile(`(?i)</iframe\s*>`),
		"noembed":  regexp.MustCompile(`(?i)</noembed\s*>`),
		"noframes": regexp.MustCompile(`(?i)</noframes\s*>`),
	}
)

// markupIDAttribute only recognizes an id attribute in the opening tag, not
// query parameters or template code inside another attribute's quoted value.
// It walks the tag once and keeps offsets relative to the original source.
func markupIDAttribute(tag string) (string, int, int, bool) {
	if len(tag) < 2 || tag[0] != '<' {
		return "", 0, 0, false
	}
	i := 1
	for i < len(tag) && !markupAttributeSpace(tag[i]) && tag[i] != '>' && tag[i] != '/' {
		i++
	}
	for i < len(tag) {
		for i < len(tag) && markupAttributeSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] == '>' {
			break
		}
		if tag[i] == '/' {
			i++
			continue
		}
		nameStart := i
		for i < len(tag) && !markupAttributeSpace(tag[i]) && tag[i] != '=' && tag[i] != '>' && tag[i] != '/' {
			i++
		}
		if nameStart == i {
			i++
			continue
		}
		name := tag[nameStart:i]
		for i < len(tag) && markupAttributeSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] != '=' {
			continue
		}
		i++
		for i < len(tag) && markupAttributeSpace(tag[i]) {
			i++
		}
		if i >= len(tag) {
			break
		}
		valueStart, valueEnd := i, i
		if tag[i] == '"' || tag[i] == '\'' {
			quote := tag[i]
			i++
			valueStart = i
			for i < len(tag) && tag[i] != quote {
				i++
			}
			if i == len(tag) {
				break // unterminated attribute value; subsequent bytes are not attributes
			}
			valueEnd = i
			i++
		} else {
			for i < len(tag) && !markupAttributeSpace(tag[i]) && tag[i] != '>' {
				if tag[i] == '/' && i+1 < len(tag) && tag[i+1] == '>' {
					break
				}
				i++
			}
			valueEnd = i
		}
		if strings.EqualFold(name, "id") && valueStart < valueEnd {
			return tag[valueStart:valueEnd], valueStart, valueEnd, true
		}
	}
	return "", 0, 0, false
}

func markupAttributeSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

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
	resourceStart, resourceEnd := -1, -1
	if language == "xml" {
		if root := xmlAndroidResourceRoot.FindStringIndex(source); root != nil {
			resourceStart = root[1]
			if closeTag := xmlAndroidResourceClose.FindStringIndex(source[resourceStart:]); closeTag != nil {
				resourceEnd = resourceStart + closeTag[0]
			} else {
				builder.MarkIncomplete()
			}
		}
	}
	for cursor := 0; cursor < len(source); {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		tag := markupTag.FindStringSubmatchIndex(source[cursor:])
		if tag == nil {
			break
		}
		start, end := cursor+tag[0], cursor+tag[1]
		raw := document.Text[start:end]
		if name, idStart, idEnd, ok := markupIDAttribute(raw); ok {
			addDocumentDataHardwareSymbol(builder, SymbolKindEntity, "id-element", name, nil,
				OffsetRange{Start: start, End: end}, OffsetRange{Start: start + idStart, End: start + idEnd})
		}
		tagName := strings.ToLower(source[cursor+tag[2] : cursor+tag[3]])
		if language == "xml" && tagName == "string" && start >= resourceStart && resourceEnd >= end && resourceStart >= 0 {
			for _, pattern := range []*regexp.Regexp{xmlResourceDoubleName, xmlResourceSingleName} {
				nameMatch := pattern.FindStringSubmatchIndex(raw)
				if nameMatch == nil {
					continue
				}
				addDocumentDataHardwareSymbol(builder, SymbolKindResource, "android-string", raw[nameMatch[2]:nameMatch[3]], nil,
					OffsetRange{Start: start, End: end}, OffsetRange{Start: start + nameMatch[2], End: start + nameMatch[3]})
				break
			}
		}
		cursor = end
		if language != "html" {
			continue
		}
		name := tagName
		if strings.HasSuffix(strings.TrimSpace(raw[:len(raw)-1]), "/") {
			continue // Self-closing tags in template hosts have no raw-text body.
		}
		if name == "plaintext" {
			break // HTML plaintext content extends to EOF.
		}
		closePattern := htmlRawTextClose[name]
		if closePattern == nil {
			continue
		}
		closeTag := closePattern.FindStringIndex(source[cursor:])
		if closeTag == nil {
			builder.MarkIncomplete()
			break // An unterminated raw-text element cannot safely expose later tags.
		}
		cursor += closeTag[1]
	}
	if language == "xml" {
		properties, complete, err := msbuildPropertyElements(ctx, document.Text)
		if err != nil {
			return AnalyzerResult{}, err
		}
		if !complete {
			builder.MarkIncomplete()
		}
		for _, property := range properties {
			addDocumentDataHardwareSymbol(builder, SymbolKindProperty, "msbuild-property", property.name, nil,
				OffsetRange{Start: property.start, End: property.end},
				OffsetRange{Start: property.nameStart, End: property.nameEnd})
		}
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

// MSBuild property values are only navigable when their XML document is
// well-formed and the element is a leaf directly inside Project/PropertyGroup.
// The decoder provides exact source offsets; no MSBuild evaluation is attempted.
type msbuildPropertyElement struct {
	name                           string
	start, end, nameStart, nameEnd int
}

func msbuildPropertyElements(ctx context.Context, source string) ([]msbuildPropertyElement, bool, error) {
	type frame struct {
		name                      xml.Name
		start, nameStart, nameEnd int
		property, nested          bool
	}
	decoder := xml.NewDecoder(strings.NewReader(source))
	var stack []frame
	var properties []msbuildPropertyElement
	for {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		start := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			return properties, true, nil
		}
		if err != nil {
			return nil, false, nil // Never expose candidate properties from malformed XML.
		}
		end := int(decoder.InputOffset())
		switch element := token.(type) {
		case xml.StartElement:
			if len(stack) == 0 {
				if element.Name.Local != "Project" ||
					(element.Name.Space != "" && element.Name.Space != "http://schemas.microsoft.com/developer/msbuild/2003") {
					return nil, true, nil // Other XML documents retain their existing contract.
				}
			}
			if len(stack) >= 96 {
				return nil, false, nil
			}
			if len(stack) > 0 {
				stack[len(stack)-1].nested = true
			}
			current := frame{name: element.Name, start: start}
			if len(stack) == 2 && stack[1].name.Local == "PropertyGroup" &&
				stack[1].name.Space == stack[0].name.Space && element.Name.Space == stack[0].name.Space {
				tag := markupTag.FindStringSubmatchIndex(source[start:end])
				if tag != nil && tag[0] == 0 {
					name := source[start+tag[2] : start+tag[3]]
					prefix := strings.LastIndexByte(name, ':')
					current.property = true
					current.nameStart = start + tag[2] + prefix + 1
					current.nameEnd = start + tag[3]
				}
			}
			stack = append(stack, current)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, false, nil
			}
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if current.property && !current.nested {
				properties = append(properties, msbuildPropertyElement{
					name: element.Name.Local, start: current.start, end: end,
					nameStart: current.nameStart, nameEnd: current.nameEnd,
				})
			}
		}
	}
}

var (
	scssVariable        = regexp.MustCompile(`(?m)^[ \t]*\$([A-Za-z_-][A-Za-z0-9_-]*)[ \t]*:`)
	scssModuleDirective = regexp.MustCompile(`(?m)^[ \t]*@(?:use|forward)\b`)
	lessVariable        = regexp.MustCompile(`(?m)^[ \t]*@([A-Za-z_-][A-Za-z0-9_-]*)[ \t]*:`)
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
	if scss || less {
		lineCommentMarkers = []string{"//"}
	}
	structural := maskSourceComments(document.Text, lineCommentMarkers, "/*", "*/")
	selectorSource := maskSourceStrings(structural, true, true, false)
	if less {
		selectorSource = maskLessSelectorPunctuation(selectorSource)
	}
	if scss {
		variableSource := maskSCSSModuleDirectives(structural, selectorSource, builder)
		for _, match := range scssVariable.FindAllStringSubmatchIndex(variableSource, -1) {
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
	cssSelectors(ctx, builder, document, selectorSource)
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

// maskLessSelectorPunctuation prevents Less interpolation braces and mixin
// argument separators from being mistaken for CSS rule boundaries. Keep every
// byte offset unchanged so extracted names still come from the original text.
func maskLessSelectorPunctuation(source string) string {
	masked := []byte(source)
	parenDepth := 0
	for index := 0; index < len(masked); index++ {
		if masked[index] == '@' && index+1 < len(masked) && masked[index+1] == '{' {
			// A nested opening brace means this interpolation cannot be proved.
			// Searching only up to the next delimiter keeps malformed input linear.
			if next := strings.IndexAny(source[index+2:], "{}"); next >= 0 && source[index+2+next] == '}' {
				close := index + 2 + next
				masked[index+1] = ' '
				masked[close] = ' '
				index = close
				continue
			}
		}
		switch masked[index] {
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case ';':
			if parenDepth > 0 {
				masked[index] = ' '
			}
		}
	}
	return string(masked)
}

// maskSCSSModuleDirectives excludes module configuration arguments from
// variable declarations. Scanning string-masked text avoids treating quoted
// semicolons as directive terminators; line endings and offsets stay intact.
func maskSCSSModuleDirectives(structural, stringMasked string, builder *SymbolBuilder) string {
	var masked []byte
	for _, match := range scssModuleDirective.FindAllStringIndex(stringMasked, -1) {
		if masked == nil {
			masked = []byte(structural)
		}
		end := len(stringMasked)
		if terminator := strings.IndexByte(stringMasked[match[1]:], ';'); terminator >= 0 {
			end = match[1] + terminator + 1
		} else {
			// A truncated module directive cannot prove all later declarations.
			builder.MarkIncomplete()
		}
		maskByteRangePreservingLines(masked, match[0], end)
	}
	if masked == nil {
		return structural
	}
	return string(masked)
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
				selector := source[segmentStart+span.Start : segmentStart+span.End]
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
	lines := sourceTextLines(document.Text)
	for index, line := range lines {
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
		if trimmed == "@function" || strings.HasPrefix(trimmed, "@function ") || strings.HasPrefix(trimmed, "@function\t") {
			rest := strings.TrimSpace(trimmed[len("@function"):])
			paren := strings.IndexByte(rest, '(')
			name := ""
			if paren > 0 && strings.IndexByte(rest[paren+1:], ')') >= 0 {
				name = strings.TrimSpace(rest[:paren])
			}
			if !sassBareElementName(name) {
				addDocumentDataHardwareDiagnostic(builder, "sass-malformed-function", "Sass function declaration has no valid name or parameter list", line.start, line.end)
				continue
			}
			nameStart := line.start + strings.Index(line.text, name)
			addDocumentDataHardwareSymbol(builder, SymbolKindFunction, "function", name, nil, OffsetRange{Start: line.start, End: line.end}, OffsetRange{Start: nameStart, End: nameStart + len(name)})
			continue
		}
		if strings.Contains(trimmed, ": ") || strings.HasPrefix(trimmed, "@") {
			continue
		}
		isSelector := strings.HasPrefix(trimmed, ".") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "&")
		if !isSelector && sassBareElementName(trimmed) {
			indent := len(line.text) - len(strings.TrimLeft(line.text, " \t"))
			for next := index + 1; next < len(lines); next++ {
				if strings.TrimSpace(stripLineComment(lines[next].text, "//")) == "" {
					continue
				}
				childIndent := len(lines[next].text) - len(strings.TrimLeft(lines[next].text, " \t"))
				isSelector = childIndent > indent
				break
			}
		}
		if isSelector {
			nameStart := line.start + strings.Index(line.text, trimmed)
			addDocumentDataHardwareSymbol(builder, SymbolKindSelector, "selector", trimmed, nil, OffsetRange{Start: line.start, End: line.end}, OffsetRange{Start: nameStart, End: nameStart + len(trimmed)})
		}
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

func sassBareElementName(name string) bool {
	for index := 0; index < len(name); index++ {
		char := name[index]
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' {
			continue
		}
		if index > 0 && (char >= '0' && char <= '9' || char == '-' || char == '_') {
			continue
		}
		return false
	}
	return len(name) > 0
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
	return AnalyzerResult{Analysis: builder.takeResult()}, nil
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
			childParent = &symbol
		}
		if text[index] == '{' {
			var childOK bool
			index, childOK = p.object(index, childParent)
			if !childOK {
				return index, false
			}
		} else {
			if text[index] == '[' && jsonArrayContainsObject(text, index, declEnd) {
				addDocumentDataHardwareDiagnostic(p.builder, "json-unmodeled-array-objects",
					"JSON object keys inside arrays are not indexed", index, declEnd)
			}
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

// jsonArrayContainsObject detects unindexed objects while ignoring braces in
// JSON string values. Scalar-only arrays do not hide structural declarations.
func jsonArrayContainsObject(text string, start, end int) bool {
	inString := false
	escaped := false
	for index := start + 1; index < end; index++ {
		c := text[index]
		if inString {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
		} else if c == '{' {
			return true
		}
	}
	return false
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

type yamlScope struct {
	indent int
	parent SymbolParent
}

// yamlMappingEntry recognizes simple block-style keys without mistaking a
// quoted colon (or a colon without separating whitespace) for an assignment.
func yamlMappingEntry(text string) (name string, nameEnd int, value string, ok bool) {
	quote := byte(0)
	if len(text) > 0 && (text[0] == '"' || text[0] == '\'') {
		quote = text[0]
	}
	for index := 0; index < len(text); index++ {
		if quote != 0 {
			if index == 0 {
				continue
			}
			if quote == '"' && text[index] == '\\' && index+1 < len(text) {
				index++
				continue
			}
			if text[index] == quote {
				if quote == '\'' && index+1 < len(text) && text[index+1] == '\'' {
					index++
					continue
				}
				quote = 0
			}
			continue
		}
		if text[index] != ':' || (index+1 < len(text) && text[index+1] != ' ' && text[index+1] != '\t') {
			continue
		}
		raw := strings.TrimRight(text[:index], " \t")
		if raw == "" || strings.ContainsAny(raw[:1], "?[{!&*") {
			return "", 0, "", false
		}
		name = raw
		if raw[0] == '"' || raw[0] == '\'' {
			if len(raw) < 2 || raw[len(raw)-1] != raw[0] {
				return "", 0, "", false
			}
			name = raw[1 : len(raw)-1]
			if raw[0] == '"' {
				// Escaped YAML keys need a full scalar decoder; do not guess names.
				if strings.Contains(name, "\\") {
					return "", 0, "", false
				}
			} else {
				name = strings.ReplaceAll(name, "''", "'")
			}
		}
		return name, len(raw), strings.TrimSpace(text[index+1:]), true
	}
	return "", 0, "", false
}

// yamlBlockScalarValue handles literal/folded blocks preceded by optional YAML
// tags or anchors without classifying arbitrary trailing text as a block marker.
func yamlBlockScalarValue(value string) bool {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts[:len(parts)-1] {
		if !strings.HasPrefix(part, "!") && !strings.HasPrefix(part, "&") {
			return false
		}
	}
	indicator := parts[len(parts)-1]
	if indicator[0] != '|' && indicator[0] != '>' {
		return false
	}
	for index := 1; index < len(indicator); index++ {
		c := indicator[index]
		if c != '+' && c != '-' && (c < '1' || c > '9') {
			return false
		}
	}
	return true
}

func (YAMLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "yaml", AnalyzerYAML)
	if err != nil {
		return AnalyzerResult{}, err
	}
	var scopes []yamlScope
	blockScalarIndent := -1
	skipSequenceIndent := -1
	for _, line := range sourceTextLines(document.Text) {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		trimmed := strings.TrimSpace(stripLineComment(line.text, "#"))
		if blockScalarIndent >= 0 {
			if trimmed == "" || line.indent > blockScalarIndent {
				continue
			}
			blockScalarIndent = -1
		}
		if skipSequenceIndent >= 0 {
			if trimmed == "" || line.indent > skipSequenceIndent {
				continue
			}
			skipSequenceIndent = -1
		}
		leading := line.text[:len(line.text)-len(strings.TrimLeft(line.text, " \t"))]
		if strings.Contains(leading, "\t") {
			addDocumentDataHardwareDiagnostic(builder, "yaml-tab-indentation", "YAML indentation contains a tab and cannot be proven structurally", line.start, line.end)
			continue
		}
		if trimmed == "" || trimmed == "---" || trimmed == "..." {
			continue
		}
		for len(scopes) > 0 && line.indent <= scopes[len(scopes)-1].indent {
			scopes = scopes[:len(scopes)-1]
		}
		if trimmed == "-" || strings.HasPrefix(trimmed, "- ") {
			addDocumentDataHardwareDiagnostic(builder, "yaml-unmodeled-sequence", "YAML sequence mapping relationships are not modeled", line.start, line.end)
			skipSequenceIndent = line.indent
			continue
		}
		if strings.HasPrefix(trimmed, "? ") || strings.HasPrefix(trimmed, ": ") || trimmed == "?" || trimmed == ":" {
			addDocumentDataHardwareDiagnostic(builder, "yaml-unmodeled-complex-key", "YAML complex mapping keys are not modeled", line.start, line.end)
			continue
		}
		name, nameEnd, value, valid := yamlMappingEntry(trimmed)
		if !valid {
			addDocumentDataHardwareDiagnostic(builder, "yaml-unmodeled-mapping", "YAML mapping key cannot be identified reliably", line.start, line.end)
			continue
		}
		var parent *SymbolParent
		if len(scopes) > 0 {
			v := scopes[len(scopes)-1].parent
			parent = &v
		}
		trimStart := line.start + strings.Index(line.text, trimmed)
		symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindKey, "key", name, parent,
			OffsetRange{Start: trimStart, End: line.end}, OffsetRange{Start: trimStart, End: trimStart + nameEnd})
		if ok && value == "" {
			scopes = append(scopes, yamlScope{indent: line.indent, parent: SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}})
		}
		if ok && yamlBlockScalarValue(value) {
			blockScalarIndent = line.indent
		}
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}

var (
	tomlSection    = regexp.MustCompile(`^\[([^\[\]]+)\]$`)
	tomlArrayTable = regexp.MustCompile(`^\[\[([^\[\]]+)\]\]$`)
)

// tomlSectionSegments normalizes spaces around separators, but retains quoted
// key spelling so a quoted dot cannot alias a structural path separator.
type tomlSectionSegment struct {
	name       string
	start, end int
}

func tomlSectionSegments(raw string) ([]tomlSectionSegment, bool) {
	segments := make([]tomlSectionSegment, 0, 3)
	at := 0
	for {
		for at < len(raw) && (raw[at] == ' ' || raw[at] == '\t') {
			at++
		}
		if at == len(raw) {
			return segments, len(segments) > 0
		}
		start := at
		if raw[at] == '"' || raw[at] == '\'' {
			quote := raw[at]
			at++
			closed := false
			for at < len(raw) {
				if quote == '"' && raw[at] == '\\' {
					at += 2
					continue
				}
				if raw[at] == quote {
					at++
					closed = true
					break
				}
				at++
			}
			if !closed || at > len(raw) {
				return nil, false
			}
		} else {
			for at < len(raw) {
				c := raw[at]
				if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
					(c >= '0' && c <= '9') || c == '_' || c == '-' {
					at++
					continue
				}
				break
			}
			if at == start {
				return nil, false
			}
		}
		segments = append(segments, tomlSectionSegment{name: raw[start:at], start: start, end: at})
		for at < len(raw) && (raw[at] == ' ' || raw[at] == '\t') {
			at++
		}
		if at == len(raw) {
			return segments, true
		}
		if raw[at] != '.' {
			return nil, false
		}
		at++
		for at < len(raw) && (raw[at] == ' ' || raw[at] == '\t') {
			at++
		}
		if at == len(raw) {
			return nil, false
		}
	}
}

// tomlAssignmentKey finds the assignment separator outside quoted key segments.
// Preserve the quoted spelling so literal dots cannot alias path separators.
func tomlAssignmentKey(text string) ([]tomlSectionSegment, int, bool) {
	quote := byte(0)
	for index := 0; index < len(text); index++ {
		c := text[index]
		if quote != 0 {
			if quote == '"' && c == '\\' && index+1 < len(text) {
				index++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		if c == '=' {
			parts, ok := tomlSectionSegments(text[:index])
			return parts, index, ok
		}
	}
	return nil, -1, false
}

func (TOMLAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "toml", AnalyzerTOML)
	if err != nil {
		return AnalyzerResult{}, err
	}
	sections := map[string]SymbolParent{}
	var current *SymbolParent
	multilineArrayDepth := 0
	invalidSection := false
	addSection := func(line sourceTextLine, trimmed string, match []int, nativeKind string) {
		raw := trimmed[match[2]:match[3]]
		parts, valid := tomlSectionSegments(raw)
		if !valid {
			addDocumentDataHardwareDiagnostic(builder, "toml-malformed-section", "TOML section path is malformed", line.start, line.end)
			invalidSection = true
			current = nil
			return
		}
		invalidSection = false
		names := make([]string, len(parts))
		for index, part := range parts {
			names[index] = part.name
		}
		full := strings.Join(names, ".")
		name := full
		var parent *SymbolParent
		trimStart := line.start + strings.Index(line.text, trimmed)
		nameStart := trimStart + match[2] + parts[0].start
		nameEnd := trimStart + match[2] + parts[len(parts)-1].end
		if len(parts) > 1 {
			if p, ok := sections[strings.Join(names[:len(names)-1], ".")]; ok {
				v := p
				parent = &v
				name = parts[len(parts)-1].name
				nameStart = trimStart + match[2] + parts[len(parts)-1].start
			}
		}
		symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindSection, nativeKind, name, parent, OffsetRange{Start: trimStart, End: line.end}, OffsetRange{Start: nameStart, End: nameEnd})
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
			invalidSection = true
			current = nil
			continue
		}
		if invalidSection {
			// Keys beneath an invalid table cannot be assigned a trustworthy parent.
			continue
		}
		parts, equals, valid := tomlAssignmentKey(trimmed)
		if equals >= 0 && !valid {
			addDocumentDataHardwareDiagnostic(builder, "toml-malformed-key", "TOML assignment has an invalid key", line.start, line.end)
			continue
		}
		if valid {
			names := make([]string, len(parts))
			for index, part := range parts {
				names[index] = part.name
			}
			name := strings.Join(names, ".")
			trimStart := line.start + strings.Index(line.text, trimmed)
			addDocumentDataHardwareSymbol(builder, SymbolKindKey, "key", name, current,
				OffsetRange{Start: trimStart, End: line.end},
				OffsetRange{Start: trimStart + parts[0].start, End: trimStart + parts[len(parts)-1].end})
			value := strings.TrimSpace(trimmed[equals+1:])
			if strings.HasPrefix(value, "[") {
				multilineArrayDepth = tomlBracketDelta(value)
				if multilineArrayDepth < 0 {
					multilineArrayDepth = 0
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

func (OpenAPIAnalyzer) Analyze(ctx context.Context, document *SourceDocument, options AnalyzeOptions) (AnalyzerResult, error) {
	builder, err := newStructuralAnalyzerBuilder(ctx, document, options, "openapi", AnalyzerOpenAPI)
	if err != nil {
		return AnalyzerResult{}, err
	}
	lines := sourceTextLines(document.Text)
	// Arazzo workflow operationIds reference API operations; they do not declare them.
	// Check all root keys before collecting symbols so a misplaced dialect key
	// cannot leave earlier references reported as OpenAPI declarations.
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		if line.indent == 0 && strings.HasPrefix(strings.TrimSpace(stripLineComment(line.text, "#")), "arazzo:") {
			builder.MarkIncomplete()
			return AnalyzerResult{Analysis: builder.Result()}, nil
		}
	}
	inSchemas := false
	schemasIndent := -1
	schemaEntryIndent := -1
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
			schemaEntryIndent = -1
			continue
		}
		if inSchemas && line.indent <= schemasIndent {
			inSchemas = false
			schemaEntryIndent = -1
		}
		if inSchemas && line.indent > schemasIndent && strings.HasSuffix(trimmed, ":") && !strings.Contains(strings.TrimSuffix(trimmed, ":"), " ") {
			if schemaEntryIndent < 0 {
				schemaEntryIndent = line.indent
			}
			if line.indent != schemaEntryIndent {
				continue
			}
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
	lines := sourceTextLines(document.Text)
	for i, line := range lines {
		if err := ctx.Err(); err != nil {
			return AnalyzerResult{}, err
		}
		trimmed := strings.TrimSpace(stripLineComment(line.text, "#"))
		if line.indent == 0 && strings.HasPrefix(trimmed, "- hosts:") {
			play = nil
			tasksIndent = -1
		}
		if play != nil && (trimmed == "tasks:" || trimmed == "pre_tasks:" || trimmed == "post_tasks:" || trimmed == "handlers:") {
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
		isPlay := false
		if line.indent == 0 {
			play = nil
			tasksIndent = -1
			// A play has a sibling hosts or task-list key, not a nested variable with that name.
			childIndent, hostsIndent, taskListIndent := -1, -1, -1
			for _, next := range lines[i+1:] {
				nextText := strings.TrimSpace(stripLineComment(next.text, "#"))
				if nextText == "" {
					continue
				}
				if next.indent <= line.indent {
					break
				}
				if childIndent < 0 || next.indent < childIndent {
					childIndent = next.indent
				}
				if strings.HasPrefix(nextText, "hosts:") && (hostsIndent < 0 || next.indent < hostsIndent) {
					hostsIndent = next.indent
				}
				if (nextText == "tasks:" || nextText == "pre_tasks:" || nextText == "post_tasks:" || nextText == "handlers:") && (taskListIndent < 0 || next.indent < taskListIndent) {
					taskListIndent = next.indent
				}
			}
			isPlay = (hostsIndent >= 0 && hostsIndent == childIndent) || (taskListIndent >= 0 && taskListIndent == childIndent)
		}
		if isPlay {
			symbol, ok := addDocumentDataHardwareSymbol(builder, SymbolKindSection, "play", name, nil, OffsetRange{Start: trimStart, End: line.end}, OffsetRange{Start: trimStart + nameOffset, End: trimStart + nameOffset + len(name)})
			if ok {
				play = &symbol
			}
			continue
		}
		var parent *SymbolParent
		if play != nil && tasksIndent >= 0 && line.indent >= tasksIndent {
			parent = play
		}
		addDocumentDataHardwareSymbol(builder, SymbolKindOperation, "task", name, parent, OffsetRange{Start: trimStart, End: line.end}, OffsetRange{Start: trimStart + nameOffset, End: trimStart + nameOffset + len(name)})
	}
	return AnalyzerResult{Analysis: builder.Result()}, nil
}
