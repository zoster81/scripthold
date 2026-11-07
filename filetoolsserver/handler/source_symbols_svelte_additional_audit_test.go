package handler

import (
	"os"
	"testing"
)

// Svelte project maintenance navigation, not UI execution or dynamic state proof.
func TestSourceSymbolsOriginalSvelteAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Svelte corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "svelte-abdenasser-neohtop", language: "svelte",
			search: "selectRefreshRate", primary: "src/lib/components/toolbar/RefreshControls.svelte",
			negativeFile:  "src/lib/components/toolbar/PaginationControls.svelte",
			negativeQuery: "selectRefreshRate",
			paths: []string{
				"src/lib/components/toolbar/RefreshControls.svelte",
				"src/lib/components/toolbar/PaginationControls.svelte",
				"src/lib/components/toolbar/ToolBar.svelte",
				"src/lib/components/stats/StatsBar.svelte",
			},
			oracles: []extraAuditEvidence{
				{"src/lib/components/toolbar/RefreshControls.svelte", "function selectRefreshRate(value: number)"},
				{"src/lib/components/toolbar/RefreshControls.svelte", "updateBehaviorConfig(\"refreshRate\", refreshRate)"},
				{"src/lib/components/toolbar/RefreshControls.svelte", "REFRESH_RATE_OPTIONS.find"},
				{"src/lib/components/toolbar/RefreshControls.svelte", "on:click|stopPropagation={() => selectRefreshRate(option.value)}"},
				{"src/lib/components/toolbar/ToolBar.svelte", "<RefreshControls bind:refreshRate bind:isFrozen />"},
				{"src/lib/components/toolbar/PaginationControls.svelte", "function selectItemsPerPage(value: number)"},
				{"src/lib/components/stats/StatsBar.svelte", "<CpuPanel cpuUsage={systemStats.cpu_usage} />"},
				{"src/lib/constants/index.ts", "export const REFRESH_RATE_OPTIONS = ["},
				{"src/lib/constants/index.ts", "{ value: 2000, label: \"2s\" }"},
			},
			targets: []extraAuditSymbol{
				{"src/lib/components/toolbar/RefreshControls.svelte", "selectRefreshRate", 27},
				{"src/lib/components/toolbar/RefreshControls.svelte", "updateBehaviorConfig", 18},
				{"src/lib/components/toolbar/RefreshControls.svelte", "getCurrentLabel", 33},
				{"src/lib/components/toolbar/PaginationControls.svelte", "selectItemsPerPage", 33},
				{"src/lib/components/toolbar/PaginationControls.svelte", "changePage", 18},
			},
		},
		{
			project: "svelte-bits", language: "svelte",
			search: "copyToClipboard", primary: "docs/src/lib/components/demo-code-tabs.svelte",
			negativeFile:  "docs/src/lib/components/copy-page-dropdown.svelte",
			negativeQuery: "copyToClipboard",
			paths: []string{
				"docs/src/lib/components/demo-code-container.svelte",
				"docs/src/lib/components/demo-code-tabs.svelte",
				"docs/src/lib/components/copy-code-button.svelte",
				"docs/src/lib/components/copy-page-dropdown.svelte",
			},
			oracles: []extraAuditEvidence{
				{"docs/src/lib/components/demo-code-container.svelte", "import DemoCodeTabs from \"./demo-code-tabs.svelte\""},
				{"docs/src/lib/components/demo-code-container.svelte", "const copyToClipboard = useCopyToClipboard();"},
				{"docs/src/lib/components/demo-code-container.svelte", "copyToClipboard?.setCodeString(codeWrapper.innerText.trim()"},
				{"docs/src/lib/components/demo-code-tabs.svelte", "const copyToClipboard = useCopyToClipboard();"},
				{"docs/src/lib/components/demo-code-tabs.svelte", "onclick={() => copyToClipboard?.copyToClipboard()}"},
				{"docs/src/lib/components/copy-code-button.svelte", "onclick={copyCode}"},
				{"docs/src/lib/components/copy-page-dropdown.svelte", "View Markdown"},
				{"docs/src/lib/utils/copy-to-clipboard.svelte.ts", "export class CopyToClipboard"},
				{"docs/src/lib/utils/copy-to-clipboard.svelte.ts", "navigator.clipboard.writeText(value).then"},
			},
			targets: []extraAuditSymbol{
				{"docs/src/lib/components/demo-code-tabs.svelte", "copyToClipboard", 29},
				{"docs/src/lib/components/demo-code-container.svelte", "copyToClipboard", 40},
				{"docs/src/lib/components/demo-code-container.svelte", "activeValue", 37},
				{"docs/src/lib/components/copy-page-dropdown.svelte", "path", 14},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
