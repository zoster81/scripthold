package handler

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// A complete original source-file inventory is not a semantic qualification:
// opaque macro-generated scopes remain explicitly partial, never excluded.
func TestSourceSymbolsOriginalCppBehaviorTreeWholeProject(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original source corpus not configured")
	}
	project := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-behaviortree-behaviortree.cpp")
	var originals []string
	err := filepath.WalkDir(project, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() && e.Name() == ".git" {
			return filepath.SkipDir
		}
		if !e.Type().IsRegular() {
			return nil
		}
		switch filepath.Ext(p) {
		case ".cpp", ".cc", ".cxx", ".hpp", ".h", ".hxx", ".ipp", ".inl", ".tpp":
			originals = append(originals, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(originals)
	const eligible = 226
	if len(originals) != eligible {
		t.Fatalf("original C++ file inventory changed: %d/%d", len(originals), eligible)
	}
	h := NewHandler([]string{root})
	fingerprints := make(map[string]string, eligible)
	// Exact original-file partial baseline, reviewed after the full project
	// scan. Do not exclude a source, or call an unresolved file complete.
	expectedPartial := map[string]bool{
		"3rdparty/cppzmq/zmq.hpp":                   true,
		"3rdparty/flatbuffers/flatbuffers/base.h":   true,
		"examples/plugin_example/plugin_action.cpp": true,
		"examples/t11_groot_howto.cpp":              true,
		"examples/t12_default_ports.cpp":            true,
		"include/behaviortree_cpp/contrib/json.hpp": true,
		"sample_nodes/crossdoor_nodes.cpp":          true,
		"sample_nodes/dummy_nodes.cpp":              true,
		"sample_nodes/movebase_node.cpp":            true,
		"sample_nodes/movebase_node.h":              true,
		"tests/gtest_async_action_node.cpp":         true,
		"tests/gtest_decorator.cpp":                 true,
		"tests/gtest_fallback.cpp":                  true,
		"tests/gtest_if_then_else.cpp":              true,
		"tests/gtest_json.cpp":                      true,
		"tests/gtest_loggers.cpp":                   true,
		"tests/gtest_name_validation.cpp":           true,
		"tests/gtest_parallel.cpp":                  true,
		"tests/gtest_plugin_issue953.cpp":           true,
		"tests/gtest_ports.cpp":                     true,
		"tests/gtest_sequence.cpp":                  true,
		"tests/gtest_switch.cpp":                    true,
		"tests/gtest_tree.cpp":                      true,
		"tests/gtest_try_catch.cpp":                 true,
		"tests/gtest_while_do_else.cpp":             true,
		"tests/plugin_issue953/plugin_issue953.cpp": true,
	}
	observedPartial := make(map[string]bool, len(expectedPartial))
	partial := 0
	for start := 0; start < eligible; start += 32 {
		end := min(start+32, eligible)
		expected := make(map[string]bool, end-start)
		for _, p := range originals[start:end] {
			expected[p] = true
		}
		fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Presentation: "compact", Paths: originals[start:end],
			Language: "cpp", Encoding: "utf-8", MaxFiles: 32,
		})
		if err != nil || fail != nil || out.Truncated || out.FilesConsidered != end-start ||
			out.FilesParsed != end-start || out.FilesSkipped != 0 || len(out.OverviewFiles) != end-start {
			t.Fatalf("original source batch %d:%d incomplete: err=%v fail=%+v parsed=%d skipped=%d",
				start, end, err, fail, out.FilesParsed, out.FilesSkipped)
		}
		batchComplete := true
		for _, f := range out.OverviewFiles {
			if !expected[f.Path] || fingerprints[f.Path] != "" || f.Status != "parsed" ||
				f.Language != "cpp" || f.SourceFingerprint == "" {
				t.Errorf("invalid or duplicated original source: %+v", f)
			}
			relative := filepath.ToSlash(strings.TrimPrefix(f.Path, project+string(filepath.Separator)))
			if f.CoverageComplete == expectedPartial[relative] {
				t.Errorf("original BehaviorTree.CPP source coverage changed unexpectedly: %s complete=%v", relative, f.CoverageComplete)
			}
			if !f.CoverageComplete {
				batchComplete = false
				partial++
				observedPartial[relative] = true
				t.Logf("PARTIAL %s", relative)
			}
			fingerprints[f.Path] = f.SourceFingerprint
			delete(expected, f.Path)
		}
		if out.CoverageComplete != batchComplete {
			t.Errorf("source batch coverage disagrees with files: %d:%d", start, end)
		}
		for p := range expected {
			t.Errorf("original source excluded: %s", p)
		}
	}
	if len(fingerprints) != eligible {
		t.Fatalf("only %d/%d source files reconciled", len(fingerprints), eligible)
	}
	if len(observedPartial) != len(expectedPartial) {
		t.Errorf("BehaviorTree.CPP partial-source inventory changed: got %d, expected %d", len(observedPartial), len(expectedPartial))
	}
	for relative := range expectedPartial {
		if !observedPartial[relative] {
			t.Errorf("original BehaviorTree.CPP expected incomplete source not reported: %s", relative)
		}
	}
	t.Logf("original BehaviorTree.CPP: %d source files, %d complete, %d explicitly partial", eligible, eligible-partial, partial)

	// Independent original full-tree grep: AlwaysFailureNode has four text
	// occurrences, AlwaysSuccessNode three. Only the class and constructor
	// in each matching header are authored declarations; others are usages.
	for _, tc := range []struct{ query, header string }{
		{"AlwaysFailureNode", "always_failure_node.h"},
		{"AlwaysSuccessNode", "always_success_node.h"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			found := map[string]bool{}
			path := filepath.Join(project, "include", "behaviortree_cpp", "actions", tc.header)
			for start := 0; start < eligible; start += 32 {
				end := min(start+32, eligible)
				fail, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Match: "exact", Query: tc.query,
					Paths: originals[start:end], Language: "cpp", Encoding: "utf-8",
					MaxFiles: 32, MaxResults: 20,
				})
				if err != nil || fail != nil || out.Search == nil || out.Coverage.FilesParsed != end-start {
					t.Fatalf("source search %d:%d failed: %v %+v", start, end, err, fail)
				}
				for _, m := range out.Search.Matches {
					key := m.QualifiedName + "@" + strconv.Itoa(m.Range.Start.Line)
					class := key == "BT."+tc.query+"@22" && string(m.Kind) == "class"
					ctor := key == "BT."+tc.query+"."+tc.query+"@25" && string(m.Kind) == "constructor"
					if (!class && !ctor) || m.Path != path || m.SymbolID == "" ||
						m.SourceFingerprint != fingerprints[m.Path] || found[key] {
						t.Errorf("false, unbound or duplicate original C++ declaration: %+v", m)
					}
					found[key] = true
				}
			}
			if !found["BT."+tc.query+"@22"] || !found["BT."+tc.query+"."+tc.query+"@25"] || len(found) != 2 {
				t.Errorf("expected original class and constructor only, got %+v", found)
			}
		})
	}
}

// This original source has an authored assignment operator and a C++
// explicitly defaulted destructor, neither a variable nor a brace body.
func TestSourceSymbolsOriginalCppBehaviorTreeDefaultedDestructor(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original source corpus not configured")
	}
	path := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-behaviortree-behaviortree.cpp", "src", "tree_node.cpp")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(original), "TreeNode& TreeNode::operator=(TreeNode&& other) noexcept") ||
		!strings.Contains(string(original), "TreeNode::~TreeNode() = default;") {
		t.Fatal("original project oracle has changed")
	}
	h := NewHandler([]string{root})
	fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128,
	})
	if err != nil || fail != nil || out.Truncated || !out.CoverageComplete || len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
		t.Fatalf("original BehaviorTree source not fully analyzed: err=%v fail=%+v out=%+v", err, fail, out)
	}
	operatorCount, defaultedCount := 0, 0
	var near []string
	for _, symbol := range out.Symbols {
		line := symbol.NameRange.Start.Line
		if line != 63 && line != 69 {
			continue
		}
		near = append(near, strconv.Itoa(line)+":"+string(symbol.Kind)+":"+symbol.QualifiedName+":"+symbol.NativeKind+":"+symbol.ParentQualifiedName)
		if symbol.Kind == "variable" {
			t.Errorf("false original declaration at line %d: %+v", line, symbol)
		}
		if line == 63 && symbol.Kind == "operator" && symbol.Name == "operator=" &&
			symbol.QualifiedName == "BT.TreeNode.operator=" && symbol.BodyRange != nil {
			operatorCount++
		}
		if line == 69 && symbol.Kind == "function" && symbol.QualifiedName == "BT.TreeNode.TreeNode" &&
			symbol.NativeKind == "qualified-destructor-defaulted-definition" && symbol.BodyRange == nil &&
			symbol.SignatureRange != nil && symbol.ParentQualifiedName == "BT" {
			defaultedCount++
		}
	}
	if operatorCount != 1 || defaultedCount != 1 {
		t.Fatalf("original C++ operator count=%d defaulted destructor count=%d; nearby=%v", operatorCount, defaultedCount, near)
	}
}

// The original class declares a deleted copy assignment and a move assignment.
// Both are operators, not fields; only the move assignment has a written body.
func TestSourceSymbolsOriginalCppBehaviorTreeAssignmentOverloadOracle(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original source corpus not configured")
	}
	base := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-behaviortree-behaviortree.cpp")
	header := filepath.Join(base, "include", "behaviortree_cpp", "tree_node.h")
	source := filepath.Join(base, "src", "tree_node.cpp")
	for _, fixture := range []struct{ path, exact string }{
		{header, "TreeNode& operator=(const TreeNode& other) = delete;"},
		{header, "TreeNode& operator=(TreeNode&& other) noexcept;"},
		{source, "TreeNode& TreeNode::operator=(TreeNode&& other) noexcept"},
	} {
		data, err := os.ReadFile(fixture.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), fixture.exact) {
			t.Fatalf("original source changed: %s", fixture.exact)
		}
	}
	h := NewHandler([]string{root})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{header, source},
		Language: "cpp", Encoding: "utf-8", MaxFiles: 2, MaxSymbols: 400,
	})
	if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || len(out.Files) != 2 {
		t.Fatalf("original assignment-operator oracle incomplete: err=%v fail=%+v", err, failed)
	}
	got := map[string]bool{}
	for _, s := range out.Symbols {
		if s.Path != header && s.Path != source {
			t.Errorf("out-of-scope source: %+v", s)
			continue
		}
		line := s.NameRange.Start.Line
		if s.Path == header && line != 171 && line != 174 {
			continue
		}
		if s.Path == source && line != 63 {
			continue
		}
		if string(s.Kind) != "operator" || s.Name != "operator=" || s.QualifiedName != "BT.TreeNode.operator=" ||
			s.ID == "" || (s.Path == header && (s.ParentID == "" || s.ParentQualifiedName != "BT.TreeNode")) ||
			(s.Path == source && (s.NativeKind != "operator-definition" || s.BodyRange == nil)) {
			t.Errorf("false original C++ operator classification: %+v", s)
			continue
		}
		got[filepath.Base(s.Path)+"@"+strconv.Itoa(line)] = true
	}
	if !got["tree_node.h@171"] || !got["tree_node.h@174"] || !got["tree_node.cpp@63"] || len(got) != 3 {
		t.Fatalf("missing original assignment overload: %+v", got)
	}
}

// Exhaustive independent declaration inventories for two small original
// headers; class constructors and methods must preserve real ownership and
// source positions, with no fabricated base-class or call symbols.
func TestSourceSymbolsOriginalCppBehaviorTreeActionHeadersOracles(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original source corpus not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct{ name, filename, returned string }{
		{"AlwaysFailureNode", "always_failure_node.h", "FAILURE"},
		{"AlwaysSuccessNode", "always_success_node.h", "SUCCESS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, "qualification", "final-candidate-corpus",
				"project-cpp-behaviortree-behaviortree.cpp", "include", "behaviortree_cpp", "actions", tc.filename)
			fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 20,
			})
			if err != nil || fail != nil || !out.CoverageComplete || out.Truncated ||
				len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
				t.Fatalf("original %s source-derived inventory unavailable: %v %+v", tc.name, err, fail)
			}
			type expected struct {
				kind, owner string
				line        int
			}
			want := map[string]expected{
				"BT":                            {"namespace", "", 17},
				"BT." + tc.name:                 {"class", "BT", 22},
				"BT." + tc.name + "." + tc.name: {"constructor", "BT." + tc.name, 25},
				"BT." + tc.name + ".tick":       {"method", "BT." + tc.name, 31},
			}
			if len(out.Symbols) != len(want) {
				t.Errorf("%s original header has 4 authored declarations, returned %d", tc.name, len(out.Symbols))
			}
			tickID := ""
			for _, s := range out.Symbols {
				e, ok := want[s.QualifiedName]
				if !ok || string(s.Kind) != e.kind || s.NameRange.Start.Line != e.line ||
					s.ParentQualifiedName != e.owner || s.ID == "" || (e.owner != "" && s.ParentID == "") {
					t.Errorf("original C++ header false or missing declaration/owner: %+v expected=%+v", s, e)
				}
				if s.QualifiedName == "BT."+tc.name+".tick" {
					tickID = s.ID
				}
				delete(want, s.QualifiedName)
			}
			for name := range want {
				t.Errorf("missing authored declaration %s", name)
			}
			if tickID == "" {
				t.Fatal("original tick() missing")
			}
			fail, show, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, SymbolID: tickID,
				SourceFingerprint: out.Files[0].SourceFingerprint,
				Language:          "cpp", Encoding: "utf-8", MaxBytes: 2048,
			})
			if err != nil || fail != nil || show.Show == nil ||
				!strings.Contains(show.Show.Text, "return NodeStatus::"+tc.returned+";") {
				t.Errorf("original tick body missing/stale for %s: %v %+v", tc.name, err, fail)
			}
		})
	}
}
