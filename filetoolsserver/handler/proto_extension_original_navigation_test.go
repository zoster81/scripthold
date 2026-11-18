package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original protobuf extension declarations must be navigable as lexical
// scopes and fields, without claiming that the referenced type is resolved.
func TestProtoPinnedFullOriginalRPCDetailedNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_PROTO_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete pinned original Proto sources are not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file  string
		count int
	}{
		{"googleapis-annotations.proto", 3},
		{"grpc-helloworld.proto", 9},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			data, err := os.ReadFile(path)
			if err != nil || len(data) == 0 || len(data) > 1<<20 {
				t.Fatalf("pinned source missing/invalid: %v", err)
			}
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "proto", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != tc.count {
				t.Fatalf("detailed original not complete: err=%v failure=%+v symbols=%d", err, failed, len(outline.Symbols))
			}
			if tc.file != "grpc-helloworld.proto" {
				return
			}
			for _, target := range []struct {
				name      string
				line, col int
				signature string
			}{
				{"SayHello", 27, 54, "  rpc SayHello (HelloRequest) returns (HelloReply) {}"},
				{"SayHelloStreamReply", 29, 72, "  rpc SayHelloStreamReply (HelloRequest) returns (stream HelloReply) {}"},
				{"SayHelloBidiStream", 31, 78, "  rpc SayHelloBidiStream (stream HelloRequest) returns (stream HelloReply) {}"},
			} {
				count := 0
				for _, sym := range outline.Symbols {
					if sym.Name != target.name {
						continue
					}
					count++
					if sym.Kind != "operation" || sym.NativeKind != "rpc" || sym.ParentQualifiedName != "helloworld.Greeter" ||
						sym.DeclarationRange.End.Line != target.line || sym.DeclarationRange.End.Column != target.col ||
						sym.SignatureRange.End.Column != target.col || sym.Signature != target.signature {
						t.Errorf("full original detailed RPC wrong: %+v", sym)
					}
				}
				if count != 1 {
					t.Errorf("RPC %s count=%d", target.name, count)
				}
			}
			fp := outline.Files[0].SourceFingerprint
			if fp == "" {
				t.Fatal("original fingerprint absent")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "proto", Encoding: "utf-8",
				Query: "SayHello", Match: "exact", MaxFiles: 1, MaxSymbols: 12,
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 || found.Symbols[0].NameRange.Start.Line != 27 {
				t.Fatalf("real original method find wrong err=%v failed=%+v count=%d", err, failed, len(found.Symbols))
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "proto", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fp, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "returns (HelloReply) {}") {
				t.Fatalf("full RPC source excerpt absent err=%v failed=%+v", err, failed)
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path}, Language: "proto", Encoding: "utf-8",
				Query: "SayHello", Match: "exact", MaxFiles: 1, MaxResults: 12,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) != 1 {
				t.Fatalf("original structural search failed err=%v failed=%+v", err, failed)
			}
		})
	}
}

func TestProtoOriginalExtensionFieldNavigation(t *testing.T) {
	google := os.Getenv("SCRIPTHOLD_REAL_PROTO_EXT_GOOGLE_ROOT")
	ctags := os.Getenv("SCRIPTHOLD_REAL_PROTO_EXT_CTAGS_ROOT")
	if google == "" || ctags == "" {
		t.Skip("two immutable original Protobuf projects not configured")
	}
	type target struct {
		name, parent, native string
		line                 int
	}
	cases := []struct {
		root, file string
		targets    []target
	}{
		{google, "annotations.proto", []target{
			{"extend google.protobuf.MethodOptions", "google.api", "extend-block", 28},
			{"http", "google.api.extend google.protobuf.MethodOptions", "extension-field", 30},
		}},
		{ctags, "Units/parser-protobuf.r/simple-protobuf.d/input.proto", []target{
			{"extend Message3", "TestPackage", "extend-block", 62},
			{"field7", "TestPackage.extend Message3", "extension-field", 63},
			{"field8", "TestPackage.extend Message3", "extension-field", 64},
		}},
		{ctags, "Units/parser-protobuf.r/protobuf-group.d/input.proto", []target{
			{"extend Reply", "my.test.ReplyExtensions", "extend-block", 109},
			{"time", "my.test.ReplyExtensions.extend Reply", "extension-field", 110},
			{"carrot", "my.test.ReplyExtensions.extend Reply", "extension-field", 111},
			{"extend OtherBase", "my.test.ReplyExtensions", "extend-block", 113},
			{"donut", "my.test.ReplyExtensions.extend OtherBase", "extension-field", 114},
			{"extend Reply", "my.test", "extend-block", 123},
			{"tag", "my.test.extend Reply", "extension-field", 124},
			{"donut", "my.test.extend Reply", "extension-field", 125},
		}},
	}
	ctx := context.Background()
	for _, sc := range cases {
		t.Run(sc.file, func(t *testing.T) {
			path := filepath.Join(sc.root, filepath.FromSlash(sc.file))
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
				t.Fatalf("source invalid %v %v", stat, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(raw), "\n")
			for _, tg := range sc.targets {
				if tg.line < 1 || tg.line > len(lines) {
					t.Fatalf("oracle outside file %+v", tg)
				}
				line := lines[tg.line-1]
				if tg.native == "extend-block" && !strings.Contains(line, tg.name+" {") {
					t.Fatalf("original extend oracle drift %s:%d", tg.name, tg.line)
				}
				if tg.native == "extension-field" && !strings.Contains(line, " "+tg.name+" = ") {
					t.Fatalf("original field oracle drift %s:%d", tg.name, tg.line)
				}
			}
			h := NewHandler([]string{sc.root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "digest", Paths: []string{path}, Language: "proto", MaxFiles: 1})
			if err != nil || digest.FilesParsed != 1 || digest.Truncated || len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original digest unavailable: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "proto", MaxFiles: 1, MaxSymbols: 150})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated {
				t.Fatalf("original outline unavailable: %v %+v", err, outline)
			}
			symbols := map[string]bool{}
			for _, s := range outline.Symbols {
				symbols[s.ID] = true
			}
			for _, tg := range sc.targets {
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "proto", Query: tg.name, Match: "exact", MaxFiles: 1, MaxSymbols: 80, FileDetails: "relevant"})
				id := ""
				for _, sym := range found.Symbols {
					kind := "field"
					if tg.native == "extend-block" {
						kind = "namespace"
					}
					if sym.Name == tg.name && sym.NativeKind == tg.native && string(sym.Kind) == kind && sym.ParentQualifiedName == tg.parent && sym.NameRange.Start.Line == tg.line {
						id = sym.ID
					}
				}
				if err != nil || id == "" || !symbols[id] || len(found.Files) != 1 {
					t.Fatalf("missing safe original extension declaration %+v: %v %+v", tg, err, found.Symbols)
				}
				file := found.Files[0]
				if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
					t.Fatalf("original selector stale %+v", file)
				}
				_, show, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "proto", Encoding: file.Encoding, SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096})
				if err != nil || show.Show == nil || !strings.Contains(show.Show.Text, tg.name) {
					t.Fatalf("extension source excerpt missing: %v %+v", err, show.Show)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "proto", Query: "commentedField2", Match: "exact", MaxFiles: 1, MaxSymbols: 80})
			if err != nil || len(negative.Symbols) != 0 {
				t.Fatalf("comment became extension field: %v %+v", err, negative.Symbols)
			}
		})
	}
}

// Unsupported proto2 group bodies must not invent fields under their enclosing
// message. Original ctags sources also prove unaffected siblings remain navigable.
func TestProtoOriginalGroupFieldsRemainPartialWithoutFalseOwners(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_PROTO_GROUP_CTAGS_ROOT")
	if root == "" {
		t.Skip("original Proto2 group source not configured")
	}
	path := filepath.Join(root, "Units", "parser-protobuf.r", "protobuf-group.d", "input.proto")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		t.Fatalf("original group source missing or invalid: %v", err)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.ReplaceAll(string(source), "\r\n", "\n"), "\n")
	for _, want := range []struct {
		line     int
		fragment string
	}{
		{68, "optional group SomeGroup = 8 {"},
		{69, "optional int32 group_field = 9;"},
		{85, "optional string get_key = 16;"},
		{149, "group SomeGroup = 14 {"},
		{150, "optional string member = 15;"},
		{154, "message Delta {}"},
	} {
		if want.line > len(lines) || !strings.Contains(lines[want.line-1], want.fragment) {
			t.Fatalf("independent original Proto2 oracle changed at %d: %s", want.line, want.fragment)
		}
	}
	ctx, h := context.Background(), NewHandler([]string{root})
	fail, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "proto", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 160,
	})
	if err != nil || fail != nil || outline.Truncated || outline.FilesParsed != 1 ||
		outline.CoverageComplete {
		t.Fatalf("unmodeled original groups must return a bounded partial outline: %v fail=%+v result=%+v", err, fail, outline)
	}
	present := map[string]bool{}
	for _, symbol := range outline.Symbols {
		present[symbol.QualifiedName] = true
	}
	for _, name := range []string{
		"my.test.Request.key", "my.test.Request.get_key",
		"my.test.Request.hue", "my.test.Communique.maybe",
		"my.test.Communique.Delta",
	} {
		if !present[name] {
			t.Errorf("supported original member missing after group exclusion: %s", name)
		}
	}
	for _, name := range []string{
		"my.test.Request.SomeGroup", "my.test.Request.group_field",
		"my.test.Communique.SomeGroup", "my.test.Communique.member",
	} {
		if present[name] {
			t.Errorf("unmodeled original group falsely exposed as direct member: %s", name)
		}
	}
	fail, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Query: "get_key", Match: "exact",
		Language: "proto", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 10,
	})
	if err != nil || fail != nil || len(found.Files) != 1 || len(found.Symbols) != 1 ||
		found.Symbols[0].QualifiedName != "my.test.Request.get_key" ||
		found.Files[0].SourceFingerprint == "" {
		t.Fatalf("supported original direct field unavailable: %v fail=%+v result=%+v", err, fail, found)
	}
	fail, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "show", Path: path, Language: "proto", Encoding: "utf-8",
		SymbolID: found.Symbols[0].ID, SourceFingerprint: found.Files[0].SourceFingerprint,
		MaxBytes: 4096,
	})
	if err != nil || fail != nil || shown.Show == nil ||
		strings.TrimSpace(shown.Show.Text) != "optional string get_key = 16;" {
		t.Fatalf("fingerprint-bound direct field excerpt incorrect: %v fail=%+v", err, fail)
	}
	fail, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
		Operation: "search", Mode: "structural", Paths: []string{path},
		Query: "group_field", Match: "exact", Language: "proto",
		Encoding: "utf-8", MaxFiles: 1, MaxResults: 20,
	})
	if err != nil || fail != nil || query.Search == nil || len(query.Search.Matches) != 0 {
		t.Fatalf("group field falsely exposed via structural search: %v fail=%+v result=%+v", err, fail, query)
	}
}
