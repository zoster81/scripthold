package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/marksplice"
	"github.com/zoster81/marksplice/workspacefs"
	"github.com/zoster81/scripthold/internal/operation"
)

const (
	MarkdownErrInvalidWorkspace        = "invalid_workspace"
	MarkdownErrWorkspaceBudgetExceeded = "workspace_budget_exceeded"

	markdownWorkspaceMaxItems = 4096
)

type MarkdownWorkspaceDiscovery struct {
	Mode    string   `json:"mode"`
	Entries []string `json:"entries,omitempty"`
}

type MarkdownWorkspaceManagedTOC struct {
	Document string `json:"document"`
	Fragment string `json:"fragment"`
}

type MarkdownWorkspaceInput struct {
	Action           string                        `json:"action"`
	Root             string                        `json:"root"`
	Discovery        MarkdownWorkspaceDiscovery    `json:"discovery"`
	MaxDocuments     *int                          `json:"maxDocuments,omitempty"`
	MaxRelationships *int                          `json:"maxRelationships,omitempty"`
	MaxBytes         *int64                        `json:"maxBytes,omitempty"`
	MaxDepth         *int                          `json:"maxDepth,omitempty"`
	Limit            int                           `json:"limit"`
	Query            string                        `json:"query,omitempty"`
	Document         string                        `json:"document,omitempty"`
	Roots            []string                      `json:"roots,omitempty"`
	ManagedTOCs      []MarkdownWorkspaceManagedTOC `json:"managedTocs,omitempty"`
}

type MarkdownWorkspaceEdge struct {
	SourceDocument string `json:"sourceDocument"`
	TargetDocument string `json:"targetDocument"`
	Kind           string `json:"kind"`
	Destination    string `json:"destination"`
	SourceOffset   int    `json:"sourceOffset"`
	Fragment       string `json:"fragment,omitempty"`
}

type MarkdownWorkspaceDiagnostic struct {
	Kind             string `json:"kind"`
	SourceDocument   string `json:"sourceDocument,omitempty"`
	TargetDocument   string `json:"targetDocument,omitempty"`
	Fragment         string `json:"fragment,omitempty"`
	SourceOffset     *int   `json:"sourceOffset,omitempty"`
	RelationshipKind string `json:"relationshipKind,omitempty"`
	Destination      string `json:"destination,omitempty"`
	Reference        string `json:"reference,omitempty"`
	ReferenceForm    string `json:"referenceForm,omitempty"`
	Image            bool   `json:"image,omitempty"`
}

type MarkdownWorkspaceOutput struct {
	Action          string                        `json:"action"`
	Root            string                        `json:"root"`
	Discovery       string                        `json:"discovery"`
	Query           string                        `json:"query,omitempty"`
	Document        string                        `json:"document,omitempty"`
	TotalDocuments  int                           `json:"totalDocuments"`
	TotalEdges      int                           `json:"totalEdges,omitempty"`
	Documents       []string                      `json:"documents,omitempty"`
	Edges           []MarkdownWorkspaceEdge       `json:"edges,omitempty"`
	Diagnostics     []MarkdownWorkspaceDiagnostic `json:"diagnostics,omitempty"`
	TotalRepairs    int                           `json:"totalRepairs,omitempty"`
	RepairDocuments []string                      `json:"repairDocuments,omitempty"`
	Truncated       bool                          `json:"truncated,omitempty"`
}

func (h *Handler) HandleMarkdownWorkspace(ctx context.Context, _ *mcp.CallToolRequest, input MarkdownWorkspaceInput) (*mcp.CallToolResult, MarkdownWorkspaceOutput, error) {
	if result := h.validateMarkdownWorkspaceInput(input); result != nil {
		return result, MarkdownWorkspaceOutput{}, nil
	}
	validated := h.ValidatePath(input.Root)
	if !validated.Ok() {
		return validated.Result, MarkdownWorkspaceOutput{}, nil
	}
	if err := ctx.Err(); err != nil {
		return errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_workspace", validated.Path, err)), MarkdownWorkspaceOutput{}, nil
	}

	adapter, err := newMarkdownWorkspaceFS(ctx, h, validated.Path, "")
	if err != nil {
		return markdownWorkspaceErrorResult(err), MarkdownWorkspaceOutput{}, nil
	}
	options := h.markdownWorkspaceOptions(input)
	var workspace *workspacefs.Workspace
	switch input.Discovery.Mode {
	case "scan":
		workspace, err = workspacefs.Scan(adapter, ".", options)
	case "follow":
		workspace, err = workspacefs.Follow(adapter, ".", input.Discovery.Entries, options)
	}
	if err != nil {
		return markdownWorkspaceErrorResult(err), MarkdownWorkspaceOutput{}, nil
	}
	if err := ctx.Err(); err != nil {
		return errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_workspace", validated.Path, err)), MarkdownWorkspaceOutput{}, nil
	}

	documents := workspace.Documents()
	output := MarkdownWorkspaceOutput{
		Action:         input.Action,
		Root:           validated.Path,
		Discovery:      input.Discovery.Mode,
		Query:          input.Query,
		Document:       input.Document,
		TotalDocuments: len(documents),
	}

	switch input.Action {
	case "inspect":
		keys := make([]string, len(documents))
		for i, document := range documents {
			keys[i] = string(document.Key)
		}
		output.Documents, output.Truncated = truncateWorkspaceStrings(keys, input.Limit)
	case "query":
		graph, graphErr := workspace.BuildGraph()
		if graphErr != nil {
			return markdownWorkspaceErrorResult(graphErr), MarkdownWorkspaceOutput{}, nil
		}
		output.TotalEdges = len(graph.Edges())
		if result := executeMarkdownWorkspaceQuery(graph, input, &output); result != nil {
			return result, MarkdownWorkspaceOutput{}, nil
		}
	case "validate":
		roots := make([]marksplice.DocumentKey, len(input.Roots))
		for i, root := range input.Roots {
			roots[i] = marksplice.DocumentKey(root)
		}
		managedTOCs, managedResult := resolveMarkdownWorkspaceManagedTOCs(documents, input.ManagedTOCs)
		if managedResult != nil {
			return managedResult, MarkdownWorkspaceOutput{}, nil
		}
		report, validationErr := workspace.Validate(marksplice.WorkspaceValidationOptions{Roots: roots, ManagedTOCs: managedTOCs})
		if validationErr != nil {
			return markdownWorkspaceErrorResult(validationErr), MarkdownWorkspaceOutput{}, nil
		}
		if graph := report.Graph(); graph != nil {
			output.TotalEdges = len(graph.Edges())
		}
		diagnostics := report.Diagnostics()
		projected := make([]MarkdownWorkspaceDiagnostic, len(diagnostics))
		for i, diagnostic := range diagnostics {
			projected[i] = projectMarkdownWorkspaceDiagnostic(diagnostic)
		}
		output.Diagnostics, output.Truncated = truncateWorkspaceDiagnostics(projected, input.Limit)
		repairs := report.RepairPlan().Repairs()
		output.TotalRepairs = len(repairs)
		repairDocuments := make([]string, len(repairs))
		for i, repair := range repairs {
			repairDocuments[i] = string(repair.Document())
		}
		var repairsTruncated bool
		output.RepairDocuments, repairsTruncated = truncateWorkspaceStrings(repairDocuments, input.Limit)
		output.Truncated = output.Truncated || repairsTruncated
	}

	if err := enforceMarkdownWorkspaceOutputBudget(output, h.maxOutputBytes()); err != nil {
		return errorResultFromError(err), MarkdownWorkspaceOutput{}, nil
	}
	return &mcp.CallToolResult{}, output, nil
}

func (h *Handler) validateMarkdownWorkspaceInput(input MarkdownWorkspaceInput) *mcp.CallToolResult {
	if strings.TrimSpace(input.Root) == "" {
		return errorResultWithCode(ErrCodeInvalidInput, "root is required")
	}
	if !filepath.IsAbs(input.Root) {
		return errorResultWithCode(ErrCodeInvalidInput, "root must be an absolute path")
	}
	if input.Limit <= 0 {
		return errorResultWithCode(ErrCodeInvalidInput, "limit must be positive")
	}
	if input.Limit > markdownWorkspaceMaxItems {
		return errorResultWithCode(ErrCodeLimit, fmt.Sprintf("limit %d exceeds Markdown workspace item limit %d", input.Limit, markdownWorkspaceMaxItems))
	}
	if input.Discovery.Mode != "scan" && input.Discovery.Mode != "follow" {
		return errorResultWithCode(ErrCodeInvalidInput, "discovery.mode must be scan or follow")
	}
	if input.Discovery.Mode == "scan" && len(input.Discovery.Entries) != 0 {
		return errorResultWithCode(ErrCodeInvalidInput, "scan discovery does not accept entries")
	}
	if input.Discovery.Mode == "follow" && len(input.Discovery.Entries) == 0 {
		return errorResultWithCode(ErrCodeInvalidInput, "follow discovery requires at least one entry")
	}

	maxDocuments := h.maxMarkdownWorkspaceDocuments()
	maxRelationships := h.maxMarkdownWorkspaceRelationships()
	maxBytes := h.maxFilesystemAggregateBytes()
	maxDepth := h.maxFilesystemRecursiveDepth()
	if input.MaxDocuments != nil && (*input.MaxDocuments <= 0 || *input.MaxDocuments > maxDocuments) {
		return workspaceNarrowingLimitResult("maxDocuments", int64(*input.MaxDocuments), int64(maxDocuments))
	}
	if input.MaxRelationships != nil && (*input.MaxRelationships <= 0 || *input.MaxRelationships > maxRelationships) {
		return workspaceNarrowingLimitResult("maxRelationships", int64(*input.MaxRelationships), int64(maxRelationships))
	}
	if input.MaxBytes != nil && (*input.MaxBytes <= 0 || *input.MaxBytes > maxBytes) {
		return workspaceNarrowingLimitResult("maxBytes", *input.MaxBytes, maxBytes)
	}
	if input.MaxDepth != nil {
		if *input.MaxDepth < 0 {
			return errorResultWithCode(ErrCodeInvalidInput, "maxDepth must not be negative")
		}
		if *input.MaxDepth > maxDepth {
			return workspaceNarrowingLimitResult("maxDepth", int64(*input.MaxDepth), int64(maxDepth))
		}
	}
	effectiveDocuments := maxDocuments
	if input.MaxDocuments != nil {
		effectiveDocuments = *input.MaxDocuments
	}
	if len(input.Discovery.Entries) > effectiveDocuments || len(input.Roots) > effectiveDocuments {
		return errorResultWithCode(ErrCodeLimit, "workspace entry/root count exceeds the effective document ceiling")
	}
	if len(input.ManagedTOCs) > markdownWorkspaceMaxItems {
		return errorResultWithCode(ErrCodeLimit, fmt.Sprintf("managedTocs count exceeds Markdown workspace item limit %d", markdownWorkspaceMaxItems))
	}

	switch input.Action {
	case "inspect":
		if input.Query != "" || input.Document != "" || len(input.Roots) != 0 || len(input.ManagedTOCs) != 0 {
			return errorResultWithCode(ErrCodeInvalidInput, "inspect does not accept query, document, roots, or managedTocs")
		}
	case "query":
		switch input.Query {
		case "edges":
			if input.Document != "" {
				return errorResultWithCode(ErrCodeInvalidInput, "edges query does not accept document")
			}
		case "outgoing", "backlinks", "reachable", "related":
			if strings.TrimSpace(input.Document) == "" {
				return errorResultWithCode(ErrCodeInvalidInput, "document is required for the selected graph query")
			}
		default:
			return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown workspace query")
		}
		if len(input.Roots) != 0 || len(input.ManagedTOCs) != 0 {
			return errorResultWithCode(ErrCodeInvalidInput, "query does not accept validation roots or managedTocs")
		}
	case "validate":
		if input.Query != "" || input.Document != "" {
			return errorResultWithCode(ErrCodeInvalidInput, "validate does not accept query or document")
		}
	default:
		return errorResultWithCode(ErrCodeInvalidInput, "action must be inspect, query, or validate")
	}
	return nil
}

func resolveMarkdownWorkspaceManagedTOCs(documents []marksplice.GraphDocument, requested []MarkdownWorkspaceManagedTOC) ([]marksplice.ManagedTOC, *mcp.CallToolResult) {
	if len(requested) == 0 {
		return nil, nil
	}
	index := make(map[marksplice.DocumentKey]*marksplice.Document, len(documents))
	for _, item := range documents {
		index[item.Key] = item.Document
	}
	result := make([]marksplice.ManagedTOC, 0, len(requested))
	for _, item := range requested {
		documentKey := marksplice.DocumentKey(item.Document)
		fragment := item.Fragment
		document := index[documentKey]
		if strings.TrimSpace(item.Document) == "" || strings.TrimSpace(fragment) == "" || document == nil {
			return nil, markdownSemanticErrorResult(ErrCodeNotFound, MarkdownErrTargetNotFound, "managed TOC document and fragment must resolve inside the discovered workspace")
		}
		target, ok := document.ResolveFragment(fragment)
		if !ok {
			return nil, markdownSemanticErrorResult(ErrCodeNotFound, MarkdownErrTargetNotFound, "managed TOC fragment does not resolve uniquely inside its document")
		}
		if target.Kind() != marksplice.FragmentTargetHeading {
			return nil, markdownSemanticErrorResult(ErrCodeUnsupported, MarkdownErrUnsupportedTargetKind, "managed TOC fragment must resolve to a heading")
		}
		result = append(result, marksplice.ManagedTOC{Document: documentKey, HeadingID: target.NodeID()})
	}
	return result, nil
}

func workspaceNarrowingLimitResult(name string, requested, ceiling int64) *mcp.CallToolResult {
	if requested <= 0 {
		return errorResultWithCode(ErrCodeInvalidInput, name+" must be within its valid positive range")
	}
	return errorResultWithCode(ErrCodeLimit, fmt.Sprintf("%s %d exceeds configured ceiling %d", name, requested, ceiling))
}

func (h *Handler) markdownWorkspaceOptions(input MarkdownWorkspaceInput) workspacefs.Options {
	limits := workspacefs.Limits{
		MaxDocuments:     h.maxMarkdownWorkspaceDocuments(),
		MaxBytes:         h.maxFilesystemAggregateBytes(),
		MaxDepth:         h.maxFilesystemRecursiveDepth(),
		MaxRelationships: h.maxMarkdownWorkspaceRelationships(),
	}
	if input.MaxDocuments != nil {
		limits.MaxDocuments = *input.MaxDocuments
	}
	if input.MaxRelationships != nil {
		limits.MaxRelationships = *input.MaxRelationships
	}
	if input.MaxBytes != nil {
		limits.MaxBytes = *input.MaxBytes
	}
	if input.MaxDepth != nil {
		limits.MaxDepth = *input.MaxDepth
	}
	return workspacefs.Options{Limits: limits}
}

func executeMarkdownWorkspaceQuery(graph *marksplice.DocumentGraph, input MarkdownWorkspaceInput, output *MarkdownWorkspaceOutput) *mcp.CallToolResult {
	switch input.Query {
	case "edges":
		output.Edges, output.Truncated = truncateWorkspaceEdges(projectMarkdownWorkspaceEdges(graph.Edges()), input.Limit)
	case "outgoing":
		edges, ok := graph.Outgoing(marksplice.DocumentKey(input.Document))
		if !ok {
			return markdownSemanticErrorResult(ErrCodeNotFound, MarkdownErrTargetNotFound, "Markdown workspace document was not found")
		}
		output.Edges, output.Truncated = truncateWorkspaceEdges(projectMarkdownWorkspaceEdges(edges), input.Limit)
	case "backlinks":
		edges, ok := graph.Backlinks(marksplice.DocumentKey(input.Document))
		if !ok {
			return markdownSemanticErrorResult(ErrCodeNotFound, MarkdownErrTargetNotFound, "Markdown workspace document was not found")
		}
		output.Edges, output.Truncated = truncateWorkspaceEdges(projectMarkdownWorkspaceEdges(edges), input.Limit)
	case "reachable":
		keys, ok := graph.ReachableFrom(marksplice.DocumentKey(input.Document))
		if !ok {
			return markdownSemanticErrorResult(ErrCodeNotFound, MarkdownErrTargetNotFound, "Markdown workspace document was not found")
		}
		output.Documents, output.Truncated = truncateWorkspaceDocumentKeys(keys, input.Limit)
	case "related":
		keys, ok := graph.RelatedDocuments(marksplice.DocumentKey(input.Document))
		if !ok {
			return markdownSemanticErrorResult(ErrCodeNotFound, MarkdownErrTargetNotFound, "Markdown workspace document was not found")
		}
		output.Documents, output.Truncated = truncateWorkspaceDocumentKeys(keys, input.Limit)
	}
	return nil
}

func projectMarkdownWorkspaceEdges(edges []marksplice.GraphEdge) []MarkdownWorkspaceEdge {
	result := make([]MarkdownWorkspaceEdge, len(edges))
	for i, edge := range edges {
		relationship := edge.Relationship()
		result[i] = MarkdownWorkspaceEdge{
			SourceDocument: string(edge.SourceDocument()),
			TargetDocument: string(edge.TargetDocument()),
			Kind:           markdownWorkspaceRelationshipKindName(relationship.Kind()),
			Destination:    relationship.Destination(),
			SourceOffset:   relationship.SourceOffset(),
		}
		if fragment, ok := edge.Fragment(); ok {
			result[i].Fragment = fragment
		}
	}
	return result
}

func projectMarkdownWorkspaceDiagnostic(diagnostic marksplice.WorkspaceDiagnostic) MarkdownWorkspaceDiagnostic {
	result := MarkdownWorkspaceDiagnostic{Kind: markdownWorkspaceDiagnosticKindName(diagnostic.Kind())}
	if value, ok := diagnostic.SourceDocument(); ok {
		result.SourceDocument = string(value)
	}
	if value, ok := diagnostic.TargetDocument(); ok {
		result.TargetDocument = string(value)
	}
	if value, ok := diagnostic.Fragment(); ok {
		result.Fragment = value
	}
	if value, ok := diagnostic.SourceOffset(); ok {
		offset := value
		result.SourceOffset = &offset
	}
	if relationship, ok := diagnostic.Relationship(); ok {
		result.RelationshipKind = markdownWorkspaceRelationshipKindName(relationship.Kind())
		result.Destination = relationship.Destination()
	}
	if unresolved, ok := diagnostic.UnresolvedReference(); ok {
		result.Reference = unresolved.Reference()
		result.ReferenceForm = markdownWorkspaceReferenceFormName(unresolved.Form())
		result.Image = unresolved.IsImage()
	}
	return result
}

func markdownWorkspaceRelationshipKindName(kind marksplice.LinkRelationshipKind) string {
	switch kind {
	case marksplice.LinkRelationshipInlineLink:
		return "inline_link"
	case marksplice.LinkRelationshipReferenceLink:
		return "reference_link"
	case marksplice.LinkRelationshipInlineImage:
		return "inline_image"
	case marksplice.LinkRelationshipReferenceImage:
		return "reference_image"
	case marksplice.LinkRelationshipAutoLink:
		return "autolink"
	default:
		return "unknown"
	}
}

func markdownWorkspaceReferenceFormName(form marksplice.ReferenceForm) string {
	switch form {
	case marksplice.ReferenceFormFull:
		return "full"
	case marksplice.ReferenceFormCollapsed:
		return "collapsed"
	case marksplice.ReferenceFormShortcut:
		return "shortcut"
	default:
		return "unknown"
	}
}

func markdownWorkspaceDiagnosticKindName(kind marksplice.WorkspaceDiagnosticKind) string {
	switch kind {
	case marksplice.WorkspaceDiagnosticMissingFragment:
		return "missing_fragment"
	case marksplice.WorkspaceDiagnosticAmbiguousFragment:
		return "ambiguous_fragment"
	case marksplice.WorkspaceDiagnosticInvalidFragment:
		return "invalid_fragment"
	case marksplice.WorkspaceDiagnosticMissingDocument:
		return "missing_document"
	case marksplice.WorkspaceDiagnosticUnresolvedReference:
		return "unresolved_reference"
	case marksplice.WorkspaceDiagnosticOrphanDocument:
		return "orphan_document"
	case marksplice.WorkspaceDiagnosticStaleGeneratedIndex:
		return "stale_generated_index"
	case marksplice.WorkspaceDiagnosticUnrecognizedGeneratedIndex:
		return "unrecognized_generated_index"
	default:
		return "unknown"
	}
}

func markdownWorkspaceErrorResult(err error) *mcp.CallToolResult {
	switch {
	case errors.Is(err, workspacefs.ErrBudgetExceeded):
		return markdownSemanticErrorResult(ErrCodeLimit, MarkdownErrWorkspaceBudgetExceeded, err.Error())
	case errors.Is(err, workspacefs.ErrInvalidInput), errors.Is(err, marksplice.ErrInvalidGraph), errors.Is(err, marksplice.ErrInvalidWorkspace):
		return markdownSemanticErrorResult(ErrCodeInvalidInput, MarkdownErrInvalidWorkspace, err.Error())
	default:
		return errorResultFromError(err)
	}
}

func truncateWorkspaceDocumentKeys(values []marksplice.DocumentKey, limit int) ([]string, bool) {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = string(value)
	}
	return truncateWorkspaceStrings(result, limit)
}

func truncateWorkspaceStrings(values []string, limit int) ([]string, bool) {
	if len(values) <= limit {
		return values, false
	}
	return values[:limit], true
}

func truncateWorkspaceEdges(values []MarkdownWorkspaceEdge, limit int) ([]MarkdownWorkspaceEdge, bool) {
	if len(values) <= limit {
		return values, false
	}
	return values[:limit], true
}

func truncateWorkspaceDiagnostics(values []MarkdownWorkspaceDiagnostic, limit int) ([]MarkdownWorkspaceDiagnostic, bool) {
	if len(values) <= limit {
		return values, false
	}
	return values[:limit], true
}

func enforceMarkdownWorkspaceOutputBudget(output MarkdownWorkspaceOutput, maximum int64) error {
	encoded, err := json.Marshal(output)
	if err != nil {
		return operation.Wrap(operation.KindUnknown, "markdown_workspace", "", err)
	}
	if int64(len(encoded)) > maximum {
		return operation.Wrap(operation.KindLimit, "markdown_workspace", "", fmt.Errorf("Markdown workspace output size %d exceeds limit %d", len(encoded), maximum))
	}
	return nil
}
