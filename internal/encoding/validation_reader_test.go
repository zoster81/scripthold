package encoding

import (
	"bytes"
	"io"
	"testing"
)

func TestValidatingReaderPreservesValidEncodedBytes(t *testing.T) {
	tests := []struct {
		charset string
		text    string
	}{
		{charset: "utf-8", text: "Text \ufffd 中文"},
		{charset: "utf-16-le", text: "Text 😀"},
		{charset: "utf-32-le", text: "Text 😀"},
		{charset: "utf-32-be", text: "Text 中文"},
		{charset: "windows-1252", text: "café"},
		{charset: "shift_jis", text: "日本語"},
		{charset: "euc-kr", text: "한국어"},
		{charset: "hz-gb-2312", text: "中文"},
		{charset: "gb18030", text: "Text \ufffd 😀"},
	}

	for _, testCase := range tests {
		t.Run(testCase.charset, func(t *testing.T) {
			var encoded []byte
			if IsUTF8(testCase.charset) {
				encoded = []byte(testCase.text)
			} else {
				registered, ok := Get(testCase.charset)
				if !ok {
					t.Fatalf("encoding %q is not registered", testCase.charset)
				}
				var err error
				encoded, err = registered.NewEncoder().Bytes([]byte(testCase.text))
				if err != nil {
					t.Fatal(err)
				}
			}

			reader, err := NewValidatingReader(&oneByteReader{data: append([]byte(nil), encoded...)}, testCase.charset)
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, encoded) {
				t.Fatalf("validating reader changed bytes: got %x want %x", got, encoded)
			}
		})
	}
}

func TestValidatingReaderRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		charset string
		data    []byte
	}{
		{charset: "utf-8", data: []byte{0xf0, 0x9f, 0x98}},
		{charset: "utf-16-le", data: []byte{0x3d, 0xd8}},
		{charset: "utf-32-le", data: []byte{0x00, 0xd8, 0x00, 0x00}},
		{charset: "utf-32-be", data: []byte{0x00, 0x11, 0x00, 0x00}},
		{charset: "windows-1252", data: []byte{0x81}},
		{charset: "shift_jis", data: []byte{0x82}},
		{charset: "euc-jp", data: []byte{0xa4}},
		{charset: "iso-2022-jp", data: []byte{0x1b, 0x24}},
		{charset: "euc-kr", data: []byte{0x81}},
		{charset: "big5", data: []byte{0x81}},
		{charset: "gb18030", data: []byte{0x81, 0x30, 0x81, 0x2f}},
		{charset: "hz-gb-2312", data: []byte{'~'}},
	}

	for _, testCase := range tests {
		t.Run(testCase.charset, func(t *testing.T) {
			reader, err := NewValidatingReader(&oneByteReader{data: append([]byte(nil), testCase.data...)}, testCase.charset)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.ReadAll(reader); err == nil {
				t.Fatal("malformed encoded input passed validation")
			}
		})
	}
}

func TestNewValidatingReaderRejectsUnsupportedEncoding(t *testing.T) {
	if _, err := NewValidatingReader(bytes.NewReader(nil), "unsupported-encoding"); err == nil {
		t.Fatal("expected unsupported encoding validator error")
	}
}
