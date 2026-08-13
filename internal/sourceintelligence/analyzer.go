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

func AnalyzerFor(descriptor LanguageDescriptor) (SourceAnalyzer, bool) {
	if !descriptor.Capabilities.SourceAnalysis || descriptor.Analyzer == "" {
		return nil, false
	}
	switch descriptor.Analyzer {
	case AnalyzerGo:
		return GoAnalyzer{}, true
	case AnalyzerCSharp:
		return CSharpAnalyzer{}, true
	case AnalyzerVBNet:
		return VBNetAnalyzer{}, true
	case AnalyzerPython:
		return PythonAnalyzer{}, true
	case AnalyzerClassicASP:
		return ClassicASPAnalyzer{}, true
	default:
		return nil, false
	}
}
