package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independently inventoried declarations from five complete original GraphQL files
// in two repositories. Current supported scope: schema, named operations,
// reusable named fragments, object/interface/input/enum types, scalar/union
// types, and typed fields. Directive definitions, enum values, anonymous
// operations, field arguments and query selections remain outside this inventory.
func TestGraphQLSelectedOriginalCompleteDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original GraphQL source corpus not configured")
	}
	type declaration struct {
		line                       int
		name, kind, native, parent string
	}
	cases := []struct {
		project, file string
		want          []declaration
	}{
		{
			project: "graphql57-hasura-audit-selected",
			file:    "cli/commands/testdata/config-v3-test-project/metadata/actions.graphql",
			want: []declaration{
				{1, "Mutation", "type", "type", ""},
				{2, "actionName", "field", "field", "Mutation"},
				{7, "SampleInput", "type", "input", ""},
				{8, "username", "field", "field", "SampleInput"},
				{9, "password", "field", "field", "SampleInput"},
				{12, "SampleOutput", "type", "type", ""},
				{13, "accessToken", "field", "field", "SampleOutput"},
			},
		},
		{
			project: "graphql57-hasura-audit-selected",
			file:    "v3/crates/engine/tests/execute/aggregates/root_field/simple_select/request.gql",
			want:    nil, // Anonymous operation and nested selections are not declarations.
		},
		{
			project: "graphql57-go-audit-selected",
			file:    "schema-kitchen-sink.graphql",
			want: []declaration{
				{3, "schema", "schema", "schema", ""},
				{8, "Foo", "type", "type", ""},
				{9, "one", "field", "field", "Foo"},
				{10, "two", "field", "field", "Foo"},
				{11, "three", "field", "field", "Foo"},
				{12, "four", "field", "field", "Foo"},
				{13, "five", "field", "field", "Foo"},
				{14, "six", "field", "field", "Foo"},
				{17, "AnnotatedObject", "type", "type", ""},
				{18, "annotatedField", "field", "field", "AnnotatedObject"},
				{21, "Bar", "interface", "interface", ""},
				{22, "one", "field", "field", "Bar"},
				{23, "four", "field", "field", "Bar"},
				{26, "AnnotatedInterface", "interface", "interface", ""},
				{27, "annotatedField", "field", "field", "AnnotatedInterface"},
				{30, "Feed", "type", "union", ""},
				{32, "AnnotatedUnion", "type", "union", ""},
				{34, "CustomScalar", "type", "scalar", ""},
				{36, "AnnotatedScalar", "type", "scalar", ""},
				{38, "Site", "enum", "enum", ""},
				{43, "AnnotatedEnum", "enum", "enum", ""},
				{48, "InputType", "type", "input", ""},
				{49, "key", "field", "field", "InputType"},
				{50, "answer", "field", "field", "InputType"},
				{53, "AnnotatedInput", "type", "input", ""},
				{54, "annotatedField", "field", "field", "AnnotatedInput"},
				{57, "Foo", "type", "type", ""},
				{58, "seven", "field", "field", "Foo"},
				{61, "Foo", "type", "type", ""},
				{63, "NoFields", "type", "type", ""},
			},
		},
		{
			project: "graphql57-go-audit-selected",
			file:    "schema-all-descriptions.graphql",
			want: []declaration{
				{4, "ScalarSingleLine", "type", "scalar", ""},
				{11, "ScalarMultiLine", "type", "scalar", ""},
				{14, "ObjectSingleLine", "type", "type", ""},
				{15, "no_description", "field", "field", "ObjectSingleLine"},
				{18, "single_line", "field", "field", "ObjectSingleLine"},
				{25, "multi_line", "field", "field", "ObjectSingleLine"},
				{46, "ObjectMultiLine", "type", "type", ""},
				{47, "foo", "field", "field", "ObjectMultiLine"},
				{51, "InterfaceSingleLine", "interface", "interface", ""},
				{52, "no_description", "field", "field", "InterfaceSingleLine"},
				{55, "single_line", "field", "field", "InterfaceSingleLine"},
				{62, "multi_line", "field", "field", "InterfaceSingleLine"},
				{83, "InterfaceMultiLine", "interface", "interface", ""},
				{84, "foo", "field", "field", "InterfaceMultiLine"},
				{88, "UnionSingleLine", "type", "union", ""},
				{95, "UnionSingleLine", "type", "union", ""},
				{98, "EnumSingleLine", "enum", "enum", ""},
				{118, "EnumMultiLine", "enum", "enum", ""},
				{123, "InputSingleLine", "type", "input", ""},
				{124, "a", "field", "field", "InputSingleLine"},
				{127, "b", "field", "field", "InputSingleLine"},
				{134, "c", "field", "field", "InputSingleLine"},
				{135, "d", "field", "field", "InputSingleLine"},
				{143, "InputMultiLine", "type", "input", ""},
				{144, "foo", "field", "field", "InputMultiLine"},
			},
		},
		{
			project: "graphql57-go-audit-selected",
			file:    "kitchen-sink.graphql",
			want: []declaration{
				{3, "namedQuery", "operation", "query", ""},
				{24, "favPost", "operation", "mutation", ""},
				{32, "PostFavSubscription", "operation", "subscription", ""},
				{45, "frag", "section", "fragment", ""},
			}, // Anonymous operations and selections remain outside the declaration scope.
		},
	}
	type original struct {
		path  string
		lines []string
	}
	originals := make([]original, len(cases))
	for i, tc := range cases {
		path := filepath.Join(root, tc.project, filepath.FromSlash(tc.file))
		requireAdditionalOriginalFile(t, path)
		bytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		originals[i] = original{path: path, lines: strings.Split(strings.ReplaceAll(string(bytes), "\r\n", "\n"), "\n")}
		for _, decl := range tc.want {
			if decl.line < 1 || decl.line > len(originals[i].lines) || !strings.Contains(originals[i].lines[decl.line-1], decl.name) {
				t.Fatalf("invalid independent original declaration %s:%d %q", tc.file, decl.line, decl.name)
			}
		}
	}
	for i, tc := range cases {
		t.Run(tc.project+"/"+filepath.Base(tc.file), func(t *testing.T) {
			src := originals[i]
			h := NewHandler([]string{filepath.Dir(src.path)})
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{src.path}, Language: "graphql",
				Encoding: "utf-8", Presentation: "detailed", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 {
				t.Fatalf("incomplete original-file outline: err=%v failed=%+v result=%+v", err, failed, out)
			}
			expected := map[int]declaration{}
			for _, d := range tc.want {
				if _, ok := expected[d.line]; ok {
					t.Fatalf("duplicate original position: %d", d.line)
				}
				expected[d.line] = d
			}
			actual := map[int]bool{}
			for _, s := range out.Symbols {
				line := s.NameRange.Start.Line
				if filepath.Clean(s.Path) != filepath.Clean(src.path) {
					t.Errorf("wrong file: %+v", s)
				}
				if actual[line] {
					t.Errorf("unexpected second symbol at %d: %s", line, s.Name)
				}
				actual[line] = true
				d, ok := expected[line]
				if !ok {
					t.Errorf("invented GraphQL declaration at %d: %s %s parent %q", line, s.Kind, s.Name, s.ParentQualifiedName)
					continue
				}
				if s.Name != d.name || string(s.Kind) != d.kind || s.NativeKind != d.native || s.ParentQualifiedName != d.parent {
					t.Errorf("incorrect declaration at %d: got %s %s native %q parent %q, want %+v", line, s.Kind, s.Name, s.NativeKind, s.ParentQualifiedName, d)
				}
			}
			for _, d := range tc.want {
				if !actual[d.line] {
					t.Errorf("missing declaration %s at %d", d.name, d.line)
				}
			}
			if len(out.Symbols) != len(tc.want) {
				t.Errorf("original file expected %d supported declarations, tool returned %d", len(tc.want), len(out.Symbols))
			}
			t.Logf("independently inventoried original declarations=%d returned=%d complete=%v", len(tc.want), len(out.Symbols), out.CoverageComplete)
		})
	}
}
