package handler

import (
	"os"
	"testing"
)

// Additional originals for provider 51 (Python), distinct from the initial
// Click and HTTPX projects. Oracles come from Git HEAD source inspection.
func TestSourceSymbolsPythonSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original Python projects not configured")
	}

	scenarios := []extraAuditScenario{
		{
			project: "python-attrs", language: "python",
			search: "optional", primary: "src/attr/validators.py",
			negativeFile: "src/attr/converters.py", negativeQuery: "instance_of",
			paths: []string{"src/attr/validators.py", "src/attr/converters.py"},
			oracles: []extraAuditEvidence{
				{file: "src/attr/validators.py", excerpt: "class _OptionalValidator:"},
				{file: "src/attr/validators.py", excerpt: "def optional(validator):"},
				{file: "src/attr/converters.py", excerpt: "def optional(converter):"},
				{file: "src/attr/validators.py", excerpt: "from .converters import default_if_none"},
			},
			targets: []extraAuditSymbol{
				{file: "src/attr/validators.py", name: "optional", line: 215},
				{file: "src/attr/validators.py", name: "_OptionalValidator", line: 202},
				{file: "src/attr/converters.py", name: "optional", line: 19},
			},
		},
		{
			project: "python-fastapi", language: "python",
			search: "Query", primary: "fastapi/param_functions.py",
			negativeFile: "fastapi/params.py", negativeQuery: "get_dependant",
			paths: []string{"fastapi/param_functions.py", "fastapi/params.py"},
			oracles: []extraAuditEvidence{
				{file: "fastapi/param_functions.py", excerpt: "def Query(  # noqa: N802"},
				{file: "fastapi/param_functions.py", excerpt: "return params.Query("},
				{file: "fastapi/params.py", excerpt: "class Query(Param):"},
				{file: "fastapi/params.py", excerpt: "class Path(Param):"},
			},
			targets: []extraAuditSymbol{
				{file: "fastapi/params.py", name: "Query", line: 221},
				{file: "fastapi/params.py", name: "Path", line: 137},
			},
		},
	}
	for _, scenario := range scenarios {
		checkAdditionalOriginalNavigation(t, corpus, scenario)
	}
}
