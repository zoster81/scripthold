package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These task oracles come from separately inspected original project sources.
// The third-party projects are read-only and never executed.
func TestSourceSymbolsOriginalMATLABOctaveTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original MATLAB/Octave corpus not configured")
	}
	type target struct {
		file, name, kind string
		line             int
	}
	cases := []struct {
		project, language, query, unrelated string
		files, evidence                     []string
		targets                             []target
	}{
		{
			project: "matlab-robotics-toolbox", language: "matlab",
			files: []string{"@SerialLink/ikine.m", "@SerialLink/ikcon.m", "@SerialLink/fkine.m"},
			evidence: []string{"opt.ilimit = 500;", "opt.rlimit = 100;",
				"if iterations > opt.ilimit", "if rejcount > opt.rlimit",
				"[q_t, err_t, ef_t, out] = fmincon(problem);",
				"function [t,allt] = fkine(robot, q, varargin)"},
			targets: []target{
				{"@SerialLink/ikine.m", "ikine", "function", 108},
				{"@SerialLink/ikcon.m", "ikcon", "function", 71},
				{"@SerialLink/fkine.m", "fkine", "function", 48},
			},
			query: "ikine", unrelated: "@SerialLink/fkine.m",
		},
		{
			project: "matlab-platemo", language: "matlab",
			files: []string{"PlatEMO/platemo.m", "PlatEMO/Algorithms/ALGORITHM.m", "PlatEMO/Problems/PROBLEM.m"},
			evidence: []string{"function [name,Setting] = getSetting(Setting,Pro)",
				"Pro.maxFE<=1000&Pro.D<=10", "function Solve(obj,Problem)",
				"nofinish = obj.pro.FE < obj.pro.maxFE;",
				"maxFE      = 10000;"},
			targets: []target{
				{"PlatEMO/platemo.m", "getSetting", "function", 112},
				{"PlatEMO/Algorithms/ALGORITHM.m", "Solve", "method", 64},
				{"PlatEMO/platemo.m", "platemo", "function", 1},
			},
			query: "getSetting", unrelated: "PlatEMO/Problems/PROBLEM.m",
		},
		{
			project: "octave-control", language: "octave",
			files: []string{"inst/@lti/feedback.m", "inst/@lti/connect.m", "inst/@lti/minreal.m"},
			evidence: []string{"function sys = feedback (sys1, sys2, feedin, feedout, fbsign = -1)",
				"function fbsign = __check_fbsign__ (fbsign)",
				"fbsign = sign (fbsign);", "sys = __sys_connect__ (sys, M);",
				"%!assert (M, Me, 1e-4);", "function sys = minreal (sys, tol = \"def\")"},
			targets: []target{
				{"inst/@lti/feedback.m", "__check_fbsign__", "function", 188},
				{"inst/@lti/feedback.m", "feedback", "function", 72},
				{"inst/@lti/minreal.m", "minreal", "function", 29},
			},
			query: "__check_fbsign__", unrelated: "inst/@lti/minreal.m",
		},
		{
			project: "octave-signal", language: "octave",
			files: []string{"inst/butter.m", "inst/buttord.m", "inst/cheby1.m"},
			evidence: []string{"function [a, b, c, d] = butter (n, wc, varargin)",
				"butter: all elements of Wc must be in the range [0,1]",
				"butter: all elements of Wc must be in the range [0,inf]",
				"%!error [a, b] = butter (3, .2, \"invalid\")",
				"validate_filter_bands (\"buttord\", Wp, Ws",
				"function [a, b, c, d] = cheby1 (n, rp, w, varargin)"},
			targets: []target{
				{"inst/butter.m", "butter", "function", 91},
				{"inst/buttord.m", "buttord", "function", 65},
				{"inst/cheby1.m", "cheby1", "function", 56},
			},
			query: "butter", unrelated: "inst/cheby1.m",
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			var original strings.Builder
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original source %s unavailable: %v", rel, err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				original.Write(data)
				paths = append(paths, path)
			}
			for _, fragment := range tc.evidence {
				if !strings.Contains(original.String(), fragment) {
					t.Fatalf("independent original-source oracle changed: %q", fragment)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: tc.language,
				Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || !digest.CoverageComplete ||
				digest.FilesParsed != len(paths) {
				t.Fatalf("compact map: %v failure=%+v result=%+v", err, failed, digest)
			}
			compact, err := json.Marshal(digest)
			if err != nil || len(compact) > 8192 {
				t.Fatalf("unbounded compact map bytes=%d err=%v", len(compact), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d reportedComplete=%v",
				digest.FilesParsed, digest.SymbolCount, len(compact), digest.CoverageComplete)
			for _, want := range tc.targets {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: want.name, Match: "exact",
					Language: tc.language, Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 10,
				})
				if err != nil || failed != nil || found.Truncated || !found.CoverageComplete || len(found.Files) != 1 {
					t.Fatalf("find %s: %v failure=%+v result=%+v", want.name, err, failed, found)
				}
				id := ""
				for _, sym := range found.Symbols {
					if sym.Name == want.name && string(sym.Kind) == want.kind &&
						sym.DeclarationRange.Start.Line == want.line {
						id = sym.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing original %s %s at %s:%d: %+v",
						want.kind, want.name, want.file, want.line, found.Symbols)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: id,
					SourceFingerprint: found.Files[0].SourceFingerprint,
					Language:          tc.language, Encoding: "utf-8", MaxBytes: 16384,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, want.name) {
					t.Fatalf("fingerprint-bound source excerpt %s failed: %v failure=%+v", want.name, err, failed)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Language: tc.language,
				Encoding: "utf-8", Query: tc.query, Match: "exact",
				Mode: "structural", MaxResults: 20,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("structural query %s failed: %v failure=%+v", tc.query, err, failed)
			}
			failed, other, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find",
				Paths:     []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
				Language:  tc.language, Encoding: "utf-8",
				Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 5,
			})
			if err != nil || failed != nil || other.Truncated || len(other.Symbols) != 0 {
				t.Fatalf("unrelated file reports %s: %v failure=%+v symbols=%+v",
					tc.query, err, failed, other.Symbols)
			}
		})
	}
}
