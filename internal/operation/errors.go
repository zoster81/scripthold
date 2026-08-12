// Package operation defines transport-independent error categories shared by
// domain primitives and MCP adapters.
package operation

import (
	"context"
	"errors"
	"io/fs"
)

// Kind identifies a stable operation failure category without changing the
// human-readable error message.
type Kind uint8

const (
	KindUnknown Kind = iota
	KindInvalidInput
	KindInvalidPath
	KindAccessDenied
	KindSymlinkEscape
	KindNotFound
	KindPermission
	KindEncoding
	KindDecoding
	KindEncodingOutput
	KindConflict
	KindPartialCommit
	KindUnsupported
	KindCancelled
	KindLimit
	KindFilesystem
)

func (kind Kind) String() string {
	switch kind {
	case KindInvalidInput:
		return "invalid_input"
	case KindInvalidPath:
		return "invalid_path"
	case KindAccessDenied:
		return "access_denied"
	case KindSymlinkEscape:
		return "symlink_escape"
	case KindNotFound:
		return "not_found"
	case KindPermission:
		return "permission"
	case KindEncoding:
		return "encoding"
	case KindDecoding:
		return "decoding"
	case KindEncodingOutput:
		return "encoding_output"
	case KindConflict:
		return "conflict"
	case KindPartialCommit:
		return "partial_commit"
	case KindUnsupported:
		return "unsupported"
	case KindCancelled:
		return "cancelled"
	case KindLimit:
		return "limit"
	case KindFilesystem:
		return "filesystem"
	default:
		return "unknown"
	}
}

// Error adds stable category and operation metadata while preserving the
// original message and errors.Is/errors.As chain.
type Error struct {
	Kind      Kind
	Operation string
	Path      string
	Err       error
}

func (err *Error) Error() string {
	if err == nil {
		return "<nil>"
	}
	if err.Err != nil {
		return err.Err.Error()
	}
	return err.Kind.String()
}

func (err *Error) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

// New creates a typed error with a stable public message.
func New(kind Kind, message string) *Error {
	return &Error{Kind: kind, Err: errors.New(message)}
}

// Wrap annotates err without changing its message. A nil error remains nil.
func Wrap(kind Kind, operation, path string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{
		Kind:      kind,
		Operation: operation,
		Path:      path,
		Err:       err,
	}
}

// WrapFilesystem classifies standard filesystem errors, preserves any existing
// typed category, and treats all remaining failures as filesystem errors.
func WrapFilesystem(operation, path string, err error) error {
	if err == nil {
		return nil
	}
	kind := KindOf(err)
	if kind == KindUnknown {
		kind = KindFilesystem
	}
	return Wrap(kind, operation, path, err)
}

// KindOf returns the first explicit typed category in the error tree. When no
// typed error exists, standard cancellation and filesystem sentinel errors are
// classified directly.
func KindOf(err error) Kind {
	if err == nil {
		return KindUnknown
	}

	var typed *Error
	if errors.As(err, &typed) && typed != nil {
		return typed.Kind
	}

	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return KindCancelled
	case errors.Is(err, fs.ErrNotExist):
		return KindNotFound
	case errors.Is(err, fs.ErrPermission):
		return KindPermission
	default:
		return KindUnknown
	}
}
