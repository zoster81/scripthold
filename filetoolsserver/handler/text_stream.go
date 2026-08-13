package handler

import (
	"context"
	"io"
	"os"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/textstream"
)

type decodedTextStream struct {
	Reader             io.Reader
	Charset            string
	DetectedEncoding   string
	EncodingConfidence int
	AutoDetected       bool
	BOM                bomInfo
	FileSizeBytes      int64
	Mode               os.FileMode

	stream *textstream.DecodedFile
}

func (h *Handler) openDecodedTextStream(ctx context.Context, path, requestedEncoding string) (*decodedTextStream, error) {
	shared, err := textstream.OpenDecodedFile(ctx, path, textstream.OpenDecodedFileOptions{
		RequestedEncoding: requestedEncoding,
	})
	if err != nil {
		return nil, err
	}
	return &decodedTextStream{
		Reader:             shared.Reader,
		Charset:            shared.Encoding,
		DetectedEncoding:   shared.DetectedEncoding,
		EncodingConfidence: shared.EncodingConfidence,
		AutoDetected:       shared.AutoDetected,
		BOM: bomInfo{
			HasBOM: shared.BOM.HasBOM,
			Type:   shared.BOM.Type,
			Bytes:  append([]byte(nil), shared.BOM.Bytes...),
		},
		FileSizeBytes: shared.FileSizeBytes,
		Mode:          shared.Mode,
		stream:        shared,
	}, nil
}

func (stream *decodedTextStream) Finish() (filesystem.FileSnapshot, error) {
	return stream.stream.Finish()
}

func (stream *decodedTextStream) Close() error {
	if stream == nil || stream.stream == nil {
		return nil
	}
	return stream.stream.Close()
}
