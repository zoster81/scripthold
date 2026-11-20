package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two independent full pinned Microsoft AL originals, inventoried from authorial
// source before the previous development output: no referenced locals or calls
// may become structural symbols, and semicolon trigger headers retain bodies.
func TestALPinnedWholeOriginalOwnerAndBodyRanges(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_AL_OUTLINE_ROOT")
	if root == "" {
		t.Skip("independently pinned AL whole originals not configured")
	}
	cases := []struct {
		file    string
		length  int
		entries []struct {
			qualified, native                        string
			nameLine, start, end, bodyStart, bodyEnd int
		}
	}{
		{
			file:   "microsoft-hello.al",
			length: 583,
			entries: []struct {
				qualified, native                        string
				nameLine, start, end, bodyStart, bodyEnd int
			}{
				{"HelloWorld", "codeunit", 6, 6, 16, 0, 0},
				{"HelloWorld.OnRun", "trigger", 10, 10, 15, 13, 15},
			},
		},
		{
			file:   "alappextensions-localexport.al",
			length: 3021,
			entries: []struct {
				qualified, native                        string
				nameLine, start, end, bodyStart, bodyEnd int
			}{
				{"Microsoft.Service.Reports", "namespace", 5, 5, 5, 0, 0},
				{"Microsoft.Service.Reports.Local Export Serv. Decl.", "codeunit", 10, 10, 65, 0, 0},
				{"Microsoft.Service.Reports.Local Export Serv. Decl..OnRun", "trigger", 14, 14, 64, 25, 64},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != int64(tc.length) {
				t.Fatalf("pinned original source identity changed: info=%v err=%v", info, err)
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, eol := range []struct{ name, src string }{
				{"LF", string(bytes)},
				{"CRLF", strings.ReplaceAll(string(bytes), "\n", "\r\n")},
			} {
				t.Run(eol.name, func(t *testing.T) {
					result, err := (ALAnalyzer{}).Analyze(context.Background(),
						testSourceDocument(tc.file, eol.src), testAnalyzeOptions(true, 256))
					if err != nil {
						t.Fatal(err)
					}
					if !result.Analysis.CoverageComplete || result.Analysis.Truncated ||
						len(result.Analysis.Symbols) != len(tc.entries) {
						t.Fatalf("whole original structural count mismatch: %+v", result.Analysis)
					}
					byName := symbolsByQualifiedName(result.Analysis.Symbols)
					for _, want := range tc.entries {
						symbol, ok := byName[want.qualified]
						if !ok || symbol.NativeKind != want.native ||
							symbol.NameRange.Start.Line != want.nameLine ||
							symbol.DeclarationRange.Start.Line != want.start ||
							symbol.DeclarationRange.End.Line != want.end {
							t.Errorf("authored %s range/owner mismatch: %+v", want.qualified, symbol)
							continue
						}
						if want.bodyStart != 0 &&
							(symbol.BodyRange == nil ||
								symbol.BodyRange.Start.Line != want.bodyStart ||
								symbol.BodyRange.End.Line != want.bodyEnd) {
							t.Errorf("authored trigger %s body truncated: %+v", want.qualified, symbol)
						}
					}
				})
			}
		})
	}
}

func TestALSourceDerivedTriggerHeaderAndNoFabricatedLocals(t *testing.T) {
	const source = "namespace Reports;\ncodeunit 50100 Example\n{\n" +
		"  trigger OnRun();\n  var\n    input: Text;\n  begin\n" +
		"    Message(input);\n  end;\n}\n"
	for _, tc := range []struct{ name, src string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (ALAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("simple.al", tc.src), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 3 {
				t.Fatalf("source invented local variable or referenced call: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			onRun, ok := byName["Reports.Example.OnRun"]
			if !ok || onRun.BodyRange == nil || onRun.BodyRange.Start.Line != 7 ||
				onRun.BodyRange.End.Line != 9 || onRun.DeclarationRange.End.Line != 9 {
				t.Errorf("trigger semicolon before variables must retain authored body: %+v", onRun)
			}
		})
	}
}
