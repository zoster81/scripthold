package sourceintelligence

import (
	"context"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Only the immediate authored literal argument to require is evidence of a
// definite import. A string nested in a computed path must not be promoted.
func TestPerlRequireDirectLiteralOriginalSources(t *testing.T) {
	const fixture = "use File::Spec;\n" +
		"require 'direct/alpha.pm';\n" +
		"require(\"direct/beta.pm\");\n" +
		"require File::Spec->catfile($base, \"fabricated/inner.pm\");\n" +
		"require(File::Spec->catfile($base, 'fabricated/nested.pm'));\n" +
		"require(\"fabricated/prefix/\" . $name);\n" +
		"require \"fabricated/$dynamic.pm\";\n" +
		"require 'direct/omega.pm';\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", fixture},
		{"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertPerlDependencies(t, tc.source, []string{
				"File::Spec", "direct/alpha.pm", "direct/beta.pm", "direct/omega.pm",
			})
		})
	}
	for _, tc := range []struct {
		name, env, signature string
		forbidden            []string
		required             []string
	}{
		{"opennms", "SCRIPTHOLD_REAL_PERL_OPENNMS",
			"require(File::Spec->catfile($PREFIX, 'bin', 'functions.pl'))",
			[]string{"bin", "functions.pl"}, []string{"Cwd", "File::Basename", "File::Spec"}},
		{"regripper", "SCRIPTHOLD_REAL_PERL_REGRIPPER",
			"require File::Spec->catfile($RealBin,\"time.pl\")",
			[]string{"time.pl", "rr_helper.pl"}, []string{"File::Spec", "JSON::PP"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Perl source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 60000 {
				t.Fatalf("original Perl source unavailable/unbounded: %v %v", info, err)
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(bytes), tc.signature) {
				t.Fatal("original Perl import oracle drift")
			}
			result := analyzePerlDependencyEvidence(t, string(bytes))
			got := dependencyValues(result.Dependencies)
			for _, forbidden := range tc.forbidden {
				if slices.Contains(got, forbidden) {
					t.Fatalf("fabricated Perl import %q: %q", forbidden, got)
				}
			}
			for _, required := range tc.required {
				if !slices.Contains(got, required) {
					t.Fatalf("lost Perl use import %q: %q", required, got)
				}
			}
		})
	}
}

func assertPerlDependencies(t *testing.T, source string, want []string) {
	t.Helper()
	got := dependencyValues(analyzePerlDependencyEvidence(t, source).Dependencies)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Perl source-backed imports=%q want=%q", got, want)
	}
}

func analyzePerlDependencyEvidence(t *testing.T, source string) AnalyzerResult {
	t.Helper()
	result, err := (PerlAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("original.pl", source), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated {
		t.Fatal("Perl source analysis unexpectedly truncated")
	}
	return result
}
