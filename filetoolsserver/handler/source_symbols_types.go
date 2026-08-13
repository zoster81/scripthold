package handler

import "github.com/zoster81/scripthold/internal/sourceintelligence"

const sourceCoordinateSystem = "unicode-scalar-1-based-half-open"

// SourceSymbolsInput is the typed internal superset. The public MCP schema is a
// strict oneOf added at integration time so operation-illegal fields are rejected
// before this handler runs.
type SourceSymbolsInput struct {
	Operation         string   `json:"operation"`
	Paths             []string `json:"paths,omitempty"`
	Path              string   `json:"path,omitempty"`
	Query             string   `json:"query,omitempty"`
	Match             string   `json:"match,omitempty"`
	Language          string   `json:"language,omitempty"`
	Encoding          string   `json:"encoding,omitempty"`
	Kinds             []string `json:"kinds,omitempty"`
	Includes          []string `json:"includes,omitempty"`
	Excludes          []string `json:"excludes,omitempty"`
	RespectGitignore  *bool    `json:"respectGitignore,omitempty"`
	IncludeSignatures bool     `json:"includeSignatures,omitempty"`
	MaxSymbols        int      `json:"maxSymbols,omitempty"`
	MaxFiles          int      `json:"maxFiles,omitempty"`
	SymbolID          string   `json:"symbolId,omitempty"`
	SourceFingerprint string   `json:"sourceFingerprint,omitempty"`
	MaxBytes          int      `json:"maxBytes,omitempty"`
}

type SourceSymbolsFile struct {
	Path              string                             `json:"path"`
	Status            string                             `json:"status"`
	Encoding          string                             `json:"encoding,omitempty"`
	SourceFingerprint string                             `json:"sourceFingerprint,omitempty"`
	SourceBytes       int64                              `json:"sourceBytes,omitempty"`
	Language          string                             `json:"language,omitempty"`
	Analyzer          string                             `json:"analyzer,omitempty"`
	Detection         sourceintelligence.DetectionResult `json:"detection"`
	CoverageComplete  bool                               `json:"coverageComplete"`
	Error             string                             `json:"error,omitempty"`
	ErrorCode         string                             `json:"errorCode,omitempty"`
}

type SourceDeclarationCount struct {
	Kind  sourceintelligence.SymbolKind `json:"kind"`
	Count int                           `json:"count"`
}

type SourceDigest struct {
	Path              string                                    `json:"path"`
	Language          string                                    `json:"language"`
	Analyzer          string                                    `json:"analyzer,omitempty"`
	SourceBytes       int64                                     `json:"sourceBytes"`
	SourceFingerprint string                                    `json:"sourceFingerprint"`
	DeclarationCounts []SourceDeclarationCount                  `json:"declarationCounts"`
	Dependencies      []sourceintelligence.StructuralDependency `json:"dependencies,omitempty"`
	Relations         []sourceintelligence.StructuralRelation   `json:"relations,omitempty"`
	Regions           []sourceintelligence.SourceRegion         `json:"regions,omitempty"`
	CoverageComplete  bool                                      `json:"coverageComplete"`
}

type SourceShow struct {
	Path              string                   `json:"path"`
	SymbolID          string                   `json:"symbolId"`
	SourceFingerprint string                   `json:"sourceFingerprint"`
	Language          string                   `json:"language"`
	Encoding          string                   `json:"encoding"`
	Range             sourceintelligence.Range `json:"range"`
	Text              string                   `json:"text"`
}

type SourceSymbolsOutput struct {
	Operation        string                                `json:"operation"`
	CoordinateSystem string                                `json:"coordinateSystem"`
	Files            []SourceSymbolsFile                   `json:"files,omitempty"`
	Symbols          []sourceintelligence.NormalizedSymbol `json:"symbols,omitempty"`
	Digests          []SourceDigest                        `json:"digests,omitempty"`
	Show             *SourceShow                           `json:"show,omitempty"`
	FilesConsidered  int                                   `json:"filesConsidered"`
	FilesParsed      int                                   `json:"filesParsed"`
	FilesSkipped     int                                   `json:"filesSkipped"`
	SymbolCount      int                                   `json:"symbolCount"`
	CoverageComplete bool                                  `json:"coverageComplete"`
	Truncated        bool                                  `json:"truncated,omitempty"`
	Ambiguous        bool                                  `json:"ambiguous,omitempty"`
}

type sourceFileAnalysis struct {
	file     SourceSymbolsFile
	analysis sourceintelligence.AnalyzerResult
}
