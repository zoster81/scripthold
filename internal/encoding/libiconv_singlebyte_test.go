package encoding

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"slices"
	"sort"
	"testing"

	xencoding "golang.org/x/text/encoding"
)

var phase5CanonicalNames = []string{
	"atarist",
	"gb-1988-80",
	"georgian-academy",
	"georgian-ps",
	"hp-roman8",
	"ibm1025",
	"ibm1026",
	"ibm1046",
	"ibm1097",
	"ibm1112",
	"ibm1122",
	"ibm1123",
	"ibm1124",
	"ibm1125",
	"ibm1129",
	"ibm1130",
	"ibm1131",
	"ibm1132",
	"ibm1133",
	"ibm1137",
	"ibm1141",
	"ibm1142",
	"ibm1143",
	"ibm1144",
	"ibm1145",
	"ibm1146",
	"ibm1147",
	"ibm1148",
	"ibm1149",
	"ibm1153",
	"ibm1154",
	"ibm1155",
	"ibm1156",
	"ibm1157",
	"ibm1158",
	"ibm1164",
	"ibm1165",
	"ibm1166",
	"ibm12712",
	"ibm16804",
	"ibm273",
	"ibm277",
	"ibm278",
	"ibm280",
	"ibm282",
	"ibm284",
	"ibm285",
	"ibm297",
	"ibm423",
	"ibm424",
	"ibm425",
	"ibm4971",
	"ibm500",
	"ibm737",
	"ibm775",
	"ibm853",
	"ibm856",
	"ibm857",
	"ibm861",
	"ibm864",
	"ibm869",
	"ibm870",
	"ibm871",
	"ibm875",
	"ibm880",
	"ibm905",
	"ibm922",
	"ibm924",
	"jis-c6220-1969-ro",
	"jis-x0201",
	"koi8-t",
	"mac-arabic",
	"mac-central-europe",
	"mac-croatian",
	"mac-greek",
	"mac-hebrew",
	"mac-iceland",
	"mac-romania",
	"mac-thai",
	"mac-turkish",
	"mac-ukraine",
	"mulelao-1",
	"nextstep",
	"pt154",
	"riscos-latin1",
	"rk1048",
	"tds565",
	"viscii",
}

func TestPhase5SelectedSingleByteMappingsAreRegistered(t *testing.T) {
	if len(phase5CanonicalNames) != 88 {
		t.Fatalf("phase-5 canonical set = %d, want 88", len(phase5CanonicalNames))
	}
	if !sort.StringsAreSorted(phase5CanonicalNames) {
		t.Fatal("phase-5 canonical names are not sorted")
	}
	if slices.Contains(phase5CanonicalNames, "iso-8859-11") || slices.Contains(phase5CanonicalNames, "tis-620") {
		t.Fatal("phase-5 set must not replace existing x/text-owned aliases")
	}

	for _, name := range phase5CanonicalNames {
		descriptor, ok := LookupDescriptor(name)
		if !ok {
			t.Errorf("LookupDescriptor(%q) failed", name)
			continue
		}
		if !descriptor.Supported || !descriptor.Readable || !descriptor.Writable {
			t.Errorf("%q lacks read/write capability: %+v", name, descriptor)
		}
		if descriptor.AutoDetectable || !descriptor.ExplicitOnly {
			t.Errorf("%q must remain explicit-only until detector hardening: %+v", name, descriptor)
		}
		if descriptor.Unicode || len(descriptor.BOM) != 0 || descriptor.AutoBOM {
			t.Errorf("%q has unexpected Unicode/BOM metadata: %+v", name, descriptor)
		}
		if _, ok := Get(name); !ok {
			t.Errorf("Get(%q) failed", name)
		}
	}
}

func TestPhase5GeneratedSingleByteMapsAreExhaustiveAndStrict(t *testing.T) {
	if generatedLibiconvRevision != "9d19c66d0a1768cffcf497b2db70bf4018b578d7" {
		t.Fatalf("generated libiconv revision = %q", generatedLibiconvRevision)
	}
	if len(generatedLibiconvSingleByteSpecs) != 88 {
		t.Fatalf("generated specs = %d, want 88", len(generatedLibiconvSingleByteSpecs))
	}

	generatedNames := make([]string, 0, len(generatedLibiconvSingleByteSpecs))
	seenSourceIDs := make(map[string]bool, len(generatedLibiconvSingleByteSpecs))
	for index := range generatedLibiconvSingleByteSpecs {
		spec := &generatedLibiconvSingleByteSpecs[index]
		generatedNames = append(generatedNames, spec.CanonicalName)
		if seenSourceIDs[spec.SourceID] {
			t.Fatalf("duplicate generated source id %q", spec.SourceID)
		}
		seenSourceIDs[spec.SourceID] = true
		if len(spec.SourceHeaderSHA256) != 64 {
			t.Fatalf("%s header SHA-256 length = %d", spec.CanonicalName, len(spec.SourceHeaderSHA256))
		}
		if _, err := hex.DecodeString(spec.SourceHeaderSHA256); err != nil {
			t.Fatalf("%s invalid header SHA-256: %v", spec.CanonicalName, err)
		}
		if spec.SourceName == "" || spec.SourceDefinition == "" {
			t.Fatalf("%s missing generated provenance: %+v", spec.CanonicalName, spec)
		}
		for _, alias := range spec.Aliases {
			if got, ok := CanonicalName(alias); !ok || got != spec.CanonicalName {
				t.Fatalf("alias %q = %q, %v; want %q, true", alias, got, ok, spec.CanonicalName)
			}
		}

		registered, ok := Get(spec.CanonicalName)
		if !ok || registered == nil {
			t.Fatalf("Get(%q) failed", spec.CanonicalName)
		}
		for value, wantRune := range spec.Decode {
			decoded, err := registered.NewDecoder().Bytes([]byte{byte(value)})
			if wantRune == undefinedSingleByteRune {
				if err == nil || !errors.Is(err, ErrInvalidEncodedSequence) {
					t.Fatalf("%s byte 0x%02X decoded without strict invalid-sequence error: %x, %v", spec.CanonicalName, value, decoded, err)
				}
				continue
			}
			if err != nil {
				t.Fatalf("%s byte 0x%02X decode: %v", spec.CanonicalName, value, err)
			}
			if string(decoded) != string(wantRune) {
				t.Fatalf("%s byte 0x%02X decoded %q, want U+%04X", spec.CanonicalName, value, decoded, wantRune)
			}
			encoded, err := registered.NewEncoder().Bytes(decoded)
			if err != nil {
				t.Fatalf("%s U+%04X encode: %v", spec.CanonicalName, wantRune, err)
			}
			if !bytes.Equal(encoded, []byte{byte(value)}) {
				t.Fatalf("%s U+%04X encoded %x, want %02X", spec.CanonicalName, wantRune, encoded, value)
			}
		}
	}
	if !slices.Equal(generatedNames, phase5CanonicalNames) {
		t.Fatalf("generated canonical set = %v, want %v", generatedNames, phase5CanonicalNames)
	}
}

func TestPhase5SingleByteStreamingAcrossOneByteChunks(t *testing.T) {
	const text = "ΑΒΓ\n"
	encodedReader, err := NewEncoderReader(&oneByteReader{data: []byte(text)}, "ibm737")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := io.ReadAll(encodedReader)
	if err != nil {
		t.Fatal(err)
	}
	decodedReader, err := NewDecoderReader(&oneByteReader{data: append([]byte(nil), encoded...)}, "ibm737")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(decodedReader)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != text {
		t.Fatalf("stream round-trip = %q, want %q", decoded, text)
	}
}

func TestPhase5SingleByteEncoderRejectsMalformedUTF8(t *testing.T) {
	registered, ok := Get("ibm737")
	if !ok {
		t.Fatal("ibm737 is not registered")
	}
	if _, err := registered.NewEncoder().Bytes([]byte{0xC3}); !errors.Is(err, xencoding.ErrInvalidUTF8) {
		t.Fatalf("malformed UTF-8 encoder error = %v, want encoding.ErrInvalidUTF8", err)
	}
}

func TestPhase5RepresentativeSourceAliases(t *testing.T) {
	tests := map[string]string{
		"CP737":             "ibm737",
		"IBM-273":           "ibm273",
		"MacCentralEurope":  "mac-central-europe",
		"GB_1988-80":        "gb-1988-80",
		"JIS_C6220-1969-RO": "jis-c6220-1969-ro",
		"JIS_X0201":         "jis-x0201",
		"GEORGIAN-ACADEMY":  "georgian-academy",
		"VISCII":            "viscii",
	}
	for alias, want := range tests {
		got, ok := CanonicalName(alias)
		if !ok || got != want {
			t.Errorf("CanonicalName(%q) = %q, %v; want %q, true", alias, got, ok, want)
		}
	}
}
