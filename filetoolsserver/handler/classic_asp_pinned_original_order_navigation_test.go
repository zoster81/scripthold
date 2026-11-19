package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassicASPPinnedWholeBase64AuthoredOutlineOrder(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CLASSIC_ASP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned complete Classic ASP originals not configured")
	}
	path := filepath.Join(root, "classic-asp-base64-1ca2387.asp")
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		Language: "classic-asp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128, IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated ||
		len(outlined.Symbols) != 14 {
		t.Fatalf("complete Base64 original symbols: err=%v failed=%+v symbols=%+v", err, failed, outlined.Symbols)
	}
	want := []string{
		"Base64Encode", "Base64Encode.oXML", "Base64Encode.oNode",
		"Base64Decode", "Base64Decode.oXML", "Base64Decode.oNode",
		"Stream_StringToBinary", "Stream_StringToBinary.adTypeText",
		"Stream_StringToBinary.adTypeBinary", "Stream_StringToBinary.BinaryStream",
		"Stream_BinaryToString", "Stream_BinaryToString.adTypeText",
		"Stream_BinaryToString.adTypeBinary", "Stream_BinaryToString.BinaryStream",
	}
	var encodeXMLID string
	for i, s := range outlined.Symbols {
		if s.QualifiedName != want[i] {
			t.Errorf("original authored order at %d: got %s want %s", i, s.QualifiedName, want[i])
		}
		if s.QualifiedName == "Base64Encode.oXML" {
			encodeXMLID = s.ID
		}
	}
	if encodeXMLID == "" || len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatal("original symbol identity or fingerprint missing")
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "classic-asp", Encoding: "utf-8",
		Query: "oXML", Match: "exact", MaxFiles: 1, MaxSymbols: 8,
	})
	if err != nil || failed != nil || len(found.Symbols) != 2 ||
		found.Symbols[0].QualifiedName != "Base64Encode.oXML" ||
		found.Symbols[1].QualifiedName != "Base64Decode.oXML" {
		t.Fatalf("original two lexical oXML owners: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: encodeXMLID,
		Language: "classic-asp", Encoding: "utf-8",
		SourceFingerprint: outlined.Files[0].SourceFingerprint, MaxBytes: 2048,
	})
	if err != nil || failed != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "Dim oXML, oNode") {
		t.Fatalf("source-backed Dim declaration show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "classic-asp", Encoding: "utf-8",
		Query: "oXML", Match: "exact", Mode: "structural", MaxResults: 8,
	})
	if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) != 2 {
		t.Fatalf("original repeated source structural search: err=%v failed=%+v search=%+v", err, failed, searched.Search)
	}
}

func TestClassicASPPinnedWholeIndependentDirectoryNavigationControl(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CLASSIC_ASP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned complete Classic ASP originals not configured")
	}
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{filepath.Join(root, "classic-asp-directory-86aefb1.asp")},
		Presentation: "detailed", Language: "classic-asp", Encoding: "utf-8",
		MaxFiles: 1, MaxSymbols: 128, IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated ||
		len(outlined.Symbols) != 13 {
		t.Fatalf("complete directory original 13 supported declarations: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
	}
}
