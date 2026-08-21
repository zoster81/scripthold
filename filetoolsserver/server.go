package filetoolsserver

import (
	"context"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/deferredoperation"
	"github.com/zoster81/scripthold/internal/responsecontinuation"
	"github.com/zoster81/scripthold/internal/toolcatalog"
)

// Version is set at build time via ldflags
var Version = "dev"

// Server instructions for AI assistants
const serverInstructions = `Scripthold provides secure, encoding-aware filesystem tools and durable asynchronous shell/script tasks.

Use these tools when encoding, BOM, line endings, bounded traversal, source intelligence, atomic mutation, backups, or persistent task execution matter. Filesystem access is limited to startup roots. Encoding detection uses BOM/content evidence, never filenames; ambiguous input requires an explicit encoding. Use source_symbols for bounded read-only outline, digest, find, and fingerprint-bound show. R27 source_query provides bounded read-only structural search, supported project relations, and fingerprint-verified task context; textual/lexical search remains available through grep_text_files, while Phase 15 binds queries to bounded process-local index generations. Mutations revalidate paths and preserve encoding/BOM/line endings where documented.

For long work use task_run, then task_get/task_logs/task_list; tasks survive MCP reconnects and support cancellation. Oversized MCP results are retained and returned as a compact handle; retrieve their bounded segments with deferred_operation. Use preview/apply workflows for sensitive edits, patch packages, restores, and GC. Tool errors expose stable error codes.

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
type toolRegistrationPolicy struct {
	callDeadline   time.Duration
	maxInlineBytes int64
	responseStore  *responsecontinuation.Store
}

func addTool[In, Out any](server *mcp.Server, policy toolRegistrationPolicy, tool *mcp.Tool, typedHandler mcp.ToolHandlerFor[In, Out]) {
	tool.OutputSchema = map[string]any{"type": "object"}
	bounded := handler.WithCallDeadline(policy.callDeadline, typedHandler)
	structured := handler.WithStructuredErrorOutput(bounded)
	mcp.AddTool(server, tool, withSafeToolResponse(policy.maxInlineBytes, policy.responseStore, structured))
}

// ServerOptions contains process-wide MCP server policy. Every connection to
// the returned server shares the same configured directories and tool policy.
type ServerOptions struct {
	Version                string
	AllowedDirectories     []string
	ProtectedDirectories   []string
	BackupStore            handler.BackupStoreReader
	TaskStore              handler.TaskStore
	DeferredEngine         *deferredoperation.Engine
	Logger                 *slog.Logger
	ToolLogger             *slog.Logger
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
	chunkBytes := cfg.Reliability.ResponseChunkBytes
	if safeMaximum := int(cfg.Reliability.MaxInlineResponseBytes / 4); safeMaximum > 0 && chunkBytes > safeMaximum {
		chunkBytes = safeMaximum
	}
	retainedBytes := int(cfg.Limits.MaxOutputBytes * 2)
	if retainedBytes < 64*1024*1024 {
		retainedBytes = 64 * 1024 * 1024
	}
	responseStore := responsecontinuation.NewStore(responsecontinuation.Limits{
		MaxEntries:    64,
		MaxTotalBytes: retainedBytes,
		MaxChunkBytes: chunkBytes,
		Retention:     time.Hour,
	}, time.Now)

	handlerOptions := []handler.Option{
		handler.WithConfig(cfg),
		handler.WithProtectedDirectories(protectedDirectories),
		handler.WithBackupStore(options.BackupStore),
		handler.WithTaskStore(options.TaskStore),
		handler.WithResponseContinuationStore(responseStore),
	}
	if options.DeferredEngine != nil {
		handlerOptions = append(handlerOptions, handler.WithDeferredOperationStore(options.DeferredEngine.Store()))
	}
	if options.ExecutionPolicy != nil {
		handlerOptions = append(handlerOptions, handler.WithExecutionPolicy(*options.ExecutionPolicy))
	}
	h := handler.NewHandler(options.AllowedDirectories, handlerOptions...)
	sdkLogger := options.Logger
	logger := options.ToolLogger
	if logger == nil {
		logger = options.Logger
	}
	if options.DeferredEngine != nil {
		recoveryLogger := logger
		if recoveryLogger == nil {
			recoveryLogger = slog.Default()
		}
		go options.DeferredEngine.RunRecoveryLoop(lifecycleCtx, h.ResolvedAllowedDirs, func(error) {
			// Recovery errors can contain private-store paths through OS wrappers;
			// keep lifecycle diagnostics deliberately path-free.
			recoveryLogger.Error("deferred_operation_recovery_failed")
		})
	}
	impl := &mcp.Implementation{
		Name:    "scripthold",
		Version: version,
	}

	serverOpts := &mcp.ServerOptions{
		Instructions:            serverInstructions,
		Logger:                  sdkLogger,
		InitializedHandler:      createInitializedHandler(lifecycleCtx, h, version, options.EnableClientRoots),
		RootsListChangedHandler: createRootsListChangedHandler(h, options.EnableClientRoots),
	}
	server := mcp.NewServer(impl, serverOpts)
	registerProjectPrompts(server)

	server.AddReceivingMiddleware(createDiscoveryMiddleware(h, options.EnableClientRoots, options.DisableModernDiscovery))
	// Repair array/object args some MCP clients send as JSON-encoded strings.
	server.AddReceivingMiddleware(handler.RepairStringifiedArrayArgs)

	toolPolicy := toolRegistrationPolicy{
		callDeadline:   time.Duration(cfg.Reliability.MaxSynchronousSeconds) * time.Second,
		maxInlineBytes: cfg.Reliability.MaxInlineResponseBytes,
		responseStore:  responseStore,
	}

	// Register all tools using the new AddTool API with annotations
	// All handlers are wrapped with recovery middleware (and logging if logger is provided)

	// Read-only tools
	addTool(server, toolPolicy, catalogTool("read_text_file"), handler.Wrap(logger, "read_text_file", h.HandleReadTextFile))

	addTool(server, toolPolicy, catalogTool("read_multiple_files"), handler.Wrap(logger, "read_multiple_files", h.HandleReadMultipleFiles))

	addTool(server, toolPolicy, catalogTool("list_directory"), handler.Wrap(logger, "list_directory", h.HandleListDirectory))

	addTool(server, toolPolicy, emptyInputCatalogTool("list_encodings"), handler.Wrap(logger, "list_encodings", h.HandleListEncodings))

	addTool(server, toolPolicy, catalogTool("detect_encoding"), handler.Wrap(logger, "detect_encoding", h.HandleDetectEncoding))

	grepDirect := handler.Wrap(logger, "grep_text_files", h.HandleGrep)
	addTool(server, toolPolicy, catalogTool("grep_text_files"), deferredReadOnlyHandler("grep_text_files", h, cfg, options.DeferredEngine, false, prepareDeferredGrep, grepDirect))

	addTool(server, toolPolicy, emptyInputCatalogTool("list_allowed_directories"), handler.Wrap(logger, "list_allowed_directories", h.HandleListAllowedDirectories))

	addTool(server, toolPolicy, catalogTool("get_file_info"), handler.Wrap(logger, "get_file_info", h.HandleGetFileInfo))

	treeDirect := handler.Wrap(logger, "tree", h.HandleTree)
	addTool(server, toolPolicy, catalogTool("tree"), deferredReadOnlyHandler("tree", h, cfg, options.DeferredEngine, false, prepareDeferredTree, treeDirect))

	searchFilesDirect := handler.Wrap(logger, "search_files", h.HandleSearchFiles)
	addTool(server, toolPolicy, catalogTool("search_files"), deferredReadOnlyHandler("search_files", h, cfg, options.DeferredEngine, false, prepareDeferredSearchFiles, searchFilesDirect))

	sourceSymbolsDirect := handler.Wrap(logger, "source_symbols", h.SourceSymbols)
	addTool(server, toolPolicy, sourceSymbolsCatalogTool(), deferredReadOnlyHandler("source_symbols", h, cfg, options.DeferredEngine, true, prepareDeferredSourceSymbols, sourceSymbolsDirect))
	// source_query remains synchronous because its returned index binding is
	// process-local and must remain reusable by follow-up frontend requests.
	addTool(server, toolPolicy, sourceQueryCatalogTool(), handler.Wrap(logger, "source_query", h.SourceQuery))

	fingerprintDirect := handler.Wrap(logger, "fingerprint_paths", h.HandleFingerprintPaths)
	addTool(server, toolPolicy, catalogTool("fingerprint_paths"), deferredFingerprintHandler(h, cfg, options.DeferredEngine, fingerprintDirect))

	addTool(server, toolPolicy, catalogTool("verify_state"), handler.Wrap(logger, "verify_state", h.HandleVerifyState))

	addTool(server, toolPolicy, catalogTool("backup_store"), handler.Wrap(logger, "backup_store", h.HandleBackupStoreRead))

	addTool(server, toolPolicy, filesystemPackageCatalogTool(), handler.Wrap(logger, "filesystem_package", h.HandleFilesystemPackage))

	addTool(server, toolPolicy, catalogTool("detect_line_endings"), handler.Wrap(logger, "detect_line_endings", h.HandleDetectLineEndings))

	// Write tools
	addTool(server, toolPolicy, catalogTool("manage_bom"), handler.Wrap(logger, "manage_bom", h.HandleManageBOMRead))

	addTool(server, toolPolicy, catalogTool("change_line_endings"), handler.Wrap(logger, "change_line_endings", h.HandleChangeLineEndings))

	addTool(server, toolPolicy, catalogTool("write_whole_file"), handler.Wrap(logger, "write_whole_file", h.HandleWriteWholeFile))

	addTool(server, toolPolicy, catalogTool("edit_file"), handler.Wrap(logger, "edit_file", h.HandleEditFilePreview))

	addTool(server, toolPolicy, catalogTool("patch_package"), handler.Wrap(logger, "patch_package", h.HandlePatchPackageRead))

	addTool(server, toolPolicy, catalogTool("convert_encoding"), handler.Wrap(logger, "convert_encoding", h.HandleConvertEncodingPreview))

	// Approval-bound apply tools accept only previewId.
	addTool(server, toolPolicy, catalogTool("edit_file_apply"), handler.Wrap(logger, "edit_file_apply", h.HandleEditFileApply))
	addTool(server, toolPolicy, catalogTool("patch_package_apply"), handler.Wrap(logger, "patch_package_apply", h.HandlePatchPackageApply))
	addTool(server, toolPolicy, catalogTool("backup_restore_apply"), handler.Wrap(logger, "backup_restore_apply", h.HandleBackupRestoreApply))
	addTool(server, toolPolicy, catalogTool("backup_gc_apply"), handler.Wrap(logger, "backup_gc_apply", h.HandleBackupGCApply))
	addTool(server, toolPolicy, catalogTool("manage_bom_apply"), handler.Wrap(logger, "manage_bom_apply", h.HandleManageBOMApply))
	addTool(server, toolPolicy, catalogTool("convert_encoding_apply"), handler.Wrap(logger, "convert_encoding_apply", h.HandleConvertEncodingApply))
	addTool(server, toolPolicy, catalogTool("filesystem_package_apply"), handler.Wrap(logger, "filesystem_package_apply", h.HandleFilesystemPackageApply))

	addTool(server, toolPolicy, catalogTool("deferred_operation"), handler.Wrap(logger, "deferred_operation", h.HandleDeferredOperation))

	// Durable asynchronous execution. The MCP call only admits, observes, or
	// cancels work; a separate worker/helper topology owns process lifetime.
	addTool(server, toolPolicy, catalogTool("task_run"), handler.Wrap(logger, "task_run", h.HandleTaskRun))
	addTool(server, toolPolicy, catalogTool("task_list"), handler.Wrap(logger, "task_list", h.HandleTaskList))
	addTool(server, toolPolicy, catalogTool("task_get"), handler.Wrap(logger, "task_get", h.HandleTaskGet))
	addTool(server, toolPolicy, catalogTool("task_logs"), handler.Wrap(logger, "task_logs", h.HandleTaskLogs))
	addTool(server, toolPolicy, catalogTool("task_cancel"), handler.Wrap(logger, "task_cancel", h.HandleTaskCancel))
	addTool(server, toolPolicy, catalogTool("check_for_updates"), handler.Wrap(logger, "check_for_updates", handler.NewCheckUpdateHandler(version)))

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
