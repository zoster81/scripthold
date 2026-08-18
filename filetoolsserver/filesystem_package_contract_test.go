package filetoolsserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPublicFilesystemPackageContract(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := NewServer([]string{t.TempDir()}, nil, nil)
	session := connectTestClient(t, ctx, server, "r24-surface")
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := make(map[string]*mcp.Tool, len(listed.Tools))
	for _, tool := range listed.Tools {
		byName[tool.Name] = tool
	}

	preview := byName["filesystem_package"]
	if preview == nil {
		t.Fatal("filesystem_package is missing")
	}
	if preview.Annotations == nil || !preview.Annotations.ReadOnlyHint {
		t.Fatalf("filesystem_package is not advertised read-only: %#v", preview.Annotations)
	}
	if preview.Annotations.DestructiveHint != nil && *preview.Annotations.DestructiveHint {
		t.Fatalf("filesystem_package is advertised destructive: %#v", preview.Annotations)
	}

	apply := byName["filesystem_package_apply"]
	if apply == nil {
		t.Fatal("filesystem_package_apply is missing")
	}
	if apply.Annotations == nil || apply.Annotations.ReadOnlyHint {
		t.Fatalf("filesystem_package_apply is incorrectly read-only: %#v", apply.Annotations)
	}
	if apply.Annotations.DestructiveHint == nil || !*apply.Annotations.DestructiveHint {
		t.Fatalf("filesystem_package_apply is not advertised destructive: %#v", apply.Annotations)
	}
	assertPreviewIDOnlySchema(t, apply)

	for _, removed := range []string{"create_directory", "copy_file", "move_file", "delete_file"} {
		if byName[removed] != nil {
			t.Fatalf("superseded filesystem tool %q is still public", removed)
		}
	}

	data, err := json.Marshal(preview.InputSchema)
	if err != nil {
		t.Fatalf("marshal filesystem_package schema: %v", err)
	}
	for _, required := range []string{
		`"filesystem-package-v1"`, `"mkdir"`, `"createFile"`, `"copyFile"`,
		`"copyDirectory"`, `"move"`, `"deleteFile"`, `"deleteDirectory"`,
		`"contentBase64"`,
	} {
		if !containsJSONToken(data, required) {
			t.Fatalf("filesystem_package schema is missing %s: %s", required, data)
		}
	}
	for _, forbidden := range []string{`"overwrite"`, `"command"`, `"environment"`, `"recursive"`} {
		if containsJSONToken(data, forbidden) {
			t.Fatalf("filesystem_package schema exposes forbidden field %s: %s", forbidden, data)
		}
	}
}

func TestFilesystemPackageRoundTripThroughMCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root := t.TempDir()
	parent := filepath.Join(root, "generated")
	target := filepath.Join(parent, "raw.bin")

	server := NewServer([]string{root}, nil, nil)
	session := connectTestClient(t, ctx, server, "r24-filesystem-package-roundtrip")
	prepared, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "filesystem_package",
		Arguments: map[string]any{
			"formatVersion": "filesystem-package-v1",
			"operations": []map[string]any{
				{"type": "mkdir", "path": parent},
				{"type": "createFile", "path": target, "contentBase64": "AP8K"},
			},
		},
	})
	if err != nil || prepared.IsError {
		t.Fatalf("filesystem package preview result=%#v err=%v", prepared, err)
	}
	var preview struct {
		PreviewID string `json:"previewId"`
		Plan      struct {
			OperationCount int `json:"operationCount"`
		} `json:"plan"`
	}
	decodeStructuredOutput(t, prepared.StructuredContent, &preview)
	if len(preview.PreviewID) != 64 || preview.Plan.OperationCount != 2 {
		t.Fatalf("filesystem package preview output=%+v", preview)
	}
	if _, statErr := os.Stat(parent); !os.IsNotExist(statErr) {
		t.Fatalf("filesystem package preview mutated destination: %v", statErr)
	}

	forbidden, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "filesystem_package_apply",
		Arguments: map[string]any{"previewId": preview.PreviewID, "path": target},
	})
	if err != nil || !forbidden.IsError {
		t.Fatalf("filesystem package apply override result=%#v err=%v", forbidden, err)
	}
	if _, statErr := os.Stat(parent); !os.IsNotExist(statErr) {
		t.Fatalf("rejected filesystem package apply mutated destination: %v", statErr)
	}

	applied, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "filesystem_package_apply",
		Arguments: map[string]any{"previewId": preview.PreviewID},
	})
	if err != nil || applied.IsError {
		t.Fatalf("filesystem package apply result=%#v err=%v", applied, err)
	}
	var applyOutput struct {
		Applied        bool `json:"applied"`
		PartialCommit  bool `json:"partialCommit"`
		OperationCount int  `json:"operationCount"`
	}
	decodeStructuredOutput(t, applied.StructuredContent, &applyOutput)
	if !applyOutput.Applied || applyOutput.PartialCommit || applyOutput.OperationCount != 2 {
		t.Fatalf("filesystem package apply output=%+v", applyOutput)
	}
	if data, readErr := os.ReadFile(target); readErr != nil || string(data) != string([]byte{0x00, 0xff, 0x0a}) {
		t.Fatalf("filesystem package raw bytes=%x err=%v", data, readErr)
	}

	replay, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "filesystem_package_apply",
		Arguments: map[string]any{"previewId": preview.PreviewID},
	})
	if err != nil || !replay.IsError {
		t.Fatalf("filesystem package replay result=%#v err=%v", replay, err)
	}
}

func containsJSONToken(data []byte, token string) bool {
	for index := 0; index+len(token) <= len(data); index++ {
		if string(data[index:index+len(token)]) == token {
			return true
		}
	}
	return false
}
