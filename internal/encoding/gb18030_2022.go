package encoding

import (
	"bytes"
	"fmt"
	"sort"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

type gb180302022DecodePatch struct {
	Packed uint32
	Length uint8
	Rune   rune
	Reject bool
}

type gb180302022EncodePatch struct {
	Rune   rune
	Bytes  string
	Reject bool
}

type gb180302022Encoding struct {
	base            encoding.Encoding
	replacementByte []byte
}

func newGB180302022Encoding() encoding.Encoding {
	base := simplifiedchinese.GB18030
	replacementByte, err := base.NewEncoder().Bytes([]byte("\ufffd"))
	if err != nil || len(replacementByte) == 0 {
		panic(fmt.Sprintf("GB18030 base cannot encode U+FFFD: %v", err))
	}
	validateGB180302022Patches()
	return &gb180302022Encoding{
		base:            base,
		replacementByte: append([]byte(nil), replacementByte...),
	}
}

func validateGB180302022Patches() {
	var previousDecode uint64
	for index, patch := range generatedGB180302022DecodePatches {
		if patch.Length != 2 && patch.Length != 4 {
			panic(fmt.Sprintf("GB18030:2022 decode patch %d has invalid length %d", index, patch.Length))
		}
		if !patch.Reject {
			validateGeneratedScalar("gb18030-2022", patch.Rune)
		}
		key := sequenceKey(patch.Packed, patch.Length)
		if index > 0 && key <= previousDecode {
			panic("GB18030:2022 decode patches are not strictly sorted")
		}
		previousDecode = key
	}
	var previousRune rune = -1
	for index, patch := range generatedGB180302022EncodePatches {
		validateGeneratedScalar("gb18030-2022", patch.Rune)
		if patch.Rune <= previousRune {
			panic("GB18030:2022 encode patches are not strictly sorted")
		}
		if !patch.Reject && len(patch.Bytes) == 0 {
			panic(fmt.Sprintf("GB18030:2022 encode patch %d has empty output", index))
		}
		previousRune = patch.Rune
	}
}

func (enc *gb180302022Encoding) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: &gb180302022Decoder{
		encoding: enc,
		base:     enc.base.NewDecoder(),
	}}
}

func (enc *gb180302022Encoding) NewEncoder() *encoding.Encoder {
	return &encoding.Encoder{Transformer: &gb180302022Encoder{
		encoding: enc,
		base:     enc.base.NewEncoder(),
	}}
}

type gb180302022Decoder struct {
	encoding *gb180302022Encoding
	base     *encoding.Decoder
	scratch  [8]byte
}

func (decoder *gb180302022Decoder) Reset() {
	decoder.base.Reset()
}

func (decoder *gb180302022Decoder) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		length, needMore, valid := gb18030SequenceLength(src[nSrc:])
		if needMore {
			if !atEOF {
				return nDst, nSrc, transform.ErrShortSrc
			}
			return nDst, nSrc, fmt.Errorf("%w: truncated GB18030:2022 sequence", ErrInvalidEncodedSequence)
		}
		if !valid {
			return nDst, nSrc, fmt.Errorf("%w: invalid GB18030:2022 byte sequence", ErrInvalidEncodedSequence)
		}
		sequence := src[nSrc : nSrc+length]
		if patch, ok := lookupGB180302022DecodePatch(sequence); ok {
			if patch.Reject {
				return nDst, nSrc, fmt.Errorf("%w: sequence is not defined by GB18030:2022", ErrInvalidEncodedSequence)
			}
			required := utf8.RuneLen(patch.Rune)
			if len(dst)-nDst < required {
				return nDst, nSrc, transform.ErrShortDst
			}
			nDst += utf8.EncodeRune(dst[nDst:], patch.Rune)
			nSrc += length
			continue
		}

		decoder.base.Reset()
		baseDst, baseSrc, baseErr := decoder.base.Transform(decoder.scratch[:], sequence, true)
		if baseErr != nil || baseSrc != length || !utf8.Valid(decoder.scratch[:baseDst]) {
			return nDst, nSrc, fmt.Errorf("%w: invalid GB18030:2022 sequence", ErrInvalidEncodedSequence)
		}
		r, size := utf8.DecodeRune(decoder.scratch[:baseDst])
		if size != baseDst || r == utf8.RuneError && !bytes.Equal(sequence, decoder.encoding.replacementByte) {
			return nDst, nSrc, fmt.Errorf("%w: undefined GB18030:2022 sequence", ErrInvalidEncodedSequence)
		}
		if len(dst)-nDst < baseDst {
			return nDst, nSrc, transform.ErrShortDst
		}
		copy(dst[nDst:], decoder.scratch[:baseDst])
		nDst += baseDst
		nSrc += length
	}
	return nDst, nSrc, nil
}

func lookupGB180302022DecodePatch(sequence []byte) (gb180302022DecodePatch, bool) {
	length := uint8(len(sequence))
	key := sequenceKey(packSourcePrefix(sequence, len(sequence)), length)
	index := sort.Search(len(generatedGB180302022DecodePatches), func(index int) bool {
		patch := generatedGB180302022DecodePatches[index]
		return sequenceKey(patch.Packed, patch.Length) >= key
	})
	if index < len(generatedGB180302022DecodePatches) {
		patch := generatedGB180302022DecodePatches[index]
		if sequenceKey(patch.Packed, patch.Length) == key {
			return patch, true
		}
	}
	return gb180302022DecodePatch{}, false
}

type gb180302022Encoder struct {
	encoding *gb180302022Encoding
	base     *encoding.Encoder
	scratch  [8]byte
}

func (encoder *gb180302022Encoder) Reset() {
	encoder.base.Reset()
}

func (encoder *gb180302022Encoder) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		r, size, err := decodeStrictUTF8Rune(src[nSrc:], atEOF)
		if err != nil {
			return nDst, nSrc, err
		}
		if patch, ok := lookupGB180302022EncodePatch(r); ok {
			if patch.Reject {
				return nDst, nSrc, fmt.Errorf("encoding gb18030-2022: rune U+%04X is not representable", r)
			}
			if len(dst)-nDst < len(patch.Bytes) {
				return nDst, nSrc, transform.ErrShortDst
			}
			copy(dst[nDst:], patch.Bytes)
			nDst += len(patch.Bytes)
			nSrc += size
			continue
		}

		encoder.base.Reset()
		baseDst, baseSrc, baseErr := encoder.base.Transform(encoder.scratch[:], src[nSrc:nSrc+size], true)
		if baseErr != nil || baseSrc != size {
			return nDst, nSrc, fmt.Errorf("encoding gb18030-2022: rune U+%04X is not representable", r)
		}
		if len(dst)-nDst < baseDst {
			return nDst, nSrc, transform.ErrShortDst
		}
		copy(dst[nDst:], encoder.scratch[:baseDst])
		nDst += baseDst
		nSrc += size
	}
	return nDst, nSrc, nil
}

func lookupGB180302022EncodePatch(r rune) (gb180302022EncodePatch, bool) {
	index := sort.Search(len(generatedGB180302022EncodePatches), func(index int) bool {
		return generatedGB180302022EncodePatches[index].Rune >= r
	})
	if index < len(generatedGB180302022EncodePatches) && generatedGB180302022EncodePatches[index].Rune == r {
		return generatedGB180302022EncodePatches[index], true
	}
	return gb180302022EncodePatch{}, false
}

func gb180302022Descriptor() EncodingDescriptor {
	return EncodingDescriptor{
		Name:         "gb18030-2022",
		Encoding:     newGB180302022Encoding(),
		DisplayName:  "GB18030:2022",
		Aliases:      []string{"gb18030:2022", "gb-18030-2022"},
		Description:  "GB18030:2022 with exhaustive pinned-libiconv differential patches over x/text",
		Supported:    true,
		Readable:     true,
		Writable:     true,
		ExplicitOnly: true,
	}
}
