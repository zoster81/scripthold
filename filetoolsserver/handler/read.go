package handler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/textstream"
	textEncoding "golang.org/x/text/encoding"
)

// encodingResult holds the result of encoding resolution
type encodingResult struct {
	encoder            textEncoding.Encoding
	name               string
	detectedEncoding   string
	encodingConfidence int
	autoDetected       bool
}

func (h *Handler) HandleReadTextFile(ctx context.Context, req *mcp.CallToolRequest, input ReadTextFileInput) (*mcp.CallToolResult, ReadTextFileOutput, error) {
	v := h.ValidatePath(input.Path)
	if !v.Ok() {
		return v.Result, ReadTextFileOutput{}, nil
	}

	if input.maxOutputBytes <= 0 {
		input.maxOutputBytes = clampBudgetToInt(h.maxOutputBytes())
		input.outputLimitName = "read output limit"
	}
	output, err := h.readTextFileStream(ctx, v.Path, input)
	if err != nil {
		return errorResultFromError(err), ReadTextFileOutput{}, nil
	}
	return &mcp.CallToolResult{}, output, nil
}

type runeLimitedBuilder struct {
	builder           strings.Builder
	maxRunes          int
	maxBytes          int
	runeCount         int
	truncated         bool
	byteLimitExceeded bool
}

func (builder *runeLimitedBuilder) append(data []byte) {
	if len(data) == 0 {
		return
	}
	if builder.maxRunes <= 0 && builder.maxBytes <= 0 {
		_, _ = builder.builder.Write(data)
		return
	}

	for len(data) > 0 {
		if builder.maxRunes > 0 && builder.runeCount >= builder.maxRunes {
			builder.truncated = true
			return
		}
		_, size := utf8.DecodeRune(data)
		if size == 0 {
			return
		}
		if builder.maxBytes > 0 && builder.builder.Len()+size > builder.maxBytes {
			builder.byteLimitExceeded = true
			return
		}
		_, _ = builder.builder.Write(data[:size])
		builder.runeCount++
		data = data[size:]
	}
}

func (h *Handler) readTextFileStream(ctx context.Context, path string, input ReadTextFileInput) (ReadTextFileOutput, error) {
	stream, err := h.openDecodedTextStream(ctx, path, input.Encoding)
	if err != nil {
		return ReadTextFileOutput{}, err
	}
	defer stream.Close()

	rangeRequested := input.Offset != nil || input.Limit != nil
	startWanted := 1
	if input.Offset != nil && *input.Offset > 1 {
		startWanted = *input.Offset
	}
	lineLimit := 0
	if input.Limit != nil && *input.Limit > 0 {
		lineLimit = *input.Limit
	}
	maxCharacters := h.maxDecodedCharacters()
	if input.MaxCharacters != nil && *input.MaxCharacters > 0 {
		if *input.MaxCharacters > maxCharacters {
			return ReadTextFileOutput{}, operation.Wrap(
				operation.KindLimit,
				"read_text_file",
				path,
				fmt.Errorf("maxCharacters %d exceeds the configured limit %d", *input.MaxCharacters, maxCharacters),
			)
		}
		maxCharacters = *input.MaxCharacters
	}

	collector := runeLimitedBuilder{maxRunes: maxCharacters, maxBytes: input.maxOutputBytes}
	selectedCount := 0
	firstSelectedLine := 0
	lastSelectedLine := 0
	totalLines, err := textstream.ScanLines(ctx, stream.Reader, h.maxLineBytes(), func(line textstream.Line) error {
		if line.Number < startWanted || lineLimit > 0 && selectedCount >= lineLimit {
			return nil
		}
		if firstSelectedLine == 0 {
			firstSelectedLine = line.Number
		}
		lastSelectedLine = line.Number
		if rangeRequested && selectedCount > 0 {
			collector.append([]byte{'\n'})
		}
		if input.LineNumbers {
			collector.append([]byte(fmt.Sprintf("%d\t", line.Number)))
		}
		collector.append(line.Data)
		if !rangeRequested {
			collector.append(line.Ending)
		}
		selectedCount++
		return nil
	})
	if err != nil {
		return ReadTextFileOutput{}, err
	}
	if _, err := stream.Finish(); err != nil {
		return ReadTextFileOutput{}, err
	}
	if collector.byteLimitExceeded {
		limitName := input.outputLimitName
		if limitName == "" {
			limitName = "output limit"
		}
		return ReadTextFileOutput{}, operation.Wrap(
			operation.KindLimit,
			"read_text_file",
			path,
			fmt.Errorf("decoded content exceeds the %d-byte %s", input.maxOutputBytes, limitName),
		)
	}

	startLine, endLine := firstSelectedLine, lastSelectedLine
	if !rangeRequested {
		startLine = 1
		endLine = totalLines
	} else if selectedCount == 0 {
		startLine = totalLines + 1
		endLine = totalLines
	}

	content := collector.builder.String()
	if collector.truncated {
		notice := fmt.Sprintf("\n\n[TRUNCATED at %d characters. File has %d lines, %d bytes. Use offset/limit for specific ranges.]",
			maxCharacters, totalLines, stream.FileSizeBytes)
		if len(content)+len(notice) <= input.maxOutputBytes {
			content += notice
		}
	}
	output := ReadTextFileOutput{
		Content:       content,
		TotalLines:    totalLines,
		FileSizeBytes: stream.FileSizeBytes,
		StartLine:     startLine,
		EndLine:       endLine,
		Truncated:     collector.truncated,
		HasBOM:        stream.BOM.HasBOM,
		BOMType:       stream.BOM.Type,
	}
	if stream.AutoDetected {
		output.DetectedEncoding = stream.DetectedEncoding
		output.EncodingConfidence = stream.EncodingConfidence
	}
	return output, nil
}

// resolveWriteEncoding returns encoding for writes: explicit > trusted existing
// file encoding > configured default for new or empty files. A non-empty
// existing file with ambiguous encoding must fail closed rather than silently
// changing its byte representation through the configured default.
func (h *Handler) resolveWriteEncoding(inputEncoding string, filePath string) (string, error) {
	// 1. Explicit encoding always wins.
	if inputEncoding != "" {
		canonical, ok := encoding.CanonicalName(inputEncoding)
		if !ok {
			return "", fmt.Errorf("%w: %s. Use list_encodings to see available encodings", ErrEncodingUnsupported, strings.ToLower(strings.TrimSpace(inputEncoding)))
		}
		return canonical, nil
	}

	// 2. Preserve a non-empty existing file only when detection is trusted.
	info, statErr := os.Stat(filePath)
	switch {
	case statErr == nil && info.Size() > 0:
		detected, err := encoding.DetectFromFile(filePath, "sample")
		if err != nil {
			return "", fmt.Errorf("failed to detect existing file encoding: %w", err)
		}
		if detected.Charset == "" || detected.Confidence < encoding.MinConfidenceThreshold {
			return "", fmt.Errorf("%w (detected %q with confidence %d); specify encoding explicitly", ErrEncodingAmbiguous, detected.Charset, detected.Confidence)
		}
		if _, ok := encoding.Get(detected.Charset); !ok {
			return "", fmt.Errorf("%w: detected %s is not a registered read/write encoding", ErrEncodingUnsupported, detected.Charset)
		}
		slog.Debug("preserving existing file encoding", "path", filePath, "encoding", detected.Charset, "confidence", detected.Confidence)
		return detected.Charset, nil
	case statErr == nil:
		// Empty files carry no encoding evidence; use the configured creation default.
	case os.IsNotExist(statErr):
		// New file: use the configured creation default.
	case statErr != nil:
		return "", fmt.Errorf("failed to inspect existing file encoding: %w", statErr)
	}

	// 3. New or empty file: use the configured default, normalized through the
	// same registry as explicit request values.
	canonical, ok := encoding.CanonicalName(h.config.DefaultEncoding)
	if !ok {
		return "", fmt.Errorf("%w: configured default encoding %s is not registered", ErrEncodingUnsupported, h.config.DefaultEncoding)
	}
	return canonical, nil
}

// decodeContent decodes the file data to UTF-8 using the resolved encoding
func decodeContent(data []byte, encResult encodingResult) (string, error) {
	if encoding.IsUTF8(encResult.name) {
		return string(data), nil
	}

	decoder := encResult.encoder.NewDecoder()
	utf8Content, err := decoder.Bytes(data)
	if err != nil {
		return "", err
	}
	return string(utf8Content), nil
}
