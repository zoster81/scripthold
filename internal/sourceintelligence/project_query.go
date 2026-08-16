package sourceintelligence

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// ProjectSelectorKind identifies the request-scoped entity class used by a
// project query. Selectors are always bound to the current source fingerprint.
type ProjectSelectorKind string

const (
	ProjectSelectorPath     ProjectSelectorKind = "path"
	ProjectSelectorSymbol   ProjectSelectorKind = "symbol"
	ProjectSelectorPosition ProjectSelectorKind = "position"
)

// ProjectSelector is the transport-independent selector used by the Phase 13
// query engine. Position selectors prefer a structural reference occurrence and
// otherwise resolve to the most specific containing declaration.
type ProjectSelector struct {
	Kind              ProjectSelectorKind
	Path              string
	SymbolID          string
	Position          *Position
	SourceFingerprint string
}

// ProjectQueryLimits bound one graph query independently from the resolver's
// build limits. Graph node/edge limits are hard correctness bounds; MaxResults
// truncates only already-proven deterministic output.
type ProjectQueryLimits struct {
	MaxResults int
	MaxNodes   int
	MaxEdges   int
	MaxDepth   int
}

// ProjectSearchMatchMode is the bounded structural-search matching policy.
type ProjectSearchMatchMode string

const (
	ProjectSearchExact    ProjectSearchMatchMode = "exact"
	ProjectSearchPrefix   ProjectSearchMatchMode = "prefix"
	ProjectSearchContains ProjectSearchMatchMode = "contains"
)

// ProjectSearchOptions configures structural search over normalized facts.
type ProjectSearchOptions struct {
	Query      string
	Match      ProjectSearchMatchMode
	Kinds      []string
	Evidence   []SymbolEvidence
	MaxResults int
}

// ProjectSearchMatch is one normalized symbol/dependency/reference occurrence.
type ProjectSearchMatch struct {
	Path              string
	Language          string
	Range             Range
	SymbolID          string
	SourceFingerprint string
	Evidence          SymbolEvidence
}

// ProjectSearchResult is a deterministic bounded structural-search result.
type ProjectSearchResult struct {
	Matches   []ProjectSearchMatch
	Truncated bool
}

// ProjectRelationResult is a deterministic bounded relationship result.
type ProjectRelationResult struct {
	Records   []RelationRecord
	Truncated bool
}

type projectQuerySelection struct {
	kind      ProjectSelectorKind
	pathKey   string
	entity    RelationEntity
	reference *ProjectReference
}

// StructuralSearch searches normalized symbols, dependencies, and relationship
// target spellings without rescanning raw source text.
func (model *ProjectModel) StructuralSearch(ctx context.Context, options ProjectSearchOptions) (ProjectSearchResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ProjectSearchResult{}, operation.Wrap(operation.KindCancelled, "project_structural_search", "", err)
	}
	if model == nil {
		return ProjectSearchResult{}, operation.New(operation.KindInvalidInput, "project model is required")
	}
	query := strings.TrimSpace(options.Query)
	if query == "" {
		return ProjectSearchResult{}, operation.New(operation.KindInvalidInput, "structural search query is required")
	}
	if options.MaxResults <= 0 {
		return ProjectSearchResult{}, operation.New(operation.KindInvalidInput, "structural search result limit must be positive")
	}
	matchMode := options.Match
	if matchMode == "" {
		matchMode = ProjectSearchExact
	}
	if matchMode != ProjectSearchExact && matchMode != ProjectSearchPrefix && matchMode != ProjectSearchContains {
		return ProjectSearchResult{}, operation.New(operation.KindInvalidInput, "unknown structural search match mode")
	}
	kindFilter := make(map[string]struct{}, len(options.Kinds))
	for _, kind := range options.Kinds {
		kind = strings.ToLower(strings.TrimSpace(kind))
		if kind != "" {
			kindFilter[kind] = struct{}{}
		}
	}
	evidenceFilter := make(map[SymbolEvidence]struct{}, len(options.Evidence))
	for _, evidence := range options.Evidence {
		evidenceFilter[evidence] = struct{}{}
	}
	acceptEvidence := func(value SymbolEvidence) bool {
		if len(evidenceFilter) == 0 {
			return true
		}
		_, ok := evidenceFilter[value]
		return ok
	}

	matches := make([]ProjectSearchMatch, 0, minInt(options.MaxResults+1, 256))
	appendMatch := func(value ProjectSearchMatch) {
		matches = append(matches, value)
	}
	for _, pathKey := range model.fileOrder {
		if err := ctx.Err(); err != nil {
			return ProjectSearchResult{}, operation.Wrap(operation.KindCancelled, "project_structural_search", model.files[pathKey].facts.Path, err)
		}
		for _, record := range model.symbolsByFile[pathKey] {
			if len(kindFilter) > 0 {
				if _, ok := kindFilter[strings.ToLower(string(record.symbol.Kind))]; !ok {
					continue
				}
			}
			if !acceptEvidence(record.symbol.Evidence) || (!projectSearchTextMatches(record.symbol.Name, query, matchMode) && !projectSearchTextMatches(record.symbol.QualifiedName, query, matchMode)) {
				continue
			}
			appendMatch(ProjectSearchMatch{Path: record.entity.Path, Language: record.entity.Language, Range: record.symbol.NameRange, SymbolID: record.entity.SymbolID, SourceFingerprint: record.entity.SourceFingerprint, Evidence: record.symbol.Evidence})
		}
		if len(kindFilter) != 0 {
			continue
		}
		for _, dependency := range model.dependenciesBySource[pathKey] {
			if !acceptEvidence(dependency.Evidence) || (!projectSearchTextMatches(dependency.Dependency.Value, query, matchMode) && !projectSearchTextMatches(dependency.Dependency.Alias, query, matchMode)) {
				continue
			}
			appendMatch(ProjectSearchMatch{Path: dependency.Source.Path, Language: dependency.Source.Language, Range: dependency.Dependency.Range, SourceFingerprint: dependency.Source.SourceFingerprint, Evidence: dependency.Evidence})
		}
	}
	if len(kindFilter) == 0 {
		for _, reference := range model.references {
			if err := ctx.Err(); err != nil {
				return ProjectSearchResult{}, operation.Wrap(operation.KindCancelled, "project_structural_search", reference.Source.Path, err)
			}
			if !acceptEvidence(reference.Evidence) || (!projectSearchTextMatches(reference.TargetSpelling, query, matchMode) && !projectSearchTextMatches(reference.StructuralKind, query, matchMode)) {
				continue
			}
			appendMatch(ProjectSearchMatch{Path: reference.Source.Path, Language: reference.Source.Language, Range: reference.Range, SourceFingerprint: reference.Source.SourceFingerprint, Evidence: reference.Evidence})
		}
	}
	return finalizeProjectSearch(matches, options.MaxResults), nil
}

// QueryRelations executes one bounded Phase 13 relationship query.
func (model *ProjectModel) QueryRelations(ctx context.Context, kind RelationKind, subject, target ProjectSelector, evidence []SymbolEvidence, limits ProjectQueryLimits) (ProjectRelationResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ProjectRelationResult{}, operation.Wrap(operation.KindCancelled, "project_relations", "", err)
	}
	if model == nil {
		return ProjectRelationResult{}, operation.New(operation.KindInvalidInput, "project model is required")
	}
	if err := validateProjectQueryLimits(limits); err != nil {
		return ProjectRelationResult{}, err
	}
	filter := newProjectEvidenceFilter(evidence)

	switch kind {
	case RelationCallers, RelationCallees, RelationOverrides:
		return ProjectRelationResult{}, operation.New(operation.KindUnsupported, fmt.Sprintf("%s relations are unavailable without analyzer-proven facts", kind))
	case RelationCycles:
		return model.queryDependencyCycles(ctx, filter, limits)
	case RelationTrace:
		return model.queryTrace(ctx, subject, target, filter, limits)
	case RelationDependencies, RelationDependents, RelationReferences, RelationDefinitions, RelationInheritance, RelationImplementations, RelationImpact:
		// handled below
	default:
		return ProjectRelationResult{}, operation.New(operation.KindInvalidInput, "unknown project relation kind")
	}

	selection, err := model.resolveProjectSelector(subject)
	if err != nil {
		return ProjectRelationResult{}, err
	}
	var records []RelationRecord
	switch kind {
	case RelationDependencies:
		if selection.kind != ProjectSelectorPath {
			return ProjectRelationResult{}, operation.New(operation.KindInvalidInput, "dependencies requires a path selector")
		}
		records = model.dependencyRecords(selection.pathKey, false, RelationDependencies, filter)
	case RelationDependents:
		if selection.kind != ProjectSelectorPath {
			return ProjectRelationResult{}, operation.New(operation.KindInvalidInput, "dependents requires a path selector")
		}
		records = model.dependencyRecords(selection.pathKey, true, RelationDependents, filter)
	case RelationReferences:
		if selection.entity.SymbolID == "" {
			return ProjectRelationResult{}, operation.New(operation.KindInvalidInput, "references requires a symbol or symbol-resolving position selector")
		}
		records = model.referenceRecordsTo(selection.entity.SymbolID, RelationReferences, nil, filter)
	case RelationDefinitions:
		records, err = model.definitionRecords(selection, filter)
		if err != nil {
			return ProjectRelationResult{}, err
		}
	case RelationInheritance:
		if selection.entity.SymbolID == "" {
			return ProjectRelationResult{}, operation.New(operation.KindInvalidInput, "inheritance requires a symbol or symbol-resolving position selector")
		}
		records = model.referenceRecordsFrom(selection.entity.SymbolID, RelationInheritance, isInheritanceStructuralKind, filter)
	case RelationImplementations:
		if selection.entity.SymbolID == "" {
			return ProjectRelationResult{}, operation.New(operation.KindInvalidInput, "implementations requires a symbol or symbol-resolving position selector")
		}
		records = model.referenceRecordsTo(selection.entity.SymbolID, RelationImplementations, isImplementationStructuralKind, filter)
	case RelationImpact:
		return model.queryImpact(ctx, selection, filter, limits)
	}
	sortRelationRecords(records)
	return truncateRelationRecords(records, limits.MaxResults), nil
}

func validateProjectQueryLimits(limits ProjectQueryLimits) error {
	if limits.MaxResults <= 0 || limits.MaxNodes <= 0 || limits.MaxEdges <= 0 || limits.MaxDepth <= 0 {
		return operation.New(operation.KindInvalidInput, "project query limits must be positive")
	}
	return nil
}

type projectEvidenceFilter map[SymbolEvidence]struct{}

func newProjectEvidenceFilter(values []SymbolEvidence) projectEvidenceFilter {
	if len(values) == 0 {
		return nil
	}
	filter := make(projectEvidenceFilter, len(values))
	for _, value := range values {
		filter[value] = struct{}{}
	}
	return filter
}

func (filter projectEvidenceFilter) allows(value SymbolEvidence) bool {
	if len(filter) == 0 {
		return true
	}
	_, ok := filter[value]
	return ok
}

func (model *ProjectModel) resolveProjectSelector(selector ProjectSelector) (projectQuerySelection, error) {
	pathKey := projectPathKey(selector.Path)
	file, ok := model.files[pathKey]
	if !ok || strings.TrimSpace(selector.Path) == "" {
		return projectQuerySelection{}, operation.Wrap(operation.KindNotFound, "resolve_project_selector", selector.Path, fmt.Errorf("project file was not found"))
	}
	if selector.SourceFingerprint != file.facts.SourceFingerprint {
		return projectQuerySelection{}, operation.Wrap(operation.KindConflict, "resolve_project_selector", file.facts.Path, fmt.Errorf("source fingerprint is stale"))
	}
	switch selector.Kind {
	case ProjectSelectorPath:
		return projectQuerySelection{kind: selector.Kind, pathKey: pathKey, entity: model.fileEntity(pathKey)}, nil
	case ProjectSelectorSymbol:
		for _, record := range model.symbolsByFile[pathKey] {
			if record.symbol.ID == selector.SymbolID {
				return projectQuerySelection{kind: selector.Kind, pathKey: pathKey, entity: record.entity}, nil
			}
		}
		return projectQuerySelection{}, operation.Wrap(operation.KindNotFound, "resolve_project_selector", file.facts.Path, fmt.Errorf("symbol was not found"))
	case ProjectSelectorPosition:
		if selector.Position == nil || selector.Position.Line <= 0 || selector.Position.Column <= 0 {
			return projectQuerySelection{}, operation.New(operation.KindInvalidInput, "position selector requires a positive position")
		}
		for index := range model.references {
			reference := &model.references[index]
			if projectPathKey(reference.Source.Path) == pathKey && rangeContainsPosition(reference.Range, *selector.Position) {
				copy := *reference
				return projectQuerySelection{kind: selector.Kind, pathKey: pathKey, entity: reference.Source, reference: &copy}, nil
			}
		}
		if record, ok := mostSpecificSymbolAt(model.symbolsByFile[pathKey], *selector.Position); ok {
			return projectQuerySelection{kind: selector.Kind, pathKey: pathKey, entity: record.entity}, nil
		}
		return projectQuerySelection{}, operation.Wrap(operation.KindNotFound, "resolve_project_selector", file.facts.Path, fmt.Errorf("position does not select a project entity"))
	default:
		return projectQuerySelection{}, operation.New(operation.KindInvalidInput, "unknown project selector kind")
	}
}

func mostSpecificSymbolAt(records []projectSymbolRecord, position Position) (projectSymbolRecord, bool) {
	var best projectSymbolRecord
	bestRank := -1
	found := false
	for _, record := range records {
		rank := -1
		switch {
		case rangeContainsPosition(record.symbol.NameRange, position):
			rank = 3
		case rangeContainsPosition(record.symbol.DeclarationRange, position):
			rank = 2
		case record.symbol.BodyRange != nil && rangeContainsPosition(*record.symbol.BodyRange, position):
			rank = 1
		}
		if rank < 0 {
			continue
		}
		if !found || rank > bestRank || (rank == bestRank && comparePosition(record.symbol.DeclarationRange.Start, best.symbol.DeclarationRange.Start) > 0) {
			best, bestRank, found = record, rank, true
		}
	}
	return best, found
}

func (model *ProjectModel) dependencyRecords(pathKey string, reverse bool, kind RelationKind, filter projectEvidenceFilter) []RelationRecord {
	var dependencies []ProjectDependency
	if reverse {
		dependencies = model.dependentsByTarget[pathKey]
	} else {
		dependencies = model.dependenciesBySource[pathKey]
	}
	result := make([]RelationRecord, 0, len(dependencies))
	for _, dependency := range dependencies {
		if !filter.allows(dependency.Evidence) {
			continue
		}
		if reverse {
			target := model.fileEntity(pathKey)
			result = append(result, RelationRecord{Kind: kind, Source: cloneRelationEntity(dependency.Source), Target: target, Evidence: dependency.Evidence, Resolution: dependency.Resolution})
			continue
		}
		if len(dependency.Targets) == 0 {
			result = append(result, RelationRecord{Kind: kind, Source: cloneRelationEntity(dependency.Source), Target: RelationEntity{Language: dependency.Source.Language, QualifiedName: dependency.Dependency.Value}, Evidence: dependency.Evidence, Resolution: dependency.Resolution})
			continue
		}
		for _, target := range dependency.Targets {
			result = append(result, RelationRecord{Kind: kind, Source: cloneRelationEntity(dependency.Source), Target: cloneRelationEntity(target), Evidence: dependency.Evidence, Resolution: dependency.Resolution})
		}
	}
	return result
}

func (model *ProjectModel) referenceRecordsTo(symbolID string, kind RelationKind, predicate func(string) bool, filter projectEvidenceFilter) []RelationRecord {
	var result []RelationRecord
	for _, reference := range model.referencesByTarget[symbolID] {
		if !filter.allows(reference.Evidence) || (predicate != nil && !predicate(reference.StructuralKind)) {
			continue
		}
		for _, target := range reference.Targets {
			if target.SymbolID != symbolID {
				continue
			}
			result = append(result, RelationRecord{Kind: kind, Source: cloneRelationEntity(reference.Source), Target: cloneRelationEntity(target), Evidence: reference.Evidence, Resolution: reference.Resolution})
		}
	}
	return result
}

func (model *ProjectModel) referenceRecordsFrom(symbolID string, kind RelationKind, predicate func(string) bool, filter projectEvidenceFilter) []RelationRecord {
	var result []RelationRecord
	for _, reference := range model.references {
		if reference.Source.SymbolID != symbolID || !filter.allows(reference.Evidence) || (predicate != nil && !predicate(reference.StructuralKind)) {
			continue
		}
		if len(reference.Targets) == 0 {
			result = append(result, RelationRecord{Kind: kind, Source: cloneRelationEntity(reference.Source), Target: RelationEntity{Language: reference.Source.Language, QualifiedName: reference.TargetSpelling, Range: cloneRangeValue(reference.Range)}, Evidence: reference.Evidence, Resolution: reference.Resolution})
			continue
		}
		for _, target := range reference.Targets {
			result = append(result, RelationRecord{Kind: kind, Source: cloneRelationEntity(reference.Source), Target: cloneRelationEntity(target), Evidence: reference.Evidence, Resolution: reference.Resolution})
		}
	}
	return result
}

func (model *ProjectModel) definitionRecords(selection projectQuerySelection, filter projectEvidenceFilter) ([]RelationRecord, error) {
	if selection.reference != nil {
		reference := *selection.reference
		if !filter.allows(reference.Evidence) {
			return nil, nil
		}
		if len(reference.Targets) == 0 {
			return []RelationRecord{{Kind: RelationDefinitions, Source: cloneRelationEntity(reference.Source), Target: RelationEntity{Language: reference.Source.Language, QualifiedName: reference.TargetSpelling, Range: cloneRangeValue(reference.Range)}, Evidence: reference.Evidence, Resolution: reference.Resolution}}, nil
		}
		result := make([]RelationRecord, 0, len(reference.Targets))
		for _, target := range reference.Targets {
			result = append(result, RelationRecord{Kind: RelationDefinitions, Source: cloneRelationEntity(reference.Source), Target: cloneRelationEntity(target), Evidence: reference.Evidence, Resolution: reference.Resolution})
		}
		return result, nil
	}
	if selection.entity.SymbolID != "" {
		if !filter.allows(SymbolEvidenceScopeResolved) {
			return nil, nil
		}
		return []RelationRecord{{Kind: RelationDefinitions, Source: cloneRelationEntity(selection.entity), Target: cloneRelationEntity(selection.entity), Evidence: SymbolEvidenceScopeResolved, Resolution: ResolutionResolved}}, nil
	}
	return nil, operation.New(operation.KindInvalidInput, "definitions requires a reference position or symbol selector")
}

func isInheritanceStructuralKind(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "inherits", "extends":
		return true
	default:
		return false
	}
}

func isImplementationStructuralKind(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "implements", "conforms":
		return true
	default:
		return false
	}
}

func (model *ProjectModel) queryTrace(ctx context.Context, subject, target ProjectSelector, filter projectEvidenceFilter, limits ProjectQueryLimits) (ProjectRelationResult, error) {
	start, err := model.resolveProjectSelector(subject)
	if err != nil {
		return ProjectRelationResult{}, err
	}
	end, err := model.resolveProjectSelector(target)
	if err != nil {
		return ProjectRelationResult{}, err
	}
	if start.kind != ProjectSelectorPath || end.kind != ProjectSelectorPath {
		return ProjectRelationResult{}, operation.New(operation.KindUnsupported, "symbol trace is unavailable until analyzer-proven symbol graph facts cover the requested relation")
	}
	if start.pathKey == end.pathKey {
		return ProjectRelationResult{}, nil
	}
	type queueItem struct {
		key   string
		depth int
	}
	queue := []queueItem{{key: start.pathKey}}
	visited := map[string]struct{}{start.pathKey: {}}
	parent := make(map[string]RelationRecord)
	nodes, edges := 1, 0
	depthLimited := false
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return ProjectRelationResult{}, operation.Wrap(operation.KindCancelled, "project_trace", model.files[queue[0].key].facts.Path, err)
		}
		current := queue[0]
		queue = queue[1:]
		if current.depth >= limits.MaxDepth {
			if len(model.localDependencyEdges(current.key, RelationTrace, filter)) > 0 {
				depthLimited = true
			}
			continue
		}
		for _, edge := range model.localDependencyEdges(current.key, RelationTrace, filter) {
			edges++
			if edges > limits.MaxEdges {
				return ProjectRelationResult{}, operation.New(operation.KindLimit, "project trace exceeded edge limit")
			}
			next := projectPathKey(edge.Target.Path)
			if _, seen := visited[next]; seen {
				continue
			}
			nodes++
			if nodes > limits.MaxNodes {
				return ProjectRelationResult{}, operation.New(operation.KindLimit, "project trace exceeded node limit")
			}
			visited[next] = struct{}{}
			parent[next] = edge
			if next == end.pathKey {
				return truncateRelationRecords(reconstructTrace(parent, start.pathKey, end.pathKey), limits.MaxResults), nil
			}
			queue = append(queue, queueItem{key: next, depth: current.depth + 1})
		}
	}
	if depthLimited {
		return ProjectRelationResult{}, operation.New(operation.KindLimit, "project trace exceeded depth limit before proving absence")
	}
	return ProjectRelationResult{}, operation.Wrap(operation.KindNotFound, "project_trace", target.Path, fmt.Errorf("no dependency path was found"))
}

func reconstructTrace(parent map[string]RelationRecord, startKey, endKey string) []RelationRecord {
	var reverse []RelationRecord
	current := endKey
	for current != startKey {
		edge, ok := parent[current]
		if !ok {
			return nil
		}
		reverse = append(reverse, edge)
		current = projectPathKey(edge.Source.Path)
	}
	result := make([]RelationRecord, len(reverse))
	for index := range reverse {
		result[len(reverse)-1-index] = reverse[index]
	}
	return result
}

func (model *ProjectModel) queryImpact(ctx context.Context, selection projectQuerySelection, filter projectEvidenceFilter, limits ProjectQueryLimits) (ProjectRelationResult, error) {
	if selection.kind != ProjectSelectorPath {
		if selection.entity.SymbolID == "" {
			return ProjectRelationResult{}, operation.New(operation.KindInvalidInput, "symbol impact requires a symbol selector")
		}
		records := model.referenceRecordsTo(selection.entity.SymbolID, RelationImpact, nil, filter)
		sortRelationRecords(records)
		return truncateRelationRecords(records, limits.MaxResults), nil
	}
	type queueItem struct {
		key   string
		depth int
	}
	queue := []queueItem{{key: selection.pathKey}}
	visited := map[string]struct{}{selection.pathKey: {}}
	var records []RelationRecord
	nodes, edges := 1, 0
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return ProjectRelationResult{}, operation.Wrap(operation.KindCancelled, "project_impact", model.files[queue[0].key].facts.Path, err)
		}
		current := queue[0]
		queue = queue[1:]
		if current.depth >= limits.MaxDepth {
			if model.hasFilteredDependent(current.key, filter) {
				return ProjectRelationResult{}, operation.New(operation.KindLimit, "project impact exceeded depth limit")
			}
			continue
		}
		for _, dependency := range model.dependentsByTarget[current.key] {
			if !filter.allows(dependency.Evidence) {
				continue
			}
			edges++
			if edges > limits.MaxEdges {
				return ProjectRelationResult{}, operation.New(operation.KindLimit, "project impact exceeded edge limit")
			}
			next := projectPathKey(dependency.Source.Path)
			if _, seen := visited[next]; seen {
				continue
			}
			nodes++
			if nodes > limits.MaxNodes {
				return ProjectRelationResult{}, operation.New(operation.KindLimit, "project impact exceeded node limit")
			}
			visited[next] = struct{}{}
			records = append(records, RelationRecord{Kind: RelationImpact, Source: cloneRelationEntity(dependency.Source), Target: model.fileEntity(current.key), Evidence: dependency.Evidence, Resolution: dependency.Resolution})
			queue = append(queue, queueItem{key: next, depth: current.depth + 1})
		}
	}
	return truncateRelationRecords(records, limits.MaxResults), nil
}

func (model *ProjectModel) hasFilteredDependent(pathKey string, filter projectEvidenceFilter) bool {
	for _, dependency := range model.dependentsByTarget[pathKey] {
		if filter.allows(dependency.Evidence) {
			return true
		}
	}
	return false
}

func (model *ProjectModel) queryDependencyCycles(ctx context.Context, filter projectEvidenceFilter, limits ProjectQueryLimits) (ProjectRelationResult, error) {
	if len(model.fileOrder) > limits.MaxNodes {
		return ProjectRelationResult{}, operation.New(operation.KindLimit, "project cycle query exceeded node limit")
	}
	adjacency := make(map[string][]string, len(model.fileOrder))
	edgeRecords := make(map[string]RelationRecord)
	edges := 0
	for _, source := range model.fileOrder {
		if err := ctx.Err(); err != nil {
			return ProjectRelationResult{}, operation.Wrap(operation.KindCancelled, "project_cycles", model.files[source].facts.Path, err)
		}
		for _, edge := range model.localDependencyEdges(source, RelationCycles, filter) {
			target := projectPathKey(edge.Target.Path)
			adjacency[source] = append(adjacency[source], target)
			edgeRecords[source+"\x00"+target] = edge
			edges++
			if edges > limits.MaxEdges {
				return ProjectRelationResult{}, operation.New(operation.KindLimit, "project cycle query exceeded edge limit")
			}
		}
		sort.Strings(adjacency[source])
	}

	index := 0
	indices := make(map[string]int, len(model.fileOrder))
	lowlink := make(map[string]int, len(model.fileOrder))
	onStack := make(map[string]bool, len(model.fileOrder))
	stack := make([]string, 0, len(model.fileOrder))
	components := make(map[string]int)
	componentSizes := make(map[int]int)
	componentID := 0
	var strongConnect func(string) error
	strongConnect = func(node string) error {
		if err := ctx.Err(); err != nil {
			return operation.Wrap(operation.KindCancelled, "project_cycles", model.files[node].facts.Path, err)
		}
		indices[node], lowlink[node] = index, index
		index++
		stack = append(stack, node)
		onStack[node] = true
		for _, next := range adjacency[node] {
			if _, seen := indices[next]; !seen {
				if err := strongConnect(next); err != nil {
					return err
				}
				if lowlink[next] < lowlink[node] {
					lowlink[node] = lowlink[next]
				}
			} else if onStack[next] && indices[next] < lowlink[node] {
				lowlink[node] = indices[next]
			}
		}
		if lowlink[node] == indices[node] {
			for {
				last := len(stack) - 1
				member := stack[last]
				stack = stack[:last]
				onStack[member] = false
				components[member] = componentID
				componentSizes[componentID]++
				if member == node {
					break
				}
			}
			componentID++
		}
		return nil
	}
	for _, node := range model.fileOrder {
		if _, seen := indices[node]; seen {
			continue
		}
		if err := strongConnect(node); err != nil {
			return ProjectRelationResult{}, err
		}
	}

	var records []RelationRecord
	for key, edge := range edgeRecords {
		separator := strings.IndexByte(key, 0)
		source, target := key[:separator], key[separator+1:]
		component := components[source]
		if component != components[target] || (componentSizes[component] == 1 && source != target) {
			continue
		}
		records = append(records, edge)
	}
	sortRelationRecords(records)
	return truncateRelationRecords(records, limits.MaxResults), nil
}

func (model *ProjectModel) localDependencyEdges(sourceKey string, kind RelationKind, filter projectEvidenceFilter) []RelationRecord {
	var result []RelationRecord
	for _, dependency := range model.dependenciesBySource[sourceKey] {
		if !filter.allows(dependency.Evidence) {
			continue
		}
		for _, target := range dependency.Targets {
			if _, ok := model.files[projectPathKey(target.Path)]; !ok {
				continue
			}
			result = append(result, RelationRecord{Kind: kind, Source: cloneRelationEntity(dependency.Source), Target: cloneRelationEntity(target), Evidence: dependency.Evidence, Resolution: dependency.Resolution})
		}
	}
	sortRelationRecords(result)
	return result
}

func truncateRelationRecords(records []RelationRecord, maximum int) ProjectRelationResult {
	if len(records) <= maximum {
		return ProjectRelationResult{Records: records}
	}
	return ProjectRelationResult{Records: append([]RelationRecord(nil), records[:maximum]...), Truncated: true}
}

func sortRelationRecords(values []RelationRecord) {
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i], values[j]
		if left.Source.Path != right.Source.Path {
			return left.Source.Path < right.Source.Path
		}
		if left.Target.Path != right.Target.Path {
			return left.Target.Path < right.Target.Path
		}
		if left.Source.SymbolID != right.Source.SymbolID {
			return left.Source.SymbolID < right.Source.SymbolID
		}
		if left.Target.SymbolID != right.Target.SymbolID {
			return left.Target.SymbolID < right.Target.SymbolID
		}
		if left.Source.QualifiedName != right.Source.QualifiedName {
			return left.Source.QualifiedName < right.Source.QualifiedName
		}
		if left.Target.QualifiedName != right.Target.QualifiedName {
			return left.Target.QualifiedName < right.Target.QualifiedName
		}
		return left.Kind < right.Kind
	})
}

func projectSearchTextMatches(value, query string, mode ProjectSearchMatchMode) bool {
	if value == "" {
		return false
	}
	switch mode {
	case ProjectSearchPrefix:
		return strings.HasPrefix(value, query)
	case ProjectSearchContains:
		return strings.Contains(value, query)
	default:
		return value == query
	}
}

func finalizeProjectSearch(matches []ProjectSearchMatch, maximum int) ProjectSearchResult {
	sortProjectSearchMatches(matches)
	if len(matches) <= maximum {
		return ProjectSearchResult{Matches: matches}
	}
	return ProjectSearchResult{Matches: append([]ProjectSearchMatch(nil), matches[:maximum]...), Truncated: true}
}

func sortProjectSearchMatches(values []ProjectSearchMatch) {
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i], values[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if comparePosition(left.Range.Start, right.Range.Start) != 0 {
			return comparePosition(left.Range.Start, right.Range.Start) < 0
		}
		if comparePosition(left.Range.End, right.Range.End) != 0 {
			return comparePosition(left.Range.End, right.Range.End) < 0
		}
		if left.SymbolID != right.SymbolID {
			return left.SymbolID < right.SymbolID
		}
		return left.Evidence < right.Evidence
	})
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
