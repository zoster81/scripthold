package handler

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/operation"
)

const maxGCFailureMessageBytes = 1024

func (h *Handler) handleBackupStoreGCDryRun(ctx context.Context) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if h.backupGCPlanner == nil {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store does not provide garbage-collection authority")), BackupStoreOutput{}, nil
	}
	plan, err := h.backupGCPlanner.PlanGC(ctx, backupstore.GCOptions{Now: time.Now().UTC()})
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	preview, err := h.gcPreviews.put(plan)
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	output := BackupStoreOutput{
		Action:  BackupStoreActionGCDryRun,
		Enabled: true,
		State:   BackupStoreStateReady,
		GC:      gcOutputFromPreview(preview),
	}
	text := gcDryRunText(output.GC)
	if err := h.checkBackupStoreOutputLimit(output, text); err != nil {
		h.gcPreviews.discard(preview.id)
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) handleBackupStoreGCApply(ctx context.Context, previewID string) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if h.backupGCApplier == nil {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store does not provide garbage-collection authority")), BackupStoreOutput{}, nil
	}
	preview, err := h.gcPreviews.claim(previewID)
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	output := BackupStoreOutput{
		Action:  BackupStoreActionGCApply,
		Enabled: true,
		State:   BackupStoreStateReady,
		GC:      gcApplyOutputFromPreview(preview),
	}
	worst := output
	worst.GC = cloneGCOutput(output.GC)
	worst.GC.PreviousGeneration = strings.Repeat("f", gcPreviewTokenBytes*2)
	worst.GC.Generation = strings.Repeat("f", gcPreviewTokenBytes*2)
	worst.GC.State = BackupStoreGCStatePartial
	worst.GC.Applied = true
	worst.GC.ManifestsRemoved = preview.plan.ManifestCount
	worst.GC.ObjectsRemoved = preview.plan.ObjectCount
	worst.GC.BytesReclaimed = preview.plan.ReclaimableBytes
	worst.GC.TrashCleanupFailures = preview.plan.ManifestCount + preview.plan.ObjectCount
	worst.GC.TrashEntriesRemaining = preview.plan.ManifestCount + preview.plan.ObjectCount
	if err := h.checkBackupStoreOutputLimit(worst, gcFailureText(worst.GC, strings.Repeat("\x00", maxGCFailureMessageBytes))); err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}

	result, applyErr := h.backupGCApplier.ApplyGC(ctx, preview.plan)
	applyGCResult(output.GC, result, applyErr)
	if applyErr != nil {
		mapping := mapOperationError(applyErr, "")
		text := gcFailureText(output.GC, boundedGCFailureMessage(mapping.Message))
		if err := h.checkBackupStoreOutputLimit(output, text); err != nil {
			return errorResultFromError(err), BackupStoreOutput{}, nil
		}
		return errorResultWithCode(mapping.BatchCode, text), output, nil
	}
	text := gcApplyText(output.GC)
	if err := h.checkBackupStoreOutputLimit(output, text); err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func gcOutputFromPreview(preview *gcPreview) *BackupStoreGCOutput {
	plan := preview.plan
	output := &BackupStoreGCOutput{
		PreviewID:                preview.id,
		CreatedAt:                preview.createdAt.Format(timeRFC3339Nano),
		ExpiresAt:                preview.expiresAt.Format(timeRFC3339Nano),
		PlannedAt:                plan.PlannedAt,
		Generation:               plan.Generation,
		RetentionDays:            plan.RetentionDays,
		MinimumVersionsPerTarget: plan.MinimumVersionsPerTarget,
		ManifestCount:            plan.ManifestCount,
		ObjectCount:              plan.ObjectCount,
		ReclaimableBytes:         plan.ReclaimableBytes,
		State:                    BackupStoreGCStatePrepared,
	}
	if len(plan.Manifests) > 0 {
		output.Manifests = make([]BackupStoreGCManifestCandidate, len(plan.Manifests))
		for index, candidate := range plan.Manifests {
			output.Manifests[index] = BackupStoreGCManifestCandidate{
				BackupID:     candidate.BackupID,
				CreatedAt:    candidate.CreatedAt,
				ObjectDigest: candidate.ObjectDigest,
				ObjectBytes:  candidate.ObjectBytes,
				Reasons:      append([]backupstore.GCReason(nil), candidate.Reasons...),
			}
		}
	}
	if len(plan.Objects) > 0 {
		output.Objects = make([]BackupStoreGCObjectCandidate, len(plan.Objects))
		for index, candidate := range plan.Objects {
			output.Objects[index] = BackupStoreGCObjectCandidate{
				Digest:           candidate.Digest,
				Bytes:            candidate.Bytes,
				ReferencesBefore: candidate.ReferencesBefore,
			}
		}
	}
	return output
}

func gcApplyOutputFromPreview(preview *gcPreview) *BackupStoreGCOutput {
	output := gcOutputFromPreview(preview)
	output.PreviewID = ""
	output.CreatedAt = ""
	output.ExpiresAt = ""
	output.Manifests = nil
	output.Objects = nil
	return output
}

func applyGCResult(output *BackupStoreGCOutput, result backupstore.GCResult, applyErr error) {
	output.PreviousGeneration = result.PreviousGeneration
	if result.Generation != "" {
		output.Generation = result.Generation
	}
	output.ManifestsRemoved = result.ManifestsRemoved
	output.ObjectsRemoved = result.ObjectsRemoved
	output.BytesReclaimed = result.BytesReclaimed
	output.TrashCleanupFailures = result.TrashCleanupFailures
	output.TrashEntriesRemaining = result.TrashEntriesRemaining
	output.Applied = result.ManifestsRemoved > 0 || result.ObjectsRemoved > 0
	switch {
	case applyErr != nil && output.Applied:
		output.State = BackupStoreGCStatePartial
	case applyErr != nil:
		output.State = BackupStoreGCStateNoop
	case output.Applied:
		output.State = BackupStoreGCStateApplied
	default:
		output.State = BackupStoreGCStateNoop
	}
}

func cloneGCOutput(output *BackupStoreGCOutput) *BackupStoreGCOutput {
	if output == nil {
		return nil
	}
	cloned := *output
	if output.Manifests != nil {
		cloned.Manifests = make([]BackupStoreGCManifestCandidate, len(output.Manifests))
		for index, candidate := range output.Manifests {
			cloned.Manifests[index] = candidate
			cloned.Manifests[index].Reasons = append([]backupstore.GCReason(nil), candidate.Reasons...)
		}
	}
	if output.Objects != nil {
		cloned.Objects = append([]BackupStoreGCObjectCandidate(nil), output.Objects...)
	}
	return &cloned
}

func gcDryRunText(output *BackupStoreGCOutput) string {
	return fmt.Sprintf("Backup GC dry run prepared.\nPreview ID: %s\nExpires: %s\nGeneration: %s\nManifest candidates: %d\nObject candidates: %d\nReclaimable bytes: %d",
		output.PreviewID, output.ExpiresAt, output.Generation, output.ManifestCount, output.ObjectCount, output.ReclaimableBytes)
}

func gcApplyText(output *BackupStoreGCOutput) string {
	return fmt.Sprintf("Backup GC apply completed.\nState: %s\nPrevious generation: %s\nGeneration: %s\nManifests removed: %d\nObjects removed: %d\nBytes reclaimed: %d\nTrash entries remaining: %d",
		output.State, output.PreviousGeneration, output.Generation, output.ManifestsRemoved, output.ObjectsRemoved, output.BytesReclaimed, output.TrashEntriesRemaining)
}

func gcFailureText(output *BackupStoreGCOutput, message string) string {
	return fmt.Sprintf("Backup GC apply failed or completed partially.\nState: %s\nReason: %s\nPrevious generation: %s\nGeneration: %s\nManifests removed: %d\nObjects removed: %d\nBytes reclaimed: %d\nTrash entries remaining: %d",
		output.State, message, output.PreviousGeneration, output.Generation, output.ManifestsRemoved, output.ObjectsRemoved, output.BytesReclaimed, output.TrashEntriesRemaining)
}

func boundedGCFailureMessage(message string) string {
	message = strings.ToValidUTF8(message, "\uFFFD")
	if len(message) <= maxGCFailureMessageBytes {
		return message
	}
	end := maxGCFailureMessageBytes
	for end > 0 && !utf8.ValidString(message[:end]) {
		end--
	}
	return message[:end]
}
