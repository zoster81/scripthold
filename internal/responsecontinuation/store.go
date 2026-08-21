package responsecontinuation

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
	"unicode/utf8"
)

const StatusCompleted = "completed"

var (
	ErrNotFound     = errors.New("deferred operation not found")
	ErrInvalidInput = errors.New("deferred operation request is invalid")
	ErrCapacity     = errors.New("deferred response retention capacity exceeded")
)

type Limits struct {
	MaxEntries    int
	MaxTotalBytes int
	MaxChunkBytes int
	Retention     time.Duration
}

type Metadata struct {
	OriginalIsError bool
	ErrorCode       string
}

type Handle struct {
	OperationID     string `json:"operationId"`
	Status          string `json:"status"`
	TotalBytes      int    `json:"totalBytes"`
	OriginalIsError bool   `json:"originalIsError,omitempty"`
	ErrorCode       string `json:"errorCode,omitempty"`
}

type Chunk struct {
	OperationID     string `json:"operationId"`
	Status          string `json:"status"`
	OriginalIsError bool   `json:"originalIsError,omitempty"`
	ErrorCode       string `json:"errorCode,omitempty"`
	TotalBytes      int    `json:"totalBytes"`
	Offset          int    `json:"offset"`
	NextOffset      int    `json:"nextOffset"`
	Data            string `json:"data"`
	Complete        bool   `json:"complete"`
}

type entry struct {
	id       string
	payload  []byte
	metadata Metadata
	created  time.Time
	expires  time.Time
}

type Store struct {
	mu         sync.Mutex
	limits     Limits
	now        func() time.Time
	entries    map[string]*entry
	totalBytes int
}

func NewStore(limits Limits, now func() time.Time) *Store {
	if limits.MaxEntries <= 0 {
		limits.MaxEntries = 64
	}
	if limits.MaxTotalBytes <= 0 {
		limits.MaxTotalBytes = 128 * 1024 * 1024
	}
	if limits.MaxChunkBytes <= 0 {
		limits.MaxChunkBytes = 512 * 1024
	}
	if limits.Retention <= 0 {
		limits.Retention = time.Hour
	}
	if now == nil {
		now = time.Now
	}
	return &Store{limits: limits, now: now, entries: make(map[string]*entry)}
}

func (store *Store) Retain(payload []byte, metadata Metadata) (Handle, error) {
	if store == nil || len(payload) == 0 || !utf8.Valid(payload) {
		return Handle{}, ErrInvalidInput
	}
	if len(payload) > store.limits.MaxTotalBytes {
		return Handle{}, ErrCapacity
	}
	id, err := randomOperationID()
	if err != nil {
		return Handle{}, err
	}
	now := store.now().UTC()

	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked(now)
	store.evictForLocked(len(payload))
	if len(store.entries) >= store.limits.MaxEntries || len(payload) > store.limits.MaxTotalBytes-store.totalBytes {
		return Handle{}, ErrCapacity
	}
	copyPayload := append([]byte(nil), payload...)
	store.entries[id] = &entry{id: id, payload: copyPayload, metadata: metadata, created: now, expires: now.Add(store.limits.Retention)}
	store.totalBytes += len(copyPayload)
	return handleFromEntry(store.entries[id]), nil
}

func (store *Store) Get(operationID string, offset, limitBytes int) (Chunk, error) {
	if store == nil || !validOperationID(operationID) || offset < 0 || limitBytes < 0 {
		return Chunk{}, ErrInvalidInput
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	now := store.now().UTC()
	store.purgeExpiredLocked(now)
	item, ok := store.entries[operationID]
	if !ok {
		return Chunk{}, ErrNotFound
	}
	if offset > len(item.payload) || (offset > 0 && offset < len(item.payload) && !utf8.RuneStart(item.payload[offset])) {
		return Chunk{}, ErrInvalidInput
	}
	limit := limitBytes
	if limit == 0 || limit > store.limits.MaxChunkBytes {
		limit = store.limits.MaxChunkBytes
	}
	end := offset + limit
	if end > len(item.payload) {
		end = len(item.payload)
	}
	for end > offset && end < len(item.payload) && !utf8.Valid(item.payload[offset:end]) {
		end--
	}
	if end == offset && offset < len(item.payload) {
		_, size := utf8.DecodeRune(item.payload[offset:])
		if size <= 0 || size > store.limits.MaxChunkBytes {
			return Chunk{}, ErrInvalidInput
		}
		end = offset + size
	}
	return Chunk{
		OperationID:     operationID,
		Status:          StatusCompleted,
		OriginalIsError: item.metadata.OriginalIsError,
		ErrorCode:       item.metadata.ErrorCode,
		TotalBytes:      len(item.payload),
		Offset:          offset,
		NextOffset:      end,
		Data:            string(item.payload[offset:end]),
		Complete:        end == len(item.payload),
	}, nil
}

func (store *Store) purgeExpiredLocked(now time.Time) {
	for id, item := range store.entries {
		if !now.Before(item.expires) {
			store.totalBytes -= len(item.payload)
			delete(store.entries, id)
		}
	}
}

func (store *Store) evictForLocked(incoming int) {
	if len(store.entries) < store.limits.MaxEntries && incoming <= store.limits.MaxTotalBytes-store.totalBytes {
		return
	}
	items := make([]*entry, 0, len(store.entries))
	for _, item := range store.entries {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].created.Equal(items[j].created) {
			return items[i].id < items[j].id
		}
		return items[i].created.Before(items[j].created)
	})
	for _, item := range items {
		if len(store.entries) < store.limits.MaxEntries && incoming <= store.limits.MaxTotalBytes-store.totalBytes {
			break
		}
		store.totalBytes -= len(item.payload)
		delete(store.entries, item.id)
	}
}

func handleFromEntry(item *entry) Handle {
	return Handle{OperationID: item.id, Status: StatusCompleted, TotalBytes: len(item.payload), OriginalIsError: item.metadata.OriginalIsError, ErrorCode: item.metadata.ErrorCode}
}

func randomOperationID() (string, error) {
	payload := make([]byte, 32)
	if _, err := rand.Read(payload); err != nil {
		return "", err
	}
	return "op_" + hex.EncodeToString(payload), nil
}

func validOperationID(value string) bool {
	if len(value) != 67 || value[:3] != "op_" {
		return false
	}
	_, err := hex.DecodeString(value[3:])
	return err == nil
}
