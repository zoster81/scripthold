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

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/operation"
)

const (
	gcPreviewTokenBytes = 32
	gcPreviewMaxEntries = 64
	gcPreviewMaxBytes   = int64(16 * 1024 * 1024)
)

type gcPreview struct {
	id            string
	createdAt     time.Time
	expiresAt     time.Time
	plan          backupstore.GCPlan
	retainedBytes int64
	element       *list.Element
}

type gcPreviewStore struct {
	mu         sync.Mutex
	ownerIndex *confirmationOwnerIndex
	entries    map[string]*gcPreview
	order      *list.List
	maxEntries int
	maxBytes   int64
	ttl        time.Duration
	totalBytes int64
	now        func() time.Time
	random     io.Reader
}

func newGCPreviewStore(maxEntries int, maxBytes int64, ttl time.Duration) *gcPreviewStore {
	return &gcPreviewStore{
		entries:    make(map[string]*gcPreview),
		order:      list.New(),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		ttl:        ttl,
		now:        time.Now,
		random:     rand.Reader,
	}
}

func (store *gcPreviewStore) put(plan backupstore.GCPlan) (*gcPreview, error) {
	if store == nil || store.maxEntries <= 0 || store.maxBytes <= 0 || store.ttl <= 0 {
		return nil, operation.New(operation.KindInvalidInput, "backup GC preview cache is not configured")
	}
	retainedBytes, err := gcPlanRetainedBytes(plan)
	if err != nil {
		return nil, err
	}
	if retainedBytes > store.maxBytes {
		return nil, operation.New(operation.KindLimit, fmt.Sprintf("backup GC plan retains %d bytes; cache limit is %d", retainedBytes, store.maxBytes))
	}
	plan = cloneGCPlan(plan)
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
	if store.ownerIndex != nil {
		if err := store.ownerIndex.reserve(id, "backup_gc"); err != nil {
			return nil, err
		}
	}
	preview := &gcPreview{
		id:            id,
		createdAt:     now,
		expiresAt:     now.Add(store.ttl),
		plan:          plan,
		retainedBytes: retainedBytes,
	}
	preview.element = store.order.PushBack(id)
	store.entries[id] = preview
	store.totalBytes += retainedBytes
	return cloneGCPreview(preview), nil
}

func (store *gcPreviewStore) claim(id string) (*gcPreview, error) {
	if !validGCPreviewID(id) {
		return nil, operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")
	}
	if store == nil {
		return nil, operation.New(operation.KindConflict, "backup GC preview is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked(store.now().UTC())
	preview, ok := store.entries[id]
	if !ok {
		return nil, operation.New(operation.KindConflict, "backup GC preview is unavailable, expired, or already consumed")
	}
	delete(store.entries, id)
	if store.ownerIndex != nil {
		store.ownerIndex.forget(id, "backup_gc")
	}
	if preview.element != nil {
		store.order.Remove(preview.element)
		preview.element = nil
	}
	store.totalBytes -= preview.retainedBytes
	if store.totalBytes < 0 {
		store.totalBytes = 0
	}
	return cloneGCPreview(preview), nil
}

func (store *gcPreviewStore) discard(id string) {
	if store == nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.removeLocked(id)
}

func (store *gcPreviewStore) len() int {
	if store == nil {
		return 0
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked(store.now().UTC())
	return len(store.entries)
}

func (store *gcPreviewStore) purgeExpiredLocked(now time.Time) {
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

func (store *gcPreviewStore) removeLocked(id string) {
	preview, ok := store.entries[id]
	if !ok {
		return
	}
	delete(store.entries, id)
	if store.ownerIndex != nil {
		store.ownerIndex.forget(id, "backup_gc")
	}
	if preview.element != nil {
		store.order.Remove(preview.element)
	}
	store.totalBytes -= preview.retainedBytes
	if store.totalBytes < 0 {
		store.totalBytes = 0
	}
}

func (store *gcPreviewStore) newIDLocked() (string, error) {
	var raw [gcPreviewTokenBytes]byte
	for range 4 {
		if _, err := io.ReadFull(store.random, raw[:]); err != nil {
			return "", operation.Wrap(operation.KindFilesystem, "create_backup_gc_preview_id", "", err)
		}
		id := hex.EncodeToString(raw[:])
		if _, exists := store.entries[id]; !exists {
			return id, nil
		}
	}
	return "", operation.New(operation.KindConflict, "could not allocate a unique backup GC preview identifier")
}

func validGCPreviewID(id string) bool {
	if len(id) != gcPreviewTokenBytes*2 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func cloneGCPreview(preview *gcPreview) *gcPreview {
	if preview == nil {
		return nil
	}
	cloned := *preview
	cloned.element = nil
	cloned.plan = cloneGCPlan(preview.plan)
	return &cloned
}

func cloneGCPlan(plan backupstore.GCPlan) backupstore.GCPlan {
	cloned := plan
	if plan.Manifests != nil {
		cloned.Manifests = make([]backupstore.GCManifestCandidate, len(plan.Manifests))
		for index, candidate := range plan.Manifests {
			cloned.Manifests[index] = candidate
			cloned.Manifests[index].Reasons = append([]backupstore.GCReason(nil), candidate.Reasons...)
		}
	}
	if plan.Objects != nil {
		cloned.Objects = append([]backupstore.GCObjectCandidate(nil), plan.Objects...)
	}
	return cloned
}

func gcPlanRetainedBytes(plan backupstore.GCPlan) (int64, error) {
	parts := []int{len(plan.PlannedAt), len(plan.Generation)}
	for _, candidate := range plan.Manifests {
		parts = append(parts, len(candidate.BackupID), len(candidate.CreatedAt), len(candidate.TargetPath), len(candidate.ObjectDigest))
		for _, reason := range candidate.Reasons {
			parts = append(parts, len(reason))
		}
	}
	for _, candidate := range plan.Objects {
		parts = append(parts, len(candidate.Digest))
	}
	var total int64
	for _, part := range parts {
		if int64(part) > math.MaxInt64-total {
			return 0, operation.New(operation.KindLimit, "backup GC plan size exceeds the supported range")
		}
		total += int64(part)
	}
	return total, nil
}
