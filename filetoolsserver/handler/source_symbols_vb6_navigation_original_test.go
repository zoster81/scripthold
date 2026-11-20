package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The expectations below come from independent reads of the original projects.
// Each scenario starts from an actual behavior change; source_* supplies evidence
// to the caller but does not select the edit location or infer unknown call sites.
func TestSourceSymbolsOriginalVB6TaskNavigation(t *testing.T) {
	type declaration struct {
		file, name, qualified string
		line                  int
		bodySnippet           string
	}
	scenarios := []struct {
		name, env   string
		scope       []string
		primary     declaration
		adjacent    []declaration
		alternative declaration
	}{
		{
			name: "rental-login-validation-and-roles", env: "SCRIPTHOLD_VB6_NAV_RENTAL_ROOT",
			scope:   []string{"Form4.frm", "Form0_user.frm", "Form1.frm"},
			primary: declaration{"Form4.frm", "Command1_Click", "Form5_login.Command1_Click", 130, "rslogin.Open"},
			adjacent: []declaration{
				{"Form0_user.frm", "cmd_perbarui_Click", "Form0_user.cmd_perbarui_Click", 433, "update login"},
				{"Form1.frm", "cmd_ganti_Click", "Form1_home.cmd_ganti_Click", 236, "Form5_login.Show"},
			},
			alternative: declaration{"Form2.frm", "cmd_simpan_Click", "Form3_mobil.cmd_simpan_Click", 883, ""},
		},
		{
			name: "cybercafe-login-and-password-check", env: "SCRIPTHOLD_VB6_NAV_CYBERCAFE_ROOT",
			scope:   []string{"MARK-2 Trail Version/login.frm", "MARK-2 Trail Version/changepassword.frm", "MARK-2 Trail Version/Homepage.frm"},
			primary: declaration{"MARK-2 Trail Version/login.frm", "true_login", "login.true_login", 175, "Text1.Text = Text2.Text"},
			adjacent: []declaration{
				{"MARK-2 Trail Version/changepassword.frm", "Text1_KeyPress", "password.Text1_KeyPress", 116, "Text1.Text = Text2.Text"},
				{"MARK-2 Trail Version/Homepage.frm", "menuchangepassword_Click", "Homepage.menuchangepassword_Click", 800, "password.Show"},
			},
			alternative: declaration{"MARK-2 Trail Version/Homepage.frm", "menuentry_Click", "Homepage.menuentry_Click", 808, ""},
		},
	}
	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			if root == "" {
				t.Skip("pinned original VB6 project not configured")
			}
			h := NewHandler([]string{root})
			ctx := context.Background()
			paths := make([]string, 0, len(tc.scope))
			for _, rel := range tc.scope {
				p := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(p)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original source must be a regular file: %s err=%v", rel, err)
				}
				paths = append(paths, p)
			}
			failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "vb6", Encoding: "utf-8",
				Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || !digest.CoverageComplete || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("project digest failed: err=%v tool=%+v out=%+v", err, failed, digest)
			}
			encoded, err := json.Marshal(digest)
			if err != nil || len(encoded) > 8192 {
				t.Fatalf("oversized project-first digest (%d bytes): %v", len(encoded), err)
			}
			find := func(d declaration, show bool) {
				t.Helper()
				p := filepath.Join(root, filepath.FromSlash(d.file))
				failed, result, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{p}, Language: "vb6", Encoding: "utf-8",
					Query: d.name, Match: "exact", MaxFiles: 1, MaxSymbols: 8,
				})
				if err != nil || failed != nil || !result.CoverageComplete || result.Truncated || len(result.Symbols) != 1 {
					t.Fatalf("missing/ambiguous %s: err=%v tool=%+v out=%+v", d.name, err, failed, result)
				}
				sym := result.Symbols[0]
				if sym.Name != d.name || sym.QualifiedName != d.qualified || sym.Path != p ||
					sym.DeclarationRange.Start.Line != d.line || string(sym.Kind) != "method" {
					t.Fatalf("wrong source location/owner for %s: %+v", d.name, sym)
				}
				if !show {
					return
				}
				if len(result.Files) != 1 || result.Files[0].SourceFingerprint == "" {
					t.Fatalf("missing source fingerprint for %s", d.name)
				}
				failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: p, SymbolID: sym.ID,
					SourceFingerprint: result.Files[0].SourceFingerprint,
					Language:          "vb6", Encoding: "utf-8", MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, d.bodySnippet) ||
					!strings.Contains(shown.Show.Text, "End Sub") {
					t.Fatalf("exact VB6 body missing for %s: err=%v tool=%+v shown=%+v", d.name, err, failed, shown.Show)
				}
			}
			find(tc.primary, true)
			for _, adjacent := range tc.adjacent {
				find(adjacent, true)
			}
			// A matching declaration in a different feature is not an inferred dependency.
			find(tc.alternative, false)
			failed, missing, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.primary.file))},
				Language: "vb6", Encoding: "utf-8",
				Query: "Height", Match: "exact", MaxFiles: 1, MaxSymbols: 8,
			})
			if err != nil || failed != nil || len(missing.Symbols) != 0 {
				t.Fatalf("VB form designer property was indexed as a declaration: err=%v tool=%+v out=%+v", err, failed, missing)
			}
			failedQuery, matches, queryErr := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Paths: paths, Language: "vb6", Encoding: "utf-8",
				Query: tc.primary.qualified, Match: "exact", Mode: "structural", MaxResults: 8,
			})
			if queryErr != nil || failedQuery != nil || matches.Search == nil ||
				len(matches.Search.Matches) != 1 || matches.Search.Matches[0].QualifiedName != tc.primary.qualified {
				t.Fatalf("structural query failed: err=%v tool=%+v out=%+v", queryErr, failedQuery, matches)
			}
		})
	}
}
