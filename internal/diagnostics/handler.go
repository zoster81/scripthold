package diagnostics

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"
)

const (
	maxEventMessageBytes = 256
	maxSafeStringBytes   = 256
)

type redactingHandler struct {
	next slog.Handler
}

func newRedactingHandler(next slog.Handler) slog.Handler {
	return &redactingHandler{next: next}
}

func (handler *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.next.Enabled(ctx, level)
}

func (handler *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, boundedString(record.Message, maxEventMessageBytes), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clean.AddAttrs(sanitizeAttr(attr))
		return true
	})
	return handler.next.Handle(ctx, clean)
}

func (handler *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		clean = append(clean, sanitizeAttr(attr))
	}
	return &redactingHandler{next: handler.next.WithAttrs(clean)}
}

func (handler *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: handler.next.WithGroup(boundedString(name, maxSafeStringBytes))}
}

func channelLogger(base *slog.Logger, channel, role string) *slog.Logger {
	if base == nil {
		return nil
	}
	return base.With(
		"channel", boundedString(channel, maxSafeStringBytes),
		"role", boundedString(role, maxSafeStringBytes),
	)
}

func sanitizeAttr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	key := strings.ToLower(attr.Key)
	if isPathKey(key) {
		if attr.Value.Kind() == slog.KindString {
			return slog.String(attr.Key, pathFingerprint(attr.Value.String()))
		}
		return slog.String(attr.Key, "[redacted]")
	}
	if isStableErrorCodeKey(key) {
		if attr.Value.Kind() == slog.KindString {
			return slog.String(attr.Key, boundedString(attr.Value.String(), maxSafeStringBytes))
		}
		return slog.String(attr.Key, "[redacted]")
	}
	if isSensitiveKey(key) {
		return slog.String(attr.Key, "[redacted]")
	}
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		clean := make([]slog.Attr, 0, len(group))
		for _, child := range group {
			clean = append(clean, sanitizeAttr(child))
		}
		return slog.Group(attr.Key, attrsToAny(clean)...)
	}
	switch attr.Value.Kind() {
	case slog.KindString:
		if !isSafeStringKey(key) {
			return slog.String(attr.Key, "[redacted]")
		}
		return slog.String(attr.Key, boundedString(attr.Value.String(), maxSafeStringBytes))
	case slog.KindBool, slog.KindDuration, slog.KindFloat64, slog.KindInt64, slog.KindTime, slog.KindUint64:
		return attr
	default:
		return slog.String(attr.Key, "[redacted]")
	}
}

func attrsToAny(attrs []slog.Attr) []any {
	values := make([]any, len(attrs))
	for index := range attrs {
		values[index] = attrs[index]
	}
	return values
}

func isPathKey(key string) bool {
	for _, marker := range []string{"path", "dir", "root", "file"} {
		if key == marker || strings.HasSuffix(key, "_"+marker) || strings.HasSuffix(key, marker+"s") {
			return true
		}
	}
	return false
}

func isStableErrorCodeKey(key string) bool {
	return key == "errorcode" || key == "error_code"
}

func isSensitiveKey(key string) bool {
	for _, marker := range []string{
		"authorization", "body", "command", "content", "cookie", "diff", "error", "message",
		"panic", "password", "preview", "secret", "session", "stack", "token", "value",
	} {
		if key == marker || strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func isSafeStringKey(key string) bool {
	switch key {
	case "channel", "role", "tool", "errorcode", "error_code", "method", "route", "transport",
		"operation", "sourceoperation", "source_operation", "outcome", "state", "phase", "kind",
		"encoding", "fallback", "name", "level", "status_text", "reason", "policy", "event":
		return true
	default:
		return false
	}
}

func pathFingerprint(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", digest[:8])
}

func boundedString(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	if len(value) <= maximum {
		return value
	}
	const suffix = "...[truncated]"
	budget := maximum - len(suffix)
	if budget <= 0 {
		return suffix[:maximum]
	}
	for budget > 0 && !utf8.ValidString(value[:budget]) {
		budget--
	}
	return value[:budget] + suffix
}
