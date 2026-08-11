package encoding

import (
	"encoding/binary"
	"fmt"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

type strictUTF16Encoding struct {
	name  string
	base  encoding.Encoding
	order binary.ByteOrder
}

func newStrictUTF16Encoding(name string, base encoding.Encoding, order binary.ByteOrder) encoding.Encoding {
	if base == nil || order == nil {
		panic("strict UTF-16 encoding requires codec and byte order: " + name)
	}
	return &strictUTF16Encoding{name: name, base: base, order: order}
}

func (enc *strictUTF16Encoding) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: transform.Chain(
		&utf16SourceValidator{name: enc.name, order: enc.order},
		enc.base.NewDecoder(),
	)}
}

func (enc *strictUTF16Encoding) NewEncoder() *encoding.Encoder {
	return enc.base.NewEncoder()
}

type utf16SourceValidator struct {
	name  string
	order binary.ByteOrder
}

func (validator *utf16SourceValidator) Reset() {}

func (validator *utf16SourceValidator) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		if len(src)-nSrc < 2 {
			if !atEOF {
				return nDst, nSrc, transform.ErrShortSrc
			}
			return nDst, nSrc, fmt.Errorf("%w for %s: odd byte length", ErrInvalidEncodedSequence, validator.name)
		}

		first := validator.order.Uint16(src[nSrc : nSrc+2])
		sequenceLength := 2
		switch {
		case 0xd800 <= first && first <= 0xdbff:
			if len(src)-nSrc < 4 {
				if !atEOF {
					return nDst, nSrc, transform.ErrShortSrc
				}
				return nDst, nSrc, fmt.Errorf("%w for %s: truncated surrogate pair", ErrInvalidEncodedSequence, validator.name)
			}
			second := validator.order.Uint16(src[nSrc+2 : nSrc+4])
			if second < 0xdc00 || second > 0xdfff {
				return nDst, nSrc, fmt.Errorf("%w for %s: high surrogate not followed by low surrogate", ErrInvalidEncodedSequence, validator.name)
			}
			sequenceLength = 4
		case 0xdc00 <= first && first <= 0xdfff:
			return nDst, nSrc, fmt.Errorf("%w for %s: isolated low surrogate", ErrInvalidEncodedSequence, validator.name)
		}

		if len(dst)-nDst < sequenceLength {
			return nDst, nSrc, transform.ErrShortDst
		}
		copy(dst[nDst:nDst+sequenceLength], src[nSrc:nSrc+sequenceLength])
		nDst += sequenceLength
		nSrc += sequenceLength
	}
	return nDst, nSrc, nil
}
