package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func TestSourceQueryConciseRelationsPartialAmbiguityAndStale(t *testing.T) {
	output := SourceQueryOutput{
		Presentation: "concise", Operation: "relations",
		Index: sourceintelligence.IndexEvidence{
			Generation:  7,
			Fingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Staleness:   sourceintelligence.IndexStale,
		},
		Relations: &SourceRelationsResult{
			Relation:      sourceintelligence.RelationDependencies,
			EvidenceScope: "indexed-relations-only",
			Relations: []sourceintelligence.RelationRecord{{
				Kind:       sourceintelligence.RelationDefinitions,
				Source:     sourceintelligence.RelationEntity{Path: "source.cpp", QualifiedName: "Widget::work"},
				Target:     sourceintelligence.RelationEntity{Path: "include/widget.h", QualifiedName: "Widget::work"},
				Evidence:   sourceintelligence.SymbolEvidenceStructural,
				Resolution: sourceintelligence.ResolutionAmbiguous,
			}},
		},
		Coverage: SourceQueryCoverage{
			AnalysisComplete: false, FileSelectionTruncated: true,
			ResultsTruncated: true, FilesSkipped: 2,
		},
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Relations struct {
			EvidenceScope string                              `json:"evidenceScope"`
			Edges         []sourceintelligence.RelationRecord `json:"edges"`
		} `json:"relations"`
		Coverage struct {
			AnalysisComplete       bool `json:"analysisComplete"`
			FileSelectionTruncated bool `json:"fileSelectionTruncated"`
			ResultsTruncated       bool `json:"resultsTruncated"`
			FilesSkipped           int  `json:"filesSkipped"`
		} `json:"coverage"`
		Index sourceintelligence.IndexEvidence `json:"index"`
	}
	if err := json.Unmarshal(encoded, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Relations.Edges) != 1 ||
		data.Relations.Edges[0].Kind != sourceintelligence.RelationDefinitions ||
		data.Relations.Edges[0].Resolution != sourceintelligence.ResolutionAmbiguous ||
		data.Relations.Edges[0].Evidence != sourceintelligence.SymbolEvidenceStructural ||
		data.Relations.Edges[0].Source.Path != "source.cpp" ||
		data.Relations.Edges[0].Target.Path != "include/widget.h" ||
		data.Relations.EvidenceScope != "indexed-relations-only" ||
		data.Index.Staleness != sourceintelligence.IndexStale ||
		data.Index.Fingerprint != output.Index.Fingerprint ||
		data.Coverage.AnalysisComplete ||
		!data.Coverage.FileSelectionTruncated || !data.Coverage.ResultsTruncated ||
		data.Coverage.FilesSkipped != 2 {
		t.Fatalf("concise relations lost critical uncertainty, kind or stale index evidence: %s", encoded)
	}
}

func TestSourceQueryConciseContextPartialAmbiguousAndStale(t *testing.T) {
	const code = "é\r\nreturn value;"
	output := SourceQueryOutput{
		Operation: "context", Presentation: "concise",
		Index: sourceintelligence.IndexEvidence{
			Generation: 9, Staleness: sourceintelligence.IndexStale,
			Fingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		Context: &SourceContextResult{
			Items: []sourceintelligence.ContextItem{{
				Entity: sourceintelligence.RelationEntity{
					Path: "a.cpp", QualifiedName: "Widget::run",
					SourceFingerprint: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
					Range: &sourceintelligence.Range{
						Start: sourceintelligence.Position{Line: 4, Column: 1},
						End:   sourceintelligence.Position{Line: 5, Column: 14},
					},
				},
				Reason:         sourceintelligence.ContextTarget,
				Representation: sourceintelligence.ContextSignature,
				Priority:       4, Text: code,
				Evidence:   sourceintelligence.SymbolEvidenceStructural,
				Resolution: sourceintelligence.ResolutionAmbiguous,
			}},
			UsedBytes: len(code), BudgetBytes: 1024,
		},
		Coverage: SourceQueryCoverage{
			AnalysisComplete: false, FileSelectionTruncated: true,
			ResultsTruncated: true, FilesSkipped: 2,
		},
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Context struct {
			Items []struct {
				Entity         sourceintelligence.RelationEntity        `json:"entity"`
				Reason         sourceintelligence.ContextReason         `json:"reason"`
				Representation sourceintelligence.ContextRepresentation `json:"representation"`
				Code           string                                   `json:"code"`
				Evidence       sourceintelligence.SymbolEvidence        `json:"evidence"`
				Resolution     sourceintelligence.ResolutionState       `json:"resolution"`
			} `json:"items"`
			UsedBytes   int `json:"usedBytes"`
			BudgetBytes int `json:"budgetBytes"`
		} `json:"context"`
		Coverage struct {
			AnalysisComplete       bool `json:"analysisComplete"`
			FileSelectionTruncated bool `json:"fileSelectionTruncated"`
			ResultsTruncated       bool `json:"resultsTruncated"`
			FilesSkipped           int  `json:"filesSkipped"`
		} `json:"coverage"`
		Index sourceintelligence.IndexEvidence `json:"index"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Context.Items) != 1 ||
		payload.Context.Items[0].Code != code ||
		payload.Context.Items[0].Resolution != sourceintelligence.ResolutionAmbiguous ||
		payload.Context.Items[0].Evidence != sourceintelligence.SymbolEvidenceStructural ||
		payload.Context.Items[0].Entity.Path != "a.cpp" ||
		payload.Context.Items[0].Entity.Range.Start.Line != 4 ||
		payload.Context.Items[0].Representation != sourceintelligence.ContextSignature ||
		payload.Context.UsedBytes != len(code) || payload.Context.BudgetBytes != 1024 ||
		payload.Coverage.AnalysisComplete || !payload.Coverage.FileSelectionTruncated ||
		!payload.Coverage.ResultsTruncated || payload.Coverage.FilesSkipped != 2 ||
		payload.Index.Staleness != sourceintelligence.IndexStale ||
		payload.Index.Fingerprint != output.Index.Fingerprint {
		t.Fatalf("concise context omitted source uncertainty or byte accounting: %s", encoded)
	}
	var general map[string]any
	if err := json.Unmarshal(encoded, &general); err != nil {
		t.Fatal(err)
	}
	item := general["context"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if _, ok := item["priority"]; ok {
		t.Fatal("internal context priority leaked")
	}
	if _, ok := item["text"]; ok {
		t.Fatal("duplicate context text leaked")
	}
	empty := output
	empty.Context = &SourceContextResult{Items: nil, UsedBytes: 0, BudgetBytes: 1024}
	emptyBytes, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	var emptyData map[string]any
	if err := json.Unmarshal(emptyBytes, &emptyData); err != nil {
		t.Fatal(err)
	}
	items := emptyData["context"].(map[string]any)["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("empty context invented snippets: %s", emptyBytes)
	}
}

func TestSourceQueryStructuredResultContract(t *testing.T) {
	output := SourceQueryOutput{
		Operation:        "relations",
		CoordinateSystem: "unicode-scalar-1-based-half-open",
		Index: sourceintelligence.IndexEvidence{
			Generation:  7,
			Fingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Staleness:   sourceintelligence.IndexStale,
		},
		Relations: &SourceRelationsResult{
			Relation: sourceintelligence.RelationReferences,
			Relations: []sourceintelligence.RelationRecord{{
				Kind:       sourceintelligence.RelationReferences,
				Source:     sourceintelligence.RelationEntity{Path: "src/a.go"},
				Target:     sourceintelligence.RelationEntity{Path: "src/b.go"},
				Evidence:   sourceintelligence.SymbolEvidenceStructural,
				Resolution: sourceintelligence.ResolutionAmbiguous,
			}},
		},
		Coverage: SourceQueryCoverage{FilesConsidered: 2, FilesParsed: 2, CoverageComplete: true},
	}

	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal source_query relation output: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode source_query relation output: %v", err)
	}
	index := decoded["index"].(map[string]any)
	if got := index["staleness"]; got != "stale" {
		t.Fatalf("index.staleness = %#v, want stale", got)
	}
	relations := decoded["relations"].(map[string]any)["relations"].([]any)
	relation := relations[0].(map[string]any)
	if got := relation["evidence"]; got != "structural" {
		t.Fatalf("relation.evidence = %#v, want structural", got)
	}
	if got := relation["resolution"]; got != "ambiguous" {
		t.Fatalf("relation.resolution = %#v, want ambiguous", got)
	}
}

func TestSourceQuerySearchIdentifiesDeclarationsWithoutAdditionalLookup(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "Service.java")
	if err := os.WriteFile(path, []byte("package demo; public class Service { public void run() {} }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Query: "Service", Mode: "structural",
		Match: "exact", Language: "java", Encoding: "utf-8", MaxResults: 8,
	})
	if err != nil || (result != nil && result.IsError) || output.Search == nil || len(output.Search.Matches) != 1 {
		t.Fatalf("source query result=%+v output=%+v err=%v", result, output, err)
	}
	encoded, err := json.Marshal(output.Search.Matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var match map[string]any
	if err := json.Unmarshal(encoded, &match); err != nil {
		t.Fatal(err)
	}
	if match["name"] != "Service" || match["kind"] != "class" {
		t.Fatalf("declaration identity missing in search output: %v", match)
	}
	if name, ok := match["qualifiedName"].(string); !ok || name == "" {
		t.Fatalf("declaration qualified name missing in search output: %v", match)
	}
}

// A second typed name on one BASIC DIM line must remain searchable in the project index.
func TestSourceQuerySearchBasicMultiDeclaratorNames(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "window.bas")
	original := "Dim win_Img As Integer, win_Img_Image As Long\nDim win_Cat As Integer, win_Cat_Text As String\nSUB Draw\nEND SUB\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	for _, language := range []string{"classic-basic", "qbasic", "freebasic", "vb6", "vba"} {
		t.Run(language, func(t *testing.T) {
			for _, tc := range []struct {
				name         string
				line, column int
			}{
				{"win_Img", 1, 5},
				{"win_Img_Image", 1, 25},
				{"win_Cat_Text", 2, 25},
				{"Draw", 3, 5},
			} {
				result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
					Operation: "search", Paths: []string{path}, Query: tc.name, Mode: "structural",
					Match: "exact", Language: language, Encoding: "utf-8", MaxResults: 16,
				})
				if err != nil || (result != nil && result.IsError) || output.Search == nil {
					t.Fatalf("search %s result=%+v output=%+v err=%v", tc.name, result, output, err)
				}
				if !output.Coverage.CoverageComplete || output.Coverage.FilesParsed != 1 {
					t.Fatalf("search %s incomplete: %+v", tc.name, output.Coverage)
				}
				found := false
				for _, match := range output.Search.Matches {
					if match.Name == tc.name && match.Range.Start.Line == tc.line && match.Range.Start.Column == tc.column {
						found = true
					}
				}
				if !found {
					t.Errorf("search %s absent at %d:%d: %+v", tc.name, tc.line, tc.column, output.Search.Matches)
				}
			}
		})
	}
}

// D aliases name structural declarations; target calls are not methods.
func TestSourceQueryDTypeAliasDeclarations(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "types.d")
	source := "module demo;\nclass Cache(T) {\n  alias TSelf = typeof(this);\n  alias TBinding = Container!(T);\n  T value;\n  void run() {}\n}\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		name string
		line int
	}{{"TSelf", 3}, {"TBinding", 4}} {
		result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: []string{path}, Query: tc.name,
			Mode: "structural", Match: "exact", Language: "d", Encoding: "utf-8", MaxResults: 8,
		})
		if err != nil || (result != nil && result.IsError) || output.Search == nil || !output.Coverage.CoverageComplete {
			t.Fatalf("alias search %q: result=%+v output=%+v err=%v", tc.name, result, output, err)
		}
		found := false
		for _, match := range output.Search.Matches {
			if match.Name == tc.name && match.Kind == sourceintelligence.SymbolKindAlias &&
				match.Range.Start.Line == tc.line && match.QualifiedName == "demo.Cache."+tc.name {
				found = true
			}
		}
		if !found {
			t.Errorf("missing alias %q at line %d: %+v", tc.name, tc.line, output.Search.Matches)
		}
	}
	toolErr, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "d", Encoding: "utf-8", MaxSymbols: 32,
	})
	if err != nil || toolErr != nil || !outline.CoverageComplete {
		t.Fatalf("outline failed: %v %+v %+v", err, toolErr, outline)
	}
	for _, symbol := range outline.Symbols {
		if symbol.Name == "typeof" || symbol.Name == "Container" {
			t.Errorf("type expression wrongly exposed as callable: %+v", symbol)
		}
	}
}

// Dart getter search must return declarations instead of calls in getter bodies.
func TestSourceQueryDartGetterNamesAndNegativeCalls(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "color.dart")
	source := "extension ColorExtension on Color {\n" +
		"  Color get opacity80 { return withValues(alpha: 0.8); }\n" +
		"  int get alpha8bit => toARGB32() >> 24;\n" +
		"  void lighten() {}\n}\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		name string
		line int
		kind sourceintelligence.SymbolKind
	}{
		{"opacity80", 2, sourceintelligence.SymbolKindProperty},
		{"alpha8bit", 3, sourceintelligence.SymbolKindProperty},
		{"lighten", 4, sourceintelligence.SymbolKindMethod},
	} {
		result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: []string{path}, Query: tc.name, Mode: "structural",
			Match: "exact", Language: "dart", Encoding: "utf-8", MaxResults: 8,
		})
		if err != nil || (result != nil && result.IsError) || output.Search == nil || !output.Coverage.CoverageComplete {
			t.Fatalf("search %s: result=%+v output=%+v err=%v", tc.name, result, output, err)
		}
		found := false
		for _, match := range output.Search.Matches {
			if match.Name == tc.name && match.Kind == tc.kind && match.Range.Start.Line == tc.line &&
				match.QualifiedName == "ColorExtension."+tc.name {
				found = true
			}
		}
		if !found {
			t.Errorf("missing getter or method %s: %+v", tc.name, output.Search.Matches)
		}
	}
	for _, falseCall := range []string{"withValues", "toARGB32"} {
		result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: []string{path}, Query: falseCall, Mode: "structural",
			Match: "exact", Language: "dart", Encoding: "utf-8", MaxResults: 8,
		})
		if err != nil || (result != nil && result.IsError) || output.Search == nil || !output.Coverage.CoverageComplete {
			t.Fatalf("false-call search %s: result=%+v output=%+v err=%v", falseCall, result, output, err)
		}
		if len(output.Search.Matches) != 0 {
			t.Errorf("getter body call %s indexed as declaration: %+v", falseCall, output.Search.Matches)
		}
	}
}

func TestSourceQueryContextResultVocabulary(t *testing.T) {
	output := SourceQueryOutput{
		Operation:        "context",
		CoordinateSystem: "unicode-scalar-1-based-half-open",
		Index:            sourceintelligence.IndexEvidence{Staleness: sourceintelligence.IndexNotIndexed},
		Context: &SourceContextResult{
			Items: []sourceintelligence.ContextItem{{
				Entity:         sourceintelligence.RelationEntity{Path: "src/main.go"},
				Reason:         sourceintelligence.ContextDirectDependency,
				Representation: sourceintelligence.ContextSignature,
				Priority:       4,
				Text:           "func Run()",
				Evidence:       sourceintelligence.SymbolEvidenceStructural,
				Resolution:     sourceintelligence.ResolutionResolved,
			}},
			UsedBytes:   10,
			BudgetBytes: 4096,
		},
		Coverage: SourceQueryCoverage{FilesConsidered: 1, FilesParsed: 1, CoverageComplete: true},
	}

	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal source_query context output: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode source_query context output: %v", err)
	}
	item := decoded["context"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if got := item["reason"]; got != "direct-dependency" {
		t.Fatalf("context.reason = %#v, want direct-dependency", got)
	}
	if got := item["representation"]; got != "signature" {
		t.Fatalf("context.representation = %#v, want signature", got)
	}
}
