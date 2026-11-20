package handler

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Static imports must remain source-backed in the public MCP digest. Check
// independent originals without executing third-party Groovy code.
func TestGroovyStaticImportsOriginalDigestEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		expected          []string
	}{
		{"rosetta", "SCRIPTHOLD_REAL_GROOVY_ROSETTA",
			"import static java.lang.Math.min",
			[]string{"groovy.transform.TailRecursive", "java.lang.Math.min"}},
		{"making-java-groovy", "SCRIPTHOLD_REAL_GROOVY_MJG",
			"import static java.lang.Math.*",
			[]string{"java.lang.Math.*", "org.apache.commons.math3.complex.Complex",
				"org.apache.commons.math3.complex.ComplexUtils", "groovy.sql.Sql"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Groovy source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 20000 {
				t.Fatalf("original Groovy file unavailable/unbounded: %v %v", info, err)
			}
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(source), tc.marker) {
				t.Fatal("independent Groovy oracle drift")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failed, output, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "groovy", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failed != nil || output.Truncated || len(output.Digests) != 1 {
				t.Fatalf("Groovy MCP digest unavailable: %v %+v %+v", err, failed, output)
			}
			got := make([]string, 0, len(output.Digests[0].Dependencies))
			for _, dep := range output.Digests[0].Dependencies {
				got = append(got, dep.Value)
			}
			if output.Digests[0].SourceFingerprint == "" || !reflect.DeepEqual(got, tc.expected) {
				t.Fatalf("Groovy source-backed MCP imports=%q expected=%q", got, tc.expected)
			}
		})
	}
}
