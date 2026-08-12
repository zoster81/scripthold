package handler

import (
	"context"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/backupstore"
)

type r23ReadOnlyBackupAuthority struct {
	root string
}

func (authority *r23ReadOnlyBackupAuthority) Root() string {
	return authority.root
}

func (*r23ReadOnlyBackupAuthority) Status(context.Context) (backupstore.StoreStatus, error) {
	return backupstore.StoreStatus{}, nil
}

func (*r23ReadOnlyBackupAuthority) List(context.Context, backupstore.ListOptions) (backupstore.ListResult, error) {
	return backupstore.ListResult{}, nil
}

func (*r23ReadOnlyBackupAuthority) Inspect(context.Context, string, backupstore.InspectOptions) (backupstore.InspectResult, error) {
	return backupstore.InspectResult{}, nil
}

func (*r23ReadOnlyBackupAuthority) Audit(context.Context, backupstore.AuditOptions) (backupstore.AuditReport, error) {
	return backupstore.AuditReport{}, nil
}

func (*r23ReadOnlyBackupAuthority) PreflightCaptureBatch(context.Context, []backupstore.CaptureRequest) error {
	return nil
}

func (*r23ReadOnlyBackupAuthority) OpenReadSource(context.Context, string, backupstore.RestoreSourceOptions) (*backupstore.ReadSource, error) {
	return nil, nil
}

func (*r23ReadOnlyBackupAuthority) RestorePlanTTL() time.Duration {
	return time.Minute
}

func (*r23ReadOnlyBackupAuthority) RestoreObjectLimit() int64 {
	return 1024
}

func (*r23ReadOnlyBackupAuthority) PlanGC(context.Context, backupstore.GCOptions) (backupstore.GCPlan, error) {
	return backupstore.GCPlan{}, nil
}

func (*r23ReadOnlyBackupAuthority) GCPlanTTL() time.Duration {
	return time.Minute
}

var (
	_ BackupStoreReader             = (*r23ReadOnlyBackupAuthority)(nil)
	_ BackupStoreCapturePreflighter = (*r23ReadOnlyBackupAuthority)(nil)
	_ BackupStoreRestoreReader      = (*r23ReadOnlyBackupAuthority)(nil)
	_ BackupStoreGCPlanner          = (*r23ReadOnlyBackupAuthority)(nil)
)

func TestR23ReadOnlyBackupAuthorityDoesNotAcquireMutationCapabilities(t *testing.T) {
	authority := &r23ReadOnlyBackupAuthority{root: t.TempDir()}
	h := NewHandler([]string{t.TempDir()}, WithBackupStore(authority))

	if h.backupStore == nil || h.backupCapturePreflight == nil || h.backupRestoreReader == nil || h.backupGCPlanner == nil {
		t.Fatalf("read-only authorities were not detected: store=%t preflight=%t restoreReader=%t gcPlanner=%t",
			h.backupStore != nil, h.backupCapturePreflight != nil, h.backupRestoreReader != nil, h.backupGCPlanner != nil)
	}
	if h.backupCapture != nil || h.backupBatchCapture != nil || h.backupRestoreStager != nil || h.backupGCApplier != nil {
		t.Fatalf("read-only authority unexpectedly acquired mutation capabilities: capture=%t batchCapture=%t restoreStager=%t gcApply=%t",
			h.backupCapture != nil, h.backupBatchCapture != nil, h.backupRestoreStager != nil, h.backupGCApplier != nil)
	}
}
