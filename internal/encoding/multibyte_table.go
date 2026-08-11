package encoding

import (
	"fmt"
	"sort"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

const maxGeneratedMultibyteSequence = 4

type multibyteDecodeEntry struct {
	Packed uint32
	Length uint8
	Rune1  rune
	Rune2  rune
}

type multibyteEncodeEntry struct {
	Rune  rune
	Bytes string
}

type multibytePairEncodeEntry struct {
	First  rune
	Second rune
	Bytes  string
}

type libiconvMultibyteSpec struct {
	CanonicalName      string
	DisplayName        string
	SourceName         string
	SourceID           string
	SourceDefinition   string
	SourceHeaderSHA256 string
	Aliases            []string
	Kind               string
	Decode             []multibyteDecodeEntry
	Encode             []multibyteEncodeEntry
	PairEncode         []multibytePairEncodeEntry
}

type libiconvRawCharsetSpec struct {
	Name   string
	Width  uint8
	Decode []multibyteDecodeEntry
}

type generatedMultibyteEncoding struct {
	name       string
	decode     []multibyteDecodeEntry
	prefixes   []uint64
	encode     []multibyteEncodeEntry
	pairEncode []multibytePairEncodeEntry
	pairLeads  []rune
}

func newGeneratedMultibyteEncoding(spec *libiconvMultibyteSpec) encoding.Encoding {
	if spec == nil || spec.CanonicalName == "" || len(spec.Decode) == 0 || len(spec.Encode) == 0 {
		panic("generated multibyte encoding requires name, decode table, and encode table")
	}
	enc := &generatedMultibyteEncoding{
		name:       spec.CanonicalName,
		decode:     spec.Decode,
		encode:     spec.Encode,
		pairEncode: spec.PairEncode,
	}
	enc.validateAndIndex()
	return enc
}

func (enc *generatedMultibyteEncoding) validateAndIndex() {
	prefixes := make([]uint64, 0, len(enc.decode))
	var previous uint64
	for index, entry := range enc.decode {
		if entry.Length == 0 || entry.Length > maxGeneratedMultibyteSequence {
			panic(fmt.Sprintf("encoding %s has invalid decode length %d", enc.name, entry.Length))
		}
		validateGeneratedScalar(enc.name, entry.Rune1)
		if entry.Rune2 != 0 {
			validateGeneratedScalar(enc.name, entry.Rune2)
		}
		key := sequenceKey(entry.Packed, entry.Length)
		if index > 0 && key <= previous {
			panic(fmt.Sprintf("encoding %s decode table is not strictly sorted", enc.name))
		}
		previous = key
		for length := uint8(1); length < entry.Length; length++ {
			prefixes = append(prefixes, sequenceKey(maskPackedPrefix(entry.Packed, length), length))
		}
	}
	sort.Slice(prefixes, func(i, j int) bool { return prefixes[i] < prefixes[j] })
	enc.prefixes = dedupeUint64(prefixes)
	for _, entry := range enc.decode {
		key := sequenceKey(entry.Packed, entry.Length)
		index := sort.Search(len(enc.prefixes), func(index int) bool { return enc.prefixes[index] >= key })
		if index < len(enc.prefixes) && enc.prefixes[index] == key {
			panic(fmt.Sprintf("encoding %s decode table is not prefix-free at length %d packed 0x%08X", enc.name, entry.Length, entry.Packed))
		}
	}

	var previousRune rune = -1
	for _, entry := range enc.encode {
		validateGeneratedScalar(enc.name, entry.Rune)
		if entry.Rune <= previousRune || entry.Bytes == "" {
			panic(fmt.Sprintf("encoding %s has invalid or unsorted encode table", enc.name))
		}
		previousRune = entry.Rune
	}

	var previousPair uint64
	pairLeads := make([]rune, 0, len(enc.pairEncode))
	for index, entry := range enc.pairEncode {
		validateGeneratedScalar(enc.name, entry.First)
		validateGeneratedScalar(enc.name, entry.Second)
		if entry.Bytes == "" {
			panic(fmt.Sprintf("encoding %s has empty pair encoding", enc.name))
		}
		key := runePairKey(entry.First, entry.Second)
		if index > 0 && key <= previousPair {
			panic(fmt.Sprintf("encoding %s pair table is not strictly sorted", enc.name))
		}
		previousPair = key
		pairLeads = append(pairLeads, entry.First)
	}
	sort.Slice(pairLeads, func(i, j int) bool { return pairLeads[i] < pairLeads[j] })
	enc.pairLeads = dedupeRunes(pairLeads)
}

func validateGeneratedScalar(name string, r rune) {
	if r < 0 || r > utf8.MaxRune || r >= 0xD800 && r <= 0xDFFF {
		panic(fmt.Sprintf("encoding %s contains invalid Unicode scalar U+%04X", name, r))
	}
}

func dedupeUint64(values []uint64) []uint64 {
	if len(values) < 2 {
		return values
	}
	write := 1
	for read := 1; read < len(values); read++ {
		if values[read] == values[write-1] {
			continue
		}
		values[write] = values[read]
		write++
	}
	return values[:write]
}

func dedupeRunes(values []rune) []rune {
	if len(values) < 2 {
		return values
	}
	write := 1
	for read := 1; read < len(values); read++ {
		if values[read] == values[write-1] {
			continue
		}
		values[write] = values[read]
		write++
	}
	return values[:write]
}

func sequenceKey(packed uint32, length uint8) uint64 {
	return uint64(length)<<32 | uint64(packed)
}

func maskPackedPrefix(packed uint32, length uint8) uint32 {
	if length >= 4 {
		return packed
	}
	shift := uint(32 - 8*length)
	return packed >> shift << shift
}

func packSourcePrefix(src []byte, length int) uint32 {
	var packed uint32
	for index := 0; index < length; index++ {
		packed |= uint32(src[index]) << uint(24-8*index)
	}
	return packed
}

func runePairKey(first, second rune) uint64 {
	return uint64(first)<<21 | uint64(second)
}

func (enc *generatedMultibyteEncoding) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: &generatedMultibyteDecoder{encoding: enc}}
}

func (enc *generatedMultibyteEncoding) NewEncoder() *encoding.Encoder {
	return &encoding.Encoder{Transformer: &generatedMultibyteEncoder{encoding: enc}}
}

type generatedMultibyteDecoder struct {
	encoding *generatedMultibyteEncoding
}

func (decoder *generatedMultibyteDecoder) Reset() {}

func (decoder *generatedMultibyteDecoder) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		start := nSrc
		remaining := src[start:]
		limit := len(remaining)
		if limit > maxGeneratedMultibyteSequence {
			limit = maxGeneratedMultibyteSequence
		}
		matchedPrefix := false
		for length := 1; length <= limit; length++ {
			packed := packSourcePrefix(remaining, length)
			if entry, ok := decoder.encoding.lookupDecode(packed, uint8(length)); ok {
				required := utf8.RuneLen(entry.Rune1)
				if entry.Rune2 != 0 {
					required += utf8.RuneLen(entry.Rune2)
				}
				if len(dst)-nDst < required {
					return nDst, nSrc, transform.ErrShortDst
				}
				nDst += utf8.EncodeRune(dst[nDst:], entry.Rune1)
				if entry.Rune2 != 0 {
					nDst += utf8.EncodeRune(dst[nDst:], entry.Rune2)
				}
				nSrc += length
				matchedPrefix = true
				break
			}
			if decoder.encoding.hasPrefix(packed, uint8(length)) {
				matchedPrefix = true
				continue
			}
			return nDst, nSrc, fmt.Errorf("%w for %s at byte 0x%02X", ErrInvalidEncodedSequence, decoder.encoding.name, remaining[0])
		}
		if nSrc > start {
			continue
		}
		if matchedPrefix && limit == len(remaining) && limit < maxGeneratedMultibyteSequence {
			if !atEOF {
				return nDst, nSrc, transform.ErrShortSrc
			}
			return nDst, nSrc, fmt.Errorf("%w for %s: truncated multibyte sequence", ErrInvalidEncodedSequence, decoder.encoding.name)
		}
		if nSrc == len(src)-len(remaining) {
			return nDst, nSrc, fmt.Errorf("%w for %s: invalid multibyte sequence", ErrInvalidEncodedSequence, decoder.encoding.name)
		}
	}
	return nDst, nSrc, nil
}

func (enc *generatedMultibyteEncoding) lookupDecode(packed uint32, length uint8) (multibyteDecodeEntry, bool) {
	key := sequenceKey(packed, length)
	index := sort.Search(len(enc.decode), func(index int) bool {
		entry := enc.decode[index]
		return sequenceKey(entry.Packed, entry.Length) >= key
	})
	if index < len(enc.decode) {
		entry := enc.decode[index]
		if sequenceKey(entry.Packed, entry.Length) == key {
			return entry, true
		}
	}
	return multibyteDecodeEntry{}, false
}

func (enc *generatedMultibyteEncoding) hasPrefix(packed uint32, length uint8) bool {
	key := sequenceKey(maskPackedPrefix(packed, length), length)
	index := sort.Search(len(enc.prefixes), func(index int) bool { return enc.prefixes[index] >= key })
	return index < len(enc.prefixes) && enc.prefixes[index] == key
}

type generatedMultibyteEncoder struct {
	encoding *generatedMultibyteEncoding
}

func (encoder *generatedMultibyteEncoder) Reset() {}

func (encoder *generatedMultibyteEncoder) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		first, firstSize, err := decodeStrictUTF8Rune(src[nSrc:], atEOF)
		if err != nil {
			return nDst, nSrc, err
		}
		standalone, hasStandalone := encoder.encoding.lookupEncode(first)
		if encoder.encoding.isPairLead(first) {
			if nSrc+firstSize == len(src) {
				if !atEOF {
					return nDst, nSrc, transform.ErrShortSrc
				}
			} else {
				second, secondSize, secondErr := decodeStrictUTF8Rune(src[nSrc+firstSize:], atEOF)
				if secondErr == nil {
					if pair, ok := encoder.encoding.lookupPair(first, second); ok {
						if len(dst)-nDst < len(pair) {
							return nDst, nSrc, transform.ErrShortDst
						}
						copy(dst[nDst:], pair)
						nDst += len(pair)
						nSrc += firstSize + secondSize
						continue
					}
				} else if secondErr == transform.ErrShortSrc && !atEOF {
					return nDst, nSrc, transform.ErrShortSrc
				}
			}
		}
		if !hasStandalone {
			return nDst, nSrc, fmt.Errorf("encoding %s: rune U+%04X is not representable", encoder.encoding.name, first)
		}
		if len(dst)-nDst < len(standalone) {
			return nDst, nSrc, transform.ErrShortDst
		}
		copy(dst[nDst:], standalone)
		nDst += len(standalone)
		nSrc += firstSize
	}
	return nDst, nSrc, nil
}

func decodeStrictUTF8Rune(src []byte, atEOF bool) (rune, int, error) {
	if !utf8.FullRune(src) {
		if !atEOF {
			return 0, 0, transform.ErrShortSrc
		}
		return 0, 0, encoding.ErrInvalidUTF8
	}
	r, size := utf8.DecodeRune(src)
	if r == utf8.RuneError && size == 1 {
		return 0, 0, encoding.ErrInvalidUTF8
	}
	return r, size, nil
}

func (enc *generatedMultibyteEncoding) lookupEncode(r rune) (string, bool) {
	index := sort.Search(len(enc.encode), func(index int) bool { return enc.encode[index].Rune >= r })
	if index < len(enc.encode) && enc.encode[index].Rune == r {
		return enc.encode[index].Bytes, true
	}
	return "", false
}

func (enc *generatedMultibyteEncoding) isPairLead(r rune) bool {
	index := sort.Search(len(enc.pairLeads), func(index int) bool { return enc.pairLeads[index] >= r })
	return index < len(enc.pairLeads) && enc.pairLeads[index] == r
}

func (enc *generatedMultibyteEncoding) lookupPair(first, second rune) (string, bool) {
	key := runePairKey(first, second)
	index := sort.Search(len(enc.pairEncode), func(index int) bool {
		entry := enc.pairEncode[index]
		return runePairKey(entry.First, entry.Second) >= key
	})
	if index < len(enc.pairEncode) {
		entry := enc.pairEncode[index]
		if runePairKey(entry.First, entry.Second) == key {
			return entry.Bytes, true
		}
	}
	return "", false
}

func buildLibiconvMultibyteDescriptors() []EncodingDescriptor {
	descriptors := make([]EncodingDescriptor, 0, len(generatedLibiconvMultibyteSpecs))
	for index := range generatedLibiconvMultibyteSpecs {
		spec := &generatedLibiconvMultibyteSpecs[index]
		var registered encoding.Encoding
		switch spec.Kind {
		case "direct":
			registered = newGeneratedMultibyteEncoding(spec)
		case "tcvn":
			registered = newTCVNEncoding(spec)
		default:
			registered = newISO2022Encoding(spec)
		}
		descriptors = append(descriptors, EncodingDescriptor{
			Name:         spec.CanonicalName,
			Encoding:     registered,
			DisplayName:  spec.DisplayName,
			Aliases:      append([]string(nil), spec.Aliases...),
			Description:  "Portable exact mapping from pinned GNU libiconv (" + spec.SourceName + ")",
			Supported:    true,
			Readable:     true,
			Writable:     true,
			ExplicitOnly: true,
		})
	}
	return descriptors
}
