package handler

import (
	"os"
	"testing"
)

// Scala original-project cases are inspected as read-only source evidence.
// Neither original program nor its test suites are executed.
func TestSourceSymbolsOriginalScalaInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Scala corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "scala-rtyley-bfg-repo-cleaner", language: "scala",
			search: "filterContentPredicate", primary: "bfg/src/main/scala/com/madgag/git/bfg/cli/CLIConfig.scala",
			negativeFile: "bfg-benchmark/src/main/scala/BenchmarkConfig.scala", negativeQuery: "filterContentPredicate",
			paths: []string{
				"bfg/src/main/scala/com/madgag/git/bfg/cli/CLIConfig.scala",
				"bfg/src/main/scala/com/madgag/git/bfg/cli/Main.scala",
				"bfg/src/test/scala/com/madgag/git/bfg/cli/CLIConfigSpecs.scala",
				"bfg-benchmark/src/main/scala/BenchmarkConfig.scala",
			},
			oracles: []extraAuditEvidence{
				{"bfg/src/main/scala/com/madgag/git/bfg/cli/CLIConfig.scala", "fileMatcher(\"filter-content-including\")"},
				{"bfg/src/main/scala/com/madgag/git/bfg/cli/CLIConfig.scala", "c.filenameFilters :+ Include(v)"},
				{"bfg/src/main/scala/com/madgag/git/bfg/cli/CLIConfig.scala", "fileMatcher(\"filter-content-excluding\")"},
				{"bfg/src/main/scala/com/madgag/git/bfg/cli/CLIConfig.scala", "c.filenameFilters :+ Exclude(v)"},
				{"bfg/src/main/scala/com/madgag/git/bfg/cli/CLIConfig.scala", "IncExcExpression(filenameFilters) includes (f.string)"},
				{"bfg/src/main/scala/com/madgag/git/bfg/cli/CLIConfig.scala", "if (filterContentPredicate(entry.filename)) Some(replacer) else None"},
				{"bfg/src/main/scala/com/madgag/git/bfg/cli/Main.scala", "CLIConfig.parser.parse(args, CLIConfig()) map"},
				{"bfg/src/test/scala/com/madgag/git/bfg/cli/CLIConfigSpecs.scala", "predicate(FileName(\"hbm.xml\")) shouldBe true"},
				{"bfg/src/test/scala/com/madgag/git/bfg/cli/CLIConfigSpecs.scala", "predicate(FileName(\"foo.xml\")) shouldBe false"},
				{"bfg-benchmark/src/main/scala/BenchmarkConfig.scala", "opt[String](\"versions\")"},
			},
			targets: []extraAuditSymbol{
				{"bfg/src/main/scala/com/madgag/git/bfg/cli/CLIConfig.scala", "CLIConfig", 128},
				{"bfg/src/main/scala/com/madgag/git/bfg/cli/CLIConfig.scala", "filterContentPredicate", 170},
				{"bfg/src/main/scala/com/madgag/git/bfg/cli/CLIConfig.scala", "blobTextModifier", 172},
				{"bfg/src/main/scala/com/madgag/git/bfg/cli/Main.scala", "Main", 27},
				{"bfg/src/test/scala/com/madgag/git/bfg/cli/CLIConfigSpecs.scala", "parse", 10},
				{"bfg-benchmark/src/main/scala/BenchmarkConfig.scala", "BenchmarkConfig", 7},
			},
		},
		{
			project: "scala-lhartikk-arnoldc", language: "scala",
			search: "ConditionStatement", primary: "src/main/scala/org/arnoldc/ArnoldParser.scala",
			negativeFile: "src/main/scala/org/arnoldc/Declaimer.scala", negativeQuery: "ConditionStatement",
			paths: []string{
				"src/main/scala/org/arnoldc/ArnoldParser.scala",
				"src/main/scala/org/arnoldc/ArnoldGenerator.scala",
				"src/main/scala/org/arnoldc/ast/ConditionNode.scala",
				"src/test/scala/org/arnoldc/BranchStatementTest.scala",
				"src/main/scala/org/arnoldc/Declaimer.scala",
			},
			oracles: []extraAuditEvidence{
				{"src/main/scala/org/arnoldc/ArnoldParser.scala", "def ConditionStatement: Rule1[ConditionNode] = rule {"},
				{"src/main/scala/org/arnoldc/ArnoldParser.scala", "Else ~ EOL ~ zeroOrMore(Statement) ~~> ConditionNode"},
				{"src/main/scala/org/arnoldc/ArnoldParser.scala", "def Statement: Rule1[StatementNode] = rule {"},
				{"src/main/scala/org/arnoldc/ArnoldGenerator.scala", "val parser = new ArnoldParser"},
				{"src/main/scala/org/arnoldc/ArnoldGenerator.scala", "val rootNode = parser.parse(arnoldCode)"},
				{"src/main/scala/org/arnoldc/ast/ConditionNode.scala", "case class ConditionNode(condition: OperandNode"},
				{"src/main/scala/org/arnoldc/ast/ConditionNode.scala", "ifBranch.foreach(_.generate(mv, symbolTable))"},
				{"src/main/scala/org/arnoldc/ast/ConditionNode.scala", "elseBranch.foreach(_.generate(mv, symbolTable))"},
				{"src/test/scala/org/arnoldc/BranchStatementTest.scala", "function using simple if else statements"},
				{"src/main/scala/org/arnoldc/Declaimer.scala", "case ConditionNode(condition, ifStmts, elseStmts) =>"},
				{"src/main/scala/org/arnoldc/Declaimer.scala", "if (elseStmts.nonEmpty) {"},
			},
			targets: []extraAuditSymbol{
				{"src/main/scala/org/arnoldc/ArnoldParser.scala", "ArnoldParser", 7},
				{"src/main/scala/org/arnoldc/ArnoldParser.scala", "ConditionStatement", 84},
				{"src/main/scala/org/arnoldc/ArnoldGenerator.scala", "generate", 8},
				{"src/main/scala/org/arnoldc/ast/ConditionNode.scala", "ConditionNode", 8},
				{"src/main/scala/org/arnoldc/ast/ConditionNode.scala", "generate", 9},
				{"src/test/scala/org/arnoldc/BranchStatementTest.scala", "BranchStatementTest", 3},
				{"src/main/scala/org/arnoldc/Declaimer.scala", "Declaimer", 13},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
