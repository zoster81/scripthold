package handler

import "github.com/zoster81/scripthold/internal/markdownintelligence"

type MarkdownCreateInput struct {
	Path        string                     `json:"path"`
	Encoding    string                     `json:"encoding,omitempty"`
	BOM         string                     `json:"bom,omitempty"`
	FrontMatter *MarkdownCreateFrontMatter `json:"frontMatter,omitempty"`
	Blocks      []MarkdownCreateBlock      `json:"blocks,omitempty"`
}

type MarkdownCreateFrontMatter struct {
	Format string                           `json:"format"`
	Fields []MarkdownCreateFrontMatterField `json:"fields"`
}

type MarkdownCreateFrontMatterField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type MarkdownCreateBlock struct {
	Type        string                   `json:"type"`
	Level       int                      `json:"level,omitempty"`
	Content     []MarkdownCreateInline   `json:"content,omitempty"`
	RawMarkdown string                   `json:"rawMarkdown,omitempty"`
	Depth       int                      `json:"depth,omitempty"`
	Kind        string                   `json:"kind,omitempty"`
	Blocks      []MarkdownCreateBlock    `json:"blocks,omitempty"`
	Ordered     bool                     `json:"ordered,omitempty"`
	Items       []MarkdownCreateListItem `json:"items,omitempty"`
	Code        string                   `json:"code,omitempty"`
	Info        string                   `json:"info,omitempty"`
	Label       string                   `json:"label,omitempty"`
	Destination string                   `json:"destination,omitempty"`
	Title       string                   `json:"title,omitempty"`
	Deferred    bool                     `json:"deferred,omitempty"`
	Body        string                   `json:"body,omitempty"`
	Payload     string                   `json:"payload,omitempty"`
	Header      []string                 `json:"header,omitempty"`
	Rows        [][]string               `json:"rows,omitempty"`
	Alignments  []string                 `json:"alignments,omitempty"`
}

type MarkdownCreateListItem struct {
	Markdown string `json:"markdown"`
	Checked  bool   `json:"checked,omitempty"`
	Depth    int    `json:"depth"`
}

type MarkdownCreateInline struct {
	Type        string                 `json:"type"`
	Text        string                 `json:"text,omitempty"`
	Children    []MarkdownCreateInline `json:"children,omitempty"`
	Destination string                 `json:"destination,omitempty"`
	Title       string                 `json:"title,omitempty"`
	Value       string                 `json:"value,omitempty"`
	Reference   string                 `json:"reference,omitempty"`
	Label       string                 `json:"label,omitempty"`
	Payload     string                 `json:"payload,omitempty"`
}

func markdownCreateDocument(input MarkdownCreateInput) markdownintelligence.CreateDocument {
	document := markdownintelligence.CreateDocument{
		Blocks: markdownCreateBlocks(input.Blocks),
	}
	if input.FrontMatter != nil {
		fields := make([]markdownintelligence.CreateFrontMatterField, len(input.FrontMatter.Fields))
		for i, field := range input.FrontMatter.Fields {
			fields[i] = markdownintelligence.CreateFrontMatterField{Key: field.Key, Value: field.Value}
		}
		document.FrontMatter = &markdownintelligence.CreateFrontMatter{
			Format: input.FrontMatter.Format,
			Fields: fields,
		}
	}
	return document
}

func markdownCreateBlocks(values []MarkdownCreateBlock) []markdownintelligence.CreateBlock {
	result := make([]markdownintelligence.CreateBlock, len(values))
	for i, value := range values {
		items := make([]markdownintelligence.CreateListItem, len(value.Items))
		for j, item := range value.Items {
			items[j] = markdownintelligence.CreateListItem{
				Markdown: item.Markdown,
				Checked:  item.Checked,
				Depth:    item.Depth,
			}
		}
		result[i] = markdownintelligence.CreateBlock{
			Type:        value.Type,
			Level:       value.Level,
			Content:     markdownCreateInlineValues(value.Content),
			RawMarkdown: value.RawMarkdown,
			Depth:       value.Depth,
			Kind:        value.Kind,
			Blocks:      markdownCreateBlocks(value.Blocks),
			Ordered:     value.Ordered,
			Items:       items,
			Code:        value.Code,
			Info:        value.Info,
			Label:       value.Label,
			Destination: value.Destination,
			Title:       value.Title,
			Deferred:    value.Deferred,
			Body:        value.Body,
			Payload:     value.Payload,
			Header:      append([]string(nil), value.Header...),
			Rows:        cloneMarkdownCreateRows(value.Rows),
			Alignments:  append([]string(nil), value.Alignments...),
		}
	}
	return result
}

func markdownCreateInlineValues(values []MarkdownCreateInline) []markdownintelligence.CreateInline {
	result := make([]markdownintelligence.CreateInline, len(values))
	for i, value := range values {
		result[i] = markdownintelligence.CreateInline{
			Type:        value.Type,
			Text:        value.Text,
			Children:    markdownCreateInlineValues(value.Children),
			Destination: value.Destination,
			Title:       value.Title,
			Value:       value.Value,
			Reference:   value.Reference,
			Label:       value.Label,
			Payload:     value.Payload,
		}
	}
	return result
}

func cloneMarkdownCreateRows(rows [][]string) [][]string {
	result := make([][]string, len(rows))
	for i := range rows {
		result[i] = append([]string(nil), rows[i]...)
	}
	return result
}
