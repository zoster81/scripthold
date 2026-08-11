package textstream

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestByteLineEndingReaderPreservesChunkBoundaryState(t *testing.T) {
	tests := []struct {
		target string
		input  string
		want   string
		crlf   int
		lf     int
	}{
		{target: "lf", input: "a\r\nb\nc\rd", want: "a\nb\nc\rd", crlf: 1, lf: 1},
		{target: "crlf", input: "a\r\nb\nc\rd", want: "a\r\nb\r\nc\rd", crlf: 1, lf: 1},
	}
	for _, testCase := range tests {
		t.Run(testCase.target, func(t *testing.T) {
			reader, err := NewByteLineEndingReader(&singleByteReader{data: []byte(testCase.input)}, testCase.target)
			if err != nil {
				t.Fatal(err)
			}
			output, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			if string(output) != testCase.want {
				t.Fatalf("output = %q, want %q", output, testCase.want)
			}
			stats := reader.Stats()
			if stats.CRLFCount != testCase.crlf || stats.LFCount != testCase.lf {
				t.Fatalf("stats = %+v", stats)
			}
		})
	}
}

func TestSingleByteLineEndingReaderSupportsNonASCIIControlBytes(t *testing.T) {
	input := []byte{0xc1, 0x0d, 0x25, 0xc2, 0x25, 0xc3}
	reader, err := NewSingleByteLineEndingReader(&singleByteReader{data: input}, "crlf", 0x0d, 0x25)
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0xc1, 0x0d, 0x25, 0xc2, 0x0d, 0x25, 0xc3}
	if !bytes.Equal(output, want) {
		t.Fatalf("output = %x, want %x", output, want)
	}
	if stats := reader.Stats(); stats.CRLFCount != 1 || stats.LFCount != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestHZLineEndingReaderPreservesContinuationEscape(t *testing.T) {
	input := []byte("alpha~\nbeta\ngamma\r\ndelta")
	reader, err := NewHZLineEndingReader(&singleByteReader{data: input}, "crlf")
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("alpha~\nbeta\r\ngamma\r\ndelta")
	if !bytes.Equal(output, want) {
		t.Fatalf("output = %q, want %q", output, want)
	}
	if stats := reader.Stats(); stats.CRLFCount != 1 || stats.LFCount != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestUTF16LineEndingReaderPreservesCodeUnitsAcrossReads(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		littleEndian bool
		order        binary.ByteOrder
	}{
		{name: "little endian", littleEndian: true, order: binary.LittleEndian},
		{name: "big endian", littleEndian: false, order: binary.BigEndian},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input := encodeUTF16Units(testCase.order, []uint16{'a', '\r', '\n', 'b', '\n', 'c'})
			reader, err := NewUTF16LineEndingReader(&singleByteReader{data: input}, "lf", testCase.littleEndian)
			if err != nil {
				t.Fatal(err)
			}
			output, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			want := encodeUTF16Units(testCase.order, []uint16{'a', '\n', 'b', '\n', 'c'})
			if !bytes.Equal(output, want) {
				t.Fatalf("output = %x, want %x", output, want)
			}
			if stats := reader.Stats(); stats.CRLFCount != 1 || stats.LFCount != 1 {
				t.Fatalf("stats = %+v", stats)
			}
		})
	}
}

func TestPhase4UTF32LineEndingReaderPreservesCodeUnitsAcrossReads(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		littleEndian bool
		order        binary.ByteOrder
	}{
		{name: "little endian", littleEndian: true, order: binary.LittleEndian},
		{name: "big endian", littleEndian: false, order: binary.BigEndian},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input := encodeUTF32Units(testCase.order, []uint32{'a', '\r', '\n', 'b', '\n', 0x1F30D})
			reader, err := NewUTF32LineEndingReader(&singleByteReader{data: input}, "lf", testCase.littleEndian)
			if err != nil {
				t.Fatal(err)
			}
			output, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			want := encodeUTF32Units(testCase.order, []uint32{'a', '\n', 'b', '\n', 0x1F30D})
			if !bytes.Equal(output, want) {
				t.Fatalf("output = %x, want %x", output, want)
			}
			if stats := reader.Stats(); stats.CRLFCount != 1 || stats.LFCount != 1 {
				t.Fatalf("stats = %+v", stats)
			}
		})
	}
}

func TestPhase4UTF32LineEndingReaderRejectsTruncatedUnit(t *testing.T) {
	reader, err := NewUTF32LineEndingReader(bytes.NewReader([]byte{'a', 0, 0}), "lf", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(reader); err == nil {
		t.Fatal("expected truncated UTF-32 byte length error")
	}
}

func TestUTF16LineEndingReaderRejectsOddByteLength(t *testing.T) {
	reader, err := NewUTF16LineEndingReader(bytes.NewReader([]byte{'a', 0, 'b'}), "lf", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(reader); err == nil {
		t.Fatal("expected odd UTF-16 byte length error")
	}
}

func TestLineEndingReaderRejectsInvalidTarget(t *testing.T) {
	if _, err := NewByteLineEndingReader(bytes.NewReader(nil), "mixed"); err == nil {
		t.Fatal("expected invalid target error")
	}
	if _, err := NewUTF16LineEndingReader(bytes.NewReader(nil), "mixed", true); err == nil {
		t.Fatal("expected invalid target error")
	}
	if _, err := NewUTF32LineEndingReader(bytes.NewReader(nil), "mixed", true); err == nil {
		t.Fatal("expected invalid UTF-32 target error")
	}
	if _, err := NewSingleByteLineEndingReader(bytes.NewReader(nil), "lf", 0x25, 0x25); err == nil {
		t.Fatal("expected identical encoded CR/LF error")
	}
}

func encodeUTF16Units(order binary.ByteOrder, units []uint16) []byte {
	data := make([]byte, len(units)*2)
	for index, unit := range units {
		order.PutUint16(data[index*2:], unit)
	}
	return data
}

func encodeUTF32Units(order binary.ByteOrder, units []uint32) []byte {
	data := make([]byte, len(units)*4)
	for index, unit := range units {
		order.PutUint32(data[index*4:], unit)
	}
	return data
}
