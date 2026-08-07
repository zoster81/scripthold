package httptransport

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	internalexecution "github.com/zoster81/scripthold/internal/execution"
)

const (
	sessionIDHeader         = "Mcp-Session-Id"
	protocolVersionHeader   = "Mcp-Protocol-Version"
	protocolVersion20260728 = "2026-07-28"
	protocolVersion20251125 = "2025-11-25"
	protocolVersion20250618 = "2025-06-18"
	protocolVersion20250326 = "2025-03-26"
	protocolVersion20241105 = "2024-11-05"
	maxForwardedHops        = 16
	sessionTrackerGrace     = time.Second
	postRequestTimeout      = time.Duration(internalexecution.MaximumTimeoutSeconds+60) * time.Second
)

type protocolGeneration uint8

const (
	protocolGenerationLegacy protocolGeneration = iota
	protocolGenerationModern
)

func classifyProtocolVersion(header http.Header) (protocolGeneration, error) {
	values := header.Values(protocolVersionHeader)
	if len(values) == 0 {
		return protocolGenerationLegacy, nil
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("repeated %s header", protocolVersionHeader)
	}
	version := values[0]
	if version == "" || strings.TrimSpace(version) != version || strings.Contains(version, ",") {
		return 0, fmt.Errorf("invalid %s header", protocolVersionHeader)
	}
	switch version {
	case protocolVersion20260728:
		return protocolGenerationModern, nil
	case protocolVersion20251125, protocolVersion20250618, protocolVersion20250326, protocolVersion20241105:
		return protocolGenerationLegacy, nil
	default:
		return 0, fmt.Errorf("unsupported %s header", protocolVersionHeader)
	}
}

// Handler applies the approved HTTP security policy before delegating to the
// pinned MCP Streamable HTTP implementation.
type Handler struct {
	config           Config
	logger           *slog.Logger
	legacyMCPHandler http.Handler
	modernMCPHandler http.Handler
	authHandler      http.Handler
	concurrency      chan struct{}
	bodyBudget       *byteBudget
	sessions         *sessionGate
	limiter          *peerLimiter
	ready            atomic.Bool
	shuttingDown     atomic.Bool
}

// NewHandler creates the secured HTTP handler for one shared MCP server.
func NewHandler(config Config, server *mcp.Server, logger *slog.Logger) *Handler {
	discardLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	serverForRequest := func(*http.Request) *mcp.Server { return server }
	legacyStreamable := mcp.NewStreamableHTTPHandler(
		serverForRequest,
		&mcp.StreamableHTTPOptions{
			Logger:              discardLogger,
			SessionTimeout:      config.SessionTimeout,
			MaxRequestBodyBytes: config.MaxBodyBytes,
		},
	)
	modernStreamable := mcp.NewStreamableHTTPHandler(
		serverForRequest,
		&mcp.StreamableHTTPOptions{
			Logger:                       discardLogger,
			Stateless:                    true,
			MaxRequestBodyBytes:          config.MaxBodyBytes,
			PropagateRequestCancellation: true,
		},
	)

	handler := &Handler{
		config:           config,
		logger:           logger,
		legacyMCPHandler: legacyStreamable,
		modernMCPHandler: modernStreamable,
		concurrency:      make(chan struct{}, config.MaxConcurrentRequests),
		bodyBudget:       newByteBudget(config.MaxInFlightBodyBytes),
		sessions:         newSessionGate(config.MaxSessions, config.SessionTimeout+sessionTrackerGrace),
		limiter: newPeerLimiter(
			defaultRatePerSec,
			defaultRateBurst,
			defaultRatePeers,
			defaultRatePeerIdle,
		),
	}
	verifier := auth.TokenVerifier(handler.verifyToken)
	handler.authHandler = auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		Scopes: []string{"mcp"},
	})(http.HandlerFunc(handler.serveAuthenticated))
	return handler
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	recorder := newStatusRecorder(w)
	started := time.Now()
	handler.serve(recorder, request)
	if handler.logger != nil {
		handler.logger.Debug("http_request",
			"method", request.Method,
			"route", handler.routeName(request.URL.Path),
			"status", recorder.statusCode(),
			"duration", time.Since(started),
		)
	}
}

func (handler *Handler) setReady(ready bool) {
	handler.ready.Store(ready)
}

func (handler *Handler) beginShutdown() {
	handler.shuttingDown.Store(true)
	handler.ready.Store(false)
}

func (handler *Handler) close() {
	handler.beginShutdown()
	handler.sessions.closeAll()
}

func (handler *Handler) serve(w http.ResponseWriter, request *http.Request) {
	peer, trustedProxy, err := handler.requestPeer(request)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if handler.config.ProxyOnlyPlaintext && !trustedProxy {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !handler.config.AllowsHost(request.Host) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if !handler.originAllowed(request) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	clientAddress, err := handler.clientAddress(request, peer, trustedProxy)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if !handler.limiter.allow(clientAddress.String()) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	switch request.URL.Path {
	case "/healthz":
		handler.serveHealth(w, request, true)
		return
	case "/readyz":
		handler.serveHealth(w, request, false)
		return
	}

	if handler.shuttingDown.Load() || !handler.ready.Load() {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	if request.URL.Path != handler.config.Path {
		http.NotFound(w, request)
		return
	}
	if request.URL.RawQuery != "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	handler.authHandler.ServeHTTP(w, request)
}

func (handler *Handler) serveHealth(w http.ResponseWriter, request *http.Request, liveness bool) {
	if request.URL.RawQuery != "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	status := http.StatusOK
	body := "ok\n"
	if !liveness {
		body = "ready\n"
		if !handler.ready.Load() || handler.shuttingDown.Load() {
			status = http.StatusServiceUnavailable
			body = "not ready\n"
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if request.Method == http.MethodGet {
		_, _ = io.WriteString(w, body)
	}
}

func (handler *Handler) serveAuthenticated(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost && request.Method != http.MethodGet && request.Method != http.MethodDelete {
		w.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if request.Header.Get("Last-Event-ID") != "" {
		http.Error(w, "Event resumption is not supported", http.StatusBadRequest)
		return
	}

	generation, err := classifyProtocolVersion(request.Header)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if generation == protocolGenerationModern {
		if request.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if len(request.Header.Values(sessionIDHeader)) != 0 {
			http.Error(w, "MCP session IDs are not supported by this protocol version", http.StatusBadRequest)
			return
		}
	}

	if request.Method != http.MethodGet {
		select {
		case handler.concurrency <- struct{}{}:
			defer func() { <-handler.concurrency }()
		default:
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
	}

	var boundedBody *boundedReadCloser
	if request.Method == http.MethodPost {
		var releaseBody func()
		var ok bool
		boundedBody, releaseBody, ok = handler.prepareBoundedBody(w, request)
		if !ok {
			return
		}
		defer releaseBody()
		ctx, cancel := context.WithTimeout(request.Context(), postRequestTimeout)
		defer cancel()
		request = request.WithContext(ctx)
	}

	if generation == protocolGenerationModern {
		recorder := newStatusRecorder(w)
		delegate := http.ResponseWriter(recorder)
		if boundedBody != nil {
			delegate = newBodyLimitResponseWriter(recorder, boundedBody)
		}
		handler.modernMCPHandler.ServeHTTP(delegate, request)
		return
	}

	sessionID := request.Header.Get(sessionIDHeader)
	var releaseSession func()
	if sessionID != "" {
		if request.Method == http.MethodPost {
			var ok bool
			releaseSession, ok = handler.sessions.acquire(sessionID)
			if !ok {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			defer releaseSession()
		} else if !handler.sessions.contains(sessionID) {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
	}

	var reservation *sessionReservation
	if request.Method == http.MethodPost && sessionID == "" {
		var ok bool
		reservation, ok = handler.sessions.reserve()
		if !ok {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		defer reservation.cancel()
	}

	recorder := newStatusRecorder(w)
	delegate := http.ResponseWriter(recorder)
	if boundedBody != nil {
		delegate = newBodyLimitResponseWriter(recorder, boundedBody)
	}
	handler.legacyMCPHandler.ServeHTTP(delegate, request)
	status := recorder.statusCode()

	if reservation != nil {
		newSessionID := recorder.Header().Get(sessionIDHeader)
		if status >= 200 && status < 300 && newSessionID != "" {
			reservation.commit(newSessionID)
		}
	}
	if sessionID != "" && (request.Method == http.MethodDelete || status == http.StatusNotFound) {
		handler.sessions.release(sessionID)
	}
}

func (handler *Handler) prepareBoundedBody(
	w http.ResponseWriter,
	request *http.Request,
) (*boundedReadCloser, func(), bool) {
	if request.ContentLength > handler.config.MaxBodyBytes {
		http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
		return nil, nil, false
	}
	reservation := request.ContentLength
	if reservation < 0 {
		reservation = handler.config.MaxBodyBytes
	}
	if !handler.bodyBudget.tryAcquire(reservation) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return nil, nil, false
	}
	bounded := newBoundedReadCloser(request.Body, handler.config.MaxBodyBytes)
	request.Body = bounded
	return bounded, func() { handler.bodyBudget.release(reservation) }, true
}

func (handler *Handler) verifyToken(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	actual := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(handler.config.TokenDigest[:], actual[:]) != 1 {
		return nil, auth.ErrInvalidToken
	}
	return &auth.TokenInfo{
		Scopes:     []string{"mcp"},
		Expiration: time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC),
		UserID:     handler.config.PrincipalID,
	}, nil
}

func (handler *Handler) originAllowed(request *http.Request) bool {
	values := request.Header.Values("Origin")
	if len(values) == 0 {
		return true
	}
	if len(values) != 1 {
		return false
	}
	return handler.config.AllowsOrigin(values[0])
}

func (handler *Handler) requestPeer(request *http.Request) (netip.Addr, bool, error) {
	peer, err := parseRemoteAddress(request.RemoteAddr)
	if err != nil {
		return netip.Addr{}, false, err
	}
	return peer, handler.isTrustedProxy(peer), nil
}

func (handler *Handler) clientAddress(request *http.Request, peer netip.Addr, trustedProxy bool) (netip.Addr, error) {
	if !trustedProxy {
		return peer, nil
	}
	forwarded := strings.TrimSpace(request.Header.Get("X-Forwarded-For"))
	if forwarded == "" {
		return peer, nil
	}
	parts := strings.Split(forwarded, ",")
	if len(parts) > maxForwardedHops {
		return netip.Addr{}, fmt.Errorf("too many forwarded hops")
	}
	addresses := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		address, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil {
			return netip.Addr{}, err
		}
		addresses = append(addresses, address.Unmap())
	}
	for index := len(addresses) - 1; index >= 0; index-- {
		if !handler.isTrustedProxy(addresses[index]) {
			return addresses[index], nil
		}
	}
	return addresses[0], nil
}

func (handler *Handler) isTrustedProxy(address netip.Addr) bool {
	for _, prefix := range handler.config.TrustedProxyCIDRs {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func parseRemoteAddress(remote string) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	address, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return netip.Addr{}, err
	}
	return address.Unmap(), nil
}

func (handler *Handler) routeName(path string) string {
	switch path {
	case handler.config.Path:
		return "mcp"
	case "/healthz":
		return "health"
	case "/readyz":
		return "ready"
	default:
		return "unknown"
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func newStatusRecorder(writer http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: writer}
}

func (recorder *statusRecorder) WriteHeader(status int) {
	if recorder.status != 0 {
		return
	}
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(data []byte) (int, error) {
	if recorder.status == 0 {
		recorder.WriteHeader(http.StatusOK)
	}
	return recorder.ResponseWriter.Write(data)
}

func (recorder *statusRecorder) Flush() {
	if recorder.status == 0 {
		recorder.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(recorder.ResponseWriter).Flush()
}

func (recorder *statusRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}

func (recorder *statusRecorder) statusCode() int {
	if recorder.status == 0 {
		return http.StatusOK
	}
	return recorder.status
}
