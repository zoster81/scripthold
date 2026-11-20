package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Original-project maintenance oracles and unrelated controls were inspected
// independently of source_symbols and the installed connector.
func TestSourceSymbolsOriginalShellAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original shell corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "c-curl", language: "shell",
			search: "debian", primary: "scripts/release-tools.sh",
			negativeFile: "scripts/cmakelint.sh", negativeQuery: "debian",
			paths: []string{"scripts/release-tools.sh", "scripts/cmakelint.sh"},
			oracles: []extraAuditEvidence{
				{"scripts/release-tools.sh", "#!/bin/sh"},
				{"scripts/release-tools.sh", "timestamp=${1:-unknown}"},
				{"scripts/release-tools.sh", "tag=$(echo \"curl-$version\" | tr '.' '_')"},
				{"scripts/release-tools.sh", "if ! command -v dpkg >/dev/null; then"},
				{"scripts/release-tools.sh", "debian() {"},
				{"scripts/release-tools.sh", "dpkg -l \"$1\" | grep ^ii | awk '{print $3}'"},
				{"scripts/release-tools.sh", "debian autoconf"},
				{"scripts/release-tools.sh", "debian automake"},
				{"scripts/release-tools.sh", "debian libtool"},
				{"scripts/maketgz", "./scripts/release-tools.sh \"$timestamp\" \"$version\" \"$commit\" > docs/RELEASE-TOOLS.md.dist"},
				{"scripts/cmakelint.sh", "#!/bin/sh"},
				{"scripts/cmakelint.sh", "git ls-files '**CMakeLists.txt' '*.cmake'"},
				{"scripts/cmakelint.sh", "cmake-lint"},
				{"scripts/cmakeopts.sh", "#!/usr/bin/env bash"},
			},
			targets: []extraAuditSymbol{
				{"scripts/release-tools.sh", "debian", 51},
			},
		},
		{
			project: "shell-shellspec-shellspec", language: "shell",
			search: "count_lines", primary: "examples/count_lines.sh",
			negativeFile: "examples/lib.sh", negativeQuery: "count_lines",
			paths: []string{"examples/count_lines.sh", "examples/spec/22.sourcced_spec.sh", "examples/lib.sh"},
			oracles: []extraAuditEvidence{
				{"README.md", "for dash, bash, ksh, zsh and **all POSIX shells**"},
				{"examples/count_lines.sh", "#!/bin/sh"},
				{"examples/count_lines.sh", "count_lines() {"},
				{"examples/count_lines.sh", "while read -r line || [ \"$line\" ]; do"},
				{"examples/count_lines.sh", "i=$(($i + 1))"},
				{"examples/count_lines.sh", "${__SOURCED__:+false} : || return 0"},
				{"examples/spec/22.sourcced_spec.sh", "Include ./count_lines.sh"},
				{"examples/spec/22.sourcced_spec.sh", "When call count_lines"},
				{"examples/spec/22.sourcced_spec.sh", "%puts \"line5 (without newline)\""},
				{"examples/spec/22.sourcced_spec.sh", "The stdout should eq 5"},
				{"examples/lib.sh", "calc() {"},
				{"examples/lib.sh", "echo \"$(($*))\""},
			},
			targets: []extraAuditSymbol{
				{"examples/count_lines.sh", "count_lines", 6},
				{"examples/lib.sh", "calc", 3},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}

func TestSourceSymbolsOriginalShellMainOnlyBoundary(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original shell corpus unavailable")
	}
	for _, tc := range []struct {
		project, file string
	}{
		{"c-curl", "scripts/cmakelint.sh"},
		{"posix-shell", "cmd/shfmt/docker-entrypoint.sh"},
	} {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.file))
			requireAdditionalOriginalFile(t, path)
			h := NewHandler([]string{root})
			failed, got, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "shell",
				Encoding: "utf-8", Presentation: "compact", MaxFiles: 1,
			})
			if err != nil || failed != nil || got.Truncated || !got.CoverageComplete || got.FilesParsed != 1 {
				t.Fatalf("main-only shell digest failed err=%v tool=%+v result=%+v", err, failed, got)
			}
			if got.SymbolCount != 0 {
				t.Fatalf("main-only shell script unexpectedly reported %d declarations", got.SymbolCount)
			}
		})
	}
}
