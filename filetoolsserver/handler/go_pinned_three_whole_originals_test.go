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

// Independent full-source inventories from chi, cobra and gorilla, with Go
// package/type/const/var/function/method names and all physical source ranges.
// Anonymous functions, imports, local bindings and call sites are not symbols.
func TestGoThreeWholePinnedOriginalDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_GO_THREE_WHOLE_ROOT")
	if root == "" {
		t.Skip("three Git-pinned Go originals not configured")
	}
	cases := []struct {
		file, blob, rows string
		size             int64
		count            int
	}{{file: "chi-request-id.go", blob: "e1d4ccb7dad3fb317e92ef30d2f677a9215aa60e", size: 2943, count: 10, rows: "middleware\u001fpackage\u001fpackage\u001f\u001fmiddleware\u001f1\u001f9\u001f1\u001f19\u001f1\u001f1\u001f1\u001f19\u001f1\u001f1\u001f1\u001f19\u001f0\u001f0\u001f0\u001f0\u001fpackage middleware\u001fpackage\u001ectxKeyRequestID\u001ftype\u001fnamed-type\u001fmiddleware\u001fmiddleware.ctxKeyRequestID\u001f18\u001f6\u001f18\u001f21\u001f18\u001f1\u001f18\u001f25\u001f18\u001f1\u001f18\u001f25\u001f0\u001f0\u001f0\u001f0\u001ftype ctxKeyRequestID int\u001fpackage\u001eRequestIDKey\u001fconstant\u001fconst\u001fmiddleware\u001fmiddleware.RequestIDKey\u001f21\u001f7\u001f21\u001f19\u001f21\u001f7\u001f21\u001f39\u001f21\u001f7\u001f21\u001f39\u001f0\u001f0\u001f0\u001f0\u001fRequestIDKey ctxKeyRequestID = 0\u001fpublic\u001eRequestIDHeader\u001fvariable\u001fvar\u001fmiddleware\u001fmiddleware.RequestIDHeader\u001f25\u001f5\u001f25\u001f20\u001f25\u001f5\u001f25\u001f37\u001f25\u001f5\u001f25\u001f37\u001f0\u001f0\u001f0\u001f0\u001fRequestIDHeader = \"X-Request-Id\"\u001fpublic\u001eprefix\u001fvariable\u001fvar\u001fmiddleware\u001fmiddleware.prefix\u001f27\u001f5\u001f27\u001f11\u001f27\u001f5\u001f27\u001f18\u001f27\u001f5\u001f27\u001f18\u001f0\u001f0\u001f0\u001f0\u001fprefix string\u001fpackage\u001ereqid\u001fvariable\u001fvar\u001fmiddleware\u001fmiddleware.reqid\u001f28\u001f5\u001f28\u001f10\u001f28\u001f5\u001f28\u001f24\u001f28\u001f5\u001f28\u001f24\u001f0\u001f0\u001f0\u001f0\u001freqid atomic.Uint64\u001fpackage\u001einit\u001ffunction\u001ffunction\u001fmiddleware\u001fmiddleware.init\u001f46\u001f6\u001f46\u001f10\u001f46\u001f1\u001f60\u001f2\u001f46\u001f1\u001f46\u001f12\u001f46\u001f13\u001f60\u001f2\u001ffunc init()\u001fpackage\u001eRequestID\u001ffunction\u001ffunction\u001fmiddleware\u001fmiddleware.RequestID\u001f67\u001f6\u001f67\u001f15\u001f67\u001f1\u001f79\u001f2\u001f67\u001f1\u001f67\u001f47\u001f67\u001f48\u001f79\u001f2\u001ffunc RequestID(next http.Handler) http.Handler\u001fpublic\u001eGetReqID\u001ffunction\u001ffunction\u001fmiddleware\u001fmiddleware.GetReqID\u001f83\u001f6\u001f83\u001f14\u001f83\u001f1\u001f91\u001f2\u001f83\u001f1\u001f83\u001f42\u001f83\u001f43\u001f91\u001f2\u001ffunc GetReqID(ctx context.Context) string\u001fpublic\u001eNextRequestID\u001ffunction\u001ffunction\u001fmiddleware\u001fmiddleware.NextRequestID\u001f94\u001f6\u001f94\u001f19\u001f94\u001f1\u001f96\u001f2\u001f94\u001f1\u001f94\u001f28\u001f94\u001f29\u001f96\u001f2\u001ffunc NextRequestID() uint64\u001fpublic"},
		{file: "cobra-args.go", blob: "7bf8c389f1f5db19216a265d23b5b21c9e12cfec", size: 4477, count: 13, rows: "cobra\u001fpackage\u001fpackage\u001f\u001fcobra\u001f15\u001f9\u001f15\u001f14\u001f15\u001f1\u001f15\u001f14\u001f15\u001f1\u001f15\u001f14\u001f0\u001f0\u001f0\u001f0\u001fpackage cobra\u001fpackage\u001ePositionalArgs\u001ftype\u001fnamed-type\u001fcobra\u001fcobra.PositionalArgs\u001f22\u001f6\u001f22\u001f20\u001f22\u001f1\u001f22\u001f60\u001f22\u001f1\u001f22\u001f60\u001f0\u001f0\u001f0\u001f0\u001ftype PositionalArgs func(cmd *Command, args []string) error\u001fpublic\u001elegacyArgs\u001ffunction\u001ffunction\u001fcobra\u001fcobra.legacyArgs\u001f28\u001f6\u001f28\u001f16\u001f28\u001f1\u001f39\u001f2\u001f28\u001f1\u001f28\u001f51\u001f28\u001f52\u001f39\u001f2\u001ffunc legacyArgs(cmd *Command, args []string) error\u001fpackage\u001eNoArgs\u001ffunction\u001ffunction\u001fcobra\u001fcobra.NoArgs\u001f42\u001f6\u001f42\u001f12\u001f42\u001f1\u001f47\u001f2\u001f42\u001f1\u001f42\u001f47\u001f42\u001f48\u001f47\u001f2\u001ffunc NoArgs(cmd *Command, args []string) error\u001fpublic\u001eOnlyValidArgs\u001ffunction\u001ffunction\u001fcobra\u001fcobra.OnlyValidArgs\u001f51\u001f6\u001f51\u001f19\u001f51\u001f1\u001f66\u001f2\u001f51\u001f1\u001f51\u001f54\u001f51\u001f55\u001f66\u001f2\u001ffunc OnlyValidArgs(cmd *Command, args []string) error\u001fpublic\u001eNoDuplicateArgs\u001ffunction\u001ffunction\u001fcobra\u001fcobra.NoDuplicateArgs\u001f69\u001f6\u001f69\u001f21\u001f69\u001f1\u001f79\u001f2\u001f69\u001f1\u001f69\u001f56\u001f69\u001f57\u001f79\u001f2\u001ffunc NoDuplicateArgs(cmd *Command, args []string) error\u001fpublic\u001eArbitraryArgs\u001ffunction\u001ffunction\u001fcobra\u001fcobra.ArbitraryArgs\u001f82\u001f6\u001f82\u001f19\u001f82\u001f1\u001f84\u001f2\u001f82\u001f1\u001f82\u001f54\u001f82\u001f55\u001f84\u001f2\u001ffunc ArbitraryArgs(cmd *Command, args []string) error\u001fpublic\u001eMinimumNArgs\u001ffunction\u001ffunction\u001fcobra\u001fcobra.MinimumNArgs\u001f87\u001f6\u001f87\u001f18\u001f87\u001f1\u001f94\u001f2\u001f87\u001f1\u001f87\u001f40\u001f87\u001f41\u001f94\u001f2\u001ffunc MinimumNArgs(n int) PositionalArgs\u001fpublic\u001eMaximumNArgs\u001ffunction\u001ffunction\u001fcobra\u001fcobra.MaximumNArgs\u001f97\u001f6\u001f97\u001f18\u001f97\u001f1\u001f104\u001f2\u001f97\u001f1\u001f97\u001f40\u001f97\u001f41\u001f104\u001f2\u001ffunc MaximumNArgs(n int) PositionalArgs\u001fpublic\u001eExactArgs\u001ffunction\u001ffunction\u001fcobra\u001fcobra.ExactArgs\u001f107\u001f6\u001f107\u001f15\u001f107\u001f1\u001f114\u001f2\u001f107\u001f1\u001f107\u001f37\u001f107\u001f38\u001f114\u001f2\u001ffunc ExactArgs(n int) PositionalArgs\u001fpublic\u001eRangeArgs\u001ffunction\u001ffunction\u001fcobra\u001fcobra.RangeArgs\u001f117\u001f6\u001f117\u001f15\u001f117\u001f1\u001f124\u001f2\u001f117\u001f1\u001f117\u001f48\u001f117\u001f49\u001f124\u001f2\u001ffunc RangeArgs(min int, max int) PositionalArgs\u001fpublic\u001eMatchAll\u001ffunction\u001ffunction\u001fcobra\u001fcobra.MatchAll\u001f127\u001f6\u001f127\u001f14\u001f127\u001f1\u001f136\u001f2\u001f127\u001f1\u001f127\u001f54\u001f127\u001f55\u001f136\u001f2\u001ffunc MatchAll(pargs ...PositionalArgs) PositionalArgs\u001fpublic\u001eExactValidArgs\u001ffunction\u001ffunction\u001fcobra\u001fcobra.ExactValidArgs\u001f142\u001f6\u001f142\u001f20\u001f142\u001f1\u001f144\u001f2\u001f142\u001f1\u001f142\u001f42\u001f142\u001f43\u001f144\u001f2\u001ffunc ExactValidArgs(n int) PositionalArgs\u001fpublic"},
		{file: "gorilla-middleware.go", blob: "a79815ca072cf052b9322cbb046261f0721eb0ca", size: 3507, count: 11, rows: "mux\u001fpackage\u001fpackage\u001f\u001fmux\u001f1\u001f9\u001f1\u001f12\u001f1\u001f1\u001f1\u001f12\u001f1\u001f1\u001f1\u001f12\u001f0\u001f0\u001f0\u001f0\u001fpackage mux\u001fpackage\u001eMiddlewareFunc\u001ftype\u001fnamed-type\u001fmux\u001fmux.MiddlewareFunc\u001f11\u001f6\u001f11\u001f20\u001f11\u001f1\u001f11\u001f52\u001f11\u001f1\u001f11\u001f52\u001f0\u001f0\u001f0\u001f0\u001ftype MiddlewareFunc func(http.Handler) http.Handler\u001fpublic\u001emiddleware\u001finterface\u001finterface\u001fmux\u001fmux.middleware\u001f14\u001f6\u001f14\u001f16\u001f14\u001f1\u001f16\u001f2\u001f14\u001f1\u001f14\u001f26\u001f14\u001f27\u001f16\u001f2\u001ftype middleware interface\u001fpackage\u001eMiddleware\u001fmethod\u001finterface-method\u001fmux.middleware\u001fmux.middleware.Middleware\u001f15\u001f2\u001f15\u001f12\u001f15\u001f2\u001f15\u001f47\u001f15\u001f2\u001f15\u001f47\u001f0\u001f0\u001f0\u001f0\u001fMiddleware(handler http.Handler) http.Handler\u001fpublic\u001eMiddleware\u001fmethod\u001fmethod\u001fmux.MiddlewareFunc\u001fmux.MiddlewareFunc.Middleware\u001f19\u001f26\u001f19\u001f36\u001f19\u001f1\u001f21\u001f2\u001f19\u001f1\u001f19\u001f71\u001f19\u001f72\u001f21\u001f2\u001ffunc (mw MiddlewareFunc) Middleware(handler http.Handler) http.Handler\u001fpublic\u001eUse\u001fmethod\u001fmethod\u001fmux.Router\u001fmux.Router.Use\u001f24\u001f18\u001f24\u001f21\u001f24\u001f1\u001f28\u001f2\u001f24\u001f1\u001f24\u001f44\u001f24\u001f45\u001f28\u001f2\u001ffunc (r *Router) Use(mwf ...MiddlewareFunc)\u001fpublic\u001euseInterface\u001fmethod\u001fmethod\u001fmux.Router\u001fmux.Router.useInterface\u001f31\u001f18\u001f31\u001f30\u001f31\u001f1\u001f33\u001f2\u001f31\u001f1\u001f31\u001f45\u001f31\u001f46\u001f33\u001f2\u001ffunc (r *Router) useInterface(mw middleware)\u001fpackage\u001eUse\u001fmethod\u001fmethod\u001fmux.Route\u001fmux.Route.Use\u001f38\u001f17\u001f38\u001f20\u001f38\u001f1\u001f44\u001f2\u001f38\u001f1\u001f38\u001f50\u001f38\u001f51\u001f44\u001f2\u001ffunc (r *Route) Use(mwf ...MiddlewareFunc) *Route\u001fpublic\u001euseInterface\u001fmethod\u001fmethod\u001fmux.Route\u001fmux.Route.useInterface\u001f47\u001f17\u001f47\u001f29\u001f47\u001f1\u001f49\u001f2\u001f47\u001f1\u001f47\u001f44\u001f47\u001f45\u001f49\u001f2\u001ffunc (r *Route) useInterface(mw middleware)\u001fpackage\u001eCORSMethodMiddleware\u001ffunction\u001ffunction\u001fmux\u001fmux.CORSMethodMiddleware\u001f55\u001f6\u001f55\u001f26\u001f55\u001f1\u001f70\u001f2\u001f55\u001f1\u001f55\u001f52\u001f55\u001f53\u001f70\u001f2\u001ffunc CORSMethodMiddleware(r *Router) MiddlewareFunc\u001fpublic\u001egetAllMethodsForRoute\u001ffunction\u001ffunction\u001fmux\u001fmux.getAllMethodsForRoute\u001f74\u001f6\u001f74\u001f27\u001f74\u001f1\u001f90\u001f2\u001f74\u001f1\u001f74\u001f75\u001f74\u001f76\u001f90\u001f2\u001ffunc getAllMethodsForRoute(r *Router, req *http.Request) ([]string, error)\u001fpackage"}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, e := os.Lstat(path)
			if e != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("Go original changed %+v %v", st, e)
			}
			b, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(b), 0)
			_, _ = hash.Write(b)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("whole Go original raw Git blob changed")
			}
			failed, out, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "go", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 120})
			if e != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("full Go source-first inventory expected=%d got=%d coverage=%v err=%v failed=%+v", tc.count, len(out.Symbols), out.CoverageComplete, e, failed)
			}
			locations := map[string]int{}
			ids := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, duplicate := locations[loc]; duplicate {
					t.Errorf("duplicate physical Go declaration position %s", loc)
				}
				locations[loc] = i
				if s.ID == "" || ids[s.ID] {
					t.Errorf("missing or duplicate Go symbol ID %+v", s)
				}
				ids[s.ID] = true
			}
			for _, row := range strings.Split(tc.rows, "\x1e") {
				f := strings.Split(row, "\x1f")
				if len(f) != 23 {
					t.Fatalf("invalid independently frozen Go row %q", row)
				}
				n := make([]int, 16)
				for i := range n {
					v, e := strconv.Atoi(f[i+5])
					if e != nil {
						t.Fatal(e)
					}
					n[i] = v
				}
				loc := fmt.Sprintf("%d:%d", n[0], n[1])
				idx, exists := locations[loc]
				if !exists {
					t.Errorf("missing authored Go original symbol %q", row)
					continue
				}
				s := out.Symbols[idx]
				if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] ||
					s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] ||
					s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] ||
					s.NameRange.End.Line != n[2] || s.NameRange.End.Column != n[3] ||
					s.DeclarationRange.Start.Line != n[4] || s.DeclarationRange.Start.Column != n[5] ||
					s.DeclarationRange.End.Line != n[6] || s.DeclarationRange.End.Column != n[7] ||
					string(s.Visibility) != f[22] {
					t.Errorf("source-first Go declaration mismatch expected=%q got=%+v", row, s)
				}
				if n[8] > 0 {
					if s.SignatureRange == nil || s.SignatureRange.Start.Line != n[8] ||
						s.SignatureRange.Start.Column != n[9] || s.SignatureRange.End.Line != n[10] ||
						s.SignatureRange.End.Column != n[11] || s.Signature != f[21] {
						t.Errorf("original Go signature mismatch expected=%q got=%+v", row, s)
					}
				} else if s.SignatureRange != nil || s.Signature != "" {
					t.Errorf("unexpected Go signature %+v", s)
				}
				if n[12] > 0 {
					if s.BodyRange == nil || s.BodyRange.Start.Line != n[12] ||
						s.BodyRange.Start.Column != n[13] || s.BodyRange.End.Line != n[14] ||
						s.BodyRange.End.Column != n[15] {
						t.Errorf("original Go body mismatch expected=%q got=%+v", row, s)
					}
				} else if s.BodyRange != nil {
					t.Errorf("unexpected Go body %+v", s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("whole original Go file fingerprint/bytes missing")
			}
			first := strings.Split(strings.Split(tc.rows, "\x1e")[0], "\x1f")[0]
			fail, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "go", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
			if e != nil || fail != nil || len(found.Symbols) != 1 || found.Symbols[0].ID != out.Symbols[0].ID {
				t.Fatalf("Go package find %v %+v %+v", e, fail, found.Symbols)
			}
			fail, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "go", Encoding: "utf-8", SymbolID: found.Symbols[0].ID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
			if e != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, first) {
				t.Fatalf("Go fingerprint-bound show %v %+v %+v", e, fail, shown.Show)
			}
			fail, q, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "go", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxResults: 120})
			if e != nil || fail != nil || q.Search == nil || len(q.Search.Matches) != 1 || q.Search.Matches[0].SymbolID != found.Symbols[0].ID {
				t.Fatalf("Go original structural search %v %+v %+v", e, fail, q)
			}
			for _, negative := range []string{"import", "return", "fmt", "http", "args", "ghostSymbol"} {
				fail, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "go", Encoding: "utf-8", Query: negative, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
				if e != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("Go local reference/import invented as symbol %q", negative)
				}
			}
		})
	}
}
