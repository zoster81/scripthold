package backupstore

import (
	"encoding/json"
	"errors"
	"io"
)

const maxRecoveryJSONDepth = 64

func decodeStrictRecoveryJSON(data []byte, maxBytes int, destination any) error {
	if len(data) == 0 || len(data) > maxBytes {
		return errors.New("recovery document size is invalid")
	}
	if err := rejectDuplicateRecoveryJSONKeys(data); err != nil {
		return err
	}

	decoder := json.NewDecoder(newByteReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return errors.New("recovery document is malformed")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("recovery document contains trailing data")
	}
	return nil
}

func rejectDuplicateRecoveryJSONKeys(data []byte) error {
	decoder := json.NewDecoder(newByteReader(data))
	if err := consumeRecoveryJSONValue(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("recovery document contains trailing data")
	}
	return nil
}

func consumeRecoveryJSONValue(decoder *json.Decoder, depth int) error {
	if depth > maxRecoveryJSONDepth {
		return errors.New("recovery document nesting is too deep")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.New("recovery document is malformed")
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}

	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return errors.New("recovery document is malformed")
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("recovery document is malformed")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("recovery document contains a duplicate field")
			}
			seen[key] = struct{}{}
			if err := consumeRecoveryJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("recovery document is malformed")
		}
		return nil
	case '[':
		for decoder.More() {
			if err := consumeRecoveryJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("recovery document is malformed")
		}
		return nil
	default:
		return errors.New("recovery document is malformed")
	}
}

type byteReader struct {
	data []byte
	off  int
}

func newByteReader(data []byte) *byteReader {
	return &byteReader{data: data}
}

func (reader *byteReader) Read(buffer []byte) (int, error) {
	if reader.off >= len(reader.data) {
		return 0, io.EOF
	}
	count := copy(buffer, reader.data[reader.off:])
	reader.off += count
	return count, nil
}
