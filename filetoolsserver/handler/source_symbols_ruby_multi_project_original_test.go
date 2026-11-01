package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rubyOriginalDeclaration struct {
	qualified  string
	kind       string
	line       int
	visibility string
	parent     string
}

// The test only reads the original pinned project files; expected declarations
// come from complete independent source reads, not from analyzer output.
func TestSourceSymbolsRealRubyFiveAdditionalProjects(t *testing.T) {
	type scenario struct {
		env     string
		path    string
		want    []rubyOriginalDeclaration
		body    string
		snippet string
		missing []string
	}
	scenarios := []scenario{
		{"SCRIPTHOLD_RUBY_ROTP_PROJECT", "lib/rotp/hotp.rb", []rubyOriginalDeclaration{
			{"ROTP", "module", 1, "public", ""},
			{"ROTP.HOTP", "class", 2, "public", "ROTP"},
			{"ROTP.HOTP.at", "method", 6, "public", "ROTP.HOTP"},
			{"ROTP.HOTP.verify", "method", 14, "public", "ROTP.HOTP"},
			{"ROTP.HOTP.provisioning_uri", "method", 27, "public", "ROTP.HOTP"},
		}, "ROTP.HOTP.at", "generate_otp(count)", nil},
		{"SCRIPTHOLD_RUBY_SCIENTIST_PROJECT", "lib/scientist/observation.rb", []rubyOriginalDeclaration{
			{"Scientist::Observation", "class", 2, "public", ""},
			{"Scientist::Observation.RESCUES", "constant", 6, "public", "Scientist::Observation"},
			{"Scientist::Observation.Observation", "constructor", 26, "public", "Scientist::Observation"},
			{"Scientist::Observation.cleaned_value", "method", 55, "public", "Scientist::Observation"},
			{"Scientist::Observation.equivalent_to?", "method", 79, "public", "Scientist::Observation"},
			{"Scientist::Observation.hash", "method", 98, "public", "Scientist::Observation"},
			{"Scientist::Observation.raised?", "method", 102, "public", "Scientist::Observation"},
			{"Scientist::Observation.capture_times", "method", 108, "private", "Scientist::Observation"},
		}, "Scientist::Observation.raised?", "!exception.nil?", []string{"Scientist::Observation.experiment", "Scientist::Observation.duration"}},
		{"SCRIPTHOLD_RUBY_TIMECOP_PROJECT", "lib/timecop/time_stack_item.rb", []rubyOriginalDeclaration{
			{"Timecop", "class", 1, "public", ""},
			{"Timecop.TimeStackItem", "class", 4, "public", "Timecop"},
			{"Timecop.TimeStackItem.TimeStackItem", "constructor", 7, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.year", "method", 18, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.month", "method", 22, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.day", "method", 26, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.hour", "method", 30, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.min", "method", 34, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.sec", "method", 38, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.utc_offset", "method", 42, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.travel_offset", "method", 46, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.travel_offset_days", "method", 50, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.scaling_factor", "method", 54, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.monotonic", "method", 58, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.current_monotonic", "method", 68, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.current_monotonic_with_mock", "method", 72, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.time", "method", 76, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.scaled_time", "method", 92, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.date", "method", 96, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.datetime", "method", 100, "public", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.rational_to_utc_offset", "method", 111, "private", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.utc_offset_to_rational", "method", 115, "private", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.parse_monotonic_time", "method", 119, "private", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.parse_time", "method", 129, "private", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.compute_travel_offset", "method", 157, "private", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.times_are_equal_within_epsilon", "method", 161, "private", "Timecop.TimeStackItem"},
			{"Timecop.TimeStackItem.time_klass", "method", 165, "private", "Timecop.TimeStackItem"},
		}, "Timecop.TimeStackItem.year", "time.year", []string{"Timecop.TimeStackItem.mock_type"}},
		{"SCRIPTHOLD_RUBY_LIQUID_PROJECT", "lib/liquid/interrupts.rb", []rubyOriginalDeclaration{
			{"Liquid", "module", 3, "public", ""},
			{"Liquid.Interrupt", "class", 5, "public", "Liquid"},
			{"Liquid.Interrupt.Interrupt", "constructor", 8, "public", "Liquid.Interrupt"},
			{"Liquid.BreakInterrupt", "class", 14, "public", "Liquid"},
			{"Liquid.ContinueInterrupt", "class", 17, "public", "Liquid"},
		}, "Liquid.Interrupt.Interrupt", "@message = message", []string{"Liquid.Interrupt.message"}},
		{"SCRIPTHOLD_RUBY_CANCANCAN_PROJECT", "lib/cancan/class_matcher.rb", []rubyOriginalDeclaration{
			{"SubjectClassMatcher", "class", 6, "public", ""},
			{"SubjectClassMatcher.matches_subject_class?", "method", 7, "public", "SubjectClassMatcher"},
			{"SubjectClassMatcher.matching_class_check", "method", 14, "public", "SubjectClassMatcher"},
			{"SubjectClassMatcher.matches_class_or_is_related", "method", 25, "public", "SubjectClassMatcher"},
		}, "SubjectClassMatcher.matches_class_or_is_related", "sub.is_a?(Module)", nil},
		{"SCRIPTHOLD_RUBY_CANCANCAN_PROJECT", "lib/cancan/relevant.rb", []rubyOriginalDeclaration{
			{"CanCan", "module", 3, "public", ""},
			{"CanCan.Relevant", "module", 4, "public", "CanCan"},
			{"CanCan.Relevant.relevant?", "method", 6, "public", "CanCan.Relevant"},
			{"CanCan.Relevant.matches_action?", "method", 13, "private", "CanCan.Relevant"},
			{"CanCan.Relevant.matches_subject?", "method", 17, "private", "CanCan.Relevant"},
			{"CanCan.Relevant.matches_subject_class?", "method", 21, "private", "CanCan.Relevant"},
		}, "CanCan.Relevant.matches_action?", "@expanded_actions.include?", nil},
	}
	for _, tc := range scenarios {
		tc := tc
		t.Run(tc.env+"/"+filepath.Base(tc.path), func(t *testing.T) {
			root := os.Getenv(tc.env)
			if root == "" {
				t.Skip("qualified real original project not set")
			}
			p := filepath.Join(root, filepath.FromSlash(tc.path))
			h := NewHandler([]string{root})
			ctx := context.Background()
			failed, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Paths: []string{p}, Language: "ruby", Encoding: "utf-8", Presentation: "detailed", MaxFiles: 1, MaxSymbols: 64})
			if err != nil || failed != nil || !out.CoverageComplete || out.Truncated || out.SymbolCount != len(tc.want) || len(out.Symbols) != len(tc.want) || len(out.Files) != 1 || !out.Files[0].CoverageComplete {
				t.Fatalf("original Ruby outline mismatch error=%v failed=%+v count=%d output=%+v", err, failed, out.SymbolCount, out)
			}
			fingerprint := out.Files[0].SourceFingerprint
			oracle := make(map[string]rubyOriginalDeclaration, len(tc.want))
			for _, w := range tc.want {
				oracle[w.qualified] = w
			}
			found := map[string]int{}
			ids := map[string]string{}
			for _, s := range out.Symbols {
				w, ok := oracle[s.QualifiedName]
				found[s.QualifiedName]++
				ids[s.QualifiedName] = s.ID
				if !ok || string(s.Kind) != w.kind || s.DeclarationRange.Start.Line != w.line || string(s.Visibility) != w.visibility || s.ParentQualifiedName != w.parent || s.Path != p {
					t.Errorf("Ruby source mismatch %+v expected=%+v exists=%v", s, w, ok)
				}
			}
			for _, w := range tc.want {
				if found[w.qualified] != 1 {
					t.Errorf("missing/duplicate %s count=%d", w.qualified, found[w.qualified])
				}
			}
			for _, s := range out.Symbols {
				if s.ParentQualifiedName != "" && ids[s.ParentQualifiedName] != s.ParentID {
					t.Errorf("incorrect parent ID %+v", s)
				}
			}
			failed, tree, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Paths: []string{p}, Language: "ruby", Encoding: "utf-8", Presentation: "tree", MaxFiles: 1, MaxSymbols: 64})
			if err != nil || failed != nil || !tree.CoverageComplete || tree.SymbolCount != len(tc.want) || len(tree.OutlineFiles) != 1 {
				t.Fatalf("Ruby tree mismatch error=%v failed=%+v", err, failed)
			}
			seen := map[string]int{}
			var visit func([]*SourceOutlineNode, string)
			visit = func(items []*SourceOutlineNode, parent string) {
				for _, n := range items {
					w, ok := oracle[n.QualifiedName]
					seen[n.QualifiedName]++
					if !ok || string(n.Kind) != w.kind || n.Line != w.line || w.parent != parent {
						t.Errorf("wrong Ruby tree node %+v parent=%s", n, parent)
					}
					visit(n.Children, n.QualifiedName)
				}
			}
			visit(tree.OutlineFiles[0].Symbols, "")
			for _, w := range tc.want {
				if seen[w.qualified] != 1 {
					t.Errorf("Ruby tree missing %s count=%d", w.qualified, seen[w.qualified])
				}
			}
			failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "digest", Paths: []string{p}, Language: "ruby", Encoding: "utf-8", Presentation: "compact", MaxFiles: 1})
			if err != nil || failed != nil || !digest.CoverageComplete || digest.SymbolCount != len(tc.want) || len(digest.OverviewFiles) != 1 || digest.OverviewFiles[0].SourceFingerprint != fingerprint {
				t.Fatalf("Ruby digest mismatch error=%v failed=%+v", err, failed)
			}
			total := 0
			for _, c := range digest.OverviewFiles[0].DeclarationCounts {
				total += c.Count
			}
			if total != len(tc.want) {
				t.Errorf("Ruby digest total %d expected %d", total, len(tc.want))
			}
			// Source search must find each independently expected declaration.
			failed, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Paths: []string{p}, Language: "ruby", Encoding: "utf-8", Query: tc.body, Match: "exact", Mode: "structural", MaxResults: 8})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) != 1 || search.Search.Matches[0].QualifiedName != tc.body {
				t.Fatalf("Ruby search mismatch error=%v failed=%+v result=%+v", err, failed, search)
			}
			for _, excluded := range tc.missing {
				failed, q, err := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Paths: []string{p}, Language: "ruby", Encoding: "utf-8", Query: excluded, Match: "exact", Mode: "structural", MaxResults: 8})
				if err != nil || failed != nil || q.Search == nil || len(q.Search.Matches) != 0 {
					t.Errorf("Ruby fabricated DSL field %s result=%+v error=%v", excluded, q, err)
				}
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: p, Language: "ruby", Encoding: "utf-8", SymbolID: ids[tc.body], SourceFingerprint: fingerprint, MaxBytes: 4096})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.snippet) {
				t.Fatalf("Ruby exact show missing body snippet %s: error=%v failed=%+v shown=%+v", tc.snippet, err, failed, shown.Show)
			}
			failed, ctxResult, err := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "context", Paths: []string{p}, Language: "ruby", Encoding: "utf-8", Targets: []SourceSelectorInput{{Kind: "symbol", Path: p, SymbolID: ids[tc.body], SourceFingerprint: fingerprint}}, BodyPolicy: "prefer", BudgetBytes: 4096, MaxDepth: 2, MaxItems: 4})
			if err != nil || failed != nil || ctxResult.Context == nil || len(ctxResult.Context.Items) == 0 || string(ctxResult.Context.Items[0].Representation) != "body" || !strings.Contains(ctxResult.Context.Items[0].Text, tc.snippet) {
				t.Fatalf("Ruby context body mismatch error=%v failed=%+v output=%+v", err, failed, ctxResult.Context)
			}
		})
	}
}
