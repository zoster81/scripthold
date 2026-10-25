package handler

import (
	"sync"

	"github.com/zoster81/scripthold/internal/operation"
)

type confirmationOwnerIndex struct {
	mu         sync.Mutex
	maxEntries int
	owners     map[string]string
}

func newConfirmationOwnerIndex(maxEntries int) *confirmationOwnerIndex {
	return &confirmationOwnerIndex{maxEntries: maxEntries, owners: make(map[string]string)}
}

// reserve requires the owning store lock and precedes preview publication.
func (index *confirmationOwnerIndex) reserve(id, owner string) error {
	if index == nil || owner == "" || !validConfirmationOwnerID(id) {
		return operation.New(operation.KindInvalidInput, "invalid confirmation owner reservation")
	}
	index.mu.Lock()
	defer index.mu.Unlock()
	if _, exists := index.owners[id]; exists {
		return operation.New(operation.KindConflict, "confirmation preview identifier collision")
	}
	if index.maxEntries <= 0 || len(index.owners) >= index.maxEntries {
		return operation.New(operation.KindLimit, "confirmation preview routing limit reached")
	}
	index.owners[id] = owner
	return nil
}

// resolve releases its lock before the owning store claims the preview.
func (index *confirmationOwnerIndex) resolve(id string) (string, bool) {
	if index == nil {
		return "", false
	}
	index.mu.Lock()
	defer index.mu.Unlock()
	owner, ok := index.owners[id]
	return owner, ok
}
func (index *confirmationOwnerIndex) forget(id, owner string) bool {
	if index == nil {
		return false
	}
	index.mu.Lock()
	defer index.mu.Unlock()
	current, exists := index.owners[id]
	if !exists || current != owner {
		return false
	}
	delete(index.owners, id)
	return true
}

// confirmationOwnerCapacity bounds the routing index to the sum of all
// owning preview-store admission capacities without integer overflow.
func confirmationOwnerCapacity(capacities ...int) int {
	maxInt := int(^uint(0) >> 1)
	total := 0
	for _, capacity := range capacities {
		if capacity <= 0 {
			continue
		}
		if capacity > maxInt-total {
			return maxInt
		}
		total += capacity
	}
	return total
}

func validConfirmationOwnerID(id string) bool {
	if len(id) != 64 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
