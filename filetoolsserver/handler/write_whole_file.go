package handler

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
)

const (
	writeWholeFileStateUnchanged = "unchanged"
	writeWholeFileStateCommitted = "committed"
	writeWholeFileStateUnknown   = "unknown"

	writeWholeFileClassificationTimeout = 30 * time.Second
)

func (h *Handler) HandleWriteWholeFile(ctx context.Context, req *mcp.CallToolRequest, input WriteWholeFileInput) (*mcp.CallToolResult, WriteWholeFileOutput, error) {
	v := h.ValidatePath(input.Path)
	if !v.Ok() {
		return v.Result, WriteWholeFileOutput{}, nil
	}
	expected, err := filesystem.CaptureSnapshot(v.Path)
	if err != nil {
		return errorResultFromError(fmt.Errorf("failed to inspect target: %w", err)), WriteWholeFileOutput{}, nil
	}
	initialExists := expected.Exists
	targetFingerprint := ""
	if initialExists && expected.Size <= h.maxFileBytes() {
		expected, err = filesystem.CaptureRegularFileSnapshotBounded(ctx, v.Path, h.maxFileBytes())
		if err != nil {
			return errorResultFromError(fmt.Errorf("failed to fingerprint target: %w", err)), WriteWholeFileOutput{}, nil
		}
		targetFingerprint, err = filesystem.FingerprintRegularFileSnapshot(expected)
		if err != nil {
			return errorResultFromError(err), WriteWholeFileOutput{}, nil
		}
	}

	policy, err := parseBOMPolicy(input.BOM, bomAuto)
	if err != nil {
		return errorResultFromError(err), WriteWholeFileOutput{}, nil
	}

	// Resolve encoding: explicit > preserve existing > configured default.
	encodingName, err := h.resolveWriteEncoding(input.Encoding, v.Path)
	if err != nil {
		return errorResultFromError(err), WriteWholeFileOutput{}, nil
	}

	document := textDocument{Charset: encodingName}
	if policy == bomPreserve {
		head, readErr := readFileHead(v.Path, 4)
		switch {
		case readErr == nil:
			if detected, found := fileEncoding.DetectBOM(head); found {
				document.BOM = bomInfo{HasBOM: true, Type: detected.Charset}
			}
		case !os.IsNotExist(readErr):
			return errorResultFromError(fmt.Errorf("failed to inspect existing BOM: %w", readErr)), WriteWholeFileOutput{}, nil
		}
	}

	contentToWrite, err := encodeTextDocument(document, input.Content, policy)
	if err != nil {
		return errorResultFromError(fmt.Errorf("failed to encode content: %w", err)), WriteWholeFileOutput{}, nil
	}
	resultFingerprint := filesystem.FingerprintRegularFileData(contentToWrite)
	resultSize := int64(len(contentToWrite))
	resultReadLimit := resultSize
	if resultReadLimit < 1 {
		resultReadLimit = 1
	}
	hasBOM, bomType := outputBOMMetadata(contentToWrite)
	output := WriteWholeFileOutput{
		Encoding:          encodingName,
		BOMPolicy:         string(policy),
		HasBOM:            hasBOM,
		BOMType:           bomType,
		TargetFingerprint: targetFingerprint,
		ResultFingerprint: resultFingerprint,
		ActualFingerprint: targetFingerprint,
		State:             writeWholeFileStateUnchanged,
	}

	select {
	case <-ctx.Done():
		return errorResultWithCode(ErrCodeCancelled, ctx.Err().Error()), WriteWholeFileOutput{}, nil
	default:
	}

	commit := h.ValidatePath(input.Path)
	if !commit.Ok() {
		return commit.Result, WriteWholeFileOutput{}, nil
	}
	if commit.Path != v.Path {
		return errorResultWithCode(ErrCodeConflict, "path changed while preparing write"), WriteWholeFileOutput{}, nil
	}

	mode := getFileMode(commit.Path)
	if err := h.replaceFile(commit.Path, contentToWrite, filesystem.ReplaceOptions{
		Mode:     mode,
		Expected: &expected,
	}); err != nil {
		failure := errorResultFromError(fmt.Errorf("failed to replace complete file contents: %w", err))
		return h.classifyWriteWholeFileFailure(input.Path, v.Path, expected, targetFingerprint, resultFingerprint, resultSize, output, failure)
	}

	post, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, commit.Path, resultReadLimit)
	if err != nil {
		return h.classifyWriteWholeFileFailure(input.Path, v.Path, expected, targetFingerprint, resultFingerprint, resultSize, output, errorResultFromError(err))
	}
	actualFingerprint, err := filesystem.FingerprintRegularFileSnapshot(post)
	if err != nil {
		return h.classifyWriteWholeFileFailure(input.Path, v.Path, expected, targetFingerprint, resultFingerprint, resultSize, output, errorResultFromError(err))
	}
	if actualFingerprint != resultFingerprint {
		failure := errorResultWithCode(ErrCodeConflict, "written file does not match the prepared result fingerprint")
		return h.classifyWriteWholeFileFailure(input.Path, v.Path, expected, targetFingerprint, resultFingerprint, resultSize, output, failure)
	}

	output.ActualFingerprint = actualFingerprint
	output.Changed = true
	output.State = writeWholeFileStateCommitted
	output.Applied = true
	output.Message = fmt.Sprintf("Successfully replaced complete file contents at %s with %d bytes (encoding: %s, BOM: %s)", input.Path, len(contentToWrite), encodingName, policy)
	return &mcp.CallToolResult{}, output, nil
}

func (h *Handler) classifyWriteWholeFileFailure(requestedPath, resolvedPath string, initial filesystem.FileSnapshot, targetFingerprint, resultFingerprint string, resultSize int64, output WriteWholeFileOutput, failure *mcp.CallToolResult) (*mcp.CallToolResult, WriteWholeFileOutput, error) {
	output.Applied = false
	output.Changed = false
	output.State = writeWholeFileStateUnknown
	output.ActualFingerprint = ""

	classificationCtx, cancel := context.WithTimeout(context.Background(), writeWholeFileClassificationTimeout)
	defer cancel()
	validation := h.ValidatePath(requestedPath)
	if validation.Ok() && validation.Path == resolvedPath && classificationCtx.Err() == nil {
		snapshot, err := filesystem.CaptureSnapshot(validation.Path)
		if err == nil {
			switch {
			case !snapshot.Exists && !initial.Exists:
				output.State = writeWholeFileStateUnchanged
			case snapshot.Exists:
				resultReadLimit := resultSize
				if resultReadLimit < 1 {
					resultReadLimit = 1
				}
				var actual string
				if snapshot.Size == resultSize {
					if post, captureErr := filesystem.CaptureRegularFileSnapshotBounded(classificationCtx, validation.Path, resultReadLimit); captureErr == nil {
						actual, _ = filesystem.FingerprintRegularFileSnapshot(post)
						output.ActualFingerprint = actual
					}
				}
				if targetFingerprint != "" && actual == targetFingerprint {
					output.State = writeWholeFileStateUnchanged
					break
				}
				metadataMatchesInitial := initial.Exists && initial.Verify(validation.Path) == nil
				switch {
				case actual == resultFingerprint && actual != "" && metadataMatchesInitial && targetFingerprint == "":
					// Without a bounded pre-state content fingerprint, identical metadata
					// cannot distinguish a pre-existing result from a completed replacement.
					output.State = writeWholeFileStateUnknown
				case actual == resultFingerprint && actual != "":
					output.State = writeWholeFileStateCommitted
					output.Changed = true
				case metadataMatchesInitial:
					output.State = writeWholeFileStateUnchanged
					output.ActualFingerprint = targetFingerprint
				default:
					output.State = writeWholeFileStateUnknown
				}
			}
		}
	}

	if output.State == writeWholeFileStateUnchanged {
		return failure, output, nil
	}
	message := "write_whole_file failed after the target may have changed"
	if failure != nil && len(failure.Content) > 0 {
		if text, ok := failure.Content[0].(*mcp.TextContent); ok && text.Text != "" {
			message = text.Text
		}
	}
	return errorResultWithCode(ErrCodePartialCommit, message), output, nil
}
