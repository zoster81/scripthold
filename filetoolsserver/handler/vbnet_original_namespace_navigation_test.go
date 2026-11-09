package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original projects are inspected read-only and need explicit local opt-in.
func TestVBNetOriginalDottedNamespaceNavigation(t *testing.T) {
	cases := []struct {
		name, env, query, qualified, file, snippet string
		line                                       int
		paths                                      []string
	}{
		{
			name: "community-csv-parser", env: "SCRIPTHOLD_REAL_VBNET_COMMUNITY_ROOT",
			query: "ReadFields", qualified: "Community.VisualBasic.FileIO.TextFieldParser.ReadFields",
			file: "TextFieldParser.vb", snippet: "Public Function ReadFields", line: 345,
			paths: []string{"Community.VisualBasic/FileIO/TextFieldParser.vb",
				"Community.VisualBasic.Tests/Community/VisualBasic/FileIO/TextFieldParserTests.vb",
				"Community.VisualBasic/FileIO/FileSystem.vb"},
		},
		{
			name: "small-visual-basic-completion", env: "SCRIPTHOLD_REAL_VBNET_SVB_ROOT",
			query: "CommitItem", qualified: "Microsoft.SmallVisualBasic.LanguageService.CompletionProvider.CommitItem",
			file: "CompletionProvider.vb", snippet: "Public Sub CommitItem", line: 155,
			paths: []string{"LangServices/CompletionProvider.vb", "LangServices/CompletionKeyboardFilter.vb",
				"LangServices/CompletionItemWrapper.vb", "SBCompiler/SBCompiler/LineScanner.vb"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			if root == "" {
				t.Skip("original project not configured")
			}
			paths := make([]string, 0, len(tc.paths))
			for _, rel := range tc.paths {
				paths = append(paths, filepath.Join(root, filepath.FromSlash(rel)))
			}
			h := NewHandler([]string{root})
			tool, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: paths, Language: "vbnet", Query: tc.query,
				Match: "exact", MaxFiles: len(paths), MaxSymbols: 32, FileDetails: "relevant",
			})
			if err != nil || tool != nil || found.FilesParsed != len(paths) || !found.CoverageComplete {
				t.Fatalf("incomplete find: tool=%v err=%v files=%d complete=%v", tool, err, found.FilesParsed, found.CoverageComplete)
			}
			var path, symbolID, fingerprint, encoding string
			for _, sym := range found.Symbols {
				if sym.QualifiedName == tc.qualified && sym.NameRange.Start.Line == tc.line && filepath.Base(sym.Path) == tc.file {
					path, symbolID = sym.Path, sym.ID
					break
				}
			}
			if symbolID == "" {
				t.Fatalf("missing qualified declaration %s (matches=%d)", tc.qualified, len(found.Symbols))
			}
			for _, f := range found.Files {
				if f.Path == path {
					fingerprint, encoding = f.SourceFingerprint, f.Encoding
				}
			}
			if fingerprint == "" || encoding == "" {
				t.Fatal("exact source selector absent")
			}
			showTool, show, showErr := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "vbnet", Encoding: encoding,
				SymbolID: symbolID, SourceFingerprint: fingerprint, MaxBytes: 16384,
			})
			if showErr != nil || showTool != nil || show.Show == nil || !strings.Contains(show.Show.Text, tc.snippet) {
				t.Fatalf("exact show failed: tool=%v err=%v", showTool, showErr)
			}
			searchTool, search, searchErr := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: paths, Language: "vbnet",
				Query: tc.query, Match: "exact", MaxFiles: len(paths), MaxResults: 32,
			})
			if searchErr != nil || (searchTool != nil && searchTool.IsError) || search.Search == nil ||
				search.Coverage.FilesParsed != len(paths) || search.Coverage.ResultsTruncated {
				t.Fatalf("structural search incomplete: tool=%v err=%v coverage=%+v", searchTool, searchErr, search.Coverage)
			}
			matched := false
			for _, item := range search.Search.Matches {
				if item.QualifiedName == tc.qualified && item.SymbolID == symbolID && item.SourceFingerprint == fingerprint {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("structural search lost %s", tc.qualified)
			}
		})
	}
}
