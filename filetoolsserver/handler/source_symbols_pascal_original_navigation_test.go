package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tasks and primary/adjacent/alternative locations were established from
// independent reads of the original Pascal projects before source_* queries.
// These tests exercise the local Go handlers; original projects are never run.
func TestSourceSymbolsOriginalPascalTaskNavigation(t *testing.T) {
	type want struct {
		rel     string
		name    string
		line    int
		excerpt string
	}
	digest := func(t *testing.T, h *Handler, root string, rels ...string) []string {
		t.Helper()
		paths := make([]string, 0, len(rels))
		for _, rel := range rels {
			path := filepath.Join(root, filepath.FromSlash(rel))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("missing original source %s: %v", rel, err)
			}
			paths = append(paths, path)
		}
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Paths: paths, Language: "pascal", Encoding: "utf-8",
			Presentation: "compact", MaxFiles: len(paths),
		})
		if err != nil || failed != nil || !out.CoverageComplete || out.Truncated || out.FilesParsed != len(paths) {
			t.Fatalf("bounded original-project digest failed: err=%v tool=%+v out=%+v", err, failed, out)
		}
		bytes, err := json.Marshal(out)
		if err != nil || len(bytes) > 8192 {
			t.Fatalf("compact digest exceeds budget %d bytes err=%v", len(bytes), err)
		}
		t.Logf("original scope: files=%d symbols=%d compactBytes=%d complete=%v", out.FilesParsed, out.SymbolCount, len(bytes), out.CoverageComplete)
		return paths
	}
	find := func(t *testing.T, h *Handler, root string, w want) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(w.rel))
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{path}, Language: "pascal", Encoding: "utf-8",
			Query: w.name, Match: "exact", MaxFiles: 1, MaxSymbols: 16,
		})
		if err != nil || failed != nil || !out.CoverageComplete || out.Truncated {
			t.Fatalf("find %s failed: err=%v tool=%+v", w.name, err, failed)
		}
		var chosen string
		for _, s := range out.Symbols {
			if s.Name == w.name && s.DeclarationRange.Start.Line == w.line {
				chosen = s.ID
				break
			}
		}
		if chosen == "" {
			t.Fatalf("source name %s line=%d missing; matches=%+v", w.name, w.line, out.Symbols)
		}
		if w.excerpt == "" {
			return
		}
		if len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
			t.Fatalf("source fingerprint missing %s", w.name)
		}
		failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "show", Path: path, SymbolID: chosen, SourceFingerprint: out.Files[0].SourceFingerprint,
			Language: "pascal", Encoding: "utf-8", MaxBytes: 8192,
		})
		if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, w.excerpt) {
			t.Fatalf("fingerprint show for %s line %d failed err=%v tool=%+v show=%+v", w.name, w.line, err, failed, shown.Show)
		}
	}
	t.Run("brook-HTTP-method-mismatch-and-router-actions", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_PASCAL_NAV_BROOK_ROOT")
		if root == "" {
			t.Skip("original Brook project not configured")
		}
		h := NewHandler([]string{root})
		paths := digest(t, h, root, "core/brookrouter.pas", "core/brookaction.pas", "core/brookhttputils.pas")
		for _, w := range []want{
			{"core/brookrouter.pas", "Route", 177, ""},
			{"core/brookrouter.pas", "Route", 706, "procedure TBrookRouter.Route"},
			{"core/brookrouter.pas", "MatchPattern", 540, "function TBrookRouter.MatchPattern"},
			{"core/brookrouter.pas", "MethodNotAllowed", 151, "class procedure MethodNotAllowed"},
			{"core/brookrouter.pas", "MethodNotAllowed", 472, "class procedure TBrookRouter.MethodNotAllowed"},
			{"core/brookrouter.pas", "ExecuteAction", 410, "procedure TBrookRouter.ExecuteAction"},
			{"core/brookaction.pas", "Register", 415, "class procedure TBrookAction.Register"},
		} {
			find(t, h, root, w)
		}
		router, err := os.ReadFile(paths[0])
		if err != nil || !strings.Contains(string(router), "BrookMatchMethod(PRoute^.Method, ARequest.Method)") ||
			!strings.Contains(string(router), "TBrookRouter.MethodNotAllowed(AResponse);") ||
			!strings.Contains(string(router), "ExecuteAction(VAct, ARequest, AResponse, VNames, VValues, PRoute^)") {
			t.Fatalf("original HTTP route decision oracle changed: %v", err)
		}
		action, err := os.ReadFile(paths[1])
		if err != nil || !strings.Contains(string(action), "class procedure TBrookAction.Register") {
			t.Fatalf("original route registration neighbor changed: %v", err)
		}
		failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Language: "pascal", Encoding: "utf-8",
			Query: "MethodNotAllowed", Match: "exact", Mode: "structural", MaxResults: 10,
		})
		if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) != 2 {
			t.Fatalf("class procedure declaration/definition missing: err=%v tool=%+v search=%+v", err, failed, search.Search)
		}
	})
	t.Run("drl-music-default-and-audio-consumer-vs-legacy-config", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_PASCAL_NAV_DRL_ROOT")
		if root == "" {
			t.Skip("original DRL project not configured")
		}
		h := NewHandler([]string{root})
		paths := digest(t, h, root, "src/drlconfiguration.pas", "src/drlaudio.pas", "src/drlconfig.pas", "src/drl.pas")
		for _, w := range []want{
			{"src/drlconfiguration.pas", "TDRLConfiguration", 11, ""},
			{"src/drlconfiguration.pas", "TDRLConfiguration", 21, "constructor TDRLConfiguration.Create"},
			{"src/drlconfiguration.pas", "Configuration", 15, ""},
			{"src/drlaudio.pas", "Reconfigure", 60, ""},
			{"src/drlaudio.pas", "Reconfigure", 131, "procedure TDRLAudio.Reconfigure"},
			{"src/drlaudio.pas", "Configure", 148, "procedure TDRLAudio.Configure"},
			{"src/drlconfig.pas", "TDRLConfig", 28, "constructor TDRLConfig.Create"},
		} {
			find(t, h, root, w)
		}
		defaults, err := os.ReadFile(paths[0])
		if err != nil || !strings.Contains(string(defaults), "AddInteger( 'volume_music', 30 )") ||
			!strings.Contains(string(defaults), "AddInteger( 'volume_sound', 70 )") {
			t.Fatalf("original registered default audio values changed: %v", err)
		}
		consumer, err := os.ReadFile(paths[1])
		if err != nil || !strings.Contains(string(consumer), "Configuration.GetInteger('volume_music')") ||
			!strings.Contains(string(consumer), "FAudio.SetMusicVolumePercent(Setting_MusicVolume)") {
			t.Fatalf("original audio consumer changed: %v", err)
		}
		alt, err := os.ReadFile(paths[2])
		if err != nil || !strings.Contains(string(alt), "Configure('GameMusic',Option_Music)") {
			t.Fatalf("separate legacy Lua audio toggle oracle changed: %v", err)
		}
		startup, err := os.ReadFile(paths[3])
		if err != nil || !strings.Contains(string(startup), "Configuration := TDRLConfiguration.Create") {
			t.Fatalf("main program config constructor oracle changed: %v", err)
		}
		failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Language: "pascal", Encoding: "utf-8",
			Query: "Reconfigure", Match: "exact", Mode: "structural", MaxResults: 10,
		})
		if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) < 2 {
			t.Fatalf("audio config routine navigation failed: err=%v tool=%+v search=%+v", err, failed, search.Search)
		}
	})
}
