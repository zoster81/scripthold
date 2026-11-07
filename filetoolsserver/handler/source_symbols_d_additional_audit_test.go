package handler

import (
	"os"
	"testing"
)

// D source-first navigation on two further independent original projects.
func TestSourceSymbolsOriginalDAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original D corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "d-dadoum-sideloader", language: "d",
			search:        "diagnostic",
			primary:       "source/tools/sidestorepairingfile.d",
			negativeFile:  "source/tools/package.d",
			negativeQuery: "SideStoreTool",
			paths: []string{
				"source/tools/sidestorepairingfile.d",
				"source/tools/package.d",
			},
			oracles: []extraAuditEvidence{
				{"source/tools/sidestorepairingfile.d", "class SideStoreTool: Tool {"},
				{"source/tools/sidestorepairingfile.d", "override string diagnostic() {"},
				{"source/tools/sidestorepairingfile.d", "override void run(bool delegate(string message, bool canCancel = true) notify) {"},
				{"source/tools/sidestorepairingfile.d", "lockdowndClient.pair()"},
				{"source/tools/sidestorepairingfile.d", "LOCKDOWN_E_PAIRING_DIALOG_RESPONSE_PENDING"},
				{"source/tools/sidestorepairingfile.d", "houseArrest.sendCommand(\"VendContainer\", sideStoreBundleId);"},
				{"source/tools/sidestorepairingfile.d", "AFCFileMode.AFC_FOPEN_WRONLY"},
				{"source/tools/sidestorepairingfile.d", "afcClient.write(remoteFile, fileData)"},
				{"source/tools/package.d", "abstract class Tool {"},
				{"source/tools/package.d", "new SideStoreTool(device),"},
				{"source/imobiledevice/afc.d", "AFC_FOPEN_WRONLY = 0x00000003"},
				{"frontends/cli/source/device.d", "struct DeviceCommand"},
			},
			targets: []extraAuditSymbol{
				{"source/tools/sidestorepairingfile.d", "SideStoreTool", 16},
				{"source/tools/sidestorepairingfile.d", "diagnostic", 40},
				{"source/tools/sidestorepairingfile.d", "run", 44},
				{"source/tools/package.d", "Tool", 5},
			},
		},
		{
			project: "d-ebay-tsv-utils", language: "d",
			search:        "numberLines",
			primary:       "number-lines/src/tsv_utils/number-lines.d",
			negativeFile:  "keep-header/src/tsv_utils/keep-header.d",
			negativeQuery: "numberLines",
			paths: []string{
				"number-lines/src/tsv_utils/number-lines.d",
				"common/src/tsv_utils/common/utils.d",
				"keep-header/src/tsv_utils/keep-header.d",
			},
			oracles: []extraAuditEvidence{
				{"number-lines/src/tsv_utils/number-lines.d", "struct NumberLinesOptions"},
				{"number-lines/src/tsv_utils/number-lines.d", "long startNum = 1;"},
				{"number-lines/src/tsv_utils/number-lines.d", "auto processArgs (ref string[] cmdArgs)"},
				{"number-lines/src/tsv_utils/number-lines.d", "if (headerString.length > 0) hasHeader = true;"},
				{"number-lines/src/tsv_utils/number-lines.d", "void numberLines(const NumberLinesOptions cmdopt, const string[] inputFiles)"},
				{"number-lines/src/tsv_utils/number-lines.d", "long lineNum = cmdopt.startNum;"},
				{"number-lines/src/tsv_utils/number-lines.d", "if (cmdopt.hasHeader && fileLineNum == 1)"},
				{"number-lines/src/tsv_utils/number-lines.d", "bufferedOutput.appendln(cmdopt.headerString, cmdopt.delim, line);"},
				{"common/src/tsv_utils/common/utils.d", "struct BufferedOutputRange(OutputTarget)"},
				{"keep-header/src/tsv_utils/keep-header.d", "int main(string[] args)"},
				{"number-lines/tests/tests.sh", "--header -n 10 input1.txt one-line-file.txt input2.txt empty-file.txt"},
			},
			targets: []extraAuditSymbol{
				{"number-lines/src/tsv_utils/number-lines.d", "NumberLinesOptions", 37},
				{"number-lines/src/tsv_utils/number-lines.d", "processArgs", 53},
				{"number-lines/src/tsv_utils/number-lines.d", "numberLines", 132},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
