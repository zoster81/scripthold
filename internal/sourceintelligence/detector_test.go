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

func TestRealWorldClassicVBExportedFormatsRouteConservatively(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		input        DetectionInput
		wantState    DetectionState
		wantLanguage string
		wantContains []string
	}{
		{
			name:      "vb6 form designer is distinctive",
			input:     DetectionInput{Path: "About.frm", Text: "VERSION 5.00\r\nBegin VB.Form frmAbout\r\nEnd\r\nAttribute VB_Name = \"frmAbout\"\r\nPrivate Sub Form_Load()\r\nEnd Sub\r\n"},
			wantState: DetectionProbable, wantLanguage: "vb6", wantContains: []string{"vb6", "vba"},
		},
		{
			name:      "vb6 user control is distinctive",
			input:     DetectionInput{Path: "Grid.ctl", Text: "VERSION 5.00\r\nBegin VB.UserControl Grid\r\nEnd\r\nAttribute VB_Name = \"Grid\"\r\nPublic Sub Refresh()\r\nEnd Sub\r\n"},
			wantState: DetectionProbable, wantLanguage: "vb6", wantContains: []string{"vb6"},
		},
		{
			name:      "vb6 designer extension corroborates classic metadata",
			input:     DetectionInput{Path: "DataEnv.dsr", Text: "VERSION 5.00\r\nBegin {C0E45035-5775-11D0-B388-00A0C9055D8E} DataEnvironment1\r\nEnd\r\nAttribute VB_Name = \"DataEnvironment1\"\r\nPrivate Sub DataEnvironment_Initialize()\r\nEnd Sub\r\n"},
			wantState: DetectionProbable, wantLanguage: "vb6", wantContains: []string{"vb6", "vba"},
		},
		{
			name:      "classic class remains ambiguous with apex and vba",
			input:     DetectionInput{Path: "Worker.cls", Text: "VERSION 1.0 CLASS\r\nBEGIN\r\n  MultiUse = -1\r\nEND\r\nAttribute VB_Name = \"Worker\"\r\nPublic Sub Run()\r\nEnd Sub\r\n"},
			wantState: DetectionAmbiguous, wantContains: []string{"apex", "vb6", "vba"},
		},
		{
			name:      "vba userform is not promoted to vb6 by frm extension",
			input:     DetectionInput{Path: "UserForm1.frm", Text: "VERSION 5.00\r\nBegin {00000000-0000-0000-0000-000000000000} UserForm1\r\nEnd\r\nAttribute VB_Name = \"UserForm1\"\r\nPrivate Sub UserForm_Initialize()\r\nEnd Sub\r\n"},
			wantState: DetectionAmbiguous, wantContains: []string{"vb6", "vba"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := DetectLanguage(context.Background(), registry, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if result.State != tc.wantState || result.Language != tc.wantLanguage {
				t.Fatalf("result=%+v want state=%s language=%q", result, tc.wantState, tc.wantLanguage)
			}
			for _, language := range tc.wantContains {
				if !hasDetectionCandidate(result, language) {
					t.Fatalf("candidates=%+v missing %s", result.Candidates, language)
				}
			}
		})
	}
}

func TestRealWorldSharedLSPExtensionRequiresCommonLispContent(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range []struct {
		name         string
		path         string
		text         string
		wantState    DetectionState
		wantLanguage string
	}{
		{
			name:      "newlisp shebang and forms do not route as common lisp",
			path:      "markdown.lsp",
			text:      "#!/usr/bin/env newlisp\n(context 'Hash)\n(define (hash s) s)\n",
			wantState: DetectionAmbiguous,
		},
		{
			name:         "common lisp lsp is corroborated by defun",
			path:         "sample.lsp",
			text:         "(in-package :foo)\n(defun add (x) x)\n",
			wantState:    DetectionProbable,
			wantLanguage: "common-lisp",
		},
		{
			name:      "newlisp also shares lisp extension",
			path:      "log-to-database.lisp",
			text:      "(module \"sqlite3.lsp\")\n(define (displayln value) (println value))\n",
			wantState: DetectionAmbiguous,
		},
		{
			name:      "shared lisp extension without content corroboration remains ambiguous",
			path:      "package-only.lisp",
			text:      "(in-package :foo)\n",
			wantState: DetectionAmbiguous,
		},
		{
			name:         "common lisp lisp is corroborated by defun",
			path:         "sample.lisp",
			text:         "(in-package :foo)\n(defun add (x) x)\n",
			wantState:    DetectionProbable,
			wantLanguage: "common-lisp",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result, detectErr := DetectLanguage(context.Background(), registry, DetectionInput{Path: testCase.path, Text: testCase.text})
			if detectErr != nil {
				t.Fatal(detectErr)
			}
			if result.State != testCase.wantState || result.Language != testCase.wantLanguage {
				t.Fatalf("result=%+v want state=%s language=%q", result, testCase.wantState, testCase.wantLanguage)
			}
			if !hasDetectionCandidate(result, "common-lisp") {
				t.Fatalf("candidates=%+v missing common-lisp", result.Candidates)
			}
		})
	}
}

func TestRealWorldSharedFExtensionRequiresFortranContent(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range []struct {
		name         string
		path         string
		text         string
		wantState    DetectionState
		wantLanguage string
	}{
		{
			name:      "forth source does not route as fortran",
			path:      "core.f",
			text:      ": immediate lastxt @ dup c@ negate swap c! ;\n: chars ;\n",
			wantState: DetectionAmbiguous,
		},
		{
			name:      "filebench source does not route as fortran",
			path:      "copyfiles.f",
			text:      "set $dir=/tmp\ndefine fileset name=bigfileset,path=$dir\ndefine process name=filereader,instances=1\n",
			wantState: DetectionAmbiguous,
		},
		{
			name:         "fixed form fortran is corroborated",
			path:         "ahcon.f",
			text:         "        SUBROUTINE AHCON (N)\n        INTEGER N\n        END SUBROUTINE AHCON\n",
			wantState:    DetectionProbable,
			wantLanguage: "fortran",
		},
		{
			name:         "modern fortran extension remains authoritative",
			path:         "solver.f90",
			text:         "plain source without a distinctive detector marker\n",
			wantState:    DetectionProbable,
			wantLanguage: "fortran",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result, detectErr := DetectLanguage(context.Background(), registry, DetectionInput{Path: testCase.path, Text: testCase.text})
			if detectErr != nil {
				t.Fatal(detectErr)
			}
			if result.State != testCase.wantState || result.Language != testCase.wantLanguage {
				t.Fatalf("result=%+v want state=%s language=%q", result, testCase.wantState, testCase.wantLanguage)
			}
			if !hasDetectionCandidate(result, "fortran") {
				t.Fatalf("candidates=%+v missing fortran", result.Candidates)
			}
		})
	}
}

func TestRealSourceCIncludeDoesNotPromoteFreeBasic(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := DetectLanguage(context.Background(), registry, DetectionInput{
		Path: "tool_main.c",
		Text: "#include \"tool_setup.h\"\nint main(void) { return 0; }\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != DetectionProbable || result.Language != "c" {
		t.Fatalf("C include detection = %+v, want probable c", result)
	}
	if hasDetectionCandidate(result, "freebasic") {
		t.Fatalf("ordinary C #include promoted FreeBASIC: %+v", result)
	}

	freeBasic, err := DetectLanguage(context.Background(), registry, DetectionInput{
		Path: "fixture",
		Text: "#include once \"common.bi\"\nNamespace Demo\nEnd Namespace\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if freeBasic.State != DetectionProbable || freeBasic.Language != "freebasic" {
		t.Fatalf("distinctive FreeBASIC include detection = %+v", freeBasic)
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

func TestDetectorKeepsRazorAndBlazorCanonicalRoutingDistinct(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		path string
		want string
	}{
		{path: "View.cshtml", want: "razor"},
		{path: "Component.razor", want: "blazor"},
	} {
		result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: testCase.path})
		if err != nil {
			t.Fatal(err)
		}
		if result.State != DetectionProbable || result.Language != testCase.want {
			t.Fatalf("%s result = %+v, want probable %s", testCase.path, result, testCase.want)
		}
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
