package handler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/config"
)

type encodingCoverageClass struct {
	name     string
	encoding string
	text     string
}

var encodingCoverageClasses = []encodingCoverageClass{
	{name: "unicode-utf32", encoding: "utf-32-le", text: "Città Привет 中文 🌍"},
	{name: "generated-singlebyte", encoding: "ibm737", text: "Ελλάδα"},
	{name: "direct-multibyte", encoding: "euc-tw", text: "中文測試"},
	{name: "stateful-iso2022", encoding: "iso-2022-jp-3", text: "日本語テスト"},
	{name: "stateful-tcvn", encoding: "tcvn", text: "Việt Nam"},
	{name: "gb18030-2022", encoding: "gb18030-2022", text: "\uE816 中文"},
}

func TestStreamingCancellationIsTypedAndNonMutating(t *testing.T) {
	for _, testCase := range encodingCoverageClasses {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "cancel.data")
			content := strings.Repeat("phase10 "+testCase.text+"\n", 32)
			encoded, ok := tryEncodeEncodingMatrix(testCase.encoding, content, false)
			if !ok {
				t.Fatalf("encode %s cancellation fixture", testCase.encoding)
			}
			if err := os.WriteFile(path, encoded, 0o644); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{root})

			cancelledContext := func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}

			readResult, _, err := h.HandleReadTextFile(cancelledContext(), nil, ReadTextFileInput{Path: path, Encoding: testCase.encoding})
			if err != nil || !readResult.IsError || readResult.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
				t.Fatalf("read cancellation result=%+v err=%v", readResult, err)
			}

			lineResult, _, err := h.HandleDetectLineEndings(cancelledContext(), nil, DetectLineEndingsInput{Path: path, Encoding: testCase.encoding})
			if err != nil || !lineResult.IsError || lineResult.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
				t.Fatalf("line-ending cancellation result=%+v err=%v", lineResult, err)
			}

			grepResult, _, err := h.HandleGrep(cancelledContext(), nil, GrepInput{Pattern: "phase10", Paths: []string{path}, Encoding: testCase.encoding})
			if err != nil || !grepResult.IsError || grepResult.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
				t.Fatalf("grep cancellation result=%+v err=%v", grepResult, err)
			}

			convertResult, _, err := h.HandleConvertEncoding(cancelledContext(), nil, ConvertEncodingInput{
				Path: path, From: testCase.encoding, To: "utf-8",
			})
			if err != nil || !convertResult.IsError || convertResult.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
				t.Fatalf("conversion cancellation result=%+v err=%v", convertResult, err)
			}
			actual, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(actual, encoded) {
				t.Fatal("cancelled conversion mutated source bytes")
			}
		})
	}
}

func TestDecodedLimitsApplyAcrossEncodingClasses(t *testing.T) {
	for _, testCase := range encodingCoverageClasses {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "limits.data")
			longLine := strings.Repeat(testCase.text+" ", 128)
			content := longLine + "\nphase10-tail\n"
			encoded, ok := tryEncodeEncodingMatrix(testCase.encoding, content, false)
			if !ok {
				t.Fatalf("encode %s limit fixture", testCase.encoding)
			}
			if err := os.WriteFile(path, encoded, 0o644); err != nil {
				t.Fatal(err)
			}

			cfg := config.LoadFromEnvironment(func(string) string { return "" })
			cfg.Limits.MaxLineBytes = 128
			cfg.Limits.MaxOutputBytes = 512
			h := NewHandler([]string{root}, WithConfig(cfg))

			readResult, _, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path, Encoding: testCase.encoding})
			if err != nil || !readResult.IsError || readResult.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
				t.Fatalf("read line limit result=%+v err=%v", readResult, err)
			}

			grepResult, _, err := h.HandleGrep(context.Background(), nil, GrepInput{
				Pattern: "phase10-tail", Paths: []string{path}, Encoding: testCase.encoding,
			})
			if err != nil || !grepResult.IsError || grepResult.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
				t.Fatalf("grep line limit result=%+v err=%v", grepResult, err)
			}
		})
	}
}

func TestMalformedClassRepresentativesFailClosed(t *testing.T) {
	tests := []struct {
		name     string
		encoding string
		data     []byte
	}{
		{name: "utf32-truncated", encoding: "utf-32-le", data: []byte{'A', 0, 0}},
		{name: "generated-singlebyte-undefined", encoding: "ibm856", data: []byte{'a', '\n', 0x9b}},
		{name: "euc-tw-truncated", encoding: "euc-tw", data: []byte{'a', '\n', 0x8e, 0xa2, 0xa1}},
		{name: "iso2022-truncated-escape", encoding: "iso-2022-jp-3", data: []byte{'a', '\n', 0x1b, '$'}},
		{name: "gb18030-2022-truncated", encoding: "gb18030-2022", data: []byte{'a', '\n', 0x81, 0x30, 0x81}},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "malformed.data")
			if err := os.WriteFile(path, testCase.data, 0o644); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{root})

			readResult, _, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path, Encoding: testCase.encoding})
			if err != nil || !readResult.IsError || readResult.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
				t.Fatalf("malformed read result=%+v err=%v", readResult, err)
			}
			convertResult, _, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{Path: path, From: testCase.encoding, To: "utf-8"})
			if err != nil || !convertResult.IsError || convertResult.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
				t.Fatalf("malformed conversion result=%+v err=%v", convertResult, err)
			}
			actual, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(actual, testCase.data) {
				t.Fatal("malformed conversion mutated source bytes")
			}
		})
	}
}
