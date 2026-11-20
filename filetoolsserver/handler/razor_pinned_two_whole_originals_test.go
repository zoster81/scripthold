package handler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Two complete independently pinned upstream Razor originals were inventoried
// before development output. Only the authored C# property in @functions is
// supported; directives, statement blocks, tag helpers and HTML without ids
// are not declarations.
func TestRazorTwoWholePinnedOriginalPositiveAndEmptyInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_RAZOR_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete upstream Razor original files not configured")
	}
	cases := []struct {
		file, blob string
		size       int64
		want       int
	}{
		{"aspnetcore-PagesHome.cshtml", "d4d5241a6072f8c5e8f30d0f7df17a7de6fb8528", 304, 1},
		{"RazorPages-Add.cshtml", "7fcfc0f458ad64ee87149478789faf91006e1caf", 1862, 0},
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != tc.size {
				t.Fatalf("complete upstream original changed: info=%+v err=%v", info, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(b), 0)
			_, _ = hash.Write(b)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("complete original Git blob changed")
			}
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "razor", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != tc.want {
				t.Fatalf("complete independently authored Razor inventory expected=%d err=%v failed=%+v outline=%+v", tc.want, err, failed, outline)
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" || outline.Files[0].SourceBytes != tc.size {
				t.Fatal("whole original Razor source fingerprint/bytes absent")
			}
			if tc.want == 0 {
				// The full upstream page has @page, @model, @{ }, @section,
				// asp-for/model references and unlabelled HTML, but no declarations.
				failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "razor", Encoding: "utf-8",
					Query: "BoardGame", Match: "exact", MaxFiles: 1, MaxSymbols: 32,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 0 {
					t.Fatalf("Razor negative original must not invent model declarations: err=%v failed=%+v found=%+v", err, failed, found)
				}
				return
			}
			s := outline.Symbols[0]
			if s.Name != "Message" || s.QualifiedName != "Message" ||
				string(s.Kind) != "property" || s.NativeKind != "property" ||
				s.Language != "csharp" || s.ParentID != "" ||
				s.ParentQualifiedName != "" || s.RegionID == "" || s.ID == "" ||
				s.NameRange.Start.Line != 11 || s.NameRange.Start.Column != 19 ||
				s.NameRange.End.Line != 11 || s.NameRange.End.Column != 26 ||
				s.DeclarationRange.Start.Line != 10 || s.DeclarationRange.Start.Column != 5 ||
				s.DeclarationRange.End.Line != 11 || s.DeclarationRange.End.Column != 40 ||
				s.Signature != "public string Message " ||
				s.SignatureRange == nil || s.SignatureRange.Start.Line != 11 ||
				s.SignatureRange.End.Line != 11 || s.SignatureRange.End.Column != 27 {
				t.Errorf("original Razor @functions C# authored property/source attribute scope mismatch %+v", s)
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "razor", Encoding: "utf-8",
				Query: "Message", Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || found.Truncated ||
				len(found.Symbols) != 1 || found.Symbols[0].ID != s.ID {
				t.Fatalf("original Razor property find: err=%v failed=%+v found=%+v", err, failed, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "razor", Encoding: "utf-8",
				SymbolID: s.ID, SourceFingerprint: fingerprint, MaxBytes: 4096,
			})
			if err != nil || failed != nil || shown.Show == nil {
				t.Fatalf("original Razor fingerprint bound show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
			}
			failed, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "razor", Encoding: "utf-8", Query: "Message", Match: "exact",
				MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || failed != nil || query.Search == nil ||
				len(query.Search.Matches) != 1 || query.Search.Matches[0].SymbolID != s.ID {
				t.Fatalf("original Razor property structural search: err=%v failed=%+v matches=%+v", err, failed, query)
			}
		})
	}
}
