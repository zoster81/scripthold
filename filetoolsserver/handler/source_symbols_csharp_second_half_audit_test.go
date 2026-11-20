package handler

import (
	"os"
	"testing"
)

// Provider 53: original MediatR and Humanizer repositories are independent
// of the initial Dapper and Serilog projects. These checks inspect source
// navigation only and never build or execute third-party .NET applications.
func TestSourceSymbolsCSharpSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original C# projects not configured")
	}
	for _, scenario := range []extraAuditScenario{
		{
			project: "csharp-mediatr", language: "csharp",
			search: "Handle", primary: "samples/MediatR.Examples/PingHandler.cs",
			negativeFile: "samples/MediatR.Examples/Ping.cs", negativeQuery: "Handle",
			paths: []string{
				"samples/MediatR.Examples/Ping.cs",
				"samples/MediatR.Examples/PingHandler.cs",
				"samples/MediatR.Examples/Pong.cs",
				"samples/MediatR.Examples/Runner.cs",
			},
			oracles: []extraAuditEvidence{
				{"samples/MediatR.Examples/Ping.cs", "public class Ping : IRequest<Pong>"},
				{"samples/MediatR.Examples/PingHandler.cs", "public class PingHandler : IRequestHandler<Ping, Pong>"},
				{"samples/MediatR.Examples/PingHandler.cs", "public async Task<Pong> Handle(Ping request, CancellationToken cancellationToken)"},
				{"samples/MediatR.Examples/PingHandler.cs", "return new Pong { Message = request.Message + \" Pong\" };"},
				{"samples/MediatR.Examples/Pong.cs", "public class Pong"},
				{"samples/MediatR.Examples/Runner.cs", "var pong = await mediator.Send(new Ping { Message = \"Ping\" });"},
			},
			targets: []extraAuditSymbol{
				{"samples/MediatR.Examples/Ping.cs", "Ping", 3},
				{"samples/MediatR.Examples/PingHandler.cs", "PingHandler", 8},
				{"samples/MediatR.Examples/PingHandler.cs", "Handle", 17},
				{"samples/MediatR.Examples/Pong.cs", "Pong", 3},
			},
		},
		{
			project: "csharp-humanizer", language: "csharp",
			search: "HumanizeWithUnitSystem", primary: "src/Humanizer/Bytes/ByteRate.cs",
			negativeFile:  "src/Humanizer/Bytes/ByteSizeUnitSystem.cs",
			negativeQuery: "HumanizeWithUnitSystem",
			paths: []string{
				"src/Humanizer/Bytes/ByteRate.cs",
				"src/Humanizer/Bytes/ByteSizeExtensions.cs",
				"src/Humanizer/Bytes/ByteSizeUnitSystem.cs",
			},
			oracles: []extraAuditEvidence{
				{"src/Humanizer/Bytes/ByteRate.cs", "public class ByteRate(ByteSize size, TimeSpan interval) :"},
				{"src/Humanizer/Bytes/ByteRate.cs", "public string HumanizeWithUnitSystem("},
				{"src/Humanizer/Bytes/ByteRate.cs", "return ScaleForExplicitUnitSystem(displayInterval)"},
				{"src/Humanizer/Bytes/ByteRate.cs", "ByteSize ScaleForExplicitUnitSystem(TimeSpan displayInterval)"},
				{"src/Humanizer/Bytes/ByteSizeExtensions.cs", "public static string HumanizeWithUnitSystem("},
				{"src/Humanizer/Bytes/ByteSizeUnitSystem.cs", "public enum ByteSizeUnitSystem"},
			},
			targets: []extraAuditSymbol{
				{"src/Humanizer/Bytes/ByteRate.cs", "ByteRate", 11},
				{"src/Humanizer/Bytes/ByteRate.cs", "HumanizeWithUnitSystem", 73},
				{"src/Humanizer/Bytes/ByteRate.cs", "ScaleForExplicitUnitSystem", 95},
				{"src/Humanizer/Bytes/ByteSizeExtensions.cs", "HumanizeWithUnitSystem", 474},
				{"src/Humanizer/Bytes/ByteSizeUnitSystem.cs", "ByteSizeUnitSystem", 12},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, scenario)
	}
}
