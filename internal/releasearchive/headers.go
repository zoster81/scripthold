package releasearchive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
)

const (
	zipLocalHeaderSize   = 30
	zipCentralHeaderSize = 46
	zipEndHeaderSize     = 22
	maxStagedNameBytes   = 240
)

type zipExtent struct {
	start, end int64
}

// preflightZIPHeaders reads only bounded ZIP metadata before archive/zip.NewReader
// can allocate for attacker-declared central-directory records. The staging
// contract deliberately accepts only single-disk, non-ZIP64 archives without
// comments or prepended data. Broader ZIP dialects require separate qualification.
func preflightZIPHeaders(source io.ReaderAt, archiveSize int64, expected map[string]ExpectedFile) (map[string]int64, error) {
	if archiveSize < zipEndHeaderSize || len(expected) == 0 || len(expected) > maxFiles {
		return nil, errors.New("invalid ZIP preflight bounds")
	}
	read := func(offset int64, dst []byte) error {
		if offset < 0 || offset > archiveSize-int64(len(dst)) {
			return io.ErrUnexpectedEOF
		}
		n, err := source.ReadAt(dst, offset)
		if err != nil || n != len(dst) {
			return errors.Join(io.ErrUnexpectedEOF, err)
		}
		return nil
	}
	var end [zipEndHeaderSize]byte
	endOffset := archiveSize - zipEndHeaderSize
	if err := read(endOffset, end[:]); err != nil {
		return nil, fmt.Errorf("read ZIP directory end: %w", err)
	}
	u16 := binary.LittleEndian.Uint16
	u32 := binary.LittleEndian.Uint32
	if u32(end[:4]) != 0x06054b50 || u16(end[4:6]) != 0 ||
		u16(end[6:8]) != 0 || u16(end[20:22]) != 0 ||
		int(u16(end[8:10])) != len(expected) ||
		int(u16(end[10:12])) != len(expected) {
		return nil, errors.New("unsupported or inconsistent ZIP directory end")
	}
	dirSize, dirOffset := int64(u32(end[12:16])), int64(u32(end[16:20]))
	if dirSize < int64(len(expected)*zipCentralHeaderSize) ||
		dirOffset <= 0 || dirOffset > endOffset ||
		dirSize != endOffset-dirOffset {
		return nil, errors.New("invalid or unsupported ZIP central directory extent")
	}

	dataOffsets := make(map[string]int64, len(expected))
	extents := make([]zipExtent, 0, len(expected))
	pos := dirOffset
	for i := 0; i < len(expected); i++ {
		var central [zipCentralHeaderSize]byte
		if pos > endOffset-zipCentralHeaderSize {
			return nil, errors.New("truncated ZIP central record")
		}
		if err := read(pos, central[:]); err != nil {
			return nil, fmt.Errorf("read ZIP central header: %w", err)
		}
		if u32(central[:4]) != 0x02014b50 {
			return nil, errors.New("invalid ZIP central record signature")
		}
		nameLen := int(u16(central[28:30]))
		extraLen := int64(u16(central[30:32]))
		commentLen := int64(u16(central[32:34]))
		recordSize := int64(zipCentralHeaderSize+nameLen) + extraLen + commentLen
		if nameLen == 0 || nameLen > maxStagedNameBytes ||
			recordSize > endOffset-pos || u16(central[34:36]) != 0 {
			return nil, errors.New("invalid ZIP central record length or disk")
		}
		nameBytes := make([]byte, nameLen)
		if err := read(pos+zipCentralHeaderSize, nameBytes); err != nil {
			return nil, err
		}
		name := string(nameBytes)
		expectedFile, ok := expected[name]
		if !ok || !safeFlatName(name) {
			return nil, fmt.Errorf("unexpected ZIP central name %q", name)
		}
		if _, duplicate := dataOffsets[name]; duplicate {
			return nil, fmt.Errorf("duplicate ZIP central name %q", name)
		}
		flags, method := u16(central[8:10]), u16(central[10:12])
		compressed, original := u32(central[20:24]), u32(central[24:28])
		if flags&1 != 0 || (method != 0 && method != 8) ||
			compressed == 0 || compressed == 0xffffffff ||
			original == 0xffffffff || int64(original) != expectedFile.Size {
			return nil, fmt.Errorf("unsupported ZIP compression or lengths for %q", name)
		}
		localOffset := int64(u32(central[42:46]))
		if localOffset > dirOffset-zipLocalHeaderSize {
			return nil, fmt.Errorf("invalid ZIP local header offset for %q", name)
		}
		var local [zipLocalHeaderSize]byte
		if err := read(localOffset, local[:]); err != nil {
			return nil, err
		}
		if u32(local[:4]) != 0x04034b50 || u16(local[6:8]) != flags ||
			u16(local[8:10]) != method || int(u16(local[26:28])) != nameLen {
			return nil, fmt.Errorf("ZIP local/central header mismatch for %q", name)
		}
		localName := make([]byte, nameLen)
		if err := read(localOffset+zipLocalHeaderSize, localName); err != nil {
			return nil, err
		}
		if !bytes.Equal(localName, nameBytes) {
			return nil, fmt.Errorf("ZIP local/central name mismatch for %q", name)
		}
		dataOffset := localOffset + zipLocalHeaderSize + int64(nameLen) +
			int64(u16(local[28:30]))
		dataEnd := dataOffset + int64(compressed)
		if dataOffset < localOffset || dataEnd > dirOffset || dataEnd < dataOffset {
			return nil, fmt.Errorf("invalid ZIP compressed data boundary for %q", name)
		}
		dataOffsets[name] = dataOffset
		extents = append(extents, zipExtent{localOffset, dataEnd})
		pos += recordSize
	}
	if pos != endOffset {
		return nil, errors.New("unexpected ZIP central directory contents")
	}
	sort.Slice(extents, func(i, j int) bool { return extents[i].start < extents[j].start })
	if extents[0].start != 0 {
		return nil, errors.New("ZIP prefix or unsupported local header layout")
	}
	for i := 1; i < len(extents); i++ {
		if extents[i].start < extents[i-1].end {
			return nil, errors.New("overlapping ZIP local file records")
		}
	}
	return dataOffsets, nil
}
