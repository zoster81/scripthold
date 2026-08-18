package sourceintelligence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestProjectIndexAnalysisFingerprintIncludesFactAffectingLimits(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	base := ProjectIndexAnalysisConfig{
		MaxFileBytes: 1024, MaxDecodedCharacters: 2048, MaxSymbols: 64, MaxSignatureBytes: 256,
		MaxDiagnostics: 16, MaxDetectorProbes: 8, MaxNesting: 32, MaxProjectEdges: 128,
	}
	first, err := ProjectIndexAnalysisFingerprint(registry, base)
	if err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.MaxDecodedCharacters++
	second, err := ProjectIndexAnalysisFingerprint(registry, changed)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("MaxDecodedCharacters change did not invalidate analysis fingerprint")
	}
}

func TestProjectIndexReusesOnlyUnchangedParsedFacts(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase15")
	aPath := filepath.Join(root, "a.ts")
	bPath := filepath.Join(root, "b.ts")
	aV1 := indexFacts(t, registry, aPath, "typescript", "export class A {}\n")
	bV1 := indexFacts(t, registry, bPath, "typescript", "export class B {}\n")
	bV2 := indexFacts(t, registry, bPath, "typescript", "export class B2 {}\n")

	manager, err := NewProjectIndexManager(ProjectIndexManagerLimits{MaxProjects: 2, MaxGenerations: 2})
	if err != nil {
		t.Fatal(err)
	}
	facts := map[string]ProjectFileFacts{aPath: aV1, bPath: bV1}
	calls := make(map[string]int)
	analyze := func(_ context.Context, snapshot ProjectIndexFileSnapshot) (ProjectIndexAnalysisResult, error) {
		calls[snapshot.Path]++
		fact, ok := facts[snapshot.Path]
		if !ok {
			t.Fatalf("unexpected analysis path %q", snapshot.Path)
		}
		return ProjectIndexAnalysisResult{ObservedFingerprint: fact.SourceFingerprint, Facts: &fact}, nil
	}
	refresh := func(analysisFingerprint string, snapshots []ProjectIndexFileSnapshot) ProjectIndexSelection {
		t.Helper()
		selection, refreshErr := manager.Refresh(context.Background(), registry, ProjectIndexRefreshOptions{
			ScopeFingerprint:    indexDigest("scope"),
			AnalysisFingerprint: analysisFingerprint,
			Snapshots:           snapshots,
			ResolverLimits:      indexResolverLimits(),
			Analyze:             analyze,
		})
		if refreshErr != nil {
			t.Fatal(refreshErr)
		}
		return selection
	}

	initialSnapshots := []ProjectIndexFileSnapshot{
		{Path: aPath, SourceFingerprint: aV1.SourceFingerprint},
		{Path: bPath, SourceFingerprint: bV1.SourceFingerprint},
	}
	first := refresh(indexDigest("analysis-v1"), initialSnapshots)
	if first.Evidence.Generation == 0 || first.Evidence.Staleness != IndexCurrent {
		t.Fatalf("first evidence = %+v", first.Evidence)
	}
	if calls[aPath] != 1 || calls[bPath] != 1 || first.Stats.AnalyzedFiles != 2 || first.Stats.ReusedFiles != 0 {
		t.Fatalf("first calls/stats = %#v / %+v", calls, first.Stats)
	}

	second := refresh(indexDigest("analysis-v1"), initialSnapshots)
	if second.Evidence != first.Evidence {
		t.Fatalf("warm evidence changed: first=%+v second=%+v", first.Evidence, second.Evidence)
	}
	if calls[aPath] != 1 || calls[bPath] != 1 || second.Stats.AnalyzedFiles != 0 || second.Stats.ReusedFiles != 2 {
		t.Fatalf("warm calls/stats = %#v / %+v", calls, second.Stats)
	}

	facts[bPath] = bV2
	changed := refresh(indexDigest("analysis-v1"), []ProjectIndexFileSnapshot{
		{Path: aPath, SourceFingerprint: aV1.SourceFingerprint},
		{Path: bPath, SourceFingerprint: bV2.SourceFingerprint},
	})
	if changed.Evidence.Generation == first.Evidence.Generation || changed.Evidence.Fingerprint == first.Evidence.Fingerprint {
		t.Fatalf("changed file did not create a new generation: first=%+v changed=%+v", first.Evidence, changed.Evidence)
	}
	if calls[aPath] != 1 || calls[bPath] != 2 || changed.Stats.AnalyzedFiles != 1 || changed.Stats.ReusedFiles != 1 {
		t.Fatalf("changed calls/stats = %#v / %+v", calls, changed.Stats)
	}

	configurationChanged := refresh(indexDigest("analysis-v2"), []ProjectIndexFileSnapshot{
		{Path: aPath, SourceFingerprint: aV1.SourceFingerprint},
		{Path: bPath, SourceFingerprint: bV2.SourceFingerprint},
	})
	if configurationChanged.Evidence.Generation == changed.Evidence.Generation {
		t.Fatalf("analysis configuration change reused generation %+v", configurationChanged.Evidence)
	}
	if calls[aPath] != 2 || calls[bPath] != 3 || configurationChanged.Stats.AnalyzedFiles != 2 || configurationChanged.Stats.ReusedFiles != 0 {
		t.Fatalf("configuration calls/stats = %#v / %+v", calls, configurationChanged.Stats)
	}
}

func TestProjectIndexBindingRetentionAndStaleness(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("project", "phase15", "binding.ts")
	versions := []ProjectFileFacts{
		indexFacts(t, registry, path, "typescript", "export class V1 {}\n"),
		indexFacts(t, registry, path, "typescript", "export class V2 {}\n"),
		indexFacts(t, registry, path, "typescript", "export class V3 {}\n"),
	}
	manager, err := NewProjectIndexManager(ProjectIndexManagerLimits{MaxProjects: 1, MaxGenerations: 2})
	if err != nil {
		t.Fatal(err)
	}
	current := versions[0]
	analyze := func(_ context.Context, snapshot ProjectIndexFileSnapshot) (ProjectIndexAnalysisResult, error) {
		fact := current
		return ProjectIndexAnalysisResult{ObservedFingerprint: snapshot.SourceFingerprint, Facts: &fact}, nil
	}
	refresh := func(binding ProjectIndexBinding) (ProjectIndexSelection, error) {
		return manager.Refresh(context.Background(), registry, ProjectIndexRefreshOptions{
			ScopeFingerprint:    indexDigest("binding-scope"),
			AnalysisFingerprint: indexDigest("analysis"),
			Snapshots:           []ProjectIndexFileSnapshot{{Path: path, SourceFingerprint: current.SourceFingerprint}},
			ResolverLimits:      indexResolverLimits(),
			Binding:             binding,
			Analyze:             analyze,
		})
	}

	first, err := refresh(ProjectIndexBinding{})
	if err != nil {
		t.Fatal(err)
	}
	current = versions[1]
	second, err := refresh(ProjectIndexBinding{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Evidence.Generation == first.Evidence.Generation {
		t.Fatal("second source version did not advance generation")
	}

	firstGeneration := first.Evidence.Generation
	if _, err := refresh(ProjectIndexBinding{Generation: &firstGeneration}); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("stale reject error = %v kind=%v", err, operation.KindOf(err))
	}
	allowed, err := refresh(ProjectIndexBinding{Generation: &firstGeneration, Fingerprint: first.Evidence.Fingerprint, AllowStale: true})
	if err != nil {
		t.Fatal(err)
	}
	if allowed.Evidence.Generation != first.Evidence.Generation || allowed.Evidence.Fingerprint != first.Evidence.Fingerprint || allowed.Evidence.Staleness != IndexStale {
		t.Fatalf("allowed stale evidence = %+v, want first generation stale", allowed.Evidence)
	}

	current = versions[2]
	if _, err := refresh(ProjectIndexBinding{}); err != nil {
		t.Fatal(err)
	}
	if _, err := refresh(ProjectIndexBinding{Generation: &firstGeneration, AllowStale: true}); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("evicted generation error = %v kind=%v", err, operation.KindOf(err))
	}
}

func TestProjectIndexSerializesConcurrentRefreshWithoutDuplicateAnalysis(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("project", "phase15", "concurrent.ts")
	fact := indexFacts(t, registry, path, "typescript", "export class Concurrent {}\n")
	manager, err := NewProjectIndexManager(ProjectIndexManagerLimits{MaxProjects: 1, MaxGenerations: 2})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	calls := 0
	analyze := func(_ context.Context, snapshot ProjectIndexFileSnapshot) (ProjectIndexAnalysisResult, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		copy := fact
		return ProjectIndexAnalysisResult{ObservedFingerprint: snapshot.SourceFingerprint, Facts: &copy}, nil
	}
	const workers = 12
	results := make(chan ProjectIndexSelection, workers)
	errors := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			selection, refreshErr := manager.Refresh(context.Background(), registry, ProjectIndexRefreshOptions{
				ScopeFingerprint:    indexDigest("concurrent-scope"),
				AnalysisFingerprint: indexDigest("analysis"),
				Snapshots:           []ProjectIndexFileSnapshot{{Path: path, SourceFingerprint: fact.SourceFingerprint}},
				ResolverLimits:      indexResolverLimits(),
				Analyze:             analyze,
			})
			if refreshErr != nil {
				errors <- refreshErr
				return
			}
			results <- selection
		}()
	}
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var evidence IndexEvidence
	for result := range results {
		if evidence.Generation == 0 {
			evidence = result.Evidence
			continue
		}
		if result.Evidence != evidence {
			t.Fatalf("concurrent refresh mixed generations: first=%+v got=%+v", evidence, result.Evidence)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("analysis calls = %d, want exactly 1", calls)
	}
}

func TestProjectIndexUnavailableCoverageAndAtomicAbort(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("project", "phase15", "unavailable.ts")
	v1 := indexFacts(t, registry, path, "typescript", "export class Available {}\n")
	v2 := indexFacts(t, registry, path, "typescript", "export class Changed {}\n")
	manager, err := NewProjectIndexManager(ProjectIndexManagerLimits{MaxProjects: 1, MaxGenerations: 3})
	if err != nil {
		t.Fatal(err)
	}
	analysisFingerprint := indexDigest("analysis")
	analyzeV1 := func(_ context.Context, snapshot ProjectIndexFileSnapshot) (ProjectIndexAnalysisResult, error) {
		fact := v1
		return ProjectIndexAnalysisResult{ObservedFingerprint: snapshot.SourceFingerprint, Facts: &fact}, nil
	}
	first, err := manager.Refresh(context.Background(), registry, ProjectIndexRefreshOptions{
		ScopeFingerprint: indexDigest("unavailable-scope"), AnalysisFingerprint: analysisFingerprint,
		Snapshots:      []ProjectIndexFileSnapshot{{Path: path, SourceFingerprint: v1.SourceFingerprint}},
		ResolverLimits: indexResolverLimits(), Analyze: analyzeV1,
	})
	if err != nil {
		t.Fatal(err)
	}

	unavailableOptions := ProjectIndexRefreshOptions{
		ScopeFingerprint: indexDigest("unavailable-scope"), AnalysisFingerprint: analysisFingerprint,
		Unavailable:    []ProjectIndexUnavailableFile{{Path: path, Reason: ProjectIndexUnavailable}},
		ResolverLimits: indexResolverLimits(), Analyze: analyzeV1,
	}
	unavailable, err := manager.Refresh(context.Background(), registry, unavailableOptions)
	if err != nil {
		t.Fatal(err)
	}
	if unavailable.Evidence.Generation == first.Evidence.Generation || unavailable.Coverage.FilesConsidered != 1 || unavailable.Coverage.FilesParsed != 0 || unavailable.Coverage.FilesSkipped != 1 || unavailable.Coverage.CoverageComplete {
		t.Fatalf("unavailable generation = %+v coverage=%+v", unavailable.Evidence, unavailable.Coverage)
	}
	warmUnavailable, err := manager.Refresh(context.Background(), registry, unavailableOptions)
	if err != nil {
		t.Fatal(err)
	}
	if warmUnavailable.Evidence != unavailable.Evidence {
		t.Fatalf("stable unavailable metadata changed generation: first=%+v warm=%+v", unavailable.Evidence, warmUnavailable.Evidence)
	}

	badAnalyze := func(_ context.Context, _ ProjectIndexFileSnapshot) (ProjectIndexAnalysisResult, error) {
		fact := v2
		return ProjectIndexAnalysisResult{ObservedFingerprint: v1.SourceFingerprint, Facts: &fact}, nil
	}
	_, err = manager.Refresh(context.Background(), registry, ProjectIndexRefreshOptions{
		ScopeFingerprint: indexDigest("unavailable-scope"), AnalysisFingerprint: analysisFingerprint,
		Snapshots:      []ProjectIndexFileSnapshot{{Path: path, SourceFingerprint: v2.SourceFingerprint}},
		ResolverLimits: indexResolverLimits(), Analyze: badAnalyze,
	})
	if operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("TOCTOU refresh error=%v kind=%v", err, operation.KindOf(err))
	}
	stillUnavailable, err := manager.Refresh(context.Background(), registry, unavailableOptions)
	if err != nil {
		t.Fatal(err)
	}
	if stillUnavailable.Evidence != unavailable.Evidence {
		t.Fatalf("failed refresh published a partial generation: before=%+v after=%+v", unavailable.Evidence, stillUnavailable.Evidence)
	}
}

func TestProjectIndexRebuildsRelationsAfterIncrementalChange(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase15", "relations")
	aPath := filepath.Join(root, "a.ts")
	bPath := filepath.Join(root, "b.ts")
	aV1 := indexFacts(t, registry, aPath, "typescript", "import { B } from \"./b\";\nexport class A extends B {}\n")
	aV2 := indexFacts(t, registry, aPath, "typescript", "export class A {}\n")
	b := indexFacts(t, registry, bPath, "typescript", "export class B {}\n")
	manager, err := NewProjectIndexManager(ProjectIndexManagerLimits{MaxProjects: 1, MaxGenerations: 2})
	if err != nil {
		t.Fatal(err)
	}
	facts := map[string]ProjectFileFacts{aPath: aV1, bPath: b}
	calls := map[string]int{}
	analyze := func(_ context.Context, snapshot ProjectIndexFileSnapshot) (ProjectIndexAnalysisResult, error) {
		calls[snapshot.Path]++
		fact := facts[snapshot.Path]
		return ProjectIndexAnalysisResult{ObservedFingerprint: snapshot.SourceFingerprint, Facts: &fact}, nil
	}
	refresh := func(a ProjectFileFacts, includeB bool) ProjectIndexSelection {
		t.Helper()
		snapshots := []ProjectIndexFileSnapshot{{Path: a.Path, SourceFingerprint: a.SourceFingerprint}}
		if includeB {
			snapshots = append(snapshots, ProjectIndexFileSnapshot{Path: b.Path, SourceFingerprint: b.SourceFingerprint})
		}
		selection, refreshErr := manager.Refresh(context.Background(), registry, ProjectIndexRefreshOptions{
			ScopeFingerprint: indexDigest("relations-scope"), AnalysisFingerprint: indexDigest("relations-analysis"),
			Snapshots: snapshots, ResolverLimits: indexResolverLimits(), Analyze: analyze,
		})
		if refreshErr != nil {
			t.Fatal(refreshErr)
		}
		return selection
	}
	first := refresh(aV1, true)
	if got := first.Model.Dependencies(aPath); len(got) != 1 || len(got[0].Targets) != 1 || got[0].Targets[0].Path != bPath {
		t.Fatalf("initial dependencies = %+v", got)
	}
	facts[aPath] = aV2
	second := refresh(aV2, true)
	if got := second.Model.Dependencies(aPath); len(got) != 0 {
		t.Fatalf("changed generation retained stale dependency = %+v", got)
	}
	if calls[aPath] != 2 || calls[bPath] != 1 || second.Stats.AnalyzedFiles != 1 || second.Stats.ReusedFiles != 1 {
		t.Fatalf("incremental relation refresh calls=%+v stats=%+v", calls, second.Stats)
	}
	third := refresh(aV2, false)
	if third.Evidence.Generation == second.Evidence.Generation || third.Coverage.FilesConsidered != 1 || third.Coverage.FilesParsed != 1 || third.Coverage.FilesSkipped != 0 {
		t.Fatalf("file deletion did not create a coherent one-file generation: evidence=%+v coverage=%+v", third.Evidence, third.Coverage)
	}
	if got := third.Model.Dependents(bPath); len(got) != 0 {
		t.Fatalf("deleted file retained stale dependent edges = %+v", got)
	}
}

func TestProjectIndexEvictionNeverReusesGenerationIdentity(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewProjectIndexManager(ProjectIndexManagerLimits{MaxProjects: 1, MaxGenerations: 1})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("project", "phase15", "identity.ts")
	fact := indexFacts(t, registry, path, "typescript", "export class Identity {}\n")
	analyze := func(_ context.Context, snapshot ProjectIndexFileSnapshot) (ProjectIndexAnalysisResult, error) {
		copy := fact
		return ProjectIndexAnalysisResult{ObservedFingerprint: snapshot.SourceFingerprint, Facts: &copy}, nil
	}
	refresh := func(scope string) ProjectIndexSelection {
		t.Helper()
		selection, refreshErr := manager.Refresh(context.Background(), registry, ProjectIndexRefreshOptions{
			ScopeFingerprint: indexDigest(scope), AnalysisFingerprint: indexDigest("identity-analysis"),
			Snapshots:      []ProjectIndexFileSnapshot{{Path: path, SourceFingerprint: fact.SourceFingerprint}},
			ResolverLimits: indexResolverLimits(), Analyze: analyze,
		})
		if refreshErr != nil {
			t.Fatal(refreshErr)
		}
		return selection
	}
	first := refresh("scope-one")
	second := refresh("scope-two")
	third := refresh("scope-one")
	if !(first.Evidence.Generation < second.Evidence.Generation && second.Evidence.Generation < third.Evidence.Generation) {
		t.Fatalf("generation identities were reused across eviction: first=%+v second=%+v third=%+v", first.Evidence, second.Evidence, third.Evidence)
	}
	old := first.Evidence.Generation
	_, err = manager.Refresh(context.Background(), registry, ProjectIndexRefreshOptions{
		ScopeFingerprint: indexDigest("scope-one"), AnalysisFingerprint: indexDigest("identity-analysis"),
		Snapshots:      []ProjectIndexFileSnapshot{{Path: path, SourceFingerprint: fact.SourceFingerprint}},
		ResolverLimits: indexResolverLimits(), Binding: ProjectIndexBinding{Generation: &old, AllowStale: true}, Analyze: analyze,
	})
	if operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("evicted generation unexpectedly selectable: err=%v kind=%v", err, operation.KindOf(err))
	}
}

func indexFacts(t *testing.T, registry *LanguageRegistry, path, language, text string) ProjectFileFacts {
	t.Helper()
	descriptor, ok := registry.Resolve(language)
	if !ok {
		t.Fatalf("language %q was not registered", language)
	}
	analyzer, ok := AnalyzerFor(descriptor)
	if !ok {
		t.Fatalf("language %q has no analyzer", language)
	}
	return projectResolverFacts(t, analyzer, path, text)
}

func indexResolverLimits() ProjectResolverLimits {
	return ProjectResolverLimits{MaxFiles: 64, MaxSymbols: 4096, MaxDependencies: 4096, MaxReferences: 4096}
}

func indexDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
