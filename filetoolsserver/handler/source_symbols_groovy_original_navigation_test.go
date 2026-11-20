package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Verify two independent Groovy project maintenance questions against
// pre-read original production code, tests and an unrelated implementation.
func TestSourceSymbolsOriginalGroovyMaintenanceNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Groovy corpus not configured")
	}
	type evidence struct{ file, text string }
	type target struct {
		file, name, excerpt string
		line                int
	}
	cases := []struct {
		project, query, negative string
		files                    []string
		oracles                  []evidence
		targets                  []target
	}{
		{
			project: "groovy-jenkins-jobdsl", query: "runScripts",
			negative: "job-dsl-core/src/main/groovy/javaposse/jobdsl/dsl/FileJobManagement.groovy",
			files: []string{
				"job-dsl-core/src/main/groovy/javaposse/jobdsl/dsl/AbstractDslScriptLoader.groovy",
				"job-dsl-core/src/main/groovy/javaposse/jobdsl/dsl/DslScriptLoader.groovy",
				"job-dsl-core/src/test/groovy/javaposse/jobdsl/dsl/DslScriptLoaderSpec.groovy",
				"job-dsl-core/src/main/groovy/javaposse/jobdsl/dsl/FileJobManagement.groovy",
			},
			oracles: []evidence{
				{"job-dsl-core/src/main/groovy/javaposse/jobdsl/dsl/AbstractDslScriptLoader.groovy", "scriptRequests.each { ScriptRequest scriptRequest ->"},
				{"job-dsl-core/src/main/groovy/javaposse/jobdsl/dsl/AbstractDslScriptLoader.groovy", "extractGeneratedItems(generatedItems, jobParent, scriptRequest)"},
				{"job-dsl-core/src/main/groovy/javaposse/jobdsl/dsl/AbstractDslScriptLoader.groovy", "runScripts([new ScriptRequest(script)])"},
				{"job-dsl-core/src/main/groovy/javaposse/jobdsl/dsl/DslScriptLoader.groovy", "extends AbstractDslScriptLoader<JobParent, GeneratedItems>"},
				{"job-dsl-core/src/test/groovy/javaposse/jobdsl/dsl/DslScriptLoaderSpec.groovy", "dslScriptLoader.runScripts([request1, request2]).jobs"},
				{"job-dsl-core/src/test/groovy/javaposse/jobdsl/dsl/DslScriptLoaderSpec.groovy", "dslScriptLoader.runScript(loadScript('simple.dsl')).jobs"},
			},
			targets: []target{
				{"job-dsl-core/src/main/groovy/javaposse/jobdsl/dsl/AbstractDslScriptLoader.groovy", "runScripts", "scriptRequests.each", 40},
				{"job-dsl-core/src/main/groovy/javaposse/jobdsl/dsl/AbstractDslScriptLoader.groovy", "runScript", "runScripts([new ScriptRequest(script)])", 86},
				{"job-dsl-core/src/main/groovy/javaposse/jobdsl/dsl/AbstractDslScriptLoader.groovy", "runScriptEngine", "createGroovyCodeSource(scriptRequest)", 91},
			},
		},
		{
			project: "groovy-jenkinsci-jenkinspipelineunit", query: "registerAllowedMethod",
			negative: "src/main/groovy/com/lesfurets/jenkins/unit/BasePipelineTest.groovy",
			files: []string{
				"src/main/groovy/com/lesfurets/jenkins/unit/PipelineTestHelper.groovy",
				"src/test/groovy/com/lesfurets/jenkins/TestCustomMethodJob.groovy",
				"src/main/groovy/com/lesfurets/jenkins/unit/BasePipelineTest.groovy",
			},
			oracles: []evidence{
				{"src/main/groovy/com/lesfurets/jenkins/unit/PipelineTestHelper.groovy", "allowedMethodCallbacks.put(method(name, args.toArray(new Class[args?.size()])), closure)"},
				{"src/main/groovy/com/lesfurets/jenkins/unit/PipelineTestHelper.groovy", "return allowedMethodCallbacks.find { k, v -> k == signature }"},
				{"src/main/groovy/com/lesfurets/jenkins/unit/PipelineTestHelper.groovy", "return getAllowedMethodEntry(name, args) != null"},
				{"src/test/groovy/com/lesfurets/jenkins/TestCustomMethodJob.groovy", "helper.registerAllowedMethod('customMethod', [], customMethodMock)"},
				{"src/test/groovy/com/lesfurets/jenkins/TestCustomMethodJob.groovy", "helper.registerAllowedMethod('customMethodWithArguments', [String, int, Collection], customMethodWithArgumentsMock)"},
				{"src/main/groovy/com/lesfurets/jenkins/unit/BasePipelineTest.groovy", "void registerAllowedMethods()"},
			},
			targets: []target{
				{"src/main/groovy/com/lesfurets/jenkins/unit/PipelineTestHelper.groovy", "registerAllowedMethod", "allowedMethodCallbacks.put(method(name), closure)", 667},
				{"src/main/groovy/com/lesfurets/jenkins/unit/PipelineTestHelper.groovy", "registerAllowedMethod", "allowedMethodCallbacks.put(method(name, args.toArray(new Class[args?.size()])), closure)", 676},
				{"src/main/groovy/com/lesfurets/jenkins/unit/PipelineTestHelper.groovy", "registerAllowedMethod", "allowedMethodCallbacks.put(methodSignature, closure)", 687},
				{"src/main/groovy/com/lesfurets/jenkins/unit/PipelineTestHelper.groovy", "registerAllowedMethod", "callback.apply(params)", 696},
				{"src/main/groovy/com/lesfurets/jenkins/unit/PipelineTestHelper.groovy", "registerAllowedMethod", "callback.accept(params)", 706},
				{"src/main/groovy/com/lesfurets/jenkins/unit/PipelineTestHelper.groovy", "getAllowedMethodEntry", "allowedMethodCallbacks.find", 510},
				{"src/main/groovy/com/lesfurets/jenkins/unit/PipelineTestHelper.groovy", "isMethodAllowed", "getAllowedMethodEntry(name, args) != null", 522},
				{"src/test/groovy/com/lesfurets/jenkins/TestCustomMethodJob.groovy", "should_run_script_with_custom_method_with_arguments_mock", "helper.registerAllowedMethod('customMethodWithArguments'", 59},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original file unavailable %s: %v", rel, err)
				}
				paths = append(paths, path)
			}
			for _, e := range tc.oracles {
				p := filepath.Join(root, filepath.FromSlash(e.file))
				info, err := os.Lstat(p)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original oracle file unavailable %s: %v", e.file, err)
				}
				body, err := os.ReadFile(p)
				if err != nil || !strings.Contains(string(body), e.text) {
					t.Fatalf("original evidence %s %q: %v", e.file, e.text, err)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "digest", Paths: paths, Language: "groovy", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths)})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact digest: error=%v failed=%+v digest=%+v", err, failed, digest)
			}
			data, err := json.Marshal(digest)
			if err != nil || len(data) > 8192 {
				t.Fatalf("digest bytes=%d err=%v", len(data), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(data), digest.CoverageComplete)
			for _, want := range tc.targets {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Query: want.name, Match: "exact", Language: "groovy", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find %s: error=%v failed=%+v found=%+v", want.name, err, failed, found)
				}
				var id string
				for _, sym := range found.Symbols {
					if sym.Name == want.name && sym.NameRange.Start.Line == want.line {
						id = sym.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing Groovy method %s:%d %s results=%+v", want.file, want.line, want.name, found.Symbols)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "show", Path: path, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, Language: "groovy", Encoding: "utf-8", MaxBytes: 8192})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.excerpt) {
					t.Fatalf("fingerprint-bound show %s failed: error=%v failed=%+v show=%+v", want.name, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{Operation: "search", Paths: paths, Query: tc.query, Language: "groovy", Encoding: "utf-8", Match: "exact", Mode: "structural", MaxResults: 32})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("structural search %s: error=%v failed=%+v search=%+v", tc.query, err, failed, search.Search)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.negative))}, Query: tc.query, Language: "groovy", Encoding: "utf-8", Match: "exact", MaxFiles: 1, MaxSymbols: 16})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated Groovy source falsely defines %s: error=%v failed=%+v symbols=%+v", tc.query, err, failed, negative.Symbols)
			}
			if tc.project == "groovy-jenkinsci-jenkinspipelineunit" {
				path := filepath.Join(root, filepath.FromSlash(tc.files[0]))
				failed, methods, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: "registerAllowedMethod", Match: "exact",
					Language: "groovy", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
				})
				if err != nil || failed != nil || methods.Truncated || len(methods.Symbols) != 5 {
					t.Fatalf("real overloads versus spurious calls: error=%v failed=%+v symbols=%+v", err, failed, methods.Symbols)
				}
				failed, ghost, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: "closeQuietly", Match: "exact",
					Language: "groovy", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
				})
				if err != nil || failed != nil || ghost.Truncated || len(ghost.Symbols) != 0 {
					t.Fatalf("closure invocation incorrectly indexed as method: error=%v failed=%+v symbols=%+v", err, failed, ghost.Symbols)
				}
			}
		})
	}
}
