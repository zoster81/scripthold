package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

const (
	sourceSearchCursorVersion   = 1
	sourceSearchCursorMaxBytes  = 12 * 1024
	sourceSearchPayloadMaxBytes = 8 * 1024
	sourceSearchCursorTTL       = 10 * time.Minute
)

type sourceSearchCursorPayload struct {
	Version     int                                   `json:"v"`
	ExpiresAt   int64                                 `json:"expires"`
	QueryHash   string                                `json:"queryHash"`
	Generation  uint64                                `json:"generation"`
	Fingerprint string                                `json:"fingerprint"`
	After       sourceintelligence.ProjectSearchMatch `json:"after"`
	Occurrences int                                   `json:"occurrences"`
}

// A search cursor binds every effective request parameter except the cursor and
// optional index assertion, which are checked independently against the selected
// immutable index generation. No historical result set is retained by the server.
func sourceSearchRequestFingerprint(input SourceQueryInput, maxFiles, maxResults int) (string, error) {
	input.Cursor = ""
	input.Index = nil
	input.MaxFiles = maxFiles
	input.MaxResults = maxResults
	if input.Match == "" {
		input.Match = string(sourceintelligence.ProjectSearchExact)
	}
	input.Operation = "search"
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", operation.Wrap(operation.KindUnknown, "source_search_cursor", "", err)
	}
	hash := sha256.Sum256(append([]byte("scripthold:source-search-request-v1\x00"), encoded...))
	return hex.EncodeToString(hash[:]), nil
}

func (h *Handler) sourceSearchCursorKey() ([]byte, error) {
	h.sourceCursorOnce.Do(func() {
		_, h.sourceCursorInitErr = rand.Read(h.sourceCursorSecret[:])
	})
	if h.sourceCursorInitErr != nil {
		return nil, operation.New(operation.KindUnknown, "source search cursor key initialization failed")
	}
	return h.sourceCursorSecret[:], nil
}

func (h *Handler) encodeSourceSearchCursor(payload sourceSearchCursorPayload) (string, error) {
	key, err := h.sourceSearchCursorKey()
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", operation.Wrap(operation.KindUnknown, "source_search_cursor", "", err)
	}
	if len(encoded) > sourceSearchPayloadMaxBytes {
		return "", operation.New(operation.KindLimit, "source search cursor payload exceeds the maximum size")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(encoded)
	raw := make([]byte, 0, len(encoded)+sha256.Size)
	raw = append(raw, encoded...)
	raw = mac.Sum(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	if len(token) > sourceSearchCursorMaxBytes {
		return "", operation.New(operation.KindLimit, "source search cursor exceeds the maximum size")
	}
	return token, nil
}

func (h *Handler) decodeSourceSearchCursor(token string) (sourceSearchCursorPayload, error) {
	invalid := func() (sourceSearchCursorPayload, error) {
		return sourceSearchCursorPayload{}, operation.New(operation.KindInvalidInput, "source search cursor is invalid")
	}
	if len(token) == 0 || len(token) > sourceSearchCursorMaxBytes {
		return invalid()
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) <= sha256.Size || base64.RawURLEncoding.EncodeToString(decoded) != token {
		return invalid()
	}
	data, tag := decoded[:len(decoded)-sha256.Size], decoded[len(decoded)-sha256.Size:]
	if len(data) > sourceSearchPayloadMaxBytes {
		return invalid()
	}
	key, err := h.sourceSearchCursorKey()
	if err != nil {
		return sourceSearchCursorPayload{}, err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	if !hmac.Equal(tag, mac.Sum(nil)) {
		return invalid()
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var payload sourceSearchCursorPayload
	if err := decoder.Decode(&payload); err != nil {
		return invalid()
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return invalid()
	}
	if payload.Version != sourceSearchCursorVersion || payload.Generation == 0 ||
		!isLowerHexDigest(payload.Fingerprint) || !isLowerHexDigest(payload.QueryHash) ||
		payload.Occurrences <= 0 || payload.After.Path == "" || payload.ExpiresAt <= 0 {
		return invalid()
	}
	if payload.ExpiresAt <= time.Now().Unix() {
		return sourceSearchCursorPayload{}, operation.New(operation.KindConflict, "source search cursor has expired")
	}
	return payload, nil
}
