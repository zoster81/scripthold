package taskstore

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestTaskstoreConcurrencyProcessHelper runs only in a separately launched test
// process. No installed Scripthold binary or live store is involved.
func TestTaskstoreConcurrencyProcessHelper(t *testing.T) {
	if os.Getenv("SCRIPTHOLD_TASKSTORE_RESTART_CHILD") != "1" {
		return
	}
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || len(os.Args) < separator+3 {
		os.Exit(2)
	}
	args := os.Args[separator+1:]
	var err error
	switch args[0] {
	case "payload":
		if len(args) != 3 {
			os.Exit(2)
		}
		err = os.WriteFile(args[1], []byte("started"), 0o600)
		if err == nil {
			err = awaitRestartTestMarker(args[2], 90*time.Second)
		}
	case "executor":
		if len(args) != 4 {
			os.Exit(2)
		}
		limits := testLimits()
		limits.MaxConcurrency = 4
		var store *Store
		store, err = OpenExecutor(args[1], limits)
		if err == nil {
			err = RunExecutor(context.Background(), store, args[2], args[3])
		}
	case "frontend":
		if len(args) != 6 {
			os.Exit(2)
		}
		var concurrency int
		concurrency, err = strconv.Atoi(args[3])
		if err == nil {
			limits := testLimits()
			limits.MaxConcurrency = concurrency
			var store *Store
			store, err = Open(args[1], []string{args[2]}, limits)
			if err == nil {
				for _, id := range args[4:] {
					var task Task
					task, err = store.Get(context.Background(), id)
					if err == nil && (task.Status != StatusRunning || task.Revision != 3) {
						err = fmt.Errorf("frontend observed changed executor state: %s", task.Status)
					}
					if err != nil {
						break
					}
				}
			}
		}
	case "worker", "supervisor":
		if len(args) != 6 {
			os.Exit(2)
		}
		var concurrency int
		concurrency, err = strconv.Atoi(args[3])
		if err == nil {
			limits := testLimits()
			limits.MaxConcurrency = concurrency
			var store *Store
			store, err = Initialize(args[1], []string{args[2]}, limits)
			if err == nil {
				var readyLog *os.File
				readyLog, err = os.Create(args[5])
				if err == nil {
					logger := slog.New(slog.NewTextHandler(readyLog, nil))
					ctx, cancel := context.WithCancel(context.Background())
					go func() {
						_ = awaitRestartTestMarker(args[4], 90*time.Second)
						cancel()
					}()
					if args[0] == "worker" {
						var worker *Worker
						worker, err = NewWorker(store, os.Args[0], []string{args[2]}, WorkerPolicy{}, logger)
						if err == nil {
							err = worker.Run(ctx)
						}
					} else {
						err = RunSupervisor(ctx, store, os.Args[0], []string{args[2]}, logger)
					}
					cancel()
					if closeErr := readyLog.Close(); err == nil {
						err = closeErr
					}
				}
			}
		}
	default:
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	os.Exit(0)
}

func awaitRestartTestMarker(path string, maximum time.Duration) error {
	deadline := time.Now().Add(maximum)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("test process marker timed out: %s", filepath.Base(path))
}

func startRestartTestProcess(t *testing.T, mode string, args ...string) *exec.Cmd {
	t.Helper()
	argv := append([]string{"-test.run=^TestTaskstoreConcurrencyProcessHelper$", "--", mode}, args...)
	cmd := exec.Command(os.Args[0], argv...)
	cmd.Env = append(os.Environ(), "SCRIPTHOLD_TASKSTORE_RESTART_CHILD=1")
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return cmd
}

func finishRestartTestProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	completed := make(chan error, 1)
	go func() { completed <- cmd.Wait() }()
	select {
	case err := <-completed:
		if err != nil {
			t.Fatalf("helper process failed: %v, stderr=%q", err, cmd.Stderr.(*bytes.Buffer).String())
		}
	case <-time.After(8 * time.Second):
		_ = cmd.Process.Kill()
		<-completed
		t.Fatal("helper process did not terminate within eight seconds")
	}
}

// waitRestartTestLock requires both a child-specific post-lock startup log and
// the OS lock being held. Another process's lock cannot attest child readiness.
func waitRestartTestLock(child *exec.Cmd, lockPath, readyPath string, timeout time.Duration) error {
	expected := "task worker started"
	if filepath.Base(lockPath) == "supervisor.lock" {
		expected = "task supervisor started"
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		logData, err := os.ReadFile(readyPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("read helper readiness: %w", err)
		}
		if strings.Contains(string(logData), expected) {
			lock, lockErr := tryAcquireWorkerLock(lockPath)
			if lockErr != nil {
				if strings.Contains(lockErr.Error(), "already running") {
					return nil
				}
				return fmt.Errorf("unexpected worker lock error: %w", lockErr)
			}
			_ = lock.close()
			return fmt.Errorf("helper startup log was emitted without holding %q", filepath.Base(lockPath))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if child.Process != nil {
		_ = child.Process.Kill()
	}
	waitErr := child.Wait()
	return fmt.Errorf("helper did not prove exclusive lock %q: process=%v, stderr=%q",
		filepath.Base(lockPath), waitErr, child.Stderr.(*bytes.Buffer).String())
}

// A busy lock is not evidence that the newly launched helper acquired it.
func TestRestartLockReadinessRejectsUnrelatedOwner(t *testing.T) {
	base := canonicalTaskStoreTestDir(t)
	public := filepath.Join(base, "public")
	if err := os.Mkdir(public, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := Initialize(filepath.Join(base, "tasks"), []string{public}, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	held, err := tryAcquireWorkerLock(filepath.Join(store.root, "supervisor.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer held.close()
	ready := filepath.Join(base, "foreign-ready")
	child := startRestartTestProcess(t, "supervisor", store.root, public, "4", filepath.Join(base, "stop"), ready)
	if err := waitRestartTestLock(child, filepath.Join(store.root, "supervisor.lock"), ready, 800*time.Millisecond); err == nil {
		t.Fatal("unrelated lock holder falsely satisfied child readiness")
	}
	// The child may not start within 800ms on a loaded Windows host. Only
	// require rejection and absence of the child-specific post-lock record.
	if data, err := os.ReadFile(ready); err == nil && strings.Contains(string(data), "task supervisor started") {
		t.Fatal("foreign lock was incorrectly reported as acquired by the child")
	}
}

func TestConcurrencyRestartAcrossWorkerSupervisorAndActiveExecutorProcesses(t *testing.T) {
	if runtime.GOOS == "windows" && os.Getenv("CI") != "" {
		t.Skip("Windows CI process-group policies differ from local supervised runs")
	}
	base := canonicalTaskStoreTestDir(t)
	public := filepath.Join(base, "public")
	if err := os.Mkdir(public, 0o700); err != nil {
		t.Fatal(err)
	}
	limits := testLimits()
	limits.MaxConcurrency = 4
	store, err := Initialize(filepath.Join(base, "private-tasks"), []string{public}, limits)
	if err != nil {
		t.Fatal(err)
	}
	type liveTask struct {
		request Request
		id      string
		release string
		exec    *exec.Cmd
	}
	live := make([]liveTask, 0, 2)
	for i := 0; i < 2; i++ {
		request := shellRequest(fmt.Sprintf("isolated-multiprocess-restart-%d", i))
		request.WorkingDirectory = public
		admitted, err := store.Submit(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		token := testRandomHex(t, 32)
		started := filepath.Join(base, fmt.Sprintf("payload-started-%d", i))
		release := filepath.Join(base, fmt.Sprintf("payload-release-%d", i))
		launch := launchRecord{
			Program:          os.Args[0],
			Args:             []string{"-test.run=^TestTaskstoreConcurrencyProcessHelper$", "--", "payload", started, release},
			WorkingDirectory: public,
			ExecutorToken:    token,
		}
		if err := writeJSONExclusive(filepath.Join(store.taskDir(admitted.Task.ID), "launch.json"), launch); err != nil {
			t.Fatal(err)
		}
		state, err := store.latestState(admitted.Task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.appendStateUnlocked(admitted.Task.ID, stateRecord{
			Status: StatusStarting, Revision: state.Revision + 1, UpdatedAt: time.Now().UTC(), ExecutorToken: token,
		}); err != nil {
			t.Fatal(err)
		}
		executor := startRestartTestProcess(t, "executor", store.root, admitted.Task.ID, token)
		// Cleanup callbacks run last-in-first-out: release and reap the executor
		// before startRestartTestProcess's fallback process kill runs.
		t.Cleanup(func() {
			if executor.ProcessState != nil {
				return
			}
			if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
				t.Errorf("release executor payload during cleanup: %v", err)
			}
			completed := make(chan error, 1)
			go func() { completed <- executor.Wait() }()
			select {
			case err := <-completed:
				if err != nil {
					t.Errorf("executor cleanup failed: %v", err)
				}
			case <-time.After(20 * time.Second):
				_ = executor.Process.Kill()
				<-completed
				t.Error("executor cleanup did not finish within twenty seconds")
			}
		})
		if err := awaitRestartTestMarker(started, 5*time.Second); err != nil {
			t.Fatal(err)
		}
		live = append(live, liveTask{request: request, id: admitted.Task.ID, release: release, exec: executor})
	}

	for _, concurrency := range []int{8, 12, 1, 4} {
		stopWorker := filepath.Join(base, fmt.Sprintf("worker-stop-%d", concurrency))
		workerReady := filepath.Join(base, fmt.Sprintf("worker-ready-%d", concurrency))
		worker := startRestartTestProcess(t, "worker", store.root, public, strconv.Itoa(concurrency), stopWorker, workerReady)
		if err := waitRestartTestLock(worker, filepath.Join(store.root, "worker.lock"), workerReady, 20*time.Second); err != nil {
			t.Fatal(err)
		}

		stopSupervisor := filepath.Join(base, fmt.Sprintf("supervisor-stop-%d", concurrency))
		supervisorReady := filepath.Join(base, fmt.Sprintf("supervisor-ready-%d", concurrency))
		supervisor := startRestartTestProcess(t, "supervisor", store.root, public, strconv.Itoa(concurrency), stopSupervisor, supervisorReady)
		if err := waitRestartTestLock(supervisor, filepath.Join(store.root, "supervisor.lock"), supervisorReady, 20*time.Second); err != nil {
			t.Fatal(err)
		}

		frontend := startRestartTestProcess(t, "frontend", store.root, public, strconv.Itoa(concurrency), live[0].id, live[1].id)
		finishRestartTestProcess(t, frontend)
		for _, active := range live {
			observed, err := store.Get(context.Background(), active.id)
			if err != nil || observed.Status != StatusRunning || observed.Revision != 3 {
				t.Fatalf("executor changed during concurrency=%d: task=%+v, error=%v", concurrency, observed, err)
			}
		}
		if err := os.WriteFile(stopSupervisor, []byte("stop"), 0o600); err != nil {
			t.Fatal(err)
		}
		finishRestartTestProcess(t, supervisor)
		if concurrency == 12 {
			// Simulate a worker crash. Its lock must be released by the OS,
			// without terminating either independently running executor.
			if err := worker.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err := worker.Wait(); err == nil {
				t.Fatal("killed worker unexpectedly exited normally")
			}
		} else {
			if err := os.WriteFile(stopWorker, []byte("stop"), 0o600); err != nil {
				t.Fatal(err)
			}
			finishRestartTestProcess(t, worker)
		}
	}

	for _, active := range live {
		if err := os.WriteFile(active.release, []byte("release"), 0o600); err != nil {
			t.Fatal(err)
		}
		finishRestartTestProcess(t, active.exec)
		final, err := store.Get(context.Background(), active.id)
		if err != nil || final.Status != StatusSucceeded || final.Revision != 4 {
			t.Fatalf("executor did not complete exactly once: task=%+v, error=%v", final, err)
		}
		repeated, err := store.Submit(context.Background(), active.request)
		if err != nil || !repeated.Duplicated || repeated.Task.ID != active.id {
			t.Fatalf("idempotency changed: result=%+v, error=%v", repeated, err)
		}
		if strings.TrimSpace(active.exec.Stderr.(*bytes.Buffer).String()) != "" {
			t.Fatalf("unexpected executor diagnostics: %q", active.exec.Stderr.(*bytes.Buffer).String())
		}
	}
	var preserved descriptor
	if err := readJSON(filepath.Join(store.root, descriptorName), &preserved, 16*1024); err != nil {
		t.Fatal(err)
	}
	if preserved.Limits != limits || preserved.Salt != store.descriptor.Salt {
		t.Fatalf("descriptor changed across process restarts")
	}
}
