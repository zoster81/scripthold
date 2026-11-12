package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Named import_playbook entries are neither play declarations nor runnable
// tasks. Keep genuine named plays, child tasks and root task-list items visible.
func TestAnsibleNamedPlaybookImportsDoNotInventTasks(t *testing.T) {
	const source = "- name: Common tasks for every playbooks\n" +
		"  import_playbook: boilerplate.yml\n" +
		"- name: Install etcd\n" +
		"  vars:\n" +
		"    etcd_cluster_setup: true\n" +
		"  import_playbook: install_etcd.yml\n" +
		"- name: Upgrade control plane\n" +
		"  hosts: kube_control_plane\n" +
		"  tasks:\n" +
		"    - name: Verify kubelet\n" +
		"      ansible.builtin.debug:\n" +
		"        msg: checking\n" +
		"- name: Root task-list entry\n" +
		"  ansible.builtin.debug:\n" +
		"    msg: standalone\n" +
		"- name: Genuine play with a nested variable\n" +
		"  hosts: all\n" +
		"  vars:\n" +
		"    import_playbook: template-value.yml\n"

	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(map[string]string{"\n": "lf", "\r\n": "crlf"}[eol], func(t *testing.T) {
			input := strings.ReplaceAll(source, "\n", eol)
			result, err := (AnsibleYAMLAnalyzer{}).Analyze(
				context.Background(), sourceDocumentForScanner(input), testAnalyzeOptions(true, 100))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("source not completely scanned: %+v", result.Analysis)
			}
			want := map[string]SymbolKind{
				"Upgrade control plane":                SymbolKindSection,
				"Upgrade control plane.Verify kubelet": SymbolKindOperation,
				"Root task-list entry":                 SymbolKindOperation,
				"Genuine play with a nested variable":  SymbolKindSection,
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			if len(got) != len(want) {
				t.Fatalf("named import falsely indexed: got=%v want=%v", sortedSymbolQualifiedNames(result.Analysis.Symbols), want)
			}
			for name, kind := range want {
				if sym, ok := got[name]; !ok || sym.Kind != kind {
					t.Errorf("symbol %q: got=%+v present=%v, want kind=%s", name, sym, ok, kind)
				}
			}
		})
	}
}
