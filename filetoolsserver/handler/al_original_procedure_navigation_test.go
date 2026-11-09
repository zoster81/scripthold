package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Opt-in original AL source files are byte-identical with the two independent
// source repositories. Their tests, neighboring objects and unrelated scopes
// are independently inspected; none of that third-party code is executed.
func TestALOriginalProcedureBodyNavigation(t *testing.T) {
	cases := []struct {
		name, env, query, qualified, bodyStart, bodyEnd string
		line                                            int
	}{
		{
			name:      "powersearch-query-filtering",
			env:       "SCRIPTHOLD_REAL_AL_POWERSEARCH",
			query:     "Search",
			qualified: "PowerSearch.BusinessCentral.PowerSearch Engine.Search",
			bodyStart: "StrLen(Query) < 2",
			bodyEnd:   "Telemetry.LogSearch",
			line:      115,
		},
		{
			name:      "sticky-notes-visibility",
			env:       "SCRIPTHOLD_REAL_AL_STICKY",
			query:     "GetActiveNotesJson",
			qualified: "DefaultPublisher.StickyNoteNotes.SN Note Manager.GetActiveNotesJson",
			bodyStart: "NowDT := CurrentDateTime()",
			bodyEnd:   "NoteArray.WriteTo(ResultText)",
			line:      25,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			root := os.Getenv("SCRIPTHOLD_REAL_AL_ROOT")
			if path == "" || root == "" {
				t.Skip("original source fixture not configured")
			}
			h := NewHandler([]string{root})
			r, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "al",
				Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 32, FileDetails: "relevant",
			})
			if err != nil || r != nil || found.FilesParsed != 1 || found.Truncated || len(found.Files) != 1 ||
				!found.CoverageComplete || !found.Files[0].CoverageComplete {
				t.Fatalf("AL find incomplete: tool=%v err=%v output=%+v", r, err, found)
			}
			file := found.Files[0]
			var selected string
			for _, sym := range found.Symbols {
				if sym.QualifiedName == tc.qualified && sym.NameRange.Start.Line == tc.line &&
					sym.BodyRange != nil && sym.DeclarationRange.End.Line > tc.line {
					selected = sym.ID
				}
			}
			if selected == "" {
				t.Fatalf("missing original owned AL method %s (%d candidates)", tc.qualified, len(found.Symbols))
			}
			if file.SourceFingerprint == "" || file.Encoding == "" {
				t.Fatal("AL original method lacks fingerprint/encoding selector")
			}
			sr, shown, se := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "al", Encoding: file.Encoding,
				SymbolID: selected, SourceFingerprint: file.SourceFingerprint, MaxBytes: 32768,
			})
			if se != nil || sr != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.bodyStart) ||
				!strings.Contains(shown.Show.Text, tc.bodyEnd) {
				t.Fatalf("fingerprint-bound show missing AL body: tool=%v err=%v show=%+v", sr, se, shown.Show)
			}
			qr, queried, qe := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Query: tc.query, Match: "exact", Language: "al", MaxFiles: 1, MaxResults: 24,
			})
			if qe != nil || (qr != nil && qr.IsError) || queried.Search == nil ||
				queried.Coverage.FilesParsed != 1 || queried.Coverage.ResultsTruncated {
				t.Fatalf("AL structural query incomplete: tool=%v err=%v coverage=%+v", qr, qe, queried.Coverage)
			}
			matched := false
			for _, match := range queried.Search.Matches {
				if match.QualifiedName == tc.qualified && match.SymbolID == selected &&
					match.SourceFingerprint == file.SourceFingerprint {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("AL search omitted exact method %s", tc.qualified)
			}
		})
	}
}
