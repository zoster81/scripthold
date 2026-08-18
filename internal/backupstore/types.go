package backupstore

import "time"

const (
	defaultMaxTotalBytes        = int64(1024 * 1024 * 1024)
	defaultMaxObjectBytes       = int64(64 * 1024 * 1024)
	defaultMaxManifests         = 10_000
	defaultMaxVersionsPerTarget = 32
	defaultMaxPinned            = 256
	defaultRetentionDays        = 30
	defaultPlanTTLSeconds       = 15 * 60

	hardMaxTotalBytes        = int64(1 << 40)
	hardMaxObjectBytes       = int64(1 << 30)
	hardMaxManifests         = 1_000_000
	hardMaxVersionsPerTarget = 10_000
	hardMaxPinned            = 100_000
	hardMaxRetentionDays     = 3650
	hardMaxPlanTTLSeconds    = 24 * 60 * 60

	maxManifestBytes = 64 * 1024
	maxIndexBytes    = 64 * 1024
	maxLabelBytes    = 256
)

// Limits bounds committed and in-flight persistent backup state.
type Limits struct {
	MaxTotalBytes        int64
	MaxObjectBytes       int64
	MaxManifests         int
	MaxVersionsPerTarget int
	MaxPinned            int
	RetentionDays        int
	PlanTTLSeconds       int
}

func defaultStoreLimits() Limits {
	return Limits{
		MaxTotalBytes:        defaultMaxTotalBytes,
		MaxObjectBytes:       defaultMaxObjectBytes,
		MaxManifests:         defaultMaxManifests,
		MaxVersionsPerTarget: defaultMaxVersionsPerTarget,
		MaxPinned:            defaultMaxPinned,
		RetentionDays:        defaultRetentionDays,
		PlanTTLSeconds:       defaultPlanTTLSeconds,
	}
}

// SourceOperation identifies the approved operation category that captured a
// manifest. It is metadata, not authority to perform that operation.
type SourceOperation string

const (
	SourceOperationEdit              SourceOperation = "edit"
	SourceOperationPatchPackage      SourceOperation = "patch_package"
	SourceOperationFilesystemPackage SourceOperation = "filesystem_package"
	SourceOperationRestore           SourceOperation = "restore"
	SourceOperationManageBOM         SourceOperation = "manage_bom"
	SourceOperationConvertEncoding   SourceOperation = "convert_encoding"
)

// CaptureRequest describes one exact pre-state capture. TargetPath must already
// be the normalized, authorized original target path.
type CaptureRequest struct {
	TargetPath      string
	SourceOperation SourceOperation
	Label           string
	Pinned          bool
}

// Manifest is the immutable durable record for one captured target state.
type Manifest struct {
	FormatVersion      string          `json:"formatVersion"`
	StoreFormatVersion string          `json:"storeFormatVersion"`
	StoreID            string          `json:"storeId"`
	BackupID           string          `json:"backupId"`
	CreatedAt          string          `json:"createdAt"`
	TargetPath         string          `json:"targetPath"`
	SourceOperation    SourceOperation `json:"sourceOperation"`
	ObjectAlgorithm    string          `json:"objectAlgorithm"`
	ObjectDigest       string          `json:"objectDigest"`
	ObjectBytes        int64           `json:"objectBytes"`
	ContentFingerprint string          `json:"contentFingerprint"`
	OriginalMode       uint32          `json:"originalMode"`
	OriginalModTime    string          `json:"originalModTime"`
	Label              string          `json:"label,omitempty"`
	Pinned             bool            `json:"pinned"`
	ManifestChecksum   string          `json:"manifestChecksum"`
}

// CaptureResult reports the committed manifest and whether this capture
// installed a new unique object rather than reusing a verified existing one.
type CaptureResult struct {
	Manifest      Manifest
	ObjectCreated bool
}

// ManifestSummary is the bounded derived index projection of one manifest.
type ManifestSummary struct {
	BackupID           string          `json:"backupId"`
	CreatedAt          string          `json:"createdAt"`
	TargetPath         string          `json:"targetPath"`
	SourceOperation    SourceOperation `json:"sourceOperation"`
	ObjectDigest       string          `json:"objectDigest"`
	ObjectBytes        int64           `json:"objectBytes"`
	ContentFingerprint string          `json:"contentFingerprint"`
	Pinned             bool            `json:"pinned"`
	ManifestChecksum   string          `json:"manifestChecksum"`
}

// ObjectSummary records one unique object and its live manifest reference count.
type ObjectSummary struct {
	Digest     string `json:"digest"`
	Bytes      int64  `json:"bytes"`
	References int    `json:"references"`
}

// TargetSummary records quota-relevant manifest counts for one target.
type TargetSummary struct {
	TargetPath    string `json:"targetPath"`
	ManifestCount int    `json:"manifestCount"`
	PinnedCount   int    `json:"pinnedCount"`
	LatestAt      string `json:"latestAt"`
}

// Index is a disposable deterministic projection rebuilt from manifests and
// object metadata. Generation is a digest of authoritative manifest state.
type Index struct {
	FormatVersion    string            `json:"formatVersion"`
	StoreID          string            `json:"storeId"`
	GeneratedAt      string            `json:"generatedAt"`
	Generation       string            `json:"generation"`
	TotalObjectBytes int64             `json:"totalObjectBytes"`
	ObjectCount      int               `json:"objectCount"`
	ManifestCount    int               `json:"manifestCount"`
	PinnedCount      int               `json:"pinnedCount"`
	Manifests        []ManifestSummary `json:"manifests"`
	Objects          []ObjectSummary   `json:"objects"`
	Targets          []TargetSummary   `json:"targets"`
}

// AuditMode selects metadata-only or complete object-integrity verification.
type AuditMode string

const (
	AuditQuick AuditMode = "quick"
	AuditFull  AuditMode = "full"
)

const (
	AuditIssueLimit          = "LIMIT"
	AuditIssueManifest       = "MANIFEST_INVALID"
	AuditIssueObjectMissing  = "OBJECT_MISSING"
	AuditIssueObjectMetadata = "OBJECT_METADATA_INVALID"
	AuditIssueObjectDigest   = "OBJECT_DIGEST_MISMATCH"
	AuditIssueStoreEntry     = "STORE_ENTRY_INVALID"
	AuditIssueIndex          = "INDEX_REBUILD_REQUIRED"
)

// AuditOptions independently bounds a read-only audit. Zero values use the
// configured store limits.
type AuditOptions struct {
	Mode       AuditMode
	MaxObjects int
	MaxBytes   int64
}

// AuditIssue is deliberately path-free and content-free.
type AuditIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// AuditReport contains bounded structural and integrity evidence.
type AuditReport struct {
	Mode              AuditMode    `json:"mode"`
	Healthy           bool         `json:"healthy"`
	Generation        string       `json:"generation"`
	ManifestCount     int          `json:"manifestCount"`
	ObjectCount       int          `json:"objectCount"`
	ReferencedBytes   int64        `json:"referencedBytes"`
	OrphanObjectCount int          `json:"orphanObjectCount"`
	OrphanObjectBytes int64        `json:"orphanObjectBytes"`
	StagingEntryCount int          `json:"stagingEntryCount"`
	StagingEntryBytes int64        `json:"stagingEntryBytes"`
	TrashEntryCount   int          `json:"trashEntryCount"`
	TrashEntryBytes   int64        `json:"trashEntryBytes"`
	IndexConsistent   bool         `json:"indexConsistent"`
	Issues            []AuditIssue `json:"issues,omitempty"`
}

type reservation struct {
	bytes      int64
	manifests  int
	pinned     int
	targetPath string
}

// GCReason explains why one unpinned manifest is eligible for explicit GC.
type GCReason string

const (
	GCReasonRetention    GCReason = "retention"
	GCReasonVersionLimit GCReason = "version_limit"
)

// GCOptions fixes the policy evaluation instant. Zero uses the current UTC time.
type GCOptions struct {
	Now time.Time
}

// GCManifestCandidate is one deterministic manifest removal candidate.
type GCManifestCandidate struct {
	BackupID     string     `json:"backupId"`
	CreatedAt    string     `json:"createdAt"`
	TargetPath   string     `json:"-"`
	ObjectDigest string     `json:"objectDigest"`
	ObjectBytes  int64      `json:"objectBytes"`
	Pinned       bool       `json:"pinned"`
	Reasons      []GCReason `json:"reasons"`
}

// GCObjectCandidate becomes unreferenced after every selected manifest is removed.
type GCObjectCandidate struct {
	Digest           string `json:"digest"`
	Bytes            int64  `json:"bytes"`
	ReferencesBefore int    `json:"referencesBefore"`
}

// GCPlan is an immutable generation-bound dry-run result. It contains no store paths.
type GCPlan struct {
	PlannedAt                string                `json:"plannedAt"`
	Generation               string                `json:"generation"`
	RetentionDays            int                   `json:"retentionDays"`
	MinimumVersionsPerTarget int                   `json:"minimumVersionsPerTarget"`
	ManifestCount            int                   `json:"manifestCount"`
	ObjectCount              int                   `json:"objectCount"`
	ReclaimableBytes         int64                 `json:"reclaimableBytes"`
	Manifests                []GCManifestCandidate `json:"manifests,omitempty"`
	Objects                  []GCObjectCandidate   `json:"objects,omitempty"`
}

// GCResult reports durable namespace removal and best-effort trash cleanup.
type GCResult struct {
	PreviousGeneration    string `json:"previousGeneration"`
	Generation            string `json:"generation"`
	ManifestsRemoved      int    `json:"manifestsRemoved"`
	ObjectsRemoved        int    `json:"objectsRemoved"`
	BytesReclaimed        int64  `json:"bytesReclaimed"`
	TrashCleanupFailures  int    `json:"trashCleanupFailures"`
	TrashEntriesRemaining int    `json:"trashEntriesRemaining"`
}

func utcTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func addNonNegativeInt64(total *int64, value int64) bool {
	const maxInt64 = int64(^uint64(0) >> 1)
	if total == nil || value < 0 || *total < 0 || *total > maxInt64-value {
		return false
	}
	*total += value
	return true
}
