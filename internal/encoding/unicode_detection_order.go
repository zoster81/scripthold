package encoding

type detectionByteOrder uint8

const (
	detectionLittleEndian detectionByteOrder = iota
	detectionBigEndian
)

func (order detectionByteOrder) uint16(first, second byte) uint16 {
	if order == detectionLittleEndian {
		return uint16(first) | uint16(second)<<8
	}
	return uint16(first)<<8 | uint16(second)
}

func (order detectionByteOrder) uint32(raw [4]byte) uint32 {
	if order == detectionLittleEndian {
		return uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
	}
	return uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])
}

func (order detectionByteOrder) encodeUint32(value uint32) [4]byte {
	if order == detectionLittleEndian {
		return [4]byte{byte(value), byte(value >> 8), byte(value >> 16), byte(value >> 24)}
	}
	return [4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}
}
