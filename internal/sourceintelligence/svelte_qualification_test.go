package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestSvelteQuotedGenericScriptOpening(t *testing.T) {
	source := `<script
 lang="ts"
 generics="
   Context extends Exclude<keyof Model, 'schemaVersion'>,
   Id extends keyof Model[Context]
 ">
 import type { Model } from './types';
 export let settingId: Id;
 function saveSetting() { return settingId; }
</script>
<main id="settings-panel">Save</main>`
	for _, tc := range []struct{ name, source string }{
		{"lf", source},
		{"crlf", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (SvelteAnalyzer{}).Analyze(context.Background(), testSourceDocument("SettingsToggle.svelte", tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatalf("valid multiline quoted generic script incorrectly partial: %+v", result.Analysis.Diagnostics)
			}
			if len(result.Regions) == 0 || result.Regions[0].Kind != "script" || result.Regions[0].Language != "typescript" {
				t.Fatalf("script region misclassified: %+v", result.Regions)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			got, ok := symbols["saveSetting"]
			if !ok || got.NameRange.Start.Line != 9 {
				t.Fatalf("missing method after quoted angle bracket in script attributes: %+v found=%v names=%v", got, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			if _, ghost := symbols["Id"]; ghost {
				t.Fatalf("generic type parameter reported as declaration: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}
