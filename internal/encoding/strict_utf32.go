package encoding

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

type strictUTF32Encoding struct {
	name  string
	base  encoding.Encoding
	order binary.ByteOrder
}

func newStrictUTF32Encoding(name string, base encoding.Encoding, order binary.ByteOrder) encoding.Encoding {
	if base == nil || order == nil {
		panic("strict UTF-32 encoding requires codec and byte order: " + name)
	}
	return &strictUTF32Encoding{name: name, base: base, order: order}
}

func (enc *strictUTF32Encoding) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: transform.Chain(
		&utf32SourceValidator{name: enc.name, order: enc.order},
		enc.base.NewDecoder(),
	)}
}

func (enc *strictUTF32Encoding) NewEncoder() *encoding.Encoder {
	return enc.base.NewEncoder()
}

type utf32SourceValidator struct {
	name  string
	order binary.ByteOrder
}

func (validator *utf32SourceValidator) Reset() {}

func (validator *utf32SourceValidator) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		if len(src)-nSrc < 4 {
			if !atEOF {
				return nDst, nSrc, transform.ErrShortSrc
			}
			return nDst, nSrc, fmt.Errorf("%w for %s: byte length is not a multiple of four", ErrInvalidEncodedSequence, validator.name)
		}

		value := validator.order.Uint32(src[nSrc : nSrc+4])
		if value > utf8.MaxRune {
			return nDst, nSrc, fmt.Errorf("%w for %s: code point U+%X exceeds U+10FFFF", ErrInvalidEncodedSequence, validator.name, value)
		}
		if value >= 0xD800 && value <= 0xDFFF {
			return nDst, nSrc, fmt.Errorf("%w for %s: surrogate code point U+%04X", ErrInvalidEncodedSequence, validator.name, value)
		}
		if len(dst)-nDst < 4 {
			return nDst, nSrc, transform.ErrShortDst
		}
		copy(dst[nDst:nDst+4], src[nSrc:nSrc+4])
		nDst += 4
		nSrc += 4
	}
	return nDst, nSrc, nil
}
