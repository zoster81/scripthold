package sourceintelligence

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// AnalyzerID is a stable internal routing identifier. Declaring an analyzer ID
// does not expose analyzer implementation details through the public contract.
type AnalyzerID string

const (
	AnalyzerGo             AnalyzerID = "go-ast"
	AnalyzerCSharp         AnalyzerID = "csharp-native"
	AnalyzerVBNet          AnalyzerID = "vbnet-native"
	AnalyzerPython         AnalyzerID = "python-native"
	AnalyzerClassicASP     AnalyzerID = "classic-asp-composite"
	AnalyzerC              AnalyzerID = "c-native"
	AnalyzerCPP            AnalyzerID = "cpp-native"
	AnalyzerJava           AnalyzerID = "java-native"
	AnalyzerKotlin         AnalyzerID = "kotlin-native"
	AnalyzerJavaScript     AnalyzerID = "javascript-native"
	AnalyzerTypeScript     AnalyzerID = "typescript-native"
	AnalyzerRust           AnalyzerID = "rust-native"
	AnalyzerPHP            AnalyzerID = "php-native"
	AnalyzerRuby           AnalyzerID = "ruby-native"
	AnalyzerSwift          AnalyzerID = "swift-native"
	AnalyzerPascal         AnalyzerID = "pascal-native"
	AnalyzerDelphi         AnalyzerID = "delphi-native"
	AnalyzerVB6            AnalyzerID = "vb6-native"
	AnalyzerVBA            AnalyzerID = "vba-native"
	AnalyzerVBScript       AnalyzerID = "vbscript-native"
	AnalyzerQBasic         AnalyzerID = "qbasic-native"
	AnalyzerClassicBasic   AnalyzerID = "classic-basic-native"
	AnalyzerFreeBasic      AnalyzerID = "freebasic-native"
	AnalyzerPureBasic      AnalyzerID = "purebasic-native"
	AnalyzerFSharp         AnalyzerID = "fsharp-native"
	AnalyzerCPPCLI         AnalyzerID = "cpp-cli-native"
	AnalyzerJScriptNet     AnalyzerID = "jscript-net-native"
	AnalyzerCIL            AnalyzerID = "cil-native"
	AnalyzerPowerShell     AnalyzerID = "powershell-native"
	AnalyzerASPNetWebForms AnalyzerID = "aspnet-webforms-composite"
	AnalyzerRazor          AnalyzerID = "razor-composite"
	AnalyzerBlazor         AnalyzerID = "blazor-composite"
	AnalyzerXAML           AnalyzerID = "xaml-native"
	AnalyzerMQL4           AnalyzerID = "mql4-native"
	AnalyzerMQL5           AnalyzerID = "mql5-native"
	AnalyzerObjectiveC     AnalyzerID = "objective-c-native"
	AnalyzerObjectiveCPP   AnalyzerID = "objective-cpp-native"
	AnalyzerDart           AnalyzerID = "dart-native"
	AnalyzerD              AnalyzerID = "d-native"
	AnalyzerZig            AnalyzerID = "zig-native"
	AnalyzerNim            AnalyzerID = "nim-native"
	AnalyzerSolidity       AnalyzerID = "solidity-native"
	AnalyzerApex           AnalyzerID = "apex-native"
	AnalyzerAL             AnalyzerID = "al-native"
	AnalyzerArduino        AnalyzerID = "arduino-native"
	AnalyzerPerl           AnalyzerID = "perl-native"
	AnalyzerLua            AnalyzerID = "lua-native"
	AnalyzerLuau           AnalyzerID = "luau-native"
	AnalyzerElixir         AnalyzerID = "elixir-native"
	AnalyzerErlang         AnalyzerID = "erlang-native"
	AnalyzerGleam          AnalyzerID = "gleam-native"
	AnalyzerGroovy         AnalyzerID = "groovy-native"
	AnalyzerShell          AnalyzerID = "shell-native"
	AnalyzerBash           AnalyzerID = "bash-native"
	AnalyzerTcl            AnalyzerID = "tcl-native"
	AnalyzerAutoHotkey     AnalyzerID = "autohotkey-native"
	AnalyzerFortran        AnalyzerID = "fortran-native"
	AnalyzerCOBOL          AnalyzerID = "cobol-native"
	AnalyzerAda            AnalyzerID = "ada-native"
	AnalyzerMATLAB         AnalyzerID = "matlab-native"
	AnalyzerOctave         AnalyzerID = "octave-native"
	AnalyzerJulia          AnalyzerID = "julia-native"
	AnalyzerR              AnalyzerID = "r-native"
	AnalyzerHaskell        AnalyzerID = "haskell-native"
	AnalyzerOCaml          AnalyzerID = "ocaml-native"
	AnalyzerCommonLisp     AnalyzerID = "common-lisp-native"
	AnalyzerClojure        AnalyzerID = "clojure-native"
	AnalyzerEmacsLisp      AnalyzerID = "emacs-lisp-native"
	AnalyzerSQL            AnalyzerID = "sql-native"
	AnalyzerPLSQL          AnalyzerID = "plsql-native"
	AnalyzerGraphQL        AnalyzerID = "graphql-native"
	AnalyzerTerraform      AnalyzerID = "terraform-native"
	AnalyzerNix            AnalyzerID = "nix-native"
	AnalyzerProto          AnalyzerID = "proto-native"
	AnalyzerVHDL           AnalyzerID = "vhdl-native"
	AnalyzerVerilog        AnalyzerID = "verilog-native"
	AnalyzerSystemVerilog  AnalyzerID = "systemverilog-native"
	AnalyzerAssembly       AnalyzerID = "assembly-native"
	AnalyzerHTML           AnalyzerID = "html-native"
	AnalyzerXML            AnalyzerID = "xml-native"
	AnalyzerCSS            AnalyzerID = "css-native"
	AnalyzerSCSS           AnalyzerID = "scss-native"
	AnalyzerSass           AnalyzerID = "sass-native"
	AnalyzerLess           AnalyzerID = "less-native"
	AnalyzerJSON           AnalyzerID = "json-native"
	AnalyzerYAML           AnalyzerID = "yaml-native"
	AnalyzerTOML           AnalyzerID = "toml-native"
	AnalyzerMarkdown       AnalyzerID = "markdown-native"
	AnalyzerOpenAPI        AnalyzerID = "openapi-native"
	AnalyzerAnsibleYAML    AnalyzerID = "ansible-yaml-native"
	AnalyzerVue            AnalyzerID = "vue-native"
	AnalyzerSvelte         AnalyzerID = "svelte-native"
	AnalyzerAstro          AnalyzerID = "astro-native"
	AnalyzerPHPHTML        AnalyzerID = "php-html-native"
	AnalyzerJSP            AnalyzerID = "jsp-native"
	AnalyzerJinja          AnalyzerID = "jinja-native"
	AnalyzerTwig           AnalyzerID = "twig-native"
	AnalyzerBlade          AnalyzerID = "blade-native"
	AnalyzerEJS            AnalyzerID = "ejs-native"
)

var knownAnalyzerIDs = map[AnalyzerID]struct{}{
	AnalyzerGo:             {},
	AnalyzerCSharp:         {},
	AnalyzerVBNet:          {},
	AnalyzerPython:         {},
	AnalyzerClassicASP:     {},
	AnalyzerC:              {},
	AnalyzerCPP:            {},
	AnalyzerJava:           {},
	AnalyzerKotlin:         {},
	AnalyzerJavaScript:     {},
	AnalyzerTypeScript:     {},
	AnalyzerRust:           {},
	AnalyzerPHP:            {},
	AnalyzerRuby:           {},
	AnalyzerSwift:          {},
	AnalyzerPascal:         {},
	AnalyzerDelphi:         {},
	AnalyzerVB6:            {},
	AnalyzerVBA:            {},
	AnalyzerVBScript:       {},
	AnalyzerQBasic:         {},
	AnalyzerClassicBasic:   {},
	AnalyzerFreeBasic:      {},
	AnalyzerPureBasic:      {},
	AnalyzerFSharp:         {},
	AnalyzerCPPCLI:         {},
	AnalyzerJScriptNet:     {},
	AnalyzerCIL:            {},
	AnalyzerPowerShell:     {},
	AnalyzerASPNetWebForms: {},
	AnalyzerRazor:          {},
	AnalyzerBlazor:         {},
	AnalyzerXAML:           {},
	AnalyzerMQL4:           {},
	AnalyzerMQL5:           {},
	AnalyzerObjectiveC:     {},
	AnalyzerObjectiveCPP:   {},
	AnalyzerDart:           {},
	AnalyzerD:              {},
	AnalyzerZig:            {},
	AnalyzerNim:            {},
	AnalyzerSolidity:       {},
	AnalyzerApex:           {},
	AnalyzerAL:             {},
	AnalyzerArduino:        {},
	AnalyzerPerl:           {},
	AnalyzerLua:            {},
	AnalyzerLuau:           {},
	AnalyzerElixir:         {},
	AnalyzerErlang:         {},
	AnalyzerGleam:          {},
	AnalyzerGroovy:         {},
	AnalyzerShell:          {},
	AnalyzerBash:           {},
	AnalyzerTcl:            {},
	AnalyzerAutoHotkey:     {},
	AnalyzerFortran:        {},
	AnalyzerCOBOL:          {},
	AnalyzerAda:            {},
	AnalyzerMATLAB:         {},
	AnalyzerOctave:         {},
	AnalyzerJulia:          {},
	AnalyzerR:              {},
	AnalyzerHaskell:        {},
	AnalyzerOCaml:          {},
	AnalyzerCommonLisp:     {},
	AnalyzerClojure:        {},
	AnalyzerEmacsLisp:      {},
	AnalyzerSQL:            {},
	AnalyzerPLSQL:          {},
	AnalyzerGraphQL:        {},
	AnalyzerTerraform:      {},
	AnalyzerNix:            {},
	AnalyzerProto:          {},
	AnalyzerVHDL:           {},
	AnalyzerVerilog:        {},
	AnalyzerSystemVerilog:  {},
	AnalyzerAssembly:       {},
	AnalyzerHTML:           {},
	AnalyzerXML:            {},
	AnalyzerCSS:            {},
	AnalyzerSCSS:           {},
	AnalyzerSass:           {},
	AnalyzerLess:           {},
	AnalyzerJSON:           {},
	AnalyzerYAML:           {},
	AnalyzerTOML:           {},
	AnalyzerMarkdown:       {},
	AnalyzerOpenAPI:        {},
	AnalyzerAnsibleYAML:    {},
	AnalyzerVue:            {},
	AnalyzerSvelte:         {},
	AnalyzerAstro:          {},
	AnalyzerPHPHTML:        {},
	AnalyzerJSP:            {},
	AnalyzerJinja:          {},
	AnalyzerTwig:           {},
	AnalyzerBlade:          {},
	AnalyzerEJS:            {},
}

var canonicalLanguageIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// LanguageCapabilities describe only capabilities actually enabled in the
// current milestone. Future registry entries can exist with all capabilities false.
type LanguageCapabilities struct {
	SourceAnalysis             bool
	Composite                  bool
	CaseInsensitive            bool
	Declarations               bool
	Hierarchy                  bool
	Signatures                 bool
	Ranges                     bool
	Dependencies               bool
	InheritanceRelations       bool
	SyntacticCalls             bool
	ScopeResolvedReferences    bool
	ProjectResolvedReferences  bool
	ProjectResolvedDefinitions bool
	Implementations            bool
	Overrides                  bool
	SemanticRelations          bool
	IncrementalIndex           bool
}

// LanguageDescriptor is the canonical routing record for one language family.
type LanguageDescriptor struct {
	ID                  string
	Aliases             []string
	ExactBasenames      []string
	CompoundSuffixes    []string
	Extensions          []string
	AmbiguousExtensions []string
	ShebangInterpreters []string
	Analyzer            AnalyzerID
	Capabilities        LanguageCapabilities
	Family              string
	DetectionEvidence   []EvidenceKind
	ScannerProfile      string
	CompositeBehavior   string
	EncodingCoverage    string
	AnalyzerStrategy    string
	AnalyzerVersion     string
	KnownLimitations    []string
}

// LanguageRegistry is an immutable validated routing table after construction.
type LanguageRegistry struct {
	byID             map[string]LanguageDescriptor
	byName           map[string]string
	exactBasenames   map[string]string
	compoundSuffixes map[string]string
	extensions       map[string][]string
	shebangs         map[string][]string
	orderedSuffixes  []string
}

type extensionRegistration struct {
	language  string
	ambiguous bool
}

// NewLanguageRegistry validates and normalizes descriptors fail-closed.
func NewLanguageRegistry(descriptors []LanguageDescriptor) (*LanguageRegistry, error) {
	registry := &LanguageRegistry{
		byID:             make(map[string]LanguageDescriptor, len(descriptors)),
		byName:           make(map[string]string, len(descriptors)*2),
		exactBasenames:   make(map[string]string),
		compoundSuffixes: make(map[string]string),
		extensions:       make(map[string][]string),
		shebangs:         make(map[string][]string),
	}
	extensionRegistrations := make(map[string][]extensionRegistration)

	for _, original := range descriptors {
		descriptor := cloneLanguageDescriptor(original)
		descriptor.ID = normalizeLanguageName(descriptor.ID)
		if !canonicalLanguageIDPattern.MatchString(descriptor.ID) {
			return nil, fmt.Errorf("invalid canonical language ID %q", original.ID)
		}
		if _, exists := registry.byID[descriptor.ID]; exists {
			return nil, fmt.Errorf("duplicate canonical language %q", descriptor.ID)
		}
		descriptor = enrichLanguageDescriptor(descriptor)
		if descriptor.Analyzer != "" {
			if _, ok := knownAnalyzerIDs[descriptor.Analyzer]; !ok {
				return nil, fmt.Errorf("language %s references unknown analyzer %q", descriptor.ID, descriptor.Analyzer)
			}
			if !descriptor.Capabilities.SourceAnalysis {
				return nil, fmt.Errorf("language %s analyzer requires source-analysis capability", descriptor.ID)
			}
		} else if descriptor.Capabilities.SourceAnalysis {
			return nil, fmt.Errorf("language %s source-analysis capability requires an analyzer", descriptor.ID)
		}
		if descriptor.Capabilities.Composite && !descriptor.Capabilities.SourceAnalysis {
			return nil, fmt.Errorf("language %s composite capability requires source-analysis capability", descriptor.ID)
		}
		if err := validateLanguageCapabilityMetadata(descriptor); err != nil {
			return nil, err
		}

		descriptor.Aliases = normalizeUniqueStrings(descriptor.Aliases, normalizeLanguageName)
		descriptor.ExactBasenames = normalizeUniqueStrings(descriptor.ExactBasenames, normalizeBasename)
		descriptor.CompoundSuffixes = normalizeUniqueStrings(descriptor.CompoundSuffixes, normalizeSuffix)
		descriptor.Extensions = normalizeUniqueStrings(descriptor.Extensions, normalizeExtension)
		descriptor.AmbiguousExtensions = normalizeUniqueStrings(descriptor.AmbiguousExtensions, normalizeExtension)
		descriptor.ShebangInterpreters = normalizeUniqueStrings(descriptor.ShebangInterpreters, normalizeInterpreter)

		registry.byID[descriptor.ID] = descriptor
		for _, name := range append([]string{descriptor.ID}, descriptor.Aliases...) {
			if previous, exists := registry.byName[name]; exists && previous != descriptor.ID {
				return nil, fmt.Errorf("duplicate language name or alias %q for %s and %s", name, previous, descriptor.ID)
			}
			registry.byName[name] = descriptor.ID
		}
		for _, basename := range descriptor.ExactBasenames {
			if previous, exists := registry.exactBasenames[basename]; exists && previous != descriptor.ID {
				return nil, fmt.Errorf("conflicting exact basename %q for %s and %s", basename, previous, descriptor.ID)
			}
			registry.exactBasenames[basename] = descriptor.ID
		}
		for _, suffix := range descriptor.CompoundSuffixes {
			if previous, exists := registry.compoundSuffixes[suffix]; exists && previous != descriptor.ID {
				return nil, fmt.Errorf("conflicting compound suffix %q for %s and %s", suffix, previous, descriptor.ID)
			}
			registry.compoundSuffixes[suffix] = descriptor.ID
		}
		for _, extension := range descriptor.Extensions {
			extensionRegistrations[extension] = append(extensionRegistrations[extension], extensionRegistration{language: descriptor.ID})
		}
		for _, extension := range descriptor.AmbiguousExtensions {
			extensionRegistrations[extension] = append(extensionRegistrations[extension], extensionRegistration{language: descriptor.ID, ambiguous: true})
		}
		for _, interpreter := range descriptor.ShebangInterpreters {
			registry.shebangs[interpreter] = append(registry.shebangs[interpreter], descriptor.ID)
		}
	}

	for extension, registrations := range extensionRegistrations {
		if len(registrations) > 1 {
			for _, registration := range registrations {
				if !registration.ambiguous {
					return nil, fmt.Errorf("conflicting extension %q includes %s without explicit ambiguity", extension, registration.language)
				}
			}
		}
		seen := make(map[string]struct{}, len(registrations))
		for _, registration := range registrations {
			if _, duplicate := seen[registration.language]; duplicate {
				return nil, fmt.Errorf("duplicate extension %q in language %s", extension, registration.language)
			}
			seen[registration.language] = struct{}{}
			registry.extensions[extension] = append(registry.extensions[extension], registration.language)
		}
		sort.Strings(registry.extensions[extension])
	}
	for interpreter := range registry.shebangs {
		sort.Strings(registry.shebangs[interpreter])
	}
	for suffix := range registry.compoundSuffixes {
		registry.orderedSuffixes = append(registry.orderedSuffixes, suffix)
	}
	sort.Slice(registry.orderedSuffixes, func(i, j int) bool {
		if len(registry.orderedSuffixes[i]) == len(registry.orderedSuffixes[j]) {
			return registry.orderedSuffixes[i] < registry.orderedSuffixes[j]
		}
		return len(registry.orderedSuffixes[i]) > len(registry.orderedSuffixes[j])
	})
	return registry, nil
}

func cloneLanguageDescriptor(descriptor LanguageDescriptor) LanguageDescriptor {
	descriptor.Aliases = append([]string(nil), descriptor.Aliases...)
	descriptor.ExactBasenames = append([]string(nil), descriptor.ExactBasenames...)
	descriptor.CompoundSuffixes = append([]string(nil), descriptor.CompoundSuffixes...)
	descriptor.Extensions = append([]string(nil), descriptor.Extensions...)
	descriptor.AmbiguousExtensions = append([]string(nil), descriptor.AmbiguousExtensions...)
	descriptor.ShebangInterpreters = append([]string(nil), descriptor.ShebangInterpreters...)
	descriptor.DetectionEvidence = append([]EvidenceKind(nil), descriptor.DetectionEvidence...)
	descriptor.KnownLimitations = append([]string(nil), descriptor.KnownLimitations...)
	return descriptor
}

func normalizeUniqueStrings(values []string, normalize func(string) string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalize(value)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result
}

func normalizeLanguageName(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func normalizeBasename(value string) string     { return strings.ToLower(strings.TrimSpace(value)) }
func normalizeInterpreter(value string) string  { return strings.ToLower(strings.TrimSpace(value)) }

func normalizeExtension(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, ".") {
		value = "." + value
	}
	return value
}

func normalizeSuffix(value string) string { return normalizeExtension(value) }

// Lookup returns a descriptor by canonical ID.
func (registry *LanguageRegistry) Lookup(id string) (LanguageDescriptor, bool) {
	if registry == nil {
		return LanguageDescriptor{}, false
	}
	descriptor, ok := registry.byID[normalizeLanguageName(id)]
	return cloneLanguageDescriptor(descriptor), ok
}

// Resolve accepts a canonical ID or registered alias.
func (registry *LanguageRegistry) Resolve(name string) (LanguageDescriptor, bool) {
	if registry == nil {
		return LanguageDescriptor{}, false
	}
	id, ok := registry.byName[normalizeLanguageName(name)]
	if !ok {
		return LanguageDescriptor{}, false
	}
	return registry.Lookup(id)
}

func (registry *LanguageRegistry) ExactBasename(name string) (LanguageDescriptor, bool) {
	if registry == nil {
		return LanguageDescriptor{}, false
	}
	id, ok := registry.exactBasenames[normalizeBasename(name)]
	if !ok {
		return LanguageDescriptor{}, false
	}
	return registry.Lookup(id)
}

func (registry *LanguageRegistry) CompoundSuffix(path string) (LanguageDescriptor, string, bool) {
	if registry == nil {
		return LanguageDescriptor{}, "", false
	}
	lower := strings.ToLower(path)
	for _, suffix := range registry.orderedSuffixes {
		if strings.HasSuffix(lower, suffix) {
			id := registry.compoundSuffixes[suffix]
			descriptor, ok := registry.Lookup(id)
			return descriptor, suffix, ok
		}
	}
	return LanguageDescriptor{}, "", false
}

// ExtensionCandidates returns deterministic candidates for one extension.
func (registry *LanguageRegistry) ExtensionCandidates(extension string) []LanguageDescriptor {
	if registry == nil {
		return nil
	}
	ids := registry.extensions[normalizeExtension(extension)]
	result := make([]LanguageDescriptor, 0, len(ids))
	for _, id := range ids {
		if descriptor, ok := registry.Lookup(id); ok {
			result = append(result, descriptor)
		}
	}
	return result
}

func (registry *LanguageRegistry) ShebangCandidates(interpreter string) []LanguageDescriptor {
	if registry == nil {
		return nil
	}
	ids := registry.shebangs[normalizeInterpreter(interpreter)]
	result := make([]LanguageDescriptor, 0, len(ids))
	for _, id := range ids {
		if descriptor, ok := registry.Lookup(id); ok {
			result = append(result, descriptor)
		}
	}
	return result
}

// DefaultLanguageRegistry contains the R25 analyzable canaries and representative
// future-family routing metadata. Future entries intentionally have no analyzer.
func DefaultLanguageRegistry() (*LanguageRegistry, error) {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		return nil, err
	}
	for _, row := range registry.CapabilityRows() {
		if row.Family == "custom" {
			return nil, fmt.Errorf("default language %s has no approved R27 family classification", row.ID)
		}
	}
	return registry, nil
}

func defaultLanguageDescriptors() []LanguageDescriptor {
	analyzable := func(id string, analyzer AnalyzerID, extensions []string, aliases ...string) LanguageDescriptor {
		return LanguageDescriptor{
			ID: id, Aliases: aliases, Extensions: extensions, Analyzer: analyzer,
			Capabilities: LanguageCapabilities{SourceAnalysis: true},
		}
	}
	planned := func(id string, extensions []string) LanguageDescriptor {
		return LanguageDescriptor{ID: id, Extensions: extensions}
	}

	goLanguage := analyzable("go", AnalyzerGo, []string{".go"}, "golang")
	csharp := analyzable("csharp", AnalyzerCSharp, []string{".cs"}, "cs", "c#")
	vbnet := analyzable("vbnet", AnalyzerVBNet, []string{".vb"}, "vb.net", "visual-basic-dotnet")
	vbnet.Capabilities.CaseInsensitive = true
	python := analyzable("python", AnalyzerPython, []string{".py", ".pyw"}, "py", "python3")
	python.ShebangInterpreters = []string{"python", "python3", "python3.11", "python3.12", "python3.13"}
	asp := analyzable("classic-asp", AnalyzerClassicASP, []string{".asp"}, "asp", "active-server-pages")
	asp.Capabilities.Composite = true
	cLanguage := analyzable("c", AnalyzerC, []string{".c"})
	cLanguage.AmbiguousExtensions = []string{".h"}
	cppLanguage := analyzable("cpp", AnalyzerCPP, []string{".cc", ".cpp", ".cxx", ".hh", ".hpp", ".hxx"}, "c++")
	cppLanguage.AmbiguousExtensions = []string{".h"}
	javaLanguage := analyzable("java", AnalyzerJava, []string{".java"})
	kotlinLanguage := analyzable("kotlin", AnalyzerKotlin, []string{".kt", ".kts"})
	javascriptLanguage := analyzable("javascript", AnalyzerJavaScript, []string{".js", ".mjs", ".cjs", ".jsx"}, "js")
	javascriptLanguage.ShebangInterpreters = []string{"node", "nodejs"}
	typescriptLanguage := analyzable("typescript", AnalyzerTypeScript, []string{".ts", ".tsx", ".mts", ".cts"}, "ts")
	typescriptLanguage.CompoundSuffixes = []string{".d.ts"}
	rustLanguage := analyzable("rust", AnalyzerRust, []string{".rs"})
	pascalLanguage := analyzable("pascal", AnalyzerPascal, []string{".pp"})
	pascalLanguage.AmbiguousExtensions = []string{".pas", ".inc"}
	pascalLanguage.Capabilities.CaseInsensitive = true
	delphiLanguage := analyzable("delphi", AnalyzerDelphi, []string{".dpr", ".dpk"})
	delphiLanguage.AmbiguousExtensions = []string{".pas", ".inc"}
	delphiLanguage.Capabilities.CaseInsensitive = true
	phpLanguage := analyzable("php", AnalyzerPHP, []string{".php", ".phtml"})
	phpLanguage.AmbiguousExtensions = []string{".inc"}
	phpLanguage.ShebangInterpreters = []string{"php"}
	rubyLanguage := analyzable("ruby", AnalyzerRuby, []string{".rb"})
	rubyLanguage.ShebangInterpreters = []string{"ruby"}
	swiftLanguage := analyzable("swift", AnalyzerSwift, []string{".swift"})
	vb6Language := analyzable("vb6", AnalyzerVB6, nil)
	vb6Language.AmbiguousExtensions = []string{".bas"}
	vb6Language.Capabilities.CaseInsensitive = true
	vbaLanguage := analyzable("vba", AnalyzerVBA, nil)
	vbaLanguage.AmbiguousExtensions = []string{".bas"}
	vbaLanguage.Capabilities.CaseInsensitive = true
	vbScriptLanguage := analyzable("vbscript", AnalyzerVBScript, []string{".vbs"})
	vbScriptLanguage.Capabilities.CaseInsensitive = true
	qbasicLanguage := analyzable("qbasic", AnalyzerQBasic, nil, "quickbasic")
	qbasicLanguage.AmbiguousExtensions = []string{".bas"}
	qbasicLanguage.Capabilities.CaseInsensitive = true
	classicBasicLanguage := analyzable("classic-basic", AnalyzerClassicBasic, nil)
	classicBasicLanguage.Capabilities.CaseInsensitive = true
	freeBasicLanguage := analyzable("freebasic", AnalyzerFreeBasic, []string{".bi"})
	freeBasicLanguage.Capabilities.CaseInsensitive = true
	pureBasicLanguage := analyzable("purebasic", AnalyzerPureBasic, []string{".pb", ".pbi"})
	pureBasicLanguage.Capabilities.CaseInsensitive = true
	fsharpLanguage := analyzable("fsharp", AnalyzerFSharp, []string{".fs", ".fsi", ".fsx"})
	cppcliLanguage := analyzable("cpp-cli", AnalyzerCPPCLI, nil)
	jscriptNetLanguage := analyzable("jscript-net", AnalyzerJScriptNet, nil)
	cilLanguage := analyzable("cil", AnalyzerCIL, []string{".il"})
	powerShellLanguage := analyzable("powershell", AnalyzerPowerShell, []string{".ps1", ".psm1", ".psd1"}, "pwsh")
	powerShellLanguage.ShebangInterpreters = []string{"pwsh", "powershell"}
	webFormsLanguage := analyzable("aspnet-webforms", AnalyzerASPNetWebForms, []string{".aspx", ".ascx", ".master"})
	webFormsLanguage.Capabilities.Composite = true
	razorLanguage := analyzable("razor", AnalyzerRazor, []string{".cshtml"})
	razorLanguage.Capabilities.Composite = true
	blazorLanguage := analyzable("blazor", AnalyzerBlazor, []string{".razor"})
	blazorLanguage.Capabilities.Composite = true
	xamlLanguage := analyzable("xaml", AnalyzerXAML, []string{".xaml"})
	mql4Language := analyzable("mql4", AnalyzerMQL4, []string{".mq4"})
	mql4Language.AmbiguousExtensions = []string{".mqh"}
	mql5Language := analyzable("mql5", AnalyzerMQL5, []string{".mq5"})
	mql5Language.AmbiguousExtensions = []string{".mqh"}
	objectiveCLanguage := analyzable("objective-c", AnalyzerObjectiveC, nil, "objc")
	objectiveCLanguage.AmbiguousExtensions = []string{".h", ".m"}
	objectiveCPPLanguage := analyzable("objective-cpp", AnalyzerObjectiveCPP, []string{".mm"}, "objc++")
	objectiveCPPLanguage.AmbiguousExtensions = []string{".h"}
	dartLanguage := analyzable("dart", AnalyzerDart, []string{".dart"})
	dLanguage := analyzable("d", AnalyzerD, []string{".d"})
	zigLanguage := analyzable("zig", AnalyzerZig, []string{".zig"})
	nimLanguage := analyzable("nim", AnalyzerNim, []string{".nim"})
	solidityLanguage := analyzable("solidity", AnalyzerSolidity, []string{".sol"})
	apexLanguage := analyzable("apex", AnalyzerApex, []string{".cls"})
	alLanguage := analyzable("al", AnalyzerAL, []string{".al"})
	alLanguage.Capabilities.CaseInsensitive = true
	arduinoLanguage := analyzable("arduino", AnalyzerArduino, []string{".ino"})
	perlLanguage := analyzable("perl", AnalyzerPerl, []string{".pl", ".pm"})
	perlLanguage.ShebangInterpreters = []string{"perl"}
	luaLanguage := analyzable("lua", AnalyzerLua, []string{".lua"})
	luaLanguage.ShebangInterpreters = []string{"lua"}
	luauLanguage := analyzable("luau", AnalyzerLuau, []string{".luau"})
	elixirLanguage := analyzable("elixir", AnalyzerElixir, []string{".ex", ".exs"})
	erlangLanguage := analyzable("erlang", AnalyzerErlang, []string{".erl", ".hrl"})
	gleamLanguage := analyzable("gleam", AnalyzerGleam, []string{".gleam"})
	groovyLanguage := analyzable("groovy", AnalyzerGroovy, []string{".groovy"})
	shellLanguage := analyzable("shell", AnalyzerShell, nil, "posix-shell", "sh-posix")
	bashLanguage := analyzable("bash", AnalyzerBash, []string{".sh", ".bash"})
	bashLanguage.ShebangInterpreters = []string{"bash", "sh"}
	tclLanguage := analyzable("tcl", AnalyzerTcl, []string{".tcl"})
	autoHotkeyLanguage := analyzable("autohotkey", AnalyzerAutoHotkey, []string{".ahk"}, "ahk")
	fortranLanguage := analyzable("fortran", AnalyzerFortran, []string{".f", ".f90", ".f95", ".f03", ".f08"})
	fortranLanguage.Capabilities.CaseInsensitive = true
	cobolLanguage := analyzable("cobol", AnalyzerCOBOL, []string{".cob", ".cbl"})
	cobolLanguage.Capabilities.CaseInsensitive = true
	adaLanguage := analyzable("ada", AnalyzerAda, []string{".adb", ".ads"})
	adaLanguage.Capabilities.CaseInsensitive = true
	matlabLanguage := analyzable("matlab", AnalyzerMATLAB, nil)
	matlabLanguage.AmbiguousExtensions = []string{".m"}
	octaveLanguage := analyzable("octave", AnalyzerOctave, nil)
	octaveLanguage.AmbiguousExtensions = []string{".m"}
	juliaLanguage := analyzable("julia", AnalyzerJulia, []string{".jl"})
	rLanguage := analyzable("r", AnalyzerR, []string{".r"})
	haskellLanguage := analyzable("haskell", AnalyzerHaskell, []string{".hs"})
	ocamlLanguage := analyzable("ocaml", AnalyzerOCaml, []string{".ml", ".mli"})
	commonLispLanguage := analyzable("common-lisp", AnalyzerCommonLisp, []string{".lisp", ".lsp"})
	clojureLanguage := analyzable("clojure", AnalyzerClojure, []string{".clj", ".cljs", ".cljc"})
	emacsLispLanguage := analyzable("emacs-lisp", AnalyzerEmacsLisp, []string{".el"})
	assemblyLanguage := analyzable("assembly", AnalyzerAssembly, []string{".asm", ".s"})
	sqlLanguage := analyzable("sql", AnalyzerSQL, []string{".sql"})
	plsqlLanguage := analyzable("plsql", AnalyzerPLSQL, []string{".pls", ".pkb", ".pks"})
	plsqlLanguage.Capabilities.CaseInsensitive = true
	graphqlLanguage := analyzable("graphql", AnalyzerGraphQL, []string{".graphql", ".gql"})
	terraformLanguage := analyzable("terraform", AnalyzerTerraform, []string{".tf", ".tfvars", ".hcl"}, "hcl")
	nixLanguage := analyzable("nix", AnalyzerNix, []string{".nix"})
	protoLanguage := analyzable("proto", AnalyzerProto, []string{".proto"}, "protobuf")
	vhdlLanguage := analyzable("vhdl", AnalyzerVHDL, []string{".vhd", ".vhdl"})
	vhdlLanguage.Capabilities.CaseInsensitive = true
	verilogLanguage := analyzable("verilog", AnalyzerVerilog, []string{".v"})
	systemVerilogLanguage := analyzable("systemverilog", AnalyzerSystemVerilog, []string{".sv", ".svh"})
	htmlLanguage := analyzable("html", AnalyzerHTML, []string{".html", ".htm"})
	xmlLanguage := analyzable("xml", AnalyzerXML, []string{".xml"})
	cssLanguage := analyzable("css", AnalyzerCSS, []string{".css"})
	scssLanguage := analyzable("scss", AnalyzerSCSS, []string{".scss"})
	sassLanguage := analyzable("sass", AnalyzerSass, []string{".sass"})
	lessLanguage := analyzable("less", AnalyzerLess, []string{".less"})
	jsonLanguage := analyzable("json", AnalyzerJSON, []string{".json"})
	yamlLanguage := analyzable("yaml", AnalyzerYAML, []string{".yaml", ".yml"})
	tomlLanguage := analyzable("toml", AnalyzerTOML, []string{".toml"})
	markdownLanguage := analyzable("markdown", AnalyzerMarkdown, []string{".md", ".markdown"})
	openAPILanguage := analyzable("openapi", AnalyzerOpenAPI, nil)
	ansibleYAMLLanguage := analyzable("ansible-yaml", AnalyzerAnsibleYAML, nil)
	vueLanguage := analyzable("vue", AnalyzerVue, []string{".vue"})
	vueLanguage.Capabilities.Composite = true
	svelteLanguage := analyzable("svelte", AnalyzerSvelte, []string{".svelte"})
	svelteLanguage.Capabilities.Composite = true
	astroLanguage := analyzable("astro", AnalyzerAstro, []string{".astro"})
	astroLanguage.Capabilities.Composite = true
	phpHTMLLanguage := analyzable("php-html", AnalyzerPHPHTML, nil)
	phpHTMLLanguage.Capabilities.Composite = true
	jspLanguage := analyzable("jsp", AnalyzerJSP, []string{".jsp"})
	jspLanguage.Capabilities.Composite = true
	jinjaLanguage := analyzable("jinja", AnalyzerJinja, []string{".jinja", ".j2"}, "jinja2")
	jinjaLanguage.Capabilities.Composite = true
	twigLanguage := analyzable("twig", AnalyzerTwig, []string{".twig"})
	twigLanguage.Capabilities.Composite = true
	bladeLanguage := analyzable("blade", AnalyzerBlade, nil)
	bladeLanguage.CompoundSuffixes = []string{".blade.php"}
	bladeLanguage.Capabilities.Composite = true
	ejsLanguage := analyzable("ejs", AnalyzerEJS, []string{".ejs"})
	ejsLanguage.Capabilities.Composite = true

	return []LanguageDescriptor{
		goLanguage, csharp, vbnet, python, asp, cLanguage, cppLanguage,
		objectiveCLanguage,
		objectiveCPPLanguage,
		matlabLanguage,
		octaveLanguage,
		javascriptLanguage, typescriptLanguage, rustLanguage,
		pascalLanguage, delphiLanguage, phpLanguage,
		mql4Language,
		mql5Language,
		vbaLanguage, vb6Language, qbasicLanguage, razorLanguage, blazorLanguage,
		vueLanguage,
		javaLanguage, kotlinLanguage,
		swiftLanguage, rubyLanguage,
		perlLanguage,
		bashLanguage,
		powerShellLanguage,
		luaLanguage,
		rLanguage,
		zigLanguage,
		dartLanguage,
		dLanguage,
		nimLanguage,
		planned("flow", nil),
		luauLanguage,
		gleamLanguage,
		fsharpLanguage, cppcliLanguage, jscriptNetLanguage, cilLanguage, vbScriptLanguage, classicBasicLanguage, freeBasicLanguage, pureBasicLanguage,
		juliaLanguage,
		ocamlLanguage,
		commonLispLanguage,
		clojureLanguage,
		emacsLispLanguage,
		shellLanguage,
		tclLanguage,
		autoHotkeyLanguage,
		assemblyLanguage,
		arduinoLanguage,
		fortranLanguage,
		cobolLanguage,
		adaLanguage,
		haskellLanguage,
		erlangLanguage,
		elixirLanguage,
		{ID: "scala", Extensions: []string{".scala", ".sc"}},
		groovyLanguage,
		sqlLanguage,
		plsqlLanguage,
		graphqlLanguage,
		nixLanguage,
		protoLanguage,
		solidityLanguage,
		apexLanguage,
		alLanguage,
		htmlLanguage,
		cssLanguage,
		scssLanguage,
		sassLanguage,
		lessLanguage,
		jsonLanguage,
		yamlLanguage,
		tomlLanguage,
		xmlLanguage, xamlLanguage,
		markdownLanguage,
		openAPILanguage,
		ansibleYAMLLanguage,
		webFormsLanguage,
		svelteLanguage,
		astroLanguage,
		phpHTMLLanguage,
		jspLanguage,
		jinjaLanguage,
		twigLanguage,
		bladeLanguage,
		ejsLanguage,
		{ID: "dockerfile", ExactBasenames: []string{"Dockerfile"}},
		{ID: "make", ExactBasenames: []string{"Makefile", "GNUmakefile"}},
		terraformLanguage,
		verilogLanguage,
		systemVerilogLanguage,
		vhdlLanguage,
	}
}
