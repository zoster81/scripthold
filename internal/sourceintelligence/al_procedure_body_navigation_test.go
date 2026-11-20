package sourceintelligence

import (
	"context"
	"testing"
)

func TestALSemicolonTerminatedTriggerHeaderKeepsAuthoredBody(t *testing.T) {
	for _, tc := range []struct {
		name, source, qualifiedName string
		bodyStart, bodyEnd          int
	}{
		{
			name:          "trigger with local variable and header semicolon",
			source:        "codeunit 70051001 HelloWorld\n{\n    trigger OnRun();\n    var\n        MessageText: Text;\n    begin\n        Message(MessageText);\n    end;\n}\n",
			qualifiedName: "HelloWorld.OnRun", bodyStart: 6, bodyEnd: 8,
		},
		{
			name:          "trigger with direct begin after header semicolon",
			source:        "codeunit 70051001 HelloWorld\n{\n    trigger OnRun();\n    begin\n        Message('Hello');\n    end;\n}\n",
			qualifiedName: "HelloWorld.OnRun", bodyStart: 4, bodyEnd: 6,
		},
		{
			name:          "trigger with CRLF line endings",
			source:        "codeunit 70051001 HelloWorld\r\n{\r\n    trigger OnRun();\r\n    var\r\n        MessageText: Text;\r\n    begin\r\n        Message(MessageText);\r\n    end;\r\n}\r\n",
			qualifiedName: "HelloWorld.OnRun", bodyStart: 6, bodyEnd: 8,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(tc.source)
			doc.Path = "trigger.al"
			result, err := (ALAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid AL source has incomplete coverage: %+v", result.Analysis)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			s, exists := symbols[tc.qualifiedName]
			if !exists || s.DeclarationRange.End.Line != tc.bodyEnd || s.BodyRange == nil ||
				s.BodyRange.Start.Line != tc.bodyStart || s.BodyRange.End.Line != tc.bodyEnd {
				t.Errorf("wrong authored AL trigger body range %q: exists=%v symbol=%+v", tc.qualifiedName, exists, s)
			}
		})
	}
}

func TestALSemicolonOnlyDeclarationDoesNotBorrowSiblingBody(t *testing.T) {
	src := "codeunit 50000 Example\n{\n    procedure First();\n    procedure Second()\n    begin\n    end;\n}\n"
	doc := sourceDocumentForScanner(src)
	doc.Path = "siblings.al"
	result, err := (ALAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("unexpected incomplete AL analysis: %+v", result.Analysis)
	}
	symbols := symbolsByQualifiedName(result.Analysis.Symbols)
	first, firstOK := symbols["Example.First"]
	second, secondOK := symbols["Example.Second"]
	if !firstOK || first.BodyRange != nil || first.DeclarationRange.End.Line != 3 {
		t.Errorf("semicolon-only method must remain bodyless, found=%v symbol=%+v", firstOK, first)
	}
	if !secondOK || second.BodyRange == nil || second.BodyRange.Start.Line != 5 ||
		second.BodyRange.End.Line != 6 || second.DeclarationRange.End.Line != 6 {
		t.Errorf("next sibling must retain own body, found=%v symbol=%+v", secondOK, second)
	}
}

func TestALProcedureBodyRangeNestedBlocksAndSibling(t *testing.T) {
	src := `namespace Sample.BC;
codeunit 50100 "Search Engine"
{
    procedure Search(Query: Text): Text
    var
        ResultText: Text;
    begin
        if Query = '' then begin
            exit('[]');
        end;
        case Query of
            'ab': begin
                ResultText := 'found';
            end;
        end;
        exit(ResultText);
    end;

    local procedure Next(): Text
    begin
        exit('next');
    end;
}

interface 50101 "IWorker"
{
    procedure Execute();
}
`
	doc := sourceDocumentForScanner(src)
	doc.Path = "sample.al"
	result, err := (ALAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid AL source incomplete: %+v", result.Analysis)
	}
	symbols := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, tc := range []struct {
		name                string
		firstLine, lastLine int
		hasBody             bool
	}{
		{"Sample.BC.Search Engine.Search", 4, 17, true},
		{"Sample.BC.Search Engine.Next", 19, 22, true},
		{"Sample.BC.IWorker.Execute", 27, 27, false},
	} {
		symbol, exists := symbols[tc.name]
		if !exists || symbol.NameRange.Start.Line != tc.firstLine || symbol.DeclarationRange.End.Line != tc.lastLine ||
			(symbol.BodyRange != nil) != tc.hasBody {
			t.Errorf("wrong AL owned method source range %s exists=%v got=%+v", tc.name, exists, symbol)
		}
	}
}
