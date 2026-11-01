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

func TestRubyAnalyzerOperatorMethodPreservesClassOwnership(t *testing.T) {
	text := "module Demo\n  class Ring\n    def initialize(size)\n      @size = size\n    end\n    def <<(element)\n      @buf << element\n    end\n    def buffer\n      @buf\n    end\n    def reset(default = 0)\n      @buf.fill(default)\n    end\n  end\nend\n"
	result, err := (RubyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		kind  SymbolKind
		line  int
		owner string
	}{
		"Demo":             {SymbolKindModule, 1, ""},
		"Demo.Ring":        {SymbolKindClass, 2, "Demo"},
		"Demo.Ring.Ring":   {SymbolKindConstructor, 3, "Demo.Ring"},
		"Demo.Ring.<<":     {SymbolKindMethod, 6, "Demo.Ring"},
		"Demo.Ring.buffer": {SymbolKindMethod, 9, "Demo.Ring"},
		"Demo.Ring.reset":  {SymbolKindMethod, 12, "Demo.Ring"},
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != len(want) {
		t.Fatalf("Ruby operator method broke outline/coverage: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for qualified, expected := range want {
		got, exists := byName[qualified]
		if !exists || got.Kind != expected.kind || got.DeclarationRange.Start.Line != expected.line ||
			got.ParentQualifiedName != expected.owner {
			t.Errorf("Ruby declaration %s: got=%+v exists=%v, expected=%+v", qualified, got, exists, expected)
		}
	}
}

func TestRubyAnalyzerBracketMethodsPreserveNestedOwners(t *testing.T) {
	source := "module Demo\n  module Queues\n    class << self\n      def [](name)\n        @jobs[name]\n      end\n      def []=(name, value)\n        @jobs[name] = value\n      end\n      def push(job)\n        @jobs << job\n      end\n    end\n  end\nend\n"
	result, err := (RubyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"Demo": 1, "Demo.Queues": 2, "Demo.Queues.[]": 4, "Demo.Queues.[]=": 7, "Demo.Queues.push": 10}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != len(want) {
		t.Fatalf("Ruby bracket-method declarations or scope missing: %+v", result.Analysis)
	}
	found := symbolsByQualifiedName(result.Analysis.Symbols)
	for name, line := range want {
		got, ok := found[name]
		if !ok || got.DeclarationRange.Start.Line != line {
			t.Errorf("Ruby missing/wrong bracket-method symbol %s: %+v", name, got)
		}
	}
	for _, name := range []string{"Demo.Queues.[]", "Demo.Queues.[]=", "Demo.Queues.push"} {
		if found[name].ParentQualifiedName != "Demo.Queues" || found[name].Kind != SymbolKindMethod {
			t.Errorf("Ruby method owner/kind %s: %+v", name, found[name])
		}
	}
}

func TestRubyAnalyzerInlineMethodVisibilityPreservesEnclosingScope(t *testing.T) {
	source := "module Demo\n  class Box\n    private def guarded(value)\n      value\n    end\n    def ordinary\n      :ok\n    end\n    protected def covered\n      :ok\n    end\n    public def exposed\n      :ok\n    end\n  end\n  module Later\n    def next_method\n      :ok\n    end\n  end\nend\n"
	result, err := (RubyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		kind       SymbolKind
		owner      string
		visibility Visibility
		line       int
	}{
		"Demo":                   {SymbolKindModule, "", VisibilityPublic, 1},
		"Demo.Box":               {SymbolKindClass, "Demo", VisibilityPublic, 2},
		"Demo.Box.guarded":       {SymbolKindMethod, "Demo.Box", VisibilityPrivate, 3},
		"Demo.Box.ordinary":      {SymbolKindMethod, "Demo.Box", VisibilityPublic, 6},
		"Demo.Box.covered":       {SymbolKindMethod, "Demo.Box", VisibilityProtected, 9},
		"Demo.Box.exposed":       {SymbolKindMethod, "Demo.Box", VisibilityPublic, 12},
		"Demo.Later":             {SymbolKindModule, "Demo", VisibilityPublic, 16},
		"Demo.Later.next_method": {SymbolKindMethod, "Demo.Later", VisibilityPublic, 17},
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != len(want) {
		t.Fatalf("inline visibility corrupted Ruby method scopes: %+v", result.Analysis)
	}
	found := symbolsByQualifiedName(result.Analysis.Symbols)
	for name, expected := range want {
		s, ok := found[name]
		if !ok || s.Kind != expected.kind || s.ParentQualifiedName != expected.owner ||
			s.Visibility != expected.visibility || s.DeclarationRange.Start.Line != expected.line {
			t.Errorf("wrong Ruby method visibility/owner for %s: got=%+v expected=%+v exists=%v", name, s, expected, ok)
		}
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
