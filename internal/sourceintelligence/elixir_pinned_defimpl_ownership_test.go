package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independent whole Git-pinned originals contain direct source declarations,
// not inferred protocol dispatch: one protocol, explicit defimpl owners and
// their authored function clauses. Quote-generated members remain excluded.
func TestElixirPinnedDirectDefimplOwners(t *testing.T) {
	elixirRoot := os.Getenv("SCRIPTHOLD_REAL_ELIXIR_OUTLINE_ROOT")
	phoenixRoot := os.Getenv("SCRIPTHOLD_REAL_ELIXIR_PHOENIX_HTML_ROOT")
	if elixirRoot == "" || phoenixRoot == "" {
		t.Skip("both independent Git-pinned originals required")
	}
	cases := []struct {
		name, path     string
		bytes, count   int
		implementation map[string]struct{ line, clauses int }
	}{
		{"elixir-string-chars", filepath.Join(elixirRoot, "elixir-string-chars.ex"), 1476, 14,
			map[string]struct{ line, clauses int }{
				"String.Chars.Atom":      {29, 2},
				"String.Chars.BitString": {39, 2},
				"String.Chars.List":      {52, 1},
				"String.Chars.Integer":   {56, 1},
				"String.Chars.Float":     {62, 1},
			}},
		{"phoenix-html-form-data", filepath.Join(phoenixRoot, "phoenix-html-form-data.ex"), 6078, 13,
			map[string]struct{ line, clauses int }{
				"Phoenix.HTML.FormData.Map": {79, 7},
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := os.Lstat(tc.path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != int64(tc.bytes) {
				t.Fatalf("Git-pinned original identity mismatch: %v %v", info, err)
			}
			original, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			for _, eol := range []struct{ name, source string }{
				{"LF", string(original)}, {"CRLF", strings.ReplaceAll(string(original), "\n", "\r\n")},
			} {
				t.Run(eol.name, func(t *testing.T) {
					result, err := (ElixirAnalyzer{}).Analyze(context.Background(),
						testSourceDocument(tc.name+".ex", eol.source), testAnalyzeOptions(true, 256))
					if err != nil {
						t.Fatal(err)
					}
					if result.Analysis.Truncated || len(result.Analysis.Symbols) != tc.count {
						t.Fatalf("all supported direct authored declarations expected %d, got %d: %+v",
							tc.count, len(result.Analysis.Symbols), result.Analysis.Diagnostics)
					}
					owners := map[string]NormalizedSymbol{}
					children := map[string]int{}
					for _, s := range result.Analysis.Symbols {
						if s.NativeKind == "defimpl" {
							if s.Kind != SymbolKindImplementation || s.NameRange.Start.Line != tc.implementation[s.QualifiedName].line ||
								s.DeclarationRange.Start.Line != s.NameRange.Start.Line {
								t.Errorf("incorrect authored implementation source identity: %+v", s)
							}
							if _, ok := tc.implementation[s.QualifiedName]; !ok {
								t.Errorf("invented implementation %+v", s)
							}
							owners[s.QualifiedName] = s
						}
						if s.ParentQualifiedName != "" {
							children[s.ParentQualifiedName]++
						}
					}
					for name, want := range tc.implementation {
						owner, ok := owners[name]
						if !ok {
							t.Errorf("missing direct authored implementation %s", name)
							continue
						}
						childCount := 0
						for _, s := range result.Analysis.Symbols {
							if s.ParentQualifiedName == name {
								childCount++
								if s.ParentID != owner.ID || s.Kind != SymbolKindFunction {
									t.Errorf("unowned/non-callable implementation child: %+v", s)
								}
							}
						}
						if childCount != want.clauses {
							t.Errorf("%s has %d methods, want %d", name, childCount, want.clauses)
						}
					}
					_ = children
				})
			}
		})
	}
}

func TestElixirDirectDefimplLexicalOwnerNegativeAndLFCRLF(t *testing.T) {
	const input = "defprotocol Demo.Fmt do\n  def show(value)\nend\n" +
		"defimpl Demo.Fmt, for: Map do\n  def show(value), do: value\nend\n" +
		"defimpl Demo.Fmt, for: [Atom, Integer] do\n  def show(value), do: value\nend\n" +
		"defmodule Other do\n  def direct(), do: :ok\n  quote do\n    defimpl Demo.Fmt, for: List do\n      def generated(value), do: value\n    end\n  end\nend\n"
	for _, eol := range []struct{ name, source string }{{"LF", input}, {"CRLF", strings.ReplaceAll(input, "\n", "\r\n")}} {
		t.Run(eol.name, func(t *testing.T) {
			result, err := (ElixirAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("demo.ex", eol.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			byQualified := symbolsByQualifiedName(result.Analysis.Symbols)
			owner, ok := byQualified["Demo.Fmt.Map"]
			child, childOK := byQualified["Demo.Fmt.Map.show"]
			if !ok || !childOK || owner.NativeKind != "defimpl" ||
				owner.NameRange.Start.Line != 4 || child.NameRange.Start.Line != 5 ||
				child.ParentID != owner.ID {
				t.Errorf("direct implementation owner absent: owner=%+v child=%+v", owner, child)
			}
			for _, name := range []string{"Demo.Fmt.List", "Demo.Fmt.Atom", "Demo.Fmt.Integer", "Demo.Fmt.List.generated"} {
				if _, ok := byQualified[name]; ok {
					t.Errorf("unsupported multi-target or quote-generated declaration %s", name)
				}
			}
			other, ok := byQualified["Other.direct"]
			if !ok || other.NameRange.Start.Line != 11 {
				t.Errorf("unrelated module direct member changed: %+v", other)
			}
		})
	}
}
