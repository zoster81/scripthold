package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The selected originals are all JavaScript-family files; Flow libdefs are
// detected from content, not from the npm package or its filename.
func TestDetectOriginalUnstatedJavascriptAndFlowLibdef(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_FLOW_UNSTATED_PROJECT")
	if root == "" {
		t.Skip("original JS project not available")
	}
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ path, want string }{
		{".babelrc.js", "javascript"},
		{"rollup.config.js", "javascript"},
		{"flow-typed/npm/jest_v22.x.x.js", "flow"},
		{"src/unstated.js", "flow"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			path := filepath.Join(root, filepath.FromSlash(tc.path))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: path, Text: string(data)})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("detection=%+v", got)
			if got.State != DetectionProbable || got.Language != tc.want {
				t.Errorf("%s: detection=%+v; want probable %s", tc.path, got, tc.want)
			}
		})
	}
}

func TestDetectJavaScriptContentCorroborationAndFlowHeader(t *testing.T) {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, path, content, want string }{
		{"commonjs", ".babelrc.js", "const options = {};\nmodule.exports = options;\n", "javascript"},
		{"esm", "rollup.config.js", "import babel from 'rollup-plugin-babel';\nexport default {plugins: [babel()]};\n", "javascript"},
		{"flow-libdef", "vendor.js", "// flow-typed signature: abcdef0123456789\n// flow-typed version: x/y/flow_>=v0.39.x\ntype Box = {value: mixed};\n", "flow"},
		{"flow-pragma", "app.js", "// @flow\nexport default function run(x: string): string {return x;}\n", "flow"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				content := strings.ReplaceAll(tc.content, "\n", eol)
				got, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path, Text: content})
				if err != nil {
					t.Fatal(err)
				}
				if got.State != DetectionProbable || got.Language != tc.want {
					t.Errorf("%s line endings %q: %+v; want probable %s", tc.name, eol, got, tc.want)
				}
			}
		})
	}
}

func TestDetectJavaScriptModuleMarkersRejectOpaqueTextAndFlowHeaderSpoofs(t *testing.T) {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, source string
		forbiddenDetail    string
	}{
		{
			name: "commented CommonJS", path: "config.js",
			source:          "// module.exports = phantom;\nconst exported = false;\n",
			forbiddenDetail: "javascript-module-export",
		},
		{
			name: "template string ESM", path: "docs.js",
			source:          "const example = `\nexport default phantom;\n`;\n",
			forbiddenDetail: "javascript-module-export",
		},
		{
			name: "Flow header inside JS multiline string", path: "docs.js",
			source:          "const example = `\n// flow-typed signature: abcdef0123456789\n// flow-typed version: x/y/flow_>=v0.39.x\n`;\n",
			forbiddenDetail: "flow-typed-libdef-header",
		},
		{
			name: "only Flow signature lacks version", path: "types.js",
			source:          "// flow-typed signature: abcdef0123456789\ntype Box = {value: mixed};\n",
			forbiddenDetail: "flow-typed-libdef-header",
		},
		{
			name: "JS content on unrelated Ruby extension", path: "example.rb",
			source:          "module.exports = something;\n",
			forbiddenDetail: "javascript-module-export",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path, Text: tc.source})
			if err != nil {
				t.Fatal(err)
			}
			for _, evidence := range result.Evidence {
				if evidence.Detail == tc.forbiddenDetail {
					t.Errorf("false positive %s in %s: %+v", tc.forbiddenDetail, tc.name, result)
				}
			}
		})
	}
}
