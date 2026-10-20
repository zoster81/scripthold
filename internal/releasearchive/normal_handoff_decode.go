package releasearchive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// The 15-file mapping contains only names, byte sizes, and SHA-256 values.
// This cap prevents an untrusted producer from allocating memory proportional
// to an arbitrarily large JSON document before any filesystem preflight.
const maxRawNormalPlanBytes = 16 * 1024

// Go's io.ReadAll is byte-bounded by LimitReader, but it can spin forever
// if a source repeatedly violates the Reader progress convention. Guard the
// caller-owned source independently without changing valid stream bytes.
type normalPlanProgressReader struct {
	source     io.Reader
	emptyReads int
}

func (reader *normalPlanProgressReader) Read(dst []byte) (int, error) {
	n, err := reader.source.Read(dst)
	if n < 0 || n > len(dst) {
		return 0, errors.New("normal staging plan reader returned invalid byte count")
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

// DecodeRawNormalHandoffPlan parses an untrusted producer mapping without
// filesystem access. It rejects unknown, repeated and case-aliased JSON fields,
// trailing data, oversized input and invalid file metadata. The caller must
// still authenticate the GitHub job, run, artifact IDs and exact asset allowlist.
func DecodeRawNormalHandoffPlan(source io.Reader) ([]RawNormalFile, error) {
	if source == nil {
		return nil, errors.New("normal staging plan source is missing")
	}
	data, err := io.ReadAll(io.LimitReader(&normalPlanProgressReader{source: source}, maxRawNormalPlanBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read normal staging plan: %w", err)
	}
	if len(data) == 0 || len(data) > maxRawNormalPlanBytes {
		return nil, errors.New("normal staging plan size invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	start, err := decoder.Token()
	if err != nil || start != json.Delim('[') {
		return nil, errors.New("normal staging plan must be a JSON array")
	}
	files := make([]RawNormalFile, 0, 15)
	for decoder.More() {
		if len(files) >= 15 {
			return nil, errors.New("normal staging plan exceeds 15 records")
		}
		file, err := decodeNormalFile(decoder)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim(']') {
		return nil, errors.New("normal staging plan array not closed")
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, errors.New("normal staging plan has trailing input")
	}
	if _, _, err := validateRawNormalEntries(files); err != nil {
		return nil, err
	}
	return files, nil
}

func decodeNormalFile(decoder *json.Decoder) (RawNormalFile, error) {
	var file RawNormalFile
	err := decodeExactJSONObject(decoder, map[string]func(*json.Decoder) error{
		"downloadName": func(dec *json.Decoder) error {
			name, err := decodeJSONString(dec)
			file.DownloadName = name
			return err
		},
		"original": func(dec *json.Decoder) error {
			return decodeExactJSONObject(dec, map[string]func(*json.Decoder) error{
				"name": func(dec *json.Decoder) error {
					name, err := decodeJSONString(dec)
					file.Original.Name = name
					return err
				},
				"size": func(dec *json.Decoder) error {
					token, err := dec.Token()
					if err != nil {
						return err
					}
					number, ok := token.(json.Number)
					if !ok {
						return errors.New("normal staging size must be an integer")
					}
					file.Original.Size, err = number.Int64()
					return err
				},
				"sha256": func(dec *json.Decoder) error {
					digest, err := decodeJSONString(dec)
					file.Original.SHA256 = digest
					return err
				},
			})
		},
	})
	return file, err
}

func decodeJSONString(decoder *json.Decoder) (string, error) {
	token, err := decoder.Token()
	if err != nil {
		return "", err
	}
	value, ok := token.(string)
	if !ok {
		return "", errors.New("normal staging field must be a JSON string")
	}
	return value, nil
}

func decodeExactJSONObject(decoder *json.Decoder, fields map[string]func(*json.Decoder) error) error {
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return errors.New("normal staging record must be a JSON object")
	}
	seen := make(map[string]bool, len(fields))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("invalid normal staging field key")
		}
		decode, allowed := fields[key]
		if !allowed || seen[key] {
			return errors.New("unexpected or repeated normal staging field")
		}
		seen[key] = true
		if err := decode(decoder); err != nil {
			return err
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || len(seen) != len(fields) {
		return errors.New("incomplete normal staging record")
	}
	return nil
}
