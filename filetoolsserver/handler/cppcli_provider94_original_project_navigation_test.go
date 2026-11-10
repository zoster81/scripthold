package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// These selected files are byte-identical to two independent original Git
// projects; the source oracle is checked before the MCP navigation requests.
func TestCPPCLIProvider94OriginalProjectNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, owner, excerpt, neighbor, independentOracle string
		line, neighborLine                                             int
	}{
		{
			name: "libyara metadata initialization", env: "SCRIPTHOLD_REAL_CPPCLI_LIBYARA",
			target: "Meta", owner: "libyaraNET.Meta", line: 15,
			neighbor: "Meta", neighborLine: 21,
			excerpt:           "Meta()",
			independentOracle: "Identifier = nullptr;",
		},
		{
			name: "FFmpeg missing-timestamp sentinel", env: "SCRIPTHOLD_REAL_CPPCLI_FFMPEG",
			target: "NOPTS_VALUE", owner: "FFMpegNet.AVFormat.AVConstants", line: 17,
			neighbor: "AVConstants", neighborLine: 12,
			excerpt:           "NOPTS_VALUE = 0x8000000000000000;",
			independentOracle: "namespace AVFormat",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_CPPCLI_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("original C++/CLI source not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original C++/CLI source only partially configured")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			if !strings.Contains(text, tc.excerpt) || !strings.Contains(text, tc.independentOracle) {
				t.Fatal("independent original C++/CLI source oracle mismatch")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "cpp-cli", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("C++/CLI file mapping failed: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "cpp-cli",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated {
				t.Fatalf("C++/CLI source outline failed: err=%v result=%+v", err, outline)
			}
			targetID, neighborOK := "", false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.ParentQualifiedName == tc.owner &&
					sym.NameRange.Start.Line == tc.line && sym.ID != "" {
					targetID = sym.ID
				}
				if sym.Name == tc.neighbor && sym.NameRange.Start.Line == tc.neighborLine {
					neighborOK = true
				}
				if sym.Kind == "variable" && sym.Name == "System" {
					t.Fatalf("namespace import fabricated as C++/CLI variable: %+v", sym)
				}
			}
			if targetID == "" || !neighborOK {
				t.Fatalf("C++/CLI edit target or neighbor missing: target=%t neighbor=%t symbols=%+v",
					targetID != "", neighborOK, outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "cpp-cli",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
				FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("C++/CLI targeted lookup failed: err=%v result=%+v", err, found)
			}
			foundTarget := false
			for _, sym := range found.Symbols {
				if sym.ID == targetID && sym.NameRange.Start.Line == tc.line {
					foundTarget = true
				}
			}
			file := found.Files[0]
			if !foundTarget || file.SourceFingerprint != digest.Digests[0].SourceFingerprint ||
				file.Encoding == "" {
				t.Fatalf("C++/CLI safe drilldown selector inconsistent: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "cpp-cli",
				Encoding: file.Encoding, SymbolID: targetID,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("C++/CLI exact authored excerpt unavailable: err=%v show=%+v", err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "cpp-cli", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 128,
			})
			if err != nil || query.Search == nil || query.Coverage.ResultsTruncated {
				t.Fatalf("C++/CLI structural search failed: err=%v query=%+v", err, query)
			}
			linked := false
			for _, match := range query.Search.Matches {
				if match.SymbolID == targetID && match.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("C++/CLI structural search failed to link exact source: %+v", query.Search)
			}
		})
	}
}
