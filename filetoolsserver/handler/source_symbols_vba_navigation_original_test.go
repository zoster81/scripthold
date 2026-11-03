package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceSymbolsVBAAndVB6PublicModuleVariables(t *testing.T) {
	for _, language := range []string{"vba", "vb6"} {
		t.Run(language, func(t *testing.T) {
			root := canonicalHandlerTestDir(t)
			path := filepath.Join(root, "Config.bas")
			source := "Attribute VB_Name = \"Config\"\nPublic JsonOptions As Variant\nPrivate cacheSize As Long\nPublic Const Limit = 4\nPublic Function Parse() As Boolean\nEnd Function\n' Public Ghost As Long\n"
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{root})
			for _, tc := range []struct {
				query string
				line  int
				kind  string
			}{
				{"JsonOptions", 2, "variable"},
				{"cacheSize", 3, "variable"},
				{"Limit", 4, "constant"},
				{"Parse", 5, "method"},
			} {
				fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: language, Encoding: "utf-8",
					Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 8,
				})
				if err != nil || fail != nil || !out.CoverageComplete || len(out.Symbols) != 1 {
					t.Fatalf("%s missing/ambiguous %s: err=%v fail=%+v out=%+v", language, tc.query, err, fail, out)
				}
				got := out.Symbols[0]
				if got.QualifiedName != "Config."+tc.query || got.DeclarationRange.Start.Line != tc.line || string(got.Kind) != tc.kind {
					t.Fatalf("%s wrong %s: %+v", language, tc.query, got)
				}
			}
			fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: language, Encoding: "utf-8",
				Query: "Ghost", Match: "exact", MaxFiles: 1, MaxSymbols: 8,
			})
			if err != nil || fail != nil || len(out.Symbols) != 0 {
				t.Fatalf("fabricated commented symbol: err=%v fail=%+v out=%+v", err, fail, out)
			}
		})
	}
}

// Independent original-source line and body oracles, not derived from tool output.
// These are two distinct projects; no original third-party code is executed.
func TestSourceSymbolsOriginalVBATaskNavigation(t *testing.T) {
	const modernEnv = "SCRIPTHOLD_VBA_NAV_MODERNJSON_PROJECT"
	const classicEnv = "SCRIPTHOLD_VBA_NAV_JSON_PROJECT"
	scenarios := []struct {
		name, env                  string
		files                      []string
		query, qualified, relative string
		line                       int
		snippet                    string
		neighbor, neighborFile     string
		neighborLine               int
		distractor, distractorFile string
		distractorLine             int
	}{
		{
			name: "modern-json-unicode-surrogate-error", env: modernEnv,
			files: []string{"vba_source/Json_Parser.bas", "Tests/Tests_JsonParser.vba", "vba_source/Json_Serializer.bas"},
			query: "JR_ReadUnicodeEscape", qualified: "Json_Parser.JR_ReadUnicodeEscape",
			relative: "vba_source/Json_Parser.bas", line: 513,
			snippet:  "Invalid surrogate pair",
			neighbor: "Test_Parse_UnicodeEscape_InvalidHighWithoutLow", neighborFile: "Tests/Tests_JsonParser.vba", neighborLine: 879,
			distractor: "JsonW_WriteEscaped", distractorFile: "vba_source/Json_Serializer.bas", distractorLine: 598,
		},
		{
			name: "vba-json-unquoted-keys-option", env: classicEnv,
			files: []string{"JsonConverter.bas", "specs/Specs.bas"},
			query: "json_ParseKey", qualified: "JsonConverter.json_ParseKey",
			relative: "JsonConverter.bas", line: 637,
			snippet:  "JsonOptions.AllowUnquotedKeys",
			neighbor: "Specs", neighborFile: "specs/Specs.bas", neighborLine: 2,
			distractor: "ConvertToJson", distractorFile: "JsonConverter.bas", distractorLine: 199,
		},
	}
	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			if root == "" {
				t.Skip("original project root not provided")
			}
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("original file missing: %s: %v", rel, err)
				}
				paths = append(paths, path)
			}
			ctx := context.Background()
			fail, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "vba", Encoding: "utf-8",
				Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || fail != nil || !digest.CoverageComplete || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact project digest: err=%v fail=%+v result=%+v", err, fail, digest)
			}
			compact, err := json.Marshal(digest)
			if err != nil || len(compact) > 8192 {
				t.Fatalf("digest not compact: bytes=%d err=%v", len(compact), err)
			}
			fail, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: paths, Language: "vba", Encoding: "utf-8",
				Query: tc.query, Match: "exact", MaxFiles: len(paths), MaxSymbols: 16,
			})
			if err != nil || fail != nil || !out.CoverageComplete || len(out.Symbols) != 1 {
				t.Fatalf("target search: err=%v fail=%+v out=%+v", err, fail, out)
			}
			target := out.Symbols[0]
			original := filepath.Join(root, filepath.FromSlash(tc.relative))
			if target.QualifiedName != tc.qualified || target.Path != original || target.DeclarationRange.Start.Line != tc.line {
				t.Fatalf("wrong maintenance edit-area candidate: %+v", target)
			}
			var fingerprint string
			for _, f := range out.Files {
				if f.Path == original {
					fingerprint = f.SourceFingerprint
				}
			}
			if fingerprint == "" {
				t.Fatalf("target missing source fingerprint: %+v", out.Files)
			}
			fail, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: original, Language: "vba", Encoding: "utf-8",
				SymbolID: target.ID, SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.snippet) {
				t.Fatalf("wrong/incomplete source body: err=%v fail=%+v show=%+v", err, fail, shown.Show)
			}
			// Independently checked adjacent test and alternative serializer declaration.
			for _, nearby := range []struct {
				name, file string
				line       int
				kind       string
			}{
				{tc.neighbor, tc.neighborFile, tc.neighborLine, "test"},
				{tc.distractor, tc.distractorFile, tc.distractorLine, "distractor"},
			} {
				nearPath := filepath.Join(root, filepath.FromSlash(nearby.file))
				fail, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{nearPath}, Language: "vba", Encoding: "utf-8",
					Query: nearby.name, Match: "exact", MaxFiles: 1, MaxSymbols: 8,
				})
				if err != nil || fail != nil || !found.CoverageComplete {
					t.Fatalf("%s search failed: err=%v fail=%+v out=%+v", nearby.kind, err, fail, found)
				}
				present := false
				for _, sym := range found.Symbols {
					if sym.Name == nearby.name && sym.Path == nearPath && sym.DeclarationRange.Start.Line == nearby.line {
						present = true
					}
				}
				if !present {
					t.Fatalf("%s source-verified declaration not discovered: %+v", nearby.kind, found.Symbols)
				}
				if nearby.kind == "distractor" && target.Path == nearPath && target.Name == nearby.name {
					t.Fatalf("unrelated serialization function selected as parser edit area: %+v", target)
				}
			}
			if tc.env == classicEnv {
				fail, option, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{original}, Language: "vba", Encoding: "utf-8",
					Query: "JsonOptions", Match: "exact", MaxFiles: 1, MaxSymbols: 8,
				})
				if err != nil || fail != nil || len(option.Symbols) != 1 ||
					option.Symbols[0].QualifiedName != "JsonConverter.JsonOptions" ||
					option.Symbols[0].DeclarationRange.Start.Line != 158 {
					t.Fatalf("public configuration declaration is missing: err=%v fail=%+v out=%+v", err, fail, option)
				}
			}
		})
	}
}
