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

	return []LanguageDescriptor{
		goLanguage, csharp, vbnet, python, asp, cLanguage, cppLanguage,
		objectiveCLanguage,
		objectiveCPPLanguage,
		{ID: "matlab", AmbiguousExtensions: []string{".m"}},
		{ID: "octave", AmbiguousExtensions: []string{".m"}},
		javascriptLanguage, typescriptLanguage, rustLanguage,
		pascalLanguage, delphiLanguage, phpLanguage,
		mql4Language,
		mql5Language,
		vbaLanguage, vb6Language, qbasicLanguage, razorLanguage, blazorLanguage,
		{ID: "vue", Extensions: []string{".vue"}},
		javaLanguage, kotlinLanguage,
		swiftLanguage, rubyLanguage,
		{ID: "perl", Extensions: []string{".pl", ".pm"}, ShebangInterpreters: []string{"perl"}},
		{ID: "bash", Extensions: []string{".sh", ".bash"}, ShebangInterpreters: []string{"bash", "sh"}},
		powerShellLanguage,
		{ID: "lua", Extensions: []string{".lua"}, ShebangInterpreters: []string{"lua"}},
		{ID: "r", Extensions: []string{".r"}},
		zigLanguage,
		dartLanguage,
		dLanguage,
		nimLanguage,
		planned("flow", nil),
		planned("luau", []string{".luau"}),
		planned("gleam", []string{".gleam"}),
		fsharpLanguage, cppcliLanguage, jscriptNetLanguage, cilLanguage, vbScriptLanguage, classicBasicLanguage, freeBasicLanguage, pureBasicLanguage,
		planned("julia", []string{".jl"}),
		planned("ocaml", []string{".ml", ".mli"}),
		planned("common-lisp", []string{".lisp", ".lsp"}),
		planned("clojure", []string{".clj", ".cljs", ".cljc"}),
		planned("emacs-lisp", []string{".el"}),
		planned("shell", nil),
		planned("tcl", []string{".tcl"}),
		planned("autohotkey", []string{".ahk"}),
		planned("assembly", []string{".asm", ".s"}),
		arduinoLanguage,
		{ID: "fortran", Extensions: []string{".f", ".f90", ".f95", ".f03", ".f08"}},
		{ID: "cobol", Extensions: []string{".cob", ".cbl"}},
		{ID: "ada", Extensions: []string{".adb", ".ads"}},
		{ID: "haskell", Extensions: []string{".hs"}},
		{ID: "erlang", Extensions: []string{".erl", ".hrl"}},
		{ID: "elixir", Extensions: []string{".ex", ".exs"}},
		{ID: "scala", Extensions: []string{".scala", ".sc"}},
		{ID: "groovy", Extensions: []string{".groovy"}},
		{ID: "sql", Extensions: []string{".sql"}},
		planned("plsql", []string{".pls", ".pkb", ".pks"}),
		planned("graphql", []string{".graphql", ".gql"}),
		planned("nix", []string{".nix"}),
		planned("proto", []string{".proto"}),
		solidityLanguage,
		apexLanguage,
		alLanguage,
		{ID: "html", Extensions: []string{".html", ".htm"}},
		{ID: "css", Extensions: []string{".css"}},
		planned("scss", []string{".scss"}),
		planned("sass", []string{".sass"}),
		planned("less", []string{".less"}),
		{ID: "json", Extensions: []string{".json"}},
		{ID: "yaml", Extensions: []string{".yaml", ".yml"}},
		{ID: "toml", Extensions: []string{".toml"}},
		{ID: "xml", Extensions: []string{".xml"}}, xamlLanguage,
		planned("markdown", []string{".md", ".markdown"}),
		planned("openapi", nil),
		planned("ansible-yaml", nil),
		webFormsLanguage,
		planned("svelte", []string{".svelte"}),
		planned("astro", []string{".astro"}),
		planned("php-html", nil),
		planned("jsp", []string{".jsp"}),
		planned("jinja", []string{".jinja", ".j2"}),
		planned("twig", []string{".twig"}),
		{ID: "blade", CompoundSuffixes: []string{".blade.php"}},
		planned("ejs", []string{".ejs"}),
		{ID: "dockerfile", ExactBasenames: []string{"Dockerfile"}},
		{ID: "make", ExactBasenames: []string{"Makefile", "GNUmakefile"}},
		{ID: "terraform", Extensions: []string{".tf", ".tfvars"}},
		{ID: "verilog", Extensions: []string{".v"}},
		{ID: "systemverilog", Extensions: []string{".sv", ".svh"}},
		{ID: "vhdl", Extensions: []string{".vhd", ".vhdl"}},
	}
}
