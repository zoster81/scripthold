package handler

import (
	"encoding/json"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

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
	FileDetails       string   `json:"fileDetails,omitempty"`
	Presentation      string   `json:"presentation,omitempty"`
	MaxSymbols        int      `json:"maxSymbols,omitempty"`
	MaxFiles          int      `json:"maxFiles,omitempty"`
	SymbolID          string   `json:"symbolId,omitempty"`
	SourceFingerprint string   `json:"sourceFingerprint,omitempty"`
	MaxBytes          int      `json:"maxBytes,omitempty"`
	Owner             string   `json:"owner,omitempty"`
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

type SourceOutlineNode struct {
	SymbolID      string                        `json:"symbolId"`
	Name          string                        `json:"name"`
	Kind          sourceintelligence.SymbolKind `json:"kind"`
	QualifiedName string                        `json:"qualifiedName,omitempty"`
	Line          int                           `json:"line"`
	Column        int                           `json:"column"`
	Signature     string                        `json:"signature,omitempty"`
	ParentID      string                        `json:"parentId,omitempty"`
	Children      []*SourceOutlineNode          `json:"children,omitempty"`
}

type SourceOutlineFile struct {
	Path              string               `json:"path"`
	Status            string               `json:"status"`
	Encoding          string               `json:"encoding,omitempty"`
	Language          string               `json:"language,omitempty"`
	SourceFingerprint string               `json:"sourceFingerprint,omitempty"`
	CoverageComplete  bool                 `json:"coverageComplete"`
	Error             string               `json:"error,omitempty"`
	ErrorCode         string               `json:"errorCode,omitempty"`
	Symbols           []*SourceOutlineNode `json:"symbols,omitempty"`
}

// SourceOverviewEntry identifies a leading public declaration; it is not a complete symbol inventory.
type SourceOverviewEntry struct {
	Name string                        `json:"name"`
	Kind sourceintelligence.SymbolKind `json:"kind"`
	Line int                           `json:"line"`
}

// SourceOverviewFile is the file-level map used to select a smaller outline scope.
type SourceOverviewFile struct {
	Path                 string                   `json:"path"`
	Status               string                   `json:"status"`
	Language             string                   `json:"language,omitempty"`
	Encoding             string                   `json:"encoding,omitempty"`
	SourceFingerprint    string                   `json:"sourceFingerprint,omitempty"`
	SourceBytes          int64                    `json:"sourceBytes,omitempty"`
	CoverageComplete     bool                     `json:"coverageComplete"`
	Error                string                   `json:"error,omitempty"`
	ErrorCode            string                   `json:"errorCode,omitempty"`
	DeclarationCounts    []SourceDeclarationCount `json:"declarationCounts,omitempty"`
	EntryPoints          []SourceOverviewEntry    `json:"entryPoints,omitempty"`
	EntryPointsTruncated bool                     `json:"entryPointsTruncated,omitempty"`
}

// SourceConciseFile is the file-level concise projection. Overview and outline
// use relative paths; find keeps the absolute path and fingerprint for safe show.
type SourceConciseFile struct {
	Path              string               `json:"path"`
	Root              string               `json:"root,omitempty"`
	Classes           []string             `json:"classes,omitempty"`
	Types             []string             `json:"types,omitempty"`
	Functions         []string             `json:"functions,omitempty"`
	Owners            []SourceConciseOwner `json:"owners,omitempty"`
	Matches           []SourceConciseMatch `json:"matches,omitempty"`
	Language          string               `json:"language,omitempty"`
	Encoding          string               `json:"encoding,omitempty"`
	SourceFingerprint string               `json:"sourceFingerprint,omitempty"`
	Status            string               `json:"status,omitempty"`
	ErrorCode         string               `json:"errorCode,omitempty"`
	Error             string               `json:"error,omitempty"`
}

// SourceConciseMember is a source-positioned member, never a guessed match.
type SourceConciseMember struct {
	Name      string                        `json:"name"`
	Kind      sourceintelligence.SymbolKind `json:"kind"`
	Line      int                           `json:"line"`
	Signature string                        `json:"signature,omitempty"`
}

// SourceConciseOwner retains unique file/line/qualified owner identity while
// omitting technical selection IDs from the visible outline.
type SourceConciseOwner struct {
	Name          string                        `json:"name"`
	QualifiedName string                        `json:"qualifiedName,omitempty"`
	Kind          sourceintelligence.SymbolKind `json:"kind"`
	Line          int                           `json:"line"`
	Methods       []SourceConciseMember         `json:"methods,omitempty"`
	Fields        []SourceConciseMember         `json:"fields,omitempty"`
}

// SourceConciseMatch retains the exact identifier required for a safe show call.
// The containing file carries its fingerprint, absolute path and decoding facts.
type SourceConciseMatch struct {
	Name      string                        `json:"name"`
	Owner     string                        `json:"owner,omitempty"`
	Kind      sourceintelligence.SymbolKind `json:"kind"`
	Line      int                           `json:"line"`
	Signature string                        `json:"signature,omitempty"`
	SymbolID  string                        `json:"symbolId"`
}

type SourceSymbolsOutput struct {
	Operation                 string                                `json:"operation"`
	CoordinateSystem          string                                `json:"coordinateSystem"`
	Files                     []SourceSymbolsFile                   `json:"files,omitempty"`
	Symbols                   []sourceintelligence.NormalizedSymbol `json:"symbols,omitempty"`
	Presentation              string                                `json:"presentation,omitempty"`
	OutlineFiles              []SourceOutlineFile                   `json:"outlineFiles,omitempty"`
	Digests                   []SourceDigest                        `json:"digests,omitempty"`
	OverviewFiles             []SourceOverviewFile                  `json:"overviewFiles,omitempty"`
	ConciseFiles              []SourceConciseFile                   `json:"-"`
	ConciseAnalysisComplete   bool                                  `json:"-"`
	ConciseSelectionTruncated bool                                  `json:"-"`
	ConciseResultsTruncated   bool                                  `json:"-"`
	Show                      *SourceShow                           `json:"show,omitempty"`
	FilesConsidered           int                                   `json:"filesConsidered"`
	FilesParsed               int                                   `json:"filesParsed"`
	FilesSkipped              int                                   `json:"filesSkipped"`
	SymbolCount               int                                   `json:"symbolCount"`
	CoverageComplete          bool                                  `json:"coverageComplete"`
	Truncated                 bool                                  `json:"truncated,omitempty"`
	Ambiguous                 bool                                  `json:"ambiguous,omitempty"`
}

// MarshalJSON leaves every legacy projection unchanged. Only the explicit
// concise projections suppress counts, hashes, and diagnostic inventories.
func (out SourceSymbolsOutput) MarshalJSON() ([]byte, error) {
	if out.Presentation != "concise" {
		type legacy SourceSymbolsOutput
		return json.Marshal(legacy(out))
	}
	type coverage struct {
		AnalysisComplete   bool `json:"analysisComplete"`
		SelectionTruncated bool `json:"fileSelectionTruncated"`
		ResultsTruncated   bool `json:"resultsTruncated"`
		FilesSkipped       int  `json:"filesSkipped,omitempty"`
	}
	var state *coverage
	if !out.CoverageComplete || out.Truncated || out.FilesSkipped != 0 {
		state = &coverage{
			AnalysisComplete:   out.ConciseAnalysisComplete,
			SelectionTruncated: out.ConciseSelectionTruncated,
			ResultsTruncated:   out.ConciseResultsTruncated,
			FilesSkipped:       out.FilesSkipped,
		}
	}
	return json.Marshal(struct {
		Files     []SourceConciseFile `json:"files"`
		Coverage  *coverage           `json:"coverage,omitempty"`
		Ambiguous bool                `json:"ambiguous,omitempty"`
	}{Files: out.ConciseFiles, Coverage: state, Ambiguous: out.Ambiguous})
}

type sourceFileAnalysis struct {
	file     SourceSymbolsFile
	analysis sourceintelligence.AnalyzerResult
}
