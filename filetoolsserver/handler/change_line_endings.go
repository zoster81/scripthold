package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/textstream"
)

// HandleChangeLineEndings converts line endings through a bounded raw-byte
// pipeline while preserving encoding, BOM state, and every unrelated byte.
func (h *Handler) HandleChangeLineEndings(ctx context.Context, req *mcp.CallToolRequest, input ChangeLineEndingsInput) (*mcp.CallToolResult, ChangeLineEndingsOutput, error) {
	validated := h.ValidatePath(input.Path)
	if !validated.Ok() {
		return validated.Result, ChangeLineEndingsOutput{}, nil
	}

	style := strings.ToLower(input.Style)
	if style != LineEndingLF && style != LineEndingCRLF {
		return errorResult("style must be \"lf\" or \"crlf\""), ChangeLineEndingsOutput{}, nil
	}

	stream, err := h.openDecodedTextStream(ctx, validated.Path, input.Encoding)
	if err != nil {
		return errorResultFromError(err), ChangeLineEndingsOutput{}, nil
	}
	defer stream.Close()

	rawSource := textstream.WithContext(ctx, stream.session)
	validatedSource, err := fileEncoding.NewValidatingReader(rawSource, stream.Charset)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to initialize encoding validation for %s: %v", stream.Charset, err)), ChangeLineEndingsOutput{}, nil
	}
	profile, ok := fileEncoding.LineEndingProfileFor(stream.Charset)
	if !ok {
		return errorResult(fmt.Sprintf("line-ending semantics are unavailable for encoding %s", stream.Charset)), ChangeLineEndingsOutput{}, nil
	}

	var transformed *textstream.LineEndingReader
	switch profile.Kind {
	case fileEncoding.LineEndingUTF16LE, fileEncoding.LineEndingUTF16BE:
		transformed, err = textstream.NewUTF16LineEndingReader(validatedSource, style, profile.Kind == fileEncoding.LineEndingUTF16LE)
	case fileEncoding.LineEndingUTF32LE, fileEncoding.LineEndingUTF32BE:
		transformed, err = textstream.NewUTF32LineEndingReader(validatedSource, style, profile.Kind == fileEncoding.LineEndingUTF32LE)
	case fileEncoding.LineEndingByte:
		if len(profile.CarriageReturn) != 1 || len(profile.LineFeed) != 1 {
			return errorResult(fmt.Sprintf("invalid single-byte line-ending profile for encoding %s", stream.Charset)), ChangeLineEndingsOutput{}, nil
		}
		transformed, err = textstream.NewSingleByteLineEndingReader(validatedSource, style, profile.CarriageReturn[0], profile.LineFeed[0])
	case fileEncoding.LineEndingHZ:
		transformed, err = textstream.NewHZLineEndingReader(validatedSource, style)
	default:
		return errorResult(fmt.Sprintf("unsupported line-ending profile %q for encoding %s", profile.Kind, stream.Charset)), ChangeLineEndingsOutput{}, nil
	}
	if err != nil {
		return errorResult(err.Error()), ChangeLineEndingsOutput{}, nil
	}

	outputReader := io.MultiReader(bytes.NewReader(stream.BOM.Bytes), transformed)
	staged, err := filesystem.StageReplacement(validated.Path, outputReader, stream.Mode.Perm(), nil)
	if err != nil {
		if errors.Is(err, fileEncoding.ErrInvalidEncodedSequence) {
			return errorResultFromError(fmt.Errorf("%w: invalid %s source bytes: %v", ErrEncodingDecode, stream.Charset, err)), ChangeLineEndingsOutput{}, nil
		}
		return errorResult(fmt.Sprintf("failed to prepare line-ending conversion: %v", err)), ChangeLineEndingsOutput{}, nil
	}
	defer staged.Cleanup()

	snapshot, err := stream.Finish()
	if err != nil {
		return errorResultFromError(err), ChangeLineEndingsOutput{}, nil
	}
	if err := stream.Close(); err != nil {
		return errorResult(fmt.Sprintf("failed to close source file before commit: %v", err)), ChangeLineEndingsOutput{}, nil
	}

	stats := transformed.Stats()
	originalStyle := determineStyle(stats.CRLFCount, stats.LFCount)
	linesChanged := stats.LFCount
	if style == LineEndingLF {
		linesChanged = stats.CRLFCount
	}
	if originalStyle == style || originalStyle == LineEndingNone {
		return &mcp.CallToolResult{}, ChangeLineEndingsOutput{
			Message:       fmt.Sprintf("File already uses %s line endings, no changes needed", style),
			OriginalStyle: originalStyle,
			NewStyle:      style,
			LinesChanged:  0,
		}, nil
	}

	commit := h.ValidatePath(input.Path)
	if !commit.Ok() {
		return commit.Result, ChangeLineEndingsOutput{}, nil
	}
	if commit.Path != validated.Path {
		return errorResult("path changed while preparing line-ending conversion"), ChangeLineEndingsOutput{}, nil
	}

	changed, err := staged.Commit(filesystem.ReplaceOptions{
		Expected:      &snapshot,
		SkipIdentical: true,
	})
	if err != nil {
		return errorResult(fmt.Sprintf("failed to write file: %v", err)), ChangeLineEndingsOutput{}, nil
	}
	if !changed {
		linesChanged = 0
	}

	return &mcp.CallToolResult{}, ChangeLineEndingsOutput{
		Message:       fmt.Sprintf("Converted %s from %s to %s (%d lines changed)", input.Path, originalStyle, style, linesChanged),
		OriginalStyle: originalStyle,
		NewStyle:      style,
		LinesChanged:  linesChanged,
	}, nil
}
