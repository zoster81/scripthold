package taskstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/execution"
	"github.com/zoster81/scripthold/internal/security"
)

func testLimits() Limits {
	return Limits{MaxConcurrency: 2, MaxQueued: 4, MaxLogBytesPerStream: 4096, MaxRuntimeSeconds: 0, RetentionDays: 7, MaxTerminal: 100, MaxTotalBytes: 32 * 1024 * 1024}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, _ := newTestStoreWithPublic(t)
	return store
}

func newTestStoreWithPublic(t *testing.T) (*Store, string) {
	t.Helper()
	base := t.TempDir()
	public := filepath.Join(base, "public")
	if err := os.Mkdir(public, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := Initialize(filepath.Join(base, "private-tasks"), []string{public}, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	return store, public
}

func shellRequest(key string) Request {
	return Request{Kind: KindShell, IdempotencyKey: key, WorkingDirectory: filepath.Clean(os.TempDir()), Command: "echo ok"}
}

func testRandomHex(t *testing.T, bytesCount int) string {
	t.Helper()
	value, err := randomHex(bytesCount)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestSubmitIsIdempotentAndRejectsConflictingReuse(t *testing.T) {
	store := newTestStore(t)
	first, err := store.Submit(context.Background(), shellRequest("same-key"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Submit(context.Background(), shellRequest("same-key"))
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicated || first.Task.ID != second.Task.ID {
		t.Fatalf("duplicate admission diverged: %#v %#v", first, second)
	}
	changed := shellRequest("same-key")
	changed.Command = "echo different"
	if _, err := store.Submit(context.Background(), changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting key error = %v", err)
	}
}

func TestSubmitRepairsIncompleteAdmissionBeforeExecution(t *testing.T) {
	store := newTestStore(t)
	request := shellRequest("repairable-key")
	admitted, err := store.Submit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(store.taskDir(admitted.Task.ID), requestName)
	if err := os.WriteFile(requestPath, []byte("{incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	repaired, err := store.Submit(context.Background(), request)
	if err != nil {
		t.Fatalf("repair admission: %v", err)
	}
	if repaired.Task.ID != admitted.Task.ID || repaired.Task.Status != StatusQueued || repaired.Duplicated {
		t.Fatalf("unexpected repaired admission: %#v", repaired)
	}
	if _, err := store.readPersistedRequest(admitted.Task.ID); err != nil {
		t.Fatalf("repaired request is unreadable: %v", err)
	}
}

func TestSubmitNeverRepairsAdmissionWithStartMarker(t *testing.T) {
	store := newTestStore(t)
	request := shellRequest("started-key")
	admitted, err := store.Submit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSONExclusive(filepath.Join(store.taskDir(admitted.Task.ID), startedName), struct{}{}); err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(store.taskDir(admitted.Task.ID), requestName)
	if err := os.WriteFile(requestPath, []byte("{incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Submit(context.Background(), request); err == nil || !strings.Contains(err.Error(), "start marker") {
		t.Fatalf("started admission repair error = %v", err)
	}
	if !fileExists(store.taskDir(admitted.Task.ID)) {
		t.Fatal("started admission was removed")
	}
}

func TestWorkerRejectsTamperedPersistedRequest(t *testing.T) {
	store, public := newTestStoreWithPublic(t)
	request := shellRequest("tampered-record")
	request.WorkingDirectory = public
	admitted, err := store.Submit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := store.readPersistedRequest(admitted.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	persisted.Request.Kind = Kind("injected")
	payload, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(store.taskDir(admitted.Task.ID), requestName)
	if err := os.WriteFile(requestPath, append(payload, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.readPersistedRequest(admitted.Task.ID); err == nil {
		t.Fatal("tampered persisted request passed validation")
	}
	worker, err := NewWorker(store, os.Args[0], []string{public}, WorkerPolicy{AllowShell: true, AllowRunScript: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err := store.latestState(admitted.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusQueued {
		t.Fatalf("tampered task state = %s, want queued and unexecuted", state.Status)
	}
}

func TestSubmitEnforcesQueueBound(t *testing.T) {
	store := newTestStore(t)
	store.limits.MaxQueued = 2
	for _, key := range []string{"one", "two"} {
		if _, err := store.Submit(context.Background(), shellRequest(key)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Submit(context.Background(), shellRequest("three")); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("queue overflow error = %v", err)
	}
}

func TestCancelFinalizesQueuedTaskWithoutWorker(t *testing.T) {
	store := newTestStore(t)
	admitted, err := store.Submit(context.Background(), shellRequest("offline-cancel"))
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.Cancel(context.Background(), admitted.Task.ID, "no longer needed")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != StatusCancelled || cancelled.Result == nil || cancelled.Result.ErrorCode != "TASK_CANCELLED" || cancelled.StartedAt != nil {
		t.Fatalf("queued cancellation = %#v", cancelled)
	}
}

func TestStoreRejectsPublicOverlap(t *testing.T) {
	root := t.TempDir()
	_, err := Initialize(filepath.Join(root, "tasks"), []string{root}, testLimits())
	if err == nil || !strings.Contains(err.Error(), "must not overlap") {
		t.Fatalf("overlap error = %v", err)
	}
}

func TestOpenRejectsChangedDurabilityPolicy(t *testing.T) {
	store := newTestStore(t)
	changed := store.limits
	changed.MaxConcurrency++
	if _, err := OpenExecutor(store.root, changed); err == nil || !strings.Contains(err.Error(), "descriptor") {
		t.Fatalf("changed policy error = %v", err)
	}
}

func TestFrontendOpenRequiresExactAllowedRootPolicy(t *testing.T) {
	store, public := newTestStoreWithPublic(t)
	if _, err := Open(store.root, []string{public}, store.limits); err != nil {
		t.Fatalf("matching policy open: %v", err)
	}
	if _, err := Open(store.root, nil, store.limits); err == nil || !strings.Contains(err.Error(), "allowed-directory policy") {
		t.Fatalf("missing policy error = %v", err)
	}
}

func TestConcurrentInitializeConvergesOnOneDescriptor(t *testing.T) {
	base := t.TempDir()
	public := filepath.Join(base, "public")
	root := filepath.Join(base, "tasks")
	if err := os.Mkdir(public, 0o700); err != nil {
		t.Fatal(err)
	}
	const count = 8
	stores := make(chan *Store, count)
	errorsSeen := make(chan error, count)
	for range count {
		go func() {
			store, err := Initialize(root, []string{public}, testLimits())
			if err != nil {
				errorsSeen <- err
				return
			}
			stores <- store
		}()
	}
	var salt string
	for range count {
		select {
		case err := <-errorsSeen:
			t.Fatalf("concurrent initialize: %v", err)
		case store := <-stores:
			if salt == "" {
				salt = store.descriptor.Salt
			} else if store.descriptor.Salt != salt {
				t.Fatalf("descriptor salts diverged: %q != %q", store.descriptor.Salt, salt)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent initialize timed out")
		}
	}
}

func TestLifetimeLockRejectsDuplicateWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worker.lock")
	first, err := tryAcquireWorkerLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.close()
	started := time.Now()
	second, err := tryAcquireWorkerLock(path)
	if second != nil {
		_ = second.close()
	}
	if err == nil {
		t.Fatal("duplicate lifetime lock unexpectedly succeeded")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatalf("duplicate lifetime lock blocked for %s", time.Since(started))
	}
}

func TestFutureHeartbeatIsNeverTreatedAsLive(t *testing.T) {
	store := newTestStore(t)
	path := filepath.Join(store.root, workerHeartbeatName)
	if err := touch(path); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if store.workerOnline() {
		t.Fatal("future-dated worker heartbeat was treated as live")
	}
}

func TestScriptSnapshotRejectsMutationAndExecutesPrivateBytes(t *testing.T) {
	store, public := newTestStoreWithPublic(t)
	scriptPath := filepath.Join(public, "job.ps1")
	original := []byte("Write-Output 'original'\n")
	if err := os.WriteFile(scriptPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(original)
	request := Request{Kind: KindScript, IdempotencyKey: "script-snapshot", WorkingDirectory: public, ScriptPath: scriptPath, ScriptDigest: hex.EncodeToString(digest[:]), ScriptSize: int64(len(original))}
	admitted, err := store.Submit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewWorker(store, os.Args[0], []string{public}, WorkerPolicy{AllowRunScript: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("Write-Output 'changed!'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.snapshotScript(admitted.Task.ID, scriptPath, request.ScriptSize, request.ScriptDigest); err == nil {
		t.Fatal("mutated script snapshot unexpectedly succeeded")
	}
	if err := os.WriteFile(scriptPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshotPath, err := worker.snapshotScript(admitted.Task.ID, scriptPath, request.ScriptSize, request.ScriptDigest)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(original) || security.PathsEqual(snapshotPath, scriptPath) {
		t.Fatalf("private snapshot mismatch: path=%q content=%q", snapshotPath, actual)
	}
}

func TestBoundedLogsRetainHeadAndTailWithAbsoluteCursors(t *testing.T) {
	store := newTestStore(t)
	admitted, err := store.Submit(context.Background(), shellRequest("logs"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := newBoundedLogWriter(store.taskDir(admitted.Task.ID), "stdout", 4096)
	if err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("H", 1024) + strings.Repeat("M", 6000) + strings.Repeat("T", 3072)
	if _, err := w.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	logs, err := store.Logs(context.Background(), admitted.Task.ID, LogOptions{LimitBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if logs.Stdout.Data != strings.Repeat("H", 1024) || logs.Stdout.DroppedBytes == 0 {
		t.Fatalf("unexpected first page: %#v", logs.Stdout)
	}
	logs, err = store.Logs(context.Background(), admitted.Task.ID, LogOptions{StdoutCursor: logs.Stdout.NextCursor, LimitBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if !logs.Stdout.Truncated || !strings.HasPrefix(logs.Stdout.Data, "T") {
		t.Fatalf("tail page did not report the gap: %#v", logs.Stdout)
	}
}

func TestExecutorCompletesAndPersistsLogs(t *testing.T) {
	store := newTestStore(t)
	admitted, err := store.Submit(context.Background(), shellRequest("executor-success"))
	if err != nil {
		t.Fatal(err)
	}
	program, args, err := execution.BuildShellCommand("", "echo durable-output")
	if err != nil {
		t.Fatal(err)
	}
	token := testRandomHex(t, 32)
	if err := writeJSONExclusive(filepath.Join(store.taskDir(admitted.Task.ID), "launch.json"), launchRecord{Program: program, Args: args, WorkingDirectory: os.TempDir(), ExecutorToken: token}); err != nil {
		t.Fatal(err)
	}
	state, _ := store.latestState(admitted.Task.ID)
	if err := store.appendStateUnlocked(admitted.Task.ID, stateRecord{Status: StatusStarting, Revision: state.Revision + 1, UpdatedAt: time.Now().UTC(), ExecutorToken: token}); err != nil {
		t.Fatal(err)
	}
	if err := RunExecutor(context.Background(), store, admitted.Task.ID, token); err != nil {
		t.Fatal(err)
	}
	task, err := store.Get(context.Background(), admitted.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusSucceeded || task.Result == nil || task.Result.ExitCode != 0 {
		t.Fatalf("unexpected terminal task: %#v", task)
	}
	logs, err := store.Logs(context.Background(), task.ID, LogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.Stdout.Data, "durable-output") {
		t.Fatalf("stdout missing: %#v", logs.Stdout)
	}
}

func TestExecutorCancellationTerminatesTask(t *testing.T) {
	if runtime.GOOS == "windows" && os.Getenv("CI") != "" {
		t.Skip("avoid host process-tree policy differences in Windows CI")
	}
	store := newTestStore(t)
	admitted, err := store.Submit(context.Background(), shellRequest("executor-cancel"))
	if err != nil {
		t.Fatal(err)
	}
	program, args, err := execution.BuildShellCommand("", longSleepCommand())
	if err != nil {
		t.Fatal(err)
	}
	token := testRandomHex(t, 32)
	if err := writeJSONExclusive(filepath.Join(store.taskDir(admitted.Task.ID), "launch.json"), launchRecord{Program: program, Args: args, WorkingDirectory: os.TempDir(), ExecutorToken: token}); err != nil {
		t.Fatal(err)
	}
	state, _ := store.latestState(admitted.Task.ID)
	if err := store.appendStateUnlocked(admitted.Task.ID, stateRecord{Status: StatusStarting, Revision: state.Revision + 1, UpdatedAt: time.Now().UTC(), ExecutorToken: token}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- RunExecutor(context.Background(), store, admitted.Task.ID, token) }()
	deadline := time.Now().Add(5 * time.Second)
	for !fileExists(filepath.Join(store.taskDir(admitted.Task.ID), heartbeatName)) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := store.Cancel(context.Background(), admitted.Task.ID, "test"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("executor did not stop after cancellation")
	}
	task, _ := store.Get(context.Background(), admitted.Task.ID)
	if task.Status != StatusCancelled {
		t.Fatalf("status = %s, want cancelled", task.Status)
	}
}

func TestExecutorHonorsExplicitRuntimeLimit(t *testing.T) {
	if runtime.GOOS == "windows" && os.Getenv("CI") != "" {
		t.Skip("avoid host process-tree policy differences in Windows CI")
	}
	store := newTestStore(t)
	admitted, err := store.Submit(context.Background(), shellRequest("executor-timeout"))
	if err != nil {
		t.Fatal(err)
	}
	program, args, err := execution.BuildShellCommand("", longSleepCommand())
	if err != nil {
		t.Fatal(err)
	}
	token := testRandomHex(t, 32)
	if err := writeJSONExclusive(filepath.Join(store.taskDir(admitted.Task.ID), "launch.json"), launchRecord{Program: program, Args: args, WorkingDirectory: os.TempDir(), MaxRuntimeSeconds: 1, ExecutorToken: token}); err != nil {
		t.Fatal(err)
	}
	state, _ := store.latestState(admitted.Task.ID)
	if err := store.appendStateUnlocked(admitted.Task.ID, stateRecord{Status: StatusStarting, Revision: state.Revision + 1, UpdatedAt: time.Now().UTC(), ExecutorToken: token}); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := RunExecutor(context.Background(), store, admitted.Task.ID, token); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > 8*time.Second {
		t.Fatal("explicit timeout did not terminate promptly")
	}
	task, err := store.Get(context.Background(), admitted.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusTimedOut || task.Result == nil || task.Result.ErrorCode != "TASK_TIMED_OUT" {
		t.Fatalf("timeout task = %#v", task)
	}
}

func TestWorkerNeverRequeuesTaskWithStartedMarker(t *testing.T) {
	store, public := newTestStoreWithPublic(t)
	request := shellRequest("stale-started")
	request.WorkingDirectory = public
	admitted, err := store.Submit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * recoveryGrace).UTC()
	if err := store.appendStateUnlocked(admitted.Task.ID, stateRecord{Status: StatusStarting, Revision: 2, UpdatedAt: old, ExecutorToken: testRandomHex(t, 32)}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONExclusive(filepath.Join(store.taskDir(admitted.Task.ID), startedName), struct{}{}); err != nil {
		t.Fatal(err)
	}
	worker, err := NewWorker(store, os.Args[0], []string{public}, WorkerPolicy{AllowShell: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	worker.startedAt = old
	worker.suspectSince[admitted.Task.ID] = old
	if err := worker.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	task, err := store.Get(context.Background(), admitted.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusInterrupted || task.Result == nil || task.Result.ErrorCode != "EXECUTOR_LOST" {
		t.Fatalf("stale started task = %#v", task)
	}
}

func TestWorkerFinalizesExecutorStartFailureWithoutDuplicateRevision(t *testing.T) {
	store, public := newTestStoreWithPublic(t)
	request := shellRequest("start-failure")
	request.WorkingDirectory = public
	admitted, err := store.Submit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	missingExecutable := filepath.Join(t.TempDir(), "missing-scripthold")
	worker, err := NewWorker(store, missingExecutable, []string{public}, WorkerPolicy{AllowShell: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	task, err := store.Get(context.Background(), admitted.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusFailed || len(task.History) != 3 || task.History[0].Revision != 1 || task.History[1].Revision != 2 || task.History[2].Revision != 3 {
		t.Fatalf("start failure task = %#v", task)
	}
}

func TestWorkerBoundsRepeatedPreStartRecovery(t *testing.T) {
	store, public := newTestStoreWithPublic(t)
	request := shellRequest("bounded-recovery")
	request.WorkingDirectory = public
	admitted, err := store.Submit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.latestState(admitted.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < maxDispatchAttempts; attempt++ {
		state = stateRecord{Status: StatusStarting, Revision: state.Revision + 1, UpdatedAt: time.Now().UTC(), ExecutorToken: testRandomHex(t, 32)}
		if err := store.appendStateUnlocked(admitted.Task.ID, state); err != nil {
			t.Fatal(err)
		}
		state = stateRecord{Status: StatusQueued, Revision: state.Revision + 1, UpdatedAt: time.Now().UTC()}
		if err := store.appendStateUnlocked(admitted.Task.ID, state); err != nil {
			t.Fatal(err)
		}
	}
	worker, err := NewWorker(store, os.Args[0], []string{public}, WorkerPolicy{AllowShell: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	task, err := store.Get(context.Background(), admitted.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusFailed || task.Result == nil || task.Result.ErrorCode != "TASK_DISPATCH_RETRIES_EXHAUSTED" || len(task.History) != 2*maxDispatchAttempts+2 {
		t.Fatalf("bounded recovery task = %#v", task)
	}
}

func longSleepCommand() string {
	if runtime.GOOS == "windows" {
		return "Start-Sleep -Seconds 30"
	}
	return "sleep 30"
}
