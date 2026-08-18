package httptransport

import (
	"testing"
	"time"
)

func TestSessionGateReservesCommitsTouchesAndReleases(t *testing.T) {
	gate := newSessionGate(1, 50*time.Millisecond)
	defer gate.closeAll()

	reservation, ok := gate.reserve()
	if !ok {
		t.Fatal("first reservation rejected")
	}
	if _, ok := gate.reserve(); ok {
		t.Fatal("reservation exceeded max sessions")
	}
	reservation.commit("session-1")
	releaseActive, ok := gate.acquire("session-1")
	if !ok {
		t.Fatal("committed session not found")
	}
	releaseActive()
	gate.release("session-1")
	if _, ok := gate.acquire("session-1"); ok {
		t.Fatal("released session remained active")
	}
	if _, ok := gate.reserve(); !ok {
		t.Fatal("capacity not restored")
	}
}

func TestSessionGateFailedInitializationReleasesCapacity(t *testing.T) {
	gate := newSessionGate(1, time.Minute)
	defer gate.closeAll()

	reservation, ok := gate.reserve()
	if !ok {
		t.Fatal("reservation rejected")
	}
	reservation.cancel()
	if _, ok := gate.reserve(); !ok {
		t.Fatal("cancelled reservation leaked capacity")
	}
}

func TestSessionGateExpiryIsIdempotent(t *testing.T) {
	gate := newSessionGate(1, time.Minute)
	defer gate.closeAll()

	reservation, _ := gate.reserve()
	reservation.commit("session-1")
	expireCurrentSession(t, gate, "session-1")
	gate.release("session-1")
	gate.release("session-1")
	if _, ok := gate.reserve(); !ok {
		t.Fatal("expired session did not release capacity")
	}
}

func TestSessionGateDoesNotExpireActivePOST(t *testing.T) {
	gate := newSessionGate(1, time.Minute)
	defer gate.closeAll()

	reservation, _ := gate.reserve()
	reservation.commit("session-1")
	releaseActive, ok := gate.acquire("session-1")
	if !ok {
		t.Fatal("session acquire failed")
	}
	expireCurrentSession(t, gate, "session-1")
	if _, ok := gate.reserve(); ok {
		t.Fatal("active session expired and released capacity")
	}
	releaseActive()
	expireCurrentSession(t, gate, "session-1")
	if _, ok := gate.reserve(); !ok {
		t.Fatal("idle session did not expire after active request completed")
	}
}

func TestSessionGateAllowsIdleExpiryDuringSSE(t *testing.T) {
	gate := newSessionGate(1, time.Minute)
	defer gate.closeAll()

	reservation, _ := gate.reserve()
	reservation.commit("session-1")
	if !gate.contains("session-1") {
		t.Fatal("committed session not found")
	}
	expireCurrentSession(t, gate, "session-1")
	if gate.contains("session-1") {
		t.Fatal("idle session remained reserved during SSE-only activity")
	}
	if _, ok := gate.reserve(); !ok {
		t.Fatal("expired SSE session did not release capacity")
	}
}

func TestSessionGateTimerExpiresIdleSession(t *testing.T) {
	gate := newSessionGate(1, 10*time.Millisecond)
	defer gate.closeAll()

	reservation, ok := gate.reserve()
	if !ok {
		t.Fatal("reservation rejected")
	}
	reservation.commit("session-1")

	deadline := time.Now().Add(time.Second)
	for gate.contains("session-1") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if gate.contains("session-1") {
		t.Fatal("idle session timer did not expire the session")
	}
}

func expireCurrentSession(t *testing.T, gate *sessionGate, sessionID string) {
	t.Helper()
	gate.mu.Lock()
	entry := gate.sessions[sessionID]
	if entry == nil {
		gate.mu.Unlock()
		t.Fatalf("session %q not found", sessionID)
	}
	generation := entry.generation
	gate.mu.Unlock()
	gate.expire(sessionID, generation)
}
