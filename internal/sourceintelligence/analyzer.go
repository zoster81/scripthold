package sourceintelligence

import "context"

// AnalyzeOptions are common per-document analyzer controls. Structural
// dependencies share MaxSymbols as their per-file retention ceiling until the
// orchestration layer applies the stricter aggregate/output budgets.
type AnalyzeOptions struct {
	IncludeSignatures bool
	MaxNesting        int
	Limits            SymbolBuilderLimits
}

// StructuralDependencyKind identifies a non-symbol source dependency fact.
type StructuralDependencyKind string

const (
	StructuralDependencyImport  StructuralDependencyKind = "import"
	StructuralDependencyInclude StructuralDependencyKind = "include"
)

// StructuralDependency is a syntax-proven dependency fact. It deliberately does
// not claim resolution to a file, package, module, or project target.
type StructuralDependency struct {
	Kind     StructuralDependencyKind `json:"kind"`
	Value    string                   `json:"value"`
	Alias    string                   `json:"alias,omitempty"`
	Range    Range                    `json:"range"`
	Evidence SymbolEvidence           `json:"evidence"`
}

// StructuralRelation is a syntax-proven declaration relationship. Targets are
// source spellings only; R25 does not claim project/type resolution.
type StructuralRelation struct {
	Kind     string         `json:"kind"`
	Source   string         `json:"source"`
	Target   string         `json:"target"`
	Range    Range          `json:"range"`
	Evidence SymbolEvidence `json:"evidence"`
}

// SourceRegion describes one language/host segment in a composite source file.
type SourceRegion struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Language  string         `json:"language,omitempty"`
	Range     Range          `json:"range"`
	Evidence  SymbolEvidence `json:"evidence"`
	Supported bool           `json:"supported"`
}

// AnalyzerResult combines common symbol/diagnostic coverage with bounded
// structural dependency, declaration-relation, and composite-region facts.
type AnalyzerResult struct {
	Analysis     AnalysisResult         `json:"analysis"`
	Dependencies []StructuralDependency `json:"dependencies,omitempty"`
	Relations    []StructuralRelation   `json:"relations,omitempty"`
	Regions      []SourceRegion         `json:"regions,omitempty"`
}

// SourceAnalyzer is the internal language-neutral analyzer contract. Concrete
// parser/token types remain private to each implementation.
type SourceAnalyzer interface {
	ID() AnalyzerID
	Language() string
	Analyze(context.Context, *SourceDocument, AnalyzeOptions) (AnalyzerResult, error)
}

// AnalyzerFor resolves only analyzers enabled by the current registry descriptor.
// Future-language metadata therefore cannot accidentally activate R27 behavior.
func scannerTokenBudget(text string) int {
	const maxRetainedTokens = 1_000_000
	budget := len(text) + 1
	if budget < 4096 {
		budget = 4096
	}
	if budget > maxRetainedTokens {
		budget = maxRetainedTokens
	}
	return budget
}

var analyzerFactories = map[AnalyzerID]func() SourceAnalyzer{
	AnalyzerGo:             func() SourceAnalyzer { return GoAnalyzer{} },
	AnalyzerCSharp:         func() SourceAnalyzer { return CSharpAnalyzer{} },
	AnalyzerVBNet:          func() SourceAnalyzer { return VBNetAnalyzer{} },
	AnalyzerPython:         func() SourceAnalyzer { return PythonAnalyzer{} },
	AnalyzerClassicASP:     func() SourceAnalyzer { return ClassicASPAnalyzer{} },
	AnalyzerC:              func() SourceAnalyzer { return CAnalyzer{} },
	AnalyzerCPP:            func() SourceAnalyzer { return CPPAnalyzer{} },
	AnalyzerJava:           func() SourceAnalyzer { return JavaAnalyzer{} },
	AnalyzerKotlin:         func() SourceAnalyzer { return KotlinAnalyzer{} },
	AnalyzerJavaScript:     func() SourceAnalyzer { return JavaScriptAnalyzer{} },
	AnalyzerTypeScript:     func() SourceAnalyzer { return TypeScriptAnalyzer{} },
	AnalyzerRust:           func() SourceAnalyzer { return RustAnalyzer{} },
	AnalyzerPHP:            func() SourceAnalyzer { return PHPAnalyzer{} },
	AnalyzerRuby:           func() SourceAnalyzer { return RubyAnalyzer{} },
	AnalyzerSwift:          func() SourceAnalyzer { return SwiftAnalyzer{} },
	AnalyzerPascal:         func() SourceAnalyzer { return PascalAnalyzer{} },
	AnalyzerDelphi:         func() SourceAnalyzer { return DelphiAnalyzer{} },
	AnalyzerVB6:            func() SourceAnalyzer { return VB6Analyzer{} },
	AnalyzerVBA:            func() SourceAnalyzer { return VBAAnalyzer{} },
	AnalyzerVBScript:       func() SourceAnalyzer { return VBScriptAnalyzer{} },
	AnalyzerQBasic:         func() SourceAnalyzer { return QBasicAnalyzer{} },
	AnalyzerClassicBasic:   func() SourceAnalyzer { return ClassicBasicAnalyzer{} },
	AnalyzerFreeBasic:      func() SourceAnalyzer { return FreeBasicAnalyzer{} },
	AnalyzerPureBasic:      func() SourceAnalyzer { return PureBasicAnalyzer{} },
	AnalyzerFSharp:         func() SourceAnalyzer { return FSharpAnalyzer{} },
	AnalyzerCPPCLI:         func() SourceAnalyzer { return CPPCLIAnalyzer{} },
	AnalyzerJScriptNet:     func() SourceAnalyzer { return JScriptNetAnalyzer{} },
	AnalyzerCIL:            func() SourceAnalyzer { return CILAnalyzer{} },
	AnalyzerPowerShell:     func() SourceAnalyzer { return PowerShellAnalyzer{} },
	AnalyzerASPNetWebForms: func() SourceAnalyzer { return ASPNetWebFormsAnalyzer{} },
	AnalyzerRazor:          func() SourceAnalyzer { return RazorAnalyzer{} },
	AnalyzerBlazor:         func() SourceAnalyzer { return BlazorAnalyzer{} },
	AnalyzerXAML:           func() SourceAnalyzer { return XAMLAnalyzer{} },
	AnalyzerMQL4:           func() SourceAnalyzer { return MQL4Analyzer{} },
	AnalyzerMQL5:           func() SourceAnalyzer { return MQL5Analyzer{} },
	AnalyzerObjectiveC:     func() SourceAnalyzer { return ObjectiveCAnalyzer{} },
	AnalyzerObjectiveCPP:   func() SourceAnalyzer { return ObjectiveCPPAnalyzer{} },
	AnalyzerDart:           func() SourceAnalyzer { return DartAnalyzer{} },
	AnalyzerD:              func() SourceAnalyzer { return DAnalyzer{} },
	AnalyzerZig:            func() SourceAnalyzer { return ZigAnalyzer{} },
	AnalyzerNim:            func() SourceAnalyzer { return NimAnalyzer{} },
	AnalyzerSolidity:       func() SourceAnalyzer { return SolidityAnalyzer{} },
	AnalyzerApex:           func() SourceAnalyzer { return ApexAnalyzer{} },
	AnalyzerAL:             func() SourceAnalyzer { return ALAnalyzer{} },
	AnalyzerArduino:        func() SourceAnalyzer { return ArduinoAnalyzer{} },
	AnalyzerPerl:           func() SourceAnalyzer { return PerlAnalyzer{} },
	AnalyzerLua:            func() SourceAnalyzer { return LuaAnalyzer{} },
	AnalyzerLuau:           func() SourceAnalyzer { return LuauAnalyzer{} },
	AnalyzerElixir:         func() SourceAnalyzer { return ElixirAnalyzer{} },
	AnalyzerErlang:         func() SourceAnalyzer { return ErlangAnalyzer{} },
	AnalyzerGleam:          func() SourceAnalyzer { return GleamAnalyzer{} },
	AnalyzerGroovy:         func() SourceAnalyzer { return GroovyAnalyzer{} },
	AnalyzerShell:          func() SourceAnalyzer { return ShellAnalyzer{} },
	AnalyzerBash:           func() SourceAnalyzer { return BashAnalyzer{} },
	AnalyzerTcl:            func() SourceAnalyzer { return TclAnalyzer{} },
	AnalyzerAutoHotkey:     func() SourceAnalyzer { return AutoHotkeyAnalyzer{} },
	AnalyzerFortran:        func() SourceAnalyzer { return FortranAnalyzer{} },
	AnalyzerCOBOL:          func() SourceAnalyzer { return COBOLAnalyzer{} },
	AnalyzerAda:            func() SourceAnalyzer { return AdaAnalyzer{} },
	AnalyzerMATLAB:         func() SourceAnalyzer { return MATLABAnalyzer{} },
	AnalyzerOctave:         func() SourceAnalyzer { return OctaveAnalyzer{} },
	AnalyzerJulia:          func() SourceAnalyzer { return JuliaAnalyzer{} },
	AnalyzerR:              func() SourceAnalyzer { return RAnalyzer{} },
	AnalyzerHaskell:        func() SourceAnalyzer { return HaskellAnalyzer{} },
	AnalyzerOCaml:          func() SourceAnalyzer { return OCamlAnalyzer{} },
	AnalyzerCommonLisp:     func() SourceAnalyzer { return CommonLispAnalyzer{} },
	AnalyzerClojure:        func() SourceAnalyzer { return ClojureAnalyzer{} },
	AnalyzerEmacsLisp:      func() SourceAnalyzer { return EmacsLispAnalyzer{} },
}

func AnalyzerFor(descriptor LanguageDescriptor) (SourceAnalyzer, bool) {
	if !descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer == "" {
		return nil, false
	}
	factory, ok := analyzerFactories[descriptor.Analyzer]
	if !ok {
		return nil, false
	}
	return factory(), true
}
