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
