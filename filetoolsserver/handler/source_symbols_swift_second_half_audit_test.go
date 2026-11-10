package handler

import (
	"os"
	"testing"
)

// Provider 55: two further independent original Swift repositories, distinct
// from the initial Alamofire and Swift Collections scenarios. The selected
// Git HEAD bytes are optional corpus fixtures; no Swift compiler, test runner
// or original program is executed.
func TestSourceSymbolsSwiftSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original Swift projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "swift55-swift-compiler-audit-selected", language: "swift",
			search: "arguments", primary: "stdlib/public/core/CommandLine.swift",
			negativeFile: "stdlib/public/core/ByteOrder.swift", negativeQuery: "arguments",
			paths: []string{
				"stdlib/public/core/CommandLine.swift",
				"test/Interpreter/process_arguments.swift",
				"validation-test/StdlibUnittest/CommandLine.swift",
				"stdlib/public/core/ByteOrder.swift",
			},
			oracles: []extraAuditEvidence{
				{"stdlib/public/core/CommandLine.swift", "public enum CommandLine: ~BitwiseCopyable {}"},
				{"stdlib/public/core/CommandLine.swift", "public static var argc: Int32 {"},
				{"stdlib/public/core/CommandLine.swift", "public static var arguments: [String] {"},
				{"stdlib/public/core/CommandLine.swift", "_swift_stdlib_getUnsafeArgvArgc"},
				{"test/Interpreter/process_arguments.swift", "for arg in CommandLine.arguments { print(arg) }"},
				{"validation-test/StdlibUnittest/CommandLine.swift", "debugPrint(CommandLine.arguments)"},
				{"stdlib/public/core/ByteOrder.swift", "public enum ByteOrder: Equatable, Hashable, Sendable {"},
			},
			targets: []extraAuditSymbol{
				{"stdlib/public/core/CommandLine.swift", "CommandLine", 23},
				{"stdlib/public/core/CommandLine.swift", "argc", 42},
				{"stdlib/public/core/CommandLine.swift", "arguments", 109},
				{"stdlib/public/core/ByteOrder.swift", "ByteOrder", 16},
				{"stdlib/public/core/ByteOrder.swift", "native", 27},
			},
		},
		{
			project: "swift55-exercism-audit-selected", language: "swift",
			search:        "orderByScores",
			primary:       "exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift",
			negativeFile:  "exercises/concept/bird-watcher/Sources/BirdWatcher/BirdWatcher.swift",
			negativeQuery: "orderByScores",
			paths: []string{
				"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift",
				"exercises/concept/high-score-board/Sources/HighScoreBoard/HighScoreBoard.swift",
				"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift",
				"exercises/concept/bird-watcher/Sources/BirdWatcher/BirdWatcher.swift",
			},
			oracles: []extraAuditEvidence{
				{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", "func orderByScores(_ scores: [String: Int]) -> [(String, Int)] {"},
				{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", "func descendingValues(_ a: (String, Int), _ b: (String, Int)) -> Bool {"},
				{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", "func orderByPlayers(_ scores: [String: Int]) -> [(String, Int)] {"},
				{"exercises/concept/high-score-board/Sources/HighScoreBoard/HighScoreBoard.swift", "fatalError(\"Please implement the orderByScores(_:) function\")"},
				{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", "func testOrderByScores() {"},
				{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", "let got = orderByScores(scoreboard)"},
				{"exercises/concept/bird-watcher/Sources/BirdWatcher/BirdWatcher.swift", "func birdsInWeek(_ birdsPerDay: [Int], weekNumber: Int) -> Int {"},
			},
			targets: []extraAuditSymbol{
				{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", "orderByScores", 32},
				{"exercises/concept/high-score-board/.meta/Sources/HighScoreBoard/HighScoreBoardExemplar.swift", "orderByPlayers", 25},
				{"exercises/concept/high-score-board/Sources/HighScoreBoard/HighScoreBoard.swift", "orderByScores", 25},
				{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", "testOrderByScores", 106},
				{"exercises/concept/high-score-board/Tests/HighScoreBoardTests/HighScoreBoardTests.swift", "testOrderByPlayers", 91},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
