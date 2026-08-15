package sourceintelligence

import (
	"context"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

var _ SourceAnalyzer = PascalAnalyzer{}
var _ SourceAnalyzer = DelphiAnalyzer{}

func TestPascalAnalyzerProgramTypesRoutinesForwardOverloadNestedAndUses(t *testing.T) {
	text := `program Demo;
uses SysUtils, Classes;

type
  TState = (Ready, Done);
  TPoint = record
    X: Integer;
    Y: Integer;
  end;
  TShape = class
  private
    FValue: Integer;
  public
    constructor Create(AValue: Integer);
    destructor Destroy; virtual;
    procedure Draw; virtual;
    function Value: Integer;
    property Number: Integer read FValue write FValue;
  end;

const
  Answer = 42;
var
  GlobalValue: Integer;

procedure Top(AValue: Integer); forward;
procedure Top(AValue: Integer); overload;
  procedure Nested;
  begin
  end;
begin
end;

function Calc(Value: Integer): Integer;
begin
  Result := Value;
end;
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.pp"
	result, err := (PascalAnalyzer{}).Analyze(context.Background(), document, phase3AnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("Pascal analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"Demo":                SymbolKindModule,
		"Demo.TState":         SymbolKindEnum,
		"Demo.TPoint":         SymbolKindRecord,
		"Demo.TPoint.X":       SymbolKindField,
		"Demo.TPoint.Y":       SymbolKindField,
		"Demo.TShape":         SymbolKindClass,
		"Demo.TShape.FValue":  SymbolKindField,
		"Demo.TShape.TShape":  SymbolKindConstructor,
		"Demo.TShape.Destroy": SymbolKindDestructor,
		"Demo.TShape.Draw":    SymbolKindMethod,
		"Demo.TShape.Value":   SymbolKindMethod,
		"Demo.TShape.Number":  SymbolKindProperty,
		"Demo.Answer":         SymbolKindConstant,
		"Demo.GlobalValue":    SymbolKindVariable,
		"Demo.Top.Nested":     SymbolKindFunction,
		"Demo.Calc":           SymbolKindFunction,
	} {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	topIDs := map[string]struct{}{}
	topNative := map[string]int{}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.QualifiedName == "Demo.Top" {
			topIDs[symbol.ID] = struct{}{}
			topNative[symbol.NativeKind]++
		}
	}
	if len(topIDs) != 2 || topNative["procedure-forward"] != 1 || topNative["procedure-definition"] != 1 {
		t.Fatalf("Pascal forward/definition identity = ids=%v kinds=%v", topIDs, topNative)
	}
	if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{"SysUtils", "Classes"}) {
		t.Fatalf("Pascal uses = %v", got)
	}
}

func TestDelphiAnalyzerUnitSectionsGenericsHelpersAndImplementationMethods(t *testing.T) {
	text := `unit Demo.Unit1;

interface

uses System.SysUtils;

type
  IBase = interface
    procedure Work;
  end;
  TBase = class
  end;
  TBox<T> = class(TBase, IBase)
  private
    FValue: T;
  public
    constructor Create(const AValue: T);
    destructor Destroy; override;
    procedure Work; overload;
    property Value: T read FValue write FValue;
  end;
  TBoxHelper = class helper for TBox<Integer>
    procedure Help;
  end;

implementation

uses System.Classes;

constructor TBox<T>.Create(const AValue: T);
begin
end;

destructor TBox<T>.Destroy;
begin
end;

procedure TBox<T>.Work;
begin
end;

procedure TBoxHelper.Help;
begin
end;

end.
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.dpr"
	result, err := (DelphiAnalyzer{}).Analyze(context.Background(), document, phase3AnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("Delphi analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"Demo.Unit1":                 SymbolKindModule,
		"Demo.Unit1.IBase":           SymbolKindInterface,
		"Demo.Unit1.IBase.Work":      SymbolKindMethod,
		"Demo.Unit1.TBase":           SymbolKindClass,
		"Demo.Unit1.TBox":            SymbolKindClass,
		"Demo.Unit1.TBox.FValue":     SymbolKindField,
		"Demo.Unit1.TBox.TBox":       SymbolKindConstructor,
		"Demo.Unit1.TBox.Destroy":    SymbolKindDestructor,
		"Demo.Unit1.TBox.Work":       SymbolKindMethod,
		"Demo.Unit1.TBox.Value":      SymbolKindProperty,
		"Demo.Unit1.TBoxHelper":      SymbolKindType,
		"Demo.Unit1.TBoxHelper.Help": SymbolKindMethod,
	} {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if !hasStructuralRelation(result.Relations, "inherits", "Demo.Unit1.TBox", "TBase") ||
		!hasStructuralRelation(result.Relations, "implements", "Demo.Unit1.TBox", "IBase") ||
		!hasStructuralRelation(result.Relations, "helper-for", "Demo.Unit1.TBoxHelper", "TBox<Integer>") {
		t.Fatalf("Delphi type relations = %+v", result.Relations)
	}
	if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{"System.SysUtils", "System.Classes"}) {
		t.Fatalf("Delphi uses = %v", got)
	}
	sections := map[string]bool{}
	for _, symbol := range result.Analysis.Symbols {
		for _, modifier := range symbol.Modifiers {
			if modifier == "interface-section" || modifier == "implementation-section" {
				sections[modifier] = true
			}
		}
	}
	if !sections["interface-section"] || !sections["implementation-section"] {
		t.Fatalf("Delphi section evidence missing: %+v", result.Analysis.Symbols)
	}
}

func TestPascalDelphiMalformedLimitsAndCancellation(t *testing.T) {
	partial, err := (PascalAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("program Good;\ntype\n  TGood = class\n  end;\n  TBroken = class\n"), phase3AnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if partial.Analysis.CoverageComplete || len(partial.Analysis.Diagnostics) == 0 {
		t.Fatalf("malformed Pascal did not report partial coverage: %+v", partial.Analysis)
	}

	limited, err := (DelphiAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("unit U;\ninterface\ntype\n  A = class end;\n  B = class end;\n  C = class end;\nimplementation\nend.\n"), phase3AnalyzeOptions(false, 2))
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Analysis.Truncated || limited.Analysis.CoverageComplete || len(limited.Analysis.Symbols) != 2 {
		t.Fatalf("Delphi bounded result = %+v", limited.Analysis)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (DelphiAnalyzer{}).Analyze(ctx, sourceDocumentForScanner("unit U; interface implementation end."), phase3AnalyzeOptions(false, 16))
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("Delphi cancellation error=%v kind=%v", err, operation.KindOf(err))
	}
}
