package sourceintelligence

import (
	"context"
	"fmt"
	"sort"

	"github.com/zoster81/scripthold/internal/operation"
)

// ProjectContextBodyPolicy controls whether a bounded context plan may retain
// verified declaration bodies or must use signatures only.
type ProjectContextBodyPolicy string

const (
	ProjectContextPrefer         ProjectContextBodyPolicy = "prefer"
	ProjectContextSignaturesOnly ProjectContextBodyPolicy = "signatures-only"
)

// ProjectContextOptions bounds one deterministic request-scoped context plan.
type ProjectContextOptions struct {
	BudgetBytes int
	MaxItems    int
	MaxDepth    int
	BodyPolicy  ProjectContextBodyPolicy
}

// ProjectContextCandidate is an internal materialization instruction. Offsets
// refer to the decoded UTF-8 source snapshot identified by Entity.SourceFingerprint.
type ProjectContextCandidate struct {
	Entity         RelationEntity
	Reason         ContextReason
	Representation ContextRepresentation
	Priority       int
	Offsets        OffsetRange
	Evidence       SymbolEvidence
	Resolution     ResolutionState
}

// ProjectContextPlan is bounded before any source text is retained.
type ProjectContextPlan struct {
	Candidates []ProjectContextCandidate
	UsedBytes  int
	Truncated  bool
}

type projectContextSeed struct {
	record     projectSymbolRecord
	evidence   SymbolEvidence
	resolution ResolutionState
	required   bool
}

type projectContextPlanner struct {
	model   *ProjectModel
	options ProjectContextOptions
	plan    ProjectContextPlan
	seen    map[string]struct{}
}

// PlanContext builds a deterministic, budget-aware Phase 14 context plan from
// normalized project facts only. Source text is deliberately materialized by
// the authorized handler after fingerprint revalidation.
func (model *ProjectModel) PlanContext(ctx context.Context, targets []ProjectSelector, options ProjectContextOptions) (ProjectContextPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ProjectContextPlan{}, operation.Wrap(operation.KindCancelled, "plan_project_context", "", err)
	}
	if model == nil {
		return ProjectContextPlan{}, operation.New(operation.KindInvalidInput, "project model is required")
	}
	if len(targets) == 0 {
		return ProjectContextPlan{}, operation.New(operation.KindInvalidInput, "project context requires at least one target")
	}
	if options.BudgetBytes <= 0 || options.MaxItems <= 0 || options.MaxDepth <= 0 {
		return ProjectContextPlan{}, operation.New(operation.KindInvalidInput, "project context limits must be positive")
	}
	if options.BodyPolicy == "" {
		options.BodyPolicy = ProjectContextPrefer
	}
	if options.BodyPolicy != ProjectContextPrefer && options.BodyPolicy != ProjectContextSignaturesOnly {
		return ProjectContextPlan{}, operation.New(operation.KindInvalidInput, "unknown project context body policy")
	}

	seeds := make([]projectContextSeed, 0, len(targets))
	for _, selector := range targets {
		if err := ctx.Err(); err != nil {
			return ProjectContextPlan{}, operation.Wrap(operation.KindCancelled, "plan_project_context", selector.Path, err)
		}
		resolved, err := model.contextSeedsForSelector(selector)
		if err != nil {
			return ProjectContextPlan{}, err
		}
		seeds = append(seeds, resolved...)
	}
	if len(seeds) == 0 {
		return ProjectContextPlan{}, nil
	}
	sortContextSeeds(seeds)

	planner := projectContextPlanner{
		model: model, options: options,
		plan: ProjectContextPlan{Candidates: make([]ProjectContextCandidate, 0, min(options.MaxItems, len(seeds)+8))},
		seen: make(map[string]struct{}),
	}
	for _, seed := range seeds {
		if err := planner.add(seed.record, ContextTarget, 1, seed.evidence, seed.resolution, seed.required); err != nil {
			return ProjectContextPlan{}, err
		}
	}
	if err := planner.addEnclosing(ctx, seeds); err != nil {
		return ProjectContextPlan{}, err
	}
	if err := planner.addDirectDependencies(ctx, seeds); err != nil {
		return ProjectContextPlan{}, err
	}
	if err := planner.addReverseAndTypeRelations(ctx, seeds); err != nil {
		return ProjectContextPlan{}, err
	}
	if err := planner.addDeeperDependencies(ctx, seeds); err != nil {
		return ProjectContextPlan{}, err
	}
	return planner.plan, nil
}

func (model *ProjectModel) contextSeedsForSelector(selector ProjectSelector) ([]projectContextSeed, error) {
	pathKey := projectPathKey(selector.Path)
	file, ok := model.files[pathKey]
	if !ok || selector.Path == "" {
		return nil, operation.Wrap(operation.KindNotFound, "resolve_project_context_selector", selector.Path, fmt.Errorf("project file was not found"))
	}
	if selector.SourceFingerprint != file.facts.SourceFingerprint {
		return nil, operation.Wrap(operation.KindConflict, "resolve_project_context_selector", file.facts.Path, fmt.Errorf("source fingerprint is stale"))
	}

	switch selector.Kind {
	case ProjectSelectorPath:
		records := model.contextRootSymbols(pathKey)
		seeds := make([]projectContextSeed, 0, len(records))
		for _, record := range records {
			seeds = append(seeds, projectContextSeed{record: record, evidence: record.symbol.Evidence, resolution: ResolutionResolved})
		}
		return seeds, nil
	case ProjectSelectorSymbol:
		record, ok := model.contextSymbolByID(pathKey, selector.SymbolID)
		if !ok {
			return nil, operation.Wrap(operation.KindNotFound, "resolve_project_context_selector", file.facts.Path, fmt.Errorf("symbol was not found"))
		}
		return []projectContextSeed{{record: record, evidence: record.symbol.Evidence, resolution: ResolutionResolved, required: true}}, nil
	case ProjectSelectorPosition:
		if selector.Position == nil || selector.Position.Line <= 0 || selector.Position.Column <= 0 {
			return nil, operation.New(operation.KindInvalidInput, "position selector requires a positive position")
		}
		for _, reference := range model.references {
			if projectPathKey(reference.Source.Path) != pathKey || !rangeContainsPosition(reference.Range, *selector.Position) {
				continue
			}
			if len(reference.Targets) > 0 {
				seeds := make([]projectContextSeed, 0, len(reference.Targets))
				for _, target := range reference.Targets {
					record, found := model.contextSymbolByID(projectPathKey(target.Path), target.SymbolID)
					if found {
						seeds = append(seeds, projectContextSeed{record: record, evidence: reference.Evidence, resolution: reference.Resolution, required: true})
					}
				}
				if len(seeds) > 0 {
					sortContextSeeds(seeds)
					return seeds, nil
				}
			}
			if reference.Source.SymbolID != "" {
				record, found := model.contextSymbolByID(pathKey, reference.Source.SymbolID)
				if found {
					return []projectContextSeed{{record: record, evidence: reference.Evidence, resolution: reference.Resolution, required: true}}, nil
				}
			}
		}
		if record, found := mostSpecificSymbolAt(model.symbolsByFile[pathKey], *selector.Position); found {
			return []projectContextSeed{{record: record, evidence: record.symbol.Evidence, resolution: ResolutionResolved, required: true}}, nil
		}
		return nil, operation.Wrap(operation.KindNotFound, "resolve_project_context_selector", file.facts.Path, fmt.Errorf("position does not select a project entity"))
	default:
		return nil, operation.New(operation.KindInvalidInput, "unknown project selector kind")
	}
}

func (planner *projectContextPlanner) add(record projectSymbolRecord, reason ContextReason, priority int, evidence SymbolEvidence, resolution ResolutionState, required bool) error {
	return planner.addWithPolicy(record, reason, priority, evidence, resolution, required, planner.options.BodyPolicy)
}

func (planner *projectContextPlanner) addWithPolicy(record projectSymbolRecord, reason ContextReason, priority int, evidence SymbolEvidence, resolution ResolutionState, required bool, bodyPolicy ProjectContextBodyPolicy) error {
	key := record.pathKey + "\x00" + record.symbol.ID
	if _, exists := planner.seen[key]; exists {
		return nil
	}
	if len(planner.plan.Candidates) >= planner.options.MaxItems {
		if required {
			return operation.New(operation.KindLimit, "project context item limit cannot retain every explicit target")
		}
		planner.plan.Truncated = true
		return nil
	}

	remaining := planner.options.BudgetBytes - planner.plan.UsedBytes
	representation, offsets, degraded, ok := contextRepresentationForSymbol(record.symbol, bodyPolicy, remaining)
	if !ok {
		if required {
			return operation.New(operation.KindLimit, "project context byte budget cannot retain an explicit target signature")
		}
		planner.plan.Truncated = true
		return nil
	}
	if degraded {
		planner.plan.Truncated = true
	}
	planner.seen[key] = struct{}{}
	planner.plan.Candidates = append(planner.plan.Candidates, ProjectContextCandidate{
		Entity: cloneRelationEntity(record.entity), Reason: reason, Representation: representation, Priority: priority,
		Offsets: offsets, Evidence: evidence, Resolution: resolution,
	})
	planner.plan.UsedBytes += offsets.End - offsets.Start
	return nil
}

func contextRepresentationForSymbol(symbol NormalizedSymbol, policy ProjectContextBodyPolicy, remaining int) (ContextRepresentation, OffsetRange, bool, bool) {
	declaration, _, signature, body := symbol.SourceOffsets()
	signatureRange, signatureOK := contextSignatureOffsets(declaration, signature, body)
	if policy == ProjectContextPrefer && body != nil && validContextOffsets(declaration) {
		bodyCost := declaration.End - declaration.Start
		if bodyCost <= remaining {
			return ContextBody, declaration, false, true
		}
		if signatureOK && signatureRange.End-signatureRange.Start <= remaining {
			return ContextSignature, signatureRange, true, true
		}
		return "", OffsetRange{}, true, false
	}
	if signatureOK && signatureRange.End-signatureRange.Start <= remaining {
		return ContextSignature, signatureRange, false, true
	}
	return "", OffsetRange{}, false, false
}

func contextSignatureOffsets(declaration OffsetRange, signature, body *OffsetRange) (OffsetRange, bool) {
	if signature != nil && validContextOffsets(*signature) {
		return *signature, true
	}
	if body != nil && validContextOffsets(declaration) && body.Start > declaration.Start && body.Start <= declaration.End {
		return OffsetRange{Start: declaration.Start, End: body.Start}, true
	}
	if validContextOffsets(declaration) {
		return declaration, true
	}
	return OffsetRange{}, false
}

func validContextOffsets(value OffsetRange) bool {
	return value.Start >= 0 && value.End > value.Start
}

func (planner *projectContextPlanner) addEnclosing(ctx context.Context, seeds []projectContextSeed) error {
	for _, seed := range seeds {
		current := seed.record
		visited := make(map[string]struct{})
		for current.symbol.ParentID != "" {
			if err := ctx.Err(); err != nil {
				return operation.Wrap(operation.KindCancelled, "plan_project_context", current.entity.Path, err)
			}
			if _, seen := visited[current.symbol.ParentID]; seen {
				break
			}
			visited[current.symbol.ParentID] = struct{}{}
			parent, ok := planner.model.contextSymbolByID(current.pathKey, current.symbol.ParentID)
			if !ok {
				break
			}
			if err := planner.add(parent, ContextEnclosing, 2, parent.symbol.Evidence, ResolutionResolved, false); err != nil {
				return err
			}
			current = parent
		}
	}
	return nil
}

func (planner *projectContextPlanner) addDirectDependencies(ctx context.Context, seeds []projectContextSeed) error {
	for _, pathKey := range contextSeedPathKeys(seeds) {
		for _, dependency := range planner.model.dependenciesBySource[pathKey] {
			if err := ctx.Err(); err != nil {
				return operation.Wrap(operation.KindCancelled, "plan_project_context", dependency.Source.Path, err)
			}
			for _, target := range dependency.Targets {
				for _, record := range planner.model.contextRootSymbols(projectPathKey(target.Path)) {
					if err := planner.add(record, ContextDirectDependency, 3, dependency.Evidence, dependency.Resolution, false); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (planner *projectContextPlanner) addReverseAndTypeRelations(ctx context.Context, seeds []projectContextSeed) error {
	records := make([]projectSymbolRecord, 0, len(seeds)*2)
	for _, seed := range seeds {
		records = append(records, seed.record)
		current := seed.record
		for current.symbol.ParentID != "" {
			parent, ok := planner.model.contextSymbolByID(current.pathKey, current.symbol.ParentID)
			if !ok {
				break
			}
			records = append(records, parent)
			current = parent
		}
	}
	sortProjectSymbolRecords(records)

	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return operation.Wrap(operation.KindCancelled, "plan_project_context", record.entity.Path, err)
		}
		for _, reference := range planner.model.references {
			if reference.Source.SymbolID != record.symbol.ID || (!isInheritanceStructuralKind(reference.StructuralKind) && !isImplementationStructuralKind(reference.StructuralKind)) {
				continue
			}
			for _, target := range reference.Targets {
				targetRecord, ok := planner.model.contextSymbolByID(projectPathKey(target.Path), target.SymbolID)
				if ok {
					if err := planner.add(targetRecord, ContextReverseOrTypeRelation, 6, reference.Evidence, reference.Resolution, false); err != nil {
						return err
					}
				}
			}
		}
		for _, reference := range planner.model.referencesByTarget[record.symbol.ID] {
			if !isInheritanceStructuralKind(reference.StructuralKind) && !isImplementationStructuralKind(reference.StructuralKind) {
				continue
			}
			sourceRecord, ok := planner.model.contextSymbolByID(projectPathKey(reference.Source.Path), reference.Source.SymbolID)
			if ok {
				if err := planner.add(sourceRecord, ContextReverseOrTypeRelation, 6, reference.Evidence, reference.Resolution, false); err != nil {
					return err
				}
			}
		}
	}
	for _, pathKey := range contextSeedPathKeys(seeds) {
		for _, dependency := range planner.model.dependentsByTarget[pathKey] {
			for _, record := range planner.model.contextRootSymbols(projectPathKey(dependency.Source.Path)) {
				if err := planner.add(record, ContextReverseOrTypeRelation, 6, dependency.Evidence, dependency.Resolution, false); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (planner *projectContextPlanner) addDeeperDependencies(ctx context.Context, seeds []projectContextSeed) error {
	type queueItem struct {
		pathKey string
		depth   int
	}
	type nextEdge struct {
		pathKey    string
		evidence   SymbolEvidence
		resolution ResolutionState
	}
	starts := contextSeedPathKeys(seeds)
	queue := make([]queueItem, 0, len(starts))
	visited := make(map[string]struct{}, len(starts))
	for _, pathKey := range starts {
		queue = append(queue, queueItem{pathKey: pathKey})
		visited[pathKey] = struct{}{}
	}
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return operation.Wrap(operation.KindCancelled, "plan_project_context", planner.model.files[queue[0].pathKey].facts.Path, err)
		}
		current := queue[0]
		queue = queue[1:]
		if current.depth >= planner.options.MaxDepth {
			if contextHasUnseenDependencyTarget(planner.model, current.pathKey, visited) {
				planner.plan.Truncated = true
			}
			continue
		}
		nextEdges := make([]nextEdge, 0)
		for _, dependency := range planner.model.dependenciesBySource[current.pathKey] {
			for _, target := range dependency.Targets {
				nextEdges = append(nextEdges, nextEdge{pathKey: projectPathKey(target.Path), evidence: dependency.Evidence, resolution: dependency.Resolution})
			}
		}
		sort.Slice(nextEdges, func(i, j int) bool {
			if nextEdges[i].pathKey != nextEdges[j].pathKey {
				return nextEdges[i].pathKey < nextEdges[j].pathKey
			}
			leftRank, leftOK := symbolEvidenceRank[nextEdges[i].evidence]
			rightRank, rightOK := symbolEvidenceRank[nextEdges[j].evidence]
			if leftOK && rightOK && leftRank != rightRank {
				return leftRank < rightRank
			}
			return nextEdges[i].resolution < nextEdges[j].resolution
		})
		for index := 0; index < len(nextEdges); {
			edge := nextEdges[index]
			index++
			for index < len(nextEdges) && nextEdges[index].pathKey == edge.pathKey {
				edge.evidence = weakerContextEvidence(edge.evidence, nextEdges[index].evidence)
				edge.resolution = weakerContextResolution(edge.resolution, nextEdges[index].resolution)
				index++
			}
			if _, seen := visited[edge.pathKey]; seen {
				continue
			}
			visited[edge.pathKey] = struct{}{}
			depth := current.depth + 1
			if depth >= 2 {
				for _, record := range planner.model.contextRootSymbols(edge.pathKey) {
					if err := planner.addWithPolicy(record, ContextDeeperRelation, 7, edge.evidence, edge.resolution, false, ProjectContextSignaturesOnly); err != nil {
						return err
					}
				}
			}
			queue = append(queue, queueItem{pathKey: edge.pathKey, depth: depth})
		}
	}
	return nil
}

func contextHasUnseenDependencyTarget(model *ProjectModel, pathKey string, visited map[string]struct{}) bool {
	for _, dependency := range model.dependenciesBySource[pathKey] {
		for _, target := range dependency.Targets {
			if _, seen := visited[projectPathKey(target.Path)]; !seen {
				return true
			}
		}
	}
	return false
}

func weakerContextEvidence(left, right SymbolEvidence) SymbolEvidence {
	leftRank, leftOK := symbolEvidenceRank[left]
	rightRank, rightOK := symbolEvidenceRank[right]
	if !leftOK {
		return left
	}
	if !rightOK {
		return right
	}
	if rightRank < leftRank {
		return right
	}
	return left
}

func weakerContextResolution(left, right ResolutionState) ResolutionState {
	if left == right {
		return left
	}
	return ResolutionAmbiguous
}

func (model *ProjectModel) contextRootSymbols(pathKey string) []projectSymbolRecord {
	records := model.symbolsByFile[pathKey]
	if len(records) == 0 {
		return nil
	}
	byID := make(map[string]projectSymbolRecord, len(records))
	for _, record := range records {
		byID[record.symbol.ID] = record
	}
	result := make([]projectSymbolRecord, 0, len(records))
	for _, record := range records {
		if record.symbol.ParentID == "" {
			result = append(result, record)
			continue
		}
		parent, ok := byID[record.symbol.ParentID]
		if ok && isContextRootContainer(parent.symbol.Kind) {
			result = append(result, record)
		}
	}
	if len(result) == 0 {
		result = append(result, records...)
	}
	sortProjectSymbolRecords(result)
	return result
}

func isContextRootContainer(kind SymbolKind) bool {
	switch kind {
	case SymbolKindPackage, SymbolKindNamespace, SymbolKindModule:
		return true
	default:
		return false
	}
}

func (model *ProjectModel) contextSymbolByID(pathKey, symbolID string) (projectSymbolRecord, bool) {
	for _, record := range model.symbolsByFile[pathKey] {
		if record.symbol.ID == symbolID {
			return record, true
		}
	}
	return projectSymbolRecord{}, false
}

func contextSeedPathKeys(seeds []projectContextSeed) []string {
	values := make([]string, 0, len(seeds))
	for _, seed := range seeds {
		values = append(values, seed.record.pathKey)
	}
	sort.Strings(values)
	return deduplicateStrings(values)
}

func sortContextSeeds(values []projectContextSeed) {
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i].record, values[j].record
		if left.pathKey != right.pathKey {
			return left.pathKey < right.pathKey
		}
		leftDecl, _, _, _ := left.symbol.SourceOffsets()
		rightDecl, _, _, _ := right.symbol.SourceOffsets()
		if leftDecl.Start != rightDecl.Start {
			return leftDecl.Start < rightDecl.Start
		}
		return left.symbol.ID < right.symbol.ID
	})
}

func deduplicateStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}
