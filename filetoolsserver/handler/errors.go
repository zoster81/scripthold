package handler

import (
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/textstream"
)

// Sentinel errors for handler operations.
// Use errors.Is() to check for specific error types.

// Input validation errors
var (
	// ErrPathRequired is returned when a required path parameter is empty.
	ErrPathRequired = operation.New(operation.KindInvalidPath, "path is required and must be a non-empty string")

	// ErrPatternRequired is returned when a required pattern parameter is empty.
	ErrPatternRequired = operation.New(operation.KindInvalidInput, "pattern is required and must be a non-empty string")

	// ErrEditsRequired is returned when the edits array is missing or empty.
	ErrEditsRequired = operation.New(operation.KindInvalidInput, "edits array is required and must not be empty")

	// ErrPathMustBeDirectory is returned when a directory is expected but a file was provided.
	ErrPathMustBeDirectory = operation.New(operation.KindInvalidPath, "path must be a directory")
)

// Encoding errors
var (
	// ErrEncodingUnsupported is returned when an unsupported encoding is specified.
	// Wrap this error to include the encoding name: fmt.Errorf("%w: %s", ErrEncodingUnsupported, name)
	ErrEncodingUnsupported = textstream.ErrEncodingUnsupported

	// ErrEncodingAmbiguous is returned when non-empty content lacks enough
	// evidence for safe automatic decoding.
	ErrEncodingAmbiguous = textstream.ErrEncodingAmbiguous

	// ErrBOMEncodingConflict is returned when an explicit or resolved encoding conflicts with the file BOM.
	ErrBOMEncodingConflict = textstream.ErrBOMEncodingConflict

	// ErrEncodingDecode is returned when file bytes cannot be decoded with the selected encoding.
	ErrEncodingDecode = operation.New(operation.KindDecoding, "encoding decode failed")

	// ErrEncodingEncode is returned when Unicode content cannot be encoded with the selected encoding.
	ErrEncodingEncode = operation.New(operation.KindEncodingOutput, "encoding encode failed")
)

// Edit operation errors
var (
	// ErrEditNoMatch is returned when old_text cannot be found in the file.
	// Wrap this error to include context: fmt.Errorf("%w:\n%s", ErrEditNoMatch, oldText)
	ErrEditNoMatch = operation.New(operation.KindConflict, "could not find exact match for edit")

	// ErrOldTextEmpty is returned when an edit operation has an empty old_text field.
	ErrOldTextEmpty = operation.New(operation.KindInvalidInput, "edit old_text cannot be empty")
)
