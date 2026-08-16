package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func (h *Handler) analyzeSourceFile(ctx context.Context, registry *sourceintelligence.LanguageRegistry, path, explicitLanguage, requestedEncoding string, includeSignatures bool, maxSymbols int, limits config.SourceConfig) sourceFileAnalysis {
	file := SourceSymbolsFile{Path: path, Status: "error", CoverageComplete: false}
	info, statErr := os.Stat(path)
	if statErr != nil {
		mapping := mapOperationError(operation.Wrap(operation.KindNotFound, "source_symbols", path, statErr), path)
		file.Error, file.ErrorCode = mapping.Message, mapping.BatchCode
		return sourceFileAnalysis{file: file}
	}
	file.SourceBytes = info.Size()
	if info.Size() > limits.MaxFileBytes {
		file.Status = "skipped"
		file.ErrorCode = ErrCodeLimit
		file.Error = fmt.Sprintf("file size %d exceeds source limit %d", info.Size(), limits.MaxFileBytes)
		return sourceFileAnalysis{file: file}
	}
	document, err := sourceintelligence.OpenSourceDocument(ctx, path, sourceintelligence.OpenDocumentOptions{
		RequestedEncoding:    requestedEncoding,
		MaxFileBytes:         limits.MaxFileBytes,
		MaxDecodedCharacters: h.maxDecodedCharacters(),
	})
	if err != nil {
		mapping := mapOperationError(err, path)
		file.Error, file.ErrorCode = mapping.Message, mapping.BatchCode
		return sourceFileAnalysis{file: file}
	}
	file.Encoding = document.Encoding
	file.SourceFingerprint = document.SourceFingerprint
	file.SourceBytes = document.FileSizeBytes
	detection, err := sourceintelligence.DetectLanguage(ctx, registry, sourceintelligence.DetectionInput{
		Path: path, Text: document.Text, ExplicitLanguage: explicitLanguage, MaxProbes: limits.MaxDetectorProbes,
	})
	file.Detection = detection
	if err != nil {
		mapping := mapOperationError(err, path)
		file.Error, file.ErrorCode = mapping.Message, mapping.BatchCode
		return sourceFileAnalysis{file: file}
	}
	if detection.State == sourceintelligence.DetectionAmbiguous || detection.State == sourceintelligence.DetectionUnknown || detection.Language == "" {
		file.Status = "skipped"
		file.ErrorCode = ErrCodeUnsupported
		file.Error = "source language is ambiguous or unknown"
		return sourceFileAnalysis{file: file}
	}
	descriptor, ok := registry.Resolve(detection.Language)
	if !ok || !descriptor.Capabilities.SourceAnalysis {
		file.Status = "skipped"
		file.Language = detection.Language
		file.ErrorCode = ErrCodeUnsupported
		file.Error = "source language has no R25 analyzer"
		return sourceFileAnalysis{file: file}
	}
	analyzer, ok := sourceintelligence.AnalyzerFor(descriptor)
	if !ok {
		file.Status = "skipped"
		file.Language = descriptor.ID
		file.ErrorCode = ErrCodeUnsupported
		file.Error = "source analyzer is unavailable"
		return sourceFileAnalysis{file: file}
	}
	analysis, err := analyzer.Analyze(ctx, document, sourceintelligence.AnalyzeOptions{
		IncludeSignatures: includeSignatures, MaxNesting: limits.MaxNesting,
		Limits: sourceintelligence.SymbolBuilderLimits{MaxSymbols: maxSymbols, MaxSignatureBytes: limits.MaxSignatureBytes, MaxDiagnostics: limits.MaxDiagnostics},
	})
	if err != nil {
		mapping := mapOperationError(err, path)
		file.Error, file.ErrorCode = mapping.Message, mapping.BatchCode
		return sourceFileAnalysis{file: file}
	}
	file.Status = "parsed"
	file.Language = descriptor.ID
	file.Analyzer = string(analyzer.ID())
	file.CoverageComplete = analysis.Analysis.CoverageComplete
	return sourceFileAnalysis{file: file, analysis: analysis}
}

func (h *Handler) sourceLimits() config.SourceConfig {
	limits := config.SourceConfig{
		MaxInputPaths: config.DefaultSourceMaxInputPaths, MaxFiles: config.DefaultSourceMaxFiles,
		MaxAggregateBytes: config.DefaultSourceMaxAggregateBytes, MaxFileBytes: config.DefaultSourceMaxFileBytes,
		MaxSymbols: config.DefaultSourceMaxSymbols, MaxSignatureBytes: config.DefaultSourceMaxSignatureBytes,
		MaxShowBytes: config.DefaultSourceMaxShowBytes, MaxDiagnostics: config.DefaultSourceMaxDiagnostics,
		MaxDetectorProbes: config.DefaultSourceMaxDetectorProbes, MaxNesting: config.DefaultSourceMaxNesting,
		MaxConcurrency: config.DefaultSourceMaxConcurrency, MaxRequestSeconds: config.DefaultSourceMaxRequestSeconds,
		MaxOutputBytes: config.DefaultSourceMaxOutputBytes, MaxResults: config.DefaultSourceMaxResults,
		MaxGraphNodes: config.DefaultSourceMaxGraphNodes, MaxGraphEdges: config.DefaultSourceMaxGraphEdges,
		MaxGraphDepth: config.DefaultSourceMaxGraphDepth, MaxContextBytes: config.DefaultSourceMaxContextBytes,
		MaxContextItems: config.DefaultSourceMaxContextItems, MaxIndexProjects: config.DefaultSourceMaxIndexProjects,
		MaxIndexGenerations: config.DefaultSourceMaxIndexGenerations,
	}
	if h != nil && h.config != nil {
		limits = h.config.Source
	}
	limits.MaxFiles = minPositiveInt(limits.MaxFiles, h.maxBatchFiles())
	limits.MaxFileBytes = minPositiveInt64(limits.MaxFileBytes, h.maxFileBytes())
	limits.MaxOutputBytes = minPositiveInt64(limits.MaxOutputBytes, h.maxOutputBytes())
	limits.MaxShowBytes = minPositiveInt(limits.MaxShowBytes, clampBudgetToInt(limits.MaxFileBytes))
	limits.MaxShowBytes = minPositiveInt(limits.MaxShowBytes, clampBudgetToInt(limits.MaxOutputBytes))
	return limits
}

func resolvePositiveLimit(requested, maximum int, name string) (int, *mcp.CallToolResult) {
	if maximum <= 0 {
		return 0, errorResultWithCode(ErrCodeInternal, name+" server limit is invalid")
	}
	if requested == 0 {
		return maximum, nil
	}
	if requested < 0 {
		return 0, errorResultWithCode(ErrCodeInvalidInput, name+" must be positive")
	}
	if requested > maximum {
		return 0, errorResultWithCode(ErrCodeLimit, fmt.Sprintf("%s %d exceeds configured limit %d", name, requested, maximum))
	}
	return requested, nil
}

func sourceSymbolMatches(symbol sourceintelligence.NormalizedSymbol, query, mode string) bool {
	switch mode {
	case "prefix":
		return strings.HasPrefix(symbol.Name, query) || strings.HasPrefix(symbol.QualifiedName, query)
	case "qualified":
		return symbol.QualifiedName == query
	default:
		return symbol.Name == query
	}
}

func buildSourceDigest(current sourceFileAnalysis) SourceDigest {
	counts := make(map[sourceintelligence.SymbolKind]int)
	for _, symbol := range current.analysis.Analysis.Symbols {
		counts[symbol.Kind]++
	}
	kinds := make([]string, 0, len(counts))
	for kind := range counts {
		kinds = append(kinds, string(kind))
	}
	sort.Strings(kinds)
	declarationCounts := make([]SourceDeclarationCount, 0, len(kinds))
	for _, kind := range kinds {
		declarationCounts = append(declarationCounts, SourceDeclarationCount{Kind: sourceintelligence.SymbolKind(kind), Count: counts[sourceintelligence.SymbolKind(kind)]})
	}
	return SourceDigest{
		Path: current.file.Path, Language: current.file.Language, Analyzer: current.file.Analyzer,
		SourceBytes: current.file.SourceBytes, SourceFingerprint: current.file.SourceFingerprint,
		DeclarationCounts: declarationCounts, Dependencies: current.analysis.Dependencies, Relations: current.analysis.Relations,
		Regions: current.analysis.Regions, CoverageComplete: current.file.CoverageComplete,
	}
}

func enforceSourceOutputBudget(output SourceSymbolsOutput, maximum int64) error {
	encoded, err := json.Marshal(output)
	if err != nil {
		return operation.Wrap(operation.KindUnknown, "source_symbols", "", err)
	}
	if int64(len(encoded)) > maximum {
		return operation.Wrap(operation.KindLimit, "source_symbols", "", fmt.Errorf("source output size %d exceeds limit %d", len(encoded), maximum))
	}
	return nil
}

func deduplicateSortedPaths(paths []string) []string {
	if len(paths) < 2 {
		return paths
	}
	write := 1
	for read := 1; read < len(paths); read++ {
		if paths[read] == paths[write-1] {
			continue
		}
		paths[write] = paths[read]
		write++
	}
	return paths[:write]
}

func minPositiveInt(left, right int) int {
	if left <= 0 {
		return right
	}
	if right <= 0 || left < right {
		return left
	}
	return right
}

func minPositiveInt64(left, right int64) int64 {
	if left <= 0 {
		return right
	}
	if right <= 0 || left < right {
		return left
	}
	return right
}
