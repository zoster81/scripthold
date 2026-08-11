package encoding

import (
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

const undefinedSingleByteRune rune = -1

type libiconvSingleByteSpec struct {
	CanonicalName      string
	DisplayName        string
	SourceName         string
	SourceID           string
	SourceDefinition   string
	SourceHeaderSHA256 string
	Aliases            []string
	Decode             [256]rune
}

type singleByteEncoding struct {
	name    string
	decode  *[256]rune
	reverse map[rune]byte
}

func newSingleByteEncoding(name string, decode *[256]rune) encoding.Encoding {
	if name == "" || decode == nil {
		panic("single-byte encoding requires a name and decode table")
	}
	reverse := make(map[rune]byte, 256)
	for value, r := range decode {
		if r == undefinedSingleByteRune {
			continue
		}
		if r < 0 || r > utf8.MaxRune || r >= 0xD800 && r <= 0xDFFF {
			panic(fmt.Sprintf("invalid Unicode scalar U+%04X in single-byte encoding %s at byte 0x%02X", r, name, value))
		}
		if previous, exists := reverse[r]; exists {
			panic(fmt.Sprintf("non-bijective single-byte encoding %s: bytes 0x%02X and 0x%02X both map to U+%04X", name, previous, value, r))
		}
		reverse[r] = byte(value)
	}
	if len(reverse) == 0 {
		panic("single-byte encoding has no defined bytes: " + name)
	}
	return &singleByteEncoding{name: name, decode: decode, reverse: reverse}
}

func (enc *singleByteEncoding) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: &singleByteDecoder{encoding: enc}}
}

func (enc *singleByteEncoding) NewEncoder() *encoding.Encoder {
	return &encoding.Encoder{Transformer: &singleByteEncoder{encoding: enc}}
}

type singleByteDecoder struct {
	encoding *singleByteEncoding
}

func (decoder *singleByteDecoder) Reset() {}

func (decoder *singleByteDecoder) Transform(dst, src []byte, _ bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		value := src[nSrc]
		r := decoder.encoding.decode[value]
		if r == undefinedSingleByteRune {
			return nDst, nSrc, fmt.Errorf("%w for %s: undefined byte 0x%02X", ErrInvalidEncodedSequence, decoder.encoding.name, value)
		}
		size := utf8.RuneLen(r)
		if len(dst)-nDst < size {
			return nDst, nSrc, transform.ErrShortDst
		}
		nDst += utf8.EncodeRune(dst[nDst:], r)
		nSrc++
	}
	return nDst, nSrc, nil
}

type singleByteEncoder struct {
	encoding *singleByteEncoding
}

func (encoder *singleByteEncoder) Reset() {}

func (encoder *singleByteEncoder) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		if !utf8.FullRune(src[nSrc:]) {
			if !atEOF {
				return nDst, nSrc, transform.ErrShortSrc
			}
			return nDst, nSrc, encoding.ErrInvalidUTF8
		}
		r, size := utf8.DecodeRune(src[nSrc:])
		if r == utf8.RuneError && size == 1 {
			return nDst, nSrc, encoding.ErrInvalidUTF8
		}
		value, ok := encoder.encoding.reverse[r]
		if !ok {
			return nDst, nSrc, fmt.Errorf("encoding %s: rune U+%04X is not representable", encoder.encoding.name, r)
		}
		if len(dst)-nDst < 1 {
			return nDst, nSrc, transform.ErrShortDst
		}
		dst[nDst] = value
		nDst++
		nSrc += size
	}
	return nDst, nSrc, nil
}

func buildLibiconvSingleByteDescriptors() []EncodingDescriptor {
	descriptors := make([]EncodingDescriptor, 0, len(generatedLibiconvSingleByteSpecs))
	for index := range generatedLibiconvSingleByteSpecs {
		spec := &generatedLibiconvSingleByteSpecs[index]
		descriptors = append(descriptors, EncodingDescriptor{
			Name:         spec.CanonicalName,
			Encoding:     newSingleByteEncoding(spec.CanonicalName, &spec.Decode),
			DisplayName:  spec.DisplayName,
			Aliases:      append([]string(nil), spec.Aliases...),
			Description:  "Portable single-byte mapping from pinned GNU libiconv (" + spec.SourceName + ")",
			Supported:    true,
			Readable:     true,
			Writable:     true,
			ExplicitOnly: true,
		})
	}
	return descriptors
}
