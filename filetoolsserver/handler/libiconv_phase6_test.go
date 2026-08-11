package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
)

func TestPhase6PinnedLibiconvFixturesMatchUTF8Oracles(t *testing.T) {
	wanted := map[string]bool{
		"big5-hkscs-1999": true,
		"big5-hkscs-2001": true,
		"big5-hkscs-2004": true,
		"big5-hkscs-2008": true,
		"iso-2022-cn":     true,
		"iso-2022-cn-ext": true,
		"iso-2022-jp-1":   true,
		"iso-2022-jp-2":   true,
		"iso-2022-jp-3":   true,
		"iso-2022-jp-ms":  true,
		"iso-2022-kr":     true,
		"tcvn":            true,
	}
	manifest := loadPhase6CorpusManifest(t)
	absoluteCorpus, err := filepath.Abs(internetCorpusDir)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{absoluteCorpus})
	seen := make(map[string]bool, len(wanted))

	for _, fixture := range manifest.Fixtures {
		if !wanted[fixture.Encoding] || fixture.Oracle == nil || fixture.SourceProject != "GNU libiconv" {
			continue
		}
		fixture := fixture
		t.Run(fixture.Encoding, func(t *testing.T) {
			seen[fixture.Encoding] = true
			path := filepath.Join(absoluteCorpus, fixture.File)
			oraclePath := filepath.Join(absoluteCorpus, fixture.Oracle.File)
			oracle, err := os.ReadFile(oraclePath)
			if err != nil {
				t.Fatal(err)
			}
			result, output, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
				Path: path, Encoding: fixture.Encoding,
			})
			if err != nil || result.IsError {
				t.Fatalf("read result=%+v output=%+v err=%v", result, output, err)
			}
			if !bytes.Equal([]byte(output.Content), oracle) {
				t.Fatalf("decoded fixture differs from pinned UTF-8 oracle: got %d bytes want %d", len(output.Content), len(oracle))
			}

			registered, ok := fileEncoding.Get(fixture.Encoding)
			if !ok || registered == nil {
				t.Fatalf("encoding %q is not registered", fixture.Encoding)
			}
			canonical, err := registered.NewEncoder().Bytes(oracle)
			if err != nil {
				t.Fatalf("canonical encode: %v", err)
			}
			roundTrip, err := registered.NewDecoder().Bytes(canonical)
			if err != nil {
				t.Fatalf("canonical decode: %v", err)
			}
			if !bytes.Equal(roundTrip, oracle) {
				t.Fatalf("canonical semantic round-trip changed oracle bytes")
			}
		})
	}
	for name := range wanted {
		if !seen[name] {
			t.Errorf("missing pinned libiconv fixture/oracle for %s", name)
		}
	}
}

func TestPhase6DetectionFixturesDecodeAndReencodeSemantically(t *testing.T) {
	fixtures := []struct {
		file     string
		encoding string
	}{
		{file: "euc-tw.fixture", encoding: "euc-tw"},
		{file: "johab.fixture", encoding: "johab"},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.encoding, func(t *testing.T) {
			original, err := os.ReadFile(filepath.Join(internetCorpusDir, fixture.file))
			if err != nil {
				t.Fatal(err)
			}
			registered, ok := fileEncoding.Get(fixture.encoding)
			if !ok || registered == nil {
				t.Fatalf("encoding %q is not registered", fixture.encoding)
			}
			decoded, err := registered.NewDecoder().Bytes(original)
			if err != nil {
				t.Fatalf("decode pinned fixture: %v", err)
			}
			canonical, err := registered.NewEncoder().Bytes(decoded)
			if err != nil {
				t.Fatalf("encode canonical form: %v", err)
			}
			roundTrip, err := registered.NewDecoder().Bytes(canonical)
			if err != nil {
				t.Fatalf("decode canonical form: %v", err)
			}
			if !bytes.Equal(roundTrip, decoded) {
				t.Fatal("canonical semantic round-trip changed decoded text")
			}
		})
	}
}

func TestPhase6ExplicitConversionRoundTrip(t *testing.T) {
	cases := []struct {
		encoding string
		content  string
	}{
		{encoding: "euc-cn", content: "中文\r\n"},
		{encoding: "euc-tw", content: "中文測試\r\n"},
		{encoding: "gb18030-2022", content: "\uE816\r\n"},
		{encoding: "ibm1162", content: "\u0E48\r\n"},
		{encoding: "ibm1163", content: "€\r\n"},
		{encoding: "johab", content: "한글 테스트\r\n"},
		{encoding: "iso-2022-jp-3", content: "日本語テスト\r\n"},
		{encoding: "iso-2022-cn-ext", content: "中文測試\r\n"},
		{encoding: "tcvn", content: "Việt Nam\r\n"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.encoding, func(t *testing.T) {
			root := t.TempDir()
			h := NewHandler([]string{root})
			path := filepath.Join(root, "phase6.data")
			if err := os.WriteFile(path, []byte(testCase.content), 0o644); err != nil {
				t.Fatal(err)
			}
			result, _, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
				Path: path, From: "utf-8", To: testCase.encoding, BOM: "never",
			})
			if err != nil || result.IsError {
				t.Fatalf("UTF-8 -> %s result=%+v err=%v", testCase.encoding, result, err)
			}
			result, output, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
				Path: path, Encoding: testCase.encoding,
			})
			if err != nil || result.IsError || output.Content != testCase.content {
				t.Fatalf("%s read result=%+v output=%+v err=%v", testCase.encoding, result, output, err)
			}
			result, _, err = h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
				Path: path, From: testCase.encoding, To: "utf-8", BOM: "never",
			})
			if err != nil || result.IsError {
				t.Fatalf("%s -> UTF-8 result=%+v err=%v", testCase.encoding, result, err)
			}
			actual, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != testCase.content {
				t.Fatalf("conversion round-trip = %q, want %q", actual, testCase.content)
			}
		})
	}
}

func TestPhase6MalformedInputFailsClosedWithoutMutation(t *testing.T) {
	cases := []struct {
		name     string
		encoding string
		data     []byte
	}{
		{name: "euc-cn-truncated", encoding: "euc-cn", data: []byte{'a', '\n', 0xa1}},
		{name: "euc-tw-truncated", encoding: "euc-tw", data: []byte{'a', '\n', 0x8e, 0xa2, 0xa1}},
		{name: "gb18030-2022-truncated", encoding: "gb18030-2022", data: []byte{'a', '\n', 0x81, 0x30, 0x81}},
		{name: "iso2022-truncated-escape", encoding: "iso-2022-jp-3", data: []byte{'a', '\n', 0x1b, '$'}},
		{name: "iso2022-cn-shift-without-designation", encoding: "iso-2022-cn", data: []byte{'a', '\n', 0x0e, 0x21, 0x21}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			h := NewHandler([]string{root})
			path := filepath.Join(root, "invalid.data")
			if err := os.WriteFile(path, testCase.data, 0o644); err != nil {
				t.Fatal(err)
			}
			readResult, _, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
				Path: path, Encoding: testCase.encoding,
			})
			if err != nil || !readResult.IsError || readResult.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
				t.Fatalf("malformed read result=%+v err=%v", readResult, err)
			}
			changeResult, _, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
				Path: path, Encoding: testCase.encoding, Style: LineEndingCRLF,
			})
			if err != nil || !changeResult.IsError || changeResult.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
				t.Fatalf("malformed line-ending result=%+v err=%v", changeResult, err)
			}
			actual, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(actual, testCase.data) {
				t.Fatalf("malformed source was mutated: got %x want %x", actual, testCase.data)
			}
		})
	}
}

func loadPhase6CorpusManifest(t *testing.T) internetCorpusManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(internetCorpusDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest internetCorpusManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}
