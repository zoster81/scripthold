package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These original API schemas define callable HTTP routes without operationId.
// The maintenance task is to locate the route contract, not infer its handler.
func TestOpenAPIMethodsWithoutOperationIDOriginalNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original OpenAPI source projects not configured")
	}
	type target struct {
		name, method string
		line         int
	}
	scenarios := []struct {
		project, file, unrelated string
		targets                  []target
	}{
		{
			project:   "openapi77-worldcoin-audit-selected",
			file:      "schemas/openapi-v3.yaml",
			unrelated: "GET /v1/shape",
			targets: []target{
				{"POST /v3/identities/{commitment}", "post", 12},
				{"DELETE /v3/identities/{commitment}", "delete", 74},
				{"GET /v3/identities/{commitment}/inclusion-proof/{inclusionProofType}", "get", 133},
				{"POST /v3/semaphore-proof/verify", "post", 197},
			},
		},
		{
			project:   "openapi77-electric-audit-selected",
			file:      "website/electric-api.yaml",
			unrelated: "POST /v3/semaphore-proof/verify",
			targets: []target{
				{"GET /v1/shape", "get", 44},
				{"POST /v1/shape", "post", 848},
				{"DELETE /v1/shape", "delete", 1081},
			},
		},
	}
	ctx := context.Background()
	for _, sc := range scenarios {
		t.Run(sc.project, func(t *testing.T) {
			root := filepath.Join(corpus, sc.project)
			path := filepath.Join(root, filepath.FromSlash(sc.file))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original OpenAPI source not bounded/regular: %v %v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(raw), "\n")
			if !strings.Contains(string(raw), "paths:\n") || strings.Contains(string(raw), "operationId:") {
				t.Fatal("independently reviewed original operationId-free API contract changed")
			}
			for _, tg := range sc.targets {
				if tg.line < 1 || tg.line > len(lines) ||
					strings.TrimSpace(lines[tg.line-1]) != tg.method+":" {
					t.Fatalf("original route declaration oracle drift: %s:%d", tg.name, tg.line)
				}
			}
			h := NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "openapi", MaxFiles: 1,
			})
			if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original OpenAPI digest missing: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "openapi", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("original OpenAPI detailed outline missing: %v %+v", err, outline)
			}
			ids := make(map[string]bool)
			for _, sym := range outline.Symbols {
				ids[sym.ID] = true
			}
			for _, tg := range sc.targets {
				t.Run(tg.name, func(t *testing.T) {
					_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
						Operation: "find", Paths: []string{path}, Language: "openapi",
						Query: tg.name, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
						FileDetails: "relevant",
					})
					id := ""
					for _, sym := range found.Symbols {
						if sym.Name == tg.name && sym.Kind == "operation" &&
							sym.NativeKind == "http-method" && sym.NameRange.Start.Line == tg.line {
							id = sym.ID
						}
					}
					if err != nil || id == "" || !ids[id] || len(found.Files) != 1 {
						t.Fatalf("original HTTP method omitted or wrong owner: err=%v found=%+v", err, found)
					}
					file := found.Files[0]
					if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
						t.Fatalf("original OpenAPI fingerprint mismatch: %+v", file)
					}
					_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
						Operation: "show", Path: path, Language: "openapi", Encoding: file.Encoding,
						SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
					})
					if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tg.method+":") {
						t.Fatalf("original route source drilldown missing %s: err=%v show=%+v", tg.name, err, shown.Show)
					}
					_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
						Operation: "search", Mode: "structural", Paths: []string{path},
						Language: "openapi", Query: tg.name, Match: "exact",
						MaxFiles: 1, MaxResults: 128,
					})
					linked := false
					if search.Search != nil {
						for _, hit := range search.Search.Matches {
							if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
								hit.Range.Start.Line == tg.line {
								linked = true
							}
						}
					}
					if err != nil || search.Search == nil || search.Coverage.ResultsTruncated || !linked {
						t.Fatalf("original OpenAPI method not structurally linked: err=%v search=%+v", err, search)
					}
				})
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "openapi",
				Query: sc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated project HTTP method invented: err=%v symbols=%+v", err, negative.Symbols)
			}
		})
	}
}
