package encoding

import (
	"bytes"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name    string
		wantOk  bool
		wantNil bool // true if encoding should be nil (UTF-8)
	}{
		{"utf-8", true, true},
		{"UTF-8", true, true},
		{"utf8", true, true},
		{"windows-1251", true, false},
		{"cp1251", true, false},
		{"CP1251", true, false},
		{"koi8-r", true, false},
		{"utf-16-le", true, false},
		{"utf-16-be", true, false},
		{"utf16le", true, false},
		{"utf16be", true, false},
		{"gbk", true, false},
		{"gb2312", true, false},
		{"gb-2312", true, false},
		{"cp936", true, false},
		{"gb18030", true, false},
		{"invalid", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, ok := Get(tt.name)
			if ok != tt.wantOk {
				t.Errorf("Get(%q) ok = %v, want %v", tt.name, ok, tt.wantOk)
			}
			if tt.wantOk && tt.wantNil && enc != nil {
				t.Errorf("Get(%q) = %v, want nil (UTF-8)", tt.name, enc)
			}
			if tt.wantOk && !tt.wantNil && enc == nil {
				t.Errorf("Get(%q) = nil, want non-nil encoding", tt.name)
			}
		})
	}
}

func TestCanonicalNameNormalizesSupportedAliases(t *testing.T) {
	tests := map[string]string{
		"UTF8":     "utf-8",
		"US-ASCII": "utf-8",
		"cp1251":   "windows-1251",
		"TIS-620":  "windows-874",
		"gb2312":   "gbk",
		"utf16le":  "utf-16-le",
	}
	for input, want := range tests {
		got, ok := CanonicalName(input)
		if !ok || got != want {
			t.Errorf("CanonicalName(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}
}

func TestIsUTF8(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"utf-8", true},
		{"UTF-8", true},
		{"utf8", true},
		{"ascii", true},
		{"cp1251", false},
		{"windows-1251", false},
		{"gbk", false},
		{"gb18030", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUTF8(tt.name); got != tt.want {
				t.Errorf("IsUTF8(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestListEncodings(t *testing.T) {
	items := ListEncodings()
	if len(items) == 0 {
		t.Fatal("ListEncodings() returned empty list")
	}

	// Check that items are sorted by DisplayName
	for i := 1; i < len(items); i++ {
		if items[i-1].DisplayName > items[i].DisplayName {
			t.Errorf("ListEncodings not sorted: %q > %q", items[i-1].DisplayName, items[i].DisplayName)
		}
	}

	// Phase 6 adds 21 portable multibyte/stateful and residual exact mappings.
	if len(items) != 168 {
		t.Errorf("ListEncodings() returned %d items, want 168", len(items))
	}
}

func TestPhase4XTextCodecSetIncludingUTF32(t *testing.T) {
	want := []string{
		"big5", "euc-jp", "euc-kr", "gb18030", "gbk", "hz-gb-2312",
		"ibm037", "ibm1047", "ibm1140", "ibm437", "ibm850", "ibm852", "ibm855", "ibm858", "ibm860", "ibm862", "ibm863", "ibm865", "ibm866",
		"iso-2022-jp", "iso-8859-1", "iso-8859-10", "iso-8859-13", "iso-8859-14", "iso-8859-15", "iso-8859-16", "iso-8859-2", "iso-8859-3", "iso-8859-4", "iso-8859-5", "iso-8859-6", "iso-8859-6-e", "iso-8859-6-i", "iso-8859-7", "iso-8859-8", "iso-8859-8-e", "iso-8859-8-i", "iso-8859-9",
		"koi8-r", "koi8-u", "macintosh", "shift_jis", "utf-16-be", "utf-16-le", "utf-32-be", "utf-32-le", "utf-8",
		"windows-1250", "windows-1251", "windows-1252", "windows-1253", "windows-1254", "windows-1255", "windows-1256", "windows-1257", "windows-1258", "windows-874",
		"x-mac-cyrillic", "x-user-defined",
	}
	gotSet := make(map[string]bool, len(ListEncodings()))
	for _, item := range ListEncodings() {
		gotSet[item.Name] = true
	}
	for _, name := range want {
		if !gotSet[name] {
			t.Fatalf("phase-4 x/text codec %q disappeared from the expanded registry", name)
		}
	}
	if len(want) != 59 {
		t.Fatalf("phase-4 baseline contains %d codecs, want 59", len(want))
	}
}

func TestPhase3XTextAliasNormalization(t *testing.T) {
	tests := map[string]string{
		"CP437":           "ibm437",
		"IBM00858":        "ibm858",
		"ISO_8859-6-E":    "iso-8859-6-e",
		"latin3":          "iso-8859-3",
		"x-mac-roman":     "macintosh",
		"x-mac-ukrainian": "x-mac-cyrillic",
		"windows-31j":     "shift_jis",
		"cp932":           "shift_jis",
		"windows-949":     "euc-kr",
		"cp949":           "euc-kr",
		"big5-hkscs":      "big5",
		"cp950":           "big5",
		"Extended_UNIX_Code_Packed_Format_for_Japanese": "euc-jp",
	}
	for input, want := range tests {
		got, ok := CanonicalName(input)
		if !ok || got != want {
			t.Errorf("CanonicalName(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}
}

func TestPhase3IncludesEveryXTextCharmapWithStrictByteSemantics(t *testing.T) {
	if len(charmap.All) != 46 {
		t.Fatalf("pinned x/text charmap.All contains %d encodings, want 46", len(charmap.All))
	}

	for _, base := range charmap.All {
		var descriptor *EncodingDescriptor
		for index := range encodingDescriptors {
			candidate := &encodingDescriptors[index]
			if candidate.Supported && candidate.xTextEncoding == base {
				if descriptor != nil {
					t.Fatalf("x/text charmap %q is registered more than once", base)
				}
				descriptor = candidate
			}
		}
		if descriptor == nil {
			t.Fatalf("x/text charmap %q is missing from the public registry", base)
		}

		t.Run(descriptor.Name, func(t *testing.T) {
			for value := 0; value <= 0xff; value++ {
				source := []byte{byte(value)}
				baseDecoded, baseErr := base.NewDecoder().Bytes(source)
				if baseErr != nil {
					t.Fatalf("base decoder rejected byte 0x%02x: %v", value, baseErr)
				}
				strictDecoded, strictErr := descriptor.Encoding.NewDecoder().Bytes(source)
				if bytes.Contains(baseDecoded, []byte("\ufffd")) {
					if strictErr == nil {
						t.Fatalf("undefined byte 0x%02x decoded without error", value)
					}
					continue
				}
				if strictErr != nil {
					t.Fatalf("valid byte 0x%02x rejected: %v", value, strictErr)
				}
				if !bytes.Equal(strictDecoded, baseDecoded) {
					t.Fatalf("byte 0x%02x decoded to %x, want %x", value, strictDecoded, baseDecoded)
				}

				baseEncoded, baseEncodeErr := base.NewEncoder().Bytes(baseDecoded)
				strictEncoded, strictEncodeErr := descriptor.Encoding.NewEncoder().Bytes(baseDecoded)
				if (baseEncodeErr == nil) != (strictEncodeErr == nil) {
					t.Fatalf("encoder disagreement for byte 0x%02x: base=%v strict=%v", value, baseEncodeErr, strictEncodeErr)
				}
				if baseEncodeErr == nil && !bytes.Equal(strictEncoded, baseEncoded) {
					t.Fatalf("encoded rune for byte 0x%02x = %x, want %x", value, strictEncoded, baseEncoded)
				}
			}
		})
	}
}

func TestIndexedAliasesNeverExposeStillUnregisteredOrAmbiguousCodecs(t *testing.T) {
	for _, name := range []string{
		"utf-32", "utf-16", "unicode", "csunicode", "csutf16", "ucs-2", "iso-10646-ucs-2", "unicodefeff", "unicodefffe",
	} {
		if canonical, ok := CanonicalName(name); ok {
			t.Fatalf("CanonicalName(%q) unexpectedly exposed %q", name, canonical)
		}
		if _, ok := Get(name); ok {
			t.Fatalf("Get(%q) unexpectedly succeeded", name)
		}
	}
}

func TestPhase3LineEndingCodeUnitsComeFromRegistry(t *testing.T) {
	for _, item := range ListEncodings() {
		cr, lf, ok := LineEndingBytes(item.Name)
		if !ok {
			t.Fatalf("LineEndingBytes(%q) is unavailable", item.Name)
		}
		wantLength := 1
		switch item.Name {
		case "utf-16-le", "utf-16-be":
			wantLength = 2
		case "utf-32-le", "utf-32-be":
			wantLength = 4
		}
		if len(cr) != wantLength || len(lf) != wantLength {
			t.Fatalf("LineEndingBytes(%q) lengths = %d/%d, want %d/%d", item.Name, len(cr), len(lf), wantLength, wantLength)
		}
	}
}

func TestPhase4UTF32CapabilitiesAreFullyExposed(t *testing.T) {
	for _, name := range []string{"utf-32-le", "utf32be"} {
		descriptor, ok := LookupDescriptor(name)
		if !ok {
			t.Fatalf("LookupDescriptor(%q) not found", name)
		}
		if !descriptor.Supported || !descriptor.Readable || !descriptor.Writable || !descriptor.AutoDetectable || descriptor.ExplicitOnly {
			t.Fatalf("%q is not a fully supported auto-detectable text codec: %+v", name, descriptor)
		}
		if descriptor.Encoding == nil || len(descriptor.BOM) != 4 || !descriptor.AutoBOM {
			t.Fatalf("%q codec/BOM metadata is incomplete: %+v", name, descriptor)
		}
		if _, ok := Get(name); !ok {
			t.Fatalf("Get(%q) failed", name)
		}
		if canonical, ok := CanonicalName(name); !ok || canonical != descriptor.Name {
			t.Fatalf("CanonicalName(%q) = %q, %v; want %q, true", name, canonical, ok, descriptor.Name)
		}
		if canonical, ok := CanonicalBOMName(name); !ok || canonical != descriptor.Name {
			t.Fatalf("CanonicalBOMName(%q) = %q, %v; want %q, true", name, canonical, ok, descriptor.Name)
		}
	}
}

func TestListEncodingsPublishesCapabilityMetadata(t *testing.T) {
	for _, item := range ListEncodings() {
		if !item.Readable || !item.Writable {
			t.Fatalf("supported item lacks read/write capability: %+v", item)
		}
		if item.AutoDetectable == item.ExplicitOnly {
			t.Fatalf("item must be exactly auto-detectable or explicit-only: %+v", item)
		}
		switch item.Name {
		case "utf-8":
			if !item.HasBOM || item.AutoBOM {
				t.Fatalf("UTF-8 BOM metadata = has:%v auto:%v, want true/false", item.HasBOM, item.AutoBOM)
			}
		case "utf-16-le", "utf-16-be", "utf-32-le", "utf-32-be":
			if !item.HasBOM || !item.AutoBOM {
				t.Fatalf("%s BOM metadata = has:%v auto:%v, want true/true", item.Name, item.HasBOM, item.AutoBOM)
			}
		}
	}
}

func TestRegistryReturnsDefensiveBOMCopies(t *testing.T) {
	first := BOMBytesFor("utf-32-le")
	if len(first) != 4 {
		t.Fatalf("UTF-32 LE BOM length = %d, want 4", len(first))
	}
	first[0] = 0
	second := BOMBytesFor("utf32le")
	if len(second) != 4 || second[0] != 0xFF {
		t.Fatalf("registry BOM was mutated through caller-owned slice: %x", second)
	}

	descriptor, ok := LookupDescriptor("utf-8")
	if !ok || len(descriptor.BOM) != 3 {
		t.Fatalf("UTF-8 descriptor BOM = %x, found=%v", descriptor.BOM, ok)
	}
	descriptor.BOM[0] = 0
	if got := BOMBytesFor("utf-8"); len(got) != 3 || got[0] != 0xEF {
		t.Fatalf("descriptor copy mutated registry BOM: %x", got)
	}
}

func TestDetectorLabelsHaveExplicitDisposition(t *testing.T) {
	labels := []string{
		"US-ASCII", "UTF-8", "UTF-8-SIG", "UTF-16", "UTF-16LE", "UTF-16BE",
		"UTF-32", "UTF-32LE", "UTF-32BE", "X-ISO-10646-UCS-4-3412", "X-ISO-10646-UCS-4-2143",
		"GB2312", "HZ-GB-2312", "Shift_JIS", "Big5", "KS_C_5601-1987", "KOI8-R", "TIS-620",
		"macintosh", "x-mac-cyrillic", "EUC-TW", "EUC-KR", "EUC-JP", "CP949",
		"Windows-1250", "Windows-1251", "Windows-1252", "Windows-1253", "Windows-1254", "Windows-1255", "Windows-1256", "Windows-1257",
		"ISO-8859-1", "ISO-8859-2", "ISO-8859-5", "ISO-8859-6", "ISO-8859-7", "ISO-8859-8", "ISO-8859-9", "ISO-8859-13",
		"ISO-2022-CN", "ISO-2022-JP", "ISO-2022-KR", "IBM855", "IBM866",
	}
	for _, label := range labels {
		if !detectorLabelHasDisposition(label) {
			t.Errorf("detector label %q has no registry disposition", label)
		}
	}
	if detectorLabelHasDisposition("future-unknown-detector-label") {
		t.Fatal("unknown detector label unexpectedly has a disposition")
	}
	if got := canonicalDetectedCharset("future-unknown-detector-label"); got != "" {
		t.Fatalf("unknown detector label canonicalized to %q, want rejection", got)
	}
}

func TestCanonicalDetectedCharsetUsesSupportedRegistryClosure(t *testing.T) {
	tests := []struct {
		label string
		want  string
	}{
		{label: "US-ASCII", want: "utf-8"},
		{label: "UTF-8", want: "utf-8"},
		{label: "Windows-1251", want: "windows-1251"},
		{label: "ISO-8859-5", want: "iso-8859-5"},
		{label: "TIS-620", want: "windows-874"},
		{label: "GB2312", want: "gbk"},
		{label: "Big5", want: "big5"},
		{label: "EUC-JP", want: "euc-jp"},
		{label: "ISO-2022-JP", want: "iso-2022-jp"},
		{label: "Shift_JIS", want: "shift_jis"},
		{label: "EUC-KR", want: "euc-kr"},
		{label: "CP949", want: "euc-kr"},
		{label: "HZ-GB-2312", want: "hz-gb-2312"},
		{label: "ISO-2022-KR", want: "iso-2022-kr"},
		{label: "IBM855", want: "ibm855"},
		{label: "macintosh", want: "macintosh"},
		{label: "x-mac-cyrillic", want: "x-mac-cyrillic"},
		{label: "ISO-8859-8", want: "iso-8859-8"},
		{label: "Windows-1250", want: ""},
		{label: "ISO-8859-2", want: ""},
		{label: "Windows-1256", want: ""},
		{label: "Windows-1257", want: ""},
		{label: "CP932", want: ""},
		{label: "KS_C_5601-1987", want: ""},
		{label: "UTF-16LE", want: ""},
		{label: "UTF-32LE", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			if got := canonicalDetectedCharset(tt.label); got != tt.want {
				t.Fatalf("canonicalDetectedCharset(%q) = %q, want %q", tt.label, got, tt.want)
			}
		})
	}
}
