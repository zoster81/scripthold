package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/marksplice"
	"github.com/zoster81/scripthold/internal/markdownintelligence"
	"github.com/zoster81/scripthold/internal/operation"
)

const (
	MarkdownErrorCodeMetaKey = "markdownErrorCode"

	MarkdownErrTargetNotFound        = "target_not_found"
	MarkdownErrUnsupportedTargetKind = "unsupported_target_kind"
	MarkdownErrInvalidQuery          = "invalid_query"

	markdownReadMaxItems = 4096
)

// MarkdownReadInput is the closed action union for one authorized Markdown
// document snapshot. Structural semantics are delegated to Marksplice through
// internal/markdownintelligence; the handler owns only host policy and I/O.
type MarkdownReadInput struct {
	Action        string                      `json:"action"`
	Path          string                      `json:"path"`
	Encoding      string                      `json:"encoding,omitempty"`
	Limit         int                         `json:"limit,omitempty"`
	Query         string                      `json:"query,omitempty"`
	Kinds         []string                    `json:"kinds,omitempty"`
	Levels        []int                       `json:"levels,omitempty"`
	Within        *markdownintelligence.Range `json:"within,omitempty"`
	TargetID      string                      `json:"targetId,omitempty"`
	IncludeSource bool                        `json:"includeSource,omitempty"`
	Fragment      string                      `json:"fragment,omitempty"`
	Generate      string                      `json:"generate,omitempty"`
}

// MarkdownReadOutput contains Scripthold-owned physical metadata together with
// Marksplice-backed semantic results for the requested action.
type MarkdownReadOutput struct {
	Action             string                                     `json:"action"`
	Path               string                                     `json:"path"`
	SourceFingerprint  string                                     `json:"sourceFingerprint"`
	Encoding           string                                     `json:"encoding"`
	DetectedEncoding   string                                     `json:"detectedEncoding,omitempty"`
	EncodingConfidence int                                        `json:"encodingConfidence,omitempty"`
	HasBOM             bool                                       `json:"hasBOM"`
	BOMType            string                                     `json:"bomType,omitempty"`
	LineEndings        LineEndingInfo                             `json:"lineEndings"`
	FileSizeBytes      int64                                      `json:"fileSizeBytes"`
	Inspect            *markdownintelligence.InspectResult        `json:"inspect,omitempty"`
	Nodes              []markdownintelligence.NodeSummary         `json:"nodes,omitempty"`
	Sections           []markdownintelligence.SectionSummary      `json:"sections,omitempty"`
	Relationships      []markdownintelligence.RelationshipSummary `json:"relationships,omitempty"`
	Target             *markdownintelligence.NodeSummary          `json:"target,omitempty"`
	Source             string                                     `json:"source,omitempty"`
	Fragment           *markdownintelligence.FragmentResolution   `json:"fragment,omitempty"`
	Valid              *bool                                      `json:"valid,omitempty"`
	TOCStale           *bool                                      `json:"tocStale,omitempty"`
	TOCRecognized      *bool                                      `json:"tocRecognized,omitempty"`
	Generated          string                                     `json:"generated,omitempty"`
	Truncated          bool                                       `json:"truncated,omitempty"`
}

func (h *Handler) HandleMarkdownRead(ctx context.Context, _ *mcp.CallToolRequest, input MarkdownReadInput) (*mcp.CallToolResult, MarkdownReadOutput, error) {
	if result := validateMarkdownReadInput(input); result != nil {
		return result, MarkdownReadOutput{}, nil
	}
	validated := h.ValidatePath(input.Path)
	if !validated.Ok() {
		return validated.Result, MarkdownReadOutput{}, nil
	}
	if err := ctx.Err(); err != nil {
		return errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_read", validated.Path, err)), MarkdownReadOutput{}, nil
	}

	document, _, err := h.readTextDocumentWithData(ctx, validated.Path, input.Encoding)
	if err != nil {
		return errorResultFromError(err), MarkdownReadOutput{}, nil
	}
	if !utf8.ValidString(document.Text) {
		return errorResultFromError(operation.Wrap(operation.KindEncoding, "markdown_read", validated.Path, fmt.Errorf("decoded Markdown is not valid UTF-8"))), MarkdownReadOutput{}, nil
	}
	snapshot, err := markdownintelligence.Parse([]byte(document.Text))
	if err != nil {
		return markdownReadErrorResult(err), MarkdownReadOutput{}, nil
	}

	output := MarkdownReadOutput{
		Action:             input.Action,
		Path:               validated.Path,
		SourceFingerprint:  snapshot.SourceFingerprint(),
		Encoding:           document.Charset,
		DetectedEncoding:   document.DetectedEncoding,
		EncodingConfidence: document.EncodingConfidence,
		HasBOM:             document.BOM.HasBOM,
		BOMType:            document.BOM.Type,
		LineEndings:        document.LineEndings,
		FileSizeBytes:      document.FileSizeBytes,
	}

	switch input.Action {
	case "inspect":
		value, inspectErr := snapshot.Inspect(input.Limit)
		if inspectErr != nil {
			return markdownReadErrorResult(inspectErr), MarkdownReadOutput{}, nil
		}
		output.Inspect = &value
		output.Truncated = value.Truncated
	case "query":
		if result := executeMarkdownQuery(snapshot, input, &output); result != nil {
			return result, MarkdownReadOutput{}, nil
		}
	case "get":
		target, ok, targetErr := snapshot.Target(input.TargetID)
		if targetErr != nil {
			return markdownReadErrorResult(targetErr), MarkdownReadOutput{}, nil
		}
		if !ok {
			return markdownSemanticErrorResult(ErrCodeNotFound, MarkdownErrTargetNotFound, "Markdown target was not found in the current source snapshot"), MarkdownReadOutput{}, nil
		}
		output.Target = &target
		if input.IncludeSource {
			source, ok := snapshot.Source(target.Range)
			if !ok {
				return markdownSemanticErrorResult(ErrCodeNotFound, MarkdownErrTargetNotFound, "Markdown target source range is unavailable in the current snapshot"), MarkdownReadOutput{}, nil
			}
			output.Source = string(source)
		}
	case "resolve":
		fragment, ok := snapshot.ResolveFragment(input.Fragment)
		if !ok {
			return markdownSemanticErrorResult(ErrCodeNotFound, MarkdownErrTargetNotFound, "Markdown fragment does not resolve uniquely"), MarkdownReadOutput{}, nil
		}
		output.Fragment = &fragment
	case "validate":
		if input.Fragment != "" {
			valid := snapshot.ValidateFragment(input.Fragment)
			output.Valid = &valid
		} else {
			stale, recognized, validationErr := snapshot.TOCStale(input.TargetID)
			if validationErr != nil {
				return markdownReadErrorResult(validationErr), MarkdownReadOutput{}, nil
			}
			output.TOCStale = &stale
			output.TOCRecognized = &recognized
		}
	case "generate":
		generated, generationErr := snapshot.Generate(input.Generate)
		if generationErr != nil {
			return markdownReadErrorResult(generationErr), MarkdownReadOutput{}, nil
		}
		output.Generated = string(generated)
	}

	if err := enforceMarkdownReadOutputBudget(output, h.maxOutputBytes()); err != nil {
		return errorResultFromError(err), MarkdownReadOutput{}, nil
	}
	return &mcp.CallToolResult{}, output, nil
}

func validateMarkdownReadInput(input MarkdownReadInput) *mcp.CallToolResult {
	if strings.TrimSpace(input.Path) == "" {
		return errorResultWithCode(ErrCodeInvalidInput, "path is required")
	}
	if utf8.RuneCountInString(input.Encoding) > 64 {
		return errorResultWithCode(ErrCodeInvalidInput, "encoding must not exceed 64 Unicode scalar values")
	}
	if len(input.Kinds) > 64 || len(input.Levels) > 6 {
		return errorResultWithCode(ErrCodeLimit, "Markdown query filters exceed their fixed safety limit")
	}
	if input.Limit < 0 || input.Limit > markdownReadMaxItems {
		if input.Limit > markdownReadMaxItems {
			return errorResultWithCode(ErrCodeLimit, fmt.Sprintf("limit %d exceeds Markdown item limit %d", input.Limit, markdownReadMaxItems))
		}
		return errorResultWithCode(ErrCodeInvalidInput, "limit must be positive")
	}
	if input.TargetID != "" && !isLowerHexDigest(input.TargetID) {
		return errorResultWithCode(ErrCodeInvalidInput, "targetId must be a lowercase SHA-256 digest")
	}
	if utf8.RuneCountInString(input.Fragment) > 2048 {
		return errorResultWithCode(ErrCodeInvalidInput, "fragment must not exceed 2048 Unicode scalar values")
	}

	switch input.Action {
	case "inspect":
		if input.Limit <= 0 || markdownReadHasQueryFields(input) || input.TargetID != "" || input.IncludeSource || input.Fragment != "" || input.Generate != "" {
			return errorResultWithCode(ErrCodeInvalidInput, "inspect requires only path, optional encoding, and a positive limit")
		}
	case "query":
		if input.Limit <= 0 || input.Query == "" || input.TargetID != "" || input.IncludeSource || input.Fragment != "" || input.Generate != "" {
			return errorResultWithCode(ErrCodeInvalidInput, "query requires query and positive limit and does not accept target/fragment/generation fields")
		}
		switch input.Query {
		case "nodes":
			if len(input.Levels) > 0 || input.Within != nil {
				return errorResultWithCode(ErrCodeInvalidInput, "nodes query does not accept levels or within")
			}
		case "sections":
			if len(input.Kinds) > 0 {
				return errorResultWithCode(ErrCodeInvalidInput, "sections query does not accept kinds")
			}
		case "relationships":
			if len(input.Kinds) > 0 || len(input.Levels) > 0 || input.Within != nil {
				return errorResultWithCode(ErrCodeInvalidInput, "relationships query does not accept kinds, levels, or within")
			}
		default:
			return errorResultWithCode(ErrCodeInvalidInput, "query must be nodes, sections, or relationships")
		}
	case "get":
		if input.TargetID == "" || input.Limit != 0 || input.Query != "" || len(input.Kinds) > 0 || len(input.Levels) > 0 || input.Within != nil || input.Fragment != "" || input.Generate != "" {
			return errorResultWithCode(ErrCodeInvalidInput, "get requires targetId and accepts only includeSource as an additional action field")
		}
	case "resolve":
		if strings.TrimSpace(input.Fragment) == "" || markdownReadHasAnySelectorFields(input) || input.IncludeSource || input.Generate != "" {
			return errorResultWithCode(ErrCodeInvalidInput, "resolve requires only a non-empty fragment")
		}
	case "validate":
		fragmentMode := strings.TrimSpace(input.Fragment) != ""
		tocMode := input.TargetID != ""
		if fragmentMode == tocMode || input.Limit != 0 || input.Query != "" || len(input.Kinds) > 0 || len(input.Levels) > 0 || input.Within != nil || input.IncludeSource || input.Generate != "" {
			return errorResultWithCode(ErrCodeInvalidInput, "validate requires exactly one of fragment or targetId")
		}
	case "generate":
		if input.Generate != "toc" && input.Generate != "canonical_markdown" {
			return errorResultWithCode(ErrCodeInvalidInput, "generate requires toc or canonical_markdown")
		}
		if input.Limit != 0 || input.Query != "" || len(input.Kinds) > 0 || len(input.Levels) > 0 || input.Within != nil || input.TargetID != "" || input.IncludeSource || input.Fragment != "" {
			return errorResultWithCode(ErrCodeInvalidInput, "generate does not accept query, target, fragment, or limit fields")
		}
	default:
		return errorResultWithCode(ErrCodeInvalidInput, "action must be inspect, query, get, resolve, validate, or generate")
	}
	return nil
}

func markdownReadHasQueryFields(input MarkdownReadInput) bool {
	return input.Query != "" || len(input.Kinds) > 0 || len(input.Levels) > 0 || input.Within != nil
}

func markdownReadHasAnySelectorFields(input MarkdownReadInput) bool {
	return input.Limit != 0 || input.Query != "" || len(input.Kinds) > 0 || len(input.Levels) > 0 || input.Within != nil || input.TargetID != ""
}

func executeMarkdownQuery(snapshot *markdownintelligence.Snapshot, input MarkdownReadInput, output *MarkdownReadOutput) *mcp.CallToolResult {
	switch input.Query {
	case "nodes":
		values, err := snapshot.QueryNodes(input.Kinds, input.Limit)
		if err != nil {
			return markdownReadErrorResult(err)
		}
		output.Nodes = values
	case "sections":
		values, truncated, err := snapshot.QuerySections(input.Levels, input.Within, input.Limit)
		if err != nil {
			return markdownReadErrorResult(err)
		}
		output.Sections = values
		output.Truncated = truncated
	case "relationships":
		values, truncated, err := snapshot.Relationships(input.Limit)
		if err != nil {
			return markdownReadErrorResult(err)
		}
		output.Relationships = values
		output.Truncated = truncated
	}
	return nil
}

func markdownReadErrorResult(err error) *mcp.CallToolResult {
	switch {
	case errors.Is(err, marksplice.ErrNodeNotFound):
		return markdownSemanticErrorResult(ErrCodeNotFound, MarkdownErrTargetNotFound, "Markdown target was not found")
	case errors.Is(err, marksplice.ErrInvalidTargetKind):
		return markdownSemanticErrorResult(ErrCodeUnsupported, MarkdownErrUnsupportedTargetKind, "Markdown target kind does not support the requested operation")
	case errors.Is(err, marksplice.ErrInvalidQuery):
		return markdownSemanticErrorResult(ErrCodeInvalidInput, MarkdownErrInvalidQuery, "Markdown query is invalid")
	default:
		return errorResultWithCode(ErrCodeOperationFailed, "Markdown operation failed")
	}
}

func markdownSemanticErrorResult(commonCode, markdownCode, message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Meta: mcp.Meta{
			ErrorCodeMetaKey:         commonCode,
			MarkdownErrorCodeMetaKey: markdownCode,
		},
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}
}

func enforceMarkdownReadOutputBudget(output MarkdownReadOutput, maximum int64) error {
	encoded, err := json.Marshal(output)
	if err != nil {
		return operation.Wrap(operation.KindUnknown, "markdown_read", "", err)
	}
	if int64(len(encoded)) > maximum {
		return operation.Wrap(operation.KindLimit, "markdown_read", "", fmt.Errorf("markdown output size %d exceeds limit %d", len(encoded), maximum))
	}
	return nil
}
