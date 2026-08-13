package handler

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/concurrency"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func (h *Handler) SourceSymbols(ctx context.Context, _ *mcp.CallToolRequest, input SourceSymbolsInput) (*mcp.CallToolResult, SourceSymbolsOutput, error) {
	operationName := strings.ToLower(strings.TrimSpace(input.Operation))
	if operationName == "show" {
		return h.sourceSymbolsShow(ctx, input)
	}
	if operationName != "outline" && operationName != "digest" && operationName != "find" {
		return errorResultWithCode(ErrCodeInvalidInput, "operation must be outline, digest, find, or show"), SourceSymbolsOutput{}, nil
	}
	if len(input.Paths) == 0 {
		return errorResultWithCode(ErrCodeInvalidInput, "paths must contain at least one path"), SourceSymbolsOutput{}, nil
	}
	limits := h.sourceLimits()
	if len(input.Paths) > limits.MaxInputPaths {
		return errorResultWithCode(ErrCodeLimit, fmt.Sprintf("path count %d exceeds source limit %d", len(input.Paths), limits.MaxInputPaths)), SourceSymbolsOutput{}, nil
	}
	maxFiles, result := resolvePositiveLimit(input.MaxFiles, limits.MaxFiles, "maxFiles")
	if result != nil {
		return result, SourceSymbolsOutput{}, nil
	}
	maxSymbols := limits.MaxSymbols
	if operationName == "outline" || operationName == "find" {
		maxSymbols, result = resolvePositiveLimit(input.MaxSymbols, limits.MaxSymbols, "maxSymbols")
		if result != nil {
			return result, SourceSymbolsOutput{}, nil
		}
	}
	if operationName == "find" {
		if strings.TrimSpace(input.Query) == "" || utf8.RuneCountInString(input.Query) > 512 {
			return errorResultWithCode(ErrCodeInvalidInput, "query must be a non-empty string up to 512 Unicode scalar values"), SourceSymbolsOutput{}, nil
		}
		if input.Match == "" {
			input.Match = "exact"
		}
		if input.Match != "exact" && input.Match != "prefix" && input.Match != "qualified" {
			return errorResultWithCode(ErrCodeInvalidInput, "match must be exact, prefix, or qualified"), SourceSymbolsOutput{}, nil
		}
	}
	if len(input.Kinds) > 32 {
		return errorResultWithCode(ErrCodeLimit, "kinds exceeds the 32-item limit"), SourceSymbolsOutput{}, nil
	}

	requestCtx := ctx
	cancel := func() {}
	if limits.MaxRequestSeconds > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, time.Duration(limits.MaxRequestSeconds)*time.Second)
	}
	defer cancel()

	validatedRoots := make([]string, 0, len(input.Paths))
	for _, path := range input.Paths {
		validated := h.ValidatePath(path)
		if !validated.Ok() {
			return validated.Result, SourceSymbolsOutput{}, nil
		}
		if _, err := os.Stat(validated.Path); err != nil {
			return errorResultFromError(operation.Wrap(operation.KindNotFound, "source_symbols", validated.Path, err)), SourceSymbolsOutput{}, nil
		}
		validatedRoots = append(validatedRoots, validated.Path)
	}
	files, collectErr := h.collectFiles(requestCtx, validatedRoots, input.Includes, input.Excludes, shouldRespectGitignore(input.RespectGitignore))
	if collectErr != nil {
		return errorResultFromError(collectErr), SourceSymbolsOutput{}, nil
	}
	sort.Strings(files)
	files = deduplicateSortedPaths(files)
	selectionTruncated := false
	if len(files) > maxFiles {
		files = files[:maxFiles]
		selectionTruncated = true
	}
	var aggregateBytes int64
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil || info.Size() > limits.MaxFileBytes {
			continue
		}
		if info.Size() > limits.MaxAggregateBytes-aggregateBytes {
			return errorResultWithCode(ErrCodeLimit, fmt.Sprintf("selected source bytes exceed aggregate limit %d", limits.MaxAggregateBytes)), SourceSymbolsOutput{}, nil
		}
		aggregateBytes += info.Size()
	}

	registry, registryErr := sourceintelligence.DefaultLanguageRegistry()
	if registryErr != nil {
		return errorResultWithCode(ErrCodeInternal, registryErr.Error()), SourceSymbolsOutput{}, nil
	}
	includeSignatures := input.IncludeSignatures && operationName != "digest"
	perFileSymbols := maxSymbols
	if operationName == "digest" || operationName == "find" {
		perFileSymbols = limits.MaxSymbols
	}
	analyses := make([]sourceFileAnalysis, 0, len(files))
	stats := concurrency.ProcessOrdered(requestCtx, files, concurrency.Options{MaxWorkers: limits.MaxConcurrency},
		func(workCtx context.Context, _ int, path string) sourceFileAnalysis {
			return h.analyzeSourceFile(workCtx, registry, path, input.Language, input.Encoding, includeSignatures, perFileSymbols, limits)
		},
		func(_ int, analysis sourceFileAnalysis) bool {
			analyses = append(analyses, analysis)
			return true
		},
	)
	if requestCtx.Err() != nil || stats.Cancelled {
		return errorResultWithCode(ErrCodeCancelled, "source analysis cancelled"), SourceSymbolsOutput{}, nil
	}

	output := SourceSymbolsOutput{
		Operation: operationName, CoordinateSystem: sourceCoordinateSystem,
		FilesConsidered: len(files), CoverageComplete: !selectionTruncated, Truncated: selectionTruncated,
	}
	kindFilter := make(map[string]struct{}, len(input.Kinds))
	for _, kind := range input.Kinds {
		kindFilter[strings.ToLower(strings.TrimSpace(kind))] = struct{}{}
	}
	for _, current := range analyses {
		output.Files = append(output.Files, current.file)
		if current.file.Status != "parsed" {
			output.FilesSkipped++
			output.CoverageComplete = false
			continue
		}
		output.FilesParsed++
		if !current.file.CoverageComplete {
			output.CoverageComplete = false
		}
		switch operationName {
		case "digest":
			output.Digests = append(output.Digests, buildSourceDigest(current))
		case "outline", "find":
			for _, symbol := range current.analysis.Analysis.Symbols {
				if len(kindFilter) > 0 {
					if _, ok := kindFilter[strings.ToLower(string(symbol.Kind))]; !ok {
						continue
					}
				}
				if operationName == "find" && !sourceSymbolMatches(symbol, input.Query, input.Match) {
					continue
				}
				if len(output.Symbols) >= maxSymbols {
					output.Truncated = true
					output.CoverageComplete = false
					break
				}
				output.Symbols = append(output.Symbols, symbol)
			}
		}
	}
	output.SymbolCount = len(output.Symbols)
	if operationName == "find" && len(output.Symbols) > 1 {
		output.Ambiguous = true
	}
	if operationName == "digest" {
		for _, current := range analyses {
			output.SymbolCount += len(current.analysis.Analysis.Symbols)
		}
	}
	if outputErr := enforceSourceOutputBudget(output, limits.MaxOutputBytes); outputErr != nil {
		return errorResultFromError(outputErr), SourceSymbolsOutput{}, nil
	}
	return nil, output, nil
}

func (h *Handler) sourceSymbolsShow(ctx context.Context, input SourceSymbolsInput) (*mcp.CallToolResult, SourceSymbolsOutput, error) {
	limits := h.sourceLimits()
	if input.Path == "" || input.SymbolID == "" || input.SourceFingerprint == "" || input.Language == "" || input.Encoding == "" {
		return errorResultWithCode(ErrCodeInvalidInput, "show requires path, symbolId, sourceFingerprint, language, and encoding"), SourceSymbolsOutput{}, nil
	}
	maxBytes, result := resolvePositiveLimit(input.MaxBytes, limits.MaxShowBytes, "maxBytes")
	if result != nil {
		return result, SourceSymbolsOutput{}, nil
	}
	validated := h.ValidatePath(input.Path)
	if !validated.Ok() {
		return validated.Result, SourceSymbolsOutput{}, nil
	}
	requestCtx := ctx
	cancel := func() {}
	if limits.MaxRequestSeconds > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, time.Duration(limits.MaxRequestSeconds)*time.Second)
	}
	defer cancel()
	document, err := sourceintelligence.OpenSourceDocument(requestCtx, validated.Path, sourceintelligence.OpenDocumentOptions{
		RequestedEncoding: input.Encoding, MaxFileBytes: limits.MaxFileBytes, MaxDecodedCharacters: h.maxDecodedCharacters(),
	})
	if err != nil {
		return errorResultFromError(err), SourceSymbolsOutput{}, nil
	}
	if document.SourceFingerprint != input.SourceFingerprint {
		return errorResultWithCode(ErrCodeConflict, "source fingerprint is stale; re-run outline/find before show"), SourceSymbolsOutput{}, nil
	}
	registry, err := sourceintelligence.DefaultLanguageRegistry()
	if err != nil {
		return errorResultWithCode(ErrCodeInternal, err.Error()), SourceSymbolsOutput{}, nil
	}
	descriptor, ok := registry.Resolve(input.Language)
	if !ok {
		return errorResultWithCode(ErrCodeInvalidInput, "unknown show language"), SourceSymbolsOutput{}, nil
	}
	analyzer, ok := sourceintelligence.AnalyzerFor(descriptor)
	if !ok {
		return errorResultWithCode(ErrCodeUnsupported, "show language has no R25 analyzer"), SourceSymbolsOutput{}, nil
	}
	analysis, err := analyzer.Analyze(requestCtx, document, sourceintelligence.AnalyzeOptions{
		IncludeSignatures: false, MaxNesting: limits.MaxNesting,
		Limits: sourceintelligence.SymbolBuilderLimits{MaxSymbols: limits.MaxSymbols, MaxSignatureBytes: limits.MaxSignatureBytes, MaxDiagnostics: limits.MaxDiagnostics},
	})
	if err != nil {
		return errorResultFromError(err), SourceSymbolsOutput{}, nil
	}
	for _, symbol := range analysis.Analysis.Symbols {
		if symbol.ID != input.SymbolID {
			continue
		}
		declaration, _, _, _ := symbol.SourceOffsets()
		text, rangeValue, sliceErr := document.SliceUTF8Offsets(declaration.Start, declaration.End, maxBytes)
		if sliceErr != nil {
			return errorResultFromError(sliceErr), SourceSymbolsOutput{}, nil
		}
		output := SourceSymbolsOutput{
			Operation: "show", CoordinateSystem: sourceCoordinateSystem,
			FilesConsidered: 1, FilesParsed: 1, SymbolCount: 1, CoverageComplete: analysis.Analysis.CoverageComplete,
			Show: &SourceShow{Path: validated.Path, SymbolID: symbol.ID, SourceFingerprint: document.SourceFingerprint, Language: descriptor.ID, Encoding: document.Encoding, Range: rangeValue, Text: text},
		}
		if outputErr := enforceSourceOutputBudget(output, limits.MaxOutputBytes); outputErr != nil {
			return errorResultFromError(outputErr), SourceSymbolsOutput{}, nil
		}
		return nil, output, nil
	}
	return errorResultWithCode(ErrCodeNotFound, "symbolId was not found in the current source"), SourceSymbolsOutput{}, nil
}
