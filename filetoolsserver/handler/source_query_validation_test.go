package handler

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/config"
)

func TestValidateSourceQueryOperationPreservesValidationOrdering(t *testing.T) {
	t.Parallel()

	limits := config.SourceConfig{
		MaxResults:      10,
		MaxGraphNodes:   10,
		MaxGraphEdges:   10,
		MaxGraphDepth:   10,
		MaxContextBytes: 1024,
		MaxContextItems: 10,
	}
	validSelector := SourceSelectorInput{
		Kind:              "path",
		Path:              "source.go",
		SourceFingerprint: strings.Repeat("a", 64),
	}
	manyTargets := make([]SourceSelectorInput, 33)
	for index := range manyTargets {
		manyTargets[index] = validSelector
	}

	tests := []struct {
		name      string
		operation string
		input     SourceQueryInput
		code      string
		message   string
	}{
		{
			name:      "unknown operation before selector validation",
			operation: "unknown",
			input:     SourceQueryInput{Subject: &SourceSelectorInput{}},
			code:      ErrCodeInvalidInput,
			message:   "operation must be search, relations, or context",
		},
		{
			name:      "search query before mode",
			operation: "search",
			input:     SourceQueryInput{Mode: "invalid"},
			code:      ErrCodeInvalidInput,
			message:   "search query must contain 1 to 512 Unicode scalar values",
		},
		{
			name:      "search mode before closed union",
			operation: "search",
			input:     SourceQueryInput{Query: "x", Mode: "invalid", Relation: "cycles"},
			code:      ErrCodeInvalidInput,
			message:   "search mode must be textual, lexical, or structural",
		},
		{
			name:      "search match before closed union",
			operation: "search",
			input:     SourceQueryInput{Query: "x", Mode: "structural", Match: "invalid", Relation: "cycles"},
			code:      ErrCodeInvalidInput,
			message:   "search match must be exact, prefix, or contains",
		},
		{
			name:      "search closed union before limit",
			operation: "search",
			input:     SourceQueryInput{Query: "x", Mode: "structural", Relation: "cycles", MaxResults: 11},
			code:      ErrCodeInvalidInput,
			message:   "search received fields that are legal only for relations or context",
		},
		{
			name:      "search limit",
			operation: "search",
			input:     SourceQueryInput{Query: "x", Mode: "structural", MaxResults: 11},
			code:      ErrCodeLimit,
			message:   "maxResults 11 exceeds configured limit 10",
		},
		{
			name:      "relations kind before closed union",
			operation: "relations",
			input:     SourceQueryInput{Relation: "invalid", Query: "x"},
			code:      ErrCodeInvalidInput,
			message:   "relations requires a supported relation kind",
		},
		{
			name:      "relations closed union before limit",
			operation: "relations",
			input:     SourceQueryInput{Relation: "cycles", Query: "x", MaxResults: 11},
			code:      ErrCodeInvalidInput,
			message:   "relations received fields that are legal only for search or context",
		},
		{
			name:      "relations limit before relation shape",
			operation: "relations",
			input:     SourceQueryInput{Relation: "cycles", MaxResults: 11, Subject: &validSelector},
			code:      ErrCodeLimit,
			message:   "maxResults 11 exceeds configured limit 10",
		},
		{
			name:      "cycles shape before selector validation",
			operation: "relations",
			input:     SourceQueryInput{Relation: "cycles", Subject: &SourceSelectorInput{}},
			code:      ErrCodeInvalidInput,
			message:   "cycles does not accept subject, target, or maxDepth",
		},
		{
			name:      "trace requires both endpoints",
			operation: "relations",
			input:     SourceQueryInput{Relation: "trace", Subject: &validSelector},
			code:      ErrCodeInvalidInput,
			message:   "trace requires subject and target",
		},
		{
			name:      "ordinary relation requires subject",
			operation: "relations",
			input:     SourceQueryInput{Relation: "dependencies"},
			code:      ErrCodeInvalidInput,
			message:   "this relation requires subject and does not accept target",
		},
		{
			name:      "ordinary relation selector validation after shape",
			operation: "relations",
			input:     SourceQueryInput{Relation: "dependencies", Subject: &SourceSelectorInput{}},
			code:      ErrCodeInvalidInput,
			message:   "selector requires path and lowercase SHA-256 sourceFingerprint",
		},
		{
			name:      "context requirements first",
			operation: "context",
			input:     SourceQueryInput{BodyPolicy: "invalid"},
			code:      ErrCodeInvalidInput,
			message:   "context requires targets and budgetBytes",
		},
		{
			name:      "context target count before body policy",
			operation: "context",
			input:     SourceQueryInput{Targets: manyTargets, BudgetBytes: 1, BodyPolicy: "invalid"},
			code:      ErrCodeLimit,
			message:   "context targets exceeds the 32-item limit",
		},
		{
			name:      "context body policy before closed union",
			operation: "context",
			input:     SourceQueryInput{Targets: []SourceSelectorInput{validSelector}, BudgetBytes: 1, BodyPolicy: "invalid", Query: "x"},
			code:      ErrCodeInvalidInput,
			message:   "context bodyPolicy must be prefer or signatures-only",
		},
		{
			name:      "context closed union before limit",
			operation: "context",
			input:     SourceQueryInput{Targets: []SourceSelectorInput{validSelector}, BudgetBytes: 1, Query: "x", MaxItems: 11},
			code:      ErrCodeInvalidInput,
			message:   "context received fields that are legal only for search or relations",
		},
		{
			name:      "context limit before selector validation",
			operation: "context",
			input:     SourceQueryInput{Targets: []SourceSelectorInput{{}}, BudgetBytes: 1, MaxItems: 11},
			code:      ErrCodeLimit,
			message:   "maxItems 11 exceeds configured limit 10",
		},
		{
			name:      "context selector validation after operation checks",
			operation: "context",
			input:     SourceQueryInput{Targets: []SourceSelectorInput{{}}, BudgetBytes: 1},
			code:      ErrCodeInvalidInput,
			message:   "selector requires path and lowercase SHA-256 sourceFingerprint",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := validateSourceQueryOperation(test.operation, test.input, limits)
			if result == nil || !result.IsError {
				t.Fatalf("result=%+v, want tool error", result)
			}
			if got := result.Meta[ErrorCodeMetaKey]; got != test.code {
				t.Fatalf("error code=%v, want %s", got, test.code)
			}
			if got := sourceQueryValidationMessage(t, result); got != test.message {
				t.Fatalf("message=%q, want %q", got, test.message)
			}
		})
	}
}

func TestValidateSourceQueryOperationAcceptsValidClosedUnionInputs(t *testing.T) {
	t.Parallel()

	limits := config.SourceConfig{
		MaxResults:      10,
		MaxGraphNodes:   10,
		MaxGraphEdges:   10,
		MaxGraphDepth:   10,
		MaxContextBytes: 1024,
		MaxContextItems: 10,
	}
	selector := SourceSelectorInput{
		Kind:              "path",
		Path:              "source.go",
		SourceFingerprint: strings.Repeat("a", 64),
	}

	tests := []struct {
		name      string
		operation string
		input     SourceQueryInput
	}{
		{
			name:      "search",
			operation: "search",
			input:     SourceQueryInput{Query: "symbol", Mode: "structural", Match: "exact", MaxResults: 1},
		},
		{
			name:      "relations cycles",
			operation: "relations",
			input:     SourceQueryInput{Relation: "cycles", MaxResults: 1, MaxNodes: 1, MaxEdges: 1},
		},
		{
			name:      "relations dependency",
			operation: "relations",
			input:     SourceQueryInput{Relation: "dependencies", Subject: &selector, MaxResults: 1, MaxNodes: 1, MaxEdges: 1, MaxDepth: 1},
		},
		{
			name:      "context",
			operation: "context",
			input:     SourceQueryInput{Targets: []SourceSelectorInput{selector}, BudgetBytes: 1, BodyPolicy: "prefer", MaxItems: 1, MaxDepth: 1},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if result := validateSourceQueryOperation(test.operation, test.input, limits); result != nil {
				t.Fatalf("result=%+v, want success", result)
			}
		})
	}
}

func sourceQueryValidationMessage(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) != 1 {
		t.Fatalf("unexpected result content: %+v", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type=%T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}
