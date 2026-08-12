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

func TestR23PublicMutationBoundaries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := NewServer([]string{t.TempDir()}, nil, nil)
	session := connectTestClient(t, ctx, server, "r23-surface")
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := make(map[string]*mcp.Tool, len(listed.Tools))
	for _, tool := range listed.Tools {
		byName[tool.Name] = tool
	}

	for _, name := range []string{"edit_file", "patch_package", "backup_store", "manage_bom", "convert_encoding"} {
		tool := byName[name]
		if tool == nil {
			t.Fatalf("read-only R23 tool %q is missing", name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("R23 preparation tool %q is not advertised read-only: %#v", name, tool.Annotations)
		}
		if tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
			t.Fatalf("R23 preparation tool %q is advertised destructive: %#v", name, tool.Annotations)
		}
	}

	for _, name := range []string{
		"edit_file_apply",
		"patch_package_apply",
		"backup_restore_apply",
		"backup_gc_apply",
		"manage_bom_apply",
		"convert_encoding_apply",
	} {
		tool := byName[name]
		if tool == nil {
			t.Fatalf("R23 apply tool %q is missing", name)
		}
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint {
			t.Fatalf("R23 apply tool %q is incorrectly read-only: %#v", name, tool.Annotations)
		}
		assertPreviewIDOnlySchema(t, tool)
	}
}

func TestR23ReadOnlyToolNamesRejectLegacyMutationForms(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	original := []byte("alpha\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	server := NewServer([]string{root}, nil, nil)
	session := connectTestClient(t, ctx, server, "r23-read-only-rejections")

	cases := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "edit_file", arguments: map[string]any{"action": "direct", "path": path, "edits": []map[string]any{{"oldText": "alpha", "newText": "omega"}}}},
		{name: "patch_package", arguments: map[string]any{"action": "apply", "manifest": map[string]any{}}},
		{name: "backup_store", arguments: map[string]any{"action": "gcApply", "previewId": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		{name: "manage_bom", arguments: map[string]any{"action": "add", "path": path, "encoding": "utf-8"}},
		{name: "convert_encoding", arguments: map[string]any{"path": path, "from": "utf-8", "to": "utf-16-le", "dryRun": false}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: test.name, Arguments: test.arguments})
			if err != nil {
				t.Fatalf("call %s: %v", test.name, err)
			}
			if !result.IsError {
				t.Fatalf("legacy mutation form for %s unexpectedly succeeded: %#v", test.name, result)
			}
			actual, readErr := os.ReadFile(path)
			if readErr != nil || string(actual) != string(original) {
				t.Fatalf("%s mutated sentinel bytes=%q err=%v", test.name, actual, readErr)
			}
			if _, statErr := os.Stat(path + ".bak"); !os.IsNotExist(statErr) {
				t.Fatalf("%s created adjacent backup during rejected read-only call: %v", test.name, statErr)
			}
		})
	}
}

func TestR23EditPreviewThroughMCPIsSideEffectFreeAndApplyIsPreviewIDOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	path := filepath.Join(root, "edit.txt")
	if err := os.WriteFile(path, []byte("alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := NewServer([]string{root}, nil, nil)
	session := connectTestClient(t, ctx, server, "r23-edit-preview-apply")

	prepared, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "edit_file",
		Arguments: map[string]any{
			"action": "preview",
			"path":   path,
			"edits":  []map[string]any{{"oldText": "alpha", "newText": "omega"}},
		},
	})
	if err != nil || prepared.IsError {
		t.Fatalf("edit preview result=%#v err=%v", prepared, err)
	}
	var preview struct {
		PreviewID         string `json:"previewId"`
		TargetFingerprint string `json:"targetFingerprint"`
		ResultFingerprint string `json:"resultFingerprint"`
	}
	decodeStructuredOutput(t, prepared.StructuredContent, &preview)
	if len(preview.PreviewID) != 64 || len(preview.TargetFingerprint) != 64 || len(preview.ResultFingerprint) != 64 || preview.TargetFingerprint == preview.ResultFingerprint {
		t.Fatalf("edit preview output=%+v", preview)
	}
	if data, readErr := os.ReadFile(path); readErr != nil || string(data) != "alpha\n" {
		t.Fatalf("preview mutated file=%q err=%v", data, readErr)
	}

	forbidden, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "edit_file_apply",
		Arguments: map[string]any{"previewId": preview.PreviewID, "path": path},
	})
	if err != nil || !forbidden.IsError {
		t.Fatalf("apply override result=%#v err=%v", forbidden, err)
	}
	if data, readErr := os.ReadFile(path); readErr != nil || string(data) != "alpha\n" {
		t.Fatalf("rejected apply mutated file=%q err=%v", data, readErr)
	}

	applied, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "edit_file_apply",
		Arguments: map[string]any{"previewId": preview.PreviewID},
	})
	if err != nil || applied.IsError {
		t.Fatalf("edit apply result=%#v err=%v", applied, err)
	}
	if data, readErr := os.ReadFile(path); readErr != nil || string(data) != "omega\n" {
		t.Fatalf("apply bytes=%q err=%v", data, readErr)
	}
	replay, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "edit_file_apply",
		Arguments: map[string]any{"previewId": preview.PreviewID},
	})
	if err != nil || !replay.IsError {
		t.Fatalf("replay result=%#v err=%v", replay, err)
	}
}

func assertPreviewIDOnlySchema(t *testing.T, tool *mcp.Tool) {
	t.Helper()
	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal %s input schema: %v", tool.Name, err)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode %s input schema: %v", tool.Name, err)
	}
	if len(schema.Properties) != 1 || schema.Properties["previewId"] == nil {
		t.Fatalf("%s input properties = %v, want previewId only", tool.Name, schema.Properties)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "previewId" {
		t.Fatalf("%s required fields = %v, want [previewId]", tool.Name, schema.Required)
	}
}
