package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type originalPowerShellEvidence struct {
	file, text string
}

type originalPowerShellTarget struct {
	file, name string
	line       int
}

type originalPowerShellTask struct {
	project, query, primary, unrelated, showText string
	files                                        []string
	evidence                                     []originalPowerShellEvidence
	targets                                      []originalPowerShellTarget
}

// These original projects are only read as fixtures; no PowerShell is executed.
func TestSourceSymbolsOriginalPowerShellTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original PowerShell corpus unavailable")
	}
	cases := []originalPowerShellTask{
		{
			project:   "powershell-scoopinstaller-scoop",
			query:     "set_config",
			showText:  "Complete-ConfigChange -Name $name -Value $value",
			primary:   "lib/core.ps1",
			unrelated: "libexec/scoop-cache.ps1",
			files: []string{
				"lib/core.ps1",
				"libexec/scoop-config.ps1",
				"test/Scoop-Config.Tests.ps1",
				"libexec/scoop-cache.ps1",
			},
			evidence: []originalPowerShellEvidence{
				{"lib/core.ps1", "$name = $name.ToLowerInvariant()"},
				{"lib/core.ps1", "Complete-ConfigChange -Name $name -Value $value"},
				{"lib/core.ps1", "ConvertTo-Json $scoopConfig | Out-UTF8File -FilePath $configFile"},
				{"libexec/scoop-config.ps1", "set_config $name $value | Out-Null"},
				{"libexec/scoop-config.ps1", "set_config $value $null | Out-Null"},
				{"test/Scoop-Config.Tests.ps1", "It 'set_config should be able to save typed values correctly'"},
				{"test/Scoop-Config.Tests.ps1", "It 'set_config should remove a value if being set to $null'"},
				{"libexec/scoop-cache.ps1", "function cacheinfo($file)"},
			},
			targets: []originalPowerShellTarget{
				{"lib/core.ps1", "load_cfg", 82},
				{"lib/core.ps1", "get_config", 97},
				{"lib/core.ps1", "set_config", 105},
				{"lib/core.ps1", "Complete-ConfigChange", 141},
			},
		},
		{
			project:   "powershell-dahlbyk-posh-git",
			query:     "Get-GitStatus",
			showText:  "switch ($settings.UntrackedFilesMode)",
			primary:   "src/GitUtils.ps1",
			unrelated: "src/TortoiseGit.ps1",
			files: []string{
				"src/GitUtils.ps1",
				"src/GitPrompt.ps1",
				"test/Get-GitStatus.Tests.ps1",
				"src/TortoiseGit.ps1",
			},
			evidence: []originalPowerShellEvidence{
				{"src/GitUtils.ps1", "switch ($settings.UntrackedFilesMode)"},
				{"src/GitUtils.ps1", "$branch = Get-GitBranch -Branch $branch -GitDir $GitDir"},
				{"src/GitPrompt.ps1", "$global:GitStatus = Get-GitStatus"},
				{"src/GitPrompt.ps1", "Write-GitStatus $GitStatus"},
				{"test/Get-GitStatus.Tests.ps1", "It 'Returns the correct number of added untracked working files'"},
				{"test/Get-GitStatus.Tests.ps1", "$status.Working.Added.Count | Should -Be 2"},
				{"src/TortoiseGit.ps1", "TortoiseGitProc.exe"},
			},
			targets: []originalPowerShellTarget{
				{"src/GitUtils.ps1", "Get-GitBranch", 62},
				{"src/GitUtils.ps1", "Get-GitStatus", 231},
				{"src/GitPrompt.ps1", "Write-GitStatus", 202},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				requireOriginalPowerShellTextFile(t, path)
				paths = append(paths, path)
			}
			for _, oracle := range tc.evidence {
				path := filepath.Join(root, filepath.FromSlash(oracle.file))
				requireOriginalPowerShellTextFile(t, path)
				body, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(body), oracle.text) {
					t.Fatalf("independent source check %s %q: %v", oracle.file, oracle.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "powershell", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact digest err=%v failed=%+v result=%+v", err, failed, digest)
			}
			encoded, err := json.Marshal(digest)
			if err != nil || len(encoded) > 8192 {
				t.Fatalf("compact digest size=%d err=%v", len(encoded), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(encoded), digest.CoverageComplete)
			for _, want := range tc.targets {
				p := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{p}, Query: want.name, Match: "exact",
					Language: "powershell", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 20,
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
					t.Fatalf("original declaration %s:%d %s missing", want.file, want.line, want.name)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: p, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
					Language: "powershell", Encoding: "utf-8", MaxBytes: 32768,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.name) {
					t.Fatalf("fingerprint show %s err=%v failed=%+v shown=%+v", want.name, err, failed, shown.Show)
				}
				if want.name == tc.query && !strings.Contains(shown.Show.Text, tc.showText) {
					t.Fatalf("fingerprint show %s omitted the source-verified implementation evidence", want.name)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural",
				Language: "powershell", Encoding: "utf-8", MaxResults: 25,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("structural search %s err=%v failed=%+v result=%+v", tc.query, err, failed, search.Search)
			}
			primary := filepath.Join(root, filepath.FromSlash(tc.primary))
			sawPrimary := false
			for _, match := range search.Search.Matches {
				if match.Name == tc.query && filepath.Clean(match.Path) == primary {
					sawPrimary = true
				}
			}
			if !sawPrimary {
				t.Fatalf("structural search did not identify primary declaration in %s: %+v", tc.primary, search.Search.Matches)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
				Query: tc.query, Match: "exact", Language: "powershell", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 20,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated source defines %s err=%v failed=%+v symbols=%+v", tc.query, err, failed, negative.Symbols)
			}
		})
	}
}

func requireOriginalPowerShellTextFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("original file %s is unavailable or not regular: %v", path, err)
	}
}
