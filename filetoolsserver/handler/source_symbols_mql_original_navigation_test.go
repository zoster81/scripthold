package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

type originalMQLTarget struct {
	file, name, kind string
	line             int
	show             bool
}

type originalMQLText struct{ file, text string }

type originalMQLNavigationCase struct {
	name, project, language, query, unrelated string
	files                                     []string
	source                                    []originalMQLText
	targets                                   []originalMQLTarget
}

// These maintenance oracles were selected through independent source reads and grep.
// The test runs only Scripthold's local Go handlers, never third-party projects.
func TestSourceSymbolsOriginalMQLTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original project corpus not configured")
	}
	cases := []originalMQLNavigationCase{
		{
			name:    "mql4-momentum-new-bar-evaluation",
			project: "mql4-execution-framework", language: "mql4",
			query: "initTradingModule", unrelated: "include/Inform.mqh",
			files: []string{"experts/MomentumEA.mq4", "include/TradingCriterionMomentumEA.mqh", "include/Trade.mqh", "experts/Skeleton.mq4"},
			source: []originalMQLText{
				{"experts/MomentumEA.mq4", "extern bool evaluateAtOpen = false;"},
				{"experts/MomentumEA.mq4", "if(!evaluateAtOpen || (evaluateAtOpen && newBar()))"},
				{"include/TradingCriterionMomentumEA.mqh", "evaluateAtOpen = TRUE;"},
				{"experts/MomentumEA.mq4", "#include <TradingCriterionMomentumEA.mqh>"},
				{"experts/Skeleton.mq4", "#include <TradingCriterionSkeleton.mqh>"},
			},
			targets: []originalMQLTarget{
				{"experts/MomentumEA.mq4", "evaluateAtOpen", "variable", 21, false},
				{"experts/MomentumEA.mq4", "start", "function", 50, false},
				{"include/TradingCriterionMomentumEA.mqh", "initTradingModule", "function", 25, true},
				{"include/TradingCriterionMomentumEA.mqh", "criterion", "function", 42, false},
				{"include/Trade.mqh", "trade", "function", 15, false},
			},
		},
		{
			name:    "mql4-account-margin-percent-and-cross-dialect-test",
			project: "mql-ea31337-classes", language: "mql4",
			query: "GetMarginUsedInPct", unrelated: "Account/AccountForex.h",
			files: []string{"Account/AccountMt.h", "Account/tests/AccountMt.test.mq4", "Account/tests/AccountMt.test.mq5", "Account/AccountForex.h"},
			source: []originalMQLText{
				{"Account/AccountMt.h", "double GetMarginUsedInPct()"},
				{"Account/AccountMt.h", "return _margin_avail > 0 ? 100 / _margin_avail * _margin_used : 100;"},
				{"Account/tests/AccountMt.test.mq4", "#include \"AccountMt.test.mq5\""},
				{"Account/tests/AccountMt.test.mq5", "assertTrueOrFail(acc.GetMarginUsed() == 0"},
				{"Account/tests/AccountMt.test.mq5", "assertTrueOrFail(acc.GetMarginAvail() == _balance"},
			},
			targets: []originalMQLTarget{
				{"Account/AccountMt.h", "GetMarginUsedInPct", "method", 191, true},
				{"Account/AccountMt.h", "GetMarginFreeInPct", "method", 213, false},
				{"Account/tests/AccountMt.test.mq5", "OnInit", "function", 34, false},
			},
		},
		{
			name:    "mql5-adx-strength-entry-gate",
			project: "mql5-argus", language: "mql5",
			query: "ADX_Threshold", unrelated: "Bollinger_MeanReversion/BollingerMeanReversionEA.mq5",
			files: []string{"ADX_TrendStrength/ADXTrendStrengthEA.mq5", "ADX_TrendStrength/ADXUtils.mqh", "Shared/ArgusCore.mqh", "Bollinger_MeanReversion/BollingerMeanReversionEA.mq5"},
			source: []originalMQLText{
				{"ADX_TrendStrength/ADXTrendStrengthEA.mq5", "ADX_Threshold        = 25.0;"},
				{"ADX_TrendStrength/ADXTrendStrengthEA.mq5", "if(adx_main[0] < ADX_Threshold) return;"},
				{"ADX_TrendStrength/ADXTrendStrengthEA.mq5", "CADXUtils::IsRising(adx_handle, 1)"},
				{"ADX_TrendStrength/ADXTrendStrengthEA.mq5", "CArgusCore::ExecuteTradeWithRetry"},
				{"ADX_TrendStrength/ADXTrendStrengthEA.mq5", "#include \"ADXUtils.mqh\""},
			},
			targets: []originalMQLTarget{
				{"ADX_TrendStrength/ADXTrendStrengthEA.mq5", "ADX_Threshold", "variable", 24, true},
				{"ADX_TrendStrength/ADXTrendStrengthEA.mq5", "OnTick", "function", 77, false},
				{"ADX_TrendStrength/ADXUtils.mqh", "IsRising", "method", 17, false},
			},
		},
		{
			name:    "mql5-telegram-request-timeout-utf16",
			project: "mql5-telegram", language: "mql5",
			query: "PostRequest", unrelated: "include/Comment.mqh",
			files: []string{"MT5 to Telegram.mq5", "include/Telegram.mqh", "include/Common.mqh", "include/Comment.mqh"},
			source: []originalMQLText{
				{"MT5 to Telegram.mq5", "#include <Telegram.mqh>"},
				{"MT5 to Telegram.mq5", "bot.GetUpdates();"},
				{"include/Telegram.mqh", "const int timeout=5000)"},
				{"include/Telegram.mqh", "const int _timeout=10000)"},
				{"include/Telegram.mqh", "WebRequest(\"POST\",url,NULL,NULL,timeout"},
			},
			targets: []originalMQLTarget{
				{"MT5 to Telegram.mq5", "OnTimer", "function", 87, false},
				{"include/Telegram.mqh", "PostRequest", "method", 217, true},
				{"include/Telegram.mqh", "SendPhoto", "method", 671, false},
				{"include/Telegram.mqh", "SendPhoto", "method", 828, false},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				p := filepath.Join(root, filepath.FromSlash(rel))
				verifyOriginalMQLRegularFile(t, p)
				paths = append(paths, p)
			}
			for _, expected := range tc.source {
				p := filepath.Join(root, filepath.FromSlash(expected.file))
				verifyOriginalMQLRegularFile(t, p)
				text := readOriginalMQLSource(t, p)
				if !strings.Contains(text, expected.text) {
					t.Fatalf("original source oracle changed: %s lacks %q", expected.file, expected.text)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: tc.language,
				Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact digest: err=%v failed=%+v digest=%+v", err, failed, digest)
			}
			raw, err := json.Marshal(digest)
			if err != nil || len(raw) > 8192 {
				t.Fatalf("compact response bytes=%d error=%v", len(raw), err)
			}
			t.Logf("files=%d symbols=%d bytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(raw), digest.CoverageComplete)
			for _, target := range tc.targets {
				p := filepath.Join(root, filepath.FromSlash(target.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{p}, Language: tc.language,
					Query: target.name, Match: "exact", MaxFiles: 1, MaxSymbols: 25,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find %s: err=%v failed=%+v result=%+v", target.name, err, failed, found)
				}
				id := ""
				for _, symbol := range found.Symbols {
					if symbol.Name == target.name && string(symbol.Kind) == target.kind && symbol.NameRange.Start.Line == target.line {
						id = symbol.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing source-confirmed declaration %s:%d %s (%s): %+v", target.file, target.line, target.name, target.kind, found.Symbols)
				}
				if target.show {
					failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
						Operation: "show", Path: p, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
						Language: tc.language, Encoding: found.Files[0].Encoding, MaxBytes: 8192,
					})
					if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.name) {
						t.Fatalf("fingerprint show %s: err=%v failed=%+v show=%+v", target.name, err, failed, shown.Show)
					}
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Language: tc.language, Query: tc.query,
				Match: "exact", Mode: "structural", MaxResults: 25,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("structural search %s: err=%v failed=%+v result=%+v", tc.query, err, failed, search.Search)
			}
			unrelated := filepath.Join(root, filepath.FromSlash(tc.unrelated))
			verifyOriginalMQLRegularFile(t, unrelated)
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{unrelated}, Language: tc.language,
				Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 10,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated file falsely defines %s: err=%v failed=%+v symbols=%+v", tc.query, err, failed, negative.Symbols)
			}
		})
	}
}

func verifyOriginalMQLRegularFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("original file unavailable or not regular %s: %v", path, err)
	}
}

func readOriginalMQLSource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) >= 2 && data[0] == 0xff && data[1] == 0xfe {
		if (len(data)-2)%2 != 0 {
			t.Fatalf("odd UTF-16LE source byte length for %s", path)
		}
		units := make([]uint16, (len(data)-2)/2)
		for i := range units {
			units[i] = uint16(data[2+2*i]) | uint16(data[3+2*i])<<8
		}
		return string(utf16.Decode(units))
	}
	return string(data)
}
