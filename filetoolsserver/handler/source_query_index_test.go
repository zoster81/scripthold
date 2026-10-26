package handler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/zoster81/scripthold/internal/operation"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func TestSourceSearchInvalidCursorDoesNotBuildIndex(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	base := SourceQueryInput{
		Operation: "search", Paths: []string{root}, Query: "Item",
		Mode: "structural", Match: "prefix", Language: "java",
		Encoding: "utf-8", MaxResults: 1,
	}
	tests := []struct {
		name  string
		token func(*testing.T, *Handler) string
		want  string
	}{
		{
			name:  "malformed",
			token: func(_ *testing.T, _ *Handler) string { return "!not-base64!" },
			want:  ErrCodeInvalidInput,
		},
		{
			name: "valid signature for other query",
			token: func(t *testing.T, h *Handler) string {
				t.Helper()
				token, err := h.encodeSourceSearchCursor(sourceSearchCursorPayload{
					Version: sourceSearchCursorVersion, ExpiresAt: time.Now().Add(time.Minute).Unix(),
					QueryHash: strings.Repeat("a", 64), Generation: 1, Fingerprint: strings.Repeat("b", 64),
					After: sourceintelligence.ProjectSearchMatch{Path: "any.java"}, Occurrences: 1,
				})
				if err != nil {
					t.Fatal(err)
				}
				return token
			},
			want: ErrCodeInvalidInput,
		},
		{
			name: "expired",
			token: func(t *testing.T, h *Handler) string {
				t.Helper()
				maxFiles, _ := resolvePositiveLimit(base.MaxFiles, h.sourceLimits().MaxFiles, "maxFiles")
				maxResults, _ := resolvePositiveLimit(base.MaxResults, h.sourceLimits().MaxResults, "maxResults")
				queryHash, err := sourceSearchRequestFingerprint(base, maxFiles, maxResults)
				if err != nil {
					t.Fatal(err)
				}
				token, err := h.encodeSourceSearchCursor(sourceSearchCursorPayload{
					Version: sourceSearchCursorVersion, ExpiresAt: time.Now().Add(-time.Minute).Unix(),
					QueryHash: queryHash, Generation: 1, Fingerprint: strings.Repeat("b", 64),
					After: sourceintelligence.ProjectSearchMatch{Path: "any.java"}, Occurrences: 1,
				})
				if err != nil {
					t.Fatal(err)
				}
				return token
			},
			want: ErrCodeConflict,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := NewHandler([]string{root})
			input := base
			input.Cursor = test.token(t, h)
			result, _, err := h.SourceQuery(context.Background(), nil, input)
			if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != test.want {
				t.Fatalf("cursor rejection: result=%+v err=%v want=%s", result, err, test.want)
			}
			if h.sourceIndex != nil {
				t.Fatal("invalid cursor must be rejected before index initialization")
			}
		})
	}
	t.Run("authorization before cursor validation", func(t *testing.T) {
		outside := canonicalHandlerTestDir(t)
		h := NewHandler([]string{root})
		input := base
		input.Paths = []string{outside}
		input.Cursor = "!not-base64!"
		result, _, err := h.SourceQuery(context.Background(), nil, input)
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeAccessDenied {
			t.Fatalf("denied path must take priority: result=%+v err=%v", result, err)
		}
		if h.sourceIndex != nil {
			t.Fatal("denied path must not initialize source index")
		}
	})
}

func TestSourceSearchCursorExpiryIsolationAndBounds(t *testing.T) {
	h := NewHandler([]string{canonicalHandlerTestDir(t)})
	payload := sourceSearchCursorPayload{
		Version: sourceSearchCursorVersion, ExpiresAt: time.Now().Add(time.Minute).Unix(),
		QueryHash: strings.Repeat("a", 64), Generation: 1, Fingerprint: strings.Repeat("b", 64),
		After: sourceintelligence.ProjectSearchMatch{Path: "example.java"}, Occurrences: 1,
	}
	valid, err := h.encodeSourceSearchCursor(payload)
	if err != nil || len(valid) == 0 {
		t.Fatalf("encode cursor: %v", err)
	}
	decoded, err := h.decodeSourceSearchCursor(valid)
	if err != nil || !reflect.DeepEqual(decoded, payload) {
		t.Fatalf("cursor round trip: payload=%+v err=%v", decoded, err)
	}
	other := NewHandler([]string{canonicalHandlerTestDir(t)})
	if _, err := other.decodeSourceSearchCursor(valid); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("cursor must not cross handler instances: %v", err)
	}
	expired := payload
	expired.ExpiresAt = time.Now().Add(-time.Minute).Unix()
	token, err := h.encodeSourceSearchCursor(expired)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.decodeSourceSearchCursor(token); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("expired token must conflict: %v", err)
	}
	if _, err := h.decodeSourceSearchCursor("invalid"); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("invalid token accepted: %v", err)
	}
	oversized := payload
	oversized.After.Path = strings.Repeat("x", sourceSearchPayloadMaxBytes)
	if _, err := h.encodeSourceSearchCursor(oversized); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("oversized payload accepted: %v", err)
	}
}

func TestSourceQuerySearchCursorPagesAndRejectsMismatches(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	for _, name := range []string{"ItemAlpha", "ItemBeta", "ItemGamma"} {
		if err := os.WriteFile(filepath.Join(root, name+".java"), []byte("package demo; public class "+name+" {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	base := SourceQueryInput{
		Operation: "search", Paths: []string{root}, Query: "Item", Mode: "structural",
		Match: "prefix", Language: "java", Encoding: "utf-8", MaxResults: 1,
	}
	call := func(input SourceQueryInput) (SourceQueryOutput, *mcp.CallToolResult) {
		t.Helper()
		result, output, err := h.SourceQuery(context.Background(), nil, input)
		if err != nil {
			t.Fatal(err)
		}
		return output, result
	}
	var combined []string
	input := base
	var cursor string
	for page := 0; page < 4; page++ {
		input.Cursor = cursor
		out, result := call(input)
		if result != nil && result.IsError {
			t.Fatalf("page %d rejected: %+v", page, result)
		}
		if out.Search == nil || len(out.Search.Matches) != 1 {
			t.Fatalf("page %d: %+v", page, out.Search)
		}
		combined = append(combined, out.Search.Matches[0].Name)
		cursor = out.Search.NextCursor
		if page < 2 && cursor == "" {
			t.Fatalf("page %d did not provide continuation", page)
		}
		if page == 2 {
			if cursor != "" || out.Coverage.ResultsTruncated {
				t.Fatalf("last page incorrectly truncated: %+v", out)
			}
			break
		}
	}
	if !reflect.DeepEqual(combined, []string{"ItemAlpha", "ItemBeta", "ItemGamma"}) {
		t.Fatalf("cursor pages: %v", combined)
	}

	first, result := call(base)
	if result != nil && result.IsError {
		t.Fatalf("initial page failed: %+v", result)
	}
	token := first.Search.NextCursor
	for _, testCase := range []struct {
		name string
		edit func(*SourceQueryInput)
		code string
	}{
		{name: "changed query", edit: func(in *SourceQueryInput) { in.Query = "Beta" }, code: ErrCodeInvalidInput},
		{name: "changed result limit", edit: func(in *SourceQueryInput) { in.MaxResults = 2 }, code: ErrCodeInvalidInput},
		{name: "forged token", edit: func(in *SourceQueryInput) {
			last := "A"
			if strings.HasSuffix(in.Cursor, last) {
				last = "B"
			}
			in.Cursor = in.Cursor[:len(in.Cursor)-1] + last
		}, code: ErrCodeInvalidInput},
		{name: "unsupported operation", edit: func(in *SourceQueryInput) { in.Operation = "relations" }, code: ErrCodeInvalidInput},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			bad := base
			bad.Cursor = token
			testCase.edit(&bad)
			_, rejected := call(bad)
			if rejected == nil || !rejected.IsError || rejected.Meta[ErrorCodeMetaKey] != testCase.code {
				t.Fatalf("cursor was accepted: %+v", rejected)
			}
		})
	}
	if err := os.WriteFile(filepath.Join(root, "ItemAlpha.java"), []byte("package demo; public class ItemAlphaChanged {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := base
	stale.Cursor = token
	_, rejected := call(stale)
	if rejected == nil || !rejected.IsError || rejected.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale generation was accepted: %+v", rejected)
	}
}

func TestSourceQuerySkipsStableDecodeFailureWithoutFalseConflict(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	validPath := filepath.Join(root, "Valid.java")
	brokenPath := filepath.Join(root, "Broken.java")
	if err := os.WriteFile(validPath, []byte("package demo;\npublic class Valid {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(brokenPath, []byte{0xc3, 0x28}, 0o600); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{root})
	result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{root}, Query: "Valid", Mode: "structural",
		Language: "java", Encoding: "utf-8", MaxResults: 32,
	})
	if err != nil || (result != nil && result.IsError) {
		t.Fatalf("query result=%+v output=%+v err=%v", result, output, err)
	}
	if output.Index.Generation == 0 || output.Index.Fingerprint == "" || output.Index.Staleness != sourceintelligence.IndexCurrent {
		t.Fatalf("index evidence = %+v", output.Index)
	}
	if output.Coverage.FilesConsidered != 2 || output.Coverage.FilesParsed != 1 || output.Coverage.FilesSkipped != 1 || output.Coverage.CoverageComplete {
		t.Fatalf("coverage = %+v", output.Coverage)
	}
	encodedCoverage, marshalErr := json.Marshal(output.Coverage)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var observed map[string]any
	if err := json.Unmarshal(encodedCoverage, &observed); err != nil {
		t.Fatal(err)
	}
	if observed["analysisComplete"] != false || observed["fileSelectionTruncated"] != false || observed["resultsTruncated"] != false {
		t.Fatalf("skipped file must be an incomplete analysis, not truncation: %s", encodedCoverage)
	}
	if output.Search == nil || len(output.Search.Matches) != 1 || output.Search.Matches[0].Path != validPath {
		t.Fatalf("search = %+v", output.Search)
	}
}

func TestSourceQueryCoverageExplainsIndependentLimits(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	for _, name := range []string{"ItemA", "ItemB"} {
		if err := os.WriteFile(filepath.Join(root, name+".java"), []byte("package demo; public class "+name+" {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	tests := []struct {
		name, query                                                string
		maxFiles, maxResults, expectedMatches                      int
		fileSelectionTruncated, resultsTruncated, coverageComplete bool
	}{
		{name: "file limit", query: "ItemA", maxFiles: 1, maxResults: 8, expectedMatches: 1, fileSelectionTruncated: true},
		{name: "miss in excluded file", query: "ItemB", maxFiles: 1, maxResults: 8, fileSelectionTruncated: true},
		{name: "complete search", query: "ItemA", maxFiles: 2, maxResults: 8, expectedMatches: 1, coverageComplete: true},
		{name: "result limit", query: "Item", maxFiles: 2, maxResults: 1, expectedMatches: 1, resultsTruncated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: []string{root}, Query: tt.query, Match: "contains", Mode: "structural",
				Language: "java", Encoding: "utf-8", MaxFiles: tt.maxFiles, MaxResults: tt.maxResults,
			})
			if err != nil || (result != nil && result.IsError) {
				t.Fatalf("query result=%+v output=%+v err=%v", result, output, err)
			}
			if output.Search == nil || len(output.Search.Matches) != tt.expectedMatches {
				t.Fatalf("matches=%+v, want count %d", output.Search, tt.expectedMatches)
			}
			data, err := json.Marshal(output.Coverage)
			if err != nil {
				t.Fatal(err)
			}
			var coverage map[string]any
			if err := json.Unmarshal(data, &coverage); err != nil {
				t.Fatal(err)
			}
			for name, expected := range map[string]bool{
				"fileSelectionTruncated": tt.fileSelectionTruncated,
				"analysisComplete":       true,
				"resultsTruncated":       tt.resultsTruncated,
				"coverageComplete":       tt.coverageComplete,
			} {
				if got, ok := coverage[name].(bool); !ok || got != expected {
					t.Fatalf("%s: coverage.%s = %v, want %v; coverage=%s", tt.name, name, coverage[name], expected, data)
				}
			}
			if output.Coverage.Truncated != (tt.fileSelectionTruncated || tt.resultsTruncated) {
				t.Fatalf("legacy truncated not preserved: %+v", output.Coverage)
			}
		})
	}
}

func TestSourceQueryIncrementalGenerationAndStaleBinding(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "Box.java")
	if err := os.WriteFile(path, []byte("package demo;\npublic class Box {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	query := SourceQueryInput{Operation: "search", Paths: []string{root}, Query: "Box", Mode: "structural", Language: "java", Encoding: "utf-8", MaxResults: 32}

	firstResult, first, err := h.SourceQuery(context.Background(), nil, query)
	if err != nil || (firstResult != nil && firstResult.IsError) {
		t.Fatalf("first query result=%+v output=%+v err=%v", firstResult, first, err)
	}
	if first.Index.Generation == 0 || first.Index.Fingerprint == "" || first.Index.Staleness != sourceintelligence.IndexCurrent {
		t.Fatalf("first index evidence = %+v", first.Index)
	}
	if first.Search == nil || len(first.Search.Matches) != 1 || first.Search.Matches[0].SourceFingerprint == "" {
		t.Fatalf("first search = %+v", first.Search)
	}

	warmResult, warm, err := h.SourceQuery(context.Background(), nil, query)
	if err != nil || (warmResult != nil && warmResult.IsError) {
		t.Fatalf("warm query result=%+v output=%+v err=%v", warmResult, warm, err)
	}
	if warm.Index != first.Index {
		t.Fatalf("warm query changed generation: first=%+v warm=%+v", first.Index, warm.Index)
	}

	if err := os.WriteFile(path, []byte("package demo;\npublic class Crate {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	currentResult, current, err := h.SourceQuery(context.Background(), nil, query)
	if err != nil || (currentResult != nil && currentResult.IsError) {
		t.Fatalf("current query result=%+v output=%+v err=%v", currentResult, current, err)
	}
	if current.Index.Generation == first.Index.Generation || current.Index.Fingerprint == first.Index.Fingerprint || current.Index.Staleness != sourceintelligence.IndexCurrent {
		t.Fatalf("changed source did not advance current generation: first=%+v current=%+v", first.Index, current.Index)
	}
	if current.Search == nil || len(current.Search.Matches) != 0 {
		t.Fatalf("current search retained stale Box match: %+v", current.Search)
	}

	oldGeneration := first.Index.Generation
	reject := query
	reject.Index = &SourceIndexBindingInput{Generation: &oldGeneration}
	rejectResult, _, rejectErr := h.SourceQuery(context.Background(), nil, reject)
	if rejectErr != nil {
		t.Fatalf("stale reject transport error: %v", rejectErr)
	}
	if rejectResult == nil || !rejectResult.IsError || rejectResult.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale reject result = %+v", rejectResult)
	}

	allow := query
	allow.Index = &SourceIndexBindingInput{Generation: &oldGeneration, Fingerprint: first.Index.Fingerprint, StalePolicy: "allow"}
	allowResult, stale, allowErr := h.SourceQuery(context.Background(), nil, allow)
	if allowErr != nil || (allowResult != nil && allowResult.IsError) {
		t.Fatalf("stale allow result=%+v output=%+v err=%v", allowResult, stale, allowErr)
	}
	if stale.Index.Generation != first.Index.Generation || stale.Index.Fingerprint != first.Index.Fingerprint || stale.Index.Staleness != sourceintelligence.IndexStale {
		t.Fatalf("stale allow evidence = %+v", stale.Index)
	}
	if stale.Search == nil || len(stale.Search.Matches) != 1 || stale.Search.Matches[0].SymbolID != first.Search.Matches[0].SymbolID {
		t.Fatalf("stale generation search = %+v, first=%+v", stale.Search, first.Search)
	}

	selector := SourceSelectorInput{
		Kind:              "symbol",
		Path:              path,
		SymbolID:          stale.Search.Matches[0].SymbolID,
		SourceFingerprint: stale.Search.Matches[0].SourceFingerprint,
	}
	contextResult, _, contextErr := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "context", Paths: []string{root}, Targets: []SourceSelectorInput{selector}, BudgetBytes: 4096,
		BodyPolicy: "prefer", Language: "java", Encoding: "utf-8", MaxItems: 8, MaxDepth: 2,
		Index: &SourceIndexBindingInput{Generation: &oldGeneration, Fingerprint: first.Index.Fingerprint, StalePolicy: "allow"},
	})
	if contextErr != nil {
		t.Fatalf("stale context transport error: %v", contextErr)
	}
	if contextResult == nil || !contextResult.IsError || contextResult.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale historical body should conflict with current source: %+v", contextResult)
	}
}
