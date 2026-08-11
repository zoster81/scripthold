package encoding

import (
	"fmt"
	"sort"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

const (
	iso2022ESC = byte(0x1b)
	iso2022SO  = byte(0x0e)
	iso2022SI  = byte(0x0f)
)

type rawCharset struct {
	name   string
	width  uint8
	decode []multibyteDecodeEntry
}

func (charset *rawCharset) lookup(src []byte) (multibyteDecodeEntry, bool) {
	if charset == nil || len(src) < int(charset.width) {
		return multibyteDecodeEntry{}, false
	}
	packed := packSourcePrefix(src, int(charset.width))
	key := sequenceKey(packed, charset.width)
	index := sort.Search(len(charset.decode), func(index int) bool {
		entry := charset.decode[index]
		return sequenceKey(entry.Packed, entry.Length) >= key
	})
	if index < len(charset.decode) {
		entry := charset.decode[index]
		if sequenceKey(entry.Packed, entry.Length) == key {
			return entry, true
		}
	}
	return multibyteDecodeEntry{}, false
}

type iso2022Encoding struct {
	name    string
	kind    string
	raw     map[string]*rawCharset
	encoder *generatedMultibyteEncoding
}

func newISO2022Encoding(spec *libiconvMultibyteSpec) encoding.Encoding {
	if spec == nil || spec.CanonicalName == "" || spec.Kind == "" || len(spec.Encode) == 0 {
		panic("ISO-2022 encoding requires generated metadata and canonical encode table")
	}
	raw := make(map[string]*rawCharset, len(generatedLibiconvRawCharsetSpecs))
	for index := range generatedLibiconvRawCharsetSpecs {
		rawSpec := &generatedLibiconvRawCharsetSpecs[index]
		if rawSpec.Name == "" || rawSpec.Width == 0 || rawSpec.Width > 2 || len(rawSpec.Decode) == 0 {
			panic("invalid generated ISO-2022 raw charset")
		}
		if _, exists := raw[rawSpec.Name]; exists {
			panic("duplicate generated ISO-2022 raw charset: " + rawSpec.Name)
		}
		raw[rawSpec.Name] = &rawCharset{name: rawSpec.Name, width: rawSpec.Width, decode: rawSpec.Decode}
	}
	encoderIndex := &generatedMultibyteEncoding{
		name:       spec.CanonicalName,
		encode:     spec.Encode,
		pairEncode: spec.PairEncode,
	}
	encoderIndex.validateAndIndex()
	registered := &iso2022Encoding{name: spec.CanonicalName, kind: spec.Kind, raw: raw, encoder: encoderIndex}
	registered.validateRawDependencies()
	return registered
}

func (enc *iso2022Encoding) validateRawDependencies() {
	var names []string
	switch enc.kind {
	case "iso2022-jp1":
		names = []string{"jis0201-roman", "jis0208", "jis0212"}
	case "iso2022-jp2":
		names = []string{"jis0201-roman", "jis0201-kana", "jis0208", "jis0212", "gb2312", "ksc5601", "iso8859-1-g2", "iso8859-7-g2"}
	case "iso2022-jp3":
		names = []string{"jis0201-roman", "jis0201-kana", "jis0208", "jis0213-plane1", "jis0213-plane2"}
	case "iso2022-jpms":
		names = []string{"jis0201-roman", "jis0201-kana", "jpms0208", "jpms0212"}
	case "iso2022-cn":
		names = []string{"gb2312", "cns1", "cns2"}
	case "iso2022-cn-ext":
		names = []string{"gb2312", "cns1", "cns2", "cns3", "cns4", "cns5", "cns6", "cns7", "isoir165"}
	case "iso2022-kr":
		names = []string{"ksc5601"}
	default:
		panic("unsupported generated ISO-2022 kind: " + enc.kind)
	}
	for _, name := range names {
		if enc.raw[name] == nil {
			panic(fmt.Sprintf("ISO-2022 encoding %s requires missing raw charset %s", enc.name, name))
		}
	}
}

func (enc *iso2022Encoding) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: &iso2022Decoder{encoding: enc}}
}

func (enc *iso2022Encoding) NewEncoder() *encoding.Encoder {
	return &encoding.Encoder{Transformer: &generatedMultibyteEncoder{encoding: enc.encoder}}
}

type iso2022State struct {
	g0      string
	g1      string
	g2      string
	g3      string
	shifted bool
}

type iso2022Decoder struct {
	encoding *iso2022Encoding
	state    iso2022State
}

func (decoder *iso2022Decoder) Reset() {
	decoder.state = iso2022State{}
}

func (decoder *iso2022Decoder) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		entry, consumed, next, err := decoder.decodeOne(src[nSrc:], atEOF)
		if err != nil {
			return nDst, nSrc, err
		}
		if consumed <= 0 {
			panic("ISO-2022 decoder made no progress")
		}
		if entry.Length == 0 {
			nSrc += consumed
			decoder.state = next
			continue
		}
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
		nSrc += consumed
		decoder.state = next
	}
	return nDst, nSrc, nil
}

func (decoder *iso2022Decoder) decodeOne(src []byte, atEOF bool) (multibyteDecodeEntry, int, iso2022State, error) {
	switch decoder.encoding.kind {
	case "iso2022-jp1":
		return decoder.decodeJP1(src, atEOF)
	case "iso2022-jp2":
		return decoder.decodeJP2(src, atEOF)
	case "iso2022-jp3":
		return decoder.decodeJP3(src, atEOF)
	case "iso2022-jpms":
		return decoder.decodeJPMS(src, atEOF)
	case "iso2022-cn":
		return decoder.decodeCN(src, atEOF, false)
	case "iso2022-cn-ext":
		return decoder.decodeCN(src, atEOF, true)
	case "iso2022-kr":
		return decoder.decodeKR(src, atEOF)
	default:
		panic("unknown ISO-2022 decoder kind")
	}
}

func (decoder *iso2022Decoder) initialG0(state iso2022State) iso2022State {
	if state.g0 == "" {
		state.g0 = "ascii"
	}
	return state
}

func shortOrInvalidISO2022(name string, atEOF bool) error {
	if !atEOF {
		return transform.ErrShortSrc
	}
	return fmt.Errorf("%w for %s: truncated escape or shifted sequence", ErrInvalidEncodedSequence, name)
}

func invalidISO2022(name string) error {
	return fmt.Errorf("%w for %s: invalid escape, shift, or code sequence", ErrInvalidEncodedSequence, name)
}

func finishISO2022StateOnly(name string, original, next iso2022State, consumed int, atEOF, neutral bool) (multibyteDecodeEntry, int, iso2022State, error) {
	if atEOF && consumed > 0 && neutral {
		return multibyteDecodeEntry{}, consumed, next, nil
	}
	return multibyteDecodeEntry{}, 0, original, shortOrInvalidISO2022(name, atEOF)
}

func asciiEntry(value byte) multibyteDecodeEntry {
	return multibyteDecodeEntry{Packed: uint32(value) << 24, Length: 1, Rune1: rune(value)}
}

func (decoder *iso2022Decoder) rawEntry(name string, src []byte) (multibyteDecodeEntry, bool) {
	charset := decoder.encoding.raw[name]
	if charset == nil {
		return multibyteDecodeEntry{}, false
	}
	return charset.lookup(src)
}

func (decoder *iso2022Decoder) decodeJP1(src []byte, atEOF bool) (multibyteDecodeEntry, int, iso2022State, error) {
	state := decoder.initialG0(decoder.state)
	i := 0
	for {
		if i >= len(src) {
			return finishISO2022StateOnly(decoder.encoding.name, decoder.state, state, i, atEOF, state.g0 == "ascii")
		}
		if src[i] != iso2022ESC {
			break
		}
		if len(src)-i < 3 {
			return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
		}
		switch {
		case src[i+1] == '(' && src[i+2] == 'B':
			state.g0 = "ascii"
			i += 3
		case src[i+1] == '(' && src[i+2] == 'J':
			state.g0 = "jis0201-roman"
			i += 3
		case src[i+1] == '$' && (src[i+2] == '@' || src[i+2] == 'B'):
			state.g0 = "jis0208"
			i += 3
		case src[i+1] == '$' && src[i+2] == '(':
			if len(src)-i < 4 {
				return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
			}
			if src[i+3] != 'D' {
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			state.g0 = "jis0212"
			i += 4
		default:
			return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
		}
	}
	return decoder.decodeJPGraphic(src, i, state, atEOF)
}

func (decoder *iso2022Decoder) decodeJP2(src []byte, atEOF bool) (multibyteDecodeEntry, int, iso2022State, error) {
	state := decoder.initialG0(decoder.state)
	i := 0
	for {
		if i >= len(src) {
			return finishISO2022StateOnly(decoder.encoding.name, decoder.state, state, i, atEOF, state.g0 == "ascii")
		}
		if src[i] != iso2022ESC {
			break
		}
		if len(src)-i < 2 {
			return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
		}
		if src[i+1] == 'N' {
			if len(src)-i < 3 {
				return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
			}
			if state.g2 == "" || src[i+2] >= 0x80 {
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			entry, ok := decoder.rawEntry(state.g2, src[i+2:i+3])
			if !ok {
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			return entry, i + 3, state, nil
		}
		if len(src)-i < 3 {
			return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
		}
		switch {
		case src[i+1] == '(' && src[i+2] == 'B':
			state.g0 = "ascii"
			i += 3
		case src[i+1] == '(' && src[i+2] == 'J':
			state.g0 = "jis0201-roman"
			i += 3
		case src[i+1] == '(' && src[i+2] == 'I':
			state.g0 = "jis0201-kana"
			i += 3
		case src[i+1] == '.' && src[i+2] == 'A':
			state.g2 = "iso8859-1-g2"
			i += 3
		case src[i+1] == '.' && src[i+2] == 'F':
			state.g2 = "iso8859-7-g2"
			i += 3
		case src[i+1] == '$' && (src[i+2] == '@' || src[i+2] == 'B'):
			state.g0 = "jis0208"
			i += 3
		case src[i+1] == '$' && src[i+2] == 'A':
			state.g0 = "gb2312"
			i += 3
		case src[i+1] == '$' && src[i+2] == '(':
			if len(src)-i < 4 {
				return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
			}
			switch src[i+3] {
			case 'D':
				state.g0 = "jis0212"
			case 'C':
				state.g0 = "ksc5601"
			default:
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			i += 4
		default:
			return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
		}
	}
	entry, consumed, next, err := decoder.decodeJPGraphic(src, i, state, atEOF)
	if err == nil && (entry.Rune1 == '\n' || entry.Rune1 == '\r') && (state.g0 == "ascii" || state.g0 == "jis0201-roman") {
		next.g2 = ""
	}
	return entry, consumed, next, err
}

func (decoder *iso2022Decoder) decodeJP3(src []byte, atEOF bool) (multibyteDecodeEntry, int, iso2022State, error) {
	state := decoder.initialG0(decoder.state)
	i := 0
	for {
		if i >= len(src) {
			return finishISO2022StateOnly(decoder.encoding.name, decoder.state, state, i, atEOF, state.g0 == "ascii")
		}
		if src[i] != iso2022ESC {
			break
		}
		if len(src)-i < 3 {
			return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
		}
		switch {
		case src[i+1] == '(' && src[i+2] == 'B':
			state.g0 = "ascii"
			i += 3
		case src[i+1] == '(' && src[i+2] == 'J':
			state.g0 = "jis0201-roman"
			i += 3
		case src[i+1] == '(' && src[i+2] == 'I':
			state.g0 = "jis0201-kana"
			i += 3
		case src[i+1] == '$' && (src[i+2] == '@' || src[i+2] == 'B'):
			state.g0 = "jis0208"
			i += 3
		case src[i+1] == '$' && src[i+2] == '(':
			if len(src)-i < 4 {
				return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
			}
			switch src[i+3] {
			case 'O', 'Q':
				state.g0 = "jis0213-plane1"
			case 'P':
				state.g0 = "jis0213-plane2"
			default:
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			i += 4
		default:
			return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
		}
	}
	return decoder.decodeJPGraphic(src, i, state, atEOF)
}

func (decoder *iso2022Decoder) decodeJPMS(src []byte, atEOF bool) (multibyteDecodeEntry, int, iso2022State, error) {
	state := decoder.initialG0(decoder.state)
	i := 0
	for {
		if i >= len(src) {
			neutral := state.g0 == "ascii" || state.g0 == "jis0201-roman"
			return finishISO2022StateOnly(decoder.encoding.name, decoder.state, state, i, atEOF, neutral)
		}
		switch src[i] {
		case iso2022ESC:
			if len(src)-i < 3 {
				return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
			}
			switch {
			case src[i+1] == '(' && src[i+2] == 'B':
				state.g0 = "ascii"
				i += 3
			case src[i+1] == '(' && src[i+2] == 'J':
				state.g0 = "jis0201-roman"
				i += 3
			case src[i+1] == '(' && src[i+2] == 'I':
				state.g0 = "jis0201-kana"
				i += 3
			case src[i+1] == '$' && (src[i+2] == '@' || src[i+2] == 'B'):
				state.g0 = "jpms0208"
				i += 3
			case src[i+1] == '$' && src[i+2] == '(':
				if len(src)-i < 4 {
					return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
				}
				if src[i+3] != 'D' {
					return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
				}
				state.g0 = "jpms0212"
				i += 4
			default:
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
		case iso2022SO:
			if state.g0 != "jis0201-roman" {
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			state.g0 = "jis0201-kana"
			i++
		case iso2022SI:
			if state.g0 != "jis0201-kana" {
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			state.g0 = "jis0201-roman"
			i++
		default:
			return decoder.decodeJPGraphic(src, i, state, atEOF)
		}
	}
}

func (decoder *iso2022Decoder) decodeJPGraphic(src []byte, offset int, state iso2022State, atEOF bool) (multibyteDecodeEntry, int, iso2022State, error) {
	if offset >= len(src) {
		return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
	}
	if state.g0 == "ascii" {
		if src[offset] >= 0x80 {
			return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
		}
		return asciiEntry(src[offset]), offset + 1, state, nil
	}
	charset := decoder.encoding.raw[state.g0]
	if charset == nil {
		return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
	}
	width := int(charset.width)
	if len(src)-offset < width {
		return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
	}
	for index := 0; index < width; index++ {
		if src[offset+index] >= 0x80 {
			return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
		}
	}
	entry, ok := charset.lookup(src[offset : offset+width])
	if !ok {
		return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
	}
	return entry, offset + width, state, nil
}

func (decoder *iso2022Decoder) decodeCN(src []byte, atEOF, extended bool) (multibyteDecodeEntry, int, iso2022State, error) {
	state := decoder.state
	i := 0
	for {
		if i >= len(src) {
			return finishISO2022StateOnly(decoder.encoding.name, decoder.state, state, i, atEOF, !state.shifted)
		}
		c := src[i]
		if c == iso2022ESC {
			if len(src)-i < 2 {
				return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
			}
			if src[i+1] == 'N' || src[i+1] == 'O' {
				if len(src)-i < 4 {
					return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
				}
				charsetName := state.g2
				if src[i+1] == 'O' {
					if !extended {
						return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
					}
					charsetName = state.g3
				}
				if charsetName == "" || src[i+2] >= 0x80 || src[i+3] >= 0x80 {
					return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
				}
				entry, ok := decoder.rawEntry(charsetName, src[i+2:i+4])
				if !ok {
					return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
				}
				return entry, i + 4, state, nil
			}
			if len(src)-i < 4 {
				return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
			}
			if src[i+1] != '$' {
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			switch {
			case src[i+2] == ')' && src[i+3] == 'A':
				state.g1 = "gb2312"
			case src[i+2] == ')' && src[i+3] == 'G':
				state.g1 = "cns1"
			case extended && src[i+2] == ')' && src[i+3] == 'E':
				state.g1 = "isoir165"
			case src[i+2] == '*' && src[i+3] == 'H':
				state.g2 = "cns2"
			case extended && src[i+2] == '+' && src[i+3] >= 'I' && src[i+3] <= 'M':
				state.g3 = fmt.Sprintf("cns%d", int(src[i+3]-'I')+3)
			default:
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			i += 4
			continue
		}
		if c == iso2022SO {
			if state.g1 == "" {
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			state.shifted = true
			i++
			continue
		}
		if c == iso2022SI {
			state.shifted = false
			i++
			continue
		}
		break
	}

	if !state.shifted {
		if src[i] >= 0x80 {
			return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
		}
		entry := asciiEntry(src[i])
		if entry.Rune1 == '\n' || entry.Rune1 == '\r' {
			state.g1, state.g2 = "", ""
			if extended {
				state.g3 = ""
			}
		}
		return entry, i + 1, state, nil
	}
	if state.g1 == "" || len(src)-i < 2 {
		if len(src)-i < 2 {
			return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
		}
		return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
	}
	if src[i] >= 0x80 || src[i+1] >= 0x80 {
		return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
	}
	entry, ok := decoder.rawEntry(state.g1, src[i:i+2])
	if !ok {
		return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
	}
	return entry, i + 2, state, nil
}

func (decoder *iso2022Decoder) decodeKR(src []byte, atEOF bool) (multibyteDecodeEntry, int, iso2022State, error) {
	state := decoder.state
	i := 0
	for {
		if i >= len(src) {
			return finishISO2022StateOnly(decoder.encoding.name, decoder.state, state, i, atEOF, !state.shifted)
		}
		switch src[i] {
		case iso2022ESC:
			if len(src)-i < 4 {
				return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
			}
			if src[i+1] != '$' || src[i+2] != ')' || src[i+3] != 'C' {
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			state.g1 = "ksc5601"
			i += 4
		case iso2022SO:
			if state.g1 != "ksc5601" {
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			state.shifted = true
			i++
		case iso2022SI:
			state.shifted = false
			i++
		default:
			if !state.shifted {
				if src[i] >= 0x80 {
					return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
				}
				return asciiEntry(src[i]), i + 1, state, nil
			}
			if len(src)-i < 2 {
				return multibyteDecodeEntry{}, 0, decoder.state, shortOrInvalidISO2022(decoder.encoding.name, atEOF)
			}
			if src[i] >= 0x80 || src[i+1] >= 0x80 {
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			entry, ok := decoder.rawEntry("ksc5601", src[i:i+2])
			if !ok {
				return multibyteDecodeEntry{}, 0, decoder.state, invalidISO2022(decoder.encoding.name)
			}
			return entry, i + 2, state, nil
		}
	}
}
