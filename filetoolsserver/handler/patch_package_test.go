package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

func TestHandlePatchPackageInspectAndDryRun(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.txt")
	secondPath := filepath.Join(root, "second.data")
	firstOriginal := []byte("alpha\nbeta\n")
	secondOriginal := encodeUTF16LEWithBOM(t, "one\r\ntwo\r\n")
	if err := os.WriteFile(firstPath, firstOriginal, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, secondOriginal, 0644); err != nil {
		t.Fatal(err)
	}

	manifest := PatchPackageManifest{
		FormatVersion:        PatchPackageFormatV1,
		Label:                "two-file update",
		FingerprintAlgorithm: "sha256",
		FingerprintMode:      "content-v1",
		Targets: []PatchPackageTarget{
			{
				Path:                firstPath,
				ExpectedFingerprint: fingerprintRegularFileForTest(t, firstPath),
				Edits:               []EditOperation{{OldText: "alpha", NewText: "omega"}},
			},
			{
				Path:                secondPath,
				ExpectedFingerprint: fingerprintRegularFileForTest(t, secondPath),
				Encoding:            "utf-16-le",
				Patch:               "--- a/second.data\n+++ b/second.data\n@@ -1,2 +1,2 @@\n one\n-two\n+three\n",
			},
		},
	}
	h := NewHandler([]string{root})

	inspectResult, inspected, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{
		Action:   "inspect",
		Manifest: manifest,
	})
	if err != nil || inspectResult.IsError {
		t.Fatalf("inspect result=%+v output=%+v err=%v", inspectResult, inspected, err)
	}
	if inspected.Action != "inspect" || inspected.TargetCount != 2 || len(inspected.Results) != 2 {
		t.Fatalf("unexpected inspect output: %+v", inspected)
	}
	if inspected.Results[0].Path != firstPath || inspected.Results[1].Path != secondPath {
		t.Fatalf("inspect order changed: %+v", inspected.Results)
	}
	assertFileBytes(t, firstPath, firstOriginal)
	assertFileBytes(t, secondPath, secondOriginal)

	dryRunResult, dryRun, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{
		Action:   "dryRun",
		Manifest: manifest,
	})
	if err != nil || dryRunResult.IsError {
		t.Fatalf("dryRun result=%+v output=%+v text=%q err=%v", dryRunResult, dryRun, extractTextFromResultRead(dryRunResult.Content), err)
	}
	if dryRun.Action != "dryRun" || dryRun.TargetCount != 2 || dryRun.ChangedCount != 2 || dryRun.UnchangedCount != 0 {
		t.Fatalf("unexpected dryRun output: %+v", dryRun)
	}
	if len(dryRun.AggregateBeforeFingerprint) != 64 || len(dryRun.AggregateAfterFingerprint) != 64 {
		t.Fatalf("aggregate fingerprints missing: %+v", dryRun)
	}
	if dryRun.AggregateBeforeFingerprint == dryRun.AggregateAfterFingerprint {
		t.Fatal("expected aggregate fingerprint to change")
	}
	if len(dryRun.Results) != 2 || !strings.Contains(dryRun.Results[0].Diff, "-alpha") || !strings.Contains(dryRun.Results[0].Diff, "+omega") {
		t.Fatalf("first target diff missing: %+v", dryRun.Results)
	}
	second := dryRun.Results[1]
	if second.Encoding != "utf-16-le" || !second.HasBOM || second.BOMType != "utf-16-le" || second.LineEndingStyle != LineEndingCRLF {
		t.Fatalf("second target metadata not preserved: %+v", second)
	}
	if !strings.Contains(second.Diff, "-two") || !strings.Contains(second.Diff, "+three") {
		t.Fatalf("second target diff missing: %+v", second)
	}
	assertFileBytes(t, firstPath, firstOriginal)
	assertFileBytes(t, secondPath, secondOriginal)
}

func TestPatchPackageInputRejectsUnknownJSONFields(t *testing.T) {
	tests := []string{
		`{"action":"inspect","manifest":{"formatVersion":"patch-package-v1","fingerprintAlgorithm":"sha256","fingerprintMode":"content-v1","targets":[]},"unknown":true}`,
		`{"action":"inspect","manifest":{"formatVersion":"patch-package-v1","fingerprintAlgorithm":"sha256","fingerprintMode":"content-v1","targets":[],"unknown":true}}`,
		`{"action":"inspect","manifest":{"formatVersion":"patch-package-v1","fingerprintAlgorithm":"sha256","fingerprintMode":"content-v1","targets":[{"path":"x","expectedFingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","edits":[{"oldText":"a","newText":"b"}],"unknown":true}]}}`,
	}
	for _, raw := range tests {
		var input PatchPackageInput
		if err := json.Unmarshal([]byte(raw), &input); err == nil {
			t.Fatalf("expected unknown field rejection for %s", raw)
		}
	}
}

func TestHandlePatchPackageRejectsInvalidManifest(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	fingerprint := fingerprintRegularFileForTest(t, path)
	validTarget := PatchPackageTarget{
		Path:                path,
		ExpectedFingerprint: fingerprint,
		Edits:               []EditOperation{{OldText: "alpha", NewText: "omega"}},
	}
	valid := PatchPackageManifest{
		FormatVersion:        PatchPackageFormatV1,
		FingerprintAlgorithm: "sha256",
		FingerprintMode:      "content-v1",
		Targets:              []PatchPackageTarget{validTarget},
	}

	tests := []struct {
		name     string
		action   string
		manifest PatchPackageManifest
	}{
		{name: "unknown action", action: "apply", manifest: valid},
		{name: "unsupported version", action: "inspect", manifest: withPatchManifest(valid, func(m *PatchPackageManifest) { m.FormatVersion = "patch-package-v2" })},
		{name: "unsupported algorithm", action: "inspect", manifest: withPatchManifest(valid, func(m *PatchPackageManifest) { m.FingerprintAlgorithm = "md5" })},
		{name: "unsupported mode", action: "inspect", manifest: withPatchManifest(valid, func(m *PatchPackageManifest) { m.FingerprintMode = "metadata-v1" })},
		{name: "no targets", action: "inspect", manifest: withPatchManifest(valid, func(m *PatchPackageManifest) { m.Targets = nil })},
		{name: "bad fingerprint", action: "inspect", manifest: withPatchManifest(valid, func(m *PatchPackageManifest) { m.Targets[0].ExpectedFingerprint = "bad" })},
		{name: "missing edit form", action: "inspect", manifest: withPatchManifest(valid, func(m *PatchPackageManifest) { m.Targets[0].Edits = nil })},
		{name: "both edit forms", action: "inspect", manifest: withPatchManifest(valid, func(m *PatchPackageManifest) {
			m.Targets[0].Patch = "--- a/target.txt\n+++ b/target.txt\n@@ -1 +1 @@\n-alpha\n+omega\n"
		})},
		{name: "empty old text", action: "inspect", manifest: withPatchManifest(valid, func(m *PatchPackageManifest) { m.Targets[0].Edits[0].OldText = "" })},
		{name: "invalid similarity", action: "inspect", manifest: withPatchManifest(valid, func(m *PatchPackageManifest) { value := 0.1; m.Targets[0].Edits[0].Similarity = &value })},
		{name: "creation patch", action: "inspect", manifest: withPatchManifest(valid, func(m *PatchPackageManifest) {
			m.Targets[0].Edits = nil
			m.Targets[0].Patch = "--- /dev/null\n+++ b/target.txt\n@@ -0,0 +1 @@\n+alpha\n"
		})},
	}

	h := NewHandler([]string{root})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: test.action, Manifest: test.manifest})
			if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestHandlePatchPackageRejectsDuplicateAndAliasedTargets(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	alias := filepath.Join(root, "alias.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, alias); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	fingerprint := fingerprintRegularFileForTest(t, path)
	makeTarget := func(targetPath string) PatchPackageTarget {
		return PatchPackageTarget{Path: targetPath, ExpectedFingerprint: fingerprint, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}}
	}
	h := NewHandler([]string{root})
	for _, targets := range [][]PatchPackageTarget{
		{makeTarget(path), makeTarget(path)},
		{makeTarget(path), makeTarget(alias)},
	} {
		result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{
			Action: "inspect",
			Manifest: PatchPackageManifest{
				FormatVersion: PatchPackageFormatV1, FingerprintAlgorithm: "sha256", FingerprintMode: "content-v1", Targets: targets,
			},
		})
		if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
}

func TestHandlePatchPackageDryRunRejectsStaleAndChangedState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	manifest := PatchPackageManifest{
		FormatVersion: PatchPackageFormatV1, FingerprintAlgorithm: "sha256", FingerprintMode: "content-v1",
		Targets: []PatchPackageTarget{{
			Path: path, ExpectedFingerprint: strings.Repeat("0", 64), Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
		}},
	}
	result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: "dryRun", Manifest: manifest})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale result=%+v err=%v", result, err)
	}

	manifest.Targets[0].ExpectedFingerprint = fingerprintRegularFileForTest(t, path)
	targets, err := h.validatePatchPackageManifest(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := openPatchPackageIdentities(targets)
	if err != nil {
		t.Fatal(err)
	}
	defer closePatchPackageIdentities(identities)
	before, err := h.capturePatchPackageFingerprints(context.Background(), targets)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := h.verifyPatchPackageDryRunSnapshot(context.Background(), targets, identities, before); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("changed verification error=%v, want CONFLICT", err)
	}
}

func TestHandlePatchPackageDryRunRejectsSameContentPathReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	displaced := filepath.Join(root, "displaced.txt")
	original := []byte("alpha")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	manifest := PatchPackageManifest{
		FormatVersion: PatchPackageFormatV1, FingerprintAlgorithm: "sha256", FingerprintMode: "content-v1",
		Targets: []PatchPackageTarget{{
			Path: path, ExpectedFingerprint: fingerprintRegularFileForTest(t, path),
			Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
		}},
	}
	h := NewHandler([]string{root})
	targets, err := h.validatePatchPackageManifest(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := openPatchPackageIdentities(targets)
	if err != nil {
		t.Fatal(err)
	}
	defer closePatchPackageIdentities(identities)
	before, err := h.capturePatchPackageFingerprints(context.Background(), targets)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, displaced); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := h.verifyPatchPackageDryRunSnapshot(context.Background(), targets, identities, before); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("same-content replacement verification error=%v, want CONFLICT", err)
	}
}

func TestHandlePatchPackageLimitsAndCancellation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	fingerprint := fingerprintRegularFileForTest(t, path)
	manifest := PatchPackageManifest{
		FormatVersion: PatchPackageFormatV1, FingerprintAlgorithm: "sha256", FingerprintMode: "content-v1",
		Targets: []PatchPackageTarget{{Path: path, ExpectedFingerprint: fingerprint, Edits: []EditOperation{{OldText: "alpha", NewText: strings.Repeat("x", 128)}}}},
	}

	tests := []struct {
		name   string
		limits config.Limits
		action string
		want   string
	}{
		{name: "manifest bytes", action: "inspect", want: ErrCodeLimit, limits: config.Limits{MaxPatchPackageBytes: 32}},
		{name: "prepared bytes", action: "dryRun", want: ErrCodeLimit, limits: config.Limits{MaxPatchPackageBytes: 1 << 20, MaxPatchPackagePreparedBytes: 32}},
		{name: "output bytes", action: "dryRun", want: ErrCodeLimit, limits: config.Limits{MaxPatchPackageBytes: 1 << 20, MaxPatchPackagePreparedBytes: 1 << 20, MaxOutputBytes: 64}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			limits := test.limits
			limits.MaxFileBytes = config.DefaultMaxFileBytes
			limits.MaxBatchFiles = config.DefaultMaxBatchFiles
			h := NewHandler([]string{root}, WithConfig(&config.Config{DefaultEncoding: "utf-8", Limits: limits}))
			result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: test.action, Manifest: manifest})
			if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != test.want {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := NewHandler([]string{root})
	result, _, err := h.HandlePatchPackage(ctx, nil, PatchPackageInput{Action: "dryRun", Manifest: manifest})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
		t.Fatalf("cancel result=%+v err=%v", result, err)
	}
}

func TestHandlePatchPackageRejectsPathOutsideAllowedRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside-patch-package.txt")
	if err := os.WriteFile(outside, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	h := NewHandler([]string{root})
	result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{
		Action: "inspect",
		Manifest: PatchPackageManifest{
			FormatVersion: PatchPackageFormatV1, FingerprintAlgorithm: "sha256", FingerprintMode: "content-v1",
			Targets: []PatchPackageTarget{{Path: outside, ExpectedFingerprint: strings.Repeat("0", 64), Edits: []EditOperation{{OldText: "a", NewText: "b"}}}},
		},
	})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeAccessDenied {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func fingerprintRegularFileForTest(t *testing.T, path string) string {
	t.Helper()
	snapshot, err := filesystem.CaptureSnapshotWithDigest(path)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := filesystem.FingerprintRegularFileSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("file %s changed\ngot:  % X\nwant: % X", path, got, want)
	}
}

func withPatchManifest(source PatchPackageManifest, mutate func(*PatchPackageManifest)) PatchPackageManifest {
	copy := source
	copy.Targets = append([]PatchPackageTarget(nil), source.Targets...)
	for index := range copy.Targets {
		copy.Targets[index].Edits = append([]EditOperation(nil), source.Targets[index].Edits...)
	}
	mutate(&copy)
	return copy
}
