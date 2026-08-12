// Package filesystempackage implements the transport-independent R24 filesystem package workflow.
package filesystempackage

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/zoster81/scripthold/internal/operation"
)

const (
	FormatV1 = "filesystem-package-v1"

	OperationMkdir           = "mkdir"
	OperationCreateFile      = "createFile"
	OperationCopyFile        = "copyFile"
	OperationCopyDirectory   = "copyDirectory"
	OperationMove            = "move"
	OperationDeleteFile      = "deleteFile"
	OperationDeleteDirectory = "deleteDirectory"

	maxManifestDecodeBytes = 64 * 1024 * 1024
)

// Limits contains the bounded resources used by one filesystem package.
type Limits struct {
	MaxOperations       int
	MaxManifestBytes    int64
	MaxPathBytes        int
	MaxFileBytes        int64
	MaxRecursiveEntries int
	MaxRecursiveDepth   int
	MaxAggregateBytes   int64
	MaxStagingBytes     int64
	MaxEntryDetails     int
	MaxOutputBytes      int64
	MaxPreviews         int
	MaxPreviewBytes     int64
	PreviewTTLSeconds   int
}

// Manifest is the complete public filesystem_package input.
type Manifest struct {
	FormatVersion string      `json:"formatVersion"`
	Operations    []Operation `json:"operations"`

	rawBytes int64
}

// UnmarshalJSON rejects unknown top-level fields and unbounded raw manifests.
func (manifest *Manifest) UnmarshalJSON(data []byte) error {
	if len(data) > maxManifestDecodeBytes {
		return operation.New(operation.KindLimit, "filesystem package manifest exceeds the hard decode limit")
	}
	type alias Manifest
	var decoded alias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	*manifest = Manifest(decoded)
	manifest.rawBytes = int64(len(data))
	return nil
}

// Operation is one of the seven closed filesystem-package-v1 operation forms.
// Content contains decoded raw bytes and is never serialized directly.
type Operation struct {
	Type          string `json:"type"`
	Path          string `json:"path,omitempty"`
	Source        string `json:"source,omitempty"`
	Destination   string `json:"destination,omitempty"`
	ContentBase64 string `json:"contentBase64,omitempty"`
	Content       []byte `json:"-"`

	contentPresent bool
}

func (item *Operation) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	rawType, ok := fields["type"]
	if !ok {
		return operation.New(operation.KindInvalidInput, "filesystem operation type is required")
	}
	var operationType string
	if err := json.Unmarshal(rawType, &operationType); err != nil || operationType == "" {
		return operation.New(operation.KindInvalidInput, "filesystem operation type must be a non-empty string")
	}

	allowed := map[string]struct{}{"type": {}}
	switch operationType {
	case OperationMkdir, OperationDeleteFile, OperationDeleteDirectory:
		allowed["path"] = struct{}{}
	case OperationCreateFile:
		allowed["path"] = struct{}{}
		allowed["contentBase64"] = struct{}{}
	case OperationCopyFile, OperationCopyDirectory, OperationMove:
		allowed["source"] = struct{}{}
		allowed["destination"] = struct{}{}
	default:
		return operation.New(operation.KindInvalidInput, fmt.Sprintf("unsupported filesystem operation type %q", operationType))
	}
	for name := range fields {
		if _, ok := allowed[name]; !ok {
			return operation.New(operation.KindInvalidInput, fmt.Sprintf("field %q is not valid for filesystem operation %q", name, operationType))
		}
	}

	decodeStrict := func(target any) error {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(target); err != nil {
			return err
		}
		return ensureJSONEOF(decoder)
	}
	var decoded Operation
	switch operationType {
	case OperationMkdir, OperationDeleteFile, OperationDeleteDirectory:
		var raw struct {
			Type string `json:"type"`
			Path string `json:"path"`
		}
		if err := decodeStrict(&raw); err != nil {
			return err
		}
		decoded.Type, decoded.Path = raw.Type, raw.Path
	case OperationCreateFile:
		var raw struct {
			Type          string `json:"type"`
			Path          string `json:"path"`
			ContentBase64 string `json:"contentBase64"`
		}
		if err := decodeStrict(&raw); err != nil {
			return err
		}
		if _, ok := fields["contentBase64"]; !ok {
			return operation.New(operation.KindInvalidInput, "createFile requires contentBase64")
		}
		content, err := base64.StdEncoding.Strict().DecodeString(raw.ContentBase64)
		if err != nil || base64.StdEncoding.EncodeToString(content) != raw.ContentBase64 {
			return operation.New(operation.KindInvalidInput, "contentBase64 must be canonical standard base64")
		}
		decoded.Type, decoded.Path = raw.Type, raw.Path
		decoded.ContentBase64, decoded.Content = raw.ContentBase64, content
		decoded.contentPresent = true
	case OperationCopyFile, OperationCopyDirectory, OperationMove:
		var raw struct {
			Type        string `json:"type"`
			Source      string `json:"source"`
			Destination string `json:"destination"`
		}
		if err := decodeStrict(&raw); err != nil {
			return err
		}
		decoded.Type, decoded.Source, decoded.Destination = raw.Type, raw.Source, raw.Destination
	}
	*item = decoded
	return nil
}

// ValidateManifest applies the configurable package limits and validates
// programmatically constructed manifests as strictly as decoded manifests.
func ValidateManifest(manifest Manifest, limits Limits) error {
	if err := validateLimits(limits); err != nil {
		return err
	}
	if manifest.FormatVersion != FormatV1 {
		return operation.New(operation.KindInvalidInput, "formatVersion must be filesystem-package-v1")
	}
	if manifest.rawBytes > limits.MaxManifestBytes {
		return operation.New(operation.KindLimit, fmt.Sprintf("filesystem package manifest exceeds limit %d", limits.MaxManifestBytes))
	}
	if len(manifest.Operations) == 0 {
		return operation.New(operation.KindInvalidInput, "filesystem package must contain at least one operation")
	}
	if len(manifest.Operations) > limits.MaxOperations {
		return operation.New(operation.KindLimit, fmt.Sprintf("filesystem package operation count exceeds limit %d", limits.MaxOperations))
	}

	encodedBytes := int64(len(`{"formatVersion":"","operations":[]}`) + len(manifest.FormatVersion))
	for index := range manifest.Operations {
		item := &manifest.Operations[index]
		if err := validateOperation(index, item, limits); err != nil {
			return err
		}
		if manifest.rawBytes != 0 {
			continue
		}
		operationBytes := int64(len(item.Type) + len(item.Path) + len(item.Source) + len(item.Destination) + 64)
		if item.Type == OperationCreateFile {
			operationBytes += int64(base64.StdEncoding.EncodedLen(len(item.Content)))
		}
		if operationBytes > limits.MaxManifestBytes-encodedBytes {
			return operation.New(operation.KindLimit, fmt.Sprintf("filesystem package manifest exceeds limit %d", limits.MaxManifestBytes))
		}
		encodedBytes += operationBytes
	}
	return nil
}

func validateOperation(index int, item *Operation, limits Limits) error {
	invalid := func(message string) error {
		return operation.New(operation.KindInvalidInput, fmt.Sprintf("operation %d: %s", index, message))
	}
	validatePath := func(name, value string) error {
		if value == "" {
			return invalid(name + " is required")
		}
		if len(value) > limits.MaxPathBytes {
			return operation.New(operation.KindLimit, fmt.Sprintf("operation %d: %s exceeds path byte limit %d", index, name, limits.MaxPathBytes))
		}
		return nil
	}

	switch item.Type {
	case OperationMkdir, OperationDeleteFile, OperationDeleteDirectory:
		if err := validatePath("path", item.Path); err != nil {
			return err
		}
		if item.Source != "" || item.Destination != "" || item.ContentBase64 != "" || item.Content != nil || item.contentPresent {
			return invalid("operation contains fields belonging to another operation type")
		}
	case OperationCreateFile:
		if err := validatePath("path", item.Path); err != nil {
			return err
		}
		if item.Source != "" || item.Destination != "" {
			return invalid("createFile accepts only path and contentBase64")
		}
		if !item.contentPresent && item.Content == nil {
			return invalid("createFile requires contentBase64")
		}
		if int64(len(item.Content)) > limits.MaxFileBytes {
			return operation.New(operation.KindLimit, fmt.Sprintf("operation %d: createFile content exceeds file byte limit %d", index, limits.MaxFileBytes))
		}
	case OperationCopyFile, OperationCopyDirectory, OperationMove:
		if err := validatePath("source", item.Source); err != nil {
			return err
		}
		if err := validatePath("destination", item.Destination); err != nil {
			return err
		}
		if item.Path != "" || item.ContentBase64 != "" || item.Content != nil || item.contentPresent {
			return invalid("operation contains fields belonging to another operation type")
		}
	default:
		return invalid("unsupported operation type")
	}
	return nil
}

func validateLimits(limits Limits) error {
	if limits.MaxOperations <= 0 || limits.MaxManifestBytes <= 0 || limits.MaxPathBytes <= 0 || limits.MaxFileBytes <= 0 ||
		limits.MaxRecursiveEntries <= 0 || limits.MaxRecursiveDepth <= 0 || limits.MaxAggregateBytes <= 0 || limits.MaxStagingBytes <= 0 {
		return operation.New(operation.KindInvalidInput, "filesystem package limits must be positive")
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return operation.New(operation.KindInvalidInput, "multiple JSON values are not allowed")
}
