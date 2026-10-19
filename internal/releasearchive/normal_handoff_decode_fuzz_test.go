package releasearchive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// FuzzDecodeRawNormalHandoffPlan keeps hostile cross-job JSON inside the
// strict 15-record/16-KiB schema. Accepted bytes must survive a canonical
// encode/decode without changing the original file or digest identities.
// This does not authenticate the source job, its manifests, or file bytes.
func FuzzDecodeRawNormalHandoffPlan(f *testing.F) {
	var canonical bytes.Buffer
	canonical.WriteByte('[')
	for i := 1; i <= 15; i++ {
		if i > 1 {
			canonical.WriteByte(',')
		}
		fmt.Fprintf(&canonical,
			`{"downloadName":"raw-%02d","original":{"name":"original-%02d","size":%d,"sha256":"%064x"}}`,
			i, i, i+100, i)
	}
	canonical.WriteByte(']')
	for _, seed := range [][]byte{
		canonical.Bytes(),
		[]byte(""),
		[]byte("null"),
		[]byte("{}"),
		[]byte("[]"),
		[]byte(`[{"downloadName":"../escape","original":null}]`),
		[]byte(`{"downloadName":"raw-01"}`),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		files, err := DecodeRawNormalHandoffPlan(bytes.NewReader(input))
		if err != nil {
			if files != nil {
				t.Fatalf("failed parse returned usable files: %v", err)
			}
			return
		}
		if len(files) != 15 {
			t.Fatalf("accepted unexpected normal original count %d", len(files))
		}
		physical, canonical, err := validateRawNormalEntries(files)
		if err != nil || len(physical) != 15 || len(canonical) != 15 {
			t.Fatalf("accepted invalid staging entries: %v", err)
		}
		// Normalize the original wire JSON, not the private Go structs:
		// ExpectedFile deliberately has no wire-format JSON field tags.
		var originalJSON any
		if err := json.Unmarshal(input, &originalJSON); err != nil {
			t.Fatalf("accepted input cannot be decoded as JSON: %v", err)
		}
		encoded, err := json.Marshal(originalJSON)
		if err != nil {
			t.Fatalf("accepted JSON cannot be normalized: %v", err)
		}
		reparsed, err := DecodeRawNormalHandoffPlan(bytes.NewReader(encoded))
		if err != nil || !reflect.DeepEqual(files, reparsed) {
			t.Fatalf("accepted record identity changes on canonical encoding: %v", err)
		}
	})
}
