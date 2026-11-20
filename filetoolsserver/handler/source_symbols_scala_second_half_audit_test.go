package handler

import (
	"os"
	"testing"
)

// Provider 61: original scala-xml and os-lib sources, independent of the
// initial BFG and ArnoldC projects. Original applications are not executed.
func TestSourceSymbolsScalaSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original Scala projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "scala61-scala-xml-audit-selected", language: "scala",
			search:        "formatNodes",
			primary:       "shared/src/main/scala/scala/xml/PrettyPrinter.scala",
			negativeFile:  "shared/src/main/scala/scala/xml/Elem.scala",
			negativeQuery: "formatNodes",
			paths: []string{
				"shared/src/main/scala/scala/xml/PrettyPrinter.scala",
				"shared/src/main/scala/scala/xml/Utility.scala",
				"shared/src/main/scala/scala/xml/Elem.scala",
				"jvm/src/test/scala/scala/xml/XMLSyntaxTest.scala",
			},
			oracles: []extraAuditEvidence{
				{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", "class PrettyPrinter(width: Int, step: Int, minimizeEmpty: Boolean) {"},
				{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", "def format(n: Node, sb: StringBuilder): Unit = format(n, TopScope, sb)"},
				{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", "def formatNodes(nodes: Seq[Node], pscope: NamespaceBinding = TopScope): String ="},
				{"shared/src/main/scala/scala/xml/Utility.scala", "object Utility extends AnyRef with parsing.TokenTests with ScalaVersionSpecificUtility {"},
				{"shared/src/main/scala/scala/xml/Utility.scala", "def serialize("},
				{"shared/src/main/scala/scala/xml/Elem.scala", "class Elem("},
				{"shared/src/main/scala/scala/xml/Elem.scala", "def copy("},
				{"jvm/src/test/scala/scala/xml/XMLSyntaxTest.scala", "class XMLSyntaxTestJVM {"},
			},
			targets: []extraAuditSymbol{
				{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", "format", 223},
				{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", "formatNodes", 272},
				{"shared/src/main/scala/scala/xml/Utility.scala", "serialize", 204},
				{"shared/src/main/scala/scala/xml/Elem.scala", "copy", 100},
			},
		},
		{
			project: "scala61-os-lib-audit-selected", language: "scala",
			search: "segmentsFromString", primary: "os/src/Path.scala",
			negativeFile:  "os/src/ProcessOps.scala",
			negativeQuery: "segmentsFromString",
			paths: []string{
				"os/src/Path.scala",
				"os/src/GlobInterpolator.scala",
				"os/src/ProcessOps.scala",
				"os/test/src/PathTests.scala",
			},
			oracles: []extraAuditEvidence{
				{"os/src/Path.scala", "object PathChunk extends PathChunkMacros {"},
				{"os/src/Path.scala", "def segmentsFromString(s: String): Array[String] = {"},
				{"os/src/Path.scala", "private def reduceUps(in: Array[String]): List[String] ="},
				{"os/src/GlobInterpolator.scala", "object GlobInterpolator {"},
				{"os/src/GlobInterpolator.scala", "def unapplySeq(s: String) = {"},
				{"os/test/src/PathTests.scala", "object PathTests extends TestSuite {"},
				{"os/test/src/PathTests.scala", "compileError"},
			},
			targets: []extraAuditSymbol{
				{"os/src/Path.scala", "segmentsFromString", 25},
				{"os/src/Path.scala", "reduceUps", 31},
				{"os/src/GlobInterpolator.scala", "unapplySeq", 5},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
