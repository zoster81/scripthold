package encoding

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

func ambiguityCorpusFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "filetoolsserver", "handler", "testdata", "internet-corpus", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func requireAmbiguousEncoding(t *testing.T, data []byte) {
	t.Helper()
	result := Detect(data)
	if result.Charset != "" || result.Confidence >= MinConfidenceThreshold {
		t.Fatalf("Detect = %+v, want explicit ambiguity", result)
	}
}

func TestPinnedHighConfidenceConfusionsBecomeAmbiguous(t *testing.T) {
	for _, fixture := range []string{
		"windows-1251.fixture",
		"windows-1253.fixture",
		"georgian-ps.fixture",
		"ibm865.fixture",
		"viscii.fixture",
		"iso-8859-1.fixture",
		"iso-8859-15.fixture",
		"windows-1257.fixture",
	} {
		t.Run(fixture, func(t *testing.T) {
			requireAmbiguousEncoding(t, ambiguityCorpusFixture(t, fixture))
		})
	}
}

func TestStrongPinnedLegacyEvidenceRemainsTrusted(t *testing.T) {
	tests := map[string]string{
		"big5.fixture":        "big5",
		"euc-jp.fixture":      "euc-jp",
		"ibm855.fixture":      "ibm855",
		"ibm866.fixture":      "ibm866",
		"koi8-r.fixture":      "koi8-r",
		"shift-jis.fixture":   "shift_jis",
		"windows-949.fixture": "euc-kr",
	}
	for fixture, want := range tests {
		t.Run(fixture, func(t *testing.T) {
			result := Detect(ambiguityCorpusFixture(t, fixture))
			if result.Charset != want || result.Confidence < HighConfidenceThreshold {
				t.Fatalf("Detect = %+v, want trusted %s", result, want)
			}
		})
	}
}

func TestStatefulSignaturesPrecedeUTF8Fallback(t *testing.T) {
	tests := map[string]string{
		"hz-gb-2312.fixture":  "hz-gb-2312",
		"iso-2022-jp.fixture": "iso-2022-jp",
		"iso-2022-kr.fixture": "iso-2022-kr",
	}
	for fixture, want := range tests {
		t.Run(fixture, func(t *testing.T) {
			result := Detect(ambiguityCorpusFixture(t, fixture))
			if result.Charset != want || result.Confidence < HighConfidenceThreshold {
				t.Fatalf("Detect = %+v, want trusted %s", result, want)
			}
		})
	}
}

func TestStatefulSyntaxAloneIsNotTrusted(t *testing.T) {
	tests := [][]byte{
		[]byte("template syntax ~{ab~} is literal ASCII text\n"),
		append([]byte("escape example: "), []byte{0x1b, '$', 'B', 'A', 'B', 0x1b, '(', 'B', '\n'}...),
	}
	for index, data := range tests {
		result := Detect(data)
		if result.Charset == "hz-gb-2312" || result.Charset == "iso-2022-jp" || result.Charset == "iso-2022-kr" {
			t.Fatalf("case %d trusted a stateful codec from syntax alone: %+v", index, result)
		}
	}
}

func TestStatefulVariantsRemainExplicitWhenBaseSyntaxCannotDecode(t *testing.T) {
	for _, fixture := range []string{"iso-2022-jp-2.fixture", "iso-2022-jp-3.fixture", "iso-2022-jp-ms.fixture"} {
		t.Run(fixture, func(t *testing.T) {
			requireAmbiguousEncoding(t, ambiguityCorpusFixture(t, fixture))
		})
	}
}

func TestGB18030FourByteEvidenceRejectsRevisionGuessing(t *testing.T) {
	registered, ok := Get("gb18030")
	if !ok {
		t.Fatal("gb18030 is not registered")
	}
	text := "GB18030 full Unicode evidence: 😀 🌍 😀 🌍 end"
	data, err := registered.NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	// Four-byte grammar proves that GBK is wrong, but Scripthold exposes both
	// generic GB18030 and exact GB18030:2022 semantics. Their revision cannot be
	// inferred from bytes alone, so auto-detection must require explicit choice.
	requireAmbiguousEncoding(t, data)
}

func TestGB18030SubsetDoesNotOverrideGBK(t *testing.T) {
	data := ambiguityCorpusFixture(t, "gb18030.fixture")
	result := Detect(data)
	if result.Charset != "gbk" || result.Confidence < HighConfidenceThreshold {
		t.Fatalf("Detect = %+v, want narrow GBK-compatible result for GB18030 subset", result)
	}
}

func TestControlHeavyValidUTF8IsBinaryAmbiguous(t *testing.T) {
	data := bytes.Repeat([]byte{0x01}, 32)
	if !utf8.Valid(data) {
		t.Fatal("control-heavy fixture must remain syntactically valid UTF-8")
	}
	requireAmbiguousEncoding(t, data)
}

func TestShortLegacyEvidenceFloor(t *testing.T) {
	for _, data := range [][]byte{
		{0xE9},
		{0xCF, 0xF0},
		{0x82, 0xA0},
	} {
		requireAmbiguousEncoding(t, data)
	}
}

func TestRejectedDetectorLabelsRemainFailClosed(t *testing.T) {
	for _, fixture := range []string{"johab.fixture", "euc-tw.fixture"} {
		t.Run(fixture, func(t *testing.T) {
			requireAmbiguousEncoding(t, ambiguityCorpusFixture(t, fixture))
		})
	}
}

func TestPinnedCorpusHasNoTrustedTextMismatch(t *testing.T) {
	type fixture struct {
		Encoding string `json:"encoding"`
		File     string `json:"file"`
		Role     string `json:"role"`
	}
	type manifest struct {
		Fixtures []fixture `json:"fixtures"`
	}

	manifestPath := filepath.Join("..", "..", "filetoolsserver", "handler", "testdata", "internet-corpus", "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var parsed manifest
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}

	for _, entry := range parsed.Fixtures {
		if entry.Role != "detection" {
			continue
		}
		t.Run(entry.File, func(t *testing.T) {
			data := ambiguityCorpusFixture(t, entry.File)
			result := Detect(data)
			if result.Charset == "" || result.Confidence < MinConfidenceThreshold {
				return
			}
			expected, ok := CanonicalName(entry.Encoding)
			if !ok {
				t.Fatalf("manifest encoding %q is not registered", entry.Encoding)
			}
			if result.Charset == expected {
				return
			}

			expectedReader, err := NewDecoderReader(bytes.NewReader(data), expected)
			if err != nil {
				t.Fatal(err)
			}
			expectedText, err := io.ReadAll(expectedReader)
			if err != nil {
				t.Fatalf("declared %s decoder rejected fixture: %v", expected, err)
			}
			actualReader, err := NewDecoderReader(bytes.NewReader(data), result.Charset)
			if err != nil {
				t.Fatal(err)
			}
			actualText, err := io.ReadAll(actualReader)
			if err != nil {
				t.Fatalf("trusted %s decoder rejected fixture: %v", result.Charset, err)
			}
			if !bytes.Equal(actualText, expectedText) {
				t.Fatalf("trusted %s/%d decodes different text than declared %s", result.Charset, result.Confidence, expected)
			}

			registered, ok := Get(result.Charset)
			if !ok {
				t.Fatalf("trusted result %q is not registered", result.Charset)
			}
			var roundTrip []byte
			if IsUTF8(result.Charset) {
				roundTrip = append([]byte(nil), actualText...)
			} else {
				roundTrip, err = registered.NewEncoder().Bytes(actualText)
				if err != nil {
					t.Fatalf("trusted %s cannot re-encode decoded text: %v", result.Charset, err)
				}
			}
			if !bytes.Equal(roundTrip, data) {
				t.Fatalf("trusted %s is text-equivalent but not byte-exact for the observed payload", result.Charset)
			}
		})
	}
}

func TestChunkedStatefulDetectionSurvivesEscapeBoundary(t *testing.T) {
	registered, ok := Get("iso-2022-jp")
	if !ok {
		t.Fatal("iso-2022-jp is not registered")
	}
	encodedTail, err := registered.NewEncoder().Bytes([]byte("日本語の状態付き境界テストです。\n"))
	if err != nil {
		t.Fatal(err)
	}
	data := append(bytes.Repeat([]byte{'A'}, ChunkSize-1), encodedTail...)
	path := filepath.Join(t.TempDir(), "stateful-boundary.data")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	full, err := DetectFromFile(path, "full")
	if err != nil {
		t.Fatal(err)
	}
	sample, err := DetectFromFile(path, "sample")
	if err != nil {
		t.Fatal(err)
	}
	chunked, err := DetectFromFile(path, "chunked")
	if err != nil {
		t.Fatal(err)
	}
	if full.Charset != "iso-2022-jp" || sample != full || chunked != full {
		t.Fatalf("full=%+v sample=%+v chunked=%+v, want identical ISO-2022-JP result", full, sample, chunked)
	}
}

func FuzzTrustedDetectionAlwaysStrictlyDecodes(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("plain UTF-8 text\n"),
		ambiguityCorpusFixtureForFuzz("big5.fixture"),
		ambiguityCorpusFixtureForFuzz("iso-2022-jp.fixture"),
		ambiguityCorpusFixtureForFuzz("windows-1251.fixture"),
		bytes.Repeat([]byte{0x01}, 32),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		result := Detect(data)
		if result.Charset == "" || result.Confidence < MinConfidenceThreshold {
			return
		}
		descriptor, ok := LookupDescriptor(result.Charset)
		if !ok {
			t.Fatalf("trusted result is not registered: %+v", result)
		}
		if !descriptor.AutoDetectable || descriptor.ExplicitOnly {
			t.Fatalf("trusted result lacks auto-detection capability: %+v descriptor=%+v", result, descriptor)
		}
		if result.HasBOM {
			bomResult, found := DetectBOM(data)
			if !found || bomResult.Charset != result.Charset {
				t.Fatalf("trusted BOM result is inconsistent: detect=%+v bom=%+v found=%v", result, bomResult, found)
			}
			return
		}
		if IsUTF8(result.Charset) && !utf8.Valid(data) {
			t.Fatalf("trusted BOMless UTF-8 result accepted malformed UTF-8: %x", data)
		}
		reader, err := NewDecoderReader(bytes.NewReader(data), result.Charset)
		if err != nil {
			t.Fatalf("construct trusted decoder for %+v: %v", result, err)
		}
		if _, err := io.ReadAll(reader); err != nil {
			t.Fatalf("trusted result does not strictly decode: %+v err=%v", result, err)
		}
	})
}

func ambiguityCorpusFixtureForFuzz(name string) []byte {
	path := filepath.Join("..", "..", "filetoolsserver", "handler", "testdata", "internet-corpus", name)
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return data
}
