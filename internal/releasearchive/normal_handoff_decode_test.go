package releasearchive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func rawNormalJSONFixture(t *testing.T) []byte {
	t.Helper()
	_, _, originals := normalHandoffFixture(t)
	var out bytes.Buffer
	out.WriteByte('[')
	for index, file := range originals {
		if index > 0 {
			out.WriteByte(',')
		}
		fmt.Fprintf(&out, `{"downloadName":%q,"original":{"name":%q,"size":%d,"sha256":%q}}`,
			file.DownloadName, file.Original.Name, file.Original.Size, file.Original.SHA256)
	}
	out.WriteByte(']')
	return out.Bytes()
}

// The source eventually reports an error, so an unfixed no-progress loop
// fails this test deterministically without hanging.
type emptyNormalReader struct{ calls int }

func (reader *emptyNormalReader) Read(_ []byte) (int, error) {
	reader.calls++
	if reader.calls <= 100 {
		return 0, nil
	}
	return 0, errors.New("reader exceeded the no-progress budget")
}

func TestDecodeRawNormalHandoffPlanRejectsNonProgressingReader(t *testing.T) {
	reader := &emptyNormalReader{}
	files, err := DecodeRawNormalHandoffPlan(reader)
	if files != nil || !errors.Is(err, io.ErrNoProgress) || reader.calls > 100 {
		t.Fatalf("expected bounded no-progress rejection: files=%v err=%v calls=%d", files, err, reader.calls)
	}
}

type intermittentNormalReader struct {
	source io.Reader
	empty  bool
}

func (reader *intermittentNormalReader) Read(dst []byte) (int, error) {
	reader.empty = !reader.empty
	if reader.empty {
		return 0, nil
	}
	return reader.source.Read(dst)
}

func TestDecodeRawNormalHandoffPlanAllowsIntermittentEmptyReads(t *testing.T) {
	reader := &intermittentNormalReader{source: bytes.NewReader(rawNormalJSONFixture(t))}
	files, err := DecodeRawNormalHandoffPlan(reader)
	if err != nil || len(files) != 15 {
		t.Fatalf("valid interrupted stream: files=%d err=%v", len(files), err)
	}
}

type invalidNormalReadCount struct{ n int }

func (reader invalidNormalReadCount) Read([]byte) (int, error) {
	return reader.n, nil
}

func TestDecodeRawNormalHandoffPlanRejectsInvalidReadCounts(t *testing.T) {
	for _, count := range []int{-1, 1 << 20} {
		files, err := DecodeRawNormalHandoffPlan(invalidNormalReadCount{n: count})
		if files != nil || err == nil || !strings.Contains(err.Error(), "invalid byte count") {
			t.Fatalf("accepted invalid source read count %d: files=%v err=%v", count, files, err)
		}
	}
}

func TestDecodeRawNormalHandoffPlanExactFifteen(t *testing.T) {
	record := rawNormalJSONFixture(t)
	got, err := DecodeRawNormalHandoffPlan(bytes.NewReader(record))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 15 || got[0].Original.Name != "original-01" ||
		got[14].DownloadName != "raw-9001-2-15" {
		t.Fatalf("wrong Go stage input: %+v", got)
	}
	// The parser is read-only and does not roundtrip or alter the source data.
	var encoded any
	if err := json.Unmarshal(record, &encoded); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeRawNormalHandoffPlanRejectsUntrustedJSON(t *testing.T) {
	base := string(rawNormalJSONFixture(t))
	for _, tc := range []struct {
		name, input string
	}{
		{"empty", ""},
		{"null", "null"},
		{"root object", "{}"},
		{"missing asset", strings.Replace(base, `,{"downloadName":"raw-9001-2-15","original":`, `,{"omit":"raw-9001-2-15","original":`, 1)},
		{"trailing JSON", base + " {}"},
		{"trailing invalid", base + " garbage"},
		{"oversized input", base + strings.Repeat(" ", 16*1024)},
		{"duplicate downloadName property", strings.Replace(base, `"downloadName":"raw-9001-2-01",`, `"downloadName":"raw-9001-2-01","downloadName":"raw-9001-2-02",`, 1)},
		{"duplicate original name property", strings.Replace(base, `"name":"original-01",`, `"name":"original-01","name":"original-02",`, 1)},
		{"case variant", strings.Replace(base, `"downloadName":`, `"DownloadName":`, 1)},
		{"unexpected path", strings.Replace(base, `"downloadName":`, `"path":"../escape","downloadName":`, 1)},
		{"unexpected nested path", strings.Replace(base, `"name":"original-01",`, `"path":"../escape","name":"original-01",`, 1)},
		{"missing nested digest", strings.Replace(base, `"sha256":`, `"digest":`, 1)},
		{"wrong numeric shape", strings.Replace(base, `"size":`, `"size":1.0,"unused":`, 1)},
		{"untrusted file path", strings.Replace(base, `"downloadName":"raw-9001-2-01"`, `"downloadName":"../escape"`, 1)},
		{"duplicate physical name", strings.Replace(base, `"downloadName":"raw-9001-2-02"`, `"downloadName":"raw-9001-2-01"`, 1)},
		{"duplicate original name", strings.Replace(base, `"name":"original-02"`, `"name":"original-01"`, 1)},
		{"bad sha", strings.Replace(base, `"sha256":`, `"sha256":"invalid","extraDigest":`, 1)},
		{"null nested", strings.Replace(base, `"original":{`, `"original":null,"unused":{`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files, err := DecodeRawNormalHandoffPlan(strings.NewReader(tc.input))
			if err == nil || files != nil {
				t.Fatalf("accepted invalid Go stage plan: files=%d err=%v", len(files), err)
			}
		})
	}
	if _, err := DecodeRawNormalHandoffPlan(nil); err == nil {
		t.Fatal("accepted nil reader")
	}
}
