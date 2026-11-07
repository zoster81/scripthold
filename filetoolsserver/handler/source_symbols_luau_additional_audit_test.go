package handler

import (
	"os"
	"testing"
)

// These navigation cases use independent, source-read maintenance oracles.
// The original Luau projects are inspected but never executed.
func TestSourceSymbolsOriginalLuauAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Luau corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "luau-benchmarks", language: "luau",
			// Where should the baseline-adjusted CSV statistics be corrected?
			search: "normalizeRawElapsed", primary: "scripts/workflow/benchmark/summarize.lune.luau",
			negativeFile: "scripts/workflow/benchmark/Agent.luau", negativeQuery: "normalizeRawElapsed",
			paths: []string{
				"scripts/workflow/benchmark/summarize.lune.luau",
				"scripts/workflow/benchmark/Agent.luau",
			},
			oracles: []extraAuditEvidence{
				{"scripts/workflow/benchmark/init.sh", "lune run scripts/workflow/benchmark/summarize.lune.luau"},
				{"scripts/workflow/benchmark/process-data.sh", "jq -r '.output.results[0].data.raw | keys[]'"},
				{"scripts/workflow/benchmark/Agent.luau", "Main()"},
				{"scripts/workflow/benchmark/summarize.lune.luau", "function parseCSV(path: string): CSV"},
				{"scripts/workflow/benchmark/summarize.lune.luau", "function unpackData(csv: CSV): { DataRow }"},
				{"scripts/workflow/benchmark/summarize.lune.luau", "if baseline >= elapsed or baseline == 0 then"},
				{"scripts/workflow/benchmark/summarize.lune.luau", "function getAverageBaseline(dataSets: { [any]: { DataRow } }): number"},
				{"scripts/workflow/benchmark/summarize.lune.luau", "local relativeSpeed = baseline / avgBaseline"},
				{"scripts/workflow/benchmark/summarize.lune.luau", "return math.ceil((rawElapsed - baseline) / relativeSpeed)"},
				{"scripts/workflow/benchmark/summarize.lune.luau", "standardDeviation = math.sqrt(variance / (count - 1))"},
				{"scripts/workflow/benchmark/summarize.lune.luau", "table.sort(temp)"},
				{"scripts/workflow/benchmark/summarize.lune.luau", "local csv = parseCSV(path)"},
				{"scripts/workflow/benchmark/summarize.lune.luau", "addSummaryRow(path:sub(#DATA_DIR + 2), averageBaseline, dataSet)"},
			},
			targets: []extraAuditSymbol{
				{"scripts/workflow/benchmark/summarize.lune.luau", "parseCSV", 24},
				{"scripts/workflow/benchmark/summarize.lune.luau", "unpackData", 78},
				{"scripts/workflow/benchmark/summarize.lune.luau", "getAverageBaseline", 113},
				{"scripts/workflow/benchmark/summarize.lune.luau", "normalizeRawElapsed", 141},
				{"scripts/workflow/benchmark/summarize.lune.luau", "summarize", 198},
				{"scripts/workflow/benchmark/summarize.lune.luau", "addSummaryRow", 266},
				{"scripts/workflow/benchmark/summarize.lune.luau", "searchForData", 331},
			},
		},
		{
			project: "luau-canonical", language: "luau",
			// Where should Markdown-to-HTML escaping/rendering behavior change?
			search: "renderNode", primary: "bench/tests/vibemark67/markdown-dir/render.luau",
			negativeFile: "bench/tests/vibemark67/markdown-dir/ast.luau", negativeQuery: "renderNode",
			paths: []string{
				"bench/tests/vibemark67/markdown.lua",
				"bench/tests/vibemark67/markdown-dir/ast.luau",
				"bench/tests/vibemark67/markdown-dir/block.luau",
				"bench/tests/vibemark67/markdown-dir/inline.luau",
				"bench/tests/vibemark67/markdown-dir/render.luau",
			},
			oracles: []extraAuditEvidence{
				{"bench/tests/vibemark67/markdown.lua", "local block = require(\"./markdown-dir/block\")"},
				{"bench/tests/vibemark67/markdown.lua", "local render = require(\"./markdown-dir/render\")"},
				{"bench/tests/vibemark67/markdown.lua", "local doc = block.parseBlocks(parser)"},
				{"bench/tests/vibemark67/markdown.lua", "local html = render.render(doc, parser.refDefs)"},
				{"bench/tests/vibemark67/markdown-dir/ast.luau", "local function createNode(nodeType: string): Node"},
				{"bench/tests/vibemark67/markdown-dir/block.luau", "local ast = require(\"./ast\")"},
				{"bench/tests/vibemark67/markdown-dir/block.luau", "local function parseBlocks(parser: BlockParser): ast.Node"},
				{"bench/tests/vibemark67/markdown-dir/inline.luau", "local function parseInlines(text: string, refDefs:"},
				{"bench/tests/vibemark67/markdown-dir/render.luau", "local inline = require(\"./inline\")"},
				{"bench/tests/vibemark67/markdown-dir/render.luau", "s = string.gsub(s, \"&\", \"&amp;\")"},
				{"bench/tests/vibemark67/markdown-dir/render.luau", "table.insert(out, escapeHtml(node.destination))"},
				{"bench/tests/vibemark67/markdown-dir/render.luau", "function renderNode(node: ast.Node, refDefs:"},
			},
			targets: []extraAuditSymbol{
				{"bench/tests/vibemark67/markdown.lua", "test", 7},
				{"bench/tests/vibemark67/markdown-dir/ast.luau", "createNode", 17},
				{"bench/tests/vibemark67/markdown-dir/block.luau", "createBlockParser", 148},
				{"bench/tests/vibemark67/markdown-dir/block.luau", "parseBlocks", 182},
				{"bench/tests/vibemark67/markdown-dir/inline.luau", "parseInlines", 5},
				{"bench/tests/vibemark67/markdown-dir/render.luau", "escapeHtml", 6},
				{"bench/tests/vibemark67/markdown-dir/render.luau", "renderNode", 68},
				{"bench/tests/vibemark67/markdown-dir/render.luau", "render", 139},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
