package diagnostics

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactingHandlerRemovesSensitiveValuesAndFingerprintsPaths(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(newRedactingHandler(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))

	logger.Info("diagnostic_event",
		"tool", "read_text_file",
		"path", `C:\private\secret.txt`,
		"error", errors.New("token=super-secret"),
		"token", "super-secret",
		"count", 7,
	)

	text := output.String()
	for _, forbidden := range []string{`C:\private\secret.txt`, "super-secret", "token=super-secret"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("log leaked %q: %s", forbidden, text)
		}
	}
	for _, expected := range []string{"msg=diagnostic_event", "tool=read_text_file", "count=7", "path=sha256:"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("log missing %q: %s", expected, text)
		}
	}
}

func TestRedactingHandlerKeepsStableErrorCodeButRedactsHumanError(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(newRedactingHandler(slog.NewTextHandler(&output, nil)))
	logger.Warn("tool_call_failed",
		"errorCode", "ACCESS_DENIED",
		"error", errors.New("denied C:\\private\\secret.txt"),
	)

	text := output.String()
	if !strings.Contains(text, "errorCode=ACCESS_DENIED") {
		t.Fatalf("stable error code was not retained: %s", text)
	}
	if strings.Contains(text, "secret.txt") || strings.Contains(text, "denied") {
		t.Fatalf("human error details leaked: %s", text)
	}
}

func TestRedactingHandlerBoundsStringAttributes(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(newRedactingHandler(slog.NewTextHandler(&output, nil)))
	logger.Info("diagnostic_event", "tool", strings.Repeat("x", maxSafeStringBytes*4))

	text := output.String()
	if len(text) > maxSafeStringBytes*2 {
		t.Fatalf("bounded log line grew unexpectedly: %d bytes", len(text))
	}
	if !strings.Contains(text, "truncated") {
		t.Fatalf("bounded string did not report truncation: %s", text)
	}
}

func TestChannelLoggerAddsOnlyFixedMetadata(t *testing.T) {
	var output bytes.Buffer
	base := slog.New(newRedactingHandler(slog.NewTextHandler(&output, nil)))
	logger := channelLogger(base, "http_access", "frontend")
	logger.Info("http_request", "method", "POST", "route", "mcp", "status", 200)

	text := output.String()
	for _, expected := range []string{"channel=http_access", "role=frontend", "method=POST", "route=mcp", "status=200"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("log missing %q: %s", expected, text)
		}
	}
}
