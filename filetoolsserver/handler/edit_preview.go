package handler

import (
	"container/list"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

const editPreviewTokenBytes = 32

type preparedEdit struct {
	requestedPath     string
	resolvedPath      string
	data              []byte
	diff              string
	targetFingerprint string
	resultFingerprint string
	encoding          string
	bomType           string
	lineEndingStyle   string
	backupPolicy      string
	hasBOM            bool
	changed           bool
	forceWritable     bool
	sourceMode        os.FileMode
	sourceSnapshot    filesystem.FileSnapshot
	identityFile      *filesystem.FileIdentity
}

type editPreview struct {
	id            string
	createdAt     time.Time
	expiresAt     time.Time
	plan          preparedEditPlan
	retainedBytes int64
	element       *list.Element
}

type editPreviewStore struct {
	mu         sync.Mutex
	entries    map[string]*editPreview
	order      *list.List
	maxEntries int
	maxBytes   int64
	ttl        time.Duration
	totalBytes int64
	now        func() time.Time
	random     io.Reader
}

func newEditPreviewStore(maxEntries int, maxBytes int64, ttl time.Duration) *editPreviewStore {
	return &editPreviewStore{
		entries:    make(map[string]*editPreview),
		order:      list.New(),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		ttl:        ttl,
		now:        time.Now,
		random:     rand.Reader,
	}
}

func (store *editPreviewStore) put(prepared preparedEdit) (*editPreview, error) {
	if store == nil || store.maxEntries <= 0 || store.maxBytes <= 0 || store.ttl <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "edit preview cache is not configured")
	}
	retainedBytes, err := prepared.retainedBytes()
	if err != nil {
		return nil, err
	}
	if retainedBytes > store.maxBytes {
		return nil, operation.New(operation.KindLimit, fmt.Sprintf("prepared edit retains %d bytes; cache limit is %d", retainedBytes, store.maxBytes))
	}

	prepared.data = append([]byte(nil), prepared.data...)
	plan, err := newPreparedEditPlan([]preparedEditPlanTarget{{
		index:                     0,
		requestedPath:             prepared.requestedPath,
		resolvedPath:              prepared.resolvedPath,
		expectedFingerprint:       prepared.targetFingerprint,
		expectedResultFingerprint: prepared.resultFingerprint,
		prepared:                  prepared,
	}})
	if err != nil {
		return nil, err
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
	preview := &editPreview{
		id:            id,
		createdAt:     now,
		expiresAt:     now.Add(store.ttl),
		plan:          plan,
		retainedBytes: retainedBytes,
	}
	preview.element = store.order.PushBack(id)
	store.entries[id] = preview
	store.totalBytes += retainedBytes
	return cloneEditPreview(preview), nil
}

func (store *editPreviewStore) claim(id string) (*editPreview, error) {
	if !validEditPreviewID(id) {
		return nil, operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")
	}
	if store == nil {
		return nil, operation.New(operation.KindConflict, "edit preview is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	store.purgeExpiredLocked(store.now().UTC())
	preview, ok := store.entries[id]
	if !ok {
		return nil, operation.New(operation.KindConflict, "edit preview is unavailable, expired, or already consumed")
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

func (store *editPreviewStore) discard(id string) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.removeLocked(id)
}

func (store *editPreviewStore) newIDLocked() (string, error) {
	var raw [editPreviewTokenBytes]byte
	for range 4 {
		if _, err := io.ReadFull(store.random, raw[:]); err != nil {
			return "", operation.Wrap(operation.KindFilesystem, "create_edit_preview_id", "", err)
		}
		id := hex.EncodeToString(raw[:])
		if _, exists := store.entries[id]; !exists {
			return id, nil
		}
	}
	return "", operation.New(operation.KindConflict, "could not allocate a unique edit preview identifier")
}

func (store *editPreviewStore) purgeExpiredLocked(now time.Time) {
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

func (store *editPreviewStore) removeLocked(id string) {
	preview, ok := store.entries[id]
	if !ok {
		return
	}
	delete(store.entries, id)
	if preview.element != nil {
		store.order.Remove(preview.element)
	}
	store.totalBytes -= preview.retainedBytes
	if store.totalBytes < 0 {
		store.totalBytes = 0
	}
	preview.plan.close()
}

func cloneEditPreview(preview *editPreview) *editPreview {
	if preview == nil {
		return nil
	}
	copy := *preview
	copy.element = nil
	copy.plan.targets = append([]preparedEditPlanTarget(nil), preview.plan.targets...)
	for index := range copy.plan.targets {
		copy.plan.targets[index].prepared.data = append([]byte(nil), preview.plan.targets[index].prepared.data...)
		copy.plan.targets[index].prepared.identityFile = nil
	}
	return &copy
}

func validEditPreviewID(id string) bool {
	if len(id) != editPreviewTokenBytes*2 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (prepared preparedEdit) retainedBytes() (int64, error) {
	parts := []int{
		len(prepared.data), len(prepared.diff), len(prepared.requestedPath), len(prepared.resolvedPath),
		len(prepared.targetFingerprint), len(prepared.resultFingerprint), len(prepared.encoding),
		len(prepared.bomType), len(prepared.lineEndingStyle), len(prepared.backupPolicy),
	}
	var total int64
	for _, part := range parts {
		if int64(part) > math.MaxInt64-total {
			return 0, operation.New(operation.KindLimit, "prepared edit size exceeds supported range")
		}
		total += int64(part)
	}
	return total, nil
}
