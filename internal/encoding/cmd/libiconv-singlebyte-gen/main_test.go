package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSelectedIDsAreUniqueAndComplete(t *testing.T) {
	if len(selectedIDs) != 88 {
		t.Fatalf("selectedIDs = %d, want 88", len(selectedIDs))
	}
	seen := make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		if seen[id] {
			t.Fatalf("duplicate selected id %q", id)
		}
		seen[id] = true
	}
}

func TestCanonicalName(t *testing.T) {
	tests := map[string]string{
		"cp737":             "ibm737",
		"ebcdic273":         "ibm273",
		"iso646_cn":         "gb-1988-80",
		"iso646_jp":         "jis-c6220-1969-ro",
		"jisx0201":          "jis-x0201",
		"mac_centraleurope": "mac-central-europe",
		"mac_ukraine":       "mac-ukraine",
		"riscos1":           "riscos-latin1",
		"viscii":            "viscii",
	}
	for input, want := range tests {
		if got := canonicalName(input); got != want {
			t.Errorf("canonicalName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLoadDefinitionsIgnoresCommentedAliases(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := `DEFENCODING(( "CP737",
              "IBM737",
            /*"COMMENTED-OUT",*/
            ),
            cp737,
            { cp737_mbtowc, NULL }, { cp737_wctomb, NULL })
`
	for _, name := range definitionFiles {
		content := ""
		if name == "encodings.def" {
			content = fixture
		}
		if err := os.WriteFile(filepath.Join(lib, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	definitions, err := loadDefinitions(root)
	if err != nil {
		t.Fatal(err)
	}
	got := definitions["cp737"]
	if got.preferred != "CP737" || !slices.Equal(got.names, []string{"CP737", "IBM737"}) {
		t.Fatalf("definition = %+v", got)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	var decode [256]string
	for index := range decode {
		decode[index] = "UNDEF"
	}
	decode[0x41] = "00000041"
	specs := []generatedSpec{{
		canonical:    "ibm737",
		display:      "CP737",
		sourceName:   "CP737",
		sourceID:     "cp737",
		definition:   "encodings_dos.def",
		headerSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		aliases:      []string{"cp737", "ibm-737"},
		decode:       decode,
	}}
	first, err := render(specs)
	if err != nil {
		t.Fatal(err)
	}
	second, err := render(specs)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(first, second) {
		t.Fatal("render output is not deterministic")
	}
}

func TestAliasesForIncludesSourceAndIBMForms(t *testing.T) {
	got := aliasesFor(definition{
		preferred: "CP737",
		id:        "cp737",
		names:     []string{"CP737", "IBM737"},
	})
	want := []string{"cp737", "ibm-737"}
	if !slices.Equal(got, want) {
		t.Fatalf("aliases = %v, want %v", got, want)
	}
}
