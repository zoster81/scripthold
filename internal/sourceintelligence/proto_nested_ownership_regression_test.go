package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A safe field edit needs the entire authored declaration, including inline
// options and its terminator, not only the number after '='.
func TestProtoPinnedFullOriginalRPCDeclarationSpans(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_PROTO_OUTLINE_ROOT")
	if root == "" {
		t.Skip("set SCRIPTHOLD_REAL_PROTO_OUTLINE_ROOT to the two immutable full upstream originals")
	}
	for _, tc := range []struct {
		file  string
		count int
		rpcs  map[string]string
	}{
		{"googleapis-annotations.proto", 3, nil},
		{"grpc-helloworld.proto", 9, map[string]string{
			"SayHello":            "rpc SayHello (HelloRequest) returns (HelloReply) {}",
			"SayHelloStreamReply": "rpc SayHelloStreamReply (HelloRequest) returns (stream HelloReply) {}",
			"SayHelloBidiStream":  "rpc SayHelloBidiStream (stream HelloRequest) returns (stream HelloReply) {}",
		}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, tc.file))
			if err != nil || len(data) == 0 || len(data) > 1<<20 {
				t.Fatalf("complete pinned original absent or invalid: %v", err)
			}
			result, err := (ProtoAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(string(data)), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated ||
				len(result.Analysis.Symbols) != tc.count {
				t.Fatalf("whole original incomplete: got=%d want=%d", len(result.Analysis.Symbols), tc.count)
			}
			seen := make(map[string]bool)
			for _, sym := range result.Analysis.Symbols {
				if want, ok := tc.rpcs[sym.Name]; ok {
					seen[sym.Name] = true
					got := strings.TrimSpace(string(data)[sym.declarationOffsets.Start:sym.declarationOffsets.End])
					if got != want {
						t.Errorf("original authored %s truncated: got %q want %q", sym.Name, got, want)
					}
					if sym.ParentQualifiedName != "helloworld.Greeter" {
						t.Errorf("original RPC owner wrong for %s: %q", sym.Name, sym.ParentQualifiedName)
					}
				}
			}
			for name := range tc.rpcs {
				if !seen[name] {
					t.Errorf("missing original RPC %s", name)
				}
			}
		})
	}
}

func TestProtoRPCSourceSpanLFCRLFAndSemicolon(t *testing.T) {
	original := "package demo;\nservice API {\n" +
		"  rpc Alpha (Req) returns (Res);\n" +
		"  rpc Beta (stream Req) returns (stream Res) {}\n" +
		"  rpc Gamma(\n    Req\n  ) returns (\n    Res\n  ) {}\n" +
		"}\n"
	want := map[string]string{
		"Alpha": "rpc Alpha (Req) returns (Res);",
		"Beta":  "rpc Beta (stream Req) returns (stream Res) {}",
		"Gamma": "rpc Gamma(\n    Req\n  ) returns (\n    Res\n  ) {}",
	}
	for _, tc := range []struct{ name, eol string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			content := strings.ReplaceAll(original, "\n", tc.eol)
			result, err := (ProtoAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(content), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			seen := make(map[string]bool)
			for _, sym := range result.Analysis.Symbols {
				if expected, ok := want[sym.Name]; ok {
					seen[sym.Name] = true
					got := strings.TrimSpace(content[sym.declarationOffsets.Start:sym.declarationOffsets.End])
					if strings.ReplaceAll(got, "\r\n", "\n") != expected {
						t.Errorf("RPC %s got %q want %q", sym.Name, got, expected)
					}
					if sym.ParentQualifiedName != "demo.API" {
						t.Errorf("unexpected parent %+v", sym)
					}
				}
			}
			for name := range want {
				if !seen[name] {
					t.Errorf("missing %s", name)
				}
			}
		})
	}
}

func TestProtoRPCSuffixRejectsMalformedAndOversizedBounds(t *testing.T) {
	for _, tc := range []struct {
		name, source string
	}{
		{"missing_semicolon", "rpc Broken (Req) returns (Res)\n rpc Good (Req) returns (Res);"},
		{"missing_return_paren", "rpc Broken (Req) returns Res;"},
		{"unbalanced_input", "rpc Broken (Req returns (Res);"},
		{"unclosed_option_block", "rpc Broken (Req) returns (Res) { option (foo) = \"};\";"},
		{"oversized_option_body", "rpc Broken (Req) returns (Res) {" + strings.Repeat(" ", 1<<16) + "}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			open := strings.IndexByte(tc.source, '(') + 1
			if open <= 0 {
				t.Fatal("invalid test input")
			}
			end, ok := protoRPCDeclarationEnd(tc.source, open, len(tc.source))
			if ok || end != open {
				t.Errorf("malformed RPC consumed another declaration: end=%d ok=%v", end, ok)
			}
		})
	}
	source := "rpc Valid (.demo.Request) returns (.demo.Reply) { option (custom) = \"not a } terminator\"; }"
	open := strings.IndexByte(source, '(') + 1
	end, ok := protoRPCDeclarationEnd(source, open, len(source))
	if !ok || end != len(source) {
		t.Errorf("valid qualified RPC with quoted brace not recognized: end=%d ok=%v", end, ok)
	}
}

func TestProtoFieldDeclarationRetainsOptionsAndSemicolon(t *testing.T) {
	const source = "syntax = \"proto3\";\n" +
		"package demo;\n" +
		"message Route {\n" +
		"  string selector = 1 [json_name = \"url;selector\", deprecated = true]; // neighbor note\n" +
		"  string body = 2;\n" +
		"}\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (ProtoAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 30))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("supported protobuf field snippet unexpectedly partial: %+v", result.Analysis)
			}
			expected := map[string]string{
				"demo.Route.selector": "string selector = 1 [json_name = \"url;selector\", deprecated = true];",
				"demo.Route.body":     "string body = 2;",
			}
			for _, sym := range result.Analysis.Symbols {
				if want, found := expected[sym.QualifiedName]; found {
					actual := strings.TrimSpace(tc.text[sym.declarationOffsets.Start:sym.declarationOffsets.End])
					if actual != want {
						t.Errorf("field declaration truncated or expanded %s: got %q want %q", sym.QualifiedName, actual, want)
					}
					delete(expected, sym.QualifiedName)
				}
			}
			if len(expected) != 0 {
				t.Fatalf("supported fields not indexed: %+v", expected)
			}
		})
	}
}

// Proto2 groups introduce their own nested message scope. A conservative
// navigator must not attribute group-only fields to the enclosing message.
func TestProtoGroupMembersNotPromotedAsDirectMessageFields(t *testing.T) {
	source := "syntax = \"proto2\";\npackage demo;\n" +
		"message Request {\n" +
		"  string direct = 1;\n" +
		"  optional group SomeGroup = 8 {\n" +
		"    optional int32 group_field = 9;\n" +
		"  }\n" +
		"  string after = 10;\n" +
		"}\n" +
		"message Communique {\n" +
		"  oneof union {\n" +
		"    string selected = 1;\n" +
		"    group Nested = 14 {\n" +
		"      optional string member = 15;\n" +
		"    }\n" +
		"  }\n" +
		"  int32 last = 16;\n" +
		"}\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", source},
		{"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (ProtoAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 80))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("unsupported nested proto2 groups must be partial, not truncated: %+v", result.Analysis)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, name := range []string{
				"demo.Request", "demo.Request.direct", "demo.Request.after",
				"demo.Communique", "demo.Communique.selected", "demo.Communique.last",
			} {
				if _, ok := symbols[name]; !ok {
					t.Errorf("supported declaration omitted after group masking: %s", name)
				}
			}
			for _, name := range []string{
				"demo.Request.SomeGroup", "demo.Request.group_field",
				"demo.Communique.Nested", "demo.Communique.member",
			} {
				if symbol, ok := symbols[name]; ok {
					t.Errorf("group declaration incorrectly promoted to direct member %s: %+v", name, symbol)
				}
			}
			foundDiagnostic := false
			for _, diagnostic := range result.Analysis.Diagnostics {
				if diagnostic.Code == "proto-unmodeled-group" {
					foundDiagnostic = true
				}
			}
			if !foundDiagnostic {
				t.Error("unmodeled group must report a diagnostic")
			}
		})
	}
}

// An unterminated field option must not be advertised as fully understood.
// A later valid sibling remains findable even when a preceding field is partial.
func TestProtoUnclosedFieldOptionsReportIncompleteWithoutHidingSibling(t *testing.T) {
	const source = "package demo;\nmessage Route {\n" +
		"  string partial = 1 [deprecated = true;\n" +
		"  string healthy = 2;\n}\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (ProtoAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 30))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("unclosed Proto field options need partial coverage: %+v", result.Analysis)
			}
			hasDiagnostic, hasHealthy := false, false
			for _, d := range result.Analysis.Diagnostics {
				if d.Code == "proto-unmodeled-field-suffix" {
					hasDiagnostic = true
				}
			}
			for _, s := range result.Analysis.Symbols {
				if s.QualifiedName == "demo.Route.healthy" &&
					strings.TrimSpace(tc.text[s.declarationOffsets.Start:s.declarationOffsets.End]) == "string healthy = 2;" {
					hasHealthy = true
				}
			}
			if !hasDiagnostic || !hasHealthy {
				t.Fatalf("partial field warning or independent healthy sibling missing: %+v", result.Analysis)
			}
		})
	}
}

// A protobuf message can nest types with identical short names. Fields within
// those types must not be exposed as fields of the enclosing message.
func TestProtoNestedMessageDeclarationsOwnTheirFields(t *testing.T) {
	source := "syntax = \"proto3\";\npackage demo;\n" +
		"message Outer {\n" +
		"  string label = 1;\n" +
		"  message Inner {\n" +
		"    string value = 1;\n" +
		"  }\n" +
		"  repeated Inner items = 2;\n" +
		"  oneof selected {\n" +
		"    string choice = 3;\n" +
		"  }\n" +
		"  enum Kind { NONE = 0; }\n" +
		"}\n" +
		"message Other {\n" +
		"  message Inner { int32 value = 1; }\n" +
		"}\n"
	result, err := (ProtoAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 80))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("unexpected partial proto analysis: %+v", result.Analysis)
	}
	got := symbolsByQualifiedName(result.Analysis.Symbols)
	want := map[string]SymbolKind{
		"demo":                   SymbolKindPackage,
		"demo.Outer":             SymbolKindType,
		"demo.Outer.label":       SymbolKindField,
		"demo.Outer.items":       SymbolKindField,
		"demo.Outer.choice":      SymbolKindField,
		"demo.Outer.Inner":       SymbolKindType,
		"demo.Outer.Inner.value": SymbolKindField,
		"demo.Outer.Kind":        SymbolKindEnum,
		"demo.Other":             SymbolKindType,
		"demo.Other.Inner":       SymbolKindType,
		"demo.Other.Inner.value": SymbolKindField,
	}
	for name, kind := range want {
		sym, ok := got[name]
		if !ok || sym.Kind != kind {
			t.Errorf("missing or incorrect protobuf ownership for %s: symbol=%+v exists=%v", name, sym, ok)
		}
	}
	for _, forbidden := range []string{
		"demo.Inner", "demo.Kind", "demo.Outer.value", "demo.Other.value",
		"demo.Outer.Inner.choice", "demo.Other.Inner.items",
	} {
		if sym, exists := got[forbidden]; exists {
			t.Errorf("fabricated protobuf declaration %s: %+v", forbidden, sym)
		}
	}
	for name := range want {
		if got[name].ID == "" {
			t.Errorf("no stable source selector for %s", name)
		}
	}
}

func TestProtoEnumValuesNestedOwnershipAndComments(t *testing.T) {
	source := "syntax = \"proto3\";\npackage demo;\n" +
		"message Outer {\n" +
		"  string STATUS = 1;\n" +
		"  enum State {\n" +
		"    ZERO = 0;\n" +
		"    NEGATIVE = -1;\n" +
		"    // PHANTOM = 99;\n" +
		"    option allow_alias = true;\n" +
		"  }\n" +
		"  enum OtherState {\n" +
		"    ZERO = 0;\n" +
		"  }\n" +
		"  int32 NEGATIVE = 2;\n" +
		"}\n" +
		"message Unrelated { string value = 1; }\n"
	for _, tc := range []struct{ name, text string }{
		{"lf", source},
		{"crlf", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (ProtoAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 80))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("bounded enum schema parsed partially: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, expected := range []struct {
				name, native string
				kind         SymbolKind
				line         int
			}{
				{"demo.Outer.State.ZERO", "enum-value", SymbolKindConstant, 6},
				{"demo.Outer.State.NEGATIVE", "enum-value", SymbolKindConstant, 7},
				{"demo.Outer.OtherState.ZERO", "enum-value", SymbolKindConstant, 12},
				{"demo.Outer.NEGATIVE", "field", SymbolKindField, 14},
				{"demo.Outer.STATUS", "field", SymbolKindField, 4},
			} {
				got, exists := byName[expected.name]
				if !exists || got.Kind != expected.kind || got.NativeKind != expected.native ||
					got.NameRange.Start.Line != expected.line || got.ID == "" {
					t.Errorf("unexpected Protobuf enum/field declaration %s: %+v", expected.name, got)
				}
			}
			for _, forbidden := range []string{
				"demo.Outer.State.PHANTOM", "demo.Outer.State.allow_alias",
				"demo.Outer.State.STATUS", "demo.Outer.NEGATIVE.ZERO",
				"demo.Outer.ZERO", "demo.ZERO",
			} {
				if symbol, exists := byName[forbidden]; exists {
					t.Errorf("false enum member or owner %s: %+v", forbidden, symbol)
				}
			}
		})
	}
}

func TestProtoMalformedOuterBlockDoesNotInventTopLevelNestedType(t *testing.T) {
	source := "package demo;\nmessage Broken {\n  message Inner { string valid = 1; }\n"
	result, err := (ProtoAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 40))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
		t.Fatalf("unclosed proto outer scope claimed complete analysis: %+v", result.Analysis)
	}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Name == "Inner" && symbol.ParentQualifiedName == "demo" {
			t.Fatalf("nested type leaked to package after malformed outer declaration: %+v", symbol)
		}
	}
}

func TestProtoQualifiedExtendTargetDoesNotCreateDottedTypeDeclarations(t *testing.T) {
	source := strings.Join([]string{
		"syntax = \"proto2\";",
		"package demo;",
		"message Good {",
		"  string name = 1;",
		"}",
		"message Invalid.Name {",
		"  message Phantom {",
		"    string ghost = 1;",
		"  }",
		"}",
		"enum Invalid.Kind {",
		"  GHOST = 0;",
		"}",
		"service Invalid.Api {",
		"  rpc Ghost(Request) returns(Response);",
		"}",
		"extend .google.protobuf.MethodOptions {",
		"  optional string http = 72295728;",
		"}",
		"",
	}, "\n")
	for _, tc := range []struct{ name, source string }{
		{"lf", source},
		{"crlf", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (ProtoAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 80))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated {
				t.Fatalf("small proto source unexpectedly truncated: %+v", result.Analysis)
			}
			if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
				t.Fatalf("invalid qualified declarations require explicit partial coverage: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, wanted := range []struct {
				name, native string
				kind         SymbolKind
			}{
				{"demo.Good", "message", SymbolKindType},
				{"demo.Good.name", "field", SymbolKindField},
				{"demo.extend .google.protobuf.MethodOptions", "extend-block", SymbolKindNamespace},
				{"demo.extend .google.protobuf.MethodOptions.http", "extension-field", SymbolKindField},
			} {
				symbol, ok := got[wanted.name]
				if !ok || symbol.Kind != wanted.kind || symbol.NativeKind != wanted.native || symbol.ID == "" {
					t.Errorf("valid Proto declaration omitted or incorrectly owned %s: %+v", wanted.name, symbol)
				}
			}
			for _, forbidden := range []string{
				"demo.Invalid.Name", "demo.Invalid.Kind", "demo.Invalid.Api",
				"demo.Phantom", "demo.Phantom.ghost", "demo.Invalid.Name.Phantom",
				"demo.GHOST", "demo.Ghost",
			} {
				if symbol, exists := got[forbidden]; exists {
					t.Errorf("invalid dotted Proto declaration or nested child invented: %s %+v", forbidden, symbol)
				}
			}
		})
	}
}
