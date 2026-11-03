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

// Every original Xiaozhi ESP32 C++ source, board variant, test and vendored
// header is accounted for. Structural coverage is not a full semantic oracle.
func TestSourceSymbolsOriginalCppXiaozhiWholeProject(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus not configured")
	}
	project := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-78-xiaozhi-esp32")
	var files []string
	err := filepath.WalkDir(project, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".cc", ".cpp", ".cxx", ".h", ".hpp", ".hxx", ".ipp", ".inl", ".tpp":
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	const expectedFiles = 602
	if len(files) != expectedFiles {
		t.Fatalf("original Xiaozhi C++ inventory changed: %d/%d", len(files), expectedFiles)
	}
	h := NewHandler([]string{root})
	fingerprints := make(map[string]string, expectedFiles)
	// Exactly one original board variant has an unresolved structural region.
	// It must remain in every inventory; no source is silently excluded.
	expectedPartial := "main/boards/espressif/esp32-s3-lcd-ev-board-2/esp32-s3-lcd-ev-board-2.cc"
	observedPartial := false
	partial := make([]string, 0)
	for start := 0; start < len(files); start += 32 {
		end := min(start+32, len(files))
		selected := files[start:end]
		want := make(map[string]bool, len(selected))
		for _, path := range selected {
			want[path] = true
		}
		fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Presentation: "compact", Paths: selected,
			Language: "cpp", MaxFiles: 32,
		})
		if err != nil || fail != nil || out.Truncated ||
			out.FilesConsidered != len(selected) || out.FilesParsed != len(selected) ||
			out.FilesSkipped != 0 || len(out.OverviewFiles) != len(selected) {
			t.Fatalf("original Xiaozhi C++ batch %d:%d not accounted for: err=%v fail=%+v considered=%d parsed=%d skipped=%d",
				start, end, err, fail, out.FilesConsidered, out.FilesParsed, out.FilesSkipped)
		}
		batchComplete := true
		for _, f := range out.OverviewFiles {
			if !want[f.Path] || fingerprints[f.Path] != "" ||
				f.Status != "parsed" || f.Language != "cpp" || f.SourceFingerprint == "" {
				t.Errorf("missing, duplicated or unbound original source: %+v", f)
			}
			relative := filepath.ToSlash(strings.TrimPrefix(f.Path, project+string(filepath.Separator)))
			if f.CoverageComplete == (relative == expectedPartial) {
				t.Errorf("original Xiaozhi structural source coverage changed: %s complete=%v", relative, f.CoverageComplete)
			}
			if !f.CoverageComplete {
				batchComplete = false
				partial = append(partial, relative)
				if relative == expectedPartial {
					observedPartial = true
				}
			}
			fingerprints[f.Path] = f.SourceFingerprint
			delete(want, f.Path)
		}
		if out.CoverageComplete != batchComplete {
			t.Errorf("original source batch %d:%d coverage flag inconsistent", start, end)
		}
		for path := range want {
			t.Errorf("original source omitted: %s", path)
		}
	}
	if len(fingerprints) != expectedFiles {
		t.Fatalf("only %d/%d source files fingerprint-bound", len(fingerprints), expectedFiles)
	}
	if !observedPartial || len(partial) != 1 {
		t.Errorf("original Xiaozhi incomplete-source baseline changed: %+v", partial)
	}
	sort.Strings(partial)
	t.Logf("original Xiaozhi ESP32 C++: %d scanned, %d structurally complete, %d explicitly partial in 19 bounded batches", expectedFiles, expectedFiles-len(partial), len(partial))
	for _, path := range partial {
		t.Logf("PARTIAL %s", path)
	}

	// Independent original grep across all 602 files finds seven textual
	// DummyAudioCodec hits in exactly these two files, including usages in
	// qualified definitions. Only the header's class/constructor/destructor
	// and source's actual methods are declarations.
	header := filepath.Join(project, "main", "audio", "codecs", "dummy_audio_codec.h")
	implementation := filepath.Join(project, "main", "audio", "codecs", "dummy_audio_codec.cc")
	foundClass := false
	seen := make(map[string]bool)
	for start := 0; start < len(files); start += 32 {
		end := min(start+32, len(files))
		selected := files[start:end]
		fail, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Mode: "structural", Match: "exact",
			Query: "DummyAudioCodec", Paths: selected, Language: "cpp",
			MaxFiles: 32, MaxResults: 18,
		})
		if err != nil || fail != nil || out.Search == nil ||
			out.Coverage.FilesParsed != len(selected) || out.Coverage.FilesSkipped != 0 {
			t.Fatalf("original Xiaozhi whole-project source query %d:%d incomplete: err=%v fail=%+v coverage=%+v",
				start, end, err, fail, out.Coverage)
		}
		for _, match := range out.Search.Matches {
			key := match.Path + "@" + strconv.Itoa(match.Range.Start.Line)
			if (match.Path != header && match.Path != implementation) ||
				match.SymbolID == "" || match.SourceFingerprint != fingerprints[match.Path] ||
				seen[key] {
				t.Errorf("invented, duplicate or unbound original C++ search match: %+v", match)
			}
			seen[key] = true
			if match.Path == header && match.QualifiedName == "DummyAudioCodec" &&
				string(match.Kind) == "class" && match.Range.Start.Line == 6 {
				foundClass = true
			}
		}
	}
	if !foundClass {
		t.Error("original DummyAudioCodec class missing from project-wide structured navigation")
	}
}

// The original CRLF header declares a class, two methods, constructor and
// destructor. No implicit constructor, field, call or macro is a declaration.
func TestSourceSymbolsOriginalCppXiaozhiDummyAudioFullHeaderOracle(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus not configured")
	}
	file := filepath.Join(root, "qualification", "final-candidate-corpus",
		"project-cpp-78-xiaozhi-esp32", "main", "audio", "codecs", "dummy_audio_codec.h")
	h := NewHandler([]string{root})
	fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{file},
		Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 18,
	})
	if err != nil || fail != nil || out.Truncated || !out.CoverageComplete ||
		len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
		t.Fatalf("original Xiaozhi complete header oracle unavailable: err=%v fail=%+v", err, fail)
	}
	type expected struct {
		kind, parent string
		line         int
	}
	want := map[string]expected{
		"DummyAudioCodec@6":                  {"class", "", 6},
		"DummyAudioCodec.Read@8":             {"method", "DummyAudioCodec", 8},
		"DummyAudioCodec.Write@9":            {"method", "DummyAudioCodec", 9},
		"DummyAudioCodec.DummyAudioCodec@12": {"constructor", "DummyAudioCodec", 12},
		"DummyAudioCodec.DummyAudioCodec@13": {"destructor", "DummyAudioCodec", 13},
	}
	if len(out.Symbols) != len(want) {
		t.Errorf("original header has five authored declarations, returned %d: %+v", len(out.Symbols), out.Symbols)
	}
	for _, symbol := range out.Symbols {
		key := symbol.QualifiedName + "@" + strconv.Itoa(symbol.NameRange.Start.Line)
		e, ok := want[key]
		if !ok || string(symbol.Kind) != e.kind || symbol.ParentQualifiedName != e.parent ||
			symbol.NameRange.Start.Line != e.line || symbol.ID == "" ||
			(e.parent != "" && symbol.ParentID == "") {
			t.Errorf("fabricated or misowned original C++ header symbol: %+v expected=%+v exists=%v", symbol, e, ok)
		}
		delete(want, key)
	}
	for name := range want {
		t.Errorf("original C++ header declaration missing: %s", name)
	}
}

func TestSourceSymbolsOriginalCppXiaozhiQualifiedDefinitions(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus not configured")
	}
	file := filepath.Join(root, "qualification", "final-candidate-corpus",
		"project-cpp-78-xiaozhi-esp32", "main", "audio", "codecs", "dummy_audio_codec.cc")
	h := NewHandler([]string{root})
	fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{file},
		Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
	})
	if err != nil || fail != nil || out.Truncated || !out.CoverageComplete ||
		len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
		t.Fatalf("original qualified C++ source unreadable: err=%v fail=%+v out=%+v", err, fail, out)
	}
	want := map[string]bool{
		"DummyAudioCodec.DummyAudioCodec@3":  false,
		"DummyAudioCodec.DummyAudioCodec@11": false,
		"DummyAudioCodec.Read@14":            false,
		"DummyAudioCodec.Write@18":           false,
	}
	if len(out.Symbols) != len(want) {
		t.Fatalf("expected four authored source definitions, got %+v", out.Symbols)
	}
	for _, symbol := range out.Symbols {
		key := symbol.QualifiedName + "@" + strconv.Itoa(symbol.NameRange.Start.Line)
		if _, exists := want[key]; !exists || want[key] || symbol.ID == "" ||
			symbol.BodyRange == nil || symbol.ParentID != "" ||
			symbol.ParentQualifiedName != "" || string(symbol.Kind) != "function" {
			t.Errorf("unresolved cross-file C++ definition misrepresented: %+v", symbol)
		}
		want[key] = true
		if key == "DummyAudioCodec.Read@14" {
			failed, shown, showErr := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: file, SymbolID: symbol.ID,
				SourceFingerprint: out.Files[0].SourceFingerprint, Language: "cpp", Encoding: "utf-8", MaxBytes: 1024,
			})
			if showErr != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, "return 0;") {
				t.Errorf("out-of-class Read body not retrievable: err=%v fail=%+v", showErr, failed)
			}
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("missing original qualified C++ definition %s", key)
		}
	}
}
