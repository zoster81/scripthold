package handler

import "github.com/zoster81/scripthold/internal/sourceintelligence"

type SourceIndexBindingInput struct {
	Generation  *uint64 `json:"generation,omitempty"`
	Fingerprint string  `json:"fingerprint,omitempty"`
	StalePolicy string  `json:"stalePolicy,omitempty"`
}

type SourcePositionInput struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type SourceSelectorInput struct {
	Kind              string               `json:"kind"`
	Path              string               `json:"path"`
	SymbolID          string               `json:"symbolId,omitempty"`
	Position          *SourcePositionInput `json:"position,omitempty"`
	SourceFingerprint string               `json:"sourceFingerprint"`
}

// SourceQueryInput is the compact R27 public request model. The public JSON
// schema rejects unknown fields; operation-specific legality is validated by
// SourceQuery so the connector catalog does not duplicate three large schemas.
type SourceQueryInput struct {
	Operation        string                   `json:"operation"`
	Paths            []string                 `json:"paths"`
	Query            string                   `json:"query,omitempty"`
	Mode             string                   `json:"mode,omitempty"`
	Match            string                   `json:"match,omitempty"`
	Relation         string                   `json:"relation,omitempty"`
	Subject          *SourceSelectorInput     `json:"subject,omitempty"`
	Target           *SourceSelectorInput     `json:"target,omitempty"`
	Targets          []SourceSelectorInput    `json:"targets,omitempty"`
	BudgetBytes      int                      `json:"budgetBytes,omitempty"`
	BodyPolicy       string                   `json:"bodyPolicy,omitempty"`
	Language         string                   `json:"language,omitempty"`
	Encoding         string                   `json:"encoding,omitempty"`
	Kinds            []string                 `json:"kinds,omitempty"`
	Includes         []string                 `json:"includes,omitempty"`
	Excludes         []string                 `json:"excludes,omitempty"`
	RespectGitignore *bool                    `json:"respectGitignore,omitempty"`
	Evidence         []string                 `json:"evidence,omitempty"`
	MaxFiles         int                      `json:"maxFiles,omitempty"`
	MaxResults       int                      `json:"maxResults,omitempty"`
	MaxNodes         int                      `json:"maxNodes,omitempty"`
	MaxEdges         int                      `json:"maxEdges,omitempty"`
	MaxDepth         int                      `json:"maxDepth,omitempty"`
	MaxItems         int                      `json:"maxItems,omitempty"`
	Index            *SourceIndexBindingInput `json:"index,omitempty"`
}

type SourceQueryCoverage struct {
	FilesConsidered  int  `json:"filesConsidered"`
	FilesParsed      int  `json:"filesParsed"`
	FilesSkipped     int  `json:"filesSkipped"`
	CoverageComplete bool `json:"coverageComplete"`
	Truncated        bool `json:"truncated,omitempty"`
}

type SourceSearchMatch struct {
	Path              string                            `json:"path"`
	Language          string                            `json:"language,omitempty"`
	Range             sourceintelligence.Range          `json:"range"`
	SymbolID          string                            `json:"symbolId,omitempty"`
	SourceFingerprint string                            `json:"sourceFingerprint"`
	Evidence          sourceintelligence.SymbolEvidence `json:"evidence"`
}

type SourceSearchResult struct {
	Matches []SourceSearchMatch `json:"matches,omitempty"`
}

type SourceRelationsResult struct {
	Relation  sourceintelligence.RelationKind     `json:"relation"`
	Relations []sourceintelligence.RelationRecord `json:"relations,omitempty"`
}

type SourceContextResult struct {
	Items       []sourceintelligence.ContextItem `json:"items,omitempty"`
	UsedBytes   int                              `json:"usedBytes"`
	BudgetBytes int                              `json:"budgetBytes"`
}

type SourceQueryOutput struct {
	Operation        string                           `json:"operation"`
	CoordinateSystem string                           `json:"coordinateSystem"`
	Index            sourceintelligence.IndexEvidence `json:"index"`
	Search           *SourceSearchResult              `json:"search,omitempty"`
	Relations        *SourceRelationsResult           `json:"relations,omitempty"`
	Context          *SourceContextResult             `json:"context,omitempty"`
	Coverage         SourceQueryCoverage              `json:"coverage"`
}
