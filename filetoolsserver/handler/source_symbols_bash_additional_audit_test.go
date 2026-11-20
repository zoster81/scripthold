package handler

import (
	"os"
	"testing"
)

// Task oracles and plausible alternatives were selected from original Bash
// source and test files before querying the source index.
func TestSourceSymbolsOriginalBashAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Bash corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "bash-bash-it-bash-it", language: "bash",
			search: "bash-it", primary: "lib/helpers.bash",
			negativeFile: "aliases/available/bash-it.aliases.bash", negativeQuery: "bash-it",
			paths: []string{
				"bash_it.sh", "lib/helpers.bash", "scripts/reloader.bash",
				"aliases/available/bash-it.aliases.bash",
			},
			oracles: []extraAuditEvidence{
				{"bash_it.sh", "source \"${BASH_IT}/lib/log.bash\""},
				{"bash_it.sh", "source \"${BASH_IT}/scripts/reloader.bash\""},
				{"lib/helpers.bash", "function bash-it() {"},
				{"lib/helpers.bash", "func=\"_enable-$component\""},
				{"lib/helpers.bash", "if [[ \"$verb\" == \"enable\" || \"$verb\" == \"disable\" ]]; then"},
				{"lib/helpers.bash", "if [[ -n \"${BASH_IT_AUTOMATIC_RELOAD_AFTER_CONFIG_CHANGE:-}\" ]]; then"},
				{"lib/helpers.bash", "function _enable-plugin() {"},
				{"lib/helpers.bash", "_enable-thing \"plugins\" \"plugin\""},
				{"lib/helpers.bash", "function _bash-it-reload() {"},
				{"lib/helpers.bash", "function _disable-plugin() {"},
				{"scripts/reloader.bash", "source \"$_bash_it_reloader_file\""},
				{"aliases/available/bash-it.aliases.bash", "alias bshenp=\"bash-it enable plugin\""},
			},
			targets: []extraAuditSymbol{
				{"lib/helpers.bash", "bash-it", 57},
				{"lib/helpers.bash", "_enable-plugin", 1046},
				{"lib/helpers.bash", "_disable-plugin", 966},
				{"lib/helpers.bash", "_bash-it-reload", 883},
			},
		},
		{
			project: "bash-git-quick-stats-git-quick-stats", language: "bash",
			search: "commitsHeatmap", primary: "git-quick-stats",
			negativeFile: "tests/commands_test.sh", negativeQuery: "commitsHeatmap",
			paths: []string{
				"git-quick-stats", "tests/commands_test.sh", "tests/assert.sh",
			},
			oracles: []extraAuditEvidence{
				{"git-quick-stats", "_commit_days=${_GIT_DAYS:-30}"},
				{"git-quick-stats", "Invalid number of days: $_commit_days. Defaulting to 30."},
				{"git-quick-stats", "function commitsCalendarByAuthor() {"},
				{"git-quick-stats", "function commitsHeatmap() {"},
				{"git-quick-stats", "optionPicked \"Commit Heatmap for the last $_commit_days days\""},
				{"git-quick-stats", "for i in $(seq $((_commit_days-1)) -1 0); do"},
				{"git-quick-stats", "-H|--commits-heatmap) commitsHeatmap;;"},
				{"tests/commands_test.sh", "-H, --commits-heatmap"},
				{"tests/commands_test.sh", "You can set _GIT_DAYS to set the number of days for the heatmap"},
				{"tests/commands_test.sh", "assert_contains \"$src --suggest-reviewers\""},
				{"tests/assert.sh", "assert_contains() {"},
			},
			targets: []extraAuditSymbol{
				{"git-quick-stats", "commitsHeatmap", 183},
				{"git-quick-stats", "commitsCalendarByAuthor", 131},
				{"git-quick-stats", "optionPicked", 306},
				{"tests/assert.sh", "assert_contains", 164},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
