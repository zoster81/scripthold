package encoding

import (
	"encoding/binary"
	"sort"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/ianaindex"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
)

// ValidationKind identifies structural validation required by an encoding.
type ValidationKind string

// LineEndingKind selects the raw, encoding-aware line-ending transformer.
type LineEndingKind string

const (
	LineEndingByte    LineEndingKind = "byte"
	LineEndingUTF16LE LineEndingKind = "utf-16-le"
	LineEndingUTF16BE LineEndingKind = "utf-16-be"
	LineEndingUTF32LE LineEndingKind = "utf-32-le"
	LineEndingUTF32BE LineEndingKind = "utf-32-be"
	LineEndingHZ      LineEndingKind = "hz-gb-2312"
)

const (
	ValidationNone    ValidationKind = "none"
	ValidationUTF8    ValidationKind = "utf-8"
	ValidationUTF16LE ValidationKind = "utf-16-le"
	ValidationUTF16BE ValidationKind = "utf-16-be"
	ValidationUTF32LE ValidationKind = "utf-32-le"
	ValidationUTF32BE ValidationKind = "utf-32-be"
)

// EncodingDescriptor is the authoritative capability record for one canonical encoding.
type EncodingDescriptor struct {
	Name             string
	Encoding         encoding.Encoding // nil is UTF-8 passthrough, or no codec when Supported is false.
	DisplayName      string
	Aliases          []string
	DetectorLabels   []string
	Description      string
	Supported        bool
	Readable         bool
	Writable         bool
	AutoDetectable   bool
	ExplicitOnly     bool
	Unicode          bool
	Validation       ValidationKind
	BOM              []byte
	AutoBOM          bool
	LineEndings      LineEndingKind
	externalDetector bool
	xTextEncoding    encoding.Encoding
	lineEndingCR     []byte
	lineEndingLF     []byte
}

var (
	utf16LEBase     = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	utf16BEBase     = unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
	utf16LEEncoding = newStrictUTF16Encoding("utf-16-le", utf16LEBase, binary.LittleEndian)
	utf16BEEncoding = newStrictUTF16Encoding("utf-16-be", utf16BEBase, binary.BigEndian)
	utf32LEBase     = utf32.UTF32(utf32.LittleEndian, utf32.IgnoreBOM)
	utf32BEBase     = utf32.UTF32(utf32.BigEndian, utf32.IgnoreBOM)
	utf32LEEncoding = newStrictUTF32Encoding("utf-32-le", utf32LEBase, binary.LittleEndian)
	utf32BEEncoding = newStrictUTF32Encoding("utf-32-be", utf32BEBase, binary.BigEndian)
)

func legacyDescriptor(name, displayName, description string, base encoding.Encoding, aliases, detectorLabels []string, autoDetect, strict bool) EncodingDescriptor {
	registered := base
	if strict {
		registered = newStrictLegacyEncoding(name, base)
	}
	descriptor := EncodingDescriptor{
		Name:           name,
		Encoding:       registered,
		DisplayName:    displayName,
		Aliases:        aliases,
		DetectorLabels: detectorLabels,
		Description:    description,
		Supported:      true,
		Readable:       true,
		Writable:       true,
		xTextEncoding:  base,
	}
	if autoDetect {
		descriptor.AutoDetectable = true
		descriptor.externalDetector = true
	} else {
		descriptor.ExplicitOnly = true
	}
	return descriptor
}

var xTextEncodingDescriptors = []EncodingDescriptor{
	{Name: "utf-8", DisplayName: "UTF-8", Aliases: []string{"utf8", "ascii", "us-ascii"}, DetectorLabels: []string{"UTF-8", "UTF-8-SIG", "US-ASCII", "Ascii"}, Description: "Unicode, no conversion", Supported: true, Readable: true, Writable: true, AutoDetectable: true, Unicode: true, Validation: ValidationUTF8, BOM: []byte{0xEF, 0xBB, 0xBF}, externalDetector: true, xTextEncoding: unicode.UTF8},
	{Name: "utf-16-le", Encoding: utf16LEEncoding, DisplayName: "UTF-16 LE", Aliases: []string{"utf16le", "utf-16le"}, DetectorLabels: []string{"UTF-16LE"}, Description: "Unicode UTF-16 Little Endian", Supported: true, Readable: true, Writable: true, AutoDetectable: true, Unicode: true, Validation: ValidationUTF16LE, BOM: []byte{0xFF, 0xFE}, AutoBOM: true, xTextEncoding: utf16LEBase},
	{Name: "utf-16-be", Encoding: utf16BEEncoding, DisplayName: "UTF-16 BE", Aliases: []string{"utf16be", "utf-16be"}, DetectorLabels: []string{"UTF-16BE"}, Description: "Unicode UTF-16 Big Endian", Supported: true, Readable: true, Writable: true, AutoDetectable: true, Unicode: true, Validation: ValidationUTF16BE, BOM: []byte{0xFE, 0xFF}, AutoBOM: true, xTextEncoding: utf16BEBase},

	legacyDescriptor("ibm037", "IBM CP037", "IBM EBCDIC US/Canada", charmap.CodePage037, []string{"cp037", "cp37"}, nil, false, true),
	legacyDescriptor("ibm437", "IBM CP437", "DOS OEM United States", charmap.CodePage437, []string{"cp437", "dos-437"}, nil, false, true),
	legacyDescriptor("ibm850", "IBM CP850", "DOS Western European", charmap.CodePage850, []string{"cp850", "dos-850"}, nil, false, true),
	legacyDescriptor("ibm852", "IBM CP852", "DOS Central European", charmap.CodePage852, []string{"cp852", "dos-852"}, nil, false, true),
	legacyDescriptor("ibm855", "IBM CP855", "DOS Cyrillic", charmap.CodePage855, []string{"cp855", "dos-855"}, []string{"IBM855"}, true, true),
	legacyDescriptor("ibm858", "IBM CP858", "DOS Western European with Euro", charmap.CodePage858, []string{"cp858", "dos-858"}, nil, false, true),
	legacyDescriptor("ibm860", "IBM CP860", "DOS Portuguese", charmap.CodePage860, []string{"cp860", "dos-860"}, nil, false, true),
	legacyDescriptor("ibm862", "IBM CP862", "DOS Hebrew", charmap.CodePage862, []string{"cp862", "dos-862"}, nil, false, true),
	legacyDescriptor("ibm863", "IBM CP863", "DOS Canadian French", charmap.CodePage863, []string{"cp863", "dos-863"}, nil, false, true),
	legacyDescriptor("ibm865", "IBM CP865", "DOS Nordic", charmap.CodePage865, []string{"cp865", "dos-865"}, nil, false, true),
	legacyDescriptor("ibm866", "IBM CP866", "DOS Cyrillic", charmap.CodePage866, []string{"cp866", "dos-866"}, []string{"IBM866"}, true, true),
	legacyDescriptor("ibm1047", "IBM CP1047", "IBM EBCDIC Latin-1/Open Systems", charmap.CodePage1047, []string{"cp1047"}, nil, false, true),
	legacyDescriptor("ibm1140", "IBM CP1140", "IBM EBCDIC CP037 with Euro", charmap.CodePage1140, []string{"cp1140"}, nil, false, true),

	legacyDescriptor("iso-8859-1", "ISO-8859-1", "Latin-1 Western European", charmap.ISO8859_1, []string{"iso88591", "latin1", "latin-1"}, []string{"ISO-8859-1"}, true, true),
	legacyDescriptor("iso-8859-2", "ISO-8859-2", "Latin-2 Central European", charmap.ISO8859_2, []string{"iso88592", "latin2"}, nil, false, true),
	legacyDescriptor("iso-8859-3", "ISO-8859-3", "Latin-3 South European", charmap.ISO8859_3, []string{"iso88593", "latin3"}, nil, false, true),
	legacyDescriptor("iso-8859-4", "ISO-8859-4", "Latin-4 North European", charmap.ISO8859_4, []string{"iso88594", "latin4"}, nil, false, true),
	legacyDescriptor("iso-8859-5", "ISO-8859-5", "ISO Cyrillic", charmap.ISO8859_5, []string{"iso88595", "cyrillic"}, []string{"ISO-8859-5"}, true, true),
	legacyDescriptor("iso-8859-6", "ISO-8859-6", "ISO Arabic", charmap.ISO8859_6, []string{"iso88596", "arabic"}, nil, false, true),
	legacyDescriptor("iso-8859-6-e", "ISO-8859-6-E", "ISO Arabic, explicit direction", charmap.ISO8859_6E, []string{"iso88596e", "iso-8859-6e"}, nil, false, true),
	legacyDescriptor("iso-8859-6-i", "ISO-8859-6-I", "ISO Arabic, implicit direction", charmap.ISO8859_6I, []string{"iso88596i", "iso-8859-6i"}, nil, false, true),
	legacyDescriptor("iso-8859-7", "ISO-8859-7", "ISO Greek", charmap.ISO8859_7, []string{"iso88597", "greek"}, []string{"ISO-8859-7"}, true, true),
	legacyDescriptor("iso-8859-8", "ISO-8859-8", "ISO Hebrew", charmap.ISO8859_8, []string{"iso88598", "hebrew", "visual"}, []string{"ISO-8859-8"}, true, true),
	legacyDescriptor("iso-8859-8-e", "ISO-8859-8-E", "ISO Hebrew, explicit direction", charmap.ISO8859_8E, []string{"iso88598e", "iso-8859-8e"}, nil, false, true),
	legacyDescriptor("iso-8859-8-i", "ISO-8859-8-I", "ISO Hebrew, implicit direction", charmap.ISO8859_8I, []string{"iso88598i", "iso-8859-8i", "logical"}, nil, false, true),
	legacyDescriptor("iso-8859-9", "ISO-8859-9", "Latin-5 Turkish", charmap.ISO8859_9, []string{"iso88599", "latin5"}, []string{"ISO-8859-9"}, true, true),
	legacyDescriptor("iso-8859-10", "ISO-8859-10", "Latin-6 Nordic", charmap.ISO8859_10, []string{"iso885910", "latin6"}, nil, false, true),
	legacyDescriptor("iso-8859-13", "ISO-8859-13", "Latin-7 Baltic Rim", charmap.ISO8859_13, []string{"iso885913"}, nil, false, true),
	legacyDescriptor("iso-8859-14", "ISO-8859-14", "Latin-8 Celtic", charmap.ISO8859_14, []string{"iso885914", "latin8"}, nil, false, true),
	legacyDescriptor("iso-8859-15", "ISO-8859-15", "Latin-9 Western European with Euro", charmap.ISO8859_15, []string{"iso885915", "latin9"}, nil, false, true),
	legacyDescriptor("iso-8859-16", "ISO-8859-16", "Latin-10 South-Eastern European", charmap.ISO8859_16, []string{"iso885916", "latin10"}, nil, false, true),

	legacyDescriptor("koi8-r", "KOI8-R", "Russian Cyrillic (Unix/Linux)", charmap.KOI8R, []string{"koi8r"}, []string{"KOI8-R"}, true, true),
	legacyDescriptor("koi8-u", "KOI8-U", "Ukrainian Cyrillic (Unix/Linux)", charmap.KOI8U, []string{"koi8u"}, nil, false, true),
	legacyDescriptor("macintosh", "Macintosh Roman", "Classic Mac OS Roman", charmap.Macintosh, []string{"mac", "macroman", "x-mac-roman"}, []string{"macintosh", "MacRoman"}, true, true),
	legacyDescriptor("x-mac-cyrillic", "Macintosh Cyrillic", "Classic Mac OS Cyrillic", charmap.MacintoshCyrillic, []string{"mac-cyrillic", "maccyrillic", "x-mac-ukrainian"}, []string{"x-mac-cyrillic", "MacCyrillic"}, true, true),

	legacyDescriptor("windows-874", "Windows-874", "Windows Thai", charmap.Windows874, []string{"cp874", "tis-620", "tis620", "iso-8859-11"}, []string{"TIS-620"}, true, true),
	legacyDescriptor("windows-1250", "Windows-1250", "Windows Central European", charmap.Windows1250, []string{"cp1250"}, nil, false, true),
	legacyDescriptor("windows-1251", "Windows-1251", "Windows Cyrillic", charmap.Windows1251, []string{"cp1251"}, []string{"Windows-1251"}, true, true),
	legacyDescriptor("windows-1252", "Windows-1252", "Windows Western European", charmap.Windows1252, []string{"cp1252"}, []string{"Windows-1252"}, true, true),
	legacyDescriptor("windows-1253", "Windows-1253", "Windows Greek", charmap.Windows1253, []string{"cp1253"}, []string{"Windows-1253"}, true, true),
	legacyDescriptor("windows-1254", "Windows-1254", "Windows Turkish", charmap.Windows1254, []string{"cp1254"}, []string{"Windows-1254"}, true, true),
	legacyDescriptor("windows-1255", "Windows-1255", "Windows Hebrew", charmap.Windows1255, []string{"cp1255"}, []string{"Windows-1255"}, true, true),
	legacyDescriptor("windows-1256", "Windows-1256", "Windows Arabic", charmap.Windows1256, []string{"cp1256"}, nil, false, true),
	legacyDescriptor("windows-1257", "Windows-1257", "Windows Baltic", charmap.Windows1257, []string{"cp1257"}, nil, false, true),
	legacyDescriptor("windows-1258", "Windows-1258", "Windows Vietnamese", charmap.Windows1258, []string{"cp1258"}, nil, false, true),
	legacyDescriptor("x-user-defined", "X-User-Defined", "WHATWG private-use single-byte encoding", charmap.XUserDefined, nil, nil, false, true),

	legacyDescriptor("gbk", "GBK", "Chinese Simplified (GBK)", simplifiedchinese.GBK, []string{"cp936", "gb2312", "gb-2312"}, []string{"GB2312"}, true, true),
	{Name: "gb18030", Encoding: newStrictGB18030Encoding(simplifiedchinese.GB18030), DisplayName: "GB18030", Aliases: []string{"gb-18030"}, Description: "Chinese Simplified (GB18030, full Unicode)", Supported: true, Readable: true, Writable: true, ExplicitOnly: true, xTextEncoding: simplifiedchinese.GB18030},
	legacyDescriptor("hz-gb-2312", "HZ-GB-2312", "HZ encoded GB 2312 Chinese", simplifiedchinese.HZGB2312, []string{"hzgb2312", "hz"}, []string{"HZ-GB-2312"}, true, true),
	legacyDescriptor("big5", "Big5", "Traditional Chinese Big5/WHATWG", traditionalchinese.Big5, []string{"cp950", "big5-hkscs"}, []string{"Big5"}, true, true),

	legacyDescriptor("euc-jp", "EUC-JP", "Japanese EUC-JP", japanese.EUCJP, []string{"eucjp", "x-euc-jp"}, []string{"EUC-JP"}, true, true),
	legacyDescriptor("iso-2022-jp", "ISO-2022-JP", "Japanese ISO-2022-JP", japanese.ISO2022JP, []string{"iso2022jp"}, []string{"ISO-2022-JP"}, true, true),
	legacyDescriptor("shift_jis", "Shift_JIS", "Japanese Shift_JIS/Windows-31J", japanese.ShiftJIS, []string{"shift-jis", "sjis", "cp932", "ms932", "windows-31j", "cp943", "ibm943", "ibm-943"}, []string{"Shift_JIS"}, true, true),
	legacyDescriptor("euc-kr", "EUC-KR", "Korean EUC-KR/Windows-949", korean.EUCKR, []string{"euckr", "cp949", "windows-949"}, []string{"EUC-KR", "CP949"}, true, true),

	{Name: "utf-32-le", Encoding: utf32LEEncoding, DisplayName: "UTF-32 LE", Aliases: []string{"utf32le", "utf-32le"}, DetectorLabels: []string{"UTF-32LE"}, Description: "Unicode UTF-32 Little Endian", Supported: true, Readable: true, Writable: true, AutoDetectable: true, Unicode: true, Validation: ValidationUTF32LE, BOM: []byte{0xFF, 0xFE, 0x00, 0x00}, AutoBOM: true, xTextEncoding: utf32LEBase},
	{Name: "utf-32-be", Encoding: utf32BEEncoding, DisplayName: "UTF-32 BE", Aliases: []string{"utf32be", "utf-32be"}, DetectorLabels: []string{"UTF-32BE"}, Description: "Unicode UTF-32 Big Endian", Supported: true, Readable: true, Writable: true, AutoDetectable: true, Unicode: true, Validation: ValidationUTF32BE, BOM: []byte{0x00, 0x00, 0xFE, 0xFF}, AutoBOM: true, xTextEncoding: utf32BEBase},
}

func enableExternalAutoDetection(descriptors []EncodingDescriptor, name string, detectorLabels []string) {
	for index := range descriptors {
		if descriptors[index].Name != name {
			continue
		}
		descriptors[index].DetectorLabels = append([]string(nil), detectorLabels...)
		descriptors[index].AutoDetectable = true
		descriptors[index].ExplicitOnly = false
		descriptors[index].externalDetector = true
		return
	}
	panic("auto-detection metadata references unknown encoding: " + name)
}

var encodingDescriptors = func() []EncodingDescriptor {
	descriptors := append([]EncodingDescriptor(nil), xTextEncodingDescriptors...)
	descriptors = append(descriptors, buildLibiconvSingleByteDescriptors()...)
	descriptors = append(descriptors, buildLibiconvMultibyteDescriptors()...)
	descriptors = append(descriptors, gb180302022Descriptor())
	enableExternalAutoDetection(descriptors, "iso-2022-kr", []string{"ISO-2022-KR"})
	return descriptors
}()

// Labels emitted by the pinned external detector that intentionally have no trusted
// public codec. Unknown labels are rejected by the same fail-closed path.
var rejectedDetectorLabels = []string{
	"UTF-16", "UTF-32", "X-ISO-10646-UCS-4-3412", "X-ISO-10646-UCS-4-2143",
	"CP932", "KS_C_5601-1987", "Johab", "EUC-TW",
	"Windows-1250", "ISO-8859-2", "Windows-1256", "Windows-1257",
	"ISO-8859-6", "ISO-8859-13", "ISO-2022-CN",
}

var (
	descriptorByName       map[string]*EncodingDescriptor
	descriptorByDetector   map[string]*EncodingDescriptor
	descriptorByIANAName   map[string]*EncodingDescriptor
	descriptorByHTMLName   map[string]*EncodingDescriptor
	rejectedDetectorLookup map[string]struct{}
	bomDescriptors         []*EncodingDescriptor
)

func init() {
	descriptorByName = make(map[string]*EncodingDescriptor)
	descriptorByDetector = make(map[string]*EncodingDescriptor)
	descriptorByIANAName = make(map[string]*EncodingDescriptor)
	descriptorByHTMLName = make(map[string]*EncodingDescriptor)
	rejectedDetectorLookup = make(map[string]struct{})

	for i := range encodingDescriptors {
		descriptor := &encodingDescriptors[i]
		if descriptor.Name == "" || normalizeRegistryName(descriptor.Name) != descriptor.Name {
			panic("invalid canonical encoding name: " + descriptor.Name)
		}
		if descriptor.Validation == "" {
			descriptor.Validation = ValidationNone
		}
		if descriptor.Supported {
			if !descriptor.Readable || !descriptor.Writable {
				panic("supported encoding lacks read/write capability: " + descriptor.Name)
			}
			if descriptor.AutoDetectable == descriptor.ExplicitOnly {
				panic("supported encoding must be auto-detectable xor explicit-only: " + descriptor.Name)
			}
			initializeLineEndingBytes(descriptor)
		}
		registerName(descriptor.Name, descriptor)
		for _, alias := range descriptor.Aliases {
			registerName(alias, descriptor)
		}
		for _, label := range descriptor.DetectorLabels {
			registerDetectorLabel(label, descriptor)
		}
		if len(descriptor.BOM) != 0 {
			bomDescriptors = append(bomDescriptors, descriptor)
		}
		if descriptor.Supported {
			registerXTextCanonicalNames(descriptor)
		}
	}
	registerIANAASCIICompatibility()

	for _, label := range rejectedDetectorLabels {
		key := normalizeRegistryName(label)
		if _, exists := descriptorByDetector[key]; exists {
			panic("detector label has both accepted and rejected dispositions: " + label)
		}
		rejectedDetectorLookup[key] = struct{}{}
	}

	// UTF-32 LE shares the UTF-16 LE prefix, so longest signatures must win.
	sort.SliceStable(bomDescriptors, func(i, j int) bool {
		return len(bomDescriptors[i].BOM) > len(bomDescriptors[j].BOM)
	})
}

func normalizeRegistryName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func registerName(name string, descriptor *EncodingDescriptor) {
	key := normalizeRegistryName(name)
	if key == "" {
		panic("empty encoding name for " + descriptor.Name)
	}
	if existing, ok := descriptorByName[key]; ok && existing != descriptor {
		panic("encoding name collision: " + name)
	}
	descriptorByName[key] = descriptor
}

func registerDetectorLabel(label string, descriptor *EncodingDescriptor) {
	key := normalizeRegistryName(label)
	if key == "" {
		panic("empty detector label for " + descriptor.Name)
	}
	if existing, ok := descriptorByDetector[key]; ok && existing != descriptor {
		panic("detector label collision: " + label)
	}
	descriptorByDetector[key] = descriptor
}

func registerXTextCanonicalNames(descriptor *EncodingDescriptor) {
	base := descriptor.xTextEncoding
	if base == nil {
		base = underlyingXTextEncoding(descriptor.Encoding)
	}
	if base == nil {
		return
	}
	if name, err := ianaindex.IANA.Name(base); err == nil && name != "" {
		registerIndexedCanonical(descriptorByIANAName, name, descriptor, "IANA")
	}
	if name, err := htmlindex.Name(base); err == nil && name != "" {
		registerIndexedCanonical(descriptorByHTMLName, name, descriptor, "HTML")
	}
}

func registerIndexedCanonical(index map[string]*EncodingDescriptor, name string, descriptor *EncodingDescriptor, source string) {
	key := normalizeRegistryName(name)
	if existing, ok := index[key]; ok && existing != descriptor {
		panic(source + " canonical encoding collision: " + name)
	}
	index[key] = descriptor
}

func registerIANAASCIICompatibility() {
	descriptor := descriptorByName["utf-8"]
	asciiEncoding, err := ianaindex.IANA.Encoding("US-ASCII")
	if err != nil || asciiEncoding == nil {
		panic("pinned IANA index does not resolve US-ASCII")
	}
	name, err := ianaindex.IANA.Name(asciiEncoding)
	if err != nil || name == "" {
		panic("pinned IANA index does not name US-ASCII")
	}
	registerIndexedCanonical(descriptorByIANAName, name, descriptor, "IANA")
}

func initializeLineEndingBytes(descriptor *EncodingDescriptor) {
	switch descriptor.Validation {
	case ValidationUTF16LE:
		descriptor.LineEndings = LineEndingUTF16LE
	case ValidationUTF16BE:
		descriptor.LineEndings = LineEndingUTF16BE
	case ValidationUTF32LE:
		descriptor.LineEndings = LineEndingUTF32LE
	case ValidationUTF32BE:
		descriptor.LineEndings = LineEndingUTF32BE
	default:
		if descriptor.Name == "hz-gb-2312" {
			descriptor.LineEndings = LineEndingHZ
		} else {
			descriptor.LineEndings = LineEndingByte
		}
	}

	if descriptor.Name == "utf-8" {
		descriptor.lineEndingCR = []byte{'\r'}
		descriptor.lineEndingLF = []byte{'\n'}
		return
	}
	if descriptor.Encoding == nil {
		panic("supported encoding has no encoder: " + descriptor.Name)
	}
	var err error
	descriptor.lineEndingCR, err = descriptor.Encoding.NewEncoder().Bytes([]byte{'\r'})
	if err != nil || len(descriptor.lineEndingCR) == 0 {
		panic("encoding cannot represent carriage return: " + descriptor.Name)
	}
	descriptor.lineEndingLF, err = descriptor.Encoding.NewEncoder().Bytes([]byte{'\n'})
	if err != nil || len(descriptor.lineEndingLF) == 0 {
		panic("encoding cannot represent line feed: " + descriptor.Name)
	}
}

var rejectedIndexedAliases = map[string]struct{}{
	"csunicode":       {},
	"csutf16":         {},
	"iso-10646-ucs-2": {},
	"ucs-2":           {},
	"unicode":         {},
	"unicodefeff":     {},
	"unicodefffe":     {},
	"utf-16":          {},
	"utf-32":          {},
	"csutf32":         {},
}

func resolveIndexedAlias(name string) (*EncodingDescriptor, bool) {
	if _, rejected := rejectedIndexedAliases[normalizeRegistryName(name)]; rejected {
		return nil, false
	}
	if candidate, err := ianaindex.IANA.Encoding(name); err == nil && candidate != nil {
		if canonical, err := ianaindex.IANA.Name(candidate); err == nil {
			if descriptor, ok := descriptorByIANAName[normalizeRegistryName(canonical)]; ok && descriptor.Supported {
				return descriptor, true
			}
		}
	}
	if candidate, err := htmlindex.Get(name); err == nil && candidate != nil {
		if canonical, err := htmlindex.Name(candidate); err == nil {
			if descriptor, ok := descriptorByHTMLName[normalizeRegistryName(canonical)]; ok && descriptor.Supported {
				return descriptor, true
			}
		}
	}
	return nil, false
}

func lookupDescriptor(name string) (*EncodingDescriptor, bool) {
	if descriptor, ok := descriptorByName[normalizeRegistryName(name)]; ok {
		return descriptor, true
	}
	return resolveIndexedAlias(name)
}

// LookupDescriptor returns a defensive copy, including known BOM-only descriptors.
func LookupDescriptor(name string) (EncodingDescriptor, bool) {
	descriptor, ok := lookupDescriptor(name)
	if !ok {
		return EncodingDescriptor{}, false
	}
	copy := *descriptor
	copy.Aliases = append([]string(nil), descriptor.Aliases...)
	copy.DetectorLabels = append([]string(nil), descriptor.DetectorLabels...)
	copy.BOM = append([]byte(nil), descriptor.BOM...)
	copy.lineEndingCR = append([]byte(nil), descriptor.lineEndingCR...)
	copy.lineEndingLF = append([]byte(nil), descriptor.lineEndingLF...)
	return copy, true
}

// Get resolves only supported public text encodings.
func Get(name string) (encoding.Encoding, bool) {
	descriptor, ok := lookupDescriptor(name)
	if !ok || !descriptor.Supported || !descriptor.Readable {
		return nil, false
	}
	return descriptor.Encoding, true
}

// CanonicalName resolves a supported public alias to its stable canonical name.
func CanonicalName(name string) (string, bool) {
	descriptor, ok := lookupDescriptor(name)
	if !ok || !descriptor.Supported {
		return "", false
	}
	return descriptor.Name, true
}

// CanonicalBOMName also resolves BOM-only encodings such as UTF-32.
func CanonicalBOMName(name string) (string, bool) {
	descriptor, ok := lookupDescriptor(name)
	if !ok || len(descriptor.BOM) == 0 {
		return "", false
	}
	return descriptor.Name, true
}

func IsUTF8(name string) bool {
	descriptor, ok := lookupDescriptor(name)
	return ok && descriptor.Supported && descriptor.Name == "utf-8"
}

func IsUTF16(name string) bool {
	descriptor, ok := lookupDescriptor(name)
	if !ok || !descriptor.Supported {
		return false
	}
	return descriptor.Validation == ValidationUTF16LE || descriptor.Validation == ValidationUTF16BE
}

func IsUTF32(name string) bool {
	descriptor, ok := lookupDescriptor(name)
	if !ok || !descriptor.Supported {
		return false
	}
	return descriptor.Validation == ValidationUTF32LE || descriptor.Validation == ValidationUTF32BE
}

func BOMBytesFor(name string) []byte {
	descriptor, ok := lookupDescriptor(name)
	if !ok || len(descriptor.BOM) == 0 {
		return nil
	}
	return append([]byte(nil), descriptor.BOM...)
}

func BOMSize(name string) int {
	descriptor, ok := lookupDescriptor(name)
	if !ok {
		return 0
	}
	return len(descriptor.BOM)
}

// LineEndingProfile describes how raw line endings are represented by a
// supported encoding. Byte slices are caller-owned.
type LineEndingProfile struct {
	Kind           LineEndingKind
	CarriageReturn []byte
	LineFeed       []byte
}

// LineEndingProfileFor returns the registry-owned line-ending semantics for a
// supported encoding.
func LineEndingProfileFor(name string) (LineEndingProfile, bool) {
	descriptor, ok := lookupDescriptor(name)
	if !ok || !descriptor.Supported || descriptor.LineEndings == "" || len(descriptor.lineEndingCR) == 0 || len(descriptor.lineEndingLF) == 0 {
		return LineEndingProfile{}, false
	}
	return LineEndingProfile{
		Kind:           descriptor.LineEndings,
		CarriageReturn: append([]byte(nil), descriptor.lineEndingCR...),
		LineFeed:       append([]byte(nil), descriptor.lineEndingLF...),
	}, true
}

// LineEndingBytes is a compatibility helper for callers that only need the
// encoded CR and LF units.
func LineEndingBytes(name string) (carriageReturn, lineFeed []byte, ok bool) {
	profile, ok := LineEndingProfileFor(name)
	if !ok {
		return nil, nil, false
	}
	return profile.CarriageReturn, profile.LineFeed, true
}

func canonicalDetectedCharset(label string) string {
	descriptor, ok := descriptorByDetector[normalizeRegistryName(label)]
	if !ok || !descriptor.Supported || !descriptor.AutoDetectable || !descriptor.externalDetector {
		return ""
	}
	return descriptor.Name
}

func detectorLabelHasDisposition(label string) bool {
	key := normalizeRegistryName(label)
	if _, ok := descriptorByDetector[key]; ok {
		return true
	}
	_, ok := rejectedDetectorLookup[key]
	return ok
}

type EncodingListItem struct {
	Name           string   `json:"name"`
	DisplayName    string   `json:"displayName"`
	Aliases        []string `json:"aliases"`
	Description    string   `json:"description"`
	Readable       bool     `json:"readable"`
	Writable       bool     `json:"writable"`
	AutoDetectable bool     `json:"autoDetectable"`
	ExplicitOnly   bool     `json:"explicitOnly"`
	Unicode        bool     `json:"unicode"`
	HasBOM         bool     `json:"hasBOM"`
	AutoBOM        bool     `json:"autoBOM"`
}

func ListEncodings() []EncodingListItem {
	var items []EncodingListItem
	for _, descriptor := range encodingDescriptors {
		if !descriptor.Supported {
			continue
		}
		items = append(items, EncodingListItem{
			Name: descriptor.Name, DisplayName: descriptor.DisplayName,
			Aliases: append([]string(nil), descriptor.Aliases...), Description: descriptor.Description,
			Readable: descriptor.Readable, Writable: descriptor.Writable,
			AutoDetectable: descriptor.AutoDetectable, ExplicitOnly: descriptor.ExplicitOnly,
			Unicode: descriptor.Unicode, HasBOM: len(descriptor.BOM) != 0, AutoBOM: descriptor.AutoBOM,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].DisplayName < items[j].DisplayName })
	return items
}
