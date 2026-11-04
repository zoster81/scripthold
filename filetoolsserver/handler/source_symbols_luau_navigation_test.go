package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original Luau projects are inspected read-only; no third-party code is run.
func TestSourceSymbolsOriginalLuauNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Luau corpus not configured")
	}
	for _, tc := range []struct {
		project, primary, query, other, partial string
		line                                    int
		files, evidence                         []string
	}{
		{"luau-evaera-cmdr", "Cmdr/CmdrClient/Shared/Dispatcher.luau", "Dispatcher:RunHooks", "Cmdr/BuiltInCommands/Utility/runif.luau", "Cmdr/BuiltInCommands/Utility/runif.luau", 260,
			[]string{"Cmdr/CmdrClient/Shared/Dispatcher.luau", "Cmdr/CmdrClient/Shared/Command.luau", "Cmdr/CmdrClient/Shared/Registry.luau", "Cmdr/BuiltInCommands/Utility/runif.luau"},
			[]string{"function Dispatcher:RunHooks", "self.Dispatcher:RunHooks(\"BeforeRun\", self)", "function Registry:RegisterHook", "context.Dispatcher:EvaluateAndRun("}},
		{"luau-rbxutil", "modules/option/init.luau", "Option:AndThen", "modules/concur/init.luau", "", 347,
			[]string{"modules/option/init.luau", "modules/option/init.test.luau", "modules/concur/init.luau"},
			[]string{"function Option:UnwrapOrElse(defaultFn)", "function Option:AndThen(andThenFn)", "should throw if an option is not returned from predicate", "function Concur:Await(timeout: number?)"}},
	} {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			h := NewHandler([]string{root})
			paths := []string{}
			all := ""
			for _, rel := range tc.files {
				p := filepath.Join(root, filepath.FromSlash(rel))
				fi, err := os.Lstat(p)
				if err != nil || !fi.Mode().IsRegular() {
					t.Fatalf("missing original %s: %v", rel, err)
				}
				raw, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				all += string(raw)
				paths = append(paths, p)
			}
			for _, e := range tc.evidence {
				if !strings.Contains(all, e) {
					t.Fatalf("original evidence changed: %q", e)
				}
			}
			failed, d, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "digest", Paths: paths, Language: "luau", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths)})
			if err != nil || failed != nil || d.Truncated || d.FilesParsed != len(paths) || len(d.OverviewFiles) != len(paths) {
				t.Fatalf("digest: %v %+v %+v", err, failed, d)
			}
			partialPath := ""
			if tc.partial != "" {
				partialPath = filepath.Join(root, filepath.FromSlash(tc.partial))
			}
			if d.CoverageComplete != (partialPath == "") {
				t.Fatalf("unexpected aggregate coverage: complete=%v expectedPartial=%q", d.CoverageComplete, tc.partial)
			}
			for _, file := range d.OverviewFiles {
				if file.Status != "parsed" || file.CoverageComplete != (file.Path != partialPath) {
					t.Fatalf("unexpected file coverage %s status=%s complete=%v", file.Path, file.Status, file.CoverageComplete)
				}
			}
			t.Logf("files=%d symbols=%d reportedComplete=%v expectedPartial=%q", d.FilesParsed, d.SymbolCount, d.CoverageComplete, tc.partial)
			p := filepath.Join(root, filepath.FromSlash(tc.primary))
			failed, f, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "find", Paths: []string{p}, Language: "luau", Encoding: "utf-8", Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 10})
			if err != nil || failed != nil || len(f.Files) != 1 {
				t.Fatalf("find: %v %+v", err, failed)
			}
			id := ""
			for _, s := range f.Symbols {
				if s.Name == tc.query && s.DeclarationRange.Start.Line == tc.line {
					id = s.ID
					break
				}
			}
			if id == "" {
				t.Fatalf("missing %s at %d: %+v", tc.query, tc.line, f.Symbols)
			}
			failed, show, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "show", Path: p, SymbolID: id, SourceFingerprint: f.Files[0].SourceFingerprint, Language: "luau", Encoding: "utf-8", MaxBytes: 16384})
			if err != nil || failed != nil || show.Show == nil || !strings.Contains(show.Show.Text, tc.query) {
				t.Fatalf("verified declaration: %v %+v", err, failed)
			}
			failed, q, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{Operation: "search", Paths: paths, Language: "luau", Encoding: "utf-8", Query: tc.query, Match: "exact", Mode: "structural", MaxResults: 10})
			if err != nil || failed != nil || q.Search == nil || len(q.Search.Matches) == 0 {
				t.Fatalf("structural search: %v %+v", err, failed)
			}
			failed, x, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.other))}, Language: "luau", Encoding: "utf-8", Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 10})
			if err != nil || failed != nil || len(x.Symbols) != 0 {
				t.Fatalf("unrelated false target: %v %+v", err, x.Symbols)
			}
		})
	}
}
