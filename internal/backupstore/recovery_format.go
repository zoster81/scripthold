package backupstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

const (
	RecoveryPlanFormatVersion = "backup-recovery-plan-v1"

	maxRecoveryPlanBytes = 64 * 1024 * 1024
	maxRecoveryWarnings  = 256
)

// RecoveryBounds freezes the source-verification limits carried by a recovery plan.
type RecoveryBounds struct {
	MaxManifests int   `json:"maxManifests"`
	MaxObjects   int   `json:"maxObjects"`
	MaxBytes     int64 `json:"maxBytes"`
}

// DefaultRecoveryBounds reuses the persistent backup-store limits for equivalent dimensions.
func DefaultRecoveryBounds() RecoveryBounds {
	return RecoveryBounds{
		MaxManifests: defaultMaxManifests,
		MaxObjects:   defaultMaxManifests,
		MaxBytes:     defaultMaxTotalBytes,
	}
}

// NormalizeRecoveryBounds fills omitted values with defaults and enforces hard store ceilings.
func NormalizeRecoveryBounds(bounds RecoveryBounds) (RecoveryBounds, error) {
	defaults := DefaultRecoveryBounds()
	if bounds.MaxManifests == 0 {
		bounds.MaxManifests = defaults.MaxManifests
	}
	if bounds.MaxObjects == 0 {
		bounds.MaxObjects = defaults.MaxObjects
	}
	if bounds.MaxBytes == 0 {
		bounds.MaxBytes = defaults.MaxBytes
	}
	if bounds.MaxManifests < 1 || bounds.MaxManifests > hardMaxManifests {
		return RecoveryBounds{}, errors.New("recovery max manifests is outside the supported range")
	}
	if bounds.MaxObjects < 1 || bounds.MaxObjects > hardMaxManifests {
		return RecoveryBounds{}, errors.New("recovery max objects is outside the supported range")
	}
	if bounds.MaxBytes < 1 || bounds.MaxBytes > hardMaxTotalBytes {
		return RecoveryBounds{}, errors.New("recovery max bytes is outside the supported range")
	}
	return bounds, nil
}

// RecoveryAction identifies one trusted manifest/object pair without carrying a target path.
type RecoveryAction struct {
	BackupID         string `json:"backupId"`
	ManifestChecksum string `json:"manifestChecksum"`
	ObjectDigest     string `json:"objectDigest"`
	ObjectBytes      int64  `json:"objectBytes"`
}

// RecoveryRejectReason is a stable path-free reason for excluding authoritative evidence.
type RecoveryRejectReason string

const (
	RecoveryRejectManifestInvalid       RecoveryRejectReason = "MANIFEST_INVALID"
	RecoveryRejectObjectMissing         RecoveryRejectReason = "OBJECT_MISSING"
	RecoveryRejectObjectInvalid         RecoveryRejectReason = "OBJECT_INVALID"
	RecoveryRejectObjectDigestMismatch  RecoveryRejectReason = "OBJECT_DIGEST_MISMATCH"
	RecoveryRejectManifestObjectInvalid RecoveryRejectReason = "MANIFEST_OBJECT_MISMATCH"
)

// RecoveryRejectedRecord retains bounded trustworthy backup identity for omitted records.
type RecoveryRejectedRecord struct {
	BackupID string               `json:"backupId"`
	Reason   RecoveryRejectReason `json:"reason"`
}

// RecoveryReasonCount retains stable aggregate reasons even when no BackupID is trustworthy.
type RecoveryReasonCount struct {
	Reason RecoveryRejectReason `json:"reason"`
	Count  int                  `json:"count"`
}

// RecoveryPlan is the persisted, reviewable, non-authorizing R26 recovery plan.
type RecoveryPlan struct {
	FormatVersion         string `json:"formatVersion"`
	PlanID                string `json:"planId,omitempty"`
	SourceStoreID         string `json:"sourceStoreId,omitempty"`
	SourceFormatVersion   string `json:"sourceFormatVersion,omitempty"`
	DescriptorFingerprint string `json:"descriptorFingerprint,omitempty"`
	EvidenceDigest        string `json:"evidenceDigest"`

	Bounds           RecoveryBounds `json:"bounds"`
	CoverageComplete bool           `json:"coverageComplete"`

	TrustedRecordCount int   `json:"trustedRecordCount"`
	TrustedObjectCount int   `json:"trustedObjectCount"`
	TrustedBytes       int64 `json:"trustedBytes"`

	DestinationManifestCount int   `json:"destinationManifestCount"`
	DestinationObjectCount   int   `json:"destinationObjectCount"`
	DestinationBytes         int64 `json:"destinationBytes"`
	DestinationPinnedCount   int   `json:"destinationPinnedCount"`

	Actions              []RecoveryAction         `json:"actions"`
	RejectedRecords      []RecoveryRejectedRecord `json:"rejectedRecords"`
	RejectedReasonCounts []RecoveryReasonCount    `json:"rejectedReasonCounts"`

	RejectedRecordCount    int      `json:"rejectedRecordCount"`
	OrphanObjectCount      int      `json:"orphanObjectCount"`
	OrphanObjectBytes      int64    `json:"orphanObjectBytes"`
	UnknownEntryCount      int      `json:"unknownEntryCount"`
	ResidueEntryCount      int      `json:"residueEntryCount"`
	ResidueEntryBytes      int64    `json:"residueEntryBytes"`
	DerivedStateIssueCount int      `json:"derivedStateIssueCount"`
	WarningCodes           []string `json:"warningCodes"`

	Applicable   bool `json:"applicable"`
	HasOmissions bool `json:"hasOmissions"`
}

// FinalizeRecoveryPlan validates canonical semantics and computes its deterministic plan ID.
func FinalizeRecoveryPlan(plan RecoveryPlan) (RecoveryPlan, error) {
	plan.PlanID = ""
	normalizeRecoveryPlanSlices(&plan)
	if err := validateRecoveryPlan(plan, false); err != nil {
		return RecoveryPlan{}, err
	}
	canonical, err := json.Marshal(plan)
	if err != nil {
		return RecoveryPlan{}, errors.New("recovery plan could not be encoded")
	}
	digest := sha256.Sum256(canonical)
	plan.PlanID = hex.EncodeToString(digest[:])
	return plan, nil
}

// EncodeRecoveryPlan serializes a validated recovery plan in compact or pretty form.
func EncodeRecoveryPlan(plan RecoveryPlan, pretty bool) ([]byte, error) {
	finalized, err := FinalizeRecoveryPlan(plan)
	if err != nil {
		return nil, err
	}
	if plan.PlanID == "" || finalized.PlanID != plan.PlanID {
		return nil, errors.New("recovery plan identifier is invalid")
	}
	plan = finalized
	var data []byte
	if pretty {
		data, err = json.MarshalIndent(plan, "", "  ")
	} else {
		data, err = json.Marshal(plan)
	}
	if err != nil {
		return nil, errors.New("recovery plan could not be encoded")
	}
	data = append(data, '\n')
	if len(data) > maxRecoveryPlanBytes {
		return nil, errors.New("recovery plan exceeds the maximum encoded size")
	}
	return data, nil
}

// DecodeRecoveryPlan strictly decodes and authenticates the semantic plan ID.
func DecodeRecoveryPlan(data []byte) (RecoveryPlan, error) {
	var plan RecoveryPlan
	if err := decodeStrictRecoveryJSON(data, maxRecoveryPlanBytes, &plan); err != nil {
		return RecoveryPlan{}, err
	}
	normalizeRecoveryPlanSlices(&plan)
	if err := validateRecoveryPlan(plan, true); err != nil {
		return RecoveryPlan{}, err
	}
	finalized, err := FinalizeRecoveryPlan(plan)
	if err != nil || finalized.PlanID != plan.PlanID {
		return RecoveryPlan{}, errors.New("recovery plan identifier is invalid")
	}
	return plan, nil
}

func validateRecoveryPlan(plan RecoveryPlan, requirePlanID bool) error {
	if plan.FormatVersion != RecoveryPlanFormatVersion {
		return errors.New("recovery plan format is unsupported")
	}
	if requirePlanID && !validHexIdentifier(plan.PlanID) {
		return errors.New("recovery plan identifier is invalid")
	}
	if !requirePlanID && plan.PlanID != "" {
		return errors.New("recovery plan identifier must be empty before finalization")
	}
	if !validHexIdentifier(plan.EvidenceDigest) {
		return errors.New("recovery evidence digest is invalid")
	}
	if err := validateRecoverySourceIdentity(plan); err != nil {
		return err
	}

	normalizedBounds, err := NormalizeRecoveryBounds(plan.Bounds)
	if err != nil || normalizedBounds != plan.Bounds {
		return errors.New("recovery plan bounds are invalid")
	}
	if plan.TrustedRecordCount < 0 || plan.TrustedObjectCount < 0 || plan.TrustedBytes < 0 ||
		plan.DestinationManifestCount < 0 || plan.DestinationObjectCount < 0 || plan.DestinationBytes < 0 || plan.DestinationPinnedCount < 0 ||
		plan.RejectedRecordCount < 0 || plan.OrphanObjectCount < 0 || plan.OrphanObjectBytes < 0 ||
		plan.UnknownEntryCount < 0 || plan.ResidueEntryCount < 0 || plan.ResidueEntryBytes < 0 || plan.DerivedStateIssueCount < 0 {
		return errors.New("recovery plan counts are invalid")
	}
	if len(plan.Actions) > plan.Bounds.MaxManifests || len(plan.RejectedRecords) > plan.Bounds.MaxManifests ||
		len(plan.RejectedReasonCounts) > len(recoveryRejectReasons()) || len(plan.WarningCodes) > maxRecoveryWarnings {
		return errors.New("recovery plan retained evidence exceeds its bounds")
	}
	if plan.TrustedRecordCount != len(plan.Actions) || plan.DestinationManifestCount != len(plan.Actions) || plan.DestinationPinnedCount > plan.DestinationManifestCount || plan.RejectedRecordCount < len(plan.RejectedRecords) {
		return errors.New("recovery plan counts are inconsistent")
	}
	if plan.HasOmissions != (plan.RejectedRecordCount > 0) {
		return errors.New("recovery plan omission state is inconsistent")
	}
	if plan.Applicable && (!plan.CoverageComplete || plan.SourceStoreID == "") {
		return errors.New("incomplete recovery evidence cannot be applicable")
	}
	if err := validateRecoveryActions(plan); err != nil {
		return err
	}
	if err := validateRecoveryRejectedEvidence(plan); err != nil {
		return err
	}
	if err := validateRecoveryWarningCodes(plan.WarningCodes); err != nil {
		return err
	}
	return nil
}

func validateRecoverySourceIdentity(plan RecoveryPlan) error {
	present := 0
	if plan.SourceStoreID != "" {
		present++
	}
	if plan.SourceFormatVersion != "" {
		present++
	}
	if plan.DescriptorFingerprint != "" {
		present++
	}
	if present == 0 {
		return nil
	}
	if present != 3 || !validHexIdentifier(plan.SourceStoreID) || plan.SourceFormatVersion != FormatVersion || !validHexIdentifier(plan.DescriptorFingerprint) {
		return errors.New("recovery source identity is invalid")
	}
	return nil
}

func validateRecoveryActions(plan RecoveryPlan) error {
	objectSizes := make(map[string]int64)
	var objectBytes int64
	previousBackupID := ""
	for _, action := range plan.Actions {
		if !validHexIdentifier(action.BackupID) || !validHexIdentifier(action.ManifestChecksum) || !validHexIdentifier(action.ObjectDigest) || action.ObjectBytes < 0 || action.ObjectBytes > hardMaxObjectBytes {
			return errors.New("recovery action is invalid")
		}
		if previousBackupID != "" && action.BackupID <= previousBackupID {
			return errors.New("recovery actions are not strictly sorted")
		}
		previousBackupID = action.BackupID
		if existing, ok := objectSizes[action.ObjectDigest]; ok {
			if existing != action.ObjectBytes {
				return errors.New("recovery object evidence is inconsistent")
			}
			continue
		}
		objectSizes[action.ObjectDigest] = action.ObjectBytes
		objectBytes += action.ObjectBytes
	}
	if len(objectSizes) != plan.TrustedObjectCount || len(objectSizes) != plan.DestinationObjectCount || objectBytes != plan.TrustedBytes || objectBytes != plan.DestinationBytes {
		return errors.New("recovery object counts are inconsistent")
	}
	if len(objectSizes) > plan.Bounds.MaxObjects || objectBytes > plan.Bounds.MaxBytes {
		return errors.New("recovery trusted object evidence exceeds its bounds")
	}
	return nil
}

func validateRecoveryRejectedEvidence(plan RecoveryPlan) error {
	previousReason := ""
	for _, reasonCount := range plan.RejectedReasonCounts {
		if !validRecoveryRejectReason(reasonCount.Reason) || reasonCount.Count <= 0 || string(reasonCount.Reason) <= previousReason {
			return errors.New("recovery rejection reason counts are invalid")
		}
		previousReason = string(reasonCount.Reason)
	}

	previousBackupID := ""
	for _, record := range plan.RejectedRecords {
		if !validHexIdentifier(record.BackupID) || !validRecoveryRejectReason(record.Reason) {
			return errors.New("recovery rejected record is invalid")
		}
		if previousBackupID != "" && record.BackupID <= previousBackupID {
			return errors.New("recovery rejected records are not strictly sorted")
		}
		previousBackupID = record.BackupID
	}
	return nil
}
func recoveryRejectReasons() []RecoveryRejectReason {
	return []RecoveryRejectReason{
		RecoveryRejectManifestInvalid,
		RecoveryRejectManifestObjectInvalid,
		RecoveryRejectObjectDigestMismatch,
		RecoveryRejectObjectInvalid,
		RecoveryRejectObjectMissing,
	}
}

func validRecoveryRejectReason(reason RecoveryRejectReason) bool {
	switch reason {
	case RecoveryRejectManifestInvalid,
		RecoveryRejectObjectMissing,
		RecoveryRejectObjectInvalid,
		RecoveryRejectObjectDigestMismatch,
		RecoveryRejectManifestObjectInvalid:
		return true
	default:
		return false
	}
}

func validateRecoveryWarningCodes(codes []string) error {
	if !sort.StringsAreSorted(codes) {
		return errors.New("recovery warning codes are not sorted")
	}
	previous := ""
	for _, code := range codes {
		if code == previous || !validRecoveryCode(code) {
			return errors.New("recovery warning code is invalid")
		}
		previous = code
	}
	return nil
}

func validRecoveryCode(code string) bool {
	if code == "" || len(code) > 64 || strings.TrimSpace(code) != code {
		return false
	}
	for _, character := range code {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func normalizeRecoveryPlanSlices(plan *RecoveryPlan) {
	if plan.Actions == nil {
		plan.Actions = []RecoveryAction{}
	}
	if plan.RejectedRecords == nil {
		plan.RejectedRecords = []RecoveryRejectedRecord{}
	}
	if plan.RejectedReasonCounts == nil {
		plan.RejectedReasonCounts = []RecoveryReasonCount{}
	}
	if plan.WarningCodes == nil {
		plan.WarningCodes = []string{}
	}
}
