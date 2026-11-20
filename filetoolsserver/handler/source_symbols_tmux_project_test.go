package handler

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Original tmux whole-project C screening plus independently complete
// declaration inventories in two headers and one implementation file.
// This does not imply all project semantics or relations are qualified.
func TestSourceSymbolsOriginalTmuxWholeProjectScreen(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	project := filepath.Join(root, "real-projects", "c-tmux-tmux")
	h := NewHandler([]string{root})
	include := []string{"**/*.c", "**/*.h"}
	exclude := []string{"**/.git/**"}
	original := map[string]bool{}
	err := filepath.WalkDir(project, func(path string, e fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if e.IsDir() && e.Name() == ".git" {
			return filepath.SkipDir
		}
		if e.Type().IsRegular() && (filepath.Ext(path) == ".c" || filepath.Ext(path) == ".h") {
			original[path] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	const files = 207 // Pinned original checkout: 198 .c files, 9 .h files.
	if len(original) != files {
		t.Fatalf("tmux original source set drift: got %d want %d", len(original), files)
	}
	fail, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "digest", Presentation: "compact", Paths: []string{project},
		Language: "c", Encoding: "utf-8", Includes: include, Excludes: exclude, MaxFiles: 240,
	})
	if err != nil || fail != nil || !digest.CoverageComplete || digest.Truncated ||
		digest.FilesConsidered != files || digest.FilesParsed != files ||
		digest.FilesSkipped != 0 || len(digest.OverviewFiles) != files {
		t.Fatalf("tmux full project source map incomplete: err=%v fail=%+v digest=%+v", err, fail, digest)
	}
	fingerprints := make(map[string]string, files)
	for _, f := range digest.OverviewFiles {
		if !original[f.Path] || fingerprints[f.Path] != "" || !f.CoverageComplete ||
			f.Status != "parsed" || f.Language != "c" || f.SourceFingerprint == "" {
			t.Errorf("unexpected/unparsed/duplicate original source: %+v", f)
		}
		fingerprints[f.Path] = f.SourceFingerprint
	}
	for path := range original {
		if fingerprints[path] == "" {
			t.Errorf("source missing from C project map: %s", path)
		}
	}

	// Source-judged complete inventories, not selected positives alone.
	// Each line is qualified name, kind, exact name line, and parent or '-'.
	for _, tc := range []struct{ file, oracle string }{
		{"tmux-protocol.h", `
msgtype enum 26 -
msg_command struct 79 -
msg_command.argc field 80 msg_command
msg_read_open struct 83 -
msg_read_open.stream field 84 msg_read_open
msg_read_open.fd field 85 msg_read_open
msg_read_data struct 88 -
msg_read_data.stream field 89 msg_read_data
msg_read_done struct 92 -
msg_read_done.stream field 93 msg_read_done
msg_read_done.error field 94 msg_read_done
msg_read_cancel struct 97 -
msg_read_cancel.stream field 98 msg_read_cancel
msg_write_open struct 101 -
msg_write_open.stream field 102 msg_write_open
msg_write_open.fd field 103 msg_write_open
msg_write_open.flags field 104 msg_write_open
msg_write_data struct 107 -
msg_write_data.stream field 108 msg_write_data
msg_write_ready struct 111 -
msg_write_ready.stream field 112 msg_write_ready
msg_write_ready.error field 113 msg_write_ready
msg_write_close struct 116 -
msg_write_close.stream field 117 msg_write_close
msg_write_done struct 120 -
msg_write_done.stream field 121 msg_write_done
msg_write_done.error field 122 msg_write_done
`},
		{filepath.Join("compat", "vis.h"), `
vis function 76 -
strvis function 77 -
stravis function 78 -
strnvis function 79 -
strvisx function 80 -
strunvis function 81 -
unvis function 82 -
strnunvis function 83 -
`},
		{"cmd-attach-session.c", `
cmd_attach_session_exec function 33 -
cmd_attach_session_entry constant 36 -
cmd_attach_session function 51 -
`},
	} {
		// cmd_attach_session_exec has both a prototype and a definition in
		// this original file. Keep their identical name as separate symbols.
		path := filepath.Join(project, tc.file)
		fail, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Presentation: "detailed", Paths: []string{path},
			Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 120,
		})
		if err != nil || fail != nil || !outline.CoverageComplete || len(outline.Files) != 1 {
			t.Fatalf("original %s outline failed: %v %+v %+v", tc.file, err, fail, outline)
		}
		type expectedSymbol struct {
			kind, parent string
			line         int
		}
		want := map[string]expectedSymbol{}
		for _, line := range strings.Split(strings.TrimSpace(tc.oracle), "\n") {
			parts := strings.Fields(line)
			if len(parts) != 4 {
				t.Fatalf("bad original oracle line %q", line)
			}
			number, err := strconv.Atoi(parts[2])
			if err != nil {
				t.Fatal(err)
			}
			parent := parts[3]
			if parent == "-" {
				parent = ""
			}
			want[parts[0]+"@"+parts[2]] = expectedSymbol{parts[1], parent, number}
		}
		if tc.file == "cmd-attach-session.c" {
			want["cmd_attach_session_exec@177"] = expectedSymbol{"function", "", 177}
		}
		if len(outline.Symbols) != len(want) {
			t.Errorf("%s declarations=%d want=%d", tc.file, len(outline.Symbols), len(want))
		}
		for _, symbol := range outline.Symbols {
			key := symbol.QualifiedName + "@" + strconv.Itoa(symbol.NameRange.Start.Line)
			expected, ok := want[key]
			if !ok || string(symbol.Kind) != expected.kind ||
				symbol.ParentQualifiedName != expected.parent || symbol.ID == "" {
				t.Errorf("%s unexpected or wrong symbol: %+v oracle=%+v found=%v", tc.file, symbol, expected, ok)
			}
			delete(want, key)
		}
		for key := range want {
			t.Errorf("%s missing original declaration %s", tc.file, key)
		}
	}

	for _, question := range []struct {
		name  string
		lines map[string]int
	}{
		{"cmd_attach_session", map[string]int{"cmd-attach-session.c": 51, "tmux.h": 3167}},
		{"cmd_attach_session_exec", map[string]int{"cmd-attach-session.c": 33}},
	} {
		fail, result, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Mode: "structural", Match: "exact", Query: question.name,
			Paths: []string{project}, Language: "c", Encoding: "utf-8",
			Includes: include, Excludes: exclude, MaxFiles: 240, MaxResults: 10,
		})
		wantCount := len(question.lines)
		if question.name == "cmd_attach_session_exec" {
			wantCount = 2 // File-local prototype and definition.
		}
		if err != nil || fail != nil || !result.Coverage.CoverageComplete ||
			result.Coverage.FilesParsed != files || result.Search == nil ||
			len(result.Search.Matches) != wantCount {
			t.Fatalf("%s project search missing: %v %+v %+v", question.name, err, fail, result)
		}
		seenLines := map[int]bool{}
		for _, match := range result.Search.Matches {
			relative := relativeSourcePath(project, match.Path)
			line := match.Range.Start.Line
			wanted := line == question.lines[relative]
			if question.name == "cmd_attach_session_exec" && relative == "cmd-attach-session.c" {
				wanted = line == 33 || line == 177
			}
			if !wanted || seenLines[line] || string(match.Kind) != "function" ||
				match.SourceFingerprint != fingerprints[match.Path] || match.SymbolID == "" {
				t.Errorf("%s incorrect structural declaration: %+v", question.name, match)
			}
			seenLines[line] = true
		}
	}

	source := filepath.Join(project, "cmd-attach-session.c")
	fail, rel, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Relation: "dependencies", Paths: []string{project},
		Language: "c", Encoding: "utf-8", Includes: include, Excludes: exclude,
		MaxFiles: 240, MaxResults: 40, MaxNodes: 90, MaxEdges: 90, MaxDepth: 1,
		Subject: &SourceSelectorInput{Kind: "path", Path: source, SourceFingerprint: fingerprints[source]},
	})
	if err != nil || fail != nil || !rel.Coverage.CoverageComplete || rel.Relations == nil ||
		rel.Relations.EvidenceScope != "indexed-relations-only" {
		t.Fatalf("tmux dependencies unavailable: %v %+v %+v", err, fail, rel)
	}
	found := false
	for _, edge := range rel.Relations.Relations {
		if edge.Source.Path == source && edge.Source.Range != nil &&
			edge.Source.Range.Start.Line == 27 &&
			edge.Target.Path == filepath.Join(project, "tmux.h") &&
			edge.Target.SourceFingerprint == fingerprints[edge.Target.Path] &&
			string(edge.Resolution) == "resolved" && string(edge.Evidence) == "project-resolved" {
			found = true
		}
	}
	if !found {
		t.Error("cmd-attach-session.c original tmux.h include not resolved")
	}
	fail, foundSymbols, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Query: "cmd_attach_session", Match: "exact",
		Paths: []string{source}, Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 8,
	})
	if err != nil || fail != nil || !foundSymbols.CoverageComplete || len(foundSymbols.Symbols) != 1 {
		t.Fatalf("tmux original function lookup incomplete: %v %+v %+v", err, fail, foundSymbols)
	}
	fail, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: source, SymbolID: foundSymbols.Symbols[0].ID,
		SourceFingerprint: fingerprints[source], Language: "c", Encoding: "utf-8", MaxBytes: 16384,
	})
	if err != nil || fail != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "sessions should be nested with care") {
		t.Fatalf("tmux original body retrieval mismatch: %v %+v %+v", err, fail, shown)
	}
	t.Logf("tmux C screen: %d files, complete three-file independent semantic oracles, cross-file declarations and source-bound body", files)
}
