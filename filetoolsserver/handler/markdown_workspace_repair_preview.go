package handler

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/marksplice"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

type preparedMarkdownWorkspaceRepairTarget struct {
	documentKey       marksplice.DocumentKey
	requestedPath     string
	resolvedPath      string
	resultData        []byte
	targetFingerprint string
	resultFingerprint string
	encoding          string
	bomType           string
	lineEndingStyle   string
	diff              string
	change            marksplice.ChangeSet
	identityFile      *filesystem.FileIdentity
	hasBOM            bool
	changed           bool
}

type preparedMarkdownWorkspaceRepair struct {
	root         string
	backupPolicy string
	targets      []preparedMarkdownWorkspaceRepairTarget
}

type markdownWorkspaceRepairTargetSet struct {
	documentKeys  map[marksplice.DocumentKey]struct{}
	physicalPaths map[string]struct{}
	physicalInfos []os.FileInfo
}

func (target *preparedMarkdownWorkspaceRepairTarget) close() {
	if target == nil || target.identityFile == nil {
		return
	}
	_ = target.identityFile.Close()
	target.identityFile = nil
}

func (prepared *preparedMarkdownWorkspaceRepair) close() {
	if prepared == nil {
		return
	}
	for index := range prepared.targets {
		prepared.targets[index].close()
	}
}

func preparedMarkdownWorkspaceRepairReplacement(target *preparedMarkdownWorkspaceRepairTarget) preparedExistingFileReplacement {
	return preparedExistingFileReplacement{
		requestedPath:     target.requestedPath,
		resolvedPath:      target.resolvedPath,
		resultData:        target.resultData,
		targetFingerprint: target.targetFingerprint,
		resultFingerprint: target.resultFingerprint,
		identitySlot:      &target.identityFile,
		changed:           target.changed,
	}
}

func (h *Handler) prepareMarkdownWorkspaceRepairPreview(ctx context.Context, root string, repairs []marksplice.WorkspaceRepair, requestedBackupPolicy string) (preparedMarkdownWorkspaceRepair, *mcp.CallToolResult) {
	if len(repairs) == 0 {
		return preparedMarkdownWorkspaceRepair{}, errorResultWithCode(ErrCodeInvalidInput, "Markdown workspace repair preview requires at least one repair")
	}
	if len(repairs) > markdownWorkspaceMaxItems {
		return preparedMarkdownWorkspaceRepair{}, errorResultWithCode(ErrCodeLimit, fmt.Sprintf("Markdown workspace repair count %d exceeds limit %d", len(repairs), markdownWorkspaceMaxItems))
	}
	backupPolicy, err := h.effectivePersistentBackupPolicy(requestedBackupPolicy)
	if err != nil {
		return preparedMarkdownWorkspaceRepair{}, errorResultFromError(err)
	}
	workspaceAdapter, err := newMarkdownWorkspaceFS(ctx, h, root, "")
	if err != nil {
		return preparedMarkdownWorkspaceRepair{}, errorResultFromError(err)
	}
	prepared := preparedMarkdownWorkspaceRepair{
		root:         workspaceAdapter.root,
		backupPolicy: backupPolicy,
		targets:      make([]preparedMarkdownWorkspaceRepairTarget, 0, len(repairs)),
	}
	keepPrepared := false
	defer func() {
		if !keepPrepared {
			prepared.close()
		}
	}()

	targetSet := newMarkdownWorkspaceRepairTargetSet(len(repairs))
	for _, repair := range repairs {
		if err := ctx.Err(); err != nil {
			return preparedMarkdownWorkspaceRepair{}, errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_workspace_repair_preview", prepared.root, err))
		}
		target, failure := h.prepareMarkdownWorkspaceRepairTarget(ctx, prepared.root, repair)
		if failure != nil {
			return preparedMarkdownWorkspaceRepair{}, failure
		}
		if failure := targetSet.add(repair.Document(), &target); failure != nil {
			return preparedMarkdownWorkspaceRepair{}, failure
		}
		prepared.targets = append(prepared.targets, target)
	}
	if failure := h.revalidatePreparedMarkdownWorkspaceRepairs(ctx, prepared.targets); failure != nil {
		return preparedMarkdownWorkspaceRepair{}, failure
	}
	if failure := h.preflightMarkdownWorkspaceRepairBackups(ctx, prepared); failure != nil {
		return preparedMarkdownWorkspaceRepair{}, failure
	}
	keepPrepared = true
	return prepared, nil
}

func newMarkdownWorkspaceRepairTargetSet(capacity int) markdownWorkspaceRepairTargetSet {
	return markdownWorkspaceRepairTargetSet{
		documentKeys:  make(map[marksplice.DocumentKey]struct{}, capacity),
		physicalPaths: make(map[string]struct{}, capacity),
		physicalInfos: make([]os.FileInfo, 0, capacity),
	}
}

func (set *markdownWorkspaceRepairTargetSet) add(key marksplice.DocumentKey, target *preparedMarkdownWorkspaceRepairTarget) *mcp.CallToolResult {
	if _, exists := set.documentKeys[key]; exists {
		target.close()
		return errorResultWithCode(ErrCodeConflict, "Markdown workspace repair plan contains duplicate document targets")
	}
	if _, exists := set.physicalPaths[target.resolvedPath]; exists {
		target.close()
		return errorResultWithCode(ErrCodeConflict, "Markdown workspace repair plan resolves multiple documents to the same physical file")
	}
	info, err := os.Stat(target.resolvedPath)
	if err != nil {
		target.close()
		return errorResultFromError(err)
	}
	for _, previous := range set.physicalInfos {
		if os.SameFile(previous, info) {
			target.close()
			return errorResultWithCode(ErrCodeConflict, "Markdown workspace repair plan references the same filesystem object more than once")
		}
	}
	set.documentKeys[key] = struct{}{}
	set.physicalPaths[target.resolvedPath] = struct{}{}
	set.physicalInfos = append(set.physicalInfos, info)
	return nil
}

func (h *Handler) prepareMarkdownWorkspaceRepairTarget(ctx context.Context, root string, repair marksplice.WorkspaceRepair) (preparedMarkdownWorkspaceRepairTarget, *mcp.CallToolResult) {
	key := repair.Document()
	path, failure := h.resolveMarkdownWorkspaceRepairPath(root, key)
	if failure != nil {
		return preparedMarkdownWorkspaceRepairTarget{}, failure
	}
	identity, err := filesystem.OpenFileIdentity(path)
	if err != nil {
		return preparedMarkdownWorkspaceRepairTarget{}, errorResultFromError(err)
	}
	target := preparedMarkdownWorkspaceRepairTarget{
		documentKey:   key,
		requestedPath: path,
		resolvedPath:  path,
		identityFile:  identity,
		change:        repair.Change(),
	}
	keepIdentity := false
	defer func() {
		if !keepIdentity {
			target.close()
		}
	}()

	document, sourceData, err := h.readTextDocumentWithData(ctx, path, "")
	if err != nil {
		return preparedMarkdownWorkspaceRepairTarget{}, errorResultFromError(err)
	}
	matches, err := identity.Matches(path)
	if err != nil || !matches {
		return preparedMarkdownWorkspaceRepairTarget{}, errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target identity changed while preparing preview")
	}
	if isReadOnly(document.Mode) {
		return preparedMarkdownWorkspaceRepairTarget{}, errorResultWithCode(ErrCodePermission, "Markdown workspace repair target is read-only")
	}
	if !utf8.ValidString(document.Text) {
		err := operation.Wrap(operation.KindEncoding, "markdown_workspace_repair_preview", path, fmt.Errorf("decoded Markdown is not valid UTF-8"))
		return preparedMarkdownWorkspaceRepairTarget{}, errorResultFromError(err)
	}
	sourceUTF8 := []byte(document.Text)
	resultUTF8, err := target.change.Apply(sourceUTF8)
	if err != nil {
		return preparedMarkdownWorkspaceRepairTarget{}, markdownEditErrorResult(err)
	}
	resultData, err := markdownPhysicalResult(document, sourceData, sourceUTF8, resultUTF8)
	if err != nil {
		return preparedMarkdownWorkspaceRepairTarget{}, errorResultFromError(err)
	}
	if int64(len(resultData)) > h.maxFileBytes() {
		err := operation.New(operation.KindLimit, fmt.Sprintf("prepared Markdown file size %d exceeds limit %d", len(resultData), h.maxFileBytes()))
		return preparedMarkdownWorkspaceRepairTarget{}, errorResultFromError(err)
	}
	targetFingerprint, err := filesystem.FingerprintRegularFileSnapshot(document.Snapshot)
	if err != nil {
		return preparedMarkdownWorkspaceRepairTarget{}, errorResultFromError(err)
	}
	target.resultData = resultData
	target.targetFingerprint = targetFingerprint
	target.resultFingerprint = filesystem.FingerprintRegularFileData(resultData)
	target.encoding = document.Charset
	target.bomType = document.BOM.Type
	target.lineEndingStyle = document.LineEndings.Style
	target.diff = createUnifiedDiff(string(sourceUTF8), string(resultUTF8), string(key))
	target.hasBOM = document.BOM.HasBOM
	target.changed = !bytes.Equal(sourceData, resultData)
	keepIdentity = true
	return target, nil
}

func (h *Handler) resolveMarkdownWorkspaceRepairPath(root string, key marksplice.DocumentKey) (string, *mcp.CallToolResult) {
	name := string(key)
	if name == "." || !fs.ValidPath(name) {
		return "", errorResultWithCode(ErrCodeInvalidInput, "Markdown workspace repair document key is invalid")
	}
	extension := strings.ToLower(filepath.Ext(name))
	if extension != ".md" && extension != ".markdown" {
		return "", errorResultWithCode(ErrCodeInvalidInput, "Markdown workspace repair target must be a Markdown document")
	}
	candidate := filepath.Join(root, filepath.FromSlash(name))
	validation := h.ValidatePath(candidate)
	if !validation.Ok() {
		return "", validation.Result
	}
	if !security.IsPathWithinAllowedDirectories(validation.Path, []string{root}) {
		return "", errorResultWithCode(ErrCodePermission, "Markdown workspace repair target escaped the authorized workspace root")
	}
	return validation.Path, nil
}

func (h *Handler) revalidatePreparedMarkdownWorkspaceRepairs(ctx context.Context, targets []preparedMarkdownWorkspaceRepairTarget) *mcp.CallToolResult {
	for index := range targets {
		target := &targets[index]
		validation := h.ValidatePath(target.requestedPath)
		if !validation.Ok() {
			return validation.Result
		}
		if validation.Path != target.resolvedPath || target.identityFile == nil {
			return errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target binding changed while preparing preview")
		}
		matches, err := target.identityFile.Matches(validation.Path)
		if err != nil || !matches {
			return errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target identity changed while preparing preview")
		}
		current, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, validation.Path, h.maxFileBytes())
		if err != nil {
			return errorResultFromError(err)
		}
		fingerprint, err := filesystem.FingerprintRegularFileSnapshot(current)
		if err != nil {
			return errorResultFromError(err)
		}
		if fingerprint != target.targetFingerprint {
			return errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target changed while preparing preview")
		}
	}
	return nil
}

func (h *Handler) preflightMarkdownWorkspaceRepairBackups(ctx context.Context, prepared preparedMarkdownWorkspaceRepair) *mcp.CallToolResult {
	if !persistentBackupRequired(prepared.backupPolicy) {
		return nil
	}
	requests := make([]backupstore.CaptureRequest, 0, len(prepared.targets))
	for index := range prepared.targets {
		target := &prepared.targets[index]
		if !target.changed {
			continue
		}
		requests = append(requests, backupstore.CaptureRequest{
			TargetPath:      target.resolvedPath,
			SourceOperation: backupstore.SourceOperationEdit,
			Pinned:          persistentBackupPinned(prepared.backupPolicy),
		})
	}
	if len(requests) == 0 {
		return nil
	}
	if h.backupCapturePreflight == nil {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "required backup preflight authority is unavailable"))
	}
	if err := h.backupCapturePreflight.PreflightCaptureBatch(ctx, requests); err != nil {
		return errorResultFromError(err)
	}
	return nil
}

func markdownWorkspaceRepairPreviewOutput(preview *markdownPreview, prepared preparedMarkdownWorkspaceRepair) *MarkdownWorkspaceRepairPreviewOutput {
	if preview == nil {
		return nil
	}
	output := &MarkdownWorkspaceRepairPreviewOutput{
		PreviewID: preview.id, CreatedAt: preview.createdAt.Format(timeRFC3339Nano),
		ExpiresAt: preview.expiresAt.Format(timeRFC3339Nano), BackupPolicy: prepared.backupPolicy,
		Targets: make([]MarkdownWorkspaceRepairPreviewTarget, len(prepared.targets)),
	}
	for index := range prepared.targets {
		target := &prepared.targets[index]
		output.Targets[index] = MarkdownWorkspaceRepairPreviewTarget{
			Document: string(target.documentKey), Path: target.requestedPath,
			TargetFingerprint: target.targetFingerprint, ResultFingerprint: target.resultFingerprint,
			Encoding: target.encoding, HasBOM: target.hasBOM, BOMType: target.bomType,
			LineEndingStyle: target.lineEndingStyle, Diff: target.diff, Changed: target.changed,
		}
	}
	return output
}

func (prepared preparedMarkdownWorkspaceRepair) retainedBytes() (int64, error) {
	parts := []int{len(prepared.root), len(prepared.backupPolicy)}
	for index := range prepared.targets {
		target := &prepared.targets[index]
		parts = append(parts,
			len(target.documentKey), len(target.requestedPath), len(target.resolvedPath), len(target.resultData),
			len(target.targetFingerprint), len(target.resultFingerprint), len(target.encoding), len(target.bomType),
			len(target.lineEndingStyle), len(target.diff),
		)
	}
	return markdownPreviewRetainedBytes(parts...)
}
