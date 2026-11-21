package handler

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		file.Error = "source language has no supported analyzer"
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

// sourcePathMaxHeap keeps the lexicographically last retained path at the root.
type sourcePathMaxHeap []string

func (values sourcePathMaxHeap) Len() int           { return len(values) }
func (values sourcePathMaxHeap) Less(i, j int) bool { return values[i] > values[j] }
func (values sourcePathMaxHeap) Swap(i, j int)      { values[i], values[j] = values[j], values[i] }
func (values *sourcePathMaxHeap) Push(value any)    { *values = append(*values, value.(string)) }
func (values *sourcePathMaxHeap) Pop() any {
	last := len(*values) - 1
	value := (*values)[last]
	*values = (*values)[:last]
	return value
}

// collectBoundedSourceFiles traverses the full authorized scope for honest
// truncation evidence without retaining all matching paths in memory.
func (h *Handler) collectBoundedSourceFiles(ctx context.Context, paths, includes, excludes []string, respectGitignore bool, maximum int) ([]string, bool, error) {
	if maximum <= 0 {
		return nil, false, operation.New(operation.KindInvalidInput, "source file selection limit must be positive")
	}
	selected := make(sourcePathMaxHeap, 0, min(maximum, 256))
	retained := make(map[string]struct{}, min(maximum, 256))
	truncated := false
	err := h.visitIncludedFiles(ctx, paths, includes, excludes, respectGitignore, func(path string) {
		if _, exists := retained[path]; exists {
			return
		}
		if len(selected) < maximum {
			heap.Push(&selected, path)
			retained[path] = struct{}{}
			return
		}
		truncated = true
		if path >= selected[0] {
			return
		}
		delete(retained, selected[0])
		selected[0] = path
		retained[path] = struct{}{}
		heap.Fix(&selected, 0)
	})
	if err != nil {
		return nil, false, err
	}
	sort.Strings(selected)
	return []string(selected), truncated, nil
}

// collectSourceRequestFiles preserves the ordered authorization and aggregate
// preflight shared by source_symbols and source_query. The latter still performs
// its separate content fingerprint and index-generation checks afterward.
func (h *Handler) collectSourceRequestFiles(
	ctx context.Context,
	operationName string,
	paths, includes, excludes []string,
	respectGitignore *bool,
	maxFiles int,
	limits config.SourceConfig,
) ([]string, []string, bool, *mcp.CallToolResult) {
	validatedRoots := make([]string, 0, len(paths))
	for _, path := range paths {
		validated := h.ValidatePath(path)
		if !validated.Ok() {
			return nil, nil, false, validated.Result
		}
		if _, err := os.Stat(validated.Path); err != nil {
			return nil, nil, false, errorResultFromError(operation.Wrap(operation.KindNotFound, operationName, validated.Path, err))
		}
		validatedRoots = append(validatedRoots, validated.Path)
	}
	files, selectionTruncated, collectErr := h.collectBoundedSourceFiles(ctx, validatedRoots, includes, excludes, shouldRespectGitignore(respectGitignore), maxFiles)
	if collectErr != nil {
		return nil, nil, false, errorResultFromError(collectErr)
	}
	var aggregateBytes int64
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil || info.Size() > limits.MaxFileBytes {
			continue
		}
		if info.Size() > limits.MaxAggregateBytes-aggregateBytes {
			return nil, nil, false, errorResultWithCode(ErrCodeLimit, fmt.Sprintf("selected source bytes exceed aggregate limit %d", limits.MaxAggregateBytes))
		}
		aggregateBytes += info.Size()
	}
	return validatedRoots, files, selectionTruncated, nil
}

func (h *Handler) collectSourceSymbolFiles(
	ctx context.Context,
	input SourceSymbolsInput,
	maxFiles int,
	limits config.SourceConfig,
) ([]string, bool, *mcp.CallToolResult) {
	_, files, truncated, failed := h.collectSourceRequestFiles(
		ctx, "source_symbols", input.Paths, input.Includes, input.Excludes,
		input.RespectGitignore, maxFiles, limits,
	)
	return files, truncated, failed
}

func buildSourceSymbolsOutput(
	operationName string,
	input SourceSymbolsInput,
	filesConsidered int,
	selectionTruncated bool,
	analyses []sourceFileAnalysis,
	maxSymbols int,
) SourceSymbolsOutput {
	output := SourceSymbolsOutput{
		Operation: operationName, CoordinateSystem: sourceCoordinateSystem,
		FilesConsidered: filesConsidered, CoverageComplete: !selectionTruncated, Truncated: selectionTruncated,
	}
	kindFilter := make(map[string]struct{}, len(input.Kinds))
	for _, kind := range input.Kinds {
		kindFilter[strings.ToLower(strings.TrimSpace(kind))] = struct{}{}
	}
	if operationName == "outline" && len(kindFilter) == 0 && maxSymbols > 0 {
		capacity := 0
		for _, current := range analyses {
			if current.file.Status != "parsed" {
				continue
			}
			capacity += min(len(current.analysis.Analysis.Symbols), maxSymbols-capacity)
			if capacity == maxSymbols {
				break
			}
		}
		if capacity > 0 {
			output.Symbols = make([]sourceintelligence.NormalizedSymbol, 0, capacity)
		}
	}
	for _, current := range analyses {
		keepFile := operationName != "find" || input.FileDetails != "relevant" || !current.file.CoverageComplete
		beforeSymbols := len(output.Symbols)
		if current.file.Status != "parsed" {
			output.Files = append(output.Files, current.file)
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
		if keepFile || len(output.Symbols) != beforeSymbols {
			output.Files = append(output.Files, current.file)
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
	return output
}

// presentSourceOutline reuses the analyzer output rather than re-parsing source.
// A tree nests only earlier parents: this avoids cycles, while filtered/missing
// or out-of-order ancestors remain visible through their parentId.
func presentSourceOutline(files []SourceSymbolsFile, symbols []sourceintelligence.NormalizedSymbol, presentation string) []SourceOutlineFile {
	out := make([]SourceOutlineFile, len(files))
	byPath := make(map[string]int, len(files))
	for i, file := range files {
		byPath[file.Path] = i
		out[i] = SourceOutlineFile{
			Path: file.Path, Status: file.Status, Encoding: file.Encoding,
			Language: file.Language, SourceFingerprint: file.SourceFingerprint,
			CoverageComplete: file.CoverageComplete, Error: file.Error, ErrorCode: file.ErrorCode,
		}
	}
	type ancestor struct {
		node *SourceOutlineNode
		path string
	}
	tree := presentation == "tree"
	var seen map[string]ancestor
	if tree {
		// Flat outlines cannot reference an earlier parent, so they do not
		// need the potentially large ancestor index.
		for _, symbol := range symbols {
			if symbol.ParentID != "" {
				seen = make(map[string]ancestor, len(symbols))
				break
			}
		}
	}
	for _, symbol := range symbols {
		index, ok := byPath[symbol.Path]
		if !ok {
			continue
		}
		node := &SourceOutlineNode{
			SymbolID: symbol.ID, Name: symbol.Name, Kind: symbol.Kind,
			QualifiedName: symbol.QualifiedName,
			Line:          symbol.DeclarationRange.Start.Line, Column: symbol.DeclarationRange.Start.Column,
			Signature: symbol.Signature,
		}
		if tree && symbol.ParentID != "" {
			if parent, ok := seen[symbol.ParentID]; ok && parent.path == symbol.Path {
				parent.node.Children = append(parent.node.Children, node)
			} else {
				node.ParentID = symbol.ParentID
				out[index].Symbols = append(out[index].Symbols, node)
			}
		} else {
			node.ParentID = symbol.ParentID
			out[index].Symbols = append(out[index].Symbols, node)
		}
		if seen != nil {
			seen[symbol.ID] = ancestor{node: node, path: symbol.Path}
		}
	}
	return out
}

// presentSourceDigestOverview builds a small file atlas from the already analyzed
// declarations. Digest entries follow parsed file order, so no path index is needed.
func presentSourceDigestOverview(analyses []sourceFileAnalysis, digests []SourceDigest) []SourceOverviewFile {
	const maxEntryPoints = 5
	out := make([]SourceOverviewFile, 0, len(analyses))
	digestIndex := 0
	for _, current := range analyses {
		file := current.file
		entry := SourceOverviewFile{
			Path: file.Path, Status: file.Status, Language: file.Language,
			Encoding: file.Encoding, SourceFingerprint: file.SourceFingerprint,
			SourceBytes: file.SourceBytes, CoverageComplete: file.CoverageComplete,
			Error: file.Error, ErrorCode: file.ErrorCode,
		}
		if file.Status == "parsed" {
			entry.DeclarationCounts = digests[digestIndex].DeclarationCounts
			digestIndex++
			// Top-level declarations may be children of a synthetic package/module node.
			rootContainers := make(map[string]struct{})
			for _, symbol := range current.analysis.Analysis.Symbols {
				if symbol.ParentID == "" && (symbol.Kind == "package" || symbol.Kind == "module" || symbol.Kind == "namespace") {
					rootContainers[symbol.ID] = struct{}{}
				}
			}
			for _, symbol := range current.analysis.Analysis.Symbols {
				if string(symbol.Visibility) != "public" {
					continue
				}
				if symbol.ParentID != "" {
					if _, ok := rootContainers[symbol.ParentID]; !ok {
						continue
					}
				}
				switch string(symbol.Kind) {
				case "function", "class", "struct", "interface", "type", "alias", "enum", "trait":
				default:
					continue
				}
				if len(entry.EntryPoints) == maxEntryPoints {
					entry.EntryPointsTruncated = true
					break
				}
				entry.EntryPoints = append(entry.EntryPoints, SourceOverviewEntry{
					Name: symbol.Name, Kind: symbol.Kind, Line: symbol.DeclarationRange.Start.Line,
				})
			}
		}
		out = append(out, entry)
	}
	return out
}

// presentSourceConciseFind retains one verified file identity for each group of
// matching declarations so clients can pass its path, encoding and fingerprint
// with a single member symbolId to source_symbols show.
func presentSourceConciseFind(files []SourceSymbolsFile, symbols []sourceintelligence.NormalizedSymbol) []SourceConciseFile {
	out := make([]SourceConciseFile, 0, len(files))
	byPath := make(map[string]int, len(files))
	for _, file := range files {
		entry := SourceConciseFile{
			Path: file.Path, Language: file.Language, Encoding: file.Encoding,
			SourceFingerprint: file.SourceFingerprint,
		}
		if file.Status != "parsed" || !file.CoverageComplete {
			entry.Status = file.Status
			if file.Status == "parsed" {
				entry.Status = "partial"
			}
			entry.Error, entry.ErrorCode = file.Error, file.ErrorCode
		}
		byPath[file.Path] = len(out)
		out = append(out, entry)
	}
	for _, symbol := range symbols {
		if index, exists := byPath[symbol.Path]; exists {
			out[index].Matches = append(out[index].Matches, SourceConciseMatch{
				Name: symbol.Name, Owner: symbol.ParentQualifiedName,
				Kind: symbol.Kind, Line: symbol.DeclarationRange.Start.Line,
				Signature: symbol.Signature, SymbolID: symbol.ID,
			})
		}
	}
	return out
}

// buildSourceConciseDigest walks the bounded analyzer facts once, without
// constructing the legacy dependency inventories or exposing selection hashes.
func buildSourceConciseDigest(analyses []sourceFileAnalysis, roots []string, selectionTruncated bool) SourceSymbolsOutput {
	out := SourceSymbolsOutput{
		Operation: "digest", Presentation: "concise",
		CoordinateSystem: sourceCoordinateSystem,
		CoverageComplete: !selectionTruncated, Truncated: selectionTruncated,
		ConciseAnalysisComplete:   true,
		ConciseSelectionTruncated: selectionTruncated,
		ConciseFiles:              make([]SourceConciseFile, 0, len(analyses)),
	}
	for _, current := range analyses {
		relative, root := sourceConciseLocation(current.file.Path, roots)
		entry := SourceConciseFile{Path: relative}
		if len(roots) > 1 {
			entry.Root = root
		}
		if current.file.Status != "parsed" {
			out.FilesSkipped++
			out.CoverageComplete = false
			out.ConciseAnalysisComplete = false
			entry.Status = current.file.Status
			entry.ErrorCode = current.file.ErrorCode
			entry.Error = current.file.Error
		} else {
			out.FilesParsed++
			if !current.file.CoverageComplete {
				out.CoverageComplete = false
				out.ConciseAnalysisComplete = false
				entry.Status = "partial"
			}
			parents := make(map[string]sourceintelligence.NormalizedSymbol, len(current.analysis.Analysis.Symbols))
			for _, symbol := range current.analysis.Analysis.Symbols {
				parents[symbol.ID] = symbol
			}
			for _, symbol := range current.analysis.Analysis.Symbols {
				if !sourceConciseTopLevel(symbol, parents) {
					continue
				}
				switch symbol.Kind {
				case sourceintelligence.SymbolKindClass:
					entry.Classes = append(entry.Classes, symbol.Name)
				case sourceintelligence.SymbolKindStruct, sourceintelligence.SymbolKindInterface,
					sourceintelligence.SymbolKindEnum, sourceintelligence.SymbolKindRecord,
					sourceintelligence.SymbolKindType, sourceintelligence.SymbolKindAlias,
					sourceintelligence.SymbolKindTrait, sourceintelligence.SymbolKindDelegate:
					entry.Types = append(entry.Types, symbol.Name)
				case sourceintelligence.SymbolKindFunction:
					entry.Functions = append(entry.Functions, symbol.Name)
				}
			}
		}
		out.ConciseFiles = append(out.ConciseFiles, entry)
	}
	return out
}

// Only package/namespace/module containers may stand between a declaration
// and file scope; a method or nested class is not a top-level declaration.
func sourceConciseTopLevel(symbol sourceintelligence.NormalizedSymbol, symbols map[string]sourceintelligence.NormalizedSymbol) bool {
	for id, steps := symbol.ParentID, 0; id != ""; steps++ {
		if steps >= len(symbols) {
			return false
		}
		parent, ok := symbols[id]
		if !ok {
			return false
		}
		switch parent.Kind {
		case sourceintelligence.SymbolKindPackage, sourceintelligence.SymbolKindNamespace, sourceintelligence.SymbolKindModule:
			id = parent.ParentID
		default:
			return false
		}
	}
	return true
}

// Keep the longest matching selected root. A file selected directly uses its
// basename; multi-root responses carry the root explicitly to avoid collisions.
func sourceConciseLocation(path string, roots []string) (string, string) {
	bestRoot, bestRelative := "", path
	for _, raw := range roots {
		root, err := filepath.Abs(raw)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || filepath.IsAbs(relative) || relative == ".." ||
			strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if len(root) > len(bestRoot) {
			bestRoot, bestRelative = root, relative
		}
	}
	if bestRoot == "" || bestRelative == "." {
		bestRelative = filepath.Base(path)
	}
	return filepath.ToSlash(bestRelative), bestRoot
}

// buildSourceConciseOutline presents method declarations only for recognized
// owners. It never guesses ownership from a common name or adjacent lines.
func buildSourceConciseOutline(analyses []sourceFileAnalysis, roots []string, requestedOwner string, kinds []string, selectionTruncated bool, maxSymbols int) SourceSymbolsOutput {
	out := SourceSymbolsOutput{
		Operation: "outline", Presentation: "concise",
		CoordinateSystem: sourceCoordinateSystem,
		CoverageComplete: !selectionTruncated, Truncated: selectionTruncated,
		ConciseAnalysisComplete:   true,
		ConciseSelectionTruncated: selectionTruncated,
		ConciseFiles:              make([]SourceConciseFile, 0, len(analyses)),
	}
	selectedKinds := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		selectedKinds[strings.ToLower(strings.TrimSpace(kind))] = true
	}
	wantKind := func(kind sourceintelligence.SymbolKind) bool {
		return len(selectedKinds) == 0 || selectedKinds[string(kind)]
	}
	retained, matchedOwners := 0, 0
	appendMember := func(destination *[]SourceConciseMember, symbol sourceintelligence.NormalizedSymbol) {
		if retained >= maxSymbols {
			out.Truncated = true
			out.ConciseResultsTruncated = true
			out.CoverageComplete = false
			return
		}
		*destination = append(*destination, SourceConciseMember{
			Name: symbol.Name, Kind: symbol.Kind,
			Line: symbol.DeclarationRange.Start.Line, Signature: symbol.Signature,
		})
		retained++
	}
	for _, current := range analyses {
		relative, root := sourceConciseLocation(current.file.Path, roots)
		entry := SourceConciseFile{Path: relative}
		if len(roots) > 1 {
			entry.Root = root
		}
		if current.file.Status != "parsed" {
			out.FilesSkipped++
			out.CoverageComplete = false
			out.ConciseAnalysisComplete = false
			entry.Status, entry.ErrorCode, entry.Error = current.file.Status, current.file.ErrorCode, current.file.Error
			out.ConciseFiles = append(out.ConciseFiles, entry)
			continue
		}
		out.FilesParsed++
		if !current.file.CoverageComplete {
			out.CoverageComplete = false
			out.ConciseAnalysisComplete = false
			entry.Status = "partial"
		}
		ownerByID := make(map[string]int)
		symbols := current.analysis.Analysis.Symbols
		parents := sourceConciseParents(symbols)
		for _, symbol := range symbols {
			if !sourceConciseOwnerKind(symbol.Kind) || (requestedOwner != "" &&
				requestedOwner != symbol.Name && requestedOwner != symbol.QualifiedName) {
				continue
			}
			matchedOwners++
			if retained >= maxSymbols {
				out.Truncated = true
				out.ConciseResultsTruncated = true
				out.CoverageComplete = false
				continue
			}
			display := SourceConciseOwner{
				Name: symbol.Name, Kind: symbol.Kind,
				Line: symbol.DeclarationRange.Start.Line,
			}
			if requestedOwner != "" && requestedOwner != symbol.Name {
				display.QualifiedName = symbol.QualifiedName
			}
			ownerByID[symbol.ID] = len(entry.Owners)
			entry.Owners = append(entry.Owners, display)
			retained++
		}
		for _, symbol := range symbols {
			if index, ok := ownerByID[symbol.ParentID]; ok {
				if !wantKind(symbol.Kind) {
					continue
				}
				switch symbol.Kind {
				case sourceintelligence.SymbolKindMethod, sourceintelligence.SymbolKindConstructor,
					sourceintelligence.SymbolKindDestructor, sourceintelligence.SymbolKindOperator,
					sourceintelligence.SymbolKindFunction:
					appendMember(&entry.Owners[index].Methods, symbol)
				case sourceintelligence.SymbolKindField, sourceintelligence.SymbolKindProperty,
					sourceintelligence.SymbolKindVariable, sourceintelligence.SymbolKindConstant:
					// Fields require explicit selection; do not dump them by default.
					if len(selectedKinds) > 0 {
						appendMember(&entry.Owners[index].Fields, symbol)
					}
				}
			} else if requestedOwner == "" && symbol.Kind == sourceintelligence.SymbolKindFunction &&
				sourceConciseTopLevel(symbol, parents) && wantKind(symbol.Kind) {
				if retained >= maxSymbols {
					out.Truncated = true
					out.ConciseResultsTruncated = true
					out.CoverageComplete = false
				} else {
					entry.Functions = append(entry.Functions, symbol.Name)
					retained++
				}
			}
		}
		if requestedOwner == "" || len(entry.Owners) > 0 || entry.Status != "" {
			out.ConciseFiles = append(out.ConciseFiles, entry)
		}
	}
	out.Ambiguous = requestedOwner != "" && matchedOwners > 1
	return out
}

func sourceConciseOwnerKind(kind sourceintelligence.SymbolKind) bool {
	switch kind {
	case sourceintelligence.SymbolKindClass, sourceintelligence.SymbolKindStruct,
		sourceintelligence.SymbolKindInterface, sourceintelligence.SymbolKindRecord,
		sourceintelligence.SymbolKindTrait, sourceintelligence.SymbolKindType,
		sourceintelligence.SymbolKindImplementation:
		return true
	default:
		return false
	}
}

func sourceConciseParents(symbols []sourceintelligence.NormalizedSymbol) map[string]sourceintelligence.NormalizedSymbol {
	parents := make(map[string]sourceintelligence.NormalizedSymbol, len(symbols))
	for _, symbol := range symbols {
		parents[symbol.ID] = symbol
	}
	return parents
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
		return symbol.QualifiedName == query || (symbol.QualifiedName == symbol.Name && symbol.MatchesOriginalName(query))
	default:
		return symbol.Name == query || symbol.MatchesOriginalName(query)
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

// sourceOutputJSONByteCounter accepts complete encoded chunks without retaining
// the JSON output only needed for its byte-accurate response budget.
type sourceOutputJSONByteCounter struct {
	bytes int64
}

func (counter *sourceOutputJSONByteCounter) Write(data []byte) (int, error) {
	if int64(len(data)) > int64(^uint64(0)>>1)-counter.bytes {
		return 0, fmt.Errorf("source output byte count overflow")
	}
	counter.bytes += int64(len(data))
	return len(data), nil
}

// sourceEncodedJSONBytes measures the same JSON output size as json.Marshal,
// without retaining the temporary encoded result. Encoder adds one newline.
func sourceEncodedJSONBytes(value any) (int64, error) {
	var counter sourceOutputJSONByteCounter
	if err := json.NewEncoder(&counter).Encode(value); err != nil {
		return 0, err
	}
	if counter.bytes == 0 {
		return 0, operation.New(operation.KindUnknown, "source output encoder returned no data")
	}
	return counter.bytes - 1, nil
}

func enforceSourceOutputBudget(output SourceSymbolsOutput, maximum int64) error {
	encodedBytes, err := sourceEncodedJSONBytes(output)
	if err != nil {
		return operation.Wrap(operation.KindUnknown, "source_symbols", "", err)
	}
	if encodedBytes > maximum {
		return operation.Wrap(operation.KindLimit, "source_symbols", "", fmt.Errorf("source output size %d exceeds limit %d", encodedBytes, maximum))
	}
	return nil
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
