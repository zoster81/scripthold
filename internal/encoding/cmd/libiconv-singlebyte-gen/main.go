package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const expectedLibiconvRevision = "9d19c66d0a1768cffcf497b2db70bf4018b578d7"

var definitionFiles = []string{
	"encodings.def",
	"encodings_aix.def",
	"encodings_dos.def",
	"encodings_extra.def",
	"encodings_zos.def",
}

var selectedIDs = []string{
	"atarist",
	"cp1046", "cp1124", "cp1125", "cp1129", "cp1131", "cp1133",
	"cp737", "cp775", "cp853", "cp856", "cp857", "cp861", "cp864", "cp869", "cp922",
	"ebcdic1025", "ebcdic1026", "ebcdic1097", "ebcdic1112", "ebcdic1122", "ebcdic1123", "ebcdic1130", "ebcdic1132", "ebcdic1137",
	"ebcdic1141", "ebcdic1142", "ebcdic1143", "ebcdic1144", "ebcdic1145", "ebcdic1146", "ebcdic1147", "ebcdic1148", "ebcdic1149",
	"ebcdic1153", "ebcdic1154", "ebcdic1155", "ebcdic1156", "ebcdic1157", "ebcdic1158",
	"ebcdic1164", "ebcdic1165", "ebcdic1166", "ebcdic12712", "ebcdic16804",
	"ebcdic273", "ebcdic277", "ebcdic278", "ebcdic280", "ebcdic282", "ebcdic284", "ebcdic285", "ebcdic297",
	"ebcdic423", "ebcdic424", "ebcdic425", "ebcdic4971", "ebcdic500", "ebcdic870", "ebcdic871", "ebcdic875", "ebcdic880", "ebcdic905", "ebcdic924",
	"georgian_academy", "georgian_ps", "hp_roman8", "iso646_cn", "iso646_jp", "jisx0201", "koi8_t", "mulelao",
	"mac_arabic", "mac_centraleurope", "mac_croatian", "mac_greek", "mac_hebrew", "mac_iceland", "mac_romania", "mac_thai", "mac_turkish", "mac_ukraine",
	"nextstep", "pt154", "riscos1", "rk1048", "tds565", "viscii",
}

type definition struct {
	preferred string
	id        string
	file      string
	names     []string
}

type generatedSpec struct {
	canonical    string
	display      string
	sourceName   string
	sourceID     string
	definition   string
	headerSHA256 string
	aliases      []string
	decode       [256]string
}

func main() {
	source := flag.String("source", "", "path to the pinned GNU libiconv checkout")
	output := flag.String("output", "", "generated Go output path")
	gcc := flag.String("gcc", "gcc", "C compiler used only by this maintainer generator")
	check := flag.Bool("check", false, "verify that output is already up to date")
	flag.Parse()

	if strings.TrimSpace(*source) == "" || strings.TrimSpace(*output) == "" {
		fatalf("-source and -output are required")
	}
	absoluteSource, err := filepath.Abs(*source)
	if err != nil {
		fatalf("resolve source: %v", err)
	}
	absoluteOutput, err := filepath.Abs(*output)
	if err != nil {
		fatalf("resolve output: %v", err)
	}
	if err := verifyRevision(absoluteSource); err != nil {
		fatalf("verify source revision: %v", err)
	}
	definitions, err := loadDefinitions(absoluteSource)
	if err != nil {
		fatalf("load definitions: %v", err)
	}
	if err := validateSelection(definitions); err != nil {
		fatalf("validate selection: %v", err)
	}

	work, err := os.MkdirTemp("", "scripthold-libiconv-singlebyte-")
	if err != nil {
		fatalf("create temporary directory: %v", err)
	}
	defer os.RemoveAll(work)

	specs := make([]generatedSpec, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		def := definitions[id]
		spec, err := probeDefinition(absoluteSource, work, *gcc, def)
		if err != nil {
			fatalf("probe %s: %v", id, err)
		}
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].canonical < specs[j].canonical })

	generated, err := render(specs)
	if err != nil {
		fatalf("render generated source: %v", err)
	}
	if *check {
		current, err := os.ReadFile(absoluteOutput)
		if err != nil {
			fatalf("read generated output: %v", err)
		}
		if !bytes.Equal(current, generated) {
			fatalf("generated output is stale: %s", absoluteOutput)
		}
		return
	}
	if err := writeAtomic(absoluteOutput, generated); err != nil {
		fatalf("write generated output: %v", err)
	}
}

func verifyRevision(source string) error {
	cmd := exec.Command("git", "-C", source, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	got := strings.TrimSpace(string(output))
	if got != expectedLibiconvRevision {
		return fmt.Errorf("revision %s, want %s", got, expectedLibiconvRevision)
	}
	return nil
}

func loadDefinitions(source string) (map[string]definition, error) {
	blockRE := regexp.MustCompile(`(?s)DEFENCODING\(\((.*?)\),\s*([A-Za-z0-9_]+),`)
	commentRE := regexp.MustCompile(`(?s)/\*.*?\*/`)
	nameRE := regexp.MustCompile(`"([^"]+)"`)
	definitions := make(map[string]definition)
	for _, file := range definitionFiles {
		path := filepath.Join(source, "lib", file)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		for _, match := range blockRE.FindAllSubmatch(data, -1) {
			clean := commentRE.ReplaceAll(match[1], nil)
			nameMatches := nameRE.FindAllSubmatch(clean, -1)
			if len(nameMatches) == 0 {
				// Some libiconv definition blocks contain only commented or
				// platform-conditional names. They are not active public names;
				// validateSelection still fails if a selected converter is absent.
				continue
			}
			id := string(match[2])
			if _, exists := definitions[id]; exists {
				return nil, fmt.Errorf("duplicate converter id %s", id)
			}
			names := make([]string, 0, len(nameMatches))
			for _, nameMatch := range nameMatches {
				names = append(names, string(nameMatch[1]))
			}
			definitions[id] = definition{preferred: names[0], id: id, file: file, names: names}
		}
	}
	return definitions, nil
}

func validateSelection(definitions map[string]definition) error {
	seen := make(map[string]struct{}, len(selectedIDs))
	for _, id := range selectedIDs {
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("duplicate selected converter %s", id)
		}
		seen[id] = struct{}{}
		if _, ok := definitions[id]; !ok {
			return fmt.Errorf("selected converter %s is absent from pinned definitions", id)
		}
	}
	if len(seen) != 88 {
		return fmt.Errorf("selected converter count %d, want 88", len(seen))
	}
	return nil
}

func probeDefinition(source, work, gcc string, def definition) (generatedSpec, error) {
	header := filepath.Join(source, "lib", def.id+".h")
	headerBytes, err := os.ReadFile(header)
	if err != nil {
		return generatedSpec{}, err
	}
	headerPath := filepath.ToSlash(header)
	cSource := fmt.Sprintf(`#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <limits.h>
typedef unsigned int ucs4_t;
typedef unsigned int state_t;
struct conv_struct { unsigned int isurface; state_t istate; unsigned int osurface; state_t ostate; };
typedef struct conv_struct * conv_t;
#define RET_SHIFT_ILSEQ(n) (-1-2*(n))
#define RET_ILSEQ RET_SHIFT_ILSEQ(0)
#define RET_TOOFEW(n) (-2-2*(n))
#define RET_ILUNI -1
#define RET_TOOSMALL -2
#define ICONV_SURFACE_EBCDIC_ZOS_UNIX 1U
static unsigned char swap_x15_x25(unsigned char x) { return (unsigned char)(x ^ ((((x)-0x15)&~0x10)==0 ? 0x30 : 0)); }
typedef struct { unsigned short indx; unsigned short used; } Summary16;
#include %s
int main(void) {
  for (int i = 0; i < 256; i++) {
    struct conv_struct decoder = {0};
    unsigned char input[8] = {0};
    input[0] = (unsigned char)i;
    ucs4_t wc = 0;
    int decoded = %s_mbtowc(&decoder, &wc, input, 1);
    if (decoded == RET_ILSEQ) { puts("UNDEF"); continue; }
    if (decoded != 1) { fprintf(stderr, "decoder consumed %%d bytes at 0x%%02X\n", decoded, i); return 3; }
    struct conv_struct encoder = {0};
    unsigned char output[8] = {0};
    int encoded = %s_wctomb(&encoder, output, wc, sizeof(output));
    if (encoded != 1 || output[0] != (unsigned char)i) {
      fprintf(stderr, "round-trip mismatch at 0x%%02X: encoded=%%d output=0x%%02X scalar=U+%%04X\n", i, encoded, encoded > 0 ? output[0] : 0, wc);
      return 4;
    }
    printf("%%08X\n", wc);
  }
  return 0;
}
`, strconv.Quote(headerPath), def.id, def.id)

	cPath := filepath.Join(work, def.id+".c")
	executable := filepath.Join(work, def.id)
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	if err := os.WriteFile(cPath, []byte(cSource), 0o600); err != nil {
		return generatedSpec{}, err
	}
	compile := exec.Command(gcc, "-std=c99", "-O2", "-I", filepath.Join(source, "lib"), cPath, "-o", executable)
	if output, err := compile.CombinedOutput(); err != nil {
		return generatedSpec{}, fmt.Errorf("compile failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	cmd := exec.Command(executable)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return generatedSpec{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return generatedSpec{}, err
	}
	scanner := bufio.NewScanner(stdout)
	var decode [256]string
	index := 0
	for scanner.Scan() {
		if index >= len(decode) {
			return generatedSpec{}, fmt.Errorf("probe returned more than 256 rows")
		}
		decode[index] = strings.TrimSpace(scanner.Text())
		index++
	}
	if err := scanner.Err(); err != nil {
		return generatedSpec{}, err
	}
	if err := cmd.Wait(); err != nil {
		return generatedSpec{}, fmt.Errorf("probe failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if index != len(decode) {
		return generatedSpec{}, fmt.Errorf("probe returned %d rows, want 256", index)
	}
	for value, scalar := range decode {
		if scalar == "UNDEF" {
			continue
		}
		if _, err := strconv.ParseUint(scalar, 16, 32); err != nil {
			return generatedSpec{}, fmt.Errorf("byte 0x%02X returned invalid scalar %q", value, scalar)
		}
	}

	sum := sha256.Sum256(headerBytes)
	return generatedSpec{
		canonical:    canonicalName(def.id),
		display:      def.preferred,
		sourceName:   def.preferred,
		sourceID:     def.id,
		definition:   def.file,
		headerSHA256: fmt.Sprintf("%x", sum[:]),
		aliases:      aliasesFor(def),
		decode:       decode,
	}, nil
}

func canonicalName(id string) string {
	if match := regexp.MustCompile(`^cp([0-9]+)$`).FindStringSubmatch(id); match != nil {
		return "ibm" + match[1]
	}
	if match := regexp.MustCompile(`^ebcdic([0-9]+)$`).FindStringSubmatch(id); match != nil {
		return "ibm" + match[1]
	}
	switch id {
	case "iso646_cn":
		return "gb-1988-80"
	case "iso646_jp":
		return "jis-c6220-1969-ro"
	case "jisx0201":
		return "jis-x0201"
	case "georgian_academy":
		return "georgian-academy"
	case "georgian_ps":
		return "georgian-ps"
	case "hp_roman8":
		return "hp-roman8"
	case "koi8_t":
		return "koi8-t"
	case "mulelao":
		return "mulelao-1"
	case "mac_centraleurope":
		return "mac-central-europe"
	case "riscos1":
		return "riscos-latin1"
	default:
		return strings.ReplaceAll(id, "_", "-")
	}
}

func aliasesFor(def definition) []string {
	canonical := canonicalName(def.id)
	seen := map[string]struct{}{canonical: {}}
	aliases := make([]string, 0, len(def.names)+2)
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		aliases = append(aliases, value)
	}
	for _, name := range def.names {
		add(name)
	}
	if strings.HasPrefix(canonical, "ibm") {
		number := strings.TrimPrefix(canonical, "ibm")
		add("cp" + number)
		add("ibm-" + number)
	}
	sort.Strings(aliases)
	return aliases
}

func render(specs []generatedSpec) ([]byte, error) {
	var out bytes.Buffer
	fmt.Fprintln(&out, "// Code generated by internal/encoding/cmd/libiconv-singlebyte-gen; DO NOT EDIT.")
	fmt.Fprintf(&out, "// Source: GNU libiconv revision %s.\n", expectedLibiconvRevision)
	fmt.Fprintln(&out, "// The generated mappings are pure Go runtime data; libiconv and GCC are maintainer-only generation oracles.")
	fmt.Fprintln(&out, "// Mapping provenance and the pinned libiconv license are recorded under filetoolsserver/handler/testdata/internet-corpus/.")
	fmt.Fprintln(&out, "package encoding")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "const generatedLibiconvRevision = %q\n\n", expectedLibiconvRevision)
	fmt.Fprintln(&out, "var generatedLibiconvSingleByteSpecs = []libiconvSingleByteSpec{")
	for _, spec := range specs {
		fmt.Fprintln(&out, "\t{")
		fmt.Fprintf(&out, "\t\tCanonicalName: %q,\n", spec.canonical)
		fmt.Fprintf(&out, "\t\tDisplayName: %q,\n", spec.display)
		fmt.Fprintf(&out, "\t\tSourceName: %q,\n", spec.sourceName)
		fmt.Fprintf(&out, "\t\tSourceID: %q,\n", spec.sourceID)
		fmt.Fprintf(&out, "\t\tSourceDefinition: %q,\n", spec.definition)
		fmt.Fprintf(&out, "\t\tSourceHeaderSHA256: %q,\n", spec.headerSHA256)
		fmt.Fprint(&out, "\t\tAliases: []string{")
		for index, alias := range spec.aliases {
			if index > 0 {
				fmt.Fprint(&out, ", ")
			}
			fmt.Fprintf(&out, "%q", alias)
		}
		fmt.Fprintln(&out, "},")
		fmt.Fprintln(&out, "\t\tDecode: [256]rune{")
		for row := 0; row < 32; row++ {
			fmt.Fprint(&out, "\t\t\t")
			for column := 0; column < 8; column++ {
				index := row*8 + column
				if spec.decode[index] == "UNDEF" {
					fmt.Fprint(&out, "undefinedSingleByteRune,")
				} else {
					fmt.Fprintf(&out, "0x%s,", spec.decode[index])
				}
				if column != 7 {
					fmt.Fprint(&out, " ")
				}
			}
			fmt.Fprintln(&out)
		}
		fmt.Fprintln(&out, "\t\t},")
		fmt.Fprintln(&out, "\t},")
	}
	fmt.Fprintln(&out, "}")
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return nil, err
	}
	return formatted, nil
}

func writeAtomic(path string, data []byte) error {
	directory := filepath.Dir(path)
	temp, err := os.CreateTemp(directory, ".libiconv-singlebyte-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempName)
		}
	}()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "libiconv-singlebyte-gen: "+format+"\n", args...)
	os.Exit(1)
}
