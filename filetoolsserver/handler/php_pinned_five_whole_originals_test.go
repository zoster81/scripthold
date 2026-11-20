package handler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Two independent complete upstream PHP project sources, plus original
// negative-only routing/provider files: every supported declaration is pinned
// by its real physical source range rather than inferred from runtime output.
func TestPHPFiveWholePinnedOriginalDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_PHP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("five independently pinned complete PHP originals not configured")
	}
	cases := []struct {
		file, blob, rows string
		size             int64
		count            int
	}{{file: "ci-home.php", blob: "59343333099a7dcb73a11582de77f78b00e6c5c7", size: 161, rows: "App\\Controllers\u001fnamespace\u001fnamespace\u001f\u001fApp\\Controllers\u001f3\u001f11\u001f3\u001f26\u001f3\u001f1\u001f3\u001f27\u001f3\u001f1\u001f3\u001f26\u001f0\u001f0\u001f0\u001f0\u001fnamespace App\\Controllers\u001f\u001f\u001eHome\u001fclass\u001fclass\u001fApp\\Controllers\u001fApp\\Controllers.Home\u001f5\u001f7\u001f5\u001f11\u001f5\u001f1\u001f11\u001f2\u001f5\u001f1\u001f6\u001f1\u001f6\u001f1\u001f11\u001f2\u001fclass Home extends BaseController\n\u001f\u001f\u001eindex\u001fmethod\u001fmethod-definition\u001fApp\\Controllers.Home\u001fApp\\Controllers.Home.index\u001f7\u001f21\u001f7\u001f26\u001f7\u001f5\u001f10\u001f6\u001f7\u001f5\u001f8\u001f5\u001f8\u001f5\u001f10\u001f6\u001fpublic function index(): string\n    \u001fpublic\u001fpublic", count: 3},
		{file: "ci-base-controller.php", blob: "ab45a7710be92de1809a5e595683fca79491ddd9", size: 1352, rows: "App\\Controllers\u001fnamespace\u001fnamespace\u001f\u001fApp\\Controllers\u001f3\u001f11\u001f3\u001f26\u001f3\u001f1\u001f3\u001f27\u001f3\u001f1\u001f3\u001f26\u001f0\u001f0\u001f0\u001f0\u001fnamespace App\\Controllers\u001f\u001f\u001eBaseController\u001fclass\u001fclass\u001fApp\\Controllers\u001fApp\\Controllers.BaseController\u001f21\u001f16\u001f21\u001f30\u001f21\u001f1\u001f45\u001f2\u001f21\u001f1\u001f22\u001f1\u001f22\u001f1\u001f45\u001f2\u001fabstract class BaseController extends Controller\n\u001f\u001fabstract\u001einitController\u001fmethod\u001fmethod-definition\u001fApp\\Controllers.BaseController\u001fApp\\Controllers.BaseController.initController\u001f33\u001f21\u001f33\u001f35\u001f33\u001f5\u001f44\u001f6\u001f33\u001f5\u001f34\u001f5\u001f34\u001f5\u001f44\u001f6\u001fpublic function initController(RequestInterface $request, ResponseInterface $response, LoggerInterface $logger)\n    \u001fpublic\u001fpublic", count: 3},
		{file: "ci-routes.php", blob: "4cf109ed3dd445137f413a2d7b3c588c5381c8e0", size: 118, rows: "", count: 0},
		{file: "joomla-display-controller.php", blob: "999dde7fe4aae63e7885573a939dd6fef47282d1", size: 707, rows: "Joomla\\Component\\Actionlogs\\Administrator\\Controller\u001fnamespace\u001fnamespace\u001f\u001fJoomla\\Component\\Actionlogs\\Administrator\\Controller\u001f11\u001f11\u001f11\u001f63\u001f11\u001f1\u001f11\u001f64\u001f11\u001f1\u001f11\u001f63\u001f0\u001f0\u001f0\u001f0\u001fnamespace Joomla\\Component\\Actionlogs\\Administrator\\Controller\u001f\u001f\u001eDisplayController\u001fclass\u001fclass\u001fJoomla\\Component\\Actionlogs\\Administrator\\Controller\u001fJoomla\\Component\\Actionlogs\\Administrator\\Controller.DisplayController\u001f24\u001f7\u001f24\u001f24\u001f24\u001f1\u001f33\u001f2\u001f24\u001f1\u001f25\u001f1\u001f25\u001f1\u001f33\u001f2\u001fclass DisplayController extends BaseController\n\u001f\u001f\u001e$default_view\u001fproperty\u001fproperty\u001fJoomla\\Component\\Actionlogs\\Administrator\\Controller.DisplayController\u001fJoomla\\Component\\Actionlogs\\Administrator\\Controller.DisplayController.$default_view\u001f32\u001f15\u001f32\u001f28\u001f32\u001f5\u001f32\u001f44\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001fprotected\u001fprotected", count: 3},
		{file: "joomla-provider.php", blob: "59795f1b6e762e9039f4ca42312c8bc1a0ce91f5", size: 1616, rows: "", count: 0}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, e := os.Lstat(path)
			if e != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("complete PHP original changed stat=%+v err=%v", st, e)
			}
			b, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			sha := sha1.New()
			_, _ = fmt.Fprintf(sha, "blob %d%c", len(b), 0)
			_, _ = sha.Write(b)
			if hex.EncodeToString(sha.Sum(nil)) != tc.blob {
				t.Fatal("full original PHP raw Git blob mismatch")
			}
			fail, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "php", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 90})
			if err != nil || fail != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("whole PHP original source-first symbols expected=%d got=%d coverage=%v err=%v fail=%+v", tc.count, len(out.Symbols), out.CoverageComplete, err, fail)
			}
			location := map[string]int{}
			ids := map[string]string{}
			unique := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, dup := location[loc]; dup {
					t.Errorf("duplicate PHP original source position %s", loc)
				}
				location[loc] = i
				if s.ID == "" || unique[s.ID] {
					t.Errorf("missing/duplicate PHP ID %+v", s)
				}
				unique[s.ID] = true
				ids[s.QualifiedName] = s.ID
			}
			if tc.count > 0 {
				for _, row := range strings.Split(tc.rows, "\x1e") {
					f := strings.Split(row, "\x1f")
					if len(f) != 24 {
						t.Fatalf("invalid PHP original source-first row %q", row)
					}
					n := make([]int, 16)
					for i := range n {
						v, e := strconv.Atoi(f[i+5])
						if e != nil {
							t.Fatal(e)
						}
						n[i] = v
					}
					key := fmt.Sprintf("%d:%d", n[0], n[1])
					idx, found := location[key]
					if !found {
						t.Errorf("missing PHP declaration in original source %q", row)
						continue
					}
					s := out.Symbols[idx]
					if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] ||
						s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] ||
						s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] ||
						s.NameRange.End.Line != n[2] || s.NameRange.End.Column != n[3] ||
						s.DeclarationRange.Start.Line != n[4] || s.DeclarationRange.Start.Column != n[5] ||
						s.DeclarationRange.End.Line != n[6] || s.DeclarationRange.End.Column != n[7] ||
						string(s.Visibility) != f[22] || strings.Join(s.Modifiers, ",") != f[23] {
						t.Errorf("PHP original metadata mismatch source=%q got=%+v", row, s)
					}
					if f[3] == "" && s.ParentID != "" {
						t.Errorf("spurious PHP root owner %+v", s)
					}
					if f[3] != "" && s.ParentID != ids[f[3]] {
						t.Errorf("incorrect PHP parent %+v", s)
					}
					if n[8] > 0 {
						if s.SignatureRange == nil || s.SignatureRange.Start.Line != n[8] ||
							s.SignatureRange.Start.Column != n[9] || s.SignatureRange.End.Line != n[10] ||
							s.SignatureRange.End.Column != n[11] || s.Signature != f[21] {
							t.Errorf("PHP original signature mismatch source=%q got=%+v", row, s)
						}
					} else if s.SignatureRange != nil || s.Signature != "" {
						t.Errorf("phantom PHP signature %+v", s)
					}
					if n[12] > 0 {
						if s.BodyRange == nil || s.BodyRange.Start.Line != n[12] ||
							s.BodyRange.Start.Column != n[13] || s.BodyRange.End.Line != n[14] ||
							s.BodyRange.End.Column != n[15] {
							t.Errorf("PHP original body mismatch source=%q got=%+v", row, s)
						}
					} else if s.BodyRange != nil {
						t.Errorf("phantom PHP body %+v", s)
					}
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("whole PHP fingerprint/size missing")
			}
			if tc.count > 0 {
				first := strings.Split(strings.Split(tc.rows, "\x1e")[0], "\x1f")[0]
				fail, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "php", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxSymbols: 90})
				if e != nil || fail != nil || len(found.Symbols) != 1 {
					t.Fatalf("PHP original find %v %+v %+v", e, fail, found.Symbols)
				}
				fail, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "php", Encoding: "utf-8", SymbolID: found.Symbols[0].ID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
				if e != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, first) {
					t.Fatalf("PHP fingerprint show %v %+v %+v", e, fail, shown.Show)
				}
				fail, q, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "php", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxResults: 90})
				if e != nil || fail != nil || q.Search == nil || len(q.Search.Matches) != 1 || q.Search.Matches[0].SymbolID != found.Symbols[0].ID {
					t.Fatalf("PHP structural search %v %+v %+v", e, fail, q)
				}
			}
			for _, neg := range []string{"use", "return", "require", "get", "namespace_fake"} {
				fail, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "php", Encoding: "utf-8", Query: neg, Match: "exact", MaxFiles: 1, MaxSymbols: 90})
				if e != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("PHP reference/statement invented as symbol %s", neg)
				}
			}
		})
	}
}
