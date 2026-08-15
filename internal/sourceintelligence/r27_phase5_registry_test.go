package sourceintelligence

import (
	"context"
	"testing"
)

func TestR27Phase5RegistryActivatesMandatoryProvidersSeparately(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]AnalyzerID{
		"php":    AnalyzerPHP,
		"ruby":   AnalyzerRuby,
		"swift":  AnalyzerSwift,
		"pascal": AnalyzerPascal,
		"delphi": AnalyzerDelphi,
	}
	for language, analyzerID := range expected {
		descriptor, ok := registry.Resolve(language)
		if !ok {
			t.Fatalf("missing %s", language)
		}
		analyzer, available := AnalyzerFor(descriptor)
		if !available || analyzer.ID() != analyzerID || analyzer.Language() != language {
			t.Fatalf("%s analyzer = %#v available=%v descriptor=%+v", language, analyzer, available, descriptor)
		}
		caps := descriptor.Capabilities
		if !caps.SourceAnalysis || !caps.Declarations || !caps.Hierarchy || !caps.Signatures || !caps.Ranges || !caps.Dependencies {
			t.Fatalf("%s incomplete Phase 5 declaration capability row: %+v", language, caps)
		}
		if caps.ProjectResolvedReferences || caps.ProjectResolvedDefinitions || caps.SemanticRelations || caps.IncrementalIndex {
			t.Fatalf("%s overclaims later R27 semantics: %+v", language, caps)
		}
	}
	if pascal, _ := registry.Resolve("pascal"); pascal.Analyzer == AnalyzerDelphi {
		t.Fatal("Pascal must not be routed through Delphi-only analyzer identity")
	}
}

func TestR27Phase5AmbiguousPascalAndIncDetectionRequiresContentEvidence(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	plain, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "model.pas", Text: "type TPoint = record X: Integer; end;\n"})
	if err != nil {
		t.Fatal(err)
	}
	if plain.State != DetectionAmbiguous || plain.Language != "" {
		t.Fatalf("plain .pas = %+v, want ambiguity between Pascal/Delphi", plain)
	}

	delphi, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "model.pas", Text: "unit Model; interface type THelper = class helper for TObject end; implementation end.\n"})
	if err != nil {
		t.Fatal(err)
	}
	if delphi.State != DetectionProbable || delphi.Language != "delphi" || !hasDetectionEvidence(delphi, EvidenceContentMarker) {
		t.Fatalf("Delphi .pas = %+v, want probable Delphi with content evidence", delphi)
	}

	inc, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "shared.inc", Text: "{$IFDEF FPC}\ntype TFPCOnly = class end;\n{$ENDIF}\n"})
	if err != nil {
		t.Fatal(err)
	}
	if inc.State != DetectionAmbiguous || inc.Language != "" {
		t.Fatalf("plain .inc = %+v, want ambiguous", inc)
	}
}

func TestR27Phase5SwiftContentMarkerDoesNotContaminateCStructs(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cResult, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "sample.c", Text: "struct CBox { int value; };\n"})
	if err != nil {
		t.Fatal(err)
	}
	if cResult.State != DetectionProbable || cResult.Language != "c" || hasDetectionCandidate(cResult, "swift") {
		t.Fatalf("ordinary C struct contaminated by Swift evidence: %+v", cResult)
	}
	swiftResult, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "sample.swift", Text: "struct SwiftBox {}\n"})
	if err != nil {
		t.Fatal(err)
	}
	if swiftResult.State != DetectionProbable || swiftResult.Language != "swift" {
		t.Fatalf("bare Swift struct with .swift extension = %+v, want probable swift", swiftResult)
	}
}
