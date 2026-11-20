package sourceintelligence

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

// A literal nested inside a computed Ruby require argument is not the loaded
// module. Only the explicitly authored immediate literal is source evidence.
func TestRubyRequireDirectLiteralOriginalSources(t *testing.T) {
	const fixture = "require \"direct.alpha\"\n" +
		"require_relative './direct.support'\n" +
		"require File.join(__dir__, \"fake.path\")\n" +
		"require_relative File.expand_path(\"fake.relative\", __dir__)\n" +
		"require(\"fake.prefix.\" + suffix)\n" +
		"require(\"fake.interpolated.#{suffix}\")\n" +
		"require(\"direct.omega\")\n"
	for _, tc := range []struct {
		name, input string
	}{
		{"LF", fixture},
		{"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rubyAssertImportValues(t, tc.input, []string{"direct.alpha", "./direct.support", "direct.omega"})
		})
	}
	for _, tc := range []struct {
		name, env, marker string
	}{
		{"jit-benchmarks-dc", "SCRIPTHOLD_REAL_RUBY_JIT_DC",
			"require File.join(File.dirname(__FILE__), \"tools\", \"cli\")"},
		{"amp-bin", "SCRIPTHOLD_REAL_RUBY_AMP_BIN",
			"require File.join(File.dirname(follow_link(__FILE__)), \"..\", \"lib\", \"amp\")"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Ruby source not configured")
			}
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() < 100 || stat.Size() > 50000 {
				t.Fatalf("original Ruby source unavailable or unbounded: %v %v", stat, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.marker) {
				t.Fatal("original Ruby source oracle drift")
			}
			rubyAssertImportValues(t, string(data), nil)
		})
	}
}

func rubyAssertImportValues(t *testing.T, code string, want []string) {
	t.Helper()
	result, err := (RubyAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("original.rb", code), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated {
		t.Fatal("Ruby analyzer unexpectedly truncated original")
	}
	got := dependencyValues(result.Dependencies)
	if len(want) == 0 {
		want = []string{}
	}
	if len(got) == 0 {
		got = []string{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Ruby source-backed imports %q, want %q", got, want)
	}
}
