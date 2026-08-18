package sourceintelligence

import (
	"strings"
	"testing"
)

func TestDefaultLanguageRegistryCompletedProvidersAndFutureShapes(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"go", "csharp", "vbnet", "python", "classic-asp"} {
		descriptor, ok := registry.Lookup(id)
		if !ok {
			t.Fatalf("missing R25 language %q", id)
		}
		if !descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer == "" {
			t.Fatalf("R25 canary %q is not routed to an analyzer: %+v", id, descriptor)
		}
	}
	asp, _ := registry.Lookup("classic-asp")
	if !asp.Capabilities.Composite {
		t.Fatalf("classic-asp must be composite: %+v", asp)
	}
	vb, _ := registry.Lookup("vbnet")
	if !vb.Capabilities.CaseInsensitive {
		t.Fatalf("vbnet must declare case-insensitive identifiers: %+v", vb)
	}

	for _, id := range []string{"c", "cpp", "java", "kotlin"} {
		descriptor, ok := registry.Lookup(id)
		if !ok {
			t.Fatalf("missing R27 Phase 3 language %q", id)
		}
		if !descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer == "" {
			t.Fatalf("R27 Phase 3 provider %q is not routed to an analyzer: %+v", id, descriptor)
		}
	}

	for _, id := range []string{"javascript", "typescript", "rust"} {
		descriptor, ok := registry.Lookup(id)
		if !ok {
			t.Fatalf("missing R27 Phase 4 language %q", id)
		}
		if !descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer == "" {
			t.Fatalf("R27 Phase 4 provider %q is not routed to an analyzer: %+v", id, descriptor)
		}
	}

	for _, id := range []string{"php", "ruby", "swift", "pascal", "delphi"} {
		descriptor, ok := registry.Lookup(id)
		if !ok {
			t.Fatalf("missing R27 Phase 5 language %q", id)
		}
		if !descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer == "" {
			t.Fatalf("R27 Phase 5 provider %q is not routed to an analyzer: %+v", id, descriptor)
		}
	}

	for _, id := range []string{"vb6", "vba", "vbscript", "qbasic", "classic-basic", "freebasic", "purebasic", "fsharp", "cpp-cli", "jscript-net", "cil", "powershell", "aspnet-webforms", "razor", "blazor", "xaml"} {
		descriptor, ok := registry.Lookup(id)
		if !ok {
			t.Fatalf("missing R27 Phase 6 language %q", id)
		}
		if !descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer == "" {
			t.Fatalf("R27 Phase 6 provider %q is not routed to an analyzer: %+v", id, descriptor)
		}
	}

	for _, id := range []string{"mql4", "mql5", "objective-c", "objective-cpp", "dart", "d", "zig", "nim", "solidity", "apex", "al", "arduino"} {
		descriptor, ok := registry.Lookup(id)
		if !ok {
			t.Fatalf("missing R27 Phase 7 language %q", id)
		}
		if !descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer == "" {
			t.Fatalf("R27 Phase 7 provider %q is not routed to an analyzer: %+v", id, descriptor)
		}
	}

	for _, id := range []string{"vue", "svelte", "astro", "php-html", "jsp", "jinja", "twig", "blade", "ejs"} {
		descriptor, ok := registry.Lookup(id)
		if !ok {
			t.Fatalf("missing R27 Phase 11 language %q", id)
		}
		if !descriptor.Capabilities.SourceAnalysis || !descriptor.Capabilities.Composite || descriptor.Analyzer == "" {
			t.Fatalf("R27 Phase 11 provider %q is not routed to a composite analyzer: %+v", id, descriptor)
		}
	}

	for _, id := range []string{"flow", "scala"} {
		descriptor, ok := registry.Lookup(id)
		if !ok {
			t.Fatalf("missing R27 Phase 16 language %q", id)
		}
		if !descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer == "" {
			t.Fatalf("R27 Phase 16 provider %q is not routed to an analyzer: %+v", id, descriptor)
		}
	}
	for _, id := range []string{"dockerfile", "make"} {
		descriptor, ok := registry.Lookup(id)
		if !ok {
			t.Fatalf("registry cannot represent auxiliary metadata row %q", id)
		}
		if descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer != "" {
			t.Fatalf("auxiliary metadata row %q was accidentally activated: %+v", id, descriptor)
		}
	}

	for alias, want := range map[string]string{
		"golang": "go",
		"cs":     "csharp",
		"c#":     "csharp",
		"py":     "python",
		"asp":    "classic-asp",
	} {
		if got, ok := registry.Resolve(alias); !ok || got.ID != want {
			t.Fatalf("resolve(%q) = %+v, %v; want %q", alias, got, ok, want)
		}
	}
}

func TestLanguageRegistryRejectsInvalidDefinitions(t *testing.T) {
	valid := LanguageDescriptor{
		ID:         "alpha",
		Aliases:    []string{"a"},
		Extensions: []string{".alpha"},
		Analyzer:   AnalyzerGo,
		Capabilities: LanguageCapabilities{
			SourceAnalysis: true,
		},
	}

	tests := []struct {
		name        string
		descriptors []LanguageDescriptor
		want        string
	}{
		{
			name:        "duplicate canonical ID",
			descriptors: []LanguageDescriptor{valid, withRegistryID(valid, "alpha")},
			want:        "duplicate canonical language",
		},
		{
			name: "duplicate alias",
			descriptors: []LanguageDescriptor{valid, {
				ID:      "beta",
				Aliases: []string{"a"},
			}},
			want: "duplicate language name or alias",
		},
		{
			name: "conflicting extension without ambiguity declaration",
			descriptors: []LanguageDescriptor{valid, {
				ID:         "beta",
				Extensions: []string{".alpha"},
			}},
			want: "conflicting extension",
		},
		{
			name: "ambiguous extension declared by one side only",
			descriptors: []LanguageDescriptor{{
				ID:                  "alpha",
				AmbiguousExtensions: []string{".x"},
			}, {
				ID:         "beta",
				Extensions: []string{".x"},
			}},
			want: "conflicting extension",
		},
		{
			name:        "duplicate exact basename",
			descriptors: []LanguageDescriptor{{ID: "alpha", ExactBasenames: []string{"Buildfile"}}, {ID: "beta", ExactBasenames: []string{"buildfile"}}},
			want:        "conflicting exact basename",
		},
		{
			name: "invalid analyzer reference",
			descriptors: []LanguageDescriptor{{
				ID:       "alpha",
				Analyzer: AnalyzerID("missing-analyzer"),
				Capabilities: LanguageCapabilities{
					SourceAnalysis: true,
				},
			}},
			want: "unknown analyzer",
		},
		{
			name: "capability without analyzer",
			descriptors: []LanguageDescriptor{{
				ID: "alpha",
				Capabilities: LanguageCapabilities{
					SourceAnalysis: true,
				},
			}},
			want: "requires an analyzer",
		},
		{
			name:        "analyzer without capability",
			descriptors: []LanguageDescriptor{{ID: "alpha", Analyzer: AnalyzerGo}},
			want:        "requires source-analysis capability",
		},
		{
			name:        "invalid ID",
			descriptors: []LanguageDescriptor{{ID: "Not Canonical"}},
			want:        "invalid canonical language ID",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewLanguageRegistry(testCase.descriptors)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("NewLanguageRegistry error = %v, want substring %q", err, testCase.want)
			}
		})
	}
}

func TestLanguageRegistryAcceptsExplicitAmbiguityClasses(t *testing.T) {
	registry, err := NewLanguageRegistry([]LanguageDescriptor{
		{ID: "c", AmbiguousExtensions: []string{".h"}},
		{ID: "cpp", AmbiguousExtensions: []string{".h"}},
		{ID: "objective-c", AmbiguousExtensions: []string{".h"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	candidates := registry.ExtensionCandidates(".H")
	if len(candidates) != 3 {
		t.Fatalf(".h candidates = %+v, want 3", candidates)
	}
	for index, want := range []string{"c", "cpp", "objective-c"} {
		if candidates[index].ID != want {
			t.Fatalf(".h candidates[%d] = %q, want %q", index, candidates[index].ID, want)
		}
	}
}

func TestCapabilityMatrixCoversApprovedCatalogWithoutAccidentalActivation(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	rows := registry.CapabilityRows()
	if len(rows) != len(defaultLanguageDescriptors()) {
		t.Fatalf("capability rows = %d, descriptors=%d", len(rows), len(defaultLanguageDescriptors()))
	}
	byID := make(map[string]LanguageCapabilityRow, len(rows))
	for _, row := range rows {
		if row.ID == "" || row.Family == "" || row.ScannerProfile == "" || row.EncodingCoverage == "" || row.AnalyzerStrategy == "" || row.AnalyzerVersion == "" {
			t.Fatalf("incomplete capability row: %+v", row)
		}
		if !containsEvidenceKind(row.DetectionEvidence, EvidenceExplicit) || !containsEvidenceKind(row.DetectionEvidence, EvidenceDirective) || !containsEvidenceKind(row.DetectionEvidence, EvidenceProjectHint) {
			t.Fatalf("capability row %s omits universal detector evidence: %v", row.ID, row.DetectionEvidence)
		}
		if row.Capabilities.SourceAnalysis {
			if row.Analyzer == "" || row.ScannerProfile == "unimplemented" || row.AnalyzerStrategy == "unimplemented" || !row.Capabilities.Declarations || !row.Capabilities.Ranges {
				t.Fatalf("active analyzer row %s is incomplete: %+v", row.ID, row)
			}
		} else if row.Analyzer != "" || row.AnalyzerStrategy != "unimplemented" || len(row.KnownLimitations) == 0 {
			t.Fatalf("inactive row %s overclaims implementation: %+v", row.ID, row)
		}
		if _, duplicate := byID[row.ID]; duplicate {
			t.Fatalf("duplicate capability row %s", row.ID)
		}
		byID[row.ID] = row
	}

	approved := []string{
		"c", "cpp", "objective-c", "objective-cpp", "csharp", "java", "kotlin", "scala", "go", "rust", "swift", "dart", "d", "zig", "nim",
		"javascript", "typescript", "flow", "python", "php", "ruby", "perl", "lua", "luau", "elixir", "erlang", "gleam", "groovy",
		"vbnet", "fsharp", "cpp-cli", "jscript-net", "cil", "powershell", "vb6", "vba", "vbscript", "qbasic", "classic-basic", "freebasic", "purebasic",
		"fortran", "cobol", "ada", "pascal", "delphi", "matlab", "octave", "julia", "r", "haskell", "ocaml", "common-lisp", "clojure", "emacs-lisp",
		"shell", "bash", "tcl", "autohotkey", "mql4", "mql5", "assembly", "vhdl", "verilog", "systemverilog", "arduino",
		"sql", "plsql", "graphql", "terraform", "nix", "proto", "solidity", "apex", "al",
		"html", "xml", "xaml", "css", "scss", "sass", "less", "json", "yaml", "toml", "markdown", "openapi", "ansible-yaml",
		"classic-asp", "aspnet-webforms", "razor", "blazor", "vue", "svelte", "astro", "php-html", "jsp", "jinja", "twig", "blade", "ejs",
	}
	for _, id := range approved {
		row, ok := byID[id]
		if !ok {
			t.Errorf("approved R27 catalog entry %q has no capability row", id)
			continue
		}
		if !row.Capabilities.SourceAnalysis || row.Analyzer == "" || row.AnalyzerStrategy == "unimplemented" {
			t.Errorf("approved R27 catalog entry %q is not production-active at the Phase 16 completion gate: %+v", id, row)
			continue
		}
		descriptor, exists := registry.Lookup(id)
		analyzer, available := AnalyzerFor(descriptor)
		if !exists || !available || analyzer == nil || analyzer.ID() != descriptor.Analyzer || analyzer.Language() != id {
			t.Errorf("approved R27 catalog entry %q has no matching active analyzer: descriptor=%+v analyzer=%#v available=%v", id, descriptor, analyzer, available)
		}
	}
}

func containsEvidenceKind(values []EvidenceKind, want EvidenceKind) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func withRegistryID(descriptor LanguageDescriptor, id string) LanguageDescriptor {
	descriptor.ID = id
	return descriptor
}
