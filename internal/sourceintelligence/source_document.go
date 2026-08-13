// Package sourceintelligence provides native, language-neutral source analysis primitives.
package sourceintelligence

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/textstream"
)

// Position is a 1-based decoded-source line/Unicode-scalar coordinate.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Range is a half-open decoded-source range.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// BOM describes a BOM consumed from the original byte stream.
type BOM struct {
	HasBOM bool   `json:"hasBOM"`
	Type   string `json:"type,omitempty"`
}

// OpenDocumentOptions bounds one SourceDocument load.
type OpenDocumentOptions struct {
	RequestedEncoding    string
	MaxFileBytes         int64
	MaxDecodedCharacters int
}

// SourceDocument owns one bounded decoded source snapshot and the internal line
// map needed to translate native parser UTF-8 byte offsets into public ranges.
type SourceDocument struct {
	Path               string
	Text               string
	Encoding           string
	DetectedEncoding   string
	EncodingConfidence int
	AutoDetected       bool
	BOM                BOM
	FileSizeBytes      int64
	SourceFingerprint  string

	lineStarts []int
}

// OpenSourceDocument decodes one regular file through the shared strict encoding
// stack and derives its content-v1 fingerprint from the same complete raw read.
func OpenSourceDocument(ctx context.Context, path string, options OpenDocumentOptions) (document *SourceDocument, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.MaxFileBytes <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "maximum source file bytes must be positive")
	}
	if options.MaxDecodedCharacters <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "maximum decoded source characters must be positive")
	}
	if err := ctx.Err(); err != nil {
		return nil, operation.Wrap(operation.KindCancelled, "open_source_document", path, err)
	}

	stream, err := textstream.OpenDecodedFile(ctx, path, textstream.OpenDecodedFileOptions{
		RequestedEncoding: options.RequestedEncoding,
		MaxFileBytes:      options.MaxFileBytes,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			err = errors.Join(err, operation.WrapFilesystem("close_source_document", path, closeErr))
		}
	}()

	maxUTF8Bytes, ok := decodedUTF8ByteLimit(options.MaxDecodedCharacters)
	if !ok {
		return nil, operation.New(operation.KindLimit, "decoded source character limit is too large")
	}
	data, err := io.ReadAll(io.LimitReader(stream.Reader, maxUTF8Bytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxUTF8Bytes {
		return nil, operation.Wrap(
			operation.KindLimit,
			"open_source_document",
			path,
			fmt.Errorf("decoded UTF-8 exceeds the %d-character limit", options.MaxDecodedCharacters),
		)
	}
	if !utf8.Valid(data) {
		return nil, operation.Wrap(operation.KindDecoding, "open_source_document", path, errors.New("decoder produced invalid UTF-8"))
	}
	if characters := utf8.RuneCount(data); characters > options.MaxDecodedCharacters {
		return nil, operation.Wrap(
			operation.KindLimit,
			"open_source_document",
			path,
			fmt.Errorf("decoded character count %d exceeds limit %d", characters, options.MaxDecodedCharacters),
		)
	}
	if err := ctx.Err(); err != nil {
		return nil, operation.Wrap(operation.KindCancelled, "open_source_document", path, err)
	}

	snapshot, err := stream.Finish()
	if err != nil {
		if errors.Is(err, filesystem.ErrConcurrentModification) || errors.Is(err, filesystem.ErrIncompleteRead) {
			return nil, operation.Wrap(operation.KindConflict, "open_source_document", path, err)
		}
		return nil, err
	}
	fingerprint, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
	if err != nil {
		return nil, err
	}

	text := string(data)
	document = &SourceDocument{
		Path:               path,
		Text:               text,
		Encoding:           stream.Encoding,
		DetectedEncoding:   stream.DetectedEncoding,
		EncodingConfidence: stream.EncodingConfidence,
		AutoDetected:       stream.AutoDetected,
		BOM:                BOM{HasBOM: stream.BOM.HasBOM, Type: stream.BOM.Type},
		FileSizeBytes:      stream.FileSizeBytes,
		SourceFingerprint:  fingerprint,
		lineStarts:         buildLineStarts(text),
	}
	return document, nil
}

func decodedUTF8ByteLimit(maxCharacters int) (int64, bool) {
	if maxCharacters <= 0 || int64(maxCharacters) > (int64(^uint64(0)>>1)-1)/utf8.UTFMax {
		return 0, false
	}
	return int64(maxCharacters) * utf8.UTFMax, true
}

func buildLineStarts(text string) []int {
	starts := make([]int, 1, 64)
	starts[0] = 0
	for index := 0; index < len(text); index++ {
		switch text[index] {
		case '\r':
			if index+1 < len(text) && text[index+1] == '\n' {
				index++
			}
			starts = append(starts, index+1)
		case '\n':
			starts = append(starts, index+1)
		}
	}
	return starts
}

// PositionAtUTF8Offset translates an internal decoded UTF-8 byte boundary into
// the frozen public 1-based Unicode-scalar coordinate system.
func (document *SourceDocument) PositionAtUTF8Offset(offset int) (Position, error) {
	if document == nil {
		return Position{}, operation.New(operation.KindInvalidInput, "source document is nil")
	}
	if offset < 0 || offset > len(document.Text) {
		return Position{}, operation.New(operation.KindInvalidInput, "UTF-8 offset is outside the source document")
	}
	if offset < len(document.Text) && !utf8.RuneStart(document.Text[offset]) {
		return Position{}, operation.New(operation.KindInvalidInput, "UTF-8 offset is not on a Unicode scalar boundary")
	}

	lineIndex := sort.Search(len(document.lineStarts), func(index int) bool {
		return document.lineStarts[index] > offset
	}) - 1
	if lineIndex < 0 {
		lineIndex = 0
	}
	lineStart := document.lineStarts[lineIndex]
	return Position{
		Line:   lineIndex + 1,
		Column: utf8.RuneCountInString(document.Text[lineStart:offset]) + 1,
	}, nil
}

// RangeFromUTF8Offsets translates an internal half-open UTF-8 byte range into
// the frozen public half-open decoded-source range.
func (document *SourceDocument) RangeFromUTF8Offsets(start, end int) (Range, error) {
	if start > end {
		return Range{}, operation.New(operation.KindInvalidInput, "source range start exceeds end")
	}
	startPosition, err := document.PositionAtUTF8Offset(start)
	if err != nil {
		return Range{}, err
	}
	endPosition, err := document.PositionAtUTF8Offset(end)
	if err != nil {
		return Range{}, err
	}
	return Range{Start: startPosition, End: endPosition}, nil
}

// SliceUTF8Offsets returns one exact decoded-source slice only when both UTF-8
// boundaries are valid and the requested bytes fit the caller's explicit cap.
func (document *SourceDocument) SliceUTF8Offsets(start, end, maxBytes int) (string, Range, error) {
	if maxBytes <= 0 {
		return "", Range{}, operation.New(operation.KindInvalidInput, "maximum source slice bytes must be positive")
	}
	rangeValue, err := document.RangeFromUTF8Offsets(start, end)
	if err != nil {
		return "", Range{}, err
	}
	if end-start > maxBytes {
		return "", Range{}, operation.Wrap(
			operation.KindLimit,
			"slice_source_document",
			document.Path,
			fmt.Errorf("source slice size %d exceeds limit %d", end-start, maxBytes),
		)
	}
	return document.Text[start:end], rangeValue, nil
}
