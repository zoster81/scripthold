package responsecontinuation

import (
	"bytes"
	"testing"
	"time"
)

func TestStoreRetainAndReadChunksRoundTrip(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	store := NewStore(Limits{MaxEntries: 4, MaxTotalBytes: 4096, MaxChunkBytes: 7, Retention: time.Hour}, func() time.Time { return now })
	payload := []byte(`{"message":"héllo world","items":[1,2,3]}`)

	handle, err := store.Retain(payload, Metadata{OriginalIsError: true, ErrorCode: "PARTIAL_COMMIT"})
	if err != nil {
		t.Fatal(err)
	}
	if len(handle.OperationID) != 67 || handle.Status != StatusCompleted || handle.TotalBytes != len(payload) || !handle.OriginalIsError || handle.ErrorCode != "PARTIAL_COMMIT" {
		t.Fatalf("unexpected handle: %+v", handle)
	}

	var rebuilt []byte
	offset := 0
	for {
		chunk, err := store.Get(handle.OperationID, offset, 0)
		if err != nil {
			t.Fatal(err)
		}
		if chunk.Offset != offset || chunk.NextOffset < chunk.Offset || chunk.NextOffset > len(payload) {
			t.Fatalf("invalid chunk bounds: %+v", chunk)
		}
		rebuilt = append(rebuilt, []byte(chunk.Data)...)
		offset = chunk.NextOffset
		if chunk.Complete {
			break
		}
	}
	if !bytes.Equal(rebuilt, payload) {
		t.Fatalf("rebuilt payload = %q, want %q", rebuilt, payload)
	}
}

func TestStoreRejectsUnknownExpiredAndOversizedResults(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	store := NewStore(Limits{MaxEntries: 1, MaxTotalBytes: 8, MaxChunkBytes: 4, Retention: time.Second}, func() time.Time { return now })

	if _, err := store.Retain([]byte("123456789"), Metadata{}); err != ErrCapacity {
		t.Fatalf("oversized retain error=%v, want %v", err, ErrCapacity)
	}
	if _, err := store.Get("op_0000000000000000000000000000000000000000000000000000000000000000", 0, 0); err != ErrNotFound {
		t.Fatalf("unknown get error=%v, want %v", err, ErrNotFound)
	}
	handle, err := store.Retain([]byte("1234"), Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	if _, err := store.Get(handle.OperationID, 0, 0); err != ErrNotFound {
		t.Fatalf("expired get error=%v, want %v", err, ErrNotFound)
	}
}

func TestStoreValidatesOffsetsAndChunkLimits(t *testing.T) {
	store := NewStore(Limits{MaxEntries: 2, MaxTotalBytes: 128, MaxChunkBytes: 8, Retention: time.Hour}, time.Now)
	handle, err := store.Retain([]byte("abcdefghijkl"), Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(handle.OperationID, -1, 0); err != ErrInvalidInput {
		t.Fatalf("negative offset error=%v, want %v", err, ErrInvalidInput)
	}
	if _, err := store.Get(handle.OperationID, 99, 0); err != ErrInvalidInput {
		t.Fatalf("large offset error=%v, want %v", err, ErrInvalidInput)
	}
	chunk, err := store.Get(handle.OperationID, 0, 99)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunk.Data) > 8 || chunk.NextOffset > 8 {
		t.Fatalf("chunk exceeded configured maximum: %+v", chunk)
	}
}
