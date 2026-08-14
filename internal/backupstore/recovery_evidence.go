package backupstore

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"

	"github.com/zoster81/scripthold/internal/operation"
)

const (
	RecoveryWarningDescriptorInvalid = "DESCRIPTOR_INVALID"
	RecoveryWarningDerivedState      = "DERIVED_STATE_ISSUE"
	RecoveryWarningLayoutInvalid     = "LAYOUT_INVALID"
	RecoveryWarningManifestRejected  = "MANIFEST_REJECTED"
	RecoveryWarningObjectRejected    = "OBJECT_REJECTED"
	RecoveryWarningOrphanObjects     = "ORPHAN_OBJECTS"
	RecoveryWarningResidue           = "RESIDUE_PRESENT"
	RecoveryWarningScanLimited       = "SCAN_LIMIT_REACHED"
	RecoveryWarningUnknownEntries    = "UNKNOWN_ENTRIES"
)

// RecoveryTrustedRecord is one authoritative manifest backed by a fully verified object.
type RecoveryTrustedRecord struct {
	Manifest Manifest

	manifestPath string
	objectPath   string
}

// RecoveryEvidence is one deterministic, mutation-free classification of an existing store.
// Internal source paths are deliberately confined to trusted records and never enter a plan.
type RecoveryEvidence struct {
	Bounds RecoveryBounds

	SourceDescriptor      Descriptor
	DescriptorValid       bool
	DescriptorFingerprint string
	EvidenceDigest        string
	CoverageComplete      bool

	TrustedRecords     []RecoveryTrustedRecord
	TrustedObjectCount int
	TrustedBytes       int64

	RejectedRecords      []RecoveryRejectedRecord
	RejectedReasonCounts []RecoveryReasonCount
	RejectedRecordCount  int

	OrphanObjectCount      int
	OrphanObjectBytes      int64
	UnknownEntryCount      int
	ResidueEntryCount      int
	ResidueEntryBytes      int64
	DerivedStateIssueCount int
	WarningCodes           []string
}

type recoveryWarningSet map[string]struct{}

func (warnings recoveryWarningSet) add(code string) {
	warnings[code] = struct{}{}
}

func (warnings recoveryWarningSet) sorted() []string {
	codes := make([]string, 0, len(warnings))
	for code := range warnings {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

type recoveryDescriptorSnapshot struct {
	descriptor  Descriptor
	valid       bool
	fingerprint string
	state       string
}

type recoveryManifestCandidate struct {
	manifest          Manifest
	path              string
	canonicalFilename bool
}

type recoveryManifestScanResult struct {
	candidates          []recoveryManifestCandidate
	indexManifests      []Manifest
	references          map[string]int
	rejectedRecords     []RecoveryRejectedRecord
	rejectedRecordCount int
	reasonCounts        map[RecoveryRejectReason]int
	events              []string
	complete            bool
	limited             bool
	structuralIssue     bool
}

type recoveryObjectState struct {
	digest  string
	bytes   int64
	path    string
	trusted bool
	reason  RecoveryRejectReason
}

type recoveryObjectScanResult struct {
	states          map[string]recoveryObjectState
	metadataObjects map[string]scannedObject
	reasonCounts    map[RecoveryRejectReason]int
	events          []string
	complete        bool
	limited         bool
	structuralIssue bool
}

// ScanRecoveryEvidence classifies an existing locked store without creating,
// repairing, deleting, renaming, or rewriting any source entry.
func (store *DiagnosticStore) ScanRecoveryEvidence(ctx context.Context, bounds RecoveryBounds) (RecoveryEvidence, error) {
	if store == nil {
		return RecoveryEvidence{}, operation.New(operation.KindInvalidInput, "backup diagnostic store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return RecoveryEvidence{}, operation.Wrap(operation.KindCancelled, "scan_backup_recovery", "", err)
	}
	normalized, err := NormalizeRecoveryBounds(bounds)
	if err != nil {
		return RecoveryEvidence{}, operation.New(operation.KindInvalidInput, err.Error())
	}

	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	if err := store.validateIdentity(); err != nil {
		return RecoveryEvidence{}, err
	}

	warnings := make(recoveryWarningSet)
	events := make([]string, 0, normalized.MaxManifests+normalized.MaxObjects+16)
	evidence := RecoveryEvidence{Bounds: normalized, CoverageComplete: true}

	descriptorBefore := inspectRecoveryDescriptor(store.root)
	evidence.SourceDescriptor = descriptorBefore.descriptor
	evidence.DescriptorValid = descriptorBefore.valid
	evidence.DescriptorFingerprint = descriptorBefore.fingerprint
	events = append(events, "descriptor|"+descriptorBefore.state+"|"+descriptorBefore.fingerprint)
	if !descriptorBefore.valid {
		warnings.add(RecoveryWarningDescriptorInvalid)
	}

	rootResult, err := scanRecoveryRootEntries(ctx, store.root, normalized)
	if err != nil {
		return RecoveryEvidence{}, err
	}
	evidence.UnknownEntryCount = rootResult.unknownCount
	if !rootResult.complete {
		evidence.CoverageComplete = false
	}
	if rootResult.unknownCount > 0 {
		warnings.add(RecoveryWarningUnknownEntries)
	}
	if rootResult.layoutIssue {
		warnings.add(RecoveryWarningLayoutInvalid)
	}
	if rootResult.limited {
		warnings.add(RecoveryWarningScanLimited)
	}
	events = append(events, rootResult.events...)

	manifests, err := scanRecoveryManifests(ctx, store.root, descriptorBefore, normalized)
	if err != nil {
		return RecoveryEvidence{}, err
	}
	if !manifests.complete {
		evidence.CoverageComplete = false
	}
	if manifests.limited {
		warnings.add(RecoveryWarningScanLimited)
	}
	if manifests.structuralIssue {
		warnings.add(RecoveryWarningLayoutInvalid)
	}
	if manifests.rejectedRecordCount > 0 || len(manifests.reasonCounts) > 0 {
		warnings.add(RecoveryWarningManifestRejected)
	}
	evidence.RejectedRecordCount = manifests.rejectedRecordCount
	evidence.RejectedRecords = append(evidence.RejectedRecords, manifests.rejectedRecords...)
	events = append(events, manifests.events...)

	objects, err := scanRecoveryObjects(ctx, store.root, manifests.references, normalized)
	if err != nil {
		return RecoveryEvidence{}, err
	}
	if !objects.complete {
		evidence.CoverageComplete = false
	}
	if objects.limited {
		warnings.add(RecoveryWarningScanLimited)
	}
	if objects.structuralIssue {
		warnings.add(RecoveryWarningLayoutInvalid)
	}
	if len(objects.reasonCounts) > 0 {
		warnings.add(RecoveryWarningObjectRejected)
	}
	events = append(events, objects.events...)

	classifyRecoveryRecords(&evidence, manifests.candidates, objects.states)
	for _, record := range evidence.RejectedRecords {
		if record.Reason != RecoveryRejectManifestInvalid && record.Reason != RecoveryRejectManifestObjectInvalid {
			warnings.add(RecoveryWarningObjectRejected)
		}
	}

	for digest, object := range objects.states {
		if !object.trusted || manifests.references[digest] != 0 {
			continue
		}
		evidence.OrphanObjectCount++
		evidence.OrphanObjectBytes += object.bytes
	}
	if evidence.OrphanObjectCount > 0 {
		warnings.add(RecoveryWarningOrphanObjects)
	}

	reasonCounts := make(map[RecoveryRejectReason]int)
	mergeRecoveryReasonCounts(reasonCounts, manifests.reasonCounts)
	mergeRecoveryReasonCounts(reasonCounts, objects.reasonCounts)
	evidence.RejectedReasonCounts = sortedRecoveryReasonCounts(reasonCounts)

	residue, err := scanRecoveryResidue(ctx, store.root, normalized.MaxObjects)
	if err != nil {
		return RecoveryEvidence{}, err
	}
	evidence.ResidueEntryCount = residue.count
	evidence.ResidueEntryBytes = residue.bytes
	if !residue.complete {
		evidence.CoverageComplete = false
		warnings.add(RecoveryWarningScanLimited)
	}
	if residue.issue || residue.count > 0 {
		warnings.add(RecoveryWarningResidue)
	}
	events = append(events, residue.events...)

	if descriptorBefore.valid {
		for digest, object := range objects.metadataObjects {
			object.References = manifests.references[digest]
			objects.metadataObjects[digest] = object
		}
		rebuilt := buildIndex(descriptorBefore.descriptor, manifests.indexManifests, objects.metadataObjects)
		persisted, loadErr := loadIndex(store.root, descriptorBefore.descriptor)
		if loadErr != nil || !indexesEquivalent(persisted, rebuilt) {
			evidence.DerivedStateIssueCount = 1
			warnings.add(RecoveryWarningDerivedState)
		}
	}

	descriptorAfter := inspectRecoveryDescriptor(store.root)
	if descriptorAfter != descriptorBefore {
		return RecoveryEvidence{}, operation.New(operation.KindConflict, "backup store descriptor changed during recovery scan")
	}
	if err := store.validateIdentity(); err != nil {
		return RecoveryEvidence{}, err
	}
	if err := ctx.Err(); err != nil {
		return RecoveryEvidence{}, operation.Wrap(operation.KindCancelled, "scan_backup_recovery", "", err)
	}

	sort.Slice(evidence.TrustedRecords, func(i, j int) bool {
		return evidence.TrustedRecords[i].Manifest.BackupID < evidence.TrustedRecords[j].Manifest.BackupID
	})
	sort.Slice(evidence.RejectedRecords, func(i, j int) bool {
		return evidence.RejectedRecords[i].BackupID < evidence.RejectedRecords[j].BackupID
	})
	evidence.WarningCodes = warnings.sorted()
	if !evidence.CoverageComplete {
		events = append(events, "coverage|limited")
	} else {
		events = append(events, "coverage|complete")
	}
	evidence.EvidenceDigest = digestRecoveryEvidenceEvents(events)
	return evidence, nil
}

func classifyRecoveryRecords(evidence *RecoveryEvidence, candidates []recoveryManifestCandidate, objects map[string]recoveryObjectState) {
	trustedObjects := make(map[string]int64)
	for _, candidate := range candidates {
		manifest := candidate.manifest
		object, exists := objects[manifest.ObjectDigest]
		if !exists {
			evidence.RejectedRecordCount++
			evidence.RejectedRecords = append(evidence.RejectedRecords, RecoveryRejectedRecord{BackupID: manifest.BackupID, Reason: RecoveryRejectObjectMissing})
			continue
		}
		if !object.trusted {
			evidence.RejectedRecordCount++
			reason := object.reason
			if reason == "" {
				reason = RecoveryRejectObjectInvalid
			}
			evidence.RejectedRecords = append(evidence.RejectedRecords, RecoveryRejectedRecord{BackupID: manifest.BackupID, Reason: reason})
			continue
		}
		if object.bytes != manifest.ObjectBytes {
			evidence.RejectedRecordCount++
			evidence.RejectedRecords = append(evidence.RejectedRecords, RecoveryRejectedRecord{BackupID: manifest.BackupID, Reason: RecoveryRejectManifestObjectInvalid})
			continue
		}
		evidence.TrustedRecords = append(evidence.TrustedRecords, RecoveryTrustedRecord{
			Manifest: manifest, manifestPath: candidate.path, objectPath: object.path,
		})
		trustedObjects[object.digest] = object.bytes
	}
	for _, bytes := range trustedObjects {
		evidence.TrustedObjectCount++
		evidence.TrustedBytes += bytes
	}
}

func mergeRecoveryReasonCounts(destination, source map[RecoveryRejectReason]int) {
	for reason, count := range source {
		destination[reason] += count
	}
}

func sortedRecoveryReasonCounts(counts map[RecoveryRejectReason]int) []RecoveryReasonCount {
	result := make([]RecoveryReasonCount, 0, len(counts))
	for reason, count := range counts {
		if count > 0 {
			result = append(result, RecoveryReasonCount{Reason: reason, Count: count})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Reason < result[j].Reason })
	return result
}

func digestRecoveryEvidenceEvents(events []string) string {
	sort.Strings(events)
	hasher := sha256.New()
	var length [8]byte
	for _, event := range events {
		binary.BigEndian.PutUint64(length[:], uint64(len(event)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write([]byte(event))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}
