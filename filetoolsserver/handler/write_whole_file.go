package handler

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
)

func (h *Handler) HandleWriteWholeFile(ctx context.Context, req *mcp.CallToolRequest, input WriteWholeFileInput) (*mcp.CallToolResult, WriteWholeFileOutput, error) {
	v := h.ValidatePath(input.Path)
	if !v.Ok() {
		return v.Result, WriteWholeFileOutput{}, nil
	}
	expected, err := filesystem.CaptureSnapshot(v.Path)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to inspect target: %v", err)), WriteWholeFileOutput{}, nil
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
			return errorResult(fmt.Sprintf("failed to inspect existing BOM: %v", readErr)), WriteWholeFileOutput{}, nil
		}
	}

	contentToWrite, err := encodeTextDocument(document, input.Content, policy)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to encode content: %v", err)), WriteWholeFileOutput{}, nil
	}

	select {
	case <-ctx.Done():
		return errorResult(ctx.Err().Error()), WriteWholeFileOutput{}, nil
	default:
	}

	commit := h.ValidatePath(input.Path)
	if !commit.Ok() {
		return commit.Result, WriteWholeFileOutput{}, nil
	}
	if commit.Path != v.Path {
		return errorResult("path changed while preparing write"), WriteWholeFileOutput{}, nil
	}

	mode := getFileMode(commit.Path)
	if err := filesystem.ReplaceFile(commit.Path, contentToWrite, filesystem.ReplaceOptions{
		Mode:     mode,
		Expected: &expected,
	}); err != nil {
		return errorResult(fmt.Sprintf("failed to replace complete file contents: %v", err)), WriteWholeFileOutput{}, nil
	}

	hasBOM, bomType := outputBOMMetadata(contentToWrite)
	message := fmt.Sprintf("Successfully replaced complete file contents at %s with %d bytes (encoding: %s, BOM: %s)", input.Path, len(contentToWrite), encodingName, policy)
	return &mcp.CallToolResult{}, WriteWholeFileOutput{
		Message:   message,
		Encoding:  encodingName,
		BOMPolicy: string(policy),
		HasBOM:    hasBOM,
		BOMType:   bomType,
	}, nil
}
