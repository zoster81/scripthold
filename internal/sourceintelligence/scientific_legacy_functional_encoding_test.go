package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestScientificLegacyFunctionalConformanceAcrossEncodingsAndDeterminism(t *testing.T) {
	tests := []struct {
		name, extension, encoding, text string
		bom                             bool
		analyzer                        SourceAnalyzer
		want                            []string
	}{
		{name: "fortran-ibm850-fixed-crlf", extension: ".f", encoding: "ibm850", analyzer: FortranAnalyzer{}, text: "C     café\r\n      MODULE DEMO\r\n      SUBROUTINE RUN()\r\n      END SUBROUTINE RUN\r\n      END MODULE DEMO\r\n", want: []string{"DEMO", "DEMO.RUN"}},
		{name: "cobol-windows1252-crlf", extension: ".cob", encoding: "windows-1252", analyzer: COBOLAnalyzer{}, text: "      * café\r\n       IDENTIFICATION DIVISION.\r\n       PROGRAM-ID. DEMO.\r\n       PROCEDURE DIVISION.\r\n       MAIN SECTION.\r\n", want: []string{"DEMO", "DEMO.MAIN"}},
		{name: "ada-windows1252-crlf", extension: ".ads", encoding: "windows-1252", analyzer: AdaAnalyzer{}, text: "-- café\r\npackage Demo is\r\n  procedure Run;\r\nend Demo;\r\n", want: []string{"Demo", "Demo.Run"}},
		{name: "matlab-utf16le", extension: ".m", encoding: "utf-16le", bom: true, analyzer: MATLABAnalyzer{}, text: "% résumé\nclassdef Worker\n  methods\n    function run(obj)\n    end\n  end\nend\n", want: []string{"Worker", "Worker.run"}},
		{name: "octave-utf16be", extension: ".m", encoding: "utf-16be", bom: true, analyzer: OctaveAnalyzer{}, text: "# résumé\nfunction run()\nendfunction\n", want: []string{"run"}},
		{name: "julia-utf32le", extension: ".jl", encoding: "utf-32le", bom: true, analyzer: JuliaAnalyzer{}, text: "# café\nmodule Demo\nfunction run()\nend\nend\n", want: []string{"Demo", "Demo.run"}},
		{name: "r-windows1252", extension: ".r", encoding: "windows-1252", analyzer: RAnalyzer{}, text: "# café\nrun <- function(x) { x }\n", want: []string{"run"}},
		{name: "haskell-utf16le", extension: ".hs", encoding: "utf-16le", bom: true, analyzer: HaskellAnalyzer{}, text: "-- résumé\nmodule Demo where\nrun :: Int -> Int\nrun x = x\n", want: []string{"Demo", "Demo.run"}},
		{name: "ocaml-windows1252", extension: ".ml", encoding: "windows-1252", analyzer: OCamlAnalyzer{}, text: "(* café *)\nmodule Demo = struct\n  let run x = x\nend\n", want: []string{"Demo", "Demo.run"}},
		{name: "common-lisp-windows1252", extension: ".lisp", encoding: "windows-1252", analyzer: CommonLispAnalyzer{}, text: "; café\n(defpackage :demo)\n(in-package :demo)\n(defun run (x) x)\n", want: []string{"demo", "demo.run"}},
		{name: "clojure-utf16le", extension: ".clj", encoding: "utf-16le", bom: true, analyzer: ClojureAnalyzer{}, text: "; résumé\n(ns demo.core)\n(defn run [x] x)\n", want: []string{"demo.core", "demo.core.run"}},
		{name: "emacs-lisp-windows1252", extension: ".el", encoding: "windows-1252", analyzer: EmacsLispAnalyzer{}, text: "; café\n(defun run (x) x)\n", want: []string{"run"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture"+tc.extension)
			if err := os.WriteFile(path, encodeSourceFixture(t, tc.encoding, tc.text, tc.bom), 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
				RequestedEncoding: tc.encoding, MaxFileBytes: 4 * 1024 * 1024, MaxDecodedCharacters: 1_000_000,
			})
			if err != nil {
				t.Fatal(err)
			}
			first, err := tc.analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			second, err := tc.analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("%s output is nondeterministic", tc.analyzer.Language())
			}
			if !first.Analysis.CoverageComplete || first.Analysis.Truncated {
				t.Fatalf("%s conformance partial: %+v", tc.analyzer.Language(), first.Analysis)
			}
			names := sortedSymbolQualifiedNames(first.Analysis.Symbols)
			for _, want := range tc.want {
				if !containsSortedString(names, want) {
					t.Fatalf("%s missing %s; symbols=%v", tc.analyzer.Language(), want, names)
				}
			}
		})
	}
}
