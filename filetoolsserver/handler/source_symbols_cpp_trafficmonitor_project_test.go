package handler

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Inventory every original TrafficMonitor C++ source, keeping the single
// original GB18030 header in its own explicitly decoded source batch.
func TestSourceSymbolsOriginalCppTrafficMonitorWholeProject(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus not configured")
	}
	project := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-zhongyang219-trafficmonitor")
	var all []string
	err := filepath.WalkDir(project, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() && e.Name() == ".git" {
			return filepath.SkipDir
		}
		if !e.Type().IsRegular() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".cpp", ".cc", ".cxx", ".h", ".hpp", ".hxx", ".ipp", ".inl", ".tpp":
			all = append(all, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(all)
	if len(all) != 222 {
		t.Fatalf("pinned original source count changed: %d instead of 222", len(all))
	}
	special := filepath.Join(project, "TrafficMonitor", "WineTaskbarDlg.h")
	var regular []string
	for _, p := range all {
		if p != special {
			regular = append(regular, p)
		}
	}
	if len(regular) != 221 {
		t.Fatal("original GB18030 source missing")
	}
	h := NewHandler([]string{root})
	fp := make(map[string]string, len(all))
	// Original partial regions are kept in the inventory, never excluded.
	expectedPartial := map[string]bool{
		"PluginDemo/OptionsDlg.cpp":                    true,
		"PluginDemo/OptionsDlg.h":                      true,
		"TrafficMonitor/AboutDlg.cpp":                  true,
		"TrafficMonitor/AboutDlg.h":                    true,
		"TrafficMonitor/AppAlreadyRuningDlg.cpp":       true,
		"TrafficMonitor/AppAlreadyRuningDlg.h":         true,
		"TrafficMonitor/BaseDialog.cpp":                true,
		"TrafficMonitor/BaseDialog.h":                  true,
		"TrafficMonitor/CAutoAdaptSettingsDlg.cpp":     true,
		"TrafficMonitor/CAutoAdaptSettingsDlg.h":       true,
		"TrafficMonitor/CMFCColorDialogEx.cpp":         true,
		"TrafficMonitor/CMFCColorDialogEx.h":           true,
		"TrafficMonitor/CSkinPreviewView.cpp":          true,
		"TrafficMonitor/CSkinPreviewView.h":            true,
		"TrafficMonitor/CTabCtrlEx.cpp":                true,
		"TrafficMonitor/CTabCtrlEx.h":                  true,
		"TrafficMonitor/ColorSettingListCtrl.cpp":      true,
		"TrafficMonitor/ColorSettingListCtrl.h":        true,
		"TrafficMonitor/ColorStatic.cpp":               true,
		"TrafficMonitor/ColorStatic.h":                 true,
		"TrafficMonitor/ComboBox2.cpp":                 true,
		"TrafficMonitor/ComboBox2.h":                   true,
		"TrafficMonitor/DisplayTextSettingDlg.cpp":     true,
		"TrafficMonitor/DisplayTextSettingDlg.h":       true,
		"TrafficMonitor/GeneralSettingsDlg.cpp":        true,
		"TrafficMonitor/GeneralSettingsDlg.h":          true,
		"TrafficMonitor/HistoryTrafficCalendarDlg.cpp": true,
		"TrafficMonitor/HistoryTrafficCalendarDlg.h":   true,
		"TrafficMonitor/HistoryTrafficDlg.cpp":         true,
		"TrafficMonitor/HistoryTrafficDlg.h":           true,
		"TrafficMonitor/HistoryTrafficListCtrl.cpp":    true,
		"TrafficMonitor/HistoryTrafficListCtrl.h":      true,
		"TrafficMonitor/HistoryTrafficListDlg.cpp":     true,
		"TrafficMonitor/HistoryTrafficListDlg.h":       true,
		"TrafficMonitor/IconSelectDlg.cpp":             true,
		"TrafficMonitor/IconSelectDlg.h":               true,
		"TrafficMonitor/LinkStatic.cpp":                true,
		"TrafficMonitor/LinkStatic.h":                  true,
		"TrafficMonitor/ListCtrlEx.cpp":                true,
		"TrafficMonitor/ListCtrlEx.h":                  true,
		"TrafficMonitor/MainWndColorDlg.cpp":           true,
		"TrafficMonitor/MainWndColorDlg.h":             true,
		"TrafficMonitor/MainWndSettingsDlg.cpp":        true,
		"TrafficMonitor/MainWndSettingsDlg.h":          true,
		"TrafficMonitor/MessageDlg.cpp":                true,
		"TrafficMonitor/MessageDlg.h":                  true,
		"TrafficMonitor/NetworkInfoDlg.cpp":            true,
		"TrafficMonitor/NetworkInfoDlg.h":              true,
		"TrafficMonitor/OptionsDlg.cpp":                true,
		"TrafficMonitor/OptionsDlg.h":                  true,
		"TrafficMonitor/PictureStatic.cpp":             true,
		"TrafficMonitor/PictureStatic.h":               true,
		"TrafficMonitor/PluginInfoDlg.cpp":             true,
		"TrafficMonitor/PluginInfoDlg.h":               true,
		"TrafficMonitor/PluginManagerDlg.cpp":          true,
		"TrafficMonitor/PluginManagerDlg.h":            true,
		"TrafficMonitor/SelectConnectionsDlg.cpp":      true,
		"TrafficMonitor/SelectConnectionsDlg.h":        true,
		"TrafficMonitor/SetItemOrderDlg.cpp":           true,
		"TrafficMonitor/SetItemOrderDlg.h":             true,
		"TrafficMonitor/SkinAutoAdaptSettingDlg.cpp":   true,
		"TrafficMonitor/SkinAutoAdaptSettingDlg.h":     true,
		"TrafficMonitor/SkinDlg.cpp":                   true,
		"TrafficMonitor/SkinDlg.h":                     true,
		"TrafficMonitor/SpinEdit.cpp":                  true,
		"TrafficMonitor/SpinEdit.h":                    true,
		"TrafficMonitor/StaticEx.cpp":                  true,
		"TrafficMonitor/StaticEx.h":                    true,
		"TrafficMonitor/TabDlg.cpp":                    true,
		"TrafficMonitor/TabDlg.h":                      true,
		"TrafficMonitor/TaskBarDlg.cpp":                true,
		"TrafficMonitor/TaskBarDlg.h":                  true,
		"TrafficMonitor/TaskBarDlgDrawCommon.h":        true,
		"TrafficMonitor/TaskBarSettingsDlg.cpp":        true,
		"TrafficMonitor/TaskBarSettingsDlg.h":          true,
		"TrafficMonitor/TaskbarColorDlg.cpp":           true,
		"TrafficMonitor/TaskbarColorDlg.h":             true,
		"TrafficMonitor/TrafficMonitor.cpp":            true,
		"TrafficMonitor/TrafficMonitor.h":              true,
		"TrafficMonitor/TrafficMonitorDlg.cpp":         true,
		"TrafficMonitor/TrafficMonitorDlg.h":           true,
		"TrafficMonitor/Win11TaskbarSettingDlg.cpp":    true,
		"TrafficMonitor/Win11TaskbarSettingDlg.h":      true,
	}
	observedPartial := make(map[string]bool, len(expectedPartial))
	check := func(paths []string, encoding string) {
		t.Helper()
		fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Presentation: "compact", Paths: paths, Language: "cpp", Encoding: encoding, MaxFiles: 32,
		})
		if err != nil || fail != nil || out.Truncated || out.FilesConsidered != len(paths) ||
			out.FilesParsed != len(paths) || out.FilesSkipped != 0 || len(out.OverviewFiles) != len(paths) {
			t.Fatalf("original source batch incomplete: err=%v fail=%+v files=%d parsed=%d skipped=%d", err, fail, out.FilesConsidered, out.FilesParsed, out.FilesSkipped)
		}
		expected := make(map[string]bool, len(paths))
		for _, p := range paths {
			expected[p] = true
		}
		for _, f := range out.OverviewFiles {
			if !expected[f.Path] || fp[f.Path] != "" || f.Status != "parsed" || f.Language != "cpp" || f.SourceFingerprint == "" {
				t.Errorf("excluded/duplicate/invalid original source: %+v", f)
			}
			relative := filepath.ToSlash(strings.TrimPrefix(f.Path, project+string(filepath.Separator)))
			if f.CoverageComplete == expectedPartial[relative] {
				t.Errorf("original TrafficMonitor source coverage drift: %s complete=%v", relative, f.CoverageComplete)
			}
			if !f.CoverageComplete {
				observedPartial[relative] = true
			}
			fp[f.Path] = f.SourceFingerprint
			delete(expected, f.Path)
		}
		for p := range expected {
			t.Errorf("original source omitted: %s", p)
		}
		batchComplete := true
		for _, p := range paths {
			if expectedPartial[filepath.ToSlash(strings.TrimPrefix(p, project+string(filepath.Separator)))] {
				batchComplete = false
			}
		}
		if out.CoverageComplete != batchComplete {
			t.Errorf("original source batch coverage changed")
		}
	}
	for start := 0; start < len(regular); start += 32 {
		check(regular[start:min(start+32, len(regular))], "")
	}
	check([]string{special}, "gb18030")
	if len(fp) != len(all) {
		t.Errorf("only %d/222 source fingerprints", len(fp))
	}
	if len(observedPartial) != len(expectedPartial) {
		t.Errorf("original TrafficMonitor partial source inventory %d, expected %d", len(observedPartial), len(expectedPartial))
	}
	for p := range expectedPartial {
		if !observedPartial[p] {
			t.Errorf("original TrafficMonitor incomplete source missing: %s", p)
		}
	}
	t.Logf("222 pinned original C++ files in eight bounded batches: %d structurally complete, %d explicit partial; one separately GB18030", len(all)-len(observedPartial), len(observedPartial))
	// Original full-tree grep: TIMERCALLBACK has one authored typedef at line 6,
	// plus two uses (field and parameter) in the same source header.
	header := filepath.Join(project, "TrafficMonitor", "HighResolutionTimer.h")
	found := false
	for start := 0; start < len(regular); start += 32 {
		paths := regular[start:min(start+32, len(regular))]
		fail, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Mode: "structural", Match: "exact", Query: "TIMERCALLBACK",
			Paths: paths, Language: "cpp", MaxFiles: 32, MaxResults: 15,
		})
		if err != nil || fail != nil || out.Search == nil || out.Coverage.FilesParsed != len(paths) {
			t.Fatalf("source search not complete in batch %d: err=%v fail=%+v", start, err, fail)
		}
		for _, m := range out.Search.Matches {
			if found || m.Path != header || m.QualifiedName != "CHighResolutionTimer.TIMERCALLBACK" ||
				string(m.Kind) != "alias" || m.Range.Start.Line != 6 || m.SymbolID == "" ||
				m.SourceFingerprint != fp[header] {
				t.Errorf("invented, duplicated or mislocated alias: %+v", m)
			}
			found = true
		}
	}
	// The separately decoded GB18030 header is also part of the search scope.
	// Original text contains no TIMERCALLBACK occurrence in that header.
	fail, tail, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Mode: "structural", Match: "exact", Query: "TIMERCALLBACK",
		Paths: []string{special}, Language: "cpp", Encoding: "gb18030", MaxFiles: 1, MaxResults: 15,
	})
	if err != nil || fail != nil || tail.Search == nil ||
		tail.Coverage.FilesParsed != 1 || tail.Coverage.FilesSkipped != 0 ||
		len(tail.Search.Matches) != 0 {
		t.Errorf("original GB18030 source search must be complete and contain no TIMERCALLBACK declaration: err=%v fail=%+v", err, fail)
	}
	if !found {
		t.Error("original TIMERCALLBACK function-pointer typedef was omitted")
	}
}

func TestSourceSymbolsOriginalCppTrafficMonitorTimerHeaderOracle(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus not configured")
	}
	file := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-zhongyang219-trafficmonitor", "TrafficMonitor", "HighResolutionTimer.h")
	h := NewHandler([]string{root})
	fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{file},
		Language: "cpp", MaxFiles: 1, MaxSymbols: 24,
	})
	if err != nil || fail != nil || out.Truncated || !out.CoverageComplete || len(out.Files) != 1 ||
		out.Files[0].SourceFingerprint == "" {
		t.Fatalf("original header oracle unavailable: err=%v fail=%+v", err, fail)
	}
	type expected struct {
		kind, parent string
		line         int
	}
	want := map[string]expected{
		"CHighResolutionTimer":                      {"class", "", 4},
		"CHighResolutionTimer.TIMERCALLBACK":        {"alias", "CHighResolutionTimer", 6},
		"CHighResolutionTimer.m_dwUser":             {"field", "CHighResolutionTimer", 9},
		"CHighResolutionTimer.m_nIDTimer":           {"field", "CHighResolutionTimer", 10},
		"CHighResolutionTimer.m_uDelay":             {"field", "CHighResolutionTimer", 11},
		"CHighResolutionTimer.m_pfnCallback":        {"field", "CHighResolutionTimer", 12},
		"CHighResolutionTimer.KillTimer":            {"method", "CHighResolutionTimer", 15},
		"CHighResolutionTimer.CHighResolutionTimer": {"constructor", "CHighResolutionTimer", 24},
		"CHighResolutionTimer.TimeProc":             {"method", "CHighResolutionTimer", 36},
		"CHighResolutionTimer.CreateTimer":          {"method", "CHighResolutionTimer", 44},
	}
	if len(out.Symbols) != 11 {
		t.Errorf("original header contains 11 declarations, got %d", len(out.Symbols))
	}
	foundDestructor := false
	for _, s := range out.Symbols {
		if string(s.Kind) == "destructor" && s.QualifiedName == "CHighResolutionTimer.CHighResolutionTimer" && s.NameRange.Start.Line == 31 {
			foundDestructor = true
			continue
		}
		w, ok := want[s.QualifiedName]
		if !ok || string(s.Kind) != w.kind || s.NameRange.Start.Line != w.line ||
			s.ParentQualifiedName != w.parent || s.ID == "" || (w.parent != "" && s.ParentID == "") {
			t.Errorf("wrong/missing original declaration: %+v expected=%+v exists=%v", s, w, ok)
		}
		delete(want, s.QualifiedName)
	}
	for name := range want {
		t.Errorf("missing authored C++ declaration %s", name)
	}
	if !foundDestructor {
		t.Error("missing authored destructor line 31")
	}
	// The method body, unlike this declaration-only typedef, can be
	// retrieved using a source fingerprint without guessing source locations.
	foundMethodBody := false
	for _, s := range out.Symbols {
		if s.Kind == "method" && s.Name == "KillTimer" {
			foundMethodBody = true
			fail, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: file, SymbolID: s.ID, SourceFingerprint: out.Files[0].SourceFingerprint,
				Language: "cpp", Encoding: "utf-8", MaxBytes: 1200,
			})
			if err != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "timeKillEvent") {
				t.Errorf("method body source binding failed: err=%v fail=%+v", err, fail)
			}
		}
	}
	if !foundMethodBody {
		t.Error("original authored KillTimer method missing")
	}
}
