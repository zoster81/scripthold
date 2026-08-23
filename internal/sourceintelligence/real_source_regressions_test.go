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

func TestRealSourceAstroSelfClosingScriptDoesNotCaptureLaterScript(t *testing.T) {
	text := `---
const id = "analytics";
---
{id && <script is:inline src={` + "`" + `https://example.test/tag.js?id=${id}` + "`" + `} />}
<script is:inline>
window.analytics = { enabled: true };
</script>
`
	requireRealSourceComplete(t, AstroAnalyzer{}, text)
}

func TestRealSourceASPNetWebFormsServerCommentIsOpaque(t *testing.T) {
	text := `<%@ Page Language="C#" %>
<%-- Styling the "Copy: Button Box --%>
<script runat="server">
public void Real() { }
</script>
`
	requireRealSourceComplete(t, ASPNetWebFormsAnalyzer{}, text)
}

func TestRealSourceBlazorRenderFragmentMarkupIsOpaqueToCSharp(t *testing.T) {
	text := `@code {
    private string Prefix { get; set; } = "demo";
    private void SetIcon()
    {
        RenderFragment item = @<span class="@($"{Prefix}-item")">value</span>;
        @* punctuation in Razor comment :) *@
    }
}
`
	requireRealSourceComplete(t, BlazorAnalyzer{}, text)
}

func TestRealSourceBlazorRenderFragmentBlocksAreOpaqueToCSharp(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{
			name: "explicit-ul-fragment",
			text: `@code {
    private void Show()
    {
        Service.Add(
            @<ul>
                <li>Here's a <strong>bold item</strong></li>
            </ul>
        );
    }
}
`,
		},
		{
			name: "text-fragment",
			text: `@code {
    RenderFragment<string> Build = version =>
        @<text>
            <Widget Anchor="@($"get-started/{version}")">Install</Widget>
            <ul>
                <li>Via Visual Studio's package manager.</li>
                <li>By editing your application's project file.</li>
            </ul>
        </text>;
}
`,
		},
		{
			name: "line-leading-markup-lambda-body",
			text: `@code {
    RenderFragment<bool> Build = value => builder =>
    {
        <Grid Rows="@(new List<int>
            {
                1,
                2
            })">
            <Cell Text="@value" />
        </Grid>
    };
}
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireRealSourceComplete(t, BlazorAnalyzer{}, tc.text)
		})
	}
}

func TestRealSourceBlazorMarkupMaskingRemainsFailClosed(t *testing.T) {
	t.Run("csharp-generic-and-comparison", func(t *testing.T) {
		text := `@code {
    private List<int> Items { get; } = new();
    private bool Less(int left, int right) => left < right;
}
`
		result := requireRealSourceComplete(t, BlazorAnalyzer{}, text)
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		for _, want := range []string{"Items", "Less"} {
			if !containsSortedString(names, want) {
				t.Fatalf("Blazor C# declaration %q missing after markup masking: %v", want, names)
			}
		}
	})
	t.Run("unterminated-markup-remains-partial", func(t *testing.T) {
		text := `@code {
    private void Build()
    {
        @<div>
            broken
    }
}
`
		result, err := (BlazorAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 256))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete {
			t.Fatalf("unterminated Blazor markup unexpectedly reported complete: %+v", result.Analysis)
		}
	})
}

func TestRealSourceDOrdinaryStringMaySpanPhysicalLines(t *testing.T) {
	text := `void real()
{
    auto files = "
        one.d two.d
        three.d
    ";
}
`
	requireRealSourceComplete(t, DAnalyzer{}, text)
}

func TestRealSourceDartRawStringsDoNotUseBackslashEscapes(t *testing.T) {
	text := `void real(String value) {
  final escaped = value.replaceAll(r'\', r'\\').replaceAll("'", r"\'");
}
`
	requireRealSourceComplete(t, DartAnalyzer{}, text)
}

func TestRealSourceDAnonymousUnionInsideStruct(t *testing.T) {
	text := `struct Request
{
    int prefix;
    union
    {
        int command;
        ubyte[5] header;
    }
    int suffix;
}
`
	result := requireRealSourceComplete(t, DAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Request", "Request.command", "Request.header", "Request.prefix", "Request.suffix"} {
		if !containsSortedString(names, want) {
			t.Fatalf("D anonymous-union declaration %q missing: %v", want, names)
		}
	}
}

func TestRealSourceEJSSplitControlFlowSharesOneJavaScriptProjection(t *testing.T) {
	text := `<main>
<% if (items.length) { %>
  <span><%= items[0] %></span>
<% } else { %>
  <span>empty</span>
<% } %>
</main>
`
	requireRealSourceComplete(t, EJSAnalyzer{}, text)
}

func TestRealSourceScalaTypedConstructorColonIsNotBodyColon(t *testing.T) {
	text := `package demo
case class Record(name: String, count: Int) extends Product
final class Failure(cause: Throwable) extends Exception(cause)
`
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaUnterminatedColonBodyRemainsPartial(t *testing.T) {
	text := `class Broken:
`
	result, err := (ScalaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Fatalf("unterminated Scala colon body unexpectedly reported complete: %+v", result.Analysis)
	}
}

func TestRealSourcePOSIXShellCasePatternsAreNotDelimiterClosers(t *testing.T) {
	text := `pick() {
    case "$1" in
        alpha | beta) printf '%s\\n' ok ;;
        *) printf '%s\\n' fallback ;;
    esac
}
`
	requireRealSourceComplete(t, ShellAnalyzer{}, text)
}

func TestRealSourcePOSIXShellParenthesizedCasePattern(t *testing.T) {
	text := `pick() {
    case "$1" in
        ( alpha | beta ) printf '%s\\n' ok ;;
        ( * ) printf '%s\\n' fallback ;;
    esac
}
`
	requireRealSourceComplete(t, ShellAnalyzer{}, text)
}

func TestRealSourceScalaInterpolatedExpressionMaySpanLines(t *testing.T) {
	text := `object Demo {
  def value(names: List[String]): String =
    s"names = [${names.mkString("\\\"", "\\\", \\\"", "\\\"")}]"
  def other(flag: Boolean): String =
    s"/${if (flag) { "yes" }
    else { "no" }}/done"
}
`
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaNestedColonBodyUsesPhysicalIndentation(t *testing.T) {
	text := `object Outer {
  object Inner:
    private var value: Int = 0
    def get: Int = value
}
`
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaInitializerBracesAreNotDeclarationBodies(t *testing.T) {
	text := `object Demo {
  val mapped = List(1, 2).map { value => value + 1 }
  def plural(values: List[Int]) = s"${values.size}${if (values.size == 1) "" else "s"}"
  def block(value: Int) = {
    value + 1
  }
}
`
	result, err := (ScalaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid Scala initializer source reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	mapped, mappedOK := byName["Demo.mapped"]
	plural, pluralOK := byName["Demo.plural"]
	block, blockOK := byName["Demo.block"]
	if !mappedOK || !pluralOK || !blockOK {
		t.Fatalf("Scala declarations missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if mapped.BodyRange != nil {
		t.Fatalf("val initializer lambda became declaration body: %+v", mapped.BodyRange)
	}
	if plural.BodyRange != nil {
		t.Fatalf("interpolation expression became method body: %+v", plural.BodyRange)
	}
	if block.BodyRange == nil {
		t.Fatalf("real braced method body was not recognized: %+v", block)
	}
}

func TestRealSourceScalaOrdinaryStringsEndingLikeInterpolatorPrefixesStayOpaque(t *testing.T) {
	text := `object Json {
  def append(c: Char, sb: StringBuilder): Unit = c match {
    case '\f' => sb.append("\\f")
    case '\n' => sb.append("\\n")
    case _ => sb.append("application/vnd.github.v3.raw")
  }
}
`
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaCustomInterpolatorExpressionMaySpanLines(t *testing.T) {
	text := `object Demo {
  def value(flag: Boolean): String =
    em"prefix ${if (flag) { "yes" }
    else { "" }} suffix"
}
`
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaDefInitializerLambdaIsNotMethodBody(t *testing.T) {
	text := `object Demo {
  def lookup: Int = List(1).collectFirst { case value => value }.get
  def block: Int = {
    1
  }
}
`
	result, err := (ScalaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid Scala def initializer source reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	lookup, lookupOK := byName["Demo.lookup"]
	block, blockOK := byName["Demo.block"]
	if !lookupOK || !blockOK {
		t.Fatalf("Scala declarations missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if lookup.BodyRange != nil {
		t.Fatalf("def initializer lambda became method body: %+v", lookup.BodyRange)
	}
	if block.BodyRange == nil {
		t.Fatalf("direct braced method body was not recognized: %+v", block)
	}
}
func TestRealSourceScalaTripleQuotesInsideLineCommentsStayOpaque(t *testing.T) {
	text := `class InitializeListener {
//  private val system = ConfigFactory.parseString("""
//    |akka {
//    |  daemonic = on
//    |}
//  """.stripMargin)

  override def contextInitialized(): Unit = {
    ()
  }
}
`
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaSameLineDeclarationAfterBracedDef(t *testing.T) {
	text := `object Sym {
  def nextSymId = { value += 1; value }; private var value = 0
}
`
	result, err := (ScalaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid Scala same-line declaration source reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	nextSymID, ok := byName["Sym.nextSymId"]
	if !ok {
		t.Fatalf("Scala method missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if nextSymID.BodyRange == nil || nextSymID.SignatureRange == nil {
		t.Fatalf("Scala method ranges missing: %+v", nextSymID)
	}
	if nextSymID.SignatureRange.End.Line > nextSymID.BodyRange.Start.Line ||
		(nextSymID.SignatureRange.End.Line == nextSymID.BodyRange.Start.Line && nextSymID.SignatureRange.End.Column > nextSymID.BodyRange.Start.Column) {
		t.Fatalf("Scala signature extends into/past body: signature=%+v body=%+v", nextSymID.SignatureRange, nextSymID.BodyRange)
	}
}

func TestRealSourceGraphQLHashInsideDirectiveStringIsNotAComment(t *testing.T) {
	text := `type View implements Node {
  fields(orderBy: FieldOrder = {field: POSITION, direction: ASC}): FieldConnection
    @deprecated(reason: "View#fields remains available")
}
`
	requireRealSourceComplete(t, GraphQLAnalyzer{}, text)
}

func TestRealSourceGraphQLDescriptionQuotedFragmentDoesNotPoisonDirectiveString(t *testing.T) {
	text := `"""
See "[guide](docs#anchor)" for details.
"""
type View {
  value: String @deprecated(reason: "View#value remains available")
}
`
	result, err := (GraphQLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid GraphQL description/directive source reported partial: %+v", result.Analysis)
	}
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "View") {
		t.Fatalf("GraphQL description/directive masking hid following type: %v", names)
	}
}

func TestRealSourcePOSIXShellArithmeticShiftsAreNotHeredocs(t *testing.T) {
	text := `hash_value() {
    value=$(( (1 << 16) + ($1 >> 2) ))
    printf '%s\\n' "$value"
}
`
	requireRealSourceComplete(t, ShellAnalyzer{}, text)
}

func TestRealSourcePOSIXShellBracketCommandIsNotStructuralDelimiter(t *testing.T) {
	text := `install_test() {
    if ! extern -pv [ >/dev/null && testcmd=$(extern -pv test); then
        ln -s $testcmd $HOME/bin/[
        if $HOME/bin/[ 1 -eq 1 ] 2>/dev/null; then
            PATH=$PATH command rm $HOME/bin/[
        fi
    fi
}
`
	requireRealSourceComplete(t, ShellAnalyzer{}, text)
}

func TestRealSourcePOSIXShellNegatedCaseCommandIsRecognized(t *testing.T) {
	text := `probe() {
    { ! : || ! case x in x) ;; esac; } && exit 1
}
`
	requireRealSourceComplete(t, ShellAnalyzer{}, text)
}

func TestRealSourcePOSIXShellNestedCaseMayStartCaseArmBody(t *testing.T) {
	text := `check_range() {
    case $2 in
        @*) case ${2#@} in (*[!0-9-]*) return 1; esac ;;
        *) case $2 in (*[!0-9]*) return 1; esac ;;
    esac
}
`
	requireRealSourceComplete(t, ShellAnalyzer{}, text)
}
func TestRealSourceTOMLArraysOfTablesAndMultilineArrays(t *testing.T) {
	text := `[tool.demo]
commands = [
  ["mypy"],
  ["pyright", "--verifytypes", "demo"],
]

[[tool.demo.overrides]]
module = "first"

[[tool.demo.overrides]]
module = "second"
`
	requireRealSourceComplete(t, TOMLAnalyzer{}, text)
}
