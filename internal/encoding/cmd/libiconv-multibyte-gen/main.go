package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/base64"
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

var definitionFiles = []string{"encodings.def", "encodings_extra.def", "encodings_aix.def"}

type candidate struct {
	id              string
	header          string
	kind            string
	canonical       string
	display         string
	reset           string
	flush           string
	preIncludes     []string
	excludeAliases  []string
	exactSingleByte bool
}

var candidates = []candidate{
	{id: "big5_2003", header: "big5_2003.h", kind: "direct", canonical: "big5-2003", display: "BIG5-2003"},
	{id: "big5hkscs1999", header: "big5hkscs1999.h", kind: "direct", canonical: "big5-hkscs-1999", display: "BIG5-HKSCS:1999", reset: "big5hkscs1999_reset", flush: "big5hkscs1999_flushwc"},
	{id: "big5hkscs2001", header: "big5hkscs2001.h", kind: "direct", canonical: "big5-hkscs-2001", display: "BIG5-HKSCS:2001", reset: "big5hkscs2001_reset", flush: "big5hkscs2001_flushwc", preIncludes: []string{"hkscs1999.h"}},
	{id: "big5hkscs2004", header: "big5hkscs2004.h", kind: "direct", canonical: "big5-hkscs-2004", display: "BIG5-HKSCS:2004", reset: "big5hkscs2004_reset", flush: "big5hkscs2004_flushwc", preIncludes: []string{"hkscs1999.h", "hkscs2001.h"}},
	{id: "big5hkscs2008", header: "big5hkscs2008.h", kind: "direct", canonical: "big5-hkscs-2008", display: "BIG5-HKSCS:2008", reset: "big5hkscs2008_reset", flush: "big5hkscs2008_flushwc", preIncludes: []string{"hkscs1999.h", "hkscs2001.h", "hkscs2004.h"}, excludeAliases: []string{"big5-hkscs", "big5hkscs"}},
	{id: "euc_cn", header: "euc_cn.h", kind: "direct", canonical: "euc-cn", display: "EUC-CN", excludeAliases: []string{"gb2312", "csgb2312"}},
	{id: "euc_jisx0213", header: "euc_jisx0213.h", kind: "direct", canonical: "euc-jisx0213", display: "EUC-JISX0213", reset: "euc_jisx0213_reset", flush: "euc_jisx0213_flushwc"},
	{id: "euc_tw", header: "euc_tw.h", kind: "direct", canonical: "euc-tw", display: "EUC-TW"},
	{id: "cp1162", header: "cp1162.h", kind: "direct", canonical: "ibm1162", display: "IBM-1162", exactSingleByte: true},
	{id: "cp1163", header: "cp1163.h", kind: "direct", canonical: "ibm1163", display: "IBM-1163", exactSingleByte: true},
	{id: "iso2022_cn", header: "iso2022_cn.h", kind: "iso2022-cn", canonical: "iso-2022-cn", display: "ISO-2022-CN", reset: "iso2022_cn_reset"},
	{id: "iso2022_cn_ext", header: "iso2022_cnext.h", kind: "iso2022-cn-ext", canonical: "iso-2022-cn-ext", display: "ISO-2022-CN-EXT", reset: "iso2022_cn_ext_reset"},
	{id: "iso2022_jp1", header: "iso2022_jp1.h", kind: "iso2022-jp1", canonical: "iso-2022-jp-1", display: "ISO-2022-JP-1", reset: "iso2022_jp1_reset"},
	{id: "iso2022_jp2", header: "iso2022_jp2.h", kind: "iso2022-jp2", canonical: "iso-2022-jp-2", display: "ISO-2022-JP-2", reset: "iso2022_jp2_reset"},
	{id: "iso2022_jp3", header: "iso2022_jp3.h", kind: "iso2022-jp3", canonical: "iso-2022-jp-3", display: "ISO-2022-JP-3", reset: "iso2022_jp3_reset", flush: "iso2022_jp3_flushwc"},
	{id: "iso2022_jpms", header: "iso2022_jpms.h", kind: "iso2022-jpms", canonical: "iso-2022-jp-ms", display: "ISO-2022-JP-MS", reset: "iso2022_jpms_reset"},
	{id: "iso2022_kr", header: "iso2022_kr.h", kind: "iso2022-kr", canonical: "iso-2022-kr", display: "ISO-2022-KR", reset: "iso2022_kr_reset"},
	{id: "johab", header: "johab.h", kind: "direct", canonical: "johab", display: "JOHAB"},
	{id: "shift_jisx0213", header: "shift_jisx0213.h", kind: "direct", canonical: "shift_jisx0213", display: "SHIFT_JISX0213", reset: "shift_jisx0213_reset", flush: "shift_jisx0213_flushwc"},
	{id: "tcvn", header: "tcvn.h", kind: "tcvn", canonical: "tcvn", display: "TCVN", flush: "tcvn_flushwc"},
}

type definition struct {
	preferred string
	id        string
	file      string
	names     []string
}

type decodeEntry struct {
	bytes []byte
	runes []uint32
}

type encodeEntry struct {
	r     uint32
	bytes []byte
}

type pairEntry struct {
	first  uint32
	second uint32
	bytes  []byte
}

type generatedSpec struct {
	candidate
	sourceName   string
	definition   string
	headerSHA256 string
	aliases      []string
	decode       []decodeEntry
	encode       []encodeEntry
	pairs        []pairEntry
}

type rawSpec struct {
	name    string
	width   int
	entries []decodeEntry
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
	if err := validateCandidates(definitions); err != nil {
		fatalf("validate candidates: %v", err)
	}

	work, err := os.MkdirTemp(filepath.Dir(absoluteOutput), ".libiconv-multibyte-gen-")
	if err != nil {
		fatalf("create workspace-confined temporary directory: %v", err)
	}
	defer os.RemoveAll(work)

	specs := make([]generatedSpec, 0, len(candidates))
	for _, item := range candidates {
		def := definitions[item.id]
		spec, err := probeCandidate(absoluteSource, work, *gcc, item, def)
		if err != nil {
			fatalf("probe %s: %v", item.id, err)
		}
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].canonical < specs[j].canonical })

	raw, err := probeRawCharsets(absoluteSource, work, *gcc)
	if err != nil {
		fatalf("probe ISO-2022 raw charsets: %v", err)
	}
	gb18030Patches, err := probeGB180302022(absoluteSource, work, *gcc)
	if err != nil {
		fatalf("probe GB18030:2022 differential: %v", err)
	}
	generated, err := render(specs, raw, gb18030Patches)
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
		data, err := os.ReadFile(filepath.Join(source, "lib", file))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		for _, match := range blockRE.FindAllSubmatch(data, -1) {
			clean := commentRE.ReplaceAll(match[1], nil)
			nameMatches := nameRE.FindAllSubmatch(clean, -1)
			if len(nameMatches) == 0 {
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

func validateCandidates(definitions map[string]definition) error {
	if len(candidates) != 20 {
		return fmt.Errorf("candidate count %d, want 20", len(candidates))
	}
	seenID := make(map[string]bool, len(candidates))
	seenCanonical := make(map[string]bool, len(candidates))
	for _, item := range candidates {
		if seenID[item.id] || seenCanonical[item.canonical] {
			return fmt.Errorf("duplicate candidate id/canonical %s/%s", item.id, item.canonical)
		}
		seenID[item.id], seenCanonical[item.canonical] = true, true
		if _, ok := definitions[item.id]; !ok {
			return fmt.Errorf("candidate %s is absent from pinned definitions", item.id)
		}
		if strings.HasPrefix(item.kind, "iso2022-") && item.reset == "" {
			return fmt.Errorf("ISO-2022 candidate %s has no reset function", item.id)
		}
	}
	return nil
}

func aliasesFor(item candidate, def definition) []string {
	excluded := make(map[string]bool, len(item.excludeAliases))
	for _, value := range item.excludeAliases {
		excluded[strings.ToLower(value)] = true
	}
	seen := map[string]bool{strings.ToLower(item.canonical): true}
	var aliases []string
	for _, value := range def.names {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] || excluded[value] {
			continue
		}
		seen[value] = true
		aliases = append(aliases, value)
	}
	sort.Strings(aliases)
	return aliases
}

func probeCandidate(source, work, gcc string, item candidate, def definition) (generatedSpec, error) {
	headerPath := filepath.Join(source, "lib", item.header)
	headerBytes, err := os.ReadFile(headerPath)
	if err != nil {
		return generatedSpec{}, err
	}
	pre := make([]string, 0, len(item.preIncludes))
	for _, include := range item.preIncludes {
		pre = append(pre, fmt.Sprintf("#include %q", include))
	}
	decodeBody := ""
	switch item.kind {
	case "direct":
		decodeBody = "for (unsigned b=0;b<256;b++){ prefix[0]=(unsigned char)b; explore(prefix,1); if(failed)return 12; }"
	case "tcvn":
		decodeBody = "for(unsigned a=0;a<256;a++){ unsigned char one[1]={(unsigned char)a}; ucs4_t out[3]={0}; int n=decode_exact(one,1,out,3); if(n==1){printf(\"D %02X %X X\\n\",a,out[0]);} else if(n<0){fprintf(stderr,\"TCVN one-byte decode failed %02X ret=%d\\n\",a,n);return 12;} for(unsigned b=0;b<256;b++){ unsigned char pair[2]={(unsigned char)a,(unsigned char)b}; ucs4_t pout[3]={0}; int pn=decode_exact(pair,2,pout,3); if(pn==1){printf(\"D %02X%02X %X X\\n\",a,b,pout[0]);} else if(pn<0){continue;} } }"
	}
	resetBody := "if (c->ostate != 0) return -99; return 0;"
	if item.reset != "" {
		resetBody = fmt.Sprintf("int rr=%s(c,out,64); if(rr>=0)c->ostate=0; return rr;", item.reset)
	}
	flushBody := "return 0;"
	if item.flush != "" {
		flushBody = fmt.Sprintf("return %s(c,wc);", item.flush)
	}
	cSource := strings.NewReplacer(
		"__HEADER__", item.header,
		"__FN__", item.id,
		"__PRE__", strings.Join(pre, "\n"),
		"__DECODE__", decodeBody,
		"__RESET_BODY__", resetBody,
		"__FLUSH_BODY__", flushBody,
	).Replace(candidateProbeTemplate)
	cPath := filepath.Join(work, item.id+".c")
	executable := filepath.Join(work, item.id)
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
	var decode []decodeEntry
	var encode []encodeEntry
	var pairs []pairEntry
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			return generatedSpec{}, fmt.Errorf("malformed probe row %q", scanner.Text())
		}
		switch fields[0] {
		case "D":
			if len(fields) != 4 && len(fields) != 5 {
				return generatedSpec{}, fmt.Errorf("malformed decode row %q", scanner.Text())
			}
			sequence, err := parseHexBytes(fields[1])
			if err != nil {
				return generatedSpec{}, err
			}
			r1, err := parseScalar(fields[2])
			if err != nil {
				return generatedSpec{}, err
			}
			runes := []uint32{r1}
			if len(fields) == 5 {
				r2, err := parseScalar(fields[3])
				if err != nil {
					return generatedSpec{}, err
				}
				runes = append(runes, r2)
			}
			decode = append(decode, decodeEntry{bytes: sequence, runes: runes})
		case "E":
			if len(fields) != 3 {
				return generatedSpec{}, fmt.Errorf("malformed encode row %q", scanner.Text())
			}
			r, err := parseScalar(fields[1])
			if err != nil {
				return generatedSpec{}, err
			}
			encoded, err := parseHexBytes(fields[2])
			if err != nil {
				return generatedSpec{}, err
			}
			encode = append(encode, encodeEntry{r: r, bytes: encoded})
		case "P":
			if len(fields) != 4 {
				return generatedSpec{}, fmt.Errorf("malformed pair row %q", scanner.Text())
			}
			first, err := parseScalar(fields[1])
			if err != nil {
				return generatedSpec{}, err
			}
			second, err := parseScalar(fields[2])
			if err != nil {
				return generatedSpec{}, err
			}
			encoded, err := parseHexBytes(fields[3])
			if err != nil {
				return generatedSpec{}, err
			}
			pairs = append(pairs, pairEntry{first: first, second: second, bytes: encoded})
		default:
			return generatedSpec{}, fmt.Errorf("unknown probe row %q", scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		return generatedSpec{}, err
	}
	if err := cmd.Wait(); err != nil {
		return generatedSpec{}, fmt.Errorf("probe failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if (item.kind == "direct" || item.kind == "tcvn") && len(decode) == 0 {
		return generatedSpec{}, fmt.Errorf("codec produced no decode mappings")
	}
	if len(encode) == 0 {
		return generatedSpec{}, fmt.Errorf("codec produced no canonical encode mappings")
	}
	sortDecodeEntries(decode)
	sort.Slice(encode, func(i, j int) bool { return encode[i].r < encode[j].r })
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].first != pairs[j].first {
			return pairs[i].first < pairs[j].first
		}
		return pairs[i].second < pairs[j].second
	})
	if item.exactSingleByte {
		if err := validateExactSingleByteMapping(item.canonical, decode, encode); err != nil {
			return generatedSpec{}, err
		}
	}
	sum := sha256.Sum256(headerBytes)
	return generatedSpec{
		candidate:    item,
		sourceName:   def.preferred,
		definition:   def.file,
		headerSHA256: fmt.Sprintf("%x", sum[:]),
		aliases:      aliasesFor(item, def),
		decode:       decode,
		encode:       encode,
		pairs:        pairs,
	}, nil
}

func validateExactSingleByteMapping(name string, decode []decodeEntry, encode []encodeEntry) error {
	byRune := make(map[uint32][]byte, len(encode))
	for _, entry := range encode {
		byRune[entry.r] = entry.bytes
	}
	for _, entry := range decode {
		if len(entry.bytes) != 1 || len(entry.runes) != 1 {
			return fmt.Errorf("%s is not fixed single-byte", name)
		}
		encoded := byRune[entry.runes[0]]
		if len(encoded) != 1 || encoded[0] != entry.bytes[0] {
			return fmt.Errorf("%s byte 0x%02X is not exact-round-trippable", name, entry.bytes[0])
		}
	}
	return nil
}

func parseScalar(value string) (uint32, error) {
	parsed, err := strconv.ParseUint(value, 16, 32)
	if err != nil || parsed > 0x10ffff || parsed >= 0xd800 && parsed <= 0xdfff {
		return 0, fmt.Errorf("invalid generated scalar %q", value)
	}
	return uint32(parsed), nil
}

func parseHexBytes(value string) ([]byte, error) {
	if value == "" || len(value)%2 != 0 {
		return nil, fmt.Errorf("invalid generated byte string %q", value)
	}
	result := make([]byte, len(value)/2)
	for index := range result {
		parsed, err := strconv.ParseUint(value[index*2:index*2+2], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid generated byte string %q", value)
		}
		result[index] = byte(parsed)
	}
	return result, nil
}

func sortDecodeEntries(entries []decodeEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if len(entries[i].bytes) != len(entries[j].bytes) {
			return len(entries[i].bytes) < len(entries[j].bytes)
		}
		return packBytes(entries[i].bytes) < packBytes(entries[j].bytes)
	})
}

func packBytes(value []byte) uint32 {
	var packed uint32
	for index, b := range value {
		packed |= uint32(b) << uint(24-8*index)
	}
	return packed
}

func probeRawCharsets(source, work, gcc string) ([]rawSpec, error) {
	cPath := filepath.Join(work, "raw-charsets.c")
	executable := filepath.Join(work, "raw-charsets")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	if err := os.WriteFile(cPath, []byte(rawProbeSource), 0o600); err != nil {
		return nil, err
	}
	compile := exec.Command(gcc, "-std=c99", "-O2", "-I", filepath.Join(source, "lib"), cPath, "-o", executable)
	if output, err := compile.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("compile failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	cmd := exec.Command(executable)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	tables := make(map[string]*rawSpec)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 5 && len(fields) != 6 || fields[0] != "R" {
			return nil, fmt.Errorf("malformed raw probe row %q", scanner.Text())
		}
		width, err := strconv.Atoi(fields[2])
		if err != nil || width < 1 || width > 2 {
			return nil, fmt.Errorf("invalid raw width in %q", scanner.Text())
		}
		sequence, err := parseHexBytes(fields[3])
		if err != nil || len(sequence) != width {
			return nil, fmt.Errorf("invalid raw byte sequence in %q", scanner.Text())
		}
		r1, err := parseScalar(fields[4])
		if err != nil {
			return nil, err
		}
		runes := []uint32{r1}
		if len(fields) == 6 {
			r2, err := parseScalar(fields[5])
			if err != nil {
				return nil, err
			}
			runes = append(runes, r2)
		}
		table := tables[fields[1]]
		if table == nil {
			table = &rawSpec{name: fields[1], width: width}
			tables[fields[1]] = table
		} else if table.width != width {
			return nil, fmt.Errorf("raw table %s changed width", fields[1])
		}
		table.entries = append(table.entries, decodeEntry{bytes: sequence, runes: runes})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("raw probe failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	result := make([]rawSpec, 0, len(tables))
	for _, table := range tables {
		if len(table.entries) == 0 {
			return nil, fmt.Errorf("raw table %s is empty", table.name)
		}
		sortDecodeEntries(table.entries)
		result = append(result, *table)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result, nil
}

func render(specs []generatedSpec, raw []rawSpec, gb18030Patches gb18030PatchSet) ([]byte, error) {
	bundle, err := encodeBundle(specs, raw)
	if err != nil {
		return nil, err
	}
	var compressed bytes.Buffer
	zw, err := zlib.NewWriterLevel(&compressed, zlib.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(bundle); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(compressed.Bytes())
	bundleHash := sha256.Sum256(bundle)

	var out bytes.Buffer
	fmt.Fprintln(&out, "// Code generated by internal/encoding/cmd/libiconv-multibyte-gen; DO NOT EDIT.")
	fmt.Fprintf(&out, "// Source: GNU libiconv revision %s.\n", expectedLibiconvRevision)
	fmt.Fprintln(&out, "// Runtime data is pure Go; libiconv and GCC are maintainer-only generation oracles.")
	fmt.Fprintln(&out, "// Mapping provenance and the pinned libiconv license are recorded under filetoolsserver/handler/testdata/internet-corpus/.")
	fmt.Fprintf(&out, "// Bundle: %d bytes uncompressed, %d bytes zlib-compressed, SHA-256 %x.\n", len(bundle), compressed.Len(), bundleHash)
	for _, spec := range specs {
		fmt.Fprintf(&out, "// %s: source=%s headerSHA256=%s decode=%d encode=%d pairs=%d.\n", spec.canonical, spec.sourceName, spec.headerSHA256, len(spec.decode), len(spec.encode), len(spec.pairs))
	}
	for _, table := range raw {
		fmt.Fprintf(&out, "// raw %s: width=%d decode=%d.\n", table.name, table.width, len(table.entries))
	}
	fmt.Fprintf(&out, "// gb18030-2022: headerSHA256=%s decodePatches=%d encodePatches=%d; differential is exhaustive against pinned x/text v0.40.0.\n", gb18030Patches.headerSHA256, len(gb18030Patches.decode), len(gb18030Patches.encode))
	fmt.Fprintln(&out, "package encoding")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "const generatedLibiconvMultibyteRevision = %q\n\n", expectedLibiconvRevision)
	fmt.Fprintln(&out, "const generatedLibiconvMultibyteData =")
	for start := 0; start < len(encoded); start += 120 {
		end := start + 120
		if end > len(encoded) {
			end = len(encoded)
		}
		fmt.Fprintf(&out, "\t%q +\n", encoded[start:end])
	}
	fmt.Fprintln(&out, "\t\"\"")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "var generatedLibiconvMultibyteSpecs, generatedLibiconvRawCharsetSpecs =")
	fmt.Fprintln(&out, "\tmustDecodeGeneratedLibiconvMultibyteData(generatedLibiconvMultibyteData)")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "const generatedGB180302022HeaderSHA256 = %q\n\n", gb18030Patches.headerSHA256)
	fmt.Fprintln(&out, "var generatedGB180302022DecodePatches = []gb180302022DecodePatch{")
	for _, patch := range gb18030Patches.decode {
		fmt.Fprintf(&out, "\t{Packed: 0x%08X, Length: %d, Rune: 0x%X, Reject: %t},\n", packBytes(patch.bytes), len(patch.bytes), patch.r, patch.reject)
	}
	fmt.Fprintln(&out, "}")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "var generatedGB180302022EncodePatches = []gb180302022EncodePatch{")
	for _, patch := range gb18030Patches.encode {
		fmt.Fprintf(&out, "\t{Rune: 0x%X, Bytes: %s, Reject: %t},\n", patch.r, strconv.Quote(string(patch.bytes)), patch.reject)
	}
	fmt.Fprintln(&out, "}")
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return nil, err
	}
	return formatted, nil
}

func encodeBundle(specs []generatedSpec, raw []rawSpec) ([]byte, error) {
	writer := &bundleWriter{}
	writer.bytes([]byte("SHM6\x01"))
	if err := writer.u16(len(specs)); err != nil {
		return nil, err
	}
	for _, spec := range specs {
		for _, value := range []string{spec.canonical, spec.display, spec.sourceName, spec.id, spec.definition, spec.headerSHA256, spec.kind} {
			if err := writer.string16(value); err != nil {
				return nil, err
			}
		}
		if err := writer.u16(len(spec.aliases)); err != nil {
			return nil, err
		}
		for _, alias := range spec.aliases {
			if err := writer.string16(alias); err != nil {
				return nil, err
			}
		}
		if err := writer.decodeEntries(spec.decode); err != nil {
			return nil, fmt.Errorf("%s decode: %w", spec.canonical, err)
		}
		if err := writer.encodeEntries(spec.encode); err != nil {
			return nil, fmt.Errorf("%s encode: %w", spec.canonical, err)
		}
		if err := writer.pairEntries(spec.pairs); err != nil {
			return nil, fmt.Errorf("%s pairs: %w", spec.canonical, err)
		}
	}
	if err := writer.u16(len(raw)); err != nil {
		return nil, err
	}
	for _, table := range raw {
		if err := writer.string16(table.name); err != nil {
			return nil, err
		}
		if table.width < 1 || table.width > 2 {
			return nil, fmt.Errorf("raw table %s has invalid width %d", table.name, table.width)
		}
		writer.u8(byte(table.width))
		if err := writer.decodeEntries(table.entries); err != nil {
			return nil, fmt.Errorf("raw table %s: %w", table.name, err)
		}
	}
	return writer.data.Bytes(), nil
}

type bundleWriter struct {
	data bytes.Buffer
}

func (writer *bundleWriter) bytes(value []byte) {
	_, _ = writer.data.Write(value)
}

func (writer *bundleWriter) u8(value byte) {
	_ = writer.data.WriteByte(value)
}

func (writer *bundleWriter) u16(value int) error {
	if value < 0 || value > 0xffff {
		return fmt.Errorf("value %d exceeds uint16 bundle field", value)
	}
	writer.bytes([]byte{byte(value >> 8), byte(value)})
	return nil
}

func (writer *bundleWriter) u32(value uint32) {
	writer.bytes([]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)})
}

func (writer *bundleWriter) count32(value int) error {
	if value < 0 || uint64(value) > uint64(^uint32(0)) {
		return fmt.Errorf("count %d exceeds uint32 bundle field", value)
	}
	writer.u32(uint32(value))
	return nil
}

func (writer *bundleWriter) string16(value string) error {
	if len(value) > 0xffff {
		return fmt.Errorf("string length %d exceeds uint16 bundle field", len(value))
	}
	if err := writer.u16(len(value)); err != nil {
		return err
	}
	writer.bytes([]byte(value))
	return nil
}

func (writer *bundleWriter) decodeEntries(entries []decodeEntry) error {
	if err := writer.count32(len(entries)); err != nil {
		return err
	}
	var previous uint64
	for index, entry := range entries {
		if len(entry.bytes) < 1 || len(entry.bytes) > 4 || len(entry.runes) < 1 || len(entry.runes) > 2 {
			return fmt.Errorf("invalid decode entry at index %d", index)
		}
		packed := packBytes(entry.bytes)
		key := uint64(len(entry.bytes))<<32 | uint64(packed)
		if index > 0 && key <= previous {
			return fmt.Errorf("decode table is not strictly sorted at index %d", index)
		}
		previous = key
		writer.u32(packed)
		writer.u8(byte(len(entry.bytes)))
		writer.u32(entry.runes[0])
		if len(entry.runes) == 2 {
			writer.u32(entry.runes[1])
		} else {
			writer.u32(0)
		}
	}
	return nil
}

func (writer *bundleWriter) encodeEntries(entries []encodeEntry) error {
	if err := writer.count32(len(entries)); err != nil {
		return err
	}
	var previous uint32
	for index, entry := range entries {
		if len(entry.bytes) == 0 {
			return fmt.Errorf("empty encode entry at index %d", index)
		}
		if index > 0 && entry.r <= previous {
			return fmt.Errorf("encode table is not strictly sorted at index %d", index)
		}
		previous = entry.r
		writer.u32(entry.r)
		if err := writer.string16(string(entry.bytes)); err != nil {
			return err
		}
	}
	return nil
}

func (writer *bundleWriter) pairEntries(entries []pairEntry) error {
	if err := writer.count32(len(entries)); err != nil {
		return err
	}
	var previous uint64
	for index, entry := range entries {
		if len(entry.bytes) == 0 {
			return fmt.Errorf("empty pair entry at index %d", index)
		}
		key := uint64(entry.first)<<21 | uint64(entry.second)
		if index > 0 && key <= previous {
			return fmt.Errorf("pair table is not strictly sorted at index %d", index)
		}
		previous = key
		writer.u32(entry.first)
		writer.u32(entry.second)
		if err := writer.string16(string(entry.bytes)); err != nil {
			return err
		}
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".libiconv-multibyte-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(name)
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
	if err := os.Rename(name, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "libiconv-multibyte-gen: "+format+"\n", args...)
	os.Exit(1)
}

const candidateProbeTemplate = `
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
typedef unsigned int ucs4_t; typedef unsigned int state_t;
struct conv_struct { unsigned int isurface; state_t istate; unsigned int osurface; state_t ostate; }; typedef struct conv_struct * conv_t;
#define RET_SHIFT_ILSEQ(n) (-1-2*(n))
#define RET_ILSEQ RET_SHIFT_ILSEQ(0)
#define RET_TOOFEW(n) (-2-2*(n))
#define DECODE_TOOFEW(r) ((unsigned int)(RET_TOOFEW(0)-(r))/2)
#define RET_ILUNI -1
#define RET_TOOSMALL -2
#define ICONV_SURFACE_EBCDIC_ZOS_UNIX 1U
static unsigned char swap_x15_x25(unsigned char x){return (unsigned char)(x ^ ((((x)-0x15)&~0x10)==0 ? 0x30 : 0));}
typedef struct { unsigned short indx; unsigned short used; } Summary16;
#include "ascii.h"
#include "cp874.h"
#include "cp1129.h"
#include "iso646_cn.h"
#include "iso8859_1.h"
#include "iso8859_7.h"
#include "jisx0201.h"
#include "jisx0208.h"
#include "jisx0212.h"
#include "gb2312.h"
#include "ksc5601.h"
#include "cns11643.h"
#include "big5.h"
#include "johab_hangul.h"
#include "cp932ext.h"
#include "isoir165.h"
__PRE__
#include "__HEADER__"
static int failed=0;
static void printhex(const unsigned char *p,int n){for(int i=0;i<n;i++)printf("%02X",p[i]);}
static int reset_state(struct conv_struct *c,unsigned char*out){__RESET_BODY__}
static int flush_state(struct conv_struct *c,ucs4_t*wc){__FLUSH_BODY__}
static int decode_exact(const unsigned char *in,int len,ucs4_t *out,int capacity){
 struct conv_struct c={0}; int pos=0,count=0,zero_progress=0;
 while(pos<len){
  ucs4_t wc=0; int r=__FN___mbtowc(&c,&wc,in+pos,(size_t)(len-pos));
  if(r>0){if(count>=capacity)return -10;out[count++]=wc;pos+=r;zero_progress=0;continue;}
  if(r==0){if(count>=capacity||++zero_progress>2)return -11;out[count++]=wc;continue;}
  if(((-r)&1)==0){unsigned shifted=DECODE_TOOFEW(r);if(shifted>0&&pos+(int)shifted<=len){pos+=(int)shifted;if(pos==len)break;continue;}return -12;}
  return -13;
 }
 ucs4_t wc=0;int fr=flush_state(&c,&wc);if(fr<0||fr>1)return -14;if(fr==1){if(count>=capacity)return -15;out[count++]=wc;}return count;
}
static void explore(unsigned char *prefix,int len){
 struct conv_struct c={0}; ucs4_t wc1=0,wc2=0; int r=__FN___mbtowc(&c,&wc1,prefix,(size_t)len);
 if(r>=0){ if(r!=len || r==0){fprintf(stderr,"unexpected decoder success %d len %d\n",r,len);failed=1;return;} int fr=flush_state(&c,&wc2); if(fr<0 || fr>1){fprintf(stderr,"flush %d\n",fr);failed=1;return;} printf("D ");printhex(prefix,len);printf(" %X",wc1);if(fr==1)printf(" %X",wc2);printf(" X\n");return; }
 if(r==RET_TOOFEW(0)){ if(len>=4){fprintf(stderr,"decoder needs >4 bytes\n");failed=1;return;} for(unsigned b=0;b<256;b++){prefix[len]=(unsigned char)b;explore(prefix,len+1);if(failed)return;} return; }
 if(r==RET_ILSEQ)return;
 fprintf(stderr,"unexpected decoder return %d len %d\n",r,len);failed=1;
}
typedef struct { ucs4_t wc; state_t state; unsigned char prefix[64]; int prefixlen; } lead_t;
static unsigned char representable[0x110000];
static lead_t leads[4096]; static unsigned nleads=0;
int main(void){
 unsigned char prefix[5]={0}; __DECODE__
 for(ucs4_t wc=0;wc<=0x10ffff;wc++){ if(wc>=0xd800&&wc<=0xdfff)continue; struct conv_struct c={0}; unsigned char out[128]={0}; int r=__FN___wctomb(&c,out,wc,64); if(r==RET_ILUNI)continue; if(r<0){fprintf(stderr,"encoder ret %d U+%X\n",r,wc);return 20;} state_t leadstate=c.ostate; ucs4_t prefixcheck[2]={0};int prefixdecoded=decode_exact(out,r,prefixcheck,2); unsigned char tail[64]={0}; int rr=reset_state(&c,tail); if(rr<0){fprintf(stderr,"reset ret %d U+%X\n",rr,wc);return 21;} if(r==0 && rr==0)continue; if(r+rr>(int)sizeof(out)){return 22;} if(prefixdecoded==0){if(nleads>=4096||r>64)return 23;leads[nleads].wc=wc;leads[nleads].state=leadstate;leads[nleads].prefixlen=r;if(r>0)memcpy(leads[nleads].prefix,out,(size_t)r);nleads++;} memcpy(out+r,tail,(size_t)rr); ucs4_t check[3]={0};int decoded=decode_exact(out,r+rr,check,3);if(decoded!=1||check[0]!=wc)continue; representable[wc]=1; printf("E %X ",wc);printhex(out,r+rr);printf("\n"); }
 for(unsigned li=0;li<nleads;li++){ for(ucs4_t wc=0;wc<=0x10ffff;wc++){ if(wc>=0xd800&&wc<=0xdfff||representable[wc])continue; struct conv_struct c={0};c.ostate=leads[li].state;unsigned char out[128]={0};int r=__FN___wctomb(&c,out+leads[li].prefixlen,wc,64);if(r==RET_ILUNI)continue;if(r<0)continue;unsigned char tail[64]={0};int rr=reset_state(&c,tail);if(rr<0)continue;int total=leads[li].prefixlen+r+rr;if(total==0||total>(int)sizeof(out))continue;if(leads[li].prefixlen>0)memcpy(out,leads[li].prefix,(size_t)leads[li].prefixlen);memcpy(out+leads[li].prefixlen+r,tail,(size_t)rr);ucs4_t check[3]={0};int decoded=decode_exact(out,total,check,3);if(decoded!=2||check[0]!=leads[li].wc||check[1]!=wc)continue;printf("P %X %X ",leads[li].wc,wc);printhex(out,total);printf("\n"); } }
 return failed?24:0;
}
`

const rawProbeSource = `
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
typedef unsigned int ucs4_t; typedef unsigned int state_t;
struct conv_struct { unsigned int isurface; state_t istate; unsigned int osurface; state_t ostate; }; typedef struct conv_struct * conv_t;
#define RET_SHIFT_ILSEQ(n) (-1-2*(n))
#define RET_ILSEQ RET_SHIFT_ILSEQ(0)
#define RET_TOOFEW(n) (-2-2*(n))
#define RET_ILUNI -1
#define RET_TOOSMALL -2
#define ICONV_SURFACE_EBCDIC_ZOS_UNIX 1U
static unsigned char swap_x15_x25(unsigned char x){return (unsigned char)(x ^ ((((x)-0x15)&~0x10)==0 ? 0x30 : 0));}
typedef struct { unsigned short indx; unsigned short used; } Summary16;
#include "ascii.h"
#include "iso646_cn.h"
#include "iso8859_1.h"
#include "iso8859_7.h"
#include "jisx0201.h"
#include "jisx0208.h"
#include "jisx0212.h"
#include "gb2312.h"
#include "ksc5601.h"
#include "cns11643.h"
#include "big5.h"
#include "johab_hangul.h"
#include "cp932ext.h"
#include "isoir165.h"
#include "jisx0213.h"
#include "iso2022_jpms.h"
static void row1(const char*name,int (*fn)(conv_t,ucs4_t*,const unsigned char*,size_t),int add){struct conv_struct c={0};for(int b=0;b<128;b++){unsigned char in=(unsigned char)(b+add);ucs4_t wc=0;int r=fn(&c,&wc,&in,1);if(r==1)printf("R %s 1 %02X %X\n",name,b,wc);}}
static void row2(const char*name,int (*fn)(conv_t,ucs4_t*,const unsigned char*,size_t)){for(int a=0x21;a<=0x7e;a++)for(int b=0x21;b<=0x7e;b++){struct conv_struct c={0};unsigned char in[2]={(unsigned char)a,(unsigned char)b};ucs4_t wc=0;int r=fn(&c,&wc,in,2);if(r==2)printf("R %s 2 %02X%02X %X\n",name,a,b,wc);}}
static void cns(const char*name,int plane){for(int a=0x21;a<=0x7e;a++)for(int b=0x21;b<=0x7e;b++){struct conv_struct c={0};unsigned char in[2]={(unsigned char)a,(unsigned char)b};ucs4_t wc=0;int r=RET_ILSEQ;switch(plane){case 1:r=cns11643_1_mbtowc(&c,&wc,in,2);break;case 2:r=cns11643_2_mbtowc(&c,&wc,in,2);break;case 3:r=cns11643_3_mbtowc(&c,&wc,in,2);break;case 4:r=cns11643_4_mbtowc(&c,&wc,in,2);break;case 5:r=cns11643_5_mbtowc(&c,&wc,in,2);break;case 6:r=cns11643_6_mbtowc(&c,&wc,in,2);break;case 7:r=cns11643_7_mbtowc(&c,&wc,in,2);break;}if(r==2)printf("R %s 2 %02X%02X %X\n",name,a,b,wc);}}
static void jis0213(const char*name,int plane){for(int a=0x21;a<=0x7e;a++)for(int b=0x21;b<=0x7e;b++){ucs4_t wc=jisx0213_to_ucs4(((unsigned)plane<<8)+(unsigned)a,(unsigned)b);if(wc){if(wc<0x80)printf("R %s 2 %02X%02X %X %X\n",name,a,b,jisx0213_to_ucs_combining[wc-1][0],jisx0213_to_ucs_combining[wc-1][1]);else printf("R %s 2 %02X%02X %X\n",name,a,b,wc);}}}
static void jpms(const char*name,state_t state){for(int a=0x21;a<=0x7e;a++)for(int b=0x21;b<=0x7e;b++){struct conv_struct c={0};c.istate=state;unsigned char in[2]={(unsigned char)a,(unsigned char)b};ucs4_t wc=0;int r=iso2022_jpms_mbtowc(&c,&wc,in,2);if(r==2)printf("R %s 2 %02X%02X %X\n",name,a,b,wc);}}
int main(void){row1("jis0201-roman",jisx0201_mbtowc,0);for(int b=0x21;b<=0x5f;b++){struct conv_struct c={0};unsigned char in=(unsigned char)(b+0x80);ucs4_t wc=0;if(jisx0201_mbtowc(&c,&wc,&in,1)==1)printf("R jis0201-kana 1 %02X %X\n",b,wc);}row2("jis0208",jisx0208_mbtowc);row2("jis0212",jisx0212_mbtowc);row2("gb2312",gb2312_mbtowc);row2("ksc5601",ksc5601_mbtowc);cns("cns1",1);cns("cns2",2);cns("cns3",3);cns("cns4",4);cns("cns5",5);cns("cns6",6);cns("cns7",7);row2("isoir165",isoir165_mbtowc);row1("iso8859-1-g2",iso8859_1_mbtowc,0x80);row1("iso8859-7-g2",iso8859_7_mbtowc,0x80);jis0213("jis0213-plane1",1);jis0213("jis0213-plane2",2);jpms("jpms0208",3);jpms("jpms0212",4);return 0;}
`
