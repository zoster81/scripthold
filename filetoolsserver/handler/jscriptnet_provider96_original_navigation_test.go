package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The two source files were independently checked against unrelated Git projects.
// These requests exercise the development MCP handler, not the installed server.
func TestJScriptNetProvider96OriginalProjectNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, owner, anchor, sourceOracle, neighbor string
		targetLine, neighborLine                                 int
	}{
		{
			name: "watcher compilation event handler", env: "SCRIPTHOLD_REAL_JSNET_WATCHER96",
			target: "onChange", owner: "Handlers", targetLine: 34,
			anchor: "static function onChange", sourceOracle: "jscp.StartInfo.FileName = \"jsc.exe\";",
			neighbor: "onRename", neighborLine: 50,
		},
		{
			name: "file reader source", env: "SCRIPTHOLD_REAL_JSNET_JSTEST96",
			target: "readFile", owner: "File", targetLine: 9,
			anchor: "function readFile", sourceOracle: "reader.ReadToEnd()",
			neighbor: "File", neighborLine: 7,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_JSNET_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("original JScript.NET source not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original JScript.NET source only partially configured")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.anchor) || !strings.Contains(string(data), tc.sourceOracle) {
				t.Fatal("original JScript.NET maintenance evidence mismatch")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "jscript-net", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("JScript.NET digest failed: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "jscript-net",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated || !outline.CoverageComplete {
				t.Fatalf("JScript.NET outline incomplete: err=%v result=%+v", err, outline)
			}
			selected, neighborOK := "", false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.ParentQualifiedName == tc.owner &&
					sym.NameRange.Start.Line == tc.targetLine && sym.ID != "" {
					selected = sym.ID
				}
				if sym.Name == tc.neighbor && sym.NameRange.Start.Line == tc.neighborLine {
					neighborOK = true
				}
				if sym.Name == "Process" || sym.Name == "ReadToEnd" {
					t.Errorf("local or invocation incorrectly indexed as declaration: %+v", sym)
				}
			}
			if selected == "" || !neighborOK {
				t.Fatalf("target or neighboring source declaration missing: target=%v neighbor=%v symbols=%+v", selected != "", neighborOK, outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "jscript-net",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
				FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("JScript.NET find failed: err=%v result=%+v", err, found)
			}
			chosen := false
			for _, sym := range found.Symbols {
				if sym.ID == selected && sym.NameRange.Start.Line == tc.targetLine {
					chosen = true
				}
			}
			file := found.Files[0]
			if !chosen || file.SourceFingerprint != digest.Digests[0].SourceFingerprint || file.Encoding == "" {
				t.Fatalf("safe JScript.NET selector missing or inconsistent: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "jscript-net",
				Encoding: file.Encoding, SymbolID: selected,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.anchor) {
				t.Fatalf("JScript.NET source-exact excerpt failed: err=%v show=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "jscript-net", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 128,
			})
			if err != nil || search.Search == nil || search.Coverage.ResultsTruncated {
				t.Fatalf("JScript.NET structural search failed: err=%v result=%+v", err, search)
			}
			linked := false
			for _, hit := range search.Search.Matches {
				if hit.SymbolID == selected && hit.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("JScript.NET structural search not linked to target: %+v", search.Search)
			}
		})
	}
}
