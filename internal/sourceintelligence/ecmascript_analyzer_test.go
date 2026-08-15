package sourceintelligence

import (
	"context"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

var _ SourceAnalyzer = JavaScriptAnalyzer{}
var _ SourceAnalyzer = TypeScriptAnalyzer{}

func TestJavaScriptAnalyzerModulesDeclarationsArrowsAndOpaqueLiterals(t *testing.T) {
	text := `import defaultThing, { named as alias } from "pkg";
export { helper } from "./helper.js";
const fs = require("fs");
const build = async (value) => value;
let ordinary = 1;
function declared(value) { return value; }
export async function exported() { return build(1); }
class Service extends Base {
    static version = 1;
    #secret = 2;
    constructor() {}
    async run(value) { return value; }
}
const pattern = /class RegexFake { method() {} }/g;
const template = ` + "`" + `function TemplateFake() {}` + "`" + `;
const view = () => (<div>class JSXFake {}</div>);
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.jsx"
	result, err := (JavaScriptAnalyzer{}).Analyze(context.Background(), document, phase3AnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("JavaScript analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"build": SymbolKindFunction, "ordinary": SymbolKindVariable, "declared": SymbolKindFunction, "exported": SymbolKindFunction,
		"Service": SymbolKindClass, "Service.version": SymbolKindField, "Service.#secret": SymbolKindField,
		"Service.Service": SymbolKindConstructor, "Service.run": SymbolKindMethod, "view": SymbolKindFunction,
	} {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	for _, symbol := range result.Analysis.Symbols {
		switch symbol.Name {
		case "RegexFake", "TemplateFake", "JSXFake", "method":
			t.Fatalf("JavaScript literal/JSX false positive: %+v", symbol)
		}
	}
	if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{"pkg", "./helper.js", "fs"}) {
		t.Fatalf("JavaScript module dependencies = %v", got)
	}
	if !hasStructuralRelation(result.Relations, "extends", "Service", "Base") {
		t.Fatalf("JavaScript class relation = %+v", result.Relations)
	}
}

func TestTypeScriptAnalyzerTypesNamespacesGenericsOverloadsAndTSX(t *testing.T) {
	text := `import type { Thing } from "./types";
export interface Repository<T> extends BaseRepo<T> {
    get(id: string): T;
    readonly size: number;
}
export type Result<T> = T | Error;
export enum State { Ready, Done }
export namespace Models {
    export interface Item { id: string; }
}
export abstract class Service<T> implements Repository<T> {
    abstract get(id: string): T;
}
export function identity<T>(value: T): T { return value; }
export function parse(value: string): string;
export function parse(value: number): number;
export function parse(value: unknown) { return String(value); }
export const mapper = <T,>(value: T): T => value;
const node = <Widget prop="class TSXFake {}" />;
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.tsx"
	result, err := (TypeScriptAnalyzer{}).Analyze(context.Background(), document, phase3AnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("TypeScript analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"Repository": SymbolKindInterface, "Repository.get": SymbolKindMethod, "Repository.size": SymbolKindProperty,
		"Result": SymbolKindAlias, "State": SymbolKindEnum, "Models": SymbolKindNamespace, "Models.Item": SymbolKindInterface,
		"Models.Item.id": SymbolKindProperty, "Service": SymbolKindClass, "Service.get": SymbolKindMethod,
		"identity": SymbolKindFunction, "mapper": SymbolKindFunction, "node": SymbolKindConstant,
	} {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	parseIDs := map[string]struct{}{}
	parseKinds := map[string]int{}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.QualifiedName == "parse" {
			parseIDs[symbol.ID] = struct{}{}
			parseKinds[symbol.NativeKind]++
		}
		if symbol.Name == "TSXFake" {
			t.Fatalf("TSX attribute text leaked a declaration: %+v", symbol)
		}
	}
	if len(parseIDs) != 3 || parseKinds["function-declaration"] != 2 || parseKinds["function-definition"] != 1 {
		t.Fatalf("TypeScript overload identity = ids=%v kinds=%v", parseIDs, parseKinds)
	}
	if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{"./types"}) {
		t.Fatalf("TypeScript imports = %v", got)
	}
	if !hasStructuralRelation(result.Relations, "extends", "Repository", "BaseRepo<T>") ||
		!hasStructuralRelation(result.Relations, "implements", "Service", "Repository<T>") {
		t.Fatalf("TypeScript type relations = %+v", result.Relations)
	}
}

func TestJavaScriptAnalyzerRegexLiteralsDoNotCorruptDelimiterState(t *testing.T) {
	text := `const closeBrace = /}/g;
const openBrace = /\{/;
const slashClass = /[\/}]/;
const ratio = total / count;
function real() { return ratio; }
`
	result, err := (JavaScriptAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("valid JavaScript regex literals corrupted lexical coverage: %+v", result.Analysis)
	}
	if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)["real"]; !ok {
		t.Fatalf("regex masking lost following declaration: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}
func TestECMAScriptAnalyzersMalformedLimitsCancellationAndASI(t *testing.T) {
	asi := sourceDocumentForScanner("const first = () => 1\nconst second = 2\nfunction third() {}\n")
	result, err := (JavaScriptAnalyzer{}).Analyze(context.Background(), asi, phase3AnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"first", "second", "third"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("ASI JavaScript lost %s: %v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}

	malformed := sourceDocumentForScanner("class Good {}\nconst broken = `unterminated\n")
	malformed.Path = "broken.js"
	partial, err := (JavaScriptAnalyzer{}).Analyze(context.Background(), malformed, phase3AnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if partial.Analysis.CoverageComplete || len(partial.Analysis.Diagnostics) == 0 {
		t.Fatalf("malformed JavaScript did not report partial coverage: %+v", partial.Analysis)
	}
	if _, ok := symbolsByQualifiedName(partial.Analysis.Symbols)["Good"]; !ok {
		t.Fatalf("malformed recovery lost Good: %v", sortedSymbolQualifiedNames(partial.Analysis.Symbols))
	}

	limited, err := (TypeScriptAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("interface A {}\ninterface B {}\ninterface C {}\n"), phase3AnalyzeOptions(false, 2))
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Analysis.Truncated || limited.Analysis.CoverageComplete || len(limited.Analysis.Symbols) != 2 {
		t.Fatalf("TypeScript bounded result = %+v", limited.Analysis)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (TypeScriptAnalyzer{}).Analyze(ctx, sourceDocumentForScanner("interface A {}"), phase3AnalyzeOptions(false, 16))
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("TypeScript cancellation error=%v kind=%v", err, operation.KindOf(err))
	}
}
