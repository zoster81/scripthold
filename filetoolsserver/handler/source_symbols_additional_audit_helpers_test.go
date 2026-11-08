package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type extraAuditEvidence struct {
	file, excerpt string
}

type extraAuditSymbol struct {
	file, name string
	line       int
}

type extraAuditScenario struct {
	project, language, search, primary, negativeFile, negativeQuery string
	paths                                                           []string
	oracles                                                         []extraAuditEvidence
	targets                                                         []extraAuditSymbol
	showAsWritten                                                   map[string]string
}

func checkAdditionalOriginalNavigation(t *testing.T, corpus string, sc extraAuditScenario) {
	t.Helper()
	t.Run(sc.project, func(t *testing.T) {
		root := filepath.Join(corpus, sc.project)
		paths := make([]string, 0, len(sc.paths))
		for _, rel := range sc.paths {
			p := filepath.Join(root, filepath.FromSlash(rel))
			requireAdditionalOriginalFile(t, p)
			paths = append(paths, p)
		}
		for _, oracle := range sc.oracles {
			p := filepath.Join(root, filepath.FromSlash(oracle.file))
			requireAdditionalOriginalFile(t, p)
			data, err := os.ReadFile(p)
			if err != nil || !strings.Contains(string(data), oracle.excerpt) {
				t.Fatalf("independent original oracle %s %q missing: %v", oracle.file, oracle.excerpt, err)
			}
		}
		h := NewHandler([]string{root})
		failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Paths: paths, Language: sc.language, Encoding: "utf-8",
			Presentation: "compact", MaxFiles: len(paths),
		})
		if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) ||
			!digest.CoverageComplete {
			t.Fatalf("compact digest incomplete err=%v failed=%+v result=%+v", err, failed, digest)
		}
		encoded, err := json.Marshal(digest)
		if err != nil || len(encoded) > 8192 {
			t.Fatalf("compact digest %d bytes, err=%v", len(encoded), err)
		}
		t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(encoded), digest.CoverageComplete)
		for _, target := range sc.targets {
			p := filepath.Join(root, filepath.FromSlash(target.file))
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{p}, Query: target.name, Match: "exact",
				Language: sc.language, Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 30,
			})
			if err != nil || failed != nil || found.Truncated || !found.CoverageComplete || len(found.Files) != 1 {
				t.Fatalf("find %s failed err=%v failed=%+v out=%+v", target.name, err, failed, found)
			}
			symbolID := ""
			for _, symbol := range found.Symbols {
				if symbol.Name == target.name && symbol.NameRange.Start.Line == target.line &&
					filepath.Clean(symbol.Path) == filepath.Clean(p) {
					symbolID = symbol.ID
					break
				}
			}
			if symbolID == "" || found.Files[0].SourceFingerprint == "" {
				t.Fatalf("missing independently located %s:%d %s: %+v", target.file, target.line, target.name, found.Symbols)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: p, SymbolID: symbolID,
				SourceFingerprint: found.Files[0].SourceFingerprint,
				Language:          sc.language, Encoding: "utf-8", MaxBytes: 16384,
			})
			wantText := sc.showAsWritten[target.name]
			if wantText == "" {
				wantText = target.name
			}
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, wantText) {
				t.Fatalf("fingerprint-bound show for %s expected %q err=%v failed=%+v output=%+v", target.name, wantText, err, failed, shown.Show)
			}
		}
		failedQuery, query, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Query: sc.search, Match: "exact",
			Mode: "structural", Language: sc.language, Encoding: "utf-8", MaxResults: 30,
		})
		if err != nil || failedQuery != nil || query.Search == nil {
			t.Fatalf("structural search %s failed err=%v tool=%+v out=%+v", sc.search, err, failedQuery, query)
		}
		wantPrimary := filepath.Join(root, filepath.FromSlash(sc.primary))
		primaryFound := false
		for _, hit := range query.Search.Matches {
			if hit.Name == sc.search && filepath.Clean(hit.Path) == filepath.Clean(wantPrimary) {
				primaryFound = true
			}
		}
		if !primaryFound {
			t.Fatalf("search did not identify primary declaration %s in %s: %+v", sc.search, sc.primary, query.Search.Matches)
		}
		failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find",
			Paths:     []string{filepath.Join(root, filepath.FromSlash(sc.negativeFile))},
			Query:     sc.negativeQuery, Match: "exact", Language: sc.language,
			Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 30,
		})
		if err != nil || failed != nil || negative.Truncated || !negative.CoverageComplete || len(negative.Symbols) != 0 {
			t.Fatalf("negative %s in unrelated %s failed err=%v failed=%+v symbols=%+v",
				sc.negativeQuery, sc.negativeFile, err, failed, negative.Symbols)
		}
	})
}

func requireAdditionalOriginalFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("original source not a regular file: %s err=%v", path, err)
	}
}
