package handler

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/filesystempackage"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

const filesystemPackageMaxPathBytes = 32 * 1024

func (h *Handler) newFilesystemPackageEngine() (*filesystempackage.Engine, error) {
	if h == nil {
		return nil, operation.New(operation.KindInvalidInput, "filesystem package handler is unavailable")
	}
	limits := h.filesystemPackageLimits()
	planner, err := filesystempackage.NewPlanner(
		limits,
		h.filesystemPackagePathEvidence,
		h.filesystemPackageResolvedAllowedDirectories,
		func(ctx context.Context, requests []backupstore.CaptureRequest) error {
			if len(requests) == 0 {
				return nil
			}
			if h.backupCapturePreflight == nil {
				return operation.New(operation.KindConflict, "persistent backup store is required for destructive filesystem packages")
			}
			return h.backupCapturePreflight.PreflightCaptureBatch(ctx, requests)
		},
	)
	if err != nil {
		return nil, err
	}
	return filesystempackage.NewEngine(planner, limits, func(ctx context.Context, requests []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error) {
		if len(requests) == 0 {
			return nil, nil
		}
		if h.backupBatchCapture == nil {
			return nil, operation.New(operation.KindConflict, "persistent backup store is required for destructive filesystem packages")
		}
		return h.backupBatchCapture.CaptureBatch(ctx, requests)
	})
}

func (h *Handler) filesystemPackageLimits() filesystempackage.Limits {
	limits := config.Limits{
		MaxFileBytes:                       config.DefaultMaxFileBytes,
		MaxOutputBytes:                     config.DefaultMaxOutputBytes,
		MaxFingerprintEntryDetails:         config.DefaultMaxFingerprintEntryDetails,
		MaxFilesystemPackageOperations:     config.DefaultMaxFilesystemPackageOperations,
		MaxFilesystemPackageBytes:          config.DefaultMaxFilesystemPackageBytes,
		MaxFilesystemRecursiveEntries:      config.DefaultMaxFilesystemRecursiveEntries,
		MaxFilesystemRecursiveDepth:        config.DefaultMaxFilesystemRecursiveDepth,
		MaxFilesystemAggregateBytes:        config.DefaultMaxFilesystemAggregateBytes,
		MaxFilesystemStagingBytes:          config.DefaultMaxFilesystemStagingBytes,
		MaxFilesystemPackagePreviews:       config.DefaultMaxFilesystemPackagePreviews,
		MaxFilesystemPackagePreviewBytes:   config.DefaultMaxFilesystemPackagePreviewBytes,
		FilesystemPackagePreviewTTLSeconds: config.DefaultFilesystemPackagePreviewTTLSeconds,
	}
	if h != nil && h.config != nil {
		limits = h.config.Limits
	}
	return filesystempackage.Limits{
		MaxOperations:       positiveIntOrDefault(limits.MaxFilesystemPackageOperations, config.DefaultMaxFilesystemPackageOperations),
		MaxManifestBytes:    positiveInt64OrDefault(limits.MaxFilesystemPackageBytes, config.DefaultMaxFilesystemPackageBytes),
		MaxPathBytes:        filesystemPackageMaxPathBytes,
		MaxFileBytes:        positiveInt64OrDefault(limits.MaxFileBytes, config.DefaultMaxFileBytes),
		MaxRecursiveEntries: positiveIntOrDefault(limits.MaxFilesystemRecursiveEntries, config.DefaultMaxFilesystemRecursiveEntries),
		MaxRecursiveDepth:   positiveIntOrDefault(limits.MaxFilesystemRecursiveDepth, config.DefaultMaxFilesystemRecursiveDepth),
		MaxAggregateBytes:   positiveInt64OrDefault(limits.MaxFilesystemAggregateBytes, config.DefaultMaxFilesystemAggregateBytes),
		MaxStagingBytes:     positiveInt64OrDefault(limits.MaxFilesystemStagingBytes, config.DefaultMaxFilesystemStagingBytes),
		MaxEntryDetails:     positiveIntOrDefault(limits.MaxFingerprintEntryDetails, config.DefaultMaxFingerprintEntryDetails),
		MaxOutputBytes:      positiveInt64OrDefault(limits.MaxOutputBytes, config.DefaultMaxOutputBytes),
		MaxPreviews:         positiveIntOrDefault(limits.MaxFilesystemPackagePreviews, config.DefaultMaxFilesystemPackagePreviews),
		MaxPreviewBytes:     positiveInt64OrDefault(limits.MaxFilesystemPackagePreviewBytes, config.DefaultMaxFilesystemPackagePreviewBytes),
		PreviewTTLSeconds:   positiveIntOrDefault(limits.FilesystemPackagePreviewTTLSeconds, config.DefaultFilesystemPackagePreviewTTLSeconds),
	}
}

func positiveIntOrDefault(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveInt64OrDefault(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func (h *Handler) filesystemPackageResolvedAllowedDirectories() []string {
	if h == nil {
		return nil
	}
	return h.GetAllowedDirectories()
}

func (h *Handler) filesystemPackagePathEvidence(path string) (security.PathEvidence, error) {
	if h == nil {
		return security.PathEvidence{}, operation.New(operation.KindAccessDenied, "filesystem package handler is unavailable")
	}
	h.mu.RLock()
	requestedAllowed := append([]string(nil), h.allowedRequestedDirs...)
	resolvedAllowed := append([]string(nil), h.allowedDirs...)
	protectedRequested := append([]string(nil), h.protectedRequestedDirs...)
	protectedResolved := append([]string(nil), h.protectedDirs...)
	h.mu.RUnlock()

	evidence, err := security.ValidatePathEvidenceWithAllowedDirectories(path, requestedAllowed, resolvedAllowed)
	if err != nil {
		return security.PathEvidence{}, err
	}
	if filesystemPackagePathIsProtected(evidence.RequestedPath, protectedRequested) ||
		filesystemPackagePathIsProtected(evidence.ResolvedPath, protectedResolved) ||
		filesystemPackagePathIsProtected(evidence.NearestExistingPath, protectedResolved) {
		return security.PathEvidence{}, operation.New(operation.KindAccessDenied, fmt.Sprintf("access denied to protected internal path: %s", path))
	}
	return evidence, nil
}

func filesystemPackagePathIsProtected(path string, protected []string) bool {
	return path != "" && len(protected) > 0 && security.IsPathWithinAllowedDirectories(path, protected)
}

func (h *Handler) HandleFilesystemPackage(ctx context.Context, _ *mcp.CallToolRequest, input filesystempackage.Manifest) (*mcp.CallToolResult, filesystempackage.PreviewOutput, error) {
	if h == nil || h.filesystemPackageEngine == nil {
		err := h.filesystemPackageInitializationError()
		return errorResultFromError(err), filesystempackage.PreviewOutput{}, nil
	}
	output, err := h.filesystemPackageEngine.Preview(ctx, input)
	if err != nil {
		return errorResultFromError(err), filesystempackage.PreviewOutput{}, nil
	}
	text := fmt.Sprintf("Filesystem package prepared: %d operation(s), %d required backup(s).", output.Plan.OperationCount, output.Plan.BackupCount)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) HandleFilesystemPackageApply(ctx context.Context, _ *mcp.CallToolRequest, input PreviewApplyInput) (*mcp.CallToolResult, filesystempackage.ApplyOutput, error) {
	if h == nil || h.filesystemPackageEngine == nil {
		err := h.filesystemPackageInitializationError()
		return errorResultFromError(err), filesystempackage.ApplyOutput{}, nil
	}
	output, err := h.filesystemPackageEngine.Apply(ctx, input.PreviewID)
	if err != nil {
		return errorResultFromError(err), output, nil
	}
	text := fmt.Sprintf("Filesystem package applied: %d operation(s) committed.", output.OperationCount)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) filesystemPackageInitializationError() error {
	if h != nil && h.filesystemPackageInitErr != nil {
		return operation.Wrap(operation.KindFilesystem, "initialize_filesystem_package", "", h.filesystemPackageInitErr)
	}
	return operation.New(operation.KindFilesystem, "filesystem package engine is unavailable")
}
