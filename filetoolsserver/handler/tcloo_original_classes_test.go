package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two independent original Git repositories exercise TCLOO class ownership,
// while unrelated Tcl procedures remain globally navigable. These sources
// are read, never built or executed.
func TestTclOOOriginalClassAndMemberNavigation(t *testing.T) {
	mal := os.Getenv("SCRIPTHOLD_REAL_TCL103_MAL_ROOT")
	ctags := os.Getenv("SCRIPTHOLD_REAL_TCLOO_CTAGS_ROOT")
	if mal == "" && ctags == "" {
		t.Skip("immutable original TclOO repositories not configured")
	}
	if mal == "" || ctags == "" {
		t.Fatal("both independent TclOO original source roots must be configured")
	}
	type target struct {
		name, kind, native, parent, excerpt string
		line                                int
	}
	cases := []struct {
		root, file, excluded string
		targets              []target
	}{
		{mal, "impls/tcl/env.tcl", "CreateShortcut", []target{
			{"Env", "class", "tcloo-class", "", "oo::class create Env {", 1},
			{"constructor", "constructor", "tcloo-constructor", "Env", "constructor {{outerenv 0}", 4},
			{"set", "method", "tcloo-method", "Env", "method set {symbol objval} {", 22},
			{"find", "method", "tcloo-method", "Env", "method find {symbol} {", 27},
			{"get", "method", "tcloo-method", "Env", "method get {symbol} {", 37},
			{"get_symbol", "method", "tcloo-method", "Env", "method get_symbol {symbol} {", 46},
		}},
		{mal, "impls/tcl/reader.tcl", "self", []target{
			{"Reader", "class", "tcloo-class", "", "oo::class create Reader {", 1},
			{"constructor", "constructor", "tcloo-constructor", "Reader", "constructor {tokens_list} {", 4},
			{"peek", "method", "tcloo-method", "Reader", "method peek {} {", 9},
			{"next", "method", "tcloo-method", "Reader", "method next {} {", 13},
			{"read_str", "function", "proc", "", "proc read_str str {", 120},
		}},
		{ctags, "Units/parser-tcloo.r/simple-tcloo.d/input.tcl", "ShouldNotBeCaptured", []target{
			{"X", "class", "tcloo-class", "", "oo::class create X {", 1},
			{"add", "method", "tcloo-method", "X", "method add v {", 4},
			{"doSomething", "method", "tcloo-method", "X", "method doSomething {v} {", 5},
		}},
		{ctags, "Units/parser-tcloo.r/no-empty-line-between-classes.d/input.tcl", "superclass", []target{
			{"S", "class", "tcloo-class", "", "oo::class create S {", 1},
			{"sx", "method", "tcloo-method", "S", "method sx {} {", 3},
			{"M", "class", "tcloo-class", "", "oo::class create M {", 7},
			{"mx", "method", "tcloo-method", "M", "method mx {} {", 9},
			{"C", "class", "tcloo-class", "", "oo::class create C {", 13},
			{"cx", "method", "tcloo-method", "C", "method cx {} {", 17},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(tc.root, filepath.FromSlash(tc.file))
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
				t.Fatalf("original TclOO source not a bounded regular file: info=%v err=%v", stat, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(raw), "\n")
			for _, wanted := range tc.targets {
				if wanted.line > len(lines) || wanted.line <= 0 || !strings.Contains(lines[wanted.line-1], wanted.excerpt) {
					t.Fatalf("source-first TclOO original oracle changed: %+v", wanted)
				}
			}
			ctx, h := context.Background(), NewHandler([]string{tc.root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "tcl", MaxFiles: 1,
			})
			if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("source digest unavailable: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "tcl", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("source outline unavailable: err=%v outline=%+v", err, outline)
			}
			indexed := make(map[string]bool)
			for _, item := range outline.Symbols {
				indexed[item.ID] = true
			}
			for _, wanted := range tc.targets {
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "tcl",
					Query: wanted.name, Match: "exact", MaxFiles: 1,
					MaxSymbols: 48, FileDetails: "relevant",
				})
				var selected string
				for _, item := range found.Symbols {
					if item.Name == wanted.name && string(item.Kind) == wanted.kind &&
						item.NativeKind == wanted.native && item.ParentQualifiedName == wanted.parent &&
						item.NameRange.Start.Line == wanted.line &&
						(wanted.parent == "" || item.ParentID != "") {
						selected = item.ID
					}
				}
				if err != nil || selected == "" || !indexed[selected] || len(found.Files) != 1 {
					t.Fatalf("TclOO declaration not indexed at authentic source location %+v: err=%v symbols=%+v", wanted, err, found.Symbols)
				}
				file := found.Files[0]
				if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
					t.Fatalf("source fingerprint mismatch: %+v", file)
				}
				if wanted.kind == "class" {
					continue // Full class source may exceed a bounded excerpt.
				}
				_, show, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "tcl",
					Encoding: file.Encoding, SymbolID: selected,
					SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
				})
				if err != nil || show.Show == nil || !strings.Contains(show.Show.Text, wanted.excerpt) {
					t.Fatalf("fingerprint-bound method excerpt absent: %v %+v", err, show.Show)
				}
				_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path},
					Language: "tcl", Query: wanted.name, Match: "exact",
					MaxFiles: 1, MaxResults: 48,
				})
				linked := false
				if query.Search != nil {
					for _, hit := range query.Search.Matches {
						if hit.SymbolID == selected && hit.SourceFingerprint == file.SourceFingerprint {
							linked = true
						}
					}
				}
				if err != nil || query.Search == nil || query.Coverage.ResultsTruncated || !linked {
					t.Fatalf("TclOO source_query not linked to source symbol %s: %v %+v", wanted.name, err, query)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "tcl",
				Query: tc.excluded, Match: "exact", MaxFiles: 1, MaxSymbols: 48,
			})
			if err != nil || len(negative.Symbols) != 0 {
				t.Fatalf("TclOO call/unimported class fabricated as declaration %s: %v %+v", tc.excluded, err, negative.Symbols)
			}
		})
	}
}
