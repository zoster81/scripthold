package sourceintelligence

import (
	"context"
	"testing"
)

func TestPHPRubySwiftPascalDoNotOverclaimSemanticRelations(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"php", "ruby", "swift", "pascal", "delphi"} {
		descriptor, _ := registry.Resolve(language)
		if descriptor.Capabilities.SemanticRelations {
			t.Fatalf("%s overclaims semantic relations: %+v", language, descriptor.Capabilities)
		}
	}
}

func TestAmbiguousPascalAndIncDetectionRequiresContentEvidence(t *testing.T) {
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

func TestSwiftContentMarkerDoesNotContaminateCStructs(t *testing.T) {
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
