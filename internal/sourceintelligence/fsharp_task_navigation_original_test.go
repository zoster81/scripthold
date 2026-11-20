package sourceintelligence

import (
	"context"
	"os"
	"testing"
)

// These optional original-project checks use independently selected F# maintenance scopes.
// They do not execute the external projects or imply complete language coverage.
func TestFSharpElmishSaturnOriginalNavigation(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want []struct {
			qualified string
			kind      SymbolKind
			line      int
		}
		forbidden []string
	}{
		{
			name: "elmish-dispatch-loop",
			env:  "SCRIPTHOLD_REAL_FS_ELMISH_PROGRAM",
			want: []struct {
				qualified string
				kind      SymbolKind
				line      int
			}{
				{"Elmish.Program.runWithDispatch", SymbolKindFunction, 160},
				{"Elmish.Program.runWith", SymbolKindFunction, 208},
			},
			forbidden: []string{"Elmish.Program.Subs.runWithDispatch", "Subs.runWithDispatch"},
		},
		{
			name: "elmish-ring-buffer",
			env:  "SCRIPTHOLD_REAL_FS_ELMISH_RING",
			want: []struct {
				qualified string
				kind      SymbolKind
				line      int
			}{
				{"Elmish.RingState", SymbolKindType, 5},
				{"Elmish.RingBuffer", SymbolKindType, 9},
				{"Elmish.RingBuffer.Pop", SymbolKindMethod, 20},
				{"Elmish.RingBuffer.Push", SymbolKindMethod, 33},
			},
			forbidden: []string{"Elmish.internal", "Elmish.internal.Push"},
		},
		{
			name: "saturn-controller",
			env:  "SCRIPTHOLD_REAL_FS_SATURN_CONTROLLER",
			want: []struct {
				qualified string
				kind      SymbolKind
				line      int
			}{
				{"Saturn.Controller.ControllerBuilder", SymbolKindType, 106},
				{"Saturn.Controller.ControllerBuilder.NotFoundHandler", SymbolKindMethod, 158},
				{"Saturn.Controller.ControllerBuilder.Run", SymbolKindMethod, 246},
			},
			forbidden: []string{"Saturn.Controller.internal", "Saturn.Controller.ControllerBuilder.internal"},
		},
		{
			name: "saturn-router",
			env:  "SCRIPTHOLD_REAL_FS_SATURN_ROUTER",
			want: []struct {
				qualified string
				kind      SymbolKind
				line      int
			}{
				{"Saturn.Router.RouterBuilder", SymbolKindType, 79},
				{"Saturn.Router.RouterBuilder.Run", SymbolKindMethod, 105},
				{"Saturn.Router.RouterBuilder.NotFoundHandler", SymbolKindMethod, 316},
			},
			forbidden: []string{"Saturn.Router.internal", "Saturn.Router.RouterBuilder.internal"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("original project fixture not configured")
			}
			doc, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
				MaxFileBytes:         1 << 20,
				MaxDecodedCharacters: 1 << 20,
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := (FSharpAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 20000))
			if err != nil {
				t.Fatal(err)
			}
			found := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range tc.want {
				got, ok := found[want.qualified]
				if !ok || got.Kind != want.kind || got.NameRange.Start.Line != want.line {
					t.Errorf("missing or misplaced %s kind=%s line=%d (found=%v, symbol=%+v)", want.qualified, want.kind, want.line, ok, got)
				}
			}
			for _, forbidden := range tc.forbidden {
				if _, ok := found[forbidden]; ok {
					t.Errorf("phantom or wrong owner: %s", forbidden)
				}
			}
		})
	}
}
