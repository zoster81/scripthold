package handler

import (
	"os"
	"testing"
)

// Two independent original C# projects; no SQL database, .NET build, or
// logging runtime is executed. Navigation covers selected symbols and tests.
func TestSourceSymbolsOriginalCSharpInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original C# corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "csharp-dapper", language: "csharp",
			search: "AddParameter", primary: "Dapper/DbString.cs",
			negativeFile: "Dapper/DefaultTypeMap.cs", negativeQuery: "AddParameter",
			paths: []string{"Dapper/DbString.cs", "tests/Dapper.Tests/MiscTests.cs", "Dapper/DefaultTypeMap.cs"},
			oracles: []extraAuditEvidence{
				{"Dapper/DbString.cs", "public sealed class DbString : SqlMapper.ICustomQueryParameter"},
				{"Dapper/DbString.cs", "public const int DefaultLength = 4000;"},
				{"Dapper/DbString.cs", "public void AddParameter(IDbCommand command, string name)"},
				{"Dapper/DbString.cs", "if (IsFixedLength && Length == -1)"},
				{"Dapper/DbString.cs", "param.Size = DefaultLength;"},
				{"Dapper/DbString.cs", "param.DbType = IsAnsi ?"},
				{"Dapper/SqlMapper.ICustomQueryParameter.cs", "void AddParameter(IDbCommand command, string name);"},
				{"tests/Dapper.Tests/MiscTests.cs", "public void TestDbString()"},
				{"tests/Dapper.Tests/MiscTests.cs", "public void DbStringNullHandling()"},
				{"tests/Dapper.Tests/MiscTests.cs", "public void TestDefaultDbStringDbType()"},
				{"Dapper/DefaultTypeMap.cs", "public sealed class DefaultTypeMap : SqlMapper.ITypeMap"},
			},
			targets: []extraAuditSymbol{
				{"Dapper/DbString.cs", "DbString", 9},
				{"Dapper/DbString.cs", "DefaultLength", 21},
				{"Dapper/DbString.cs", "AddParameter", 71},
				{"tests/Dapper.Tests/MiscTests.cs", "TestDbString", 640},
				{"tests/Dapper.Tests/MiscTests.cs", "DbStringNullHandling", 661},
				{"tests/Dapper.Tests/MiscTests.cs", "TestDefaultDbStringDbType", 702},
				{"Dapper/DefaultTypeMap.cs", "DefaultTypeMap", 11},
			},
		},
		{
			project: "csharp-serilog", language: "csharp",
			search: "PushProperty", primary: "src/Serilog/Context/LogContext.cs",
			negativeFile:  "src/Serilog/Core/LoggingLevelSwitch.cs",
			negativeQuery: "PushProperty",
			paths: []string{
				"src/Serilog/Context/LogContext.cs",
				"test/Serilog.Tests/Context/LogContextTests.cs",
				"src/Serilog/Core/LoggingLevelSwitch.cs",
			},
			oracles: []extraAuditEvidence{
				{"src/Serilog/Context/LogContext.cs", "public static class LogContext"},
				{"src/Serilog/Context/LogContext.cs", "static readonly AsyncLocal<EnricherStack?> Data = new();"},
				{"src/Serilog/Context/LogContext.cs", "public static IDisposable PushProperty(string name, object? value, bool destructureObjects = false)"},
				{"src/Serilog/Context/LogContext.cs", "return Push(new PropertyEnricher(name, value, destructureObjects));"},
				{"src/Serilog/Context/LogContext.cs", "public static ILogEventEnricher Clone()"},
				{"test/Serilog.Tests/Context/LogContextTests.cs", "public void PushedPropertiesAreAvailableToLoggers()"},
				{"test/Serilog.Tests/Context/LogContextTests.cs", "using (LogContext.PushProperty(\"A\", 1))"},
				{"test/Serilog.Tests/Context/LogContextTests.cs", "public void ClonedLogContextCanSharedAcrossThreads()"},
				{"src/Serilog/Configuration/LoggerEnrichmentConfiguration.cs", "public LoggerConfiguration FromLogContext() => With<LogContextEnricher>();"},
				{"src/Serilog/Core/LoggingLevelSwitch.cs", "public class LoggingLevelSwitch"},
			},
			targets: []extraAuditSymbol{
				{"src/Serilog/Context/LogContext.cs", "LogContext", 38},
				{"src/Serilog/Context/LogContext.cs", "PushProperty", 55},
				{"src/Serilog/Context/LogContext.cs", "Clone", 158},
				{"test/Serilog.Tests/Context/LogContextTests.cs", "PushedPropertiesAreAvailableToLoggers", 63},
				{"test/Serilog.Tests/Context/LogContextTests.cs", "ClonedLogContextCanSharedAcrossThreads", 110},
				{"src/Serilog/Core/LoggingLevelSwitch.cs", "LoggingLevelSwitch", 20},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
