package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Independent original Classic ASP projects exercise VBScript class methods,
// the default callable, and server-side JScript navigation. Nothing is executed.
func TestClassicASPOriginalProjectNavigation(t *testing.T) {
	tests := []struct {
		name, env, target, owner, sibling, siblingOwner, oracle, excerpt, unrelated string
		line, siblingLine                                                           int
	}{
		{
			name:   "VBScript logger default callable and log-file configuration",
			env:    "SCRIPTHOLD_REAL_CLASSICASP_LOGGER",
			target: "log", owner: "logger", line: 33,
			sibling: "setLogFile", siblingOwner: "logger", siblingLine: 44,
			oracle: "set log = me", excerpt: "public default function log(p_string)",
			unrelated: "Validate",
		},
		{
			name:   "VBScript anti-forgery form and cookie validation",
			env:    "SCRIPTHOLD_REAL_CLASSICASP_ANTIFORGERY",
			target: "Validate", owner: "AntiForgeryValidator", line: 46,
			sibling: "GetCookie", siblingOwner: "AntiForgeryValidator", siblingLine: 15,
			oracle:  "Request.Form(\"RequestVerificationToken\")",
			excerpt: "Function Validate", unrelated: "setLogFile",
		},
		{
			name:   "Server-side JScript session endpoint and request routing",
			env:    "SCRIPTHOLD_REAL_CLASSICASP_SESSION",
			target: "run", owner: "", line: 35,
			sibling: "SetSessionFromBodyJSON", siblingOwner: "", siblingLine: 94,
			oracle:  "Request.ServerVariables(\"REMOTE_ADDR\")",
			excerpt: "function run(){", unrelated: "Validate",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_CLASSICASP_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("independent original Classic ASP sources not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original Classic ASP project fixture partially configured")
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("original Classic ASP project cannot be read: %v", err)
			}
			if !strings.Contains(string(contents), tc.oracle) ||
				!strings.Contains(string(contents), tc.excerpt) {
				t.Fatal("original Classic ASP maintenance oracle does not match source")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "classic-asp", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Classic ASP digest unavailable: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "classic-asp",
				MaxFiles: 1, MaxSymbols: 96, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("Classic ASP original outline incomplete: err=%v outline=%+v", err, outline)
			}
			targetOK, siblingOK := false, false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line &&
					sym.ParentQualifiedName == tc.owner {
					targetOK = true
				}
				if sym.Name == tc.sibling && sym.NameRange.Start.Line == tc.siblingLine &&
					sym.ParentQualifiedName == tc.siblingOwner {
					siblingOK = true
				}
			}
			if !targetOK || !siblingOK {
				t.Fatalf("Classic ASP maintenance target/sibling missing: target=%v sibling=%v symbols=%+v", targetOK, siblingOK, outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "classic-asp",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated ||
				len(found.Files) != 1 {
				t.Fatalf("Classic ASP targeted find incomplete: err=%v found=%+v", err, found)
			}
			targetID := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line &&
					sym.ParentQualifiedName == tc.owner {
					targetID = sym.ID
				}
			}
			file := found.Files[0]
			if targetID == "" || file.Encoding == "" ||
				file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("Classic ASP source selector unavailable: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "classic-asp",
				Encoding: file.Encoding, SymbolID: targetID,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("Classic ASP fingerprint-verified excerpt wrong: err=%v shown=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "classic-asp", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || search.Search == nil || !search.Coverage.CoverageComplete ||
				search.Coverage.ResultsTruncated {
				t.Fatalf("Classic ASP structural search unavailable: err=%v result=%+v", err, search)
			}
			linked := false
			for _, hit := range search.Search.Matches {
				if hit.Name == tc.target && hit.SymbolID == targetID &&
					hit.SourceFingerprint == file.SourceFingerprint &&
					hit.Range.Start.Line == tc.line {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("Classic ASP search cannot navigate to the exact edit target: %+v", search.Search)
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "classic-asp",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || !negative.CoverageComplete || len(negative.Symbols) != 0 {
				t.Fatalf("Classic ASP unrelated cross-project name falsely indexed: err=%v symbols=%+v", err, negative.Symbols)
			}
		})
	}
}
