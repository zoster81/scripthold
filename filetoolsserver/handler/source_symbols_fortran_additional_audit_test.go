package handler

import (
	"os"
	"testing"
)

// Maintenance navigation on independent FDS and xtb originals; no Fortran
// compilation, runtime allocation or numerical/scientific behavior is verified.
func TestSourceSymbolsOriginalFortranAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Fortran corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "fortran-firemodels-fds", language: "fortran",
			search: "ChkMemErr", primary: "Utilities/Misc/checkmem_src/checkmem.f90",
			negativeFile:  "Utilities/Misc/loop_test/main.f90",
			negativeQuery: "ChkMemErr",
			paths: []string{
				"Utilities/Misc/checkmem_src/checkmem.f90",
				"Utilities/Misc/loop_test/main.f90",
			},
			oracles: []extraAuditEvidence{
				{"Utilities/Misc/checkmem_src/checkmem.f90", "PROGRAM CHECKMEM"},
				{"Utilities/Misc/checkmem_src/checkmem.f90", "CALL ChkMemErr(NITER,TOT_REALS,IZERO)"},
				{"Utilities/Misc/checkmem_src/checkmem.f90", "SUBROUTINE ChkMemErr(NITER,TOT_REALS,IZERO)"},
				{"Utilities/Misc/checkmem_src/checkmem.f90", "CALL SHUTDOWN(MESSAGE)"},
				{"Utilities/Misc/loop_test/main.f90", "PROGRAM LOOP3D"},
				{"Utilities/Misc/loop_test/main.f90", "INTEGER, PARAMETER :: NSOLVES = 640"},
				{"Source/main.f90", "PROGRAM FDS"},
			},
			targets: []extraAuditSymbol{
				{"Utilities/Misc/checkmem_src/checkmem.f90", "CHECKMEM", 1},
				{"Utilities/Misc/checkmem_src/checkmem.f90", "ChkMemErr", 28},
				{"Utilities/Misc/checkmem_src/checkmem.f90", "SHUTDOWN", 37},
				{"Utilities/Misc/loop_test/main.f90", "LOOP3D", 1},
			},
		},
		{
			project: "fortran-grimme-lab-xtb", language: "fortran",
			search: "c_f_character", primary: "src/api/utils.f90",
			negativeFile: "src/api/version.f90", negativeQuery: "c_f_character",
			paths: []string{
				"src/api/utils.f90",
				"src/api/environment.f90",
				"src/api/version.f90",
			},
			oracles: []extraAuditEvidence{
				{"src/api/utils.f90", "module xtb_api_utils"},
				{"src/api/utils.f90", "subroutine c_f_character(rhs, lhs)"},
				{"src/api/utils.f90", "subroutine f_c_character(rhs, lhs, len)"},
				{"src/api/environment.f90", "use xtb_api_utils"},
				{"src/api/environment.f90", "call c_f_character(charptr, message)"},
				{"src/api/environment.f90", "call c_f_character(charptr, filename)"},
				{"src/api/environment.f90", "subroutine setOutput_api(venv, charptr) &"},
				{"src/api/version.f90", "function getAPIVersion_api() result(version) &"},
				{"src/api/version.f90", "version = 10000_c_int * apiMajor + 100_c_int * apiMinor + apiPatch"},
			},
			targets: []extraAuditSymbol{
				{"src/api/utils.f90", "xtb_api_utils", 19},
				{"src/api/utils.f90", "c_f_character", 32},
				{"src/api/utils.f90", "f_c_character", 50},
				{"src/api/utils.f90", "checkGlobalEnv", 75},
				{"src/api/environment.f90", "setOutput_api", 131},
				{"src/api/version.f90", "getAPIVersion_api", 34},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
