package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Provider 78 uses two further original Ansible projects, not the earlier
// ansible-examples and DigitalOcean community examples. Original files were
// independently read in full and verified against immutable upstream blobs.
func TestSourceSymbolsAnsibleSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Ansible project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "ansible78-geerlingguy-audit-selected", language: "ansible-yaml",
			search:        "Update SSH configuration to be more secure.",
			primary:       "security/main.yml",
			negativeFile:  "deployments-rolling/playbooks/deploy.yml",
			negativeQuery: "Update SSH configuration to be more secure.",
			paths: []string{
				"security/main.yml",
				"deployments-rolling/playbooks/deploy.yml",
				"deployments-rolling/playbooks/provision.yml",
			},
			oracles: []extraAuditEvidence{
				{"security/main.yml", "Port 2849"},
				{"security/main.yml", "notify: restart ssh"},
				{"security/main.yml", "- name: Configure open ports with firewalld."},
				{"security/main.yml", "- name: Install fail2ban (RedHat)."},
				{"deployments-rolling/playbooks/deploy.yml", "when: app_updated.changed"},
				{"deployments-rolling/playbooks/deploy.yml", "notify: restart forever apps"},
				{"deployments-rolling/playbooks/provision.yml", "- geerlingguy.firewall"},
			},
			targets: []extraAuditSymbol{
				{"security/main.yml", "restart ssh", 6},
				{"security/main.yml", "Allow sshd to listen on tcp port 2849.", 11},
				{"security/main.yml", "Update SSH configuration to be more secure.", 19},
				{"security/main.yml", "Configure open ports with firewalld.", 103},
				{"security/main.yml", "Install fail2ban (RedHat).", 122},
				{"deployments-rolling/playbooks/deploy.yml", "Ensure Node.js API app is started.", 36},
				{"deployments-rolling/playbooks/provision.yml", "Update apt cache if needed.", 15},
			},
		},
		{
			project: "ansible78-kubespray-audit-selected", language: "ansible-yaml",
			search: "Patch Kubernetes for Windows", primary: "playbooks/upgrade_cluster.yml",
			negativeFile: "playbooks/reset.yml", negativeQuery: "Patch Kubernetes for Windows",
			paths: []string{
				"playbooks/upgrade_cluster.yml",
				"playbooks/reset.yml",
				"extra_playbooks/upgrade-only-k8s.yml",
			},
			oracles: []extraAuditEvidence{
				{"playbooks/upgrade_cluster.yml", "serial: 1"},
				{"playbooks/upgrade_cluster.yml", "role: win_nodes/kubernetes_patch"},
				{"playbooks/reset.yml", "prompt: \"Are you sure you want to reset cluster state?"},
				{"playbooks/reset.yml", "- { role: reset, tags: reset }"},
				{"extra_playbooks/upgrade-only-k8s.yml", "### * Will not upgrade etcd"},
				{"extra_playbooks/upgrade-only-k8s.yml", "role: kubernetes/control-plane"},
			},
			targets: []extraAuditSymbol{
				{"playbooks/upgrade_cluster.yml", "Prepare nodes for upgrade", 19},
				{"playbooks/upgrade_cluster.yml", "Handle upgrades to control plane components first to maintain backwards compat.", 46},
				{"playbooks/upgrade_cluster.yml", "Finally handle worker upgrades, based on given batch size", 80},
				{"playbooks/upgrade_cluster.yml", "Patch Kubernetes for Windows", 98},
				{"playbooks/reset.yml", "Reset cluster", 8},
				{"playbooks/reset.yml", "Reset Confirmation", 12},
				{"playbooks/reset.yml", "Check confirmation", 21},
				{"extra_playbooks/upgrade-only-k8s.yml", "Handle upgrades to control plane components first to maintain backwards compat.", 40},
				{"extra_playbooks/upgrade-only-k8s.yml", "Finally handle worker upgrades, based on given batch size", 53},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}

	// Kubespray labels some imported playbooks with - name. A source-navigation
	// operation must not fabricate executable tasks for those imports.
	root := filepath.Join(corpus, "ansible78-kubespray-audit-selected")
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file, name string
	}{
		{"playbooks/upgrade_cluster.yml", "Common tasks for every playbooks"},
		{"playbooks/upgrade_cluster.yml", "Gather facts"},
		{"playbooks/upgrade_cluster.yml", "Install etcd"},
		{"playbooks/reset.yml", "Common tasks for every playbooks"},
		{"playbooks/reset.yml", "Gather facts"},
	} {
		t.Run("named-import-not-task/"+tc.file+"/"+tc.name, func(t *testing.T) {
			path := filepath.Join(root, filepath.FromSlash(tc.file))
			requireAdditionalOriginalFile(t, path)
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ansible-yaml",
				Query: tc.name, Match: "exact", MaxFiles: 1, MaxSymbols: 30,
			})
			if err != nil || failed != nil || !found.CoverageComplete || found.Truncated || len(found.Symbols) != 0 {
				t.Fatalf("named import %q invented as a task: err=%v tool=%+v symbols=%+v", tc.name, err, failed, found.Symbols)
			}
		})
	}

	// The same legitimate upgrade play occurs in different original playbooks.
	// A multi-file query must return both locations rather than collapse them.
	want := map[string]int{
		filepath.Join(root, "playbooks", "upgrade_cluster.yml"):        46,
		filepath.Join(root, "extra_playbooks", "upgrade-only-k8s.yml"): 40,
	}
	paths := make([]string, 0, len(want))
	for file := range want {
		paths = append(paths, file)
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: paths, Language: "ansible-yaml",
		Query: "Handle upgrades to control plane components first to maintain backwards compat.",
		Match: "exact", MaxFiles: 2, MaxSymbols: 10,
	})
	if err != nil || failed != nil || !found.CoverageComplete || found.Truncated || len(found.Symbols) != 2 {
		t.Fatalf("distinct upgrade plays not preserved: err=%v tool=%+v symbols=%+v", err, failed, found.Symbols)
	}
	for _, sym := range found.Symbols {
		line, ok := want[filepath.Clean(sym.Path)]
		if !ok || sym.NameRange.Start.Line != line {
			t.Fatalf("wrong upgrade play identity or location: %+v", sym)
		}
	}
}
