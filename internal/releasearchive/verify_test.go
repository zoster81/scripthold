package releasearchive

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func archiveFixture(t *testing.T, files []struct {
	name    string
	content string
	mode    os.FileMode
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: zip.Store}
		h.SetMode(f.mode)
		out, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(out, f.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func fileExpect(name, content string) ExpectedFile {
	hash := sha256.Sum256([]byte(content))
	return ExpectedFile{Name: name, Size: int64(len(content)), SHA256: hex.EncodeToString(hash[:])}
}

type stalledPayloadReaderAt struct {
	data  []byte
	start int64
	end   int64
	reads int
}

func (reader *stalledPayloadReaderAt) ReadAt(dst []byte, offset int64) (int, error) {
	if offset >= reader.start && offset < reader.end {
		reader.reads++
		if reader.reads <= 100 {
			return 0, nil
		}
		return 0, errors.New("test reader exceeded payload no-progress budget")
	}
	return bytes.NewReader(reader.data).ReadAt(dst, offset)
}

func TestVerifyRejectsNonProgressingZIPPayload(t *testing.T) {
	const content = "original synthetic release bytes"
	archive := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{"checksums.txt", content, 0o644}})
	offset := bytes.Index(archive, []byte(content))
	if offset < 0 {
		t.Fatal("missing ZIP payload fixture")
	}
	reader := &stalledPayloadReaderAt{
		data: archive, start: int64(offset), end: int64(offset + len(content)),
	}
	err := Verify(reader, int64(len(archive)), []ExpectedFile{fileExpect("checksums.txt", content)})
	if !errors.Is(err, io.ErrNoProgress) || reader.reads > 100 {
		t.Fatalf("expected bounded ZIP payload progress rejection, reads=%d err=%v", reader.reads, err)
	}
}

type intermittentPayloadReaderAt struct {
	data  []byte
	start int64
	end   int64
	reads int
}

func (reader *intermittentPayloadReaderAt) ReadAt(dst []byte, offset int64) (int, error) {
	if offset >= reader.start && offset < reader.end {
		reader.reads++
		if reader.reads%2 == 1 {
			return 0, nil
		}
	}
	return bytes.NewReader(reader.data).ReadAt(dst, offset)
}

func TestVerifyAllowsIntermittentEmptyZIPPayloadReads(t *testing.T) {
	const content = "original synthetic ZIP data"
	archive := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{"checksums.txt", content, 0o644}})
	offset := bytes.Index(archive, []byte(content))
	if offset < 0 {
		t.Fatal("missing ZIP payload fixture")
	}
	reader := &intermittentPayloadReaderAt{
		data: archive, start: int64(offset), end: int64(offset + len(content)),
	}
	if err := Verify(reader, int64(len(archive)), []ExpectedFile{
		fileExpect("checksums.txt", content),
	}); err != nil {
		t.Fatalf("rejected ZIP with occasional empty reads: %v", err)
	}
}

func TestVerifyReadsAndChecksEveryOriginalByteWithoutExtraction(t *testing.T) {
	src := []struct {
		name, content string
		mode          os.FileMode
	}{
		{"checksums.txt", "SHA256 synthetic checksum", 0o644},
		{"scripthold_linux_amd64", "synthetic ordinary bytes", 0o755},
	}
	zipData := archiveFixture(t, src)
	want := []ExpectedFile{fileExpect(src[0].name, src[0].content), fileExpect(src[1].name, src[1].content)}
	if err := Verify(bytes.NewReader(zipData), int64(len(zipData)), want); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejectsMaliciousArchiveNamesAndTypes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []struct {
			name, content string
			mode          os.FileMode
		}
	}{
		{"traversal", []struct {
			name, content string
			mode          os.FileMode
		}{{"../escape", "binary", 0o644}}},
		{"nested", []struct {
			name, content string
			mode          os.FileMode
		}{{"dir/checksums.txt", "binary", 0o644}}},
		{"backslash", []struct {
			name, content string
			mode          os.FileMode
		}{{"dir\\checksums.txt", "binary", 0o644}}},
		{"symlink", []struct {
			name, content string
			mode          os.FileMode
		}{{"checksums.txt", "target", os.ModeSymlink | 0o777}}},
		{"directory", []struct {
			name, content string
			mode          os.FileMode
		}{{"checksums.txt", "", os.ModeDir | 0o755}}},
		{"duplicate", []struct {
			name, content string
			mode          os.FileMode
		}{{"checksums.txt", "x", 0o644}, {"checksums.txt", "y", 0o644}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := archiveFixture(t, tc.files)
			if err := Verify(bytes.NewReader(b), int64(len(b)), []ExpectedFile{fileExpect("checksums.txt", "binary")}); err == nil {
				t.Fatal("accepted malformed ZIP members")
			}
		})
	}
}

func TestVerifyRejectsCorruptionAndChangedContents(t *testing.T) {
	original := "PLAIN_CONTENT_SENTINEL"
	zipData := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{"checksums.txt", original, 0o644}})
	want := []ExpectedFile{fileExpect("checksums.txt", original)}
	broken := bytes.Clone(zipData)
	i := bytes.Index(broken, []byte(original))
	if i < 0 {
		t.Fatal("payload not found")
	}
	broken[i] = 'X'
	for _, tc := range []struct {
		name     string
		data     []byte
		expected []ExpectedFile
	}{
		{"CRC corruption", broken, want},
		{"wrong original SHA", zipData, []ExpectedFile{fileExpect("checksums.txt", "DIFFERENT_CONTENT")}},
		{"truncated archive", zipData[:len(zipData)-16], want},
		{"unexpected expected size", zipData, []ExpectedFile{{Name: want[0].Name, Size: want[0].Size - 1, SHA256: want[0].SHA256}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := Verify(bytes.NewReader(tc.data), int64(len(tc.data)), tc.expected); err == nil {
				t.Fatal("accepted bad ZIP")
			}
		})
	}
}

func TestVerifyRejectsUnsafeNameWithMatchingExpectedBytes(t *testing.T) {
	for _, name := range []string{"../escape", "dir/entry", "dir\\entry", ".hidden", "bad..name"} {
		t.Run(name, func(t *testing.T) {
			const content = "synthetic bytes"
			archive := archiveFixture(t, []struct {
				name, content string
				mode          os.FileMode
			}{{name, content, 0o644}})
			// Keep the digest and length correct to isolate filename rejection.
			if err := Verify(bytes.NewReader(archive), int64(len(archive)),
				[]ExpectedFile{fileExpect(name, content)}); err == nil {
				t.Fatal("accepted unsafe name with matching content")
			}
		})
	}
}

func TestVerifyRejectsUnsafeModeWithMatchingContent(t *testing.T) {
	for _, mode := range []os.FileMode{
		os.ModeSymlink | 0o777,
		os.ModeDir | 0o755,
		os.ModeSetuid | 0o755,
		os.ModeSetgid | 0o755,
		os.ModeSticky | 0o755,
	} {
		const content = "matching synthetic bytes"
		archive := archiveFixture(t, []struct {
			name, content string
			mode          os.FileMode
		}{{"checksums.txt", content, mode}})
		if err := Verify(bytes.NewReader(archive), int64(len(archive)),
			[]ExpectedFile{fileExpect("checksums.txt", content)}); err == nil {
			t.Fatalf("accepted unsafe member mode %v", mode)
		}
	}
}

func TestVerifyReadsDeflateEntries(t *testing.T) {
	const name = "checksums.txt"
	content := strings.Repeat("compressed synthetic content ", 64)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: name, Method: zip.Deflate}
	h.SetMode(0o644)
	entry, err := zw.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(entry, content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Verify(bytes.NewReader(buf.Bytes()), int64(buf.Len()),
		[]ExpectedFile{fileExpect(name, content)}); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejectsDuplicateEntryWithMatchingEntryCount(t *testing.T) {
	const first = "first bytes"
	const second = "second bytes"
	archive := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{
		{"checksums.txt", first, 0o644},
		{"checksums.txt", first, 0o644},
	})
	expected := []ExpectedFile{
		fileExpect("checksums.txt", first),
		fileExpect("scripthold_linux_amd64", second),
	}
	err := Verify(bytes.NewReader(archive), int64(len(archive)), expected)
	if err == nil || !strings.Contains(err.Error(), "duplicate ZIP") {
		t.Fatalf("duplicate ZIP entry not specifically rejected: %v", err)
	}
}

func TestVerifyDetectsCRCFailureIndependentlyOfSHA256(t *testing.T) {
	const original = "CRC32_CHECK_SENTINEL"
	archive := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{"checksums.txt", original, 0o644}})
	index := bytes.Index(archive, []byte(original))
	if index < 0 {
		t.Fatal("missing synthetic payload")
	}
	broken := bytes.Clone(archive)
	broken[index] ^= 1
	err := Verify(bytes.NewReader(broken), int64(len(broken)),
		[]ExpectedFile{fileExpect("checksums.txt", original)})
	if !errors.Is(err, zip.ErrChecksum) {
		t.Fatalf("expected ZIP CRC mismatch, got %v", err)
	}
}

func TestVerifyRejectsConflictingZIPLocalHeaders(t *testing.T) {
	const name = "checksums.txt"
	const content = "unchanged original bytes"
	archive := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{name, content, 0o644}})
	want := []ExpectedFile{fileExpect(name, content)}
	for _, tc := range []struct {
		name   string
		change func([]byte)
	}{
		{"local filename differs from central filename", func(b []byte) { b[30] ^= 1 }},
		{"local method differs from central method", func(b []byte) { b[8] = 8 }},
		{"local flags differ from central flags", func(b []byte) { b[6] ^= 0x08 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutated := bytes.Clone(archive)
			tc.change(mutated)
			if err := Verify(bytes.NewReader(mutated), int64(len(mutated)), want); err == nil {
				t.Fatal("accepted inconsistent local and central ZIP headers")
			}
		})
	}
}

func TestVerifyRejectsMisleadingZIPDirectoryCounts(t *testing.T) {
	const name = "checksums.txt"
	const content = "bounded ZIP bytes"
	archive := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{name, content, 0o644}})
	want := []ExpectedFile{fileExpect(name, content)}
	for _, count := range []uint16{0, 23, 0xffff} {
		mutated := bytes.Clone(archive)
		eocd := len(mutated) - 22
		if !bytes.Equal(mutated[eocd:eocd+4], []byte{'P', 'K', 5, 6}) {
			t.Fatal("unexpected ZIP test fixture footer")
		}
		binary.LittleEndian.PutUint16(mutated[eocd+8:eocd+10], count)
		binary.LittleEndian.PutUint16(mutated[eocd+10:eocd+12], count)
		if err := Verify(bytes.NewReader(mutated), int64(len(mutated)), want); err == nil {
			t.Fatalf("accepted false ZIP directory count %d", count)
		}
	}
}

func TestVerifyRejectsUnsupportedZIPLayoutsBeforeParsing(t *testing.T) {
	const name = "checksums.txt"
	const content = "nonrelease ZIP fixture"
	original := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{name, content, 0o644}})
	want := []ExpectedFile{fileExpect(name, content)}
	for _, tc := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"central record offset changed", func(b []byte) []byte {
			index := bytes.Index(b, []byte{'P', 'K', 1, 2})
			if index < 0 {
				t.Fatal("missing central header")
			}
			binary.LittleEndian.PutUint32(b[index+42:index+46], 1)
			return b
		}},
		{"ZIP64 footer record count", func(b []byte) []byte {
			eocd := len(b) - 22
			binary.LittleEndian.PutUint16(b[eocd+8:eocd+10], 0xffff)
			binary.LittleEndian.PutUint16(b[eocd+10:eocd+12], 0xffff)
			return b
		}},
		{"ZIP central directory displaced", func(b []byte) []byte {
			eocd := len(b) - 22
			binary.LittleEndian.PutUint32(b[eocd+16:eocd+20], 0)
			return b
		}},
		{"appended trailer", func(b []byte) []byte {
			return append(b, []byte("unrecognized data")...)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutated := tc.mutate(bytes.Clone(original))
			if err := Verify(bytes.NewReader(mutated), int64(len(mutated)), want); err == nil {
				t.Fatal("accepted unsupported ZIP archive layout")
			}
		})
	}
}

func TestVerifyRejectsConflictingZIPDataDescriptors(t *testing.T) {
	archive := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{"checksums.txt", "synthetic descriptor bytes", 0o644}})
	want := []ExpectedFile{fileExpect("checksums.txt", "synthetic descriptor bytes")}
	central := bytes.Index(archive, []byte{'P', 'K', 1, 2})
	if central < 0 || binary.LittleEndian.Uint16(archive[central+8:central+10])&8 == 0 {
		t.Fatal("synthetic ZIP must have a data descriptor")
	}
	data := int(binary.LittleEndian.Uint32(archive[central+20 : central+24]))
	descriptor := 30 + len("checksums.txt") + data
	if descriptor+16 > central ||
		!bytes.Equal(archive[descriptor:descriptor+4], []byte{'P', 'K', 7, 8}) {
		t.Fatal("unexpected signed ZIP descriptor layout")
	}
	for _, tc := range []struct {
		name   string
		offset int
	}{
		{"changed compressed CRC", descriptor + 4},
		{"changed compressed length", descriptor + 8},
		{"changed uncompressed length", descriptor + 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutated := bytes.Clone(archive)
			mutated[tc.offset] ^= 1
			if err := Verify(bytes.NewReader(mutated), int64(len(mutated)), want); err == nil {
				t.Fatal("accepted conflicting ZIP data descriptor")
			}
		})
	}
}

func TestVerifyRejectsUnaccountedZIPGap(t *testing.T) {
	archive := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{"checksums.txt", "synthetic bytes", 0o644}})
	central := bytes.Index(archive, []byte{'P', 'K', 1, 2})
	if central < 0 {
		t.Fatal("missing central header")
	}
	// Deliberately insert a byte before the central directory and point the
	// end record to its new location. Original file bytes and CRC remain valid.
	mutated := append(bytes.Clone(archive[:central]), append([]byte{0}, archive[central:]...)...)
	eocd := len(mutated) - 22
	binary.LittleEndian.PutUint32(mutated[eocd+16:eocd+20], uint32(central+1))
	want := []ExpectedFile{fileExpect("checksums.txt", "synthetic bytes")}
	if err := Verify(bytes.NewReader(mutated), int64(len(mutated)), want); err == nil {
		t.Fatal("accepted unaccounted ZIP bytes before the central directory")
	}
}

func TestVerifyAcceptsUnsignedZIPDataDescriptor(t *testing.T) {
	const name, content = "checksums.txt", "legitimate unsigned descriptor"
	original := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{name, content, 0o644}})
	central := bytes.Index(original, []byte{'P', 'K', 1, 2})
	if central < 16 || !bytes.Equal(original[central-16:central-12], []byte{'P', 'K', 7, 8}) {
		t.Fatal("synthetic ZIP must have a signed 16-byte descriptor")
	}
	// Remove only the optional signature; update the central directory
	// offset. Preserve the checksum, original bytes and all sizes.
	mutated := append(bytes.Clone(original[:central-16]), original[central-12:]...)
	eocd := len(mutated) - 22
	binary.LittleEndian.PutUint32(mutated[eocd+16:eocd+20], uint32(central-4))
	if err := Verify(bytes.NewReader(mutated), int64(len(mutated)),
		[]ExpectedFile{fileExpect(name, content)}); err != nil {
		t.Fatalf("rejected legitimate ZIP descriptor without signature: %v", err)
	}
}

func TestVerifyChecksLocalHeaderValuesWithoutDescriptor(t *testing.T) {
	const name, content = "checksums.txt", "legitimate stored bytes"
	original := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{name, content, 0o644}})
	central := bytes.Index(original, []byte{'P', 'K', 1, 2})
	if central < 16 || !bytes.Equal(original[central-16:central-12], []byte{'P', 'K', 7, 8}) {
		t.Fatal("synthetic ZIP must have a signed descriptor")
	}
	// Convert the in-memory test fixture to a legal ZIP with no data
	// descriptor and sizes/checksum stored in the local file header.
	mutated := append(bytes.Clone(original[:central-16]), original[central:]...)
	newCentral := central - 16
	binary.LittleEndian.PutUint16(mutated[6:8], 0)
	binary.LittleEndian.PutUint16(mutated[newCentral+8:newCentral+10], 0)
	copy(mutated[14:26], mutated[newCentral+16:newCentral+28])
	eocd := len(mutated) - 22
	binary.LittleEndian.PutUint32(mutated[eocd+16:eocd+20], uint32(newCentral))
	want := []ExpectedFile{fileExpect(name, content)}
	if err := Verify(bytes.NewReader(mutated), int64(len(mutated)), want); err != nil {
		t.Fatalf("rejected valid ZIP without a data descriptor: %v", err)
	}
	for _, off := range []int{14, 18, 22} {
		broken := bytes.Clone(mutated)
		broken[off] ^= 1
		if err := Verify(bytes.NewReader(broken), int64(len(broken)), want); err == nil {
			t.Fatalf("accepted inconsistent local header byte at offset %d", off)
		}
	}
}

func TestVerifyRejectsUnreviewedZIPExtraFieldsAndComments(t *testing.T) {
	const name, content = "checksums.txt", "synthetic ZIP member"
	original := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{name, content, 0o644}})
	central := bytes.Index(original, []byte{'P', 'K', 1, 2})
	if central < 0 || bytes.Index(original, []byte{'P', 'K', 5, 6}) != len(original)-zipEndHeaderSize {
		t.Fatal("unexpected synthetic ZIP fixture")
	}
	extra := []byte{0xfe, 0xca, 0, 0} // well-formed, unknown extra field
	for _, tc := range []struct {
		name   string
		mutate func() []byte
	}{
		{"local extra", func() []byte {
			pos := zipLocalHeaderSize + int(binary.LittleEndian.Uint16(original[26:28]))
			b := append(bytes.Clone(original[:pos]), append(bytes.Clone(extra), original[pos:]...)...)
			binary.LittleEndian.PutUint16(b[28:30], uint16(len(extra)))
			eocd := len(b) - zipEndHeaderSize
			binary.LittleEndian.PutUint32(b[eocd+16:eocd+20], uint32(central+len(extra)))
			return b
		}},
		{"central extra", func() []byte {
			pos := central + zipCentralHeaderSize + int(binary.LittleEndian.Uint16(original[central+28:central+30]))
			b := append(bytes.Clone(original[:pos]), append(bytes.Clone(extra), original[pos:]...)...)
			binary.LittleEndian.PutUint16(b[central+30:central+32], uint16(len(extra)))
			eocd := len(b) - zipEndHeaderSize
			binary.LittleEndian.PutUint32(b[eocd+12:eocd+16],
				binary.LittleEndian.Uint32(original[len(original)-zipEndHeaderSize+12:])+uint32(len(extra)))
			return b
		}},
		{"central comment", func() []byte {
			pos := central + zipCentralHeaderSize + int(binary.LittleEndian.Uint16(original[central+28:central+30]))
			b := append(bytes.Clone(original[:pos]), append(bytes.Clone(extra), original[pos:]...)...)
			binary.LittleEndian.PutUint16(b[central+32:central+34], uint16(len(extra)))
			eocd := len(b) - zipEndHeaderSize
			binary.LittleEndian.PutUint32(b[eocd+12:eocd+16],
				binary.LittleEndian.Uint32(original[len(original)-zipEndHeaderSize+12:])+uint32(len(extra)))
			return b
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.mutate()
			// This dialect is intentionally unsupported even with unchanged
			// original payload bytes and authenticated digest.
			if err := Verify(bytes.NewReader(b), int64(len(b)),
				[]ExpectedFile{fileExpect(name, content)}); err == nil {
				t.Fatal("accepted unreviewed ZIP metadata extension")
			}
		})
	}
}

func TestVerifyRejectsBadAuthorityAndImpossibleLimits(t *testing.T) {
	data := archiveFixture(t, []struct {
		name, content string
		mode          os.FileMode
	}{{"checksums.txt", "data", 0o644}})
	good := fileExpect("checksums.txt", "data")
	for _, tc := range []struct {
		name     string
		expected []ExpectedFile
		size     int64
	}{
		{"empty manifest", nil, int64(len(data))},
		{"duplicate expected", []ExpectedFile{good, good}, int64(len(data))},
		{"invalid expected path", []ExpectedFile{{Name: "../evil", Size: good.Size, SHA256: good.SHA256}}, int64(len(data))},
		{"uppercase sha", []ExpectedFile{{Name: good.Name, Size: good.Size, SHA256: strings.ToUpper(good.SHA256)}}, int64(len(data))},
		{"empty file", []ExpectedFile{{Name: good.Name, Size: 0, SHA256: good.SHA256}}, int64(len(data))},
		{"oversized declared asset", []ExpectedFile{{Name: good.Name, Size: 3 << 30, SHA256: good.SHA256}}, int64(len(data))},
		{"archive exceeds cap", []ExpectedFile{good}, 5 << 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := Verify(bytes.NewReader(data), tc.size, tc.expected); err == nil {
				t.Fatal("accepted invalid authority")
			}
		})
	}
}
