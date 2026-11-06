package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type svelteOriginalEvidence struct{ file, text string }
type svelteOriginalTarget struct {
	file, symbol string
	line         int
}
type svelteOriginalScenario struct {
	project, search, primary, unrelated      string
	files                                    []string
	oracles                                  []svelteOriginalEvidence
	targets                                  []svelteOriginalTarget
	linkedFile, linkedLanguage, linkedSymbol string
	linkedLine                               int
}

func TestSourceSymbolsOriginalSvelteTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Svelte corpus unavailable")
	}
	cases := []svelteOriginalScenario{
		{
			project: "svelte-flowbite", search: "handleToggle",
			primary:   "src/lib/accordion/AccordionItem.svelte",
			unrelated: "src/lib/darkmode/DarkMode.svelte",
			files: []string{
				"src/lib/accordion/Accordion.svelte", "src/lib/accordion/AccordionItem.svelte",
				"src/lib/tabs/Tabs.svelte", "src/lib/darkmode/DarkMode.svelte",
			},
			oracles: []svelteOriginalEvidence{
				{"src/lib/accordion/Accordion.svelte", "multiple = false"},
				{"src/lib/accordion/Accordion.svelte", "createSingleSelectionContext(untrack(() => multiple))"},
				{"src/lib/accordion/AccordionItem.svelte", "useSingleSelection<symbol>"},
				{"src/lib/accordion/AccordionItem.svelte", "updateSingleSelection(open, self)"},
				{"src/lib/accordion/AccordionItem.svelte", "onclick={handleToggle}"},
				{"src/lib/accordion/AccordionItem.svelte", "open = !open"},
				{"src/lib/tabs/Tabs.svelte", "createSingleSelectionContext<SelectedTab>()"},
				{"src/lib/tabs/Tabs.svelte", "const updateSelection = useSingleSelection<SelectedTab>"},
				{"src/lib/darkmode/DarkMode.svelte", "const toggleTheme ="},
				{"src/lib/utils/singleselection.svelte.js", "export function createSingleSelectionContext(nonReactive = false)"},
				{"src/lib/utils/singleselection.svelte.js", "export function useSingleSelection(callback)"},
				{"src/lib/utils/singleselection.svelte.js", "if (nonReactive) return setContext(SINGLE_SELECTION_KEY, {})"},
			},
			targets: []svelteOriginalTarget{
				{"src/lib/accordion/AccordionItem.svelte", "handleToggle", 65},
				{"src/lib/tabs/Tabs.svelte", "registerFn", 59},
				{"src/lib/darkmode/DarkMode.svelte", "toggleTheme", 18},
			},
			linkedFile: "src/lib/utils/singleselection.svelte.js", linkedLanguage: "javascript",
			linkedSymbol: "useSingleSelection", linkedLine: 42,
		},
		{
			project: "svelte-imputnet-cobalt", search: "settingId",
			primary:   "web/src/components/buttons/SettingsToggle.svelte",
			unrelated: "web/src/components/settings/ResetSettingsButton.svelte",
			files: []string{
				"web/src/components/buttons/SettingsToggle.svelte",
				"web/src/components/buttons/SettingsButton.svelte",
				"web/src/routes/settings/appearance/+page.svelte",
				"web/src/components/settings/ResetSettingsButton.svelte",
			},
			oracles: []svelteOriginalEvidence{
				{"web/src/components/buttons/SettingsToggle.svelte", "Context extends Exclude<keyof CobaltSettings, 'schemaVersion'>"},
				{"web/src/components/buttons/SettingsToggle.svelte", "export let settingId: Id;"},
				{"web/src/components/buttons/SettingsToggle.svelte", "$: isEnabled = !!setting;"},
				{"web/src/components/buttons/SettingsToggle.svelte", "[settingId]: !isEnabled"},
				{"web/src/components/buttons/SettingsButton.svelte", "[settingId]: settingValue"},
				{"web/src/routes/settings/appearance/+page.svelte", "settingId=\"autoLanguage\""},
				{"web/src/routes/settings/appearance/+page.svelte", "settingId=\"theme\""},
				{"web/src/components/settings/ResetSettingsButton.svelte", "action: () => resetSettings()"},
				{"web/src/lib/state/settings.ts", "export function updateSetting(partial: PartialSettings)"},
				{"web/src/lib/state/settings.ts", "writeToStorage("},
				{"web/src/lib/state/settings.ts", "export function resetSettings()"},
			},
			targets: []svelteOriginalTarget{
				{"web/src/components/buttons/SettingsToggle.svelte", "settingId", 14},
				{"web/src/components/buttons/SettingsToggle.svelte", "settingContext", 15},
				{"web/src/components/buttons/SettingsButton.svelte", "settingValue", 16},
				{"web/src/routes/settings/appearance/+page.svelte", "dropdownItems", 14},
				{"web/src/components/settings/ResetSettingsButton.svelte", "resetDialog", 9},
			},
			linkedFile: "web/src/lib/state/settings.ts", linkedLanguage: "typescript", linkedSymbol: "updateSetting", linkedLine: 69,
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				p := filepath.Join(root, filepath.FromSlash(rel))
				requireOriginalSvelteFile(t, p)
				paths = append(paths, p)
			}
			for _, oracle := range tc.oracles {
				p := filepath.Join(root, filepath.FromSlash(oracle.file))
				requireOriginalSvelteFile(t, p)
				body, err := os.ReadFile(p)
				if err != nil || !strings.Contains(string(body), oracle.text) {
					t.Fatalf("original source %s missing %q err=%v", oracle.file, oracle.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "svelte", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("digest err=%v failed=%+v result=%+v", err, failed, digest)
			}
			encoded, err := json.Marshal(digest)
			if err != nil || len(encoded) > 8192 {
				t.Fatalf("compact digest %d bytes err=%v", len(encoded), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(encoded), digest.CoverageComplete)
			for _, want := range tc.targets {
				checkOriginalSvelteTarget(t, h, root, want, "svelte")
			}
			checkOriginalSvelteTarget(t, h, root, svelteOriginalTarget{tc.linkedFile, tc.linkedSymbol, tc.linkedLine}, tc.linkedLanguage)
			failed, query, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.search, Match: "exact", Mode: "structural", Language: "svelte", Encoding: "utf-8", MaxResults: 25,
			})
			if err != nil || failed != nil || query.Search == nil {
				t.Fatalf("structural query %s err=%v failed=%+v query=%+v", tc.search, err, failed, query.Search)
			}
			primary := filepath.Join(root, filepath.FromSlash(tc.primary))
			saw := false
			for _, match := range query.Search.Matches {
				if match.Name == tc.search && filepath.Clean(match.Path) == primary {
					saw = true
				}
			}
			if !saw {
				t.Fatalf("structural query omitted primary %s: %+v", tc.primary, query.Search.Matches)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
				Query: tc.search, Match: "exact", Language: "svelte", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 15,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated %s symbols err=%v failed=%+v found=%+v", tc.search, err, failed, negative.Symbols)
			}
		})
	}
}

func checkOriginalSvelteTarget(t *testing.T, h *Handler, root string, want svelteOriginalTarget, language string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(want.file))
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{p}, Query: want.symbol, Match: "exact",
		Language: language, Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 20,
	})
	if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
		t.Fatalf("find %s err=%v failed=%+v result=%+v", want.symbol, err, failed, found)
	}
	id := ""
	for _, symbol := range found.Symbols {
		if symbol.Name == want.symbol && symbol.NameRange.Start.Line == want.line {
			id = symbol.ID
			break
		}
	}
	if id == "" {
		t.Fatalf("missing %s:%d %s: %+v", want.file, want.line, want.symbol, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: p, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
		Language: language, Encoding: "utf-8", MaxBytes: 16384,
	})
	if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.symbol) {
		t.Fatalf("fingerprint show %s err=%v failed=%+v show=%+v", want.symbol, err, failed, shown.Show)
	}
}

func requireOriginalSvelteFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("original source file %s unavailable: %v", path, err)
	}
}
