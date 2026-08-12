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

const (
	byteMutationPreviewTokenBytes = 32
	byteMutationKindBOM           = "manage_bom"
	byteMutationKindEncoding      = "convert_encoding"
)

type preparedByteMutationTarget struct {
	requestedPath      string
	resolvedPath       string
	data               []byte
	targetFingerprint  string
	resultFingerprint  string
	sourceSnapshot     filesystem.FileSnapshot
	sourceMode         os.FileMode
	identityFile       *filesystem.FileIdentity
	changed            bool
	adjacentBackupPath string
	sourceEncoding     string
	targetEncoding     string
	bomPolicy          string
	bomType            string
	hasBOM             bool
}

type preparedByteMutation struct {
	kind         string
	action       string
	backupPolicy string
	targets      []preparedByteMutationTarget
}

type byteMutationPreview struct {
	id            string
	createdAt     time.Time
	expiresAt     time.Time
	prepared      preparedByteMutation
	retainedBytes int64
	element       *list.Element
}

type byteMutationPreviewStore struct {
	mu         sync.Mutex
	entries    map[string]*byteMutationPreview
	order      *list.List
	maxEntries int
	maxBytes   int64
	ttl        time.Duration
	totalBytes int64
	now        func() time.Time
	random     io.Reader
}

func newByteMutationPreviewStore(maxEntries int, maxBytes int64, ttl time.Duration) *byteMutationPreviewStore {
	return &byteMutationPreviewStore{
		entries:    make(map[string]*byteMutationPreview),
		order:      list.New(),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		ttl:        ttl,
		now:        time.Now,
		random:     rand.Reader,
	}
}

func (store *byteMutationPreviewStore) put(prepared preparedByteMutation) (*byteMutationPreview, error) {
	if store == nil || store.maxEntries <= 0 || store.maxBytes <= 0 || store.ttl <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "byte mutation preview cache is not configured")
	}
	if prepared.kind != byteMutationKindBOM && prepared.kind != byteMutationKindEncoding {
		return nil, operation.New(operation.KindInvalidInput, "byte mutation preview kind is invalid")
	}
	retainedBytes, err := prepared.retainedBytes()
	if err != nil {
		return nil, err
	}
	if retainedBytes > store.maxBytes {
		return nil, operation.New(operation.KindLimit, fmt.Sprintf("prepared mutation retains %d bytes; cache limit is %d", retainedBytes, store.maxBytes))
	}
	prepared = clonePreparedByteMutation(prepared, true)

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
		prepared.close()
		return nil, err
	}
	preview := &byteMutationPreview{
		id:            id,
		createdAt:     now,
		expiresAt:     now.Add(store.ttl),
		prepared:      prepared,
		retainedBytes: retainedBytes,
	}
	preview.element = store.order.PushBack(id)
	store.entries[id] = preview
	store.totalBytes += retainedBytes
	return cloneByteMutationPreview(preview), nil
}

func (store *byteMutationPreviewStore) claim(id, expectedKind string) (*byteMutationPreview, error) {
	if !validByteMutationPreviewID(id) {
		return nil, operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")
	}
	if store == nil {
		return nil, operation.New(operation.KindConflict, "mutation preview is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked(store.now().UTC())
	preview, ok := store.entries[id]
	if !ok {
		return nil, operation.New(operation.KindConflict, "mutation preview is unavailable, expired, or already consumed")
	}
	if preview.prepared.kind != expectedKind {
		return nil, operation.New(operation.KindConflict, "previewId belongs to a different mutation capability")
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

func (store *byteMutationPreviewStore) newIDLocked() (string, error) {
	var raw [byteMutationPreviewTokenBytes]byte
	for range 4 {
		if _, err := io.ReadFull(store.random, raw[:]); err != nil {
			return "", operation.Wrap(operation.KindFilesystem, "create_byte_mutation_preview_id", "", err)
		}
		id := hex.EncodeToString(raw[:])
		if _, exists := store.entries[id]; !exists {
			return id, nil
		}
	}
	return "", operation.New(operation.KindConflict, "could not allocate a unique mutation preview identifier")
}

func (store *byteMutationPreviewStore) purgeExpiredLocked(now time.Time) {
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

func (store *byteMutationPreviewStore) removeLocked(id string) {
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
	preview.prepared.close()
}

func cloneByteMutationPreview(preview *byteMutationPreview) *byteMutationPreview {
	if preview == nil {
		return nil
	}
	cloned := *preview
	cloned.element = nil
	cloned.prepared = clonePreparedByteMutation(preview.prepared, false)
	return &cloned
}

func clonePreparedByteMutation(prepared preparedByteMutation, retainIdentities bool) preparedByteMutation {
	cloned := prepared
	cloned.targets = make([]preparedByteMutationTarget, len(prepared.targets))
	for index := range prepared.targets {
		cloned.targets[index] = prepared.targets[index]
		cloned.targets[index].data = append([]byte(nil), prepared.targets[index].data...)
		if !retainIdentities {
			cloned.targets[index].identityFile = nil
		}
	}
	return cloned
}

func (prepared *preparedByteMutation) close() {
	if prepared == nil {
		return
	}
	for index := range prepared.targets {
		if prepared.targets[index].identityFile != nil {
			_ = prepared.targets[index].identityFile.Close()
			prepared.targets[index].identityFile = nil
		}
	}
}

func (prepared preparedByteMutation) retainedBytes() (int64, error) {
	parts := []int{len(prepared.kind), len(prepared.action), len(prepared.backupPolicy)}
	var total int64
	for _, part := range parts {
		if int64(part) > math.MaxInt64-total {
			return 0, operation.New(operation.KindLimit, "prepared mutation size exceeds supported range")
		}
		total += int64(part)
	}
	for _, target := range prepared.targets {
		for _, part := range []int{
			len(target.data), len(target.requestedPath), len(target.resolvedPath),
			len(target.targetFingerprint), len(target.resultFingerprint), len(target.adjacentBackupPath),
			len(target.sourceEncoding), len(target.targetEncoding), len(target.bomPolicy), len(target.bomType),
		} {
			if int64(part) > math.MaxInt64-total {
				return 0, operation.New(operation.KindLimit, "prepared mutation size exceeds supported range")
			}
			total += int64(part)
		}
	}
	return total, nil
}

func validByteMutationPreviewID(id string) bool {
	if len(id) != byteMutationPreviewTokenBytes*2 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
