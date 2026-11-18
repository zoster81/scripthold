package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Entire Git-pinned upstream originals are inspected, never executed.
func TestOCamlPinnedWholeOriginalMultilineScope(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_OCAML_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two pinned original OCaml sources are not configured")
	}
	for _, tc := range []struct {
		file               string
		count              int
		name               string
		endLine, endColumn int
	}{
		{"ocaml-compiler-scheduling.ml", 1, "fundecl", 30, 18},
		{"exercism-acronym.ml", 3, "acronym", 12, 25},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original OCaml fixture unavailable or unsafe: %v %v", info, err)
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (OCamlAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, string(src)), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Symbols) != tc.count {
				t.Fatalf("complete original symbols and coverage: %+v", result.Analysis)
			}
			found := false
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Name != tc.name {
					continue
				}
				found = true
				if symbol.DeclarationRange.End.Line != tc.endLine ||
					symbol.DeclarationRange.End.Column != tc.endColumn ||
					symbol.SignatureRange == nil ||
					symbol.SignatureRange.End != symbol.DeclarationRange.End ||
					!strings.HasSuffix(symbol.Signature, strings.TrimSpace(strings.Split(string(src), "\n")[tc.endLine-1])) {
					t.Errorf("source-authored OCaml function range/signature lost: %+v", symbol)
				}
			}
			if !found {
				t.Fatalf("missing OCaml root function %q: %+v", tc.name, result.Analysis.Symbols)
			}
		})
	}
}

func TestOCamlIndentedPipelineContinuationLFCRLF(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(strings.ReplaceAll(eol, "\r", "CR"), func(t *testing.T) {
			source := strings.Join([]string{
				"let pipeline xs =",
				"  xs",
				"  |> List.map (fun x -> x)",
				"  |> List.length",
				"let sibling n = n",
				"",
			}, eol)
			result, err := (OCamlAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("pipeline.ml", source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 2 {
				t.Fatalf("unexpected pipeline symbols/coverage: %+v", result.Analysis)
			}
			first, next := result.Analysis.Symbols[0], result.Analysis.Symbols[1]
			if first.Name != "pipeline" || first.DeclarationRange.End.Line != 4 ||
				first.DeclarationRange.End.Column != len("  |> List.length")+1 ||
				first.SignatureRange == nil || first.SignatureRange.End != first.DeclarationRange.End ||
				!strings.Contains(first.Signature, "List.map (fun x -> x)") ||
				next.Name != "sibling" || next.DeclarationRange.Start.Line != 5 {
				t.Errorf("OCaml multiline continuation consumed/omitted source: first=%+v sibling=%+v", first, next)
			}
		})
	}
}
