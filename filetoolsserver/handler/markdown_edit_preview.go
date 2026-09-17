package handler

import (
	"container/list"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/markdownintelligence"
	"github.com/zoster81/scripthold/internal/operation"
)

const markdownEditPreviewTokenBytes = 32

type preparedMarkdownEdit struct {
	requestedPath     string
	resolvedPath      string
	resultUTF8        []byte
	targetFingerprint string
	resultFingerprint string
	encoding          string
	bomType           string
	lineEndingStyle   string
	diff              string
	backupPolicy      string
	semantic          markdownintelligence.PreparedChange
	identityFile      *filesystem.FileIdentity
	hasBOM            bool
	changed           bool
}

type markdownEditPreview struct {
	id            string
	createdAt     time.Time
	expiresAt     time.Time
	prepared      preparedMarkdownEdit
	retainedBytes int64
	element       *list.Element
}

type markdownEditPreviewStore struct {
	mu         sync.Mutex
	entries    map[string]*markdownEditPreview
	order      *list.List
	maxEntries int
	maxBytes   int64
	ttl        time.Duration
	totalBytes int64
	now        func() time.Time
	random     io.Reader
}

func newMarkdownEditPreviewStore(maxEntries int, maxBytes int64, ttl time.Duration) *markdownEditPreviewStore {
	return &markdownEditPreviewStore{
		entries:    make(map[string]*markdownEditPreview),
		order:      list.New(),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		ttl:        ttl,
		now:        time.Now,
		random:     rand.Reader,
	}
}

func (store *markdownEditPreviewStore) put(prepared preparedMarkdownEdit) (*markdownEditPreview, error) {
	if store == nil || store.maxEntries <= 0 || store.maxBytes <= 0 || store.ttl <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "markdown edit preview cache is not configured")
	}
	retainedBytes, err := prepared.retainedBytes()
	if err != nil {
		return nil, err
	}
	if retainedBytes > store.maxBytes {
		return nil, operation.New(operation.KindLimit, fmt.Sprintf("prepared markdown edit retains %d bytes; cache limit is %d", retainedBytes, store.maxBytes))
	}
	prepared.resultUTF8 = append([]byte(nil), prepared.resultUTF8...)

	store.mu.Lock()
	defer store.mu.Unlock()
	now := store.now().UTC()
	store.purgeExpiredLocked(now)
	for len(store.entries) >= store.maxEntries || store.totalBytes > store.maxBytes-retainedBytes {
		oldest := store.order.Front()
		if oldest == nil {
			break
		}
		store.removeLocked(oldest.Value.(string))
	}
	id, err := store.newIDLocked()
	if err != nil {
		return nil, err
	}
	preview := &markdownEditPreview{
		id:            id,
		createdAt:     now,
		expiresAt:     now.Add(store.ttl),
		prepared:      prepared,
		retainedBytes: retainedBytes,
	}
	preview.element = store.order.PushBack(id)
	store.entries[id] = preview
	store.totalBytes += retainedBytes
	return cloneMarkdownEditPreview(preview), nil
}

func (store *markdownEditPreviewStore) claim(id string) (*markdownEditPreview, error) {
	if !validMarkdownEditPreviewID(id) {
		return nil, operation.New(operation.KindInvalidInput, "previewId must be 64 lowercase hexadecimal characters")
	}
	if store == nil {
		return nil, operation.New(operation.KindConflict, "markdown edit preview is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked(store.now().UTC())
	preview, ok := store.entries[id]
	if !ok {
		return nil, operation.New(operation.KindConflict, "markdown edit preview is unavailable, expired, evicted, or already consumed")
	}
	delete(store.entries, id)
	if preview.element != nil {
		store.order.Remove(preview.element)
		preview.element = nil
	}
	store.totalBytes -= preview.retainedBytes
	if store.totalBytes < 0 {
		store.totalBytes = 0
	}
	return preview, nil
}

func (store *markdownEditPreviewStore) discard(id string) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.removeLocked(id)
}

func (store *markdownEditPreviewStore) purgeExpiredLocked(now time.Time) {
	for element := store.order.Front(); element != nil; {
		next := element.Next()
		id := element.Value.(string)
		preview := store.entries[id]
		if preview != nil && !preview.expiresAt.After(now) {
			store.removeLocked(id)
		}
		element = next
	}
}

func (store *markdownEditPreviewStore) removeLocked(id string) {
	preview, ok := store.entries[id]
	if !ok {
		return
	}
	delete(store.entries, id)
	if preview.element != nil {
		store.order.Remove(preview.element)
		preview.element = nil
	}
	store.totalBytes -= preview.retainedBytes
	if store.totalBytes < 0 {
		store.totalBytes = 0
	}
	if preview.prepared.identityFile != nil {
		_ = preview.prepared.identityFile.Close()
		preview.prepared.identityFile = nil
	}
}

func (store *markdownEditPreviewStore) newIDLocked() (string, error) {
	var raw [markdownEditPreviewTokenBytes]byte
	for range 4 {
		if _, err := io.ReadFull(store.random, raw[:]); err != nil {
			return "", operation.Wrap(operation.KindFilesystem, "create_markdown_edit_preview_id", "", err)
		}
		id := hex.EncodeToString(raw[:])
		if _, exists := store.entries[id]; !exists {
			return id, nil
		}
	}
	return "", operation.New(operation.KindConflict, "could not allocate a unique markdown edit preview identifier")
}

func validMarkdownEditPreviewID(id string) bool {
	if len(id) != markdownEditPreviewTokenBytes*2 {
		return false
	}
	for _, value := range id {
		if (value < '0' || value > '9') && (value < 'a' || value > 'f') {
			return false
		}
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func cloneMarkdownEditPreview(preview *markdownEditPreview) *markdownEditPreview {
	if preview == nil {
		return nil
	}
	copy := *preview
	copy.element = nil
	copy.prepared.resultUTF8 = append([]byte(nil), preview.prepared.resultUTF8...)
	copy.prepared.identityFile = nil
	return &copy
}

func (prepared preparedMarkdownEdit) retainedBytes() (int64, error) {
	parts := []int{
		len(prepared.resultUTF8), len(prepared.requestedPath), len(prepared.resolvedPath),
		len(prepared.targetFingerprint), len(prepared.resultFingerprint), len(prepared.encoding),
		len(prepared.bomType), len(prepared.lineEndingStyle), len(prepared.diff), len(prepared.backupPolicy),
	}
	var total int64
	for _, part := range parts {
		if int64(part) > math.MaxInt64-total {
			return 0, operation.New(operation.KindLimit, "prepared markdown edit size exceeds supported range")
		}
		total += int64(part)
	}
	return total, nil
}
