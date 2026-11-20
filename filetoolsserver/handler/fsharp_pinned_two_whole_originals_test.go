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

// Independent complete original inventories for FSharp.Data and Elmish.
// Operator "?" and same-name type/module remain distinct authored entities.
func TestFSharpTwoWholePinnedOriginalDeclarations(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_FSHARP_TWO_WHOLE_ROOT")
	if root == "" {
		t.Skip("two complete Git-pinned FSharp originals not configured")
	}
	cases := []struct {
		file, blob, rows string
		size             int64
		count            int
	}{{file: "fsharp-data-csv-extensions.fs", blob: "f267d3925e2b3e15366939c6036808a469eb0424", size: 5863, count: 13, rows: "FSharp.Data\u001fnamespace\u001fnamespace\u001f\u001fFSharp.Data\u001f3\u001f11\u001f3\u001f22\u001f3\u001f1\u001f3\u001f22\u001f3\u001f1\u001f3\u001f22\u001fnamespace FSharp.Data\u001eStringExtensions\u001ftype\u001ftype\u001fFSharp.Data\u001fFSharp.Data.StringExtensions\u001f14\u001f6\u001f14\u001f22\u001f14\u001f1\u001f14\u001f24\u001f14\u001f1\u001f14\u001f24\u001ftype StringExtensions =\u001eAsInteger\u001fmethod\u001fmember\u001fFSharp.Data.StringExtensions\u001fFSharp.Data.StringExtensions.AsInteger\u001f20\u001f19\u001f20\u001f28\u001f20\u001f5\u001f20\u001f68\u001f20\u001f5\u001f20\u001f68\u001fstatic member AsInteger(x: String, [<Optional>] ?cultureInfo) =\u001eAsInteger64\u001fmethod\u001fmember\u001fFSharp.Data.StringExtensions\u001fFSharp.Data.StringExtensions.AsInteger64\u001f31\u001f19\u001f31\u001f30\u001f31\u001f5\u001f31\u001f70\u001f31\u001f5\u001f31\u001f70\u001fstatic member AsInteger64(x: String, [<Optional>] ?cultureInfo) =\u001eAsDecimal\u001fmethod\u001fmember\u001fFSharp.Data.StringExtensions\u001fFSharp.Data.StringExtensions.AsDecimal\u001f42\u001f19\u001f42\u001f28\u001f42\u001f5\u001f42\u001f68\u001f42\u001f5\u001f42\u001f68\u001fstatic member AsDecimal(x: String, [<Optional>] ?cultureInfo) =\u001eAsFloat\u001fmethod\u001fmember\u001fFSharp.Data.StringExtensions\u001fFSharp.Data.StringExtensions.AsFloat\u001f54\u001f19\u001f54\u001f26\u001f54\u001f5\u001f54\u001f95\u001f54\u001f5\u001f54\u001f95\u001fstatic member AsFloat(x: String, [<Optional>] ?cultureInfo, [<Optional>] ?missingValues) =\u001eAsBoolean\u001fmethod\u001fmember\u001fFSharp.Data.StringExtensions\u001fFSharp.Data.StringExtensions.AsBoolean\u001f65\u001f19\u001f65\u001f28\u001f65\u001f5\u001f65\u001f41\u001f65\u001f5\u001f65\u001f41\u001fstatic member AsBoolean(x: String) =\u001eAsDateTime\u001fmethod\u001fmember\u001fFSharp.Data.StringExtensions\u001fFSharp.Data.StringExtensions.AsDateTime\u001f74\u001f19\u001f74\u001f29\u001f74\u001f5\u001f74\u001f69\u001f74\u001f5\u001f74\u001f69\u001fstatic member AsDateTime(x: String, [<Optional>] ?cultureInfo) =\u001eAsDateTimeOffset\u001fmethod\u001fmember\u001fFSharp.Data.StringExtensions\u001fFSharp.Data.StringExtensions.AsDateTimeOffset\u001f85\u001f19\u001f85\u001f35\u001f85\u001f5\u001f85\u001f67\u001f85\u001f5\u001f85\u001f67\u001fstatic member AsDateTimeOffset(x, [<Optional>] ?cultureInfo) =\u001eAsTimeSpan\u001fmethod\u001fmember\u001fFSharp.Data.StringExtensions\u001fFSharp.Data.StringExtensions.AsTimeSpan\u001f96\u001f19\u001f96\u001f29\u001f96\u001f5\u001f96\u001f69\u001f96\u001f5\u001f96\u001f69\u001fstatic member AsTimeSpan(x: String, [<Optional>] ?cultureInfo) =\u001eAsGuid\u001fmethod\u001fmember\u001fFSharp.Data.StringExtensions\u001fFSharp.Data.StringExtensions.AsGuid\u001f106\u001f19\u001f106\u001f25\u001f106\u001f5\u001f106\u001f38\u001f106\u001f5\u001f106\u001f38\u001fstatic member AsGuid(x: String) =\u001eCsvExtensions\u001fmodule\u001fmodule\u001fFSharp.Data\u001fFSharp.Data.CsvExtensions\u001f113\u001f8\u001f113\u001f21\u001f113\u001f1\u001f113\u001f23\u001f113\u001f1\u001f113\u001f23\u001fmodule CsvExtensions =\u001e?\u001ffunction\u001flet\u001fFSharp.Data.CsvExtensions\u001fFSharp.Data.CsvExtensions.?\u001f116\u001f10\u001f116\u001f11\u001f116\u001f5\u001f116\u001f72\u001f116\u001f5\u001f116\u001f72\u001flet (?) (csvRow: CsvRow) (columnName: string) = csvRow.[columnName]"},
		{file: "elmish-program.fs", blob: "24058814050d32c770b980d4fdb84886bdbed88a", size: 8577, count: 24, rows: "Elmish\u001fnamespace\u001fnamespace\u001f\u001fElmish\u001f8\u001f11\u001f8\u001f17\u001f8\u001f1\u001f8\u001f17\u001f8\u001f1\u001f8\u001f17\u001fnamespace Elmish\u001eProgram\u001ftype\u001ftype\u001fElmish\u001fElmish.Program\u001f12\u001f6\u001f12\u001f13\u001f12\u001f1\u001f12\u001f52\u001f12\u001f1\u001f12\u001f52\u001ftype Program<'arg, 'model, 'msg, 'view> = private {\u001eProgram\u001fmodule\u001fmodule\u001fElmish\u001fElmish.Program\u001f25\u001f8\u001f25\u001f15\u001f25\u001f1\u001f25\u001f17\u001f25\u001f1\u001f25\u001f17\u001fmodule Program =\u001emkProgram\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.mkProgram\u001f27\u001f9\u001f27\u001f18\u001f27\u001f5\u001f27\u001f18\u001f27\u001f5\u001f27\u001f18\u001flet mkProgram\u001emkSimple\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.mkSimple\u001f40\u001f9\u001f40\u001f17\u001f40\u001f5\u001f40\u001f17\u001f40\u001f5\u001f40\u001f17\u001flet mkSimple\u001ewithSubscription\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.withSubscription\u001f55\u001f9\u001f55\u001f25\u001f55\u001f5\u001f55\u001f107\u001f55\u001f5\u001f55\u001f107\u001flet withSubscription (subscribe : 'model -> Sub<'msg>) (program: Program<'arg, 'model, 'msg, 'view>) =\u001emapSubscription\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.mapSubscription\u001f60\u001f9\u001f60\u001f24\u001f60\u001f5\u001f60\u001f76\u001f60\u001f5\u001f60\u001f76\u001flet mapSubscription map (program: Program<'arg, 'model, 'msg, 'view>) =\u001ewithConsoleTrace\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.withConsoleTrace\u001f65\u001f9\u001f65\u001f25\u001f65\u001f5\u001f65\u001f73\u001f65\u001f5\u001f65\u001f73\u001flet withConsoleTrace (program: Program<'arg, 'model, 'msg, 'view>) =\u001ewithTrace\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.withTrace\u001f88\u001f9\u001f88\u001f18\u001f88\u001f5\u001f88\u001f72\u001f88\u001f5\u001f88\u001f72\u001flet withTrace trace (program: Program<'arg, 'model, 'msg, 'view>) =\u001ewithErrorHandler\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.withErrorHandler\u001f98\u001f9\u001f98\u001f25\u001f98\u001f5\u001f98\u001f81\u001f98\u001f5\u001f98\u001f81\u001flet withErrorHandler onError (program: Program<'arg, 'model, 'msg, 'view>) =\u001ewithTermination\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.withTermination\u001f103\u001f9\u001f103\u001f24\u001f103\u001f5\u001f103\u001f126\u001f103\u001f5\u001f103\u001f126\u001flet withTermination (predicate: 'msg -> bool) (terminate: 'model -> unit) (program: Program<'arg, 'model, 'msg, 'view>) =\u001emapTermination\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.mapTermination\u001f108\u001f9\u001f108\u001f23\u001f108\u001f5\u001f108\u001f75\u001f108\u001f5\u001f108\u001f75\u001flet mapTermination map (program: Program<'arg, 'model, 'msg, 'view>) =\u001emapErrorHandler\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.mapErrorHandler\u001f113\u001f9\u001f113\u001f24\u001f113\u001f5\u001f113\u001f76\u001f113\u001f5\u001f113\u001f76\u001flet mapErrorHandler map (program: Program<'arg, 'model, 'msg, 'view>) =\u001eonError\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.onError\u001f118\u001f9\u001f118\u001f16\u001f118\u001f5\u001f118\u001f64\u001f118\u001f5\u001f118\u001f64\u001flet onError (program: Program<'arg, 'model, 'msg, 'view>) =\u001ewithSetState\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.withSetState\u001f122\u001f9\u001f122\u001f21\u001f122\u001f5\u001f122\u001f65\u001f122\u001f5\u001f122\u001f65\u001flet withSetState (setState:'model -> Dispatch<'msg> -> unit)\u001esetState\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.setState\u001f128\u001f9\u001f128\u001f17\u001f128\u001f5\u001f128\u001f65\u001f128\u001f5\u001f128\u001f65\u001flet setState (program: Program<'arg, 'model, 'msg, 'view>) =\u001eview\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.view\u001f132\u001f9\u001f132\u001f13\u001f132\u001f5\u001f132\u001f61\u001f132\u001f5\u001f132\u001f61\u001flet view (program: Program<'arg, 'model, 'msg, 'view>) =\u001einit\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.init\u001f136\u001f9\u001f136\u001f13\u001f136\u001f5\u001f136\u001f61\u001f136\u001f5\u001f136\u001f61\u001flet init (program: Program<'arg, 'model, 'msg, 'view>) =\u001eupdate\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.update\u001f140\u001f9\u001f140\u001f15\u001f140\u001f5\u001f140\u001f63\u001f140\u001f5\u001f140\u001f63\u001flet update (program: Program<'arg, 'model, 'msg, 'view>) =\u001emap\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.map\u001f144\u001f9\u001f144\u001f12\u001f144\u001f5\u001f144\u001f78\u001f144\u001f5\u001f144\u001f78\u001flet map mapInit mapUpdate mapView mapSetState mapSubscribe mapTermination\u001eSubs\u001fmodule\u001fmodule\u001fElmish.Program\u001fElmish.Program.Subs\u001f154\u001f12\u001f154\u001f16\u001f154\u001f5\u001f154\u001f31\u001f154\u001f5\u001f154\u001f31\u001fmodule Subs = Sub.Internal\u001erunWithDispatch\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.runWithDispatch\u001f160\u001f9\u001f160\u001f24\u001f160\u001f5\u001f160\u001f133\u001f160\u001f5\u001f160\u001f133\u001flet runWithDispatch (syncDispatch: Dispatch<'msg> -> Dispatch<'msg>) (arg: 'arg) (program: Program<'arg, 'model, 'msg, 'view>) =\u001erunWith\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.runWith\u001f208\u001f9\u001f208\u001f16\u001f208\u001f5\u001f208\u001f107\u001f208\u001f5\u001f208\u001f107\u001flet runWith (arg: 'arg) (program: Program<'arg, 'model, 'msg, 'view>) = runWithDispatch id arg program\u001erun\u001ffunction\u001flet\u001fElmish.Program\u001fElmish.Program.run\u001f211\u001f9\u001f211\u001f12\u001f211\u001f5\u001f211\u001f79\u001f211\u001f5\u001f211\u001f79\u001flet run (program: Program<unit, 'model, 'msg, 'view>) = runWith () program"}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, e := os.Lstat(path)
			if e != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("original FSharp source changed stat=%+v err=%v", st, e)
			}
			bytes, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			sha := sha1.New()
			_, _ = fmt.Fprintf(sha, "blob %d%c", len(bytes), 0)
			_, _ = sha.Write(bytes)
			if hex.EncodeToString(sha.Sum(nil)) != tc.blob {
				t.Fatal("full independent FSharp original Git blob mismatch")
			}
			fail, out, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "fsharp", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 120})
			if e != nil || fail != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("whole original FSharp inventory want=%d got=%d coverage=%v err=%v fail=%+v", tc.count, len(out.Symbols), out.CoverageComplete, e, fail)
			}
			locations := map[string]int{}
			seen := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, exists := locations[loc]; exists {
					t.Errorf("duplicate FSharp authored source position %s", loc)
				}
				locations[loc] = i
				if s.ID == "" || seen[s.ID] {
					t.Errorf("duplicate or missing FSharp ID %+v", s)
				}
				seen[s.ID] = true
			}
			for _, row := range strings.Split(tc.rows, "\x1e") {
				f := strings.Split(row, "\x1f")
				if len(f) != 18 {
					t.Fatalf("invalid original FSharp oracle row %q", row)
				}
				n := make([]int, 12)
				for i := range n {
					v, e := strconv.Atoi(f[i+5])
					if e != nil {
						t.Fatal(e)
					}
					n[i] = v
				}
				loc := fmt.Sprintf("%d:%d", n[0], n[1])
				idx, ok := locations[loc]
				if !ok {
					t.Errorf("FSharp declaration missing from complete original %q", row)
					continue
				}
				s := out.Symbols[idx]
				if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] ||
					s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] ||
					s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] ||
					s.NameRange.End.Line != n[2] || s.NameRange.End.Column != n[3] ||
					s.DeclarationRange.Start.Line != n[4] || s.DeclarationRange.Start.Column != n[5] ||
					s.DeclarationRange.End.Line != n[6] || s.DeclarationRange.End.Column != n[7] ||
					s.SignatureRange == nil || s.SignatureRange.Start.Line != n[8] ||
					s.SignatureRange.Start.Column != n[9] || s.SignatureRange.End.Line != n[10] ||
					s.SignatureRange.End.Column != n[11] || s.Signature != f[17] {
					t.Errorf("full original FSharp source-first mismatch expected=%q got=%+v", row, s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("FSharp original fingerprint/bytes missing")
			}
			first := strings.Split(strings.Split(tc.rows, "\x1e")[0], "\x1f")[0]
			fail, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "fsharp", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
			if e != nil || fail != nil || len(found.Symbols) != 1 || found.Symbols[0].ID != out.Symbols[0].ID {
				t.Fatalf("FSharp namespace find %v %+v %+v", e, fail, found.Symbols)
			}
			fail, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "fsharp", Encoding: "utf-8", SymbolID: found.Symbols[0].ID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 16000})
			if e != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, first) {
				t.Fatalf("FSharp fingerprint show %v %+v %+v", e, fail, shown.Show)
			}
			fail, q, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "fsharp", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxResults: 120})
			matchedStructural := false
			if q.Search != nil {
				for _, match := range q.Search.Matches {
					if match.SymbolID == found.Symbols[0].ID && match.Name == first {
						matchedStructural = true
					}
				}
			}
			if e != nil || fail != nil || !matchedStructural {
				t.Fatalf("FSharp original structural result missing among project-resolved matches %v %+v %+v", e, fail, q)
			}
			for _, negative := range []string{"namespace_fake", "open", "return", "match", "printfn", "ghostSymbol"} {
				fail, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "fsharp", Encoding: "utf-8", Query: negative, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
				if e != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("FSharp local/reference invented as declaration %q", negative)
				}
			}
		})
	}
}
