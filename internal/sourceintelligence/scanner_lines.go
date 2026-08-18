package sourceintelligence

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/operation"
)

// LogicalLine is a scanner-token view of one language-level statement line.
type LogicalLine struct {
	Tokens      []Token
	StartOffset int
	EndOffset   int
	Indent      int
}

// LogicalLineProfile defines only shared line assembly behavior. Declaration
// meaning remains with the language analyzer.
type LogicalLineProfile struct {
	Separators       []string
	TrackIndentation bool
	SkipDirectives   bool
}

func BuildLogicalLines(tokens []Token, profile LogicalLineProfile) []LogicalLine {
	separatorSet := make(map[string]struct{}, len(profile.Separators))
	for _, separator := range profile.Separators {
		if separator != "" {
			separatorSet[separator] = struct{}{}
		}
	}
	indent := 0
	lineIndent := 0
	var current []Token
	var result []LogicalLine
	flush := func() {
		if len(current) == 0 {
			return
		}
		result = append(result, LogicalLine{
			Tokens: append([]Token(nil), current...), StartOffset: current[0].StartOffset,
			EndOffset: current[len(current)-1].EndOffset, Indent: lineIndent,
		})
		current = current[:0]
	}
	for _, token := range tokens {
		switch token.Kind {
		case TokenIndent, TokenDedent:
			if profile.TrackIndentation {
				indent = token.Nesting
			}
			continue
		case TokenNewline:
			flush()
			continue
		case TokenEOF:
			flush()
			return result
		case TokenDirective:
			if profile.SkipDirectives {
				continue
			}
		}
		if _, separator := separatorSet[token.Text]; separator && token.Nesting == 0 {
			flush()
			continue
		}
		if len(current) == 0 {
			lineIndent = indent
		}
		current = append(current, token)
	}
	flush()
	return result
}

// KeywordScopeEvent represents an analyzer-proven keyword scope open/close.
type KeywordScopeEvent struct {
	Line  int
	Label string
	Open  bool
}

type KeywordScopePairing struct {
	Pairs     map[int]int
	Unmatched []int
}

// PairKeywordScopes pairs only the current top scope. It deliberately refuses
// to search through intervening scopes because malformed source must not be
// silently re-parented.
func PairKeywordScopes(events []KeywordScopeEvent, caseInsensitive bool) KeywordScopePairing {
	result := KeywordScopePairing{Pairs: make(map[int]int)}
	var stack []int
	equal := func(left, right string) bool {
		left = strings.TrimSpace(left)
		right = strings.TrimSpace(right)
		if caseInsensitive {
			return strings.EqualFold(left, right)
		}
		return left == right
	}
	for index, event := range events {
		if strings.TrimSpace(event.Label) == "" {
			result.Unmatched = append(result.Unmatched, index)
			continue
		}
		if event.Open {
			stack = append(stack, index)
			continue
		}
		if len(stack) == 0 || !equal(events[stack[len(stack)-1]].Label, event.Label) {
			result.Unmatched = append(result.Unmatched, index)
			continue
		}
		open := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		result.Pairs[open] = index
	}
	result.Unmatched = append(result.Unmatched, stack...)
	return result
}

type LineModelKind string

const (
	LineModelFree  LineModelKind = "free"
	LineModelFixed LineModelKind = "fixed"
)

type FixedLineProfile struct {
	CommentColumnOne   []string
	LabelStartColumn   int
	LabelEndColumn     int
	ContinuationColumn int
	CodeStartColumn    int
	CodeEndColumn      int
}

type LineModelProfile struct {
	Kind  LineModelKind
	Fixed FixedLineProfile
}

type LineModelLimits struct {
	MaxLines     int
	MaxLineBytes int
}

type SourceLine struct {
	Physical     OffsetRange
	Code         OffsetRange
	Label        OffsetRange
	Comment      bool
	Continuation bool
}

// BuildSourceLines builds bounded physical-line metadata in decoded UTF-8 byte
// offsets. Fixed-format columns count Unicode scalar values, matching the public
// decoded source model rather than original encoded bytes.
func BuildSourceLines(ctx context.Context, document *SourceDocument, profile LineModelProfile, limits LineModelLimits) ([]SourceLine, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil || !utf8.ValidString(document.Text) {
		return nil, operation.New(operation.KindInvalidInput, "valid UTF-8 source document is required")
	}
	if limits.MaxLines <= 0 || limits.MaxLineBytes <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "line-model limits must be positive")
	}
	if profile.Kind != LineModelFree && profile.Kind != LineModelFixed {
		return nil, operation.New(operation.KindInvalidInput, "line model must be free or fixed")
	}
	if profile.Kind == LineModelFixed {
		fixed := profile.Fixed
		if fixed.LabelStartColumn <= 0 || fixed.LabelEndColumn < fixed.LabelStartColumn || fixed.ContinuationColumn <= 0 || fixed.CodeStartColumn <= 0 || fixed.CodeEndColumn < fixed.CodeStartColumn {
			return nil, operation.New(operation.KindInvalidInput, "fixed line columns are invalid")
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, operation.Wrap(operation.KindCancelled, "build_source_lines", document.Path, err)
	}

	text := document.Text
	var lines []SourceLine
	for start := 0; start < len(text); {
		if len(lines) >= limits.MaxLines {
			return nil, operation.Wrap(operation.KindLimit, "build_source_lines", document.Path, fmt.Errorf("line count exceeds limit %d", limits.MaxLines))
		}
		if len(lines)&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, operation.Wrap(operation.KindCancelled, "build_source_lines", document.Path, err)
			}
		}
		end := start
		for end < len(text) && !isNewlineStart(text[end]) {
			end++
		}
		if end-start > limits.MaxLineBytes {
			return nil, operation.Wrap(operation.KindLimit, "build_source_lines", document.Path, fmt.Errorf("line bytes %d exceeds limit %d", end-start, limits.MaxLineBytes))
		}
		line := SourceLine{Physical: OffsetRange{Start: start, End: end}, Code: OffsetRange{Start: start, End: end}}
		if profile.Kind == LineModelFixed {
			line = buildFixedSourceLine(text, start, end, profile.Fixed)
		}
		lines = append(lines, line)
		if end >= len(text) {
			break
		}
		if text[end] == '\r' && end+1 < len(text) && text[end+1] == '\n' {
			start = end + 2
		} else {
			start = end + 1
		}
	}
	if len(text) == 0 {
		return nil, nil
	}
	return lines, nil
}

func buildFixedSourceLine(text string, start, end int, profile FixedLineProfile) SourceLine {
	lineText := text[start:end]
	result := SourceLine{Physical: OffsetRange{Start: start, End: end}}
	firstRune, _ := utf8.DecodeRuneInString(lineText)
	if lineText != "" {
		first := string(firstRune)
		for _, marker := range profile.CommentColumnOne {
			if first == marker {
				result.Comment = true
				result.Code = OffsetRange{Start: end, End: end}
				return result
			}
		}
	}
	labelStart := start + scalarColumnOffset(lineText, profile.LabelStartColumn)
	labelEnd := start + scalarColumnOffset(lineText, profile.LabelEndColumn)
	result.Label = trimHorizontalRange(text, OffsetRange{Start: labelStart, End: labelEnd})
	continuationStart := start + scalarColumnOffset(lineText, profile.ContinuationColumn)
	continuationEnd := start + scalarColumnOffset(lineText, profile.ContinuationColumn+1)
	if continuationStart < continuationEnd {
		value := text[continuationStart:continuationEnd]
		result.Continuation = strings.TrimSpace(value) != "" && value != "0"
	}
	codeStart := start + scalarColumnOffset(lineText, profile.CodeStartColumn)
	codeEnd := start + scalarColumnOffset(lineText, profile.CodeEndColumn)
	result.Code = trimHorizontalRange(text, OffsetRange{Start: codeStart, End: codeEnd})
	return result
}

func scalarColumnOffset(text string, column int) int {
	if column <= 1 {
		return 0
	}
	currentColumn := 1
	for offset := range text {
		if currentColumn == column {
			return offset
		}
		currentColumn++
	}
	return len(text)
}

func trimHorizontalRange(text string, value OffsetRange) OffsetRange {
	if value.Start < 0 || value.End < value.Start || value.End > len(text) {
		return OffsetRange{}
	}
	for value.Start < value.End && isHorizontalSpace(text[value.Start]) {
		value.Start++
	}
	for value.End > value.Start && isHorizontalSpace(text[value.End-1]) {
		value.End--
	}
	return value
}

type LineLabelStyle string

const (
	LineLabelFixedField LineLabelStyle = "fixed-field"
	LineLabelColon      LineLabelStyle = "colon"
)

type LineLabelProfile struct {
	Style      LineLabelStyle
	Identifier IdentifierPolicy
}

type LineLabel struct {
	Name  string
	Range OffsetRange
}

func RecognizeLineLabel(document *SourceDocument, line SourceLine, profile LineLabelProfile) (LineLabel, bool) {
	if document == nil || line.Physical.Start < 0 || line.Physical.End > len(document.Text) || line.Physical.End < line.Physical.Start {
		return LineLabel{}, false
	}
	switch profile.Style {
	case LineLabelFixedField:
		value := trimHorizontalRange(document.Text, line.Label)
		if value.End <= value.Start {
			return LineLabel{}, false
		}
		name := document.Text[value.Start:value.End]
		for _, current := range name {
			if current < '0' || current > '9' {
				return LineLabel{}, false
			}
		}
		return LineLabel{Name: name, Range: value}, true
	case LineLabelColon:
		value := trimHorizontalRange(document.Text, line.Code)
		if value.End <= value.Start {
			return LineLabel{}, false
		}
		offset := value.Start
		first, size := utf8.DecodeRuneInString(document.Text[offset:value.End])
		if !identifierStart(profile.Identifier, first) {
			return LineLabel{}, false
		}
		offset += size
		for offset < value.End {
			current, runeSize := utf8.DecodeRuneInString(document.Text[offset:value.End])
			if !identifierContinue(profile.Identifier, current) {
				break
			}
			offset += runeSize
		}
		nameEnd := offset
		for offset < value.End && isHorizontalSpace(document.Text[offset]) {
			offset++
		}
		if offset >= value.End || document.Text[offset] != ':' {
			return LineLabel{}, false
		}
		return LineLabel{Name: document.Text[value.Start:nameEnd], Range: OffsetRange{Start: value.Start, End: nameEnd}}, true
	default:
		return LineLabel{}, false
	}
}
