package sourceintelligence

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/zoster81/scripthold/internal/operation"
)

const projectIndexFingerprintVersion = "scripthold:r27-project-index-generation-v1"

// ProjectIndexAnalysisConfig contains only limits/options that can change
// detection or analyzer facts retained by a project generation.
type ProjectIndexAnalysisConfig struct {
	MaxFileBytes         int64
	MaxDecodedCharacters int
	MaxSymbols           int
	MaxSignatureBytes    int
	MaxDiagnostics       int
	MaxDetectorProbes    int
	MaxNesting           int
	MaxProjectEdges      int
	IncludeSignatures    bool
}

// ProjectIndexAnalysisFingerprint binds cached facts to the complete immutable
// language-routing/analyzer registry and the analysis options that affect output.
func ProjectIndexAnalysisFingerprint(registry *LanguageRegistry, config ProjectIndexAnalysisConfig) (string, error) {
	if registry == nil {
		return "", operation.New(operation.KindInvalidInput, "language registry is required for project index fingerprinting")
	}
	hasher := sha256.New()
	writeProjectIndexHashPart(hasher, "scripthold:r27-project-index-analysis-v1")
	writeProjectIndexHashPart(hasher, fmt.Sprintf("%d", config.MaxFileBytes))
	writeProjectIndexHashPart(hasher, fmt.Sprintf("%d", config.MaxDecodedCharacters))
	writeProjectIndexHashPart(hasher, fmt.Sprintf("%d", config.MaxSymbols))
	writeProjectIndexHashPart(hasher, fmt.Sprintf("%d", config.MaxSignatureBytes))
	writeProjectIndexHashPart(hasher, fmt.Sprintf("%d", config.MaxDiagnostics))
	writeProjectIndexHashPart(hasher, fmt.Sprintf("%d", config.MaxDetectorProbes))
	writeProjectIndexHashPart(hasher, fmt.Sprintf("%d", config.MaxNesting))
	writeProjectIndexHashPart(hasher, fmt.Sprintf("%d", config.MaxProjectEdges))
	writeProjectIndexHashPart(hasher, fmt.Sprintf("%t", config.IncludeSignatures))
	ids := make([]string, 0, len(registry.byID))
	for id := range registry.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		descriptor := registry.byID[id]
		writeProjectIndexHashPart(hasher, descriptor.ID)
		writeProjectIndexHashStrings(hasher, descriptor.Aliases)
		writeProjectIndexHashStrings(hasher, descriptor.ExactBasenames)
		writeProjectIndexHashStrings(hasher, descriptor.CompoundSuffixes)
		writeProjectIndexHashStrings(hasher, descriptor.Extensions)
		writeProjectIndexHashStrings(hasher, descriptor.AmbiguousExtensions)
		writeProjectIndexHashStrings(hasher, descriptor.ShebangInterpreters)
		writeProjectIndexHashPart(hasher, string(descriptor.Analyzer))
		writeProjectIndexHashPart(hasher, fmt.Sprintf("%+v", descriptor.Capabilities))
		writeProjectIndexHashPart(hasher, descriptor.Family)
		for _, evidence := range descriptor.DetectionEvidence {
			writeProjectIndexHashPart(hasher, string(evidence))
		}
		writeProjectIndexHashPart(hasher, descriptor.ScannerProfile)
		writeProjectIndexHashPart(hasher, descriptor.CompositeBehavior)
		writeProjectIndexHashPart(hasher, descriptor.AnalyzerStrategy)
		writeProjectIndexHashPart(hasher, descriptor.AnalyzerVersion)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// ProjectIndexManagerLimits bound process-local project scopes and immutable
// generations. Full source bodies are never retained by the index manager.
type ProjectIndexManagerLimits struct {
	MaxProjects    int
	MaxGenerations int
}

// ProjectIndexFileSnapshot is the authoritative current-source identity supplied
// by the filesystem-owning orchestration layer before incremental reuse occurs.
type ProjectIndexFileSnapshot struct {
	Path              string
	SourceFingerprint string
}

// ProjectIndexUnavailableReason classifies a selected file that cannot safely
// contribute retained facts to the current generation.
type ProjectIndexUnavailableReason string

const (
	ProjectIndexUnavailable ProjectIndexUnavailableReason = "unavailable"
	ProjectIndexNotRegular  ProjectIndexUnavailableReason = "not-regular"
	ProjectIndexOverLimit   ProjectIndexUnavailableReason = "over-limit"
)

// ProjectIndexUnavailableFile retains only path/reason coverage metadata. No
// content, guessed fingerprint, or previous facts survive an unavailable input.
type ProjectIndexUnavailableFile struct {
	Path   string
	Reason ProjectIndexUnavailableReason
}

// ProjectIndexAnalysisResult binds one analyzer result to the source snapshot it
// actually observed. A nil Facts value represents a safely skipped source file.
type ProjectIndexAnalysisResult struct {
	ObservedFingerprint string
	Facts               *ProjectFileFacts
}

// ProjectIndexAnalyzeFunc analyzes one snapshot that cannot be safely reused.
type ProjectIndexAnalyzeFunc func(context.Context, ProjectIndexFileSnapshot) (ProjectIndexAnalysisResult, error)

// ProjectIndexBinding is the transport-neutral form of the public generation /
// fingerprint binding. Empty bindings select the authoritative current generation.
type ProjectIndexBinding struct {
	Generation  *uint64
	Fingerprint string
	AllowStale  bool
}

// ProjectIndexRefreshOptions describe one canonical source-query scope.
type ProjectIndexRefreshOptions struct {
	ScopeFingerprint    string
	AnalysisFingerprint string
	Snapshots           []ProjectIndexFileSnapshot
	Unavailable         []ProjectIndexUnavailableFile
	SelectionTruncated  bool
	ResolverLimits      ProjectResolverLimits
	Binding             ProjectIndexBinding
	Analyze             ProjectIndexAnalyzeFunc
}

// ProjectIndexRefreshStats expose deterministic warm-update work for tests and
// orchestration diagnostics without leaking source contents.
type ProjectIndexRefreshStats struct {
	AnalyzedFiles int
	ReusedFiles   int
}

// ProjectIndexCoverage summarizes the immutable generation's retained facts.
type ProjectIndexCoverage struct {
	FilesConsidered  int
	FilesParsed      int
	FilesSkipped     int
	CoverageComplete bool
	Truncated        bool
}

// ProjectIndexSelection is one coherent immutable generation selected after the
// current scope has been refreshed. Model has no exported mutation operations.
type ProjectIndexSelection struct {
	Evidence IndexEvidence
	Model    *ProjectModel
	Coverage ProjectIndexCoverage
	Stats    ProjectIndexRefreshStats
}

type indexedProjectFile struct {
	snapshot ProjectIndexFileSnapshot
	facts    *ProjectFileFacts
}

type projectIndexGeneration struct {
	evidence            IndexEvidence
	analysisFingerprint string
	files               map[string]*indexedProjectFile
	unavailable         []ProjectIndexUnavailableFile
	model               *ProjectModel
	coverage            ProjectIndexCoverage
}

type projectIndexState struct {
	gate        chan struct{}
	generations []*projectIndexGeneration
	refs        int
	lastUsed    uint64
}

// ProjectIndexManager owns bounded process-local incremental generations. A
// per-project cancellable gate serializes refreshes for the same canonical scope;
// unrelated project scopes can refresh concurrently.
type ProjectIndexManager struct {
	mu             sync.Mutex
	limits         ProjectIndexManagerLimits
	projects       map[string]*projectIndexState
	accessSequence uint64
	nextGeneration uint64
}

func NewProjectIndexManager(limits ProjectIndexManagerLimits) (*ProjectIndexManager, error) {
	if limits.MaxProjects <= 0 || limits.MaxGenerations <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "project index manager limits must be positive")
	}
	return &ProjectIndexManager{limits: limits, projects: make(map[string]*projectIndexState)}, nil
}

// Refresh reuses only fingerprint-identical parsed facts from the same analysis
// configuration, rebuilds all project-level resolver state on every changed
// generation, and publishes only a fully constructed immutable generation.
func (manager *ProjectIndexManager) Refresh(ctx context.Context, registry *LanguageRegistry, options ProjectIndexRefreshOptions) (ProjectIndexSelection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ProjectIndexSelection{}, operation.Wrap(operation.KindCancelled, "refresh_project_index", "", err)
	}
	if manager == nil || registry == nil {
		return ProjectIndexSelection{}, operation.New(operation.KindInvalidInput, "project index manager and language registry are required")
	}
	if !validProjectFingerprint(options.ScopeFingerprint) || !validProjectFingerprint(options.AnalysisFingerprint) {
		return ProjectIndexSelection{}, operation.New(operation.KindInvalidInput, "project index scope and analysis fingerprints must be lowercase SHA-256 digests")
	}
	if options.Analyze == nil {
		return ProjectIndexSelection{}, operation.New(operation.KindInvalidInput, "project index analyze callback is required")
	}
	if err := validateProjectIndexBinding(options.Binding); err != nil {
		return ProjectIndexSelection{}, err
	}
	snapshots, unavailable, err := normalizeProjectIndexInputs(options.Snapshots, options.Unavailable, options.ResolverLimits.MaxFiles)
	if err != nil {
		return ProjectIndexSelection{}, err
	}

	state, err := manager.acquireState(options.ScopeFingerprint)
	if err != nil {
		return ProjectIndexSelection{}, err
	}
	defer manager.releaseState(state)
	select {
	case <-ctx.Done():
		return ProjectIndexSelection{}, operation.Wrap(operation.KindCancelled, "refresh_project_index", "", ctx.Err())
	case <-state.gate:
	}
	defer func() { state.gate <- struct{}{} }()

	var latest *projectIndexGeneration
	if len(state.generations) > 0 {
		latest = state.generations[0]
	}
	forceAnalyze := latest == nil || latest.analysisFingerprint != options.AnalysisFingerprint
	files := make(map[string]*indexedProjectFile, len(snapshots))
	stats := ProjectIndexRefreshStats{}
	changed := forceAnalyze || latest == nil || len(snapshots) != lenGenerationFiles(latest) ||
		!equivalentProjectIndexUnavailable(unavailable, generationUnavailable(latest)) ||
		(latest != nil && latest.coverage.Truncated != options.SelectionTruncated)
	for _, snapshot := range snapshots {
		if err := ctx.Err(); err != nil {
			return ProjectIndexSelection{}, operation.Wrap(operation.KindCancelled, "refresh_project_index", snapshot.Path, err)
		}
		key := projectPathKey(snapshot.Path)
		if !forceAnalyze && latest != nil {
			if previous, ok := latest.files[key]; ok && previous.facts != nil && previous.snapshot.Path == snapshot.Path && previous.snapshot.SourceFingerprint == snapshot.SourceFingerprint {
				files[key] = previous
				stats.ReusedFiles++
				continue
			}
		}

		analysis, analyzeErr := options.Analyze(ctx, snapshot)
		if analyzeErr != nil {
			return ProjectIndexSelection{}, analyzeErr
		}
		stats.AnalyzedFiles++
		if analysis.ObservedFingerprint != snapshot.SourceFingerprint {
			return ProjectIndexSelection{}, operation.Wrap(operation.KindConflict, "refresh_project_index", snapshot.Path, fmt.Errorf("source changed between fingerprint and analysis"))
		}
		current := &indexedProjectFile{snapshot: snapshot}
		if analysis.Facts != nil {
			if projectPathKey(analysis.Facts.Path) != key || analysis.Facts.Path != snapshot.Path || analysis.Facts.SourceFingerprint != snapshot.SourceFingerprint {
				return ProjectIndexSelection{}, operation.Wrap(operation.KindConflict, "refresh_project_index", snapshot.Path, fmt.Errorf("analysis result does not match the requested source snapshot"))
			}
			cloned := cloneProjectFileFactsForIndex(*analysis.Facts)
			current.facts = &cloned
		}
		files[key] = current
		if !changed && !equivalentIndexedProjectFile(latest.files[key], current) {
			changed = true
		}
	}

	currentGeneration := latest
	if changed {
		facts := make([]ProjectFileFacts, 0, len(files))
		for _, snapshot := range snapshots {
			current := files[projectPathKey(snapshot.Path)]
			if current != nil && current.facts != nil {
				facts = append(facts, *current.facts)
			}
		}
		model, buildErr := BuildProjectModel(ctx, registry, facts, options.ResolverLimits)
		if buildErr != nil {
			return ProjectIndexSelection{}, buildErr
		}
		fingerprint := projectIndexGenerationFingerprint(options.ScopeFingerprint, options.AnalysisFingerprint, files, unavailable, options.SelectionTruncated)
		generation, allocationErr := manager.allocateGeneration()
		if allocationErr != nil {
			return ProjectIndexSelection{}, allocationErr
		}
		currentGeneration = &projectIndexGeneration{
			evidence:            IndexEvidence{Generation: generation, Fingerprint: fingerprint, Staleness: IndexCurrent},
			analysisFingerprint: options.AnalysisFingerprint,
			files:               files,
			unavailable:         unavailable,
			model:               model,
			coverage:            projectIndexGenerationCoverage(files, unavailable, options.SelectionTruncated),
		}
		state.generations = append([]*projectIndexGeneration{currentGeneration}, state.generations...)
		if len(state.generations) > manager.limits.MaxGenerations {
			state.generations = state.generations[:manager.limits.MaxGenerations]
		}
	}
	if currentGeneration == nil {
		return ProjectIndexSelection{}, operation.New(operation.KindUnknown, "project index refresh produced no generation")
	}
	selected, stale, err := selectProjectIndexGeneration(state.generations, currentGeneration, options.Binding)
	if err != nil {
		return ProjectIndexSelection{}, err
	}
	evidence := selected.evidence
	if stale {
		evidence.Staleness = IndexStale
	} else {
		evidence.Staleness = IndexCurrent
	}
	return ProjectIndexSelection{Evidence: evidence, Model: selected.model, Coverage: selected.coverage, Stats: stats}, nil
}

func (manager *ProjectIndexManager) acquireState(scope string) (*projectIndexState, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.accessSequence++
	if state := manager.projects[scope]; state != nil {
		state.refs++
		state.lastUsed = manager.accessSequence
		return state, nil
	}
	if len(manager.projects) >= manager.limits.MaxProjects {
		var victimKey string
		var victim *projectIndexState
		for key, candidate := range manager.projects {
			if candidate.refs != 0 {
				continue
			}
			if victim == nil || candidate.lastUsed < victim.lastUsed || (candidate.lastUsed == victim.lastUsed && key < victimKey) {
				victimKey, victim = key, candidate
			}
		}
		if victim == nil {
			return nil, operation.New(operation.KindLimit, "all bounded project index scopes are currently in use")
		}
		delete(manager.projects, victimKey)
	}
	state := &projectIndexState{gate: make(chan struct{}, 1), refs: 1, lastUsed: manager.accessSequence}
	state.gate <- struct{}{}
	manager.projects[scope] = state
	return state, nil
}

func (manager *ProjectIndexManager) releaseState(state *projectIndexState) {
	manager.mu.Lock()
	if state.refs > 0 {
		state.refs--
	}
	manager.mu.Unlock()
}

func (manager *ProjectIndexManager) allocateGeneration() (uint64, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.nextGeneration == ^uint64(0) {
		return 0, operation.New(operation.KindLimit, "project index generation counter is exhausted")
	}
	manager.nextGeneration++
	return manager.nextGeneration, nil
}

func normalizeProjectIndexInputs(snapshotInput []ProjectIndexFileSnapshot, unavailableInput []ProjectIndexUnavailableFile, maxFiles int) ([]ProjectIndexFileSnapshot, []ProjectIndexUnavailableFile, error) {
	if maxFiles <= 0 {
		return nil, nil, operation.New(operation.KindInvalidInput, "project resolver file limit must be positive")
	}
	if len(snapshotInput)+len(unavailableInput) > maxFiles {
		return nil, nil, operation.Wrap(operation.KindLimit, "refresh_project_index", "", fmt.Errorf("file count %d exceeds limit %d", len(snapshotInput)+len(unavailableInput), maxFiles))
	}
	snapshots := append([]ProjectIndexFileSnapshot(nil), snapshotInput...)
	sort.Slice(snapshots, func(i, j int) bool { return projectPathKey(snapshots[i].Path) < projectPathKey(snapshots[j].Path) })
	seen := make(map[string]struct{}, len(snapshotInput)+len(unavailableInput))
	for _, snapshot := range snapshots {
		if strings.TrimSpace(snapshot.Path) == "" || !validProjectFingerprint(snapshot.SourceFingerprint) {
			return nil, nil, operation.New(operation.KindInvalidInput, "project index snapshots require path and lowercase SHA-256 source fingerprint")
		}
		key := projectPathKey(snapshot.Path)
		if _, exists := seen[key]; exists {
			return nil, nil, operation.Wrap(operation.KindInvalidInput, "refresh_project_index", snapshot.Path, fmt.Errorf("duplicate project index path"))
		}
		seen[key] = struct{}{}
	}
	unavailable := append([]ProjectIndexUnavailableFile(nil), unavailableInput...)
	sort.Slice(unavailable, func(i, j int) bool {
		left, right := projectPathKey(unavailable[i].Path), projectPathKey(unavailable[j].Path)
		if left != right {
			return left < right
		}
		return unavailable[i].Reason < unavailable[j].Reason
	})
	for _, current := range unavailable {
		if strings.TrimSpace(current.Path) == "" || !validProjectIndexUnavailableReason(current.Reason) {
			return nil, nil, operation.New(operation.KindInvalidInput, "project index unavailable inputs require path and supported reason")
		}
		key := projectPathKey(current.Path)
		if _, exists := seen[key]; exists {
			return nil, nil, operation.Wrap(operation.KindInvalidInput, "refresh_project_index", current.Path, fmt.Errorf("duplicate project index path"))
		}
		seen[key] = struct{}{}
	}
	return snapshots, unavailable, nil
}

func validProjectIndexUnavailableReason(reason ProjectIndexUnavailableReason) bool {
	switch reason {
	case ProjectIndexUnavailable, ProjectIndexNotRegular, ProjectIndexOverLimit:
		return true
	default:
		return false
	}
}

func validateProjectIndexBinding(binding ProjectIndexBinding) error {
	if binding.Generation != nil && *binding.Generation == 0 {
		return operation.New(operation.KindInvalidInput, "project index generation must be positive")
	}
	if binding.Fingerprint != "" && !validProjectFingerprint(binding.Fingerprint) {
		return operation.New(operation.KindInvalidInput, "project index fingerprint must be a lowercase SHA-256 digest")
	}
	return nil
}

func selectProjectIndexGeneration(generations []*projectIndexGeneration, current *projectIndexGeneration, binding ProjectIndexBinding) (*projectIndexGeneration, bool, error) {
	if binding.Generation == nil && binding.Fingerprint == "" {
		return current, false, nil
	}
	for _, generation := range generations {
		if binding.Generation != nil && generation.evidence.Generation != *binding.Generation {
			continue
		}
		if binding.Fingerprint != "" && generation.evidence.Fingerprint != binding.Fingerprint {
			continue
		}
		if generation == current {
			return generation, false, nil
		}
		if !binding.AllowStale {
			return nil, false, operation.New(operation.KindConflict, "requested project index generation is stale")
		}
		return generation, true, nil
	}
	return nil, false, operation.New(operation.KindConflict, "requested project index generation is unavailable")
}

func lenGenerationFiles(generation *projectIndexGeneration) int {
	if generation == nil {
		return 0
	}
	return len(generation.files)
}

func generationUnavailable(generation *projectIndexGeneration) []ProjectIndexUnavailableFile {
	if generation == nil {
		return nil
	}
	return generation.unavailable
}

func equivalentProjectIndexUnavailable(left, right []ProjectIndexUnavailableFile) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equivalentIndexedProjectFile(left, right *indexedProjectFile) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.snapshot != right.snapshot || (left.facts == nil) != (right.facts == nil) {
		return false
	}
	if left.facts == nil {
		return true
	}
	return left.facts.Path == right.facts.Path && left.facts.Language == right.facts.Language && left.facts.SourceFingerprint == right.facts.SourceFingerprint &&
		left.facts.Analysis.Analysis.CoverageComplete == right.facts.Analysis.Analysis.CoverageComplete
}

func projectIndexGenerationCoverage(files map[string]*indexedProjectFile, unavailable []ProjectIndexUnavailableFile, truncated bool) ProjectIndexCoverage {
	coverage := ProjectIndexCoverage{
		FilesConsidered:  len(files) + len(unavailable),
		FilesSkipped:     len(unavailable),
		CoverageComplete: len(unavailable) == 0 && !truncated,
		Truncated:        truncated,
	}
	for _, file := range files {
		if file == nil || file.facts == nil {
			coverage.FilesSkipped++
			coverage.CoverageComplete = false
			continue
		}
		coverage.FilesParsed++
		if !file.facts.Analysis.Analysis.CoverageComplete {
			coverage.CoverageComplete = false
		}
	}
	return coverage
}

func projectIndexGenerationFingerprint(scopeFingerprint, analysisFingerprint string, files map[string]*indexedProjectFile, unavailable []ProjectIndexUnavailableFile, truncated bool) string {
	hasher := sha256.New()
	writeProjectIndexHashPart(hasher, projectIndexFingerprintVersion)
	writeProjectIndexHashPart(hasher, scopeFingerprint)
	writeProjectIndexHashPart(hasher, analysisFingerprint)
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		file := files[key]
		writeProjectIndexHashPart(hasher, key)
		writeProjectIndexHashPart(hasher, file.snapshot.SourceFingerprint)
		if file.facts == nil {
			writeProjectIndexHashPart(hasher, "skipped")
			continue
		}
		writeProjectIndexHashPart(hasher, "parsed")
		writeProjectIndexHashPart(hasher, file.facts.Language)
		if file.facts.Analysis.Analysis.CoverageComplete {
			writeProjectIndexHashPart(hasher, "complete")
		} else {
			writeProjectIndexHashPart(hasher, "partial")
		}
	}
	for _, current := range unavailable {
		writeProjectIndexHashPart(hasher, "unavailable")
		writeProjectIndexHashPart(hasher, projectPathKey(current.Path))
		writeProjectIndexHashPart(hasher, string(current.Reason))
	}
	writeProjectIndexHashPart(hasher, fmt.Sprintf("truncated:%t", truncated))
	return hex.EncodeToString(hasher.Sum(nil))
}

func writeProjectIndexHashPart(hasher interface{ Write([]byte) (int, error) }, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = hasher.Write(length[:])
	_, _ = hasher.Write([]byte(value))
}

func writeProjectIndexHashStrings(hasher interface{ Write([]byte) (int, error) }, values []string) {
	writeProjectIndexHashPart(hasher, fmt.Sprintf("%d", len(values)))
	for _, value := range values {
		writeProjectIndexHashPart(hasher, value)
	}
}

func cloneProjectFileFactsForIndex(input ProjectFileFacts) ProjectFileFacts {
	output := input
	output.Analysis.Analysis.Symbols = append([]NormalizedSymbol(nil), input.Analysis.Analysis.Symbols...)
	for index := range output.Analysis.Analysis.Symbols {
		symbol := &output.Analysis.Analysis.Symbols[index]
		symbol.Modifiers = append([]string(nil), symbol.Modifiers...)
		if symbol.SignatureRange != nil {
			copy := *symbol.SignatureRange
			symbol.SignatureRange = &copy
		}
		if symbol.BodyRange != nil {
			copy := *symbol.BodyRange
			symbol.BodyRange = &copy
		}
		if symbol.signatureOffsets != nil {
			copy := *symbol.signatureOffsets
			symbol.signatureOffsets = &copy
		}
		if symbol.bodyOffsets != nil {
			copy := *symbol.bodyOffsets
			symbol.bodyOffsets = &copy
		}
	}
	output.Analysis.Analysis.Diagnostics = append([]AnalysisDiagnostic(nil), input.Analysis.Analysis.Diagnostics...)
	for index := range output.Analysis.Analysis.Diagnostics {
		if output.Analysis.Analysis.Diagnostics[index].Range != nil {
			copy := *output.Analysis.Analysis.Diagnostics[index].Range
			output.Analysis.Analysis.Diagnostics[index].Range = &copy
		}
	}
	output.Analysis.Dependencies = append([]StructuralDependency(nil), input.Analysis.Dependencies...)
	output.Analysis.Relations = append([]StructuralRelation(nil), input.Analysis.Relations...)
	output.Analysis.Regions = append([]SourceRegion(nil), input.Analysis.Regions...)
	return output
}
