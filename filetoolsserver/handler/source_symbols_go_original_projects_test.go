package handler

import (
	"os"
	"testing"
)

// Independently acquired Cobra and Chi source is inspected, never executed.
// The scenarios validate bounded Go declaration navigation, not CLI or HTTP runtime behavior.
func TestSourceSymbolsOriginalGoInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Go corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "go-cobra", language: "go",
			search: "ValidateFlagGroups", primary: "flag_groups.go",
			negativeFile: "args.go", negativeQuery: "ValidateFlagGroups",
			paths: []string{"flag_groups.go", "flag_groups_test.go", "args.go"},
			oracles: []extraAuditEvidence{
				{"flag_groups.go", "func (c *Command) ValidateFlagGroups() error {"},
				{"flag_groups.go", "if c.DisableFlagParsing {"},
				{"flag_groups.go", "validateRequiredFlagGroups(groupStatus)"},
				{"flag_groups.go", "validateOneRequiredFlagGroups(oneRequiredGroupStatus)"},
				{"flag_groups.go", "validateExclusiveFlagGroups(mutuallyExclusiveGroupStatus)"},
				{"flag_groups.go", "func (c *Command) MarkFlagsMutuallyExclusive(flagNames ...string) {"},
				{"flag_groups_test.go", "func TestValidateFlagGroups(t *testing.T) {"},
				{"flag_groups_test.go", "desc:                \"Exclusive flag group not satisfied\""},
				{"command.go", "if err := c.ValidateFlagGroups(); err != nil {"},
				{"args.go", "func NoArgs(cmd *Command, args []string) error {"},
			},
			targets: []extraAuditSymbol{
				{"flag_groups.go", "ValidateFlagGroups", 81},
				{"flag_groups.go", "MarkFlagsMutuallyExclusive", 65},
				{"flag_groups.go", "validateExclusiveFlagGroups", 188},
				{"flag_groups_test.go", "TestValidateFlagGroups", 22},
			},
		},
		{
			project: "go-chi", language: "go",
			search: "Recoverer", primary: "middleware/recoverer.go",
			negativeFile: "middleware/request_id.go", negativeQuery: "Recoverer",
			paths: []string{
				"middleware/recoverer.go",
				"middleware/recoverer_test.go",
				"middleware/request_id.go",
				"middleware/request_id_test.go",
			},
			oracles: []extraAuditEvidence{
				{"middleware/recoverer.go", "func Recoverer(next http.Handler) http.Handler {"},
				{"middleware/recoverer.go", "if rvr == http.ErrAbortHandler {"},
				{"middleware/recoverer.go", "w.WriteHeader(http.StatusInternalServerError)"},
				{"middleware/recoverer.go", "func PrintPrettyStack(rvr interface{}) {"},
				{"middleware/recoverer_test.go", "func TestRecoverer(t *testing.T) {"},
				{"middleware/recoverer_test.go", "func TestRecovererNoColor(t *testing.T) {"},
				{"middleware/recoverer_test.go", "func TestRecovererAbortHandler(t *testing.T) {"},
				{"middleware/request_id.go", "func RequestID(next http.Handler) http.Handler {"},
				{"middleware/request_id_test.go", "func TestRequestID(t *testing.T) {"},
			},
			targets: []extraAuditSymbol{
				{"middleware/recoverer.go", "Recoverer", 22},
				{"middleware/recoverer.go", "PrintPrettyStack", 54},
				{"middleware/recoverer_test.go", "TestRecoverer", 19},
				{"middleware/recoverer_test.go", "TestRecovererNoColor", 48},
				{"middleware/recoverer_test.go", "TestRecovererAbortHandler", 78},
				{"middleware/request_id.go", "RequestID", 67},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
