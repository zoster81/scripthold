package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Expected declarations were independently read from the original PureBasic
// projects, before querying Source Intelligence. No project code is executed.
func TestSourceSymbolsOriginalPureBasicTaskNavigation(t *testing.T) {
	type declaration struct {
		file, name, qualified, native string
		line                          int
		excerpt                       string
	}
	check := func(t *testing.T, h *Handler, root string, d declaration) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(d.file))
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{path}, Language: "purebasic", Encoding: "utf-8",
			Query: d.name, Match: "exact", MaxFiles: 1, MaxSymbols: 16,
		})
		if err != nil || failed != nil || out.Truncated || out.FilesParsed != 1 {
			t.Fatalf("find %s error=%v tool=%+v out=%+v", d.name, err, failed, out)
		}
		var selected string
		for _, s := range out.Symbols {
			if s.Name == d.name && s.QualifiedName == d.qualified &&
				s.DeclarationRange.Start.Line == d.line && s.NameRange.Start.Line == d.line &&
				(d.native == "" || s.NativeKind == d.native) {
				selected = s.ID
				break
			}
		}
		if selected == "" {
			t.Fatalf("missing %s %s line=%d; candidates=%+v coverage=%v", d.name, d.qualified, d.line, out.Symbols, out.CoverageComplete)
		}
		if d.excerpt == "" {
			return
		}
		if len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
			t.Fatalf("missing fingerprint for %s", d.name)
		}
		failed, show, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "show", Path: path, SymbolID: selected, SourceFingerprint: out.Files[0].SourceFingerprint,
			Language: "purebasic", Encoding: "utf-8", MaxBytes: 8192,
		})
		if err != nil || failed != nil || show.Show == nil || !strings.Contains(show.Show.Text, d.excerpt) {
			t.Fatalf("fingerprinted source not available for %s: err=%v tool=%+v show=%+v", d.name, err, failed, show.Show)
		}
	}
	digest := func(t *testing.T, h *Handler, root string, rels ...string) []string {
		t.Helper()
		paths := make([]string, 0, len(rels))
		for _, rel := range rels {
			path := filepath.Join(root, filepath.FromSlash(rel))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("expected original source file %s: %v", rel, err)
			}
			paths = append(paths, path)
		}
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Paths: paths, Language: "purebasic", Encoding: "utf-8",
			Presentation: "compact", MaxFiles: len(paths),
		})
		if err != nil || failed != nil || out.FilesParsed != len(paths) || out.Truncated {
			t.Fatalf("original-project digest failure: err=%v tool=%+v out=%+v", err, failed, out)
		}
		data, err := json.Marshal(out)
		if err != nil || len(data) > 8192 {
			t.Fatalf("first-pass digest too large: %d bytes err=%v", len(data), err)
		}
		t.Logf("original scope: files=%d symbols=%d compactBytes=%d coverageComplete=%v", out.FilesParsed, out.SymbolCount, len(data), out.CoverageComplete)
		if !out.CoverageComplete {
			t.Logf("original-project digest reports partial coverage; not whole-file certified")
		}
		return paths
	}
	t.Run("websocket-frame-payload-default-and-receiver", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_PUREBASIC_NAV_WEBSOCKET_ROOT")
		if root == "" {
			t.Skip("original WebSocket project not configured")
		}
		h := NewHandler([]string{root})
		paths := digest(t, h, root, "Includes/WebSocket_Server.pbi", "Example_Chat_Server.pb", "Example_Chat_Server_Polling.pb")
		for _, d := range []declaration{
			{"Includes/WebSocket_Server.pbi", "Create", "WebSocket_Server.Create", "declare-procedure", 242, "Declare.i Create("},
			{"Includes/WebSocket_Server.pbi", "Create", "WebSocket_Server.Create", "procedure", 1361, "*Object\\Frame_Payload_Max = Frame_Payload_Max"},
			{"Includes/WebSocket_Server.pbi", "Thread_Receive_Frame", "WebSocket_Server.Thread_Receive_Frame", "procedure", 647, ""},
			{"Includes/WebSocket_Server.pbi", "Frame_Payload_Max", "WebSocket_Server.Object.Frame_Payload_Max", "field", 351, ""},
			{"Includes/WebSocket_Server.pbi", "Frame_Send", "WebSocket_Server.Frame_Send", "declare-procedure", 246, ""},
		} {
			check(t, h, root, d)
		}
		source, err := os.ReadFile(paths[0])
		if err != nil || !strings.Contains(string(source), "#Frame_Payload_Max = 10000000") ||
			!strings.Contains(string(source), "If *TempFrame\\Payload_Size > *Object\\Frame_Payload_Max") {
			t.Fatal("frame limit constant/enforcement oracle changed")
		}
		for _, example := range paths[1:] {
			contents, err := os.ReadFile(example)
			if err != nil || !strings.Contains(string(contents), "WebSocket_Server::Create(8090") {
				t.Fatalf("original example call missing: %s err=%v", example, err)
			}
		}
		failed, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Language: "purebasic", Encoding: "utf-8",
			Query: "WebSocket_Server.Create", Match: "exact", Mode: "structural", MaxResults: 8,
		})
		if err != nil || failed != nil || out.Search == nil || len(out.Search.Matches) != 2 {
			t.Fatalf("Create prototype+implementation not found: err=%v tool=%+v out=%+v", err, failed, out)
		}
	})
	t.Run("kenmo-json-string-escaping-and-input-variants", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_PUREBASIC_NAV_JSON_ROOT")
		if root == "" {
			t.Skip("original JSON library project not configured")
		}
		h := NewHandler([]string{root})
		paths := digest(t, h, root, "JSON.pbi", "JSON_Helper.pbi", "OJSON.pbi")
		for _, d := range []declaration{
			{"JSON.pbi", "xEscapeJSONString", "JSON.xEscapeJSONString", "procedure", 214, "Case '\"'"},
			{"JSON.pbi", "xParseJSONString", "JSON.xParseJSONString", "procedure", 320, "Case 'u'"},
			{"JSON.pbi", "xWriteJSONNode", "JSON.xWriteJSONNode", "procedure", 844, "xEscapeJSONString(*JN\\Key)"},
			{"JSON_Helper.pbi", "NormalizeJSON", "NormalizeJSON", "procedure", 560, ""},
			{"JSON_Helper.pbi", "ParseJSONEx", "ParseJSONEx", "procedure", 682, "NormalizeJSON(Input)"},
			{"JSON.pbi", "xParseJSONNumber", "JSON.xParseJSONNumber", "procedure", 446, ""},
			{"OJSON.pbi", "CreateOJSON", "CreateOJSON", "declare-procedure", 74, ""},
		} {
			check(t, h, root, d)
		}
		contents, err := os.ReadFile(paths[0])
		if err != nil || !strings.Contains(string(contents), "*JN\\Raw    =  xEscapeJSONString(Text)") {
			t.Fatal("original JSON escaping consumer changed")
		}
		failed, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Language: "purebasic", Encoding: "utf-8",
			Query: "JSON.xEscapeJSONString", Match: "exact", Mode: "structural", MaxResults: 8,
		})
		if err != nil || failed != nil || out.Search == nil || len(out.Search.Matches) != 1 {
			t.Fatalf("source query could not find original JSON escaping implementation: err=%v tool=%+v out=%+v", err, failed, out)
		}
	})
}
