package sourceintelligence

import (
	"strings"
	"testing"
)

func TestDefaultLanguageRegistryR25CanariesAndFutureShapes(t *testing.T) {
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

	for _, id := range []string{"cpp", "rust", "typescript", "razor", "vue", "mql4", "mql5", "delphi"} {
		descriptor, ok := registry.Lookup(id)
		if !ok {
			t.Fatalf("registry cannot represent future language %q", id)
		}
		if descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer != "" {
			t.Fatalf("future R27 entry %q was accidentally activated: %+v", id, descriptor)
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

func withRegistryID(descriptor LanguageDescriptor, id string) LanguageDescriptor {
	descriptor.ID = id
	return descriptor
}
