package filetoolsserver

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/toolcatalog"
)

// Version is set at build time via ldflags
var Version = "dev"

// Server instructions for AI assistants
const serverInstructions = `Scripthold provides secure, encoding-aware filesystem tools and durable asynchronous shell/script tasks.

Use these tools when encoding, BOM, line endings, bounded traversal, atomic mutation, backups, or persistent task execution matter. Filesystem access is limited to startup roots. Encoding detection uses BOM/content evidence, never filenames; ambiguous input requires an explicit encoding. Mutations revalidate paths and preserve encoding/BOM/line endings where documented.

For long work use task_run, then task_get/task_logs/task_list; tasks survive MCP reconnects and support cancellation. Use preview/apply workflows for sensitive edits, patch packages, restores, and GC. Tool errors expose stable error codes.

Call check_for_updates once at session start and report available updates.`

func catalogTool(name string) *mcp.Tool {
	definition := toolcatalog.Must(name)
	return &mcp.Tool{
		Name:        definition.Name,
		Description: definition.Description,
		Annotations: &mcp.ToolAnnotations{
			Title:           definition.Title,
			ReadOnlyHint:    definition.Annotations.ReadOnlyHint,
			IdempotentHint:  definition.Annotations.IdempotentHint,
			DestructiveHint: definition.Annotations.DestructiveHint,
			OpenWorldHint:   definition.Annotations.OpenWorldHint,
		},
	}
}

// emptyInputCatalogTool supplies an explicit properties object for no-argument
// tools. Some MCP-to-function-schema bridges require it even though JSON Schema
// permits an object schema without properties.
func emptyInputCatalogTool(name string) *mcp.Tool {
	tool := catalogTool(name)
	tool.InputSchema = map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
	return tool
}

// addTool keeps typed validation and structured results while replacing large
// inferred output schemas with a compact object schema. Detailed output
// schemas are optional in MCP and can exceed connector definition budgets.
func addTool[In, Out any](server *mcp.Server, tool *mcp.Tool, typedHandler mcp.ToolHandlerFor[In, Out]) {
	tool.OutputSchema = map[string]any{"type": "object"}
	mcp.AddTool(server, tool, typedHandler)
}

// ServerOptions contains process-wide MCP server policy. Every connection to
// the returned server shares the same configured directories and tool policy.
type ServerOptions struct {
	Version                string
	AllowedDirectories     []string
	ProtectedDirectories   []string
	BackupStore            handler.BackupStoreReader
	TaskStore              handler.TaskStore
	Logger                 *slog.Logger
	Config                 *config.Config
	ExecutionPolicy        *handler.ExecutionPolicy
	EnableClientRoots      bool
	DisableModernDiscovery bool
	LifecycleContext       context.Context
}

// BuildServer creates the shared MCP server without starting a transport.
func BuildServer(options ServerOptions) *mcp.Server {
	version := options.Version
	if version == "" {
		version = Version
	}
	cfg := options.Config
	if cfg == nil {
		cfg = config.Load()
	}
	lifecycleCtx := options.LifecycleContext
	if lifecycleCtx == nil {
		lifecycleCtx = context.Background()
	}

	protectedDirectories := append([]string(nil), options.ProtectedDirectories...)
	handlerOptions := []handler.Option{
		handler.WithConfig(cfg),
		handler.WithProtectedDirectories(protectedDirectories),
		handler.WithBackupStore(options.BackupStore),
		handler.WithTaskStore(options.TaskStore),
	}
	if options.ExecutionPolicy != nil {
		handlerOptions = append(handlerOptions, handler.WithExecutionPolicy(*options.ExecutionPolicy))
	}
	h := handler.NewHandler(options.AllowedDirectories, handlerOptions...)
	logger := options.Logger
	impl := &mcp.Implementation{
		Name:    "scripthold",
		Version: version,
	}

	serverOpts := &mcp.ServerOptions{
		Instructions:            serverInstructions,
		Logger:                  logger,
		InitializedHandler:      createInitializedHandler(lifecycleCtx, h, version, options.EnableClientRoots),
		RootsListChangedHandler: createRootsListChangedHandler(h, options.EnableClientRoots),
	}
	server := mcp.NewServer(impl, serverOpts)
	registerProjectPrompts(server)

	server.AddReceivingMiddleware(createDiscoveryMiddleware(h, options.EnableClientRoots, options.DisableModernDiscovery))
	// Repair array/object args some MCP clients send as JSON-encoded strings.
	server.AddReceivingMiddleware(handler.RepairStringifiedArrayArgs)

	// Register all tools using the new AddTool API with annotations
	// All handlers are wrapped with recovery middleware (and logging if logger is provided)

	// Read-only tools
	addTool(server, catalogTool("read_text_file"), handler.Wrap(logger, "read_text_file", h.HandleReadTextFile))

	addTool(server, catalogTool("read_multiple_files"), handler.Wrap(logger, "read_multiple_files", h.HandleReadMultipleFiles))

	addTool(server, catalogTool("list_directory"), handler.Wrap(logger, "list_directory", h.HandleListDirectory))

	addTool(server, emptyInputCatalogTool("list_encodings"), handler.Wrap(logger, "list_encodings", h.HandleListEncodings))

	addTool(server, catalogTool("detect_encoding"), handler.Wrap(logger, "detect_encoding", h.HandleDetectEncoding))

	addTool(server, catalogTool("grep_text_files"), handler.Wrap(logger, "grep_text_files", h.HandleGrep))

	addTool(server, emptyInputCatalogTool("list_allowed_directories"), handler.Wrap(logger, "list_allowed_directories", h.HandleListAllowedDirectories))

	addTool(server, catalogTool("get_file_info"), handler.Wrap(logger, "get_file_info", h.HandleGetFileInfo))

	addTool(server, catalogTool("tree"), handler.Wrap(logger, "tree", h.HandleTree))

	addTool(server, catalogTool("search_files"), handler.Wrap(logger, "search_files", h.HandleSearchFiles))

	addTool(server, catalogTool("fingerprint_paths"), handler.Wrap(logger, "fingerprint_paths", h.HandleFingerprintPaths))

	addTool(server, catalogTool("verify_state"), handler.Wrap(logger, "verify_state", h.HandleVerifyState))

	addTool(server, catalogTool("backup_store"), handler.Wrap(logger, "backup_store", h.HandleBackupStoreRead))

	addTool(server, catalogTool("detect_line_endings"), handler.Wrap(logger, "detect_line_endings", h.HandleDetectLineEndings))

	// Write tools
	addTool(server, catalogTool("manage_bom"), handler.Wrap(logger, "manage_bom", h.HandleManageBOMRead))

	addTool(server, catalogTool("change_line_endings"), handler.Wrap(logger, "change_line_endings", h.HandleChangeLineEndings))

	addTool(server, catalogTool("create_directory"), handler.Wrap(logger, "create_directory", h.HandleCreateDirectory))

	addTool(server, catalogTool("write_whole_file"), handler.Wrap(logger, "write_whole_file", h.HandleWriteWholeFile))

	addTool(server, catalogTool("move_file"), handler.Wrap(logger, "move_file", h.HandleMoveFile))

	addTool(server, catalogTool("copy_file"), handler.Wrap(logger, "copy_file", h.HandleCopyFile))

	addTool(server, catalogTool("delete_file"), handler.Wrap(logger, "delete_file", h.HandleDeleteFile))

	addTool(server, catalogTool("edit_file"), handler.Wrap(logger, "edit_file", h.HandleEditFilePreview))

	addTool(server, catalogTool("patch_package"), handler.Wrap(logger, "patch_package", h.HandlePatchPackageRead))

	addTool(server, catalogTool("convert_encoding"), handler.Wrap(logger, "convert_encoding", h.HandleConvertEncodingPreview))

	// Approval-bound apply tools accept only previewId.
	addTool(server, catalogTool("edit_file_apply"), handler.Wrap(logger, "edit_file_apply", h.HandleEditFileApply))
	addTool(server, catalogTool("patch_package_apply"), handler.Wrap(logger, "patch_package_apply", h.HandlePatchPackageApply))
	addTool(server, catalogTool("backup_restore_apply"), handler.Wrap(logger, "backup_restore_apply", h.HandleBackupRestoreApply))
	addTool(server, catalogTool("backup_gc_apply"), handler.Wrap(logger, "backup_gc_apply", h.HandleBackupGCApply))
	addTool(server, catalogTool("manage_bom_apply"), handler.Wrap(logger, "manage_bom_apply", h.HandleManageBOMApply))
	addTool(server, catalogTool("convert_encoding_apply"), handler.Wrap(logger, "convert_encoding_apply", h.HandleConvertEncodingApply))

	// Durable asynchronous execution. The MCP call only admits, observes, or
	// cancels work; a separate worker/helper topology owns process lifetime.
	addTool(server, catalogTool("task_run"), handler.Wrap(logger, "task_run", h.HandleTaskRun))
	addTool(server, catalogTool("task_list"), handler.Wrap(logger, "task_list", h.HandleTaskList))
	addTool(server, catalogTool("task_get"), handler.Wrap(logger, "task_get", h.HandleTaskGet))
	addTool(server, catalogTool("task_logs"), handler.Wrap(logger, "task_logs", h.HandleTaskLogs))
	addTool(server, catalogTool("task_cancel"), handler.Wrap(logger, "task_cancel", h.HandleTaskCancel))
	addTool(server, catalogTool("check_for_updates"), handler.Wrap(logger, "check_for_updates", handler.NewCheckUpdateHandler(version)))

	return server
}

// NewServer preserves the existing embedding API while delegating to the
// transport-independent builder.
func NewServer(allowedDirs []string, logger *slog.Logger, cfg *config.Config) *mcp.Server {
	return BuildServer(ServerOptions{
		Version:            Version,
		AllowedDirectories: allowedDirs,
		Logger:             logger,
		Config:             cfg,
		EnableClientRoots:  true,
		LifecycleContext:   context.Background(),
	})
}
