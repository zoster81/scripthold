package handler

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
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

func (h *Handler) HandleConvertEncoding(ctx context.Context, req *mcp.CallToolRequest, input ConvertEncodingInput) (*mcp.CallToolResult, ConvertEncodingOutput, error) {
	if strings.TrimSpace(input.To) == "" {
		return errorResult("target encoding (to) is required"), ConvertEncodingOutput{}, nil
	}
	targetEncoding, ok := fileEncoding.CanonicalName(input.To)
	if !ok {
		return errorResult(fmt.Sprintf("unsupported target encoding: %s. Use list_encodings to see available encodings.", input.To)), ConvertEncodingOutput{}, nil
	}
	policy, err := parseBOMPolicy(input.BOM, bomAuto)
	if err != nil {
		return errorResultFromError(err), ConvertEncodingOutput{}, nil
	}

	if input.Path != "" && len(input.Paths) > 0 {
		return errorResult("path and paths are mutually exclusive"), ConvertEncodingOutput{}, nil
	}
	if input.Path == "" && len(input.Paths) == 0 {
		return errorResult("path or paths is required"), ConvertEncodingOutput{}, nil
	}
	if len(input.Paths) > h.maxBatchFiles() {
		return errorResultWithCode(ErrCodeLimit, fmt.Sprintf("paths contains %d files; configured limit is %d", len(input.Paths), h.maxBatchFiles())), ConvertEncodingOutput{}, nil
	}
	if len(input.Paths) > 0 {
		if err := h.validateConversionBatchPaths(input.Paths, input.Backup && !input.DryRun); err != nil {
			return errorResult("invalid conversion batch: " + err.Error()), ConvertEncodingOutput{}, nil
		}
	}

	if input.Path != "" {
		result, convertErr := h.convertEncodingPath(ctx, input.Path, input, targetEncoding, policy)
		if convertErr != nil {
			return errorResultFromError(convertErr), ConvertEncodingOutput{}, nil
		}
		if result.UnsupportedCount > 0 {
			return errorResultWithCode(ErrCodeEncoding, formatUnsupportedError(result)), ConvertEncodingOutput{}, nil
		}
		return &mcp.CallToolResult{}, flattenConvertResult(result, targetEncoding, input.DryRun), nil
	}

	output := ConvertEncodingOutput{
		Message:        fmt.Sprintf("Processed %d files for conversion to %s", len(input.Paths), targetEncoding),
		TargetEncoding: targetEncoding,
		DryRun:         input.DryRun,
		Results:        make([]ConvertFileResult, 0, len(input.Paths)),
	}
	errorSummary := newBoundedErrorSummary(h.maxOutputBytes())
	for _, path := range input.Paths {
		result, convertErr := h.convertEncodingPath(ctx, path, input, targetEncoding, policy)
		if convertErr != nil {
			mapped := mapOperationError(convertErr, path)
			result = ConvertFileResult{
				Path:              path,
				Error:             mapped.Message,
				ErrorCode:         mapped.BatchCode,
				EncodingErrorCode: encodingErrorCode(convertErr),
			}
		}
		if result.UnsupportedCount > 0 && result.Error == "" {
			result.Error = formatUnsupportedError(result)
			result.ErrorCode = ErrCodeEncoding
			result.EncodingErrorCode = EncodingErrorUnrepresentable
		}
		if result.Error != "" {
			output.ErrorCount++
			errorSummary.Add(fmt.Sprintf("%s: %s", path, result.Error))
		} else {
			output.SuccessCount++
		}
		output.Results = append(output.Results, result)
	}
	output.Errors = errorSummary.Items()
	output.ErrorsOmitted = errorSummary.Omitted()
	output.ErrorsTruncated = output.ErrorsOmitted > 0
	return &mcp.CallToolResult{}, output, nil
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

func flattenConvertResult(result ConvertFileResult, target string, dryRun bool) ConvertEncodingOutput {
	return ConvertEncodingOutput{
		Message:        result.Message,
		SourceEncoding: result.SourceEncoding,
		TargetEncoding: target,
		BackupPath:     result.BackupPath,
		BOMPolicy:      result.BOMPolicy,
		HasBOM:         result.HasBOM,
		BOMType:        result.BOMType,
		Changed:        result.Changed,
		DryRun:         dryRun,
	}
}

func formatUnsupportedError(result ConvertFileResult) string {
	if len(result.Unsupported) == 0 {
		return fmt.Sprintf("%s contains %d characters unsupported by the target encoding", result.Path, result.UnsupportedCount)
	}
	first := result.Unsupported[0]
	return fmt.Sprintf("%s contains %d characters unsupported by the target encoding; first is %s at line %d, column %d", result.Path, result.UnsupportedCount, first.Code, first.Line, first.Column)
}

func (h *Handler) convertEncodingPath(ctx context.Context, requestedPath string, input ConvertEncodingInput, targetEncoding string, policy bomPolicy) (ConvertFileResult, error) {
	result := ConvertFileResult{Path: requestedPath, BOMPolicy: string(policy)}
	validated := h.ValidatePath(requestedPath)
	if !validated.Ok() {
		return result, validated.Err
	}

	preview, err := h.openDecodedTextStream(ctx, validated.Path, input.From)
	if err != nil {
		return result, err
	}
	result.SourceEncoding = preview.Charset
	targetBOM, err := documentBOMBytes(textDocument{Charset: targetEncoding, BOM: preview.BOM}, policy)
	if err != nil {
		_ = preview.Close()
		return result, err
	}
	unsupported, unsupportedCount, err := inspectUnsupportedCharacters(ctx, preview.Reader, targetEncoding)
	if err != nil {
		_ = preview.Close()
		return result, err
	}
	previewSnapshot, err := preview.Finish()
	if err != nil {
		_ = preview.Close()
		return result, err
	}
	if err := preview.Close(); err != nil {
		return result, err
	}
	result.Unsupported = unsupported
	result.UnsupportedCount = unsupportedCount
	result.HasBOM = len(targetBOM) > 0
	if result.HasBOM {
		result.BOMType = canonicalBOMEncoding(targetEncoding)
	}
	sourceCanonical, _ := fileEncoding.CanonicalName(result.SourceEncoding)
	result.Changed = sourceCanonical != targetEncoding || !bytes.Equal(preview.BOM.Bytes, targetBOM)
	if unsupportedCount > 0 {
		return result, nil
	}
	if input.DryRun {
		changed, compareErr := h.previewEncodingChange(ctx, validated.Path, result.SourceEncoding, targetEncoding, targetBOM, previewSnapshot)
		if compareErr != nil {
			return result, compareErr
		}
		result.Changed = changed
		if changed {
			result.Message = fmt.Sprintf("Dry run: %s would change when converted from %s to %s", requestedPath, result.SourceEncoding, targetEncoding)
		} else {
			result.Message = fmt.Sprintf("Dry run: %s is byte-identical under the requested encoding and BOM state", requestedPath)
		}
		return result, nil
	}

	stream, err := h.openDecodedTextStream(ctx, validated.Path, result.SourceEncoding)
	if err != nil {
		return result, err
	}
	defer stream.Close()
	encoded, err := fileEncoding.NewEncoderReader(stream.Reader, targetEncoding)
	if err != nil {
		return result, err
	}
	outputReader := io.MultiReader(bytes.NewReader(targetBOM), &encodingOutputReader{reader: encoded, target: targetEncoding})
	staged, err := filesystem.StageReplacement(validated.Path, outputReader, stream.Mode.Perm(), nil)
	if err != nil {
		return result, err
	}
	defer staged.Cleanup()

	snapshot, err := stream.Finish()
	if err != nil {
		return result, err
	}
	if err := stream.Close(); err != nil {
		return result, err
	}

	backupPath := ""
	if input.Backup {
		backup := h.ValidatePath(validated.Path + ".bak")
		if !backup.Ok() {
			return result, backup.Err
		}
		backupPath = backup.Path
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	commit := h.ValidatePath(requestedPath)
	if !commit.Ok() {
		return result, commit.Err
	}
	if commit.Path != validated.Path {
		return result, operation.Wrap(operation.KindConflict, "convert_encoding", requestedPath, fmt.Errorf("path changed while preparing encoding conversion"))
	}
	changed, err := staged.Commit(filesystem.ReplaceOptions{
		Expected:      &snapshot,
		BackupPath:    backupPath,
		SkipIdentical: true,
	})
	if err != nil {
		return result, err
	}
	result.Changed = changed
	if changed {
		result.BackupPath = backupPath
		result.Message = fmt.Sprintf("Successfully converted %s from %s to %s (BOM: %s)", requestedPath, result.SourceEncoding, targetEncoding, policy)
		if backupPath != "" {
			result.Message += fmt.Sprintf(" (backup: %s)", backupPath)
		}
	} else {
		result.Message = fmt.Sprintf("No conversion needed for %s: target bytes are unchanged", requestedPath)
	}
	return result, nil
}

func (h *Handler) previewEncodingChange(ctx context.Context, path, sourceEncoding, targetEncoding string, targetBOM []byte, expected filesystem.FileSnapshot) (bool, error) {
	stream, err := h.openDecodedTextStream(ctx, path, sourceEncoding)
	if err != nil {
		return false, err
	}
	defer stream.Close()

	encoded, err := fileEncoding.NewEncoderReader(stream.Reader, targetEncoding)
	if err != nil {
		return false, err
	}
	hasher := sha256.New()
	if _, err := hasher.Write(targetBOM); err != nil {
		return false, err
	}
	written, err := io.Copy(hasher, encoded)
	if err != nil {
		if operation.KindOf(err) == operation.KindCancelled {
			return false, err
		}
		return false, operation.Wrap(operation.KindEncodingOutput, "preview_encoding", path, err)
	}
	current, err := stream.Finish()
	if err != nil {
		return false, err
	}
	if !expected.Equal(current) {
		return false, operation.Wrap(operation.KindConflict, "preview_encoding", path, fmt.Errorf("file changed during conversion preview"))
	}
	targetSize := int64(len(targetBOM)) + written
	return !expected.MatchesContentDigest(targetSize, hasher.Sum(nil)), nil
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
