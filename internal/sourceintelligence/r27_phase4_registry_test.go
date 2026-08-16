package sourceintelligence

import (
	"context"
	"testing"
)

func TestR27Phase4RegistryActivatesJavaScriptTypeScriptAndRustSeparately(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]AnalyzerID{
		"javascript": AnalyzerJavaScript,
		"typescript": AnalyzerTypeScript,
		"rust":       AnalyzerRust,
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
			t.Fatalf("%s incomplete Phase 4 declaration capability row: %+v", language, caps)
		}
		if caps.SemanticRelations {
			t.Fatalf("%s overclaims unimplemented semantic capability: %+v", language, caps)
		}
	}
	if js, _ := registry.Resolve("javascript"); js.Analyzer == expected["typescript"] {
		t.Fatal("TypeScript must not be routed through JavaScript-only analyzer identity")
	}
}

func TestR27Phase4DetectorRoutesJSXTSXAndRustWithoutConflation(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		path, text, want string
	}{
		{"component.jsx", "export const App = () => <main/>;\n", "javascript"},
		{"component.tsx", "export interface Props { value: string }\nexport const App = (p: Props) => <main/>;\n", "typescript"},
		{"lib.rs", "pub trait Store { fn get(&self); }\n", "rust"},
	} {
		result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: testCase.path, Text: testCase.text})
		if err != nil {
			t.Fatal(err)
		}
		if result.State != DetectionProbable || result.Language != testCase.want {
			t.Fatalf("%s result = %+v, want probable %s", testCase.path, result, testCase.want)
		}
	}
}
