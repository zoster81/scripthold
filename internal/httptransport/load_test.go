package httptransport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestConcurrentHTTPLoadKeepsAdmissionStateBounded(t *testing.T) {
	const (
		requestCount = 512
		concurrency  = 8
		sessionID    = "load-session"
	)

	cfg := validTestConfig(2)
	cfg.MaxConcurrentRequests = concurrency
	cfg.MaxBodyBytes = 1024
	cfg.MaxInFlightBodyBytes = 8 * 1024
	cfg.SessionTimeout = time.Minute

	server := mcp.NewServer(&mcp.Implementation{Name: "load-test", Version: "test"}, nil)
	handler := NewHandler(cfg, server, nil)
	handler.limiter = newPeerLimiter(1_000_000, 1_000_000, defaultRatePeers, defaultRatePeerIdle)
	handler.setReady(true)
	defer handler.close()

	reservation, ok := handler.sessions.reserve()
	if !ok {
		t.Fatal("failed to reserve load-test session")
	}
	reservation.commit(sessionID)

	var active atomic.Int64
	var maximum atomic.Int64
	handler.legacyMCPHandler = http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		_, _ = io.Copy(io.Discard, request.Body)
		_ = request.Body.Close()
		time.Sleep(2 * time.Millisecond)
		w.WriteHeader(http.StatusAccepted)
	})

	var accepted atomic.Int64
	var rejected atomic.Int64
	var unexpected atomic.Int64
	var wait sync.WaitGroup
	wait.Add(requestCount)
	start := make(chan struct{})
	for index := 0; index < requestCount; index++ {
		go func() {
			defer wait.Done()
			<-start
			request := authenticatedPostRequest("http://127.0.0.1:8765/mcp", `{}`)
			request.Header.Set(sessionIDHeader, sessionID)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			switch recorder.Code {
			case http.StatusAccepted:
				accepted.Add(1)
			case http.StatusTooManyRequests:
				rejected.Add(1)
			default:
				unexpected.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()

	if unexpected.Load() != 0 {
		t.Fatalf("unexpected HTTP responses = %d", unexpected.Load())
	}
	if accepted.Load() == 0 || rejected.Load() == 0 {
		t.Fatalf("load did not exercise both admission paths: accepted=%d rejected=%d", accepted.Load(), rejected.Load())
	}
	if got := maximum.Load(); got > concurrency {
		t.Fatalf("maximum active handlers = %d, want <= %d", got, concurrency)
	}
	if got := active.Load(); got != 0 {
		t.Fatalf("active handlers after load = %d", got)
	}

	handler.bodyBudget.mu.Lock()
	bodyBytes := handler.bodyBudget.used
	handler.bodyBudget.mu.Unlock()
	if bodyBytes != 0 {
		t.Fatalf("in-flight body budget leaked %d bytes", bodyBytes)
	}

	handler.sessions.mu.Lock()
	entry := handler.sessions.sessions[sessionID]
	pending := handler.sessions.pending
	sessionCount := len(handler.sessions.sessions)
	activeSessionRequests := 0
	if entry != nil {
		activeSessionRequests = entry.active
	}
	handler.sessions.mu.Unlock()
	if pending != 0 || sessionCount != 1 || entry == nil || activeSessionRequests != 0 {
		t.Fatalf(
			"session accounting after load: pending=%d sessions=%d entry=%v active=%d",
			pending,
			sessionCount,
			entry != nil,
			activeSessionRequests,
		)
	}
}
