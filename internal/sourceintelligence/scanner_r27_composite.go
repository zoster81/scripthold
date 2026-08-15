package sourceintelligence

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/operation"
)

type CompositeDelimiterRule struct {
	Open     string
	Close    string
	Kind     string
	Language string
}

type CompositeProfile struct {
	HostKind     string
	HostLanguage string
	Rules        []CompositeDelimiterRule
}

type CompositeSegment struct {
	Kind     string
	Language string
	Full     OffsetRange
	Content  OffsetRange
}

// SegmentCompositeSource splits one decoded document without copying embedded
// text. Every range remains in the original decoded UTF-8 coordinate space.
func SegmentCompositeSource(ctx context.Context, document *SourceDocument, profile CompositeProfile, maxSegments int) ([]CompositeSegment, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil || !utf8.ValidString(document.Text) {
		return nil, false, operation.New(operation.KindInvalidInput, "valid UTF-8 source document is required")
	}
	if strings.TrimSpace(profile.HostKind) == "" || len(profile.Rules) == 0 || len(profile.Rules) > 32 || maxSegments <= 0 {
		return nil, false, operation.New(operation.KindInvalidInput, "composite profile and segment limit are invalid")
	}
	seen := make(map[string]struct{}, len(profile.Rules))
	for _, rule := range profile.Rules {
		if rule.Open == "" || rule.Close == "" || rule.Kind == "" || len(rule.Open) > 128 || len(rule.Close) > 128 || !utf8.ValidString(rule.Open) || !utf8.ValidString(rule.Close) {
			return nil, false, operation.New(operation.KindInvalidInput, "composite delimiters require bounded non-empty UTF-8 open/close/kind values")
		}
		if _, duplicate := seen[rule.Open]; duplicate {
			return nil, false, operation.New(operation.KindInvalidInput, "composite opening delimiters must be unique")
		}
		seen[rule.Open] = struct{}{}
	}
	if err := ctx.Err(); err != nil {
		return nil, false, operation.Wrap(operation.KindCancelled, "segment_composite_source", document.Path, err)
	}

	text := document.Text
	segments := make([]CompositeSegment, 0, min(maxSegments, 16))
	appendSegment := func(segment CompositeSegment) error {
		if segment.Full.End <= segment.Full.Start {
			return nil
		}
		if len(segments)+1 > maxSegments {
			return operation.Wrap(operation.KindLimit, "segment_composite_source", document.Path, fmt.Errorf("segment count exceeds limit %d", maxSegments))
		}
		segments = append(segments, segment)
		return nil
	}
	position := 0
	complete := true
	for position < len(text) {
		if err := ctx.Err(); err != nil {
			return nil, false, operation.Wrap(operation.KindCancelled, "segment_composite_source", document.Path, err)
		}
		next, ruleIndex, err := nextCompositeOpening(ctx, text, position, profile.Rules)
		if err != nil {
			return nil, false, operation.Wrap(operation.KindCancelled, "segment_composite_source", document.Path, err)
		}
		if next < 0 {
			if err := appendSegment(CompositeSegment{Kind: profile.HostKind, Language: profile.HostLanguage, Full: OffsetRange{Start: position, End: len(text)}, Content: OffsetRange{Start: position, End: len(text)}}); err != nil {
				return nil, false, err
			}
			break
		}
		if next > position {
			if err := appendSegment(CompositeSegment{Kind: profile.HostKind, Language: profile.HostLanguage, Full: OffsetRange{Start: position, End: next}, Content: OffsetRange{Start: position, End: next}}); err != nil {
				return nil, false, err
			}
		}
		rule := profile.Rules[ruleIndex]
		contentStart := next + len(rule.Open)
		closeAt, err := findCompositeClose(ctx, text, contentStart, rule.Close)
		if err != nil {
			return nil, false, operation.Wrap(operation.KindCancelled, "segment_composite_source", document.Path, err)
		}
		if closeAt < 0 {
			complete = false
			if err := appendSegment(CompositeSegment{Kind: rule.Kind, Language: rule.Language, Full: OffsetRange{Start: next, End: len(text)}, Content: OffsetRange{Start: contentStart, End: len(text)}}); err != nil {
				return nil, false, err
			}
			break
		}
		end := closeAt + len(rule.Close)
		if err := appendSegment(CompositeSegment{Kind: rule.Kind, Language: rule.Language, Full: OffsetRange{Start: next, End: end}, Content: OffsetRange{Start: contentStart, End: closeAt}}); err != nil {
			return nil, false, err
		}
		position = end
	}
	return segments, complete, nil
}

func nextCompositeOpening(ctx context.Context, text string, start int, rules []CompositeDelimiterRule) (int, int, error) {
	for offset := start; offset < len(text); offset++ {
		if offset&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return -1, -1, err
			}
		}
		bestRule := -1
		bestLength := -1
		for index, rule := range rules {
			if len(rule.Open) <= bestLength || !strings.HasPrefix(text[offset:], rule.Open) {
				continue
			}
			bestRule = index
			bestLength = len(rule.Open)
		}
		if bestRule >= 0 {
			return offset, bestRule, nil
		}
	}
	return -1, -1, nil
}

func findCompositeClose(ctx context.Context, text string, start int, close string) (int, error) {
	for offset := start; offset+len(close) <= len(text); offset++ {
		if offset&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return -1, err
			}
		}
		if strings.HasPrefix(text[offset:], close) {
			return offset, nil
		}
	}
	return -1, nil
}

// MaskOutsideRanges replaces bytes outside approved decoded-source ranges with
// ASCII spaces while preserving CR/LF bytes and exact byte offsets. Since kept
// ranges must start/end on UTF-8 rune boundaries, the result remains valid UTF-8.
func MaskOutsideRanges(text string, keep []OffsetRange) (string, error) {
	if !utf8.ValidString(text) {
		return "", operation.New(operation.KindInvalidInput, "mask source must be valid UTF-8")
	}
	ranges := append([]OffsetRange(nil), keep...)
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start != ranges[j].Start {
			return ranges[i].Start < ranges[j].Start
		}
		return ranges[i].End < ranges[j].End
	})
	previousEnd := 0
	for _, value := range ranges {
		if value.Start < previousEnd || value.Start < 0 || value.End < value.Start || value.End > len(text) || !utf8Boundary(text, value.Start) || !utf8Boundary(text, value.End) {
			return "", operation.New(operation.KindInvalidInput, "mask ranges must be ordered non-overlapping UTF-8 boundaries")
		}
		previousEnd = value.End
	}

	masked := []byte(text)
	for index := range masked {
		if masked[index] != '\r' && masked[index] != '\n' {
			masked[index] = ' '
		}
	}
	for _, value := range ranges {
		copy(masked[value.Start:value.End], text[value.Start:value.End])
	}
	return string(masked), nil
}

func utf8Boundary(text string, offset int) bool {
	return offset == 0 || offset == len(text) || offset > 0 && offset < len(text) && text[offset]&0xc0 != 0x80
}
