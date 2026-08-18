package handler

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/execution"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/filesystempackage"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
	"github.com/zoster81/scripthold/internal/sourceintelligence"
	"github.com/zoster81/scripthold/internal/taskstore"
)

// BackupStoreReader is the read-only management contract exposed publicly.
type BackupStoreReader interface {
	Root() string
	Status(context.Context) (backupstore.StoreStatus, error)
	List(context.Context, backupstore.ListOptions) (backupstore.ListResult, error)
	Inspect(context.Context, string, backupstore.InspectOptions) (backupstore.InspectResult, error)
	Audit(context.Context, backupstore.AuditOptions) (backupstore.AuditReport, error)
}

// BackupStoreCapturer is the internal mutation authority detected separately
// from the public read-only management contract.
type BackupStoreCapturer interface {
	Capture(context.Context, backupstore.CaptureRequest) (backupstore.CaptureResult, error)
}

// BackupStoreCapturePreflighter is the read-only admission contract. It cannot
// reserve quota, create objects, or commit manifests.
type BackupStoreCapturePreflighter interface {
	PreflightCaptureBatch(context.Context, []backupstore.CaptureRequest) error
}

// BackupStoreBatchCapturer is the package-wide persistent mutation authority.
type BackupStoreBatchCapturer interface {
	CaptureBatch(context.Context, []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error)
}

// BackupStoreRestoreReader exposes only immutable verified source reads needed
// by restore previews and backup comparisons.
type BackupStoreRestoreReader interface {
	OpenReadSource(context.Context, string, backupstore.RestoreSourceOptions) (*backupstore.ReadSource, error)
	RestorePlanTTL() time.Duration
	RestoreObjectLimit() int64
}

// BackupStoreRestoreStager is the target-adjacent staging authority used only
// by restore apply after its one-shot capability has been consumed.
type BackupStoreRestoreStager interface {
	StageReadSource(context.Context, *backupstore.ReadSource, string, os.FileMode, *time.Time) (*filesystem.StagedReplacement, error)
}

// BackupStoreGCPlanner is the read-only generation-bound GC planning contract.
type BackupStoreGCPlanner interface {
	PlanGC(context.Context, backupstore.GCOptions) (backupstore.GCPlan, error)
	GCPlanTTL() time.Duration
}

// BackupStoreGCApplier owns destructive GC application only.
type BackupStoreGCApplier interface {
	ApplyGC(context.Context, backupstore.GCPlan) (backupstore.GCResult, error)
}

// TaskStore is the transport-independent durable task registry contract.
type TaskStore interface {
	Root() string
	Submit(context.Context, taskstore.Request) (taskstore.SubmitResult, error)
	List(context.Context, taskstore.ListOptions) (taskstore.ListResult, error)
	Get(context.Context, string) (taskstore.Task, error)
	Logs(context.Context, string, taskstore.LogOptions) (taskstore.LogsResult, error)
	Cancel(context.Context, string, string) (taskstore.Task, error)
}

// Default permissions for new files and directories
const (
	DefaultFileMode os.FileMode = 0644
	DefaultDirMode  os.FileMode = 0755
)

// Handler handles all file tool operations
type Handler struct {
	config                         *config.Config
	executionPolicy                *ExecutionPolicy
	configuredRequestedDirs        []string // immutable lexical baseline; always allowed
	configuredDirs                 []string // immutable resolved baseline; always allowed
	allowedRequestedDirs           []string
	allowedDirs                    []string
	protectedRequestedDirs         []string // immutable internal roots denied to public tools
	protectedDirs                  []string // resolved internal roots denied to public tools
	backupStore                    BackupStoreReader
	backupCapture                  BackupStoreCapturer
	backupCapturePreflight         BackupStoreCapturePreflighter
	backupBatchCapture             BackupStoreBatchCapturer
	backupRestoreReader            BackupStoreRestoreReader
	backupRestoreStager            BackupStoreRestoreStager
	backupGCPlanner                BackupStoreGCPlanner
	backupGCApplier                BackupStoreGCApplier
	taskStore                      TaskStore
	editPreviews                   *editPreviewStore
	restorePreviews                *restorePreviewStore
	gcPreviews                     *gcPreviewStore
	patchPackagePreviews           *patchPackagePreviewStore
	byteMutationPreviews           *byteMutationPreviewStore
	filesystemPackageEngine        *filesystempackage.Engine
	filesystemPackageInitErr       error
	patchPackageStageReplacement   func(context.Context, string, []byte, os.FileMode) (*filesystem.StagedReplacement, error)
	patchPackageCommitReplacement  func(int, *filesystem.StagedReplacement, filesystem.ReplaceOptions) (bool, error)
	patchPackageCleanupReplacement func(*filesystem.StagedReplacement) error
	restoreStageReplacement        func(context.Context, *backupstore.ReadSource, string, os.FileMode, *time.Time) (*filesystem.StagedReplacement, error)
	restoreCommitReplacement       func(*filesystem.StagedReplacement, filesystem.ReplaceOptions) (bool, error)
	restoreCleanupReplacement      func(*filesystem.StagedReplacement) error
	verifyGitExecutable            func() (string, error)
	verifyGitRun                   func(context.Context, verificationGitRequest) (execution.Result, error)
	replaceFile                    func(string, []byte, filesystem.ReplaceOptions) error
	sourceIndexOnce                sync.Once
	sourceIndex                    *sourceintelligence.ProjectIndexManager
	sourceIndexInitErr             error
	mu                             sync.RWMutex
}

// Option is a functional option for configuring Handler
type Option func(*Handler)

// WithConfig sets the configuration for the handler.
func WithConfig(cfg *config.Config) Option {
	return func(h *Handler) {
		if cfg != nil {
			h.config = cfg
		}
	}
}

// WithExecutionPolicy sets an immutable transport-specific execution policy.
func WithExecutionPolicy(policy ExecutionPolicy) Option {
	return func(h *Handler) {
		policyCopy := policy
		h.executionPolicy = &policyCopy
	}
}

// WithProtectedDirectories configures internal process roots that ordinary file
// tools must never expose, even if a dynamic MCP root later overlaps them.
func WithProtectedDirectories(dirs []string) Option {
	return func(h *Handler) {
		requested, resolved := normalizeAllowedDirectorySets(dirs)
		h.protectedRequestedDirs = mergeUniqueDirectories(h.protectedRequestedDirs, requested)
		h.protectedDirs = mergeUniqueDirectories(h.protectedDirs, resolved)
	}
}

// WithBackupStore configures optional read-only management and detects whether
// the same store also provides the internal capture authority.
func WithBackupStore(store BackupStoreReader) Option {
	return func(h *Handler) {
		h.backupStore = nil
		h.backupCapture = nil
		h.backupCapturePreflight = nil
		h.backupBatchCapture = nil
		h.backupRestoreReader = nil
		h.backupRestoreStager = nil
		h.backupGCPlanner = nil
		h.backupGCApplier = nil
		if backupStoreReaderIsNil(store) {
			return
		}
		h.backupStore = store
		if capturer, ok := store.(BackupStoreCapturer); ok {
			h.backupCapture = capturer
		}
		if preflighter, ok := store.(BackupStoreCapturePreflighter); ok {
			h.backupCapturePreflight = preflighter
		}
		if capturer, ok := store.(BackupStoreBatchCapturer); ok {
			h.backupBatchCapture = capturer
		}
		if reader, ok := store.(BackupStoreRestoreReader); ok {
			h.backupRestoreReader = reader
		}
		if stager, ok := store.(BackupStoreRestoreStager); ok {
			h.backupRestoreStager = stager
		}
		if planner, ok := store.(BackupStoreGCPlanner); ok {
			h.backupGCPlanner = planner
		}
		if applier, ok := store.(BackupStoreGCApplier); ok {
			h.backupGCApplier = applier
		}
		if store != nil && store.Root() != "" {
			requested, resolved := normalizeAllowedDirectorySets([]string{store.Root()})
			h.protectedRequestedDirs = mergeUniqueDirectories(h.protectedRequestedDirs, requested)
			h.protectedDirs = mergeUniqueDirectories(h.protectedDirs, resolved)
		}
	}
}

// WithTaskStore configures the durable asynchronous execution registry and
// makes its private root inaccessible to ordinary filesystem tools.
func WithTaskStore(store TaskStore) Option {
	return func(h *Handler) {
		h.taskStore = nil
		if taskStoreIsNil(store) {
			return
		}
		h.taskStore = store
		if store.Root() != "" {
			requested, resolved := normalizeAllowedDirectorySets([]string{store.Root()})
			h.protectedRequestedDirs = mergeUniqueDirectories(h.protectedRequestedDirs, requested)
			h.protectedDirs = mergeUniqueDirectories(h.protectedDirs, resolved)
		}
	}
}

func taskStoreIsNil(store TaskStore) bool {
	if store == nil {
		return true
	}
	value := reflect.ValueOf(store)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func backupStoreReaderIsNil(store BackupStoreReader) bool {
	if store == nil {
		return true
	}
	value := reflect.ValueOf(store)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// NewHandler creates a new Handler with allowed directories and optional configuration.
// If no config is provided via WithConfig, default configuration is used.
func NewHandler(allowedDirs []string, opts ...Option) *Handler {
	// Retain the configured spelling for lexical containment while exposing and
	// traversing through the resolved representation. This preserves legitimate
	// directory aliases without allowing an external link to become an entry point.
	configuredRequestedDirs, configuredDirs := normalizeAllowedDirectorySets(allowedDirs)

	h := &Handler{}
	for _, opt := range opts {
		opt(h)
	}
	configuredRequestedDirs, configuredDirs = h.filterProtectedDirectorySets(configuredRequestedDirs, configuredDirs)
	h.configuredRequestedDirs = configuredRequestedDirs
	h.configuredDirs = configuredDirs
	h.allowedRequestedDirs = mergeUniqueDirectories(configuredRequestedDirs, configuredDirs)
	h.allowedDirs = append([]string(nil), configuredDirs...)
	if h.config == nil {
		h.config = config.Load()
	}
	h.editPreviews = newEditPreviewStore(
		h.maxEditPreviews(),
		h.maxEditPreviewBytes(),
		time.Duration(h.editPreviewTTLSeconds())*time.Second,
	)
	restoreTTL := 15 * time.Minute
	if h.backupRestoreReader != nil && h.backupRestoreReader.RestorePlanTTL() > 0 {
		restoreTTL = h.backupRestoreReader.RestorePlanTTL()
	}
	h.restorePreviews = newRestorePreviewStore(restorePreviewMaxEntries, restorePreviewMaxBytes, restoreTTL)
	gcTTL := 15 * time.Minute
	if h.backupGCPlanner != nil && h.backupGCPlanner.GCPlanTTL() > 0 {
		gcTTL = h.backupGCPlanner.GCPlanTTL()
	}
	h.gcPreviews = newGCPreviewStore(gcPreviewMaxEntries, gcPreviewMaxBytes, gcTTL)
	h.replaceFile = filesystem.ReplaceFile
	h.patchPackagePreviews = newPatchPackagePreviewStore(
		h.maxPatchPackagePreviews(),
		h.maxPatchPackagePreviewBytes(),
		time.Duration(h.patchPackagePreviewTTLSeconds())*time.Second,
	)
	h.byteMutationPreviews = newByteMutationPreviewStore(
		h.maxByteMutationPreviews(),
		h.maxByteMutationPreviewBytes(),
		time.Duration(h.byteMutationPreviewTTLSeconds())*time.Second,
	)
	h.patchPackageStageReplacement = stagePatchPackageReplacement
	h.patchPackageCommitReplacement = commitPatchPackageReplacement
	h.patchPackageCleanupReplacement = func(staged *filesystem.StagedReplacement) error { return staged.Cleanup() }
	h.restoreStageReplacement = func(ctx context.Context, source *backupstore.ReadSource, target string, mode os.FileMode, modTime *time.Time) (*filesystem.StagedReplacement, error) {
		if h.backupRestoreStager == nil {
			return nil, operation.New(operation.KindConflict, "backup restore staging authority is unavailable")
		}
		return h.backupRestoreStager.StageReadSource(ctx, source, target, mode, modTime)
	}
	h.restoreCommitReplacement = func(staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		return staged.Commit(options)
	}
	h.restoreCleanupReplacement = func(staged *filesystem.StagedReplacement) error { return staged.Cleanup() }
	h.verifyGitExecutable = findVerificationGit
	h.verifyGitRun = h.runVerificationGit
	h.filesystemPackageEngine, h.filesystemPackageInitErr = h.newFilesystemPackageEngine()

	return h
}

// GetAllowedDirectories returns a copy of the allowed directories.
func (h *Handler) GetAllowedDirectories() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	dirs := make([]string, len(h.allowedDirs))
	copy(dirs, h.allowedDirs)
	return dirs
}

// ResolvedAllowedDirs returns allowed directories with symlinks resolved.
func (h *Handler) ResolvedAllowedDirs() []string {
	return security.ResolveAllowedDirs(h.GetAllowedDirectories())
}

// UpdateAllowedDirectories updates the allowed directories (for MCP Roots protocol)
func (h *Handler) UpdateAllowedDirectories(newDirs []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	requested, resolved := normalizeAllowedDirectorySets(newDirs)
	requested, resolved = h.filterProtectedDirectorySets(requested, resolved)
	h.allowedRequestedDirs = mergeUniqueDirectories(requested, resolved)
	h.allowedDirs = resolved
}

// HasConfiguredDirectories reports whether this process started with an
// authoritative directory baseline.
func (h *Handler) HasConfiguredDirectories() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.configuredDirs) > 0
}

// MergeAllowedDirectories sets the allowed directories to the deduped union of
// the process baseline and newDirs, so MCP roots never replace configured access.
func (h *Handler) MergeAllowedDirectories(newDirs []string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	normalizedRequestedDirs, normalizedNewDirs := normalizeAllowedDirectorySets(newDirs)
	normalizedRequestedDirs, normalizedNewDirs = h.filterProtectedDirectorySets(normalizedRequestedDirs, normalizedNewDirs)
	h.allowedRequestedDirs = mergeUniqueDirectories(
		h.configuredRequestedDirs,
		h.configuredDirs,
		normalizedRequestedDirs,
		normalizedNewDirs,
	)
	h.allowedDirs = mergeUniqueDirectories(h.configuredDirs, normalizedNewDirs)

	result := make([]string, len(h.allowedDirs))
	copy(result, h.allowedDirs)
	return result
}

func normalizeAllowedDirectorySets(dirs []string) (requested, resolved []string) {
	requested = make([]string, 0, len(dirs))
	resolved = make([]string, 0, len(dirs))
	for _, dir := range dirs {
		set, err := security.NormalizeAllowedDirectorySet([]string{dir})
		if err == nil && len(set.Requested) == 1 && len(set.Resolved) == 1 {
			requested = append(requested, set.Requested[0])
			resolved = append(resolved, set.Resolved[0])
			continue
		}
		requested = append(requested, dir)
		resolved = append(resolved, dir)
	}
	return requested, resolved
}

func mergeUniqueDirectories(groups ...[]string) []string {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	seen := make(map[string]struct{}, total)
	merged := make([]string, 0, total)
	for _, group := range groups {
		for _, dir := range group {
			if _, ok := seen[dir]; ok {
				continue
			}
			seen[dir] = struct{}{}
			merged = append(merged, dir)
		}
	}
	return merged
}

func (h *Handler) filterProtectedDirectorySets(requested, resolved []string) ([]string, []string) {
	if len(h.protectedRequestedDirs) == 0 && len(h.protectedDirs) == 0 {
		return requested, resolved
	}
	protectedDirectories := mergeUniqueDirectories(h.protectedRequestedDirs, h.protectedDirs)
	filteredRequested := make([]string, 0, len(requested))
	filteredResolved := make([]string, 0, len(resolved))
	for index := 0; index < len(requested) && index < len(resolved); index++ {
		blocked := false
		for _, candidate := range []string{requested[index], resolved[index]} {
			for _, protected := range protectedDirectories {
				if security.PathsOverlap(candidate, protected) {
					blocked = true
					break
				}
			}
			if blocked {
				break
			}
		}
		if !blocked {
			filteredRequested = append(filteredRequested, requested[index])
			filteredResolved = append(filteredResolved, resolved[index])
		}
	}
	return filteredRequested, filteredResolved
}

// validatePath validates a path against allowed directories and rejects every
// protected internal root after both lexical and resolved checks.
func (h *Handler) validatePath(path string) (string, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	validated, err := security.ValidatePathWithAllowedDirectories(path, h.allowedRequestedDirs, h.allowedDirs)
	if err != nil {
		return "", err
	}
	requested := path
	if absolute, absoluteErr := filepath.Abs(security.ExpandHome(path)); absoluteErr == nil {
		requested = absolute
	}
	if security.IsPathWithinAllowedDirectories(requested, h.protectedRequestedDirs) ||
		security.IsPathWithinAllowedDirectories(validated, h.protectedDirs) {
		return "", operation.New(operation.KindAccessDenied, "access denied - path reserved for internal storage")
	}
	return validated, nil
}

// getFileMode returns the file's current permissions, or DefaultFileMode if file doesn't exist.
func getFileMode(path string) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return DefaultFileMode
	}
	return info.Mode().Perm()
}
