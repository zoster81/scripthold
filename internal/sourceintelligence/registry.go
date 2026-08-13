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
	AnalyzerGo         AnalyzerID = "go-ast"
	AnalyzerCSharp     AnalyzerID = "csharp-native"
	AnalyzerVBNet      AnalyzerID = "vbnet-native"
	AnalyzerPython     AnalyzerID = "python-native"
	AnalyzerClassicASP AnalyzerID = "classic-asp-composite"
)

var knownAnalyzerIDs = map[AnalyzerID]struct{}{
	AnalyzerGo:         {},
	AnalyzerCSharp:     {},
	AnalyzerVBNet:      {},
	AnalyzerPython:     {},
	AnalyzerClassicASP: {},
}

var canonicalLanguageIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// LanguageCapabilities describe only capabilities actually enabled in the
// current milestone. Future registry entries can exist with all capabilities false.
type LanguageCapabilities struct {
	SourceAnalysis  bool
	Composite       bool
	CaseInsensitive bool
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
	return NewLanguageRegistry(defaultLanguageDescriptors())
}

func defaultLanguageDescriptors() []LanguageDescriptor {
	analyzable := func(id string, analyzer AnalyzerID, extensions []string, aliases ...string) LanguageDescriptor {
		return LanguageDescriptor{
			ID: id, Aliases: aliases, Extensions: extensions, Analyzer: analyzer,
			Capabilities: LanguageCapabilities{SourceAnalysis: true},
		}
	}

	goLanguage := analyzable("go", AnalyzerGo, []string{".go"}, "golang")
	csharp := analyzable("csharp", AnalyzerCSharp, []string{".cs"}, "cs", "c#")
	vbnet := analyzable("vbnet", AnalyzerVBNet, []string{".vb"}, "vb.net", "visual-basic-dotnet")
	vbnet.Capabilities.CaseInsensitive = true
	python := analyzable("python", AnalyzerPython, []string{".py", ".pyw"}, "py", "python3")
	python.ShebangInterpreters = []string{"python", "python3", "python3.11", "python3.12", "python3.13"}
	asp := analyzable("classic-asp", AnalyzerClassicASP, []string{".asp"}, "asp", "active-server-pages")
	asp.Capabilities.Composite = true

	return []LanguageDescriptor{
		goLanguage, csharp, vbnet, python, asp,
		{ID: "c", Extensions: []string{".c"}, AmbiguousExtensions: []string{".h"}},
		{ID: "cpp", Aliases: []string{"c++"}, Extensions: []string{".cc", ".cpp", ".cxx", ".hh", ".hpp", ".hxx"}, AmbiguousExtensions: []string{".h"}},
		{ID: "objective-c", Extensions: []string{".mm"}, AmbiguousExtensions: []string{".h", ".m"}},
		{ID: "matlab", AmbiguousExtensions: []string{".m"}},
		{ID: "javascript", Aliases: []string{"js"}, Extensions: []string{".js", ".mjs", ".cjs", ".jsx"}, ShebangInterpreters: []string{"node", "nodejs"}},
		{ID: "typescript", Aliases: []string{"ts"}, Extensions: []string{".ts", ".tsx", ".mts", ".cts"}, CompoundSuffixes: []string{".d.ts"}},
		{ID: "rust", Extensions: []string{".rs"}},
		{ID: "pascal", Extensions: []string{".pp"}, AmbiguousExtensions: []string{".pas"}},
		{ID: "delphi", Extensions: []string{".dpr", ".dpk"}, AmbiguousExtensions: []string{".pas", ".inc"}},
		{ID: "php", Extensions: []string{".php", ".phtml"}, AmbiguousExtensions: []string{".inc"}, ShebangInterpreters: []string{"php"}},
		{ID: "mql4", Extensions: []string{".mq4"}, AmbiguousExtensions: []string{".mqh"}},
		{ID: "mql5", Extensions: []string{".mq5"}, AmbiguousExtensions: []string{".mqh"}},
		{ID: "vba", AmbiguousExtensions: []string{".bas"}},
		{ID: "vb6", AmbiguousExtensions: []string{".bas"}},
		{ID: "qbasic", Aliases: []string{"quickbasic"}, AmbiguousExtensions: []string{".bas"}},
		{ID: "razor", Extensions: []string{".cshtml", ".razor"}},
		{ID: "vue", Extensions: []string{".vue"}},
		{ID: "java", Extensions: []string{".java"}},
		{ID: "kotlin", Extensions: []string{".kt", ".kts"}},
		{ID: "swift", Extensions: []string{".swift"}},
		{ID: "ruby", Extensions: []string{".rb"}, ShebangInterpreters: []string{"ruby"}},
		{ID: "perl", Extensions: []string{".pl", ".pm"}, ShebangInterpreters: []string{"perl"}},
		{ID: "bash", Extensions: []string{".sh", ".bash"}, ShebangInterpreters: []string{"bash", "sh"}},
		{ID: "powershell", Aliases: []string{"pwsh"}, Extensions: []string{".ps1", ".psm1", ".psd1"}, ShebangInterpreters: []string{"pwsh", "powershell"}},
		{ID: "lua", Extensions: []string{".lua"}, ShebangInterpreters: []string{"lua"}},
		{ID: "r", Extensions: []string{".r"}},
		{ID: "zig", Extensions: []string{".zig"}},
		{ID: "fortran", Extensions: []string{".f", ".f90", ".f95", ".f03", ".f08"}},
		{ID: "cobol", Extensions: []string{".cob", ".cbl"}},
		{ID: "ada", Extensions: []string{".adb", ".ads"}},
		{ID: "haskell", Extensions: []string{".hs"}},
		{ID: "erlang", Extensions: []string{".erl", ".hrl"}},
		{ID: "elixir", Extensions: []string{".ex", ".exs"}},
		{ID: "scala", Extensions: []string{".scala", ".sc"}},
		{ID: "groovy", Extensions: []string{".groovy"}},
		{ID: "sql", Extensions: []string{".sql"}},
		{ID: "html", Extensions: []string{".html", ".htm"}},
		{ID: "css", Extensions: []string{".css"}},
		{ID: "json", Extensions: []string{".json"}},
		{ID: "yaml", Extensions: []string{".yaml", ".yml"}},
		{ID: "toml", Extensions: []string{".toml"}},
		{ID: "xml", Extensions: []string{".xml"}},
		{ID: "dockerfile", ExactBasenames: []string{"Dockerfile"}},
		{ID: "make", ExactBasenames: []string{"Makefile", "GNUmakefile"}},
		{ID: "terraform", Extensions: []string{".tf", ".tfvars"}},
		{ID: "verilog", Extensions: []string{".v"}},
		{ID: "systemverilog", Extensions: []string{".sv", ".svh"}},
		{ID: "vhdl", Extensions: []string{".vhd", ".vhdl"}},
	}
}
