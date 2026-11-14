package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTOMLOriginalMultilineStringNavigation(t *testing.T) {
	cases := []struct {
		name, env, key, parent string
		start, end             int
		marker                 string
	}{
		{"zoxide package description", "SCRIPTHOLD_REAL_TOML_MULTILINE_ZOXIDE_ROOT", "extended-description", "package.metadata.deb", 104, 107, "zoxide is a smarter cd command"},
		{"ripgrep package description", "SCRIPTHOLD_REAL_TOML_MULTILINE_RIPGREP_ROOT", "description", "package", 5, 9, "first class support on Windows"},
	}
	count := 0
	for _, tc := range cases {
		if os.Getenv(tc.env) != "" {
			count++
		}
	}
	if count == 0 {
		t.Skip("independent original TOML project roots not configured")
	}
	if count != len(cases) {
		t.Fatal("both original TOML project roots must be configured")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			path := filepath.Join(root, "Cargo.toml")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original regular source missing: %v %v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if len(lines) < tc.end || !strings.HasPrefix(strings.TrimSpace(lines[tc.start-1]), tc.key+" = \"\"\"") ||
				!strings.Contains(strings.Join(lines[tc.start-1:tc.end], "\n"), tc.marker) {
				t.Fatalf("independently read source oracle changed: %s %d-%d", tc.key, tc.start, tc.end)
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "toml", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("TOML original digest: %v %+v", err, digest)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "toml", Query: tc.key, Match: "exact",
				MaxFiles: 1, MaxSymbols: 16, FileDetails: "relevant",
			})
			if err != nil {
				t.Fatal(err)
			}
			id := ""
			for _, s := range found.Symbols {
				if s.Name == tc.key && string(s.Kind) == "key" && s.ParentQualifiedName == tc.parent &&
					s.DeclarationRange.Start.Line == tc.start && s.DeclarationRange.End.Line == tc.end {
					id = s.ID
				}
			}
			if id == "" || len(found.Files) != 1 {
				t.Fatalf("multi-line original TOML key missing/short: %+v", found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("original TOML fingerprint mismatch: %+v", file)
			}
			_, show, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "toml", Encoding: file.Encoding,
				SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || show.Show == nil || !strings.Contains(show.Show.Text, tc.marker) ||
				!strings.Contains(show.Show.Text, tc.key+" = \"\"\"") {
				t.Fatalf("TOML original multi-line show incomplete: %s %v %+v", tc.key, err, show.Show)
			}
		})
	}
}
