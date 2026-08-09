package taskstore

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// RunSupervisor keeps exactly one task worker available without owning task
// executors. Stopping or crashing the supervisor does not terminate the worker;
// starting another supervisor safely resumes monitoring through a store lock.
func RunSupervisor(ctx context.Context, store *Store, executable string, allowedDirectories []string, logger *slog.Logger) error {
	if store == nil {
		return ErrDisabled
	}
	if executable == "" || len(allowedDirectories) == 0 {
		return errors.New("task supervisor requires an executable and allowed directories")
	}
	if logger == nil {
		logger = slog.Default()
	}
	lock, err := tryAcquireWorkerLock(filepath.Join(store.root, "supervisor.lock"))
	if err != nil {
		return err
	}
	defer lock.close()
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := touch(filepath.Join(store.root, "supervisor.heartbeat")); err != nil {
			return err
		}
		if store.workerOnline() {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
				continue
			}
		}
		arguments := append([]string{"task-worker", "--"}, allowedDirectories...)
		command := exec.Command(executable, arguments...)
		command.Env = os.Environ()
		command.Stdin, command.Stdout, command.Stderr = nil, nil, nil
		configureDetachedHelper(command)
		if err := command.Start(); err != nil {
			logger.Error("task supervisor could not start worker", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
				continue
			}
		}
		logger.Info("task supervisor started worker", "pid", command.Process.Pid)
		wait := make(chan error, 1)
		go func() { wait <- command.Wait() }()
		heartbeat := time.NewTicker(time.Second)
		workerExited := false
		for !workerExited {
			select {
			case <-ctx.Done():
				heartbeat.Stop()
				return nil
			case <-heartbeat.C:
				if err := touch(filepath.Join(store.root, "supervisor.heartbeat")); err != nil {
					heartbeat.Stop()
					return err
				}
			case err := <-wait:
				logger.Warn("task worker exited; restart scheduled", "error", err)
				workerExited = true
			}
		}
		heartbeat.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
	}
}
