package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRubyPinnedWholeKamalNestedMethodBodyRanges(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_RUBY_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole Ruby originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "ruby-kamal-proxy-eee0083.rb"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 1529 {
		t.Fatalf("whole upstream Kamal source size changed: %d", len(b))
	}
	for _, tc := range []struct{ name, src string }{
		{"git-lf", string(b)}, {"worktree-crlf", strings.ReplaceAll(string(b), "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (RubyAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("proxy.rb", tc.src), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Symbols) != 3 {
				t.Fatalf("whole Kamal original 3 supported symbols count=%d complete=%v truncated=%v", len(result.Analysis.Symbols), result.Analysis.CoverageComplete, result.Analysis.Truncated)
			}
			want := []struct {
				name       string
				start, end int
			}{
				{"Kamal::Configuration::Validator::Proxy", 1, 1},
				{"Kamal::Configuration::Validator::Proxy.validate!", 2, 36},
				{"Kamal::Configuration::Validator::Proxy.ensure_valid_bind_ips", 39, 44},
			}
			for i, s := range result.Analysis.Symbols {
				if s.QualifiedName != want[i].name || s.DeclarationRange.Start.Line != want[i].start ||
					s.DeclarationRange.End.Line != want[i].end {
					t.Errorf("whole Kamal authored member span index=%d actual=%+v expected=%+v", i, s, want[i])
				}
			}
		})
	}
}

func TestRubyPinnedWholeSidekiqSimpleMethodBodyControl(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_RUBY_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole Ruby originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "ruby-sidekiq-ring-1bb4aa0.rb"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 504 {
		t.Fatalf("whole upstream Sidekiq source size changed: %d", len(b))
	}
	result, err := (RubyAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("ring_buffer.rb", string(b)), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Symbols) != 6 {
		t.Fatalf("whole Sidekiq 6 supported declarations count=%d complete=%v", len(result.Analysis.Symbols), result.Analysis.CoverageComplete)
	}
	expected := map[string]int{"Sidekiq.RingBuffer": 6, "Sidekiq.RingBuffer.RingBuffer": 16,
		"Sidekiq.RingBuffer.<<": 22, "Sidekiq.RingBuffer.buffer": 26, "Sidekiq.RingBuffer.reset": 30}
	for _, s := range result.Analysis.Symbols {
		if end, ok := expected[s.QualifiedName]; ok && s.DeclarationRange.End.Line != end {
			t.Errorf("whole Sidekiq body changed: %+v expected end=%d", s, end)
		}
	}
}

func TestRubyNestedMemberBodiesConservativeBoundaries(t *testing.T) {
	cases := []struct {
		name, src, method string
		end               int
	}{
		{"nested-if-unless", "class K\n  def work\n    unless missing?\n      if enabled?\n        call\n      end\n    end\n  end\nend\n", "K.work", 8},
		{"postfix-unless", "class K\n  def work\n    call unless enabled?\n  end\nend\n", "K.work", 4},
		{"do-block", "class K\n  def work\n    items.each do |x|\n      call(x)\n    end\n  end\nend\n", "K.work", 6},
		{"nested-if-else", "class K\n  def work\n    if yes?\n      call\n    else\n      other\n    end\n  end\nend\n", "K.work", 8},
		{"inline-end-ambiguous", "class K\n  def work\n    do_something if flag; end\n  end\nend\n", "K.work", 2},
		{"case-unproven", "class K\n  def work\n    case value\n    when 1\n      call\n    end\n  end\nend\n", "K.work", 2},
		{"begin-unproven", "class K\n  def work\n    begin\n      call\n    ensure\n      cleanup\n    end\n  end\nend\n", "K.work", 2},
		{"while-unproven", "class K\n  def work\n    while ready?\n      call\n    end\n  end\nend\n", "K.work", 2},
		{"nested-definition-unproven", "class K\n  def work\n    def nested\n      call\n    end\n  end\nend\n", "K.work", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (RubyAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("member.rb", tc.src), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			var matched bool
			for _, s := range got.Analysis.Symbols {
				if s.QualifiedName == tc.method {
					matched = true
					if s.DeclarationRange.End.Line != tc.end {
						t.Errorf("member end line %d want %d: %+v", s.DeclarationRange.End.Line, tc.end, s)
					}
				}
			}
			if !matched {
				t.Fatal("missing authored member")
			}
		})
	}
}
