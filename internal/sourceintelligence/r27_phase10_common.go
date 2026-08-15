package sourceintelligence

import (
	"context"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

type phase10Line struct {
	text       string
	trimmed    string
	start, end int
	indent     int
}

func newPhase10Builder(ctx context.Context, document *SourceDocument, options AnalyzeOptions, language string, analyzer AnalyzerID) (*SymbolBuilder, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return nil, operation.New(operation.KindInvalidInput, "source document is required")
	}
	builder := NewSymbolBuilder(document, SymbolBuilderOptions{
		Context: ctx, Language: language, Analyzer: string(analyzer), IncludeSignatures: options.IncludeSignatures,
		MaxEvidence: SymbolEvidenceStructural, Limits: options.Limits,
	})
	if err := builder.checkReady(); err != nil {
		return nil, err
	}
	return builder, nil
}

func phase10Lines(text string) []phase10Line {
	lines := make([]phase10Line, 0, strings.Count(text, "\n")+1)
	for start := 0; start <= len(text); {
		end := start
		for end < len(text) && text[end] != '\r' && text[end] != '\n' {
			end++
		}
		raw := text[start:end]
		trimmed := strings.TrimSpace(raw)
		indent := 0
		for indent < len(raw) && (raw[indent] == ' ' || raw[indent] == '\t') {
			indent++
		}
		lines = append(lines, phase10Line{text: raw, trimmed: trimmed, start: start, end: end, indent: indent})
		if end >= len(text) {
			break
		}
		if text[end] == '\r' && end+1 < len(text) && text[end+1] == '\n' {
			start = end + 2
		} else {
			start = end + 1
		}
	}
	return lines
}

func phase10AddSymbol(builder *SymbolBuilder, kind SymbolKind, native, name string, parent *SymbolParent, declaration, nameRange OffsetRange) (NormalizedSymbol, bool) {
	name = strings.TrimSpace(name)
	if name == "" || declaration.End <= declaration.Start || nameRange.End <= nameRange.Start {
		return NormalizedSymbol{}, false
	}
	signature := declaration
	symbol, err := builder.Add(SymbolSpec{Kind: kind, NativeKind: native, Name: name, Parent: parent, Declaration: declaration, NameRange: nameRange, Signature: &signature, Evidence: SymbolEvidenceStructural})
	if err == nil {
		return symbol, true
	}
	if operation.KindOf(err) != operation.KindLimit {
		builder.MarkIncomplete()
	}
	return NormalizedSymbol{}, false
}

func phase10Parent(symbol NormalizedSymbol) *SymbolParent {
	if symbol.ID == "" {
		return nil
	}
	return &SymbolParent{ID: symbol.ID, QualifiedName: symbol.QualifiedName}
}

func phase10AddDependency(document *SourceDocument, dependencies *[]StructuralDependency, kind StructuralDependencyKind, value string, start, end int) {
	value = strings.TrimSpace(value)
	if value == "" || start < 0 || end <= start || end > len(document.Text) {
		return
	}
	rangeValue, err := document.RangeFromUTF8Offsets(start, end)
	if err != nil {
		return
	}
	*dependencies = appendUniqueDependencies(*dependencies, []StructuralDependency{{Kind: kind, Value: value, Range: rangeValue, Evidence: SymbolEvidenceStructural}})
}

func phase10Diagnostic(builder *SymbolBuilder, code, message string, start, end int) {
	builder.MarkIncomplete()
	var value *OffsetRange
	if end > start && start >= 0 {
		r := OffsetRange{Start: start, End: end}
		value = &r
	}
	_ = builder.AddDiagnostic(DiagnosticSpec{Code: code, Message: message, Severity: DiagnosticWarning, Range: value, AffectsCoverage: true})
}

func phase10StripLineComment(text string, markers ...string) string {
	quote := byte(0)
	for i := 0; i < len(text); i++ {
		if quote != 0 {
			if text[i] == quote {
				if i+1 < len(text) && text[i+1] == quote {
					i++
					continue
				}
				quote = 0
			}
			continue
		}
		if text[i] == '\'' || text[i] == '"' {
			quote = text[i]
			continue
		}
		for _, marker := range markers {
			if marker != "" && strings.HasPrefix(text[i:], marker) {
				return text[:i]
			}
		}
	}
	return text
}

func phase10Unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		return value[1 : len(value)-1]
	}
	return value
}

func phase10MaskRange(masked []byte, start, end int) {
	if start < 0 {
		start = 0
	}
	if end > len(masked) {
		end = len(masked)
	}
	for index := start; index < end; index++ {
		if masked[index] != '\r' && masked[index] != '\n' {
			masked[index] = ' '
		}
	}
}

// phase10MaskComments blanks comments while preserving byte offsets and quoted
// strings. String contents remain available to language recognizers that need
// literal labels or dependency paths.
func phase10MaskComments(text string, lineMarkers []string, blockStart, blockEnd string) string {
	masked := []byte(text)
	for index := 0; index < len(text); {
		if text[index] == '\'' || text[index] == '"' {
			quote := text[index]
			index++
			for index < len(text) {
				if text[index] == '\\' {
					index += min(2, len(text)-index)
					continue
				}
				if text[index] == quote {
					if index+1 < len(text) && text[index+1] == quote {
						index += 2
						continue
					}
					index++
					break
				}
				index++
			}
			continue
		}
		matched := false
		for _, marker := range lineMarkers {
			if marker != "" && strings.HasPrefix(text[index:], marker) {
				end := index
				for end < len(text) && text[end] != '\r' && text[end] != '\n' {
					end++
				}
				phase10MaskRange(masked, index, end)
				index = end
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if blockStart != "" && blockEnd != "" && strings.HasPrefix(text[index:], blockStart) {
			end := strings.Index(text[index+len(blockStart):], blockEnd)
			if end < 0 {
				phase10MaskRange(masked, index, len(text))
				break
			}
			end += index + len(blockStart) + len(blockEnd)
			phase10MaskRange(masked, index, end)
			index = end
			continue
		}
		index++
	}
	return string(masked)
}

func phase10MaskStrings(text string, single, double, tripleDouble bool) string {
	masked := []byte(text)
	for index := 0; index < len(text); {
		if tripleDouble && strings.HasPrefix(text[index:], "\"\"\"") {
			end := strings.Index(text[index+3:], "\"\"\"")
			if end < 0 {
				phase10MaskRange(masked, index, len(text))
				break
			}
			end += index + 6
			phase10MaskRange(masked, index, end)
			index = end
			continue
		}
		quote := byte(0)
		if single && text[index] == '\'' {
			quote = '\''
		} else if double && text[index] == '"' {
			quote = '"'
		}
		if quote == 0 {
			index++
			continue
		}
		start := index
		index++
		for index < len(text) {
			if text[index] == '\\' {
				index += min(2, len(text)-index)
				continue
			}
			if text[index] == quote {
				if index+1 < len(text) && text[index+1] == quote {
					index += 2
					continue
				}
				index++
				break
			}
			index++
		}
		phase10MaskRange(masked, start, index)
	}
	return string(masked)
}

func phase10MaskDelimitedRegions(text string, regions [][2]string) string {
	masked := []byte(text)
	for index := 0; index < len(text); {
		matched := false
		for _, region := range regions {
			if region[0] == "" || region[1] == "" || !strings.HasPrefix(text[index:], region[0]) {
				continue
			}
			end := strings.Index(text[index+len(region[0]):], region[1])
			if end < 0 {
				phase10MaskRange(masked, index, len(text))
				return string(masked)
			}
			end += index + len(region[0]) + len(region[1])
			phase10MaskRange(masked, index, end)
			index = end
			matched = true
			break
		}
		if !matched {
			index++
		}
	}
	return string(masked)
}

func phase10MaskHeredocs(text string) string {
	masked := []byte(text)
	lines := phase10Lines(text)
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		marker := strings.Index(line.text, "<<")
		if marker < 0 {
			continue
		}
		cursor := marker + 2
		if cursor < len(line.text) && line.text[cursor] == '-' {
			cursor++
		}
		for cursor < len(line.text) && (line.text[cursor] == ' ' || line.text[cursor] == '\t') {
			cursor++
		}
		start := cursor
		for cursor < len(line.text) {
			value := line.text[cursor]
			if !(value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9') {
				break
			}
			cursor++
		}
		if cursor == start {
			continue
		}
		terminator := line.text[start:cursor]
		for body := index + 1; body < len(lines); body++ {
			if strings.TrimSpace(lines[body].text) == terminator {
				if body > index+1 {
					phase10MaskRange(masked, lines[index+1].start, lines[body].start)
				}
				index = body
				break
			}
		}
	}
	return string(masked)
}
