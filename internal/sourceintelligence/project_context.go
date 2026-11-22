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

type projectContextKey struct {
	pathKey  string
	symbolID string
}

type projectContextPlanner struct {
	model   *ProjectModel
	options ProjectContextOptions
	plan    ProjectContextPlan
	seen    map[projectContextKey]struct{}
	roots   map[string][]projectSymbolRecord
}

// PlanContext builds a deterministic, budget-aware project context plan from
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

	if len(targets) == 1 {
		if plan, eligible, err := model.planFlatPathContext(ctx, targets[0], options); eligible {
			return plan, err
		}
	} else {
		if plan, eligible, err := model.planFlatMultiPathContext(ctx, targets, options); eligible {
			return plan, err
		}
	}

	seeds := make([]projectContextSeed, 0, len(targets))
	pathTargets := make([]string, 0, len(targets))
	for _, selector := range targets {
		if err := ctx.Err(); err != nil {
			return ProjectContextPlan{}, operation.Wrap(operation.KindCancelled, "plan_project_context", selector.Path, err)
		}
		var err error
		seeds, err = model.appendContextSeedsForSelector(seeds, selector)
		if err != nil {
			return ProjectContextPlan{}, err
		}
		if selector.Kind == ProjectSelectorPath {
			pathTargets = append(pathTargets, projectPathKey(selector.Path))
		}
	}
	if len(seeds) == 0 {
		return ProjectContextPlan{}, nil
	}
	sortContextSeeds(seeds)
	sort.Strings(pathTargets)
	pathTargets = deduplicateStrings(pathTargets)

	candidateCapacity := min(options.MaxItems, len(seeds)+8)
	if len(targets) == 1 && len(pathTargets) == 1 &&
		options.MaxItems >= 128 && options.BudgetBytes/512 >= options.MaxItems {
		// Count only distinct directly imported files. The hint is request-local
		// and may overestimate, but cannot grow beyond the configured item cap.
		estimated := len(seeds)
		visited := map[string]struct{}{pathTargets[0]: {}}
		for _, dependency := range model.dependenciesBySource[pathTargets[0]] {
			for _, target := range dependency.Targets {
				key := projectPathKey(target.Path)
				if _, exists := visited[key]; exists {
					continue
				}
				visited[key] = struct{}{}
				// Only parentless declarations are a safe lower-bound hint;
				// nested declarations can greatly outnumber context roots.
				for _, record := range model.symbolsByFile[key] {
					if record.symbol.ParentID == "" {
						estimated++
					}
					if estimated >= options.MaxItems {
						break
					}
				}
				if estimated >= options.MaxItems {
					break
				}
			}
			if estimated >= options.MaxItems {
				break
			}
		}
		candidateCapacity = max(candidateCapacity, min(options.MaxItems, estimated))
	}
	planner := projectContextPlanner{
		model: model, options: options,
		plan: ProjectContextPlan{Candidates: make([]ProjectContextCandidate, 0, candidateCapacity)},
		seen: make(map[projectContextKey]struct{}),
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
	if err := planner.addReverseAndTypeRelations(ctx, seeds, pathTargets); err != nil {
		return ProjectContextPlan{}, err
	}
	if err := planner.addDeeperDependencies(ctx, seeds); err != nil {
		return ProjectContextPlan{}, err
	}
	return planner.plan, nil
}

// planFlatPathContext avoids copying full symbol records when an oversized,
// single-file context has no parents, structural references or dependencies.
// All other requests continue through the general context planner.
func (model *ProjectModel) planFlatPathContext(ctx context.Context, selector ProjectSelector, options ProjectContextOptions) (ProjectContextPlan, bool, error) {
	if selector.Kind != ProjectSelectorPath || options.MaxItems > 256 {
		return ProjectContextPlan{}, false, nil
	}
	pathKey := projectPathKey(selector.Path)
	file, exists := model.files[pathKey]
	if !exists || selector.Path == "" || selector.SourceFingerprint != file.facts.SourceFingerprint {
		return ProjectContextPlan{}, false, nil
	}
	records := model.symbolsByFile[pathKey]
	if len(records) <= options.MaxItems*3 ||
		len(model.references) != 0 || len(model.referencesByTarget) != 0 ||
		len(model.referencesByTargetIndices) != 0 ||
		len(model.dependenciesBySource[pathKey]) != 0 ||
		len(model.dependentsByTarget[pathKey]) != 0 {
		return ProjectContextPlan{}, false, nil
	}
	// The general root selector filters nested records and re-sorts records
	// that are not strictly ordered. Neither case is safe for this shortcut.
	// When declaration offsets are also sorted, iterate the model directly.
	orderedByOffset := true
	for i := range records {
		if records[i].pathKey != pathKey || records[i].symbol.ParentID != "" ||
			(i > 0 && !projectSymbolRecordLess(records[i-1], records[i])) {
			return ProjectContextPlan{}, false, nil
		}
		if i > 0 {
			previous, current := records[i-1].symbol, records[i].symbol
			if previous.declarationOffsets.Start > current.declarationOffsets.Start ||
				(previous.declarationOffsets.Start == current.declarationOffsets.Start && previous.ID >= current.ID) {
				orderedByOffset = false
			}
		}
	}
	planner := projectContextPlanner{
		model: model, options: options,
		plan: ProjectContextPlan{Candidates: make([]ProjectContextCandidate, 0, min(options.MaxItems, len(records)+8))},
		seen: make(map[projectContextKey]struct{}),
	}
	if orderedByOffset {
		for _, record := range records {
			if err := planner.add(record, ContextTarget, 1, record.symbol.Evidence, ResolutionResolved, false); err != nil {
				return ProjectContextPlan{}, true, err
			}
			if planner.flatTargetsComplete() {
				break
			}
		}
	} else {
		indices := make([]int, len(records))
		for i := range indices {
			indices[i] = i
		}
		sort.Slice(indices, func(i, j int) bool {
			left, right := records[indices[i]], records[indices[j]]
			if left.symbol.declarationOffsets.Start != right.symbol.declarationOffsets.Start {
				return left.symbol.declarationOffsets.Start < right.symbol.declarationOffsets.Start
			}
			return left.symbol.ID < right.symbol.ID
		})
		for _, index := range indices {
			record := records[index]
			if err := planner.add(record, ContextTarget, 1, record.symbol.Evidence, ResolutionResolved, false); err != nil {
				return ProjectContextPlan{}, true, err
			}
			if planner.flatTargetsComplete() {
				break
			}
		}
	}
	// In the general route, deeper dependency traversal checks cancellation
	// after every seed has been offered to the bounded planner.
	if err := ctx.Err(); err != nil {
		return ProjectContextPlan{}, true, operation.Wrap(operation.KindCancelled, "plan_project_context", file.facts.Path, err)
	}
	return planner.plan, true, nil
}

// planFlatMultiPathContext visits already ordered immutable records without
// allocating and sorting one full seed copy per path. Only independent flat
// files are eligible; all related, nested or ambiguous models use the general
// planner, which preserves their complete context graph behavior.
func (model *ProjectModel) planFlatMultiPathContext(ctx context.Context, selectors []ProjectSelector, options ProjectContextOptions) (ProjectContextPlan, bool, error) {
	if len(selectors) < 2 || len(selectors) > 64 || options.MaxItems > 256 ||
		len(model.dependencies) != 0 || len(model.dependenciesBySource) != 0 ||
		len(model.dependentsByTarget) != 0 || len(model.references) != 0 ||
		len(model.referencesByTarget) != 0 || len(model.referencesByTargetIndices) != 0 ||
		len(model.referencesBySource) != 0 || len(model.referencesBySourceID) != 0 ||
		len(model.definitionsByReference) != 0 {
		return ProjectContextPlan{}, false, nil
	}
	paths := make([]string, 0, len(selectors))
	visited := make(map[string]struct{}, len(selectors))
	total := 0
	for _, selector := range selectors {
		if err := ctx.Err(); err != nil {
			return ProjectContextPlan{}, true, operation.Wrap(operation.KindCancelled, "plan_project_context", selector.Path, err)
		}
		if selector.Kind != ProjectSelectorPath || selector.Path == "" {
			return ProjectContextPlan{}, false, nil
		}
		pathKey := projectPathKey(selector.Path)
		file, exists := model.files[pathKey]
		if !exists || selector.SourceFingerprint != file.facts.SourceFingerprint {
			return ProjectContextPlan{}, false, nil
		}
		if _, exists := visited[pathKey]; exists {
			return ProjectContextPlan{}, false, nil
		}
		visited[pathKey] = struct{}{}
		records := model.symbolsByFile[pathKey]
		if len(records) == 0 {
			return ProjectContextPlan{}, false, nil
		}
		for index := range records {
			record := records[index]
			if record.pathKey != pathKey || record.symbol.ParentID != "" ||
				(index > 0 && !projectSymbolRecordLess(records[index-1], record)) {
				return ProjectContextPlan{}, false, nil
			}
			if index > 0 {
				previous := records[index-1].symbol
				if previous.declarationOffsets.Start > record.symbol.declarationOffsets.Start ||
					(previous.declarationOffsets.Start == record.symbol.declarationOffsets.Start && previous.ID >= record.symbol.ID) {
					return ProjectContextPlan{}, false, nil
				}
			}
		}
		total += len(records)
		paths = append(paths, pathKey)
	}
	if total <= options.MaxItems*4 {
		return ProjectContextPlan{}, false, nil
	}
	sort.Strings(paths)
	planner := projectContextPlanner{
		model: model, options: options,
		plan: ProjectContextPlan{Candidates: make([]ProjectContextCandidate, 0, min(options.MaxItems, total))},
		seen: make(map[projectContextKey]struct{}),
	}
	for _, pathKey := range paths {
		for _, record := range model.symbolsByFile[pathKey] {
			if err := planner.add(record, ContextTarget, 1, record.symbol.Evidence, ResolutionResolved, false); err != nil {
				return ProjectContextPlan{}, true, err
			}
			if planner.flatTargetsComplete() {
				break
			}
		}
		if planner.flatTargetsComplete() {
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return ProjectContextPlan{}, true, operation.Wrap(operation.KindCancelled, "plan_project_context", "", err)
	}
	return planner.plan, true, nil
}

// All remaining flat targets are nonrequired; once the item limit is
// conclusively exceeded, later targets cannot alter this plan.
func (planner *projectContextPlanner) flatTargetsComplete() bool {
	return len(planner.plan.Candidates) >= planner.options.MaxItems && planner.plan.Truncated
}

// appendContextSeedsForSelector writes path roots directly into the request's
// seed slice, avoiding a second full copy of large immutable model records.
func (model *ProjectModel) appendContextSeedsForSelector(seeds []projectContextSeed, selector ProjectSelector) ([]projectContextSeed, error) {
	if selector.Kind != ProjectSelectorPath {
		resolved, err := model.contextSeedsForSelector(selector)
		if err != nil {
			return nil, err
		}
		return append(seeds, resolved...), nil
	}
	pathKey := projectPathKey(selector.Path)
	file, ok := model.files[pathKey]
	if !ok || selector.Path == "" {
		return nil, operation.Wrap(operation.KindNotFound, "resolve_project_context_selector", selector.Path, fmt.Errorf("project file was not found"))
	}
	if selector.SourceFingerprint != file.facts.SourceFingerprint {
		return nil, operation.Wrap(operation.KindConflict, "resolve_project_context_selector", file.facts.Path, fmt.Errorf("source fingerprint is stale"))
	}
	records := model.contextRootSymbols(pathKey)
	if cap(seeds)-len(seeds) < len(records) {
		needed := len(seeds) + len(records)
		capacity := needed
		// Amortize repeated path selectors without retaining scratch between
		// requests. Bound speculative reserve relative to the latest path so
		// a tiny file after a large file cannot double memory consumption.
		if prior := cap(seeds); prior > 0 && prior <= int(^uint(0)>>1)/2 && prior*2 > needed {
			spare := prior*2 - needed
			if len(records) <= int(^uint(0)>>1)/4 && spare > 4*len(records) {
				spare = 4 * len(records)
			}
			capacity = needed + spare
		}
		grown := make([]projectContextSeed, len(seeds), capacity)
		copy(grown, seeds)
		seeds = grown
	}
	for _, record := range records {
		seeds = append(seeds, projectContextSeed{record: record, evidence: record.symbol.Evidence, resolution: ResolutionResolved})
	}
	return seeds, nil
}

func (model *ProjectModel) contextSeedsForSelector(selector ProjectSelector) ([]projectContextSeed, error) {
	if selector.Kind == ProjectSelectorPath {
		return model.appendContextSeedsForSelector(nil, selector)
	}
	pathKey := projectPathKey(selector.Path)
	file, ok := model.files[pathKey]
	if !ok || selector.Path == "" {
		return nil, operation.Wrap(operation.KindNotFound, "resolve_project_context_selector", selector.Path, fmt.Errorf("project file was not found"))
	}
	if selector.SourceFingerprint != file.facts.SourceFingerprint {
		return nil, operation.Wrap(operation.KindConflict, "resolve_project_context_selector", file.facts.Path, fmt.Errorf("source fingerprint is stale"))
	}

	switch selector.Kind {
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
		for _, index := range model.referencesBySource[pathKey] {
			reference := model.references[index]
			if !rangeContainsPosition(reference.Range, *selector.Position) {
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
	key := projectContextKey{pathKey: record.pathKey, symbolID: record.symbol.ID}
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

// visitContextAncestors centralizes cycle and cancellation protection for both
// context consumers without allocating an intermediate ancestry slice.
func (model *ProjectModel) visitContextAncestors(ctx context.Context, current projectSymbolRecord, visit func(projectSymbolRecord) error) error {
	visited := make(map[string]struct{})
	for current.symbol.ParentID != "" {
		if err := ctx.Err(); err != nil {
			return operation.Wrap(operation.KindCancelled, "plan_project_context", current.entity.Path, err)
		}
		if _, seen := visited[current.symbol.ParentID]; seen {
			break
		}
		visited[current.symbol.ParentID] = struct{}{}
		parent, ok := model.contextSymbolByID(current.pathKey, current.symbol.ParentID)
		if !ok {
			break
		}
		if err := visit(parent); err != nil {
			return err
		}
		current = parent
	}
	return nil
}

func (planner *projectContextPlanner) addEnclosing(ctx context.Context, seeds []projectContextSeed) error {
	for _, seed := range seeds {
		if err := planner.model.visitContextAncestors(ctx, seed.record, func(parent projectSymbolRecord) error {
			return planner.add(parent, ContextEnclosing, 2, parent.symbol.Evidence, ResolutionResolved, false)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (planner *projectContextPlanner) addDirectDependencies(ctx context.Context, seeds []projectContextSeed) error {
	// Every dependency into the same file visits the same root symbols.
	// The first occurrence determines their evidence; later visits cannot
	// add new candidates because the planner already tracks each symbol.
	visitedTargets := make(map[string]struct{})
	for _, pathKey := range contextSeedPathKeys(seeds) {
		for _, dependency := range planner.model.dependenciesBySource[pathKey] {
			if err := ctx.Err(); err != nil {
				return operation.Wrap(operation.KindCancelled, "plan_project_context", dependency.Source.Path, err)
			}
			for _, target := range dependency.Targets {
				targetPathKey := projectPathKey(target.Path)
				if _, visited := visitedTargets[targetPathKey]; visited {
					continue
				}
				visitedTargets[targetPathKey] = struct{}{}
				for _, record := range planner.contextRootSymbols(targetPathKey) {
					if err := planner.add(record, ContextDirectDependency, 3, dependency.Evidence, dependency.Resolution, false); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (planner *projectContextPlanner) addReverseAndTypeRelations(ctx context.Context, seeds []projectContextSeed, pathTargets []string) error {
	if len(planner.model.references) == 0 && len(planner.model.referencesByTarget) == 0 &&
		len(planner.model.referencesByTargetIndices) == 0 {
		// No structural edge can contribute either direction. Retain reverse
		// dependency context without allocating or sorting the seed graph.
		if len(seeds) > 0 {
			if err := ctx.Err(); err != nil {
				return operation.Wrap(operation.KindCancelled, "plan_project_context", seeds[0].record.entity.Path, err)
			}
		}
		return planner.addReverseDependents(pathTargets)
	}
	records := make([]projectSymbolRecord, 0, len(seeds)*2)
	for _, seed := range seeds {
		records = append(records, seed.record)
		if err := planner.model.visitContextAncestors(ctx, seed.record, func(parent projectSymbolRecord) error {
			records = append(records, parent)
			return nil
		}); err != nil {
			return err
		}
	}
	sortProjectSymbolRecords(records)

	// Group only relevant outgoing type relations by source ID. Indexes refer
	// to immutable model records, avoiding copied relations and repeated scans.
	outgoing := make(map[string][]int, len(records))
	for _, record := range records {
		outgoing[record.symbol.ID] = nil
	}
	appendOutgoing := func(index int) {
		reference := planner.model.references[index]
		if !isInheritanceStructuralKind(reference.StructuralKind) && !isImplementationStructuralKind(reference.StructuralKind) {
			return
		}
		if _, wanted := outgoing[reference.Source.SymbolID]; wanted {
			outgoing[reference.Source.SymbolID] = append(outgoing[reference.Source.SymbolID], index)
		}
	}
	if len(planner.model.references) >= 32 && len(planner.model.referencesBySource) > 0 {
		// Select each source file once, even when several seeds share it.
		// Per-file indices retain global reference order. The later path check
		// still separates identical symbol IDs belonging to different files.
		seenPaths := make(map[string]struct{}, len(records))
		for _, record := range records {
			if _, seen := seenPaths[record.pathKey]; seen {
				continue
			}
			seenPaths[record.pathKey] = struct{}{}
			for _, index := range planner.model.referencesBySource[record.pathKey] {
				appendOutgoing(index)
			}
		}
	} else {
		for index := range planner.model.references {
			appendOutgoing(index)
		}
	}

	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return operation.Wrap(operation.KindCancelled, "plan_project_context", record.entity.Path, err)
		}
		for _, index := range outgoing[record.symbol.ID] {
			reference := planner.model.references[index]
			if projectPathKey(reference.Source.Path) != record.pathKey {
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
		if err := planner.model.forEachReverseReference(record.symbol.ID, func(reference ProjectReference) error {
			if !isInheritanceStructuralKind(reference.StructuralKind) && !isImplementationStructuralKind(reference.StructuralKind) {
				return nil
			}
			sourceRecord, ok := planner.model.contextSymbolByID(projectPathKey(reference.Source.Path), reference.Source.SymbolID)
			if ok {
				return planner.add(sourceRecord, ContextReverseOrTypeRelation, 6, reference.Evidence, reference.Resolution, false)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return planner.addReverseDependents(pathTargets)
}

func (planner *projectContextPlanner) addReverseDependents(pathTargets []string) error {
	for _, pathKey := range pathTargets {
		for _, dependency := range planner.model.dependentsByTarget[pathKey] {
			for _, record := range planner.contextRootSymbols(projectPathKey(dependency.Source.Path)) {
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
				for _, record := range planner.contextRootSymbols(edge.pathKey) {
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

// contextRootSymbols reuses immutable normalized records across repeated edges
// within one plan without growing the retained project-generation index.
func (planner *projectContextPlanner) contextRootSymbols(pathKey string) []projectSymbolRecord {
	if records, ok := planner.roots[pathKey]; ok {
		return records
	}
	// Model records are sorted and immutable after construction. Borrow them
	// only when filtering cannot change the root set; planner callers read
	// these records without modifying the retained model backing array.
	records := planner.model.symbolsByFile[pathKey]
	flat := len(records) > 0
	for _, record := range records {
		if record.symbol.ParentID != "" {
			flat = false
			break
		}
	}
	if flat {
		// Do not let an append through this borrowed view write into spare
		// capacity of the model's immutable backing array.
		records = records[:len(records):len(records)]
	} else {
		records = planner.model.contextRootSymbols(pathKey)
	}
	if planner.roots == nil {
		planner.roots = make(map[string][]projectSymbolRecord)
	}
	planner.roots[pathKey] = records
	return records
}

func (model *ProjectModel) contextRootSymbols(pathKey string) []projectSymbolRecord {
	records := model.symbolsByFile[pathKey]
	if len(records) == 0 {
		return nil
	}
	// The immutable model already sorts its records. A flat file with strictly
	// increasing sort keys needs neither a parent map nor a copied root slice.
	// Preserve the original sort for ties, unsorted input and nested ownership.
	flatSorted := true
	for index := range records {
		if records[index].symbol.ParentID != "" ||
			(index > 0 && !projectSymbolRecordLess(records[index-1], records[index])) {
			flatSorted = false
			break
		}
	}
	if flatSorted {
		return records[:len(records):len(records)]
	}
	kindByID := make(map[string]SymbolKind, len(records))
	for _, record := range records {
		kindByID[record.symbol.ID] = record.symbol.Kind
	}
	result := make([]projectSymbolRecord, 0, len(records))
	for _, record := range records {
		if record.symbol.ParentID == "" {
			result = append(result, record)
			continue
		}
		parentKind, ok := kindByID[record.symbol.ParentID]
		if ok && isContextRootContainer(parentKind) {
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
	records := model.symbolsByFile[pathKey]
	if byID, indexed := model.symbolIndicesByFile[pathKey]; indexed {
		index, found := byID[symbolID]
		if found {
			return records[index], true
		}
		return projectSymbolRecord{}, false
	}
	for _, record := range records {
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
		// The declaration offsets are immutable inside the model. The public
		// accessor clones optional signature/body offsets we do not use here.
		if left.symbol.declarationOffsets.Start != right.symbol.declarationOffsets.Start {
			return left.symbol.declarationOffsets.Start < right.symbol.declarationOffsets.Start
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
