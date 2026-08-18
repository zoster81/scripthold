package sourceintelligence

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

type providerContractManifest struct {
	SchemaVersion              int                `json:"schemaVersion"`
	ReviewedAgainst            []string           `json:"reviewedAgainst"`
	ActiveBaselineCapabilities []string           `json:"activeBaselineCapabilities"`
	CompositeLanguages         []string           `json:"compositeLanguages"`
	ProjectResolvedLanguages   []string           `json:"projectResolvedLanguages"`
	ImplementationLanguages    []string           `json:"implementationLanguages"`
	Providers                  []providerContract `json:"providers"`
}

type providerContract struct {
	Language string `json:"language"`
	Provider string `json:"provider,omitempty"`
	Status   string `json:"status"`
}

func TestProviderContractManifestMatchesRegistryAndDocumentation(t *testing.T) {
	manifest := loadProviderContractManifest(t)
	if manifest.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", manifest.SchemaVersion)
	}
	if len(manifest.ReviewedAgainst) == 0 {
		t.Fatal("provider contract manifest must name independent reviewed source-of-truth documents")
	}
	root := sourceIntelligenceRepositoryRoot(t)
	for _, source := range manifest.ReviewedAgainst {
		if filepath.IsAbs(source) || source == "" {
			t.Fatalf("reviewed source must be a non-empty repository-relative path: %q", source)
		}
		clean := filepath.Clean(filepath.FromSlash(source))
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			t.Fatalf("reviewed source escapes repository: %q", source)
		}
		if _, err := os.Stat(filepath.Join(root, clean)); err != nil {
			t.Fatalf("reviewed source %q: %v", source, err)
		}
	}

	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	documentation, err := os.ReadFile(filepath.Join(root, "docs", "LANGUAGE_CAPABILITIES.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertProviderContractRegistryAndDocumentation(t, manifest, registry, string(documentation))
}

func assertProviderContractRegistryAndDocumentation(t testing.TB, manifest providerContractManifest, registry *LanguageRegistry, documentation string) {
	t.Helper()
	rows := registry.CapabilityRows()
	if len(rows) != len(manifest.Providers) {
		t.Fatalf("registry rows = %d, independent provider contracts = %d", len(rows), len(manifest.Providers))
	}
	documentedRows := capabilityDocumentationRows(documentation)
	baselineCapabilities := stringSetForProviderContract(t, "activeBaselineCapabilities", manifest.ActiveBaselineCapabilities)
	compositeLanguages := stringSetForProviderContract(t, "compositeLanguages", manifest.CompositeLanguages)
	projectResolvedLanguages := stringSetForProviderContract(t, "projectResolvedLanguages", manifest.ProjectResolvedLanguages)
	implementationLanguages := stringSetForProviderContract(t, "implementationLanguages", manifest.ImplementationLanguages)

	languageSeen := make(map[string]bool, len(manifest.Providers))
	providerSeen := make(map[string]string, len(manifest.Providers))
	activeLanguages := make(map[string]bool, len(manifest.Providers))
	for _, contract := range manifest.Providers {
		if contract.Language == "" || strings.TrimSpace(contract.Language) != contract.Language {
			t.Fatalf("invalid contract language %q", contract.Language)
		}
		if languageSeen[contract.Language] {
			t.Fatalf("duplicate provider contract language %q", contract.Language)
		}
		languageSeen[contract.Language] = true

		descriptor, ok := registry.Resolve(contract.Language)
		if !ok || descriptor.ID != contract.Language {
			t.Fatalf("contract language %q missing as canonical registry row", contract.Language)
		}
		row, ok := documentedRows[contract.Language]
		if !ok {
			t.Fatalf("contract language %q missing from capability documentation", contract.Language)
		}

		switch contract.Status {
		case "active":
			activeLanguages[contract.Language] = true
			if contract.Provider == "" {
				t.Fatalf("active contract %q has no provider", contract.Language)
			}
			if prior, duplicate := providerSeen[contract.Provider]; duplicate {
				t.Fatalf("provider %q is assigned to both %q and %q", contract.Provider, prior, contract.Language)
			}
			providerSeen[contract.Provider] = contract.Language
			if !descriptor.Capabilities.SourceAnalysis {
				t.Fatalf("contract language %q is active but registry sourceAnalysis=false", contract.Language)
			}
			if got := string(descriptor.Analyzer); got != contract.Provider {
				t.Fatalf("contract language %q provider=%q, registry=%q", contract.Language, contract.Provider, got)
			}
			analyzer, available := AnalyzerFor(descriptor)
			if !available {
				t.Fatalf("active contract language %q has no analyzer", contract.Language)
			}
			if string(analyzer.ID()) != contract.Provider || analyzer.Language() != contract.Language {
				t.Fatalf("analyzer identity mismatch for %q: id=%q language=%q", contract.Language, analyzer.ID(), analyzer.Language())
			}
			if !strings.Contains(row, contract.Provider+" - ") {
				t.Fatalf("capability documentation row for %q does not contain provider %q", contract.Language, contract.Provider)
			}
			for capability := range baselineCapabilities {
				if !providerHasCapability(descriptor.Capabilities, capability) {
					t.Fatalf("active contract %q is missing baseline capability %q", contract.Language, capability)
				}
				if !documentationRowHasCapability(row, capability) {
					t.Fatalf("capability documentation row for %q is missing baseline capability %q", contract.Language, capability)
				}
			}
			if got, want := descriptor.Capabilities.Composite, compositeLanguages[contract.Language]; got != want {
				t.Fatalf("contract language %q composite=%v, want %v", contract.Language, got, want)
			}
			projectResolved := descriptor.Capabilities.ProjectResolvedReferences && descriptor.Capabilities.ProjectResolvedDefinitions
			if got, want := projectResolved, projectResolvedLanguages[contract.Language]; got != want {
				t.Fatalf("contract language %q project-resolved capability=%v, want %v", contract.Language, got, want)
			}
			if descriptor.Capabilities.ProjectResolvedReferences != descriptor.Capabilities.ProjectResolvedDefinitions {
				t.Fatalf("contract language %q has asymmetric project reference/definition claims", contract.Language)
			}
			if got, want := descriptor.Capabilities.Implementations, implementationLanguages[contract.Language]; got != want {
				t.Fatalf("contract language %q implementations=%v, want %v", contract.Language, got, want)
			}
		case "inactive":
			if contract.Provider != "" {
				t.Fatalf("inactive contract %q unexpectedly declares provider %q", contract.Language, contract.Provider)
			}
			if descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer != "" {
				t.Fatalf("inactive contract %q unexpectedly activates source analysis: %+v", contract.Language, descriptor)
			}
			if analyzer, available := AnalyzerFor(descriptor); available || analyzer != nil {
				t.Fatalf("inactive contract %q unexpectedly resolves analyzer", contract.Language)
			}
			if !strings.Contains(row, "unimplemented/none") {
				t.Fatalf("inactive contract %q is not documented as unimplemented", contract.Language)
			}
		default:
			t.Fatalf("contract language %q has invalid status %q", contract.Language, contract.Status)
		}
	}

	var registryOnly []string
	for _, row := range rows {
		if !languageSeen[row.ID] {
			registryOnly = append(registryOnly, row.ID)
		}
	}
	if len(registryOnly) != 0 {
		sort.Strings(registryOnly)
		t.Fatalf("registry contains languages absent from independent provider contract: %v", registryOnly)
	}
	for label, values := range map[string]map[string]bool{
		"compositeLanguages":       compositeLanguages,
		"projectResolvedLanguages": projectResolvedLanguages,
		"implementationLanguages":  implementationLanguages,
	} {
		for language := range values {
			if !activeLanguages[language] {
				t.Fatalf("%s references non-active language %q", label, language)
			}
		}
	}
}

func TestProviderContractSharedAnalyzerInvariants(t *testing.T) {
	manifest := loadProviderContractManifest(t)
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range manifest.Providers {
		if contract.Status != "active" {
			continue
		}
		t.Run(contract.Language, func(t *testing.T) {
			descriptor, ok := registry.Resolve(contract.Language)
			if !ok {
				t.Fatal("missing registry descriptor")
			}
			analyzer, ok := AnalyzerFor(descriptor)
			if !ok {
				t.Fatal("missing analyzer")
			}
			document := sourceDocumentForScanner(" \n")
			document.Path = "contract-" + contract.Language + ".fixture"
			before := cloneSourceDocumentForContract(document)
			options := testAnalyzeOptions(true, 16)

			first, err := analyzer.Analyze(context.Background(), document, options)
			if err != nil {
				t.Fatalf("neutral input failed: %v", err)
			}
			second, err := analyzer.Analyze(context.Background(), document, options)
			if err != nil {
				t.Fatalf("second neutral analysis failed: %v", err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("analyzer output is nondeterministic on identical input")
			}
			if !reflect.DeepEqual(before, *document) {
				t.Fatal("analyzer mutated SourceDocument")
			}
			assertAnalyzerResultWithinContractBounds(t, first, 16)
			assertAnalyzerResultCoordinates(t, first)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err = analyzer.Analyze(ctx, document, options)
			if operation.KindOf(err) != operation.KindCancelled {
				t.Fatalf("cancelled analysis error=%v kind=%v, want CANCELLED", err, operation.KindOf(err))
			}
		})
	}
}

func TestProviderContractMalformedInputNeverPanicsOrEscapesBounds(t *testing.T) {
	manifest := loadProviderContractManifest(t)
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	malformed := []string{
		"\x00\x00\x00\n",
		"/* unterminated ${ \\\" ' ` {{{ [[[ (((\n",
		"\"\"\"unterminated\n<% {{ {% @@@ ###\n",
		strings.Repeat("(", 512) + strings.Repeat(")", 127),
	}
	for _, contract := range manifest.Providers {
		if contract.Status != "active" {
			continue
		}
		t.Run(contract.Language, func(t *testing.T) {
			descriptor, _ := registry.Resolve(contract.Language)
			analyzer, _ := AnalyzerFor(descriptor)
			for index, text := range malformed {
				func() {
					defer func() {
						if recovered := recover(); recovered != nil {
							t.Fatalf("malformed fixture %d panicked: %v", index, recovered)
						}
					}()
					document := sourceDocumentForScanner(text)
					result, analyzeErr := analyzer.Analyze(context.Background(), document, testAnalyzeOptions(false, 8))
					if analyzeErr == nil {
						assertAnalyzerResultWithinContractBounds(t, result, 8)
						assertAnalyzerResultCoordinates(t, result)
					}
				}()
			}
		})
	}
}

func loadProviderContractManifest(t testing.TB) providerContractManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "provider-contracts.json"))
	if err != nil {
		t.Fatalf("read provider contract manifest: %v", err)
	}
	var manifest providerContractManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode provider contract manifest: %v", err)
	}
	return manifest
}

func sourceIntelligenceRepositoryRoot(t testing.TB) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func stringSetForProviderContract(t testing.TB, field string, values []string) map[string]bool {
	t.Helper()
	if len(values) == 0 {
		t.Fatalf("%s must not be empty", field)
	}
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || strings.TrimSpace(value) != value || result[value] {
			t.Fatalf("%s contains invalid or duplicate value %q", field, value)
		}
		result[value] = true
	}
	return result
}

func providerHasCapability(value LanguageCapabilities, code string) bool {
	switch code {
	case "decl":
		return value.Declarations
	case "hier":
		return value.Hierarchy
	case "sig":
		return value.Signatures
	case "range":
		return value.Ranges
	case "dep":
		return value.Dependencies
	case "inh":
		return value.InheritanceRelations
	case "call":
		return value.SyntacticCalls
	case "scope-ref":
		return value.ScopeResolvedReferences
	case "project-ref":
		return value.ProjectResolvedReferences
	case "project-def":
		return value.ProjectResolvedDefinitions
	case "impl":
		return value.Implementations
	case "override":
		return value.Overrides
	case "semantic":
		return value.SemanticRelations
	case "index":
		return value.IncrementalIndex
	default:
		return false
	}
}

func documentationRowHasCapability(row, capability string) bool {
	columns := strings.Split(row, " | ")
	if len(columns) < 7 {
		return false
	}
	for _, value := range strings.Split(strings.TrimSpace(columns[5]), ",") {
		if strings.TrimSpace(value) == capability {
			return true
		}
	}
	return false
}

func capabilityDocumentationRows(markdown string) map[string]string {
	rows := make(map[string]string)
	for _, line := range strings.Split(markdown, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		end := strings.Index(line[3:], "`")
		if end < 0 {
			continue
		}
		rows[line[3:3+end]] = line
	}
	return rows
}

func cloneSourceDocumentForContract(document *SourceDocument) SourceDocument {
	clone := *document
	clone.lineStarts = append([]int(nil), document.lineStarts...)
	return clone
}

func assertAnalyzerResultWithinContractBounds(t testing.TB, result AnalyzerResult, max int) {
	t.Helper()
	if len(result.Analysis.Symbols) > max || len(result.Dependencies) > max || len(result.Relations) > max || len(result.Regions) > max {
		t.Fatalf("result exceeded bound %d: symbols=%d dependencies=%d relations=%d regions=%d", max, len(result.Analysis.Symbols), len(result.Dependencies), len(result.Relations), len(result.Regions))
	}
}

func assertAnalyzerResultCoordinates(t testing.TB, result AnalyzerResult) {
	t.Helper()
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Name == "" || symbol.QualifiedName == "" || symbol.DeclarationRange.Start.Line <= 0 || symbol.DeclarationRange.Start.Column <= 0 || symbol.DeclarationRange.End.Line <= 0 || symbol.DeclarationRange.End.Column <= 0 {
			t.Fatalf("invalid symbol: %+v", symbol)
		}
	}
	for _, dependency := range result.Dependencies {
		if dependency.Value == "" || dependency.Range.Start.Line <= 0 || dependency.Range.Start.Column <= 0 || dependency.Range.End.Line <= 0 || dependency.Range.End.Column <= 0 {
			t.Fatalf("invalid dependency: %+v", dependency)
		}
	}
	for _, relation := range result.Relations {
		if relation.Source == "" || relation.Target == "" || relation.Range.Start.Line <= 0 || relation.Range.Start.Column <= 0 || relation.Range.End.Line <= 0 || relation.Range.End.Column <= 0 {
			t.Fatalf("invalid relation: %+v", relation)
		}
	}
	for _, region := range result.Regions {
		if region.ID == "" || region.Kind == "" || region.Range.Start.Line <= 0 || region.Range.Start.Column <= 0 || region.Range.End.Line <= 0 || region.Range.End.Column <= 0 {
			t.Fatalf("invalid region: %+v", region)
		}
	}
}
