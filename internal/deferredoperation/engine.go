package deferredoperation

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	enginePollInterval   = 25 * time.Millisecond
	recoveryPollInterval = 5 * time.Second
)

type Engine struct {
	store  *Store
	launch func(string) error
}

func NewEngine(store *Store, executable string) (*Engine, error) {
	if store == nil {
		return nil, ErrDisabled
	}
	if strings.TrimSpace(executable) == "" {
		return nil, ErrInvalidInput
	}
	engine := &Engine{store: store}
	engine.launch = func(operationID string) error {
		command := exec.Command(executable, "_deferred-exec", store.root, operationID)
		command.Stdin = nil
		command.Stdout = nil
		command.Stderr = nil
		command.Env = minimalExecutorEnvironment(os.Environ())
		configureDetachedHelper(command)
		if err := command.Start(); err != nil {
			return err
		}
		if command.Process != nil {
			return command.Process.Release()
		}
		return nil
	}
	return engine, nil
}

func newEngineWithLauncher(store *Store, launch func(string) error) *Engine {
	return &Engine{store: store, launch: launch}
}

func (engine *Engine) Store() *Store {
	if engine == nil {
		return nil
	}
	return engine.store
}

// Recover safely relaunches only operations that never crossed the durable
// started boundary. Started operations with a lost executor are interrupted and
// never replayed; root-policy changes fail closed before any relaunch.
func (engine *Engine) Recover(ctx context.Context, currentAllowedDirectories []string) error {
	if engine == nil || engine.store == nil || engine.launch == nil {
		return ErrDisabled
	}
	ctx = nonNilContext(ctx)
	operationIDs, recoveryErr := engine.store.prepareRecovery(ctx, currentAllowedDirectories)
	for _, operationID := range operationIDs {
		if err := ctx.Err(); err != nil {
			return errors.Join(recoveryErr, err)
		}
		if err := engine.launch(operationID); err != nil {
			_, failErr := engine.store.Fail(operationID, StatusFailed, "DISPATCH_FAILED", "deferred executor process could not be restarted")
			recoveryErr = errors.Join(recoveryErr, wrapRecoveryFailure(recoveryFailureDispatch, errors.Join(err, failErr)))
		}
	}
	return recoveryErr
}

// RunRecoveryLoop periodically reconciles durable operations against the
// currently authorized root set. Empty roots are treated as "policy not known
// yet" so stdio clients using negotiated MCP roots cannot lose admitted work
// during startup before roots/list completes.
func (engine *Engine) RunRecoveryLoop(ctx context.Context, currentAllowedDirectories func() []string, report func(error)) {
	ctx = nonNilContext(ctx)
	ticker := time.NewTicker(recoveryPollInterval)
	defer ticker.Stop()
	engine.runRecoveryLoop(ctx, currentAllowedDirectories, ticker.C, report)
}

func (engine *Engine) runRecoveryLoop(ctx context.Context, currentAllowedDirectories func() []string, ticks <-chan time.Time, report func(error)) {
	if engine == nil || currentAllowedDirectories == nil || ticks == nil {
		return
	}
	lastReportedReason := ""
	recoverCurrent := func() {
		roots := currentAllowedDirectories()
		if len(roots) == 0 {
			return
		}
		err := engine.Recover(ctx, roots)
		if err == nil {
			lastReportedReason = ""
			return
		}
		if errors.Is(err, context.Canceled) || report == nil {
			return
		}
		reason := RecoveryFailureReason(err)
		if reason == lastReportedReason {
			return
		}
		report(err)
		lastReportedReason = reason
	}
	recoverCurrent()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok {
				return
			}
			recoverCurrent()
		}
	}
}

// Submit durably admits work before spawning its detached executor. The caller
// context governs admission and launch only; it is deliberately not inherited
// by the executor process.
func (engine *Engine) Submit(ctx context.Context, request Request) (Operation, error) {
	if engine == nil || engine.store == nil || engine.launch == nil {
		return Operation{}, ErrDisabled
	}
	ctx = nonNilContext(ctx)
	operation, err := engine.store.Admit(ctx, request)
	if err != nil {
		return Operation{}, err
	}
	operation, err = engine.store.MarkStarting(ctx, operation.OperationID)
	if err != nil {
		_, _ = engine.store.Fail(operation.OperationID, StatusFailed, "DISPATCH_FAILED", "deferred operation could not enter dispatch")
		return operation, err
	}
	if err := engine.launch(operation.OperationID); err != nil {
		failed, failErr := engine.store.Fail(operation.OperationID, StatusFailed, "DISPATCH_FAILED", "deferred executor process could not be started")
		if failErr == nil {
			operation = failed
		}
		return operation, errors.Join(err, failErr)
	}
	return operation, nil
}

// Wait observes independently owned work for at most the supplied duration.
// A false finished result is the handoff boundary; it never cancels the work.
func (engine *Engine) Wait(ctx context.Context, operationID string, currentAllowedDirectories []string, maximum time.Duration) (Operation, bool, error) {
	if engine == nil || engine.store == nil || !ValidOperationID(operationID) || maximum < 0 {
		return Operation{}, false, ErrInvalidInput
	}
	ctx = nonNilContext(ctx)
	deadline := time.Now().Add(maximum)
	for {
		operation, err := engine.store.GetContext(ctx, operationID, currentAllowedDirectories)
		if err != nil {
			return Operation{}, false, err
		}
		if operation.Status.Terminal() {
			return operation, true, nil
		}
		if maximum == 0 || !time.Now().Before(deadline) {
			return operation, false, nil
		}
		remaining := time.Until(deadline)
		pause := enginePollInterval
		if remaining < pause {
			pause = remaining
		}
		timer := time.NewTimer(pause)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return Operation{}, false, ctx.Err()
		case <-timer.C:
		}
	}
}

func minimalExecutorEnvironment(environment []string) []string {
	allowed := map[string]struct{}{
		"SYSTEMROOT": {},
		"WINDIR":     {},
		"TEMP":       {},
		"TMP":        {},
		"TMPDIR":     {},
		"TZ":         {},
	}
	filtered := make([]string, 0, len(allowed))
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, keep := allowed[strings.ToUpper(name)]; keep {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
