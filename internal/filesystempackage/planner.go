package filesystempackage

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
	"golang.org/x/text/unicode/norm"
)

// AuthorizeFunc returns current path evidence after the caller's complete
// allowed/protected-root policy has been applied.
type AuthorizeFunc func(string) (security.PathEvidence, error)

// AllowedDirsFunc returns the current resolved allowed-directory snapshot.
type AllowedDirsFunc func() []string

// BackupPreflightFunc is the read-only persistent-backup admission boundary.
type BackupPreflightFunc func(context.Context, []backupstore.CaptureRequest) error

// Planner is the read-only R24 manifest planner.
type Planner struct {
	limits          Limits
	authorize       AuthorizeFunc
	allowedDirs     AllowedDirsFunc
	backupPreflight BackupPreflightFunc
}

func NewPlanner(limits Limits, authorize AuthorizeFunc, allowedDirs AllowedDirsFunc, backupPreflight ...BackupPreflightFunc) (*Planner, error) {
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if authorize == nil || allowedDirs == nil {
		return nil, operation.New(operation.KindInvalidInput, "filesystem package planner requires authorization callbacks")
	}
	var preflight BackupPreflightFunc
	if len(backupPreflight) > 1 {
		return nil, operation.New(operation.KindInvalidInput, "filesystem package planner accepts at most one backup preflight callback")
	}
	if len(backupPreflight) == 1 {
		preflight = backupPreflight[0]
	}
	return &Planner{limits: limits, authorize: authorize, allowedDirs: allowedDirs, backupPreflight: preflight}, nil
}

// BackupRequirement binds one regular file that must be durably captured before
// an irreversible delete may proceed.
type BackupRequirement struct {
	Path                string
	ExpectedFingerprint string
	Bytes               int64
	OperationIndex      int
}

// PreparedOperation contains immutable read-only evidence retained by a preview.
type PreparedOperation struct {
	Index       int
	Operation   Operation
	Path        security.PathEvidence
	Source      security.PathEvidence
	Destination security.PathEvidence

	TargetIdentity filesystem.ObjectIdentity
	SourceIdentity filesystem.ObjectIdentity
	SourceSnapshot filesystem.FileSnapshot
	Tree           *filesystem.ExactTree

	ImmediateParentPath     string
	ParentProviderIndex     int
	NearestAncestorIdentity filesystem.ObjectIdentity
	SourceParentIdentity    filesystem.ObjectIdentity

	ExpectedResultFingerprint string
	Bytes                     int64
	FileCount                 int
	DirectoryCount            int
	BackupCount               int
}

// PreparedPackage is a deterministic, fully read-only R24 plan.
type PreparedPackage struct {
	FormatVersion      string
	Operations         []PreparedOperation
	BackupRequirements []BackupRequirement
	TotalSourceBytes   int64
	TotalStagingBytes  int64
}

// OperationSummary is stable preview output independent from capability IDs.
type OperationSummary struct {
	Index                     int    `json:"index"`
	Type                      string `json:"type"`
	Path                      string `json:"path,omitempty"`
	Source                    string `json:"source,omitempty"`
	Destination               string `json:"destination,omitempty"`
	FileCount                 int    `json:"fileCount,omitempty"`
	DirectoryCount            int    `json:"directoryCount,omitempty"`
	Bytes                     int64  `json:"bytes,omitempty"`
	BackupCount               int    `json:"backupCount,omitempty"`
	ExpectedResultFingerprint string `json:"expectedResultFingerprint,omitempty"`
}

// PlanSummary is the deterministic effect description returned by preview.
type PlanSummary struct {
	FormatVersion     string             `json:"formatVersion"`
	OperationCount    int                `json:"operationCount"`
	BackupCount       int                `json:"backupCount"`
	TotalSourceBytes  int64              `json:"totalSourceBytes"`
	TotalStagingBytes int64              `json:"totalStagingBytes"`
	Operations        []OperationSummary `json:"operations"`
}

func (prepared PreparedPackage) Summary() PlanSummary {
	summary := PlanSummary{
		FormatVersion: prepared.FormatVersion, OperationCount: len(prepared.Operations),
		BackupCount: len(prepared.BackupRequirements), TotalSourceBytes: prepared.TotalSourceBytes,
		TotalStagingBytes: prepared.TotalStagingBytes, Operations: make([]OperationSummary, 0, len(prepared.Operations)),
	}
	for _, item := range prepared.Operations {
		summary.Operations = append(summary.Operations, OperationSummary{
			Index: item.Index, Type: item.Operation.Type, Path: item.Operation.Path,
			Source: item.Operation.Source, Destination: item.Operation.Destination,
			FileCount: item.FileCount, DirectoryCount: item.DirectoryCount, Bytes: item.Bytes,
			BackupCount: item.BackupCount, ExpectedResultFingerprint: item.ExpectedResultFingerprint,
		})
	}
	return summary
}

type plannerOperand struct {
	index    int
	role     string
	path     string
	key      string
	typeName string
}

// Plan validates and prepares a complete package without mutating disk.
func (planner *Planner) Plan(ctx context.Context, manifest Manifest) (PreparedPackage, error) {
	if planner == nil {
		return PreparedPackage{}, operation.New(operation.KindInvalidInput, "filesystem package planner is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return PreparedPackage{}, operation.Wrap(operation.KindCancelled, "plan_filesystem_package", "", err)
	}
	if err := ValidateManifest(manifest, planner.limits); err != nil {
		return PreparedPackage{}, err
	}
	allowed := planner.allowedDirs()
	if len(allowed) == 0 {
		return PreparedPackage{}, operation.New(operation.KindAccessDenied, "no resolved allowed directories are available")
	}
	treeOptions := filesystem.ExactTreeOptions{
		ResolvedAllowedDirs: allowed,
		MaxEntries:          planner.limits.MaxRecursiveEntries,
		MaxDepth:            planner.limits.MaxRecursiveDepth,
		MaxFileBytes:        planner.limits.MaxFileBytes,
		MaxAggregateBytes:   planner.limits.MaxAggregateBytes,
	}

	prepared := PreparedPackage{FormatVersion: FormatV1, Operations: make([]PreparedOperation, 0, len(manifest.Operations))}
	operands := make([]plannerOperand, 0, len(manifest.Operations)*2)
	identityOwners := make(map[string]int)
	mkdirByPath := make(map[string]int)

	registerIdentity := func(identity filesystem.ObjectIdentity, operationIndex int) error {
		key := identity.StableKey()
		if key == "" {
			return operation.New(operation.KindUnsupported, "stable object identity is required for filesystem package planning")
		}
		if owner, ok := identityOwners[key]; ok && owner != operationIndex {
			return operation.New(operation.KindInvalidInput, fmt.Sprintf("operations %d and %d alias the same filesystem object", owner, operationIndex))
		}
		identityOwners[key] = operationIndex
		return nil
	}
	registerTreeIdentities := func(tree filesystem.ExactTree, operationIndex int) error {
		for _, entry := range tree.Entries {
			if err := registerIdentity(entry.Identity, operationIndex); err != nil {
				return err
			}
		}
		return nil
	}
	addSourceBytes := func(value int64) error {
		if value < 0 || value > math.MaxInt64-prepared.TotalSourceBytes || prepared.TotalSourceBytes+value > planner.limits.MaxAggregateBytes {
			return operation.New(operation.KindLimit, fmt.Sprintf("filesystem package aggregate source bytes exceed limit %d", planner.limits.MaxAggregateBytes))
		}
		prepared.TotalSourceBytes += value
		return nil
	}
	addStagingBytes := func(value int64) error {
		if value < 0 || value > math.MaxInt64-prepared.TotalStagingBytes || prepared.TotalStagingBytes+value > planner.limits.MaxStagingBytes {
			return operation.New(operation.KindLimit, fmt.Sprintf("filesystem package staging bytes exceed limit %d", planner.limits.MaxStagingBytes))
		}
		prepared.TotalStagingBytes += value
		return nil
	}
	registerOperand := func(index int, role, path, typeName string) {
		operands = append(operands, plannerOperand{index: index, role: role, path: path, key: plannerPathKey(path), typeName: typeName})
	}

	for index, declared := range manifest.Operations {
		if err := ctx.Err(); err != nil {
			return PreparedPackage{}, operation.Wrap(operation.KindCancelled, "plan_filesystem_package", "", err)
		}
		item := PreparedOperation{Index: index, Operation: declared, ParentProviderIndex: -1}
		switch declared.Type {
		case OperationMkdir:
			target, parentPath, provider, nearestIdentity, err := planner.prepareMissingTarget(declared.Path, mkdirByPath)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			item.Path, item.ImmediateParentPath, item.ParentProviderIndex, item.NearestAncestorIdentity = target, parentPath, provider, nearestIdentity
			registerOperand(index, "target", target.ResolvedPath, declared.Type)
			mkdirByPath[plannerPathKey(target.ResolvedPath)] = index
			item.DirectoryCount = 1

		case OperationCreateFile:
			target, parentPath, provider, nearestIdentity, err := planner.prepareMissingTarget(declared.Path, mkdirByPath)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			item.Path, item.ImmediateParentPath, item.ParentProviderIndex, item.NearestAncestorIdentity = target, parentPath, provider, nearestIdentity
			item.Bytes, item.FileCount = int64(len(declared.Content)), 1
			item.ExpectedResultFingerprint = filesystem.FingerprintRegularFileData(declared.Content)
			if err := addStagingBytes(item.Bytes); err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			registerOperand(index, "target", target.ResolvedPath, declared.Type)

		case OperationCopyFile:
			source, sourceIdentity, sourceSnapshot, sourceParent, err := planner.prepareRegularSource(ctx, declared.Source)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			target, parentPath, provider, nearestIdentity, err := planner.prepareMissingTarget(declared.Destination, mkdirByPath)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			if err := registerIdentity(sourceIdentity, index); err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			fingerprint, err := filesystem.FingerprintRegularFileSnapshot(sourceSnapshot)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			item.Source, item.SourceIdentity, item.SourceSnapshot, item.SourceParentIdentity = source, sourceIdentity, sourceSnapshot, sourceParent
			item.Destination, item.ImmediateParentPath, item.ParentProviderIndex, item.NearestAncestorIdentity = target, parentPath, provider, nearestIdentity
			item.Bytes, item.FileCount, item.ExpectedResultFingerprint = sourceSnapshot.Size, 1, fingerprint
			if err := addSourceBytes(item.Bytes); err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			if err := addStagingBytes(item.Bytes); err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			registerOperand(index, "source", source.ResolvedPath, declared.Type)
			registerOperand(index, "target", target.ResolvedPath, declared.Type)

		case OperationCopyDirectory:
			source, err := planner.authorizeExisting(declared.Source)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			sourceIdentity, err := filesystem.CaptureObjectIdentity(source.ResolvedPath)
			if err != nil || !sourceIdentity.IsDirectory() {
				if err == nil {
					err = operation.New(operation.KindInvalidInput, "copyDirectory source must be a real directory")
				}
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			tree, err := filesystem.EnumerateExactTree(ctx, source.ResolvedPath, treeOptions)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			target, parentPath, provider, nearestIdentity, err := planner.prepareMissingTarget(declared.Destination, mkdirByPath)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			if security.PathsOverlap(source.ResolvedPath, target.ResolvedPath) {
				return PreparedPackage{}, annotatePlanError(index, operation.New(operation.KindInvalidInput, "copyDirectory source and destination must not overlap"))
			}
			if err := registerTreeIdentities(tree, index); err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			sourceParent, err := planner.captureExistingDirectoryIdentity(filepath.Dir(source.ResolvedPath))
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			item.Source, item.SourceIdentity, item.SourceParentIdentity = source, sourceIdentity, sourceParent
			item.Destination, item.ImmediateParentPath, item.ParentProviderIndex, item.NearestAncestorIdentity = target, parentPath, provider, nearestIdentity
			item.Tree = &tree
			item.Bytes, item.FileCount, item.DirectoryCount, item.ExpectedResultFingerprint = tree.TotalBytes, tree.FileCount, tree.DirectoryCount, tree.ContentFingerprint
			if err := addSourceBytes(item.Bytes); err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			if err := addStagingBytes(item.Bytes); err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			registerOperand(index, "source", source.ResolvedPath, declared.Type)
			registerOperand(index, "target", target.ResolvedPath, declared.Type)

		case OperationMove:
			source, err := planner.authorizeExisting(declared.Source)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			sourceIdentity, err := filesystem.CaptureObjectIdentity(source.ResolvedPath)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			target, parentPath, provider, nearestIdentity, err := planner.prepareMissingTarget(declared.Destination, mkdirByPath)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			if sourceIdentity.IsDirectory() && security.PathsOverlap(source.ResolvedPath, target.ResolvedPath) {
				return PreparedPackage{}, annotatePlanError(index, operation.New(operation.KindInvalidInput, "directory move source and destination must not overlap"))
			}
			if err := registerIdentity(sourceIdentity, index); err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			sourceParent, err := planner.captureExistingDirectoryIdentity(filepath.Dir(source.ResolvedPath))
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			sameVolume, err := sourceIdentity.SameVolume(nearestIdentity)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			if !sameVolume {
				return PreparedPackage{}, annotatePlanError(index, operation.New(operation.KindUnsupported, "move source and destination are on different filesystem volumes"))
			}
			item.Source, item.SourceIdentity, item.SourceParentIdentity = source, sourceIdentity, sourceParent
			item.Destination, item.ImmediateParentPath, item.ParentProviderIndex, item.NearestAncestorIdentity = target, parentPath, provider, nearestIdentity
			if !sourceIdentity.IsDirectory() {
				snapshot, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, source.ResolvedPath, planner.limits.MaxFileBytes)
				if err != nil {
					return PreparedPackage{}, annotatePlanError(index, err)
				}
				fingerprint, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
				if err != nil {
					return PreparedPackage{}, annotatePlanError(index, err)
				}
				item.SourceSnapshot, item.Bytes, item.FileCount, item.ExpectedResultFingerprint = snapshot, snapshot.Size, 1, fingerprint
				if err := addSourceBytes(item.Bytes); err != nil {
					return PreparedPackage{}, annotatePlanError(index, err)
				}
			} else {
				item.DirectoryCount = 1
			}
			registerOperand(index, "source", source.ResolvedPath, declared.Type)
			registerOperand(index, "target", target.ResolvedPath, declared.Type)

		case OperationDeleteFile:
			target, targetIdentity, snapshot, parentIdentity, err := planner.prepareRegularSource(ctx, declared.Path)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			if err := registerIdentity(targetIdentity, index); err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			fingerprint, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			item.Path, item.TargetIdentity, item.SourceSnapshot, item.SourceParentIdentity = target, targetIdentity, snapshot, parentIdentity
			item.Bytes, item.FileCount, item.BackupCount = snapshot.Size, 1, 1
			item.ExpectedResultFingerprint = fingerprint
			prepared.BackupRequirements = append(prepared.BackupRequirements, BackupRequirement{Path: target.ResolvedPath, ExpectedFingerprint: fingerprint, Bytes: snapshot.Size, OperationIndex: index})
			if err := addSourceBytes(item.Bytes); err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			registerOperand(index, "target", target.ResolvedPath, declared.Type)

		case OperationDeleteDirectory:
			target, err := planner.authorizeExisting(declared.Path)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			targetIdentity, err := filesystem.CaptureObjectIdentity(target.ResolvedPath)
			if err != nil || !targetIdentity.IsDirectory() {
				if err == nil {
					err = operation.New(operation.KindInvalidInput, "deleteDirectory target must be a real directory")
				}
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			tree, err := filesystem.EnumerateExactTree(ctx, target.ResolvedPath, treeOptions)
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			if err := registerTreeIdentities(tree, index); err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			parentIdentity, err := planner.captureExistingDirectoryIdentity(filepath.Dir(target.ResolvedPath))
			if err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			item.Path, item.TargetIdentity, item.SourceParentIdentity, item.Tree = target, targetIdentity, parentIdentity, &tree
			item.Bytes, item.FileCount, item.DirectoryCount, item.BackupCount = tree.TotalBytes, tree.FileCount, tree.DirectoryCount, tree.FileCount
			item.ExpectedResultFingerprint = tree.ContentFingerprint
			for _, entry := range tree.Entries {
				if entry.IsDirectory {
					continue
				}
				fingerprint, err := filesystem.FingerprintRegularFileSnapshot(entry.Snapshot)
				if err != nil {
					return PreparedPackage{}, annotatePlanError(index, err)
				}
				prepared.BackupRequirements = append(prepared.BackupRequirements, BackupRequirement{Path: entry.Path, ExpectedFingerprint: fingerprint, Bytes: entry.Size, OperationIndex: index})
			}
			if err := addSourceBytes(item.Bytes); err != nil {
				return PreparedPackage{}, annotatePlanError(index, err)
			}
			registerOperand(index, "target", target.ResolvedPath, declared.Type)
		}
		prepared.Operations = append(prepared.Operations, item)
	}

	if err := validateOperandConflicts(operands); err != nil {
		return PreparedPackage{}, err
	}
	if len(prepared.BackupRequirements) > 0 {
		if planner.backupPreflight == nil {
			return PreparedPackage{}, operation.New(operation.KindInvalidInput, "persistent backup store is required for destructive filesystem packages")
		}
		requests := make([]backupstore.CaptureRequest, 0, len(prepared.BackupRequirements))
		for _, requirement := range prepared.BackupRequirements {
			requests = append(requests, backupstore.CaptureRequest{
				TargetPath: requirement.Path, SourceOperation: backupstore.SourceOperationFilesystemPackage,
			})
		}
		if err := planner.backupPreflight(ctx, requests); err != nil {
			return PreparedPackage{}, err
		}
		for _, requirement := range prepared.BackupRequirements {
			snapshot, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, requirement.Path, planner.limits.MaxFileBytes)
			if err != nil {
				return PreparedPackage{}, operation.Wrap(operation.KindConflict, "verify_backup_preflight_source", requirement.Path, err)
			}
			fingerprint, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
			if err != nil || fingerprint != requirement.ExpectedFingerprint || snapshot.Size != requirement.Bytes {
				if err == nil {
					err = operation.New(operation.KindConflict, "backup source changed during preview preflight")
				}
				return PreparedPackage{}, operation.Wrap(operation.KindConflict, "verify_backup_preflight_source", requirement.Path, err)
			}
		}
	}
	return prepared, nil
}

func (planner *Planner) prepareMissingTarget(path string, mkdirByPath map[string]int) (security.PathEvidence, string, int, filesystem.ObjectIdentity, error) {
	target, err := planner.authorize(path)
	if err != nil {
		return security.PathEvidence{}, "", -1, filesystem.ObjectIdentity{}, err
	}
	if target.Exists {
		return security.PathEvidence{}, "", -1, filesystem.ObjectIdentity{}, operation.New(operation.KindConflict, fmt.Sprintf("destination already exists: %s", target.ResolvedPath))
	}
	nearestIdentity, err := filesystem.CaptureObjectIdentity(target.NearestExistingPath)
	if err != nil || !nearestIdentity.IsDirectory() {
		if err == nil {
			err = operation.New(operation.KindInvalidPath, "nearest existing destination ancestor is not a real directory")
		}
		return security.PathEvidence{}, "", -1, filesystem.ObjectIdentity{}, err
	}
	parentPath := filepath.Dir(target.ResolvedPath)
	parentEvidence, err := planner.authorize(parentPath)
	if err != nil {
		return security.PathEvidence{}, "", -1, filesystem.ObjectIdentity{}, err
	}
	if parentEvidence.Exists {
		parentIdentity, err := filesystem.CaptureObjectIdentity(parentEvidence.ResolvedPath)
		if err != nil || !parentIdentity.IsDirectory() {
			if err == nil {
				err = operation.New(operation.KindInvalidPath, "destination parent is not a real directory")
			}
			return security.PathEvidence{}, "", -1, filesystem.ObjectIdentity{}, err
		}
		return target, parentEvidence.ResolvedPath, -1, parentIdentity, nil
	}
	provider, ok := mkdirByPath[plannerPathKey(parentEvidence.ResolvedPath)]
	if !ok {
		return security.PathEvidence{}, "", -1, filesystem.ObjectIdentity{}, operation.New(operation.KindInvalidInput, fmt.Sprintf("destination parent does not exist and is not provided by an earlier mkdir: %s", parentPath))
	}
	return target, parentEvidence.ResolvedPath, provider, nearestIdentity, nil
}

func (planner *Planner) authorizeExisting(path string) (security.PathEvidence, error) {
	evidence, err := planner.authorize(path)
	if err != nil {
		return security.PathEvidence{}, err
	}
	if !evidence.Exists {
		return security.PathEvidence{}, operation.New(operation.KindNotFound, fmt.Sprintf("source path does not exist: %s", path))
	}
	return evidence, nil
}

func (planner *Planner) prepareRegularSource(ctx context.Context, path string) (security.PathEvidence, filesystem.ObjectIdentity, filesystem.FileSnapshot, filesystem.ObjectIdentity, error) {
	evidence, err := planner.authorizeExisting(path)
	if err != nil {
		return security.PathEvidence{}, filesystem.ObjectIdentity{}, filesystem.FileSnapshot{}, filesystem.ObjectIdentity{}, err
	}
	identity, err := filesystem.CaptureObjectIdentity(evidence.ResolvedPath)
	if err != nil {
		return security.PathEvidence{}, filesystem.ObjectIdentity{}, filesystem.FileSnapshot{}, filesystem.ObjectIdentity{}, err
	}
	if identity.IsDirectory() {
		return security.PathEvidence{}, filesystem.ObjectIdentity{}, filesystem.FileSnapshot{}, filesystem.ObjectIdentity{}, operation.New(operation.KindInvalidInput, fmt.Sprintf("regular file required: %s", path))
	}
	snapshot, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, evidence.ResolvedPath, planner.limits.MaxFileBytes)
	if err != nil {
		return security.PathEvidence{}, filesystem.ObjectIdentity{}, filesystem.FileSnapshot{}, filesystem.ObjectIdentity{}, err
	}
	parentIdentity, err := planner.captureExistingDirectoryIdentity(filepath.Dir(evidence.ResolvedPath))
	if err != nil {
		return security.PathEvidence{}, filesystem.ObjectIdentity{}, filesystem.FileSnapshot{}, filesystem.ObjectIdentity{}, err
	}
	return evidence, identity, snapshot, parentIdentity, nil
}

func (planner *Planner) captureExistingDirectoryIdentity(path string) (filesystem.ObjectIdentity, error) {
	evidence, err := planner.authorize(path)
	if err != nil {
		return filesystem.ObjectIdentity{}, err
	}
	if !evidence.Exists {
		return filesystem.ObjectIdentity{}, operation.New(operation.KindConflict, fmt.Sprintf("expected parent directory is missing: %s", path))
	}
	identity, err := filesystem.CaptureObjectIdentity(evidence.ResolvedPath)
	if err != nil {
		return filesystem.ObjectIdentity{}, err
	}
	if !identity.IsDirectory() {
		return filesystem.ObjectIdentity{}, operation.New(operation.KindInvalidPath, fmt.Sprintf("expected directory: %s", path))
	}
	return identity, nil
}

func validateOperandConflicts(operands []plannerOperand) error {
	seen := make(map[string]plannerOperand, len(operands))
	for _, operand := range operands {
		if previous, ok := seen[operand.key]; ok {
			return operation.New(operation.KindInvalidInput, fmt.Sprintf("operations %d and %d use the same canonical path %s", previous.index, operand.index, operand.path))
		}
		seen[operand.key] = operand
	}
	for firstIndex := 0; firstIndex < len(operands); firstIndex++ {
		for secondIndex := firstIndex + 1; secondIndex < len(operands); secondIndex++ {
			first, second := operands[firstIndex], operands[secondIndex]
			if !security.PathsOverlap(first.path, second.path) || security.PathsEqual(first.path, second.path) {
				continue
			}
			if approvedMkdirParent(first, second) || approvedMkdirParent(second, first) {
				continue
			}
			return operation.New(operation.KindInvalidInput, fmt.Sprintf("operations %d and %d have overlapping filesystem operands", first.index, second.index))
		}
	}
	return nil
}

func approvedMkdirParent(parent, child plannerOperand) bool {
	return parent.typeName == OperationMkdir && parent.role == "target" && parent.index < child.index && child.role == "target" &&
		security.PathsOverlap(parent.path, child.path) && !security.PathsEqual(parent.path, child.path)
}

func plannerPathKey(path string) string {
	key := norm.NFC.String(filepath.Clean(path))
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}

func annotatePlanError(index int, err error) error {
	if err == nil {
		return nil
	}
	return operation.Wrap(operation.KindOf(err), fmt.Sprintf("plan_operation_%d", index), "", err)
}
