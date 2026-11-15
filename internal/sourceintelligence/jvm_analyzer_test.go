package sourceintelligence

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

var _ SourceAnalyzer = JavaAnalyzer{}
var _ SourceAnalyzer = KotlinAnalyzer{}

func TestJavaAnalyzerPackagesImportsTypesMembersAndRelations(t *testing.T) {
	text := `package demo.core;
import java.util.List;
import static java.util.Collections.emptyList;
public sealed class Box<T> extends Base implements Runnable permits Child {
    private T value;
    public Box(T value) { this.value = value; }
    public T get() { return value; }
    public void run() {}
    record Nested(int id) {}
    String fake = "class Fake { void Nope() {} }";
}
interface Service extends AutoCloseable { void execute(); }
enum State { READY, DONE }
record Pair(int left, int right) implements Serializable {}
`
	document := sourceDocumentForScanner(text)
	document.Path = "Box.java"
	result, err := (JavaAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Java partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"demo.core": SymbolKindPackage, "demo.core.Box": SymbolKindClass, "demo.core.Box.value": SymbolKindField,
		"demo.core.Box.Box": SymbolKindConstructor, "demo.core.Box.get": SymbolKindMethod, "demo.core.Box.run": SymbolKindMethod,
		"demo.core.Box.Nested": SymbolKindRecord, "demo.core.Service": SymbolKindInterface,
		"demo.core.Service.execute": SymbolKindMethod, "demo.core.State": SymbolKindEnum, "demo.core.Pair": SymbolKindRecord,
	} {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if _, ok := byName["demo.core.Box.Fake"]; ok {
		t.Fatal("Java string leaked Fake")
	}
	if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{"java.util.List", "java.util.Collections.emptyList"}) {
		t.Fatalf("Java imports = %v", got)
	}
	for _, rel := range [][3]string{{"extends", "demo.core.Box", "Base"}, {"implements", "demo.core.Box", "Runnable"}, {"permits", "demo.core.Box", "Child"}, {"extends", "demo.core.Service", "AutoCloseable"}, {"implements", "demo.core.Pair", "Serializable"}} {
		if !hasStructuralRelation(result.Relations, rel[0], rel[1], rel[2]) {
			t.Fatalf("missing Java relation %v in %+v", rel, result.Relations)
		}
	}
}

func TestKotlinComplexEnumEntriesStayPartial(t *testing.T) {
	source := "package demo\nenum class Complex(val code: Int) {\n  FIRST(1),\n  SECOND(2);\n  fun label() {}\n}\nclass After {}\n"
	for _, eol := range []string{"\n", "\r\n"} {
		result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
			sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol)), testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete {
			t.Errorf("%q complex enum syntax must not claim complete coverage", eol)
		}
		symbols := symbolsByQualifiedName(result.Analysis.Symbols)
		if _, ok := symbols["demo.Complex"]; !ok {
			t.Errorf("%q owning enum disappeared", eol)
		}
		for _, fake := range []string{"demo.Complex.FIRST", "demo.Complex.SECOND", "demo.Complex.1", "demo.Complex.2"} {
			if _, exists := symbols[fake]; exists {
				t.Errorf("%q fabricated unsupported enum member %s", eol, fake)
			}
		}
		if next, ok := symbols["demo.After"]; !ok || next.ParentQualifiedName != "demo" ||
			next.DeclarationRange.Start.Line != 7 {
			t.Errorf("%q following class was swallowed: %+v exists=%t", eol, next, ok)
		}
	}
}

func TestKotlinSimpleEnumEntriesAndBoundaries(t *testing.T) {
	cases := []struct {
		name, source, owner string
		entries             map[string]int
		sibling             string
		siblingLine         int
		method              string
		methodLine          int
	}{
		{
			name:   "ordinary enum values",
			source: "package demo\nenum class Action {\n  POSITIVE,\n  NEGATIVE,\n  NEUTRAL,\n  CANCELED\n}\nclass After {}\n",
			owner:  "demo.Action", entries: map[string]int{"POSITIVE": 3, "NEGATIVE": 4, "NEUTRAL": 5, "CANCELED": 6},
			sibling: "demo.After", siblingLine: 8,
		},
		{
			name:   "trailing comma and method after semicolon",
			source: "package demo\nenum class Close {\n  Closes,\n  DoesNothing,\n  Unsupported,\n  ;\n  fun explain() {}\n}\nclass After {}\n",
			owner:  "demo.Close", entries: map[string]int{"Closes": 3, "DoesNothing": 4, "Unsupported": 5},
			method: "demo.Close.explain", methodLine: 7, sibling: "demo.After", siblingLine: 9,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
					sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol)), testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
					t.Errorf("simple enum unexpectedly partial %q: %+v", eol, result.Analysis.Diagnostics)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				if enum, ok := symbols[tc.owner]; !ok || enum.Kind != SymbolKindEnum || enum.NativeKind != "enum-class" {
					t.Errorf("missing Kotlin enum %q: %+v exists=%t", tc.owner, enum, ok)
				}
				for name, line := range tc.entries {
					sym, ok := symbols[tc.owner+"."+name]
					if !ok || sym.Kind != SymbolKindConstant || sym.NativeKind != "enum-value" ||
						sym.ParentQualifiedName != tc.owner || sym.DeclarationRange.Start.Line != line ||
						sym.DeclarationRange.End.Line != line {
						t.Errorf("%q enum entry %s: %+v exists=%t", eol, name, sym, ok)
					}
				}
				if tc.method != "" {
					sym, ok := symbols[tc.method]
					if !ok || sym.Kind != SymbolKindMethod || sym.ParentQualifiedName != tc.owner ||
						sym.DeclarationRange.Start.Line != tc.methodLine {
						t.Errorf("%q enum method: %+v exists=%t", eol, sym, ok)
					}
				}
				sym, ok := symbols[tc.sibling]
				if !ok || sym.Kind != SymbolKindClass || sym.DeclarationRange.Start.Line != tc.siblingLine ||
					sym.ParentQualifiedName != "demo" {
					t.Errorf("%q following class: %+v exists=%t", eol, sym, ok)
				}
				for _, fake := range []string{tc.owner + ".fun", tc.owner + ".class"} {
					if _, exists := symbols[fake]; exists {
						t.Errorf("false declaration %q", fake)
					}
				}
			}
		})
	}
}

func TestKotlinImplicitCompanionObjectOwners(t *testing.T) {
	source := "package demo\nclass Options {\n  companion object {\n    const val FLAG: Int = 7\n    fun of(): Options = Options()\n  }\n  fun instance() {}\n}\nclass Next {}\n"
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(strings.ReplaceAll(eol, "\r", "CR"), func(t *testing.T) {
			document := sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol))
			result, err := (KotlinAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Errorf("implicit companion unexpectedly partial: %+v", result.Analysis.Diagnostics)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			companion, ok := symbols["demo.Options.Companion"]
			if !ok || companion.Kind != SymbolKindClass || companion.NativeKind != "companion-object" ||
				companion.ParentQualifiedName != "demo.Options" ||
				companion.DeclarationRange.Start.Line != 3 || companion.DeclarationRange.End.Line != 6 ||
				companion.NameRange.Start.Line != 3 {
				t.Errorf("implicit companion owner incorrect: %+v exists=%t, all=%v",
					companion, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			if ok && document.Text[companion.nameOffsets.Start:companion.nameOffsets.End] != "object" {
				t.Errorf("synthetic companion name must be anchored at actual object keyword: %+v", companion)
			}
			for name, parent := range map[string]string{
				"demo.Options.Companion.FLAG": "demo.Options.Companion",
				"demo.Options.Companion.of":   "demo.Options.Companion",
				"demo.Options.instance":       "demo.Options",
				"demo.Next":                   "demo",
			} {
				symbol, exists := symbols[name]
				if !exists || symbol.ParentQualifiedName != parent {
					t.Errorf("%s has parent %q, expected %q, exists=%t", name, symbol.ParentQualifiedName, parent, exists)
				}
			}
			for _, fake := range []string{"demo.Options.FLAG", "demo.Options.of", "demo.Options.Companion.instance"} {
				if _, exists := symbols[fake]; exists {
					t.Errorf("incorrect flattened member %q", fake)
				}
			}
		})
	}
}

func TestKotlinCompanionNamesAndAnonymousObjectNegatives(t *testing.T) {
	source := "package demo\nclass Api {\n  public companion object Factory {\n    fun make() {}\n  }\n  val runner = object : Runnable { override fun run() {} }\n}\nobject Named { fun ping() {} }\nclass After {}\n"
	for _, eol := range []string{"\n", "\r\n"} {
		result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
			sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol)),
			testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		symbols := symbolsByQualifiedName(result.Analysis.Symbols)
		for name, parent := range map[string]string{
			"demo.Api.Factory":      "demo.Api",
			"demo.Api.Factory.make": "demo.Api.Factory",
			"demo.Named":            "demo",
			"demo.Named.ping":       "demo.Named",
			"demo.After":            "demo",
		} {
			symbol, exists := symbols[name]
			if !exists || symbol.ParentQualifiedName != parent {
				t.Errorf("%q: %s has parent %q expected %q exists=%t", eol, name, symbol.ParentQualifiedName, parent, exists)
			}
		}
		for _, fake := range []string{"demo.Api.Companion", "demo.Api.runner.Companion", "demo.Companion"} {
			if _, exists := symbols[fake]; exists {
				t.Errorf("invented companion symbol %q", fake)
			}
		}
	}
}

func TestKotlinAnnotatedTypeSourceRangesAndOwners(t *testing.T) {
	cases := []struct {
		name, source, className, classNative, member, sibling string
		start, end, memberLine, siblingLine                   int
	}{
		{
			name:      "stacked annotations before class",
			source:    "package demo\n@Suppress(\"unused\")\n@AutoService(value = [Registry::class])\nclass Registry {\n  fun run() {}\n}\nfun after() {}\n",
			className: "demo.Registry", classNative: "class", member: "demo.Registry.run", sibling: "demo.after",
			start: 2, end: 6, memberLine: 5, siblingLine: 7,
		},
		{
			name:      "value class with annotated constructor parameter",
			source:    "package demo\n@Parcelize\n@JvmInline\nvalue class DocumentUri(val value: @WriteWith<Parceler> Uri) {\n  fun id() {}\n}\nfun after() {}\n",
			className: "demo.DocumentUri", classNative: "value-class", member: "demo.DocumentUri.id", sibling: "demo.after",
			start: 2, end: 6, memberLine: 5, siblingLine: 7,
		},
		{
			name:      "file annotation does not attach to next class",
			source:    "@file:Suppress(\"unused\")\npackage demo\nclass Plain { fun go() {} }\nfun after() {}\n",
			className: "demo.Plain", classNative: "class", member: "demo.Plain.go", sibling: "demo.after",
			start: 3, end: 3, memberLine: 3, siblingLine: 4,
		},
		{
			name:      "annotated function does not attach to next class",
			source:    "package demo\n@Deprecated(\"old\")\nfun parse() {}\nclass Following { fun run() {} }\nfun after() {}\n",
			className: "demo.Following", classNative: "class", member: "demo.Following.run", sibling: "demo.after",
			start: 4, end: 4, memberLine: 4, siblingLine: 5,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
					sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol)),
					testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
					t.Errorf("annotated class coverage unexpectedly partial %q: %+v", eol, result.Analysis.Diagnostics)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				class, ok := symbols[tc.className]
				if !ok || class.Kind != SymbolKindClass || class.NativeKind != tc.classNative ||
					class.DeclarationRange.Start.Line != tc.start || class.DeclarationRange.End.Line != tc.end {
					t.Errorf("annotated class range %q: %+v exists=%t", eol, class, ok)
				}
				member, ok := symbols[tc.member]
				if !ok || member.Kind != SymbolKindMethod ||
					member.ParentQualifiedName != tc.className ||
					member.DeclarationRange.Start.Line != tc.memberLine {
					t.Errorf("annotated class member %q: %+v exists=%t", eol, member, ok)
				}
				sibling, ok := symbols[tc.sibling]
				if !ok || sibling.Kind != SymbolKindFunction ||
					sibling.DeclarationRange.Start.Line != tc.siblingLine {
					t.Errorf("next top-level sibling corrupted %q: %+v exists=%t", eol, sibling, ok)
				}
			}
		})
	}
}

func TestKotlinAnnotatedFunctionExcerptsAndSiblings(t *testing.T) {
	cases := []struct {
		name, source, target, sibling string
		start, end, siblingLine       int
		kind                          SymbolKind
	}{
		{
			name:   "member function with separate throws annotation",
			source: "package demo\nclass Bridge {\n  @Throws(IOException::class)\n  open fun read(): Long = 0L\n  fun next() {}\n}\n",
			target: "demo.Bridge.read", sibling: "demo.Bridge.next",
			start: 3, end: 4, siblingLine: 5, kind: SymbolKindMethod,
		},
		{
			name:   "extension methods after file level annotation",
			source: "@file:Suppress(\"unused\")\npackage demo\nobject Codec {\n  @JvmStatic\n  fun ByteArray.decode(): Int = 1\n  @JvmStatic\n  fun String.encode(): Int = 2\n}\n",
			target: "demo.Codec.decode", sibling: "demo.Codec.encode",
			start: 4, end: 5, siblingLine: 6, kind: SymbolKindMethod,
		},
		{
			name:   "top-level annotated function",
			source: "package demo\n@Deprecated(\"old\")\nfun parse(): Int = 1\nfun next(): Int = 2\n",
			target: "demo.parse", sibling: "demo.next",
			start: 2, end: 3, siblingLine: 4, kind: SymbolKindFunction,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
					sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol)),
					testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
					t.Errorf("annotated function unexpectedly partial %q: %+v", eol, result.Analysis.Diagnostics)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				target, ok := symbols[tc.target]
				if !ok || target.Kind != tc.kind ||
					target.DeclarationRange.Start.Line != tc.start ||
					target.DeclarationRange.End.Line != tc.end {
					t.Errorf("annotated function excerpt %q: %+v exists=%t", eol, target, ok)
				}
				sibling, ok := symbols[tc.sibling]
				if !ok || sibling.Kind != tc.kind ||
					sibling.DeclarationRange.Start.Line != tc.siblingLine {
					t.Errorf("following function ownership %q: %+v exists=%t", eol, sibling, ok)
				}
			}
		})
	}
}

func TestKotlinAccessorAnnotationsInPropertySourceRanges(t *testing.T) {
	cases := []struct {
		name, source, property, sibling string
		start, end, siblingLine         int
	}{
		{
			name:     "member getter use-site annotation",
			source:   "package demo\nclass Navigation {\n  @get:DrawableRes\n  protected abstract val iconRes: Int?\n  fun title() {}\n}\n",
			property: "demo.Navigation.iconRes", sibling: "demo.Navigation.title",
			start: 3, end: 4, siblingLine: 5,
		},
		{
			name:     "constructor property getter and setter annotations",
			source:   "package demo\nclass Forwarder(\n  @get:JvmName(\"delegate\")\n  @set:JvmSynthetic\n  var delegate: Timeout,\n) {\n  fun setDelegate() {}\n}\n",
			property: "demo.Forwarder.delegate", sibling: "demo.Forwarder.setDelegate",
			start: 3, end: 5, siblingLine: 7,
		},
		{
			name:     "annotation on method is not part of next property",
			source:   "package demo\nclass Navigation {\n  @Deprecated(\"old\")\n  fun old() {}\n  val title: Int = 0\n  fun after() {}\n}\n",
			property: "demo.Navigation.title", sibling: "demo.Navigation.after",
			start: 5, end: 5, siblingLine: 6,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
					sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol)),
					testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
					t.Errorf("annotated property coverage unexpectedly partial %q: %+v", eol, result.Analysis.Diagnostics)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				property, ok := symbols[tc.property]
				if !ok || property.Kind != SymbolKindProperty ||
					property.DeclarationRange.Start.Line != tc.start ||
					property.DeclarationRange.End.Line != tc.end {
					t.Errorf("annotated property excerpt %q: %+v exists=%t", eol, property, ok)
				}
				sibling, ok := symbols[tc.sibling]
				if !ok || sibling.Kind != SymbolKindMethod ||
					sibling.DeclarationRange.Start.Line != tc.siblingLine {
					t.Errorf("adjacent method corrupted %q: %+v exists=%t", eol, sibling, ok)
				}
			}
		})
	}
}

func TestKotlinPackageKeywordSegmentsAndMalformedNames(t *testing.T) {
	cases := []struct {
		name, source, packageName, className, memberName string
		complete                                         bool
	}{
		{
			name:        "keyword at end of dotted package",
			source:      "package demo.internal\nclass Service { fun execute() {} }\n",
			packageName: "demo.internal", className: "demo.internal.Service", memberName: "demo.internal.Service.execute",
			complete: true,
		},
		{
			name:        "single soft-keyword package",
			source:      "package internal\nclass Service { fun execute() {} }\n",
			packageName: "internal", className: "internal.Service", memberName: "internal.Service.execute",
			complete: true,
		},
		{
			name:        "soft keyword in middle of package",
			source:      "package demo.internal.api\nclass Service { fun execute() {} }\n",
			packageName: "demo.internal.api", className: "demo.internal.api.Service", memberName: "demo.internal.api.Service.execute",
			complete: true,
		},
		{
			name:        "ordinary package is unchanged",
			source:      "package demo.core\nclass Service { fun execute() {} }\n",
			packageName: "demo.core", className: "demo.core.Service", memberName: "demo.core.Service.execute",
			complete: true,
		},
		{
			name:   "trailing dot is not a complete package",
			source: "package demo.internal.\nclass Service {}\n",
		},
		{
			name:   "expression is not a package",
			source: "package demo.internal + extra\nclass Service {}\n",
		},
		{
			name:   "hard keyword is not a valid bare segment",
			source: "package demo.class\nclass Service {}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				document := sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol))
				result, err := (KotlinAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				if result.Analysis.CoverageComplete != tc.complete || result.Analysis.Truncated {
					t.Errorf("unexpected package coverage %q: complete=%t diagnostics=%+v",
						eol, result.Analysis.CoverageComplete, result.Analysis.Diagnostics)
				}
				if !tc.complete {
					for _, symbol := range result.Analysis.Symbols {
						if symbol.Kind == SymbolKindPackage {
							t.Errorf("malformed package unexpectedly indexed: %+v", symbol)
						}
					}
					return
				}
				pkg, ok := symbols[tc.packageName]
				if !ok || pkg.Kind != SymbolKindPackage || pkg.Name != tc.packageName ||
					pkg.DeclarationRange.Start.Line != 1 || pkg.DeclarationRange.End.Line != 1 {
					t.Errorf("missing full package %q: %+v exists=%t", eol, pkg, ok)
				}
				cl, ok := symbols[tc.className]
				if !ok || cl.Kind != SymbolKindClass || cl.ParentQualifiedName != tc.packageName {
					t.Errorf("incorrect class owner %q: %+v exists=%t", eol, cl, ok)
				}
				member, ok := symbols[tc.memberName]
				if !ok || member.Kind != SymbolKindMethod || member.ParentQualifiedName != tc.className {
					t.Errorf("incorrect method owner %q: %+v exists=%t", eol, member, ok)
				}
			}
		})
	}
}

func TestKotlinMultilinePrimaryConstructorClassOwners(t *testing.T) {
	cases := []struct {
		name, source, class, member, sibling string
		classEnd, memberLine, siblingLine    int
	}{
		{
			name:   "class header followed by qualified primary constructor",
			source: "package demo\nactual open class ByteString\ninternal actual constructor(\n  internal actual val data: ByteArray,\n) : Comparable<ByteString> {\n  var utf8: String? = null\n  fun decode() {}\n}\nfun following() {}\n",
			class:  "demo.ByteString", member: "demo.ByteString.utf8", sibling: "demo.following",
			classEnd: 8, memberLine: 6, siblingLine: 9,
		},
		{
			name:   "annotation before multiline primary constructor",
			source: "package demo\ninternal class JsonValueSource\n@JvmOverloads\nconstructor(\n  val source: String,\n) : Source {\n  fun read() {}\n}\nfun following() {}\n",
			class:  "demo.JsonValueSource", member: "demo.JsonValueSource.read", sibling: "demo.following",
			classEnd: 8, memberLine: 7, siblingLine: 9,
		},
		{
			name:   "bodyless class must not absorb next function",
			source: "package demo\nclass Empty\nfun following() {}\n",
			class:  "demo.Empty", member: "", sibling: "demo.following",
			classEnd: 2, siblingLine: 3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
					sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol)), testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
					t.Errorf("Kotlin class header unexpectedly partial for %q: %+v", eol, result.Analysis.Diagnostics)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				class, ok := symbols[tc.class]
				if !ok || class.Kind != SymbolKindClass || class.DeclarationRange.Start.Line != 2 ||
					class.DeclarationRange.End.Line != tc.classEnd {
					t.Errorf("class range/owner %q: %+v exists=%t", eol, class, ok)
				}
				if tc.member != "" {
					member, ok := symbols[tc.member]
					if !ok || member.DeclarationRange.Start.Line != tc.memberLine ||
						member.ParentQualifiedName != tc.class {
						t.Errorf("class member %q: %+v exists=%t", eol, member, ok)
					}
				}
				sibling, ok := symbols[tc.sibling]
				if !ok || sibling.Kind != SymbolKindFunction || sibling.DeclarationRange.Start.Line != tc.siblingLine {
					t.Errorf("following sibling %q: %+v exists=%t", eol, sibling, ok)
				}
			}
		})
	}
}

func TestKotlinPropertyNamesAndGetterScopes(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   map[string]SymbolKind
		absent []string
	}{
		{
			name:   "extension properties keep their declared names",
			source: "package demo\nclass ActionState\nval ActionState.isReady: Boolean\n    get() = true\nval ActionState.isRunning: Boolean\n    get() = false\n",
			want:   map[string]SymbolKind{"demo.isReady": SymbolKindProperty, "demo.isRunning": SymbolKindProperty},
			absent: nil,
		},
		{
			name:   "getter local variables do not leak into class",
			source: "package demo\nclass IndexedParameterMap {\n    val entries: Set<String>\n        get() {\n            val allPossibleEntries = listOf(\"x\")\n            return allPossibleEntries.toSet()\n        }\n    fun next() {}\n}\n",
			want:   map[string]SymbolKind{"demo.IndexedParameterMap.entries": SymbolKindProperty, "demo.IndexedParameterMap.next": SymbolKindMethod},
			absent: []string{"demo.IndexedParameterMap.allPossibleEntries"},
		},
		{
			name:   "delegate property keeps name and following members",
			source: "package okio\nexpect abstract class ForwardingSource constructor(\n    delegate: Source,\n) : Source {\n    val delegate: Source\n    override fun timeout(): Timeout\n}\n",
			want:   map[string]SymbolKind{"okio.ForwardingSource.delegate": SymbolKindProperty, "okio.ForwardingSource.timeout": SymbolKindMethod},
			absent: []string{"okio.ForwardingSource.Source"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				t.Run("eol-"+map[string]string{"\n": "LF", "\r\n": "CRLF"}[eol], func(t *testing.T) {
					source := strings.ReplaceAll(tc.source, "\n", eol)
					result, err := (KotlinAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
					if err != nil {
						t.Fatal(err)
					}
					if !result.Analysis.CoverageComplete {
						t.Fatalf("incomplete: %+v", result.Analysis.Diagnostics)
					}
					found := symbolsByQualifiedName(result.Analysis.Symbols)
					for name, kind := range tc.want {
						symbol, ok := found[name]
						if !ok || symbol.Kind != kind {
							t.Errorf("%s: expected %s, got %+v (exists=%v); names=%v", name, kind, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
						}
					}
					if tc.name == "extension properties keep their declared names" {
						classCount := 0
						for _, symbol := range result.Analysis.Symbols {
							if symbol.QualifiedName == "demo.ActionState" {
								if symbol.Kind != SymbolKindClass {
									t.Errorf("spurious extension receiver property: %+v", symbol)
								}
								classCount++
							}
						}
						if classCount != 1 {
							t.Errorf("ActionState declarations=%d, expected one class", classCount)
						}
					}
					for _, name := range tc.absent {
						if symbol, ok := found[name]; ok {
							t.Errorf("spurious declaration %s: %+v", name, symbol)
						}
					}
				})
			}
		})
	}
}

func TestKotlinOriginalOkioForwardingSource(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_KOTLIN_OKIO_FORWARDING_SOURCE")
	if path == "" {
		t.Skip("set SCRIPTHOLD_REAL_KOTLIN_OKIO_FORWARDING_SOURCE to the original Okio ForwardingSource.kt")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 1<<20 {
		t.Fatal("original Kotlin fixture exceeds 1 MiB")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document := sourceDocumentForScanner(string(contents))
	document.Path = path
	result, err := (KotlinAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("original Kotlin analysis partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"okio.ForwardingSource":          SymbolKindClass,
		"okio.ForwardingSource.delegate": SymbolKindProperty,
		"okio.ForwardingSource.read":     SymbolKindMethod,
		"okio.ForwardingSource.timeout":  SymbolKindMethod,
		"okio.ForwardingSource.close":    SymbolKindMethod,
	} {
		symbol, exists := byName[qualified]
		if !exists || symbol.Kind != kind {
			t.Fatalf("%s: got %+v, exists=%v; symbols=%v", qualified, symbol, exists, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if falseSymbol, exists := byName["okio.ForwardingSource.Source"]; exists {
		t.Fatalf("Kotlin property type mistaken for property name: %+v", falseSymbol)
	}
	if !hasStructuralRelation(result.Relations, "supertype", "okio.ForwardingSource", "Source") {
		t.Fatalf("Kotlin original supertype lost: %+v", result.Relations)
	}
}

func TestKotlinAnalyzerPackagesImportsPropertiesConstructorsAndRelations(t *testing.T) {
	text := `package demo.core
import kotlin.collections.List
import foo.bar.Baz as Qux
sealed class Box<T>(val value: T) : Base(), Runnable {
    constructor() : this(defaultValue())
    fun get(): T = value
    val size: Int get() = 1
    var count: Int = 0
    data class Nested(val id: Int)
    override fun run() {}
    val fake = "class Fake { fun nope() {} }"
}
interface Service : AutoCloseable { fun execute() }
enum class State { READY, DONE }
typealias Alias = Box<Int>
fun top(value: Int): Int = value
const val Answer: Int = 42
`
	document := sourceDocumentForScanner(text)
	document.Path = "Box.kt"
	result, err := (KotlinAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Kotlin partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"demo.core": SymbolKindPackage, "demo.core.Box": SymbolKindClass, "demo.core.Box.value": SymbolKindProperty,
		"demo.core.Box.get": SymbolKindMethod, "demo.core.Box.size": SymbolKindProperty, "demo.core.Box.count": SymbolKindProperty,
		"demo.core.Box.Nested": SymbolKindClass, "demo.core.Box.run": SymbolKindMethod, "demo.core.Service": SymbolKindInterface,
		"demo.core.Service.execute": SymbolKindMethod, "demo.core.State": SymbolKindEnum, "demo.core.Alias": SymbolKindAlias,
		"demo.core.top": SymbolKindFunction, "demo.core.Answer": SymbolKindConstant,
	} {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	constructors := 0
	constructorIDs := map[string]struct{}{}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.QualifiedName == "demo.core.Box.Box" && symbol.Kind == SymbolKindConstructor {
			constructors++
			constructorIDs[symbol.ID] = struct{}{}
		}
		if symbol.Name == "Fake" || symbol.Name == "nope" {
			t.Fatalf("Kotlin false positive: %+v", symbol)
		}
	}
	if constructors != 2 || len(constructorIDs) != 2 {
		t.Fatalf("Kotlin constructors=%d ids=%v", constructors, constructorIDs)
	}
	if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{"kotlin.collections.List", "foo.bar.Baz"}) {
		t.Fatalf("Kotlin imports = %v", got)
	}
	if result.Dependencies[1].Alias != "Qux" {
		t.Fatalf("Kotlin import alias = %+v", result.Dependencies[1])
	}
	if !hasStructuralRelation(result.Relations, "supertype", "demo.core.Box", "Base") || !hasStructuralRelation(result.Relations, "supertype", "demo.core.Box", "Runnable") || !hasStructuralRelation(result.Relations, "supertype", "demo.core.Service", "AutoCloseable") {
		t.Fatalf("Kotlin supertypes = %+v", result.Relations)
	}
}

func TestKotlinNamedObjectDeclarations(t *testing.T) {
	const source = `package demo
object Config {
    fun init() {}
    val tag: String = "config"
}
data object Singleton
class Wrapper {
    private object Nested {
        fun run() {}
    }
}
fun factory() = object : Runnable {
    override fun run() {}
}
val fake = "object Pretend { fun bogus() {} }"
`
	for _, tc := range []struct{ name, source string }{
		{"LF", source},
		{"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (KotlinAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("Kotlin object declaration incomplete: %+v", result.Analysis)
			}
			found := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range map[string]SymbolKind{
				"demo.Config":             SymbolKindClass,
				"demo.Config.init":        SymbolKindMethod,
				"demo.Config.tag":         SymbolKindProperty,
				"demo.Singleton":          SymbolKindClass,
				"demo.Wrapper":            SymbolKindClass,
				"demo.Wrapper.Nested":     SymbolKindClass,
				"demo.Wrapper.Nested.run": SymbolKindMethod,
			} {
				if symbol, ok := found[name]; !ok || symbol.Kind != kind {
					t.Errorf("%s = %+v exists=%v; symbols=%v", name, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				} else if name == "demo.Config" && symbol.NativeKind != "object" {
					t.Errorf("Kotlin named object native kind = %q", symbol.NativeKind)
				}
			}
			for _, name := range []string{"demo.Pretend", "demo.bogus", "demo.Runnable"} {
				if symbol, exists := found[name]; exists {
					t.Errorf("Kotlin anonymous/string false positive %s = %+v", name, symbol)
				}
			}
		})
	}
}

func TestKotlinOriginalNamedObjects(t *testing.T) {
	for _, tc := range []struct {
		env, qualified string
		line           int
	}{
		{"SCRIPTHOLD_REAL_KOTLIN_REBUGGER_CONFIG", "com.theapache64.rebugger.RebuggerConfig", 5},
		{"SCRIPTHOLD_REAL_KOTLIN_COMPRESSOR", "id.zelory.compressor.Compressor", 17},
	} {
		t.Run(tc.env, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skipf("set %s to the qualified original Kotlin source", tc.env)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() > 1<<20 {
				t.Fatal("original Kotlin fixture exceeds 1 MiB")
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc := sourceDocumentForScanner(string(contents))
			doc.Path = path
			result, err := (KotlinAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("original Kotlin object analysis incomplete: %+v", result.Analysis)
			}
			found := symbolsByQualifiedName(result.Analysis.Symbols)
			obj, ok := found[tc.qualified]
			if !ok || obj.Kind != SymbolKindClass || obj.NativeKind != "object" || obj.NameRange.Start.Line != tc.line {
				t.Fatalf("original Kotlin object %s got %+v (exists=%v); symbols=%v", tc.qualified, obj, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			method := "init"
			if tc.line == 17 {
				method = "compress"
			}
			member, ok := found[tc.qualified+"."+method]
			if !ok || member.Kind != SymbolKindMethod || member.ParentQualifiedName != tc.qualified {
				t.Fatalf("original Kotlin member %s.%s got %+v (exists=%v)", tc.qualified, method, member, ok)
			}
		})
	}
}

func TestKotlinAnalyzerGenericSupertypeCommaDoesNotSplitRelation(t *testing.T) {
	text := `package demo
class Empty
abstract class Generic<A, B>
class Child : Generic<Empty, Empty>(), Runnable
class NestedChild : Generic<Map<String, Pair<Int, Int>>, Empty>()
`
	result, err := (KotlinAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Kotlin generic supertype analysis partial: %+v", result.Analysis)
	}
	if !hasStructuralRelation(result.Relations, "supertype", "demo.Child", "Generic<Empty,Empty>") ||
		!hasStructuralRelation(result.Relations, "supertype", "demo.Child", "Runnable") ||
		!hasStructuralRelation(result.Relations, "supertype", "demo.NestedChild", "Generic<Map<String,Pair<Int,Int>>,Empty>") {
		t.Fatalf("Kotlin generic supertypes = %+v", result.Relations)
	}
}

func TestKotlinAnalyzerSupertypeConstructorArgumentsAreNotPartOfTarget(t *testing.T) {
	text := `package demo
open class Base(value: Int)
interface Other
class InlineChild : Base(value = 1), Other
class MultilineChild : Base(
    value = 2,
), Other
`
	result, err := (KotlinAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Kotlin constructor-argument supertype analysis partial: %+v", result.Analysis)
	}
	for _, source := range []string{"demo.InlineChild", "demo.MultilineChild"} {
		if !hasStructuralRelation(result.Relations, "supertype", source, "Base") ||
			!hasStructuralRelation(result.Relations, "supertype", source, "Other") {
			t.Fatalf("Kotlin constructor-argument supertypes for %s = %+v", source, result.Relations)
		}
	}
}

func TestKotlinAnalyzerFunInterfaceCoexistsWithSameNameFactory(t *testing.T) {
	text := `package demo
fun SizeResolver(value: Int): SizeResolver = error("unused")
fun interface SizeResolver {
    fun size(): Int
}
class ConstraintsSizeResolver : SizeResolver {
    override fun size(): Int = 1
}
`
	result, err := (KotlinAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	var functionFound, interfaceFound bool
	for _, symbol := range result.Analysis.Symbols {
		if symbol.QualifiedName != "demo.SizeResolver" {
			continue
		}
		switch symbol.Kind {
		case SymbolKindFunction:
			functionFound = true
		case SymbolKindInterface:
			interfaceFound = true
		}
	}
	if !functionFound || !interfaceFound {
		t.Fatalf("same-name factory/interface symbols missing: %+v", result.Analysis.Symbols)
	}
	if !hasStructuralRelation(result.Relations, "supertype", "demo.ConstraintsSizeResolver", "SizeResolver") {
		t.Fatalf("fun-interface supertype relation missing: %+v", result.Relations)
	}
}

func TestKotlinAnalyzerAnnotationClassLiteralsDoNotStealTypeDeclaration(t *testing.T) {
	text := `package demo
import sample.Base
@Database(entities = [Entity::class], version = 1)
@TypeConverters(value = [Converter::class])
abstract class Store : Base() {
    abstract fun dao(): Dao
}
`
	result, err := (KotlinAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Kotlin annotation class literals made analysis partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if symbol, ok := byName["demo.Store"]; !ok || symbol.Kind != SymbolKindClass {
		t.Fatalf("Store = %+v exists=%v; symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if symbol, ok := byName["demo.Store.dao"]; !ok || symbol.Kind != SymbolKindMethod {
		t.Fatalf("Store.dao = %+v exists=%v; symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if !hasStructuralRelation(result.Relations, "supertype", "demo.Store", "Base") {
		t.Fatalf("Store supertype relation missing: %+v", result.Relations)
	}
}

func TestJVMAnalyzerMalformedLimitsAndCancellation(t *testing.T) {
	malformed := sourceDocumentForScanner("class Good { int x; }\nclass Broken { String s = \"unterminated\n")
	malformed.Path = "Broken.java"
	result, err := (JavaAnalyzer{}).Analyze(context.Background(), malformed, testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
		t.Fatalf("malformed Java did not report partial coverage: %+v", result.Analysis)
	}
	if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Good"]; !ok {
		t.Fatalf("malformed recovery lost Good: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}

	limitedResult, err := (KotlinAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("class A {}\nclass B {}\nclass C {}\n"), testAnalyzeOptions(false, 2))
	if err != nil {
		t.Fatal(err)
	}
	if !limitedResult.Analysis.Truncated || limitedResult.Analysis.CoverageComplete || len(limitedResult.Analysis.Symbols) != 2 {
		t.Fatalf("Kotlin bounded result = %+v", limitedResult.Analysis)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (JavaAnalyzer{}).Analyze(ctx, sourceDocumentForScanner("class A {}"), testAnalyzeOptions(false, 16))
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("Java cancellation error=%v kind=%v", err, operation.KindOf(err))
	}
}

func TestJavaAnalyzerAnnotationsDoNotStealMethodDeclarators(t *testing.T) {
	text := `package demo;
@Deprecated
public class Service {
    @Route(path = "/work", retries = 2)
    public String work(int value) { return "ok"; }
    @Flag(enabled = true)
    private int count;
}
`
	result, err := (JavaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if symbol, ok := byName["demo.Service.work"]; !ok || symbol.Kind != SymbolKindMethod {
		t.Fatalf("annotated method = %+v exists=%v; symbols=%+v", symbol, ok, result.Analysis.Symbols)
	}
	if symbol, ok := byName["demo.Service.count"]; !ok || symbol.Kind != SymbolKindField {
		t.Fatalf("annotated field = %+v exists=%v; symbols=%+v", symbol, ok, result.Analysis.Symbols)
	}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Name == "Route" || symbol.Name == "Flag" || symbol.Name == "path" || symbol.Name == "retries" || symbol.Name == "enabled" {
			t.Fatalf("annotation leaked declaration: %+v", symbol)
		}
	}
}

func TestKotlinExplicitGetterPropertyRanges(t *testing.T) {
	const source = "package demo\n" +
		"class Container {\n" +
		"  val first: String\n" +
		"    get() = \"yes\"\n" +
		"  fun next(): String = \"ok\"\n" +
		"  val second: String\n" +
		"    get() {\n" +
		"      val local = \"no\"\n" +
		"      return local\n" +
		"    }\n" +
		"  fun last(): String = \"ok\"\n" +
		"}\n"
	for _, variant := range []struct{ name, eol string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(variant.name, func(t *testing.T) {
			result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(strings.ReplaceAll(source, "\n", variant.eol)), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("supported explicit getters reported partial: %+v", result.Analysis)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, tc := range []struct {
				name       string
				kind       SymbolKind
				start, end int
			}{
				{"demo.Container.first", SymbolKindProperty, 3, 4},
				{"demo.Container.next", SymbolKindMethod, 5, 5},
				{"demo.Container.second", SymbolKindProperty, 6, 10},
				{"demo.Container.last", SymbolKindMethod, 11, 11},
			} {
				sym, ok := symbols[tc.name]
				if !ok || sym.Kind != tc.kind || sym.DeclarationRange.Start.Line != tc.start || sym.DeclarationRange.End.Line != tc.end {
					t.Errorf("%s: got %+v exists=%v, want lines %d-%d", tc.name, sym, ok, tc.start, tc.end)
				}
			}
			if _, wrong := symbols["demo.Container.local"]; wrong {
				t.Error("getter-local binding leaked into class scope")
			}
		})
	}
}

func TestKotlinUnsupportedAccessorContinuationsRemainPartial(t *testing.T) {
	for _, tc := range []struct {
		name, accessor string
	}{
		{"chained-member-at-getter-indent", "    get() = \"hello\"\n    .uppercase()\n"},
		{"unfinished-wrapped-expression", "    get() =\n      \"hello\" +\n"},
		{"unsupported-expression-setter", "    get() = \"hello\"\n    set(value) = value\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "package demo\nclass Store {\n  var key: String\n" +
				tc.accessor + "  fun after(): String = \"ok\"\n}\n"
			for _, eol := range []string{"\n", "\r\n"} {
				result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
					sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol)),
					testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.CoverageComplete {
					t.Errorf("unsupported Kotlin accessor %q claimed complete with EOL %q", tc.name, eol)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				if sibling, ok := symbols["demo.Store.after"]; !ok || sibling.Kind != SymbolKindMethod {
					t.Errorf("following declaration lost for %q EOL %q", tc.name, eol)
				}
				if key, ok := symbols["demo.Store.key"]; !ok || key.DeclarationRange.End.Line > 5 {
					t.Errorf("unsupported accessor absorbed subsequent declaration: %+v", key)
				}
			}
		})
	}
}

func TestKotlinIndentedMultilineGetterRanges(t *testing.T) {
	for _, tc := range []struct {
		name, source, property, sibling string
		end, siblingLine                int
	}{
		{
			name:     "elvis chain and trailing lambda",
			source:   "package demo\nclass Logger {\n  val tag: String?\n    get() =\n      originalTag\n        ?: Throwable()\n          .stackTrace\n          .first { it.className != \"skip\" }\n          .let(::makeTag)\n  fun after() {}\n}\n",
			property: "demo.Logger.tag", sibling: "demo.Logger.after", end: 9, siblingLine: 10,
		},
		{
			name:     "if else with nested when",
			source:   "package demo\nclass Compat {\n  val directory: String?\n    get() =\n      if (level > 1) {\n        \"new\"\n      } else {\n        when (mode) {\n          0 -> \"old\"\n          else -> null\n        }\n      }\n  fun after() {}\n}\n",
			property: "demo.Compat.directory", sibling: "demo.Compat.after", end: 12, siblingLine: 13,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				t.Run(map[string]string{"\n": "LF", "\r\n": "CRLF"}[eol], func(t *testing.T) {
					result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
						sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol)),
						testAnalyzeOptions(true, 128))
					if err != nil {
						t.Fatal(err)
					}
					if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
						t.Fatalf("bounded Kotlin multiline getter partial: %+v", result.Analysis.Diagnostics)
					}
					symbols := symbolsByQualifiedName(result.Analysis.Symbols)
					property, ok := symbols[tc.property]
					if !ok || property.Kind != SymbolKindProperty ||
						property.DeclarationRange.Start.Line != 3 || property.DeclarationRange.End.Line != tc.end {
						t.Errorf("getter range invalid: %+v exists=%t want 3-%d", property, ok, tc.end)
					}
					sibling, ok := symbols[tc.sibling]
					if !ok || sibling.Kind != SymbolKindMethod ||
						sibling.DeclarationRange.Start.Line != tc.siblingLine {
						t.Errorf("following sibling corrupted: %+v exists=%t", sibling, ok)
					}
				})
			}
		})
	}
}

func TestKotlinPropertySetterOwnedSourceRanges(t *testing.T) {
	cases := []struct {
		name, source, property, sibling string
		propertyEnd, siblingLine        int
	}{
		{
			name:     "getter and setter",
			source:   "package demo\nclass Wrapper {\n  var direction: Int\n    get() = readDirection()\n    set(value) {\n      applyDirection(value)\n    }\n  fun after() {}\n}\n",
			property: "demo.Wrapper.direction", sibling: "demo.Wrapper.after", propertyEnd: 7, siblingLine: 8,
		},
		{
			name:     "initialized setter only",
			source:   "package demo\nclass Data {\n  var utf8: String? = null\n    set(value) {\n      cache(value)\n    }\n  fun after() {}\n}\n",
			property: "demo.Data.utf8", sibling: "demo.Data.after", propertyEnd: 6, siblingLine: 7,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
					sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol)),
					testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
					t.Errorf("%s coverage unexpectedly partial for %q", tc.name, eol)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				property, ok := symbols[tc.property]
				if !ok || property.Kind != SymbolKindProperty ||
					property.DeclarationRange.Start.Line != 3 ||
					property.DeclarationRange.End.Line != tc.propertyEnd {
					t.Errorf("wrong property range for %q: %+v exists=%t", eol, property, ok)
				}
				sibling, ok := symbols[tc.sibling]
				if !ok || sibling.Kind != SymbolKindMethod ||
					sibling.DeclarationRange.Start.Line != tc.siblingLine {
					t.Errorf("separate method missing for %q: %+v", eol, sibling)
				}
				if _, falseMethod := symbols["demo.Wrapper.set"]; falseMethod {
					t.Error("property accessor leaked as separate method")
				}
			}
		})
	}
}
