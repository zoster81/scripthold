package handler

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
)

func encodePhase3Text(t *testing.T, charset, text string) []byte {
	t.Helper()
	if fileEncoding.IsUTF8(charset) {
		return []byte(text)
	}
	registered, ok := fileEncoding.Get(charset)
	if !ok {
		t.Fatalf("encoding %q is not registered", charset)
	}
	encoded, err := registered.NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatalf("encode %q as %s: %v", text, charset, err)
	}
	return encoded
}

func TestAllRegisteredEncodingsLineEndingRoundTrip(t *testing.T) {
	for _, item := range fileEncoding.ListEncodings() {
		item := item
		t.Run(item.Name, func(t *testing.T) {
			original := encodePhase3Text(t, item.Name, "alpha\nbeta\n")
			wantCRLF := encodePhase3Text(t, item.Name, "alpha\r\nbeta\r\n")

			changed, crlfStats := transformLineEndingFixture(t, item.Name, original, LineEndingCRLF)
			if crlfStats.LFCount != 2 || crlfStats.CRLFCount != 0 {
				t.Fatalf("LF -> CRLF stats = %+v, want two LF", crlfStats)
			}
			if !bytes.Equal(changed, wantCRLF) {
				t.Fatalf("CRLF bytes = %x, want %x", changed, wantCRLF)
			}

			roundTripped, lfStats := transformLineEndingFixture(t, item.Name, changed, LineEndingLF)
			if lfStats.CRLFCount != 2 || lfStats.LFCount != 0 {
				t.Fatalf("CRLF -> LF stats = %+v, want two CRLF", lfStats)
			}
			if !bytes.Equal(roundTripped, original) {
				t.Fatalf("line-ending round trip changed bytes: got %x want %x", roundTripped, original)
			}
		})
	}
}

func TestPhase3StatefulLineEndingRoundTrips(t *testing.T) {
	tests := []struct {
		charset string
		lfText  string
	}{
		{charset: "iso-2022-jp", lfText: "日本語\n日本語\n"},
		{charset: "hz-gb-2312", lfText: "中文\n中文\n"},
	}
	for _, testCase := range tests {
		t.Run(testCase.charset, func(t *testing.T) {
			original := encodePhase3Text(t, testCase.charset, testCase.lfText)
			wantCRLF := encodePhase3Text(t, testCase.charset, ConvertLineEndings(testCase.lfText, LineEndingCRLF))

			root := t.TempDir()
			path := filepath.Join(root, "stateful.fixture")
			if err := os.WriteFile(path, original, 0644); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{root})

			result, _, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
				Path: path, Style: LineEndingCRLF, Encoding: testCase.charset,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("stateful LF -> CRLF failed: %+v", result)
			}
			changed, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(changed, wantCRLF) {
				t.Fatalf("stateful CRLF bytes = %x, want %x", changed, wantCRLF)
			}

			result, _, err = h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
				Path: path, Style: LineEndingLF, Encoding: testCase.charset,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("stateful CRLF -> LF failed: %+v", result)
			}
			roundTripped, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(roundTripped, original) {
				t.Fatalf("stateful line-ending round trip changed bytes: got %x want %x", roundTripped, original)
			}
		})
	}
}

func TestPhase3HZContinuationIsNotAContentLineEnding(t *testing.T) {
	original := []byte("alpha~\nbeta\n")
	if _, ok := fileEncoding.Get("hz-gb-2312"); !ok {
		t.Fatal("HZ-GB-2312 is not registered")
	}
	reader, err := fileEncoding.NewDecoderReader(bytes.NewReader(original), "hz-gb-2312")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != "alphabeta\n" {
		t.Fatalf("HZ fixture decodes to %q, want %q", decoded, "alphabeta\n")
	}

	root := t.TempDir()
	path := filepath.Join(root, "continuation.hz")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	result, output, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
		Path: path, Style: LineEndingCRLF, Encoding: "hz-gb-2312",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || output.LinesChanged != 1 {
		t.Fatalf("HZ continuation conversion failed: result=%+v output=%+v", result, output)
	}
	changed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("alpha~\nbeta\r\n")
	if !bytes.Equal(changed, want) {
		t.Fatalf("HZ continuation bytes = %q, want %q", changed, want)
	}
}

func TestPhase3ChangeLineEndingsRejectsMalformedSourceWithoutMutation(t *testing.T) {
	tests := []struct {
		charset string
		data    []byte
	}{
		{charset: "utf-8", data: []byte{'a', '\n', 0xf0, 0x9f, 0x98}},
		{charset: "windows-1252", data: []byte{'a', '\n', 0x81}},
		{charset: "shift_jis", data: []byte{'a', '\n', 0x82}},
		{charset: "hz-gb-2312", data: []byte("a\n~")},
		{charset: "utf-16-le", data: []byte{'a', 0, '\n', 0, 0x3d, 0xd8}},
	}

	for _, testCase := range tests {
		t.Run(testCase.charset, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "malformed.fixture")
			original := append([]byte(nil), testCase.data...)
			if err := os.WriteFile(path, original, 0644); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{root})
			result, _, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
				Path: path, Style: LineEndingCRLF, Encoding: testCase.charset,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatal("malformed source unexpectedly succeeded")
			}
			if result.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
				t.Fatalf("error code = %v, want %s", result.Meta[ErrorCodeMetaKey], ErrCodeEncoding)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, original) {
				t.Fatalf("malformed-source failure mutated target: got %x want %x", got, original)
			}
		})
	}
}

func TestPhase3PinnedCorpusDetection(t *testing.T) {
	tests := map[string]string{
		"big5.fixture":        "big5",
		"euc-jp.fixture":      "euc-jp",
		"hz-gb-2312.fixture":  "hz-gb-2312",
		"ibm855.fixture":      "ibm855",
		"iso-2022-jp.fixture": "iso-2022-jp",
		"iso-2022-kr.fixture": "iso-2022-kr",
		"shift-jis.fixture":   "shift_jis",
		"windows-949.fixture": "euc-kr",
	}
	for file, want := range tests {
		data, err := os.ReadFile(filepath.Join(internetCorpusDir, file))
		if err != nil {
			t.Fatal(err)
		}
		got := fileEncoding.Detect(data)
		if got.Charset != want || got.Confidence < fileEncoding.MinConfidenceThreshold {
			t.Fatalf("Detect(%s) = %q/%d, want %q with confidence >= %d", file, got.Charset, got.Confidence, want, fileEncoding.MinConfidenceThreshold)
		}
	}
}

func TestPhase3PinnedStatefulCodecOracles(t *testing.T) {
	tests := []struct {
		charset string
		fixture string
		oracle  string
	}{
		{charset: "hz-gb-2312", fixture: "hz-gb-2312.fixture", oracle: "hz-gb-2312.oracle.utf8"},
		{charset: "iso-2022-jp", fixture: "iso-2022-jp-libiconv.fixture", oracle: "iso-2022-jp-libiconv.oracle.utf8"},
	}
	for _, testCase := range tests {
		t.Run(testCase.charset, func(t *testing.T) {
			encoded, err := os.ReadFile(filepath.Join(internetCorpusDir, testCase.fixture))
			if err != nil {
				t.Fatal(err)
			}
			oracle, err := os.ReadFile(filepath.Join(internetCorpusDir, testCase.oracle))
			if err != nil {
				t.Fatal(err)
			}
			reader, err := fileEncoding.NewDecoderReader(bytes.NewReader(encoded), testCase.charset)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decoded, oracle) {
				t.Fatalf("decoded bytes differ from pinned UTF-8 oracle")
			}
		})
	}
}
