package handler

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"golang.org/x/text/unicode/norm"
)

const (
	patchPackageActionInspect     = "inspect"
	patchPackageActionDryRun      = "dryRun"
	patchPackageAggregateModeV1   = "patch-package-aggregate-v1"
	maxPatchPackageLabelBytes     = 256
	maxPatchPackageEditsPerTarget = 1000
)

type validatedPatchPackageTarget struct {
	declared                  PatchPackageTarget
	resolvedPath              string
	canonicalManifestPath     string
	expectedFingerprint       string
	expectedResultFingerprint string
	info                      os.FileInfo
}

// HandlePatchPackage is retained only as a package-level compatibility bridge
// for pre-R23 regression coverage. It is not registered as an MCP tool.
// Deprecated: MCP callers use HandlePatchPackageRead and HandlePatchPackageApply.
func (h *Handler) HandlePatchPackage(ctx context.Context, _ *mcp.CallToolRequest, input PatchPackageInput) (*mcp.CallToolResult, PatchPackageOutput, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	action, err := validatePatchPackageActionInput(input)
	if err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	if action == patchPackageActionApply {
		return h.handlePatchPackageApply(ctx, input.PreviewID)
	}

	targets, err := h.validatePatchPackageManifest(ctx, input.Manifest)
	if err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	switch action {
	case patchPackageActionInspect:
		return h.handlePatchPackageInspect(input.Manifest, targets)
	case patchPackageActionVerify:
		return h.handlePatchPackageVerify(ctx, input.Manifest, targets)
	default:
		return h.handlePatchPackageDryRun(ctx, input.Manifest, targets)
	}
}

func (h *Handler) handlePatchPackageInspect(manifest PatchPackageManifest, targets []validatedPatchPackageTarget) (*mcp.CallToolResult, PatchPackageOutput, error) {
	output := patchPackageBaseOutput(patchPackageActionInspect, manifest, targets)
	text := fmt.Sprintf("Patch package inspected: %d existing regular-file targets are structurally valid.", len(targets))
	if err := h.checkPatchPackageResponseLimit(output, text); err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) handlePatchPackageDryRun(ctx context.Context, manifest PatchPackageManifest, targets []validatedPatchPackageTarget) (*mcp.CallToolResult, PatchPackageOutput, error) {
	effectiveBackupPolicy, err := h.effectivePersistentBackupPolicy(manifest.BackupPolicy)
	if err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	manifest.BackupPolicy = effectiveBackupPolicy
	identities, err := openPatchPackageIdentities(targets)
	if err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	defer closePatchPackageIdentities(identities)

	before, err := h.capturePatchPackageFingerprints(ctx, targets)
	if err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	for index := range targets {
		if before[index] != targets[index].expectedFingerprint {
			err := operation.New(operation.KindConflict, fmt.Sprintf("patch package target %d (%s) fingerprint does not match the declared precondition", index, targets[index].declared.Path))
			return errorResultFromError(err), PatchPackageOutput{}, nil
		}
	}

	preparedTargets := make([]preparedPatchPackageTarget, len(targets))
	resultFingerprints := make([]string, len(targets))
	var preparedBytes int64
	for index, target := range targets {
		if err := ctx.Err(); err != nil {
			cancelled := operation.Wrap(operation.KindCancelled, "patch_package_prepare", target.declared.Path, err)
			return errorResultFromError(cancelled), PatchPackageOutput{}, nil
		}
		prepared, failure := h.prepareEdit(ctx, EditFileInput{
			Action:        editActionDirect,
			Path:          target.declared.Path,
			Edits:         target.declared.Edits,
			Patch:         target.declared.Patch,
			DryRun:        true,
			Encoding:      target.declared.Encoding,
			ForceWritable: target.declared.ForceWritable,
		})
		if failure != nil {
			return prefixPatchPackageFailure(index, target.declared.Path, failure), PatchPackageOutput{}, nil
		}
		if prepared.targetFingerprint != before[index] || prepared.targetFingerprint != target.expectedFingerprint {
			err := operation.New(operation.KindConflict, fmt.Sprintf("patch package target %d (%s) changed while being prepared", index, target.declared.Path))
			return errorResultFromError(err), PatchPackageOutput{}, nil
		}
		if target.expectedResultFingerprint != "" && prepared.resultFingerprint != target.expectedResultFingerprint {
			err := operation.New(operation.KindConflict, fmt.Sprintf("patch package target %d (%s) prepared result does not match expectedResultFingerprint", index, target.declared.Path))
			return errorResultFromError(err), PatchPackageOutput{}, nil
		}
		retained, sizeErr := prepared.retainedBytes()
		if sizeErr != nil {
			return errorResultFromError(sizeErr), PatchPackageOutput{}, nil
		}
		if retained > math.MaxInt64-preparedBytes {
			err := operation.New(operation.KindLimit, "patch package prepared byte count exceeds supported range")
			return errorResultFromError(err), PatchPackageOutput{}, nil
		}
		preparedBytes += retained
		if preparedBytes > h.maxPatchPackagePreparedBytes() {
			err := operation.New(operation.KindLimit, fmt.Sprintf("patch package prepared state exceeds limit %d bytes", h.maxPatchPackagePreparedBytes()))
			return errorResultFromError(err), PatchPackageOutput{}, nil
		}
		preparedTargets[index] = preparedPatchPackageTarget{
			index:                     index,
			requestedPath:             target.declared.Path,
			resolvedPath:              target.resolvedPath,
			canonicalManifestPath:     target.canonicalManifestPath,
			expectedFingerprint:       target.expectedFingerprint,
			expectedResultFingerprint: target.expectedResultFingerprint,
			prepared:                  prepared,
		}
		resultFingerprints[index] = prepared.resultFingerprint
	}

	if err := h.verifyPatchPackageDryRunSnapshot(ctx, targets, identities, before); err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	if manifest.BackupPolicy == editBackupPolicyRequired {
		requests := patchPackageCaptureRequests(manifest.Label, preparedTargets)
		if len(requests) > 0 {
			if h.backupCapturePreflight == nil {
				return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store does not provide package backup preflight authority")), PatchPackageOutput{}, nil
			}
			if err := h.backupCapturePreflight.PreflightCaptureBatch(ctx, requests); err != nil {
				return errorResultFromError(err), PatchPackageOutput{}, nil
			}
		}
	}

	preparedPackage := preparedPatchPackage{
		formatVersion:              manifest.FormatVersion,
		label:                      manifest.Label,
		fingerprintAlgorithm:       manifest.FingerprintAlgorithm,
		fingerprintMode:            manifest.FingerprintMode,
		backupPolicy:               manifest.BackupPolicy,
		aggregateMode:              patchPackageAggregateModeV1,
		aggregateBeforeFingerprint: patchPackageAggregate(targets, before),
		aggregateAfterFingerprint:  patchPackageAggregate(targets, resultFingerprints),
		targets:                    preparedTargets,
	}
	retainedPackageBytes, err := preparedPackage.retainedBytes()
	if err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	if retainedPackageBytes > h.maxPatchPackagePreparedBytes() {
		err := operation.New(operation.KindLimit, fmt.Sprintf("patch package prepared state exceeds limit %d bytes", h.maxPatchPackagePreparedBytes()))
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	for index := range preparedPackage.targets {
		preparedPackage.targets[index].prepared.identityFile = identities[index]
		identities[index] = nil
	}
	preview, err := h.patchPackagePreviews.put(preparedPackage)
	if err != nil {
		preparedPackage.close()
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	output := patchPackageOutputFromPreview(preview, patchPackageActionDryRun)
	text := patchPackageDryRunText(output)
	if err := h.checkPatchPackageResponseLimit(output, text); err != nil {
		h.patchPackagePreviews.discard(preview.id)
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) validatePatchPackageManifest(ctx context.Context, manifest PatchPackageManifest) ([]validatedPatchPackageTarget, error) {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, operation.Wrap(operation.KindInvalidInput, "encode_patch_package_manifest", "", err)
	}
	if int64(len(encoded)) > h.maxPatchPackageBytes() {
		return nil, operation.New(operation.KindLimit, fmt.Sprintf("patch package manifest exceeds limit %d bytes", h.maxPatchPackageBytes()))
	}
	if manifest.FormatVersion != PatchPackageFormatV1 {
		return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("formatVersion must be %s", PatchPackageFormatV1))
	}
	if manifest.FingerprintAlgorithm != "sha256" {
		return nil, operation.New(operation.KindInvalidInput, "fingerprintAlgorithm must be sha256")
	}
	if manifest.FingerprintMode != "content-v1" {
		return nil, operation.New(operation.KindInvalidInput, "fingerprintMode must be content-v1")
	}
	if manifest.BackupPolicy != "" && manifest.BackupPolicy != editBackupPolicyRequired {
		return nil, operation.New(operation.KindInvalidInput, "backupPolicy must be exactly required when provided")
	}
	if len(manifest.Label) > maxPatchPackageLabelBytes || strings.ContainsRune(manifest.Label, '\x00') {
		return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("label must not contain NUL and must be at most %d bytes", maxPatchPackageLabelBytes))
	}
	if len(manifest.Targets) == 0 {
		return nil, operation.New(operation.KindInvalidInput, "patch package must contain at least one target")
	}
	if len(manifest.Targets) > h.maxBatchFiles() {
		return nil, operation.New(operation.KindLimit, fmt.Sprintf("patch package target count exceeds limit %d", h.maxBatchFiles()))
	}

	validated := make([]validatedPatchPackageTarget, 0, len(manifest.Targets))
	seenManifestPaths := make(map[string]int, len(manifest.Targets))
	seenResolvedPaths := make(map[string]int, len(manifest.Targets))
	for index, target := range manifest.Targets {
		if err := ctx.Err(); err != nil {
			return nil, operation.Wrap(operation.KindCancelled, "inspect_patch_package", target.Path, err)
		}
		if strings.TrimSpace(target.Path) == "" {
			return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("patch package target %d path is required", index))
		}
		expected, err := normalizePatchPackageFingerprint(target.ExpectedFingerprint)
		if err != nil {
			return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("patch package target %d expectedFingerprint: %v", index, err))
		}
		expectedResult := ""
		if target.ExpectedResultFingerprint != "" {
			expectedResult, err = normalizePatchPackageFingerprint(target.ExpectedResultFingerprint)
			if err != nil {
				return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("patch package target %d expectedResultFingerprint: %v", index, err))
			}
		}
		hasEdits := len(target.Edits) > 0
		hasPatch := strings.TrimSpace(target.Patch) != ""
		if hasEdits == hasPatch {
			return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("patch package target %d must contain exactly one of edits or patch", index))
		}
		if hasEdits {
			if len(target.Edits) > maxPatchPackageEditsPerTarget {
				return nil, operation.New(operation.KindLimit, fmt.Sprintf("patch package target %d contains more than %d edits", index, maxPatchPackageEditsPerTarget))
			}
			for editIndex, edit := range target.Edits {
				if edit.OldText == "" {
					return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("patch package target %d edit %d oldText cannot be empty", index, editIndex))
				}
				if edit.Similarity != nil && (math.IsNaN(*edit.Similarity) || math.IsInf(*edit.Similarity, 0) || *edit.Similarity < minFuzzySimilarity || *edit.Similarity > 1) {
					return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("patch package target %d edit %d similarity must be between %.2f and 1.0", index, editIndex, minFuzzySimilarity))
				}
			}
		} else if _, err := parseUnifiedPatch(target.Patch, target.Path, h.maxFileBytes()); err != nil {
			return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("patch package target %d patch: %v", index, err))
		}

		canonicalManifestPath := canonicalPatchPackagePath(target.Path)
		manifestKey := patchPackagePathKey(canonicalManifestPath)
		if previous, duplicate := seenManifestPaths[manifestKey]; duplicate {
			return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("patch package targets %d and %d normalize to the same manifest path", previous, index))
		}
		seenManifestPaths[manifestKey] = index

		pathValidation := h.ValidatePath(target.Path)
		if !pathValidation.Ok() {
			return nil, pathValidation.Err
		}
		info, err := os.Stat(pathValidation.Path)
		if err != nil {
			return nil, operation.WrapFilesystem("inspect_patch_package_target", target.Path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("patch package target %d is not an existing regular file: %s", index, target.Path))
		}
		resolvedKey := patchPackagePathKey(pathValidation.Path)
		if previous, duplicate := seenResolvedPaths[resolvedKey]; duplicate {
			return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("patch package targets %d and %d resolve to the same file", previous, index))
		}
		for previousIndex := range validated {
			if os.SameFile(validated[previousIndex].info, info) {
				return nil, operation.New(operation.KindInvalidInput, fmt.Sprintf("patch package targets %d and %d reference the same filesystem object", previousIndex, index))
			}
		}
		seenResolvedPaths[resolvedKey] = index
		target.ExpectedFingerprint = expected
		target.ExpectedResultFingerprint = expectedResult
		validated = append(validated, validatedPatchPackageTarget{
			declared:                  target,
			resolvedPath:              pathValidation.Path,
			canonicalManifestPath:     canonicalManifestPath,
			expectedFingerprint:       expected,
			expectedResultFingerprint: expectedResult,
			info:                      info,
		})
	}
	return validated, nil
}

func openPatchPackageIdentities(targets []validatedPatchPackageTarget) ([]*filesystem.FileIdentity, error) {
	identities := make([]*filesystem.FileIdentity, 0, len(targets))
	for index, target := range targets {
		identity, err := filesystem.OpenFileIdentity(target.declared.Path)
		if err != nil {
			closePatchPackageIdentities(identities)
			return nil, err
		}
		identities = append(identities, identity)
		matches, err := identity.Matches(target.resolvedPath)
		if err != nil {
			closePatchPackageIdentities(identities)
			return nil, err
		}
		if !matches {
			closePatchPackageIdentities(identities)
			return nil, operation.New(operation.KindConflict, fmt.Sprintf("patch package target %d (%s) identity changed before dryRun", index, target.declared.Path))
		}
	}
	return identities, nil
}

func (h *Handler) verifyPatchPackageDryRunSnapshot(ctx context.Context, targets []validatedPatchPackageTarget, identities []*filesystem.FileIdentity, before []string) error {
	if err := verifyPatchPackageIdentities(targets, identities); err != nil {
		return err
	}
	verified, err := h.capturePatchPackageFingerprints(ctx, targets)
	if err != nil {
		if operation.KindOf(err) == operation.KindCancelled {
			return err
		}
		return operation.Wrap(operation.KindConflict, "patch_package_verify", "", err)
	}
	if len(verified) != len(before) {
		return operation.New(operation.KindConflict, "patch package verification fingerprint set is incomplete")
	}
	for index := range targets {
		if verified[index] != before[index] {
			return operation.New(operation.KindConflict, fmt.Sprintf("patch package target %d (%s) changed during dryRun", index, targets[index].declared.Path))
		}
	}
	return nil
}

func verifyPatchPackageIdentities(targets []validatedPatchPackageTarget, identities []*filesystem.FileIdentity) error {
	if len(identities) != len(targets) {
		return operation.New(operation.KindConflict, "patch package identity set is incomplete")
	}
	for index, target := range targets {
		matchesDeclared, err := identities[index].Matches(target.declared.Path)
		if err != nil {
			return operation.Wrap(operation.KindConflict, "verify_patch_package_identity", target.declared.Path, err)
		}
		matchesResolved, err := identities[index].Matches(target.resolvedPath)
		if err != nil {
			return operation.Wrap(operation.KindConflict, "verify_patch_package_identity", target.resolvedPath, err)
		}
		if !matchesDeclared || !matchesResolved {
			return operation.New(operation.KindConflict, fmt.Sprintf("patch package target %d (%s) identity changed during dryRun", index, target.declared.Path))
		}
	}
	return nil
}

func closePatchPackageIdentities(identities []*filesystem.FileIdentity) {
	for _, identity := range identities {
		_ = identity.Close()
	}
}

func (h *Handler) capturePatchPackageFingerprints(ctx context.Context, targets []validatedPatchPackageTarget) ([]string, error) {
	first, err := h.capturePatchPackageFingerprintsOnce(ctx, targets)
	if err != nil {
		return nil, err
	}
	second, err := h.capturePatchPackageFingerprintsOnce(ctx, targets)
	if err != nil {
		switch operation.KindOf(err) {
		case operation.KindCancelled, operation.KindSymlinkEscape:
			return nil, err
		default:
			return nil, operation.Wrap(operation.KindConflict, "verify_patch_package_fingerprints", "", err)
		}
	}
	for index := range first {
		if first[index] != second[index] {
			return nil, operation.New(operation.KindConflict, fmt.Sprintf("patch package target %d changed during fingerprint capture", index))
		}
	}
	return first, nil
}

func (h *Handler) capturePatchPackageFingerprintsOnce(ctx context.Context, targets []validatedPatchPackageTarget) ([]string, error) {
	fingerprints := make([]string, len(targets))
	for index, target := range targets {
		if err := ctx.Err(); err != nil {
			return nil, operation.Wrap(operation.KindCancelled, "capture_patch_package_fingerprints", target.resolvedPath, err)
		}
		requestedPath := target.declared.Path
		if requestedPath == "" {
			requestedPath = target.resolvedPath
		}
		validation := h.ValidatePath(requestedPath)
		if !validation.Ok() {
			return nil, validation.Err
		}
		if target.resolvedPath != "" && validation.Path != target.resolvedPath {
			return nil, operation.New(operation.KindConflict, fmt.Sprintf("patch package target %d path changed during fingerprint capture", index))
		}
		fingerprint, err := filesystem.FingerprintRegularFilePathBounded(ctx, validation.Path, h.maxFileBytes())
		if err != nil {
			return nil, err
		}
		fingerprints[index] = fingerprint
	}
	return fingerprints, nil
}

func patchPackageCaptureRequests(label string, targets []preparedPatchPackageTarget) []backupstore.CaptureRequest {
	requests := make([]backupstore.CaptureRequest, 0, len(targets))
	for index := range targets {
		if !targets[index].prepared.changed {
			continue
		}
		requests = append(requests, backupstore.CaptureRequest{
			TargetPath:      targets[index].resolvedPath,
			SourceOperation: backupstore.SourceOperationPatchPackage,
			Label:           label,
		})
	}
	return requests
}

func patchPackageBaseOutput(action string, manifest PatchPackageManifest, targets []validatedPatchPackageTarget) PatchPackageOutput {
	output := PatchPackageOutput{
		Action:               action,
		FormatVersion:        manifest.FormatVersion,
		Label:                manifest.Label,
		FingerprintAlgorithm: manifest.FingerprintAlgorithm,
		FingerprintMode:      manifest.FingerprintMode,
		BackupPolicy:         manifest.BackupPolicy,
		TargetCount:          len(targets),
		Results:              make([]PatchPackageTargetResult, len(targets)),
	}
	for index, target := range targets {
		output.Results[index] = PatchPackageTargetResult{
			Index:                     index,
			Path:                      target.declared.Path,
			ExpectedFingerprint:       target.expectedFingerprint,
			ExpectedResultFingerprint: target.expectedResultFingerprint,
		}
	}
	return output
}

func patchPackageDryRunText(output PatchPackageOutput) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Patch package dry run prepared %d targets (%d changed, %d unchanged).\nPreview ID: %s\nExpires: %s\nAggregate before: %s\nAggregate after: %s",
		output.TargetCount, output.ChangedCount, output.UnchangedCount, output.PreviewID, output.ExpiresAt, output.AggregateBeforeFingerprint, output.AggregateAfterFingerprint)
	if output.BackupPolicy != "" {
		fmt.Fprintf(&builder, "\nBackup policy: %s", output.BackupPolicy)
	}
	for _, result := range output.Results {
		fmt.Fprintf(&builder, "\n\n[%d] %s", result.Index, result.Path)
		if result.Diff != "" {
			builder.WriteByte('\n')
			builder.WriteString(result.Diff)
		}
	}
	return builder.String()
}

func (h *Handler) checkPatchPackageResponseLimit(output PatchPackageOutput, text string) error {
	encoded, err := json.Marshal(output)
	if err != nil {
		return operation.Wrap(operation.KindFilesystem, "encode_patch_package_output", "", err)
	}
	if int64(len(encoded))+int64(len(text)) > h.maxOutputBytes() {
		return operation.New(operation.KindLimit, fmt.Sprintf("patch package output exceeds limit %d bytes", h.maxOutputBytes()))
	}
	return nil
}

func prefixPatchPackageFailure(index int, path string, failure *mcp.CallToolResult) *mcp.CallToolResult {
	code := ErrCodeOperationFailed
	if failure != nil {
		if value, ok := failure.Meta[ErrorCodeMetaKey].(string); ok && value != "" {
			code = value
		}
	}
	message := "target preparation failed"
	if failure != nil && len(failure.Content) > 0 {
		if text, ok := failure.Content[0].(*mcp.TextContent); ok && text.Text != "" {
			message = text.Text
		}
	}
	return errorResultWithCode(code, fmt.Sprintf("patch package target %d (%s): %s", index, path, message))
}

func normalizePatchPackageFingerprint(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	decoded, err := hex.DecodeString(trimmed)
	if err != nil || len(decoded) != sha256.Size {
		return "", fmt.Errorf("must be 64 hexadecimal characters")
	}
	return strings.ToLower(trimmed), nil
}

func canonicalPatchPackagePath(path string) string {
	canonical := filepath.ToSlash(filepath.Clean(path))
	return norm.NFC.String(canonical)
}

func patchPackagePathKey(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}

func patchPackageAggregate(targets []validatedPatchPackageTarget, fingerprints []string) string {
	canonical := make([]string, len(targets))
	for index := range targets {
		canonical[index] = targets[index].canonicalManifestPath
	}
	return patchPackageAggregateCanonical(canonical, fingerprints)
}

func patchPackageAggregateCanonical(canonicalPaths, fingerprints []string) string {
	aggregate := sha256.New()
	_, _ = aggregate.Write([]byte("mcp-file-tools:patch-package:aggregate-v1\x00"))
	writePatchPackageUint64(aggregate, uint64(len(canonicalPaths)))
	for index, canonicalPath := range canonicalPaths {
		writePatchPackageUint64(aggregate, uint64(index))
		writePatchPackageString(aggregate, canonicalPath)
		decoded, _ := hex.DecodeString(fingerprints[index])
		_, _ = aggregate.Write(decoded)
	}
	return hex.EncodeToString(aggregate.Sum(nil))
}

func writePatchPackageString(target hash.Hash, value string) {
	writePatchPackageUint64(target, uint64(len(value)))
	_, _ = target.Write([]byte(value))
}

func writePatchPackageUint64(target hash.Hash, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = target.Write(encoded[:])
}
