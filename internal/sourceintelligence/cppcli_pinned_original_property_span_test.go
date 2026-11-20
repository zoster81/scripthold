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
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 38 {
				t.Fatalf("expected 38 supported and block-property declarations, got %d complete=%v", len(result.Analysis.Symbols), result.Analysis.CoverageComplete)
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

func TestCPPCLISimpleManagedPropertyKind(t *testing.T) {
	const sample = "namespace N {\nref class Holder {\nproperty int Count;\nint Field;\nint propertyCount;\n};\n}\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", sample},
		{"CRLF", strings.ReplaceAll(sample, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CPPCLIAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("Holder.h", tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("simple managed property analysis incomplete: %+v", result.Analysis)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			prop, ok := symbols["N.Holder.Count"]
			if !ok || prop.Kind != SymbolKindProperty || prop.NativeKind != "simple-property" ||
				prop.ParentQualifiedName != "N.Holder" || prop.NameRange.Start.Line != 3 ||
				prop.DeclarationRange.Start.Line != 3 {
				t.Errorf("simple managed property: %+v exists=%v", prop, ok)
			}
			for _, name := range []string{"Field", "propertyCount"} {
				field, exists := symbols["N.Holder."+name]
				if !exists || field.Kind != SymbolKindField || field.NativeKind != "field" {
					t.Errorf("normal class member %s changed: %+v", name, field)
				}
			}
		})
	}
}

func TestCPPCLIPinnedWholeOriginalSimplePropertyKinds(t *testing.T) {
	root := os.Getenv("R30_WORKSPACE")
	if root == "" {
		t.Skip("complete pinned originals not configured")
	}
	for _, tc := range []struct {
		name          string
		path          string
		count         int
		propertyLines map[string]int
		blockLines    map[string]int
	}{
		{"libyara", filepath.Join("source-intelligence-corpus", "cppcli-libyara-cpp-cli-selected", "files", "cpp-cli", "Meta.h"),
			6, map[string]int{"libyaraNET.Meta.Identifier": 12, "libyaraNET.Meta.Value": 13}, nil},
		{"physx", filepath.Join("source-intelligence-corpus", "cppcli92-physx-selected", "files", "cpp-cli", "Actor.h"),
			38, map[string]int{"PhysX.Actor.UserData": 117, "PhysX.Actor.UnmanagedOwner": 127},
			map[string]int{"Disposed": 40, "Physics": 57, "Scene": 64, "Type": 72, "Name": 80,
				"Flags": 89, "DominanceGroup": 100, "OwnerClient": 110, "UnmanagedPointer": 122}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(root, tc.path))
			if err != nil {
				t.Fatal(err)
			}
			result, err := (CPPCLIAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("whole.h", string(source)), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated ||
				len(result.Analysis.Symbols) != tc.count {
				t.Fatalf("original symbols count=%d expected %d complete=%v",
					len(result.Analysis.Symbols), tc.count, result.Analysis.CoverageComplete)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, line := range tc.propertyLines {
				s, exists := symbols[name]
				if !exists || s.Kind != SymbolKindProperty || s.NativeKind != "simple-property" ||
					s.NameRange.Start.Line != line || s.DeclarationRange.Start.Line != line {
					t.Errorf("source-authored simple property %s: %+v exists=%v", name, s, exists)
				}
			}
			for name, line := range tc.blockLines {
				qualified := "PhysX.Actor." + name
				owner, ok := symbols[qualified]
				if !ok || owner.Kind != SymbolKindProperty || owner.NativeKind != "block-property" ||
					owner.NameRange.Start.Line != line || owner.DeclarationRange.Start.Line != line ||
					owner.ParentQualifiedName != "PhysX.Actor" || owner.BodyRange == nil {
					t.Errorf("original block property %s: %+v exists=%v", qualified, owner, ok)
				}
				wantAccessors := 1
				if name == "Name" || name == "Flags" || name == "DominanceGroup" || name == "OwnerClient" {
					wantAccessors = 2
				}
				found := 0
				for _, s := range result.Analysis.Symbols {
					if s.ParentQualifiedName == qualified && (s.Name == "get" || s.Name == "set") && s.Kind == SymbolKindMethod {
						if s.ParentID != owner.ID {
							t.Errorf("accessor parent ID for %s: %q != %q", qualified, s.ParentID, owner.ID)
						}
						found++
					}
				}
				if found != wantAccessors {
					t.Errorf("%s accessor count=%d expected=%d", qualified, found, wantAccessors)
				}
			}
			for name, s := range symbols {
				if strings.HasSuffix(name, ".get") || strings.HasSuffix(name, ".set") {
					if s.Kind != SymbolKindMethod {
						t.Errorf("accessor changed: %+v", s)
					}
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
	var get, set, next, simple, size int
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
			size++
			if s.Kind != SymbolKindProperty || s.NativeKind != "block-property" || s.ParentQualifiedName != "N.Box" || s.DeclarationRange.Start.Line != 3 {
				t.Errorf("block property owner: %+v", s)
			}
		}
	}
	if get != 1 || set != 1 || next != 1 || simple != 1 || size != 1 {
		t.Errorf("missing authored members get=%d set=%d next=%d simple=%d size=%d", get, set, next, simple, size)
	}
}

func TestCPPCLIMalformedBlockPropertyDoesNotInventOwner(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{"missing-name", "ref class Box {\nproperty int {\n int get();\n}\nint Next();\n};\n"},
		{"qualified-type-without-name", "ref class Box {\nproperty Foo::Bar {\n int get();\n}\nint Next();\n};\n"},
	}
	for _, tc := range cases {
		for _, crlf := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "-lf", true: "-crlf"}[crlf], func(t *testing.T) {
				source := tc.source
				if crlf {
					source = strings.ReplaceAll(source, "\n", "\r\n")
				}
				result, err := (CPPCLIAnalyzer{}).Analyze(context.Background(),
					testSourceDocument("invalid.h", source), testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				var next bool
				for _, symbol := range result.Analysis.Symbols {
					if symbol.Kind == SymbolKindProperty && symbol.NativeKind == "block-property" {
						t.Errorf("invented owner in malformed header: %+v", symbol)
					}
					if symbol.Name == "Next" && symbol.ParentQualifiedName == "Box" {
						next = true
					}
				}
				if !next {
					t.Error("missing following authored method")
				}
			})
		}
	}
}
