package handler

import (
	"io"
	"os"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
)

func readFileHead(path string, maxBytes int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	buffer := make([]byte, max(0, maxBytes))
	read, err := io.ReadFull(file, buffer)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buffer[:read], nil
}

func detectBOMPrefix(session *filesystem.ReadSession) (fileEncoding.DetectionResult, bool, error) {
	length := min(int64(4), session.Size())
	prefix := make([]byte, int(length))
	if length > 0 {
		read, err := session.ReadAt(prefix, 0)
		if err != nil && err != io.EOF {
			return fileEncoding.DetectionResult{}, false, err
		}
		prefix = prefix[:read]
	}
	result, found := fileEncoding.DetectBOM(prefix)
	return result, found, nil
}
