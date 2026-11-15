package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// A task's executable action and conditional settings are part of its authored
// declaration. A name-only source excerpt is not a safe maintenance target.
func TestAnsibleTaskDeclarationRetainsActionAndGuards(t *testing.T) {
	const source = "- name: Deploy nodejs\n" +
		"  hosts: nodejs-api\n" +
		"  tasks:\n" +
		"    - name: Run Node.js API app tests.\n" +
		"      command: \"npm test chdir={{ app_directory }}\"\n" +
		"      when: app_updated.changed\n" +
		"    - name: Get list of all running Node.js apps.\n" +
		"      command: forever list\n" +
		"      register: forever_list\n" +
		"  handlers:\n" +
		"    - name: restart forever apps\n" +
		"      command: \"forever restartall\"\n"

	for _, eol := range []struct{ name, value string }{
		{"LF", "\n"}, {"CRLF", "\r\n"},
	} {
		t.Run(eol.name, func(t *testing.T) {
			result, err := (AnsibleYAMLAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol.value)), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("task excerpt unavailable: %+v", result.Analysis)
			}
			want := map[string]struct {
				startLine, endLine int
				required, excluded string
			}{
				"Deploy nodejs.Run Node.js API app tests.":            {4, 6, "when: app_updated.changed", "Get list of all running"},
				"Deploy nodejs.Get list of all running Node.js apps.": {7, 9, "register: forever_list", "handlers:"},
				"Deploy nodejs.restart forever apps":                  {11, 12, "command: \"forever restartall\"", "Get list of all running"},
			}
			seen := make(map[string]bool)
			for _, symbol := range result.Analysis.Symbols {
				tc, ok := want[symbol.QualifiedName]
				if !ok {
					continue
				}
				seen[symbol.QualifiedName] = true
				if symbol.Kind != SymbolKindOperation || symbol.DeclarationRange.Start.Line != tc.startLine ||
					symbol.DeclarationRange.End.Line != tc.endLine ||
					!strings.Contains(symbol.Signature, tc.required) || strings.Contains(symbol.Signature, tc.excluded) {
					t.Errorf("task declaration is not exact: %+v want %+v", symbol, tc)
				}
			}
			for name := range want {
				if !seen[name] {
					t.Errorf("expected task %q missing: %+v", name, result.Analysis.Symbols)
				}
			}
		})
	}
}
