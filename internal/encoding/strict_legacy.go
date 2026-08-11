package encoding

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

// ErrInvalidEncodedSequence reports malformed source bytes that a permissive
// upstream decoder would otherwise replace with U+FFFD.
var ErrInvalidEncodedSequence = errors.New("invalid encoded byte sequence")

type strictLegacyEncoding struct {
	name string
	base encoding.Encoding
}

func newStrictLegacyEncoding(name string, base encoding.Encoding) encoding.Encoding {
	if base == nil {
		panic("strict legacy encoding requires a base codec: " + name)
	}
	if _, err := base.NewEncoder().Bytes([]byte(string(utf8.RuneError))); err == nil {
		panic("strict replacement rejection is unsafe for encoding that can represent U+FFFD: " + name)
	}
	return &strictLegacyEncoding{name: name, base: base}
}

func (enc *strictLegacyEncoding) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: transform.Chain(
		enc.base.NewDecoder(),
		&rejectReplacementTransformer{name: enc.name},
	)}
}

func (enc *strictLegacyEncoding) NewEncoder() *encoding.Encoder {
	return enc.base.NewEncoder()
}

func underlyingXTextEncoding(enc encoding.Encoding) encoding.Encoding {
	switch strict := enc.(type) {
	case *strictLegacyEncoding:
		return strict.base
	case *strictGB18030Encoding:
		return strict.base
	case *strictUTF16Encoding:
		return strict.base
	case *strictUTF32Encoding:
		return strict.base
	default:
		return enc
	}
}

type rejectReplacementTransformer struct {
	name string
}

func (transformer *rejectReplacementTransformer) Reset() {}

func (transformer *rejectReplacementTransformer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		if !utf8.FullRune(src[nSrc:]) {
			if !atEOF {
				return nDst, nSrc, transform.ErrShortSrc
			}
			return nDst, nSrc, fmt.Errorf("%w for %s: decoder produced truncated UTF-8", ErrInvalidEncodedSequence, transformer.name)
		}

		r, size := utf8.DecodeRune(src[nSrc:])
		if r == utf8.RuneError {
			return nDst, nSrc, fmt.Errorf("%w for %s", ErrInvalidEncodedSequence, transformer.name)
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
