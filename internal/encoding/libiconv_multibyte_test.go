package encoding

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/transform"
)

var phase6CanonicalNames = []string{
	"big5-2003",
	"big5-hkscs-1999",
	"big5-hkscs-2001",
	"big5-hkscs-2004",
	"big5-hkscs-2008",
	"euc-cn",
	"euc-jisx0213",
	"euc-tw",
	"gb18030-2022",
	"ibm1162",
	"ibm1163",
	"iso-2022-cn",
	"iso-2022-cn-ext",
	"iso-2022-jp-1",
	"iso-2022-jp-2",
	"iso-2022-jp-3",
	"iso-2022-jp-ms",
	"iso-2022-kr",
	"johab",
	"shift_jisx0213",
	"tcvn",
}

func TestPhase6SelectedMultibyteStatefulEncodingsAreRegistered(t *testing.T) {
	if len(phase6CanonicalNames) != 21 {
		t.Fatalf("phase-6 canonical set = %d, want 21", len(phase6CanonicalNames))
	}
	if !sort.StringsAreSorted(phase6CanonicalNames) {
		t.Fatal("phase-6 canonical names are not sorted")
	}
	if slices.Contains(phase6CanonicalNames, "big5-hkscs") {
		t.Fatal("unversioned big5-hkscs must retain the existing x/text-owned alias semantics")
	}

	for _, name := range phase6CanonicalNames {
		descriptor, ok := LookupDescriptor(name)
		if !ok {
			t.Errorf("LookupDescriptor(%q) failed", name)
			continue
		}
		if !descriptor.Supported || !descriptor.Readable || !descriptor.Writable {
			t.Errorf("%q lacks read/write capability: %+v", name, descriptor)
		}
		if name == "iso-2022-kr" {
			if !descriptor.AutoDetectable || descriptor.ExplicitOnly {
				t.Errorf("%q must be auto-detectable after phase-7 signature hardening: %+v", name, descriptor)
			}
		} else if descriptor.AutoDetectable || !descriptor.ExplicitOnly {
			t.Errorf("%q must remain explicit-only without phase-7 detection evidence: %+v", name, descriptor)
		}
		if descriptor.Unicode || len(descriptor.BOM) != 0 || descriptor.AutoBOM {
			t.Errorf("%q has unexpected Unicode/BOM metadata: %+v", name, descriptor)
		}
		if registered, ok := Get(name); !ok || registered == nil {
			t.Errorf("Get(%q) failed", name)
		}
	}
}

func TestPhase6RepresentativeAliases(t *testing.T) {
	tests := map[string]string{
		"EUC-CN":           "euc-cn",
		"EUCCN":            "euc-cn",
		"EUCTW":            "euc-tw",
		"csEUCTW":          "euc-tw",
		"CP1162":           "ibm1162",
		"CP1163":           "ibm1163",
		"CP1361":           "johab",
		"BIG5-HKSCS:1999":  "big5-hkscs-1999",
		"BIG5-HKSCS:2008":  "big5-hkscs-2008",
		"EUC-JIS-2004":     "euc-jisx0213",
		"SHIFT_JIS-2004":   "shift_jisx0213",
		"ISO-2022-JP-2004": "iso-2022-jp-3",
		"CP50221":          "iso-2022-jp-ms",
		"csISO2022JP2":     "iso-2022-jp-2",
		"csISO2022CN":      "iso-2022-cn",
		"csISO2022KR":      "iso-2022-kr",
		"GB18030:2022":     "gb18030-2022",
		"TCVN-5712":        "tcvn",
	}
	for alias, want := range tests {
		got, ok := CanonicalName(alias)
		if !ok || got != want {
			t.Errorf("CanonicalName(%q) = %q, %v; want %q, true", alias, got, ok, want)
		}
	}
}

func TestPhase6DoesNotReplaceExistingXTextCompatibilityAliases(t *testing.T) {
	tests := map[string]string{
		"big5-hkscs": "big5",
		"cp932":      "shift_jis",
		"cp936":      "gbk",
		"cp949":      "euc-kr",
		"cp950":      "big5",
		"cp943":      "shift_jis",
		"gb2312":     "gbk",
		"csgb2312":   "gbk",
		"hz":         "hz-gb-2312",
	}
	for alias, want := range tests {
		got, ok := CanonicalName(alias)
		if !ok || got != want {
			t.Errorf("CanonicalName(%q) = %q, %v; want preserved %q, true", alias, got, ok, want)
		}
	}
}

func TestPhase6GeneratedBundleMetadata(t *testing.T) {
	if generatedLibiconvMultibyteRevision != "9d19c66d0a1768cffcf497b2db70bf4018b578d7" {
		t.Fatalf("generated revision = %q", generatedLibiconvMultibyteRevision)
	}
	if len(generatedLibiconvMultibyteSpecs) != 20 {
		t.Fatalf("generated specs = %d, want 20", len(generatedLibiconvMultibyteSpecs))
	}
	if len(generatedLibiconvRawCharsetSpecs) != 20 {
		t.Fatalf("generated raw charsets = %d, want 20", len(generatedLibiconvRawCharsetSpecs))
	}
	generatedNames := make([]string, 0, len(generatedLibiconvMultibyteSpecs))
	for index := range generatedLibiconvMultibyteSpecs {
		spec := &generatedLibiconvMultibyteSpecs[index]
		generatedNames = append(generatedNames, spec.CanonicalName)
		if len(spec.SourceHeaderSHA256) != 64 {
			t.Fatalf("%s header SHA-256 length = %d", spec.CanonicalName, len(spec.SourceHeaderSHA256))
		}
		if _, err := hex.DecodeString(spec.SourceHeaderSHA256); err != nil {
			t.Fatalf("%s invalid header SHA-256: %v", spec.CanonicalName, err)
		}
		if spec.SourceName == "" || spec.SourceID == "" || spec.SourceDefinition == "" || spec.Kind == "" {
			t.Fatalf("%s missing generated provenance: %+v", spec.CanonicalName, spec)
		}
		if len(spec.Encode) == 0 {
			t.Fatalf("%s generated no canonical encode mappings", spec.CanonicalName)
		}
		if (spec.Kind == "direct" || spec.Kind == "tcvn") && len(spec.Decode) == 0 {
			t.Fatalf("%s generated no decode mappings", spec.CanonicalName)
		}
		if strings.HasPrefix(spec.Kind, "iso2022-") && len(spec.Decode) != 0 {
			t.Fatalf("%s unexpectedly embeds ISO-2022 full-state decode mappings", spec.CanonicalName)
		}
	}
	wantGeneratedNames := make([]string, 0, len(phase6CanonicalNames)-1)
	for _, name := range phase6CanonicalNames {
		if name != "gb18030-2022" {
			wantGeneratedNames = append(wantGeneratedNames, name)
		}
	}
	if !slices.Equal(generatedNames, wantGeneratedNames) {
		t.Fatalf("generated canonical set = %v, want %v", generatedNames, wantGeneratedNames)
	}
}

func TestPhase6GB180302022DifferentialPatches(t *testing.T) {
	if len(generatedGB180302022HeaderSHA256) != 64 {
		t.Fatalf("GB18030:2022 header SHA-256 length = %d", len(generatedGB180302022HeaderSHA256))
	}
	if _, err := hex.DecodeString(generatedGB180302022HeaderSHA256); err != nil {
		t.Fatalf("GB18030:2022 invalid header SHA-256: %v", err)
	}
	if got := len(generatedGB180302022DecodePatches); got != 2087 {
		t.Fatalf("GB18030:2022 decode patches = %d, want 2087", got)
	}
	if got := len(generatedGB180302022EncodePatches); got != 2087 {
		t.Fatalf("GB18030:2022 encode patches = %d, want 2087", got)
	}

	registered, ok := Get("gb18030-2022")
	if !ok {
		t.Fatal("gb18030-2022 is not registered")
	}
	decoded, err := registered.NewDecoder().Bytes([]byte{0xFE, 0x51})
	if err != nil || string(decoded) != "\uE816" {
		t.Fatalf("GB18030:2022 FE51 decode = %q, %v; want U+E816", decoded, err)
	}
	encoded, err := registered.NewEncoder().Bytes([]byte("\uE816"))
	if err != nil || !bytes.Equal(encoded, []byte{0xFE, 0x51}) {
		t.Fatalf("GB18030:2022 U+E816 encode = %x, %v; want FE51", encoded, err)
	}

	text := "\uE816🌍\n"
	encodedReader, err := NewEncoderReader(&oneByteReader{data: []byte(text)}, "gb18030-2022")
	if err != nil {
		t.Fatal(err)
	}
	chunkedEncoded, err := io.ReadAll(encodedReader)
	if err != nil {
		t.Fatalf("GB18030:2022 one-byte source encoding: %v", err)
	}
	decodedReader, err := NewDecoderReader(&oneByteReader{data: chunkedEncoded}, "gb18030-2022")
	if err != nil {
		t.Fatal(err)
	}
	chunkedDecoded, err := io.ReadAll(decodedReader)
	if err != nil || string(chunkedDecoded) != text {
		t.Fatalf("GB18030:2022 one-byte chunk round-trip = %q, %v; want %q", chunkedDecoded, err, text)
	}
}

func TestPhase10GB180302022TransformerAllocationsDoNotScaleWithInput(t *testing.T) {
	registered, ok := Get("gb18030-2022")
	if !ok || registered == nil {
		t.Fatal("gb18030-2022 is not registered")
	}

	measure := func(size int) (decoderAllocs, encoderAllocs float64) {
		source := bytes.Repeat([]byte{'A'}, size)
		decoded := make([]byte, size)
		encoded := make([]byte, size*4)
		decoder := registered.NewDecoder()
		encoder := registered.NewEncoder()

		decoderAllocs = testing.AllocsPerRun(3, func() {
			decoder.Reset()
			nDst, nSrc, err := decoder.Transform(decoded, source, true)
			if err != nil || nDst != size || nSrc != size {
				t.Fatalf("decode size=%d nDst=%d nSrc=%d err=%v", size, nDst, nSrc, err)
			}
		})
		encoderAllocs = testing.AllocsPerRun(3, func() {
			encoder.Reset()
			nDst, nSrc, err := encoder.Transform(encoded, source, true)
			if err != nil || nDst != size || nSrc != size {
				t.Fatalf("encode size=%d nDst=%d nSrc=%d err=%v", size, nDst, nSrc, err)
			}
		})
		return decoderAllocs, encoderAllocs
	}

	smallDecoder, smallEncoder := measure(1 << 10)
	largeDecoder, largeEncoder := measure(64 << 10)
	if largeDecoder > smallDecoder+8 {
		t.Fatalf("decoder allocations scale with input: 1KiB=%.0f 64KiB=%.0f", smallDecoder, largeDecoder)
	}
	if largeEncoder > smallEncoder+8 {
		t.Fatalf("encoder allocations scale with input: 1KiB=%.0f 64KiB=%.0f", smallEncoder, largeEncoder)
	}
}

func TestPhase6GeneratedMappingsAreSemanticallyClosed(t *testing.T) {
	for index := range generatedLibiconvMultibyteSpecs {
		spec := &generatedLibiconvMultibyteSpecs[index]
		registered, ok := Get(spec.CanonicalName)
		if !ok || registered == nil {
			t.Fatalf("Get(%q) failed", spec.CanonicalName)
		}
		decoder := registered.NewDecoder()
		encoder := registered.NewEncoder()

		for _, entry := range spec.Decode {
			decoder.Reset()
			source := unpackPhase6Sequence(entry.Packed, entry.Length)
			var dst [8]byte
			nDst, nSrc, err := decoder.Transform(dst[:], source, true)
			if err != nil {
				t.Fatalf("%s decode %x: %v", spec.CanonicalName, source, err)
			}
			if nSrc != len(source) {
				t.Fatalf("%s decode %x consumed %d/%d bytes", spec.CanonicalName, source, nSrc, len(source))
			}
			want := phase6UTF8(entry.Rune1, entry.Rune2)
			if !bytes.Equal(dst[:nDst], want) {
				t.Fatalf("%s decode %x = %x, want %x", spec.CanonicalName, source, dst[:nDst], want)
			}
		}

		for _, entry := range spec.Encode {
			source := []byte(string(entry.Rune))
			encoder.Reset()
			var encoded [32]byte
			nDst, nSrc, err := encoder.Transform(encoded[:], source, true)
			if err != nil {
				t.Fatalf("%s encode U+%04X: %v", spec.CanonicalName, entry.Rune, err)
			}
			if nSrc != len(source) || string(encoded[:nDst]) != entry.Bytes {
				t.Fatalf("%s encode U+%04X = %x consumed=%d, want %x consumed=%d", spec.CanonicalName, entry.Rune, encoded[:nDst], nSrc, []byte(entry.Bytes), len(source))
			}

			decoder.Reset()
			var decoded [8]byte
			nDecoded, nEncoded, err := decoder.Transform(decoded[:], []byte(entry.Bytes), true)
			if err != nil {
				t.Fatalf("%s canonical decode U+%04X bytes %x: %v", spec.CanonicalName, entry.Rune, []byte(entry.Bytes), err)
			}
			if nEncoded != len(entry.Bytes) || string(decoded[:nDecoded]) != string(entry.Rune) {
				t.Fatalf("%s canonical bytes %x decode to %q consumed=%d, want %q consumed=%d", spec.CanonicalName, []byte(entry.Bytes), decoded[:nDecoded], nEncoded, string(entry.Rune), len(entry.Bytes))
			}
		}

		for _, entry := range spec.PairEncode {
			source := []byte(string([]rune{entry.First, entry.Second}))
			encoder.Reset()
			var encoded [32]byte
			nDst, nSrc, err := encoder.Transform(encoded[:], source, true)
			if err != nil {
				t.Fatalf("%s pair U+%04X U+%04X encode: %v", spec.CanonicalName, entry.First, entry.Second, err)
			}
			if nSrc != len(source) || string(encoded[:nDst]) != entry.Bytes {
				t.Fatalf("%s pair U+%04X U+%04X = %x, want %x", spec.CanonicalName, entry.First, entry.Second, encoded[:nDst], []byte(entry.Bytes))
			}
			decoder.Reset()
			var decoded [8]byte
			nDecoded, nEncoded, err := decoder.Transform(decoded[:], []byte(entry.Bytes), true)
			if err != nil {
				t.Fatalf("%s pair bytes %x decode: %v", spec.CanonicalName, []byte(entry.Bytes), err)
			}
			if nEncoded != len(entry.Bytes) || string(decoded[:nDecoded]) != string([]rune{entry.First, entry.Second}) {
				t.Fatalf("%s pair bytes %x decode to %q, want %q", spec.CanonicalName, []byte(entry.Bytes), decoded[:nDecoded], string([]rune{entry.First, entry.Second}))
			}
		}
	}
}

func TestPhase6DirectCodecsRejectTruncatedSequences(t *testing.T) {
	for index := range generatedLibiconvMultibyteSpecs {
		spec := &generatedLibiconvMultibyteSpecs[index]
		if spec.Kind != "direct" {
			continue
		}
		var selected multibyteDecodeEntry
		for _, entry := range spec.Decode {
			if entry.Length > 1 {
				selected = entry
				break
			}
		}
		if selected.Length <= 1 {
			continue
		}
		registered, _ := Get(spec.CanonicalName)
		prefix := unpackPhase6Sequence(selected.Packed, selected.Length-1)
		decoder := registered.NewDecoder()
		var dst [8]byte
		if nDst, nSrc, err := decoder.Transform(dst[:], prefix, false); err != transform.ErrShortSrc || nDst != 0 || nSrc != 0 {
			t.Fatalf("%s non-EOF truncated %x = nDst=%d nSrc=%d err=%v", spec.CanonicalName, prefix, nDst, nSrc, err)
		}
		decoder.Reset()
		if _, _, err := decoder.Transform(dst[:], prefix, true); !errors.Is(err, ErrInvalidEncodedSequence) {
			t.Fatalf("%s EOF truncated %x error = %v", spec.CanonicalName, prefix, err)
		}
	}
}

func TestPhase6ISO2022RejectsTruncatedEscapeAndZeroByteTags(t *testing.T) {
	for index := range generatedLibiconvMultibyteSpecs {
		spec := &generatedLibiconvMultibyteSpecs[index]
		if !strings.HasPrefix(spec.Kind, "iso2022-") {
			continue
		}
		registered, _ := Get(spec.CanonicalName)
		decoder := registered.NewDecoder()
		var dst [8]byte
		if nDst, nSrc, err := decoder.Transform(dst[:], []byte{iso2022ESC}, false); err != transform.ErrShortSrc || nDst != 0 || nSrc != 0 {
			t.Fatalf("%s partial ESC = nDst=%d nSrc=%d err=%v", spec.CanonicalName, nDst, nSrc, err)
		}
		decoder.Reset()
		if _, _, err := decoder.Transform(dst[:], []byte{iso2022ESC}, true); !errors.Is(err, ErrInvalidEncodedSequence) {
			t.Fatalf("%s EOF ESC error = %v", spec.CanonicalName, err)
		}
	}

	registered, _ := Get("iso-2022-jp-2")
	for _, r := range []rune{0xE0001, 0xE0020, 0xE007F} {
		if _, err := registered.NewEncoder().Bytes([]byte(string(r))); err == nil {
			t.Fatalf("ISO-2022-JP-2 Unicode tag U+%04X was silently accepted", r)
		}
	}
}

func TestPhase6StreamingAcrossOneByteChunks(t *testing.T) {
	for index := range generatedLibiconvMultibyteSpecs {
		spec := &generatedLibiconvMultibyteSpecs[index]
		var representative multibyteEncodeEntry
		for _, entry := range spec.Encode {
			if entry.Rune > 0x7f {
				representative = entry
				break
			}
		}
		if representative.Bytes == "" {
			t.Fatalf("%s has no non-ASCII representative", spec.CanonicalName)
		}
		text := string(representative.Rune) + "\n"
		encodedReader, err := NewEncoderReader(&oneByteReader{data: []byte(text)}, spec.CanonicalName)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := io.ReadAll(encodedReader)
		if err != nil {
			t.Fatalf("%s one-byte source encoding: %v", spec.CanonicalName, err)
		}
		decodedReader, err := NewDecoderReader(&oneByteReader{data: append([]byte(nil), encoded...)}, spec.CanonicalName)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := io.ReadAll(decodedReader)
		if err != nil {
			t.Fatalf("%s one-byte encoded decoding: %v", spec.CanonicalName, err)
		}
		if string(decoded) != text {
			t.Fatalf("%s one-byte chunk round-trip = %q, want %q", spec.CanonicalName, decoded, text)
		}
	}
}

func TestPhase6PairEncoderWaitsAcrossChunks(t *testing.T) {
	for index := range generatedLibiconvMultibyteSpecs {
		spec := &generatedLibiconvMultibyteSpecs[index]
		if len(spec.PairEncode) == 0 {
			continue
		}
		pair := spec.PairEncode[0]
		registered, _ := Get(spec.CanonicalName)
		encoder := registered.NewEncoder()
		lead := []byte(string(pair.First))
		var dst [32]byte
		if nDst, nSrc, err := encoder.Transform(dst[:], lead, false); err != transform.ErrShortSrc || nDst != 0 || nSrc != 0 {
			t.Fatalf("%s pair lead U+%04X = nDst=%d nSrc=%d err=%v", spec.CanonicalName, pair.First, nDst, nSrc, err)
		}
		encoder.Reset()
		full := []byte(string([]rune{pair.First, pair.Second}))
		nDst, nSrc, err := encoder.Transform(dst[:], full, true)
		if err != nil || nSrc != len(full) || string(dst[:nDst]) != pair.Bytes {
			t.Fatalf("%s pair full encode = %x consumed=%d err=%v, want %x", spec.CanonicalName, dst[:nDst], nSrc, err, []byte(pair.Bytes))
		}
	}
}

func unpackPhase6Sequence(packed uint32, length uint8) []byte {
	result := make([]byte, length)
	for index := range result {
		result[index] = byte(packed >> uint(24-8*index))
	}
	return result
}

func phase6UTF8(first, second rune) []byte {
	var buffer [8]byte
	length := utf8.EncodeRune(buffer[:], first)
	if second != 0 {
		length += utf8.EncodeRune(buffer[length:], second)
	}
	return append([]byte(nil), buffer[:length]...)
}
