package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original flake-parts indented strings document example code, not live let declarations.
func TestNixIndentedStringOriginalNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original independent Nix source projects not configured")
	}
	cases := []struct {
		project, file, target, negative, oracle string
		line, negativeA, negativeB              int
	}{
		{"nix83-flake-parts-audit-selected", "modules/perSystem.nix", "rootConfig", "perSystem", "          perSystem = { config, ${param}', ... }: {", 16, 33, 53},
		{"nix83-disko-audit-selected", "lib/interactive-vm.nix", "rootDisk", "", "  virtualisation.qemu.drives = [ rootDisk ] ++ otherDisks;", 29, 0, 0},
	}
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.file))
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
				t.Fatalf("unsafe original file: info=%v err=%v", stat, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(data), "\n")
			if tc.project == "nix83-flake-parts-audit-selected" {
				if len(lines) < 68 || !strings.Contains(lines[32], tc.oracle) || !strings.Contains(lines[52], "          perSystem = { config, ... }: {") || !strings.Contains(lines[67], "otherMemoizedSystems =") {
					t.Fatal("original flake-parts source oracle mismatch")
				}
			} else if len(lines) < 71 || !strings.Contains(lines[70], tc.oracle) || !strings.Contains(lines[28], "rootDisk = {") {
				t.Fatal("original disko source oracle mismatch")
			}
			h := NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "digest", Paths: []string{path}, Language: "nix", MaxFiles: 1})
			if err != nil || digest.Truncated || digest.CoverageComplete || digest.FilesParsed != 1 || len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original Nix partial digest invalid: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "nix", MaxFiles: 1, MaxSymbols: 128})
			if err != nil || outline.Truncated || outline.CoverageComplete || outline.FilesParsed != 1 {
				t.Fatalf("Nix bounded partial outline invalid: %v %+v", err, outline)
			}
			for _, s := range outline.Symbols {
				if s.Name == tc.negative && tc.negative != "" && (s.NameRange.Start.Line == tc.negativeA || s.NameRange.Start.Line == tc.negativeB) {
					t.Fatalf("Nix indented-string example indexed as live binding at line %d: %+v", s.NameRange.Start.Line, s)
				}
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "nix", Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64, FileDetails: "relevant"})
			id := ""
			for _, s := range found.Symbols {
				if s.Name == tc.target && s.NameRange.Start.Line == tc.line {
					id = s.ID
				}
			}
			if err != nil || id == "" || len(found.Files) != 1 || found.Files[0].SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("original live let binding missing: %v %+v", err, found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "nix", Encoding: found.Files[0].Encoding, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 4096})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.target+" =") {
				t.Fatalf("Nix original live binding excerpt missing: %v %+v", err, shown.Show)
			}
		})
	}
}

func TestNixOriginalDirectLetBindingOwnership(t *testing.T) {
	cases := []struct {
		name, env, file, target, sibling, excluded, marker string
		targetLine, siblingLine, excludedLine              int
	}{
		{
			name:   "flake-parts transposition helper",
			env:    "SCRIPTHOLD_REAL_NIX_DIRECT_FLAKE_ROOT",
			file:   filepath.Join("modules", "transposition.nix"),
			target: "perInputAttributeError", targetLine: 30,
			sibling: "transpositionModule", siblingLine: 14,
			excluded: "attrPath", excludedLine: 41,
			marker: "perInputAttributeError = { flake, attrName, system, attrConfig }:",
		},
		{
			name:   "Disko interactive VM disk helpers",
			env:    "SCRIPTHOLD_REAL_NIX_DIRECT_DISKO_ROOT",
			file:   filepath.Join("lib", "interactive-vm.nix"),
			target: "rootDisk", targetLine: 29,
			sibling: "hostPkgs", siblingLine: 50,
			excluded: "name", excludedLine: 30,
			marker: "rootDisk = {",
		},
	}
	enabled := 0
	for _, tc := range cases {
		if os.Getenv(tc.env) != "" {
			enabled++
		}
	}
	if enabled == 0 {
		t.Skip("two independent original Nix projects not configured")
	}
	if enabled != len(cases) {
		t.Fatal("both Nix original project roots must be configured")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original regular file unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
			if len(lines) <= tc.siblingLine || !strings.Contains(lines[tc.targetLine-1], tc.marker) ||
				!strings.Contains(lines[tc.siblingLine-1], tc.sibling+" =") ||
				!strings.Contains(lines[tc.excludedLine-1], tc.excluded+" =") {
				t.Fatal("original Nix selected source differs from independent oracle")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "nix", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || digest.CoverageComplete ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Nix partial digest incorrect: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "nix", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || outline.Truncated || outline.CoverageComplete {
				t.Fatalf("Nix original detailed outline invalid: %v %+v", err, outline)
			}
			want := map[string]int{tc.target: tc.targetLine, tc.sibling: tc.siblingLine}
			for _, sym := range outline.Symbols {
				if sym.Name == tc.excluded && sym.NameRange.Start.Line == tc.excludedLine {
					t.Fatalf("nested attribute/function-local binding misreported as top-level: %+v", sym)
				}
				if line, ok := want[sym.Name]; ok && sym.NameRange.Start.Line == line &&
					sym.ParentQualifiedName == "" && sym.ID != "" {
					delete(want, sym.Name)
				}
			}
			if len(want) != 0 {
				t.Fatalf("real direct let declarations missing: %+v", want)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "nix",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
				FileDetails: "relevant",
			})
			if err != nil || len(found.Files) != 1 || found.Files[0].Encoding == "" ||
				found.Files[0].SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("Nix direct binding selector invalid: %v %+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.targetLine {
					id = sym.ID
				}
			}
			if id == "" {
				t.Fatalf("target direct let missing: %+v", found.Symbols)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "nix", Encoding: found.Files[0].Encoding,
				SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.marker) {
				t.Fatalf("Nix exact source excerpt invalid: %v %+v", err, shown.Show)
			}
			_, excluded, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "nix",
				Query: tc.excluded, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || len(excluded.Symbols) != 0 {
				t.Fatalf("Nix nested declaration leaked through exact find: %v %+v", err, excluded.Symbols)
			}
		})
	}
}
