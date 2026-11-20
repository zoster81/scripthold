package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Nix currently indexes a bounded let-binding subset. Unlike the common
// complete-scan original helper, this checks useful navigation while requiring
// an honest incomplete-coverage signal for result attribute sets.
func TestSourceSymbolsNixSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Nix project snapshots not configured")
	}
	for _, tc := range []struct {
		project, primary, distractor, target, excerpt, negative string
		paths                                                   []string
		oracles                                                 []extraAuditEvidence
		targets                                                 []extraAuditSymbol
	}{
		{
			project: "nix83-disko-audit-selected", primary: "lib/interactive-vm.nix",
			distractor: "lib/binfmt.nix", target: "rootDisk", excerpt: "rootDisk = {",
			negative: "rootDisk",
			paths:    []string{"lib/interactive-vm.nix", "lib/binfmt.nix"},
			oracles: []extraAuditEvidence{
				{"lib/interactive-vm.nix", "vm_disko = (diskoLib.testLib.prepareDiskoConfig"},
				{"lib/interactive-vm.nix", "otherDisks = map (disk: {"},
				{"lib/interactive-vm.nix", "virtualisation.qemu.drives = [ rootDisk ] ++ otherDisks;"},
				{"lib/binfmt.nix", "makeBinfmtLine ="},
				{"lib/binfmt.nix", "magics = {"},
			},
			targets: []extraAuditSymbol{
				{"lib/interactive-vm.nix", "vm_disko", 11},
				{"lib/interactive-vm.nix", "rootDisk", 29},
				{"lib/interactive-vm.nix", "otherDisks", 37},
				{"lib/binfmt.nix", "makeBinfmtLine", 11},
			},
		},
		{
			project: "nix83-flake-parts-audit-selected", primary: "modules/perSystem.nix",
			distractor: "modules/transposition.nix", target: "otherMemoizedSystems",
			excerpt:  "otherMemoizedSystems = flake-parts-lib.memoizeStr",
			negative: "otherMemoizedSystems",
			paths:    []string{"modules/perSystem.nix", "modules/transposition.nix"},
			oracles: []extraAuditEvidence{
				{"modules/perSystem.nix", "rootConfig = config;"},
				{"modules/perSystem.nix", "throwAliasError = param:"},
				{"modules/perSystem.nix", "otherMemoizedSystems = flake-parts-lib.memoizeStr config.perSystem;"},
				{"modules/transposition.nix", "transpositionModule = {"},
				{"modules/transposition.nix", "perInputAttributeError = { flake, attrName, system, attrConfig }:"},
			},
			targets: []extraAuditSymbol{
				{"modules/perSystem.nix", "rootConfig", 16},
				{"modules/perSystem.nix", "throwAliasError", 43},
				{"modules/perSystem.nix", "otherMemoizedSystems", 68},
				{"modules/transposition.nix", "transpositionModule", 14},
				{"modules/transposition.nix", "perInputAttributeError", 30},
			},
		},
	} {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.paths))
			for _, rel := range tc.paths {
				p := filepath.Join(root, filepath.FromSlash(rel))
				requireAdditionalOriginalFile(t, p)
				paths = append(paths, p)
			}
			for _, oracle := range tc.oracles {
				p := filepath.Join(root, filepath.FromSlash(oracle.file))
				data, err := os.ReadFile(p)
				if err != nil || !strings.Contains(string(data), oracle.excerpt) {
					t.Fatalf("original source oracle %s missing: %v", oracle.file, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "nix",
				Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.CoverageComplete || digest.FilesParsed != len(paths) {
				t.Fatalf("Nix digest did not report bounded partial coverage: err=%v tool=%+v out=%+v", err, failed, digest)
			}
			encoded, err := json.Marshal(digest)
			if err != nil || len(encoded) > 8192 {
				t.Fatalf("Nix digest output too large: %d err=%v", len(encoded), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(encoded), digest.CoverageComplete)
			for _, target := range tc.targets {
				p := filepath.Join(root, filepath.FromSlash(target.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{p}, Language: "nix",
					Encoding: "utf-8", Query: target.name, Match: "exact", MaxFiles: 1, MaxSymbols: 30,
				})
				if err != nil || failed != nil || found.Truncated || found.CoverageComplete || len(found.Files) != 1 {
					t.Fatalf("Nix find must stay partial: %s err=%v tool=%+v out=%+v", target.name, err, failed, found)
				}
				id := ""
				for _, symbol := range found.Symbols {
					if symbol.Name == target.name && symbol.NameRange.Start.Line == target.line && filepath.Clean(symbol.Path) == filepath.Clean(p) {
						id = symbol.ID
					}
				}
				if id == "" || found.Files[0].SourceFingerprint == "" {
					t.Fatalf("missing source-backed Nix let binding %s:%d %s: %+v", target.file, target.line, target.name, found.Symbols)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: p, Language: "nix", Encoding: "utf-8", SymbolID: id,
					SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.name) {
					t.Fatalf("fingerprint-bound show missing %s err=%v tool=%+v show=%+v", target.name, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: paths, Language: "nix",
				Encoding: "utf-8", Query: tc.target, Match: "exact", MaxResults: 30,
			})
			if err != nil || failed != nil || search.Search == nil || search.Coverage.CoverageComplete || search.Coverage.ResultsTruncated {
				t.Fatalf("Nix structural query must remain partial: %v %+v %+v", err, failed, search)
			}
			primary := filepath.Join(root, filepath.FromSlash(tc.primary))
			seen := false
			for _, m := range search.Search.Matches {
				if m.Name == tc.target && filepath.Clean(m.Path) == filepath.Clean(primary) {
					seen = true
				}
			}
			if !seen {
				t.Fatalf("primary Nix binding not navigable: %s %v", tc.target, search.Search.Matches)
			}
			p := filepath.Join(root, filepath.FromSlash(tc.distractor))
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{p}, Language: "nix", Encoding: "utf-8",
				Query: tc.negative, Match: "exact", MaxFiles: 1, MaxSymbols: 20,
			})
			if err != nil || failed != nil || negative.CoverageComplete || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated Nix source invented %s: err=%v tool=%+v hits=%+v", tc.negative, err, failed, negative.Symbols)
			}
		})
	}
}
