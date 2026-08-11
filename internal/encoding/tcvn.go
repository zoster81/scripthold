package encoding

import (
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

type tcvnEncoding struct {
	name      string
	single    [256]rune
	defined   [256]bool
	pair      map[uint16]rune
	pairLeads [256]bool
	encoder   *generatedMultibyteEncoding
}

func newTCVNEncoding(spec *libiconvMultibyteSpec) encoding.Encoding {
	if spec == nil || spec.CanonicalName == "" || len(spec.Decode) == 0 || len(spec.Encode) == 0 {
		panic("TCVN encoding requires generated decode and encode mappings")
	}
	enc := &tcvnEncoding{
		name: spec.CanonicalName,
		pair: make(map[uint16]rune),
		encoder: &generatedMultibyteEncoding{
			name:       spec.CanonicalName,
			encode:     spec.Encode,
			pairEncode: spec.PairEncode,
		},
	}
	enc.encoder.validateAndIndex()
	for _, entry := range spec.Decode {
		switch entry.Length {
		case 1:
			value := byte(entry.Packed >> 24)
			if entry.Rune2 != 0 || enc.defined[value] {
				panic(fmt.Sprintf("TCVN has invalid duplicate one-byte mapping for 0x%02X", value))
			}
			enc.single[value] = entry.Rune1
			enc.defined[value] = true
		case 2:
			if entry.Rune2 != 0 {
				panic("TCVN pair mapping must emit exactly one Unicode scalar")
			}
			first := byte(entry.Packed >> 24)
			second := byte(entry.Packed >> 16)
			key := uint16(first)<<8 | uint16(second)
			if _, exists := enc.pair[key]; exists {
				panic(fmt.Sprintf("TCVN has duplicate pair mapping %02X%02X", first, second))
			}
			enc.pair[key] = entry.Rune1
			enc.pairLeads[first] = true
		default:
			panic(fmt.Sprintf("TCVN generated mapping has invalid length %d", entry.Length))
		}
	}
	for value := 0; value < 256; value++ {
		if !enc.defined[value] {
			panic(fmt.Sprintf("TCVN generated mapping is missing standalone byte 0x%02X", value))
		}
	}
	return enc
}

func (enc *tcvnEncoding) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: &tcvnDecoder{encoding: enc}}
}

func (enc *tcvnEncoding) NewEncoder() *encoding.Encoder {
	return &encoding.Encoder{Transformer: &generatedMultibyteEncoder{encoding: enc.encoder}}
}

type tcvnDecoder struct {
	encoding *tcvnEncoding
}

func (decoder *tcvnDecoder) Reset() {}

func (decoder *tcvnDecoder) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		first := src[nSrc]
		r := decoder.encoding.single[first]
		consume := 1
		if decoder.encoding.pairLeads[first] {
			if nSrc+1 >= len(src) {
				if !atEOF {
					return nDst, nSrc, transform.ErrShortSrc
				}
			} else {
				key := uint16(first)<<8 | uint16(src[nSrc+1])
				if composed, ok := decoder.encoding.pair[key]; ok {
					r = composed
					consume = 2
				}
			}
		}
		required := utf8.RuneLen(r)
		if len(dst)-nDst < required {
			return nDst, nSrc, transform.ErrShortDst
		}
		nDst += utf8.EncodeRune(dst[nDst:], r)
		nSrc += consume
	}
	return nDst, nSrc, nil
}
