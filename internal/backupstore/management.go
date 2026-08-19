package backupstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zoster81/scripthold/internal/operation"
)

const (
	defaultListPageSize       = 50
	maxListPageSize           = 100
	maxListCursorBytes        = 2048
	maxListCursorDecodedBytes = maxListCursorBytes / 4 * 3
	maxListCursorPayloadBytes = maxListCursorDecodedBytes - sha256.Size
	listCursorVersion         = "backup-list-cursor-v1"
)

// StoreStatus is the redacted verified state returned by the internal
// read-only management surface. It deliberately excludes StoreID and paths.
type StoreStatus struct {
	FormatVersion     string       `json:"formatVersion"`
	ManifestVersion   string       `json:"manifestVersion"`
	IndexVersion      string       `json:"indexVersion"`
	ObjectAlgorithm   string       `json:"objectAlgorithm"`
	Healthy           bool         `json:"healthy"`
	Generation        string       `json:"generation"`
	TotalObjectBytes  int64        `json:"totalObjectBytes"`
	ObjectCount       int          `json:"objectCount"`
	ManifestCount     int          `json:"manifestCount"`
	PinnedCount       int          `json:"pinnedCount"`
	OrphanObjectCount int          `json:"orphanObjectCount"`
	StagingEntryCount int          `json:"stagingEntryCount"`
	TrashEntryCount   int          `json:"trashEntryCount"`
	Limits            Limits       `json:"limits"`
	Issues            []AuditIssue `json:"issues,omitempty"`
}

func (status StoreStatus) String() string {
	encoded, _ := json.Marshal(status)
	return string(encoded)
}

// ListOptions selects one deterministic newest-first page.
type ListOptions struct {
	Cursor          string
	Limit           int
	TargetPath      string
	Pinned          *bool
	VisibilityScope string
	TargetVisible   func(string) bool
}

// ListResult is bound to one store generation and exact filter set.
type ListResult struct {
	Generation string            `json:"generation"`
	Items      []ManifestSummary `json:"items"`
	NextCursor string            `json:"nextCursor,omitempty"`
}

// InspectOptions applies caller authorization after strict manifest parsing but
// before the referenced object is hashed. The callback runs outside store locks.
type InspectOptions struct {
	AuthorizeTarget func(string) error
}

// InspectResult contains verified metadata only. Object bytes and internal
// store paths are never returned.
type InspectResult struct {
	Manifest       Manifest `json:"manifest"`
	ObjectVerified bool     `json:"objectVerified"`
}

func (result InspectResult) String() string {
	redacted := struct {
		BackupID           string          `json:"backupId"`
		CreatedAt          string          `json:"createdAt"`
		TargetPath         string          `json:"targetPath"`
		SourceOperation    SourceOperation `json:"sourceOperation"`
		ObjectAlgorithm    string          `json:"objectAlgorithm"`
		ObjectDigest       string          `json:"objectDigest"`
		ObjectBytes        int64           `json:"objectBytes"`
		ContentFingerprint string          `json:"contentFingerprint"`
		OriginalMode       uint32          `json:"originalMode"`
		OriginalModTime    string          `json:"originalModTime"`
		Label              string          `json:"label,omitempty"`
		Pinned             bool            `json:"pinned"`
		ManifestChecksum   string          `json:"manifestChecksum"`
		ObjectVerified     bool            `json:"objectVerified"`
	}{
		BackupID:           result.Manifest.BackupID,
		CreatedAt:          result.Manifest.CreatedAt,
		TargetPath:         result.Manifest.TargetPath,
		SourceOperation:    result.Manifest.SourceOperation,
		ObjectAlgorithm:    result.Manifest.ObjectAlgorithm,
		ObjectDigest:       result.Manifest.ObjectDigest,
		ObjectBytes:        result.Manifest.ObjectBytes,
		ContentFingerprint: result.Manifest.ContentFingerprint,
		OriginalMode:       result.Manifest.OriginalMode,
		OriginalModTime:    result.Manifest.OriginalModTime,
		Label:              result.Manifest.Label,
		Pinned:             result.Manifest.Pinned,
		ManifestChecksum:   result.Manifest.ManifestChecksum,
		ObjectVerified:     result.ObjectVerified,
	}
	encoded, _ := json.Marshal(redacted)
	return string(encoded)
}

type listCursorPayload struct {
	Version        string `json:"version"`
	Generation     string `json:"generation"`
	FilterHash     string `json:"filterHash"`
	AfterCreatedAt string `json:"afterCreatedAt"`
	AfterBackupID  string `json:"afterBackupId"`
}

// Status verifies current structure and returns redacted aggregate state.
func (store *Store) Status(ctx context.Context) (StoreStatus, error) {
	if store == nil {
		return StoreStatus{}, operation.New(operation.KindInvalidInput, "backup store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	if store.isClosed() {
		return StoreStatus{}, operation.New(operation.KindConflict, "backup store is closed")
	}
	if err := store.validateRootIdentity(); err != nil {
		return StoreStatus{}, err
	}
	scan, err := scanStore(ctx, store.root, store.descriptor, scanOptions{
		mode:       AuditQuick,
		maxObjects: store.limits.MaxManifests,
		maxBytes:   store.limits.MaxTotalBytes,
		checkIndex: true,
	})
	if err != nil {
		return StoreStatus{}, err
	}
	index := store.Index()
	return StoreStatus{
		FormatVersion:     FormatVersion,
		ManifestVersion:   ManifestVersion,
		IndexVersion:      IndexVersion,
		ObjectAlgorithm:   ObjectAlgorithm,
		Healthy:           scan.report.Healthy,
		Generation:        scan.report.Generation,
		TotalObjectBytes:  index.TotalObjectBytes,
		ObjectCount:       index.ObjectCount,
		ManifestCount:     index.ManifestCount,
		PinnedCount:       index.PinnedCount,
		OrphanObjectCount: scan.report.OrphanObjectCount,
		StagingEntryCount: scan.report.StagingEntryCount,
		TrashEntryCount:   scan.report.TrashEntryCount,
		Limits:            store.limits,
		Issues:            append([]AuditIssue(nil), scan.report.Issues...),
	}, nil
}

// List returns a deterministic newest-first page. Cursors are authenticated,
// generation-bound, and bound to the exact target/pinned filters.
func (store *Store) List(ctx context.Context, options ListOptions) (ListResult, error) {
	if store == nil {
		return ListResult{}, operation.New(operation.KindInvalidInput, "backup store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	limit := options.Limit
	if limit == 0 {
		limit = defaultListPageSize
	}
	if limit < 0 {
		return ListResult{}, operation.New(operation.KindInvalidInput, "backup list limit must not be negative")
	}
	if limit > maxListPageSize {
		return ListResult{}, operation.New(operation.KindLimit, "backup list limit exceeds the maximum page size")
	}
	if len(options.Cursor) > maxListCursorBytes {
		return ListResult{}, operation.New(operation.KindLimit, "backup list cursor exceeds the maximum encoded size")
	}
	if options.TargetPath != "" {
		clean := filepath.Clean(options.TargetPath)
		if strings.Contains(options.TargetPath, "\x00") || !filepath.IsAbs(clean) || clean != options.TargetPath {
			return ListResult{}, operation.New(operation.KindInvalidPath, "backup list target path must be normalized and absolute")
		}
	}
	filterHash := backupListFilterHash(options.TargetPath, options.Pinned, options.VisibilityScope)

	index, startPosition, err := func() (Index, int, error) {
		store.transactionMu.Lock()
		defer store.transactionMu.Unlock()
		if store.isClosed() {
			return Index{}, 0, operation.New(operation.KindConflict, "backup store is closed")
		}
		if err := store.validateIdentityAndLayout(); err != nil {
			return Index{}, 0, err
		}
		if err := ctx.Err(); err != nil {
			return Index{}, 0, operation.Wrap(operation.KindCancelled, "list_backup_store", "", err)
		}
		index := store.Index()
		if options.Cursor == "" {
			return index, len(index.Manifests) - 1, nil
		}
		cursor, err := store.decodeListCursor(options.Cursor)
		if err != nil {
			return Index{}, 0, err
		}
		if cursor.FilterHash != filterHash {
			return Index{}, 0, operation.New(operation.KindInvalidInput, "backup list cursor does not match the requested filters")
		}
		if cursor.Generation != index.Generation {
			return Index{}, 0, operation.New(operation.KindConflict, "backup list cursor is stale")
		}
		position := sort.Search(len(index.Manifests), func(position int) bool {
			item := index.Manifests[position]
			if item.CreatedAt != cursor.AfterCreatedAt {
				return item.CreatedAt >= cursor.AfterCreatedAt
			}
			return item.BackupID >= cursor.AfterBackupID
		})
		if position >= len(index.Manifests) || index.Manifests[position].CreatedAt != cursor.AfterCreatedAt ||
			index.Manifests[position].BackupID != cursor.AfterBackupID {
			return Index{}, 0, operation.New(operation.KindConflict, "backup list cursor no longer identifies its last record")
		}
		return index, position - 1, nil
	}()
	if err != nil {
		return ListResult{}, err
	}

	filtered := make([]ManifestSummary, 0, min(len(index.Manifests), limit+1))
	for position := startPosition; position >= 0; position-- {
		if err := ctx.Err(); err != nil {
			return ListResult{}, operation.Wrap(operation.KindCancelled, "list_backup_store", "", err)
		}
		item := index.Manifests[position]
		if options.TargetPath != "" && item.TargetPath != options.TargetPath {
			continue
		}
		if options.Pinned != nil && item.Pinned != *options.Pinned {
			continue
		}
		if options.TargetVisible != nil && !options.TargetVisible(item.TargetPath) {
			continue
		}
		filtered = append(filtered, item)
		if len(filtered) > limit {
			break
		}
	}

	result := ListResult{Generation: index.Generation}
	hasMore := len(filtered) > limit
	if hasMore {
		filtered = filtered[:limit]
	}
	result.Items = filtered
	if hasMore {
		last := filtered[len(filtered)-1]
		cursor, err := store.encodeListCursor(listCursorPayload{
			Version:        listCursorVersion,
			Generation:     index.Generation,
			FilterHash:     filterHash,
			AfterCreatedAt: last.CreatedAt,
			AfterBackupID:  last.BackupID,
		})
		if err != nil {
			return ListResult{}, err
		}
		result.NextCursor = cursor
	}
	return result, nil
}

// Inspect verifies one immutable manifest and hashes its referenced object only
// after caller authorization succeeds.
func (store *Store) Inspect(ctx context.Context, backupID string, options InspectOptions) (InspectResult, error) {
	if store == nil {
		return InspectResult{}, operation.New(operation.KindInvalidInput, "backup store is unavailable")
	}
	if !validHexIdentifier(backupID) {
		return InspectResult{}, operation.New(operation.KindInvalidInput, "backup identifier is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	manifest, err := store.readInspectManifest(ctx, backupID)
	if err != nil {
		return InspectResult{}, err
	}
	if options.AuthorizeTarget != nil {
		if err := options.AuthorizeTarget(manifest.TargetPath); err != nil {
			return InspectResult{}, err
		}
	}

	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	if store.isClosed() {
		return InspectResult{}, operation.New(operation.KindConflict, "backup store is closed")
	}
	if err := store.validateIdentityAndLayout(); err != nil {
		return InspectResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return InspectResult{}, operation.Wrap(operation.KindCancelled, "inspect_backup_store", "", err)
	}
	path := manifestPath(store.root, backupID)
	info, err := os.Lstat(path)
	if err != nil {
		return InspectResult{}, operation.New(operation.KindConflict, "backup manifest changed during inspection")
	}
	currentManifest, err := readManifest(path, info, store.descriptor)
	if err != nil || currentManifest != manifest {
		return InspectResult{}, operation.New(operation.KindConflict, "backup manifest changed during inspection")
	}
	object := objectPath(store.root, manifest.ObjectDigest)
	objectInfo, err := os.Lstat(object)
	if err != nil {
		return InspectResult{}, operation.New(operation.KindFilesystem, "referenced backup object is unavailable")
	}
	if err := verifyExistingObject(ctx, object, objectInfo, manifest.ObjectDigest, manifest.ObjectBytes); err != nil {
		return InspectResult{}, err
	}
	return InspectResult{Manifest: manifest, ObjectVerified: true}, nil
}

func (store *Store) readInspectManifest(ctx context.Context, backupID string) (Manifest, error) {
	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	if store.isClosed() {
		return Manifest{}, operation.New(operation.KindConflict, "backup store is closed")
	}
	if err := store.validateIdentityAndLayout(); err != nil {
		return Manifest{}, err
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, operation.Wrap(operation.KindCancelled, "inspect_backup_store", "", err)
	}
	path := manifestPath(store.root, backupID)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return Manifest{}, operation.New(operation.KindInvalidInput, "backup identifier was not found")
	}
	if err != nil {
		return Manifest{}, sanitizedFilesystemError("backup manifest cannot be inspected", err)
	}
	manifest, err := readManifest(path, info, store.descriptor)
	if err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func backupListFilterHash(targetPath string, pinned *bool, visibilityScope string) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("mcp-file-tools:backup-list-filter-v1\x00"))
	_, _ = hasher.Write([]byte(targetPath))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(visibilityScope))
	if pinned == nil {
		_, _ = hasher.Write([]byte{0})
	} else if *pinned {
		_, _ = hasher.Write([]byte{2})
	} else {
		_, _ = hasher.Write([]byte{1})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func (store *Store) encodeListCursor(payload listCursorPayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", operation.New(operation.KindFilesystem, "backup list cursor could not be encoded")
	}
	if len(data) > maxListCursorPayloadBytes {
		return "", operation.New(operation.KindLimit, "backup list cursor exceeds the maximum encoded size")
	}
	mac := hmac.New(sha256.New, store.cursorKey())
	_, _ = mac.Write(data)
	tag := mac.Sum(nil)
	encoded := make([]byte, 0, maxListCursorDecodedBytes)
	encoded = append(encoded, data...)
	encoded = append(encoded, tag...)
	cursor := base64.RawURLEncoding.EncodeToString(encoded)
	if len(cursor) > maxListCursorBytes {
		return "", operation.New(operation.KindLimit, "backup list cursor exceeds the maximum encoded size")
	}
	return cursor, nil
}

func (store *Store) decodeListCursor(cursor string) (listCursorPayload, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || len(decoded) <= sha256.Size || base64.RawURLEncoding.EncodeToString(decoded) != cursor {
		return listCursorPayload{}, operation.New(operation.KindInvalidInput, "backup list cursor is malformed")
	}
	data := decoded[:len(decoded)-sha256.Size]
	tag := decoded[len(decoded)-sha256.Size:]
	mac := hmac.New(sha256.New, store.cursorKey())
	_, _ = mac.Write(data)
	if !hmac.Equal(tag, mac.Sum(nil)) {
		return listCursorPayload{}, operation.New(operation.KindInvalidInput, "backup list cursor authentication failed")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var payload listCursorPayload
	if err := decoder.Decode(&payload); err != nil {
		return listCursorPayload{}, operation.New(operation.KindInvalidInput, "backup list cursor is malformed")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return listCursorPayload{}, operation.New(operation.KindInvalidInput, "backup list cursor contains trailing data")
	}
	if payload.Version != listCursorVersion || !validHexIdentifier(payload.Generation) ||
		!validHexIdentifier(payload.FilterHash) || !validHexIdentifier(payload.AfterBackupID) {
		return listCursorPayload{}, operation.New(operation.KindInvalidInput, "backup list cursor fields are invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, payload.AfterCreatedAt); err != nil {
		return listCursorPayload{}, operation.New(operation.KindInvalidInput, "backup list cursor timestamp is invalid")
	}
	return payload, nil
}

func (store *Store) cursorKey() []byte {
	digest := sha256.Sum256([]byte("mcp-file-tools:backup-list-cursor-key-v1\x00" + store.descriptor.StoreID))
	return digest[:]
}

func min(first, second int) int {
	if first < second {
		return first
	}
	return second
}
