package backupstore

import (
	"bytes"
	"strings"
	"testing"
)

func testRecoveryPlan(t *testing.T) RecoveryPlan {
	t.Helper()
	plan, err := FinalizeRecoveryPlan(RecoveryPlan{
		FormatVersion:         RecoveryPlanFormatVersion,
		SourceStoreID:         strings.Repeat("a", 64),
		SourceFormatVersion:   FormatVersion,
		DescriptorFingerprint: strings.Repeat("b", 64),
		EvidenceDigest:        strings.Repeat("c", 64),
		Bounds:                RecoveryBounds{MaxManifests: 2, MaxObjects: 2, MaxBytes: 12},
		CoverageComplete:      true,
		TrustedRecordCount:    1, TrustedObjectCount: 1, TrustedBytes: 5,
		DestinationManifestCount: 1, DestinationObjectCount: 1, DestinationBytes: 5,
		Actions: []RecoveryAction{{
			BackupID:         strings.Repeat("1", 64),
			ManifestChecksum: strings.Repeat("2", 64),
			ObjectDigest:     strings.Repeat("3", 64),
			ObjectBytes:      5,
		}},
		RejectedRecords: []RecoveryRejectedRecord{{
			BackupID: strings.Repeat("4", 64), Reason: RecoveryRejectObjectMissing,
		}},
		RejectedReasonCounts: []RecoveryReasonCount{{Reason: RecoveryRejectManifestInvalid, Count: 2}},
		RejectedRecordCount:  1,
		Applicable:           true,
		HasOmissions:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestRecoveryPlanCodecStrictCanonicalAndPathFree(t *testing.T) {
	plan := testRecoveryPlan(t)
	compact, err := EncodeRecoveryPlan(plan, false)
	if err != nil {
		t.Fatal(err)
	}
	pretty, err := EncodeRecoveryPlan(plan, true)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(compact, pretty) {
		t.Fatal("pretty encoding not distinct")
	}
	if bytes.Contains(compact, []byte("targetPath")) || bytes.Contains(compact, []byte(`C:\`)) {
		t.Fatalf("plan exposed path data: %s", compact)
	}
	for _, data := range [][]byte{compact, pretty} {
		got, err := DecodeRecoveryPlan(data)
		if err != nil {
			t.Fatal(err)
		}
		if got.PlanID != plan.PlanID || got.EvidenceDigest != plan.EvidenceDigest {
			t.Fatalf("roundtrip mismatch: %#v", got)
		}
	}
	again, err := FinalizeRecoveryPlan(plan)
	if err != nil || again.PlanID != plan.PlanID {
		t.Fatalf("plan id unstable: %#v %v", again, err)
	}

	duplicate := bytes.Replace(compact, []byte(`"formatVersion":`), []byte(`"formatVersion":"backup-recovery-plan-v1","formatVersion":`), 1)
	for name, data := range map[string][]byte{
		"duplicate": duplicate,
		"trailing":  append(append([]byte(nil), compact...), []byte(`{}`)...),
		"unknown":   append([]byte(`{"unknown":true,`), compact[1:]...),
	} {
		if _, err := DecodeRecoveryPlan(data); err == nil {
			t.Fatalf("%s input accepted", name)
		}
	}

	bad := plan
	bad.PlanID = ""
	bad.TrustedRecordCount++
	if _, err := FinalizeRecoveryPlan(bad); err == nil {
		t.Fatal("inconsistent counts accepted")
	}
	bad = plan
	bad.PlanID = ""
	bad.RejectedReasonCounts[0].Count = 0
	if _, err := FinalizeRecoveryPlan(bad); err == nil {
		t.Fatal("invalid rejection reason count accepted")
	}
	bad = plan
	bad.PlanID = ""
	bad.CoverageComplete = false
	if _, err := FinalizeRecoveryPlan(bad); err == nil {
		t.Fatal("limited applicable plan accepted")
	}
	bad = plan
	bad.PlanID = ""
	bad.DestinationPinnedCount = bad.DestinationManifestCount + 1
	if _, err := FinalizeRecoveryPlan(bad); err == nil {
		t.Fatal("invalid destination pinned count accepted")
	}
}

func TestRecoveryStateAndReportStrict(t *testing.T) {
	state := RecoveryState{
		FormatVersion: RecoveryStateFormatVersion,
		PlanID:        strings.Repeat("a", 64), DestinationKey: strings.Repeat("b", 64),
		DestinationStoreID: strings.Repeat("c", 64), Phase: RecoveryPhaseBuilding,
	}
	data, err := EncodeRecoveryState(state)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeRecoveryState(data); err != nil || got != state {
		t.Fatalf("state=%#v err=%v", got, err)
	}
	dup := bytes.Replace(data, []byte(`"phase":`), []byte(`"phase":"building","phase":`), 1)
	if _, err := DecodeRecoveryState(dup); err == nil {
		t.Fatal("duplicate state field accepted")
	}

	report := RecoveryReport{
		FormatVersion: RecoveryReportFormatVersion,
		PlanID:        strings.Repeat("d", 64), DestinationStoreID: strings.Repeat("e", 64),
		Status: RecoveryStatusRecoveredWithOmissions, Generation: strings.Repeat("f", 64),
		ManifestCount: 1, ObjectCount: 1, TotalObjectBytes: 5, OmittedRecordCount: 1,
		FullAudit: true,
	}
	data, err = EncodeRecoveryReport(report, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeRecoveryReport(data); err != nil || got != report {
		t.Fatalf("report=%#v err=%v", got, err)
	}
	badReport := bytes.Replace(data, []byte(`"fullAudit": true`), []byte(`"fullAudit": false`), 1)
	if _, err := DecodeRecoveryReport(badReport); err == nil {
		t.Fatal("report without full audit accepted")
	}
	report.PinnedCount = report.ManifestCount + 1
	if _, err := EncodeRecoveryReport(report, false); err == nil {
		t.Fatal("invalid report pinned count accepted")
	}
}

func FuzzDecodeRecoveryPlan(f *testing.F) {
	f.Add([]byte(`{"formatVersion":"backup-recovery-plan-v1"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxRecoveryPlanBytes+1 {
			t.Skip()
		}
		_, _ = DecodeRecoveryPlan(data)
	})
}

func FuzzDecodeRecoveryState(f *testing.F) {
	f.Add([]byte(`{"formatVersion":"backup-recovery-state-v1"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxRecoveryStateBytes+1 {
			t.Skip()
		}
		_, _ = DecodeRecoveryState(data)
	})
}

func FuzzDecodeRecoveryReport(f *testing.F) {
	f.Add([]byte(`{"formatVersion":"backup-recovery-report-v1"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxRecoveryReportBytes+1 {
			t.Skip()
		}
		_, _ = DecodeRecoveryReport(data)
	})
}
