package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBEAMProtocolAndQuotedModuleOwnership(t *testing.T) {
	cases := []struct {
		name      string
		analyzer  SourceAnalyzer
		source    string
		owner     string
		kind      SymbolKind
		child     string
		childLine int
		ownerLine int
	}{
		{"elixir", ElixirAnalyzer{}, "defprotocol Demo.Encoder do\n  def encode(value)\nend\ndefmodule Demo.Other do\n  def outside(), do: :ok\nend\n", "Demo.Encoder", SymbolKindInterface, "Demo.Encoder.encode", 2, 1},
		{"erlang", ErlangAnalyzer{}, "-module('chat').\nlogin(User, Password) -> {User, Password}.\n", "chat", SymbolKindModule, "chat.login", 2, 1},
	}
	for _, tc := range cases {
		for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"/"+eol.name, func(t *testing.T) {
				source := strings.ReplaceAll(tc.source, "\n", eol.value)
				result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				parent, ok := symbols[tc.owner]
				if !ok || parent.Kind != tc.kind || parent.NameRange.Start.Line != tc.ownerLine {
					t.Fatalf("owner %s: %+v found=%v symbols=%v", tc.owner, parent, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
				child, ok := symbols[tc.child]
				if !ok || child.Kind != SymbolKindFunction || child.ParentID != parent.ID || child.ParentQualifiedName != tc.owner || child.NameRange.Start.Line != tc.childLine {
					t.Fatalf("child %s: %+v found=%v symbols=%v", tc.child, child, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			})
		}
	}
}

func TestElixirPinnedQuoteGeneratedMembers(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_ELIXIR_OUTLINE_ROOT")
	if root == "" {
		t.Skip("originals not configured")
	}
	for _, tc := range []struct {
		file string
		want int
	}{{"phoenix-pipeline.ex", 24}, {"elixir-string-chars.ex", 9}} {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, tc.file))
			if err != nil || len(data) == 0 || len(data) > 1<<20 {
				t.Fatalf("missing complete original: %v", err)
			}
			result, err := (ElixirAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(string(data)), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != tc.want {
				t.Errorf("complete original got %d direct symbols, want %d", len(result.Analysis.Symbols), tc.want)
			}
			if tc.file == "phoenix-pipeline.ex" {
				negative := map[int]bool{17: true, 20: true, 30: true, 93: true, 109: true}
				for _, sym := range result.Analysis.Symbols {
					if negative[sym.NameRange.Start.Line] {
						t.Errorf("quote AST misreported as direct member: %+v", sym)
					}
				}
			}
		})
	}
}

func TestElixirQuotedScopesLFCRLF(t *testing.T) {
	lines := []string{
		"defmodule Demo do",
		"  defmacro build do",
		"    quote do",
		"      def phantom, do: :ok",
		"      if true do",
		"        def nested_phantom, do: :ok",
		"      end",
		"    end",
		"  end",
		"  defp other do",
		"    ast = quote bind_quoted: [value: 1] do",
		"      def bound_phantom, do: value",
		"    end",
		"  end",
		"  def direct, do: :ok",
		"end",
	}
	for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.Join(lines, eol.value) + eol.value)
			result, err := (ElixirAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Errorf("properly closed quoted blocks marked partial: %+v", result.Analysis.Diagnostics)
			}
			names := map[string]int{}
			for _, sym := range result.Analysis.Symbols {
				names[sym.QualifiedName]++
			}
			for _, name := range []string{"Demo", "Demo.build", "Demo.other", "Demo.direct"} {
				if names[name] != 1 {
					t.Errorf("authored declaration %s count %d", name, names[name])
				}
			}
			for _, name := range []string{"Demo.phantom", "Demo.nested_phantom", "Demo.bound_phantom"} {
				if names[name] != 0 {
					t.Errorf("quote-only declaration %s leaked: %+v", name, names)
				}
			}
			if len(result.Analysis.Symbols) != 4 {
				t.Errorf("expected four direct symbols, got %d: %+v", len(result.Analysis.Symbols), names)
			}
		})
	}
}

func TestBEAMOriginalProtocolAndQuotedModuleOwnership(t *testing.T) {
	cases := []struct {
		env       string
		analyzer  SourceAnalyzer
		owner     string
		kind      SymbolKind
		ownerLine int
		child     string
		childLine int
	}{
		{"SCRIPTHOLD_REAL_ELIXIR_PLAUSIBLE_PROTOCOL", ElixirAnalyzer{}, "Plausible.Audit.Encoder", SymbolKindInterface, 5, "Plausible.Audit.Encoder.encode", 6},
		{"SCRIPTHOLD_REAL_ERLANG_YAWS_CHAT", ErlangAnalyzer{}, "chat", SymbolKindModule, 7, "chat.login", 38},
	}
	for _, tc := range cases {
		t.Run(tc.owner, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("original not configured")
			}
			fi, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if fi.Size() > 1<<20 {
				t.Fatal("original exceeds 1MiB bound")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc := sourceDocumentForScanner(string(data))
			doc.Path = path
			result, err := tc.analyzer.Analyze(context.Background(), doc, testAnalyzeOptions(true, 20000))
			if err != nil {
				t.Fatal(err)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			parent, ok := symbols[tc.owner]
			if !ok || parent.Kind != tc.kind || parent.NameRange.Start.Line != tc.ownerLine {
				t.Fatalf("owner %s wrong: %+v found=%v", tc.owner, parent, ok)
			}
			foundChild := false
			for _, child := range result.Analysis.Symbols {
				if child.QualifiedName == tc.child && child.ParentID == parent.ID && child.NameRange.Start.Line == tc.childLine {
					foundChild = true
				}
			}
			if !foundChild {
				t.Fatalf("child %s not found at original line %d", tc.child, tc.childLine)
			}
		})
	}
}
