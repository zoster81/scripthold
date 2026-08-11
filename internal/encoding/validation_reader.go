package encoding

import (
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"golang.org/x/text/transform"
)

const validationOutputBufferSize = 256 * 1024

// ValidatingReader returns the original encoded bytes unchanged while feeding
// the same bytes through the registry's strict decoder. Any malformed source
// sequence aborts the read before a caller can commit staged output.
type ValidatingReader struct {
	source      io.Reader
	transformer transform.Transformer
	pending     []byte
	terminalErr error
	scratch     [validationOutputBufferSize]byte
}

// NewValidatingReader constructs a byte-preserving strict validator for one
// supported public encoding.
func NewValidatingReader(source io.Reader, charset string) (*ValidatingReader, error) {
	if source == nil {
		return nil, fmt.Errorf("validation source is nil")
	}

	var transformer transform.Transformer
	if IsUTF8(charset) {
		transformer = &strictUTF8SourceValidator{}
	} else {
		registered, ok := Get(charset)
		if !ok || registered == nil {
			return nil, fmt.Errorf("unsupported encoding: %s", charset)
		}
		transformer = registered.NewDecoder().Transformer
	}
	return &ValidatingReader{source: source, transformer: transformer}, nil
}

func (reader *ValidatingReader) Read(dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	if reader.terminalErr != nil {
		err := reader.terminalErr
		reader.terminalErr = nil
		return 0, err
	}

	n, sourceErr := reader.source.Read(dst)
	atEOF := errors.Is(sourceErr, io.EOF)
	if n > 0 {
		input := make([]byte, len(reader.pending)+n)
		copy(input, reader.pending)
		copy(input[len(reader.pending):], dst[:n])
		reader.pending = reader.pending[:0]

		if err := reader.validate(input, atEOF); err != nil {
			return 0, err
		}
		if sourceErr != nil {
			if atEOF {
				reader.terminalErr = io.EOF
			} else {
				reader.terminalErr = sourceErr
			}
		}
		return n, nil
	}

	if sourceErr != nil {
		if atEOF {
			if err := reader.validate(reader.pending, true); err != nil {
				reader.pending = reader.pending[:0]
				return 0, err
			}
			reader.pending = reader.pending[:0]
		}
		return 0, sourceErr
	}
	return 0, io.ErrNoProgress
}

func (reader *ValidatingReader) validate(input []byte, atEOF bool) error {
	for len(input) > 0 {
		_, consumed, err := reader.transformer.Transform(reader.scratch[:], input, atEOF)
		input = input[consumed:]
		switch err {
		case nil:
			if consumed == 0 && len(input) > 0 {
				return errors.New("encoding validator made no progress")
			}
		case transform.ErrShortDst:
			if consumed == 0 {
				return errors.New("encoding validator output buffer is too small")
			}
			continue
		case transform.ErrShortSrc:
			if atEOF {
				return fmt.Errorf("%w: truncated encoded sequence", ErrInvalidEncodedSequence)
			}
			reader.pending = append(reader.pending[:0], input...)
			return nil
		default:
			return err
		}
	}

	if atEOF {
		_, _, err := reader.transformer.Transform(reader.scratch[:], nil, true)
		if err == transform.ErrShortSrc {
			return fmt.Errorf("%w: truncated encoded sequence", ErrInvalidEncodedSequence)
		}
		if err != nil && err != transform.ErrShortDst {
			return err
		}
	}
	return nil
}

type strictUTF8SourceValidator struct{}

func (*strictUTF8SourceValidator) Reset() {}

func (*strictUTF8SourceValidator) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		if !utf8.FullRune(src[nSrc:]) {
			if !atEOF {
				return nDst, nSrc, transform.ErrShortSrc
			}
			return nDst, nSrc, fmt.Errorf("%w for utf-8: truncated sequence", ErrInvalidEncodedSequence)
		}
		r, size := utf8.DecodeRune(src[nSrc:])
		if r == utf8.RuneError && size == 1 {
			return nDst, nSrc, fmt.Errorf("%w for utf-8", ErrInvalidEncodedSequence)
		}
		if len(dst)-nDst < size {
			return nDst, nSrc, transform.ErrShortDst
		}
		copy(dst[nDst:nDst+size], src[nSrc:nSrc+size])
		nDst += size
		nSrc += size
	}
	return nDst, nSrc, nil
}
