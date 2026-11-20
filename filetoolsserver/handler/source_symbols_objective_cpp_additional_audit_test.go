package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two independent original-project tasks exercise Objective-C++ and its
// embedded C++ function navigation. The original projects are never run.
func TestSourceSymbolsOriginalObjectiveCPPAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Objective-C++ corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "objective-cpp-facebook-fbretaincycledetector", language: "objective-cpp",
			// Investigate why filtering one retained ivar suppresses a
			// reference while filtering two affects both; inspect test cases,
			// detector configuration and an unrelated association manager.
			search:        "FBFilterBlockWithObjectIvarRelation",
			primary:       "FBRetainCycleDetector/Filtering/FBStandardGraphEdgeFilters.mm",
			negativeFile:  "FBRetainCycleDetector/Associations/FBAssociationManager.mm",
			negativeQuery: "FBFilterBlockWithObjectIvarRelation",
			paths: []string{
				"FBRetainCycleDetector/Filtering/FBStandardGraphEdgeFilters.mm",
				"FBRetainCycleDetector/Filtering/FBStandardGraphEdgeFilters.h",
				"FBRetainCycleDetectorTests/FBGraphEdgeFilterTests.m",
				"FBRetainCycleDetector/Detector/FBRetainCycleDetector.mm",
				"FBRetainCycleDetector/Associations/FBAssociationManager.mm",
			},
			oracles: []extraAuditEvidence{
				{"FBRetainCycleDetector/Filtering/FBStandardGraphEdgeFilters.mm", "FBFilterBlockWithObjectIvarRelation(Class aCls, NSString *ivarName)"},
				{"FBRetainCycleDetector/Filtering/FBStandardGraphEdgeFilters.mm", "return FBFilterBlockWithObjectToManyIvarsRelation(aCls, [NSSet setWithObject:ivarName]);"},
				{"FBRetainCycleDetector/Filtering/FBStandardGraphEdgeFilters.mm", "return FBGraphEdgeInvalid;"},
				{"FBRetainCycleDetector/Filtering/FBStandardGraphEdgeFilters.h", "FBFilterBlockWithObjectToManyIvarsRelation(Class _Nonnull aCls,"},
				{"FBRetainCycleDetectorTests/FBGraphEdgeFilterTests.m", "- (void)testIfApplyingFilterForBothPropertiesWillFilterOutBothProperties"},
				{"FBRetainCycleDetectorTests/FBGraphEdgeFilterTests.m", "XCTAssertEqual([retainCycles count], 0);"},
				{"FBRetainCycleDetector/Detector/FBRetainCycleDetector.mm", "initWithFilterBlocks:FBGetStandardGraphEdgeFilters()"},
				{"FBRetainCycleDetector/Associations/FBAssociationManager.mm", "namespace FB { namespace AssociationManager {"},
			},
			targets: []extraAuditSymbol{
				{"FBRetainCycleDetector/Filtering/FBStandardGraphEdgeFilters.mm", "FBFilterBlockWithObjectIvarRelation", 32},
				{"FBRetainCycleDetector/Filtering/FBStandardGraphEdgeFilters.mm", "FBFilterBlockWithObjectToManyIvarsRelation", 36},
				{"FBRetainCycleDetector/Filtering/FBStandardGraphEdgeFilters.mm", "FBGetStandardGraphEdgeFilters", 64},
				{"FBRetainCycleDetectorTests/FBGraphEdgeFilterTests.m", "testIfApplyingFilterForBothPropertiesWillFilterOutBothProperties", 87},
				{"FBRetainCycleDetector/Detector/FBRetainCycleDetector.mm", "init", 40},
			},
		},
		{
			project: "objective-cpp-gree-unity-webview", language: "objective-cpp",
			// Investigate an iOS navigation allow/deny/hook regex decision
			// and native Unity bridge. UIWebView and Mac variants differ.
			search:        "_CWebViewPlugin_SetURLPattern",
			primary:       "plugins/iOS/WebView.mm",
			negativeFile:  "plugins/Mac/Sources/WebView.mm",
			negativeQuery: "webView:shouldStartLoadWithRequest:navigationType:",
			paths: []string{
				"plugins/iOS/WebView.mm",
				"plugins/iOS/WebViewWithUIWebView.mm",
				"plugins/Mac/Sources/WebView.mm",
			},
			oracles: []extraAuditEvidence{
				{"plugins/iOS/WebView.mm", "decidePolicyForNavigationAction:(WKNavigationAction *)navigationAction decisionHandler:"},
				{"plugins/iOS/WebView.mm", "denyRegex != nil && [denyRegex firstMatchInString:url"},
				{"plugins/iOS/WebView.mm", "- (BOOL)setURLPattern:(const char *)allowPattern and:(const char *)denyPattern and:(const char *)hookPattern"},
				{"plugins/iOS/WebView.mm", "return [webViewPlugin setURLPattern:allowPattern and:denyPattern and:hookPattern];"},
				{"plugins/iOS/WebViewWithUIWebView.mm", "shouldStartLoadWithRequest:(NSURLRequest *)request navigationType:"},
				{"plugins/iOS/WebViewWithUIWebView.mm", "- (BOOL)setURLPattern:(const char *)allowPattern and:(const char *)denyPattern and:(const char *)hookPattern"},
				{"plugins/Mac/Sources/WebView.mm", "@interface CNSWindow : NSWindow"},
				{"plugins/WebViewObject.cs", "return _CWebViewPlugin_SetURLPattern(webView, allowPattern, denyPattern, hookPattern);"},
			},
			targets: []extraAuditSymbol{
				{"plugins/iOS/WebView.mm", "_CWebViewPlugin_SetURLPattern", 1153},
				{"plugins/iOS/WebViewWithUIWebView.mm", "_CWebViewPlugin_SetURLPattern", 1235},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
	// An exact three-part selector is not a contiguous substring of its
	// authored declaration. Validate all parts against independent source,
	// then use the fingerprint from the exact-match lookup for bounded show.
	t.Run("gree-unity-webview-three-part-selector", func(t *testing.T) {
		root := filepath.Join(corpus, "objective-cpp-gree-unity-webview")
		h := NewHandler([]string{root})
		files := []struct {
			rel  string
			line int
		}{
			{"plugins/iOS/WebView.mm", 835},
			{"plugins/iOS/WebViewWithUIWebView.mm", 941},
		}
		const exact = "setURLPattern:and:and:"
		paths := make([]string, 0, len(files))
		for _, file := range files {
			path := filepath.Join(root, filepath.FromSlash(file.rel))
			requireAdditionalOriginalFile(t, path)
			data, err := os.ReadFile(path)
			const signature = "- (BOOL)setURLPattern:(const char *)allowPattern and:(const char *)denyPattern and:(const char *)hookPattern"
			if err != nil || !strings.Contains(string(data), signature) {
				t.Fatalf("independent authored keyword selector missing in %s: %v", file.rel, err)
			}
			paths = append(paths, path)
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Query: exact, Match: "exact",
				Language: "objective-cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || failed != nil || found.Truncated || !found.CoverageComplete || len(found.Files) != 1 {
				t.Fatalf("three-part find: err=%v failed=%+v found=%+v", err, failed, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == exact && sym.NameRange.Start.Line == file.line {
					id = sym.ID
				}
			}
			if id == "" {
				t.Fatalf("missing full selector in %s:%d, got=%+v", file.rel, file.line, found.Symbols)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
				Language: "objective-cpp", Encoding: "utf-8", MaxBytes: 16384,
			})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, signature) {
				t.Fatalf("source-backed selector show: err=%v failed=%+v show=%+v", err, failed, shown.Show)
			}
			failed, incorrect, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Query: "setURLPattern:allowPattern:denyPattern:",
				Match: "exact", Language: "objective-cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || failed != nil || len(incorrect.Symbols) != 0 {
				t.Fatalf("misleading argument-name selector in %s: err=%v failed=%+v symbols=%+v", file.rel, err, failed, incorrect.Symbols)
			}
		}
		failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Query: exact, Match: "exact",
			Mode: "structural", Language: "objective-cpp", Encoding: "utf-8", MaxResults: 16,
		})
		if err != nil || failed != nil || searched.Search == nil {
			t.Fatalf("three-part structural search: err=%v failed=%+v results=%+v", err, failed, searched)
		}
		for _, path := range paths {
			found := false
			for _, match := range searched.Search.Matches {
				if match.Name == exact && filepath.Clean(match.Path) == filepath.Clean(path) {
					found = true
				}
			}
			if !found {
				t.Errorf("source_query lacks exact %s in %s", exact, path)
			}
		}
	})
}
