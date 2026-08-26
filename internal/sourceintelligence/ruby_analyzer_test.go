package sourceintelligence

import (
	"context"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

var _ SourceAnalyzer = RubyAnalyzer{}

func TestRubyAnalyzerModulesClassesMethodsConstantsMixinsAndReopen(t *testing.T) {
	text := `require "json"
require_relative "./support"

module Demo
  module Mix
    def mixed
    end
  end

  class Service < Base
    include Mix
    extend ClassMix
    CONST = 1

    def initialize(value)
      @value = value
    end

    def run(value)
      value
    end

    def self.build(value)
      new(value)
    end

    class << self
      def singleton
      end
    end
  end

  class Service
    def reopened
    end
  end
end

define_method(:dynamic_name) { }
fake = "class StringFake; def nope; end; end"
# class CommentFake; end
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.rb"
	result, err := (RubyAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("Ruby analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, kind := range map[string]SymbolKind{
		"Demo":                   SymbolKindModule,
		"Demo.Mix":               SymbolKindModule,
		"Demo.Mix.mixed":         SymbolKindMethod,
		"Demo.Service.CONST":     SymbolKindConstant,
		"Demo.Service.Service":   SymbolKindConstructor,
		"Demo.Service.run":       SymbolKindMethod,
		"Demo.Service.build":     SymbolKindMethod,
		"Demo.Service.singleton": SymbolKindMethod,
		"Demo.Service.reopened":  SymbolKindMethod,
	} {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	classIDs := map[string]struct{}{}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.QualifiedName == "Demo.Service" && symbol.Kind == SymbolKindClass {
			classIDs[symbol.ID] = struct{}{}
		}
		switch symbol.Name {
		case "dynamic_name", "StringFake", "CommentFake", "nope":
			t.Fatalf("Ruby dynamic/string/comment false positive: %+v", symbol)
		}
	}
	if len(classIDs) != 2 {
		t.Fatalf("Ruby reopen must preserve two declaration identities: %v", classIDs)
	}
	if got := dependencyValues(result.Dependencies); !reflect.DeepEqual(got, []string{"json", "./support"}) {
		t.Fatalf("Ruby dependencies = %v", got)
	}
	if !hasStructuralRelation(result.Relations, "inherits", "Demo.Service", "Base") ||
		!hasStructuralRelation(result.Relations, "includes", "Demo.Service", "Mix") ||
		!hasStructuralRelation(result.Relations, "extends-mixin", "Demo.Service", "ClassMix") {
		t.Fatalf("Ruby relations = %+v", result.Relations)
	}
}

func TestRubyAnalyzerAssignmentConditionalExpressionsPreserveScopeBalance(t *testing.T) {
	text := `class Service
  def value(flag)
    result = if flag
               :yes
             else
               :no
             end
    result
  end
end
`
	result, err := (RubyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Ruby assignment conditional expression made analysis partial: %+v", result.Analysis)
	}
	if symbol, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Service.value"]; !ok || symbol.Kind != SymbolKindMethod {
		t.Fatalf("Service.value = %+v exists=%v; symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRubyAnalyzerCompoundAssignmentConditionalExpressionsPreserveScopeBalance(t *testing.T) {
	text := `class Service
  def memo(flag)
    @memo ||= if flag
                :yes
              else
                :no
              end
  end
end
`
	result, err := (RubyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Ruby compound-assignment conditional expression made analysis partial: %+v", result.Analysis)
	}
	if symbol, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Service.memo"]; !ok || symbol.Kind != SymbolKindMethod {
		t.Fatalf("Service.memo = %+v exists=%v; symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRubyAnalyzerHeredocsRemainOpaqueAndPreserveScopeBalance(t *testing.T) {
	text := `class Service
  def fail!(condition)
    raise <<-MESSAGE unless condition
Heredoc body may contain unmatched delimiters: ) ] }
class HeredocFake
    MESSAGE
  end

  def after
  end
end
`
	result, err := (RubyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Ruby heredoc made analysis partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if symbol, ok := byName["Service.fail!"]; !ok || symbol.Kind != SymbolKindMethod {
		t.Fatalf("Service.fail! = %+v exists=%v; symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if symbol, ok := byName["Service.after"]; !ok || symbol.Kind != SymbolKindMethod {
		t.Fatalf("Service.after = %+v exists=%v; symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if _, exists := byName["Service.HeredocFake"]; exists {
		t.Fatalf("heredoc body leaked declaration: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRubyAnalyzerShiftOperatorsAreNotHeredocs(t *testing.T) {
	text := `class Service
  def decorate(value)
    values = []
    values << value
    values <<= 1
    super << :_prefixes
  end
end
`
	result, err := (RubyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Ruby shift operators were misclassified as heredocs: %+v", result.Analysis)
	}
	if symbol, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Service.decorate"]; !ok || symbol.Kind != SymbolKindMethod {
		t.Fatalf("Service.decorate = %+v exists=%v; symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRubyAnalyzerMalformedLimitsAndCancellation(t *testing.T) {
	partial, err := (RubyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("class Good\nend\nclass Broken\n"), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if partial.Analysis.CoverageComplete || len(partial.Analysis.Diagnostics) == 0 {
		t.Fatalf("malformed Ruby did not report partial coverage: %+v", partial.Analysis)
	}
	if _, ok := symbolsByQualifiedName(partial.Analysis.Symbols)["Good"]; !ok {
		t.Fatalf("malformed Ruby lost Good: %v", sortedSymbolQualifiedNames(partial.Analysis.Symbols))
	}

	limited, err := (RubyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("class A\nend\nclass B\nend\nclass C\nend\n"), testAnalyzeOptions(false, 2))
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Analysis.Truncated || limited.Analysis.CoverageComplete || len(limited.Analysis.Symbols) != 2 {
		t.Fatalf("Ruby bounded result = %+v", limited.Analysis)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (RubyAnalyzer{}).Analyze(ctx, sourceDocumentForScanner("class A\nend\n"), testAnalyzeOptions(false, 16))
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("Ruby cancellation error=%v kind=%v", err, operation.KindOf(err))
	}
}
