package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func (h *Handler) executeSourceQuery(ctx context.Context, input SourceQueryInput, limits config.SourceConfig) (*mcp.CallToolResult, SourceQueryOutput, error) {
	maxFiles, result := resolvePositiveLimit(input.MaxFiles, limits.MaxFiles, "maxFiles")
	if result != nil {
		return result, SourceQueryOutput{}, nil
	}
	requestCtx := ctx
	cancel := func() {}
	if limits.MaxRequestSeconds > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, time.Duration(limits.MaxRequestSeconds)*time.Second)
	}
	defer cancel()

	selection, coverage, _, collectResult := h.collectSourceQueryIndex(requestCtx, input, limits, maxFiles)
	if collectResult != nil {
		return collectResult, SourceQueryOutput{}, nil
	}
	model := selection.Model
	if model == nil {
		return errorResultWithCode(ErrCodeInternal, "source query index returned no project model"), SourceQueryOutput{}, nil
	}

	output := SourceQueryOutput{
		Operation:        strings.ToLower(strings.TrimSpace(input.Operation)),
		CoordinateSystem: sourceCoordinateSystem,
		Index:            selection.Evidence,
		Coverage:         coverage,
	}
	switch output.Operation {
	case "search":
		maxResults, limitResult := resolvePositiveLimit(input.MaxResults, limits.MaxResults, "maxResults")
		if limitResult != nil {
			return limitResult, SourceQueryOutput{}, nil
		}
		match := sourceintelligence.ProjectSearchMatchMode(input.Match)
		if match == "" {
			match = sourceintelligence.ProjectSearchExact
		}
		evidence := make([]sourceintelligence.SymbolEvidence, 0, len(input.Evidence))
		for _, value := range input.Evidence {
			evidence = append(evidence, sourceintelligence.SymbolEvidence(value))
		}
		search, searchErr := model.StructuralSearch(requestCtx, sourceintelligence.ProjectSearchOptions{
			Query: input.Query, Match: match, Kinds: input.Kinds, Evidence: evidence, MaxResults: maxResults,
		})
		if searchErr != nil {
			return errorResultFromError(searchErr), SourceQueryOutput{}, nil
		}
		matches := make([]SourceSearchMatch, 0, len(search.Matches))
		for _, match := range search.Matches {
			matches = append(matches, SourceSearchMatch{
				Path: match.Path, Language: match.Language, Range: match.Range, SymbolID: match.SymbolID,
				SourceFingerprint: match.SourceFingerprint, Evidence: match.Evidence,
			})
		}
		output.Search = &SourceSearchResult{Matches: matches}
		if search.Truncated {
			output.Coverage.Truncated = true
			output.Coverage.CoverageComplete = false
		}
	case "relations":
		maxResults, limitResult := resolvePositiveLimit(input.MaxResults, limits.MaxResults, "maxResults")
		if limitResult != nil {
			return limitResult, SourceQueryOutput{}, nil
		}
		maxNodes, limitResult := resolvePositiveLimit(input.MaxNodes, limits.MaxGraphNodes, "maxNodes")
		if limitResult != nil {
			return limitResult, SourceQueryOutput{}, nil
		}
		maxEdges, limitResult := resolvePositiveLimit(input.MaxEdges, limits.MaxGraphEdges, "maxEdges")
		if limitResult != nil {
			return limitResult, SourceQueryOutput{}, nil
		}
		maxDepth, limitResult := resolvePositiveLimit(input.MaxDepth, limits.MaxGraphDepth, "maxDepth")
		if limitResult != nil {
			return limitResult, SourceQueryOutput{}, nil
		}
		var subject, target sourceintelligence.ProjectSelector
		if input.Subject != nil {
			resolved, selectorResult := h.projectSelectorFromInput(*input.Subject)
			if selectorResult != nil {
				return selectorResult, SourceQueryOutput{}, nil
			}
			subject = resolved
		}
		if input.Target != nil {
			resolved, selectorResult := h.projectSelectorFromInput(*input.Target)
			if selectorResult != nil {
				return selectorResult, SourceQueryOutput{}, nil
			}
			target = resolved
		}
		evidence := make([]sourceintelligence.SymbolEvidence, 0, len(input.Evidence))
		for _, value := range input.Evidence {
			evidence = append(evidence, sourceintelligence.SymbolEvidence(value))
		}
		queried, queryErr := model.QueryRelations(requestCtx, sourceintelligence.RelationKind(input.Relation), subject, target, evidence, sourceintelligence.ProjectQueryLimits{
			MaxResults: maxResults, MaxNodes: maxNodes, MaxEdges: maxEdges, MaxDepth: maxDepth,
		})
		if queryErr != nil {
			return errorResultFromError(queryErr), SourceQueryOutput{}, nil
		}
		output.Relations = &SourceRelationsResult{Relation: sourceintelligence.RelationKind(input.Relation), Relations: queried.Records}
		if queried.Truncated {
			output.Coverage.Truncated = true
			output.Coverage.CoverageComplete = false
		}
	case "context":
		budgetBytes, limitResult := resolvePositiveLimit(input.BudgetBytes, limits.MaxContextBytes, "budgetBytes")
		if limitResult != nil {
			return limitResult, SourceQueryOutput{}, nil
		}
		maxItems, limitResult := resolvePositiveLimit(input.MaxItems, limits.MaxContextItems, "maxItems")
		if limitResult != nil {
			return limitResult, SourceQueryOutput{}, nil
		}
		maxDepth, limitResult := resolvePositiveLimit(input.MaxDepth, limits.MaxGraphDepth, "maxDepth")
		if limitResult != nil {
			return limitResult, SourceQueryOutput{}, nil
		}
		targets := make([]sourceintelligence.ProjectSelector, 0, len(input.Targets))
		for _, target := range input.Targets {
			resolved, selectorResult := h.projectSelectorFromInput(target)
			if selectorResult != nil {
				return selectorResult, SourceQueryOutput{}, nil
			}
			targets = append(targets, resolved)
		}
		bodyPolicy := sourceintelligence.ProjectContextBodyPolicy(input.BodyPolicy)
		if bodyPolicy == "" {
			bodyPolicy = sourceintelligence.ProjectContextPrefer
		}
		plan, planErr := model.PlanContext(requestCtx, targets, sourceintelligence.ProjectContextOptions{
			BudgetBytes: budgetBytes, MaxItems: maxItems, MaxDepth: maxDepth, BodyPolicy: bodyPolicy,
		})
		if planErr != nil {
			return errorResultFromError(planErr), SourceQueryOutput{}, nil
		}
		items, usedBytes, materializeErr := h.materializeSourceContext(requestCtx, plan, input.Encoding, limits)
		if materializeErr != nil {
			return errorResultFromError(materializeErr), SourceQueryOutput{}, nil
		}
		output.Context = &SourceContextResult{Items: items, UsedBytes: usedBytes, BudgetBytes: budgetBytes}
		if plan.Truncated {
			output.Coverage.Truncated = true
			output.Coverage.CoverageComplete = false
		}
	default:
		return errorResultWithCode(ErrCodeUnsupported, "source_query operation is not implemented in this phase"), SourceQueryOutput{}, nil
	}
	if budgetErr := enforceSourceQueryOutputBudget(output, limits.MaxOutputBytes); budgetErr != nil {
		return errorResultFromError(budgetErr), SourceQueryOutput{}, nil
	}
	return nil, output, nil
}

func (h *Handler) projectSelectorFromInput(input SourceSelectorInput) (sourceintelligence.ProjectSelector, *mcp.CallToolResult) {
	validated := h.ValidatePath(input.Path)
	if !validated.Ok() {
		return sourceintelligence.ProjectSelector{}, validated.Result
	}
	selector := sourceintelligence.ProjectSelector{
		Kind: sourceintelligence.ProjectSelectorKind(input.Kind), Path: validated.Path,
		SymbolID: input.SymbolID, SourceFingerprint: input.SourceFingerprint,
	}
	if input.Position != nil {
		selector.Position = &sourceintelligence.Position{Line: input.Position.Line, Column: input.Position.Column}
	}
	return selector, nil
}

func (h *Handler) materializeSourceContext(ctx context.Context, plan sourceintelligence.ProjectContextPlan, requestedEncoding string, limits config.SourceConfig) ([]sourceintelligence.ContextItem, int, error) {
	documents := make(map[string]*sourceintelligence.SourceDocument)
	items := make([]sourceintelligence.ContextItem, 0, len(plan.Candidates))
	usedBytes := 0
	for _, candidate := range plan.Candidates {
		if err := ctx.Err(); err != nil {
			return nil, 0, operation.Wrap(operation.KindCancelled, "materialize_source_context", candidate.Entity.Path, err)
		}
		validated := h.ValidatePath(candidate.Entity.Path)
		if !validated.Ok() {
			return nil, 0, validated.Err
		}
		document := documents[validated.Path]
		if document == nil {
			opened, err := sourceintelligence.OpenSourceDocument(ctx, validated.Path, sourceintelligence.OpenDocumentOptions{
				RequestedEncoding: requestedEncoding, MaxFileBytes: limits.MaxFileBytes, MaxDecodedCharacters: h.maxDecodedCharacters(),
			})
			if err != nil {
				return nil, 0, err
			}
			if opened.SourceFingerprint != candidate.Entity.SourceFingerprint {
				return nil, 0, operation.Wrap(operation.KindConflict, "materialize_source_context", validated.Path, fmt.Errorf("source fingerprint is stale"))
			}
			document = opened
			documents[validated.Path] = opened
		} else if document.SourceFingerprint != candidate.Entity.SourceFingerprint {
			return nil, 0, operation.Wrap(operation.KindConflict, "materialize_source_context", validated.Path, fmt.Errorf("context plan mixes source fingerprints for one file"))
		}
		maximum := candidate.Offsets.End - candidate.Offsets.Start
		text, rangeValue, err := document.SliceUTF8Offsets(candidate.Offsets.Start, candidate.Offsets.End, maximum)
		if err != nil {
			return nil, 0, err
		}
		entity := candidate.Entity
		entity.Path = validated.Path
		entity.Range = &rangeValue
		items = append(items, sourceintelligence.ContextItem{
			Entity: entity, Reason: candidate.Reason, Representation: candidate.Representation, Priority: candidate.Priority,
			Text: text, Evidence: candidate.Evidence, Resolution: candidate.Resolution,
		})
		usedBytes += len(text)
	}
	return items, usedBytes, nil
}

func enforceSourceQueryOutputBudget(output SourceQueryOutput, maximum int64) error {
	encoded, err := json.Marshal(output)
	if err != nil {
		return operation.Wrap(operation.KindUnknown, "source_query", "", err)
	}
	if int64(len(encoded)) > maximum {
		return operation.Wrap(operation.KindLimit, "source_query", "", fmt.Errorf("source query output size %d exceeds limit %d", len(encoded), maximum))
	}
	return nil
}
