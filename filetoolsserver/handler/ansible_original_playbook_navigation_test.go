package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Fixtures are byte-identical with independently tracked original playbooks.
// Related roles, configuration templates and distractors are checked in the
// source project; a task's name is not a claim of runtime Ansible behavior.
func TestAnsibleOriginalProjectTaskNavigation(t *testing.T) {
	cases := []struct {
		name, env, task, sibling, unrelated string
		line, siblingLine                   int
	}{
		{
			name: "HAProxy rolling update maintenance",
			env:  "SCRIPTHOLD_REAL_ANSIBLE_EXAMPLES",
			task: "disable the server in haproxy", line: 24,
			sibling: "enable the server in haproxy", siblingLine: 39,
			unrelated: "restart haproxy",
		},
		{
			name: "Nginx virtual host deployment maintenance",
			env:  "SCRIPTHOLD_REAL_ANSIBLE_DO_COMMUNITY",
			task: "Apply Nginx template", line: 21,
			sibling: "Restart Nginx", siblingLine: 35,
			unrelated: "Update apt cache and make sure Vim, Curl and Unzip are installed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_ANSIBLE_ROOT"), os.Getenv(tc.env)
			if root == "" || path == "" {
				t.Skip("Ansible original source fixtures not configured")
			}
			ctx := context.Background()
			h := NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "ansible-yaml", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Ansible original digest incomplete: err=%v result=%+v", err, digest)
			}
			_, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "ansible-yaml",
				MaxFiles: 1, MaxSymbols: 40, Presentation: "detailed",
			})
			if err != nil || !outlined.CoverageComplete || outlined.Truncated ||
				outlined.FilesParsed != 1 {
				t.Fatalf("Ansible original outline incomplete: err=%v result=%+v", err, outlined)
			}
			present := false
			for _, sym := range outlined.Symbols {
				if sym.Name == tc.task && sym.NameRange.Start.Line == tc.line {
					present = true
				}
			}
			if !present {
				t.Fatalf("Ansible maintenance task missing from original outline: %q", tc.task)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ansible-yaml",
				Query: tc.task, Match: "exact", MaxFiles: 1, MaxSymbols: 40,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("Ansible task find incomplete: err=%v result=%+v", err, found)
			}
			selected := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.task && sym.NameRange.Start.Line == tc.line &&
					string(sym.Kind) == "operation" {
					selected = sym.ID
				}
			}
			if selected == "" {
				t.Fatalf("original task %q unavailable as structural declaration: %+v", tc.task, found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatal("Ansible safe source selector unavailable")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "ansible-yaml",
				Encoding: file.Encoding, SymbolID: selected,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, "name: "+tc.task) {
				t.Fatalf("Ansible exact task name source not retrieved: err=%v show=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "ansible-yaml", Query: tc.task, Match: "exact",
				MaxFiles: 1, MaxResults: 30,
			})
			if err != nil || search.Search == nil || !search.Coverage.CoverageComplete ||
				search.Coverage.ResultsTruncated {
				t.Fatalf("Ansible task search incomplete: err=%v result=%+v", err, search)
			}
			linked := false
			for _, match := range search.Search.Matches {
				if match.Name == tc.task && match.SymbolID == selected &&
					match.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("Ansible original task %q missing from structural search", tc.task)
			}
			_, sibling, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ansible-yaml",
				Query: tc.sibling, Match: "exact", MaxFiles: 1, MaxSymbols: 40,
			})
			if err != nil || len(sibling.Symbols) != 1 ||
				sibling.Symbols[0].NameRange.Start.Line != tc.siblingLine {
				t.Fatalf("Ansible related task %s omitted: err=%v result=%+v",
					tc.sibling, err, sibling.Symbols)
			}
			_, unrelated, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ansible-yaml",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 40,
			})
			if err != nil || !unrelated.CoverageComplete || len(unrelated.Symbols) != 0 {
				t.Fatalf("Ansible neighbor task incorrectly promoted in different file: err=%v result=%+v",
					err, unrelated.Symbols)
			}
		})
	}
}
