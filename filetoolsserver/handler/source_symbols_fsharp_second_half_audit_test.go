package handler

import (
	"os"
	"testing"
)

// Provider 66: FSharp.Data and FsCheck are independent original projects,
// different from the initial Elmish and Saturn navigation cases. The selected
// implementation, neighboring and test files were all read in full first.
// These cases check bounded change navigation, not entire F# grammar coverage.
func TestSourceSymbolsFSharpSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original F# projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "fsharp66-fsharp-data-audit-selected", language: "fsharp",
			search: "TryGetColumnIndex", primary: "src/FSharp.Data.Csv.Core/CsvFile.fs",
			negativeFile: "src/FSharp.Data.Csv.Core/CsvExtensions.fs", negativeQuery: "TryGetColumnIndex",
			paths: []string{
				"src/FSharp.Data.Csv.Core/CsvFile.fs",
				"src/FSharp.Data.Csv.Core/CsvExtensions.fs",
				"tests/FSharp.Data.Core.Tests/CsvFile.fs",
			},
			oracles: []extraAuditEvidence{
				{"src/FSharp.Data.Csv.Core/CsvFile.fs", "member _.GetColumnIndex columnName = headerDic.[columnName]"},
				{"src/FSharp.Data.Csv.Core/CsvFile.fs", "member _.TryGetColumnIndex columnName ="},
				{"src/FSharp.Data.Csv.Core/CsvFile.fs", "| false, _ -> None"},
				{"src/FSharp.Data.Csv.Core/CsvFile.fs", "static member Parse"},
				{"src/FSharp.Data.Csv.Core/CsvExtensions.fs", "static member AsInteger(x: String, [<Optional>] ?cultureInfo) ="},
				{"src/FSharp.Data.Csv.Core/CsvExtensions.fs", "let (?) (csvRow: CsvRow) (columnName: string) = csvRow.[columnName]"},
				{"tests/FSharp.Data.Core.Tests/CsvFile.fs", "let ``TryGetColumnIndex returns None for invalid column`` () ="},
				{"tests/FSharp.Data.Core.Tests/CsvFile.fs", "let ``CsvRow dynamic operator works`` () ="},
			},
			targets: []extraAuditSymbol{
				{"src/FSharp.Data.Csv.Core/CsvFile.fs", "GetColumnIndex", 74},
				{"src/FSharp.Data.Csv.Core/CsvFile.fs", "TryGetColumnIndex", 77},
				{"src/FSharp.Data.Csv.Core/CsvFile.fs", "Parse", 83},
				{"src/FSharp.Data.Csv.Core/CsvFile.fs", "AsyncLoad", 190},
				{"src/FSharp.Data.Csv.Core/CsvExtensions.fs", "AsInteger", 20},
				{"src/FSharp.Data.Csv.Core/CsvExtensions.fs", "AsDateTimeOffset", 85},
			},
		},
		{
			project: "fsharp66-fscheck-audit-selected", language: "fsharp",
			search: "TryFinally", primary: "src/FsCheck/FSharp.GenBuilder.fs",
			negativeFile: "src/FsCheck/FSharp.Prop.fs", negativeQuery: "TryFinally",
			paths: []string{
				"src/FsCheck/FSharp.GenBuilder.fs",
				"src/FsCheck/FSharp.Prop.fs",
				"tests/FsCheck.Test/GenBuilder.fs",
			},
			oracles: []extraAuditEvidence{
				{"src/FsCheck/FSharp.GenBuilder.fs", "let inline private tryFinally (Gen m) handler ="},
				{"src/FsCheck/FSharp.GenBuilder.fs", "member _.TryFinally(m: Gen<'T>, handler: unit->unit) = tryFinally m handler"},
				{"src/FsCheck/FSharp.GenBuilder.fs", "member _.Using (a: 'Disposable, k: 'Disposable -> Gen<'T>) =  using a k"},
				{"src/FsCheck/FSharp.GenBuilder.fs", "member _.MergeSources3(gen1: Gen<'T1>, gen2: Gen<'T2>, gen3: Gen<'T3>) ="},
				{"src/FsCheck/FSharp.Prop.fs", "let classify b name : ('Testable -> Property) ="},
				{"tests/FsCheck.Test/GenBuilder.fs", "let ``try finally``() ="},
				{"tests/FsCheck.Test/GenBuilder.fs", "assertTrue finallyHandlerCalled"},
				{"tests/FsCheck.Test/GenBuilder.fs", "and! d = Gen.constant 35"},
			},
			targets: []extraAuditSymbol{
				{"src/FsCheck/FSharp.GenBuilder.fs", "GenBuilder", 35},
				{"src/FsCheck/FSharp.GenBuilder.fs", "TryFinally", 44},
				{"src/FsCheck/FSharp.GenBuilder.fs", "TryWith", 45},
				{"src/FsCheck/FSharp.GenBuilder.fs", "Using", 46},
				{"src/FsCheck/FSharp.GenBuilder.fs", "For", 49},
				{"src/FsCheck/FSharp.GenBuilder.fs", "MergeSources3", 53},
				{"src/FsCheck/FSharp.GenBuilder.fs", "Bind3Return", 60},
				{"src/FsCheck/FSharp.Prop.fs", "classify", 48},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
