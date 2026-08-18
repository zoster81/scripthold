package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/concurrency"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/textstream"
)

const (
	defaultMaxMatches = 1000
	binaryCheckSize   = 8192 // 8KB to catch files with text header but binary payload
	grepMatchOverhead = 256
	grepLineOverhead  = 32
)

// HandleGrep searches for one or more regex patterns with bounded paging.
func (h *Handler) HandleGrep(ctx context.Context, req *mcp.CallToolRequest, input GrepInput) (*mcp.CallToolResult, GrepOutput, error) {
	patterns := make([]string, 0, 1+len(input.Patterns))
	if strings.TrimSpace(input.Pattern) != "" {
		patterns = append(patterns, input.Pattern)
	}
	for _, pattern := range input.Patterns {
		if strings.TrimSpace(pattern) != "" {
			patterns = append(patterns, pattern)
		}
	}
	if len(patterns) == 0 {
		return errorResult("pattern or patterns is required"), GrepOutput{}, nil
	}
	if len(input.Paths) == 0 {
		return errorResult("paths is required"), GrepOutput{}, nil
	}
	if input.Offset < 0 {
		return errorResult("offset must be non-negative"), GrepOutput{}, nil
	}
	outputMode := strings.ToLower(strings.TrimSpace(input.OutputMode))
	if outputMode == "" {
		outputMode = "content"
	}
	if outputMode != "content" && outputMode != "files_with_matches" && outputMode != "count" {
		return errorResult("outputMode must be content, files_with_matches, or count"), GrepOutput{}, nil
	}
	re, err := compilePatterns(patterns, input.CaseSensitive)
	if err != nil {
		return errorResult(fmt.Sprintf("invalid regex pattern: %v", err)), GrepOutput{}, nil
	}
	maxMatches := input.MaxMatches
	if maxMatches <= 0 {
		maxMatches = min(defaultMaxMatches, h.maxMatches())
	}
	if maxMatches > h.maxMatches() || input.Offset > h.maxMatches()-maxMatches {
		return errorResultWithCode(
			ErrCodeLimit,
			fmt.Sprintf("offset + maxMatches exceeds the configured limit %d", h.maxMatches()),
		), GrepOutput{}, nil
	}
	includes := append([]string(nil), input.Includes...)
	if input.Include != "" {
		includes = append(includes, input.Include)
	}
	excludes := append([]string(nil), input.Excludes...)
	if input.Exclude != "" {
		excludes = append(excludes, input.Exclude)
	}
	files, collectErr := h.collectFiles(ctx, input.Paths, includes, excludes, shouldRespectGitignore(input.RespectGitignore))
	if collectErr != nil {
		if operation.KindOf(collectErr) == operation.KindCancelled {
			return errorResultFromError(collectErr), GrepOutput{}, nil
		}
		return errorResult("grep traversal failed: " + collectErr.Error()), GrepOutput{}, nil
	}
	if len(files) == 0 {
		return &mcp.CallToolResult{}, GrepOutput{Matches: []GrepMatch{}, FilesSearched: 0, CoverageComplete: true}, nil
	}

	output := GrepOutput{Matches: []GrepMatch{}, FilesSearched: len(files)}
	failures := newPartialFileErrorCollector(h.maxOutputBytes())
	if outputMode == "files_with_matches" || outputMode == "count" {
		needed := input.Offset + maxMatches + 1
		summaries, summaryTruncated, filesScanned, summaryErr := h.searchFileSummaries(ctx, files, re, input, needed, failures)
		if summaryErr != nil {
			return errorResultFromError(summaryErr), GrepOutput{}, nil
		}
		output.FilesScanned = filesScanned
		output.FilesSkipped = failures.Total()
		output.CoverageComplete = output.FilesSkipped == 0 && !summaryTruncated && output.FilesScanned == len(files)
		output.FilesMatched = len(summaries)
		start, end := pageBounds(len(summaries), input.Offset, maxMatches)
		selected := summaries[start:end]
		if outputMode == "files_with_matches" {
			output.Files = make([]string, len(selected))
			for index, summary := range selected {
				output.Files[index] = summary.Path
			}
			output.TotalMatches = len(output.Files)
		} else {
			output.Counts = append([]GrepFileCount(nil), selected...)
			for _, summary := range summaries {
				output.TotalMatches += summary.Count
			}
		}
		output.Truncated = summaryTruncated || end < len(summaries)
		if output.Truncated {
			output.NextOffset = end
		}
		applyGrepFailures(&output, failures, remainingGrepDiagnosticBudget(h.maxOutputBytes(), grepSummaryRetainedBytes(output)))
		return &mcp.CallToolResult{}, output, nil
	}

	scanLimit := input.Offset + maxMatches + 1
	matches, filesMatched, filesScanned, scanComplete, scanTruncated, err := h.searchFiles(ctx, files, re, input, scanLimit, failures)
	if err != nil {
		return errorResultFromError(err), GrepOutput{}, nil
	}
	output.FilesScanned = filesScanned
	output.FilesSkipped = failures.Total()
	output.CoverageComplete = output.FilesSkipped == 0 && scanComplete && output.FilesScanned == len(files)
	output.FilesMatched = filesMatched
	start, end := pageBounds(len(matches), input.Offset, maxMatches)
	output.Matches = append([]GrepMatch(nil), matches[start:end]...)
	if input.MatchesOnly {
		for index := range output.Matches {
			output.Matches[index].Text = re.FindString(output.Matches[index].Text)
		}
	}
	output.TotalMatches = len(output.Matches)
	output.Truncated = scanTruncated || end < len(matches)
	if output.Truncated {
		output.NextOffset = end
	}
	applyGrepFailures(&output, failures, remainingGrepDiagnosticBudget(h.maxOutputBytes(), grepMatchesRetainedBytes(output.Matches)))
	return &mcp.CallToolResult{}, output, nil
}

func applyGrepFailures(output *GrepOutput, failures *partialFileErrorCollector, detailBudget int64) {
	if output == nil || failures == nil {
		return
	}
	output.FilesSkipped = failures.Total()
	output.SkippedFiles = failures.DetailsWithinBudget(detailBudget)
	output.SkippedFilesOmitted = failures.Total() - len(output.SkippedFiles)
	output.SkippedFilesTruncated = output.SkippedFilesOmitted > 0
}

func remainingGrepDiagnosticBudget(maxOutputBytes, retainedResultBytes int64) int64 {
	if maxOutputBytes <= retainedResultBytes {
		return 0
	}
	return maxOutputBytes - retainedResultBytes
}

func grepSummaryRetainedBytes(output GrepOutput) int64 {
	var total int64
	for _, path := range output.Files {
		total = saturatingAdd(total, int64(grepMatchOverhead+len(path)))
	}
	for _, count := range output.Counts {
		total = saturatingAdd(total, int64(grepMatchOverhead+len(count.Path)+32))
	}
	return total
}

func pageBounds(length, offset, limit int) (int, int) {
	start := min(offset, length)
	end := min(length, start+limit)
	return start, end
}

func compilePatterns(patterns []string, caseSensitive *bool) (*regexp.Regexp, error) {
	wrapped := make([]string, len(patterns))
	for index, pattern := range patterns {
		wrapped[index] = "(?:" + pattern + ")"
	}
	combined := strings.Join(wrapped, "|")
	if caseSensitive != nil && !*caseSensitive {
		combined = "(?i:" + combined + ")"
	}
	return regexp.Compile(combined)
}

// collectFiles gathers all files to search from the given paths.
func (h *Handler) collectFiles(ctx context.Context, paths, includes, excludes []string, respectGitignore bool) ([]string, error) {
	var files []string
	seen := make(map[string]bool)
	allowedDirs := h.ResolvedAllowedDirs()
	for _, path := range paths {
		// Check for cancellation between paths
		select {
		case <-ctx.Done():
			return files, ctx.Err()
		default:
		}
		v := h.ValidatePath(path)
		if !v.Ok() {
			continue
		}
		info, err := os.Stat(v.Path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			err := filesystem.Walk(ctx, v.Path, filesystem.WalkOptions{
				ResolvedAllowedDirs: allowedDirs,
				RespectGitignore:    respectGitignore,
				OnError: func(path string, _ int, err error) error {
					slog.Debug("skipping path due to error", "path", path, "error", err)
					return nil
				},
			}, func(entry filesystem.Entry) (filesystem.WalkAction, error) {
				if entry.DirEntry.IsDir() {
					return filesystem.WalkContinue, nil
				}
				if shouldIncludeFile(entry.Path, includes, excludes) && !seen[entry.ResolvedPath] {
					seen[entry.ResolvedPath] = true
					files = append(files, entry.ResolvedPath)
				}
				return filesystem.WalkContinue, nil
			})
			if err != nil {
				if ctx.Err() != nil {
					return files, ctx.Err()
				}
				return files, err
			}
		} else if shouldIncludeFile(v.Path, includes, excludes) && !seen[v.Path] {
			seen[v.Path] = true
			files = append(files, v.Path)
		}
	}
	return files, nil
}

// shouldIncludeFile checks if a file matches include/exclude patterns.
// Matches against both full path (with forward slashes) and basename.
func shouldIncludeFile(filePath string, includes, excludes []string) bool {
	base := filepath.Base(filePath)
	normalized := filepath.ToSlash(filePath)
	matchesAny := func(patterns []string) bool {
		for _, pattern := range patterns {
			if matchedBase, _ := filepath.Match(pattern, base); matchedBase {
				return true
			}
			if matchedPath, _ := filepath.Match(filepath.ToSlash(pattern), normalized); matchedPath {
				return true
			}
		}
		return false
	}
	if matchesAny(excludes) {
		return false
	}
	return len(includes) == 0 || matchesAny(includes)
}

func (h *Handler) searchFileSummaries(ctx context.Context, files []string, re *regexp.Regexp, input GrepInput, limit int, failures *partialFileErrorCollector) ([]GrepFileCount, bool, int, error) {
	summaries := make([]GrepFileCount, 0, min(limit, len(files)))
	filesScanned := 0
	for index, path := range files {
		count, err := h.countMatchingLines(ctx, path, re, input.Encoding)
		if err != nil {
			kind := operation.KindOf(err)
			if ctx.Err() != nil || kind == operation.KindCancelled {
				if ctx.Err() != nil {
					return nil, false, filesScanned, ctx.Err()
				}
				return nil, false, filesScanned, err
			}
			if kind == operation.KindLimit {
				return nil, false, filesScanned, err
			}
			failures.Add(path, err)
			slog.Debug("skipping file due to grep summary error", "path", path, "error", err)
			continue
		}
		filesScanned++
		if count == 0 {
			continue
		}
		summaries = append(summaries, GrepFileCount{Path: path, Count: count})
		if len(summaries) >= limit {
			return summaries, index < len(files)-1, filesScanned, nil
		}
	}
	return summaries, false, filesScanned, nil
}

func (h *Handler) countMatchingLines(ctx context.Context, path string, re *regexp.Regexp, requestedEncoding string) (int, error) {
	validated := h.ValidatePath(path)
	if !validated.Ok() {
		return 0, validated.Err
	}
	stream, err := h.openDecodedTextStream(ctx, validated.Path, requestedEncoding)
	if err != nil {
		return 0, err
	}
	defer stream.Close()

	prefix := make([]byte, binaryCheckSize)
	read, prefixErr := io.ReadFull(stream.Reader, prefix)
	if prefixErr != nil && !errors.Is(prefixErr, io.EOF) && !errors.Is(prefixErr, io.ErrUnexpectedEOF) {
		return 0, prefixErr
	}
	prefix = prefix[:read]
	if len(prefix) == 0 || isLikelyBinaryText(string(trimIncompleteUTF8Suffix(prefix))) {
		return 0, nil
	}

	count := 0
	reader := io.MultiReader(bytes.NewReader(prefix), stream.Reader)
	_, scanErr := textstream.ScanLines(ctx, reader, h.maxLineBytes(), func(line textstream.Line) error {
		for _, segment := range bytes.Split(line.Data, []byte{'\r'}) {
			if re.Match(segment) {
				count++
			}
		}
		return nil
	})
	if scanErr != nil {
		return 0, scanErr
	}
	if _, err := stream.Finish(); err != nil {
		return 0, err
	}
	return count, nil
}

type grepFileResult struct {
	matches   []GrepMatch
	truncated bool
	err       error
}

type grepPlan struct {
	path               string
	worstRetainedBytes int64
}

// searchFiles keeps deterministic file order while bounding both match count
// and retained output/context memory. Parallelism is used only when the
// aggregate decoded worst case fits the configured budget.
func (h *Handler) searchFiles(ctx context.Context, files []string, re *regexp.Regexp, input GrepInput, maxMatches int, failures *partialFileErrorCollector) ([]GrepMatch, int, int, bool, bool, error) {
	budget := h.maxOutputBytes()
	plans, worstTotal := h.planGrep(files, input, maxMatches)
	maxWorkers := 0
	if worstTotal > budget {
		maxWorkers = 1
	}

	remaining := maxMatches
	var remainingMatches atomic.Int64
	remainingMatches.Store(int64(maxMatches))
	var remainingOutput atomic.Int64
	remainingOutput.Store(budget)
	allMatches := make([]GrepMatch, 0, min(maxMatches, 64))
	matchedFiles := make(map[string]struct{})
	filesScanned := 0
	truncated := false
	var terminalErr error

	stats := concurrency.ProcessOrdered(ctx, plans, concurrency.Options{MaxWorkers: maxWorkers}, func(ctx context.Context, _ int, plan grepPlan) grepFileResult {
		limit := int(remainingMatches.Load())
		if limit <= 0 {
			limit = 1
		}
		fileBudget := plan.worstRetainedBytes
		if maxWorkers == 1 || fileBudget <= 0 {
			fileBudget = remainingOutput.Load()
		}
		if fileBudget <= 0 {
			return grepFileResult{err: grepBudgetError(plan.path, 0)}
		}
		return h.searchSingleFileWithBudget(ctx, plan.path, re, input, limit, fileBudget)
	}, func(index int, current grepFileResult) bool {
		if current.err != nil {
			kind := operation.KindOf(current.err)
			if kind == operation.KindLimit || kind == operation.KindCancelled {
				terminalErr = current.err
				return false
			}
			if ctx.Err() != nil {
				terminalErr = ctx.Err()
				return false
			}
			failures.Add(plans[index].path, current.err)
			return true
		}
		filesScanned++
		if len(current.matches) > 0 {
			take := min(remaining, len(current.matches))
			selected := current.matches[:take]
			allMatches = append(allMatches, selected...)
			remaining -= take
			remainingMatches.Store(int64(remaining))
			remainingOutput.Add(-grepMatchesRetainedBytes(selected))
			for _, match := range selected {
				matchedFiles[match.Path] = struct{}{}
			}
			if current.truncated || take < len(current.matches) {
				truncated = true
			}
		}

		if remaining == 0 && truncated {
			return false
		}
		return ctx.Err() == nil
	})
	if terminalErr == nil && ctx.Err() != nil {
		terminalErr = ctx.Err()
	}
	scanComplete := terminalErr == nil && stats.Committed == len(plans) && !truncated

	return allMatches, len(matchedFiles), filesScanned, scanComplete, truncated, terminalErr
}

func (h *Handler) planGrep(files []string, input GrepInput, maxMatches int) ([]grepPlan, int64) {
	plans := make([]grepPlan, len(files))
	var total int64
	contextFactor := saturatingAdd(2, int64(max(0, input.ContextBefore)))
	contextFactor = saturatingAdd(contextFactor, int64(max(0, input.ContextAfter)))
	for index, path := range files {
		plans[index].path = path
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			plans[index].worstRetainedBytes = math.MaxInt64
			total = math.MaxInt64
			continue
		}
		decoded := worstDecodedBytes(info.Size())
		contentWorst := saturatingMultiply(decoded, contextFactor)
		metadataPerMatch := int64(grepMatchOverhead + len(path) + 64)
		metadataWorst := saturatingMultiply(int64(maxMatches), metadataPerMatch)
		plans[index].worstRetainedBytes = saturatingAdd(contentWorst, metadataWorst)
		total = saturatingAdd(total, plans[index].worstRetainedBytes)
	}
	return plans, total
}

func saturatingMultiply(first, second int64) int64 {
	if first <= 0 || second <= 0 {
		return 0
	}
	if first > math.MaxInt64/second {
		return math.MaxInt64
	}
	return first * second
}

func saturatingAdd(first, second int64) int64 {
	if second > 0 && first > math.MaxInt64-second {
		return math.MaxInt64
	}
	return first + second
}

var errStopGrepScan = errors.New("grep scan complete")

type pendingGrepMatch struct {
	match          GrepMatch
	remainingAfter int
}

func grepBudgetError(path string, budget int64) error {
	return operation.Wrap(
		operation.KindLimit,
		"grep_text_files",
		path,
		fmt.Errorf("retained grep state exceeds the %d-byte grep output budget", budget),
	)
}

func grepMatchesRetainedBytes(matches []GrepMatch) int64 {
	var total int64
	for _, match := range matches {
		total = saturatingAdd(total, int64(grepMatchOverhead+len(match.Path)+len(match.Encoding)+len(match.Text)))
		for _, line := range match.Before {
			total = saturatingAdd(total, int64(grepLineOverhead+len(line)))
		}
		for _, line := range match.After {
			total = saturatingAdd(total, int64(grepLineOverhead+len(line)))
		}
	}
	return total
}

// searchSingleFileWithBudget decodes and searches one file incrementally while
// bounding line, context, match, and binary-probe state.
func (h *Handler) searchSingleFileWithBudget(ctx context.Context, path string, re *regexp.Regexp, input GrepInput, maxMatches int, maxOutputBytes int64) grepFileResult {
	result := grepFileResult{}
	if maxMatches <= 0 {
		return result
	}

	validated := h.ValidatePath(path)
	if !validated.Ok() {
		result.err = validated.Err
		return result
	}

	stream, err := h.openDecodedTextStream(ctx, validated.Path, input.Encoding)
	if err != nil {
		result.err = err
		return result
	}
	defer stream.Close()

	prefix := make([]byte, binaryCheckSize)
	read, prefixErr := io.ReadFull(stream.Reader, prefix)
	if prefixErr != nil && !errors.Is(prefixErr, io.EOF) && !errors.Is(prefixErr, io.ErrUnexpectedEOF) {
		result.err = prefixErr
		return result
	}
	prefix = prefix[:read]
	if len(prefix) == 0 || isLikelyBinaryText(string(trimIncompleteUTF8Suffix(prefix))) {
		return result
	}
	reader := io.MultiReader(bytes.NewReader(prefix), stream.Reader)

	result.matches = make([]GrepMatch, 0, min(maxMatches, 16))
	beforeCapacity := min(max(0, input.ContextBefore), 64)
	before := make([]string, 0, beforeCapacity)
	pending := make([]pendingGrepMatch, 0, min(maxMatches, 16))
	logicalLine := 0
	var retainedBytes int64

	reserve := func(amount int64) error {
		if amount < 0 || amount > maxOutputBytes-retainedBytes {
			return grepBudgetError(validated.Path, maxOutputBytes)
		}
		retainedBytes += amount
		return nil
	}
	release := func(amount int64) {
		retainedBytes -= amount
		if retainedBytes < 0 {
			retainedBytes = 0
		}
	}

	processLine := func(line []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		logicalLine++

		if len(pending) > 0 {
			remainingPending := pending[:0]
			for _, current := range pending {
				if current.remainingAfter > 0 {
					if err := reserve(int64(grepLineOverhead + len(line))); err != nil {
						return err
					}
					current.match.After = append(current.match.After, string(line))
					current.remainingAfter--
				}
				if current.remainingAfter == 0 {
					result.matches = append(result.matches, current.match)
				} else {
					remainingPending = append(remainingPending, current)
				}
			}
			pending = remainingPending
		}

		if loc := re.FindIndex(line); loc != nil {
			selected := len(result.matches) + len(pending)
			if selected < maxMatches {
				required := int64(grepMatchOverhead + len(validated.Path) + len(stream.Charset) + len(line))
				for _, contextLine := range before {
					required = saturatingAdd(required, int64(grepLineOverhead+len(contextLine)))
				}
				if err := reserve(required); err != nil {
					return err
				}
				match := GrepMatch{
					Path:     validated.Path,
					Line:     logicalLine,
					Column:   loc[0] + 1,
					Text:     string(line),
					Encoding: stream.Charset,
				}
				if len(before) > 0 {
					match.Before = make([]string, len(before))
					for index, contextLine := range before {
						match.Before[index] = strings.Clone(contextLine)
					}
				}
				if input.ContextAfter > 0 {
					match.After = make([]string, 0, min(input.ContextAfter, 64))
					pending = append(pending, pendingGrepMatch{match: match, remainingAfter: input.ContextAfter})
				} else {
					result.matches = append(result.matches, match)
				}
			} else {
				result.truncated = true
			}
		}

		if input.ContextBefore > 0 {
			if len(before) == input.ContextBefore {
				release(int64(grepLineOverhead + len(before[0])))
				copy(before, before[1:])
				before = before[:len(before)-1]
			}
			if err := reserve(int64(grepLineOverhead + len(line))); err != nil {
				return err
			}
			before = append(before, string(line))
		}
		if result.truncated && len(pending) == 0 {
			return errStopGrepScan
		}
		return nil
	}

	_, scanErr := textstream.ScanLines(ctx, reader, h.maxLineBytes(), func(line textstream.Line) error {
		for _, segment := range bytes.Split(line.Data, []byte{'\r'}) {
			if err := processLine(segment); err != nil {
				return err
			}
		}
		return nil
	})
	if scanErr != nil && !errors.Is(scanErr, errStopGrepScan) {
		result.matches = nil
		result.err = scanErr
		return result
	}

	for _, current := range pending {
		result.matches = append(result.matches, current.match)
	}
	if scanErr == nil {
		if _, err := stream.Finish(); err != nil {
			result.matches = nil
			result.err = err
		}
	}
	return result
}

// trimIncompleteUTF8Suffix removes only a trailing partial UTF-8 sequence from
// a bounded probe. Invalid bytes inside the probe remain visible to binary
// classification.
func trimIncompleteUTF8Suffix(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	for trim := 1; trim < utf8.UTFMax && trim < len(data); trim++ {
		candidate := data[:len(data)-trim]
		if utf8.Valid(candidate) {
			return candidate
		}
	}
	return data
}

// isLikelyBinaryText classifies decoded content instead of raw encoded bytes.
func isLikelyBinaryText(content string) bool {
	if !utf8.ValidString(content) {
		return true
	}

	controlCount := 0
	runeCount := 0
	for byteIndex, r := range content {
		if byteIndex >= binaryCheckSize {
			break
		}
		runeCount++
		if r == 0 {
			return true
		}
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			controlCount++
		}
	}
	return runeCount > 0 && controlCount*10 >= runeCount
}
