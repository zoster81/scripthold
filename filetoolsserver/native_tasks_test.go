package filetoolsserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/deferredoperation"
)

func nativeTasksTestLimits() deferredoperation.Limits {
	return deferredoperation.Limits{
		MaxConcurrency: 2, MaxQueued: 4, MaxRuntimeSeconds: 30, RetentionSeconds: 3600,
		MaxTerminal: 8, MaxTotalBytes: 16 * 1024 * 1024, MaxResultBytes: 4 * 1024 * 1024, MaxChunkBytes: 64 * 1024,
	}
}

func newNativeTasksFixture(t *testing.T) (*handler.Handler, *deferredoperation.Store, string) {
	t.Helper()
	public := canonicalServerTestDir(t)
	store, err := deferredoperation.Initialize(filepath.Join(canonicalServerTestDir(t), "deferred"), []string{public}, nil, nativeTasksTestLimits())
	if err != nil {
		t.Fatal(err)
	}
	return handler.NewHandler([]string{public}, handler.WithDeferredOperationStore(store)), store, public
}

func nativeTasksMeta() mcp.Meta {
	return nativeTasksMetaForProtocol(tasksMinimumProtocolVersion)
}

func nativeTasksMetaForProtocol(protocolVersion string) mcp.Meta {
	return mcp.Meta{
		"io.modelcontextprotocol/protocolVersion": protocolVersion,
		tasksClientCapabilitiesMetaKey: map[string]any{
			"extensions": map[string]any{tasksExtensionID: map[string]any{}},
		},
	}
}

func admitNativeTask(t *testing.T, store *deferredoperation.Store, public string) deferredoperation.Operation {
	t.Helper()
	operation, err := store.Admit(context.Background(), deferredoperation.Request{
		Tool:               "fingerprint_paths",
		Arguments:          json.RawMessage(`{"paths":["fixture"]}`),
		AllowedDirectories: []string{public},
		OriginPaths:        []string{public},
		MaxRuntimeSeconds:  30,
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err = store.MarkStarting(context.Background(), operation.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	operation, err = store.MarkExposed(context.Background(), operation.OperationID, []string{public})
	if err != nil {
		t.Fatal(err)
	}
	return operation
}

func TestNativeTasksMiddlewareTransformsOnlyNegotiatedDurableHandoff(t *testing.T) {
	h, store, public := newNativeTasksFixture(t)
	operation := admitNativeTask(t, store, public)
	middleware := createNativeTasksMiddleware(store, h.ResolvedAllowedDirs)
	call := func(meta mcp.Meta, result *mcp.CallToolResult) mcp.Result {
		t.Helper()
		req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
			Session: &mcp.ServerSession{},
			Params:  &mcp.CallToolParamsRaw{Name: "fingerprint_paths", Meta: meta},
		}
		got, err := middleware(func(context.Context, string, mcp.Request) (mcp.Result, error) { return result, nil })(context.Background(), "tools/call", req)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	handoff := &mcp.CallToolResult{Meta: mcp.Meta{"operationId": operation.OperationID, "deferred": true}}
	got := call(nativeTasksMeta(), handoff)
	created, ok := got.(*nativeCreateTaskResult)
	if !ok {
		t.Fatalf("negotiated result type = %T, want *nativeCreateTaskResult", got)
	}
	if created.ResultType != "task" || created.TaskID != operation.OperationID || created.Status != "working" || created.TTLMS != 3600_000 || created.PollIntervalMS != 1000 {
		t.Fatalf("created task = %+v", created)
	}

	if legacy := call(nil, handoff); legacy != handoff {
		t.Fatalf("non-negotiated handoff changed: %T %+v", legacy, legacy)
	}
	segmented := &mcp.CallToolResult{Meta: mcp.Meta{"operationId": "rsp_fixture", "resultSegmented": true}}
	if got := call(nativeTasksMeta(), segmented); got != segmented {
		t.Fatalf("oversized continuation changed: %T %+v", got, got)
	}
	if got := call(nativeTasksMetaForProtocol(legacyProtocolVersion), handoff); got != handoff {
		t.Fatalf("legacy protocol unexpectedly enabled Tasks: %T %+v", got, got)
	}
}

func TestNativeTasksWrapsRealDeferredFingerprintAfterSyncWindow(t *testing.T) {
	h, cfg, store, path := newDeferredFingerprintFixture(t)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	engine := &fakeDeferredEngine{store: store, releaseWork: release}
	deferred := deferredFingerprintHandler(h, cfg, engine, h.HandleFingerprintPaths)
	middleware := createNativeTasksMiddleware(store, h.ResolvedAllowedDirs)
	req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Session: &mcp.ServerSession{},
		Params:  &mcp.CallToolParamsRaw{Name: "fingerprint_paths", Meta: nativeTasksMeta()},
	}

	result, err := middleware(func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		callResult, _, callErr := deferred(ctx, &mcp.CallToolRequest{}, handler.FingerprintPathsInput{Paths: []string{path}})
		return callResult, callErr
	})(context.Background(), "tools/call", req)
	if err != nil {
		t.Fatal(err)
	}
	created, ok := result.(*nativeCreateTaskResult)
	if !ok || created.ResultType != "task" || created.Status != "working" || !deferredoperation.ValidOperationID(created.TaskID) {
		t.Fatalf("real deferred task result = %T %+v", result, result)
	}

	close(release)
	params := &nativeTaskParams{TaskID: created.TaskID}
	params.Meta = nativeTasksMeta()
	adapter := nativeTasksAdapter{store: store, allowedDirectories: h.ResolvedAllowedDirs}
	deadline := time.Now().Add(3 * time.Second)
	for {
		observed, getErr := adapter.get(context.Background(), nil, params)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if observed.Status == "completed" {
			if len(observed.Result) == 0 {
				t.Fatal("completed native task lost the deferred fingerprint result")
			}
			var final mcp.CallToolResult
			if err := json.Unmarshal(observed.Result, &final); err != nil {
				t.Fatal(err)
			}
			if final.IsError || len(final.Content) == 0 {
				t.Fatalf("final deferred fingerprint result = %+v", final)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("native task did not complete: %+v", observed)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNativeTasksThreeConcurrentSlowCallsPreserveFastPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h, cfg, store, path := newDeferredFingerprintFixture(t)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	started := make(chan string, 3)
	executionErrors := make(chan error, 3)
	slowEngine := &fakeDeferredEngine{
		store:               store,
		releaseWork:         release,
		started:             started,
		executionErrors:     executionErrors,
		handoffAfterStarted: true,
	}
	slow := deferredFingerprintHandler(h, cfg, slowEngine, h.HandleFingerprintPaths)
	middleware := createNativeTasksMiddleware(store, h.ResolvedAllowedDirs)

	type slowResult struct {
		task *nativeCreateTaskResult
		err  error
	}
	results := make(chan slowResult, 3)
	for range 3 {
		go func() {
			req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Session: &mcp.ServerSession{},
				Params:  &mcp.CallToolParamsRaw{Name: "fingerprint_paths", Meta: nativeTasksMeta()},
			}
			result, err := middleware(func(callCtx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
				callResult, _, callErr := slow(callCtx, &mcp.CallToolRequest{}, handler.FingerprintPathsInput{Paths: []string{path}})
				return callResult, callErr
			})(ctx, "tools/call", req)
			if err != nil {
				results <- slowResult{err: err}
				return
			}
			task, ok := result.(*nativeCreateTaskResult)
			if !ok {
				results <- slowResult{err: fmt.Errorf("slow result type = %T", result)}
				return
			}
			results <- slowResult{task: task}
		}()
	}

	for range 3 {
		select {
		case <-started:
		case executeErr := <-executionErrors:
			t.Fatalf("slow deferred execution failed before the started boundary: %v", executeErr)
		case <-ctx.Done():
			t.Fatal("slow deferred operations did not all cross the started boundary")
		}
	}

	fastEngine := &fakeDeferredEngine{store: store, executeNow: true}
	fast := deferredFingerprintHandler(h, cfg, fastEngine, h.HandleFingerprintPaths)
	fastResult, _, err := fast(ctx, &mcp.CallToolRequest{}, handler.FingerprintPathsInput{Paths: []string{path}})
	if err != nil || fastResult == nil || fastResult.IsError {
		t.Fatalf("fast path under slow load result=%+v err=%v", fastResult, err)
	}
	fastReq := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
		Session: &mcp.ServerSession{},
		Params:  &mcp.CallToolParamsRaw{Name: "fingerprint_paths", Meta: nativeTasksMeta()},
	}
	observedFast, err := middleware(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return fastResult, nil
	})(ctx, "tools/call", fastReq)
	if err != nil || observedFast != fastResult {
		t.Fatalf("fast negotiated result changed under slow load: %T %+v err=%v", observedFast, observedFast, err)
	}

	tasks := make([]*nativeCreateTaskResult, 0, 3)
	seen := make(map[string]struct{}, 3)
	for range 3 {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.task == nil || result.task.ResultType != "task" || result.task.Status != "working" {
				t.Fatalf("slow native task = %+v", result.task)
			}
			if _, duplicate := seen[result.task.TaskID]; duplicate {
				t.Fatalf("duplicate native task id %s", result.task.TaskID)
			}
			seen[result.task.TaskID] = struct{}{}
			tasks = append(tasks, result.task)
		case <-ctx.Done():
			t.Fatal("slow calls did not return native task handles after the soft window")
		}
	}

	close(release)
	adapter := nativeTasksAdapter{store: store, allowedDirectories: h.ResolvedAllowedDirs}
	for _, task := range tasks {
		params := &nativeTaskParams{TaskID: task.TaskID}
		params.Meta = nativeTasksMeta()
		for {
			observed, getErr := adapter.get(ctx, nil, params)
			if getErr != nil {
				t.Fatal(getErr)
			}
			if observed.Status == "completed" {
				if len(observed.Result) == 0 {
					t.Fatal("completed stress task lost its final tool result")
				}
				break
			}
			select {
			case <-ctx.Done():
				t.Fatalf("stress task did not complete: %+v", observed)
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
}

func TestNativeTasksMiddlewareRejectsLegacyTaskMethods(t *testing.T) {
	h, store, public := newNativeTasksFixture(t)
	operation := admitNativeTask(t, store, public)
	middleware := createNativeTasksMiddleware(store, h.ResolvedAllowedDirs)
	params := &nativeTaskParams{TaskID: operation.OperationID}
	params.Meta = nativeTasksMetaForProtocol(legacyProtocolVersion)
	req := &mcp.ServerRequest[*nativeTaskParams]{Session: &mcp.ServerSession{}, Params: params}
	_, err := middleware(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &nativeGetTaskResult{}, nil
	})(context.Background(), tasksGetMethod, req)
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeMethodNotFound {
		t.Fatalf("legacy tasks/get error = %#v", err)
	}
}

func TestNativeTasksAdapterGetCompletedResultAndCancel(t *testing.T) {
	h, store, public := newNativeTasksFixture(t)
	adapter := nativeTasksAdapter{store: store, allowedDirectories: h.ResolvedAllowedDirs}

	operation := admitNativeTask(t, store, public)
	payload, metadata, err := encodeDurableToolEnvelope(&mcp.CallToolResult{
		Meta:    mcp.Meta{handler.ErrorCodeMetaKey: "TEST_TOOL_ERROR"},
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: "tool-level failure"}},
	}, map[string]any{"ok": false})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Execute(context.Background(), operation.OperationID, func(context.Context, deferredoperation.Request) ([]byte, deferredoperation.ResultMetadata, error) {
		return payload, metadata, nil
	}); err != nil {
		t.Fatal(err)
	}

	params := &nativeTaskParams{TaskID: operation.OperationID}
	params.Meta = nativeTasksMeta()
	result, err := adapter.get(context.Background(), nil, params)
	if err != nil {
		t.Fatal(err)
	}
	if result.ResultType != "complete" || result.Status != "completed" || len(result.Result) == 0 || result.Error != nil {
		t.Fatalf("completed task = %+v", result)
	}
	var final mcp.CallToolResult
	if err := json.Unmarshal(result.Result, &final); err != nil {
		t.Fatal(err)
	}
	if !final.IsError || resultErrorCode(&final) != "TEST_TOOL_ERROR" {
		t.Fatalf("final tool result = %+v", final)
	}

	cancelled := admitNativeTask(t, store, public)
	cancelParams := &nativeTaskParams{TaskID: cancelled.OperationID}
	cancelParams.Meta = nativeTasksMeta()
	ack, err := adapter.cancel(context.Background(), nil, cancelParams)
	if err != nil {
		t.Fatal(err)
	}
	if ack.ResultType != "complete" {
		t.Fatalf("cancel ack = %+v", ack)
	}
	observed, err := store.Get(cancelled.OperationID, []string{public})
	if err != nil || observed.Status != deferredoperation.StatusCancelled {
		t.Fatalf("cancelled operation = %+v err=%v", observed, err)
	}
}

func TestNativeTasksAdapterUpdateAcknowledgesKnownTaskAndIgnoresUnknownResponses(t *testing.T) {
	h, store, public := newNativeTasksFixture(t)
	adapter := nativeTasksAdapter{store: store, allowedDirectories: h.ResolvedAllowedDirs}
	operation := admitNativeTask(t, store, public)
	params := &nativeUpdateTaskParams{
		TaskID: operation.OperationID,
		InputResponses: map[string]json.RawMessage{
			"never-issued": json.RawMessage(`{"action":"accept"}`),
		},
	}
	params.Meta = nativeTasksMeta()
	ack, err := adapter.update(context.Background(), nil, params)
	if err != nil {
		t.Fatal(err)
	}
	if ack == nil || ack.ResultType != "complete" {
		t.Fatalf("update ack = %+v", ack)
	}

	missingResponses := &nativeUpdateTaskParams{TaskID: operation.OperationID}
	missingResponses.Meta = nativeTasksMeta()
	_, err = adapter.update(context.Background(), nil, missingResponses)
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("missing inputResponses error = %#v", err)
	}

	revoked := nativeTasksAdapter{store: store, allowedDirectories: func() []string { return nil }}
	_, err = revoked.update(context.Background(), nil, params)
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("root-revoked tasks/update error = %#v", err)
	}
}

func TestNativeTasksAdapterMapsExecutionFailureAndRootRevocation(t *testing.T) {
	h, store, public := newNativeTasksFixture(t)
	adapter := nativeTasksAdapter{store: store, allowedDirectories: h.ResolvedAllowedDirs}

	operation := admitNativeTask(t, store, public)
	failed, err := store.Fail(operation.OperationID, deferredoperation.StatusInterrupted, "EXECUTOR_LOST", "deferred executor heartbeat was lost; operation was not rerun")
	if err != nil {
		t.Fatal(err)
	}
	params := &nativeTaskParams{TaskID: failed.OperationID}
	params.Meta = nativeTasksMeta()
	result, err := adapter.get(context.Background(), nil, params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || result.Error == nil || result.Error.Code != jsonrpc.CodeInternalError || result.StatusMessage == "" {
		t.Fatalf("failed task result = %+v", result)
	}

	revoked := nativeTasksAdapter{store: store, allowedDirectories: func() []string { return nil }}
	_, err = revoked.get(context.Background(), nil, params)
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("root-revoked task error = %#v", err)
	}
}

func TestNativeTasksAdapterRejectsMissingCapabilityAndUnknownTask(t *testing.T) {
	h, store, _ := newNativeTasksFixture(t)
	adapter := nativeTasksAdapter{store: store, allowedDirectories: h.ResolvedAllowedDirs}

	_, err := adapter.get(context.Background(), nil, &nativeTaskParams{TaskID: "op_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != tasksMissingCapabilityCode {
		t.Fatalf("missing capability error = %#v", err)
	}

	unknown := &nativeTaskParams{TaskID: "op_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	unknown.Meta = nativeTasksMeta()
	_, err = adapter.get(context.Background(), nil, unknown)
	if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("unknown task error = %#v", err)
	}
}

func TestNativeTasksRawWireCallToolReturnsTaskResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	h, store, public := newNativeTasksFixture(t)
	operation := admitNativeTask(t, store, public)
	server := mcp.NewServer(&mcp.Implementation{Name: "native-task-wire-server", Version: "test"}, nil)
	type input struct{}
	type output struct {
		Accepted bool `json:"accepted"`
	}
	mcp.AddTool(server, &mcp.Tool{Name: "native_task_probe"}, func(context.Context, *mcp.CallToolRequest, input) (*mcp.CallToolResult, output, error) {
		return &mcp.CallToolResult{Meta: mcp.Meta{"operationId": operation.OperationID, "deferred": true}}, output{Accepted: true}, nil
	})
	server.AddReceivingMiddleware(createNativeTasksMiddleware(store, h.ResolvedAllowedDirs))

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	connection, err := clientTransport.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	wireRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "native_task_probe",
			"arguments": map[string]any{},
			"_meta": map[string]any{
				"io.modelcontextprotocol/protocolVersion": tasksMinimumProtocolVersion,
				"io.modelcontextprotocol/clientInfo":      map[string]any{"name": "raw-tasks-client", "version": "test"},
				tasksClientCapabilitiesMetaKey:            map[string]any{"extensions": map[string]any{tasksExtensionID: map[string]any{}}},
			},
		},
	}
	encodedRequest, err := json.Marshal(wireRequest)
	if err != nil {
		t.Fatal(err)
	}
	message, err := jsonrpc.DecodeMessage(encodedRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Write(ctx, message); err != nil {
		t.Fatal(err)
	}
	message, err = connection.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	response, ok := message.(*jsonrpc.Response)
	if !ok || response.Error != nil {
		t.Fatalf("raw task response = %#v", message)
	}
	encodedResponse, err := jsonrpc.EncodeMessage(response)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Result struct {
			ResultType string `json:"resultType"`
			TaskID     string `json:"taskId"`
			Status     string `json:"status"`
		} `json:"result"`
	}
	if err := json.Unmarshal(encodedResponse, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Result.ResultType != "task" || envelope.Result.TaskID != operation.OperationID || envelope.Result.Status != "working" {
		t.Fatalf("raw tools/call task result = %+v", envelope.Result)
	}
}

func TestNativeTasksWireRegistrationAndDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, store, public := newNativeTasksFixture(t)
	operation := admitNativeTask(t, store, public)
	payload, metadata, err := encodeDurableToolEnvelope(&mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "wire result"}},
	}, map[string]any{"wire": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Execute(context.Background(), operation.OperationID, func(context.Context, deferredoperation.Request) ([]byte, deferredoperation.ResultMetadata, error) {
		return payload, metadata, nil
	}); err != nil {
		t.Fatal(err)
	}

	engine, err := deferredoperation.NewEngine(store, "unused-native-tasks-test-executable")
	if err != nil {
		t.Fatal(err)
	}
	server := BuildServer(ServerOptions{
		Version:            "native-tasks-wire-test",
		AllowedDirectories: []string{public},
		DeferredEngine:     engine,
		LifecycleContext:   ctx,
	})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "native-tasks-wire-client", Version: "test"}, nil)
	if err := mcp.AddSendingCustomMethod[*nativeTaskParams, *nativeGetTaskResult](client, tasksGetMethod); err != nil {
		t.Fatal(err)
	}
	if err := mcp.AddSendingCustomMethod[*nativeUpdateTaskParams, *nativeUpdateTaskResult](client, tasksUpdateMethod); err != nil {
		t.Fatal(err)
	}
	if err := mcp.AddSendingCustomMethod[*nativeTaskParams, *nativeCancelTaskResult](client, tasksCancelMethod); err != nil {
		t.Fatal(err)
	}
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	initialization := clientSession.InitializeResult()
	if initialization == nil || initialization.Capabilities == nil || initialization.Capabilities.Extensions == nil {
		t.Fatalf("native Tasks capabilities missing from discovery: %#v", initialization)
	}
	if _, ok := initialization.Capabilities.Extensions[tasksExtensionID]; !ok {
		t.Fatalf("native Tasks extension missing from discovery: %+v", initialization.Capabilities.Extensions)
	}

	params := &nativeTaskParams{TaskID: operation.OperationID}
	params.Meta = nativeTasksMeta()
	got, err := mcp.CallCustomMethod[*nativeTaskParams, *nativeGetTaskResult](ctx, clientSession, tasksGetMethod, params)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResultType != "complete" || got.Status != "completed" || len(got.Result) == 0 {
		t.Fatalf("wire tasks/get result = %+v", got)
	}
	var final mcp.CallToolResult
	if err := json.Unmarshal(got.Result, &final); err != nil {
		t.Fatal(err)
	}
	if len(final.Content) != 1 {
		t.Fatalf("wire final tool result = %+v", final)
	}

	update := &nativeUpdateTaskParams{TaskID: operation.OperationID, InputResponses: map[string]json.RawMessage{"unknown": json.RawMessage(`{"action":"accept"}`)}}
	update.Meta = nativeTasksMeta()
	updateAck, err := mcp.CallCustomMethod[*nativeUpdateTaskParams, *nativeUpdateTaskResult](ctx, clientSession, tasksUpdateMethod, update)
	if err != nil {
		t.Fatal(err)
	}
	if updateAck == nil || updateAck.ResultType != "complete" {
		t.Fatalf("wire tasks/update ack = %+v", updateAck)
	}

	cancellable := admitNativeTask(t, store, public)
	cancelParams := &nativeTaskParams{TaskID: cancellable.OperationID}
	cancelParams.Meta = nativeTasksMeta()
	cancelAck, err := mcp.CallCustomMethod[*nativeTaskParams, *nativeCancelTaskResult](ctx, clientSession, tasksCancelMethod, cancelParams)
	if err != nil {
		t.Fatal(err)
	}
	if cancelAck == nil || cancelAck.ResultType != "complete" {
		t.Fatalf("wire tasks/cancel ack = %+v", cancelAck)
	}
	cancelled, err := store.Get(cancellable.OperationID, []string{public})
	if err != nil || cancelled.Status != deferredoperation.StatusCancelled {
		t.Fatalf("wire cancelled task = %+v err=%v", cancelled, err)
	}
}

func TestNativeTasksMultiClientReconnectAndStoreRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	public := canonicalServerTestDir(t)
	storeDir := filepath.Join(canonicalServerTestDir(t), "deferred")
	store, err := deferredoperation.Initialize(storeDir, []string{public}, nil, nativeTasksTestLimits())
	if err != nil {
		t.Fatal(err)
	}
	operation := admitNativeTask(t, store, public)
	payload, metadata, err := encodeDurableToolEnvelope(&mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "persistent result"}},
	}, map[string]any{"persistent": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Execute(ctx, operation.OperationID, func(context.Context, deferredoperation.Request) ([]byte, deferredoperation.ResultMetadata, error) {
		return payload, metadata, nil
	}); err != nil {
		t.Fatal(err)
	}

	engine, err := deferredoperation.NewEngine(store, "unused-native-tasks-stress-executable")
	if err != nil {
		t.Fatal(err)
	}
	serverCtx, stopServer := context.WithCancel(ctx)
	server := BuildServer(ServerOptions{
		Version:            "native-tasks-multiclient-test",
		AllowedDirectories: []string{public},
		DeferredEngine:     engine,
		LifecycleContext:   serverCtx,
	})

	type connectedClient struct {
		serverSession *mcp.ServerSession
		clientSession *mcp.ClientSession
	}
	connect := func(name string) (connectedClient, error) {
		serverTransport, clientTransport := mcp.NewInMemoryTransports()
		serverSession, err := server.Connect(ctx, serverTransport, nil)
		if err != nil {
			return connectedClient{}, err
		}
		client := mcp.NewClient(&mcp.Implementation{Name: name, Version: "test"}, nil)
		if err := mcp.AddSendingCustomMethod[*nativeTaskParams, *nativeGetTaskResult](client, tasksGetMethod); err != nil {
			_ = serverSession.Close()
			return connectedClient{}, err
		}
		clientSession, err := client.Connect(ctx, clientTransport, nil)
		if err != nil {
			_ = serverSession.Close()
			return connectedClient{}, err
		}
		return connectedClient{serverSession: serverSession, clientSession: clientSession}, nil
	}
	closeClient := func(client connectedClient) {
		if client.clientSession != nil {
			_ = client.clientSession.Close()
		}
		if client.serverSession != nil {
			_ = client.serverSession.Close()
		}
	}

	clients := make([]connectedClient, 3)
	for i := range clients {
		clients[i], err = connect("native-tasks-concurrent-client")
		if err != nil {
			t.Fatal(err)
		}
		defer closeClient(clients[i])
	}

	errorsCh := make(chan error, len(clients))
	for _, connected := range clients {
		connected := connected
		go func() {
			params := &nativeTaskParams{TaskID: operation.OperationID}
			params.Meta = nativeTasksMeta()
			for range 25 {
				result, callErr := mcp.CallCustomMethod[*nativeTaskParams, *nativeGetTaskResult](ctx, connected.clientSession, tasksGetMethod, params)
				if callErr != nil {
					errorsCh <- callErr
					return
				}
				if result.Status != "completed" || len(result.Result) == 0 {
					errorsCh <- errors.New("concurrent tasks/get lost completed result")
					return
				}
				if _, callErr := connected.clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "list_allowed_directories", Arguments: map[string]any{}}); callErr != nil {
					errorsCh <- callErr
					return
				}
			}
			errorsCh <- nil
		}()
	}
	for range clients {
		if err := <-errorsCh; err != nil {
			t.Fatal(err)
		}
	}

	closeClient(clients[0])
	reconnected, err := connect("native-tasks-reconnected-client")
	if err != nil {
		t.Fatal(err)
	}
	params := &nativeTaskParams{TaskID: operation.OperationID}
	params.Meta = nativeTasksMeta()
	result, err := mcp.CallCustomMethod[*nativeTaskParams, *nativeGetTaskResult](ctx, reconnected.clientSession, tasksGetMethod, params)
	if err != nil || result.Status != "completed" {
		t.Fatalf("reconnected tasks/get = %+v err=%v", result, err)
	}
	closeClient(reconnected)

	for i := 1; i < len(clients); i++ {
		closeClient(clients[i])
	}
	stopServer()

	reopenedStore, err := deferredoperation.Initialize(storeDir, []string{public}, nil, nativeTasksTestLimits())
	if err != nil {
		t.Fatal(err)
	}
	reopenedEngine, err := deferredoperation.NewEngine(reopenedStore, "unused-native-tasks-restart-executable")
	if err != nil {
		t.Fatal(err)
	}
	restartCtx, stopRestart := context.WithCancel(ctx)
	defer stopRestart()
	restartedServer := BuildServer(ServerOptions{
		Version:            "native-tasks-restart-test",
		AllowedDirectories: []string{public},
		DeferredEngine:     reopenedEngine,
		LifecycleContext:   restartCtx,
	})
	server = restartedServer
	restarted, err := connect("native-tasks-restart-client")
	if err != nil {
		t.Fatal(err)
	}
	defer closeClient(restarted)
	result, err = mcp.CallCustomMethod[*nativeTaskParams, *nativeGetTaskResult](ctx, restarted.clientSession, tasksGetMethod, params)
	if err != nil || result.Status != "completed" || len(result.Result) == 0 {
		t.Fatalf("restarted tasks/get = %+v err=%v", result, err)
	}
}

func TestDiscoveryAdvertisesTasksOnlyWhenEnabled(t *testing.T) {
	req := &mcp.ServerRequest[*mcp.DiscoverParams]{Session: &mcp.ServerSession{}, Params: &mcp.DiscoverParams{}}
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.DiscoverResult{Capabilities: &mcp.ServerCapabilities{}}, nil
	}

	withTasks, err := createNativeTasksDiscoveryMiddleware()(next)(context.Background(), methodDiscover, req)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := withTasks.(*mcp.DiscoverResult).Capabilities
	if capabilities.Extensions == nil {
		t.Fatal("tasks extension was not advertised")
	}
	if _, ok := capabilities.Extensions[tasksExtensionID]; !ok {
		t.Fatalf("tasks extension missing: %+v", capabilities.Extensions)
	}

	withoutTasks, err := next(context.Background(), methodDiscover, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := withoutTasks.(*mcp.DiscoverResult).Capabilities.Extensions[tasksExtensionID]; ok {
		t.Fatalf("tasks extension advertised while disabled: %+v", withoutTasks.(*mcp.DiscoverResult).Capabilities.Extensions)
	}
}
