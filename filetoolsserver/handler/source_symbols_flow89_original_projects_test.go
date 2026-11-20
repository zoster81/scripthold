package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two additional independent pinned original projects, never executed.
// Paths are opt-in because the source snapshots are private, not test fixtures.
func TestFlow89AdditionalIndependentOriginalProjectNavigation(t *testing.T) {
	metroRoot := os.Getenv("SCRIPTHOLD_REAL_FLOW89_METRO_ROOT")
	jestRoot := os.Getenv("SCRIPTHOLD_REAL_FLOW89_JEST_ROOT")
	if metroRoot == "" && jestRoot == "" {
		t.Skip("two additional original Flow projects not configured")
	}
	if metroRoot == "" || jestRoot == "" {
		t.Fatal("both independently pinned Flow projects must be configured together")
	}
	cases := []struct {
		name, root, primary, neighbor, target, owner, anchor, related, negative string
		primaryLine, neighborLine                                               int
		paths                                                                   []string
	}{
		{
			name: "Metro EventEmitter typed on-listener contract",
			root: metroRoot, primary: "eventemitter3.js.flow", neighbor: "accepts.js",
			target: "EventEmitter", primaryLine: 6,
			owner: "", anchor: "declare class EventEmitter", related: "ListenerFn", negative: "IncomingMessage",
			neighborLine: 12,
			paths:        []string{"eventemitter3.js.flow", "accepts.js"},
		},
		{
			name: "Jest React Native Intro text versus app registration",
			root: jestRoot, primary: "Intro.js", neighbor: "index.js",
			target: "Intro", primaryLine: 30,
			owner: "", anchor: "Welcome to React Native!", related: "App", negative: "AppRegistry",
			neighborLine: 13,
			paths:        []string{"Intro.js", "index.js"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			paths := make([]string, 0, len(tc.paths))
			files := map[string]string{}
			for _, name := range tc.paths {
				path := filepath.Join(tc.root, name)
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("original source absent, nonregular or oversized: %s info=%+v err=%v", name, info, err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				files[name] = string(raw)
				paths = append(paths, path)
			}
			relatedSource := files[tc.neighbor]
			if tc.target == "EventEmitter" {
				relatedSource = files[tc.primary]
			}
			if !strings.Contains(files[tc.primary], tc.anchor) ||
				!strings.Contains(relatedSource, tc.related) ||
				!strings.Contains(files[tc.neighbor], tc.negative) {
				t.Fatalf("original source evidence changed or unrelated file oracle invalid: %s", tc.name)
			}
			ctx := context.Background()
			h := NewHandler([]string{tc.root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Presentation: "compact", Paths: paths,
				Language: "flow", MaxFiles: len(paths),
			})
			if err != nil || digest.FilesParsed != len(paths) || digest.Truncated ||
				len(digest.OverviewFiles) != len(paths) {
				t.Fatalf("bounded digest failed: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: paths,
				Language: "flow", MaxFiles: len(paths), MaxSymbols: 128,
			})
			if err != nil || outline.FilesParsed != len(paths) || outline.Truncated {
				t.Fatalf("original outline failed: err=%v outline=%+v", err, outline)
			}
			targetPath := filepath.Join(tc.root, tc.primary)
			neighborPath := filepath.Join(tc.root, tc.neighbor)
			seen, adjacent := 0, false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.Path == targetPath && sym.NameRange.Start.Line == tc.primaryLine {
					seen++
				}
				if sym.Name == tc.related &&
					((tc.name == "Metro EventEmitter typed on-listener contract" && sym.Path == targetPath && sym.NameRange.Start.Line == 5) ||
						(tc.name != "Metro EventEmitter typed on-listener contract" && sym.Path == neighborPath && sym.NameRange.Start.Line == tc.neighborLine)) {
					adjacent = true
				}
				if sym.Name == tc.negative {
					t.Errorf("external type/call fabricated as declaration: %+v", sym)
				}
			}
			if seen != 1 || !adjacent {
				t.Fatalf("source-driven target or adjacent declaration missing (target=%d adjacent=%v coverage=%v): %+v", seen, adjacent, outline.CoverageComplete, outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: paths, Language: "flow",
				Query: tc.target, Match: "exact", MaxFiles: len(paths),
				MaxSymbols: 32, FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("exact source edit target missing or unsafe: err=%v found=%+v", err, found)
			}
			// An explicitly declared constructor and its class can share a
			// search name. Preserve the legitimate ambiguity and select by kind.
			expectedHits := 1
			if tc.target == "EventEmitter" {
				expectedHits = 2 // class and declared static constructor
			}
			if len(found.Symbols) != expectedHits || found.Ambiguous != (expectedHits > 1) {
				t.Fatalf("real same-name declaration ambiguity lost: found=%+v", found.Symbols)
			}
			var symID string
			for _, candidate := range found.Symbols {
				if candidate.Name == tc.target && candidate.Path == targetPath &&
					candidate.NameRange.Start.Line == tc.primaryLine && string(candidate.Kind) == "class" {
					if symID != "" {
						t.Fatal("duplicate real class identity in exact result")
					}
					symID = candidate.ID
				}
			}
			sym, file := found.Symbols[0], found.Files[0]
			for _, candidate := range found.Symbols {
				if candidate.ID == symID {
					sym = candidate
					break
				}
			}
			if symID == "" || sym.Path != targetPath || sym.NameRange.Start.Line != tc.primaryLine ||
				sym.ID != symID || file.Encoding == "" || file.SourceFingerprint == "" {
				t.Fatalf("unsafe source target identity: symbol=%+v file=%+v", sym, file)
			}
			matchedFingerprint := false
			for _, f := range digest.OverviewFiles {
				if f.Path == targetPath && f.SourceFingerprint == file.SourceFingerprint {
					matchedFingerprint = true
				}
			}
			if !matchedFingerprint {
				t.Fatal("first-pass and targeted source fingerprints differ")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: targetPath, Language: "flow",
				SymbolID: sym.ID, SourceFingerprint: file.SourceFingerprint,
				Encoding: file.Encoding, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.anchor) {
				t.Fatalf("fingerprint-bound original excerpt absent: err=%v show=%+v", err, shown.Show)
			}
			_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Query: tc.target, Match: "exact",
				Paths: paths, Language: "flow", MaxFiles: len(paths), MaxResults: 32,
			})
			linked := 0
			if searched.Search != nil {
				for _, match := range searched.Search.Matches {
					if match.SymbolID == sym.ID && match.SourceFingerprint == file.SourceFingerprint {
						linked++
					}
				}
			}
			if err != nil || searched.Search == nil || searched.Coverage.ResultsTruncated ||
				len(searched.Search.Matches) != expectedHits || linked != 1 {
				t.Fatalf("structural original search disagrees: err=%v result=%+v", err, searched)
			}
			_, neg, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{neighborPath}, Language: "flow",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || len(neg.Symbols) != 0 {
				t.Fatalf("unrelated file provides false edit target: err=%v found=%+v", err, neg.Symbols)
			}
			t.Logf("original project navigation PASS on %d selected files, indexed=%d coverageComplete=%v", len(paths), len(outline.Symbols), outline.CoverageComplete)
		})
	}
}
