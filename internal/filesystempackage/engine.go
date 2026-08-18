package filesystempackage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

const (
	StateCommitted          = "committed"
	StateUnchanged          = "unchanged"
	StatePartiallyCommitted = "partially_committed"
	StateUnknown            = "unknown"

	defaultR24DirectoryMode fs.FileMode = 0o755
	defaultR24FileMode      fs.FileMode = 0o644

	maxCleanupResidueItems        = 16
	maxCleanupResidueMessageBytes = 512
	maxFailureMessageBytes        = 1024
	maxMCPEnvelopeReserveBytes    = 512
)

// PreviewOutput is the read-only capability returned by filesystem_package.
type PreviewOutput struct {
	PreviewID string      `json:"previewId"`
	CreatedAt string      `json:"createdAt"`
	ExpiresAt string      `json:"expiresAt"`
	Plan      PlanSummary `json:"plan"`
}

// ApplyOperationResult is bounded post-state evidence in manifest order.
type ApplyOperationResult struct {
	Index       int    `json:"index"`
	Type        string `json:"type"`
	Path        string `json:"path,omitempty"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
	State       string `json:"state"`
}

// ApplyOutput reports whole-package success or deterministic partial-state evidence.
type ApplyOutput struct {
	Applied        bool                   `json:"applied"`
	PartialCommit  bool                   `json:"partialCommit"`
	OperationCount int                    `json:"operationCount"`
	BackupIDs      []string               `json:"backupIds,omitempty"`
	FailedIndex    *int                   `json:"failedIndex,omitempty"`
	FailureKind    string                 `json:"failureKind,omitempty"`
	FailureMessage string                 `json:"failureMessage,omitempty"`
	CleanupResidue []string               `json:"cleanupResidue,omitempty"`
	Results        []ApplyOperationResult `json:"results"`
}

// Engine owns R24 one-shot capabilities and apply orchestration.
type Engine struct {
	planner   *Planner
	limits    Limits
	capture   BackupCaptureFunc
	previews  *filesystemPackagePreviewStore
	commitOps commitOperations
}

func NewEngine(planner *Planner, limits Limits, capture BackupCaptureFunc) (*Engine, error) {
	if planner == nil {
		return nil, operation.New(operation.KindInvalidInput, "filesystem package engine requires a planner")
	}
	if limits.MaxPreviews <= 0 || limits.MaxPreviewBytes <= 0 || limits.PreviewTTLSeconds <= 0 || limits.MaxOutputBytes <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "filesystem package preview/output limits must be positive")
	}
	return &Engine{
		planner: planner, limits: limits, capture: capture,
		previews:  newFilesystemPackagePreviewStore(limits.MaxPreviews, limits.MaxPreviewBytes, time.Duration(limits.PreviewTTLSeconds)*time.Second),
		commitOps: defaultCommitOperations(),
	}, nil
}

// Preview prepares and stores a process-local one-shot package capability.
func (engine *Engine) Preview(ctx context.Context, manifest Manifest) (PreviewOutput, error) {
	if engine == nil || engine.planner == nil {
		return PreviewOutput{}, operation.New(operation.KindInvalidInput, "filesystem package engine is unavailable")
	}
	prepared, err := engine.planner.Plan(ctx, manifest)
	if err != nil {
		return PreviewOutput{}, err
	}
	if err := engine.checkPreviewOutput(prepared); err != nil {
		return PreviewOutput{}, err
	}
	if err := engine.checkWorstCaseApplyOutput(prepared); err != nil {
		return PreviewOutput{}, err
	}
	preview, err := engine.previews.put(prepared)
	if err != nil {
		return PreviewOutput{}, err
	}
	return PreviewOutput{
		PreviewID: preview.id,
		CreatedAt: preview.createdAt.Format(time.RFC3339Nano),
		ExpiresAt: preview.expiresAt.Format(time.RFC3339Nano),
		Plan:      preview.prepared.Summary(),
	}, nil
}

// Apply consumes one previewId, then revalidates, backs up, stages, commits, and
// verifies in the fixed R24 order. A token is never returned to the cache.
func (engine *Engine) Apply(ctx context.Context, previewID string) (ApplyOutput, error) {
	if engine == nil || engine.previews == nil {
		return ApplyOutput{}, operation.New(operation.KindConflict, "filesystem package engine is unavailable")
	}
	preview, err := engine.previews.claim(previewID)
	if err != nil {
		return ApplyOutput{}, err
	}
	prepared := preview.prepared
	output := newApplyOutput(prepared)
	if err := engine.checkWorstCaseApplyOutput(prepared); err != nil {
		return engine.failBeforeMutation(ctx, prepared, output, err)
	}
	if err := engine.planner.RevalidatePrepared(ctx, prepared); err != nil {
		return engine.failBeforeMutation(ctx, prepared, output, err)
	}
	treeOptions, err := engine.treeOptions()
	if err != nil {
		return engine.failBeforeMutation(ctx, prepared, output, err)
	}
	proof, err := CapturePreparedBackups(ctx, prepared, engine.capture, treeOptions)
	if err != nil {
		return engine.failBeforeMutation(ctx, prepared, output, err)
	}
	output.BackupIDs = proof.BackupIDs()
	if err := engine.planner.RevalidatePrepared(ctx, prepared); err != nil {
		return engine.failBeforeMutation(ctx, prepared, output, err)
	}

	staged, err := engine.stagePackage(ctx, prepared, treeOptions)
	if err != nil {
		cleanup := cleanupStagedPackage(staged)
		output.CleanupResidue = cleanup
		if len(cleanup) > 0 {
			return engine.failWithPartialState(ctx, prepared, output, nil, -1, errors.Join(err, operation.New(operation.KindFilesystem, "staging cleanup left bounded residue")))
		}
		return engine.failBeforeMutation(ctx, prepared, output, err)
	}
	defer func() {
		// Successful publishes consume their staging objects. This defer only
		// removes any still-unpublished exact staging objects on early returns.
		_ = cleanupStagedPackage(staged)
	}()
	if err := engine.planner.RevalidatePrepared(ctx, prepared); err != nil {
		cleanup := cleanupStagedPackage(staged)
		output.CleanupResidue = cleanup
		if len(cleanup) > 0 {
			return engine.failWithPartialState(ctx, prepared, output, nil, -1, errors.Join(err, operation.New(operation.KindFilesystem, "staging cleanup left bounded residue")))
		}
		return engine.failBeforeMutation(ctx, prepared, output, err)
	}

	createdDirectories := make(map[int]filesystem.ObjectIdentity)
	succeeded := make(map[int]bool, len(prepared.Operations))
	for _, item := range prepared.Operations {
		if err := ctx.Err(); err != nil {
			cleanup := cleanupStagedPackage(staged)
			output.CleanupResidue = cleanup
			if len(succeeded) > 0 || len(cleanup) > 0 {
				return engine.failWithPartialState(ctx, prepared, output, succeeded, item.Index, operation.Wrap(operation.KindCancelled, "apply_filesystem_package", preparedOperationPath(item), err))
			}
			return engine.failBeforeMutation(ctx, prepared, output, operation.Wrap(operation.KindCancelled, "apply_filesystem_package", preparedOperationPath(item), err))
		}
		err := engine.commitOperation(ctx, item, staged[item.Index], proof, treeOptions, createdDirectories)
		if err != nil {
			cleanup := cleanupStagedPackage(staged)
			output.CleanupResidue = cleanup
			return engine.failWithPartialState(ctx, prepared, output, succeeded, item.Index, err)
		}
		succeeded[item.Index] = true
		output.Results[item.Index].State = StateCommitted
	}
	output.Applied = true
	output.PartialCommit = false
	return output, nil
}

type stagedOperation struct {
	file      *filesystem.StagedFile
	directory *filesystem.StagedDirectory
}

func (engine *Engine) stagePackage(ctx context.Context, prepared PreparedPackage, treeOptions filesystem.ExactTreeOptions) (map[int]*stagedOperation, error) {
	staged := make(map[int]*stagedOperation)
	for _, item := range prepared.Operations {
		if err := ctx.Err(); err != nil {
			return staged, operation.Wrap(operation.KindCancelled, "stage_filesystem_package", preparedOperationPath(item), err)
		}
		stagingDir := destinationStagingDirectory(item)
		switch item.Operation.Type {
		case OperationCreateFile:
			file, err := filesystem.StageRawFile(ctx, stagingDir, bytes.NewReader(item.Operation.Content), defaultR24FileMode, nil, int64(len(item.Operation.Content)))
			if err != nil {
				return staged, err
			}
			staged[item.Index] = &stagedOperation{file: file}
		case OperationCopyFile:
			file, err := filesystem.StagePreparedRegularFile(ctx, item.Source.ResolvedPath, stagingDir, item.SourceSnapshot)
			if err != nil {
				return staged, err
			}
			staged[item.Index] = &stagedOperation{file: file}
		case OperationCopyDirectory:
			if item.Tree == nil {
				return staged, operation.New(operation.KindConflict, "copyDirectory prepared tree is missing during staging")
			}
			directory, err := filesystem.StageExactDirectoryCopy(ctx, *item.Tree, stagingDir, treeOptions)
			if err != nil {
				return staged, err
			}
			staged[item.Index] = &stagedOperation{directory: directory}
		}
	}
	return staged, nil
}

func (engine *Engine) commitOperation(ctx context.Context, item PreparedOperation, staged *stagedOperation, proof VerifiedBackupBatch, treeOptions filesystem.ExactTreeOptions, createdDirectories map[int]filesystem.ObjectIdentity) error {
	parentIdentity, err := destinationParentIdentity(item, createdDirectories)
	if err != nil {
		return err
	}
	switch item.Operation.Type {
	case OperationMkdir:
		if err := verifyPreparedIdentity(item.ImmediateParentPath, parentIdentity, "mkdir destination parent"); err != nil {
			return err
		}
		if err := engine.commitOps.createDirectory(item.Path.ResolvedPath, defaultR24DirectoryMode); err != nil {
			return err
		}
		identity, err := engine.commitOps.captureObjectIdentity(item.Path.ResolvedPath)
		if err != nil || !identity.IsDirectory() {
			if err == nil {
				err = operation.New(operation.KindConflict, "mkdir result is not a real directory")
			}
			return err
		}
		createdDirectories[item.Index] = identity
		return verifyPreparedIdentity(item.ImmediateParentPath, parentIdentity, "mkdir destination parent")

	case OperationCreateFile, OperationCopyFile:
		if staged == nil || staged.file == nil {
			return operation.New(operation.KindConflict, "prepared file staging object is missing")
		}
		if err := verifyPreparedIdentity(item.ImmediateParentPath, parentIdentity, "file destination parent"); err != nil {
			return err
		}
		destination := item.Path.ResolvedPath
		if item.Operation.Type == OperationCopyFile {
			destination = item.Destination.ResolvedPath
		}
		if err := engine.commitOps.publishFile(staged.file, destination); err != nil {
			return err
		}
		if err := verifyPreparedIdentity(item.ImmediateParentPath, parentIdentity, "file destination parent"); err != nil {
			return err
		}
		return engine.verifyRegularFileResult(ctx, destination, item.ExpectedResultFingerprint)

	case OperationCopyDirectory:
		if staged == nil || staged.directory == nil {
			return operation.New(operation.KindConflict, "prepared directory staging object is missing")
		}
		if err := verifyPreparedIdentity(item.ImmediateParentPath, parentIdentity, "directory destination parent"); err != nil {
			return err
		}
		if err := engine.commitOps.publishDirectory(staged.directory, item.Destination.ResolvedPath); err != nil {
			return err
		}
		if err := verifyPreparedIdentity(item.ImmediateParentPath, parentIdentity, "directory destination parent"); err != nil {
			return err
		}
		actual, err := filesystem.EnumerateExactTree(ctx, item.Destination.ResolvedPath, treeOptions)
		if err != nil {
			return err
		}
		if item.Tree == nil || !filesystem.ExactTreeContentEqual(*item.Tree, actual) {
			return operation.New(operation.KindConflict, "published directory does not match prepared source tree")
		}
		return nil

	case OperationMove:
		if err := verifyPreparedIdentity(item.ImmediateParentPath, parentIdentity, "move destination parent"); err != nil {
			return err
		}
		return engine.commitOps.movePrepared(item.Source.ResolvedPath, item.Destination.ResolvedPath, item.SourceIdentity, item.SourceParentIdentity, parentIdentity)

	case OperationDeleteFile:
		return DeletePreparedFile(ctx, item, proof)
	case OperationDeleteDirectory:
		return DeletePreparedDirectory(ctx, item, proof, treeOptions)
	default:
		return operation.New(operation.KindInvalidInput, "unknown prepared filesystem operation")
	}
}

func destinationParentIdentity(item PreparedOperation, created map[int]filesystem.ObjectIdentity) (filesystem.ObjectIdentity, error) {
	if item.Operation.Type == OperationDeleteFile || item.Operation.Type == OperationDeleteDirectory {
		return filesystem.ObjectIdentity{}, nil
	}
	if item.Operation.Type == OperationMove || item.Operation.Type == OperationMkdir || item.Operation.Type == OperationCreateFile || item.Operation.Type == OperationCopyFile || item.Operation.Type == OperationCopyDirectory {
		if item.ParentProviderIndex >= 0 {
			identity, ok := created[item.ParentProviderIndex]
			if !ok {
				return filesystem.ObjectIdentity{}, operation.New(operation.KindConflict, fmt.Sprintf("required earlier mkdir %d is not committed", item.ParentProviderIndex))
			}
			return identity, nil
		}
		return item.NearestAncestorIdentity, nil
	}
	return filesystem.ObjectIdentity{}, operation.New(operation.KindInvalidInput, "operation has no destination parent")
}

func destinationStagingDirectory(item PreparedOperation) string {
	if item.Path.NearestExistingPath != "" {
		return item.Path.NearestExistingPath
	}
	return item.Destination.NearestExistingPath
}

func cleanupStagedPackage(staged map[int]*stagedOperation) []string {
	var residue []string
	indices := make([]int, 0, len(staged))
	for index := range staged {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		item := staged[index]
		if item == nil {
			continue
		}
		if item.file != nil {
			if err := item.file.Cleanup(); err != nil {
				residue = appendBoundedCleanup(residue, err)
			}
		}
		if item.directory != nil {
			if err := item.directory.Cleanup(); err != nil {
				residue = appendBoundedCleanup(residue, err)
			}
		}
	}
	return residue
}

func appendBoundedCleanup(current []string, err error) []string {
	if len(current) >= maxCleanupResidueItems {
		return current
	}
	message := err.Error()
	if len(message) > maxCleanupResidueMessageBytes {
		message = message[:maxCleanupResidueMessageBytes]
	}
	return append(current, message)
}

func (engine *Engine) verifyRegularFileResult(ctx context.Context, path, expectedFingerprint string) error {
	snapshot, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, path, engine.limits.MaxFileBytes)
	if err != nil {
		return err
	}
	fingerprint, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
	if err != nil {
		return err
	}
	if fingerprint != expectedFingerprint {
		return operation.New(operation.KindConflict, "committed file fingerprint does not match prepared result")
	}
	return nil
}

func (engine *Engine) treeOptions() (filesystem.ExactTreeOptions, error) {
	allowed := engine.planner.allowedDirs()
	if len(allowed) == 0 {
		return filesystem.ExactTreeOptions{}, operation.New(operation.KindAccessDenied, "no resolved allowed directories are available")
	}
	return filesystem.ExactTreeOptions{
		ResolvedAllowedDirs: allowed, MaxEntries: engine.limits.MaxRecursiveEntries,
		MaxDepth: engine.limits.MaxRecursiveDepth, MaxFileBytes: engine.limits.MaxFileBytes,
		MaxAggregateBytes: engine.limits.MaxAggregateBytes,
	}, nil
}

func newApplyOutput(prepared PreparedPackage) ApplyOutput {
	output := ApplyOutput{
		OperationCount: len(prepared.Operations),
		Results:        make([]ApplyOperationResult, len(prepared.Operations)),
	}
	for _, item := range prepared.Operations {
		output.Results[item.Index] = ApplyOperationResult{
			Index: item.Index, Type: item.Operation.Type, Path: item.Operation.Path,
			Source: item.Operation.Source, Destination: item.Operation.Destination, State: StateUnchanged,
		}
	}
	return output
}

func (engine *Engine) failBeforeMutation(ctx context.Context, prepared PreparedPackage, output ApplyOutput, failure error) (ApplyOutput, error) {
	output.FailureKind = operation.KindOf(failure).String()
	output.FailureMessage = boundedFailureMessage(failure)
	output.Results = engine.classifyPackage(ctx, prepared, nil)
	return output, boundedPublicFailure(failure)
}

func (engine *Engine) failWithPartialState(ctx context.Context, prepared PreparedPackage, output ApplyOutput, succeeded map[int]bool, failedIndex int, failure error) (ApplyOutput, error) {
	if failedIndex >= 0 {
		index := failedIndex
		output.FailedIndex = &index
	}
	output.FailureKind = operation.KindOf(failure).String()
	output.FailureMessage = boundedFailureMessage(failure)
	output.Results = engine.classifyPackage(ctx, prepared, succeeded)
	partial := len(output.CleanupResidue) > 0
	for _, result := range output.Results {
		if result.State == StateCommitted || result.State == StatePartiallyCommitted || result.State == StateUnknown {
			partial = true
			break
		}
	}
	output.PartialCommit = partial
	if !partial {
		return output, boundedPublicFailure(failure)
	}
	return output, operation.New(operation.KindPartialCommit, output.FailureMessage)
}

func boundedFailureMessage(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > maxFailureMessageBytes {
		return message[:maxFailureMessageBytes]
	}
	return message
}

func boundedPublicFailure(err error) error {
	if err == nil {
		return nil
	}
	return operation.New(operation.KindOf(err), boundedFailureMessage(err))
}

func (engine *Engine) classifyPackage(ctx context.Context, prepared PreparedPackage, succeeded map[int]bool) []ApplyOperationResult {
	results := newApplyOutput(prepared).Results
	treeOptions, treeErr := engine.treeOptions()
	for _, item := range prepared.Operations {
		if succeeded != nil && succeeded[item.Index] {
			results[item.Index].State = StateCommitted
			continue
		}
		if err := ctx.Err(); err != nil {
			results[item.Index].State = StateUnknown
			continue
		}
		results[item.Index].State = engine.classifyOperation(ctx, item, treeOptions, treeErr)
	}
	return results
}

func (engine *Engine) classifyOperation(ctx context.Context, item PreparedOperation, treeOptions filesystem.ExactTreeOptions, treeErr error) string {
	switch item.Operation.Type {
	case OperationMkdir:
		info, err := os.Lstat(item.Path.ResolvedPath)
		if os.IsNotExist(err) {
			return StateUnchanged
		}
		if err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return StateCommitted
		}
		return StateUnknown
	case OperationCreateFile:
		return engine.classifyCreatedFile(ctx, item.Path.ResolvedPath, item.ExpectedResultFingerprint)
	case OperationCopyFile:
		return engine.classifyCreatedFile(ctx, item.Destination.ResolvedPath, item.ExpectedResultFingerprint)
	case OperationCopyDirectory:
		if _, err := os.Lstat(item.Destination.ResolvedPath); os.IsNotExist(err) {
			return StateUnchanged
		} else if err != nil || treeErr != nil || item.Tree == nil {
			return StateUnknown
		}
		actual, err := filesystem.EnumerateExactTree(ctx, item.Destination.ResolvedPath, treeOptions)
		if err != nil {
			return StateUnknown
		}
		if filesystem.ExactTreeContentEqual(*item.Tree, actual) {
			return StateCommitted
		}
		return StatePartiallyCommitted
	case OperationMove:
		sourceMatches, _ := item.SourceIdentity.Matches(item.Source.ResolvedPath)
		destinationIdentity, destinationErr := filesystem.CaptureObjectIdentity(item.Destination.ResolvedPath)
		destinationMatches := destinationErr == nil && item.SourceIdentity.Equal(destinationIdentity)
		if !sourceMatches && destinationMatches {
			return StateCommitted
		}
		if sourceMatches && os.IsNotExist(destinationErr) {
			return StateUnchanged
		}
		if sourceMatches || destinationMatches {
			return StatePartiallyCommitted
		}
		return StateUnknown
	case OperationDeleteFile:
		if _, err := os.Lstat(item.Path.ResolvedPath); os.IsNotExist(err) {
			return StateCommitted
		}
		matches, _ := item.TargetIdentity.Matches(item.Path.ResolvedPath)
		if matches && item.SourceSnapshot.Verify(item.Path.ResolvedPath) == nil {
			return StateUnchanged
		}
		return StateUnknown
	case OperationDeleteDirectory:
		if _, err := os.Lstat(item.Path.ResolvedPath); os.IsNotExist(err) {
			return StateCommitted
		}
		if treeErr != nil || item.Tree == nil {
			return StateUnknown
		}
		if filesystem.VerifyExactTree(ctx, *item.Tree, treeOptions) == nil {
			return StateUnchanged
		}
		for _, entry := range item.Tree.Entries {
			if _, err := os.Lstat(entry.Path); os.IsNotExist(err) {
				return StatePartiallyCommitted
			}
		}
		return StateUnknown
	default:
		return StateUnknown
	}
}

func (engine *Engine) classifyCreatedFile(ctx context.Context, path, expectedFingerprint string) string {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return StateUnchanged
	}
	snapshot, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, path, engine.limits.MaxFileBytes)
	if err != nil {
		return StateUnknown
	}
	fingerprint, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
	if err == nil && fingerprint == expectedFingerprint {
		return StateCommitted
	}
	return StateUnknown
}

func (engine *Engine) checkPreviewOutput(prepared PreparedPackage) error {
	worst := PreviewOutput{
		PreviewID: strings.Repeat("f", filesystemPackagePreviewTokenBytes*2),
		CreatedAt: strings.Repeat("9", len(time.RFC3339Nano)),
		ExpiresAt: strings.Repeat("9", len(time.RFC3339Nano)),
		Plan:      prepared.Summary(),
	}
	encoded, err := json.Marshal(worst)
	if err != nil {
		return operation.Wrap(operation.KindFilesystem, "encode_filesystem_package_preview", "", err)
	}
	if int64(len(encoded))+maxMCPEnvelopeReserveBytes > engine.limits.MaxOutputBytes {
		return operation.New(operation.KindLimit, fmt.Sprintf("filesystem package preview response exceeds output limit %d", engine.limits.MaxOutputBytes))
	}
	return nil
}

func (engine *Engine) checkWorstCaseApplyOutput(prepared PreparedPackage) error {
	worst := newApplyOutput(prepared)
	worst.Applied = true
	worst.PartialCommit = true
	if len(prepared.Operations) > 0 {
		index := len(prepared.Operations) - 1
		worst.FailedIndex = &index
	}
	worst.FailureKind = operation.KindPartialCommit.String()
	worst.FailureMessage = strings.Repeat("\x01", maxFailureMessageBytes)
	worst.CleanupResidue = make([]string, maxCleanupResidueItems)
	for index := range worst.CleanupResidue {
		worst.CleanupResidue[index] = strings.Repeat("\x01", maxCleanupResidueMessageBytes)
	}
	worst.BackupIDs = make([]string, len(prepared.BackupRequirements))
	for index := range worst.BackupIDs {
		worst.BackupIDs[index] = strings.Repeat("f", 64)
	}
	for index := range worst.Results {
		worst.Results[index].State = StatePartiallyCommitted
	}
	encoded, err := json.Marshal(worst)
	if err != nil {
		return operation.Wrap(operation.KindFilesystem, "encode_filesystem_package_apply", "", err)
	}
	// The MCP error text can repeat the bounded failure message. A control byte
	// is the maximum JSON expansion (six bytes), so reserve that plus envelope text.
	textReserve := int64(maxFailureMessageBytes*6 + maxMCPEnvelopeReserveBytes)
	if int64(len(encoded))+textReserve > engine.limits.MaxOutputBytes {
		return operation.New(operation.KindLimit, fmt.Sprintf("worst-case filesystem package apply response exceeds output limit %d", engine.limits.MaxOutputBytes))
	}
	return nil
}
