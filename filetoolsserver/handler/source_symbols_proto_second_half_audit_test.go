package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// These immutable Google APIs and gRPC originals are independent of the
// earlier Gleam/Sonar proto sources. We verify authored declarations and
// bounded navigation, not generated stubs or runtime protocol semantics.
func TestSourceSymbolsProtoSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Protocol Buffers project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "proto85-googleapis-audit-selected", language: "proto",
			primary: "http.proto", search: "additional_bindings",
			negativeFile: "annotations.proto", negativeQuery: "additional_bindings",
			paths: []string{"http.proto", "annotations.proto"},
			oracles: []extraAuditEvidence{
				{"http.proto", "message HttpRule {"},
				{"http.proto", "oneof pattern {"},
				{"http.proto", "string body = 7;"},
				{"http.proto", "repeated HttpRule additional_bindings = 11;"},
				{"http.proto", "message CustomHttpPattern {"},
				{"annotations.proto", "import \"google/api/http.proto\";"},
				{"annotations.proto", "extend google.protobuf.MethodOptions {"},
			},
			targets: []extraAuditSymbol{
				{"http.proto", "Http", 28},
				{"http.proto", "HttpRule", 307},
				{"http.proto", "get", 320},
				{"http.proto", "body", 347},
				{"http.proto", "additional_bindings", 360},
				{"http.proto", "CustomHttpPattern", 364},
				{"annotations.proto", "google.api", 17},
			},
		},
		{
			project: "proto85-grpc-audit-selected", language: "proto",
			primary: "health.proto", search: "Watch",
			negativeFile: "helloworld.proto", negativeQuery: "Watch",
			paths: []string{"health.proto", "helloworld.proto"},
			oracles: []extraAuditEvidence{
				{"health.proto", "message HealthCheckResponse {"},
				{"health.proto", "enum ServingStatus {"},
				{"health.proto", "map<string, HealthCheckResponse> statuses = 1;"},
				{"health.proto", "rpc Check(HealthCheckRequest) returns (HealthCheckResponse);"},
				{"health.proto", "rpc List(HealthListRequest) returns (HealthListResponse);"},
				{"health.proto", "rpc Watch(HealthCheckRequest) returns (stream HealthCheckResponse);"},
				{"helloworld.proto", "service Greeter {"},
				{"helloworld.proto", "rpc SayHello (HelloRequest) returns (HelloReply) {}"},
			},
			targets: []extraAuditSymbol{
				{"health.proto", "HealthCheckResponse", 33},
				{"health.proto", "ServingStatus", 34},
				{"health.proto", "status", 40},
				{"health.proto", "HealthListResponse", 45},
				{"health.proto", "statuses", 47},
				{"health.proto", "Health", 53},
				{"health.proto", "Check", 61},
				{"health.proto", "List", 74},
				{"health.proto", "Watch", 91},
				{"helloworld.proto", "Greeter", 25},
				{"helloworld.proto", "SayHello", 27},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
	t.Run("real nested health enum has exact owner", func(t *testing.T) {
		root := filepath.Join(corpus, "proto85-grpc-audit-selected")
		path := filepath.Join(root, "health.proto")
		requireAdditionalOriginalFile(t, path)
		_, outline, err := NewHandler([]string{root}).SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Paths: []string{path}, Language: "proto",
			Encoding: "utf-8", Presentation: "detailed", MaxFiles: 1, MaxSymbols: 80,
		})
		if err != nil || !outline.CoverageComplete || outline.Truncated {
			t.Fatalf("original health enum scope incomplete: err=%v out=%+v", err, outline)
		}
		for _, s := range outline.Symbols {
			if s.Name == "ServingStatus" {
				if s.ParentQualifiedName != "grpc.health.v1.HealthCheckResponse" ||
					s.NameRange.Start.Line != 34 {
					t.Fatalf("nested enum has invented package-level owner: %+v", s)
				}
				return
			}
		}
		t.Fatal("source-backed nested health enum not navigable")
	})
}
