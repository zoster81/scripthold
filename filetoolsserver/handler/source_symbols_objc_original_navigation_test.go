package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each maintenance question, source oracle, neighboring test and separate
// implementation was selected from original code before checking source_*.
func TestSourceSymbolsOriginalObjectiveCFamilyNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Objective-C corpus is not configured")
	}
	type evidence struct{ file, snippet string }
	type target struct {
		file, selector, kind, excerpt string
		line                          int
	}
	cases := []struct {
		project, language, query, negative string
		files                              []string
		oracles                            []evidence
		targets                            []target
	}{
		{
			project: "objective-c-sdwebimage-sdwebimage", language: "objective-c",
			query:    "createDownloaderOperationWithUrl:options:context:",
			negative: "SDWebImage/Core/SDImageCacheConfig.m",
			files: []string{
				"SDWebImage/Core/SDWebImageDownloaderConfig.m",
				"SDWebImage/Core/SDWebImageDownloader.m",
				"Tests/Tests/SDWebImageDownloaderTests.m",
				"SDWebImage/Core/SDImageCacheConfig.m",
			},
			oracles: []evidence{
				{"SDWebImage/Core/SDWebImageDownloaderConfig.m", "_downloadTimeout = 15.0;"},
				{"SDWebImage/Core/SDWebImageDownloaderConfig.m", "config.downloadTimeout = self.downloadTimeout;"},
				{"SDWebImage/Core/SDWebImageDownloader.m", "NSTimeInterval timeoutInterval = self.config.downloadTimeout;"},
				{"SDWebImage/Core/SDWebImageDownloader.m", "timeoutInterval = 15.0;"},
				{"SDWebImage/Core/SDWebImageDownloader.m", "initWithURL:url cachePolicy:cachePolicy timeoutInterval:timeoutInterval"},
				{"Tests/Tests/SDWebImageDownloaderTests.m", "- (void)test04ThatASimpleDownloadWorks"},
			},
			targets: []target{
				{"SDWebImage/Core/SDWebImageDownloaderConfig.m", "init", "constructor", "- (instancetype)init", 24},
				{"SDWebImage/Core/SDWebImageDownloaderConfig.m", "copyWithZone:", "method", "- (id)copyWithZone:", 35},
				{"SDWebImage/Core/SDWebImageDownloader.m", "createDownloaderOperationWithUrl:options:context:", "method", "context:(nullable SDWebImageContext *)context", 290},
				{"Tests/Tests/SDWebImageDownloaderTests.m", "test04ThatASimpleDownloadWorks", "method", "- (void)test04ThatASimpleDownloadWorks", 59},
			},
		},
		{
			project: "objective-c-jdg-mbprogresshud", language: "objective-c",
			query:    "hideAnimated:afterDelay:",
			negative: "Demo/HudDemo/MBHudDemoViewController.m",
			files:    []string{"MBProgressHUD.m", "Demo/HudTests/HudTests.m", "Demo/HudDemo/MBHudDemoViewController.m"},
			oracles: []evidence{
				{"MBProgressHUD.m", "if (self.minShowTime > 0.0 && self.showStarted)"},
				{"MBProgressHUD.m", "timerWithTimeInterval:(self.minShowTime - interv)"},
				{"MBProgressHUD.m", "[self hideUsingAnimation:self.useAnimation];"},
				{"Demo/HudTests/HudTests.m", "- (void)testMinShowTime"},
				{"Demo/HudTests/HudTests.m", "hud.minShowTime = 2.;"},
				{"Demo/HudTests/HudTests.m", "- (void)testGraceTime"},
			},
			targets: []target{
				{"MBProgressHUD.m", "hideAnimated:", "method", "- (void)hideAnimated:(BOOL)animated", 147},
				{"MBProgressHUD.m", "hideAnimated:afterDelay:", "method", "afterDelay:(NSTimeInterval)delay", 167},
				{"MBProgressHUD.m", "handleMinShowTimer:", "method", "- (void)handleMinShowTimer:", 185},
				{"Demo/HudTests/HudTests.m", "testMinShowTime", "method", "- (void)testMinShowTime", 354},
			},
		},
		{
			project: "objective-cpp-zoontek-react-native-permissions", language: "objective-cpp",
			query:    "requestWithResolver:rejecter:",
			negative: "ios/Camera/RNPermissionHandlerCamera.h",
			files: []string{
				"ios/Camera/RNPermissionHandlerCamera.mm",
				"ios/Microphone/RNPermissionHandlerMicrophone.mm",
				"ios/RNPermissions.mm",
				"ios/Camera/RNPermissionHandlerCamera.h",
			},
			oracles: []evidence{
				{"ios/Camera/RNPermissionHandlerCamera.mm", "AVAuthorizationStatusDenied:"},
				{"ios/Camera/RNPermissionHandlerCamera.mm", "return RNPermissionStatusDenied;"},
				{"ios/Camera/RNPermissionHandlerCamera.mm", "requestAccessForMediaType:AVMediaTypeVideo"},
				{"ios/Microphone/RNPermissionHandlerMicrophone.mm", "AVAudioApplicationRecordPermissionDenied"},
				{"ios/RNPermissions.mm", "RNPermissionHandlerCamera handlerUniqueId"},
				{"ios/RNPermissions.mm", "[RNPermissionHandlerCamera new]"},
			},
			targets: []target{
				{"ios/Camera/RNPermissionHandlerCamera.mm", "currentStatus", "method", "- (RNPermissionStatus)currentStatus", 15},
				{"ios/Camera/RNPermissionHandlerCamera.mm", "requestWithResolver:rejecter:", "method", "rejecter:(void (^ _Nonnull)(NSError * _Nonnull))reject", 32},
				{"ios/Microphone/RNPermissionHandlerMicrophone.mm", "requestWithResolver:rejecter:", "method", "rejecter:(void (^ _Nonnull)(NSError * _Nonnull))reject", 41},
			},
		},
		{
			project: "objective-cpp-facebook-componentkit", language: "objective-cpp",
			query:    "initWithImage:attributes:size:",
			negative: "ComponentKitApplicationTests/CKNetworkImageComponentTests.mm",
			files: []string{
				"ComponentKit/Components/CKImageComponent.mm",
				"ComponentKit/Components/CKAutoSizedImageComponent.mm",
				"ComponentKitApplicationTests/CKImageComponentTests.mm",
				"ComponentKitApplicationTests/CKNetworkImageComponentTests.mm",
			},
			oracles: []evidence{
				{"ComponentKit/Components/CKImageComponent.mm", "initWithImage:(UIImage *)image"},
				{"ComponentKit/Components/CKImageComponent.mm", "size:(const RCComponentSize &)size"},
				{"ComponentKit/Components/CKAutoSizedImageComponent.mm", ".size(RCComponentSize::fromCGSize(image.size))"},
				{"ComponentKitApplicationTests/CKImageComponentTests.mm", "- (void)testImageComponentWithFixedSize"},
				{"ComponentKitApplicationTests/CKImageComponentTests.mm", "- (void)testImageComponentWithImageSize"},
				{"ComponentKitApplicationTests/CKNetworkImageComponentTests.mm", "CKNetworkImageComponent"},
			},
			targets: []target{
				{"ComponentKit/Components/CKImageComponent.mm", "initWithImage:attributes:size:", "constructor", "size:(const RCComponentSize &)size", 15},
				{"ComponentKit/Components/CKAutoSizedImageComponent.mm", "initWithImage:attributes:", "constructor", "attributes:(const CKViewComponentAttributeValueMap &)attributes", 19},
				{"ComponentKitApplicationTests/CKImageComponentTests.mm", "testImageComponentWithFixedSize", "method", "- (void)testImageComponentWithFixedSize", 59},
				{"ComponentKitApplicationTests/CKImageComponentTests.mm", "testImageComponentWithImageSize", "method", "- (void)testImageComponentWithImageSize", 48},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original project file unavailable: %s: %v", rel, err)
				}
				paths = append(paths, path)
			}
			for _, oracle := range tc.oracles {
				data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(oracle.file)))
				if err != nil || !strings.Contains(string(data), oracle.snippet) {
					t.Fatalf("independent source oracle %s %q: %v", oracle.file, oracle.snippet, err)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: tc.language, Encoding: "utf-8",
				Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact digest: err=%v failed=%+v digest=%+v", err, failed, digest)
			}
			data, err := json.Marshal(digest)
			if err != nil || len(data) > 8192 {
				t.Fatalf("digest too large: bytes=%d err=%v", len(data), err)
			}
			t.Logf("language=%s files=%d symbols=%d bytes=%d coverageComplete=%v",
				tc.language, digest.FilesParsed, digest.SymbolCount, len(data), digest.CoverageComplete)
			for _, want := range tc.targets {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: want.selector,
					Match: "exact", Language: tc.language, Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find %s: err=%v failed=%+v result=%+v", want.selector, err, failed, found)
				}
				var id string
				for _, sym := range found.Symbols {
					if sym.Name == want.selector && string(sym.Kind) == want.kind &&
						sym.NameRange.Start.Line == want.line {
						id = sym.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing/wrong name-kind-line for %s:%d %s; results=%+v",
						want.file, want.line, want.selector, found.Symbols)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: id,
					SourceFingerprint: found.Files[0].SourceFingerprint,
					Language:          tc.language, Encoding: "utf-8", MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, want.excerpt) {
					t.Fatalf("fingerprint-bound method excerpt %s: err=%v failed=%+v show=%+v",
						want.selector, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Language: tc.language, Encoding: "utf-8",
				Query: tc.query, Match: "exact", Mode: "structural", MaxResults: 20,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("structural search %s: err=%v failed=%+v search=%+v", tc.query, err, failed, search.Search)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.negative))},
				Language: tc.language, Encoding: "utf-8", Query: tc.query, Match: "exact",
				MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("negative scope falsely declares %s: err=%v failed=%+v results=%+v",
					tc.query, err, failed, negative.Symbols)
			}
		})
	}
}
