package backupstore

import (
	"encoding/json"
	"errors"
)

const (
	RecoveryStateFormatVersion  = "backup-recovery-state-v1"
	RecoveryReportFormatVersion = "backup-recovery-report-v1"

	maxRecoveryStateBytes  = 64 * 1024
	maxRecoveryReportBytes = 1024 * 1024
)

// RecoveryPhase records the durable apply checkpoint represented by recovery state.
type RecoveryPhase string

const (
	RecoveryPhaseBuilding RecoveryPhase = "building"
	RecoveryPhaseAudited  RecoveryPhase = "audited"
	RecoveryPhasePromoted RecoveryPhase = "promoted"
)

// RecoveryState binds recognized staging state to one exact plan and destination identity.
type RecoveryState struct {
	FormatVersion      string        `json:"formatVersion"`
	PlanID             string        `json:"planId"`
	DestinationKey     string        `json:"destinationKey"`
	DestinationStoreID string        `json:"destinationStoreId"`
	Phase              RecoveryPhase `json:"phase"`
}

// EncodeRecoveryState serializes strict owner-only recovery checkpoint state.
func EncodeRecoveryState(state RecoveryState) ([]byte, error) {
	if err := validateRecoveryState(state); err != nil {
		return nil, err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return nil, errors.New("recovery state could not be encoded")
	}
	data = append(data, '\n')
	if len(data) > maxRecoveryStateBytes {
		return nil, errors.New("recovery state exceeds the maximum encoded size")
	}
	return data, nil
}

// DecodeRecoveryState strictly decodes plan-bound recovery checkpoint state.
func DecodeRecoveryState(data []byte) (RecoveryState, error) {
	var state RecoveryState
	if err := decodeStrictRecoveryJSON(data, maxRecoveryStateBytes, &state); err != nil {
		return RecoveryState{}, err
	}
	if err := validateRecoveryState(state); err != nil {
		return RecoveryState{}, err
	}
	return state, nil
}

func validateRecoveryState(state RecoveryState) error {
	if state.FormatVersion != RecoveryStateFormatVersion || !validHexIdentifier(state.PlanID) ||
		!validHexIdentifier(state.DestinationKey) || !validHexIdentifier(state.DestinationStoreID) {
		return errors.New("recovery state identity is invalid")
	}
	switch state.Phase {
	case RecoveryPhaseBuilding, RecoveryPhaseAudited, RecoveryPhasePromoted:
		return nil
	default:
		return errors.New("recovery state phase is invalid")
	}
}

// RecoveryStatus distinguishes exact recovery from a healthy recovered subset.
type RecoveryStatus string

const (
	RecoveryStatusRecovered              RecoveryStatus = "recovered"
	RecoveryStatusRecoveredWithOmissions RecoveryStatus = "recovered_with_omissions"
)

// RecoveryReport is the mandatory path-free provenance sidecar for a completed apply.
type RecoveryReport struct {
	FormatVersion      string         `json:"formatVersion"`
	PlanID             string         `json:"planId"`
	DestinationStoreID string         `json:"destinationStoreId"`
	Status             RecoveryStatus `json:"status"`
	Generation         string         `json:"generation"`
	ManifestCount      int            `json:"manifestCount"`
	ObjectCount        int            `json:"objectCount"`
	PinnedCount        int            `json:"pinnedCount"`
	TotalObjectBytes   int64          `json:"totalObjectBytes"`
	OmittedRecordCount int            `json:"omittedRecordCount"`
	FullAudit          bool           `json:"fullAudit"`
	AuditIssueCount    int            `json:"auditIssueCount"`
}

// EncodeRecoveryReport serializes a completed recovery report in compact or pretty form.
func EncodeRecoveryReport(report RecoveryReport, pretty bool) ([]byte, error) {
	if err := validateRecoveryReport(report); err != nil {
		return nil, err
	}
	var data []byte
	var err error
	if pretty {
		data, err = json.MarshalIndent(report, "", "  ")
	} else {
		data, err = json.Marshal(report)
	}
	if err != nil {
		return nil, errors.New("recovery report could not be encoded")
	}
	data = append(data, '\n')
	if len(data) > maxRecoveryReportBytes {
		return nil, errors.New("recovery report exceeds the maximum encoded size")
	}
	return data, nil
}

// DecodeRecoveryReport strictly decodes completed recovery provenance.
func DecodeRecoveryReport(data []byte) (RecoveryReport, error) {
	var report RecoveryReport
	if err := decodeStrictRecoveryJSON(data, maxRecoveryReportBytes, &report); err != nil {
		return RecoveryReport{}, err
	}
	if err := validateRecoveryReport(report); err != nil {
		return RecoveryReport{}, err
	}
	return report, nil
}

func validateRecoveryReport(report RecoveryReport) error {
	if report.FormatVersion != RecoveryReportFormatVersion || !validHexIdentifier(report.PlanID) ||
		!validHexIdentifier(report.DestinationStoreID) || !validHexIdentifier(report.Generation) {
		return errors.New("recovery report identity is invalid")
	}
	if report.ManifestCount < 0 || report.ObjectCount < 0 || report.PinnedCount < 0 || report.PinnedCount > report.ManifestCount || report.TotalObjectBytes < 0 ||
		report.OmittedRecordCount < 0 || report.AuditIssueCount < 0 {
		return errors.New("recovery report counts are invalid")
	}
	if !report.FullAudit || report.AuditIssueCount != 0 {
		return errors.New("recovery report requires a successful full audit")
	}
	switch report.Status {
	case RecoveryStatusRecovered:
		if report.OmittedRecordCount != 0 {
			return errors.New("recovery report omission state is inconsistent")
		}
	case RecoveryStatusRecoveredWithOmissions:
		if report.OmittedRecordCount == 0 {
			return errors.New("recovery report omission state is inconsistent")
		}
	default:
		return errors.New("recovery report status is invalid")
	}
	return nil
}
