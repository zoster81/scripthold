package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The gRPC health enum is a maintenance location: an LLM must distinguish
// individual serving statuses from the enclosing enum and from the status field.
func TestProtoEnumValuesOriginalGRPCNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_PROTO_ENUM_GRPC_ROOT")
	if root == "" {
		t.Skip("original gRPC health schema not configured")
	}
	health := filepath.Join(root, "health.proto")
	hello := filepath.Join(root, "helloworld.proto")
	for _, path := range []string{health, hello} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			t.Fatalf("original Protobuf source missing or oversized %s: info=%v err=%v", path, info, err)
		}
	}
	raw, err := os.ReadFile(health)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	owner := "grpc.health.v1.HealthCheckResponse.ServingStatus"
	targets := []struct {
		name, source string
		line         int
	}{
		{"UNKNOWN", "UNKNOWN = 0;", 35},
		{"SERVING", "SERVING = 1;", 36},
		{"NOT_SERVING", "NOT_SERVING = 2;", 37},
		{"SERVICE_UNKNOWN", "SERVICE_UNKNOWN = 3;", 38},
	}
	for _, tg := range targets {
		if tg.line <= 0 || tg.line > len(lines) ||
			!strings.Contains(lines[tg.line-1], tg.source) {
			t.Fatalf("independent original gRPC enum oracle drift %s:%d", tg.name, tg.line)
		}
	}
	if !strings.Contains(string(raw), "ServingStatus status = 1;") ||
		!strings.Contains(string(raw), "rpc Watch(HealthCheckRequest)") {
		t.Fatal("original serving-status field and RPC neighbor changed")
	}
	ctx, h := context.Background(), NewHandler([]string{root})
	_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "digest", Paths: []string{health}, Language: "proto", MaxFiles: 1,
	})
	if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
		len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
		t.Fatalf("original proto digest invalid: err=%v digest=%+v", err, digest)
	}
	_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{health},
		Language: "proto", MaxFiles: 1, MaxSymbols: 100,
	})
	if err != nil || outline.Truncated || outline.FilesParsed != 1 || len(outline.Files) != 1 {
		t.Fatalf("original proto outline unavailable: err=%v outline=%+v", err, outline)
	}
	ids := make(map[string]bool)
	for _, sym := range outline.Symbols {
		ids[sym.ID] = true
		if sym.Name == "status" && sym.ParentQualifiedName == owner {
			t.Fatalf("health status message field falsely owned by its enum: %+v", sym)
		}
	}
	for _, tg := range targets {
		t.Run(tg.name, func(t *testing.T) {
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{health}, Language: "proto",
				Query: tg.name, Match: "exact", MaxFiles: 1, MaxSymbols: 100,
				FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original proto enum value missing %s: err=%v found=%+v", tg.name, err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tg.name && sym.Kind == "constant" &&
					sym.NativeKind == "enum-value" && sym.ParentQualifiedName == owner &&
					sym.NameRange.Start.Line == tg.line {
					id = sym.ID
				}
			}
			if id == "" || !ids[id] {
				t.Fatalf("original enum status %s not safely indexed at line %d: %+v", tg.name, tg.line, found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("original source fingerprint mismatch %s: %+v", tg.name, file)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: health, Language: "proto", Encoding: file.Encoding,
				SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tg.source) {
				t.Fatalf("original enum value source excerpt missing %s: err=%v shown=%+v", tg.name, err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{health},
				Language: "proto", Query: tg.name, Match: "exact",
				MaxFiles: 1, MaxResults: 100,
			})
			linked := false
			if query.Search != nil {
				for _, hit := range query.Search.Matches {
					if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
						hit.Range.Start.Line == tg.line {
						linked = true
					}
				}
			}
			if err != nil || query.Search == nil || query.Coverage.ResultsTruncated || !linked {
				t.Fatalf("original enum value structural identity missing %s: err=%v search=%+v", tg.name, err, query)
			}
		})
	}
	_, unrelated, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{hello}, Language: "proto",
		Query: "SERVICE_UNKNOWN", Match: "exact", MaxFiles: 1, MaxSymbols: 50,
	})
	if err != nil || unrelated.Truncated || len(unrelated.Symbols) != 0 {
		t.Fatalf("unrelated original gRPC hello service fabricated status enum: err=%v symbols=%+v", err, unrelated.Symbols)
	}
}
