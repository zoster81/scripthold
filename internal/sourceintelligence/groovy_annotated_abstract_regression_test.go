package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A Groovy annotation with array arguments must not hide the following class
// and its user-facing methods behind apparently complete source coverage.
func TestGroovyAnnotatedAbstractClassNavigation(t *testing.T) {
	const body = "abstract class ProtobufExtension {\n" +
		"  static String computeJavaExePath() { return 'java' }\n" +
		"  public void protoc(Action configureAction) { configureAction.execute(null) }\n" +
		"}\n"
	for _, tc := range []struct{ name, prefix string }{
		{"no-annotations", ""},
		{"simple-annotation", "@CompileStatic\n"},
		{"array-annotation", "@SuppressWarnings(['one', 'two'])\n"},
		{"stacked-annotations", "@CompileStatic\n@SuppressWarnings(['one', 'two'])\n"},
		{"import-before-annotation", "import org.gradle.api.Project\n@SuppressWarnings(['one', 'two'])\n"},
	} {
		for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"/"+eol.name, func(t *testing.T) {
				src := "package example\n" + tc.prefix + body
				doc := testSourceDocument("ProtobufExtension.groovy", strings.ReplaceAll(src, "\n", eol.value))
				result, err := (GroovyAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 256))
				if err != nil {
					t.Fatal(err)
				}
				requireGroovyQualifiedSymbols(t, result, "example.ProtobufExtension",
					"example.ProtobufExtension.computeJavaExePath",
					"example.ProtobufExtension.protoc")
			})
		}
	}
}

func TestGroovyOriginalProtobufExtensionNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original corpus not configured")
	}
	path := filepath.Join(corpus, "real-projects", "groovy-google-protobuf-gradle-plugin",
		"src", "main", "groovy", "com", "google", "protobuf", "gradle", "ProtobufExtension.groovy")
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(bytes)
	for _, literal := range []string{
		"@SuppressWarnings([",
		"abstract class ProtobufExtension",
		"static String computeJavaExePath()",
		"public void protoc(Action<ExecutableLocator> configureAction)",
	} {
		if !strings.Contains(text, literal) {
			t.Fatalf("independent source oracle missing %q", literal)
		}
	}
	result, err := (GroovyAnalyzer{}).Analyze(context.Background(),
		testSourceDocument(path, text), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	requireGroovyQualifiedSymbols(t, result, "com.google.protobuf.gradle.ProtobufExtension",
		"com.google.protobuf.gradle.ProtobufExtension.computeJavaExePath",
		"com.google.protobuf.gradle.ProtobufExtension.protoc")
}

func requireGroovyQualifiedSymbols(t *testing.T, result AnalyzerResult, names ...string) {
	t.Helper()
	got := make(map[string]bool, len(result.Analysis.Symbols))
	for _, s := range result.Analysis.Symbols {
		got[s.QualifiedName] = true
	}
	for _, name := range names {
		if !got[name] {
			t.Errorf("missing Groovy declaration %s (reported coverageComplete=%v, indexed=%v)",
				name, result.Analysis.CoverageComplete, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}
