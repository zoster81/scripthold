package sourceintelligence

import (
	"fmt"
	"testing"
)

func TestSymbolBuilderSourceOffsetsPreserveOptionalPresenceAndIsolation(t *testing.T) {
	const text = "abc"
	options := SymbolBuilderOptions{
		Language: "go", Analyzer: "test", MaxEvidence: SymbolEvidenceStructural,
		Limits: SymbolBuilderLimits{MaxSymbols: 8, MaxSignatureBytes: 1024, MaxDiagnostics: 8},
	}

	without := NewSymbolBuilder(sourceDocumentForScanner(text), options)
	plain, err := without.Add(SymbolSpec{
		Kind: SymbolKindVariable, NativeKind: "variable", Name: "a",
		Declaration: OffsetRange{Start: 0, End: 3}, NameRange: OffsetRange{Start: 0, End: 1},
		Evidence: SymbolEvidenceStructural,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, signature, body := plain.SourceOffsets()
	if signature != nil || body != nil {
		t.Fatalf("absent optional offsets = signature=%+v body=%+v, want both nil", signature, body)
	}

	emptySignature := OffsetRange{Start: 1, End: 1}
	emptyBody := OffsetRange{Start: 2, End: 2}
	builder := NewSymbolBuilder(sourceDocumentForScanner(text), options)
	symbol, err := builder.Add(SymbolSpec{
		Kind: SymbolKindVariable, NativeKind: "variable", Name: "a",
		Declaration: OffsetRange{Start: 0, End: 3}, NameRange: OffsetRange{Start: 0, End: 1},
		Signature: &emptySignature, Body: &emptyBody, Evidence: SymbolEvidenceStructural,
	})
	if err != nil {
		t.Fatal(err)
	}

	assertOptionalOffsets := func(label string, source NormalizedSymbol) {
		t.Helper()
		_, _, signature, body := source.SourceOffsets()
		if signature == nil || body == nil {
			t.Fatalf("%s: empty optional ranges lost presence: signature=%+v body=%+v", label, signature, body)
		}
		if *signature != emptySignature || *body != emptyBody {
			t.Fatalf("%s: offsets=%+v body=%+v, want signature=%+v body=%+v", label, *signature, *body, emptySignature, emptyBody)
		}
		signature.Start = 0
		body.End = 3
		_, _, freshSignature, freshBody := source.SourceOffsets()
		if freshSignature == nil || freshBody == nil || *freshSignature != emptySignature || *freshBody != emptyBody {
			t.Fatalf("%s: SourceOffsets returned aliased state: signature=%+v body=%+v", label, freshSignature, freshBody)
		}
	}
	assertOptionalOffsets("Add", symbol)

	snapshot := builder.Result()
	if len(snapshot.Symbols) != 1 {
		t.Fatalf("snapshot symbols=%d, want 1", len(snapshot.Symbols))
	}
	assertOptionalOffsets("Result", snapshot.Symbols[0])
}

func TestSymbolBuilderOptionalOffsetCloneAllocationBudget(t *testing.T) {
	const symbolCount = 256
	var text string
	specs := make([]SymbolSpec, 0, symbolCount)
	for index := 0; index < symbolCount; index++ {
		name := fmt.Sprintf("item%03d", index)
		start := len(text)
		text += name + " value\n"
		nameEnd := start + len(name)
		signature := OffsetRange{Start: start, End: nameEnd}
		body := OffsetRange{Start: nameEnd, End: len(text) - 1}
		specs = append(specs, SymbolSpec{
			Kind: SymbolKindFunction, NativeKind: "function", Name: name,
			Declaration: OffsetRange{Start: start, End: len(text) - 1},
			NameRange:   OffsetRange{Start: start, End: nameEnd},
			Signature:   &signature, Body: &body, Evidence: SymbolEvidenceStructural,
		})
	}
	document := sourceDocumentForScanner(text)
	options := SymbolBuilderOptions{
		Language: "test", Analyzer: "optional-offset-clones", MaxEvidence: SymbolEvidenceStructural,
		Limits: SymbolBuilderLimits{MaxSymbols: symbolCount, MaxSignatureBytes: 1024, MaxDiagnostics: 8},
	}

	allocations := testing.AllocsPerRun(10, func() {
		builder := NewSymbolBuilder(document, options)
		builder.reserveSymbols(symbolCount)
		for _, spec := range specs {
			if _, err := builder.Add(spec); err != nil {
				panic(err)
			}
		}
		if len(builder.Result().Symbols) != symbolCount {
			panic("unexpected symbol count")
		}
	})
	if allocations > 2600 {
		t.Fatalf("optional offset clone allocations = %.0f, want <= 2600", allocations)
	}
}
