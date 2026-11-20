package handler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fortranOriginalDeclaration struct {
	name, kind, native, qualified                            string
	line, nameCol, nameEnd, declCol, declEndLine, declEndCol int
	signature                                                string
}

// Every supported module and procedure declaration was independently annotated
// from two complete immutable Git originals before the development outline.
// Fortran exposes logical procedure headers here, not inferred whole-body ranges.
func TestFortranTwoWholePinnedOriginalOutlineInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_FORTRAN_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two independent complete Fortran upstream sources not configured")
	}
	cases := []struct {
		file         string
		size         int
		blob, target string
		want         []fortranOriginalDeclaration
	}{
		{
			file: "fpm.f90", size: 43744, blob: "8b8c9746d4f342352cdce68504113566adc055e0",
			target: "compact_list_all",
			want: []fortranOriginalDeclaration{
				{"fpm", "module", "module", "fpm", 1, 8, 11, 1, 1, 11, "module fpm"},
				{"build_model", "function", "subroutine", "fpm.build_model", 44, 12, 23, 1, 44, 63, "subroutine build_model(model, settings, package_config, error)"},
				{"assemble_flags", "function", "function", "fpm.assemble_flags", 357, 15, 29, 1, 357, 77, "pure function assemble_flags(cli_flag, package_flag, fallback) result(flags)"},
				{"new_compiler_flags", "function", "subroutine", "fpm.new_compiler_flags", 370, 12, 30, 1, 370, 56, "subroutine new_compiler_flags(model, settings, package)"},
				{"check_modules_for_duplicates", "function", "subroutine", "fpm.check_modules_for_duplicates", 422, 12, 40, 1, 422, 65, "subroutine check_modules_for_duplicates(model, duplicates_found)"},
				{"check_module_names", "function", "subroutine", "fpm.check_module_names", 466, 12, 30, 1, 466, 44, "subroutine check_module_names(model, error)"},
				{"cmd_build", "function", "subroutine", "fpm.cmd_build", 558, 12, 21, 1, 558, 31, "subroutine cmd_build(settings)"},
				{"cmd_run", "function", "subroutine", "fpm.cmd_run", 617, 12, 19, 1, 617, 34, "subroutine cmd_run(settings,test)"},
				{"compact_list_all", "function", "subroutine", "fpm.cmd_run.compact_list_all", 789, 16, 32, 5, 789, 34, "subroutine compact_list_all()"},
				{"compact_list", "function", "subroutine", "fpm.cmd_run.compact_list", 815, 16, 28, 5, 815, 30, "subroutine compact_list()"},
				{"delete_skip", "function", "subroutine", "fpm.delete_skip", 832, 12, 23, 1, 832, 43, "subroutine delete_skip(is_unix, build_dir)"},
				{"delete_targets_by_scope", "function", "subroutine", "fpm.delete_targets_by_scope", 849, 12, 35, 1, 849, 76, "subroutine delete_targets_by_scope(targets, scope, scope_name, deleted_any)"},
				{"delete_targets", "function", "subroutine", "fpm.delete_targets", 871, 12, 26, 1, 871, 43, "subroutine delete_targets(settings, error)"},
				{"cmd_clean", "function", "subroutine", "fpm.cmd_clean", 930, 12, 21, 1, 930, 31, "subroutine cmd_clean(settings)"},
				{"sort_executables", "function", "subroutine", "fpm.sort_executables", 984, 17, 33, 1, 984, 56, "pure subroutine sort_executables(target_ID,executables)"},
				{"swap", "function", "subroutine", "fpm.sort_executables.swap", 1008, 26, 30, 5, 1008, 43, "elemental subroutine swap(t1,t2,e1,e2)"},
				{"should_be_run", "function", "function", "fpm.should_be_run", 1025, 18, 31, 1, 1025, 62, "logical function should_be_run(settings,run_scope,exe_target)"},
				{"save_library_path", "function", "function", "fpm.save_library_path", 1063, 10, 27, 1, 1063, 42, "function save_library_path() result(path)"},
				{"set_library_path", "function", "subroutine", "fpm.set_library_path", 1077, 12, 28, 1, 1077, 51, "subroutine set_library_path(model, targets, error)"},
				{"restore_library_path", "function", "subroutine", "fpm.restore_library_path", 1124, 12, 32, 1, 1124, 51, "subroutine restore_library_path(saved_path, error)"},
			},
		},
		{
			file: "kepler_module.f90", size: 29362, blob: "e5ea77f913202be30c9ff06c656957251ad7b46f",
			target: "psi_in",
			want: []fortranOriginalDeclaration{
				{"kepler_module", "module", "module", "kepler_module", 5, 12, 25, 5, 5, 25, "module kepler_module"},
				{"kepler_classical", "function", "subroutine", "kepler_module.kepler_classical", 30, 16, 32, 5, 30, 48, "subroutine kepler_classical(x0, dt, mu, xf)"},
				{"kepde_", "function", "subroutine", "kepler_module.kepler_classical.kepde_", 140, 20, 26, 9, 140, 31, "subroutine kepde_(x,f)"},
				{"d_kepde_", "function", "subroutine", "kepler_module.kepler_classical.d_kepde_", 147, 20, 28, 9, 147, 33, "subroutine d_kepde_(x,f)"},
				{"kepdh_", "function", "subroutine", "kepler_module.kepler_classical.kepdh_", 154, 20, 26, 9, 154, 31, "subroutine kepdh_(x,f)"},
				{"d_kepdh_", "function", "subroutine", "kepler_module.kepler_classical.d_kepdh_", 161, 20, 28, 9, 161, 33, "subroutine d_kepdh_(x,f)"},
				{"kepe", "function", "function", "kepler_module.kepe", 175, 19, 23, 5, 175, 34, "pure function kepe(e, m, ecc)"},
				{"d_kepe", "function", "function", "kepler_module.d_kepe", 193, 19, 25, 5, 193, 33, "pure function d_kepe(e, ecc)"},
				{"kepde", "function", "function", "kepler_module.kepde", 211, 19, 24, 5, 211, 53, "pure function kepde(de, dm, sigma0, sqrta, a, r)"},
				{"d_kepde", "function", "function", "kepler_module.d_kepde", 233, 19, 26, 5, 233, 51, "pure function d_kepde(de, sigma0, sqrta, a, r)"},
				{"kepdh", "function", "function", "kepler_module.kepdh", 253, 19, 24, 5, 253, 53, "pure function kepdh(dh, dn, sigma0, sqrta, a, r)"},
				{"d_kepdh", "function", "function", "kepler_module.d_kepdh", 275, 19, 26, 5, 275, 51, "pure function d_kepdh(dh, sigma0, sqrta, a, r)"},
				{"barker", "function", "function", "kepler_module.barker", 295, 19, 25, 5, 295, 37, "pure function barker(r1, r2, mu)"},
				{"kepds", "function", "function", "kepler_module.kepds", 324, 19, 24, 5, 324, 52, "pure function kepds(ds, dt, r0, vr0, alpha, mu)"},
				{"d_kepds", "function", "function", "kepler_module.d_kepds", 352, 19, 26, 5, 352, 50, "pure function d_kepds(ds, r0, vr0, alpha, mu)"},
				{"stumpff_s", "function", "function", "kepler_module.stumpff_s", 380, 19, 28, 5, 380, 41, "pure function stumpff_s(z) result(s)"},
				{"stumpff_c", "function", "function", "kepler_module.stumpff_c", 402, 19, 28, 5, 402, 41, "pure function stumpff_c(z) result(c)"},
				{"kepler_shepperd", "function", "subroutine", "kepler_module.kepler_shepperd", 432, 16, 31, 5, 432, 52, "subroutine kepler_shepperd(mu,rv1,dt,rv2,istat)"},
				{"kepler_goodyear_stienon_klumpp", "function", "subroutine", "kepler_module.kepler_goodyear_stienon_klumpp", 605, 16, 46, 5, 605, 67, "subroutine kepler_goodyear_stienon_klumpp(rv0,tau,mu,accy,rvf)"},
				{"div", "function", "function", "kepler_module.kepler_goodyear_stienon_klumpp.div", 859, 23, 26, 9, 859, 35, "pure function div(num,den)"},
				{"psi_in", "function", "function", "kepler_module.kepler_goodyear_stienon_klumpp.psi_in", 879, 23, 29, 9, 879, 34, "pure function psi_in(psi)"},
			},
		},
	}
	ctx := context.Background()
	h := NewHandler([]string{root})
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() != int64(tc.size) {
				t.Fatalf("full pinned Fortran original unavailable or changed: stat=%+v err=%v", stat, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Git blob identity, not merely length or filename, protects all source oracles.
			hasher := sha1.New()
			_, _ = fmt.Fprintf(hasher, "blob %d%c", len(data), 0)
			_, _ = hasher.Write(data)
			if hex.EncodeToString(hasher.Sum(nil)) != tc.blob {
				t.Fatal("pinned original Git blob mismatch")
			}
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "fortran", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != len(tc.want) {
				t.Fatalf("complete original Fortran outline: err=%v failure=%+v count=%d want=%d", err, failed, len(outline.Symbols), len(tc.want))
			}
			symbols := make(map[string]int, len(outline.Symbols))
			ids := make(map[string]string, len(outline.Symbols))
			for i, sym := range outline.Symbols {
				if _, duplicate := symbols[sym.QualifiedName]; duplicate {
					t.Errorf("duplicate qualified symbol %q", sym.QualifiedName)
				}
				symbols[sym.QualifiedName] = i
				if sym.ID == "" {
					t.Errorf("missing symbol ID %s", sym.QualifiedName)
				}
				ids[sym.QualifiedName] = sym.ID
			}
			for _, expected := range tc.want {
				i, exists := symbols[expected.qualified]
				if !exists {
					t.Errorf("missing authored Fortran declaration %s", expected.qualified)
					continue
				}
				sym := outline.Symbols[i]
				parent := ""
				if dot := strings.LastIndex(expected.qualified, "."); dot >= 0 {
					parent = expected.qualified[:dot]
				}
				if sym.Name != expected.name || string(sym.Kind) != expected.kind ||
					sym.NativeKind != expected.native || sym.QualifiedName != expected.qualified ||
					sym.ParentQualifiedName != parent || sym.NameRange.Start.Line != expected.line ||
					sym.NameRange.Start.Column != expected.nameCol || sym.NameRange.End.Line != expected.line ||
					sym.NameRange.End.Column != expected.nameEnd ||
					sym.DeclarationRange.Start.Line != expected.line ||
					sym.DeclarationRange.Start.Column != expected.declCol ||
					sym.DeclarationRange.End.Line != expected.declEndLine ||
					sym.DeclarationRange.End.Column != expected.declEndCol ||
					sym.Signature != expected.signature ||
					sym.SignatureRange == nil ||
					sym.SignatureRange.End.Line != expected.declEndLine ||
					sym.SignatureRange.End.Column != expected.declEndCol {
					t.Errorf("source-first Fortran declaration mismatch want=%+v got=%+v", expected, sym)
				}
				if parent == "" && sym.ParentID != "" {
					t.Errorf("unexpected root owner: %+v", sym)
				}
				if parent != "" && sym.ParentID != ids[parent] {
					t.Errorf("broken original parent-ID chain: %+v", sym)
				}
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" || outline.Files[0].SourceBytes != int64(tc.size) {
				t.Fatal("original fingerprint or byte identity missing")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "fortran", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 1 ||
				found.Symbols[0].Name != tc.target || found.Symbols[0].ID == "" {
				t.Fatalf("nested original procedure find: err=%v failure=%+v symbols=%+v", err, failed, found.Symbols)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, SymbolID: found.Symbols[0].ID,
				SourceFingerprint: fingerprint, Language: "fortran", Encoding: "utf-8", MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(strings.ToLower(shown.Show.Text), strings.ToLower(tc.target)) {
				t.Fatalf("fingerprint show nested original: err=%v failure=%+v shown=%+v", err, failed, shown.Show)
			}
			failed, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path}, Language: "fortran",
				Encoding: "utf-8", Query: tc.target, Match: "exact", MaxFiles: 1, MaxResults: 128,
			})
			if err != nil || failed != nil || queried.Search == nil || len(queried.Search.Matches) != 1 ||
				queried.Search.Matches[0].SymbolID != found.Symbols[0].ID {
				t.Fatalf("nested original structural search: err=%v failure=%+v result=%+v", err, failed, queried)
			}
		})
	}
}
