package taskstore

import (
	"bytes"
	"context"
	"fmt"
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
		if len(args) != 5 {
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
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				go func() {
					_ = awaitRestartTestMarker(args[4], 90*time.Second)
					cancel()
				}()
				if args[0] == "worker" {
					var worker *Worker
					worker, err = NewWorker(store, os.Args[0], []string{args[2]}, WorkerPolicy{}, nil)
					if err == nil {
						err = worker.Run(ctx)
					}
				} else {
					err = RunSupervisor(ctx, store, os.Args[0], []string{args[2]}, nil)
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

func waitRestartTestLock(t *testing.T, lockPath string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		lock, err := tryAcquireWorkerLock(lockPath)
		if err != nil {
			if !strings.Contains(err.Error(), "already running") {
				t.Fatalf("unexpected worker lock error: %v", err)
			}
			return // A different process holds the exclusive lock.
		}
		_ = lock.close()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("helper never acquired exclusive lock %q", filepath.Base(lockPath))
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
		t.Cleanup(func() { _ = os.WriteFile(release, []byte("release"), 0o600) })
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
		if err := awaitRestartTestMarker(started, 5*time.Second); err != nil {
			t.Fatal(err)
		}
		live = append(live, liveTask{request: request, id: admitted.Task.ID, release: release, exec: executor})
	}

	for _, concurrency := range []int{8, 12, 1, 4} {
		stopWorker := filepath.Join(base, fmt.Sprintf("worker-stop-%d", concurrency))
		worker := startRestartTestProcess(t, "worker", store.root, public, strconv.Itoa(concurrency), stopWorker)
		waitRestartTestLock(t, filepath.Join(store.root, "worker.lock"))

		stopSupervisor := filepath.Join(base, fmt.Sprintf("supervisor-stop-%d", concurrency))
		supervisor := startRestartTestProcess(t, "supervisor", store.root, public, strconv.Itoa(concurrency), stopSupervisor)
		waitRestartTestLock(t, filepath.Join(store.root, "supervisor.lock"))

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
