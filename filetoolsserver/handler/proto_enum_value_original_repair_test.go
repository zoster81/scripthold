package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Both independently reviewed original schemas need complete field statements
// for safe editor-facing drilldown, not just the numeric field identifiers.
func TestProtoOriginalFieldShowIncludesTerminator(t *testing.T) {
	cases := []struct {
		name, env, filename, owner, field, declaration, neighbor string
		line                                                     int
	}{
		{
			name:     "Google APIs HTTP selector",
			env:      "SCRIPTHOLD_REAL_PROTO_FIELD_GOOGLE_ROOT",
			filename: "http.proto",
			owner:    "google.api.HttpRule", field: "selector", line: 312,
			declaration: "string selector = 1;",
			neighbor:    "string body = 7;",
		},
		{
			name:     "gRPC health serving status",
			env:      "SCRIPTHOLD_REAL_PROTO_FIELD_GRPC_ROOT",
			filename: "health.proto",
			owner:    "grpc.health.v1.HealthCheckResponse", field: "status", line: 40,
			declaration: "ServingStatus status = 1;",
			neighbor:    "map<string, HealthCheckResponse> statuses = 1;",
		},
	}
	configured := 0
	for _, tc := range cases {
		if os.Getenv(tc.env) != "" {
			configured++
		}
	}
	if configured == 0 {
		t.Skip("independent protobuf original schemas not configured")
	}
	if configured != len(cases) {
		t.Fatal("both independent protobuf original roots must be supplied")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			path := filepath.Join(root, tc.filename)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("independent original protobuf unavailable: info=%v err=%v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
			if len(lines) <= tc.line || strings.TrimSpace(lines[tc.line-1]) != tc.declaration ||
				!strings.Contains(string(data), tc.neighbor) {
				t.Fatal("independently reviewed original protobuf schema changed")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "proto", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || digest.Truncated || !digest.CoverageComplete ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original protobuf digest inaccurate: %v %+v", err, digest)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "proto",
				Query: tc.field, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 ||
				found.Files[0].SourceFingerprint != digest.Digests[0].SourceFingerprint ||
				found.Files[0].Encoding == "" {
				t.Fatalf("original protobuf selector missing: %v %+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.field && sym.ParentQualifiedName == tc.owner &&
					sym.NameRange.Start.Line == tc.line && sym.NativeKind == "field" {
					id = sym.ID
				}
			}
			if id == "" {
				t.Fatalf("protobuf field owner/source position missing: %+v", found.Symbols)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "proto", Encoding: found.Files[0].Encoding,
				SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
				MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || strings.TrimSpace(shown.Show.Text) != tc.declaration {
				t.Fatalf("protobuf complete original field source missing: %v %+v", err, shown.Show)
			}
		})
	}
}

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
