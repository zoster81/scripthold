package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independent original-source maintenance questions; no third-party code runs.
func TestSourceSymbolsOriginalRubyMaintenanceNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Ruby corpus not configured")
	}
	type oracle struct{ file, text string }
	type target struct {
		file, name, excerpt string
		line                int
	}
	cases := []struct {
		project, query, unrelated string
		files                     []string
		oracles                   []oracle
		targets                   []target
	}{
		{project: "project-ruby-sidekiq-sidekiq", query: "delay_for",
			unrelated: "lib/sidekiq/scheduled.rb",
			files:     []string{"lib/sidekiq/job_retry.rb", "test/retry_test.rb", "test/retry_exhausted_test.rb", "lib/sidekiq/scheduled.rb"},
			oracles: []oracle{
				{"lib/sidekiq/job_retry.rb", "DEFAULT_MAX_RETRY_ATTEMPTS = 25"},
				{"lib/sidekiq/job_retry.rb", "@max_retries = Sidekiq.default_configuration[:max_retries] || DEFAULT_MAX_RETRY_ATTEMPTS"},
				{"lib/sidekiq/job_retry.rb", "max_retry_attempts = retry_attempts_from(msg[\"retry\"], @max_retries)"},
				{"lib/sidekiq/job_retry.rb", "delay = (count**4) + 15"},
				{"test/retry_test.rb", "it \"allows a numeric retry\" do"},
			},
			targets: []target{
				{"lib/sidekiq/job_retry.rb", "JobRetry", "DEFAULT_MAX_RETRY_ATTEMPTS", 75},
				{"lib/sidekiq/job_retry.rb", "process_retry", "def process_retry(", 149},
				{"lib/sidekiq/job_retry.rb", "delay_for", "def delay_for(", 219},
			},
		},
		{project: "project-ruby-heartcombo-devise", query: "timedout?",
			unrelated: "lib/devise/models/rememberable.rb",
			files:     []string{"lib/devise.rb", "lib/devise/models/timeoutable.rb", "lib/devise/hooks/timeoutable.rb", "test/models/timeoutable_test.rb", "lib/devise/models/rememberable.rb"},
			oracles: []oracle{
				{"lib/devise.rb", "@@timeout_in = 30.minutes"},
				{"lib/devise/models/timeoutable.rb", "!timeout_in.nil? && last_access && last_access <= timeout_in.ago"},
				{"lib/devise/models/timeoutable.rb", "self.class.timeout_in"},
				{"lib/devise/hooks/timeoutable.rb", "record.timedout?(last_request_at)"},
				{"test/models/timeoutable_test.rb", "assert new_user.timedout?(31.minutes.ago)"},
				{"test/models/timeoutable_test.rb", "assert_not new_user.timedout?(29.minutes.ago)"},
			},
			targets: []target{
				{"lib/devise/models/timeoutable.rb", "timedout?", "last_access <= timeout_in.ago", 30},
				{"lib/devise/models/timeoutable.rb", "timeout_in", "self.class.timeout_in", 34},
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
					t.Fatalf("original source missing %s: %v", rel, err)
				}
				paths = append(paths, path)
			}
			for _, e := range tc.oracles {
				path := filepath.Join(root, filepath.FromSlash(e.file))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original oracle source unavailable %s: %v", e.file, err)
				}
				b, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(b), e.text) {
					t.Fatalf("original oracle %s %q: %v", e.file, e.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "ruby", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("Ruby digest err=%v failed=%+v digest=%+v", err, failed, digest)
			}
			b, err := json.Marshal(digest)
			if err != nil || len(b) > 8192 {
				t.Fatalf("Ruby compact bytes=%d err=%v", len(b), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(b), digest.CoverageComplete)
			for _, want := range tc.targets {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: want.name, Match: "exact", Language: "ruby", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 20,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("Ruby find %s err=%v failed=%+v found=%+v", want.name, err, failed, found)
				}
				var id string
				for _, sym := range found.Symbols {
					if sym.Name == want.name && sym.NameRange.Start.Line == want.line {
						id = sym.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("Ruby declaration missing %s:%d %s symbols=%+v", want.file, want.line, want.name, found.Symbols)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, Language: "ruby", Encoding: "utf-8", MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.excerpt) {
					t.Fatalf("Ruby verified excerpt %s err=%v failed=%+v show=%+v", want.name, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural", Language: "ruby", Encoding: "utf-8", MaxResults: 30,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("Ruby structural search %s err=%v failed=%+v search=%+v", tc.query, err, failed, search.Search)
			}
			failed, neg, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
				Query: tc.query, Match: "exact", Language: "ruby", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || failed != nil || neg.Truncated || len(neg.Symbols) != 0 {
				t.Fatalf("Ruby unrelated source falsely defines %s err=%v failed=%+v symbols=%+v", tc.query, err, failed, neg.Symbols)
			}
		})
	}
}
