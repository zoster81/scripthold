package encoding

import (
	"bytes"
	"fmt"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

type strictGB18030Encoding struct {
	base            encoding.Encoding
	replacementByte []byte
}

func newStrictGB18030Encoding(base encoding.Encoding) encoding.Encoding {
	if base == nil {
		panic("strict GB18030 encoding requires a base codec")
	}
	replacement, err := base.NewEncoder().Bytes([]byte("\ufffd"))
	if err != nil || len(replacement) == 0 {
		panic("GB18030 must encode U+FFFD")
	}
	return &strictGB18030Encoding{
		base:            base,
		replacementByte: append([]byte(nil), replacement...),
	}
}

func (enc *strictGB18030Encoding) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: transform.Chain(
		&gb18030SourceValidator{base: enc.base, replacementByte: enc.replacementByte},
		enc.base.NewDecoder(),
	)}
}

func (enc *strictGB18030Encoding) NewEncoder() *encoding.Encoder {
	return enc.base.NewEncoder()
}

type gb18030SourceValidator struct {
	base            encoding.Encoding
	replacementByte []byte
}

func (validator *gb18030SourceValidator) Reset() {}

func (validator *gb18030SourceValidator) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		sequenceLength, needMore, valid := gb18030SequenceLength(src[nSrc:])
		if needMore {
			if !atEOF {
				return nDst, nSrc, transform.ErrShortSrc
			}
			return nDst, nSrc, fmt.Errorf("%w for gb18030: truncated sequence", ErrInvalidEncodedSequence)
		}
		if !valid {
			return nDst, nSrc, fmt.Errorf("%w for gb18030 at byte 0x%02x", ErrInvalidEncodedSequence, src[nSrc])
		}
		if len(dst)-nDst < sequenceLength {
			return nDst, nSrc, transform.ErrShortDst
		}

		sequence := src[nSrc : nSrc+sequenceLength]
		if sequenceLength > 1 {
			decoded, decodeErr := validator.base.NewDecoder().Bytes(sequence)
			if decodeErr != nil {
				return nDst, nSrc, fmt.Errorf("%w for gb18030: %v", ErrInvalidEncodedSequence, decodeErr)
			}
			if bytes.Equal(decoded, []byte("\ufffd")) && !bytes.Equal(sequence, validator.replacementByte) {
				return nDst, nSrc, fmt.Errorf("%w for gb18030: undefined sequence %x", ErrInvalidEncodedSequence, sequence)
			}
		}

		copy(dst[nDst:nDst+sequenceLength], sequence)
		nDst += sequenceLength
		nSrc += sequenceLength
	}
	return nDst, nSrc, nil
}

func gb18030SequenceLength(src []byte) (length int, needMore, valid bool) {
	if len(src) == 0 {
		return 0, true, false
	}
	first := src[0]
	if first <= 0x7f || first == 0x80 {
		return 1, false, true
	}
	if first < 0x81 || first > 0xfe {
		return 0, false, false
	}
	if len(src) < 2 {
		return 0, true, false
	}

	second := src[1]
	if (0x40 <= second && second <= 0x7e) || (0x80 <= second && second <= 0xfe) {
		return 2, false, true
	}
	if second < 0x30 || second > 0x39 {
		return 0, false, false
	}
	if len(src) < 4 {
		return 0, true, false
	}
	third, fourth := src[2], src[3]
	if third < 0x81 || third > 0xfe || fourth < 0x30 || fourth > 0x39 {
		return 0, false, false
	}
	return 4, false, true
}
