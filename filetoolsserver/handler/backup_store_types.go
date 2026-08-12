package handler

import (
	"bytes"
	"encoding/json"

	"github.com/zoster81/scripthold/internal/backupstore"
)

const (
	BackupStoreActionStatus         = "status"
	BackupStoreActionList           = "list"
	BackupStoreActionHistory        = "history"
	BackupStoreActionInspect        = "inspect"
	BackupStoreActionCompare        = "compare"
	BackupStoreActionAudit          = "audit"
	BackupStoreActionRestorePreview = "restorePreview"
	BackupStoreActionRestoreApply   = "restoreApply"
	BackupStoreActionGCDryRun       = "gcDryRun"
	BackupStoreActionGCApply        = "gcApply"

	BackupStoreStateDisabled = "disabled"
	BackupStoreStateReady    = "ready"
	BackupStoreStateDegraded = "degraded"

	BackupStoreRestoreStatePrepared  = "prepared"
	BackupStoreRestoreStateRestored  = "restored"
	BackupStoreRestoreStateUnchanged = "unchanged"
	BackupStoreRestoreStateMissing   = "missing"
	BackupStoreRestoreStateUnknown   = "unknown"

	BackupStoreGCStatePrepared = "prepared"
	BackupStoreGCStateApplied  = "applied"
	BackupStoreGCStatePartial  = "partial"
	BackupStoreGCStateNoop     = "no_op"
)

// BackupStoreInput selects one strict backup management or restore action.
type BackupStoreInput struct {
	Action        string `json:"action"`
	Cursor        string `json:"cursor,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	TargetPath    string `json:"targetPath,omitempty"`
	Pinned        *bool  `json:"pinned,omitempty"`
	BackupID      string `json:"backupId,omitempty"`
	OtherBackupID string `json:"otherBackupId,omitempty"`
	PreviewID     string `json:"previewId,omitempty"`
	AuditMode     string `json:"auditMode,omitempty"`
	MaxObjects    int    `json:"maxObjects,omitempty"`
	MaxBytes      int64  `json:"maxBytes,omitempty"`
}

// UnmarshalJSON rejects unknown fields and trailing JSON values.
func (input *BackupStoreInput) UnmarshalJSON(data []byte) error {
	type alias BackupStoreInput
	var decoded alias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	*input = BackupStoreInput(decoded)
	return nil
}

// BackupStoreOutput is the redacted management and restore action union.
type BackupStoreOutput struct {
	Action     string                    `json:"action"`
	Enabled    bool                      `json:"enabled"`
	State      string                    `json:"state"`
	Status     *BackupStoreStatusOutput  `json:"status,omitempty"`
	Items      []BackupStoreManifestItem `json:"items,omitempty"`
	Generation string                    `json:"generation,omitempty"`
	NextCursor string                    `json:"nextCursor,omitempty"`
	Manifest   *BackupStoreInspectOutput `json:"manifest,omitempty"`
	Compare    *BackupStoreCompareOutput `json:"compare,omitempty"`
	Audit      *BackupStoreAuditOutput   `json:"audit,omitempty"`
	Restore    *BackupStoreRestoreOutput `json:"restore,omitempty"`
	GC         *BackupStoreGCOutput      `json:"gc,omitempty"`
}

type BackupStoreLimitsOutput struct {
	MaxTotalBytes        int64 `json:"maxTotalBytes"`
	MaxObjectBytes       int64 `json:"maxObjectBytes"`
	MaxManifests         int   `json:"maxManifests"`
	MaxVersionsPerTarget int   `json:"maxVersionsPerTarget"`
	MaxPinned            int   `json:"maxPinned"`
	RetentionDays        int   `json:"retentionDays"`
	PlanTTLSeconds       int   `json:"planTTLSeconds"`
}

type BackupStoreStatusOutput struct {
	FormatVersion     string                  `json:"formatVersion"`
	DefaultPolicy     string                  `json:"defaultPolicy"`
	ManifestVersion   string                  `json:"manifestVersion"`
	IndexVersion      string                  `json:"indexVersion"`
	ObjectAlgorithm   string                  `json:"objectAlgorithm"`
	Healthy           bool                    `json:"healthy"`
	Generation        string                  `json:"generation"`
	TotalObjectBytes  int64                   `json:"totalObjectBytes"`
	ObjectCount       int                     `json:"objectCount"`
	ManifestCount     int                     `json:"manifestCount"`
	PinnedCount       int                     `json:"pinnedCount"`
	OrphanObjectCount int                     `json:"orphanObjectCount"`
	StagingEntryCount int                     `json:"stagingEntryCount"`
	TrashEntryCount   int                     `json:"trashEntryCount"`
	Limits            BackupStoreLimitsOutput `json:"limits"`
	Issues            []BackupStoreAuditIssue `json:"issues,omitempty"`
}

type BackupStoreManifestItem struct {
	BackupID           string                      `json:"backupId"`
	CreatedAt          string                      `json:"createdAt"`
	TargetPath         string                      `json:"targetPath"`
	SourceOperation    backupstore.SourceOperation `json:"sourceOperation"`
	ObjectDigest       string                      `json:"objectDigest"`
	ObjectBytes        int64                       `json:"objectBytes"`
	ContentFingerprint string                      `json:"contentFingerprint"`
	Pinned             bool                        `json:"pinned"`
	ManifestChecksum   string                      `json:"manifestChecksum"`
}

type BackupStoreInspectOutput struct {
	BackupID           string                      `json:"backupId"`
	CreatedAt          string                      `json:"createdAt"`
	TargetPath         string                      `json:"targetPath"`
	SourceOperation    backupstore.SourceOperation `json:"sourceOperation"`
	ObjectAlgorithm    string                      `json:"objectAlgorithm"`
	ObjectDigest       string                      `json:"objectDigest"`
	ObjectBytes        int64                       `json:"objectBytes"`
	ContentFingerprint string                      `json:"contentFingerprint"`
	OriginalMode       uint32                      `json:"originalMode"`
	OriginalModTime    string                      `json:"originalModTime"`
	Label              string                      `json:"label,omitempty"`
	Pinned             bool                        `json:"pinned"`
	ManifestChecksum   string                      `json:"manifestChecksum"`
	ObjectVerified     bool                        `json:"objectVerified"`
}

type BackupStoreCompareOutput struct {
	BackupID             string `json:"backupId"`
	OtherBackupID        string `json:"otherBackupId,omitempty"`
	TargetPath           string `json:"targetPath"`
	BackupFingerprint    string `json:"backupFingerprint"`
	OtherFingerprint     string `json:"otherFingerprint,omitempty"`
	OtherKind            string `json:"otherKind"`
	OtherExists          bool   `json:"otherExists"`
	Equal                bool   `json:"equal"`
	BackupObjectVerified bool   `json:"backupObjectVerified"`
	OtherObjectVerified  bool   `json:"otherObjectVerified,omitempty"`
	Diff                 string `json:"diff,omitempty"`
	DiffAvailable        bool   `json:"diffAvailable"`
}

type BackupStoreRestoreOutput struct {
	BackupID           string `json:"backupId"`
	PreviewID          string `json:"previewId,omitempty"`
	CreatedAt          string `json:"createdAt,omitempty"`
	ExpiresAt          string `json:"expiresAt,omitempty"`
	TargetPath         string `json:"targetPath"`
	TargetExisted      bool   `json:"targetExisted"`
	CurrentFingerprint string `json:"currentFingerprint,omitempty"`
	ResultFingerprint  string `json:"resultFingerprint"`
	ActualFingerprint  string `json:"actualFingerprint,omitempty"`
	ObjectBytes        int64  `json:"objectBytes"`
	ObjectVerified     bool   `json:"objectVerified"`
	Diff               string `json:"diff,omitempty"`
	SafetyBackupID     string `json:"safetyBackupId,omitempty"`
	State              string `json:"state"`
	Applied            bool   `json:"applied"`
	ReadOnlyCleared    bool   `json:"readOnlyCleared,omitempty"`
}

type BackupStoreGCManifestCandidate struct {
	BackupID     string                 `json:"backupId"`
	CreatedAt    string                 `json:"createdAt"`
	ObjectDigest string                 `json:"objectDigest"`
	ObjectBytes  int64                  `json:"objectBytes"`
	Reasons      []backupstore.GCReason `json:"reasons"`
}

type BackupStoreGCObjectCandidate struct {
	Digest           string `json:"digest"`
	Bytes            int64  `json:"bytes"`
	ReferencesBefore int    `json:"referencesBefore"`
}

type BackupStoreGCOutput struct {
	PreviewID                string                           `json:"previewId,omitempty"`
	CreatedAt                string                           `json:"createdAt,omitempty"`
	ExpiresAt                string                           `json:"expiresAt,omitempty"`
	PlannedAt                string                           `json:"plannedAt"`
	Generation               string                           `json:"generation"`
	PreviousGeneration       string                           `json:"previousGeneration,omitempty"`
	RetentionDays            int                              `json:"retentionDays"`
	MinimumVersionsPerTarget int                              `json:"minimumVersionsPerTarget"`
	ManifestCount            int                              `json:"manifestCount"`
	ObjectCount              int                              `json:"objectCount"`
	ReclaimableBytes         int64                            `json:"reclaimableBytes"`
	Manifests                []BackupStoreGCManifestCandidate `json:"manifests,omitempty"`
	Objects                  []BackupStoreGCObjectCandidate   `json:"objects,omitempty"`
	ManifestsRemoved         int                              `json:"manifestsRemoved,omitempty"`
	ObjectsRemoved           int                              `json:"objectsRemoved,omitempty"`
	BytesReclaimed           int64                            `json:"bytesReclaimed,omitempty"`
	TrashCleanupFailures     int                              `json:"trashCleanupFailures,omitempty"`
	TrashEntriesRemaining    int                              `json:"trashEntriesRemaining,omitempty"`
	State                    string                           `json:"state"`
	Applied                  bool                             `json:"applied"`
}

type BackupStoreAuditIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type BackupStoreAuditOutput struct {
	Mode              backupstore.AuditMode   `json:"mode"`
	Healthy           bool                    `json:"healthy"`
	Generation        string                  `json:"generation"`
	ManifestCount     int                     `json:"manifestCount"`
	ObjectCount       int                     `json:"objectCount"`
	ReferencedBytes   int64                   `json:"referencedBytes"`
	OrphanObjectCount int                     `json:"orphanObjectCount"`
	OrphanObjectBytes int64                   `json:"orphanObjectBytes"`
	StagingEntryCount int                     `json:"stagingEntryCount"`
	StagingEntryBytes int64                   `json:"stagingEntryBytes"`
	TrashEntryCount   int                     `json:"trashEntryCount"`
	TrashEntryBytes   int64                   `json:"trashEntryBytes"`
	IndexConsistent   bool                    `json:"indexConsistent"`
	Issues            []BackupStoreAuditIssue `json:"issues,omitempty"`
}
