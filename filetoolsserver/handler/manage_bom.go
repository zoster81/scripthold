package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/textstream"
)

func readFileHead(path string, maxBytes int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	buffer := make([]byte, max(0, maxBytes))
	read, err := io.ReadFull(file, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buffer[:read], nil
}

func detectBOMPrefix(session *filesystem.ReadSession) (fileEncoding.DetectionResult, bool, error) {
	length := min(int64(4), session.Size())
	prefix := make([]byte, int(length))
	if length > 0 {
		read, err := session.ReadAt(prefix, 0)
		if err != nil && err != io.EOF {
			return fileEncoding.DetectionResult{}, false, err
		}
		prefix = prefix[:read]
	}
	result, found := fileEncoding.DetectBOM(prefix)
	return result, found, nil
}

// HandleManageBom is retained only as a package-level compatibility bridge for
// pre-R23 regression coverage. It is not registered as an MCP tool.
// Deprecated: MCP callers use HandleManageBOMRead and HandleManageBOMApply.
func (h *Handler) HandleManageBom(ctx context.Context, req *mcp.CallToolRequest, input ManageBomInput) (*mcp.CallToolResult, ManageBomOutput, error) {
	validated := h.ValidatePath(input.Path)
	if !validated.Ok() {
		return validated.Result, ManageBomOutput{}, nil
	}

	action := strings.ToLower(input.Action)
	if action != "detect" && action != "strip" && action != "add" {
		return errorResult("action must be \"detect\", \"strip\", or \"add\""), ManageBomOutput{}, nil
	}

	session, err := filesystem.OpenReadSession(validated.Path)
	if err != nil {
		return errorResultFromError(err), ManageBomOutput{}, nil
	}
	defer session.Close()

	detected, hasBOM, err := detectBOMPrefix(session)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to inspect BOM: %v", err)), ManageBomOutput{}, nil
	}
	bomSize := 0
	if hasBOM {
		bomSize = fileEncoding.BOMSize(detected.Charset)
	}

	switch action {
	case "detect":
		if !hasBOM {
			return &mcp.CallToolResult{}, ManageBomOutput{Message: "No BOM detected"}, nil
		}
		return &mcp.CallToolResult{}, ManageBomOutput{
			Message:  fmt.Sprintf("BOM detected: %s (%d bytes)", detected.Charset, bomSize),
			HasBOM:   true,
			BOMType:  detected.Charset,
			BOMBytes: bomSize,
		}, nil

	case "strip":
		if !hasBOM {
			return &mcp.CallToolResult{}, ManageBomOutput{Message: "No BOM to strip"}, nil
		}
		if err := session.Start(int64(bomSize)); err != nil {
			return errorResultFromError(err), ManageBomOutput{}, nil
		}
		staged, err := filesystem.StageReplacement(
			validated.Path,
			textstream.WithContext(ctx, session),
			session.Mode().Perm(),
			nil,
		)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to stage BOM removal: %v", err)), ManageBomOutput{}, nil
		}
		defer staged.Cleanup()
		snapshot, err := session.Finish()
		if err != nil {
			return errorResultFromError(err), ManageBomOutput{}, nil
		}
		if err := session.Close(); err != nil {
			return errorResult(fmt.Sprintf("failed to close source file before commit: %v", err)), ManageBomOutput{}, nil
		}
		if result := h.commitBOMReplacement(input.Path, validated.Path, staged, snapshot); result != nil {
			return result, ManageBomOutput{}, nil
		}
		return &mcp.CallToolResult{}, ManageBomOutput{
			Message:  fmt.Sprintf("Stripped %s BOM (%d bytes) from %s", detected.Charset, bomSize, input.Path),
			BOMType:  detected.Charset,
			BOMBytes: bomSize,
			Changed:  true,
		}, nil

	case "add":
		if input.Encoding == "" {
			return errorResult("encoding is required for add action"), ManageBomOutput{}, nil
		}
		bom := fileEncoding.BOMBytesFor(input.Encoding)
		if len(bom) == 0 {
			return errorResult("encoding must be utf-8, utf-16-le, utf-16-be, utf-32-le, or utf-32-be"), ManageBomOutput{}, nil
		}
		if hasBOM {
			return errorResult(fmt.Sprintf("file already has a %s BOM", detected.Charset)), ManageBomOutput{}, nil
		}
		if err := session.Start(0); err != nil {
			return errorResultFromError(err), ManageBomOutput{}, nil
		}
		staged, err := filesystem.StageReplacement(
			validated.Path,
			io.MultiReader(bytes.NewReader(bom), textstream.WithContext(ctx, session)),
			session.Mode().Perm(),
			nil,
		)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to stage BOM addition: %v", err)), ManageBomOutput{}, nil
		}
		defer staged.Cleanup()
		snapshot, err := session.Finish()
		if err != nil {
			return errorResultFromError(err), ManageBomOutput{}, nil
		}
		if err := session.Close(); err != nil {
			return errorResult(fmt.Sprintf("failed to close source file before commit: %v", err)), ManageBomOutput{}, nil
		}
		if result := h.commitBOMReplacement(input.Path, validated.Path, staged, snapshot); result != nil {
			return result, ManageBomOutput{}, nil
		}
		charset := canonicalBOMEncoding(input.Encoding)
		return &mcp.CallToolResult{}, ManageBomOutput{
			Message:  fmt.Sprintf("Added %s BOM (%d bytes) to %s", charset, len(bom), input.Path),
			HasBOM:   true,
			BOMType:  charset,
			BOMBytes: len(bom),
			Changed:  true,
		}, nil
	}

	return errorResult("unsupported BOM action"), ManageBomOutput{}, nil
}

func (h *Handler) commitBOMReplacement(inputPath, preparedPath string, staged *filesystem.StagedReplacement, snapshot filesystem.FileSnapshot) *mcp.CallToolResult {
	commit := h.ValidatePath(inputPath)
	if !commit.Ok() {
		return commit.Result
	}
	if commit.Path != preparedPath {
		return errorResult("path changed while preparing BOM mutation")
	}
	if _, err := staged.Commit(filesystem.ReplaceOptions{Expected: &snapshot}); err != nil {
		return errorResult(fmt.Sprintf("failed to write file: %v", err))
	}
	return nil
}
