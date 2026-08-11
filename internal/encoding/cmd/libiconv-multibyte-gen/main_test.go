package main

import (
	"bytes"
	"slices"
	"testing"
)

func TestCandidatesAreUniqueAndComplete(t *testing.T) {
	if len(candidates) != 20 {
		t.Fatalf("candidates = %d, want 20", len(candidates))
	}
	seenID := make(map[string]bool, len(candidates))
	seenCanonical := make(map[string]bool, len(candidates))
	for _, item := range candidates {
		if item.id == "" || item.canonical == "" || item.header == "" || item.kind == "" {
			t.Fatalf("candidate has incomplete metadata: %+v", item)
		}
		if seenID[item.id] {
			t.Fatalf("duplicate candidate id %q", item.id)
		}
		if seenCanonical[item.canonical] {
			t.Fatalf("duplicate candidate canonical name %q", item.canonical)
		}
		seenID[item.id] = true
		seenCanonical[item.canonical] = true
	}
}

func TestAliasesForPreservesExistingBig5HKSCSOwnership(t *testing.T) {
	item := candidate{
		id:             "big5hkscs2008",
		canonical:      "big5-hkscs-2008",
		excludeAliases: []string{"big5-hkscs", "big5hkscs"},
	}
	got := aliasesFor(item, definition{
		preferred: "BIG5-HKSCS",
		id:        "big5hkscs2008",
		names:     []string{"BIG5-HKSCS", "BIG5HKSCS", "BIG5-HKSCS:2008"},
	})
	want := []string{"big5-hkscs:2008"}
	if !slices.Equal(got, want) {
		t.Fatalf("aliases = %v, want %v", got, want)
	}
}

func TestParseHexBytes(t *testing.T) {
	got, err := parseHexBytes("1B24422821")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x1b, 0x24, 0x42, 0x28, 0x21}
	if !bytes.Equal(got, want) {
		t.Fatalf("parseHexBytes = %x, want %x", got, want)
	}
	for _, invalid := range []string{"", "0", "GG", "123"} {
		if _, err := parseHexBytes(invalid); err == nil {
			t.Fatalf("parseHexBytes(%q) unexpectedly succeeded", invalid)
		}
	}
}

func TestParseScalarRejectsInvalidUnicode(t *testing.T) {
	for _, value := range []string{"D800", "DFFF", "110000", "xyz"} {
		if _, err := parseScalar(value); err == nil {
			t.Fatalf("parseScalar(%q) unexpectedly succeeded", value)
		}
	}
	if got, err := parseScalar("10FFFF"); err != nil || got != 0x10ffff {
		t.Fatalf("parseScalar(10FFFF) = %X, %v", got, err)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	specs := []generatedSpec{{
		candidate: candidate{
			id:        "euc_tw",
			kind:      "direct",
			canonical: "euc-tw",
			display:   "EUC-TW",
		},
		sourceName:   "EUC-TW",
		definition:   "encodings.def",
		headerSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		aliases:      []string{"csEUCTW", "euctw"},
		decode: []decodeEntry{
			{bytes: []byte{0x41}, runes: []uint32{0x41}},
		},
		encode: []encodeEntry{
			{r: 0x41, bytes: []byte{0x41}},
		},
	}}
	raw := []rawSpec{{
		name:  "jis0208",
		width: 2,
		entries: []decodeEntry{
			{bytes: []byte{0x21, 0x21}, runes: []uint32{0x3000}},
		},
	}}
	patches := gb18030PatchSet{headerSHA256: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"}
	first, err := render(specs, raw, patches)
	if err != nil {
		t.Fatal(err)
	}
	second, err := render(specs, raw, patches)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("render output is not deterministic")
	}
}

func TestValidateExactSingleByteMappingRejectsSemanticButNotByteRoundTrip(t *testing.T) {
	decode := []decodeEntry{{bytes: []byte{0xA0}, runes: []uint32{0x0E48}}}
	encode := []encodeEntry{{r: 0x0E48, bytes: []byte{0xE8}}}
	if err := validateExactSingleByteMapping("cp1161", decode, encode); err == nil {
		t.Fatal("non-byte-exact mapping unexpectedly passed")
	}
	encode[0].bytes = []byte{0xA0}
	if err := validateExactSingleByteMapping("cp1162", decode, encode); err != nil {
		t.Fatalf("exact mapping rejected: %v", err)
	}
}

func TestPackBytes(t *testing.T) {
	if got := packBytes([]byte{0x8e, 0xa2, 0xa1, 0xa1}); got != 0x8ea2a1a1 {
		t.Fatalf("packBytes = %08X", got)
	}
}
