package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The maintenance questions and relevant/distractor files were selected from
// independent original-source reads before evaluating source_* results.
// Original third-party projects are read-only and are never executed.
func TestSourceSymbolsOriginalShellBashTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original source corpus not configured")
	}
	type expected struct {
		file, name string
		line       int
		source     string
	}
	type project struct {
		name, directory, language string
		files                     []string
		oracle                    []expected
		query                     string
	}
	cases := []project{
		{
			name:      "bash-nvm-version-source-and-install-regression",
			directory: "bash-nvm-sh-nvm", language: "bash",
			files: []string{"install.sh", "test/install_script/nvm_source", "nvm.sh"},
			oracle: []expected{
				{"install.sh", "nvm_latest_version", 47, "v0.40.7"},
				{"install.sh", "nvm_source", 71, "NVM_INSTALL_VERSION"},
				{"install.sh", "nvm_download", 119, "nvm_has_executable"},
				{"test/install_script/nvm_source", "die", 7, "cleanup"},
				{"nvm.sh", "nvm_download", 133, "curl"},
			},
			query: "nvm_source",
		},
		{
			name:      "bash-acme-nginx-deploy-dispatch-and-docker-alternative",
			directory: "bash-acmesh-official-acme.sh", language: "bash",
			files: []string{"deploy/nginx.sh", "deploy/docker.sh", "acme.sh"},
			oracle: []expected{
				{"deploy/nginx.sh", "nginx_deploy", 10, "Not implemented yet"},
				{"deploy/docker.sh", "docker_deploy", 17, "DEPLOY_DOCKER_CONTAINER_LABEL"},
				{"acme.sh", "_deploy", 6727, "_findHook"},
			},
			query: "nginx_deploy",
		},
		{
			name:      "posix-getoptions-unknown-option-and-regression",
			directory: "shell-ko1nksm-getoptions", language: "shell",
			files: []string{"lib/getoptions_base.sh", "spec/getoptions_base_spec.sh", "src/getoptions"},
			oracle: []expected{
				{"lib/getoptions_base.sh", "getoptions", 3, "Unrecognized option"},
				{"spec/getoptions_base_spec.sh", "parse", 5, "getoptions"},
			},
			query: "getoptions",
		},
		{
			name:      "posix-shdotenv-unknown-option-and-config-errors",
			directory: "shell-ko1nksm-shdotenv", language: "shell",
			files: []string{"src/shdotenv", "spec/parser_spec.sh", "spec/formatter_spec.sh"},
			oracle: []expected{
				{"src/shdotenv", "parse_options", 58, "unknown option"},
				{"src/shdotenv", "read_config_file", 120, "unknown line"},
				{"src/shdotenv", "exec_parser", 132, "SHDOTENV_AWK"},
				{"spec/parser_spec.sh", "parse_env", 2, "parser.awk"},
			},
			query: "read_config_file",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(corpus, tc.directory)
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original source unavailable %s: %v", rel, err)
				}
				paths = append(paths, path)
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: tc.language,
				Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("digest err=%v tool=%+v result=%+v", err, failed, digest)
			}
			data, err := json.Marshal(digest)
			if err != nil || len(data) > 16384 {
				t.Fatalf("initial map unexpectedly large: bytes=%d err=%v", len(data), err)
			}
			t.Logf("originalProjects=1 files=%d symbols=%d compactBytes=%d reportedComplete=%v",
				digest.FilesParsed, digest.SymbolCount, len(data), digest.CoverageComplete)
			for _, want := range tc.oracle {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				source, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(source), want.source) {
					t.Fatalf("independent source oracle changed for %s: %v", want.file, err)
				}
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: tc.language,
					Encoding: "utf-8", Query: want.name, Match: "exact",
					MaxFiles: 1, MaxSymbols: 200,
				})
				if err != nil || failed != nil || found.Truncated {
					t.Fatalf("find %s err=%v tool=%+v", want.name, err, failed)
				}
				id := ""
				for _, symbol := range found.Symbols {
					if symbol.Name == want.name && string(symbol.Kind) == "function" &&
						symbol.DeclarationRange.Start.Line == want.line {
						id = symbol.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing independently observed function %s:%d (%s): %+v",
						want.file, want.line, want.name, found.Symbols)
				}
				if len(found.Files) != 1 || found.Files[0].SourceFingerprint == "" {
					t.Fatalf("missing source fingerprint %s", want.name)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: id,
					SourceFingerprint: found.Files[0].SourceFingerprint,
					Language:          tc.language, Encoding: "utf-8", MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.name) {
					t.Fatalf("fingerprint-bound source excerpt missing %s err=%v tool=%+v show=%+v",
						want.name, err, failed, shown.Show)
				}
			}
			failed, result, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Language: tc.language, Encoding: "utf-8",
				Query: tc.query, Match: "exact", Mode: "structural", MaxResults: 20,
			})
			if err != nil || failed != nil || result.Search == nil || len(result.Search.Matches) == 0 {
				t.Fatalf("structural search %s err=%v tool=%+v result=%+v",
					tc.query, err, failed, result.Search)
			}
		})
	}
}
