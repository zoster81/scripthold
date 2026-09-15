package handler

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/operation"
)

const maxUnsupportedCharacters = 64

type encodingOutputReader struct {
	reader io.Reader
	target string
}

func (reader *encodingOutputReader) Read(buffer []byte) (int, error) {
	read, err := reader.reader.Read(buffer)
	if err != nil && err != io.EOF && operation.KindOf(err) != operation.KindCancelled {
		err = operation.Wrap(
			operation.KindEncodingOutput,
			"encode_stream",
			"",
			fmt.Errorf("%w: failed to encode content to %s: %v", ErrEncodingEncode, reader.target, err),
		)
	}
	return read, err
}

func (h *Handler) validateConversionBatchPaths(paths []string, backup bool) error {
	targets := make(map[string]string, len(paths))
	validatedPaths := make([]string, len(paths))
	for index, requested := range paths {
		validated := h.ValidatePath(requested)
		if !validated.Ok() {
			return fmt.Errorf("%s: %v", requested, validated.Err)
		}
		validatedPaths[index] = validated.Path
		key := conversionPathKey(validated.Path)
		if previous, exists := targets[key]; exists {
			return fmt.Errorf("%s resolves to the same file as %s", requested, previous)
		}
		targets[key] = requested
	}
	if !backup {
		return nil
	}
	for index, target := range validatedPaths {
		backupPath := target + ".bak"
		validatedBackup := h.ValidatePath(backupPath)
		if !validatedBackup.Ok() {
			return fmt.Errorf("backup for %s: %v", paths[index], validatedBackup.Err)
		}
		if conflicting, exists := targets[conversionPathKey(validatedBackup.Path)]; exists {
			return fmt.Errorf("backup path for %s collides with requested target %s", paths[index], conflicting)
		}
	}
	return nil
}

func conversionPathKey(path string) string {
	key := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}

func formatUnsupportedError(result ConvertFileResult) string {
	if len(result.Unsupported) == 0 {
		return fmt.Sprintf("%s contains %d characters unsupported by the target encoding", result.Path, result.UnsupportedCount)
	}
	first := result.Unsupported[0]
	return fmt.Sprintf("%s contains %d characters unsupported by the target encoding; first is %s at line %d, column %d", result.Path, result.UnsupportedCount, first.Code, first.Line, first.Column)
}

func inspectUnsupportedCharacters(ctx context.Context, reader io.Reader, targetEncoding string) ([]UnsupportedCharacter, int, error) {
	if fileEncoding.IsUTF8(targetEncoding) {
		_, err := io.Copy(io.Discard, reader)
		return nil, 0, err
	}
	registered, ok := fileEncoding.Get(targetEncoding)
	if !ok || registered == nil {
		return nil, 0, fmt.Errorf("unsupported target encoding: %s", targetEncoding)
	}

	buffered := bufio.NewReader(reader)
	cache := make(map[rune]bool, 256)
	unsupported := make([]UnsupportedCharacter, 0)
	unsupportedCount := 0
	line, column := 1, 1
	for {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		r, _, err := buffered.ReadRune()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		representable, cached := cache[r]
		if !cached {
			_, encodeErr := registered.NewEncoder().String(string(r))
			representable = encodeErr == nil
			if len(cache) < 4096 {
				cache[r] = representable
			}
		}
		if !representable {
			unsupportedCount++
			if len(unsupported) < maxUnsupportedCharacters {
				unsupported = append(unsupported, UnsupportedCharacter{
					Rune: string(r), Code: fmt.Sprintf("U+%04X", r), Line: line, Column: column,
				})
			}
		}
		if r == '\n' {
			line, column = line+1, 1
		} else {
			column++
		}
	}
	return unsupported, unsupportedCount, nil
}
