package encoding

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func encodeUTF32Fixture(t *testing.T, charset, content string) []byte {
	t.Helper()
	enc, ok := Get(charset)
	if !ok {
		t.Fatalf("encoding %q is not registered", charset)
	}
	data, err := enc.NewEncoder().Bytes([]byte(content))
	if err != nil {
		t.Fatalf("encode %s fixture: %v", charset, err)
	}
	return data
}

func appendUTF32Unit(data []byte, order binary.ByteOrder, unit uint32) []byte {
	var encoded [4]byte
	order.PutUint32(encoded[:], unit)
	return append(data, encoded[:]...)
}

func TestPhase4DetectBOMlessUTF32IsContentBased(t *testing.T) {
	content := "title = UTF-32 acceptance\r\nLatin: Città\r\nCyrillic: Привет\r\nCJK: 中文\r\nEmoji: 🌍\r\n"
	for _, charset := range []string{"utf-32-le", "utf-32-be"} {
		t.Run(charset, func(t *testing.T) {
			data := encodeUTF32Fixture(t, charset, content)
			direct := Detect(data)
			if direct.Charset != charset || direct.Confidence < HighConfidenceThreshold {
				t.Fatalf("Detect = %+v, want %s with confidence >= %d", direct, charset, HighConfidenceThreshold)
			}

			root := t.TempDir()
			for _, name := range []string{"sample.txt", "sample.dat", "extensionless", "sample.random"} {
				path := filepath.Join(root, name)
				if err := os.WriteFile(path, data, 0644); err != nil {
					t.Fatal(err)
				}
				for _, mode := range []string{"sample", "chunked", "full"} {
					result, err := DetectFromFile(path, mode)
					if err != nil {
						t.Fatalf("%s/%s: %v", name, mode, err)
					}
					if result.Charset != charset || result.Confidence < HighConfidenceThreshold {
						t.Fatalf("%s/%s result = %+v, want %s", name, mode, result, charset)
					}
				}
			}
		})
	}
}

func TestPhase4DetectBOMlessUTF32RejectsMalformedScalars(t *testing.T) {
	for _, testCase := range []struct {
		charset string
		order   binary.ByteOrder
	}{
		{charset: "utf-32-le", order: binary.LittleEndian},
		{charset: "utf-32-be", order: binary.BigEndian},
	} {
		t.Run(testCase.charset, func(t *testing.T) {
			prefix := encodeUTF32Fixture(t, testCase.charset, "valid text\n")
			malformed := [][]byte{
				append(append([]byte(nil), prefix...), 0x41),
				appendUTF32Unit(append([]byte(nil), prefix...), testCase.order, 0xD800),
				appendUTF32Unit(append([]byte(nil), prefix...), testCase.order, 0x110000),
			}
			for index, data := range malformed {
				result := Detect(data)
				if result.Charset == testCase.charset {
					t.Fatalf("malformed case %d classified as %+v", index, result)
				}
			}
		})
	}
}

func TestPhase4DetectBOMlessUTF32RejectsBinaryAndShortAmbiguity(t *testing.T) {
	randomData := make([]byte, 1024)
	rand.New(rand.NewSource(84)).Read(randomData)
	tests := [][]byte{
		encodeUTF32Fixture(t, "utf-32-le", "Hi"),
		encodeUTF32Fixture(t, "utf-32-be", "Hi"),
		bytes.Repeat([]byte{0, 0, 0, 1}, 32),
		randomData,
	}
	for index, data := range tests {
		result := Detect(data)
		if result.Charset == "utf-32-le" || result.Charset == "utf-32-be" {
			t.Fatalf("ambiguous/binary case %d classified as %+v", index, result)
		}
	}
}

func TestPhase4DetectUTF32BOMRemainsAuthoritativeOverUTF16Prefix(t *testing.T) {
	data := append(BOMBytesFor("utf-32-le"), encodeUTF32Fixture(t, "utf-32-le", "A")...)
	result := Detect(data)
	if result.Charset != "utf-32-le" || !result.HasBOM || result.Confidence != 100 {
		t.Fatalf("Detect = %+v, want authoritative UTF-32 LE BOM", result)
	}
}

func TestPhase4UTF32DetectionDoesNotOverrideUTF16(t *testing.T) {
	content := "ordinary UTF-16 text with enough printable characters\r\nsecond line\r\n"
	for _, charset := range []string{"utf-16-le", "utf-16-be"} {
		data := encodeUTF16Fixture(t, charset, content)
		result := Detect(data)
		if result.Charset != charset {
			t.Fatalf("Detect(%s) = %+v, want %s", charset, result, charset)
		}
	}
}

func FuzzDetectUTF32Validation(f *testing.F) {
	for _, seed := range [][]byte{
		encodeUTF32FixtureForFuzz("utf-32-le", "fuzz UTF-32 text\n"),
		encodeUTF32FixtureForFuzz("utf-32-be", "中文 UTF-32\n"),
		{0xff, 0xfe, 0x00, 0x00, 0x00, 0xd8, 0x00, 0x00},
		{0x00, 0x00, 0xfe, 0xff, 0x00, 0x11, 0x00, 0x00},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		result := Detect(data)
		if result.Charset != "utf-32-le" && result.Charset != "utf-32-be" {
			return
		}
		// A BOM identifies byte order authoritatively even when the following
		// payload is malformed; strict payload validation happens in the decoder.
		// The detector/decoder closure property applies to BOMless classification.
		if result.HasBOM {
			bomResult, found := DetectBOM(data)
			if !found || bomResult.Charset != result.Charset {
				t.Fatalf("inconsistent UTF-32 BOM result: detect=%+v bom=%+v found=%v", result, bomResult, found)
			}
			return
		}
		registered, ok := Get(result.Charset)
		if !ok {
			t.Fatalf("detector exposed unregistered UTF-32 result %+v", result)
		}
		if _, err := registered.NewDecoder().Bytes(data); err != nil {
			t.Fatalf("BOMless detector accepted malformed UTF-32 input: result=%+v data=%x err=%v", result, data, err)
		}
	})
}

func encodeUTF32FixtureForFuzz(charset, content string) []byte {
	registered, ok := Get(charset)
	if !ok {
		panic("UTF-32 fuzz codec is not registered: " + charset)
	}
	encoded, err := registered.NewEncoder().Bytes([]byte(content))
	if err != nil {
		panic(err)
	}
	return encoded
}

func TestPhase4DetectFromFileBOMlessUTF32AcrossChunkBoundary(t *testing.T) {
	content := bytes.Repeat([]byte("A"), ChunkSize/2+257)
	for _, charset := range []string{"utf-32-le", "utf-32-be"} {
		t.Run(charset, func(t *testing.T) {
			data := encodeUTF32Fixture(t, charset, string(content))
			path := filepath.Join(t.TempDir(), "boundary.data")
			if err := os.WriteFile(path, data, 0644); err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{"sample", "chunked", "full"} {
				result, err := DetectFromFile(path, mode)
				if err != nil {
					t.Fatalf("%s: %v", mode, err)
				}
				if result.Charset != charset || result.Confidence < HighConfidenceThreshold {
					t.Fatalf("%s result = %+v, want %s", mode, result, charset)
				}
			}
		})
	}
}
