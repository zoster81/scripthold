package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The edit questions, primary locations, direct consumers, tests and alternate
// features come from independent original-project reads, not from source_* output.
// Third-party projects are read-only and never executed by this test.
func TestSourceSymbolsOriginalDelphiTaskNavigation(t *testing.T) {
	type expected struct {
		path, name, kind string
		line             int
		excerpt          string
	}
	digest := func(t *testing.T, h *Handler, root string, names ...string) []string {
		t.Helper()
		paths := make([]string, 0, len(names))
		for _, name := range names {
			path := filepath.Join(root, filepath.FromSlash(name))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("original source unavailable %s: %v", name, err)
			}
			paths = append(paths, path)
		}
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Paths: paths, Language: "delphi", Encoding: "utf-8",
			Presentation: "compact", MaxFiles: len(paths),
		})
		if err != nil || failed != nil || !out.CoverageComplete || out.Truncated || out.FilesParsed != len(paths) {
			t.Fatalf("digest failed err=%v tool=%+v output=%+v", err, failed, out)
		}
		data, err := json.Marshal(out)
		if err != nil || len(data) > 8192 {
			t.Fatalf("compact digest exceeded 8KiB budget: %d bytes err=%v", len(data), err)
		}
		t.Logf("original files=%d symbols=%d compactBytes=%d reportedComplete=%v",
			out.FilesParsed, out.SymbolCount, len(data), out.CoverageComplete)
		return paths
	}
	find := func(t *testing.T, h *Handler, root string, e expected) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(e.path))
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{path}, Language: "delphi", Encoding: "utf-8",
			Query: e.name, Match: "exact", MaxFiles: 1, MaxSymbols: 20,
		})
		if err != nil || failed != nil || !out.CoverageComplete || out.Truncated {
			t.Fatalf("find %s failed err=%v tool=%+v result=%+v", e.name, err, failed, out)
		}
		selected := ""
		for _, symbol := range out.Symbols {
			if symbol.Name == e.name && string(symbol.Kind) == e.kind && symbol.DeclarationRange.Start.Line == e.line {
				selected = symbol.ID
				break
			}
		}
		if selected == "" {
			t.Fatalf("missing %s kind=%s line=%d: %+v", e.name, e.kind, e.line, out.Symbols)
		}
		if e.excerpt == "" {
			return
		}
		if len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
			t.Fatalf("missing fingerprint for %s", e.name)
		}
		failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "show", Path: path, SymbolID: selected,
			SourceFingerprint: out.Files[0].SourceFingerprint,
			Language:          "delphi", Encoding: "utf-8", MaxBytes: 8192,
		})
		if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, e.excerpt) {
			t.Fatalf("fingerprint-bound show for %s:%d err=%v tool=%+v show=%+v",
				e.name, e.line, err, failed, shown.Show)
		}
	}
	t.Run("theme-help-insight-preference-and-application", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_DELPHI_NAV_THEME_ROOT")
		if root == "" {
			t.Skip("original Delphi theme project not configured")
		}
		h := NewHandler([]string{root})
		settings := "delphi-ide-theme-editor/Units/DITE.Settings.pas"
		main := "delphi-ide-theme-editor/DITE.Main.pas"
		help := "delphi-ide-theme-editor/Units/DITE.HelpInsight.pas"
		paths := digest(t, h, root, settings, main, help)
		for _, e := range []expected{
			{settings, "TSettings", "class", 31, ""},
			{settings, "ApplyThemeHelpInsight", "property", 43, ""},
			{settings, "ReadSettings", "function", 74, ""},
			{settings, "ReadSettings", "function", 136, "procedure ReadSettings"},
			{settings, "WriteSettings", "function", 163, "procedure WriteSettings"},
			{settings, "BtnSaveClick", "method", 184, "procedure TFrmSettings.BtnSaveClick"},
			{settings, "LoadSettings", "method", 248, "procedure TFrmSettings.LoadSettings"},
			{main, "ApplyCurrentTheme", "method", 481, "procedure TFrmMain.ApplyCurrentTheme"},
			{help, "ApplyThemeHelpInsight", "function", 122, "procedure ApplyThemeHelpInsight"},
			{help, "SetHelpInsightDefault", "function", 85, "function SetHelpInsightDefault"},
		} {
			find(t, h, root, e)
		}
		data, err := os.ReadFile(paths[0])
		if err != nil || !strings.Contains(string(data), "LIniFile.ReadBool('Global', 'ApplyThemeHelpInsight', True)") ||
			!strings.Contains(string(data), "LIniFile.WriteBool('Global', 'ApplyThemeHelpInsight',") ||
			!strings.Contains(string(data), "FSettings.ApplyThemeHelpInsight := CheckBoxHelpInsight.Checked") {
			t.Fatalf("settings default/save source oracle changed: %v", err)
		}
		data, err = os.ReadFile(paths[1])
		if err != nil || !strings.Contains(string(data), "if Settings.ApplyThemeHelpInsight and") ||
			!strings.Contains(string(data), "ApplyThemeHelpInsight(FCurrentTheme, IDEData);") {
			t.Fatalf("main theme application source oracle changed: %v", err)
		}
		data, err = os.ReadFile(paths[2])
		if err != nil || !strings.Contains(string(data), "SetHelpInsightDefault") ||
			!strings.Contains(string(data), "HelpInsight_Template.css") {
			t.Fatalf("renderer versus reset feature oracle changed: %v", err)
		}
		failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Language: "delphi", Encoding: "utf-8",
			Query: "ReadSettings", Match: "exact", Mode: "structural", MaxResults: 12,
		})
		if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) != 2 {
			t.Fatalf("theme preferences declarations missing err=%v tool=%+v search=%+v", err, failed, search.Search)
		}
	})
	t.Run("worker-tree-validation-shutdown-and-regression-test", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_DELPHI_NAV_TREE_ROOT")
		if root == "" {
			t.Skip("original Delphi worker-thread project not configured")
		}
		h := NewHandler([]string{root})
		worker := "Source/VirtualTrees.WorkerThread.pas"
		tree := "Source/VirtualTrees.BaseTree.pas"
		test := "Tests/VTWorkerThreadIssue1001Tests.pas"
		paths := digest(t, h, root, worker, tree, test)
		for _, e := range []expected{
			{worker, "RemoveTree", "method", 32, "class procedure RemoveTree"},
			{worker, "RemoveTree", "method", 223, "class procedure TWorkerThread.RemoveTree"},
			{worker, "WaitForValidationTermination", "method", 20, ""},
			{worker, "WaitForValidationTermination", "method", 125, "procedure TWorkerThread.WaitForValidationTermination"},
			{worker, "Dispose", "method", 64, "class procedure TWorkerThread.Dispose"},
			{worker, "Execute", "method", 137, "procedure TWorkerThread.Execute"},
			{tree, "InterruptValidation", "method", 4088, "procedure TBaseVirtualTree.InterruptValidation"},
			{test, "TestDestroyWhileWorkerThreadBusy", "method", 33, ""},
			{test, "TestDestroyWhileWorkerThreadBusy", "method", 61, "procedure TVTWorkerThreadIssue1001Tests.TestDestroyWhileWorkerThreadBusy"},
		} {
			find(t, h, root, e)
		}
		data, err := os.ReadFile(paths[0])
		if err != nil || !strings.Contains(string(data), "WorkerThread.WaitForValidationTermination(pTree)") ||
			!strings.Contains(string(data), "FCurrentTree := nil; // Important") ||
			!strings.Contains(string(data), "CheckSynchronize();") {
			t.Fatalf("worker thread synchronization source oracle changed: %v", err)
		}
		data, err = os.ReadFile(paths[1])
		if err != nil || !strings.Contains(string(data), "TWorkerThread.RemoveTree(Self, pWaitForValidationTermination)") {
			t.Fatalf("tree consumer source oracle changed: %v", err)
		}
		data, err = os.ReadFile(paths[2])
		if err != nil || !strings.Contains(string(data), "RepeatTestAttribute(100)") ||
			!strings.Contains(string(data), "fTree.SetChildCount(fTree.RootNode, 10000)") {
			t.Fatalf("independent worker regression-test oracle changed: %v", err)
		}
		failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Language: "delphi", Encoding: "utf-8",
			Query: "RemoveTree", Match: "exact", Mode: "structural", MaxResults: 8,
		})
		if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) != 2 {
			t.Fatalf("worker class-method pair missing err=%v tool=%+v search=%+v", err, failed, search.Search)
		}
	})
}
