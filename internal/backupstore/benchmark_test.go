package backupstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const benchmarkObjectBytes = 4 * 1024

func BenchmarkBackupLifecycle(b *testing.B) {
	b.Run("preflight-batch-16", benchmarkPreflightBatch16)
	b.Run("capture-fresh", benchmarkCaptureFresh)
	b.Run("capture-deduplicated", benchmarkCaptureDeduplicated)
	b.Run("capture-pinned-fresh", benchmarkCapturePinnedFresh)
	b.Run("capture-sequential-distinct-8", benchmarkCaptureSequentialDistinct8)
	b.Run("capture-concurrent-distinct-8", benchmarkCaptureConcurrentDistinct8)
	b.Run("capture-batch-distinct-32", benchmarkCaptureBatchDistinct32)
	b.Run("capture-retention-rollover-64", benchmarkCaptureRetentionRollover64)
	b.Run("capture-batch-retention-8", benchmarkCaptureBatchRetention8)
	b.Run("plan-gc", benchmarkPlanGC)
	b.Run("apply-gc", benchmarkApplyGC)
	b.Run("open-restore-source", benchmarkOpenRestoreSource)
	b.Run("restore-read-all", benchmarkRestoreReadAll)
	b.Run("audit-quick", benchmarkAuditQuick)
	b.Run("delete-explicit", benchmarkDeleteExplicit)
}

func benchmarkPreflightBatch16(b *testing.B) {
	root := benchmarkTempDir(b, "preflight")
	defer os.RemoveAll(root)
	store := benchmarkOpenStore(b, filepath.Join(root, "store"), benchmarkLimits())
	defer store.Close()

	requests := make([]CaptureRequest, 16)
	for index := range requests {
		target := filepath.Join(root, fmt.Sprintf("target-%02d.dat", index))
		benchmarkWriteFile(b, target, benchmarkPayload(index))
		requests[index] = CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}
	}

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := store.PreflightCaptureBatch(ctx, requests); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkCaptureFresh(b *testing.B) {
	base := benchmarkTempDir(b, "capture-fresh")
	defer os.RemoveAll(base)
	payload := benchmarkPayload(1)
	ctx := context.Background()

	b.ReportAllocs()
	for index := range b.N {
		b.StopTimer()
		root := filepath.Join(base, fmt.Sprintf("case-%08d", index))
		store := benchmarkOpenStore(b, filepath.Join(root, "store"), benchmarkLimits())
		target := filepath.Join(root, "target.dat")
		benchmarkWriteFile(b, target, payload)
		b.StartTimer()

		if _, err := store.Capture(ctx, CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
		if err := os.RemoveAll(root); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}

func benchmarkCaptureDeduplicated(b *testing.B) {
	base := benchmarkTempDir(b, "capture-dedup")
	defer os.RemoveAll(base)
	payload := benchmarkPayload(2)
	ctx := context.Background()

	b.ReportAllocs()
	for index := range b.N {
		b.StopTimer()
		root := filepath.Join(base, fmt.Sprintf("case-%08d", index))
		store := benchmarkOpenStore(b, filepath.Join(root, "store"), benchmarkLimits())
		first := filepath.Join(root, "first.dat")
		second := filepath.Join(root, "second.dat")
		benchmarkWriteFile(b, first, payload)
		benchmarkWriteFile(b, second, payload)
		if _, err := store.Capture(ctx, CaptureRequest{TargetPath: first, SourceOperation: SourceOperationEdit}); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		if _, err := store.Capture(ctx, CaptureRequest{TargetPath: second, SourceOperation: SourceOperationEdit}); err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
		if err := os.RemoveAll(root); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}

func benchmarkCapturePinnedFresh(b *testing.B) {
	base := benchmarkTempDir(b, "capture-pinned")
	defer os.RemoveAll(base)
	payload := benchmarkPayload(4)
	ctx := context.Background()

	b.ReportAllocs()
	for index := range b.N {
		b.StopTimer()
		root := filepath.Join(base, fmt.Sprintf("case-%08d", index))
		store := benchmarkOpenStore(b, filepath.Join(root, "store"), benchmarkLimits())
		target := filepath.Join(root, "target.dat")
		benchmarkWriteFile(b, target, payload)
		b.StartTimer()

		if _, err := store.Capture(ctx, CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit, Pinned: true}); err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
		if err := os.RemoveAll(root); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}

func benchmarkCaptureSequentialDistinct8(b *testing.B) {
	benchmarkCaptureDistinctGroup(b, false)
}

func benchmarkCaptureConcurrentDistinct8(b *testing.B) {
	benchmarkCaptureDistinctGroup(b, true)
}

func benchmarkCaptureDistinctGroup(b *testing.B, concurrent bool) {
	base := benchmarkTempDir(b, "capture-distinct-group")
	defer os.RemoveAll(base)
	ctx := context.Background()

	b.ReportAllocs()
	for iteration := range b.N {
		b.StopTimer()
		root := filepath.Join(base, fmt.Sprintf("case-%08d", iteration))
		store := benchmarkOpenStore(b, filepath.Join(root, "store"), benchmarkLimits())
		for index := 0; index < 32; index++ {
			target := filepath.Join(root, fmt.Sprintf("baseline-%02d.dat", index))
			benchmarkWriteFile(b, target, benchmarkPayload(index))
			if _, err := store.Capture(ctx, CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
				b.Fatal(err)
			}
		}
		requests := make([]CaptureRequest, 8)
		for index := range requests {
			target := filepath.Join(root, fmt.Sprintf("target-%02d.dat", index))
			benchmarkWriteFile(b, target, benchmarkPayload(1_000+index))
			requests[index] = CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}
		}
		b.StartTimer()

		if concurrent {
			var wait sync.WaitGroup
			errorsCh := make(chan error, len(requests))
			for _, request := range requests {
				request := request
				wait.Add(1)
				go func() {
					defer wait.Done()
					_, err := store.Capture(ctx, request)
					errorsCh <- err
				}()
			}
			wait.Wait()
			close(errorsCh)
			for err := range errorsCh {
				if err != nil {
					b.Fatal(err)
				}
			}
		} else {
			for _, request := range requests {
				if _, err := store.Capture(ctx, request); err != nil {
					b.Fatal(err)
				}
			}
		}

		b.StopTimer()
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
		if err := os.RemoveAll(root); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}

func benchmarkCaptureBatchDistinct32(b *testing.B) {
	base := benchmarkTempDir(b, "capture-batch-distinct")
	defer os.RemoveAll(base)
	ctx := context.Background()

	b.ReportAllocs()
	for iteration := range b.N {
		b.StopTimer()
		root := filepath.Join(base, fmt.Sprintf("case-%08d", iteration))
		store := benchmarkOpenStore(b, filepath.Join(root, "store"), benchmarkLimits())
		for index := 0; index < 32; index++ {
			target := filepath.Join(root, fmt.Sprintf("baseline-%02d.dat", index))
			benchmarkWriteFile(b, target, benchmarkPayload(index))
			if _, err := store.Capture(ctx, CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
				b.Fatal(err)
			}
		}
		requests := make([]CaptureRequest, 32)
		for index := range requests {
			target := filepath.Join(root, fmt.Sprintf("target-%02d.dat", index))
			benchmarkWriteFile(b, target, benchmarkPayload(2_000+index))
			requests[index] = CaptureRequest{TargetPath: target, SourceOperation: SourceOperationPatchPackage}
		}
		b.StartTimer()

		if _, err := store.CaptureBatch(ctx, requests); err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
		if err := os.RemoveAll(root); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}

func benchmarkCaptureRetentionRollover64(b *testing.B) {
	root := benchmarkTempDir(b, "capture-retention")
	defer os.RemoveAll(root)
	limits := benchmarkLimits()
	limits.MaxVersionsPerTarget = 64
	store := benchmarkOpenStore(b, filepath.Join(root, "store"), limits)
	defer store.Close()
	target := filepath.Join(root, "target.dat")
	ctx := context.Background()

	for index := 0; index < limits.MaxVersionsPerTarget; index++ {
		benchmarkWriteFile(b, target, benchmarkPayload(index))
		if _, err := store.Capture(ctx, CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for index := range b.N {
		b.StopTimer()
		benchmarkWriteFile(b, target, benchmarkPayload(10_000+index))
		b.StartTimer()
		if _, err := store.Capture(ctx, CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkCaptureBatchRetention8(b *testing.B) {
	root := benchmarkTempDir(b, "capture-batch")
	defer os.RemoveAll(root)
	limits := benchmarkLimits()
	limits.MaxVersionsPerTarget = 1
	store := benchmarkOpenStore(b, filepath.Join(root, "store"), limits)
	defer store.Close()
	ctx := context.Background()
	targets := make([]string, 8)
	requests := make([]CaptureRequest, len(targets))
	for index := range targets {
		targets[index] = filepath.Join(root, fmt.Sprintf("target-%02d.dat", index))
		benchmarkWriteFile(b, targets[index], benchmarkPayload(index))
		requests[index] = CaptureRequest{TargetPath: targets[index], SourceOperation: SourceOperationEdit}
	}
	if _, err := store.CaptureBatch(ctx, requests); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for iteration := range b.N {
		b.StopTimer()
		for index := range targets {
			benchmarkWriteFile(b, targets[index], benchmarkPayload(20_000+iteration*len(targets)+index))
		}
		b.StartTimer()
		if _, err := store.CaptureBatch(ctx, requests); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkPlanGC(b *testing.B) {
	root := benchmarkTempDir(b, "plan-gc")
	defer os.RemoveAll(root)
	limits := benchmarkLimits()
	limits.MaxVersionsPerTarget = 64
	store := benchmarkOpenStore(b, filepath.Join(root, "store"), limits)
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	for targetIndex := 0; targetIndex < 8; targetIndex++ {
		target := filepath.Join(root, fmt.Sprintf("target-%02d.dat", targetIndex))
		for version := 0; version < 8; version++ {
			benchmarkWriteFile(b, target, benchmarkPayload(targetIndex*100+version))
			if _, err := store.Capture(ctx, CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := store.PlanGC(ctx, GCOptions{Now: now}); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkApplyGC(b *testing.B) {
	root := benchmarkTempDir(b, "apply-gc")
	defer os.RemoveAll(root)
	limits := benchmarkLimits()
	limits.MaxVersionsPerTarget = 64
	store := benchmarkOpenStore(b, filepath.Join(root, "store"), limits)
	defer store.Close()
	target := filepath.Join(root, "target.dat")
	ctx := context.Background()
	gcNow := time.Now().UTC().AddDate(0, 0, limits.RetentionDays+1)

	benchmarkWriteFile(b, target, benchmarkPayload(40_000))
	if _, err := store.Capture(ctx, CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for iteration := range b.N {
		b.StopTimer()
		benchmarkWriteFile(b, target, benchmarkPayload(40_001+iteration))
		if _, err := store.Capture(ctx, CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
			b.Fatal(err)
		}
		plan, err := store.PlanGC(ctx, GCOptions{Now: gcNow})
		if err != nil {
			b.Fatal(err)
		}
		if plan.ManifestCount == 0 {
			b.Fatal("GC benchmark did not produce a candidate")
		}
		b.StartTimer()

		if _, err := store.ApplyGC(ctx, plan); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkOpenRestoreSource(b *testing.B) {
	root := benchmarkTempDir(b, "restore-source")
	defer os.RemoveAll(root)
	store := benchmarkOpenStore(b, filepath.Join(root, "store"), benchmarkLimits())
	defer store.Close()
	target := filepath.Join(root, "target.dat")
	benchmarkWriteFile(b, target, benchmarkPayload(3))
	captured, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		source, err := store.OpenRestoreSource(ctx, captured.Manifest.BackupID, RestoreSourceOptions{})
		if err != nil {
			b.Fatal(err)
		}
		if err := source.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkRestoreReadAll(b *testing.B) {
	root := benchmarkTempDir(b, "restore-read")
	defer os.RemoveAll(root)
	store := benchmarkOpenStore(b, filepath.Join(root, "store"), benchmarkLimits())
	defer store.Close()
	target := filepath.Join(root, "target.dat")
	payload := benchmarkPayload(5)
	benchmarkWriteFile(b, target, payload)
	captured, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit})
	if err != nil {
		b.Fatal(err)
	}
	source, err := store.OpenRestoreSource(context.Background(), captured.Manifest.BackupID, RestoreSourceOptions{})
	if err != nil {
		b.Fatal(err)
	}
	defer source.Close()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		data, err := source.ReadAll(ctx, int64(len(payload)))
		if err != nil {
			b.Fatal(err)
		}
		if len(data) != len(payload) {
			b.Fatalf("restore read returned %d bytes, want %d", len(data), len(payload))
		}
	}
}

func benchmarkAuditQuick(b *testing.B) {
	root := benchmarkTempDir(b, "audit")
	defer os.RemoveAll(root)
	store := benchmarkOpenStore(b, filepath.Join(root, "store"), benchmarkLimits())
	defer store.Close()
	for index := 0; index < 32; index++ {
		target := filepath.Join(root, fmt.Sprintf("target-%02d.dat", index))
		benchmarkWriteFile(b, target, benchmarkPayload(index))
		if _, err := store.Capture(context.Background(), CaptureRequest{TargetPath: target, SourceOperation: SourceOperationEdit}); err != nil {
			b.Fatal(err)
		}
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := store.Audit(ctx, AuditOptions{Mode: AuditQuick}); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkDeleteExplicit(b *testing.B) {
	root := benchmarkTempDir(b, "delete")
	defer os.RemoveAll(root)
	store := benchmarkOpenStore(b, filepath.Join(root, "store"), benchmarkLimits())
	defer store.Close()
	target := filepath.Join(root, "target.dat")
	ctx := context.Background()

	b.ReportAllocs()
	for iteration := range b.N {
		b.StopTimer()
		benchmarkWriteFile(b, target, benchmarkPayload(30_000+iteration))
		captured, err := store.Capture(ctx, CaptureRequest{
			TargetPath: target, SourceOperation: SourceOperationEdit, Pinned: true,
		})
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		if _, err := store.DeleteBackup(ctx, captured.Manifest.BackupID); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkLimits() Limits {
	return Limits{
		MaxTotalBytes:        512 * 1024 * 1024,
		MaxObjectBytes:       8 * 1024 * 1024,
		MaxManifests:         10_000,
		MaxVersionsPerTarget: 64,
		MaxPinned:            1_000,
		RetentionDays:        defaultRetentionDays,
		PlanTTLSeconds:       defaultPlanTTLSeconds,
	}
}

func benchmarkPayload(seed int) []byte {
	data := make([]byte, benchmarkObjectBytes)
	for index := range data {
		data[index] = byte((seed*31 + index*17) % 251)
	}
	return data
}

func benchmarkOpenStore(b *testing.B, root string, limits Limits) *Store {
	b.Helper()
	store, err := Open(Options{Directory: root, Limits: limits})
	if err != nil {
		b.Fatal(err)
	}
	return store
}

func benchmarkTempDir(b *testing.B, prefix string) string {
	b.Helper()
	base, err := os.MkdirTemp("", "scripthold-backup-benchmark-"+prefix+"-*")
	if err != nil {
		b.Fatal(err)
	}
	return base
}

func benchmarkWriteFile(b *testing.B, path string, data []byte) {
	b.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		b.Fatal(err)
	}
}
