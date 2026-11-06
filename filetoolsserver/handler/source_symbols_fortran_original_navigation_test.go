package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type originalFortranEvidence struct{ file, text string }
type originalFortranTarget struct {
	file, name, signature string
	line                  int
}
type originalFortranCase struct {
	project, query, primary, unrelated string
	files                              []string
	evidence                           []originalFortranEvidence
	targets                            []originalFortranTarget
}

func TestSourceSymbolsOriginalFortranTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Fortran corpus unavailable")
	}
	cases := []originalFortranCase{
		{
			project:   "fortran-fortran-lang-fpm",
			query:     "new_version_from_string",
			primary:   "src/fpm/versioning.f90",
			unrelated: "src/fpm_strings.f90",
			files:     []string{"src/fpm/versioning.f90", "test/fpm_test/test_versioning.f90", "src/fpm/fpm_release.F90", "src/fpm_strings.f90"},
			evidence: []originalFortranEvidence{
				{"src/fpm/versioning.f90", "integer, parameter :: max_limit = 3"},
				{"src/fpm/versioning.f90", "if (nn >= max_limit) then"},
				{"src/fpm/versioning.f90", "Too many subversions found"},
				{"src/fpm/versioning.f90", "call new_version(self, num(:nn))"},
				{"test/fpm_test/test_versioning.f90", "invalid-overflow"},
				{"test/fpm_test/test_versioning.f90", "call new_version(version, \"0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0\", error)"},
				{"src/fpm/fpm_release.F90", "type(version_t) function fpm_version()"},
				{"src/fpm/fpm_release.F90", "call new_version(fpm_version,ver_string,error)"},
				{"src/fpm_strings.f90", "module fpm_strings"},
			},
			targets: []originalFortranTarget{
				{"src/fpm/versioning.f90", "fpm_versioning", "module fpm_versioning", 2},
				{"src/fpm/versioning.f90", "new_version_from_string", "subroutine new_version_from_string(self, string, error)", 77},
				{"src/fpm/versioning.f90", "next", "subroutine next(string, istart, iend, is_number, error)", 128},
				{"test/fpm_test/test_versioning.f90", "test_invalid_overflow", "subroutine test_invalid_overflow(error)", 495},
				{"src/fpm/fpm_release.F90", "fpm_version", "type(version_t) function fpm_version()", 15},
			},
		},
		{
			project:   "fortran-stdlib",
			query:     "int16_radix_sort",
			primary:   "src/sorting/stdlib_sorting_radix_sort.f90",
			unrelated: "example/sorting/example_ord_sort.f90",
			files:     []string{"src/sorting/stdlib_sorting_radix_sort.f90", "example/sorting/example_radix_sort.f90", "example/sorting/example_ord_sort.f90", "example/sorting/example_sort.f90"},
			evidence: []originalFortranEvidence{
				{"src/sorting/stdlib_sorting_radix_sort.f90", "call radix_sort_u16_helper(N, array, buffer)"},
				{"src/sorting/stdlib_sorting_radix_sort.f90", "if (array(1) >= 0 .and. array(N) < 0) then"},
				{"src/sorting/stdlib_sorting_radix_sort.f90", "buffer(1:(N - middle + 1)) = array(middle:N)"},
				{"src/sorting/stdlib_sorting_radix_sort.f90", "if (optval(reverse, .false.)) then"},
				{"example/sorting/example_radix_sort.f90", "arri16 = [-32767, 32767, 0, 0, -3, 2, -3]"},
				{"example/sorting/example_radix_sort.f90", "call radix_sort(arri16, reverse=.true.)"},
				{"example/sorting/example_ord_sort.f90", "call ord_sort(array1, work)"},
				{"example/sorting/example_sort.f90", "call sort(array)"},
				// Fypp templates are read-only independent evidence; not parsed as Fortran.
				{"test/sorting/test_sorting.fypp", "new_unittest('int_radix_sorts', test_int_radix_sorts)"},
				{"test/sorting/test_sorting.fypp", "subroutine test_int_radix_sorts(error)"},
			},
			targets: []originalFortranTarget{
				{"src/sorting/stdlib_sorting_radix_sort.f90", "radix_sort_u8_helper", "subroutine radix_sort_u8_helper(N, arr)", 15},
				{"src/sorting/stdlib_sorting_radix_sort.f90", "radix_sort_u16_helper", "subroutine radix_sort_u16_helper(N, arr, buf)", 36},
				{"src/sorting/stdlib_sorting_radix_sort.f90", "int8_radix_sort", "module subroutine int8_radix_sort(array, reverse)", 201},
				{"src/sorting/stdlib_sorting_radix_sort.f90", "int16_radix_sort", "module subroutine int16_radix_sort(array, work, reverse)", 217},
				{"src/sorting/stdlib_sorting_radix_sort.f90", "int32_radix_sort", "module subroutine int32_radix_sort(array, work, reverse)", 269},
				{"example/sorting/example_radix_sort.f90", "example_radix_sort", "program example_radix_sort", 1},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				p := filepath.Join(root, filepath.FromSlash(rel))
				requireOriginalFortranFile(t, p)
				paths = append(paths, p)
			}
			for _, oracle := range tc.evidence {
				p := filepath.Join(root, filepath.FromSlash(oracle.file))
				requireOriginalFortranFile(t, p)
				body, err := os.ReadFile(p)
				if err != nil || !strings.Contains(string(body), oracle.text) {
					t.Fatalf("independent source oracle %s %q err=%v", oracle.file, oracle.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "fortran", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact digest err=%v failed=%+v digest=%+v", err, failed, digest)
			}
			encoded, err := json.Marshal(digest)
			if err != nil || len(encoded) > 8192 {
				t.Fatalf("compact digest bytes=%d err=%v", len(encoded), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(encoded), digest.CoverageComplete)
			for _, want := range tc.targets {
				p := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{p}, Query: want.name, Match: "exact", Language: "fortran", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 20,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find %s err=%v failed=%+v result=%+v", want.name, err, failed, found)
				}
				id := ""
				for _, symbol := range found.Symbols {
					if symbol.Name == want.name && symbol.NameRange.Start.Line == want.line {
						id = symbol.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("declaration %s:%d %s missing: %+v", want.file, want.line, want.name, found.Symbols)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: p, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
					Language: "fortran", Encoding: "utf-8", MaxBytes: 16384,
				})
				// The bounded Fortran show excerpt contains a signature, not the entire body.
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.signature) {
					t.Fatalf("fingerprint show %s err=%v failed=%+v shown=%+v", want.name, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural", Language: "fortran", Encoding: "utf-8", MaxResults: 25,
			})
			if err != nil || failed != nil || search.Search == nil {
				t.Fatalf("structural query %s err=%v failed=%+v search=%+v", tc.query, err, failed, search.Search)
			}
			primary := filepath.Join(root, filepath.FromSlash(tc.primary))
			saw := false
			for _, match := range search.Search.Matches {
				if match.Name == tc.query && filepath.Clean(match.Path) == primary {
					saw = true
				}
			}
			if !saw {
				t.Fatalf("structural query missed primary %s: %+v", tc.query, search.Search.Matches)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))}, Query: tc.query, Match: "exact",
				Language: "fortran", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 15,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated file defines %s err=%v failed=%+v symbols=%+v", tc.query, err, failed, negative.Symbols)
			}
		})
	}
}

func requireOriginalFortranFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("original file %s unavailable or not regular: %v", path, err)
	}
}
