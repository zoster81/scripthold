package backupstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

const (
	gcMinimumVersionsPerTarget = 1
	gcManifestTrashPrefix      = "gc-manifest-"
	gcObjectTrashPrefix        = "gc-object-"
)

type gcTrashEntry struct {
	kind    string
	id      string
	path    string
	bytes   int64
	removed bool
}

// GCPlanTTL returns the configured one-shot GC plan lifetime.
func (store *Store) GCPlanTTL() time.Duration {
	if store == nil || store.limits.PlanTTLSeconds <= 0 {
		return 0
	}
	return time.Duration(store.limits.PlanTTLSeconds) * time.Second
}

// PlanGC computes one deterministic generation-bound plan without modifying the
// store. Active capture reservations make the snapshot ineligible for planning.
func (store *Store) PlanGC(ctx context.Context, options GCOptions) (GCPlan, error) {
	if store == nil {
		return GCPlan{}, operation.New(operation.KindInvalidInput, "backup store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	plannedAt := options.Now
	if plannedAt.IsZero() {
		plannedAt = time.Now()
	}
	plannedAt = plannedAt.UTC()
	if err := ctx.Err(); err != nil {
		return GCPlan{}, operation.Wrap(operation.KindCancelled, "plan_backup_gc", "", err)
	}

	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	if store.isClosed() {
		return GCPlan{}, operation.New(operation.KindConflict, "backup store is closed")
	}
	if err := store.validateIdentityAndLayout(); err != nil {
		return GCPlan{}, err
	}
	index, activeManifests, activeObjects, err := store.gcPlanningSnapshot(ctx)
	if err != nil {
		return GCPlan{}, err
	}
	return buildGCPlan(index, store.limits, plannedAt, activeManifests, activeObjects)
}

// ApplyGC revalidates the complete retained plan and performs manifest-first
// namespace removal. Trash deletion is best effort and any cleanup error is
// surfaced together with durable progress evidence.
func (store *Store) ApplyGC(ctx context.Context, plan GCPlan) (result GCResult, err error) {
	if store == nil {
		return GCResult{}, operation.New(operation.KindInvalidInput, "backup store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	plannedAt, err := validateGCPlan(plan)
	if err != nil {
		return GCResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return GCResult{}, operation.Wrap(operation.KindCancelled, "apply_backup_gc", "", err)
	}

	store.transactionMu.Lock()
	defer store.transactionMu.Unlock()
	if store.isClosed() {
		return GCResult{}, operation.New(operation.KindConflict, "backup store is closed")
	}
	if err := store.validateIdentityAndLayout(); err != nil {
		return GCResult{}, err
	}
	if err := store.beginGC(); err != nil {
		return GCResult{}, err
	}
	defer store.endGC()

	index, activeManifests, activeObjects, err := store.gcAuthoritativeSnapshot(ctx)
	if err != nil {
		return GCResult{}, err
	}
	currentPlan, err := buildGCPlan(index, store.limits, plannedAt, activeManifests, activeObjects)
	if err != nil {
		return GCResult{}, err
	}
	if plan.Generation != index.Generation || !gcPlansEqual(plan, currentPlan) {
		return GCResult{}, operation.New(operation.KindConflict, "backup GC plan is stale")
	}

	result.PreviousGeneration = plan.Generation
	result.Generation = plan.Generation
	if len(plan.Manifests) == 0 && len(plan.Objects) == 0 {
		return result, nil
	}

	var trash []gcTrashEntry
	durableStateChanged := false
	indexRefreshed := false
	defer func() {
		if durableStateChanged && !indexRefreshed {
			refreshErr := store.refreshDerivedIndex(context.Background())
			err = errors.Join(err, refreshErr)
			result.Generation = store.Index().Generation
		}
	}()

	for _, candidate := range plan.Manifests {
		if err := ctx.Err(); err != nil {
			return result, operation.Wrap(operation.KindCancelled, "trash_backup_manifest", "", err)
		}
		source := manifestPath(store.root, candidate.BackupID)
		info, statErr := os.Lstat(source)
		if statErr != nil {
			return result, operation.New(operation.KindConflict, "backup manifest changed after GC dry run")
		}
		manifest, readErr := readManifest(source, info, store.descriptor)
		if readErr != nil || !gcManifestMatchesCandidate(manifest, candidate) {
			return result, operation.New(operation.KindConflict, "backup manifest changed after GC dry run")
		}
		destination := gcManifestTrashPath(store.root, candidate.BackupID)
		moved, moveErr := store.ops.moveGCEntry(source, destination, info, "backup manifest")
		if moved {
			durableStateChanged = true
			result.ManifestsRemoved++
			trash = append(trash, gcTrashEntry{kind: "manifest", id: candidate.BackupID, path: destination})
		}
		if moveErr != nil {
			return result, moveErr
		}
	}

	postManifestIndex, _, _, scanErr := store.gcAuthoritativeSnapshot(ctx)
	if scanErr != nil {
		return result, scanErr
	}
	postReferences := make(map[string]int, len(postManifestIndex.Objects))
	for _, object := range postManifestIndex.Objects {
		postReferences[object.Digest] = object.References
	}
	for _, candidate := range plan.Objects {
		if postReferences[candidate.Digest] != 0 {
			return result, operation.New(operation.KindConflict, "backup object gained a live reference during GC")
		}
		if err := ctx.Err(); err != nil {
			return result, operation.Wrap(operation.KindCancelled, "trash_backup_object", "", err)
		}
		source := objectPath(store.root, candidate.Digest)
		info, statErr := os.Lstat(source)
		if statErr != nil {
			return result, operation.New(operation.KindConflict, "backup object changed after GC dry run")
		}
		if verifyErr := verifyExistingObject(ctx, source, info, candidate.Digest, candidate.Bytes); verifyErr != nil {
			return result, verifyErr
		}
		destination := gcObjectTrashPath(store.root, candidate.Digest)
		moved, moveErr := store.ops.moveGCEntry(source, destination, info, "backup object")
		if moved {
			durableStateChanged = true
			result.ObjectsRemoved++
			trash = append(trash, gcTrashEntry{kind: "object", id: candidate.Digest, path: destination, bytes: candidate.Bytes})
		}
		if moveErr != nil {
			return result, moveErr
		}
	}

	for index := range trash {
		entry := &trash[index]
		removed, removeErr := store.ops.removeGCTrashEntry(entry.path)
		entry.removed = removed
		if removed && entry.kind == "object" {
			if !addNonNegativeInt64(&result.BytesReclaimed, entry.bytes) {
				err = errors.Join(err, operation.New(operation.KindLimit, "reclaimed backup bytes exceed the supported range"))
			}
		}
		if removeErr != nil {
			result.TrashCleanupFailures++
			err = errors.Join(err, removeErr)
		}
	}
	for _, entry := range trash {
		if !entry.removed {
			result.TrashEntriesRemaining++
		}
	}

	refreshErr := store.refreshDerivedIndex(ctx)
	result.Generation = store.Index().Generation
	indexRefreshed = refreshErr == nil
	if refreshErr != nil {
		err = errors.Join(err, refreshErr)
	}
	return result, err
}

func (store *Store) gcPlanningSnapshot(ctx context.Context) (Index, map[string]int, map[string]int, error) {
	index, activeManifests, activeObjects, err := store.gcAuthoritativeSnapshot(ctx)
	if err != nil {
		return Index{}, nil, nil, err
	}
	store.stateMu.RLock()
	defer store.stateMu.RUnlock()
	if store.gcActive || store.hasReservationsLocked() {
		return Index{}, nil, nil, operation.New(operation.KindConflict, "backup GC requires no active capture reservations")
	}
	return index, activeManifests, activeObjects, nil
}

func (store *Store) gcAuthoritativeSnapshot(ctx context.Context) (Index, map[string]int, map[string]int, error) {
	if err := ctx.Err(); err != nil {
		return Index{}, nil, nil, operation.Wrap(operation.KindCancelled, "scan_backup_gc", "", err)
	}
	scan, err := scanStore(ctx, store.root, store.descriptor, scanOptions{
		mode:       AuditQuick,
		maxObjects: store.limits.MaxManifests,
		maxBytes:   store.limits.MaxTotalBytes,
		checkIndex: false,
	})
	if err != nil {
		return Index{}, nil, nil, err
	}
	if structuralErr := firstStructuralIssue(scan.report); structuralErr != nil {
		return Index{}, nil, nil, structuralErr
	}
	store.stateMu.RLock()
	activeManifests := cloneCountMap(store.activeRestoreManifests)
	activeObjects := cloneCountMap(store.activeRestoreObjects)
	store.stateMu.RUnlock()
	return buildIndex(store.descriptor, scan.manifests, scan.objects), activeManifests, activeObjects, nil
}

func (store *Store) beginGC() error {
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	if store.closed {
		return operation.New(operation.KindConflict, "backup store is closed")
	}
	if store.gcActive || store.hasReservationsLocked() {
		return operation.New(operation.KindConflict, "backup GC requires no active capture reservations")
	}
	store.gcActive = true
	return nil
}

func (store *Store) endGC() {
	store.stateMu.Lock()
	store.gcActive = false
	store.stateMu.Unlock()
}

func (store *Store) hasReservationsLocked() bool {
	if store.reservedBytes != 0 || store.reservedManifests != 0 || store.reservedPinned != 0 {
		return true
	}
	for _, count := range store.reservedTargets {
		if count > 0 {
			return true
		}
	}
	return false
}

func buildGCPlan(index Index, limits Limits, plannedAt time.Time, activeManifests, activeObjects map[string]int) (GCPlan, error) {
	plannedAt = plannedAt.UTC()
	plan := GCPlan{
		PlannedAt:                utcTimestamp(plannedAt),
		Generation:               index.Generation,
		RetentionDays:            limits.RetentionDays,
		MinimumVersionsPerTarget: gcMinimumVersionsPerTarget,
	}
	cutoff := plannedAt.AddDate(0, 0, -limits.RetentionDays)
	groups := make(map[string][]ManifestSummary)
	for _, manifest := range index.Manifests {
		groups[manifest.TargetPath] = append(groups[manifest.TargetPath], manifest)
	}

	for _, manifests := range groups {
		remaining := len(manifests)
		unpinned := 0
		for _, manifest := range manifests {
			if !manifest.Pinned {
				unpinned++
			}
		}
		versionNeeded := max(0, unpinned-limits.MaxVersionsPerTarget)
		for _, manifest := range manifests {
			createdAt, err := time.Parse(time.RFC3339Nano, manifest.CreatedAt)
			if err != nil {
				return GCPlan{}, operation.New(operation.KindFilesystem, "backup manifest timestamp is invalid during GC planning")
			}
			reasons := make([]GCReason, 0, 2)
			if createdAt.Before(cutoff) {
				reasons = append(reasons, GCReasonRetention)
			}
			if versionNeeded > 0 {
				reasons = append(reasons, GCReasonVersionLimit)
			}
			if manifest.Pinned || activeManifests[manifest.BackupID] > 0 || remaining <= gcMinimumVersionsPerTarget || len(reasons) == 0 {
				continue
			}
			plan.Manifests = append(plan.Manifests, GCManifestCandidate{
				BackupID:     manifest.BackupID,
				CreatedAt:    manifest.CreatedAt,
				TargetPath:   manifest.TargetPath,
				ObjectDigest: manifest.ObjectDigest,
				ObjectBytes:  manifest.ObjectBytes,
				Pinned:       manifest.Pinned,
				Reasons:      reasons,
			})
			remaining--
			if versionNeeded > 0 {
				versionNeeded--
			}
		}
	}
	sort.Slice(plan.Manifests, func(i, j int) bool {
		if plan.Manifests[i].CreatedAt != plan.Manifests[j].CreatedAt {
			return plan.Manifests[i].CreatedAt < plan.Manifests[j].CreatedAt
		}
		return plan.Manifests[i].BackupID < plan.Manifests[j].BackupID
	})

	remainingReferences := make(map[string]int, len(index.Objects))
	objects := make(map[string]ObjectSummary, len(index.Objects))
	for _, object := range index.Objects {
		remainingReferences[object.Digest] = object.References
		objects[object.Digest] = object
	}
	for _, manifest := range plan.Manifests {
		remainingReferences[manifest.ObjectDigest]--
	}
	digests := make([]string, 0, len(objects))
	for digest := range objects {
		digests = append(digests, digest)
	}
	sort.Strings(digests)
	for _, digest := range digests {
		object := objects[digest]
		if remainingReferences[digest] != 0 || activeObjects[digest] > 0 {
			continue
		}
		plan.Objects = append(plan.Objects, GCObjectCandidate{
			Digest:           digest,
			Bytes:            object.Bytes,
			ReferencesBefore: object.References,
		})
		if !addNonNegativeInt64(&plan.ReclaimableBytes, object.Bytes) {
			return GCPlan{}, operation.New(operation.KindLimit, "GC reclaimable bytes exceed the supported range")
		}
	}
	plan.ManifestCount = len(plan.Manifests)
	plan.ObjectCount = len(plan.Objects)
	return plan, nil
}

func validateGCPlan(plan GCPlan) (time.Time, error) {
	plannedAt, err := time.Parse(time.RFC3339Nano, plan.PlannedAt)
	if err != nil || plannedAt.Location() != time.UTC || !strings.HasSuffix(plan.PlannedAt, "Z") {
		return time.Time{}, operation.New(operation.KindInvalidInput, "backup GC plan timestamp is invalid")
	}
	if !validHexIdentifier(plan.Generation) || plan.RetentionDays <= 0 || plan.MinimumVersionsPerTarget != gcMinimumVersionsPerTarget ||
		plan.ManifestCount != len(plan.Manifests) || plan.ObjectCount != len(plan.Objects) || plan.ReclaimableBytes < 0 {
		return time.Time{}, operation.New(operation.KindInvalidInput, "backup GC plan fields are invalid")
	}
	manifestIDs := make(map[string]struct{}, len(plan.Manifests))
	for index, candidate := range plan.Manifests {
		if !validHexIdentifier(candidate.BackupID) || !validHexIdentifier(candidate.ObjectDigest) || candidate.ObjectBytes < 0 || candidate.Pinned || len(candidate.Reasons) == 0 {
			return time.Time{}, operation.New(operation.KindInvalidInput, "backup GC manifest candidate is invalid")
		}
		if _, duplicate := manifestIDs[candidate.BackupID]; duplicate {
			return time.Time{}, operation.New(operation.KindInvalidInput, "backup GC plan contains duplicate manifests")
		}
		manifestIDs[candidate.BackupID] = struct{}{}
		if index > 0 && gcManifestCandidateLess(candidate, plan.Manifests[index-1]) {
			return time.Time{}, operation.New(operation.KindInvalidInput, "backup GC manifest candidates are not ordered")
		}
		for reasonIndex, reason := range candidate.Reasons {
			if reason != GCReasonRetention && reason != GCReasonVersionLimit {
				return time.Time{}, operation.New(operation.KindInvalidInput, "backup GC reason is invalid")
			}
			if reasonIndex > 0 && candidate.Reasons[reasonIndex-1] >= reason {
				return time.Time{}, operation.New(operation.KindInvalidInput, "backup GC reasons are not canonical")
			}
		}
	}
	objectIDs := make(map[string]struct{}, len(plan.Objects))
	var reclaimable int64
	for index, candidate := range plan.Objects {
		if !validHexIdentifier(candidate.Digest) || candidate.Bytes < 0 || candidate.ReferencesBefore < 0 {
			return time.Time{}, operation.New(operation.KindInvalidInput, "backup GC object candidate is invalid")
		}
		if _, duplicate := objectIDs[candidate.Digest]; duplicate {
			return time.Time{}, operation.New(operation.KindInvalidInput, "backup GC plan contains duplicate objects")
		}
		objectIDs[candidate.Digest] = struct{}{}
		if index > 0 && plan.Objects[index-1].Digest >= candidate.Digest {
			return time.Time{}, operation.New(operation.KindInvalidInput, "backup GC object candidates are not ordered")
		}
		if !addNonNegativeInt64(&reclaimable, candidate.Bytes) {
			return time.Time{}, operation.New(operation.KindLimit, "backup GC bytes exceed the supported range")
		}
	}
	if reclaimable != plan.ReclaimableBytes {
		return time.Time{}, operation.New(operation.KindInvalidInput, "backup GC reclaimable byte count is inconsistent")
	}
	return plannedAt, nil
}

func gcManifestCandidateLess(first, second GCManifestCandidate) bool {
	if first.CreatedAt != second.CreatedAt {
		return first.CreatedAt < second.CreatedAt
	}
	return first.BackupID < second.BackupID
}

func gcPlansEqual(first, second GCPlan) bool {
	return reflect.DeepEqual(first, second)
}

func gcManifestMatchesCandidate(manifest Manifest, candidate GCManifestCandidate) bool {
	return manifest.BackupID == candidate.BackupID && manifest.CreatedAt == candidate.CreatedAt &&
		manifest.TargetPath == candidate.TargetPath && manifest.ObjectDigest == candidate.ObjectDigest &&
		manifest.ObjectBytes == candidate.ObjectBytes && manifest.Pinned == candidate.Pinned
}

func moveGCEntry(source, destination string, expected os.FileInfo, description string) (bool, error) {
	if expected == nil || !expected.Mode().IsRegular() || isLinkOrReparse(expected) {
		return false, operation.New(operation.KindConflict, description+" identity is invalid")
	}
	moveErr := filesystem.MoveNoReplace(source, destination)
	destinationInfo, destinationErr := os.Lstat(destination)
	_, sourceErr := os.Lstat(source)
	moved := destinationErr == nil && os.IsNotExist(sourceErr) && os.SameFile(expected, destinationInfo)
	if !moved {
		if moveErr != nil {
			return false, sanitizedFilesystemError(description+" could not be moved to GC trash", moveErr)
		}
		return false, operation.New(operation.KindConflict, description+" move outcome is uncertain")
	}
	if isLinkOrReparse(destinationInfo) || !destinationInfo.Mode().IsRegular() || validateSingleLink(destination, destinationInfo) != nil ||
		validatePathPermissions(destination, false) != nil {
		return true, operation.New(operation.KindFilesystem, description+" GC trash identity is invalid")
	}
	if moveErr != nil {
		return true, sanitizedFilesystemError(description+" was moved but its directories could not be synchronized", moveErr)
	}
	return true, nil
}

func removeGCTrashEntry(path string) (bool, error) {
	snapshot, err := filesystem.CaptureSnapshot(path)
	if err != nil {
		return false, sanitizedFilesystemError("GC trash entry cannot be inspected", err)
	}
	if !snapshot.Exists {
		return true, nil
	}
	removeErr := filesystem.RemoveFile(path, &snapshot)
	_, statErr := os.Lstat(path)
	removed := os.IsNotExist(statErr)
	if removeErr != nil {
		return removed, sanitizedFilesystemError("GC trash entry could not be removed durably", removeErr)
	}
	if !removed {
		return false, operation.New(operation.KindConflict, "GC trash entry still exists after removal")
	}
	return true, nil
}

func gcManifestTrashPath(root, backupID string) string {
	return filepath.Join(root, "trash", gcManifestTrashPrefix+backupID+".json")
}

func gcObjectTrashPath(root, digest string) string {
	return filepath.Join(root, "trash", gcObjectTrashPrefix+digest)
}

func (store *Store) recoverGCTrash(ctx context.Context, liveManifests []Manifest) (bool, error) {
	trashRoot := filepath.Join(store.root, "trash")
	limit := store.limits.MaxManifests
	if limit > int(^uint(0)>>1)/2 {
		limit = int(^uint(0) >> 1)
	} else {
		limit *= 2
	}
	entries, overflow, err := readDirectoryBounded(trashRoot, limit)
	if err != nil {
		return false, sanitizedFilesystemError("backup GC trash cannot be inspected", err)
	}
	if overflow {
		return false, nil
	}
	references := make(map[string]int)
	for _, manifest := range liveManifests {
		references[manifest.ObjectDigest]++
	}
	changed := false
	var objectBytes int64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return changed, operation.Wrap(operation.KindCancelled, "recover_backup_gc", "", err)
		}
		name := entry.Name()
		path := filepath.Join(trashRoot, name)
		info, statErr := os.Lstat(path)
		if statErr != nil || isLinkOrReparse(info) || !info.Mode().IsRegular() {
			continue
		}
		switch {
		case strings.HasPrefix(name, gcManifestTrashPrefix) && strings.HasSuffix(name, ".json"):
			backupID := strings.TrimSuffix(strings.TrimPrefix(name, gcManifestTrashPrefix), ".json")
			if !validHexIdentifier(backupID) {
				continue
			}
			manifest, readErr := readManifest(path, info, store.descriptor)
			if readErr != nil || manifest.BackupID != backupID {
				continue
			}
		case strings.HasPrefix(name, gcObjectTrashPrefix):
			digest := strings.TrimPrefix(name, gcObjectTrashPrefix)
			if !validHexIdentifier(digest) {
				continue
			}
			if references[digest] != 0 {
				continue
			}
			if !addNonNegativeInt64(&objectBytes, info.Size()) || objectBytes > store.limits.MaxTotalBytes {
				continue
			}
			if verifyErr := verifyExistingObject(ctx, path, info, digest, info.Size()); verifyErr != nil {
				continue
			}
		default:
			continue
		}
		removed, removeErr := removeGCTrashEntry(path)
		if removed {
			changed = true
		}
		if removeErr != nil {
			return changed, removeErr
		}
	}
	return changed, nil
}

func cloneCountMap(source map[string]int) map[string]int {
	cloned := make(map[string]int, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func (store *Store) retainRestoreReference(manifest Manifest) {
	store.stateMu.Lock()
	store.activeRestoreManifests[manifest.BackupID]++
	store.activeRestoreObjects[manifest.ObjectDigest]++
	store.stateMu.Unlock()
}

func (store *Store) releaseRestoreReference(manifest Manifest) {
	store.stateMu.Lock()
	decrementCountMap(store.activeRestoreManifests, manifest.BackupID)
	decrementCountMap(store.activeRestoreObjects, manifest.ObjectDigest)
	store.stateMu.Unlock()
}

func decrementCountMap(values map[string]int, key string) {
	values[key]--
	if values[key] <= 0 {
		delete(values, key)
	}
}
