package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run the local MCP handler against two complete, immutable Arduino sketches.
func TestArduinoPinnedWholeOriginalNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_ARDUINO_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two whole Arduino originals are not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file, name, declaration string
		count, line, column     int
	}{
		{"servo-sweep.ino", "myservo", "Servo myservo;", 4, 12, 7},
		{"neopixel-simple.ino", "pixels", "Adafruit_NeoPixel pixels(", 3, 20, 19},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("entire Arduino original unavailable or unsafe: %v %v", info, err)
			}
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "arduino", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != tc.count {
				t.Fatalf("whole Arduino original outline: err=%v failed=%+v symbols=%+v", err, failed, outline.Symbols)
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("missing source fingerprint")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "arduino", Encoding: "utf-8",
				Query: tc.name, Match: "exact", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 {
				t.Fatalf("exact original variable find: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
			}
			symbol := found.Symbols[0]
			if symbol.Name != tc.name || symbol.Kind != "variable" || symbol.NativeKind != "variable" ||
				symbol.SignatureRange != nil || symbol.Signature != "" ||
				symbol.NameRange.Start.Line != tc.line || symbol.NameRange.Start.Column != tc.column {
				t.Fatalf("global object identity and position: %+v", symbol)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "arduino", Encoding: "utf-8",
				SymbolID: symbol.ID, SourceFingerprint: fingerprint, MaxBytes: 16384,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.declaration) {
				t.Fatalf("fingerprint-protected original show: err=%v failed=%+v", err, failed)
			}
			failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "arduino", Encoding: "utf-8", Query: tc.name, Match: "exact",
				MaxFiles: 1, MaxResults: 256,
			})
			if err != nil || failed != nil || searched.Search == nil ||
				len(searched.Search.Matches) != 1 {
				t.Fatalf("whole Arduino original structural search: err=%v failed=%+v", err, failed)
			}
		})
	}
}
