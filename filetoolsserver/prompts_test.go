package filetoolsserver

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProjectPromptsAvailableOnSharedServer(t *testing.T) {
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := NewServer([]string{t.TempDir()}, nil, nil).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "prompt-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	seen := map[string]bool{}
	for prompt, listErr := range clientSession.Prompts(ctx, nil) {
		if listErr != nil {
			t.Fatal(listErr)
		}
		seen[prompt.Name] = true
	}
	for _, name := range []string{"audit_encodings", "fix_mojibake", "migrate_to_utf8"} {
		if !seen[name] {
			t.Fatalf("prompt %q missing from %#v", name, seen)
		}
	}
	tests := []struct {
		name      string
		arguments map[string]string
		contains  []string
	}{
		{
			name:      "audit_encodings",
			arguments: map[string]string{"path": "/project"},
			contains:  []string{"read-only", "detect_encoding", "detect_line_endings", "Do not modify any file"},
		},
		{
			name:      "fix_mojibake",
			arguments: map[string]string{"path": "/project/legacy.data"},
			contains:  []string{"Detect the encoding", "explicit likely legacy encodings", "approval", "backup=true", "read the file again"},
		},
		{
			name:      "migrate_to_utf8",
			arguments: map[string]string{"path": "/project", "pattern": "*.txt"},
			contains:  []string{"convert_encoding", "dryRun=true", "approval", "backup=true", "final dry run", "*.txt"},
		},
	}
	for _, test := range tests {
		result, err := clientSession.GetPrompt(ctx, &mcp.GetPromptParams{Name: test.name, Arguments: test.arguments})
		if err != nil {
			t.Fatalf("GetPrompt(%s): %v", test.name, err)
		}
		if len(result.Messages) != 1 {
			t.Fatalf("%s messages = %d", test.name, len(result.Messages))
		}
		text, ok := result.Messages[0].Content.(*mcp.TextContent)
		if !ok {
			t.Fatalf("%s unexpected prompt content: %#v", test.name, result.Messages[0].Content)
		}
		for _, expected := range test.contains {
			if !strings.Contains(text.Text, expected) {
				t.Fatalf("%s prompt missing %q: %s", test.name, expected, text.Text)
			}
		}
	}
}
