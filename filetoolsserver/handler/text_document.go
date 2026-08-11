package handler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

type bomInfo struct {
	HasBOM bool
	Type   string
	Bytes  []byte
}

type textDocument struct {
	Text               string
	Charset            string
	DetectedEncoding   string
	EncodingConfidence int
	AutoDetected       bool
	BOM                bomInfo
	LineEndings        LineEndingInfo
	FileSizeBytes      int64
	Mode               os.FileMode
	Snapshot           filesystem.FileSnapshot
}

type bomPolicy string

const (
	bomPreserve bomPolicy = "preserve"
	bomAlways   bomPolicy = "always"
	bomNever    bomPolicy = "never"
	bomAuto     bomPolicy = "auto"
)

func parseBOMPolicy(value string, defaultPolicy bomPolicy) (bomPolicy, error) {
	if strings.TrimSpace(value) == "" {
		return defaultPolicy, nil
	}

	policy := bomPolicy(strings.ToLower(strings.TrimSpace(value)))
	switch policy {
	case bomPreserve, bomAlways, bomNever, bomAuto:
		return policy, nil
	default:
		return "", operation.Wrap(
			operation.KindInvalidInput,
			"parse_bom_policy",
			"",
			fmt.Errorf("invalid BOM policy %q: valid values are auto, always, never, preserve", value),
		)
	}
}

func canonicalBOMEncoding(name string) string {
	if canonical, ok := fileEncoding.CanonicalBOMName(name); ok {
		return canonical
	}
	if canonical, ok := fileEncoding.CanonicalName(name); ok {
		return canonical
	}
	return strings.ToLower(strings.TrimSpace(name))
}

func splitTransportBOM(data []byte, encodingName string) ([]byte, bomInfo, error) {
	result, found := fileEncoding.DetectBOM(data)
	if !found {
		return data, bomInfo{}, nil
	}
	if canonicalBOMEncoding(encodingName) != result.Charset {
		return nil, bomInfo{}, fmt.Errorf("%w: file BOM indicates %s but selected encoding is %s", ErrBOMEncodingConflict, result.Charset, encodingName)
	}

	bomSize := fileEncoding.BOMSize(result.Charset)
	bomBytes := append([]byte(nil), data[:bomSize]...)
	return data[bomSize:], bomInfo{
		HasBOM: true,
		Type:   result.Charset,
		Bytes:  bomBytes,
	}, nil
}

func (h *Handler) resolveEncodingFromDataDetailed(inputEncoding string, data []byte, filePath string) (encodingResult, error) {
	result := encodingResult{}

	if inputEncoding != "" {
		canonical, ok := fileEncoding.CanonicalName(inputEncoding)
		if !ok {
			return result, fmt.Errorf("%w: %s. Use list_encodings to see available encodings", ErrEncodingUnsupported, strings.ToLower(strings.TrimSpace(inputEncoding)))
		}
		result.name = canonical
		result.encoder, _ = fileEncoding.Get(canonical)
		return result, nil
	}

	result.autoDetected = true
	if len(data) == 0 {
		result.name = "utf-8"
		result.detectedEncoding = "utf-8"
		enc, _ := fileEncoding.Get(result.name)
		result.encoder = enc
		return result, nil
	}

	detection := fileEncoding.Detect(data)
	result.detectedEncoding = detection.Charset
	result.encodingConfidence = detection.Confidence
	if detection.Charset == "" || detection.Confidence < fileEncoding.MinConfidenceThreshold {
		return result, fmt.Errorf("%w (detected %q with confidence %d)", ErrEncodingAmbiguous, detection.Charset, detection.Confidence)
	}
	result.name = detection.Charset

	enc, ok := fileEncoding.Get(result.name)
	if !ok {
		return result, fmt.Errorf("%w: detected %s is not a registered read/write encoding", ErrEncodingUnsupported, result.name)
	}
	result.encoder = enc

	slog.Debug("resolved encoding from loaded data",
		"path", filePath,
		"encoding", result.name,
		"detected", result.detectedEncoding,
		"confidence", result.encodingConfidence,
	)
	return result, nil
}

func encodeTextDocument(document textDocument, content string, policy bomPolicy) (result []byte, err error) {
	defer func() {
		if err != nil {
			err = operation.Wrap(operation.KindEncodingOutput, "encode_text_document", "", err)
		}
	}()

	var encoded []byte
	if fileEncoding.IsUTF8(document.Charset) {
		encoded = []byte(content)
	} else {
		enc, ok := fileEncoding.Get(document.Charset)
		if !ok {
			return nil, fmt.Errorf("%w: unsupported encoding %s", ErrEncodingEncode, document.Charset)
		}
		var err error
		encoded, err = enc.NewEncoder().Bytes([]byte(content))
		if err != nil {
			return nil, fmt.Errorf("%w: failed to encode content to %s: %v", ErrEncodingEncode, document.Charset, err)
		}
	}

	bom, err := documentBOMBytes(document, policy)
	if err != nil {
		return nil, err
	}
	if len(bom) == 0 {
		return encoded, nil
	}

	result = make([]byte, 0, len(bom)+len(encoded))
	result = append(result, bom...)
	result = append(result, encoded...)
	return result, nil
}

func restoreDocumentLineEndings(content, style string) string {
	switch style {
	case LineEndingCRLF:
		return ConvertLineEndings(content, LineEndingCRLF)
	case LineEndingLF, LineEndingMixed, LineEndingNone:
		// Mixed files historically normalize to LF during edit_file writes.
		return ConvertLineEndings(content, LineEndingLF)
	default:
		return ConvertLineEndings(content, LineEndingLF)
	}
}

func documentBOMBytes(document textDocument, policy bomPolicy) ([]byte, error) {
	charset := canonicalBOMEncoding(document.Charset)

	switch policy {
	case bomNever:
		return nil, nil
	case bomAuto:
		descriptor, ok := fileEncoding.LookupDescriptor(charset)
		if ok && descriptor.AutoBOM {
			return requiredBOMBytes(charset, policy)
		}
		return nil, nil
	case bomAlways:
		return requiredBOMBytes(charset, policy)
	case bomPreserve:
		if !document.BOM.HasBOM {
			return nil, nil
		}
		return requiredBOMBytes(charset, policy)
	default:
		return nil, fmt.Errorf("unsupported BOM policy: %s", policy)
	}
}

func requiredBOMBytes(charset string, policy bomPolicy) ([]byte, error) {
	bom := fileEncoding.BOMBytesFor(charset)
	if len(bom) == 0 {
		return nil, fmt.Errorf("%w: BOM policy %s is not supported for encoding %s", ErrEncodingEncode, policy, charset)
	}
	return bom, nil
}

func outputBOMMetadata(data []byte) (bool, string) {
	result, found := fileEncoding.DetectBOM(data)
	if !found {
		return false, ""
	}
	return true, result.Charset
}

func (h *Handler) readTextDocument(ctx context.Context, path, requestedEncoding string) (textDocument, error) {
	document, _, err := h.readTextDocumentWithData(ctx, path, requestedEncoding)
	return document, err
}

func (h *Handler) readTextDocumentWithData(ctx context.Context, path, requestedEncoding string) (document textDocument, data []byte, err error) {
	defer func() {
		err = operation.WrapFilesystem("read_text_document", path, err)
	}()

	select {
	case <-ctx.Done():
		return textDocument{}, nil, ctx.Err()
	default:
	}

	info, err := os.Stat(path)
	if err != nil {
		return textDocument{}, nil, fmt.Errorf("failed to stat file: %w", err)
	}
	budget := h.maxFileBytes()
	if info.Size() > budget {
		return textDocument{}, nil, operation.Wrap(
			operation.KindLimit,
			"read_text_document",
			path,
			fmt.Errorf("file size %d exceeds the %d-byte edit limit", info.Size(), budget),
		)
	}

	data, err = os.ReadFile(path)
	if err != nil {
		return textDocument{}, nil, fmt.Errorf("failed to read file: %w", err)
	}
	snapshot, err := filesystem.CaptureSnapshotWithData(path, data)
	if err != nil {
		return textDocument{}, nil, fmt.Errorf("failed to snapshot file: %w", err)
	}
	if snapshot.Size != info.Size() || !snapshot.ModTime.Equal(info.ModTime()) || snapshot.Mode != info.Mode() {
		return textDocument{}, nil, fmt.Errorf("%w: file changed while being read", filesystem.ErrConcurrentModification)
	}

	select {
	case <-ctx.Done():
		return textDocument{}, nil, ctx.Err()
	default:
	}

	encResult, err := h.resolveEncodingFromDataDetailed(requestedEncoding, data, path)
	if err != nil {
		return textDocument{}, nil, err
	}

	payload, bom, err := splitTransportBOM(data, encResult.name)
	if err != nil {
		return textDocument{}, nil, err
	}

	content, err := decodeContent(payload, encResult)
	if err != nil {
		return textDocument{}, nil, fmt.Errorf("%w: failed to decode file content with %s: %v", ErrEncodingDecode, encResult.name, err)
	}

	return textDocument{
		Text:               content,
		Charset:            encResult.name,
		DetectedEncoding:   encResult.detectedEncoding,
		EncodingConfidence: encResult.encodingConfidence,
		AutoDetected:       encResult.autoDetected,
		BOM:                bom,
		LineEndings:        DetectLineEndings([]byte(content)),
		FileSizeBytes:      info.Size(),
		Mode:               info.Mode().Perm(),
		Snapshot:           snapshot,
	}, data, nil
}
