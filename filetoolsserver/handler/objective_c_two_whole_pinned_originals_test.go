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

// Two complete independent Objective-C and Objective-C++ Git originals.
// Physical selector spelling, root implementation and all authored methods
// are inventoried without guessing method ownership from call sites.
func TestObjectiveCTwoWholePinnedOriginalSelectorInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_OBJECTIVEC_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two whole Objective-C originals not configured")
	}
	cases := []struct {
		file, language, blob, rows string
		size                       int64
		count                      int
	}{{file: "react-native-RCTAppDelegate.mm", language: "objective-cpp", blob: "32e83f0649a4c62bbe0c8cfac9f1d6e67133b996", size: 2025, rows: "RCTAppDelegate\u001fimplementation\u001f@implementation\u001f\u001fRCTAppDelegate@implementation\u001f31\u001f17\u001f31\u001f31\u001f31\u001f1\u001f31\u001f31\u001f31\u001f1\u001f31\u001f31\u001f@implementation RCTAppDelegate\u001f0\u001einit\u001fconstructor\u001finstance-method\u001fRCTAppDelegate\u001fRCTAppDelegate.init\u001f33\u001f17\u001f33\u001f21\u001f33\u001f1\u001f33\u001f21\u001f33\u001f1\u001f33\u001f21\u001f- (instancetype)init\u001f0\u001eapplication:didFinishLaunchingWithOptions:\u001fmethod\u001finstance-method\u001fRCTAppDelegate\u001fRCTAppDelegate.application:didFinishLaunchingWithOptions:\u001f41\u001f9\u001f41\u001f20\u001f41\u001f1\u001f41\u001f109\u001f41\u001f1\u001f41\u001f109\u001f- (BOOL)application:(UIApplication *)application didFinishLaunchingWithOptions:(NSDictionary *)launchOptions\u001f0\u001eloadReactNativeWindow:\u001fmethod\u001finstance-method\u001fRCTAppDelegate\u001fRCTAppDelegate.loadReactNativeWindow:\u001f52\u001f9\u001f52\u001f30\u001f52\u001f1\u001f52\u001f60\u001f52\u001f1\u001f52\u001f60\u001f- (void)loadReactNativeWindow:(NSDictionary *)launchOptions\u001f0\u001erootViewFactory\u001fmethod\u001finstance-method\u001fRCTAppDelegate\u001fRCTAppDelegate.rootViewFactory\u001f65\u001f25\u001f65\u001f40\u001f65\u001f1\u001f65\u001f40\u001f65\u001f1\u001f65\u001f40\u001f- (RCTRootViewFactory *)rootViewFactory\u001f0", count: 5},
		{file: "ctags-objc-minimal.m", language: "objective-c", blob: "d0011b4b1f54dd71bc7687493e9238207dbd0696", size: 128, rows: "MyClass\u001fimplementation\u001f@implementation\u001f\u001fMyClass@implementation\u001f1\u001f17\u001f1\u001f24\u001f1\u001f1\u001f1\u001f24\u001f1\u001f1\u001f1\u001f24\u001f@implementation MyClass\u001f0\u001emyClassMethod:with:\u001fmethod\u001fclass-method\u001fMyClass\u001fMyClass.myClassMethod:with:\u001f3\u001f9\u001f3\u001f22\u001f3\u001f1\u001f3\u001f47\u001f3\u001f1\u001f3\u001f47\u001f+ (void)myClassMethod:(int)arg1 with:(id)arg2;\u001f0\u001emyInstanceMethod:with:\u001fmethod\u001finstance-method\u001fMyClass\u001fMyClass.myInstanceMethod:with:\u001f4\u001f9\u001f4\u001f25\u001f4\u001f1\u001f4\u001f50\u001f4\u001f1\u001f4\u001f50\u001f- (void)myInstanceMethod:(int)arg1 with:(id)arg2;\u001f0", count: 3}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, err := os.Lstat(path)
			if err != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("complete original Objective-C source changed %+v %v", st, err)
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(bytes), 0)
			_, _ = hash.Write(bytes)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("Objective-C whole original raw Git blob mismatch")
			}
			fail, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: tc.language,
				Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 100,
			})
			if err != nil || fail != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 ||
				len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("complete source-first Objective-C declarations expected=%d got=%d coverage=%v err=%v fail=%+v", tc.count, len(out.Symbols), out.CoverageComplete, err, fail)
			}
			physical := map[string]int{}
			ids := map[string]string{}
			uniqueIDs := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, dup := physical[loc]; dup {
					t.Errorf("duplicate Objective-C physical source position %s", loc)
				}
				physical[loc] = i
				if s.ID == "" || uniqueIDs[s.ID] {
					t.Errorf("missing/duplicate original selector ID %+v", s)
				}
				uniqueIDs[s.ID] = true
				ids[s.QualifiedName] = s.ID
			}
			for _, row := range strings.Split(tc.rows, "\x1e") {
				f := strings.Split(row, "\x1f")
				if len(f) != 19 {
					t.Fatalf("bad independent Objective-C oracle row %q", row)
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
				idx, ok := physical[loc]
				if !ok {
					t.Errorf("missing supported Objective-C selector %q", row)
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
					t.Errorf("Objective-C original source-first range/signature mismatch expected=%q got=%+v", row, s)
				}
				if f[18] == "0" && s.ParentID != "" {
					t.Errorf("unexpected implementation parent ID %+v", s)
				}
				if f[18] == "1" && s.ParentID == "" {
					t.Errorf("missing Objective-C interface owner ID %+v", s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("complete Objective-C original file identity missing")
			}
			target := strings.Split(strings.Split(tc.rows, "\x1e")[0], "\x1f")[0]
			fail, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: tc.language, Encoding: "utf-8", Query: target, Match: "exact", MaxFiles: 1, MaxSymbols: 100})
			if e != nil || fail != nil || len(found.Symbols) != 1 {
				t.Fatalf("original Objective-C find %v %+v %+v", e, fail, found.Symbols)
			}
			fail, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: tc.language, Encoding: "utf-8", SymbolID: found.Symbols[0].ID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
			if e != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target) {
				t.Fatalf("original Objective-C show %v %+v %+v", e, fail, shown.Show)
			}
			fail, q, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: tc.language, Encoding: "utf-8", Query: target, Match: "exact", MaxFiles: 1, MaxResults: 100})
			if e != nil || fail != nil || q.Search == nil || len(q.Search.Matches) != 1 || q.Search.Matches[0].SymbolID != found.Symbols[0].ID {
				t.Fatalf("original Objective-C structural search %v %+v %+v", e, fail, q)
			}
			for _, negative := range []string{"#import", "alloc", "return", "self", "@end"} {
				fail, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: tc.language, Encoding: "utf-8", Query: negative, Match: "exact", MaxFiles: 1, MaxSymbols: 100})
				if e != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("Objective-C code reference invented as declaration %q", negative)
				}
			}
		})
	}
}
