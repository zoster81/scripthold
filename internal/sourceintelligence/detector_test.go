package sourceintelligence

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestLanguageDetectorOrderedEvidenceAndAmbiguity(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		input        DetectionInput
		wantState    DetectionState
		wantLanguage string
		wantEvidence EvidenceKind
		wantContains []string
	}{
		{
			name: "explicit alias is exact",
			input: DetectionInput{
				Path:             "misleading.py",
				Text:             "package main\n",
				ExplicitLanguage: "c#",
			},
			wantState: DetectionExact, wantLanguage: "csharp", wantEvidence: EvidenceExplicit,
		},
		{
			name:      "compound suffix",
			input:     DetectionInput{Path: "types.d.ts", Text: "export interface X {}\n"},
			wantState: DetectionExact, wantLanguage: "typescript", wantEvidence: EvidenceCompoundSuffix,
		},
		{
			name:      "extensionless shebang",
			input:     DetectionInput{Path: "script", Text: "#!/usr/bin/env python3\nprint('ok')\n"},
			wantState: DetectionProbable, wantLanguage: "python", wantEvidence: EvidenceShebang,
		},
		{
			name:      "misleading extension content disagreement",
			input:     DetectionInput{Path: "wrong.py", Text: "package main\nfunc main() {}\n"},
			wantState: DetectionAmbiguous, wantEvidence: EvidenceExtension, wantContains: []string{"python", "go"},
		},
		{
			name:      "header ambiguity",
			input:     DetectionInput{Path: "api.h", Text: "struct Item;\n"},
			wantState: DetectionAmbiguous, wantEvidence: EvidenceExtension, wantContains: []string{"c", "cpp", "objective-c"},
		},
		{
			name:      "objective c matlab ambiguity",
			input:     DetectionInput{Path: "model.m", Text: "value = 1;\n"},
			wantState: DetectionAmbiguous, wantEvidence: EvidenceExtension, wantContains: []string{"matlab", "objective-c"},
		},
		{
			name:      "include ambiguity",
			input:     DetectionInput{Path: "shared.inc", Text: "const X = 1;\n"},
			wantState: DetectionAmbiguous, wantEvidence: EvidenceExtension, wantContains: []string{"delphi", "php"},
		},
		{
			name:      "basic ambiguity",
			input:     DetectionInput{Path: "legacy.bas", Text: "Sub Main()\nEnd Sub\n"},
			wantState: DetectionAmbiguous, wantEvidence: EvidenceExtension, wantContains: []string{"qbasic", "vb6", "vba"},
		},
		{
			name:      "classic asp directive",
			input:     DetectionInput{Path: "default.asp", Text: "<%@ Language=\"VBScript\" %>\r\n<% Response.Write \"ok\" %>"},
			wantState: DetectionProbable, wantLanguage: "classic-asp", wantEvidence: EvidenceDirective,
		},
		{
			name:      "project hint remains non authoritative",
			input:     DetectionInput{Path: "unknown", ProjectLanguages: []string{"python"}},
			wantState: DetectionProbable, wantLanguage: "python", wantEvidence: EvidenceProjectHint,
		},
		{
			name:      "unknown",
			input:     DetectionInput{Path: "unknown", Text: "plain prose"},
			wantState: DetectionUnknown,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := DetectLanguage(context.Background(), registry, testCase.input)
			if err != nil {
				t.Fatal(err)
			}
			if result.State != testCase.wantState || result.Language != testCase.wantLanguage {
				t.Fatalf("result = %+v, want state=%s language=%q", result, testCase.wantState, testCase.wantLanguage)
			}
			if testCase.wantEvidence != "" && !hasDetectionEvidence(result, testCase.wantEvidence) {
				t.Fatalf("result evidence = %+v, want %s", result.Evidence, testCase.wantEvidence)
			}
			for _, language := range testCase.wantContains {
				if !hasDetectionCandidate(result, language) {
					t.Fatalf("candidates = %+v, missing %s", result.Candidates, language)
				}
			}
		})
	}
}

func TestLanguageDetectorUniqueExtensionUsesIndependentContentCorroboration(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		input    DetectionInput
		want     string
		contains []string
	}{
		{
			name: "go source containing embedded declaration syntax",
			input: DetectionInput{
				Path: "fixture.go",
				Text: "package fixture\n\nconst schema = `\nenum Bar {\n  VALUE = 0;\n}\n`\n",
			},
			want: "go", contains: []string{"go", "csharp", "vbnet"},
		},
		{
			name: "csharp declaration overlaps vbnet marker",
			input: DetectionInput{
				Path: "Widget.cs",
				Text: "namespace Demo;\n\npublic class Widget {}\n",
			},
			want: "csharp", contains: []string{"csharp", "vbnet"},
		},
		{
			name: "python class overlaps csharp and vbnet markers",
			input: DetectionInput{
				Path: "core.py",
				Text: "class Context:\n    def close(self) -> None:\n        pass\n",
			},
			want: "python", contains: []string{"python", "csharp", "vbnet"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := DetectLanguage(context.Background(), registry, testCase.input)
			if err != nil {
				t.Fatal(err)
			}
			if result.State != DetectionProbable || result.Language != testCase.want {
				t.Fatalf("result = %+v, want probable %q", result, testCase.want)
			}
			for _, language := range testCase.contains {
				if !hasDetectionCandidate(result, language) {
					t.Fatalf("candidates = %+v, missing %s", result.Candidates, language)
				}
			}
		})
	}
}

func TestLanguageDetectorCorroboratedExtensionDoesNotOverrideStrongerConflict(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}

	result, err := DetectLanguage(context.Background(), registry, DetectionInput{
		Path: "script.py",
		Text: "#!/usr/bin/env ruby\n\ndef main():\n    pass\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != DetectionAmbiguous || result.Language != "" {
		t.Fatalf("result = %+v, want ambiguous", result)
	}
	for _, language := range []string{"ruby", "python"} {
		if !hasDetectionCandidate(result, language) {
			t.Fatalf("candidates = %+v, missing %s", result.Candidates, language)
		}
	}
}

func TestLanguageDetectorExactBasenameAndInternalModeline(t *testing.T) {
	registry, err := NewLanguageRegistry([]LanguageDescriptor{
		{ID: "alpha", ExactBasenames: []string{"Buildfile"}},
		{ID: "python", Aliases: []string{"py"}, Extensions: []string{".py"}, ShebangInterpreters: []string{"python", "python3"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	basename, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "BUILDfile", Text: "anything"})
	if err != nil {
		t.Fatal(err)
	}
	if basename.State != DetectionExact || basename.Language != "alpha" || !hasDetectionEvidence(basename, EvidenceExactBasename) {
		t.Fatalf("exact basename = %+v", basename)
	}

	modeline, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "script", Text: "# vim: set ft=py:\nvalue = 1\n"})
	if err != nil {
		t.Fatal(err)
	}
	if modeline.State != DetectionProbable || modeline.Language != "python" || !hasDetectionEvidence(modeline, EvidenceDirective) {
		t.Fatalf("modeline = %+v", modeline)
	}
}

func TestLanguageDetectorRejectsSpoofedDirectives(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		text string
	}{
		{name: "wrong attribute", text: `<%@ SomeLanguage="VBScript" %>`},
		{name: "empty value", text: `<%@ Language= %>`},
		{name: "empty quoted value", text: `<%@ Language="" %>`},
		{name: "unterminated quoted value", text: `<%@ Language="VBScript %>`},
		{name: "word collision", text: `<%@ MyLanguage="VBScript" %>`},
		{name: "overlong directive", text: `<%@ ` + strings.Repeat("x", 2050) + ` Language="VBScript" %>`},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "template", Text: testCase.text})
			if err != nil {
				t.Fatal(err)
			}
			if hasDetectionEvidence(result, EvidenceDirective) {
				t.Fatalf("spoofed directive was trusted: %+v", result)
			}
		})
	}
}

func TestLanguageDetectorRejectsSpoofedShebangAndUnknownExplicitLanguage(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}

	spoofed, err := DetectLanguage(context.Background(), registry, DetectionInput{
		Path: "script",
		Text: "#!/usr/bin/env python3;rm -rf /\nplain prose\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if hasDetectionEvidence(spoofed, EvidenceShebang) || spoofed.Language == "python" {
		t.Fatalf("spoofed shebang was trusted: %+v", spoofed)
	}

	_, err = DetectLanguage(context.Background(), registry, DetectionInput{ExplicitLanguage: "definitely-unknown"})
	if operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("unknown explicit language error = %v, kind=%v", err, operation.KindOf(err))
	}
}

func TestLanguageDetectorBoundedContentProbeDeterminismAndCancellation(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}

	outsideProbeWindow := strings.Repeat("x", DetectionContentProbeBytes+32) + "\npackage main\n"
	bounded, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "source", Text: outsideProbeWindow})
	if err != nil {
		t.Fatal(err)
	}
	if bounded.State != DetectionUnknown {
		t.Fatalf("content beyond probe window affected detection: %+v", bounded)
	}

	input := DetectionInput{Path: "main.go", Text: "package main\nfunc main() {}\n"}
	first, err := DetectLanguage(context.Background(), registry, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DetectLanguage(context.Background(), registry, input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("detector is nondeterministic:\nfirst=%+v\nsecond=%+v", first, second)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = DetectLanguage(ctx, registry, input)
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancel error = %v, kind=%v", err, operation.KindOf(err))
	}
}

func TestLanguageDetectorBoundsAnalyzerProbesCandidatesAndEvidence(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	probes := []LanguageProbe{
		{Language: "rust", Probe: func(context.Context, string) (bool, error) { calls++; return true, nil }},
		{Language: "cpp", Probe: func(context.Context, string) (bool, error) { calls++; return false, nil }},
		{Language: "python", Probe: func(context.Context, string) (bool, error) { calls++; return true, nil }},
	}
	result, err := DetectLanguage(context.Background(), registry, DetectionInput{
		Path:      "unknown",
		Text:      "unrecognized source",
		MaxProbes: 2,
		Probes:    probes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || result.Language != "rust" || result.State != DetectionProbable || !hasDetectionEvidence(result, EvidenceAnalyzerProbe) {
		t.Fatalf("bounded probe result=%+v calls=%d", result, calls)
	}

	descriptors := make([]LanguageDescriptor, 0, DetectionMaxCandidates+4)
	for index := 0; index < DetectionMaxCandidates+4; index++ {
		descriptors = append(descriptors, LanguageDescriptor{
			ID:                  "lang-" + string(rune('a'+index)),
			AmbiguousExtensions: []string{".many"},
		})
	}
	manyRegistry, err := NewLanguageRegistry(descriptors)
	if err != nil {
		t.Fatal(err)
	}
	many, err := DetectLanguage(context.Background(), manyRegistry, DetectionInput{Path: "file.many"})
	if err != nil {
		t.Fatal(err)
	}
	if len(many.Candidates) != DetectionMaxCandidates || len(many.Evidence) > DetectionMaxEvidence || !many.Truncated {
		t.Fatalf("bounded many-candidate result = %+v", many)
	}
}

func hasDetectionEvidence(result DetectionResult, kind EvidenceKind) bool {
	for _, evidence := range result.Evidence {
		if evidence.Kind == kind {
			return true
		}
	}
	return false
}

func hasDetectionCandidate(result DetectionResult, language string) bool {
	for _, candidate := range result.Candidates {
		if candidate.Language == language {
			return true
		}
	}
	return false
}
