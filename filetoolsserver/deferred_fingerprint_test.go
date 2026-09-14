package filetoolsserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/deferredoperation"
)

type fakeDeferredEngine struct {
	store               *deferredoperation.Store
	executeNow          bool
	releaseWork         <-chan struct{}
	started             chan<- string
	executionErrors     chan<- error
	handoffAfterStarted bool
}

func (engine *fakeDeferredEngine) Store() *deferredoperation.Store { return engine.store }

func (engine *fakeDeferredEngine) Submit(ctx context.Context, request deferredoperation.Request) (deferredoperation.Operation, error) {
	operation, err := engine.store.Admit(ctx, request)
	if err != nil {
		return operation, err
	}
	operation, err = engine.store.MarkStarting(ctx, operation.OperationID)
	if err != nil {
		return operation, err
	}
	run := func() {
		executeErr := engine.store.Execute(context.Background(), operation.OperationID, func(ctx context.Context, request deferredoperation.Request) ([]byte, deferredoperation.ResultMetadata, error) {
			if engine.started != nil {
				select {
				case engine.started <- operation.OperationID:
				default:
				}
			}
			if engine.releaseWork != nil {
				select {
				case <-engine.releaseWork:
				case <-ctx.Done():
					return nil, deferredoperation.ResultMetadata{}, ctx.Err()
				}
			}
			return ExecuteDeferredOperation(ctx, request)
		})
		if executeErr != nil && engine.executionErrors != nil {
			select {
			case engine.executionErrors <- executeErr:
			default:
			}
		}
	}
	if engine.executeNow {
		run()
	} else {
		go run()
	}
	return operation, nil
}

func (engine *fakeDeferredEngine) Wait(ctx context.Context, operationID string, allowed []string, maximum time.Duration) (deferredoperation.Operation, bool, error) {
	deadline := time.Now().Add(maximum)
	for {
		operation, err := engine.store.GetContext(ctx, operationID, allowed)
		if err != nil {
			return deferredoperation.Operation{}, false, err
		}
		if operation.Status.Terminal() {
			return operation, true, nil
		}
		if engine.handoffAfterStarted && operation.Started {
			return operation, false, nil
		}
		if !engine.handoffAfterStarted && (maximum == 0 || !time.Now().Before(deadline)) {
			return operation, false, nil
		}
		select {
		case <-ctx.Done():
			return deferredoperation.Operation{}, false, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func newDeferredFingerprintFixture(t *testing.T) (*handler.Handler, *config.Config, *deferredoperation.Store, string) {
	t.Helper()
	public := canonicalServerTestDir(t)
	path := filepath.Join(public, "fixture.txt")
	if err := os.WriteFile(path, []byte("deferred fingerprint fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.LoadFromEnvironment(func(string) string { return "" })
	cfg.Reliability.DeferredSyncWaitSeconds = 1
	cfg.Reliability.DeferredMaxRuntimeSeconds = 30
	store, err := deferredoperation.Initialize(filepath.Join(canonicalServerTestDir(t), "deferred"), []string{public}, nil, deferredoperation.Limits{
		MaxConcurrency: 4, MaxQueued: 8, MaxRuntimeSeconds: 30, RetentionSeconds: 3600,
		MaxTerminal: 8, MaxTotalBytes: 16 * 1024 * 1024, MaxResultBytes: 4 * 1024 * 1024, MaxChunkBytes: 64 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := handler.NewHandler([]string{public}, handler.WithConfig(cfg), handler.WithDeferredOperationStore(store))
	return h, cfg, store, path
}

func TestDeferredFingerprintFastCompletionPreservesNormalOutput(t *testing.T) {
	h, cfg, store, path := newDeferredFingerprintFixture(t)
	engine := &fakeDeferredEngine{store: store, executeNow: true}
	wrapped := deferredFingerprintHandler(h, cfg, engine, h.HandleFingerprintPaths)
	result, raw, err := wrapped(context.Background(), &mcp.CallToolRequest{}, handler.FingerprintPathsInput{Paths: []string{path}})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("fast deferred result=%+v err=%v", result, err)
	}
	output, ok := raw.(handler.FingerprintPathsOutput)
	if !ok || len(output.Fingerprint) != 64 || output.FileCount != 1 {
		t.Fatalf("fast deferred output=%T %+v", raw, raw)
	}
}

func TestDeferredFingerprintSlowCompletionReturnsRecoverableHandoff(t *testing.T) {
	h, cfg, store, path := newDeferredFingerprintFixture(t)
	release := make(chan struct{})
	engine := &fakeDeferredEngine{store: store, releaseWork: release}
	wrapped := deferredFingerprintHandler(h, cfg, engine, h.HandleFingerprintPaths)
	result, raw, err := wrapped(context.Background(), &mcp.CallToolRequest{}, handler.FingerprintPathsInput{Paths: []string{path}})
	if err != nil || result == nil || result.IsError {
		close(release)
		t.Fatalf("slow deferred result=%+v err=%v", result, err)
	}
	handoff, ok := raw.(deferredHandoffOutput)
	if !ok || !handoff.Deferred || !deferredoperation.ValidOperationID(handoff.OperationID) || handoff.Status != "working" {
		close(release)
		t.Fatalf("handoff=%T %+v", raw, raw)
	}
	observed, getErr := store.Get(handoff.OperationID, h.ResolvedAllowedDirs())
	if getErr != nil || !observed.Started || !observed.Exposed || observed.Status != deferredoperation.StatusRunning {
		close(release)
		t.Fatalf("observed=%+v err=%v", observed, getErr)
	}
	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		observed, getErr = store.Get(handoff.OperationID, h.ResolvedAllowedDirs())
		if getErr == nil && observed.Status == deferredoperation.StatusCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if observed.Status != deferredoperation.StatusCompleted {
		t.Fatalf("operation did not complete: %+v err=%v", observed, getErr)
	}
	chunkResult, chunk, chunkErr := h.HandleDeferredOperation(context.Background(), nil, handler.DeferredOperationInput{Operation: "get", OperationID: handoff.OperationID})
	if chunkErr != nil || chunkResult.IsError || !chunk.Complete || chunk.Data == "" {
		t.Fatalf("recovery result=%+v chunk=%+v err=%v", chunkResult, chunk, chunkErr)
	}
	var envelope durableToolEnvelope
	if err := json.Unmarshal([]byte(chunk.Data), &envelope); err != nil || len(envelope.Output) == 0 {
		t.Fatalf("retained envelope invalid: %v", err)
	}
}
