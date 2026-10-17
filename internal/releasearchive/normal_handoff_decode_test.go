package releasearchive

import (
	"bytes"
	"encoding/json"
	"fmt"
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
