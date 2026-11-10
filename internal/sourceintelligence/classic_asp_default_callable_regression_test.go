package sourceintelligence

import (
	"context"
	"os"
	"testing"
)

func TestClassicASPDefaultCallableAndVBScriptSourceNavigation(t *testing.T) {
	cases := []struct {
		name     string
		analyzer SourceAnalyzer
		source   string
	}{
		{
			name:     "vbscript",
			analyzer: VBScriptAnalyzer{},
			source:   "Class Logger\n Public Default Function Log(p_string)\n  Log = p_string\n End Function\n Public Function SetLogFile(path)\n End Function\nEnd Class\n",
		},
		{
			name:     "classic-asp",
			analyzer: ClassicASPAnalyzer{},
			source:   "<%\nClass Logger\n Public Default Function Log(p_string)\n  Log = p_string\n End Function\n Public Function SetLogFile(path)\n End Function\nEnd Class\n%>\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			mapped := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, name := range []string{"Logger", "Logger.Log", "Logger.SetLogFile"} {
				if _, ok := mapped[name]; !ok {
					t.Errorf("missing source declaration %s in %s (coverageComplete=%v), symbols=%+v", name, tc.name, result.Analysis.CoverageComplete, result.Analysis.Symbols)
				}
			}
			if _, ok := mapped["Logger.Function"]; ok {
				t.Error("default function keyword was misread as callable name")
			}
		})
	}
}

func TestClassicASPOriginalLoggerDefaultCallable(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_CLASSICASP_LOGGER")
	if path == "" {
		t.Skip("original Classic ASP logger not configured")
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (ClassicASPAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(string(source)), testAnalyzeOptions(true, 80))
	if err != nil {
		t.Fatal(err)
	}
	mapped := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"logger", "logger.log", "logger.setLogFile", "logger.m_log"} {
		if _, ok := mapped[name]; !ok {
			t.Errorf("original logger is missing %q (complete=%v): %+v", name,
				result.Analysis.CoverageComplete, result.Analysis.Symbols)
		}
	}
}
