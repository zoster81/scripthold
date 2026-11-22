package handler

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/concurrency"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

type sourceQueryFingerprintProbe struct {
	snapshot    *sourceintelligence.ProjectIndexFileSnapshot
	unavailable *sourceintelligence.ProjectIndexUnavailableFile
}

func (h *Handler) collectSourceQueryIndex(ctx context.Context, input SourceQueryInput, limits config.SourceConfig, maxFiles int) (sourceintelligence.ProjectIndexSelection, SourceQueryCoverage, *sourceintelligence.LanguageRegistry, *mcp.CallToolResult) {
	validatedRoots, files, selectionTruncated, failed := h.collectSourceRequestFiles(
		ctx, "source_query", input.Paths, input.Includes, input.Excludes,
		input.RespectGitignore, maxFiles, limits,
	)
	if failed != nil {
		return sourceintelligence.ProjectIndexSelection{}, SourceQueryCoverage{}, nil, failed
	}

	registry, registryErr := sourceintelligence.DefaultLanguageRegistry()
	if registryErr != nil {
		return sourceintelligence.ProjectIndexSelection{}, SourceQueryCoverage{}, nil, errorResultWithCode(ErrCodeInternal, registryErr.Error())
	}
	probes := make([]sourceQueryFingerprintProbe, 0, len(files))
	probeStats := concurrency.ProcessOrdered(ctx, files, concurrency.Options{MaxWorkers: limits.MaxConcurrency},
		func(workCtx context.Context, _ int, path string) sourceQueryFingerprintProbe {
			info, statErr := os.Stat(path)
			if statErr != nil {
				unavailable := sourceintelligence.ProjectIndexUnavailableFile{Path: path, Reason: sourceintelligence.ProjectIndexUnavailable}
				return sourceQueryFingerprintProbe{unavailable: &unavailable}
			}
			if !info.Mode().IsRegular() {
				unavailable := sourceintelligence.ProjectIndexUnavailableFile{Path: path, Reason: sourceintelligence.ProjectIndexNotRegular}
				return sourceQueryFingerprintProbe{unavailable: &unavailable}
			}
			if info.Size() > limits.MaxFileBytes {
				unavailable := sourceintelligence.ProjectIndexUnavailableFile{Path: path, Reason: sourceintelligence.ProjectIndexOverLimit}
				return sourceQueryFingerprintProbe{unavailable: &unavailable}
			}
			fingerprint, fingerprintErr := filesystem.FingerprintRegularFilePathBounded(workCtx, path, limits.MaxFileBytes)
			if fingerprintErr != nil {
				unavailable := sourceintelligence.ProjectIndexUnavailableFile{Path: path, Reason: sourceintelligence.ProjectIndexUnavailable}
				return sourceQueryFingerprintProbe{unavailable: &unavailable}
			}
			snapshot := sourceintelligence.ProjectIndexFileSnapshot{Path: path, SourceFingerprint: fingerprint}
			return sourceQueryFingerprintProbe{snapshot: &snapshot}
		},
		func(_ int, probe sourceQueryFingerprintProbe) bool {
			probes = append(probes, probe)
			return true
		},
	)
	if ctx.Err() != nil || probeStats.Cancelled {
		return sourceintelligence.ProjectIndexSelection{}, SourceQueryCoverage{}, nil, errorResultWithCode(ErrCodeCancelled, "source index fingerprinting cancelled")
	}
	snapshots := make([]sourceintelligence.ProjectIndexFileSnapshot, 0, len(probes))
	unavailable := make([]sourceintelligence.ProjectIndexUnavailableFile, 0, len(probes))
	for _, probe := range probes {
		if probe.snapshot != nil {
			snapshots = append(snapshots, *probe.snapshot)
		}
		if probe.unavailable != nil {
			unavailable = append(unavailable, *probe.unavailable)
		}
	}

	analysisFingerprint, fingerprintErr := h.sourceQueryAnalysisFingerprint(registry, sourceintelligence.ProjectIndexAnalysisConfig{
		MaxFileBytes: limits.MaxFileBytes, MaxDecodedCharacters: h.maxDecodedCharacters(), MaxSymbols: limits.MaxSymbols, MaxSignatureBytes: limits.MaxSignatureBytes,
		MaxDiagnostics: limits.MaxDiagnostics, MaxDetectorProbes: limits.MaxDetectorProbes, MaxNesting: limits.MaxNesting,
		MaxProjectEdges: limits.MaxGraphEdges,
	})
	if fingerprintErr != nil {
		return sourceintelligence.ProjectIndexSelection{}, SourceQueryCoverage{}, nil, errorResultFromError(fingerprintErr)
	}
	scopeFingerprint := sourceQueryScopeFingerprint(validatedRoots, input, maxFiles)
	manager, managerErr := h.sourceQueryIndexManager()
	if managerErr != nil {
		return sourceintelligence.ProjectIndexSelection{}, SourceQueryCoverage{}, nil, errorResultWithCode(ErrCodeInternal, managerErr.Error())
	}
	binding := sourceintelligence.ProjectIndexBinding{}
	if input.Index != nil {
		binding.Generation = input.Index.Generation
		binding.Fingerprint = input.Index.Fingerprint
		binding.AllowStale = input.Index.StalePolicy == "allow"
	}
	selection, refreshErr := manager.Refresh(ctx, registry, sourceintelligence.ProjectIndexRefreshOptions{
		ScopeFingerprint:    scopeFingerprint,
		AnalysisFingerprint: analysisFingerprint,
		Snapshots:           snapshots,
		Unavailable:         unavailable,
		SelectionTruncated:  selectionTruncated,
		ResolverLimits: sourceintelligence.ProjectResolverLimits{
			MaxFiles: maxFiles, MaxSymbols: limits.MaxSymbols,
			MaxDependencies: limits.MaxGraphEdges, MaxReferences: limits.MaxGraphEdges,
		},
		Binding: binding,
		Analyze: func(analyzeCtx context.Context, snapshot sourceintelligence.ProjectIndexFileSnapshot) (sourceintelligence.ProjectIndexAnalysisResult, error) {
			analysis := h.analyzeSourceFile(analyzeCtx, registry, snapshot.Path, input.Language, input.Encoding, false, limits.MaxSymbols, limits)
			result := sourceintelligence.ProjectIndexAnalysisResult{ObservedFingerprint: analysis.file.SourceFingerprint}
			if result.ObservedFingerprint == "" {
				observed, observedErr := filesystem.FingerprintRegularFilePathBounded(analyzeCtx, snapshot.Path, limits.MaxFileBytes)
				if observedErr != nil {
					return sourceintelligence.ProjectIndexAnalysisResult{}, observedErr
				}
				result.ObservedFingerprint = observed
			}
			if analysis.file.Status == "parsed" {
				facts := sourceintelligence.ProjectFileFacts{
					Path: analysis.file.Path, Language: analysis.file.Language, SourceFingerprint: analysis.file.SourceFingerprint, Analysis: analysis.analysis,
				}
				result.Facts = &facts
			}
			return result, nil
		},
	})
	if refreshErr != nil {
		return sourceintelligence.ProjectIndexSelection{}, SourceQueryCoverage{}, nil, errorResultFromError(refreshErr)
	}
	coverage := SourceQueryCoverage{
		FilesConsidered:        selection.Coverage.FilesConsidered,
		FilesParsed:            selection.Coverage.FilesParsed,
		FilesSkipped:           selection.Coverage.FilesSkipped,
		FileSelectionTruncated: selection.Coverage.Truncated,
		AnalysisComplete:       selection.Coverage.AnalysisComplete,
		CoverageComplete:       selection.Coverage.CoverageComplete,
		Truncated:              selection.Coverage.Truncated,
	}
	return selection, coverage, registry, nil
}

type sourceQueryAnalysisFingerprintCache struct {
	mu          sync.Mutex
	registry    *sourceintelligence.LanguageRegistry
	config      sourceintelligence.ProjectIndexAnalysisConfig
	fingerprint string
}

// sourceQueryAnalysisFingerprint caches one immutable registry/config fingerprint
// per handler. The single-entry cache bounds retained state even if configuration
// changes; file content fingerprints and generation checks still run per request.
func (h *Handler) sourceQueryAnalysisFingerprint(
	registry *sourceintelligence.LanguageRegistry,
	config sourceintelligence.ProjectIndexAnalysisConfig,
) (string, error) {
	cache := &h.sourceAnalysisCache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.fingerprint != "" && cache.registry == registry && cache.config == config {
		return cache.fingerprint, nil
	}
	fingerprint, err := sourceintelligence.ProjectIndexAnalysisFingerprint(registry, config)
	if err != nil {
		return "", err
	}
	cache.registry = registry
	cache.config = config
	cache.fingerprint = fingerprint
	return fingerprint, nil
}

func (h *Handler) sourceQueryIndexManager() (*sourceintelligence.ProjectIndexManager, error) {
	h.sourceIndexOnce.Do(func() {
		limits := h.sourceLimits()
		h.sourceIndex, h.sourceIndexInitErr = sourceintelligence.NewProjectIndexManager(sourceintelligence.ProjectIndexManagerLimits{
			MaxProjects: limits.MaxIndexProjects, MaxGenerations: limits.MaxIndexGenerations,
		})
	})
	return h.sourceIndex, h.sourceIndexInitErr
}

func sourceQueryScopeFingerprint(validatedRoots []string, input SourceQueryInput, maxFiles int) string {
	hasher := sha256.New()
	writeSourceQueryIndexHashPart(hasher, "scripthold:r27-source-query-scope-v1")
	roots := append([]string(nil), validatedRoots...)
	sort.Strings(roots)
	for _, root := range roots {
		writeSourceQueryIndexHashPart(hasher, root)
	}
	writeSourceQueryIndexHashPart(hasher, strings.ToLower(strings.TrimSpace(input.Language)))
	writeSourceQueryIndexHashPart(hasher, strings.ToLower(strings.TrimSpace(input.Encoding)))
	includes := append([]string(nil), input.Includes...)
	excludes := append([]string(nil), input.Excludes...)
	sort.Strings(includes)
	sort.Strings(excludes)
	for _, include := range includes {
		writeSourceQueryIndexHashPart(hasher, "include:"+include)
	}
	for _, exclude := range excludes {
		writeSourceQueryIndexHashPart(hasher, "exclude:"+exclude)
	}
	writeSourceQueryIndexHashPart(hasher, fmt.Sprintf("gitignore:%t", shouldRespectGitignore(input.RespectGitignore)))
	writeSourceQueryIndexHashPart(hasher, fmt.Sprintf("maxFiles:%d", maxFiles))
	return hex.EncodeToString(hasher.Sum(nil))
}

func writeSourceQueryIndexHashPart(hasher interface{ Write([]byte) (int, error) }, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = hasher.Write(length[:])
	_, _ = hasher.Write([]byte(value))
}
