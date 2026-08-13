package textstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	fileencoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

var (
	// ErrEncodingUnsupported reports an explicit or detected charset that is not
	// registered for strict decoding.
	ErrEncodingUnsupported = operation.New(operation.KindEncoding, "unsupported encoding")
	// ErrEncodingAmbiguous reports insufficient evidence for safe automatic decoding.
	ErrEncodingAmbiguous = operation.New(operation.KindEncoding, "encoding is ambiguous; specify encoding explicitly")
	// ErrBOMEncodingConflict reports disagreement between the selected charset and BOM.
	ErrBOMEncodingConflict = operation.New(operation.KindEncoding, "BOM encoding conflict")
)

// BOMInfo describes a transport BOM removed before decoded text is exposed.
type BOMInfo struct {
	HasBOM bool
	Type   string
	Bytes  []byte
}

// OpenDecodedFileOptions controls generic strict decoded-file opening. A zero
// MaxFileBytes preserves existing callers that enforce their own file limit.
type OpenDecodedFileOptions struct {
	RequestedEncoding string
	MaxFileBytes      int64
}

// DecodedFile streams one regular file as strict decoded UTF-8 while the
// underlying ReadSession hashes the complete original byte stream.
type DecodedFile struct {
	Reader             io.Reader
	Encoding           string
	DetectedEncoding   string
	EncodingConfidence int
	AutoDetected       bool
	BOM                BOMInfo
	FileSizeBytes      int64
	Mode               os.FileMode

	session *filesystem.ReadSession
}

type resolvedFileEncoding struct {
	name               string
	detectedEncoding   string
	encodingConfidence int
	autoDetected       bool
}

type decodingErrorReader struct {
	ctx    context.Context
	source io.Reader
	path   string
}

func (reader *decodingErrorReader) Read(buffer []byte) (int, error) {
	read, err := reader.source.Read(buffer)
	if err == nil || err == io.EOF {
		return read, err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || reader.ctx.Err() != nil {
		return read, operation.Wrap(operation.KindCancelled, "decode_text_stream", reader.path, err)
	}
	if operation.KindOf(err) == operation.KindCancelled {
		return read, err
	}
	return read, operation.Wrap(operation.KindDecoding, "decode_text_stream", reader.path, err)
}

// OpenDecodedFile opens a regular file, resolves/validates its encoding without
// using its filename, removes a matching BOM, and exposes strict decoded UTF-8.
func OpenDecodedFile(ctx context.Context, path string, options OpenDecodedFileOptions) (stream *DecodedFile, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		err = operation.WrapFilesystem("open_decoded_file", path, err)
	}()
	if err := ctx.Err(); err != nil {
		return nil, operation.Wrap(operation.KindCancelled, "open_decoded_file", path, err)
	}

	session, err := filesystem.OpenReadSession(path)
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			err = errors.Join(err, session.Close())
		}
	}()

	if options.MaxFileBytes > 0 && session.Size() > options.MaxFileBytes {
		return nil, operation.Wrap(
			operation.KindLimit,
			"open_decoded_file",
			path,
			fmt.Errorf("file size %d exceeds limit %d", session.Size(), options.MaxFileBytes),
		)
	}

	resolved, err := resolveFileEncoding(options.RequestedEncoding, session)
	if err != nil {
		return nil, err
	}

	headLength := min(int64(4), session.Size())
	head := make([]byte, int(headLength))
	if headLength > 0 {
		read, readErr := session.ReadAt(head, 0)
		if readErr != nil && readErr != io.EOF {
			return nil, fmt.Errorf("failed to read file prefix: %w", readErr)
		}
		head = head[:read]
	}

	var bom BOMInfo
	if detected, found := fileencoding.DetectBOM(head); found {
		if canonicalBOMEncoding(resolved.name) != detected.Charset {
			return nil, fmt.Errorf("%w: file BOM indicates %s but selected encoding is %s", ErrBOMEncodingConflict, detected.Charset, resolved.name)
		}
		size := fileencoding.BOMSize(detected.Charset)
		bom = BOMInfo{HasBOM: true, Type: detected.Charset, Bytes: append([]byte(nil), head[:size]...)}
	}

	if err := session.Start(int64(len(bom.Bytes))); err != nil {
		return nil, err
	}
	decoded, err := fileencoding.NewDecoderReader(WithContext(ctx, session), resolved.name)
	if err != nil {
		return nil, operation.Wrap(operation.KindDecoding, "open_decoded_file", path, err)
	}

	stream = &DecodedFile{
		Reader:             &decodingErrorReader{ctx: ctx, source: decoded, path: path},
		Encoding:           resolved.name,
		DetectedEncoding:   resolved.detectedEncoding,
		EncodingConfidence: resolved.encodingConfidence,
		AutoDetected:       resolved.autoDetected,
		BOM:                bom,
		FileSizeBytes:      session.Size(),
		Mode:               session.Mode(),
		session:            session,
	}
	cleanup = false
	return stream, nil
}

func resolveFileEncoding(requestedEncoding string, session *filesystem.ReadSession) (resolvedFileEncoding, error) {
	result := resolvedFileEncoding{}
	if requestedEncoding != "" {
		canonical, ok := fileencoding.CanonicalName(requestedEncoding)
		if !ok {
			return result, fmt.Errorf("%w: %s. Use list_encodings to see available encodings", ErrEncodingUnsupported, requestedEncoding)
		}
		result.name = canonical
		return result, nil
	}

	result.autoDetected = true
	if session.Size() == 0 {
		result.name = "utf-8"
		result.detectedEncoding = "utf-8"
		return result, nil
	}

	detection, err := fileencoding.DetectFromReaderAt(session, session.Size(), "sample")
	if err != nil {
		return result, err
	}
	result.detectedEncoding = detection.Charset
	result.encodingConfidence = detection.Confidence
	if detection.Charset == "" || detection.Confidence < fileencoding.MinConfidenceThreshold {
		return result, fmt.Errorf("%w (detected %q with confidence %d)", ErrEncodingAmbiguous, detection.Charset, detection.Confidence)
	}
	result.name = detection.Charset
	if _, ok := fileencoding.Get(result.name); !ok {
		return result, fmt.Errorf("%w: detected %s is not a registered read/write encoding", ErrEncodingUnsupported, result.name)
	}
	return result, nil
}

func canonicalBOMEncoding(name string) string {
	if canonical, ok := fileencoding.CanonicalBOMName(name); ok {
		return canonical
	}
	if canonical, ok := fileencoding.CanonicalName(name); ok {
		return canonical
	}
	return name
}

// RawReader exposes the original post-BOM byte stream for byte-preserving
// consumers. Reads still pass through the same digesting ReadSession.
func (stream *DecodedFile) RawReader() io.Reader {
	if stream == nil {
		return nil
	}
	return stream.session
}

// Finish verifies that the full original byte stream was consumed unchanged and
// returns the digest-bearing snapshot used by the shared content-v1 fingerprint.
func (stream *DecodedFile) Finish() (filesystem.FileSnapshot, error) {
	if stream == nil || stream.session == nil {
		return filesystem.FileSnapshot{}, operation.New(operation.KindInvalidInput, "decoded file stream is not open")
	}
	return stream.session.Finish()
}

// Close releases the underlying regular-file handle.
func (stream *DecodedFile) Close() error {
	if stream == nil || stream.session == nil {
		return nil
	}
	return stream.session.Close()
}
