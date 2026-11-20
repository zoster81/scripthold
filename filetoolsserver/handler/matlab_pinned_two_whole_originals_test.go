package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each complete source comes from an independent Git-pinned upstream project.
// The two authored function declarations and forbidden false declarations were
// independently inventoried from original bytes, not the handler output.
func TestMATLABTwoWholePinnedOriginalFunctionNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_MATLAB_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two independent complete MATLAB originals not configured")
	}
	cases := []struct {
		file, name, signature, bodyEvidence string
		size                                int64
		nameStart, nameEnd, signatureEnd    int
	}{
		{"stk-customscalar.m", "Matlab_CustomScalar",
			"function [output] = Matlab_CustomScalar(input)", "output.sca = norm(apoVector);",
			2034, 21, 40, 47},
		{"aft-user-input.m", "user_input",
			"function [parameters, listing_masks] = user_input(parent_d)", "writetable(T,fullfile([parent_d '/output'], 'parameters.txt'))",
			4111, 40, 50, 60},
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatalf("complete original missing: %v", err)
			}
			if !info.Mode().IsRegular() || info.Size() != tc.size {
				t.Fatalf("complete pinned original type/size changed: %+v", info)
			}
			failed, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "matlab", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || outlined.Truncated ||
				!outlined.CoverageComplete || outlined.FilesParsed != 1 ||
				len(outlined.Files) != 1 || len(outlined.Symbols) != 1 {
				t.Fatalf("full whole-original MATLAB outline: err=%v failed=%+v outlined=%+v", err, failed, outlined)
			}
			sym := outlined.Symbols[0]
			if sym.Name != tc.name || sym.QualifiedName != tc.name ||
				string(sym.Kind) != "function" || sym.NativeKind != "function" ||
				sym.ParentID != "" || sym.ParentQualifiedName != "" ||
				sym.NameRange.Start.Line != 1 || sym.NameRange.Start.Column != tc.nameStart ||
				sym.NameRange.End.Line != 1 || sym.NameRange.End.Column != tc.nameEnd ||
				sym.DeclarationRange.Start.Line != 1 || sym.DeclarationRange.Start.Column != 1 ||
				sym.DeclarationRange.End.Line != 1 || sym.DeclarationRange.End.Column != tc.signatureEnd ||
				sym.Signature != tc.signature || sym.SignatureRange == nil ||
				sym.SignatureRange.End.Line != 1 || sym.SignatureRange.End.Column != tc.signatureEnd {
				t.Errorf("original authored MATLAB declaration mismatch: %+v", sym)
			}
			fingerprint := outlined.Files[0].SourceFingerprint
			if sym.ID == "" || fingerprint == "" {
				t.Fatal("source identity missing")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "matlab", Encoding: "utf-8",
				Query: tc.name, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || found.Truncated ||
				len(found.Symbols) != 1 || found.Symbols[0].ID != sym.ID {
				t.Fatalf("original function find mismatch err=%v failed=%+v found=%+v", err, failed, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, SymbolID: sym.ID, SourceFingerprint: fingerprint,
				Language: "matlab", Encoding: "utf-8", MaxBytes: 16384,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.signature) {
				t.Fatalf("original function fingerprint show mismatch err=%v failed=%+v shown=%+v", err, failed, shown.Show)
			}
			// The production implementation may intentionally return only the
			// declaration line for MATLAB: do not infer full body ranges from source.
			failed, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path}, Language: "matlab",
				Encoding: "utf-8", Query: tc.name, Match: "exact", MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || failed != nil || searched.Search == nil ||
				len(searched.Search.Matches) != 1 ||
				searched.Search.Matches[0].SymbolID != sym.ID {
				t.Fatalf("source-backed MATLAB structural search mismatch err=%v failed=%+v searched=%+v", err, failed, searched)
			}
			original, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(original), tc.bodyEvidence) {
				t.Fatalf("original MATLAB source evidence changed: %v", err)
			}
		})
	}
}
