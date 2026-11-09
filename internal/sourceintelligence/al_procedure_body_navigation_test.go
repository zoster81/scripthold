package sourceintelligence

import (
	"context"
	"testing"
)

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
