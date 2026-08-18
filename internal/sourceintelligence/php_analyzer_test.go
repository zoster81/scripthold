package sourceintelligence

import (
	"context"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

var _ SourceAnalyzer = PHPAnalyzer{}

func TestPHPAnalyzerNamespacesUsesTypesMembersFunctionsAndIncludes(t *testing.T) {
	text := `<?php
namespace App\Core;
use Vendor\Pkg\Thing as AliasThing;
use function Vendor\Pkg\helper;
include "bootstrap.php";
require_once $dynamicPath;

interface Contract extends ParentContract {
    public function run(string $value): void;
}
trait SharedTrait {
    public function helper(): void {}
}
enum State: string { case Ready = "ready"; }
class Service extends BaseService implements Contract {
    use SharedTrait;
    public const VERSION = 1;
    private string $name;
    public function __construct(string $name) { $this->name = $name; }
    public function run(string $value): void {}
}
function top(int $value): int { return $value; }
$fake = "class StringFake { function nope() {} }";
$doc = <<<TXT
class HeredocFake { function hidden() {} }
TXT;
// class CommentFake {}
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.php"
	result, err := (PHPAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("PHP analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		`App\Core`:                    SymbolKindNamespace,
		`App\Core.Contract`:           SymbolKindInterface,
		`App\Core.Contract.run`:       SymbolKindMethod,
		`App\Core.SharedTrait`:        SymbolKindTrait,
		`App\Core.SharedTrait.helper`: SymbolKindMethod,
		`App\Core.State`:              SymbolKindEnum,
		`App\Core.Service`:            SymbolKindClass,
		`App\Core.Service.VERSION`:    SymbolKindConstant,
		`App\Core.Service.$name`:      SymbolKindProperty,
		`App\Core.Service.Service`:    SymbolKindConstructor,
		`App\Core.Service.run`:        SymbolKindMethod,
		`App\Core.top`:                SymbolKindFunction,
	} {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	for _, symbol := range result.Analysis.Symbols {
		switch symbol.Name {
		case "StringFake", "HeredocFake", "CommentFake", "nope", "hidden":
			t.Fatalf("PHP opaque region leaked declaration: %+v", symbol)
		}
	}
	if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{`Vendor\Pkg\Thing`, `Vendor\Pkg\helper`, "bootstrap.php"}) {
		t.Fatalf("PHP dependencies = %v", got)
	}
	if !hasStructuralRelation(result.Relations, "extends", `App\Core.Contract`, "ParentContract") ||
		!hasStructuralRelation(result.Relations, "extends", `App\Core.Service`, "BaseService") ||
		!hasStructuralRelation(result.Relations, "implements", `App\Core.Service`, "Contract") ||
		!hasStructuralRelation(result.Relations, "uses-trait", `App\Core.Service`, "SharedTrait") {
		t.Fatalf("PHP relations = %+v", result.Relations)
	}
}

func TestPHPAnalyzerMalformedLimitsAndCancellation(t *testing.T) {
	malformed := sourceDocumentForScanner("<?php class Good {}\n$doc = <<<TXT\nunterminated\n")
	partial, err := (PHPAnalyzer{}).Analyze(context.Background(), malformed, testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if partial.Analysis.CoverageComplete || len(partial.Analysis.Diagnostics) == 0 {
		t.Fatalf("malformed PHP did not report partial coverage: %+v", partial.Analysis)
	}
	if _, ok := symbolsByQualifiedName(partial.Analysis.Symbols)["Good"]; !ok {
		t.Fatalf("malformed PHP lost Good: %v", sortedSymbolQualifiedNames(partial.Analysis.Symbols))
	}

	limited, err := (PHPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("<?php class A {} class B {} class C {}"), testAnalyzeOptions(false, 2))
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Analysis.Truncated || limited.Analysis.CoverageComplete || len(limited.Analysis.Symbols) != 2 {
		t.Fatalf("PHP bounded result = %+v", limited.Analysis)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (PHPAnalyzer{}).Analyze(ctx, sourceDocumentForScanner("<?php class A {}"), testAnalyzeOptions(false, 16))
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("PHP cancellation error=%v kind=%v", err, operation.KindOf(err))
	}
}
