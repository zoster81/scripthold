package handler

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

func TestMapOperationErrorToBatch(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		path        string
		wantMessage string
		wantCode    string
	}{
		{name: "invalid path", err: ErrPathRequired, wantMessage: ErrPathRequired.Error(), wantCode: ErrCodeInvalidPath},
		{name: "access denied", err: security.ErrPathDenied, wantMessage: security.ErrPathDenied.Error(), wantCode: ErrCodeAccessDenied},
		{name: "symlink escape", err: security.ErrSymlinkDenied, wantMessage: security.ErrSymlinkDenied.Error(), wantCode: ErrCodeSymlinkEscape},
		{name: "not found", err: fs.ErrNotExist, path: "missing.txt", wantMessage: "file not found: missing.txt", wantCode: ErrCodeNotFound},
		{name: "permission", err: fs.ErrPermission, path: "locked.txt", wantMessage: "permission denied: locked.txt", wantCode: ErrCodePermission},
		{name: "encoding", err: ErrEncodingUnsupported, wantMessage: ErrEncodingUnsupported.Error(), wantCode: ErrCodeEncoding},
		{name: "decoding", err: ErrEncodingDecode, wantMessage: ErrEncodingDecode.Error(), wantCode: ErrCodeEncoding},
		{name: "encoding output", err: ErrEncodingEncode, wantMessage: ErrEncodingEncode.Error(), wantCode: ErrCodeEncoding},
		{name: "ambiguous encoding", err: ErrEncodingAmbiguous, wantMessage: ErrEncodingAmbiguous.Error(), wantCode: ErrCodeEncodingAmbiguous},
		{name: "cancelled", err: context.Canceled, wantMessage: "operation cancelled", wantCode: ErrCodeCancelled},
		{name: "deadline", err: context.DeadlineExceeded, wantMessage: "operation cancelled", wantCode: ErrCodeCancelled},
		{name: "conflict", err: operation.New(operation.KindConflict, "target changed"), wantMessage: "target changed", wantCode: ErrCodeConflict},
		{name: "partial commit", err: operation.New(operation.KindPartialCommit, "package partially committed"), wantMessage: "package partially committed", wantCode: ErrCodePartialCommit},
		{name: "unsupported", err: operation.New(operation.KindUnsupported, "cross-filesystem move is unsupported"), wantMessage: "cross-filesystem move is unsupported", wantCode: ErrCodeUnsupported},
		{name: "limit", err: operation.New(operation.KindLimit, "result limit exceeded"), wantMessage: "result limit exceeded", wantCode: ErrCodeLimit},
		{name: "filesystem", err: operation.New(operation.KindFilesystem, "disk failure"), wantMessage: "disk failure", wantCode: ErrCodeIO},
		{name: "unknown", err: errors.New("boom"), wantMessage: "boom", wantCode: ErrCodeIO},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapOperationError(tt.err, tt.path)
			if got.Message != tt.wantMessage || got.BatchCode != tt.wantCode {
				t.Fatalf("mapOperationError() = (%q, %q), want (%q, %q)", got.Message, got.BatchCode, tt.wantMessage, tt.wantCode)
			}
		})
	}
}

func TestErrorResultFromErrorPreservesPublicMessage(t *testing.T) {
	err := operation.Wrap(operation.KindFilesystem, "read", "file.txt", errors.New("failed to read file: disk failure"))
	result := errorResultFromError(err)

	if result == nil || !result.IsError {
		t.Fatal("expected MCP error result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("content count = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", result.Content[0])
	}
	if got, want := text.Text, "failed to read file: disk failure"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	if got, want := result.Meta[ErrorCodeMetaKey], ErrCodeIO; got != want {
		t.Fatalf("error code = %#v, want %q", got, want)
	}
}
