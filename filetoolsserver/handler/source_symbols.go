package handler

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/concurrency"
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

	files, selectionTruncated, selectionErr := h.collectSourceSymbolFiles(requestCtx, input, maxFiles, limits)
	if selectionErr != nil {
		return selectionErr, SourceSymbolsOutput{}, nil
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

	output := buildSourceSymbolsOutput(operationName, input, len(files), selectionTruncated, analyses, maxSymbols)
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
		return errorResultWithCode(ErrCodeUnsupported, "show language has no supported analyzer"), SourceSymbolsOutput{}, nil
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
