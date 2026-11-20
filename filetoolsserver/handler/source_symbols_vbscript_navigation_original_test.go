package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The maintenance questions and locations were determined from the original
// VBScript projects by independent source reads, not Source Intelligence output.
// Source Intelligence reports declarations; grep/read verifies statements and calls.
func TestSourceSymbolsOriginalVBScriptTaskNavigation(t *testing.T) {
	type expected struct {
		rel, name, qualified string
		line                 int
		excerpt              string
	}
	find := func(t *testing.T, h *Handler, root string, e expected) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(e.rel))
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{path}, Language: "vbscript", Encoding: "utf-8",
			Query: e.name, Match: "exact", MaxFiles: 1, MaxSymbols: 8,
		})
		if err != nil || failed != nil || !out.CoverageComplete || out.Truncated || len(out.Symbols) != 1 {
			t.Fatalf("missing/ambiguous %s: err=%v tool=%+v out=%+v", e.name, err, failed, out)
		}
		sym := out.Symbols[0]
		if sym.Name != e.name || sym.QualifiedName != e.qualified ||
			sym.DeclarationRange.Start.Line != e.line || sym.NameRange.Start.Line != e.line {
			t.Fatalf("wrong source for %s: %+v", e.name, sym)
		}
		if e.excerpt == "" {
			return
		}
		if len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
			t.Fatalf("no fingerprint for %s", e.name)
		}
		failed, show, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "show", Path: path, SymbolID: sym.ID,
			SourceFingerprint: out.Files[0].SourceFingerprint,
			Language:          "vbscript", Encoding: "utf-8", MaxBytes: 8192,
		})
		if err != nil || failed != nil || show.Show == nil ||
			!strings.Contains(show.Show.Text, e.excerpt) {
			t.Fatalf("fingerprinted source for %s missing: err=%v tool=%+v shown=%+v", e.name, err, failed, show.Show)
		}
	}
	digest := func(t *testing.T, h *Handler, root string, rels ...string) []string {
		t.Helper()
		paths := make([]string, 0, len(rels))
		for _, rel := range rels {
			path := filepath.Join(root, filepath.FromSlash(rel))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("original source missing or not regular %s: %v", rel, err)
			}
			paths = append(paths, path)
		}
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Paths: paths, Language: "vbscript", Encoding: "utf-8",
			Presentation: "compact", MaxFiles: len(paths),
		})
		if err != nil || failed != nil || !out.CoverageComplete || out.Truncated || out.FilesParsed != len(paths) {
			t.Fatalf("project digest failed: err=%v tool=%+v out=%+v", err, failed, out)
		}
		data, err := json.Marshal(out)
		if err != nil || len(data) > 8192 {
			t.Fatalf("project digest exceeds 8 KiB: %d bytes err=%v", len(data), err)
		}
		t.Logf("original scope: files=%d symbols=%d compactBytes=%d coverageComplete=%v", out.FilesParsed, out.SymbolCount, len(data), out.CoverageComplete)
		return paths
	}
	t.Run("avg-update-and-quick-full-scan-command", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_VBS_NAV_NABLE_ROOT")
		if root == "" {
			t.Skip("original N-able project not configured")
		}
		h := NewHandler([]string{root})
		full := "AVG - Quick or Full Scan/AVG8_Full_Scan.vbs"
		quick := "AVG - Quick or Full Scan/AVG8_Quick_Scan.vbs"
		other := "AV Status/AVStatus.vbs"
		paths := digest(t, h, root, full, quick, other)
		for _, e := range []expected{
			{full, "scancmd", "scancmd", 18, "scancmd"},
			{quick, "scancmd", "scancmd", 21, "scancmd"},
			{full, "updatecmd", "updatecmd", 18, "updatecmd"},
			{quick, "Executable", "Executable", 21, "Executable"},
			{other, "NoAVBehavior", "NoAVBehavior", 41, "NoAVBehavior"},
			{other, "OSType", "OSType", 394, "SELECT AddressWidth FROM Win32_Processor"},
		} {
			find(t, h, root, e)
		}
		for i, path := range paths[:2] {
			contents, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(contents), " /PRIORITY HIGH/") ||
				!strings.Contains(string(contents), "avgupd.exe") {
				t.Fatalf("original AVG priority/update oracle changed: file=%d err=%v", i, err)
			}
			switch i {
			case 0:
				if !strings.Contains(string(contents), " /COMP /CLEAN ") {
					t.Fatal("original full scan option missing")
				}
			case 1:
				if !strings.Contains(string(contents), " /QT /CLEAN ") {
					t.Fatal("original quick scan option missing")
				}
			}
		}
		failed, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Language: "vbscript", Encoding: "utf-8",
			Query: "scancmd", Match: "exact", Mode: "structural", MaxResults: 8,
		})
		if err != nil || failed != nil || out.Search == nil || len(out.Search.Matches) != 2 {
			t.Fatalf("quick/full scan declarations not both found: err=%v tool=%+v out=%+v", err, failed, out)
		}
	})
	t.Run("comadmin-remote-catalog-vs-directory-query", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_VBS_NAV_WINDOWS_ROOT")
		if root == "" {
			t.Skip("original Windows sample project not configured")
		}
		h := NewHandler([]string{root})
		main := "Samples/Win7Samples/com/administration/scripted administration/comadmin.vbs"
		unrelated := "Samples/Win7Samples/netds/adsi/general/ADOQuery/adoquery.vbs"
		paths := digest(t, h, root, main, unrelated)
		for _, e := range []expected{
			{main, "strServerName", "strServerName", 48, "Dim strServerName"},
			{main, "catalog", "catalog", 493, "Dim catalog"},
			{main, "applications", "applications", 497, "Dim applications"},
			{main, "root", "root", 499, "Dim root"},
			{main, "roleName", "roleName", 51, "Dim roleName"},
			{unrelated, "ADS_SCOPE_SUBTREE", "ADS_SCOPE_SUBTREE", 15, "ADS_SCOPE_SUBTREE"},
		} {
			find(t, h, root, e)
		}
		data, err := os.ReadFile(paths[0])
		if err != nil ||
			!strings.Contains(string(data), "Set root = catalog.Connect(strServerName)") ||
			!strings.Contains(string(data), "Set applications = root.GetCollection(\"Applications\", \"\")") ||
			!strings.Contains(string(data), "Set applications = catalog.GetCollection(\"Applications\")") ||
			!strings.Contains(string(data), "strServerName = \"localhost\"") {
			t.Fatal("remote versus local COM+ catalog routing oracle changed")
		}
		data, err = os.ReadFile(paths[1])
		if err != nil || !strings.Contains(string(data), "adDomainPath = \"LDAP://") {
			t.Fatal("unrelated ADO LDAP query oracle changed")
		}
		failed, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Language: "vbscript", Encoding: "utf-8",
			Query: "strServerName", Match: "exact", Mode: "structural", MaxResults: 8,
		})
		if err != nil || failed != nil || out.Search == nil || len(out.Search.Matches) != 1 ||
			out.Search.Matches[0].QualifiedName != "strServerName" {
			t.Fatalf("COMAdmin server variable not isolated: err=%v tool=%+v out=%+v", err, failed, out)
		}
	})
}
