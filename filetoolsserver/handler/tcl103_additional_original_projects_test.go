package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type tcl103Target struct {
	file, name, excerpt string
	line                int
}

// Two independent original Tcl projects exercise exact source navigation.
// TclOO classes and commands sourced by the MAL entrypoint are read as direct
// source evidence, not represented as invented source_query relationships.
func TestTcl103AdditionalOriginalProjectNavigation(t *testing.T) {
	mal := os.Getenv("SCRIPTHOLD_REAL_TCL103_MAL_ROOT")
	samples := os.Getenv("SCRIPTHOLD_REAL_TCL103_SAMPLES_ROOT")
	if mal == "" && samples == "" {
		t.Skip("additional independent original Tcl projects not configured")
	}
	if mal == "" || samples == "" {
		t.Fatal("both independent original Tcl project roots are required")
	}
	cases := []struct {
		name, root, subdir, negativeFile, negativeName string
		code, sources                                  []string
		evidence                                       []struct{ file, text string }
		targets                                        []tcl103Target
	}{
		{
			name: "MAL Tcl reader, core calls and evaluator navigation",
			root: mal, subdir: "impls/tcl",
			code:         []string{"env.tcl", "core.tcl", "reader.tcl", "stepA_mal.tcl"},
			sources:      []string{"env.tcl", "core.tcl", "reader.tcl", "stepA_mal.tcl"},
			negativeFile: "stepA_mal.tcl", negativeName: "binarySearch",
			evidence: []struct{ file, text string }{
				{"env.tcl", "oo::class create Env {"},
				{"env.tcl", "method find {symbol} {"},
				{"env.tcl", "return [$outer find $symbol]"},
				{"reader.tcl", "oo::class create Reader {"},
				{"reader.tcl", "proc read_str str {"},
				{"core.tcl", "set funcenv [Env new $env $binds $apply_args]"},
				{"stepA_mal.tcl", "source env.tcl"},
				{"stepA_mal.tcl", "source reader.tcl"},
				{"stepA_mal.tcl", "source core.tcl"},
				{"stepA_mal.tcl", "return [$env get $varname]"},
			},
			targets: []tcl103Target{
				{"reader.tcl", "read_form", "proc read_form {reader} {", 98},
				{"reader.tcl", "read_str", "proc read_str str {", 120},
				{"core.tcl", "mal_assoc", "proc mal_assoc {a} {", 175},
				{"core.tcl", "mal_apply", "proc mal_apply {a} {", 285},
				{"stepA_mal.tcl", "EVAL", "proc EVAL {ast env} {", 58},
				{"stepA_mal.tcl", "REP", "proc REP {str env} {", 213},
			},
		},
		{
			name: "Sample Programs independent Tcl search and graph examples",
			root: samples, subdir: "archive/t/tcl",
			code:         []string{"binary-search.tcl", "dijkstra.tcl", "depth-first-search.tcl"},
			sources:      []string{"binary-search.tcl", "dijkstra.tcl", "depth-first-search.tcl", "README.md"},
			negativeFile: "binary-search.tcl", negativeName: "createGraph",
			evidence: []struct{ file, text string }{
				{"binary-search.tcl", "if {![isSorted $numbers]} {"},
				{"binary-search.tcl", "puts [expr {[binarySearch $numbers $value] ? \"true\" : \"false\"}]"},
				{"dijkstra.tcl", "package require struct::graph::op"},
				{"dijkstra.tcl", "set distances [::struct::graph::op::dijkstra $g $src -outputformat distances]"},
				{"depth-first-search.tcl", "-command [list dfsCommand $target visited found]"},
				{"README.md", "Binary Search in Tcl"},
				{"README.md", "Depth First Search in Tcl"},
				{"README.md", "Dijkstra in Tcl"},
				{"README.md", "This page is auto-generated. DO NOT EDIT!"},
			},
			targets: []tcl103Target{
				{"binary-search.tcl", "isSorted", "proc isSorted {lst} {", 32},
				{"binary-search.tcl", "binarySearch", "proc binarySearch {lst value} {", 43},
				{"dijkstra.tcl", "createGraph", "proc createGraph {matrix vertices} {", 26},
				{"dijkstra.tcl", "main", "proc main {argv} {", 48},
				{"depth-first-search.tcl", "dfsCommand", "proc dfsCommand {target visitedRef foundRef args} {", 41},
				{"depth-first-search.tcl", "dfs", "proc dfs {graph start target} {", 59},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := make(map[string]string, len(tc.sources))
			for _, name := range tc.sources {
				path := filepath.Join(tc.root, filepath.FromSlash(tc.subdir), name)
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("unavailable or unbounded original Tcl source %s: info=%v err=%v", name, info, err)
				}
				bytes, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				source[name] = string(bytes)
			}
			for _, edge := range tc.evidence {
				if !strings.Contains(source[edge.file], edge.text) {
					t.Fatalf("independent Tcl source evidence missing %s: %q", edge.file, edge.text)
				}
			}
			for _, target := range tc.targets {
				lines := strings.Split(source[target.file], "\n")
				if target.line < 1 || target.line > len(lines) ||
					!strings.Contains(lines[target.line-1], target.excerpt) {
					t.Fatalf("original Tcl declaration changed at %s:%d %s", target.file, target.line, target.name)
				}
			}
			h, ctx := NewHandler([]string{tc.root}), context.Background()
			outlineIDs := make(map[string]map[string]bool)
			digests := make(map[string]string)
			for _, name := range tc.code {
				path := filepath.Join(tc.root, filepath.FromSlash(tc.subdir), name)
				_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "digest", Paths: []string{path}, Language: "tcl", MaxFiles: 1,
				})
				if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
					len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
					t.Fatalf("Tcl original digest failed %s: err=%v result=%+v", name, err, digest)
				}
				digests[name] = digest.Digests[0].SourceFingerprint
				_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "outline", Presentation: "detailed", Paths: []string{path},
					Language: "tcl", MaxFiles: 1, MaxSymbols: 256,
				})
				if err != nil || outline.Truncated || outline.FilesParsed != 1 || len(outline.Files) != 1 {
					t.Fatalf("Tcl original outline failed %s: err=%v result=%+v", name, err, outline)
				}
				ids := make(map[string]bool, len(outline.Symbols))
				for _, symbol := range outline.Symbols {
					ids[symbol.ID] = true
				}
				outlineIDs[name] = ids
			}
			for _, target := range tc.targets {
				path := filepath.Join(tc.root, filepath.FromSlash(tc.subdir), target.file)
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "tcl",
					Query: target.name, Match: "exact", MaxFiles: 1,
					MaxSymbols: 128, FileDetails: "relevant",
				})
				if err != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("Tcl find failed %s:%d: err=%v found=%+v", target.file, target.line, err, found)
				}
				id := ""
				for _, symbol := range found.Symbols {
					if symbol.Path == path && symbol.Name == target.name &&
						symbol.NativeKind == "proc" && symbol.ParentQualifiedName == "" &&
						symbol.NameRange.Start.Line == target.line {
						id = symbol.ID
					}
				}
				if id == "" || !outlineIDs[target.file][id] {
					t.Fatalf("Tcl source target missing or incorrectly owned %s:%d %s: %+v", target.file, target.line, target.name, found.Symbols)
				}
				file := found.Files[0]
				if file.Encoding == "" || file.SourceFingerprint != digests[target.file] {
					t.Fatalf("original Tcl source identity mismatch: %+v", file)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "tcl", Encoding: file.Encoding,
					SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 16384,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.excerpt) {
					t.Fatalf("Tcl fingerprint-bound excerpt missing %s:%d: err=%v show=%+v", target.file, target.line, err, shown.Show)
				}
				_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path},
					Language: "tcl", Query: target.name, Match: "exact", MaxFiles: 1, MaxResults: 128,
				})
				linked := false
				if search.Search != nil {
					for _, hit := range search.Search.Matches {
						if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
							hit.Range.Start.Line == target.line {
							linked = true
						}
					}
				}
				if err != nil || search.Search == nil || search.Coverage.ResultsTruncated || !linked {
					t.Fatalf("Tcl structural search not linked to source %s:%d: err=%v search=%+v", target.file, target.line, err, search)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(tc.root, filepath.FromSlash(tc.subdir), tc.negativeFile)},
				Language: "tcl", Query: tc.negativeName, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("fabricated cross-project Tcl procedure %s: err=%v symbols=%+v", tc.negativeName, err, negative.Symbols)
			}
			t.Logf("original Tcl maintenance navigation: %d selected files, %d positive declarations", len(tc.sources), len(tc.targets))
		})
	}
}
