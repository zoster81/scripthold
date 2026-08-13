package httptransport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver"
)

func TestClassifyProtocolVersion(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		want    protocolGeneration
		wantErr bool
	}{
		{name: "absent", want: protocolGenerationLegacy},
		{name: "modern", values: []string{protocolVersion20260728}, want: protocolGenerationModern},
		{name: "modern with OWS", values: []string{" \t" + protocolVersion20260728 + " \t"}, wantErr: true},
		{name: "legacy 2025-11-25", values: []string{protocolVersion20251125}, want: protocolGenerationLegacy},
		{name: "legacy 2025-06-18", values: []string{protocolVersion20250618}, want: protocolGenerationLegacy},
		{name: "legacy 2025-03-26", values: []string{protocolVersion20250326}, want: protocolGenerationLegacy},
		{name: "legacy 2024-11-05", values: []string{protocolVersion20241105}, want: protocolGenerationLegacy},
		{name: "empty", values: []string{""}, wantErr: true},
		{name: "repeated same", values: []string{protocolVersion20260728, protocolVersion20260728}, wantErr: true},
		{name: "repeated contradictory", values: []string{protocolVersion20260728, protocolVersion20251125}, wantErr: true},
		{name: "comma joined", values: []string{protocolVersion20260728 + ", " + protocolVersion20251125}, wantErr: true},
		{name: "future unsupported", values: []string{"2027-01-01"}, want: protocolGenerationUnsupported},
		{name: "unknown token", values: []string{"v999.0.0"}, want: protocolGenerationUnsupported},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := make(http.Header)
			for _, value := range test.values {
				header.Add(protocolVersionHeader, value)
			}
			got, err := classifyProtocolVersion(header)
			if (err != nil) != test.wantErr {
				t.Fatalf("classifyProtocolVersion() error = %v, wantErr=%v", err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("classifyProtocolVersion() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestHandlerRoutesModernProtocolToStatelessHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:            "modern-http-test",
		AllowedDirectories: []string{t.TempDir()},
		EnableClientRoots:  false,
		LifecycleContext:   ctx,
	})
	cfg := validTestConfig(2)
	unstarted := httptest.NewUnstartedServer(nil)
	cfg.AllowedHosts = map[string]struct{}{strings.ToLower(unstarted.Listener.Addr().String()): {}}
	h := NewHandler(cfg, server, nil)
	h.setReady(true)
	unstarted.Config.Handler = h
	unstarted.Start()
	defer unstarted.Close()

	session := connectHTTPClient(t, ctx, unstarted.URL+cfg.Path)
	defer session.Close()
	initialization := session.InitializeResult()
	if initialization == nil {
		t.Fatal("missing initialization result")
	}
	if got := initialization.ProtocolVersion; got != protocolVersion20260728 {
		t.Fatalf("protocol version = %q, want %q", got, protocolVersion20260728)
	}
	if got := session.ID(); got != "" {
		t.Fatalf("stateless modern session ID = %q, want empty", got)
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list modern tools: %v", err)
	}
	if len(tools.Tools) != 35 {
		t.Fatalf("modern tool count = %d, want 35", len(tools.Tools))
	}
	prompts, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("list modern prompts: %v", err)
	}
	if len(prompts.Prompts) != 3 {
		t.Fatalf("modern prompt count = %d, want 3", len(prompts.Prompts))
	}

	trackedSessions, pendingReservations := sessionGateState(h.sessions)
	if trackedSessions != 0 || pendingReservations != 0 {
		t.Fatalf("modern traffic touched legacy session gate: sessions=%d pending=%d", trackedSessions, pendingReservations)
	}
}

func TestHandlerRejectsInvalidProtocolRoutingBeforeSDK(t *testing.T) {
	h := newTestHandler(t, 2)
	h.setReady(true)
	var legacyCalls atomic.Int64
	var modernCalls atomic.Int64
	h.legacyMCPHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		legacyCalls.Add(1)
		w.WriteHeader(http.StatusAccepted)
	})
	h.modernMCPHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		modernCalls.Add(1)
		w.WriteHeader(http.StatusAccepted)
	})

	tests := []struct {
		name   string
		values []string
	}{
		{name: "empty", values: []string{""}},
		{name: "repeated", values: []string{protocolVersion20260728, protocolVersion20260728}},
		{name: "contradictory", values: []string{protocolVersion20260728, protocolVersion20251125}},
		{name: "comma joined", values: []string{protocolVersion20260728 + "," + protocolVersion20251125}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := authenticatedPostRequest("http://127.0.0.1:8765/mcp", `{}`)
			for _, value := range test.values {
				req.Header.Add(protocolVersionHeader, value)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body %q", rec.Code, rec.Body.String())
			}
		})
	}
	if legacyCalls.Load() != 0 || modernCalls.Load() != 0 {
		t.Fatalf("invalid routing reached SDK handlers: legacy=%d modern=%d", legacyCalls.Load(), modernCalls.Load())
	}
}

func TestHandlerReturnsStructuredUnsupportedProtocolVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:           "unsupported-version-test",
		EnableClientRoots: false,
		LifecycleContext:  ctx,
	})
	cfg := validTestConfig(2)
	unstarted := httptest.NewUnstartedServer(nil)
	cfg.AllowedHosts = map[string]struct{}{strings.ToLower(unstarted.Listener.Addr().String()): {}}
	h := NewHandler(cfg, server, nil)
	h.setReady(true)
	unstarted.Config.Handler = h
	unstarted.Start()
	defer unstarted.Close()

	const requested = "v999.0.0"
	body := `{"jsonrpc":"2.0","id":301,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"` + requested + `","io.modelcontextprotocol/clientCapabilities":{}}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, unstarted.URL+cfg.Path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set(protocolVersionHeader, requested)
	req.Header.Set("Mcp-Method", "server/discover")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send unsupported version request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body %q", response.StatusCode, readBody(t, response))
	}
	var payload struct {
		ID    int `json:"id"`
		Error struct {
			Code int `json:"code"`
			Data struct {
				Supported []string `json:"supported"`
				Requested string   `json:"requested"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode unsupported-version response: %v", err)
	}
	if payload.ID != 301 || payload.Error.Code != mcp.CodeUnsupportedProtocolVersion {
		t.Fatalf("response id/code = %d/%d, want 301/%d", payload.ID, payload.Error.Code, mcp.CodeUnsupportedProtocolVersion)
	}
	if payload.Error.Data.Requested != requested {
		t.Fatalf("requested version = %q, want %q", payload.Error.Data.Requested, requested)
	}
	if len(payload.Error.Data.Supported) == 0 {
		t.Fatal("unsupported-version response did not advertise supported versions")
	}
}
func TestModernRoutingSharesOuterSecurityBoundary(t *testing.T) {
	h := newTestHandler(t, 2)
	h.setReady(true)
	var calls atomic.Int64
	h.modernMCPHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusAccepted)
	})

	tests := []struct {
		name       string
		configure  func(*http.Request)
		wantStatus int
	}{
		{
			name: "missing authentication",
			configure: func(req *http.Request) {
				req.Header.Del("Authorization")
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "disallowed host",
			configure: func(req *http.Request) {
				req.Host = "attacker.example"
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "disallowed origin",
			configure: func(req *http.Request) {
				req.Header.Set("Origin", "https://attacker.example")
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "event resumption",
			configure: func(req *http.Request) {
				req.Header.Set("Last-Event-ID", "event-1")
			},
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := authenticatedPostRequest("http://127.0.0.1:8765/mcp", `{}`)
			req.Header.Set(protocolVersionHeader, protocolVersion20260728)
			test.configure(req)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d, body %q", rec.Code, test.wantStatus, rec.Body.String())
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("modern security rejection reached SDK handler %d times", calls.Load())
	}
}

func TestModernRoutingKeepsOuterBodyLimit(t *testing.T) {
	cfg := validTestConfig(2)
	cfg.MaxBodyBytes = 8
	cfg.MaxInFlightBodyBytes = 8
	h := NewHandler(cfg, filetoolsserver.BuildServer(filetoolsserver.ServerOptions{Version: "modern-body-limit-test"}), nil)
	h.setReady(true)
	var calls atomic.Int64
	h.modernMCPHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusAccepted)
	})

	req := authenticatedPostRequest("http://127.0.0.1:8765/mcp", strings.Repeat("x", 9))
	req.Header.Set(protocolVersionHeader, protocolVersion20260728)
	req.ContentLength = 9
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("modern oversized body status = %d, want 413", rec.Code)
	}
	if calls.Load() != 0 {
		t.Fatalf("modern oversized body reached SDK handler %d times", calls.Load())
	}
}

func TestModernRoutingLeavesStandardHeaderValidationToSDK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:            "modern-header-validation-test",
		AllowedDirectories: []string{t.TempDir()},
		EnableClientRoots:  false,
		LifecycleContext:   ctx,
	})
	cfg := validTestConfig(2)
	unstarted := httptest.NewUnstartedServer(nil)
	cfg.AllowedHosts = map[string]struct{}{strings.ToLower(unstarted.Listener.Addr().String()): {}}
	h := NewHandler(cfg, server, nil)
	h.setReady(true)
	unstarted.Config.Handler = h
	unstarted.Start()
	defer unstarted.Close()

	var mismatchStatus atomic.Int64
	client := mcp.NewClient(&mcp.Implementation{Name: "header-mismatch-test", Version: "test"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint: unstarted.URL + cfg.Path,
		HTTPClient: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			req.Header.Set("Authorization", "Bearer "+testToken)
			tampered := req.Header.Get("Mcp-Method") == "tools/list"
			if tampered {
				req.Header.Set("Mcp-Method", "tools/call")
			}
			response, err := http.DefaultTransport.RoundTrip(req)
			if tampered && response != nil {
				mismatchStatus.Store(int64(response.StatusCode))
			}
			return response, err
		})},
		MaxRetries: -1,
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect modern client: %v", err)
	}
	defer session.Close()
	if _, err := session.ListTools(ctx, nil); err == nil {
		t.Fatal("SDK accepted contradictory Mcp-Method and JSON-RPC method")
	}
	if got := mismatchStatus.Load(); got != http.StatusBadRequest {
		t.Fatalf("mismatched Mcp-Method status = %d, want 400", got)
	}
}

func TestHandlerEnforcesModernStatelessMethodAndSessionPolicy(t *testing.T) {
	h := newTestHandler(t, 2)
	h.setReady(true)
	var calls atomic.Int64
	h.modernMCPHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusAccepted)
	})

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "http://127.0.0.1:8765/mcp", nil)
			req.Host = "127.0.0.1:8765"
			req.Header.Set("Authorization", "Bearer "+testToken)
			req.Header.Set(protocolVersionHeader, protocolVersion20260728)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "POST" {
				t.Fatalf("status=%d allow=%q, want 405/POST", rec.Code, rec.Header().Get("Allow"))
			}
		})
	}

	req := authenticatedPostRequest("http://127.0.0.1:8765/mcp", `{}`)
	req.Header.Set(protocolVersionHeader, protocolVersion20260728)
	req.Header[sessionIDHeader] = []string{""}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty modern session header status = %d, want 400", rec.Code)
	}

	req = authenticatedPostRequest("http://127.0.0.1:8765/mcp", `{}`)
	req.Header.Set(protocolVersionHeader, protocolVersion20260728)
	req.Header.Set(sessionIDHeader, "legacy-session")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("modern session header status = %d, want 400", rec.Code)
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid modern requests reached SDK handler %d times", calls.Load())
	}
}

func TestHandlerPreservesLegacyStatefulInitialization(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:           "legacy-http-test",
		EnableClientRoots: false,
		LifecycleContext:  ctx,
	})
	cfg := validTestConfig(2)
	unstarted := httptest.NewUnstartedServer(nil)
	cfg.AllowedHosts = map[string]struct{}{strings.ToLower(unstarted.Listener.Addr().String()): {}}
	h := NewHandler(cfg, server, nil)
	h.setReady(true)
	unstarted.Config.Handler = h
	unstarted.Start()
	defer unstarted.Close()

	sessionID := legacyInitialize(t, ctx, unstarted.URL+cfg.Path)
	if sessionID == "" {
		t.Fatal("legacy initialize returned no session ID")
	}
	if !h.sessions.contains(sessionID) {
		t.Fatalf("legacy session %q was not tracked", sessionID)
	}

	initialized := legacyRequest(t, ctx, http.MethodPost, unstarted.URL+cfg.Path,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`, sessionID)
	if initialized.StatusCode != http.StatusAccepted {
		t.Fatalf("initialized status = %d, want 202, body %q", initialized.StatusCode, readBody(t, initialized))
	}
	_ = initialized.Body.Close()

	deleted := legacyRequest(t, ctx, http.MethodDelete, unstarted.URL+cfg.Path, "", sessionID)
	if deleted.StatusCode < 200 || deleted.StatusCode >= 300 {
		t.Fatalf("legacy delete status = %d, body %q", deleted.StatusCode, readBody(t, deleted))
	}
	_ = deleted.Body.Close()
	if h.sessions.contains(sessionID) {
		t.Fatalf("legacy session %q remained tracked after DELETE", sessionID)
	}
}

func TestModernTrafficBypassesFullLegacySessionCapacity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:           "mixed-http-test",
		EnableClientRoots: false,
		LifecycleContext:  ctx,
	})
	cfg := validTestConfig(1)
	unstarted := httptest.NewUnstartedServer(nil)
	cfg.AllowedHosts = map[string]struct{}{strings.ToLower(unstarted.Listener.Addr().String()): {}}
	h := NewHandler(cfg, server, nil)
	h.setReady(true)
	unstarted.Config.Handler = h
	unstarted.Start()
	defer unstarted.Close()

	legacySessionID := legacyInitialize(t, ctx, unstarted.URL+cfg.Path)
	tracked, pending := sessionGateState(h.sessions)
	if tracked != 1 || pending != 0 {
		t.Fatalf("legacy gate state = sessions=%d pending=%d, want 1/0", tracked, pending)
	}

	modern := connectHTTPClient(t, ctx, unstarted.URL+cfg.Path)
	defer modern.Close()
	if got := modern.InitializeResult().ProtocolVersion; got != protocolVersion20260728 {
		t.Fatalf("modern protocol version = %q, want %q", got, protocolVersion20260728)
	}
	if modern.ID() != "" {
		t.Fatalf("modern session ID = %q, want empty", modern.ID())
	}
	tracked, pending = sessionGateState(h.sessions)
	if tracked != 1 || pending != 0 {
		t.Fatalf("modern traffic changed legacy gate state: sessions=%d pending=%d", tracked, pending)
	}

	deleted := legacyRequest(t, ctx, http.MethodDelete, unstarted.URL+cfg.Path, "", legacySessionID)
	_ = deleted.Body.Close()
}

func sessionGateState(gate *sessionGate) (sessions, pending int) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	return len(gate.sessions), gate.pending
}

const legacyInitializePayload = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"legacy-test","version":"test"}}}`

func legacyInitializeResponse(t *testing.T, ctx context.Context, endpoint string) *http.Response {
	t.Helper()
	return legacyRequest(t, ctx, http.MethodPost, endpoint, legacyInitializePayload, "")
}

func legacyInitialize(t *testing.T, ctx context.Context, endpoint string) string {
	t.Helper()
	response := legacyInitializeResponse(t, ctx, endpoint)
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("legacy initialize status = %d, body %q", response.StatusCode, readBody(t, response))
	}
	_, _ = io.Copy(io.Discard, response.Body)
	return response.Header.Get(sessionIDHeader)
}

func legacyRequest(t *testing.T, ctx context.Context, method, endpoint, body, sessionID string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Accept", "application/json, text/event-stream")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if sessionID != "" {
		req.Header.Set(sessionIDHeader, sessionID)
		req.Header.Set(protocolVersionHeader, protocolVersion20251125)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
