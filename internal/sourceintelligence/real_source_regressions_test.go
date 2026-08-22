package sourceintelligence

import (
	"context"
	"testing"
)

func requireRealSourceComplete(t *testing.T, analyzer SourceAnalyzer, text string) AnalyzerResult {
	t.Helper()
	result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("%s valid source reported partial: %+v", analyzer.Language(), result.Analysis)
	}
	return result
}

func TestRealSourceElixirAnonymousFnAndMultilineStrings(t *testing.T) {
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

func TestRealSourceElixirSigilsDoNotLeakDelimiters(t *testing.T) {
	text := "~w(hello #{ [\"has\", []] } world)s\n" +
		"~s{Escapes \\{ and \\}, with {balancing} # text }\n" +
		"~R'this + i\\s \"a\" regex'\n" +
		"defmodule M do\nend\n"
	requireRealSourceComplete(t, ElixirAnalyzer{}, text)
}

func TestRealSourceRubyAssignedCaseBlock(t *testing.T) {
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

func TestRealSourceRubyAssignedBeginBlock(t *testing.T) {
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

func TestRealSourceRubyPrivateClassMethodDef(t *testing.T) {
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

func TestRealSourceRubyDoBlockParameters(t *testing.T) {
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

func TestRealSourceKotlinBodylessClassAtEOF(t *testing.T) {
	text := "package addressbook\nclass Country(val name: String)"
	result := requireRealSourceComplete(t, KotlinAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "addressbook.Country") {
		t.Fatalf("Kotlin EOF class missing: %v", names)
	}
}

func TestRealSourceAutoHotkeySemicolonInsideCommandArgument(t *testing.T) {
	text := "FileSelectFile, file,,, Images (*.gif; *.jpg; *.png)\n" +
		"helper()\n{\n  return\n}\n"
	requireRealSourceComplete(t, AutoHotkeyAnalyzer{}, text)
}

func TestRealSourceAutoHotkeyOpaqueStringAndContinuationForms(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{
			name: "legacy-command-apostrophe",
			text: "MsgBox, Don't hide this\n" +
				"helper() { return }\n",
		},
		{
			name: "legacy-command-expanded-variable-apostrophes",
			text: "MsgBox, 64, %AppName%, %t_Lang161%`n`n'%SchedDate%'\n" +
				"helper() { return }\n",
		},
		{
			name: "single-quoted-string",
			text: "value := 'quoted \" text { [ ( still data'\n" +
				"helper() { return }\n",
		},
		{
			name: "single-quoted-continuation-string",
			text: "get_identify_regex() => '\n" +
				"(\n" +
				"opaque regex data { [ ( \"\n" +
				")'\n" +
				"helper() { return }\n",
		},
		{
			name: "double-quoted-continuation-string",
			text: "mcode := \"\n" +
				"(LTrim Join\n" +
				"ABC{[()]}DEF\n" +
				")\"\n" +
				"helper() { return }\n",
		},
		{
			name: "legacy-continuation-section",
			text: "Script =\n" +
				"(\n" +
				"#define PmcName \"Pulover's Macro Creator\"\n" +
				"opaque text { [ (\n" +
				")\n" +
				"helper() { return }\n",
		},
		{
			name: "parenthesized-expression-is-not-continuation-section",
			text: "outer() {\n" +
				"  Call(\n" +
				"    (flag ? 1 : 2)\n" +
				"  )\n" +
				"}\n" +
				"helper() { return }\n",
		},
		{
			name: "block-comment-parenthesis-is-not-continuation-section",
			text: "class UIA {\n" +
				"  /*\n" +
				"  (comment text)\n" +
				"  */\n" +
				"  method() {\n" +
				"    Call(\n" +
				"      1\n" +
				"    )\n" +
				"  }\n" +
				"}\n" +
				"helper() { return }\n",
		},
		{
			name: "legacy-assignment-regex-payload",
			text: "pattern=iS)^\\t*global Value:=\"(.+?)\"\n" +
				"helper() { return }\n",
		},
		{
			name: "multiline-expression-string-continuation",
			text: "SetOnlyList := \"GoHome,GoBack,GoForward\n" +
				"    ,Navigate,Focus,Click\"\n" +
				"helper() { return }\n",
		},
		{
			name: "apostrophe-hotkey-label",
			text: "'::\n" +
				"<!'::\n" +
				"helper() { return }\n",
		},
		{
			name: "single-quoted-colon-string-remains-a-string",
			text: "value := '::'\n" +
				"helper() { return }\n",
		},
		{
			name: "modulo-before-single-quoted-string-remains-a-string",
			text: "value := 5 % 'opaque { [ ('\n" +
				"helper() { return }\n",
		},
		{
			name: "v2-equality-expression-remains-structural",
			text: "outer() {\n" +
				"  x = Call(\n" +
				"    1\n" +
				"  )\n" +
				"}\n" +
				"helper() { return }\n",
		}, {
			name: "chained-single-quoted-continuations",
			text: "assert_match pattern, '\n" +
				"(\n" +
				"first\n" +
				")', ['\n" +
				"(\n" +
				"second\n" +
				")']\n" +
				"helper() { return }\n",
		},
		{
			name: "quoted-continuation-closing-line-content",
			text: "value := '(?>\n" +
				"(Join|\n" +
				"body\n" +
				"))'\n" +
				"helper() { return }\n",
		},
		{
			name: "spaced-legacy-raw-assignment",
			text: "pattern = iS)\"name\": \".*\"\n" +
				"other = \"name\": \".*\"\n" +
				"helper() { return }\n",
		},
		{
			name: "legacy-command-backtick-escaped-quotes",
			text: "MsgBox, 35, title, text`\"%CurrentFileName%`\"\n" +
				"helper() { return }\n",
		},
		{
			name: "quoted-literal-backtick",
			text: "value := \"``\"\n" +
				"helper() { return }\n",
		},
		{
			name: "square-bracket-hotkey-labels",
			text: "[::sendinput, x\n" +
				"+[::sendinput, y\n" +
				"helper() { return }\n",
		},
		{
			name: "v2-array-remains-structural",
			text: "value := [1, 2]\n" +
				"helper() { return }\n",
		},
		{
			name: "legacy-command-raw-brace-transition",
			text: "StringReplace, sKey, tKey, +, Shift Down}{\n" +
				"StringReplace, sKey, sKey, !, Alt Down}{\n" +
				"helper() { return }\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := requireRealSourceComplete(t, AutoHotkeyAnalyzer{}, tc.text)
			if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "helper") {
				t.Fatalf("AutoHotkey helper declaration missing after opaque data: %v", names)
			}
		})
	}
}

func TestRealSourceLispCharacterLiteralsAndMultilineStrings(t *testing.T) {
	t.Run("clojure", func(t *testing.T) {
		text := "(def semi (int \\;))\n(def doc \"line one\nline two\")\n"
		requireRealSourceComplete(t, ClojureAnalyzer{}, text)
	})
	t.Run("emacs-lisp", func(t *testing.T) {
		text := "(defconst quote-char ?\\\")\n(defconst brace-char ?\\{)\n(defun real () \"line one\nline two\")\n"
		requireRealSourceComplete(t, EmacsLispAnalyzer{}, text)
	})
}

func TestRealSourceScalaSymbolLiteral(t *testing.T) {
	text := "object M {\n  val symbol = 'symbol\n  val char = 'a'\n}\n"
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaTripleStringClosingQuoteRun(t *testing.T) {
	text := "object M {\n  val value = \"\"\"one \", two \"\", three \"\"\"\"\"\"\n}\n"
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceDQStringFamilies(t *testing.T) {
	text := "void real() {\n" +
		"  auto a = q\"{text { nested } text}\";\n" +
		"  auto b = q\"[text [ nested ] text]\";\n" +
		"  auto c = q\"/text \\\" quoted /\";\n" +
		"  auto d = q\"TOKEN\ntext { with braces }\nTOKEN\";\n" +
		"  auto e = q{ token { nested } string };\n" +
		"}\n"
	requireRealSourceComplete(t, DAnalyzer{}, text)
}

func TestRealSourceNimDocBlockComment(t *testing.T) {
	text := "proc real =\n" +
		"  ##[\n" +
		"  docs [with] delimiters\n" +
		"  ]##\n" +
		"  discard\n"
	requireRealSourceComplete(t, NimAnalyzer{}, text)
}

func TestRealSourceNimNumericSuffixAndQuotedIdentifier(t *testing.T) {
	text := "template real =\n" +
		"  let a = 0xdeadBEEF'wrap\n" +
		"  let b = -123'32\n" +
		"proc `'quoted`(a: string): int = discard\n"
	result := requireRealSourceComplete(t, NimAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "real") {
		t.Fatalf("Nim declaration missing after numeric suffixes: %v", names)
	}
}

func TestRealSourceTwigCompositeSymbolsStayOrdered(t *testing.T) {
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

func TestRealSourcePowerShellNestedHereStringsInsideInterpolation(t *testing.T) {
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
