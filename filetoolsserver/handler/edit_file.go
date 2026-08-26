package handler

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pmezard/go-difflib/difflib"
)

// HandleEditFile is retained only as a package-level compatibility bridge for
// pre-R23 regression coverage. It is not registered as an MCP tool.
// Deprecated: MCP callers use HandleEditFilePreview and HandleEditFileApply.
func (h *Handler) HandleEditFile(ctx context.Context, _ *mcp.CallToolRequest, input EditFileInput) (*mcp.CallToolResult, EditFileOutput, error) {
	action, err := validateEditActionInput(input)
	if err != nil {
		return errorResultFromError(err), EditFileOutput{}, nil
	}
	switch action {
	case editActionPreview:
		return h.handleEditPreview(ctx, input)
	case editActionApply:
		return h.handleEditApply(ctx, input.PreviewID)
	default:
		return h.handleDirectEdit(ctx, input)
	}
}

// applyEdits applies edits sequentially, trying exact then whitespace-flexible match.
// On failure it returns ErrEditNoMatch with a hint pointing at the closest match.
func applyEdits(content string, edits []EditOperation) (string, error) {
	modifiedContent := content

	for _, edit := range edits {
		if edit.OldText == "" {
			return "", ErrOldTextEmpty
		}

		normalizedOld := ConvertLineEndings(edit.OldText, LineEndingLF)
		normalizedNew := ConvertLineEndings(edit.NewText, LineEndingLF)

		// Try exact match first
		if strings.Contains(modifiedContent, normalizedOld) {
			modifiedContent = strings.Replace(modifiedContent, normalizedOld, normalizedNew, 1)
			continue
		}

		// Try whitespace-flexible line matching.
		matched, result := tryFlexibleMatch(modifiedContent, normalizedOld, normalizedNew)
		if matched {
			modifiedContent = result
			continue
		}

		// Fuzzy matching is opt-in, complexity-bounded, and requires one unique
		// best candidate at or above the requested similarity threshold.
		if edit.Similarity != nil {
			matched, result, fuzzyErr := tryFuzzyMatch(modifiedContent, normalizedOld, normalizedNew, *edit.Similarity)
			if fuzzyErr != nil {
				return "", fuzzyErr
			}
			if matched {
				modifiedContent = result
				continue
			}
		}

		return "", noMatchError(modifiedContent, normalizedOld, edit.OldText)
	}

	return modifiedContent, nil
}

// noMatchError wraps ErrEditNoMatch, appending the closest matching block if found.
func noMatchError(content, normalizedOld, rawOld string) error {
	line, count := longestMatchingBlock(content, normalizedOld)
	if count == 0 {
		return fmt.Errorf("%w:\n%s", ErrEditNoMatch, rawOld)
	}

	lines := strings.Split(content, "\n")
	start := max(0, line-1)
	end := min(len(lines), line+count+1)
	snippet := strings.Join(lines[start:end], "\n")

	return fmt.Errorf("%w:\n%s\n\n"+
		"HINT: the closest match starts at line %d (%d consecutive lines matched, ignoring whitespace).\n"+
		"Actual file content there:\n%s\n\n"+
		"Copy the snippet above into oldText and retry",
		ErrEditNoMatch, rawOld, line+1, count, snippet)
}

// longestMatchingBlock returns the start line and length of the longest run of
// consecutive lines (ignoring whitespace) shared by content and oldText, or (-1, 0).
func longestMatchingBlock(content, oldText string) (startLine, length int) {
	contentLines := strings.Split(content, "\n")
	oldLines := strings.Split(oldText, "\n")

	startLine, length = -1, 0
	for i := range contentLines {
		for j := range oldLines {
			n := 0
			for i+n < len(contentLines) && j+n < len(oldLines) &&
				strings.TrimSpace(contentLines[i+n]) == strings.TrimSpace(oldLines[j+n]) {
				n++
			}
			if n > length {
				startLine, length = i, n
			}
		}
	}
	return startLine, length
}

// tryFlexibleMatch matches oldText ignoring whitespace differences, preserving file indentation.
func tryFlexibleMatch(content, oldText, newText string) (bool, string) {
	oldLines := strings.Split(oldText, "\n")
	contentLines := strings.Split(content, "\n")

	if len(contentLines) < len(oldLines) {
		return false, ""
	}

	for i := 0; i <= len(contentLines)-len(oldLines); i++ {
		potentialMatch := contentLines[i : i+len(oldLines)]

		isMatch := true
		for j, oldLine := range oldLines {
			if strings.TrimSpace(oldLine) != strings.TrimSpace(potentialMatch[j]) {
				isMatch = false
				break
			}
		}

		if isMatch {
			originalIndent := getLeadingWhitespace(contentLines[i])
			newLines := strings.Split(newText, "\n")

			for j := range newLines {
				if j == 0 {
					newLines[j] = originalIndent + strings.TrimLeft(newLines[j], " \t")
				} else {
					newLines[j] = adjustRelativeIndent(oldLines, newLines[j], j, originalIndent)
				}
			}

			result := make([]string, 0, len(contentLines)-len(oldLines)+len(newLines))
			result = append(result, contentLines[:i]...)
			result = append(result, newLines...)
			result = append(result, contentLines[i+len(oldLines):]...)

			return true, strings.Join(result, "\n")
		}
	}

	return false, ""
}

// adjustRelativeIndent applies baseIndent plus the indentation delta between old and new lines.
func adjustRelativeIndent(oldLines []string, newLine string, lineIndex int, baseIndent string) string {
	if lineIndex >= len(oldLines) {
		return newLine
	}

	oldIndent := getLeadingWhitespace(oldLines[lineIndex])
	newIndent := getLeadingWhitespace(newLine)

	relativeIndent := len(newIndent) - len(oldIndent)
	trimmedContent := strings.TrimLeft(newLine, " \t")
	switch {
	case relativeIndent > 0:
		return baseIndent + strings.Repeat(" ", relativeIndent) + trimmedContent
	case relativeIndent < 0:
		// Negative indent: trim characters from the end of baseIndent
		trim := -relativeIndent
		if trim >= len(baseIndent) {
			return trimmedContent
		}
		return baseIndent[:len(baseIndent)-trim] + trimmedContent
	default:
		return baseIndent + trimmedContent
	}
}

func getLeadingWhitespace(s string) string {
	for i, c := range s {
		if c != ' ' && c != '\t' {
			return s[:i]
		}
	}
	return s // entire string is whitespace
}

const (
	unifiedDiffContextLines         = 3
	boundedUnifiedDiffMaxWindowByte = 512 * 1024
)

func createUnifiedDiff(original, modified, filepath string) string {
	if diff, ok := createBoundedUnifiedDiff(original, modified, filepath); ok {
		return diff
	}
	return createUnifiedDiffFull(original, modified, filepath)
}

func createBoundedUnifiedDiff(original, modified, filepath string) (string, bool) {
	if original == modified {
		return "", true
	}

	commonLimit := min(len(original), len(modified))
	commonPrefix := 0
	for commonPrefix < commonLimit && original[commonPrefix] == modified[commonPrefix] {
		commonPrefix++
	}
	changeLineStart := 0
	if commonPrefix > 0 {
		if newline := strings.LastIndexByte(original[:commonPrefix], '\n'); newline >= 0 {
			changeLineStart = newline + 1
		}
	}

	maxSuffix := min(len(original)-commonPrefix, len(modified)-commonPrefix)
	commonSuffix := 0
	for commonSuffix < maxSuffix && original[len(original)-1-commonSuffix] == modified[len(modified)-1-commonSuffix] {
		commonSuffix++
	}
	oldSuffixStart := len(original) - commonSuffix
	newSuffixStart := len(modified) - commonSuffix
	if commonSuffix > 0 && !bothLineStarts(original, oldSuffixStart, modified, newSuffixStart) {
		newline := strings.IndexByte(original[oldSuffixStart:], '\n')
		if newline < 0 {
			oldSuffixStart = len(original)
			newSuffixStart = len(modified)
		} else {
			oldSuffixStart += newline + 1
			newSuffixStart += newline + 1
		}
	}

	windowStart := unifiedDiffContextStart(original, changeLineStart, unifiedDiffContextLines)
	oldWindowEnd := unifiedDiffContextEnd(original, oldSuffixStart, unifiedDiffContextLines)
	newWindowEnd := unifiedDiffContextEnd(modified, newSuffixStart, unifiedDiffContextLines)
	if oldWindowEnd-windowStart+newWindowEnd-windowStart > boundedUnifiedDiffMaxWindowByte {
		return "", false
	}

	oldLineOffset := strings.Count(original[:windowStart], "\n")
	newLineOffset := strings.Count(modified[:windowStart], "\n")
	diff := createUnifiedDiffFull(original[windowStart:oldWindowEnd], modified[windowStart:newWindowEnd], filepath)
	adjusted, ok := adjustUnifiedDiffHunkOffsets(diff, oldLineOffset, newLineOffset)
	if !ok {
		return "", false
	}
	return adjusted, true
}

func bothLineStarts(left string, leftOffset int, right string, rightOffset int) bool {
	leftStart := leftOffset == 0 || leftOffset > 0 && left[leftOffset-1] == '\n'
	rightStart := rightOffset == 0 || rightOffset > 0 && right[rightOffset-1] == '\n'
	return leftStart && rightStart
}

func unifiedDiffContextStart(text string, lineStart, contextLines int) int {
	position := min(max(lineStart, 0), len(text))
	for range contextLines {
		if position == 0 {
			break
		}
		searchEnd := position - 1
		newline := strings.LastIndexByte(text[:searchEnd], '\n')
		if newline < 0 {
			position = 0
			break
		}
		position = newline + 1
	}
	return position
}

func unifiedDiffContextEnd(text string, lineStart, contextLines int) int {
	position := min(max(lineStart, 0), len(text))
	for range contextLines {
		if position >= len(text) {
			break
		}
		newline := strings.IndexByte(text[position:], '\n')
		if newline < 0 {
			return len(text)
		}
		position += newline + 1
	}
	return position
}

func adjustUnifiedDiffHunkOffsets(diff string, oldOffset, newOffset int) (string, bool) {
	if diff == "" || oldOffset == 0 && newOffset == 0 {
		return diff, true
	}
	var output strings.Builder
	output.Grow(len(diff) + 32)
	for start := 0; start < len(diff); {
		lineEnd := strings.IndexByte(diff[start:], '\n')
		hasNewline := lineEnd >= 0
		if hasNewline {
			lineEnd += start
		} else {
			lineEnd = len(diff)
		}
		line := diff[start:lineEnd]
		if strings.HasPrefix(line, "@@ -") {
			adjusted, ok := adjustUnifiedDiffHunkHeader(line, oldOffset, newOffset)
			if !ok {
				return "", false
			}
			output.WriteString(adjusted)
		} else {
			output.WriteString(line)
		}
		if hasNewline {
			output.WriteByte('\n')
			start = lineEnd + 1
		} else {
			start = lineEnd
		}
	}
	return output.String(), true
}

func adjustUnifiedDiffHunkHeader(line string, oldOffset, newOffset int) (string, bool) {
	if !strings.HasPrefix(line, "@@ -") {
		return "", false
	}
	oldStart, oldEnd, ok := parseUnifiedDiffLineNumber(line, 4)
	if !ok {
		return "", false
	}
	plusRelative := strings.Index(line[oldEnd:], " +")
	if plusRelative < 0 {
		return "", false
	}
	plus := oldEnd + plusRelative
	newNumberAt := plus + 2
	newStart, newEnd, ok := parseUnifiedDiffLineNumber(line, newNumberAt)
	if !ok {
		return "", false
	}
	var output strings.Builder
	output.Grow(len(line) + 24)
	output.WriteString(line[:4])
	output.WriteString(strconv.Itoa(oldStart + oldOffset))
	output.WriteString(line[oldEnd:newNumberAt])
	output.WriteString(strconv.Itoa(newStart + newOffset))
	output.WriteString(line[newEnd:])
	return output.String(), true
}

func parseUnifiedDiffLineNumber(line string, start int) (int, int, bool) {
	if start < 0 || start >= len(line) || line[start] < '0' || line[start] > '9' {
		return 0, start, false
	}
	value := 0
	end := start
	for end < len(line) && line[end] >= '0' && line[end] <= '9' {
		value = value*10 + int(line[end]-'0')
		end++
	}
	return value, end, true
}

func createUnifiedDiffFull(original, modified, filepath string) string {
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(original),
		B:        difflib.SplitLines(modified),
		FromFile: filepath,
		ToFile:   filepath,
		Context:  unifiedDiffContextLines,
	}
	text, _ := difflib.GetUnifiedDiffString(diff)
	return text
}

func isReadOnly(mode os.FileMode) bool {
	return mode&0200 == 0
}

// clearReadOnly adds owner write permission to the file.
func clearReadOnly(path string, currentMode os.FileMode) error {
	newMode := currentMode | 0200
	return os.Chmod(path, newMode)
}
