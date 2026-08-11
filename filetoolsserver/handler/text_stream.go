package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/textstream"
)

type decodingErrorReader struct {
	source io.Reader
	path   string
}

func (reader *decodingErrorReader) Read(buffer []byte) (int, error) {
	read, err := reader.source.Read(buffer)
	if err != nil && err != io.EOF && operation.KindOf(err) != operation.KindCancelled {
		err = operation.Wrap(operation.KindDecoding, "decode_text_stream", reader.path, err)
	}
	return read, err
}

type decodedTextStream struct {
	Reader             io.Reader
	Charset            string
	DetectedEncoding   string
	EncodingConfidence int
	AutoDetected       bool
	BOM                bomInfo
	FileSizeBytes      int64
	Mode               os.FileMode

	session *filesystem.ReadSession
}

func (h *Handler) openDecodedTextStream(ctx context.Context, path, requestedEncoding string) (stream *decodedTextStream, err error) {
	defer func() {
		err = operation.WrapFilesystem("open_decoded_text_stream", path, err)
	}()

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

	resolved, err := h.resolveStreamEncoding(requestedEncoding, session)
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

	var bom bomInfo
	if detected, found := fileEncoding.DetectBOM(head); found {
		if canonicalBOMEncoding(resolved.name) != detected.Charset {
			return nil, fmt.Errorf("%w: file BOM indicates %s but selected encoding is %s", ErrBOMEncodingConflict, detected.Charset, resolved.name)
		}
		size := fileEncoding.BOMSize(detected.Charset)
		bom = bomInfo{HasBOM: true, Type: detected.Charset, Bytes: append([]byte(nil), head[:size]...)}
	}

	if err := session.Start(int64(len(bom.Bytes))); err != nil {
		return nil, err
	}
	decoded, err := fileEncoding.NewDecoderReader(textstream.WithContext(ctx, session), resolved.name)
	if err != nil {
		return nil, operation.Wrap(operation.KindDecoding, "open_decoded_text_stream", path, err)
	}

	stream = &decodedTextStream{
		Reader:             &decodingErrorReader{source: decoded, path: path},
		Charset:            resolved.name,
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

func (h *Handler) resolveStreamEncoding(requestedEncoding string, session *filesystem.ReadSession) (encodingResult, error) {
	result := encodingResult{}
	if requestedEncoding != "" {
		canonical, ok := fileEncoding.CanonicalName(requestedEncoding)
		if !ok {
			return result, fmt.Errorf("%w: %s. Use list_encodings to see available encodings", ErrEncodingUnsupported, requestedEncoding)
		}
		result.name = canonical
		result.encoder, _ = fileEncoding.Get(canonical)
		return result, nil
	}

	result.autoDetected = true
	if session.Size() == 0 {
		result.name = "utf-8"
		result.detectedEncoding = "utf-8"
		registered, _ := fileEncoding.Get(result.name)
		result.encoder = registered
		return result, nil
	}

	detection, err := fileEncoding.DetectFromReaderAt(session, session.Size(), "sample")
	if err != nil {
		return result, err
	}
	result.detectedEncoding = detection.Charset
	result.encodingConfidence = detection.Confidence
	if detection.Charset == "" || detection.Confidence < fileEncoding.MinConfidenceThreshold {
		return result, fmt.Errorf("%w (detected %q with confidence %d)", ErrEncodingAmbiguous, detection.Charset, detection.Confidence)
	}
	result.name = detection.Charset

	registered, ok := fileEncoding.Get(result.name)
	if !ok {
		return result, fmt.Errorf("%w: detected %s is not a registered read/write encoding", ErrEncodingUnsupported, result.name)
	}
	result.encoder = registered

	slog.Debug("resolved encoding from stream",
		"encoding", result.name,
		"detected", result.detectedEncoding,
		"confidence", result.encodingConfidence,
	)
	return result, nil
}

func (stream *decodedTextStream) Finish() (filesystem.FileSnapshot, error) {
	return stream.session.Finish()
}

func (stream *decodedTextStream) Close() error {
	if stream == nil || stream.session == nil {
		return nil
	}
	return stream.session.Close()
}
