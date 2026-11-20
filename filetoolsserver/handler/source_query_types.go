package handler

import (
	"encoding/json"
	"fmt"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

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

// SourceQueryInput is the compact public source-query request model. The public JSON
// schema rejects unknown fields; operation-specific legality is validated by
// SourceQuery so the connector catalog does not duplicate three large schemas.
type SourceQueryInput struct {
	Operation        string                   `json:"operation"`
	Paths            []string                 `json:"paths"`
	Query            string                   `json:"query,omitempty"`
	Mode             string                   `json:"mode,omitempty"`
	Match            string                   `json:"match,omitempty"`
	Cursor           string                   `json:"cursor,omitempty"`
	Presentation     string                   `json:"presentation,omitempty"`
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
	FilesConsidered        int  `json:"filesConsidered"`
	FilesParsed            int  `json:"filesParsed"`
	FilesSkipped           int  `json:"filesSkipped"`
	FileSelectionTruncated bool `json:"fileSelectionTruncated"`
	AnalysisComplete       bool `json:"analysisComplete"`
	ResultsTruncated       bool `json:"resultsTruncated"`
	CoverageComplete       bool `json:"coverageComplete"`
	Truncated              bool `json:"truncated,omitempty"`
}

type SourceSearchMatch struct {
	Path              string                            `json:"path"`
	Language          string                            `json:"language,omitempty"`
	Name              string                            `json:"name,omitempty"`
	Kind              sourceintelligence.SymbolKind     `json:"kind,omitempty"`
	QualifiedName     string                            `json:"qualifiedName,omitempty"`
	Range             sourceintelligence.Range          `json:"range"`
	SymbolID          string                            `json:"symbolId,omitempty"`
	SourceFingerprint string                            `json:"sourceFingerprint"`
	Evidence          sourceintelligence.SymbolEvidence `json:"evidence"`
}

type SourceSearchResult struct {
	Matches    []SourceSearchMatch `json:"matches,omitempty"`
	NextCursor string              `json:"nextCursor,omitempty"`
}

type SourceRelationsResult struct {
	Relation      sourceintelligence.RelationKind     `json:"relation"`
	EvidenceScope string                              `json:"evidenceScope"`
	Relations     []sourceintelligence.RelationRecord `json:"relations,omitempty"`
}

type SourceContextResult struct {
	Items       []sourceintelligence.ContextItem `json:"items,omitempty"`
	UsedBytes   int                              `json:"usedBytes"`
	BudgetBytes int                              `json:"budgetBytes"`
}

type SourceQueryOutput struct {
	Presentation      string                           `json:"-"`
	RequestedEncoding string                           `json:"-"`
	Operation         string                           `json:"operation"`
	CoordinateSystem  string                           `json:"coordinateSystem"`
	Index             sourceintelligence.IndexEvidence `json:"index"`
	Search            *SourceSearchResult              `json:"search,omitempty"`
	Relations         *SourceRelationsResult           `json:"relations,omitempty"`
	Context           *SourceContextResult             `json:"context,omitempty"`
	Coverage          SourceQueryCoverage              `json:"coverage"`
}

// SourceSearchConciseMatch includes evidence and an exact selector only when
// independently available; other references remain source-site observations.
type SourceSearchConciseMatch struct {
	Symbol   string                            `json:"symbol,omitempty"`
	Kind     sourceintelligence.SymbolKind     `json:"kind,omitempty"`
	Range    sourceintelligence.Range          `json:"range"`
	SymbolID string                            `json:"symbolId,omitempty"`
	Evidence sourceintelligence.SymbolEvidence `json:"evidence"`
}

type SourceSearchConciseFile struct {
	Path              string                     `json:"path"`
	Language          string                     `json:"language,omitempty"`
	Encoding          string                     `json:"encoding,omitempty"`
	SourceFingerprint string                     `json:"sourceFingerprint"`
	Matches           []SourceSearchConciseMatch `json:"matches"`
}

// SourceConciseRelationEdge preserves verified endpoints and reference-site
// locations. Kind is emitted only when it differs from the requested relation.
type SourceConciseRelationEdge struct {
	Kind       sourceintelligence.RelationKind    `json:"kind,omitempty"`
	Source     sourceintelligence.RelationEntity  `json:"source"`
	Target     sourceintelligence.RelationEntity  `json:"target"`
	Evidence   sourceintelligence.SymbolEvidence  `json:"evidence"`
	Resolution sourceintelligence.ResolutionState `json:"resolution"`
}

// Relations do not infer links: only already-indexed, bounded records are projected.
func (out SourceQueryOutput) marshalConciseRelations() ([]byte, error) {
	if out.Relations == nil {
		return nil, fmt.Errorf("concise relations require indexed records")
	}
	edges := make([]SourceConciseRelationEdge, 0, len(out.Relations.Relations))
	for _, record := range out.Relations.Relations {
		edge := SourceConciseRelationEdge{
			Source: record.Source, Target: record.Target,
			Evidence: record.Evidence, Resolution: record.Resolution,
		}
		if record.Kind != out.Relations.Relation {
			edge.Kind = record.Kind
		}
		edges = append(edges, edge)
	}
	coverage := struct {
		AnalysisComplete       bool `json:"analysisComplete"`
		FileSelectionTruncated bool `json:"fileSelectionTruncated"`
		ResultsTruncated       bool `json:"resultsTruncated"`
		FilesSkipped           int  `json:"filesSkipped,omitempty"`
	}{
		AnalysisComplete:       out.Coverage.AnalysisComplete,
		FileSelectionTruncated: out.Coverage.FileSelectionTruncated,
		ResultsTruncated:       out.Coverage.ResultsTruncated,
		FilesSkipped:           out.Coverage.FilesSkipped,
	}
	compact := struct {
		Relations struct {
			Relation      sourceintelligence.RelationKind `json:"relation"`
			EvidenceScope string                          `json:"evidenceScope"`
			Edges         []SourceConciseRelationEdge     `json:"edges"`
		} `json:"relations"`
		Coverage any                               `json:"coverage"`
		Index    *sourceintelligence.IndexEvidence `json:"index,omitempty"`
	}{Coverage: coverage}
	compact.Relations.Relation = out.Relations.Relation
	compact.Relations.EvidenceScope = out.Relations.EvidenceScope
	compact.Relations.Edges = edges
	if out.Index.Staleness != sourceintelligence.IndexCurrent {
		compact.Index = &out.Index
	}
	return json.Marshal(compact)
}

// SourceConciseContextItem keeps the original candidate order and all source
// verification facts. Priority is not repeated: order and reason express the
// selected context plan without exposing an internal ranking.
type SourceConciseContextItem struct {
	Entity         sourceintelligence.RelationEntity        `json:"entity"`
	Reason         sourceintelligence.ContextReason         `json:"reason"`
	Representation sourceintelligence.ContextRepresentation `json:"representation"`
	Code           string                                   `json:"code"`
	Evidence       sourceintelligence.SymbolEvidence        `json:"evidence"`
	Resolution     sourceintelligence.ResolutionState       `json:"resolution"`
}

// marshalConciseContext projects only already fingerprint-verified decoded
// excerpts; never slices or reorders source text to reduce the response.
func (out SourceQueryOutput) marshalConciseContext() ([]byte, error) {
	if out.Context == nil {
		return nil, fmt.Errorf("concise context requires verified excerpts")
	}
	items := make([]SourceConciseContextItem, 0, len(out.Context.Items))
	for _, item := range out.Context.Items {
		items = append(items, SourceConciseContextItem{
			Entity: item.Entity, Reason: item.Reason,
			Representation: item.Representation, Code: item.Text,
			Evidence: item.Evidence, Resolution: item.Resolution,
		})
	}
	coverage := struct {
		AnalysisComplete       bool `json:"analysisComplete"`
		FileSelectionTruncated bool `json:"fileSelectionTruncated"`
		ResultsTruncated       bool `json:"resultsTruncated"`
		FilesSkipped           int  `json:"filesSkipped,omitempty"`
	}{
		AnalysisComplete:       out.Coverage.AnalysisComplete,
		FileSelectionTruncated: out.Coverage.FileSelectionTruncated,
		ResultsTruncated:       out.Coverage.ResultsTruncated,
		FilesSkipped:           out.Coverage.FilesSkipped,
	}
	compact := struct {
		Context struct {
			Items       []SourceConciseContextItem `json:"items"`
			UsedBytes   int                        `json:"usedBytes"`
			BudgetBytes int                        `json:"budgetBytes"`
		} `json:"context"`
		Coverage any                               `json:"coverage"`
		Index    *sourceintelligence.IndexEvidence `json:"index,omitempty"`
	}{Coverage: coverage}
	compact.Context.Items = items
	compact.Context.UsedBytes = out.Context.UsedBytes
	compact.Context.BudgetBytes = out.Context.BudgetBytes
	if out.Index.Staleness != sourceintelligence.IndexCurrent {
		compact.Index = &out.Index
	}
	return json.Marshal(compact)
}

// MarshalJSON keeps default results byte-schema-compatible. The explicit
// concise search representation is derived from the same indexed matches,
// without changing ordering, analysis, cursor binding, or source identities.
func (out SourceQueryOutput) MarshalJSON() ([]byte, error) {
	if out.Presentation != "concise" {
		type legacy SourceQueryOutput
		return json.Marshal(legacy(out))
	}
	if out.Operation == "relations" {
		return out.marshalConciseRelations()
	}
	if out.Operation == "context" {
		return out.marshalConciseContext()
	}
	if out.Operation != "search" || out.Search == nil {
		return nil, fmt.Errorf("concise source query requires search, relations, or context results")
	}
	type identity struct{ path, language, fingerprint string }
	files := make([]SourceSearchConciseFile, 0)
	byIdentity := make(map[identity]int)
	for _, match := range out.Search.Matches {
		id := identity{match.Path, match.Language, match.SourceFingerprint}
		index, ok := byIdentity[id]
		if !ok {
			index = len(files)
			byIdentity[id] = index
			files = append(files, SourceSearchConciseFile{
				Path: match.Path, Language: match.Language, Encoding: out.RequestedEncoding,
				SourceFingerprint: match.SourceFingerprint,
				Matches:           make([]SourceSearchConciseMatch, 0, 1),
			})
		}
		name := match.QualifiedName
		if name == "" {
			name = match.Name
		}
		files[index].Matches = append(files[index].Matches, SourceSearchConciseMatch{
			Symbol: name, Kind: match.Kind, Range: match.Range,
			SymbolID: match.SymbolID, Evidence: match.Evidence,
		})
	}
	coverage := struct {
		AnalysisComplete       bool `json:"analysisComplete"`
		FileSelectionTruncated bool `json:"fileSelectionTruncated"`
		ResultsTruncated       bool `json:"resultsTruncated"`
		FilesSkipped           int  `json:"filesSkipped,omitempty"`
	}{
		AnalysisComplete:       out.Coverage.AnalysisComplete,
		FileSelectionTruncated: out.Coverage.FileSelectionTruncated,
		ResultsTruncated:       out.Coverage.ResultsTruncated && out.Search.NextCursor == "",
		FilesSkipped:           out.Coverage.FilesSkipped,
	}
	compact := struct {
		Search struct {
			Files      []SourceSearchConciseFile `json:"files"`
			NextCursor string                    `json:"nextCursor,omitempty"`
		} `json:"search"`
		Scope    string                            `json:"scope"`
		Coverage any                               `json:"coverage"`
		Index    *sourceintelligence.IndexEvidence `json:"index,omitempty"`
	}{Scope: "indexed-structural", Coverage: coverage}
	compact.Search.Files = files
	compact.Search.NextCursor = out.Search.NextCursor
	if out.Index.Staleness != sourceintelligence.IndexCurrent {
		compact.Index = &out.Index
	}
	return json.Marshal(compact)
}
