package releasearchive

import (
	"errors"
	"io"
)

// boundedProgressReader rejects malformed read counts and 100 consecutive
// zero-byte reads without an error. Byte limits belong to the caller; this
// adapter does not interrupt a source whose Read itself blocks forever.
type boundedProgressReader struct {
	source     io.Reader
	emptyReads int
}

func (reader *boundedProgressReader) Read(dst []byte) (int, error) {
	n, err := reader.source.Read(dst)
	if n < 0 || n > len(dst) {
		return 0, errors.New("release asset reader returned invalid byte count")
	}
	if n == 0 && err == nil {
		reader.emptyReads++
		if reader.emptyReads >= 100 {
			return 0, io.ErrNoProgress
		}
	} else {
		reader.emptyReads = 0
	}
	return n, err
}
