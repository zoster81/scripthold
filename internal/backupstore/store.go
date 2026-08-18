package backupstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

const (
	FormatVersion   = "backup-store-v1"
	ObjectAlgorithm = "sha256"
	ManifestVersion = "backup-manifest-v1"
	IndexVersion    = "backup-index-v1"

	maxDescriptorBytes = 64 * 1024
)

var expectedRootEntries = map[string]struct{}{
	"store.json": {},
	"store.lock": {},
	"objects":    {},
	"manifests":  {},
	"index":      {},
	"staging":    {},
	"trash":      {},
}

// Options configures one process-owned backup store.
type Options struct {
	Directory                string
	PublicAllowedDirectories []string
	Limits                   Limits
}

// Descriptor is the immutable store identity written to store.json.
type Descriptor struct {
	FormatVersion   string `json:"formatVersion"`
	StoreID         string `json:"storeId"`
	CreatedAt       string `json:"createdAt"`
	ObjectAlgorithm string `json:"objectAlgorithm"`
	ManifestVersion string `json:"manifestVersion"`
	IndexVersion    string `json:"indexVersion"`
}

// Store owns the exclusive process lock and immutable descriptor.
type Store struct {
	root       string
	rootInfo   fs.FileInfo
	descriptor Descriptor
	limits     Limits
	lock       *storeLock

	transactionMu sync.Mutex
	stateMu       sync.RWMutex
	index         Index
	closed        bool

	reservedBytes     int64
	reservedManifests int
	reservedPinned    int
	reservedTargets   map[string]int
	gcActive          bool

	activeRestoreManifests map[string]int
	activeRestoreObjects   map[string]int

	ops       storeOperations
	closeOnce sync.Once
	closeErr  error
}

// Open validates, creates, exclusively locks, and initializes one store.
func Open(options Options) (_ *Store, err error) {
	limits, err := normalizeLimits(options.Limits)
	if err != nil {
		return nil, err
	}
	root, err := validateDedicatedStorePath(options.Directory, options.PublicAllowedDirectories)
	if err != nil {
		return nil, err
	}
	if err := createDirectoryPath(root); err != nil {
		return nil, sanitizedFilesystemError("backup store directory could not be created", err)
	}
	root, err = validateDedicatedStorePath(root, options.PublicAllowedDirectories)
	if err != nil {
		return nil, err
	}

	lock, err := acquireStoreLock(filepath.Join(root, "store.lock"))
	if err != nil {
		if isLockConflict(err) {
			return nil, operation.Wrap(operation.KindConflict, "open_backup_store", "", errors.New("backup store is already in use"))
		}
		return nil, sanitizedFilesystemError("backup store lock could not be acquired", err)
	}
	store := &Store{
		root:                   root,
		limits:                 limits,
		lock:                   lock,
		reservedTargets:        make(map[string]int),
		activeRestoreManifests: make(map[string]int),
		activeRestoreObjects:   make(map[string]int),
		ops:                    defaultStoreOperations(),
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, store.Close())
		}
	}()

	if err := validateRootEntries(root); err != nil {
		return nil, err
	}
	descriptor, err := loadOrCreateDescriptor(root)
	if err != nil {
		return nil, err
	}
	if err := ensureStoreLayout(root); err != nil {
		return nil, err
	}
	if err := validateRootEntries(root); err != nil {
		return nil, err
	}
	rootInfo, err := captureStoreRootIdentity(root)
	if err != nil {
		return nil, err
	}
	store.rootInfo = rootInfo
	store.descriptor = descriptor
	if err := store.validateIdentityAndLayout(); err != nil {
		return nil, err
	}

	scan, err := scanStore(context.Background(), root, descriptor, scanOptions{
		mode:       AuditQuick,
		maxObjects: limits.MaxManifests,
		maxBytes:   limits.MaxTotalBytes,
		checkIndex: false,
	})
	if err != nil {
		return nil, err
	}
	if structuralErr := firstStructuralIssue(scan.report); structuralErr != nil {
		return nil, structuralErr
	}
	if _, err := store.recoverGCTrash(context.Background(), scan.manifests); err != nil {
		return nil, err
	}
	index := buildIndex(descriptor, scan.manifests, scan.objects)
	persisted, loadErr := loadIndex(root, descriptor)
	if loadErr != nil || !indexesEquivalent(persisted, index) {
		if err := persistIndex(root, index); err != nil {
			return nil, err
		}
	}
	store.index = index
	return store, nil
}

// Root returns the validated internal path for process wiring. It must not be
// exposed through MCP results or ordinary logs.
func (store *Store) Root() string {
	if store == nil {
		return ""
	}
	return store.root
}

// Descriptor returns a copy of the immutable store descriptor.
func (store *Store) Descriptor() Descriptor {
	if store == nil {
		return Descriptor{}
	}
	return store.descriptor
}

// Index returns a detached copy of the current derived index.
func (store *Store) Index() Index {
	if store == nil {
		return Index{}
	}
	store.stateMu.RLock()
	defer store.stateMu.RUnlock()
	return cloneIndex(store.index)
}

// Close releases the lifetime writer lock. It is safe to call repeatedly.
func (store *Store) Close() error {
	if store == nil {
		return nil
	}
	store.closeOnce.Do(func() {
		store.transactionMu.Lock()
		defer store.transactionMu.Unlock()
		store.stateMu.Lock()
		store.closed = true
		store.stateMu.Unlock()
		if store.lock != nil {
			store.closeErr = store.lock.close()
			store.lock = nil
		}
	})
	return store.closeErr
}

func (store *Store) isClosed() bool {
	store.stateMu.RLock()
	defer store.stateMu.RUnlock()
	return store.closed
}

func captureStoreRootIdentity(root string) (fs.FileInfo, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, sanitizedFilesystemError("backup store root cannot be inspected", err)
	}
	if isLinkOrReparse(info) || !info.IsDir() {
		return nil, operation.New(operation.KindFilesystem, "backup store root is not a real directory")
	}
	if err := validatePathPermissions(root, true); err != nil {
		return nil, sanitizedFilesystemError("backup store root permissions are not owner-only", err)
	}
	return info, nil
}

func (store *Store) validateRootIdentity() error {
	if store == nil || store.rootInfo == nil {
		return operation.New(operation.KindConflict, "backup store identity is unavailable")
	}
	info, err := os.Lstat(store.root)
	if err != nil {
		return sanitizedFilesystemError("backup store root cannot be inspected", err)
	}
	if isLinkOrReparse(info) || !info.IsDir() || !os.SameFile(store.rootInfo, info) {
		return operation.New(operation.KindConflict, "backup store root identity changed")
	}
	if err := validatePathPermissions(store.root, true); err != nil {
		return sanitizedFilesystemError("backup store root permissions are not owner-only", err)
	}
	return nil
}

func (store *Store) validateIdentityAndLayout() error {
	if err := store.validateRootIdentity(); err != nil {
		return err
	}
	if err := validateRootEntries(store.root); err != nil {
		return err
	}
	algorithmRoot := filepath.Join(store.root, "objects", ObjectAlgorithm)
	algorithmInfo, err := os.Lstat(algorithmRoot)
	if err != nil {
		return sanitizedFilesystemError("backup object algorithm directory cannot be inspected", err)
	}
	if isLinkOrReparse(algorithmInfo) || !algorithmInfo.IsDir() {
		return operation.New(operation.KindFilesystem, "backup object algorithm directory is invalid")
	}
	if err := validatePathPermissions(algorithmRoot, true); err != nil {
		return sanitizedFilesystemError("backup object algorithm directory permissions are not owner-only", err)
	}
	indexFile := indexPath(store.root)
	indexInfo, err := os.Lstat(indexFile)
	if err == nil {
		if isLinkOrReparse(indexInfo) || !indexInfo.Mode().IsRegular() {
			return operation.New(operation.KindFilesystem, "backup index entry is not a regular file")
		}
		if err := validateSingleLink(indexFile, indexInfo); err != nil {
			return operation.New(operation.KindFilesystem, "backup index hard-link state is invalid")
		}
		if err := validatePathPermissions(indexFile, false); err != nil {
			return sanitizedFilesystemError("backup index permissions are not owner-only", err)
		}
	} else if !os.IsNotExist(err) {
		return sanitizedFilesystemError("backup index cannot be inspected", err)
	}
	return nil
}

func normalizeLimits(limits Limits) (Limits, error) {
	defaults := defaultStoreLimits()
	if limits.MaxTotalBytes == 0 {
		limits.MaxTotalBytes = defaults.MaxTotalBytes
	}
	if limits.MaxObjectBytes == 0 {
		limits.MaxObjectBytes = defaults.MaxObjectBytes
	}
	if limits.MaxManifests == 0 {
		limits.MaxManifests = defaults.MaxManifests
	}
	if limits.MaxVersionsPerTarget == 0 {
		limits.MaxVersionsPerTarget = defaults.MaxVersionsPerTarget
	}
	if limits.MaxPinned == 0 {
		limits.MaxPinned = defaults.MaxPinned
	}
	if limits.RetentionDays == 0 {
		limits.RetentionDays = defaults.RetentionDays
	}
	if limits.PlanTTLSeconds == 0 {
		limits.PlanTTLSeconds = defaults.PlanTTLSeconds
	}
	if limits.MaxTotalBytes < 0 || limits.MaxObjectBytes < 0 || limits.MaxManifests < 0 ||
		limits.MaxVersionsPerTarget < 0 || limits.MaxPinned < 0 || limits.RetentionDays < 0 || limits.PlanTTLSeconds < 0 {
		return Limits{}, operation.New(operation.KindInvalidInput, "backup store limits must be positive")
	}
	if limits.MaxTotalBytes > hardMaxTotalBytes || limits.MaxObjectBytes > hardMaxObjectBytes ||
		limits.MaxManifests > hardMaxManifests || limits.MaxVersionsPerTarget > hardMaxVersionsPerTarget ||
		limits.MaxPinned > hardMaxPinned || limits.RetentionDays > hardMaxRetentionDays ||
		limits.PlanTTLSeconds > hardMaxPlanTTLSeconds {
		return Limits{}, operation.New(operation.KindLimit, "backup store limits exceed supported hard maxima")
	}
	return limits, nil
}

func validateDedicatedStorePath(directory string, publicRoots []string) (string, error) {
	cleanDirectory := filepath.Clean(directory)
	if directory == "" || strings.Contains(directory, "\x00") || !filepath.IsAbs(cleanDirectory) {
		return "", operation.Wrap(operation.KindInvalidPath, "validate_backup_store", "", errors.New("backup store directory must be an absolute path"))
	}
	if filepath.Dir(cleanDirectory) == cleanDirectory {
		return "", operation.Wrap(operation.KindInvalidPath, "validate_backup_store", "", errors.New("backup store directory must not be a filesystem root"))
	}
	if err := validateExistingComponents(directory); err != nil {
		return "", sanitizedFilesystemError("backup store path contains an invalid component", err)
	}
	storeSet, err := security.NormalizeAllowedDirectorySet([]string{directory})
	if err != nil || len(storeSet.Requested) != 1 || len(storeSet.Resolved) != 1 {
		return "", operation.Wrap(operation.KindInvalidPath, "validate_backup_store", "", errors.New("backup store directory cannot be normalized safely"))
	}
	if !security.PathsEqual(storeSet.Requested[0], storeSet.Resolved[0]) {
		return "", operation.Wrap(operation.KindInvalidPath, "validate_backup_store", "", errors.New("backup store directory must not use a symlink, junction, reparse point, or path alias"))
	}

	for _, publicRoot := range publicRoots {
		if publicRoot == "" {
			continue
		}
		rootSet, rootErr := security.NormalizeAllowedDirectorySet([]string{publicRoot})
		if rootErr != nil || len(rootSet.Requested) != 1 || len(rootSet.Resolved) != 1 {
			return "", operation.Wrap(operation.KindInvalidPath, "validate_backup_store", "", errors.New("public allowed directory cannot be normalized safely"))
		}
		for _, storePath := range []string{storeSet.Requested[0], storeSet.Resolved[0]} {
			for _, rootPath := range []string{rootSet.Requested[0], rootSet.Resolved[0]} {
				if security.PathsOverlap(storePath, rootPath) {
					return "", operation.Wrap(operation.KindAccessDenied, "validate_backup_store", "", errors.New("backup store must not overlap a public allowed directory"))
				}
			}
		}
	}
	return storeSet.Resolved[0], nil
}

func createDirectoryPath(path string) error {
	clean := filepath.Clean(path)
	missing := make([]string, 0, 4)
	current := clean
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if isLinkOrReparse(info) || !info.IsDir() {
				return errors.New("existing store component is not a real directory")
			}
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}

	for index := len(missing) - 1; index >= 0; index-- {
		next := filepath.Join(current, missing[index])
		created := false
		if err := os.Mkdir(next, 0o700); err != nil {
			if !os.IsExist(err) {
				return err
			}
		} else {
			created = true
		}
		info, err := os.Lstat(next)
		if err != nil {
			return err
		}
		if isLinkOrReparse(info) || !info.IsDir() {
			return errors.New("created store component is not a real directory")
		}
		if created {
			if err := restrictPathPermissions(next, true); err != nil {
				return err
			}
			if err := syncDirectory(current); err != nil {
				return err
			}
		} else if err := validatePathPermissions(next, true); err != nil {
			return err
		}
		current = next
	}
	return validatePathPermissions(clean, true)
}

func validateExistingComponents(path string) error {
	current := filepath.Clean(path)
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if isLinkOrReparse(info) {
				return errors.New("store path contains a link or reparse point")
			}
			if !info.IsDir() {
				return errors.New("store path component is not a directory")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

func ensureStoreLayout(root string) error {
	for _, relative := range []string{
		"objects",
		filepath.Join("objects", "sha256"),
		"manifests",
		"index",
		"staging",
		"trash",
	} {
		path := filepath.Join(root, relative)
		if err := ensureDirectory(path); err != nil {
			return sanitizedFilesystemError("backup store layout could not be initialized", err)
		}
	}
	return nil
}

func ensureDirectory(path string) error {
	info, err := os.Lstat(path)
	created := false
	if os.IsNotExist(err) {
		if err := os.Mkdir(path, 0o700); err != nil {
			return err
		}
		created = true
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if isLinkOrReparse(info) || !info.IsDir() {
		return errors.New("store layout entry is not a real directory")
	}
	if created {
		if err := restrictPathPermissions(path, true); err != nil {
			return err
		}
		return syncDirectory(filepath.Dir(path))
	}
	return validatePathPermissions(path, true)
}

func validateRootEntries(root string) error {
	entries, overflow, err := readDirectoryBounded(root, len(expectedRootEntries))
	if err != nil {
		return sanitizedFilesystemError("backup store root cannot be inspected", err)
	}
	if overflow {
		return operation.Wrap(operation.KindFilesystem, "validate_backup_store", "", errors.New("backup store root contains unexpected entries"))
	}
	for _, entry := range entries {
		name := entry.Name()
		if _, expected := expectedRootEntries[name]; !expected {
			return operation.Wrap(operation.KindFilesystem, "validate_backup_store", "", errors.New("backup store root contains an unexpected entry"))
		}
		info, err := os.Lstat(filepath.Join(root, name))
		if err != nil {
			return sanitizedFilesystemError("backup store root entry cannot be inspected", err)
		}
		if isLinkOrReparse(info) {
			return operation.Wrap(operation.KindFilesystem, "validate_backup_store", "", errors.New("backup store root contains a linked or reparse-backed entry"))
		}
		entryPath := filepath.Join(root, name)
		if name == "store.json" || name == "store.lock" {
			if !info.Mode().IsRegular() {
				return operation.Wrap(operation.KindFilesystem, "validate_backup_store", "", errors.New("backup store metadata entry is not a regular file"))
			}
			if name == "store.json" {
				if err := validateSingleLink(entryPath, info); err != nil {
					return operation.Wrap(operation.KindFilesystem, "validate_backup_store", "", errors.New("backup store descriptor hard-link state is invalid"))
				}
			}
			if err := validatePathPermissions(entryPath, false); err != nil {
				return sanitizedFilesystemError("backup store metadata permissions are not owner-only", err)
			}
			continue
		}
		if !info.IsDir() {
			return operation.Wrap(operation.KindFilesystem, "validate_backup_store", "", errors.New("backup store layout entry is not a directory"))
		}
		if err := validatePathPermissions(entryPath, true); err != nil {
			return sanitizedFilesystemError("backup store directory permissions are not owner-only", err)
		}
	}
	return nil
}

func loadOrCreateDescriptor(root string) (Descriptor, error) {
	path := filepath.Join(root, "store.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		entries, overflow, readErr := readDirectoryBounded(root, 1)
		if readErr != nil {
			return Descriptor{}, sanitizedFilesystemError("backup store root cannot be inspected", readErr)
		}
		if overflow {
			return Descriptor{}, operation.Wrap(operation.KindFilesystem, "validate_backup_store", "", errors.New("backup store descriptor is missing from a non-empty store"))
		}
		for _, entry := range entries {
			if entry.Name() != "store.lock" {
				return Descriptor{}, operation.Wrap(operation.KindFilesystem, "validate_backup_store", "", errors.New("backup store descriptor is missing from a non-empty store"))
			}
		}
		descriptor, createErr := newDescriptor()
		if createErr != nil {
			return Descriptor{}, createErr
		}
		if createErr := createDescriptor(path, descriptor); createErr != nil {
			return Descriptor{}, createErr
		}
		return descriptor, nil
	}
	if err != nil {
		return Descriptor{}, sanitizedFilesystemError("backup store descriptor cannot be inspected", err)
	}
	if isLinkOrReparse(info) || !info.Mode().IsRegular() {
		return Descriptor{}, operation.Wrap(operation.KindFilesystem, "validate_backup_store", "", errors.New("backup store descriptor is not a regular file"))
	}
	if err := validatePathPermissions(path, false); err != nil {
		return Descriptor{}, sanitizedFilesystemError("backup store descriptor permissions are not owner-only", err)
	}
	return readDescriptor(path, info)
}

func newDescriptor() (Descriptor, error) {
	identifier := make([]byte, 32)
	if _, err := rand.Read(identifier); err != nil {
		return Descriptor{}, operation.Wrap(operation.KindFilesystem, "create_backup_store", "", errors.New("backup store identifier could not be generated"))
	}
	return Descriptor{
		FormatVersion:   FormatVersion,
		StoreID:         hex.EncodeToString(identifier),
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		ObjectAlgorithm: ObjectAlgorithm,
		ManifestVersion: ManifestVersion,
		IndexVersion:    IndexVersion,
	}, nil
}

func createDescriptor(path string, descriptor Descriptor) error {
	data, err := json.MarshalIndent(descriptor, "", "  ")
	if err != nil {
		return operation.Wrap(operation.KindFilesystem, "create_backup_store", "", errors.New("backup store descriptor could not be encoded"))
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return sanitizedFilesystemError("backup store descriptor could not be created", err)
	}
	writeErr := writeAndSync(file, data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return sanitizedFilesystemError("backup store descriptor could not be persisted", errors.Join(writeErr, closeErr))
	}
	if err := restrictPathPermissions(path, false); err != nil {
		return sanitizedFilesystemError("backup store descriptor permissions could not be restricted", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return sanitizedFilesystemError("backup store descriptor directory could not be synchronized", err)
	}
	return nil
}

func writeAndSync(file *os.File, data []byte) error {
	written, err := file.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	return file.Sync()
}

func readDescriptor(path string, lstatInfo fs.FileInfo) (Descriptor, error) {
	file, err := os.Open(path)
	if err != nil {
		return Descriptor{}, sanitizedFilesystemError("backup store descriptor cannot be opened", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Descriptor{}, sanitizedFilesystemError("backup store descriptor cannot be inspected", err)
	}
	if !info.Mode().IsRegular() || !os.SameFile(lstatInfo, info) || info.Size() > maxDescriptorBytes {
		return Descriptor{}, operation.Wrap(operation.KindFilesystem, "validate_backup_store", "", errors.New("backup store descriptor identity or size is invalid"))
	}
	return decodeDescriptor(io.LimitReader(file, maxDescriptorBytes+1))
}

func decodeDescriptor(reader io.Reader) (Descriptor, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var descriptor Descriptor
	if err := decoder.Decode(&descriptor); err != nil {
		return Descriptor{}, operation.Wrap(operation.KindInvalidInput, "validate_backup_store", "", errors.New("backup store descriptor is malformed"))
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Descriptor{}, operation.Wrap(operation.KindInvalidInput, "validate_backup_store", "", errors.New("backup store descriptor contains trailing data"))
	}
	if err := validateDescriptor(descriptor); err != nil {
		return Descriptor{}, err
	}
	return descriptor, nil
}

func validateDescriptor(descriptor Descriptor) error {
	if descriptor.FormatVersion != FormatVersion || descriptor.ObjectAlgorithm != ObjectAlgorithm ||
		descriptor.ManifestVersion != ManifestVersion || descriptor.IndexVersion != IndexVersion {
		return operation.Wrap(operation.KindInvalidInput, "validate_backup_store", "", errors.New("backup store descriptor uses an unsupported format"))
	}
	if len(descriptor.StoreID) != 64 || strings.ToLower(descriptor.StoreID) != descriptor.StoreID {
		return operation.Wrap(operation.KindInvalidInput, "validate_backup_store", "", errors.New("backup store identifier is invalid"))
	}
	identifier, err := hex.DecodeString(descriptor.StoreID)
	if err != nil || len(identifier) != 32 {
		return operation.Wrap(operation.KindInvalidInput, "validate_backup_store", "", errors.New("backup store identifier is invalid"))
	}
	createdAt, err := time.Parse(time.RFC3339Nano, descriptor.CreatedAt)
	if err != nil || createdAt.Location() != time.UTC || !strings.HasSuffix(descriptor.CreatedAt, "Z") {
		return operation.Wrap(operation.KindInvalidInput, "validate_backup_store", "", errors.New("backup store creation timestamp is invalid"))
	}
	return nil
}

func sanitizedFilesystemError(message string, err error) error {
	kind := operation.KindFilesystem
	if errors.Is(err, fs.ErrPermission) {
		kind = operation.KindPermission
	}
	return operation.Wrap(kind, "backup_store", "", errors.New(message))
}
