package filetoolsserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerProjectPrompts(server *mcp.Server) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "audit_encodings",
		Title:       "Audit text encodings",
		Description: "Inspect a project tree and summarize encodings, BOMs, and line endings without modifying files.",
		Arguments: []*mcp.PromptArgument{
			{Name: "path", Description: "Directory to inspect", Required: true},
		},
	}, textPrompt(func(arguments map[string]string) string {
		return fmt.Sprintf(`Perform a read-only encoding audit under %s.

Use tree with showEncoding=true, then verify uncertain or important files with detect_encoding in chunked mode and detect_line_endings. Summarize counts by encoding and BOM state, list representative paths, and flag ambiguous files, mixed line endings, or BOMs that may affect scripts. Do not modify any file.`, arguments["path"])
	}))

	server.AddPrompt(&mcp.Prompt{
		Name:        "fix_mojibake",
		Title:       "Diagnose mojibake",
		Description: "Diagnose garbled legacy text and prepare a verified repair.",
		Arguments: []*mcp.PromptArgument{
			{Name: "path", Description: "File displaying garbled text", Required: true},
		},
	}, textPrompt(func(arguments map[string]string) string {
		return fmt.Sprintf(`Diagnose the garbled text in %s without guessing.

Detect the encoding in chunked mode, read using that result, and compare explicit likely legacy encodings only when the detected reading is still wrong. Distinguish a file-encoding problem from a consumer decoding problem. Prepare the conversion with convert_encoding dryRun=true and backup=true, obtain explicit user approval for the returned fingerprints, apply only that previewId with convert_encoding_apply, and read the file again afterward to verify it.`, arguments["path"])
	}))

	server.AddPrompt(&mcp.Prompt{
		Name:        "migrate_to_utf8",
		Title:       "Plan a UTF-8 migration",
		Description: "Preview and execute a controlled project migration to UTF-8.",
		Arguments: []*mcp.PromptArgument{
			{Name: "path", Description: "Project directory", Required: true},
			{Name: "pattern", Description: "Glob selecting files; defaults to *", Required: false},
		},
	}, textPrompt(func(arguments map[string]string) string {
		pattern := arguments["pattern"]
		if pattern == "" {
			pattern = "*"
		}
		return fmt.Sprintf(`Prepare a controlled UTF-8 migration for files matching %q under %s.

Build the file list with search_files, then call convert_encoding with paths=<list>, to="utf-8", backup=true, and dryRun=true. Report files that would change, files already compatible, and any unsupported-character or decoding failures with locations. Request explicit approval for the returned previewId and fingerprints before writing. After approval, call convert_encoding_apply with only that previewId, then verify the result with a final dry run.`, pattern, arguments["path"])
	}))
}

func textPrompt(build func(map[string]string) string) mcp.PromptHandler {
	return func(ctx context.Context, request *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: build(request.Params.Arguments)},
			}},
		}, nil
	}
}
