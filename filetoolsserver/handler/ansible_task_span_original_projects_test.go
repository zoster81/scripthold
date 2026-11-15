package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These independent original playbooks are inspected, never executed. A task
// selector must reveal the actual action/guard and stop before its next peer.
func TestAnsibleOriginalTaskShowIncludesActionAndGuards(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original Ansible roots not configured")
	}
	tests := []struct {
		name, project, file, task, owner, excerpt, later, sibling string
		first, last                                               int
	}{
		{
			name:    "Geerlingguy Node.js test action",
			project: "ansible78-geerlingguy-audit-selected",
			file:    "deployments-rolling/playbooks/deploy.yml",
			task:    "Run Node.js API app tests.", first: 27, last: 29,
			excerpt: "command: \"npm test chdir={{ app_directory }}\"",
			later:   "when: app_updated.changed",
			sibling: "- name: Get list of all running Node.js apps.",
		},
		{
			name:    "Geerlingguy SSH security loop and notify",
			project: "ansible78-geerlingguy-audit-selected",
			file:    "security/main.yml",
			task:    "Update SSH configuration to be more secure.", first: 19, last: 33,
			excerpt: "line: \"Port 2849\"",
			later:   "notify: restart ssh",
			sibling: "- name: Add a deployment user.",
		},
		{
			name:    "Kubespray reset confirmation task",
			project: "ansible78-kubespray-audit-selected",
			file:    "playbooks/reset.yml",
			task:    "Reset Confirmation", owner: "Reset cluster", first: 12, last: 19,
			excerpt: "prompt: \"Are you sure you want to reset cluster state?",
			later:   "- reset_confirmation is not defined",
			sibling: "- name: Check confirmation",
		},
		{
			name:    "Kubespray service facts stop before play environment",
			project: "ansible78-kubespray-audit-selected",
			file:    "playbooks/reset.yml",
			task:    "Gather information about installed services", owner: "Reset cluster", first: 28, last: 29,
			excerpt: "service_facts:",
			later:   "service_facts:",
			sibling: "environment: \"{{ proxy_disable_env }}\"",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.file))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("unsafe or missing original %s: info=%v err=%v", tc.file, info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if tc.first < 1 || tc.last >= len(lines) ||
				!strings.Contains(lines[tc.first-1], "- name: "+tc.task) ||
				!strings.Contains(strings.Join(lines[tc.first-1:tc.last], "\n"), tc.excerpt) ||
				!strings.Contains(lines[tc.last-1], tc.later) {
				t.Fatalf("original task source oracle drift: %s lines %d-%d", tc.file, tc.first, tc.last)
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "ansible-yaml", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 || digest.Truncated {
				t.Fatalf("original task digest incomplete: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "ansible-yaml",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("original task outline incomplete: %v %+v", err, outline)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ansible-yaml",
				Query: tc.task, Match: "exact", FileDetails: "relevant", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original task find incomplete: %v %+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.task && sym.NameRange.Start.Line == tc.first && sym.ParentQualifiedName == tc.owner &&
					sym.DeclarationRange.End.Line == tc.last && string(sym.Kind) == "operation" {
					id = sym.ID
				}
			}
			if id == "" {
				t.Fatalf("exact original task span/ownership absent: %s:%d %+v", tc.file, tc.first, found.Symbols)
			}
			inOutline := false
			for _, sym := range outline.Symbols {
				if sym.ID == id {
					inOutline = true
				}
			}
			if !inOutline {
				t.Fatalf("task identity missing in detailed outline: %s:%d", tc.file, tc.first)
			}
			file := found.Files[0]
			if file.SourceFingerprint == "" || file.Encoding == "" {
				t.Fatalf("unsafe selector: %+v", file)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "ansible-yaml",
				Encoding: file.Encoding, SymbolID: id,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.excerpt) ||
				!strings.Contains(shown.Show.Text, tc.later) ||
				strings.Contains(shown.Show.Text, tc.sibling) {
				t.Fatalf("task show does not bound original authored action: %s:%d err=%v show=%+v",
					tc.file, tc.first, err, shown.Show)
			}
		})
	}
}
