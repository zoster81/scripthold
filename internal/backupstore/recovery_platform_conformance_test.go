package backupstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoveryPlatformUnicodeLongPathAndMetadataRoundTrip(t *testing.T) {
	base := canonicalTempDir(t)
	deep := base
	for index := 0; index < 4; index++ {
		deep = filepath.Join(deep, fmt.Sprintf("long-recovery-component-%d-%s", index, strings.Repeat("x", 45)))
		if err := os.Mkdir(deep, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	unicodeDirectory := filepath.Join(deep, "dati-Ω-資料-é")
	if err := os.Mkdir(unicodeDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(unicodeDirectory, "sorgente-Привет-漢字.txt")
	content := []byte("Unicode recovery payload: Καλημέρα 世界")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(deep, "backup-store-資料")
	store, err := Open(Options{Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	label := "Etichetta Ω — 東京 — café"
	capture, err := store.Capture(context.Background(), CaptureRequest{
		TargetPath: target, SourceOperation: SourceOperationPatchPackage, Label: label, Pinned: true,
	})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before := snapshotDiagnosticTree(t, root)

	diagnostic, evidence, plan, _, destinationPath, _, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	if len(evidence.TrustedRecords) != 1 || plan.DestinationPinnedCount != 1 {
		t.Fatalf("unicode recovery evidence=%#v plan=%#v", evidence, plan)
	}
	if _, err := diagnostic.FinalizeRecoveryDestination(context.Background(), session, plan, true); err != nil {
		t.Fatal(err)
	}
	destinationDescriptor := inspectRecoveryDescriptor(destinationPath)
	if !destinationDescriptor.valid {
		t.Fatalf("promoted unicode destination descriptor=%#v", destinationDescriptor)
	}
	manifestPathValue := manifestPath(destinationPath, capture.Manifest.BackupID)
	manifestInfo, err := os.Lstat(manifestPathValue)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := readManifest(manifestPathValue, manifestInfo, destinationDescriptor.descriptor)
	if err != nil {
		t.Fatal(err)
	}
	expected := capture.Manifest
	expected.StoreID = destinationDescriptor.descriptor.StoreID
	expected.ManifestChecksum = ""
	expected, err = finalizeManifestChecksum(expected)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != expected || recovered.TargetPath != target || recovered.Label != label || !recovered.Pinned {
		t.Fatalf("unicode/long-path manifest mismatch:\nrecovered=%#v\nexpected=%#v", recovered, expected)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestRecoveryPlatformRejectsHardLinkedAuthoritativeSourceRecordsWithoutMutation(t *testing.T) {
	for _, kind := range []string{"object", "manifest"} {
		t.Run(kind, func(t *testing.T) {
			root, manifests := createRecoveryScanStore(t, []byte("hard-linked authoritative recovery record"))
			var source string
			switch kind {
			case "object":
				source = objectPath(root, manifests[0].ObjectDigest)
			case "manifest":
				source = manifestPath(root, manifests[0].BackupID)
			default:
				t.Fatal("invalid test kind")
			}
			alias := filepath.Join(filepath.Dir(root), "external-"+kind+"-hardlink")
			if err := os.Link(source, alias); err != nil {
				t.Skipf("hard links unavailable: %v", err)
			}
			before := snapshotDiagnosticTree(t, root)
			diagnostic := openRecoveryDiagnosticStore(t, root)
			evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
			if err != nil {
				t.Fatal(err)
			}
			if len(evidence.TrustedRecords) != 0 || evidence.RejectedRecordCount == 0 {
				t.Fatalf("hard-linked %s became authoritative recovery evidence: %#v", kind, evidence)
			}
			plan, err := BuildRecoveryPlan(evidence)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Actions) != 0 || !plan.HasOmissions {
				t.Fatalf("hard-linked %s became a recovery action: %#v", kind, plan)
			}
			if err := diagnostic.Close(); err != nil {
				t.Fatal(err)
			}
			assertDiagnosticTreeUnchanged(t, root, before)
		})
	}
}

func TestRecoveryPlatformUnsupportedDescriptorStaysNonApplicableAndMutationFree(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("unsupported descriptor evidence"))
	descriptorPath := filepath.Join(root, "store.json")
	data, err := os.ReadFile(descriptorPath)
	if err != nil {
		t.Fatal(err)
	}
	var descriptor Descriptor
	if err := json.Unmarshal(data, &descriptor); err != nil {
		t.Fatal(err)
	}
	descriptor.FormatVersion = "backup-store-v999"
	data, err = json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(descriptorPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(descriptorPath, false); err != nil {
		t.Fatal(err)
	}
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.DescriptorValid || len(evidence.TrustedRecords) != 0 || !containsRecoveryWarning(evidence.WarningCodes, RecoveryWarningDescriptorInvalid) {
		t.Fatalf("unsupported descriptor became trusted: %#v", evidence)
	}
	plan, err := BuildRecoveryPlan(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable || plan.SourceStoreID != "" {
		t.Fatalf("unsupported descriptor produced applicable recovery plan: %#v", plan)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestRecoveryPlatformOversizedPlanRejectedBeforeDecodeWithoutSourceMutation(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("oversized plan source"))
	before := snapshotDiagnosticTree(t, root)
	planPath := filepath.Join(filepath.Dir(root), "oversized-recovery-plan.json")
	file, err := os.OpenFile(planPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(int64(maxRecoveryPlanBytes) + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(planPath, false); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRecoveryPlan(planPath); err == nil {
		t.Fatal("oversized recovery plan was decoded")
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestRecoveryPlatformOrphanNamespaceLimitIsExplicitAndMutationFree(t *testing.T) {
	base := canonicalTempDir(t)
	root := filepath.Join(base, "orphan-namespace-store")
	store, err := Open(Options{Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 40; index++ {
		content := []byte(fmt.Sprintf("orphan-object-%03d", index))
		digestBytes := sha256.Sum256(content)
		digest := hex.EncodeToString(digestBytes[:])
		path := objectPath(root, digest)
		if err := ensureDirectory(filepath.Dir(path)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := restrictPathPermissions(path, false); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{
		MaxManifests: 8,
		MaxObjects:   8,
		MaxBytes:     1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.CoverageComplete || !containsRecoveryWarning(evidence.WarningCodes, RecoveryWarningScanLimited) {
		t.Fatalf("bounded orphan namespace was reported complete: %#v", evidence)
	}
	plan, err := BuildRecoveryPlan(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable {
		t.Fatalf("limited orphan namespace produced applicable plan: %#v", plan)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}
