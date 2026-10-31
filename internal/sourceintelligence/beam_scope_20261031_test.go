package sourceintelligence

import (
	"context"
	"os"
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
