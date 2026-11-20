package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestAnsibleYAMLTaskClassification(t *testing.T) {
	tests := []struct {
		name string
		text string
		want map[string]struct {
			kind   SymbolKind
			native string
		}
	}{
		{
			name: "standalone-tasks",
			text: "- name: Install dependencies\n  ansible.builtin.package:\n    name: example\n- name: Verify service\n  ansible.builtin.service:\n    name: example\n",
			want: map[string]struct {
				kind   SymbolKind
				native string
			}{
				"Install dependencies": {SymbolKindOperation, "task"},
				"Verify service":       {SymbolKindOperation, "task"},
			},
		},
		{
			name: "unnamed-play-and-pre-tasks",
			text: "- hosts: all\n  pre_tasks:\n    - name: Check prerequisites\n      ansible.builtin.debug:\n        msg: ready\n  tasks:\n    - name: Install packages\n      ansible.builtin.package:\n        name: example\n",
			want: map[string]struct {
				kind   SymbolKind
				native string
			}{
				"Check prerequisites": {SymbolKindOperation, "task"},
				"Install packages":    {SymbolKindOperation, "task"},
			},
		},
		{
			name: "named-play-with-owned-tasks",
			text: "- name: Configure web\n  hosts: web\n  tasks:\n    - name: Install nginx\n      ansible.builtin.package:\n        name: nginx\n- name: Cleanup\n  hosts: web\n  tasks:\n    - name: Restart service\n      ansible.builtin.service:\n        name: nginx\n",
			want: map[string]struct {
				kind   SymbolKind
				native string
			}{
				"Configure web":               {SymbolKindSection, "play"},
				"Configure web.Install nginx": {SymbolKindOperation, "task"},
				"Cleanup":                     {SymbolKindSection, "play"},
				"Cleanup.Restart service":     {SymbolKindOperation, "task"},
			},
		},
		{
			name: "indentless-task-sequence",
			text: "- name: First\n  tasks:\n  - name: taskOne\n    ansible.builtin.debug:\n- name: Second\n  hosts: all\n",
			want: map[string]struct {
				kind   SymbolKind
				native string
			}{
				"First":         {SymbolKindSection, "play"},
				"First.taskOne": {SymbolKindOperation, "task"},
				"Second":        {SymbolKindSection, "play"},
			},
		},
		{
			name: "variable-hosts-is-not-play",
			text: "- name: Configure settings\n  vars:\n    hosts: web\n  ansible.builtin.debug:\n    msg: ready\n",
			want: map[string]struct {
				kind   SymbolKind
				native string
			}{
				"Configure settings": {SymbolKindOperation, "task"},
			},
		},
		{
			name: "task-block-is-not-play",
			text: "- name: Run block\n  block:\n    - name: Nested task\n      ansible.builtin.debug:\n        msg: nested\n",
			want: map[string]struct {
				kind   SymbolKind
				native string
			}{
				"Run block":   {SymbolKindOperation, "task"},
				"Nested task": {SymbolKindOperation, "task"},
			},
		},
	}
	for _, tc := range tests {
		for _, eol := range []struct{ name, sequence string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"/"+eol.name, func(t *testing.T) {
				text := strings.ReplaceAll(tc.text, "\n", eol.sequence)
				document := sourceDocumentForScanner(text)
				document.Path = "fixture.yml"
				result, err := (AnsibleYAMLAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 256))
				if err != nil {
					t.Fatal(err)
				}
				if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
					t.Fatalf("incomplete analysis: %+v", result.Analysis)
				}
				actual := symbolsByQualifiedName(result.Analysis.Symbols)
				if len(actual) != len(tc.want) {
					t.Fatalf("unexpected symbols: got %v, want %v", sortedSymbolQualifiedNames(result.Analysis.Symbols), tc.want)
				}
				for name, want := range tc.want {
					symbol, ok := actual[name]
					if !ok || symbol.Kind != want.kind || symbol.NativeKind != want.native {
						t.Errorf("%s: got %+v (exists=%v), want kind=%s native=%s", name, symbol, ok, want.kind, want.native)
					}
				}
			})
		}
	}
}

func TestAnsibleYAMLQualifiedOriginalTaskClassification(t *testing.T) {
	tests := []struct {
		name, env, declaration string
		line                   int
		kind                   SymbolKind
		native                 string
	}{
		{"rhel7-task-list", "SCRIPTHOLD_REAL_ANSIBLE_RHEL7_PREREQS", "PREREQ | Check required packages installed | Python2", 3, SymbolKindOperation, "task"},
		{"terrible-unnamed-play", "SCRIPTHOLD_REAL_ANSIBLE_TERRIBLE_MAIN", "Check if we're running normally or limits are applied", 13, SymbolKindOperation, "task"},
		{"k3s-named-play", "SCRIPTHOLD_REAL_ANSIBLE_K3S_SITE", "Cluster prep", 2, SymbolKindSection, "play"},
		{"rhel8-named-play", "SCRIPTHOLD_REAL_ANSIBLE_RHEL8_CONVERGE", "Converge", 3, SymbolKindSection, "play"},
		{"ansible-core-unnamed-play", "SCRIPTHOLD_REAL_ANSIBLE_CORE_PLAYBOOK", "explicitly refresh inventory", 7, SymbolKindOperation, "task"},
		{"ansistrano-named-play", "SCRIPTHOLD_REAL_ANSIBLE_ANSISTRANO_DEPLOY", "Deploy example app to my-server.com", 2, SymbolKindSection, "play"},
		{"jboss-task-list", "SCRIPTHOLD_REAL_ANSIBLE_JBOSS_TASKS", "Download JBoss from jboss.org", 12, SymbolKindOperation, "task"},
		{"aap-nested-task", "SCRIPTHOLD_REAL_ANSIBLE_AAP_CONFIG", "Call dispatch role", 15, SymbolKindOperation, "task"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("set " + tc.env + " to the pinned original Ansible YAML source")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() > 1<<20 {
				t.Fatal("original Ansible YAML fixture exceeds 1 MiB")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			document := sourceDocumentForScanner(string(data))
			document.Path = path
			result, err := (AnsibleYAMLAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 4096))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("incomplete original analysis: %+v", result.Analysis)
			}
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Name != tc.declaration || symbol.NameRange.Start.Line != tc.line {
					continue
				}
				if symbol.Kind != tc.kind || symbol.NativeKind != tc.native {
					t.Fatalf("incorrect original declaration kind: %+v; want %s/%s", symbol, tc.kind, tc.native)
				}
				return
			}
			t.Fatalf("missing original declaration %q at line %d", tc.declaration, tc.line)
		})
	}
}
