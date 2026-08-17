package sourceintelligence

import (
	"context"
	"testing"
)

func requireRealSourceComplete(t *testing.T, analyzer SourceAnalyzer, text string) AnalyzerResult {
	t.Helper()
	result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("%s valid source reported partial: %+v", analyzer.Language(), result.Analysis)
	}
	return result
}

func TestR27RealSourceElixirAnonymousFnAndMultilineStrings(t *testing.T) {
	text := "defmodule M do\n" +
		"  @doc \"line one\nline two\"\n" +
		"  Enum.reduce([1], 0, fn x, acc ->\n" +
		"    x + acc\n" +
		"  end)\n" +
		"  def map(xs) do\n" +
		"    Enum.map(xs, fn x -> x end)\n" +
		"  end\n" +
		"end\n" +
		"defprotocol P do\n  def run(x)\nend\n" +
		"defimpl P, for: Atom do\nend\n"
	requireRealSourceComplete(t, ElixirAnalyzer{}, text)
}

func TestR27RealSourceElixirSigilsDoNotLeakDelimiters(t *testing.T) {
	text := "~w(hello #{ [\"has\", []] } world)s\n" +
		"~s{Escapes \\{ and \\}, with {balancing} # text }\n" +
		"~R'this + i\\s \"a\" regex'\n" +
		"defmodule M do\nend\n"
	requireRealSourceComplete(t, ElixirAnalyzer{}, text)
}

func TestR27RealSourceRubyAssignedCaseBlock(t *testing.T) {
	text := "module M\n" +
		"  def run(value)\n" +
		"    result = case value\n" +
		"    when 1 then :one\n" +
		"    else :other\n" +
		"    end\n" +
		"    result\n" +
		"  end\n" +
		"end\n"
	requireRealSourceComplete(t, RubyAnalyzer{}, text)
}

func TestR27RealSourceRubyAssignedBeginBlock(t *testing.T) {
	text := "module M\n" +
		"  def run\n" +
		"    @value ||= begin\n" +
		"      1\n" +
		"    rescue\n" +
		"      2\n" +
		"    ensure\n" +
		"      cleanup\n" +
		"    end\n" +
		"  end\n" +
		"end\n"
	requireRealSourceComplete(t, RubyAnalyzer{}, text)
}

func TestR27RealSourceRubyPrivateClassMethodDef(t *testing.T) {
	text := "class M\n" +
		"  private_class_method def self.run(value)\n" +
		"    value\n" +
		"  end\n" +
		"end\n"
	result := requireRealSourceComplete(t, RubyAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "M.run") {
		t.Fatalf("Ruby private class method missing: %v", names)
	}
}

func TestR27RealSourceRubyDoBlockParameters(t *testing.T) {
	text := "module M\n" +
		"  [1].each do |value|\n" +
		"    value\n" +
		"  end\n" +
		"  def run\n" +
		"    1\n" +
		"  end\n" +
		"end\n"
	result := requireRealSourceComplete(t, RubyAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "M.run") {
		t.Fatalf("Ruby method missing after do block: %v", names)
	}
}

func TestR27RealSourceKotlinBodylessClassAtEOF(t *testing.T) {
	text := "package addressbook\nclass Country(val name: String)"
	result := requireRealSourceComplete(t, KotlinAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "addressbook.Country") {
		t.Fatalf("Kotlin EOF class missing: %v", names)
	}
}

func TestR27RealSourceAutoHotkeySemicolonInsideCommandArgument(t *testing.T) {
	text := "FileSelectFile, file,,, Images (*.gif; *.jpg; *.png)\n" +
		"helper()\n{\n  return\n}\n"
	requireRealSourceComplete(t, AutoHotkeyAnalyzer{}, text)
}

func TestR27RealSourceLispCharacterLiteralsAndMultilineStrings(t *testing.T) {
	t.Run("clojure", func(t *testing.T) {
		text := "(def semi (int \\;))\n(def doc \"line one\nline two\")\n"
		requireRealSourceComplete(t, ClojureAnalyzer{}, text)
	})
	t.Run("emacs-lisp", func(t *testing.T) {
		text := "(defconst quote-char ?\\\")\n(defconst brace-char ?\\{)\n(defun real () \"line one\nline two\")\n"
		requireRealSourceComplete(t, EmacsLispAnalyzer{}, text)
	})
}

func TestR27RealSourceScalaSymbolLiteral(t *testing.T) {
	text := "object M {\n  val symbol = 'symbol\n  val char = 'a'\n}\n"
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestR27RealSourceScalaTripleStringClosingQuoteRun(t *testing.T) {
	text := "object M {\n  val value = \"\"\"one \", two \"\", three \"\"\"\"\"\"\n}\n"
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestR27RealSourceDQStringFamilies(t *testing.T) {
	text := "void real() {\n" +
		"  auto a = q\"{text { nested } text}\";\n" +
		"  auto b = q\"[text [ nested ] text]\";\n" +
		"  auto c = q\"/text \\\" quoted /\";\n" +
		"  auto d = q\"TOKEN\ntext { with braces }\nTOKEN\";\n" +
		"  auto e = q{ token { nested } string };\n" +
		"}\n"
	requireRealSourceComplete(t, DAnalyzer{}, text)
}

func TestR27RealSourceNimDocBlockComment(t *testing.T) {
	text := "proc real =\n" +
		"  ##[\n" +
		"  docs [with] delimiters\n" +
		"  ]##\n" +
		"  discard\n"
	requireRealSourceComplete(t, NimAnalyzer{}, text)
}

func TestR27RealSourceNimNumericSuffixAndQuotedIdentifier(t *testing.T) {
	text := "template real =\n" +
		"  let a = 0xdeadBEEF'wrap\n" +
		"  let b = -123'32\n" +
		"proc `'quoted`(a: string): int = discard\n"
	result := requireRealSourceComplete(t, NimAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "real") {
		t.Fatalf("Nim declaration missing after numeric suffixes: %v", names)
	}
}

func TestR27RealSourceTwigCompositeSymbolsStayOrdered(t *testing.T) {
	text := "{% macro input(name) %}<input name=\"{{ name }}\">{% endmacro %}\n<div id=\"later\"></div>\n"
	result := requireRealSourceComplete(t, TwigAnalyzer{}, text)
	for index := 1; index < len(result.Analysis.Symbols); index++ {
		left := result.Analysis.Symbols[index-1]
		right := result.Analysis.Symbols[index]
		if right.declarationOffsets.Start < left.declarationOffsets.Start || right.declarationOffsets.Start == left.declarationOffsets.Start && right.declarationOffsets.End < left.declarationOffsets.End {
			t.Fatalf("Twig symbols out of source order at %d: left=%+v right=%+v", index, left, right)
		}
	}
}

func TestR27RealSourcePowerShellNestedHereStringsInsideInterpolation(t *testing.T) {
	text := "function Real {\n" +
		"@\"\n" +
		"outer\n" +
		"$(\n" +
		"@\"\ninner $value\n\"@\n" +
		"@'\nliteral\n'@\n" +
		")\n" +
		"outer tail\n" +
		"\"@\n" +
		"}\n"
	result := requireRealSourceComplete(t, PowerShellAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "Real") {
		t.Fatalf("PowerShell function missing after nested here-string: %v", names)
	}
}
