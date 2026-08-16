package sourceintelligence

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/zoster81/scripthold/internal/operation"
)

// ProjectResolutionStage identifies the deterministic resolver stage that
// produced a candidate set. It records how a structural fact was resolved
// without upgrading it to compiler/type-checker semantics.
type ProjectResolutionStage string

const (
	ProjectResolutionSameFile         ProjectResolutionStage = "same-file"
	ProjectResolutionSameModule       ProjectResolutionStage = "same-module"
	ProjectResolutionExplicitImport   ProjectResolutionStage = "explicit-import"
	ProjectResolutionDependencyExport ProjectResolutionStage = "dependency-export"
	ProjectResolutionProject          ProjectResolutionStage = "project"
	ProjectResolutionNone             ProjectResolutionStage = "none"
)

// ProjectFileFacts is the immutable per-file input consumed by the Phase 12
// resolver. Facts must already come from the normalized analyzer layer.
type ProjectFileFacts struct {
	Path              string
	Language          string
	SourceFingerprint string
	Analysis          AnalyzerResult
}

// ProjectResolverLimits bound one in-memory resolver build. Candidate sets are
// additionally capped so repeated ambiguous names cannot multiply memory without
// bound. A zero MaxCandidatesPerResolution uses a conservative internal default.
type ProjectResolverLimits struct {
	MaxFiles                   int
	MaxSymbols                 int
	MaxDependencies            int
	MaxReferences              int
	MaxCandidatesPerResolution int
}

// ProjectDependency is one normalized dependency fact plus its project-local
// candidate files. External dependencies intentionally carry no invented target.
type ProjectDependency struct {
	Source     RelationEntity         `json:"source"`
	Dependency StructuralDependency   `json:"dependency"`
	Targets    []RelationEntity       `json:"targets,omitempty"`
	Stage      ProjectResolutionStage `json:"stage"`
	Evidence   SymbolEvidence         `json:"evidence"`
	Resolution ResolutionState        `json:"resolution"`
}

// ProjectReference is one structural declaration relation interpreted as a
// reference occurrence and resolved against project symbol tables.
type ProjectReference struct {
	ID             string                 `json:"id"`
	Source         RelationEntity         `json:"source"`
	StructuralKind string                 `json:"structuralKind"`
	TargetSpelling string                 `json:"targetSpelling"`
	Range          Range                  `json:"range"`
	Targets        []RelationEntity       `json:"targets,omitempty"`
	Stage          ProjectResolutionStage `json:"stage"`
	Evidence       SymbolEvidence         `json:"evidence"`
	Resolution     ResolutionState        `json:"resolution"`
}

type projectFileRecord struct {
	facts      ProjectFileFacts
	pathKey    string
	languageID string
}

type projectSymbolRecord struct {
	symbol       NormalizedSymbol
	entity       RelationEntity
	pathKey      string
	languageID   string
	qualifiedKey string
	nameKey      string
}

// ProjectModel is the deterministic, immutable Phase 12 symbol/dependency model.
// It remains memory-only and may back retained process-local Phase 15 generations;
// persistent on-disk indexing is not introduced.
type ProjectModel struct {
	files                  map[string]projectFileRecord
	filesByStem            map[string][]string
	fileOrder              []string
	symbolsByFile          map[string][]projectSymbolRecord
	symbolsByQualified     map[string][]projectSymbolRecord
	symbolsByName          map[string][]projectSymbolRecord
	dependencies           []ProjectDependency
	dependenciesBySource   map[string][]ProjectDependency
	dependentsByTarget     map[string][]ProjectDependency
	references             []ProjectReference
	definitionsByReference map[string][]RelationEntity
	referencesByTarget     map[string][]ProjectReference
}

const defaultProjectResolutionCandidates = 64

// BuildProjectModel constructs deterministic symbol tables, dependency
// adjacency, and resolved structural references from normalized analyzer facts.
func BuildProjectModel(ctx context.Context, registry *LanguageRegistry, input []ProjectFileFacts, limits ProjectResolverLimits) (*ProjectModel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, operation.Wrap(operation.KindCancelled, "build_project_model", "", err)
	}
	if registry == nil {
		return nil, operation.New(operation.KindInvalidInput, "language registry is required")
	}
	if limits.MaxFiles <= 0 || limits.MaxSymbols <= 0 || limits.MaxDependencies <= 0 || limits.MaxReferences <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "project resolver limits must be positive")
	}
	maxCandidates := limits.MaxCandidatesPerResolution
	if maxCandidates == 0 {
		maxCandidates = defaultProjectResolutionCandidates
	}
	if maxCandidates < 0 {
		return nil, operation.New(operation.KindInvalidInput, "project resolver candidate limit must not be negative")
	}
	if len(input) > limits.MaxFiles {
		return nil, operation.Wrap(operation.KindLimit, "build_project_model", "", fmt.Errorf("file count %d exceeds limit %d", len(input), limits.MaxFiles))
	}

	model := &ProjectModel{
		files:                  make(map[string]projectFileRecord, len(input)),
		filesByStem:            make(map[string][]string, len(input)),
		symbolsByFile:          make(map[string][]projectSymbolRecord),
		symbolsByQualified:     make(map[string][]projectSymbolRecord),
		symbolsByName:          make(map[string][]projectSymbolRecord),
		dependenciesBySource:   make(map[string][]ProjectDependency),
		dependentsByTarget:     make(map[string][]ProjectDependency),
		definitionsByReference: make(map[string][]RelationEntity),
		referencesByTarget:     make(map[string][]ProjectReference),
	}

	files := append([]ProjectFileFacts(nil), input...)
	sort.Slice(files, func(i, j int) bool { return projectPathKey(files[i].Path) < projectPathKey(files[j].Path) })
	var symbolCount, dependencyCount, referenceCount int
	for _, facts := range files {
		if err := ctx.Err(); err != nil {
			return nil, operation.Wrap(operation.KindCancelled, "build_project_model", facts.Path, err)
		}
		path := strings.TrimSpace(facts.Path)
		if path == "" {
			return nil, operation.New(operation.KindInvalidInput, "project file path is required")
		}
		pathKey := projectPathKey(path)
		if _, duplicate := model.files[pathKey]; duplicate {
			return nil, operation.Wrap(operation.KindInvalidInput, "build_project_model", path, fmt.Errorf("duplicate project file path"))
		}
		descriptor, ok := registry.Resolve(facts.Language)
		if !ok || !descriptor.Capabilities.SourceAnalysis {
			return nil, operation.Wrap(operation.KindInvalidInput, "build_project_model", path, fmt.Errorf("unknown or unsupported project language %q", facts.Language))
		}
		if !validProjectFingerprint(facts.SourceFingerprint) {
			return nil, operation.Wrap(operation.KindInvalidInput, "build_project_model", path, fmt.Errorf("source fingerprint must be a lowercase SHA-256 digest"))
		}
		symbolCount += len(facts.Analysis.Analysis.Symbols)
		dependencyCount += len(facts.Analysis.Dependencies)
		referenceCount += len(facts.Analysis.Relations)
		if symbolCount > limits.MaxSymbols {
			return nil, operation.Wrap(operation.KindLimit, "build_project_model", path, fmt.Errorf("symbol count exceeds limit %d", limits.MaxSymbols))
		}
		if dependencyCount > limits.MaxDependencies {
			return nil, operation.Wrap(operation.KindLimit, "build_project_model", path, fmt.Errorf("dependency count exceeds limit %d", limits.MaxDependencies))
		}
		if referenceCount > limits.MaxReferences {
			return nil, operation.Wrap(operation.KindLimit, "build_project_model", path, fmt.Errorf("reference count exceeds limit %d", limits.MaxReferences))
		}
		facts.Path = path
		facts.Language = descriptor.ID
		record := projectFileRecord{facts: facts, pathKey: pathKey, languageID: descriptor.ID}
		model.files[pathKey] = record
		stemKey := projectPathStemKey(path)
		model.filesByStem[stemKey] = append(model.filesByStem[stemKey], pathKey)
		model.fileOrder = append(model.fileOrder, pathKey)
	}

	if err := model.buildSymbolTables(ctx, registry); err != nil {
		return nil, err
	}
	if err := model.buildDependencies(ctx, registry, maxCandidates); err != nil {
		return nil, err
	}
	if err := model.buildReferences(ctx, registry, maxCandidates); err != nil {
		return nil, err
	}
	return model, nil
}

func (model *ProjectModel) buildSymbolTables(ctx context.Context, registry *LanguageRegistry) error {
	for _, pathKey := range model.fileOrder {
		file := model.files[pathKey]
		for _, symbol := range file.facts.Analysis.Analysis.Symbols {
			if err := ctx.Err(); err != nil {
				return operation.Wrap(operation.KindCancelled, "build_project_symbol_tables", file.facts.Path, err)
			}
			languageID := strings.TrimSpace(symbol.Language)
			if languageID == "" {
				languageID = file.languageID
			}
			descriptor, ok := registry.Resolve(languageID)
			if !ok {
				continue
			}
			entity := relationEntityForSymbol(symbol, file.facts.SourceFingerprint)
			record := projectSymbolRecord{
				symbol: symbol, entity: entity, pathKey: pathKey, languageID: descriptor.ID,
				qualifiedKey: projectSymbolLookupKey(descriptor, symbol.QualifiedName),
				nameKey:      projectSymbolLookupKey(descriptor, symbol.Name),
			}
			model.symbolsByFile[pathKey] = append(model.symbolsByFile[pathKey], record)
			model.symbolsByQualified[descriptor.ID+"\x00"+record.qualifiedKey] = append(model.symbolsByQualified[descriptor.ID+"\x00"+record.qualifiedKey], record)
			model.symbolsByName[descriptor.ID+"\x00"+record.nameKey] = append(model.symbolsByName[descriptor.ID+"\x00"+record.nameKey], record)
		}
	}
	for key := range model.symbolsByFile {
		sortProjectSymbolRecords(model.symbolsByFile[key])
	}
	for key := range model.symbolsByQualified {
		sortProjectSymbolRecords(model.symbolsByQualified[key])
	}
	for key := range model.symbolsByName {
		sortProjectSymbolRecords(model.symbolsByName[key])
	}
	return nil
}

func (model *ProjectModel) buildDependencies(ctx context.Context, registry *LanguageRegistry, maxCandidates int) error {
	for _, pathKey := range model.fileOrder {
		file := model.files[pathKey]
		descriptor, _ := registry.Resolve(file.languageID)
		for _, dependency := range file.facts.Analysis.Dependencies {
			if err := ctx.Err(); err != nil {
				return operation.Wrap(operation.KindCancelled, "resolve_project_dependency", file.facts.Path, err)
			}
			targets, err := model.resolveDependencyTargets(file, descriptor, dependency, maxCandidates)
			if err != nil {
				return err
			}
			resolved := ProjectDependency{
				Source:     RelationEntity{Path: file.facts.Path, Language: file.languageID, SourceFingerprint: file.facts.SourceFingerprint, Range: cloneRangeValue(dependency.Range)},
				Dependency: dependency, Targets: targets, Stage: ProjectResolutionExplicitImport, Evidence: dependency.Evidence,
			}
			switch len(targets) {
			case 0:
				resolved.Resolution = ResolutionExternal
			case 1:
				resolved.Resolution = ResolutionResolved
				resolved.Evidence = SymbolEvidenceProjectResolved
			default:
				resolved.Resolution = ResolutionAmbiguous
			}
			model.dependencies = append(model.dependencies, resolved)
			model.dependenciesBySource[pathKey] = append(model.dependenciesBySource[pathKey], resolved)
			for _, target := range targets {
				key := projectPathKey(target.Path)
				model.dependentsByTarget[key] = append(model.dependentsByTarget[key], resolved)
			}
		}
	}
	sortProjectDependencies(model.dependencies)
	for key := range model.dependenciesBySource {
		sortProjectDependencies(model.dependenciesBySource[key])
	}
	for key := range model.dependentsByTarget {
		sortProjectDependencies(model.dependentsByTarget[key])
	}
	return nil
}

func (model *ProjectModel) resolveDependencyTargets(file projectFileRecord, descriptor LanguageDescriptor, dependency StructuralDependency, maxCandidates int) ([]RelationEntity, error) {
	value := strings.TrimSpace(dependency.Value)
	if value == "" {
		return nil, nil
	}
	pathLike := dependencyLooksPathLike(value, dependency.Kind)
	pathMatches := model.dependencyPathMatches(file, dependency)
	if len(pathMatches) > 0 {
		return boundedUniqueEntities(pathMatches, maxCandidates, "dependency", file.facts.Path)
	}
	if pathLike {
		return nil, nil
	}

	normalized := projectSymbolLookupKey(descriptor, value)
	if normalized == "" {
		return nil, nil
	}
	qualified := model.symbolsByQualified[descriptor.ID+"\x00"+normalized]
	if dependencyLooksQualified(value) {
		if len(qualified) > 0 {
			return boundedUniqueEntities(fileEntitiesForSymbols(model, qualified), maxCandidates, "dependency", file.facts.Path)
		}
		leaf := projectDependencyLeaf(value)
		if leaf != "" {
			records := model.symbolsByName[descriptor.ID+"\x00"+projectSymbolLookupKey(descriptor, leaf)]
			if len(records) > 0 {
				return boundedUniqueEntities(fileEntitiesForSymbols(model, records), maxCandidates, "dependency", file.facts.Path)
			}
		}
		return nil, nil
	}

	moduleRecords := make([]projectSymbolRecord, 0, len(qualified))
	for _, record := range qualified {
		if isModuleLikeSymbol(record.symbol.Kind) {
			moduleRecords = append(moduleRecords, record)
		}
	}
	if len(moduleRecords) > 0 {
		return boundedUniqueEntities(fileEntitiesForSymbols(model, moduleRecords), maxCandidates, "dependency", file.facts.Path)
	}
	return nil, nil
}

func (model *ProjectModel) dependencyPathMatches(file projectFileRecord, dependency StructuralDependency) []RelationEntity {
	value := strings.TrimSpace(dependency.Value)
	if !dependencyLooksPathLike(value, dependency.Kind) {
		return nil
	}
	value = strings.Trim(value, "\"'")
	value = filepath.FromSlash(strings.ReplaceAll(value, "\\", "/"))
	candidate := filepath.Clean(filepath.Join(filepath.Dir(file.facts.Path), value))
	keys := []string{projectPathKey(candidate)}
	if filepath.Ext(value) == "" {
		keys = append(keys, projectPathKey(candidate)+"\x00stem", projectPathKey(filepath.Join(candidate, "index"))+"\x00stem")
	}
	seen := make(map[string]RelationEntity)
	for _, key := range keys {
		if strings.HasSuffix(key, "\x00stem") {
			stem := strings.TrimSuffix(key, "\x00stem")
			for _, pathKey := range model.filesByStem[stem] {
				seen[pathKey] = model.fileEntity(pathKey)
			}
			continue
		}
		if _, ok := model.files[key]; ok {
			seen[key] = model.fileEntity(key)
		}
	}
	return sortedEntityMap(seen)
}

func (model *ProjectModel) buildReferences(ctx context.Context, registry *LanguageRegistry, maxCandidates int) error {
	for _, pathKey := range model.fileOrder {
		file := model.files[pathKey]
		descriptor, _ := registry.Resolve(file.languageID)
		for _, relation := range file.facts.Analysis.Relations {
			if err := ctx.Err(); err != nil {
				return operation.Wrap(operation.KindCancelled, "resolve_project_reference", file.facts.Path, err)
			}
			source := model.resolveRelationSource(file, descriptor, relation)
			targets, stage, err := model.resolveRelationTargets(file, descriptor, relation, maxCandidates)
			if err != nil {
				return err
			}
			reference := ProjectReference{
				Source: source, StructuralKind: relation.Kind, TargetSpelling: relation.Target, Range: relation.Range,
				Targets: targets, Stage: stage, Evidence: relation.Evidence,
			}
			switch len(targets) {
			case 0:
				reference.Resolution = ResolutionUnresolved
				reference.Stage = ProjectResolutionNone
			case 1:
				reference.Resolution = ResolutionResolved
				if stage == ProjectResolutionSameFile {
					reference.Evidence = SymbolEvidenceScopeResolved
				} else {
					reference.Evidence = SymbolEvidenceProjectResolved
				}
			default:
				reference.Resolution = ResolutionAmbiguous
			}
			reference.ID = deterministicProjectReferenceID(reference)
			model.references = append(model.references, reference)
			model.definitionsByReference[reference.ID] = cloneRelationEntities(targets)
			for _, target := range targets {
				if target.SymbolID != "" {
					model.referencesByTarget[target.SymbolID] = append(model.referencesByTarget[target.SymbolID], reference)
				}
			}
		}
	}
	sortProjectReferences(model.references)
	for key := range model.referencesByTarget {
		sortProjectReferences(model.referencesByTarget[key])
	}
	return nil
}

func (model *ProjectModel) resolveRelationSource(file projectFileRecord, descriptor LanguageDescriptor, relation StructuralRelation) RelationEntity {
	lookup := projectSymbolLookupKey(descriptor, relation.Source)
	candidates := make([]projectSymbolRecord, 0, 2)
	for _, record := range model.symbolsByFile[file.pathKey] {
		if record.languageID != descriptor.ID || !isRelationEndpointKind(record.symbol.Kind) {
			continue
		}
		if record.qualifiedKey == lookup || record.nameKey == lookup {
			candidates = append(candidates, record)
		}
	}
	if len(candidates) == 1 {
		return candidates[0].entity
	}
	for _, candidate := range candidates {
		if rangeContainsPosition(candidate.symbol.DeclarationRange, relation.Range.Start) {
			return candidate.entity
		}
	}
	return RelationEntity{
		Path: file.facts.Path, Language: descriptor.ID, QualifiedName: strings.TrimSpace(relation.Source),
		SourceFingerprint: file.facts.SourceFingerprint, Range: cloneRangeValue(relation.Range),
	}
}

func (model *ProjectModel) resolveRelationTargets(file projectFileRecord, descriptor LanguageDescriptor, relation StructuralRelation, maxCandidates int) ([]RelationEntity, ProjectResolutionStage, error) {
	target := projectSymbolLookupKey(descriptor, relation.Target)
	if target == "" {
		return nil, ProjectResolutionNone, nil
	}
	source := projectSymbolLookupKey(descriptor, relation.Source)
	parent := qualifiedParent(source)

	if candidates := model.sameFileCandidates(file, descriptor, parent, target); len(candidates) > 0 {
		entities, err := boundedUniqueEntities(symbolEntities(candidates), maxCandidates, "reference", file.facts.Path)
		return entities, ProjectResolutionSameFile, err
	}
	if parent != "" {
		qualified := parent + "." + target
		candidates := model.targetableRecords(model.symbolsByQualified[descriptor.ID+"\x00"+qualified])
		candidates = excludePath(candidates, file.pathKey)
		if len(candidates) > 0 {
			entities, err := boundedUniqueEntities(symbolEntities(candidates), maxCandidates, "reference", file.facts.Path)
			return entities, ProjectResolutionSameModule, err
		}
	}

	if candidates := model.explicitImportCandidates(file, descriptor, relation.Target); len(candidates) > 0 {
		entities, err := boundedUniqueEntities(symbolEntities(candidates), maxCandidates, "reference", file.facts.Path)
		return entities, ProjectResolutionExplicitImport, err
	}
	if candidates := model.dependencyExportCandidates(file, descriptor, target); len(candidates) > 0 {
		entities, err := boundedUniqueEntities(symbolEntities(candidates), maxCandidates, "reference", file.facts.Path)
		return entities, ProjectResolutionDependencyExport, err
	}

	var candidates []projectSymbolRecord
	if strings.Contains(target, ".") {
		candidates = model.targetableRecords(model.symbolsByQualified[descriptor.ID+"\x00"+target])
	} else {
		candidates = model.targetableRecords(model.symbolsByName[descriptor.ID+"\x00"+target])
	}
	if len(candidates) == 0 {
		return nil, ProjectResolutionNone, nil
	}
	entities, err := boundedUniqueEntities(symbolEntities(candidates), maxCandidates, "reference", file.facts.Path)
	return entities, ProjectResolutionProject, err
}

func (model *ProjectModel) sameFileCandidates(file projectFileRecord, descriptor LanguageDescriptor, parent, target string) []projectSymbolRecord {
	if strings.Contains(target, ".") {
		return model.targetableRecords(filterRecordsByQualified(model.symbolsByFile[file.pathKey], descriptor.ID, target))
	}
	if parent != "" {
		qualified := parent + "." + target
		return model.targetableRecords(filterRecordsByQualified(model.symbolsByFile[file.pathKey], descriptor.ID, qualified))
	}
	return model.targetableRecords(filterRecordsByName(model.symbolsByFile[file.pathKey], descriptor.ID, target))
}

func (model *ProjectModel) explicitImportCandidates(file projectFileRecord, descriptor LanguageDescriptor, rawTarget string) []projectSymbolRecord {
	target := projectSymbolLookupKey(descriptor, rawTarget)
	for _, dependency := range file.facts.Analysis.Dependencies {
		binding := strings.TrimSpace(dependency.Alias)
		if binding == "" {
			binding = projectDependencyLeaf(dependency.Value)
		}
		if projectSymbolLookupKey(descriptor, binding) != target {
			continue
		}
		dependencyValue := projectSymbolLookupKey(descriptor, dependency.Value)
		if records := model.targetableRecords(model.symbolsByQualified[descriptor.ID+"\x00"+dependencyValue]); len(records) > 0 {
			return records
		}
		var records []projectSymbolRecord
		for _, edge := range model.dependenciesBySource[file.pathKey] {
			if edge.Dependency.Value != dependency.Value || edge.Dependency.Alias != dependency.Alias {
				continue
			}
			for _, targetFile := range edge.Targets {
				for _, record := range model.symbolsByFile[projectPathKey(targetFile.Path)] {
					if record.languageID == descriptor.ID && isRelationEndpointKind(record.symbol.Kind) && record.nameKey == projectSymbolLookupKey(descriptor, projectDependencyLeaf(dependency.Value)) {
						records = append(records, record)
					}
				}
			}
		}
		if len(records) > 0 {
			sortProjectSymbolRecords(records)
			return records
		}
	}
	return nil
}

func (model *ProjectModel) dependencyExportCandidates(file projectFileRecord, descriptor LanguageDescriptor, target string) []projectSymbolRecord {
	seen := make(map[string]projectSymbolRecord)
	for _, dependency := range model.dependenciesBySource[file.pathKey] {
		if dependency.Resolution != ResolutionResolved && dependency.Resolution != ResolutionAmbiguous {
			continue
		}
		for _, targetFile := range dependency.Targets {
			for _, record := range model.symbolsByFile[projectPathKey(targetFile.Path)] {
				if record.languageID != descriptor.ID || !isRelationEndpointKind(record.symbol.Kind) {
					continue
				}
				if record.nameKey == target || record.qualifiedKey == target || strings.HasSuffix(record.qualifiedKey, "."+target) {
					seen[record.symbol.ID] = record
				}
			}
		}
	}
	result := make([]projectSymbolRecord, 0, len(seen))
	for _, record := range seen {
		result = append(result, record)
	}
	sortProjectSymbolRecords(result)
	return result
}

func (model *ProjectModel) targetableRecords(records []projectSymbolRecord) []projectSymbolRecord {
	result := make([]projectSymbolRecord, 0, len(records))
	for _, record := range records {
		if isRelationEndpointKind(record.symbol.Kind) {
			result = append(result, record)
		}
	}
	return result
}

// References returns a defensive deterministic snapshot of all structural
// reference facts in the project model.
func (model *ProjectModel) References() []ProjectReference {
	if model == nil {
		return nil
	}
	return cloneProjectReferences(model.references)
}

// ReferencesTo returns all resolved or ambiguous references that include the
// requested symbol among their candidate definitions.
func (model *ProjectModel) ReferencesTo(symbolID string) []ProjectReference {
	if model == nil {
		return nil
	}
	return cloneProjectReferences(model.referencesByTarget[strings.TrimSpace(symbolID)])
}

// Definitions returns the deterministic candidate definitions for one project
// reference ID. Ambiguous references deliberately return every retained candidate.
func (model *ProjectModel) Definitions(referenceID string) []RelationEntity {
	if model == nil {
		return nil
	}
	return cloneRelationEntities(model.definitionsByReference[strings.TrimSpace(referenceID)])
}

// AllDependencies returns every dependency edge in deterministic source order.
func (model *ProjectModel) AllDependencies() []ProjectDependency {
	if model == nil {
		return nil
	}
	return cloneProjectDependencies(model.dependencies)
}

// Dependencies returns dependency edges originating in path.
func (model *ProjectModel) Dependencies(path string) []ProjectDependency {
	if model == nil {
		return nil
	}
	return cloneProjectDependencies(model.dependenciesBySource[projectPathKey(path)])
}

// Dependents returns dependency edges that can resolve to path.
func (model *ProjectModel) Dependents(path string) []ProjectDependency {
	if model == nil {
		return nil
	}
	return cloneProjectDependencies(model.dependentsByTarget[projectPathKey(path)])
}

func relationEntityForSymbol(symbol NormalizedSymbol, fingerprint string) RelationEntity {
	return RelationEntity{
		Path: symbol.Path, Language: symbol.Language, SymbolID: symbol.ID, QualifiedName: symbol.QualifiedName,
		SourceFingerprint: fingerprint, Range: cloneRangeValue(symbol.DeclarationRange),
	}
}

func (model *ProjectModel) fileEntity(pathKey string) RelationEntity {
	file := model.files[pathKey]
	return RelationEntity{Path: file.facts.Path, Language: file.languageID, SourceFingerprint: file.facts.SourceFingerprint}
}

func fileEntitiesForSymbols(model *ProjectModel, records []projectSymbolRecord) []RelationEntity {
	seen := make(map[string]RelationEntity)
	for _, record := range records {
		seen[record.pathKey] = model.fileEntity(record.pathKey)
	}
	return sortedEntityMap(seen)
}

func symbolEntities(records []projectSymbolRecord) []RelationEntity {
	result := make([]RelationEntity, 0, len(records))
	for _, record := range records {
		result = append(result, record.entity)
	}
	return result
}

func boundedUniqueEntities(values []RelationEntity, maximum int, label, sourcePath string) ([]RelationEntity, error) {
	seen := make(map[string]RelationEntity, len(values))
	for _, value := range values {
		key := value.Path + "\x00" + value.SymbolID + "\x00" + value.QualifiedName
		seen[key] = value
	}
	if len(seen) > maximum {
		return nil, operation.Wrap(operation.KindLimit, "resolve_project_"+label, sourcePath, fmt.Errorf("candidate count %d exceeds limit %d", len(seen), maximum))
	}
	result := make([]RelationEntity, 0, len(seen))
	for _, value := range seen {
		result = append(result, value)
	}
	sortRelationEntities(result)
	return result, nil
}

func sortedEntityMap(values map[string]RelationEntity) []RelationEntity {
	result := make([]RelationEntity, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sortRelationEntities(result)
	return result
}

func sortRelationEntities(values []RelationEntity) {
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i], values[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.QualifiedName != right.QualifiedName {
			return left.QualifiedName < right.QualifiedName
		}
		return left.SymbolID < right.SymbolID
	})
}

func sortProjectSymbolRecords(values []projectSymbolRecord) {
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i], values[j]
		if left.entity.Path != right.entity.Path {
			return left.entity.Path < right.entity.Path
		}
		if left.entity.QualifiedName != right.entity.QualifiedName {
			return left.entity.QualifiedName < right.entity.QualifiedName
		}
		return left.entity.SymbolID < right.entity.SymbolID
	})
}

func sortProjectDependencies(values []ProjectDependency) {
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i], values[j]
		if left.Source.Path != right.Source.Path {
			return left.Source.Path < right.Source.Path
		}
		if comparePosition(left.Dependency.Range.Start, right.Dependency.Range.Start) != 0 {
			return comparePosition(left.Dependency.Range.Start, right.Dependency.Range.Start) < 0
		}
		if left.Dependency.Value != right.Dependency.Value {
			return left.Dependency.Value < right.Dependency.Value
		}
		return left.Dependency.Alias < right.Dependency.Alias
	})
}

func sortProjectReferences(values []ProjectReference) {
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i], values[j]
		if left.Source.Path != right.Source.Path {
			return left.Source.Path < right.Source.Path
		}
		if comparePosition(left.Range.Start, right.Range.Start) != 0 {
			return comparePosition(left.Range.Start, right.Range.Start) < 0
		}
		if left.StructuralKind != right.StructuralKind {
			return left.StructuralKind < right.StructuralKind
		}
		if left.TargetSpelling != right.TargetSpelling {
			return left.TargetSpelling < right.TargetSpelling
		}
		return left.ID < right.ID
	})
}

func comparePosition(left, right Position) int {
	if left.Line < right.Line {
		return -1
	}
	if left.Line > right.Line {
		return 1
	}
	if left.Column < right.Column {
		return -1
	}
	if left.Column > right.Column {
		return 1
	}
	return 0
}

func filterRecordsByQualified(values []projectSymbolRecord, languageID, qualified string) []projectSymbolRecord {
	result := make([]projectSymbolRecord, 0, len(values))
	for _, record := range values {
		if record.languageID == languageID && record.qualifiedKey == qualified {
			result = append(result, record)
		}
	}
	return result
}

func filterRecordsByName(values []projectSymbolRecord, languageID, name string) []projectSymbolRecord {
	result := make([]projectSymbolRecord, 0, len(values))
	for _, record := range values {
		if record.languageID == languageID && record.nameKey == name {
			result = append(result, record)
		}
	}
	return result
}

func excludePath(values []projectSymbolRecord, pathKey string) []projectSymbolRecord {
	result := make([]projectSymbolRecord, 0, len(values))
	for _, value := range values {
		if value.pathKey != pathKey {
			result = append(result, value)
		}
	}
	return result
}

func isRelationEndpointKind(kind SymbolKind) bool {
	switch kind {
	case SymbolKindModule, SymbolKindNamespace, SymbolKindClass, SymbolKindStruct, SymbolKindInterface,
		SymbolKindEnum, SymbolKindRecord, SymbolKindType, SymbolKindAlias, SymbolKindTrait:
		return true
	default:
		return false
	}
}

func isModuleLikeSymbol(kind SymbolKind) bool {
	return kind == SymbolKindModule || kind == SymbolKindNamespace || kind == SymbolKindPackage
}

func projectSymbolLookupKey(descriptor LanguageDescriptor, value string) string {
	value = normalizeProjectReference(value)
	if descriptor.Capabilities.CaseInsensitive {
		value = strings.ToLower(value)
	}
	return value
}

func normalizeProjectReference(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "()")
	value = stripBalancedGenericArguments(value)
	value = strings.ReplaceAll(value, "::", ".")
	value = strings.ReplaceAll(value, "\\", ".")
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"crate.", "self.", "global."} {
		value = strings.TrimPrefix(value, prefix)
	}
	value = strings.Trim(value, ".")
	return value
}

func stripBalancedGenericArguments(value string) string {
	var builder strings.Builder
	depth := 0
	for _, char := range value {
		switch char {
		case '<':
			depth++
		case '>':
			if depth == 0 {
				return strings.TrimSpace(value)
			}
			depth--
		default:
			if depth == 0 {
				builder.WriteRune(char)
			}
		}
	}
	if depth != 0 {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(builder.String())
}

func qualifiedParent(value string) string {
	index := strings.LastIndex(value, ".")
	if index <= 0 {
		return ""
	}
	return value[:index]
}

func dependencyLooksPathLike(value string, kind StructuralDependencyKind) bool {
	value = strings.TrimSpace(value)
	return kind == StructuralDependencyInclude || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || strings.HasPrefix(value, `.\\`) || strings.HasPrefix(value, `..\\`) || strings.Contains(value, "/") || strings.Contains(value, "\\")
}

func dependencyLooksQualified(value string) bool {
	return strings.Contains(value, ".") || strings.Contains(value, "::") || strings.Contains(value, "\\") || strings.Contains(value, "{")
}

func projectDependencyLeaf(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'")
	value = strings.ReplaceAll(value, "::", ".")
	value = strings.ReplaceAll(value, "\\", ".")
	value = strings.TrimSuffix(value, ".*")
	if open := strings.LastIndex(value, "{"); open >= 0 {
		value = value[:open]
	}
	value = strings.TrimRight(value, ".")
	if index := strings.LastIndex(value, "/"); index >= 0 {
		value = value[index+1:]
	}
	if index := strings.LastIndex(value, "."); index >= 0 {
		value = value[index+1:]
	}
	return strings.TrimSpace(value)
}

func projectPathKey(value string) string {
	key := filepath.Clean(strings.TrimSpace(value))
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}

func projectPathStemKey(value string) string {
	clean := filepath.Clean(strings.TrimSpace(value))
	extension := filepath.Ext(clean)
	if extension != "" {
		clean = strings.TrimSuffix(clean, extension)
	}
	return projectPathKey(clean)
}

func validProjectFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func deterministicProjectReferenceID(reference ProjectReference) string {
	hash := sha256.New()
	parts := []string{
		reference.Source.Path, reference.Source.Language, reference.Source.SymbolID, reference.Source.QualifiedName,
		reference.StructuralKind, reference.TargetSpelling,
		fmt.Sprintf("%d", reference.Range.Start.Line), fmt.Sprintf("%d", reference.Range.Start.Column),
		fmt.Sprintf("%d", reference.Range.End.Line), fmt.Sprintf("%d", reference.Range.End.Column),
	}
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func rangeContainsPosition(value Range, position Position) bool {
	return comparePosition(value.Start, position) <= 0 && comparePosition(position, value.End) < 0
}

func cloneRangeValue(value Range) *Range {
	copy := value
	return &copy
}

func cloneRelationEntities(values []RelationEntity) []RelationEntity {
	result := make([]RelationEntity, len(values))
	for index, value := range values {
		result[index] = value
		if value.Range != nil {
			rangeCopy := *value.Range
			result[index].Range = &rangeCopy
		}
	}
	return result
}

func cloneProjectReferences(values []ProjectReference) []ProjectReference {
	result := make([]ProjectReference, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Source = cloneRelationEntity(value.Source)
		result[index].Targets = cloneRelationEntities(value.Targets)
	}
	return result
}

func cloneProjectDependencies(values []ProjectDependency) []ProjectDependency {
	result := make([]ProjectDependency, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Source = cloneRelationEntity(value.Source)
		result[index].Targets = cloneRelationEntities(value.Targets)
	}
	return result
}

func cloneRelationEntity(value RelationEntity) RelationEntity {
	if value.Range != nil {
		rangeCopy := *value.Range
		value.Range = &rangeCopy
	}
	return value
}
