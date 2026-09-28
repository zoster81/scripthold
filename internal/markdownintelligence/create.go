package markdownintelligence

import (
	"fmt"
	"strings"

	"github.com/zoster81/marksplice"
	"github.com/zoster81/scripthold/internal/operation"
)

const (
	MaxCreateElements = 4096
	MaxCreateDepth    = 64
)

type CreateLimits struct {
	MaxElements  int
	MaxDepth     int
	MaxTextBytes int64
}

type CreateDocument struct {
	FrontMatter *CreateFrontMatter
	Blocks      []CreateBlock
}

type CreateFrontMatter struct {
	Format string
	Fields []CreateFrontMatterField
}

type CreateFrontMatterField struct {
	Key   string
	Value string
}

type CreateBlock struct {
	Type        string
	Level       int
	Content     []CreateInline
	RawMarkdown string
	Depth       int
	Kind        string
	Blocks      []CreateBlock
	Ordered     bool
	Items       []CreateListItem
	Code        string
	Info        string
	Label       string
	Destination string
	Title       string
	Deferred    bool
	Body        string
	Payload     string
	Header      []string
	Rows        [][]string
	Alignments  []string
}

type CreateListItem struct {
	Markdown string
	Checked  bool
	Depth    int
}

type CreateInline struct {
	Type        string
	Text        string
	Children    []CreateInline
	Destination string
	Title       string
	Value       string
	Reference   string
	Label       string
	Payload     string
}

// BuildDocument converts Scripthold's closed construction model into the
// released Marksplice DocumentBuilder API. Markdown syntax and semantic
// validation remain entirely Marksplice-owned.
func BuildDocument(document CreateDocument, limits CreateLimits) ([]byte, error) {
	state := createState{limits: normalizeCreateLimits(limits)}
	if err := state.measureDocument(document, 0); err != nil {
		return nil, err
	}
	builder := marksplice.NewDocumentBuilder()
	if document.FrontMatter != nil {
		fields := make([]marksplice.FrontMatterFieldInput, len(document.FrontMatter.Fields))
		for i, field := range document.FrontMatter.Fields {
			fields[i] = marksplice.FrontMatterFieldInput{Key: field.Key, Value: field.Value}
		}
		var err error
		switch document.FrontMatter.Format {
		case "yaml":
			err = builder.SetYAMLFrontMatter(fields...)
		case "toml":
			err = builder.SetTOMLFrontMatter(fields...)
		default:
			return nil, invalidConstruction("front matter format must be yaml or toml")
		}
		if err != nil {
			return nil, err
		}
	}
	if err := state.appendBlocks(builder, document.Blocks); err != nil {
		return nil, err
	}
	return builder.Markdown()
}

type createState struct {
	limits    CreateLimits
	elements  int
	textBytes int64
}

func normalizeCreateLimits(limits CreateLimits) CreateLimits {
	if limits.MaxElements <= 0 || limits.MaxElements > MaxCreateElements {
		limits.MaxElements = MaxCreateElements
	}
	if limits.MaxDepth <= 0 || limits.MaxDepth > MaxCreateDepth {
		limits.MaxDepth = MaxCreateDepth
	}
	return limits
}

func (s *createState) addElement(depth int) error {
	if depth > s.limits.MaxDepth {
		return operation.New(operation.KindLimit, fmt.Sprintf("markdown construction depth exceeds limit %d", s.limits.MaxDepth))
	}
	s.elements++
	if s.elements > s.limits.MaxElements {
		return operation.New(operation.KindLimit, fmt.Sprintf("markdown construction elements exceed limit %d", s.limits.MaxElements))
	}
	return nil
}

func (s *createState) addText(value string) error {
	if value == "" {
		return nil
	}
	if int64(len(value)) > int64(^uint64(0)>>1)-s.textBytes {
		return operation.New(operation.KindLimit, "markdown construction text size exceeds supported range")
	}
	s.textBytes += int64(len(value))
	if s.limits.MaxTextBytes > 0 && s.textBytes > s.limits.MaxTextBytes {
		return operation.New(operation.KindLimit, fmt.Sprintf("markdown construction text exceeds byte limit %d", s.limits.MaxTextBytes))
	}
	return nil
}

func (s *createState) measureDocument(document CreateDocument, depth int) error {
	if document.FrontMatter != nil {
		if err := s.addElement(depth + 1); err != nil {
			return err
		}
		for _, field := range document.FrontMatter.Fields {
			if err := s.addElement(depth + 2); err != nil {
				return err
			}
			if err := s.addText(field.Key); err != nil {
				return err
			}
			if err := s.addText(field.Value); err != nil {
				return err
			}
		}
	}
	return s.measureBlocks(document.Blocks, depth+1)
}

func (s *createState) measureBlocks(blocks []CreateBlock, depth int) error {
	for _, block := range blocks {
		if err := s.measureBlock(block, depth); err != nil {
			return err
		}
	}
	return nil
}

func (s *createState) measureBlock(block CreateBlock, depth int) error {
	if err := s.addElement(depth); err != nil {
		return err
	}
	if err := s.measureTextValues(
		block.RawMarkdown, block.Kind, block.Code, block.Info, block.Label,
		block.Destination, block.Title, block.Body, block.Payload,
	); err != nil {
		return err
	}
	if err := s.measureListItems(block.Items, depth+1); err != nil {
		return err
	}
	if err := s.measureTable(block, depth+1); err != nil {
		return err
	}
	if err := s.measureInline(block.Content, depth+1); err != nil {
		return err
	}
	return s.measureBlocks(block.Blocks, depth+1)
}

func (s *createState) measureTextValues(values ...string) error {
	for _, value := range values {
		if err := s.addText(value); err != nil {
			return err
		}
	}
	return nil
}

func (s *createState) measureListItems(items []CreateListItem, depth int) error {
	for _, item := range items {
		if err := s.addElement(depth); err != nil {
			return err
		}
		if err := s.addText(item.Markdown); err != nil {
			return err
		}
	}
	return nil
}

func (s *createState) measureTable(block CreateBlock, depth int) error {
	for _, cell := range block.Header {
		if err := s.addElement(depth); err != nil {
			return err
		}
		if err := s.addText(cell); err != nil {
			return err
		}
	}
	for range block.Alignments {
		if err := s.addElement(depth); err != nil {
			return err
		}
	}
	for _, row := range block.Rows {
		if err := s.addElement(depth); err != nil {
			return err
		}
		for _, cell := range row {
			if err := s.addElement(depth + 1); err != nil {
				return err
			}
			if err := s.addText(cell); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *createState) measureInline(values []CreateInline, depth int) error {
	for _, value := range values {
		if err := s.addElement(depth); err != nil {
			return err
		}
		for _, text := range []string{value.Text, value.Destination, value.Title, value.Value, value.Reference, value.Label, value.Payload} {
			if err := s.addText(text); err != nil {
				return err
			}
		}
		if err := s.measureInline(value.Children, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (s *createState) appendBlocks(builder *marksplice.DocumentBuilder, blocks []CreateBlock) error {
	for _, block := range blocks {
		if !block.Deferred {
			continue
		}
		switch block.Type {
		case "reference_definition":
			var err error
			if block.Title == "" {
				err = builder.DeferReferenceDefinition(block.Label, block.Destination)
			} else {
				err = builder.DeferReferenceDefinitionWithTitle(block.Label, block.Destination, block.Title)
			}
			if err != nil {
				return err
			}
		case "footnote_definition":
			if err := appendFootnoteDefinition(builder, block.Label, block.Body, true); err != nil {
				return err
			}
		}
	}
	for _, block := range blocks {
		if block.Deferred && (block.Type == "reference_definition" || block.Type == "footnote_definition") {
			continue
		}
		if err := s.appendBlock(builder, block); err != nil {
			return err
		}
	}
	return nil
}

func (s *createState) appendBlock(builder *marksplice.DocumentBuilder, block CreateBlock) error {
	switch block.Type {
	case "heading":
		return s.appendHeading(builder, block)
	case "paragraph":
		return s.appendParagraph(builder, block)
	case "thematic_break":
		return builder.AppendThematicBreak()
	case "blockquote":
		return s.appendBlockquote(builder, block)
	case "alert":
		return s.appendAlert(builder, block)
	case "list":
		return appendList(builder, block)
	case "task_list":
		return appendTaskList(builder, block)
	case "fenced_code":
		return builder.AppendFencedCode(block.Code, block.Info)
	case "reference_definition":
		return appendReferenceDefinition(builder, block)
	case "footnote_definition":
		return appendFootnoteDefinition(builder, block.Label, block.Body, false)
	case "math_block":
		return builder.AppendMathBlock(block.Payload)
	case "table":
		return appendTable(builder, block)
	default:
		return invalidConstruction(fmt.Sprintf("unsupported construction block %q", block.Type))
	}
}

func appendFootnoteDefinition(builder *marksplice.DocumentBuilder, label, body string, deferred bool) error {
	multiline := strings.Contains(body, "\n")
	if deferred {
		if multiline {
			return builder.DeferFootnoteDefinitionMultiline(label, body)
		}
		return builder.DeferFootnoteDefinition(label, body)
	}
	if multiline {
		return builder.AppendFootnoteDefinitionMultiline(label, body)
	}
	return builder.AppendFootnoteDefinition(label, body)
}

func (s *createState) appendHeading(builder *marksplice.DocumentBuilder, block CreateBlock) error {
	if block.RawMarkdown != "" {
		return builder.AppendHeading(block.Level, block.RawMarkdown)
	}
	content, err := s.inlineValues(block.Content)
	if err != nil {
		return err
	}
	return builder.AppendHeadingContent(block.Level, content...)
}

func (s *createState) appendParagraph(builder *marksplice.DocumentBuilder, block CreateBlock) error {
	if block.RawMarkdown != "" {
		return builder.AppendParagraph(block.RawMarkdown)
	}
	content, err := s.inlineValues(block.Content)
	if err != nil {
		return err
	}
	return builder.AppendParagraphContent(content...)
}

func (s *createState) appendBlockquote(builder *marksplice.DocumentBuilder, block CreateBlock) error {
	if len(block.Blocks) > 0 {
		child := marksplice.NewDocumentBuilder()
		if err := s.appendBlocks(child, block.Blocks); err != nil {
			return err
		}
		return builder.AppendBlockquoteBlocks(block.Depth, child)
	}
	if block.RawMarkdown != "" {
		if block.Depth <= 1 {
			return builder.AppendBlockquote(block.RawMarkdown)
		}
		return builder.AppendNestedBlockquote(block.Depth, block.RawMarkdown)
	}
	content, err := s.inlineValues(block.Content)
	if err != nil {
		return err
	}
	if block.Depth <= 1 {
		return builder.AppendBlockquoteContent(content...)
	}
	return builder.AppendNestedBlockquoteContent(block.Depth, content...)
}

func (s *createState) appendAlert(builder *marksplice.DocumentBuilder, block CreateBlock) error {
	kind, ok := createAlertKind(block.Kind)
	if !ok {
		return invalidConstruction("alert kind is invalid")
	}
	if len(block.Blocks) > 0 {
		child := marksplice.NewDocumentBuilder()
		if err := s.appendBlocks(child, block.Blocks); err != nil {
			return err
		}
		return builder.AppendAlertBlocks(kind, child)
	}
	if block.RawMarkdown != "" {
		return builder.AppendAlert(kind, block.RawMarkdown)
	}
	content, err := s.inlineValues(block.Content)
	if err != nil {
		return err
	}
	return builder.AppendAlertContent(kind, content...)
}

func appendList(builder *marksplice.DocumentBuilder, block CreateBlock) error {
	items := make([]marksplice.ListItemInput, len(block.Items))
	for i, item := range block.Items {
		items[i] = marksplice.ListItemInput{InlineGFM: item.Markdown, Depth: item.Depth}
	}
	if block.Ordered {
		return builder.AppendNestedOrderedList(items...)
	}
	return builder.AppendNestedUnorderedList(items...)
}

func appendTaskList(builder *marksplice.DocumentBuilder, block CreateBlock) error {
	items := make([]marksplice.TaskListItemInput, len(block.Items))
	for i, item := range block.Items {
		items[i] = marksplice.TaskListItemInput{InlineGFM: item.Markdown, Checked: item.Checked, Depth: item.Depth}
	}
	if block.Ordered {
		return builder.AppendNestedOrderedTaskList(items...)
	}
	return builder.AppendNestedUnorderedTaskList(items...)
}

func appendReferenceDefinition(builder *marksplice.DocumentBuilder, block CreateBlock) error {
	if block.Title == "" {
		return builder.AppendReferenceDefinition(block.Label, block.Destination)
	}
	return builder.AppendReferenceDefinitionWithTitle(block.Label, block.Destination, block.Title)
}

func appendTable(builder *marksplice.DocumentBuilder, block CreateBlock) error {
	rows := make([][]string, len(block.Rows))
	for i := range block.Rows {
		rows[i] = append([]string(nil), block.Rows[i]...)
	}
	if len(block.Alignments) == 0 {
		return builder.AppendTable(append([]string(nil), block.Header...), rows...)
	}
	alignments := make([]marksplice.TableAlignment, len(block.Alignments))
	for i, value := range block.Alignments {
		alignment, ok := createTableAlignment(value)
		if !ok {
			return invalidConstruction("table alignment is invalid")
		}
		alignments[i] = alignment
	}
	return builder.AppendTableWithAlignments(append([]string(nil), block.Header...), alignments, rows...)
}

func (s *createState) inlineValues(values []CreateInline) ([]marksplice.Inline, error) {
	result := make([]marksplice.Inline, len(values))
	for i, value := range values {
		inline, err := s.inlineValue(value)
		if err != nil {
			return nil, err
		}
		result[i] = inline
	}
	return result, nil
}

func (s *createState) inlineValue(value CreateInline) (marksplice.Inline, error) {
	children, err := s.inlineValues(value.Children)
	if err != nil {
		return marksplice.Inline{}, err
	}
	switch value.Type {
	case "text":
		return marksplice.TextInline(value.Text), nil
	case "code":
		return marksplice.CodeInline(value.Text), nil
	case "emphasis":
		return marksplice.EmphasisInline(children...), nil
	case "strong":
		return marksplice.StrongInline(children...), nil
	case "strikethrough":
		return marksplice.StrikethroughInline(children...), nil
	case "autolink":
		return marksplice.AutoLinkInline(value.Value), nil
	case "bare_autolink":
		return marksplice.BareAutoLinkInline(value.Value), nil
	case "footnote_reference":
		return marksplice.FootnoteReferenceInline(value.Label), nil
	case "math":
		return marksplice.MathInline(value.Payload), nil
	case "math_backtick":
		return marksplice.MathBacktickInline(value.Payload), nil
	default:
		return structuredInlineValue(value, children)
	}
}

func structuredInlineValue(value CreateInline, children []marksplice.Inline) (marksplice.Inline, error) {
	switch value.Type {
	case "link":
		if value.Title == "" {
			return marksplice.LinkInline(value.Destination, children...), nil
		}
		return marksplice.LinkInlineWithTitle(value.Destination, value.Title, children...), nil
	case "image":
		if value.Title == "" {
			return marksplice.ImageInline(value.Destination, children...), nil
		}
		return marksplice.ImageInlineWithTitle(value.Destination, value.Title, children...), nil
	case "reference_link":
		return marksplice.ReferenceLinkInline(value.Reference, children...), nil
	case "reference_image":
		return marksplice.ReferenceImageInline(value.Reference, children...), nil
	case "forward_reference_link":
		return marksplice.ForwardReferenceLinkInline(value.Reference, children...), nil
	case "forward_reference_image":
		return marksplice.ForwardReferenceImageInline(value.Reference, children...), nil
	case "collapsed_reference_link":
		return marksplice.CollapsedReferenceLinkInline(children...), nil
	case "collapsed_reference_image":
		return marksplice.CollapsedReferenceImageInline(children...), nil
	case "shortcut_reference_link":
		return marksplice.ShortcutReferenceLinkInline(children...), nil
	case "shortcut_reference_image":
		return marksplice.ShortcutReferenceImageInline(children...), nil
	default:
		return marksplice.Inline{}, invalidConstruction(fmt.Sprintf("unsupported inline construction %q", value.Type))
	}
}

func createAlertKind(value string) (marksplice.AlertKind, bool) {
	switch value {
	case "note":
		return marksplice.AlertKindNote, true
	case "tip":
		return marksplice.AlertKindTip, true
	case "important":
		return marksplice.AlertKindImportant, true
	case "warning":
		return marksplice.AlertKindWarning, true
	case "caution":
		return marksplice.AlertKindCaution, true
	default:
		return marksplice.AlertKindUnknown, false
	}
}

func createTableAlignment(value string) (marksplice.TableAlignment, bool) {
	switch value {
	case "default":
		return marksplice.TableAlignmentDefault, true
	case "left":
		return marksplice.TableAlignmentLeft, true
	case "right":
		return marksplice.TableAlignmentRight, true
	case "center":
		return marksplice.TableAlignmentCenter, true
	default:
		return marksplice.TableAlignmentDefault, false
	}
}

func invalidConstruction(message string) error {
	return fmt.Errorf("%w: %s", marksplice.ErrInvalidConstruction, message)
}
