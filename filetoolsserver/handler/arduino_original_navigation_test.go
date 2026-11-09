package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The opt-in sketches are byte-identical to files tracked by independent
// original projects; related examples and library methods are checked directly
// in those sources, not guessed from call-site names.
func TestArduinoOriginalSketchTaskNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, marker, later, sibling, entity, entityKind, absent string
		targetLine, endLine, siblingLine, entityLine                          int
	}{
		{
			name:   "OneWire temperature reading and CRC handling",
			env:    "SCRIPTHOLD_REAL_ARDUINO_ONEWIRE",
			target: "loop", targetLine: 16, endLine: 112,
			marker: "OneWire::crc8(addr, 7)", later: "fahrenheit = celsius * 1.8",
			sibling: "setup", siblingLine: 12,
			entity: "ds", entityKind: "variable", entityLine: 10,
			absent: "crc8",
		},
		{
			name:   "Arduino EEPROM custom object read",
			env:    "SCRIPTHOLD_REAL_ARDUINO_CORE",
			target: "secondTest", targetLine: 54, endLine: 64,
			marker: "EEPROM.get(eeAddress, customVar)", later: "Serial.println(customVar.name)",
			sibling: "setup", siblingLine: 20,
			entity: "MyObject", entityKind: "struct", entityLine: 48,
			absent: "EEPROM.get",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_ARDUINO_ROOT"), os.Getenv(tc.env)
			if root == "" || path == "" {
				t.Skip("original Arduino source fixture not configured")
			}
			h, ctx := NewHandler([]string{root}), context.Background()
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "arduino", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Arduino digest unavailable: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "arduino",
				MaxFiles: 1, MaxSymbols: 30, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("Arduino outline incomplete: err=%v result=%+v", err, outline)
			}
			outlined := false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.targetLine {
					outlined = true
				}
			}
			if !outlined {
				t.Fatalf("Arduino outline omitted target %q", tc.target)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "arduino",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 30, FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("Arduino find incomplete: err=%v found=%+v", err, found)
			}
			selected := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && string(sym.Kind) == "function" &&
					sym.NameRange.Start.Line == tc.targetLine &&
					sym.DeclarationRange.End.Line == tc.endLine && sym.BodyRange != nil {
					selected = sym.ID
				}
			}
			if selected == "" {
				t.Fatalf("Arduino task function missing complete body: %+v", found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatal("Arduino missing safe file selector")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "arduino", Encoding: file.Encoding,
				SymbolID: selected, SourceFingerprint: file.SourceFingerprint, MaxBytes: 16384,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.marker) ||
				!strings.Contains(shown.Show.Text, tc.later) {
				t.Fatalf("Arduino exact body excerpt unavailable: err=%v show=%+v", err, shown.Show)
			}
			_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "arduino", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 30,
			})
			if err != nil || searched.Search == nil || !searched.Coverage.CoverageComplete ||
				searched.Coverage.ResultsTruncated {
				t.Fatalf("Arduino search incomplete: err=%v result=%+v", err, searched)
			}
			matched := false
			for _, hit := range searched.Search.Matches {
				if hit.Name == tc.target && hit.SymbolID == selected &&
					hit.SourceFingerprint == file.SourceFingerprint {
					matched = true
				}
			}
			if !matched {
				t.Fatal("Arduino structural search omitted original target")
			}
			for _, member := range []struct {
				name, kind string
				line       int
			}{
				{tc.sibling, "function", tc.siblingLine},
				{tc.entity, tc.entityKind, tc.entityLine},
			} {
				_, sibling, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "arduino",
					Query: member.name, Match: "exact", MaxFiles: 1, MaxSymbols: 30,
				})
				if err != nil || len(sibling.Symbols) != 1 ||
					sibling.Symbols[0].NameRange.Start.Line != member.line ||
					string(sibling.Symbols[0].Kind) != member.kind {
					t.Fatalf("Arduino related declaration %q invalid: err=%v found=%+v",
						member.name, err, sibling.Symbols)
				}
			}
			_, absent, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "arduino",
				Query: tc.absent, Match: "exact", MaxFiles: 1, MaxSymbols: 30,
			})
			if err != nil || !absent.CoverageComplete || len(absent.Symbols) != 0 {
				t.Fatalf("Arduino referenced external function %q invented as declaration: err=%v symbols=%+v", tc.absent, err, absent.Symbols)
			}
		})
	}
}
