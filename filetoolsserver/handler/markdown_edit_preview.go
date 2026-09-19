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

const markdownPreviewTokenBytes = 32

type markdownPreviewKind string

const (
	markdownPreviewEdit   markdownPreviewKind = "edit"
	markdownPreviewCreate markdownPreviewKind = "create"
)

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

type preparedMarkdownCreate struct {
	requestedPath     string
	resolvedPath      string
	parentPath        string
	resultData        []byte
	resultFingerprint string
	encoding          string
	bomType           string
	lineEndingStyle   string
	expectedTarget    filesystem.FileSnapshot
	parentIdentity    filesystem.ObjectIdentity
	hasBOM            bool
}

type markdownPreview struct {
	id            string
	createdAt     time.Time
	expiresAt     time.Time
	kind          markdownPreviewKind
	edit          *preparedMarkdownEdit
	create        *preparedMarkdownCreate
	retainedBytes int64
	element       *list.Element
}

type markdownPreviewStore struct {
	mu         sync.Mutex
	entries    map[string]*markdownPreview
	order      *list.List
	maxEntries int
	maxBytes   int64
	ttl        time.Duration
	totalBytes int64
	now        func() time.Time
	random     io.Reader
}

func newMarkdownPreviewStore(maxEntries int, maxBytes int64, ttl time.Duration) *markdownPreviewStore {
	return &markdownPreviewStore{
		entries:    make(map[string]*markdownPreview),
		order:      list.New(),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		ttl:        ttl,
		now:        time.Now,
		random:     rand.Reader,
	}
}

func (store *markdownPreviewStore) putEdit(prepared preparedMarkdownEdit) (*markdownPreview, error) {
	owned := prepared
	owned.resultUTF8 = append([]byte(nil), prepared.resultUTF8...)
	return store.put(&markdownPreview{kind: markdownPreviewEdit, edit: &owned})
}

func (store *markdownPreviewStore) putCreate(prepared preparedMarkdownCreate) (*markdownPreview, error) {
	owned := prepared
	owned.resultData = append([]byte(nil), prepared.resultData...)
	return store.put(&markdownPreview{kind: markdownPreviewCreate, create: &owned})
}

func (store *markdownPreviewStore) put(candidate *markdownPreview) (*markdownPreview, error) {
	if store == nil || store.maxEntries <= 0 || store.maxBytes <= 0 || store.ttl <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "markdown preview cache is not configured")
	}
	if err := candidate.validatePayload(); err != nil {
		return nil, err
	}
	retainedBytes, err := candidate.payloadRetainedBytes()
	if err != nil {
		return nil, err
	}
	if retainedBytes > store.maxBytes {
		return nil, operation.New(operation.KindLimit, fmt.Sprintf("prepared markdown preview retains %d bytes; cache limit is %d", retainedBytes, store.maxBytes))
	}

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
	preview := &markdownPreview{
		id:            id,
		createdAt:     now,
		expiresAt:     now.Add(store.ttl),
		kind:          candidate.kind,
		edit:          candidate.edit,
		create:        candidate.create,
		retainedBytes: retainedBytes,
	}
	preview.element = store.order.PushBack(id)
	store.entries[id] = preview
	store.totalBytes += retainedBytes
	return cloneMarkdownPreview(preview), nil
}

func (store *markdownPreviewStore) claim(id string) (*markdownPreview, error) {
	if !validMarkdownPreviewID(id) {
		return nil, operation.New(operation.KindInvalidInput, "previewId must be 64 lowercase hexadecimal characters")
	}
	if store == nil {
		return nil, operation.New(operation.KindConflict, "markdown preview is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked(store.now().UTC())
	preview, ok := store.entries[id]
	if !ok {
		return nil, operation.New(operation.KindConflict, "markdown preview is unavailable, expired, evicted, or already consumed")
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

func (store *markdownPreviewStore) discard(id string) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.removeLocked(id)
}

func (store *markdownPreviewStore) purgeExpiredLocked(now time.Time) {
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

func (store *markdownPreviewStore) removeLocked(id string) {
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
	preview.releaseOwnedResources()
}

func (store *markdownPreviewStore) newIDLocked() (string, error) {
	var raw [markdownPreviewTokenBytes]byte
	for range 4 {
		if _, err := io.ReadFull(store.random, raw[:]); err != nil {
			return "", operation.Wrap(operation.KindFilesystem, "create_markdown_preview_id", "", err)
		}
		id := hex.EncodeToString(raw[:])
		if _, exists := store.entries[id]; !exists {
			return id, nil
		}
	}
	return "", operation.New(operation.KindConflict, "could not allocate a unique markdown preview identifier")
}

func validMarkdownPreviewID(id string) bool {
	if len(id) != markdownPreviewTokenBytes*2 {
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

func cloneMarkdownPreview(preview *markdownPreview) *markdownPreview {
	if preview == nil {
		return nil
	}
	copy := *preview
	copy.element = nil
	if preview.edit != nil {
		edit := *preview.edit
		edit.resultUTF8 = append([]byte(nil), preview.edit.resultUTF8...)
		edit.identityFile = nil
		copy.edit = &edit
	}
	if preview.create != nil {
		create := *preview.create
		create.resultData = append([]byte(nil), preview.create.resultData...)
		copy.create = &create
	}
	return &copy
}

func (preview *markdownPreview) validatePayload() error {
	if preview == nil {
		return operation.New(operation.KindInvalidInput, "markdown preview payload is unavailable")
	}
	switch preview.kind {
	case markdownPreviewEdit:
		if preview.edit == nil || preview.create != nil {
			return operation.New(operation.KindInvalidInput, "markdown edit preview payload is invalid")
		}
	case markdownPreviewCreate:
		if preview.create == nil || preview.edit != nil {
			return operation.New(operation.KindInvalidInput, "markdown create preview payload is invalid")
		}
	default:
		return operation.New(operation.KindInvalidInput, "markdown preview kind is invalid")
	}
	return nil
}

func (preview *markdownPreview) payloadRetainedBytes() (int64, error) {
	if err := preview.validatePayload(); err != nil {
		return 0, err
	}
	if preview.edit != nil {
		return preview.edit.retainedBytes()
	}
	return preview.create.retainedBytes()
}

func (preview *markdownPreview) releaseOwnedResources() {
	if preview == nil || preview.edit == nil || preview.edit.identityFile == nil {
		return
	}
	_ = preview.edit.identityFile.Close()
	preview.edit.identityFile = nil
}

func validMarkdownEditPreviewID(id string) bool {
	return validMarkdownPreviewID(id)
}

func (prepared preparedMarkdownEdit) retainedBytes() (int64, error) {
	parts := []int{
		len(prepared.resultUTF8), len(prepared.requestedPath), len(prepared.resolvedPath),
		len(prepared.targetFingerprint), len(prepared.resultFingerprint), len(prepared.encoding),
		len(prepared.bomType), len(prepared.lineEndingStyle), len(prepared.diff), len(prepared.backupPolicy),
	}
	return markdownPreviewRetainedBytes(parts...)
}

func (prepared preparedMarkdownCreate) retainedBytes() (int64, error) {
	parts := []int{
		len(prepared.resultData), len(prepared.requestedPath), len(prepared.resolvedPath), len(prepared.parentPath),
		len(prepared.resultFingerprint), len(prepared.encoding), len(prepared.bomType), len(prepared.lineEndingStyle),
	}
	return markdownPreviewRetainedBytes(parts...)
}

func markdownPreviewRetainedBytes(parts ...int) (int64, error) {
	var total int64
	for _, part := range parts {
		if int64(part) > math.MaxInt64-total {
			return 0, operation.New(operation.KindLimit, "prepared markdown preview size exceeds supported range")
		}
		total += int64(part)
	}
	return total, nil
}
