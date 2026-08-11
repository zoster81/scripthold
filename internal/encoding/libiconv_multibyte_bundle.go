package encoding

import (
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const generatedMultibyteBundleMagic = "SHM6\x01"

type generatedBundleReader struct {
	data   []byte
	offset int
}

func mustDecodeGeneratedLibiconvMultibyteData(encoded string) ([]libiconvMultibyteSpec, []libiconvRawCharsetSpec) {
	compressed := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded))
	zr, err := zlib.NewReader(compressed)
	if err != nil {
		panic("open generated multibyte mapping bundle: " + err.Error())
	}
	data, readErr := io.ReadAll(zr)
	closeErr := zr.Close()
	if readErr != nil {
		panic("read generated multibyte mapping bundle: " + readErr.Error())
	}
	if closeErr != nil {
		panic("close generated multibyte mapping bundle: " + closeErr.Error())
	}
	reader := &generatedBundleReader{data: data}
	if got := string(reader.take(len(generatedMultibyteBundleMagic))); got != generatedMultibyteBundleMagic {
		panic(fmt.Sprintf("invalid generated multibyte mapping bundle magic %q", got))
	}

	specCount := int(reader.u16())
	specs := make([]libiconvMultibyteSpec, specCount)
	for index := range specs {
		spec := &specs[index]
		spec.CanonicalName = reader.string16()
		spec.DisplayName = reader.string16()
		spec.SourceName = reader.string16()
		spec.SourceID = reader.string16()
		spec.SourceDefinition = reader.string16()
		spec.SourceHeaderSHA256 = reader.string16()
		spec.Kind = reader.string16()
		aliasCount := int(reader.u16())
		spec.Aliases = make([]string, aliasCount)
		for aliasIndex := range spec.Aliases {
			spec.Aliases[aliasIndex] = reader.string16()
		}
		spec.Decode = reader.decodeEntries()
		spec.Encode = reader.encodeEntries()
		spec.PairEncode = reader.pairEntries()
	}

	rawCount := int(reader.u16())
	raw := make([]libiconvRawCharsetSpec, rawCount)
	for index := range raw {
		raw[index].Name = reader.string16()
		raw[index].Width = reader.u8()
		raw[index].Decode = reader.decodeEntries()
	}
	if reader.offset != len(reader.data) {
		panic(fmt.Sprintf("generated multibyte mapping bundle has %d trailing bytes", len(reader.data)-reader.offset))
	}
	return specs, raw
}

func (reader *generatedBundleReader) decodeEntries() []multibyteDecodeEntry {
	count := int(reader.u32())
	entries := make([]multibyteDecodeEntry, count)
	for index := range entries {
		entries[index] = multibyteDecodeEntry{
			Packed: reader.u32(),
			Length: reader.u8(),
			Rune1:  rune(reader.u32()),
			Rune2:  rune(reader.u32()),
		}
	}
	return entries
}

func (reader *generatedBundleReader) encodeEntries() []multibyteEncodeEntry {
	count := int(reader.u32())
	entries := make([]multibyteEncodeEntry, count)
	for index := range entries {
		entries[index] = multibyteEncodeEntry{
			Rune:  rune(reader.u32()),
			Bytes: reader.string16(),
		}
	}
	return entries
}

func (reader *generatedBundleReader) pairEntries() []multibytePairEncodeEntry {
	count := int(reader.u32())
	entries := make([]multibytePairEncodeEntry, count)
	for index := range entries {
		entries[index] = multibytePairEncodeEntry{
			First:  rune(reader.u32()),
			Second: rune(reader.u32()),
			Bytes:  reader.string16(),
		}
	}
	return entries
}

func (reader *generatedBundleReader) string16() string {
	length := int(reader.u16())
	return string(reader.take(length))
}

func (reader *generatedBundleReader) u8() uint8 {
	return reader.take(1)[0]
}

func (reader *generatedBundleReader) u16() uint16 {
	data := reader.take(2)
	return uint16(data[0])<<8 | uint16(data[1])
}

func (reader *generatedBundleReader) u32() uint32 {
	data := reader.take(4)
	return uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
}

func (reader *generatedBundleReader) take(length int) []byte {
	if length < 0 || reader.offset > len(reader.data)-length {
		panic("truncated generated multibyte mapping bundle")
	}
	start := reader.offset
	reader.offset += length
	return reader.data[start:reader.offset]
}
