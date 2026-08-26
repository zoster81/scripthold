package sourceintelligence

import (
	"context"
	"reflect"
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
