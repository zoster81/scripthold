package textstream

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const lineEndingInputBufferSize = 64 * 1024

type LineEndingStats struct {
	CRLFCount int
	LFCount   int
}

type LineEndingReader struct {
	source       io.Reader
	targetCRLF   bool
	utf16        bool
	utf32        bool
	littleEndian bool
	stats        LineEndingStats

	input          [lineEndingInputBufferSize]byte
	output         []byte
	outputOffset   int
	pendingErr     error
	pendingCR      bool
	pendingByte    byte
	hasPendingByte bool
	pendingUnit    [4]byte
	pendingUnitLen int
	crByte         byte
	lfByte         byte
	hz             bool
	hzPendingTilde bool
}

func NewByteLineEndingReader(source io.Reader, target string) (*LineEndingReader, error) {
	return NewSingleByteLineEndingReader(source, target, '\r', '\n')
}

// NewSingleByteLineEndingReader converts line endings whose CR and LF runes
// each have one encoded byte. This includes ASCII-compatible encodings and
// single-byte EBCDIC charmaps.
func NewSingleByteLineEndingReader(source io.Reader, target string, crByte, lfByte byte) (*LineEndingReader, error) {
	targetCRLF, err := parseLineEndingTarget(target)
	if err != nil {
		return nil, err
	}
	if crByte == lfByte {
		return nil, fmt.Errorf("encoded CR and LF bytes must differ")
	}
	return &LineEndingReader{
		source:     source,
		targetCRLF: targetCRLF,
		crByte:     crByte,
		lfByte:     lfByte,
	}, nil
}

// NewHZLineEndingReader preserves HZ's ~\n continuation escape, which does
// not decode to a text line ending, while converting actual CR/LF bytes.
func NewHZLineEndingReader(source io.Reader, target string) (*LineEndingReader, error) {
	reader, err := NewSingleByteLineEndingReader(source, target, '\r', '\n')
	if err != nil {
		return nil, err
	}
	reader.hz = true
	return reader, nil
}

func NewUTF16LineEndingReader(source io.Reader, target string, littleEndian bool) (*LineEndingReader, error) {
	targetCRLF, err := parseLineEndingTarget(target)
	if err != nil {
		return nil, err
	}
	return &LineEndingReader{source: source, targetCRLF: targetCRLF, utf16: true, littleEndian: littleEndian}, nil
}

func NewUTF32LineEndingReader(source io.Reader, target string, littleEndian bool) (*LineEndingReader, error) {
	targetCRLF, err := parseLineEndingTarget(target)
	if err != nil {
		return nil, err
	}
	return &LineEndingReader{source: source, targetCRLF: targetCRLF, utf32: true, littleEndian: littleEndian}, nil
}

func parseLineEndingTarget(target string) (bool, error) {
	switch target {
	case "lf":
		return false, nil
	case "crlf":
		return true, nil
	default:
		return false, fmt.Errorf("invalid line-ending target %q", target)
	}
}

func (reader *LineEndingReader) Stats() LineEndingStats {
	return reader.stats
}

func (reader *LineEndingReader) Read(buffer []byte) (int, error) {
	for reader.outputOffset == len(reader.output) && reader.pendingErr == nil {
		reader.output = reader.output[:0]
		reader.outputOffset = 0
		reader.fill()
	}
	if reader.outputOffset < len(reader.output) {
		read := copy(buffer, reader.output[reader.outputOffset:])
		reader.outputOffset += read
		return read, nil
	}
	if reader.pendingErr != nil {
		err := reader.pendingErr
		reader.pendingErr = nil
		return 0, err
	}
	return 0, io.EOF
}

func (reader *LineEndingReader) fill() {
	read, err := reader.source.Read(reader.input[:])
	if read > 0 {
		switch {
		case reader.utf32:
			reader.transformUTF32(reader.input[:read])
		case reader.utf16:
			reader.transformUTF16(reader.input[:read])
		default:
			reader.transformBytes(reader.input[:read])
		}
	}

	if err != nil {
		if errors.Is(err, io.EOF) {
			reader.finish()
			if reader.pendingErr == nil {
				reader.pendingErr = io.EOF
			}
		} else {
			reader.pendingErr = err
		}
		return
	}
	if read == 0 {
		reader.pendingErr = io.ErrNoProgress
	}
}

func (reader *LineEndingReader) transformBytes(data []byte) {
	for _, current := range data {
		if reader.hz {
			if reader.hzPendingTilde {
				reader.output = append(reader.output, '~', current)
				reader.hzPendingTilde = false
				// HZ's ~\n escape is a line continuation, not a decoded LF.
				// All other two-byte tilde escapes are copied byte-identically.
				continue
			}
			if current == '~' {
				reader.hzPendingTilde = true
				continue
			}
		}

		if reader.pendingCR {
			if current == reader.lfByte {
				reader.stats.CRLFCount++
				reader.appendTargetEnding()
				reader.pendingCR = false
				continue
			}
			reader.output = append(reader.output, reader.crByte)
			reader.pendingCR = false
		}

		switch current {
		case reader.crByte:
			reader.pendingCR = true
		case reader.lfByte:
			reader.stats.LFCount++
			reader.appendTargetEnding()
		default:
			reader.output = append(reader.output, current)
		}
	}
}

func (reader *LineEndingReader) transformUTF16(data []byte) {
	for _, current := range data {
		if !reader.hasPendingByte {
			reader.pendingByte = current
			reader.hasPendingByte = true
			continue
		}
		first := reader.pendingByte
		reader.hasPendingByte = false
		var pair [2]byte
		pair[0], pair[1] = first, current
		var order binary.ByteOrder = binary.BigEndian
		if reader.littleEndian {
			order = binary.LittleEndian
		}
		reader.transformUnit(order.Uint16(pair[:]))
	}
}

func (reader *LineEndingReader) transformUTF32(data []byte) {
	for _, current := range data {
		reader.pendingUnit[reader.pendingUnitLen] = current
		reader.pendingUnitLen++
		if reader.pendingUnitLen < len(reader.pendingUnit) {
			continue
		}
		var order binary.ByteOrder = binary.BigEndian
		if reader.littleEndian {
			order = binary.LittleEndian
		}
		reader.transformUTF32Unit(order.Uint32(reader.pendingUnit[:]))
		reader.pendingUnitLen = 0
	}
}

func (reader *LineEndingReader) transformUnit(unit uint16) {
	if reader.pendingCR {
		if unit == '\n' {
			reader.stats.CRLFCount++
			reader.appendTargetUTF16Ending()
			reader.pendingCR = false
			return
		}
		reader.appendUTF16Unit('\r')
		reader.pendingCR = false
	}

	switch unit {
	case '\r':
		reader.pendingCR = true
	case '\n':
		reader.stats.LFCount++
		reader.appendTargetUTF16Ending()
	default:
		reader.appendUTF16Unit(unit)
	}
}

func (reader *LineEndingReader) transformUTF32Unit(unit uint32) {
	if reader.pendingCR {
		if unit == '\n' {
			reader.stats.CRLFCount++
			reader.appendTargetUTF32Ending()
			reader.pendingCR = false
			return
		}
		reader.appendUTF32Unit('\r')
		reader.pendingCR = false
	}

	switch unit {
	case '\r':
		reader.pendingCR = true
	case '\n':
		reader.stats.LFCount++
		reader.appendTargetUTF32Ending()
	default:
		reader.appendUTF32Unit(unit)
	}
}

func (reader *LineEndingReader) appendTargetEnding() {
	if reader.targetCRLF {
		reader.output = append(reader.output, reader.crByte)
	}
	reader.output = append(reader.output, reader.lfByte)
}

func (reader *LineEndingReader) appendTargetUTF16Ending() {
	if reader.targetCRLF {
		reader.appendUTF16Unit('\r')
	}
	reader.appendUTF16Unit('\n')
}

func (reader *LineEndingReader) appendUTF16Unit(unit uint16) {
	var pair [2]byte
	if reader.littleEndian {
		binary.LittleEndian.PutUint16(pair[:], unit)
	} else {
		binary.BigEndian.PutUint16(pair[:], unit)
	}
	reader.output = append(reader.output, pair[:]...)
}

func (reader *LineEndingReader) appendTargetUTF32Ending() {
	if reader.targetCRLF {
		reader.appendUTF32Unit('\r')
	}
	reader.appendUTF32Unit('\n')
}

func (reader *LineEndingReader) appendUTF32Unit(unit uint32) {
	var encoded [4]byte
	if reader.littleEndian {
		binary.LittleEndian.PutUint32(encoded[:], unit)
	} else {
		binary.BigEndian.PutUint32(encoded[:], unit)
	}
	reader.output = append(reader.output, encoded[:]...)
}

func (reader *LineEndingReader) finish() {
	if reader.hzPendingTilde {
		reader.output = append(reader.output, '~')
		reader.hzPendingTilde = false
	}
	if reader.utf16 && reader.hasPendingByte {
		reader.pendingErr = fmt.Errorf("invalid UTF-16 byte length: trailing byte")
		reader.hasPendingByte = false
	}
	if reader.utf32 && reader.pendingUnitLen != 0 {
		reader.pendingErr = fmt.Errorf("invalid UTF-32 byte length: trailing bytes")
		reader.pendingUnitLen = 0
	}
	if reader.pendingCR {
		switch {
		case reader.utf32:
			reader.appendUTF32Unit('\r')
		case reader.utf16:
			reader.appendUTF16Unit('\r')
		default:
			reader.output = append(reader.output, reader.crByte)
		}
		reader.pendingCR = false
	}
}
