package markdownintelligence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/zoster81/marksplice"
)

const maxTargetScanNodes = 100_000

// Range is a half-open byte range in the exact BOM-free UTF-8 snapshot passed
// to Marksplice. Its semantic meaning is defined by Marksplice's public range
// accessor for the corresponding node kind.
type Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// NodeSummary is Scripthold's transport-neutral projection of one Marksplice
// public node. TargetID is derived only from the exact snapshot fingerprint and
// Marksplice-owned public kind/range metadata; Marksplice NodeID is never
// serialized or treated as durable identity.
type NodeSummary struct {
	TargetID   string         `json:"targetId"`
	Kind       string         `json:"kind"`
	Range      Range          `json:"range"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// Snapshot owns one immutable Marksplice document together with the exact
// BOM-free UTF-8 source bytes from which it was parsed.
type Snapshot struct {
	document    *marksplice.Document
	source      []byte
	fingerprint string

	nodeTargetIndexOnce sync.Once
	nodeTargetIndex     map[string]resolvedNodeTarget
	nodeTargetIndexErr  error

	sectionTargetIndexOnce sync.Once
	sectionTargetIndex     map[string]marksplice.NodeID
	sectionTargetIndexErr  error
}

// Parse creates an immutable Markdown snapshot using Marksplice as the sole
// parser and semantic authority.
func Parse(source []byte) (*Snapshot, error) {
	document, err := marksplice.Parse(source)
	if err != nil {
		return nil, err
	}
	owned := append([]byte(nil), source...)
	digest := sha256.Sum256(owned)
	return &Snapshot{
		document:    document,
		source:      owned,
		fingerprint: hex.EncodeToString(digest[:]),
	}, nil
}

// SourceFingerprint returns the SHA-256 fingerprint of the exact UTF-8 source
// snapshot supplied to Marksplice.
func (s *Snapshot) SourceFingerprint() string {
	if s == nil {
		return ""
	}
	return s.fingerprint
}

// QueryNodes returns at most limit promoted Marksplice nodes in source order.
// Marksplice remains authoritative for query validation and structural ranges.
func (s *Snapshot) QueryNodes(kinds []string, limit int) ([]NodeSummary, error) {
	if s == nil || s.document == nil {
		return nil, fmt.Errorf("%w: markdown snapshot is unavailable", marksplice.ErrInvalidQuery)
	}
	markspliceKinds, err := parseKinds(kinds)
	if err != nil {
		return nil, err
	}
	matches, err := s.document.QueryNodes(marksplice.NodeQuery{Kinds: markspliceKinds, Limit: limit})
	if err != nil {
		return nil, err
	}
	result := make([]NodeSummary, 0, len(matches))
	for _, match := range matches {
		result = append(result, s.summarize(match))
	}
	return result, nil
}

// Target resolves one opaque target ID against this exact source snapshot. The
// bounded scan is necessary because Marksplice NodeID is intentionally not a
// persistence or round-trip format.
func (s *Snapshot) Target(targetID string) (NodeSummary, bool, error) {
	resolved, ok, err := s.resolveNodeTarget(targetID)
	if err != nil || !ok {
		return NodeSummary{}, ok, err
	}
	return s.summarizeNode(resolved.node, resolved.rangeValue), true, nil
}

// Source returns a caller-owned copy of an exact Marksplice-backed source range.
func (s *Snapshot) Source(r Range) ([]byte, bool) {
	if s == nil || s.document == nil {
		return nil, false
	}
	return s.document.SourceRange(marksplice.Range{Start: r.Start, End: r.End})
}

func (s *Snapshot) summarize(match marksplice.NodeMatch) NodeSummary {
	return s.summarizeNode(match.Node(), match.Range())
}

func (s *Snapshot) summarizeNode(node marksplice.Node, rangeValue marksplice.Range) NodeSummary {
	kind := kindName(node.Kind())
	return NodeSummary{
		TargetID:   targetID(s.fingerprint, kind, rangeValue),
		Kind:       kind,
		Range:      Range{Start: rangeValue.Start, End: rangeValue.End},
		Attributes: s.nodeAttributes(node),
	}
}

func (s *Snapshot) nodeAttributes(node marksplice.Node) map[string]any {
	id := node.ID()
	attributes := make(map[string]any)
	switch node.Kind() {
	case marksplice.KindHeading:
		if value, ok := s.document.Heading(id); ok {
			attributes["text"] = value.Text()
			attributes["level"] = value.Level()
			attributes["style"] = headingStyleName(value.Style())
		}
	case marksplice.KindListItem:
		if value, ok := s.document.ListItem(id); ok {
			attributes["ordered"] = value.Ordered()
			attributes["marker"] = string([]byte{value.Marker()})
			attributes["hasChildren"] = value.HasChildren()
			if subtree, ok := value.SubtreeRange(); ok {
				attributes["subtreeRange"] = Range{Start: subtree.Start, End: subtree.End}
			}
		}
	case marksplice.KindTask:
		if value, ok := s.document.Task(id); ok {
			attributes["checked"] = value.Checked()
		}
	case marksplice.KindTable:
		if value, ok := s.document.Table(id); ok {
			attributes["columnCount"] = value.ColumnCount()
			attributes["bodyRowCount"] = value.BodyRowCount()
		}
	case marksplice.KindTableRow:
		if value, ok := s.document.TableRow(id); ok {
			attributes["columnCount"] = value.ColumnCount()
		}
	case marksplice.KindTableCell:
		if value, ok := s.document.TableCell(id); ok {
			attributes["column"] = value.Column()
			attributes["header"] = value.Header()
		}
	case marksplice.KindFencedCode:
		if value, ok := s.document.FencedBlock(id); ok {
			attributes["closed"] = value.Closed()
			attributes["fence"] = string([]byte{value.FenceChar()})
			attributes["openingFenceLength"] = value.OpeningFenceLength()
			attributes["openingIndent"] = value.OpeningIndent()
			if info, ok := value.Info(); ok {
				attributes["info"] = info
			}
			if language, ok := value.Language(); ok {
				attributes["language"] = language
			}
		}
	case marksplice.KindInlineLink:
		if value, ok := s.document.InlineLink(id); ok {
			attributes["destination"] = value.Destination()
			if title, ok := value.Title(); ok {
				attributes["title"] = title
			}
		}
	case marksplice.KindReferenceDefinition:
		if value, ok := s.document.ReferenceDefinition(id); ok {
			attributes["label"] = value.Label()
			attributes["destination"] = value.Destination()
			if title, ok := value.Title(); ok {
				attributes["title"] = title
			}
		}
	case marksplice.KindAutoLink:
		if value, ok := s.document.AutoLink(id); ok {
			attributes["value"] = value.Value()
			attributes["email"] = value.IsEmail()
		}
	case marksplice.KindFrontMatterField:
		if value, ok := s.document.FrontMatterField(id); ok {
			attributes["key"] = value.Key()
			attributes["format"] = frontMatterFormatName(value.Format())
		}
	case marksplice.KindHTMLAnchor:
		if value, ok := s.document.HTMLAnchor(id); ok {
			attributes["attribute"] = htmlAnchorAttributeName(value.Attribute())
		}
	case marksplice.KindBlockquote:
		if alert, ok := s.document.Alert(id); ok {
			attributes["alertKind"] = alertKindName(alert.Kind())
		}
	case marksplice.KindFootnoteDefinition:
		if value, ok := s.document.FootnoteDefinition(id); ok {
			attributes["label"] = value.Label()
		}
	case marksplice.KindMathExpression:
		if value, ok := s.document.MathExpression(id); ok {
			attributes["style"] = mathExpressionStyleName(value.Style())
		}
	}
	if len(attributes) == 0 {
		return nil
	}
	return attributes
}

func targetID(sourceFingerprint, kind string, r marksplice.Range) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("scripthold-markdown-target-v1\x00"))
	_, _ = hasher.Write([]byte(sourceFingerprint))
	_, _ = hasher.Write([]byte{'\x00'})
	_, _ = hasher.Write([]byte(kind))
	_, _ = hasher.Write([]byte{'\x00'})
	_, _ = hasher.Write(strconv.AppendInt(nil, int64(r.Start), 10))
	_, _ = hasher.Write([]byte{'\x00'})
	_, _ = hasher.Write(strconv.AppendInt(nil, int64(r.End), 10))
	return hex.EncodeToString(hasher.Sum(nil))
}

func validTargetID(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func parseKinds(values []string) ([]marksplice.Kind, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make([]marksplice.Kind, 0, len(values))
	seen := make(map[marksplice.Kind]struct{}, len(values))
	for _, value := range values {
		kind, ok := parseKind(value)
		if !ok {
			return nil, fmt.Errorf("%w: unsupported markdown node kind %q", marksplice.ErrInvalidQuery, value)
		}
		if _, duplicate := seen[kind]; duplicate {
			continue
		}
		seen[kind] = struct{}{}
		result = append(result, kind)
	}
	return result, nil
}

func parseKind(value string) (marksplice.Kind, bool) {
	switch strings.TrimSpace(value) {
	case "paragraph":
		return marksplice.KindParagraph, true
	case "heading":
		return marksplice.KindHeading, true
	case "list_item":
		return marksplice.KindListItem, true
	case "task":
		return marksplice.KindTask, true
	case "table_cell":
		return marksplice.KindTableCell, true
	case "fenced_code":
		return marksplice.KindFencedCode, true
	case "strikethrough":
		return marksplice.KindStrikethrough, true
	case "code_span":
		return marksplice.KindCodeSpan, true
	case "emphasis":
		return marksplice.KindEmphasis, true
	case "strong":
		return marksplice.KindStrong, true
	case "inline_link":
		return marksplice.KindInlineLink, true
	case "reference_definition":
		return marksplice.KindReferenceDefinition, true
	case "autolink":
		return marksplice.KindAutoLink, true
	case "front_matter_field":
		return marksplice.KindFrontMatterField, true
	case "html_comment":
		return marksplice.KindHTMLComment, true
	case "html_anchor":
		return marksplice.KindHTMLAnchor, true
	case "image":
		return marksplice.KindImage, true
	case "table_row":
		return marksplice.KindTableRow, true
	case "table":
		return marksplice.KindTable, true
	case "thematic_break":
		return marksplice.KindThematicBreak, true
	case "blockquote":
		return marksplice.KindBlockquote, true
	case "footnote_definition":
		return marksplice.KindFootnoteDefinition, true
	case "math_expression":
		return marksplice.KindMathExpression, true
	default:
		return marksplice.KindUnknown, false
	}
}

func kindName(kind marksplice.Kind) string {
	switch kind {
	case marksplice.KindParagraph:
		return "paragraph"
	case marksplice.KindHeading:
		return "heading"
	case marksplice.KindListItem:
		return "list_item"
	case marksplice.KindTask:
		return "task"
	case marksplice.KindTableCell:
		return "table_cell"
	case marksplice.KindFencedCode:
		return "fenced_code"
	case marksplice.KindStrikethrough:
		return "strikethrough"
	case marksplice.KindCodeSpan:
		return "code_span"
	case marksplice.KindEmphasis:
		return "emphasis"
	case marksplice.KindStrong:
		return "strong"
	case marksplice.KindInlineLink:
		return "inline_link"
	case marksplice.KindReferenceDefinition:
		return "reference_definition"
	case marksplice.KindAutoLink:
		return "autolink"
	case marksplice.KindFrontMatterField:
		return "front_matter_field"
	case marksplice.KindHTMLComment:
		return "html_comment"
	case marksplice.KindHTMLAnchor:
		return "html_anchor"
	case marksplice.KindImage:
		return "image"
	case marksplice.KindTableRow:
		return "table_row"
	case marksplice.KindTable:
		return "table"
	case marksplice.KindThematicBreak:
		return "thematic_break"
	case marksplice.KindBlockquote:
		return "blockquote"
	case marksplice.KindFootnoteDefinition:
		return "footnote_definition"
	case marksplice.KindMathExpression:
		return "math_expression"
	default:
		return "unknown"
	}
}

func headingStyleName(style marksplice.HeadingStyle) string {
	switch style {
	case marksplice.HeadingStyleATX:
		return "atx"
	case marksplice.HeadingStyleSetext:
		return "setext"
	default:
		return "unknown"
	}
}

func frontMatterFormatName(format marksplice.FrontMatterFormat) string {
	switch format {
	case marksplice.FrontMatterFormatYAML:
		return "yaml"
	case marksplice.FrontMatterFormatTOML:
		return "toml"
	default:
		return "unknown"
	}
}

func htmlAnchorAttributeName(attribute marksplice.HTMLAnchorAttribute) string {
	switch attribute {
	case marksplice.HTMLAnchorAttributeID:
		return "id"
	case marksplice.HTMLAnchorAttributeName:
		return "name"
	default:
		return "unknown"
	}
}

func alertKindName(kind marksplice.AlertKind) string {
	switch kind {
	case marksplice.AlertKindNote:
		return "note"
	case marksplice.AlertKindTip:
		return "tip"
	case marksplice.AlertKindImportant:
		return "important"
	case marksplice.AlertKindWarning:
		return "warning"
	case marksplice.AlertKindCaution:
		return "caution"
	default:
		return "unknown"
	}
}

func mathExpressionStyleName(style marksplice.MathExpressionStyle) string {
	switch style {
	case marksplice.MathExpressionInlineDollar:
		return "inline_dollar"
	case marksplice.MathExpressionInlineBacktick:
		return "inline_backtick"
	case marksplice.MathExpressionBlockDollar:
		return "block_dollar"
	case marksplice.MathExpressionFencedBlock:
		return "fenced_block"
	default:
		return "unknown"
	}
}
