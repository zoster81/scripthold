package handler

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/config"
)

func (h *Handler) SourceQuery(_ context.Context, _ *mcp.CallToolRequest, input SourceQueryInput) (*mcp.CallToolResult, SourceQueryOutput, error) {
	operation := strings.ToLower(strings.TrimSpace(input.Operation))
	limits := h.sourceLimits()
	if result := validateSourceQueryCommon(input, limits); result != nil {
		return result, SourceQueryOutput{}, nil
	}
	if result := validateSourceQueryOperation(operation, input, limits); result != nil {
		return result, SourceQueryOutput{}, nil
	}
	return errorResultWithCode(ErrCodeUnsupported, "source_query engine is not implemented yet"), SourceQueryOutput{}, nil
}

func validateSourceQueryCommon(input SourceQueryInput, limits config.SourceConfig) *mcp.CallToolResult {
	if len(input.Paths) == 0 {
		return errorResultWithCode(ErrCodeInvalidInput, "paths must contain at least one path")
	}
	if len(input.Paths) > limits.MaxInputPaths {
		return errorResultWithCode(ErrCodeLimit, "paths exceeds the configured source limit")
	}
	for _, path := range input.Paths {
		if strings.TrimSpace(path) == "" {
			return errorResultWithCode(ErrCodeInvalidInput, "paths must not contain empty values")
		}
	}
	if _, result := resolvePositiveLimit(input.MaxFiles, limits.MaxFiles, "maxFiles"); result != nil {
		return result
	}
	if utf8.RuneCountInString(input.Language) > 64 || utf8.RuneCountInString(input.Encoding) > 64 {
		return errorResultWithCode(ErrCodeInvalidInput, "language and encoding must not exceed 64 Unicode scalar values")
	}
	if len(input.Kinds) > 32 || len(input.Includes) > 64 || len(input.Excludes) > 64 {
		return errorResultWithCode(ErrCodeLimit, "source query filter count exceeds its configured contract")
	}
	for _, pattern := range append(append([]string(nil), input.Includes...), input.Excludes...) {
		if strings.TrimSpace(pattern) == "" || utf8.RuneCountInString(pattern) > 512 {
			return errorResultWithCode(ErrCodeInvalidInput, "include/exclude patterns must contain 1 to 512 Unicode scalar values")
		}
	}
	if result := validateSourceEvidenceFilter(input.Evidence); result != nil {
		return result
	}
	if input.Index != nil {
		if result := validateSourceIndexBinding(*input.Index); result != nil {
			return result
		}
	}
	return nil
}

func validateSourceQueryOperation(operation string, input SourceQueryInput, limits config.SourceConfig) *mcp.CallToolResult {
	switch operation {
	case "search":
		if strings.TrimSpace(input.Query) == "" || utf8.RuneCountInString(input.Query) > 512 {
			return errorResultWithCode(ErrCodeInvalidInput, "search query must contain 1 to 512 Unicode scalar values")
		}
		if input.Mode != "textual" && input.Mode != "lexical" && input.Mode != "structural" {
			return errorResultWithCode(ErrCodeInvalidInput, "search mode must be textual, lexical, or structural")
		}
		if input.Match != "" && input.Match != "exact" && input.Match != "prefix" && input.Match != "contains" {
			return errorResultWithCode(ErrCodeInvalidInput, "search match must be exact, prefix, or contains")
		}
		if input.Relation != "" || input.Subject != nil || input.Target != nil || len(input.Targets) > 0 || input.BudgetBytes != 0 || input.BodyPolicy != "" || input.MaxNodes != 0 || input.MaxEdges != 0 || input.MaxDepth != 0 || input.MaxItems != 0 {
			return errorResultWithCode(ErrCodeInvalidInput, "search received fields that are legal only for relations or context")
		}
		if _, result := resolvePositiveLimit(input.MaxResults, limits.MaxResults, "maxResults"); result != nil {
			return result
		}
	case "relations":
		if !isSourceRelationKind(input.Relation) {
			return errorResultWithCode(ErrCodeInvalidInput, "relations requires a supported relation kind")
		}
		if input.Query != "" || input.Mode != "" || input.Match != "" || len(input.Kinds) > 0 || len(input.Targets) > 0 || input.BudgetBytes != 0 || input.BodyPolicy != "" || input.MaxItems != 0 {
			return errorResultWithCode(ErrCodeInvalidInput, "relations received fields that are legal only for search or context")
		}
		for _, pair := range []struct {
			value   int
			maximum int
			name    string
		}{{input.MaxResults, limits.MaxResults, "maxResults"}, {input.MaxNodes, limits.MaxGraphNodes, "maxNodes"}, {input.MaxEdges, limits.MaxGraphEdges, "maxEdges"}} {
			if _, result := resolvePositiveLimit(pair.value, pair.maximum, pair.name); result != nil {
				return result
			}
		}
		switch input.Relation {
		case "cycles":
			if input.Subject != nil || input.Target != nil || input.MaxDepth != 0 {
				return errorResultWithCode(ErrCodeInvalidInput, "cycles does not accept subject, target, or maxDepth")
			}
		case "trace":
			if input.Subject == nil || input.Target == nil {
				return errorResultWithCode(ErrCodeInvalidInput, "trace requires subject and target")
			}
			if _, result := resolvePositiveLimit(input.MaxDepth, limits.MaxGraphDepth, "maxDepth"); result != nil {
				return result
			}
		default:
			if input.Subject == nil || input.Target != nil {
				return errorResultWithCode(ErrCodeInvalidInput, "this relation requires subject and does not accept target")
			}
			if _, result := resolvePositiveLimit(input.MaxDepth, limits.MaxGraphDepth, "maxDepth"); result != nil {
				return result
			}
		}
	case "context":
		if len(input.Targets) == 0 || input.BudgetBytes <= 0 {
			return errorResultWithCode(ErrCodeInvalidInput, "context requires targets and budgetBytes")
		}
		if len(input.Targets) > 32 {
			return errorResultWithCode(ErrCodeLimit, "context targets exceeds the 32-item limit")
		}
		if input.BodyPolicy != "" && input.BodyPolicy != "prefer" && input.BodyPolicy != "signatures-only" {
			return errorResultWithCode(ErrCodeInvalidInput, "context bodyPolicy must be prefer or signatures-only")
		}
		if input.Query != "" || input.Mode != "" || input.Match != "" || input.Relation != "" || input.Subject != nil || input.Target != nil || len(input.Kinds) > 0 || len(input.Evidence) > 0 || input.MaxResults != 0 || input.MaxNodes != 0 || input.MaxEdges != 0 {
			return errorResultWithCode(ErrCodeInvalidInput, "context received fields that are legal only for search or relations")
		}
		for _, pair := range []struct {
			value   int
			maximum int
			name    string
		}{{input.BudgetBytes, limits.MaxContextBytes, "budgetBytes"}, {input.MaxItems, limits.MaxContextItems, "maxItems"}, {input.MaxDepth, limits.MaxGraphDepth, "maxDepth"}} {
			if _, result := resolvePositiveLimit(pair.value, pair.maximum, pair.name); result != nil {
				return result
			}
		}
	default:
		return errorResultWithCode(ErrCodeInvalidInput, "operation must be search, relations, or context")
	}

	if input.Subject != nil {
		if result := validateSourceSelector(*input.Subject); result != nil {
			return result
		}
	}
	if input.Target != nil {
		if result := validateSourceSelector(*input.Target); result != nil {
			return result
		}
	}
	for _, target := range input.Targets {
		if result := validateSourceSelector(target); result != nil {
			return result
		}
	}
	return nil
}

func validateSourceEvidenceFilter(values []string) *mcp.CallToolResult {
	if len(values) > 6 {
		return errorResultWithCode(ErrCodeLimit, "evidence exceeds the six-item contract")
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		switch value {
		case "textual", "lexical", "structural", "scope-resolved", "project-resolved", "semantic":
		default:
			return errorResultWithCode(ErrCodeInvalidInput, "unknown evidence value")
		}
		if _, exists := seen[value]; exists {
			return errorResultWithCode(ErrCodeInvalidInput, "evidence values must be unique")
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateSourceIndexBinding(index SourceIndexBindingInput) *mcp.CallToolResult {
	if index.Generation == nil && index.Fingerprint == "" {
		return errorResultWithCode(ErrCodeInvalidInput, "index requires generation or fingerprint")
	}
	if index.Generation != nil && *index.Generation == 0 {
		return errorResultWithCode(ErrCodeInvalidInput, "index generation must be positive")
	}
	if index.Fingerprint != "" && !isLowerHexDigest(index.Fingerprint) {
		return errorResultWithCode(ErrCodeInvalidInput, "index fingerprint must be a lowercase SHA-256 digest")
	}
	if index.StalePolicy != "" && index.StalePolicy != "reject" && index.StalePolicy != "allow" {
		return errorResultWithCode(ErrCodeInvalidInput, "index stalePolicy must be reject or allow")
	}
	return nil
}

func validateSourceSelector(selector SourceSelectorInput) *mcp.CallToolResult {
	if strings.TrimSpace(selector.Path) == "" || !isLowerHexDigest(selector.SourceFingerprint) {
		return errorResultWithCode(ErrCodeInvalidInput, "selector requires path and lowercase SHA-256 sourceFingerprint")
	}
	switch selector.Kind {
	case "path":
		if selector.SymbolID != "" || selector.Position != nil {
			return errorResultWithCode(ErrCodeInvalidInput, "path selector does not accept symbolId or position")
		}
	case "symbol":
		if !isLowerHexDigest(selector.SymbolID) || selector.Position != nil {
			return errorResultWithCode(ErrCodeInvalidInput, "symbol selector requires lowercase SHA-256 symbolId and does not accept position")
		}
	case "position":
		if selector.Position == nil || selector.Position.Line <= 0 || selector.Position.Column <= 0 || selector.SymbolID != "" {
			return errorResultWithCode(ErrCodeInvalidInput, "position selector requires positive line/column and does not accept symbolId")
		}
	default:
		return errorResultWithCode(ErrCodeInvalidInput, "selector kind must be symbol, position, or path")
	}
	return nil
}

func isSourceRelationKind(value string) bool {
	switch value {
	case "dependencies", "dependents", "references", "definitions", "inheritance", "implementations", "overrides", "callers", "callees", "trace", "impact", "cycles":
		return true
	default:
		return false
	}
}

func isLowerHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
