package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independent original-source oracles determine the maintenance questions:
// change LocalSend's network-port default, or Spotube's initial player volume.
// The original applications are read only and are never executed.
func TestSourceSymbolsOriginalDartMaintenanceNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Dart projects are not configured")
	}
	type evidence struct{ file, text string }
	type target struct {
		file, name, excerpt string
		line                int
	}
	cases := []struct {
		project, query, unrelated string
		files                     []string
		evidence                  []evidence
		targets                   []target
	}{
		{
			project:   "dart-localsend-localsend",
			query:     "defaultPort",
			unrelated: "app/lib/model/state/settings_state.dart",
			files: []string{
				"packages/localsend_isolates/lib/constants.dart",
				"app/lib/provider/persistence_provider.dart",
				"app/lib/provider/settings_provider.dart",
				"app/lib/provider/network/server/server_provider.dart",
				"app/lib/pages/tabs/settings_tab.dart",
				"app/lib/model/state/settings_state.dart",
			},
			evidence: []evidence{
				{"packages/localsend_isolates/lib/constants.dart", "const defaultPort = 53317;"},
				{"app/lib/provider/persistence_provider.dart", "return _prefs.getInt(_portKey) ?? defaultPort;"},
				{"app/lib/provider/settings_provider.dart", "port: _persistence.getPort(),"},
				{"app/lib/provider/settings_provider.dart", "await _persistence.setPort(port);"},
				{"app/lib/provider/network/server/server_provider.dart", "port: settings.port,"},
				{"app/lib/provider/network/server/server_provider.dart", "port = defaultPort;"},
				{"app/lib/pages/tabs/settings_tab.dart", "ref.notifier(settingsProvider).setPort(port);"},
				{"app/lib/pages/tabs/settings_tab.dart", "vm.settings.port != defaultPort"},
				{"app/lib/model/state/settings_state.dart", "final int port;"},
			},
			targets: []target{
				{"packages/localsend_isolates/lib/constants.dart", "defaultPort", "53317", 14},
				{"app/lib/provider/persistence_provider.dart", "getPort", "_prefs.getInt(_portKey) ?? defaultPort", 340},
				{"app/lib/provider/settings_provider.dart", "setPort", "await _persistence.setPort(port);", 122},
				{"app/lib/provider/network/server/server_provider.dart", "startServerFromSettings", "port: settings.port,", 102},
				{"app/lib/provider/network/server/server_provider.dart", "startServer", "port = defaultPort;", 116},
			},
		},
		{
			project:   "dart-krtirtho-spotube",
			query:     "volume",
			unrelated: "lib/provider/metadata_plugin/audio_source/quality_label.dart",
			files: []string{
				"lib/services/kv_store/kv_store.dart",
				"lib/provider/volume_provider.dart",
				"lib/services/audio_player/audio_player_impl.dart",
				"lib/modules/player/volume_slider.dart",
				"lib/provider/metadata_plugin/audio_source/quality_label.dart",
			},
			evidence: []evidence{
				{"lib/services/kv_store/kv_store.dart", "static double get volume => sharedPreferences.getDouble('volume') ?? 1.0;"},
				{"lib/services/kv_store/kv_store.dart", "await sharedPreferences.setDouble('volume', value);"},
				{"lib/provider/volume_provider.dart", "audioPlayer.setVolume(KVStoreService.volume);"},
				{"lib/provider/volume_provider.dart", "KVStoreService.setVolume(volume);"},
				{"lib/services/audio_player/audio_player_impl.dart", "assert(volume >= 0 && volume <= 1);"},
				{"lib/services/audio_player/audio_player_impl.dart", "_mkPlayer.setVolume(volume * 100);"},
				{"lib/modules/player/volume_slider.dart", "final newValue = value - .2;"},
				{"lib/modules/player/volume_slider.dart", "onChanged(newValue > 1 ? 1 : newValue);"},
			},
			targets: []target{
				{"lib/services/kv_store/kv_store.dart", "volume", "getDouble('volume') ?? 1.0", 83},
				{"lib/provider/volume_provider.dart", "build", "audioPlayer.setVolume(KVStoreService.volume)", 11},
				{"lib/provider/volume_provider.dart", "setVolume", "KVStoreService.setVolume(volume)", 16},
				{"lib/services/audio_player/audio_player_impl.dart", "setVolume", "_mkPlayer.setVolume(volume * 100)", 24},
				{"lib/modules/player/volume_slider.dart", "build", "scrollDelta.dy > 0", 21},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original file unavailable %s: %v", rel, err)
				}
				paths = append(paths, path)
			}
			for _, want := range tc.evidence {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original oracle unavailable %s: %v", want.file, err)
				}
				bytes, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(bytes), want.text) {
					t.Fatalf("independent original oracle %s %q: %v", want.file, want.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "dart", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact Dart digest: err=%v failed=%+v digest=%+v", err, failed, digest)
			}
			data, err := json.Marshal(digest)
			if err != nil || len(data) > 8192 {
				t.Fatalf("compact Dart response bytes=%d err=%v", len(data), err)
			}
			t.Logf("originalProject=1 files=%d symbols=%d compactBytes=%d coverageComplete=%v",
				digest.FilesParsed, digest.SymbolCount, len(data), digest.CoverageComplete)
			for _, want := range tc.targets {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: want.name, Match: "exact", Language: "dart", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 30,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("Dart exact find %s: err=%v failed=%+v found=%+v", want.name, err, failed, found)
				}
				var id string
				for _, sym := range found.Symbols {
					if sym.Name == want.name && sym.NameRange.Start.Line == want.line {
						id = sym.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing original Dart declaration %s:%d %s, results=%+v", want.file, want.line, want.name, found.Symbols)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, Language: "dart", Encoding: "utf-8", MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.excerpt) {
					t.Fatalf("Dart fingerprint show %s: err=%v failed=%+v shown=%+v", want.name, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural", Language: "dart", Encoding: "utf-8", MaxResults: 30,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("Dart structural search %s: err=%v failed=%+v search=%+v", tc.query, err, failed, search.Search)
			}
			failed, unrelated, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
				Query: tc.query, Match: "exact", Language: "dart", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || failed != nil || unrelated.Truncated || len(unrelated.Symbols) != 0 {
				t.Fatalf("unrelated Dart component falsely defines target %s: err=%v failed=%+v symbols=%+v", tc.query, err, failed, unrelated.Symbols)
			}
		})
	}
}
