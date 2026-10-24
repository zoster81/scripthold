package sourceintelligence

import (
	"context"
	"testing"
)

func TestDetectorRoutesJSXTSXAndRustWithoutConflation(t *testing.T) {
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
