package handler

import (
	"context"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/backupstore"
)

type readOnlyBackupAuthority struct {
	root string
}

func (authority *readOnlyBackupAuthority) Root() string {
	return authority.root
}

func (*readOnlyBackupAuthority) Status(context.Context) (backupstore.StoreStatus, error) {
	return backupstore.StoreStatus{}, nil
}

func (*readOnlyBackupAuthority) List(context.Context, backupstore.ListOptions) (backupstore.ListResult, error) {
	return backupstore.ListResult{}, nil
}

func (*readOnlyBackupAuthority) Inspect(context.Context, string, backupstore.InspectOptions) (backupstore.InspectResult, error) {
	return backupstore.InspectResult{}, nil
}

func (*readOnlyBackupAuthority) Audit(context.Context, backupstore.AuditOptions) (backupstore.AuditReport, error) {
	return backupstore.AuditReport{}, nil
}

func (*readOnlyBackupAuthority) PreflightCaptureBatch(context.Context, []backupstore.CaptureRequest) error {
	return nil
}

func (*readOnlyBackupAuthority) OpenReadSource(context.Context, string, backupstore.RestoreSourceOptions) (*backupstore.ReadSource, error) {
	return nil, nil
}

func (*readOnlyBackupAuthority) RestorePlanTTL() time.Duration {
	return time.Minute
}

func (*readOnlyBackupAuthority) RestoreObjectLimit() int64 {
	return 1024
}

func (*readOnlyBackupAuthority) PlanGC(context.Context, backupstore.GCOptions) (backupstore.GCPlan, error) {
	return backupstore.GCPlan{}, nil
}

func (*readOnlyBackupAuthority) GCPlanTTL() time.Duration {
	return time.Minute
}

var (
	_ BackupStoreReader             = (*readOnlyBackupAuthority)(nil)
	_ BackupStoreCapturePreflighter = (*readOnlyBackupAuthority)(nil)
	_ BackupStoreRestoreReader      = (*readOnlyBackupAuthority)(nil)
	_ BackupStoreGCPlanner          = (*readOnlyBackupAuthority)(nil)
)

func TestReadOnlyBackupAuthorityDoesNotAcquireMutationCapabilities(t *testing.T) {
	authority := &readOnlyBackupAuthority{root: t.TempDir()}
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
