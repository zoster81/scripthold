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
