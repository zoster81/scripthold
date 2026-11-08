package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestZigReturnedAnonymousStructMethodsAreNavigable(t *testing.T) {
	source := `pub fn Factory(comptime T: type) type {
    return struct {
        const Self = @This();
        value: T,
        pub fn init(self: *Self) void { _ = self; }
        pub fn push(self: *Self, v: T) void { self.value = v; }
    };
}
pub fn ordinary() void {
    const Local = struct { pub fn hidden() void {} };
    _ = Local;
}
pub const Named = struct { pub fn existing() void {} };
`
	got, err := (ZigAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Analysis.CoverageComplete || got.Analysis.Truncated {
		t.Fatalf("unexpected partial analysis: %+v", got.Analysis)
	}
	symbols := symbolsByQualifiedName(got.Analysis.Symbols)
	for name, kind := range map[string]SymbolKind{
		"Factory": SymbolKindFunction, "Factory.init": SymbolKindMethod,
		"Factory.push": SymbolKindMethod, "Named.existing": SymbolKindMethod,
	} {
		symbol, ok := symbols[name]
		if !ok || symbol.Kind != kind {
			t.Errorf("missing %s kind %s; got %v; all=%v", name, kind, symbol.Kind, sortedSymbolQualifiedNames(got.Analysis.Symbols))
		}
	}
	for _, forbidden := range []string{"ordinary.hidden", "Local.hidden", "hidden", "ordinary.Local"} {
		if _, found := symbols[forbidden]; found {
			t.Errorf("unreturned local type leaked %s", forbidden)
		}
	}
}

func TestZigOriginalReturnedQueueMethods(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Zig corpus unavailable")
	}
	path := filepath.Join(corpus, "zig-mitchellh-libxev", "src", "queue_mpsc.zig")
	document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
		RequestedEncoding: "utf-8", MaxFileBytes: 2 << 20, MaxDecodedCharacters: 1_000_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := (ZigAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Analysis.CoverageComplete || got.Analysis.Truncated {
		t.Fatalf("original Zig analysis partial: %+v", got.Analysis)
	}
	symbols := symbolsByQualifiedName(got.Analysis.Symbols)
	for name, line := range map[string]int{"Intrusive.init": 32, "Intrusive.push": 40, "Intrusive.pop": 48} {
		symbol, ok := symbols[name]
		if !ok || symbol.Kind != SymbolKindMethod || symbol.NameRange.Start.Line != line {
			t.Errorf("missing original %s at %d; found=%+v exists=%v all=%v", name, line, symbol, ok, sortedSymbolQualifiedNames(got.Analysis.Symbols))
		}
	}
}
