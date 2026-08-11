package encoding

import (
	"bytes"
	"io"
	"testing"
)

type oneByteReader struct {
	data []byte
}

func (r *oneByteReader) Read(buffer []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	buffer[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}

func TestNewDecoderReaderPreservesMultibyteSequencesAcrossReads(t *testing.T) {
	tests := []struct {
		charset string
		text    string
	}{
		{charset: "utf-8", text: "Città Привет 中文 🌍"},
		{charset: "utf-16-le", text: "Città Привет 中文 🌍"},
		{charset: "utf-16-be", text: "Città Привет 中文 🌍"},
		{charset: "utf-32-le", text: "Città Привет 中文 🌍"},
		{charset: "utf-32-be", text: "Città Привет 中文 🌍"},
		{charset: "gb18030", text: "中文 🌍"},
		{charset: "euc-jp", text: "日本語"},
		{charset: "iso-2022-jp", text: "日本語"},
		{charset: "shift_jis", text: "日本語"},
		{charset: "euc-kr", text: "한국어"},
		{charset: "hz-gb-2312", text: "中文"},
		{charset: "big5", text: "中文"},
	}

	for _, testCase := range tests {
		t.Run(testCase.charset, func(t *testing.T) {
			encoded := []byte(testCase.text)
			if !IsUTF8(testCase.charset) {
				registered, ok := Get(testCase.charset)
				if !ok {
					t.Fatalf("encoding %q is not registered", testCase.charset)
				}
				var err error
				encoded, err = registered.NewEncoder().Bytes(encoded)
				if err != nil {
					t.Fatal(err)
				}
			}

			reader, err := NewDecoderReader(&oneByteReader{data: append([]byte(nil), encoded...)}, testCase.charset)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decoded, []byte(testCase.text)) {
				t.Fatalf("decoded = %q, want %q", decoded, testCase.text)
			}
		})
	}
}

func TestPhase3LegacyDecodersRejectMalformedInput(t *testing.T) {
	tests := []struct {
		charset string
		data    []byte
	}{
		{charset: "utf-8", data: []byte{0xc3, 0x28}},
		{charset: "windows-1252", data: []byte{0x81}},
		{charset: "iso-8859-3", data: []byte{0xa5}},
		{charset: "shift_jis", data: []byte{0x82}},
		{charset: "euc-jp", data: []byte{0xa4}},
		{charset: "iso-2022-jp", data: []byte{0x1b, 0x24}},
		{charset: "euc-kr", data: []byte{0x81}},
		{charset: "big5", data: []byte{0x81}},
		{charset: "gb18030", data: []byte{0x81}},
		{charset: "gb18030", data: []byte{0x81, 0x30, 0x81, 0x2f}},
		{charset: "utf-16-le", data: []byte{0x41}},
		{charset: "utf-16-le", data: []byte{0x3d, 0xd8}},
		{charset: "utf-16-be", data: []byte{0xd8, 0x3d}},
		{charset: "utf-32-le", data: []byte{0x41, 0x00, 0x00}},
		{charset: "utf-32-le", data: []byte{0x00, 0xd8, 0x00, 0x00}},
		{charset: "utf-32-le", data: []byte{0x00, 0x00, 0x11, 0x00}},
		{charset: "utf-32-be", data: []byte{0x00, 0x00, 0xd8, 0x00}},
		{charset: "utf-32-be", data: []byte{0x00, 0x11, 0x00, 0x00}},
	}
	for _, testCase := range tests {
		t.Run(testCase.charset, func(t *testing.T) {
			reader, err := NewDecoderReader(bytes.NewReader(testCase.data), testCase.charset)
			if err != nil {
				t.Fatalf("construct decoder: %v", err)
			}
			if _, err := io.ReadAll(reader); err == nil {
				t.Fatal("malformed input decoded without an error")
			}
		})
	}
}

func TestPhase3GB18030AllowsLegitimateReplacementRune(t *testing.T) {
	registered, ok := Get("gb18030")
	if !ok {
		t.Fatal("gb18030 is not registered")
	}
	encoded, err := registered.NewEncoder().Bytes([]byte("\ufffd"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewDecoderReader(bytes.NewReader(encoded), "gb18030")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != "\ufffd" {
		t.Fatalf("decoded = %q, want U+FFFD", decoded)
	}
}

func TestNewDecoderReaderRejectsUnsupportedEncoding(t *testing.T) {
	if _, err := NewDecoderReader(bytes.NewReader(nil), "not-an-encoding"); err == nil {
		t.Fatal("expected unsupported encoding error")
	}
}

func FuzzDecoderReaderChunkInvariance(f *testing.F) {
	charsets := []string{
		"utf-8",
		"utf-16-le",
		"utf-16-be",
		"utf-32-le",
		"utf-32-be",
		"windows-1251",
		"windows-1252",
		"gbk",
		"gb18030",
		"ibm737",
		"ibm856",
		"euc-tw",
		"johab",
		"iso-2022-jp-3",
		"iso-2022-cn-ext",
		"euc-cn",
		"gb18030-2022",
		"tcvn",
	}
	for _, seed := range []struct {
		selector byte
		data     []byte
	}{
		{selector: 0, data: []byte("Città Привет 中文 🌍")},
		{selector: 1, data: []byte{'A', 0, 'B', 0}},
		{selector: 2, data: []byte{0, 'A', 0, 'B'}},
		{selector: 5, data: []byte{0xd6, 0xd0, 0xce, 0xc4}},
		{selector: 6, data: []byte{0x94, 0x39, 0xfc, 0x36}},
		{selector: 9, data: []byte{0x80, 0x81}},
		{selector: 10, data: []byte{0x9b}},
		{selector: 11, data: []byte{0x8e, 0xa2, 0xa1, 0xa1}},
		{selector: 12, data: []byte("JOHAB ASCII boundary")},
		{selector: 13, data: []byte{0x1b, '$', '(', 'Q', 0x24, 0x77, 0x1b, '(', 'B'}},
		{selector: 14, data: []byte{0x1b, '$', ')', 'A', 0x0e, 0x21, 0x21, 0x0f}},
		{selector: 15, data: []byte{0xd6, 0xd0}},
		{selector: 16, data: []byte{0xfe, 0x51}},
		{selector: 17, data: []byte{0x41, 0xb3}},
		{selector: 13, data: []byte{0x1b, '$'}},
		{selector: 1, data: []byte{0xd8}},
	} {
		f.Add(seed.selector, seed.data)
	}

	f.Fuzz(func(t *testing.T, selector byte, data []byte) {
		if len(data) > 64*1024 {
			t.Skip()
		}
		charset := charsets[int(selector)%len(charsets)]

		directReader, err := NewDecoderReader(bytes.NewReader(data), charset)
		if err != nil {
			t.Fatalf("construct direct decoder for %q: %v", charset, err)
		}
		direct, directErr := io.ReadAll(directReader)

		chunkedReader, err := NewDecoderReader(&oneByteReader{data: append([]byte(nil), data...)}, charset)
		if err != nil {
			t.Fatalf("construct chunked decoder for %q: %v", charset, err)
		}
		chunked, chunkedErr := io.ReadAll(chunkedReader)

		if (directErr == nil) != (chunkedErr == nil) {
			t.Fatalf("decoder error differs by chunking for %q: direct=%v chunked=%v", charset, directErr, chunkedErr)
		}
		if !bytes.Equal(direct, chunked) {
			t.Fatalf("decoder output differs by chunking for %q: %x != %x", charset, direct, chunked)
		}
		if directErr == nil && len(direct) > len(data)*4+4 {
			t.Fatalf("decoder output expansion is unexpectedly large for %q: input=%d output=%d", charset, len(data), len(direct))
		}
	})
}
