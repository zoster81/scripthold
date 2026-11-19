package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCPPCLIPinnedWholeOriginalPropertyDeclarationStarts(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CPPCLI_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned C++/CLI originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "cppcli-actor-f8c14d9f.h"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 2895 {
		t.Fatalf("original actor size changed: %d", len(b))
	}
	wants := []struct {
		line int
		name string
	}{
		{42, "get"}, {48, "GetWorldBounds"}, {59, "get"}, {66, "get"},
		{74, "get"}, {82, "get"}, {91, "get"}, {102, "get"}, {112, "get"},
		{117, "UserData"}, {124, "get"}, {127, "UnmanagedOwner"},
	}
	for _, tc := range []struct{ name, source string }{
		{"git-lf", string(b)}, {"worktree-crlf", strings.ReplaceAll(string(b), "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CPPCLIAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("Actor.h", tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 29 {
				t.Fatalf("expected complete 29 supported source declarations, got %d complete=%v", len(result.Analysis.Symbols), result.Analysis.CoverageComplete)
			}
			for _, want := range wants {
				found := false
				for _, s := range result.Analysis.Symbols {
					if s.NameRange.Start.Line != want.line || s.Name != want.name {
						continue
					}
					found = true
					if s.DeclarationRange.Start.Line != want.line {
						t.Errorf("%s line %d: authored declaration starts at %d; got %d (signature=%q)",
							want.name, want.line, want.line, s.DeclarationRange.Start.Line, s.Signature)
					}
				}
				if !found {
					t.Errorf("missing source-authored %s on line %d", want.name, want.line)
				}
			}
		})
	}
}

func TestCPPCLIBlockPropertyMaskIsLexicalAndBounded(t *testing.T) {
	tests := []struct {
		name, source      string
		masked, preserved []string
	}{
		{
			name:   "nested-getter-body-and-braces-in-string",
			source: "ref class Box {\nproperty int Value {\n int get() { return 2; }\n void set(int v) { const char* msg = \"}\"; }\n}\nint Next();\n};\n",
			masked: []string{"property int Value"}, preserved: []string{"int get()", "void set(int v)", "int Next();", "\"}\""},
		},
		{
			name:      "ordinary-identifier-not-a-property",
			source:    "ref class Box {\nint f() { auto property = Value{3}; return 0; }\n};\n",
			preserved: []string{"auto property = Value{3};"},
		},
		{
			name:      "simple-property-is-not-block-property",
			source:    "ref class Box {\nproperty int Simple;\nint Next();\n};\n",
			preserved: []string{"property int Simple;", "int Next();"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := maskCPPCLIModifiers(tc.source)
			masked, err := maskCPPCLIBlockPropertyDelimiters(context.Background(), testSourceDocument("bounded.h", original), original, 128)
			if err != nil {
				t.Fatal(err)
			}
			if len(masked) != len(original) {
				t.Errorf("mask changes original source offsets: %d -> %d", len(original), len(masked))
			}
			for _, part := range tc.masked {
				if strings.Contains(masked, part) {
					t.Errorf("expected property header masked: %q", part)
				}
			}
			for _, part := range tc.preserved {
				if !strings.Contains(masked, part) {
					t.Errorf("real source token unexpectedly masked: %q", part)
				}
			}
		})
	}
}

func TestCPPCLIBlockPropertyHeaderDoesNotCreateFalseDeclarations(t *testing.T) {
	source := "namespace N {\npublic ref class Box {\nproperty int Size {\n int get();\n void set(int value);\n}\nint Next();\nproperty int Simple;\n};\n}\n"
	result, err := (CPPCLIAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("blockproperty.h", source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Error("source should remain structurally covered")
	}
	var get, set, next, simple int
	for _, s := range result.Analysis.Symbols {
		switch s.Name {
		case "get":
			get++
			if s.DeclarationRange.Start.Line != 4 {
				t.Errorf("getter range: %+v", s)
			}
		case "set":
			set++
			if s.DeclarationRange.Start.Line != 5 {
				t.Errorf("setter range: %+v", s)
			}
		case "Next":
			next++
			if s.DeclarationRange.Start.Line != 7 {
				t.Errorf("following method range: %+v", s)
			}
		case "Simple":
			simple++
			if s.DeclarationRange.Start.Line != 8 {
				t.Errorf("simple property range: %+v", s)
			}
		case "Size":
			t.Errorf("unsupported block-property should not invent symbol: %+v", s)
		}
	}
	if get != 1 || set != 1 || next != 1 || simple != 1 {
		t.Errorf("missing authored members get=%d set=%d next=%d simple=%d", get, set, next, simple)
	}
}
