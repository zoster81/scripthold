// Package config provides configuration management for the Scripthold server.
package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/zoster81/scripthold/internal/encoding"
)

const (
	EnvDefaultEncoding               = "MCP_DEFAULT_ENCODING"
	EnvMemoryThreshold               = "MCP_MEMORY_THRESHOLD" // Deprecated 1.x fallback for file/output limits.
	EnvMaxFileBytes                  = "MCP_MAX_FILE_BYTES"
	EnvMaxDecodedCharacters          = "MCP_MAX_DECODED_CHARACTERS"
	EnvMaxLineBytes                  = "MCP_MAX_LINE_BYTES"
	EnvMaxBatchFiles                 = "MCP_MAX_BATCH_FILES"
	EnvMaxMatches                    = "MCP_MAX_MATCHES"
	EnvMaxOutputBytes                = "MCP_MAX_OUTPUT_BYTES"
	EnvMaxFingerprintEntries         = "MCP_MAX_FINGERPRINT_ENTRIES"
	EnvMaxFingerprintEntryDetails    = "MCP_MAX_FINGERPRINT_ENTRY_DETAILS"
	EnvMaxEditPreviews               = "MCP_MAX_EDIT_PREVIEWS"
	EnvMaxEditPreviewBytes           = "MCP_MAX_EDIT_PREVIEW_BYTES"
	EnvEditPreviewTTLSeconds         = "MCP_EDIT_PREVIEW_TTL_SECONDS"
	EnvMaxPatchPackageBytes          = "MCP_MAX_PATCH_PACKAGE_BYTES"
	EnvMaxPatchPackagePreparedBytes  = "MCP_MAX_PATCH_PACKAGE_PREPARED_BYTES"
	EnvMaxPatchPackagePreviews       = "MCP_MAX_PATCH_PACKAGE_PREVIEWS"
	EnvMaxPatchPackagePreviewBytes   = "MCP_MAX_PATCH_PACKAGE_PREVIEW_BYTES"
	EnvPatchPackagePreviewTTLSeconds = "MCP_PATCH_PACKAGE_PREVIEW_TTL_SECONDS"
	EnvMaxByteMutationPreviews       = "MCP_MAX_BYTE_MUTATION_PREVIEWS"
	EnvMaxByteMutationPreviewBytes   = "MCP_MAX_BYTE_MUTATION_PREVIEW_BYTES"
	EnvByteMutationPreviewTTLSeconds = "MCP_BYTE_MUTATION_PREVIEW_TTL_SECONDS"
	EnvMaxSessions                   = "MCP_MAX_SESSIONS" // Maximum live native Streamable HTTP sessions.

	EnvBackupStoreDir             = "MCP_BACKUP_STORE_DIR"
	EnvBackupDefaultPolicy        = "MCP_BACKUP_DEFAULT_POLICY"
	EnvBackupMaxTotalBytes        = "MCP_BACKUP_MAX_TOTAL_BYTES"
	EnvBackupMaxObjectBytes       = "MCP_BACKUP_MAX_OBJECT_BYTES"
	EnvBackupMaxManifests         = "MCP_BACKUP_MAX_MANIFESTS"
	EnvBackupMaxVersionsPerTarget = "MCP_BACKUP_MAX_VERSIONS_PER_TARGET"
	EnvBackupMaxPinned            = "MCP_BACKUP_MAX_PINNED"
	EnvBackupRetentionDays        = "MCP_BACKUP_RETENTION_DAYS"
	EnvBackupPlanTTLSeconds       = "MCP_BACKUP_PLAN_TTL_SECONDS"

	EnvTaskStoreDir             = "MCP_TASK_STORE_DIR"
	EnvTaskMaxConcurrency       = "MCP_TASK_MAX_CONCURRENCY"
	EnvTaskMaxQueued            = "MCP_TASK_MAX_QUEUED"
	EnvTaskMaxLogBytesPerStream = "MCP_TASK_MAX_LOG_BYTES_PER_STREAM"
	EnvTaskMaxRuntimeSeconds    = "MCP_TASK_MAX_RUNTIME_SECONDS"
	EnvTaskRetentionDays        = "MCP_TASK_RETENTION_DAYS"
	EnvTaskMaxTerminal          = "MCP_TASK_MAX_TERMINAL"
	EnvTaskMaxTotalBytes        = "MCP_TASK_MAX_TOTAL_BYTES"

	DefaultEncoding                      = "utf-8"
	DefaultMaxFileBytes                  = int64(64 * 1024 * 1024)
	DefaultMaxDecodedCharacters          = 16 * 1024 * 1024
	DefaultMaxLineBytes                  = 16 * 1024 * 1024
	DefaultMaxBatchFiles                 = 256
	DefaultMaxMatches                    = 10_000
	DefaultMaxOutputBytes                = int64(64 * 1024 * 1024)
	DefaultMaxFingerprintEntries         = 100_000
	DefaultMaxFingerprintEntryDetails    = 1_000
	DefaultMaxEditPreviews               = 128
	DefaultMaxEditPreviewBytes           = int64(64 * 1024 * 1024)
	DefaultEditPreviewTTLSeconds         = 15 * 60
	DefaultMaxPatchPackageBytes          = int64(16 * 1024 * 1024)
	DefaultMaxPatchPackagePreparedBytes  = int64(64 * 1024 * 1024)
	DefaultMaxPatchPackagePreviews       = 16
	DefaultMaxPatchPackagePreviewBytes   = int64(128 * 1024 * 1024)
	DefaultPatchPackagePreviewTTLSeconds = 15 * 60
	DefaultMaxByteMutationPreviews       = 32
	DefaultMaxByteMutationPreviewBytes   = int64(256 * 1024 * 1024)
	DefaultByteMutationPreviewTTLSeconds = 15 * 60
	DefaultMaxSessions                   = 128

	BackupPolicyDisabled = "disabled"
	BackupPolicyRequired = "required"

	DefaultBackupPolicy               = BackupPolicyDisabled
	DefaultBackupMaxTotalBytes        = int64(1024 * 1024 * 1024)
	DefaultBackupMaxObjectBytes       = int64(64 * 1024 * 1024)
	DefaultBackupMaxManifests         = 10_000
	DefaultBackupMaxVersionsPerTarget = 32
	DefaultBackupMaxPinned            = 256
	DefaultBackupRetentionDays        = 30
	DefaultBackupPlanTTLSeconds       = 15 * 60

	HardMaxBackupTotalBytes        = int64(1 << 40) // 1 TiB.
	HardMaxBackupObjectBytes       = int64(1 << 30) // 1 GiB.
	HardMaxBackupManifests         = 1_000_000
	HardMaxBackupVersionsPerTarget = 10_000
	HardMaxBackupPinned            = 100_000
	HardMaxBackupRetentionDays     = 3650
	HardMaxBackupPlanTTLSeconds    = 24 * 60 * 60

	DefaultTaskMaxConcurrency       = 2
	DefaultTaskMaxQueued            = 64
	DefaultTaskMaxLogBytesPerStream = int64(8 * 1024 * 1024)
	DefaultTaskMaxRuntimeSeconds    = 0 // Unlimited unless the operator opts in.
	DefaultTaskRetentionDays        = 7
	DefaultTaskMaxTerminal          = 1000
	DefaultTaskMaxTotalBytes        = int64(512 * 1024 * 1024)

	HardMaxTaskConcurrency       = 32
	HardMaxTaskQueued            = 10_000
	HardMaxTaskLogBytesPerStream = int64(256 * 1024 * 1024)
	HardMaxTaskRuntimeSeconds    = 365 * 24 * 60 * 60
	HardMaxTaskRetentionDays     = 3650
	HardMaxTaskTerminal          = 1_000_000
	HardMaxTaskTotalBytes        = int64(1 << 40)
)

// Limits contains server-wide hard limits. Request-level limits may be lower
// but must never exceed these values.
type Limits struct {
	MaxFileBytes                  int64
	MaxDecodedCharacters          int
	MaxLineBytes                  int
	MaxBatchFiles                 int
	MaxMatches                    int
	MaxOutputBytes                int64
	MaxFingerprintEntries         int
	MaxFingerprintEntryDetails    int
	MaxEditPreviews               int
	MaxEditPreviewBytes           int64
	EditPreviewTTLSeconds         int
	MaxPatchPackageBytes          int64
	MaxPatchPackagePreparedBytes  int64
	MaxPatchPackagePreviews       int
	MaxPatchPackagePreviewBytes   int64
	PatchPackagePreviewTTLSeconds int
	MaxByteMutationPreviews       int
	MaxByteMutationPreviewBytes   int64
	ByteMutationPreviewTTLSeconds int
	MaxSessions                   int
}

// BackupLimits bounds the future persistent backup store independently from
// request output and source-file limits.
type BackupLimits struct {
	MaxTotalBytes        int64
	MaxObjectBytes       int64
	MaxManifests         int
	MaxVersionsPerTarget int
	MaxPinned            int
	RetentionDays        int
	PlanTTLSeconds       int
}

// BackupConfig contains the disabled-by-default persistent store configuration.
type BackupConfig struct {
	StoreDir      string
	DefaultPolicy string
	Limits        BackupLimits
}

// TaskConfig controls the durable asynchronous execution subsystem. The
// store remains disabled until an operator explicitly supplies StoreDir.
type TaskConfig struct {
	StoreDir             string
	MaxConcurrency       int
	MaxQueued            int
	MaxLogBytesPerStream int64
	MaxRuntimeSeconds    int
	RetentionDays        int
	MaxTerminal          int
	MaxTotalBytes        int64
}

// Enabled reports whether an operator explicitly configured a task store.
func (cfg TaskConfig) Enabled() bool { return cfg.StoreDir != "" }

// Enabled reports whether an operator explicitly configured a store directory.
func (cfg BackupConfig) Enabled() bool {
	return cfg.StoreDir != ""
}

// Config holds server configuration loaded from environment variables.
type Config struct {
	// DefaultEncoding is used for newly created files when no encoding is supplied.
	DefaultEncoding string
	Limits          Limits
	Backup          BackupConfig
	Tasks           TaskConfig
}

// Load reads configuration from the process environment with conservative defaults.
func Load() *Config {
	return LoadFromEnvironment(os.Getenv)
}

// LoadFromEnvironment reads configuration through getenv. It is used by the
// command bootstrap so tests and embedders can supply an isolated environment.
func LoadFromEnvironment(getenv func(string) string) *Config {
	if getenv == nil {
		getenv = os.Getenv
	}
	cfg := &Config{
		DefaultEncoding: DefaultEncoding,
		Limits: Limits{
			MaxFileBytes:                  DefaultMaxFileBytes,
			MaxDecodedCharacters:          DefaultMaxDecodedCharacters,
			MaxLineBytes:                  DefaultMaxLineBytes,
			MaxBatchFiles:                 DefaultMaxBatchFiles,
			MaxMatches:                    DefaultMaxMatches,
			MaxOutputBytes:                DefaultMaxOutputBytes,
			MaxFingerprintEntries:         DefaultMaxFingerprintEntries,
			MaxFingerprintEntryDetails:    DefaultMaxFingerprintEntryDetails,
			MaxEditPreviews:               DefaultMaxEditPreviews,
			MaxEditPreviewBytes:           DefaultMaxEditPreviewBytes,
			EditPreviewTTLSeconds:         DefaultEditPreviewTTLSeconds,
			MaxPatchPackageBytes:          DefaultMaxPatchPackageBytes,
			MaxPatchPackagePreparedBytes:  DefaultMaxPatchPackagePreparedBytes,
			MaxPatchPackagePreviews:       DefaultMaxPatchPackagePreviews,
			MaxPatchPackagePreviewBytes:   DefaultMaxPatchPackagePreviewBytes,
			PatchPackagePreviewTTLSeconds: DefaultPatchPackagePreviewTTLSeconds,
			MaxByteMutationPreviews:       DefaultMaxByteMutationPreviews,
			MaxByteMutationPreviewBytes:   DefaultMaxByteMutationPreviewBytes,
			ByteMutationPreviewTTLSeconds: DefaultByteMutationPreviewTTLSeconds,
			MaxSessions:                   DefaultMaxSessions,
		},
		Backup: BackupConfig{
			StoreDir:      getenv(EnvBackupStoreDir),
			DefaultPolicy: backupDefaultPolicyEnvironment(getenv),
			Limits: BackupLimits{
				MaxTotalBytes:        DefaultBackupMaxTotalBytes,
				MaxObjectBytes:       DefaultBackupMaxObjectBytes,
				MaxManifests:         DefaultBackupMaxManifests,
				MaxVersionsPerTarget: DefaultBackupMaxVersionsPerTarget,
				MaxPinned:            DefaultBackupMaxPinned,
				RetentionDays:        DefaultBackupRetentionDays,
				PlanTTLSeconds:       DefaultBackupPlanTTLSeconds,
			},
		},
		Tasks: TaskConfig{
			StoreDir:             getenv(EnvTaskStoreDir),
			MaxConcurrency:       DefaultTaskMaxConcurrency,
			MaxQueued:            DefaultTaskMaxQueued,
			MaxLogBytesPerStream: DefaultTaskMaxLogBytesPerStream,
			MaxRuntimeSeconds:    DefaultTaskMaxRuntimeSeconds,
			RetentionDays:        DefaultTaskRetentionDays,
			MaxTerminal:          DefaultTaskMaxTerminal,
			MaxTotalBytes:        DefaultTaskMaxTotalBytes,
		},
	}

	if enc := getenv(EnvDefaultEncoding); enc != "" {
		if canonical, ok := encoding.CanonicalName(enc); ok {
			cfg.DefaultEncoding = canonical
		} else {
			slog.Warn("invalid MCP_DEFAULT_ENCODING, using default", "value", enc, "fallback", DefaultEncoding)
		}
	}

	// Keep the 1.x threshold as a compatibility fallback. Specific 2.0 limits
	// below take precedence when both are configured.
	if legacy, ok := positiveInt64Environment(getenv, EnvMemoryThreshold); ok {
		cfg.Limits.MaxFileBytes = legacy
		cfg.Limits.MaxOutputBytes = legacy
	}

	cfg.Limits.MaxFileBytes = int64Environment(getenv, EnvMaxFileBytes, cfg.Limits.MaxFileBytes)
	cfg.Limits.MaxOutputBytes = int64Environment(getenv, EnvMaxOutputBytes, cfg.Limits.MaxOutputBytes)
	cfg.Limits.MaxDecodedCharacters = intEnvironment(getenv, EnvMaxDecodedCharacters, cfg.Limits.MaxDecodedCharacters)
	cfg.Limits.MaxLineBytes = intEnvironment(getenv, EnvMaxLineBytes, cfg.Limits.MaxLineBytes)
	cfg.Limits.MaxBatchFiles = intEnvironment(getenv, EnvMaxBatchFiles, cfg.Limits.MaxBatchFiles)
	cfg.Limits.MaxMatches = intEnvironment(getenv, EnvMaxMatches, cfg.Limits.MaxMatches)
	cfg.Limits.MaxFingerprintEntries = intEnvironment(getenv, EnvMaxFingerprintEntries, cfg.Limits.MaxFingerprintEntries)
	cfg.Limits.MaxFingerprintEntryDetails = intEnvironment(getenv, EnvMaxFingerprintEntryDetails, cfg.Limits.MaxFingerprintEntryDetails)
	cfg.Limits.MaxEditPreviews = intEnvironment(getenv, EnvMaxEditPreviews, cfg.Limits.MaxEditPreviews)
	cfg.Limits.MaxEditPreviewBytes = int64Environment(getenv, EnvMaxEditPreviewBytes, cfg.Limits.MaxEditPreviewBytes)
	cfg.Limits.EditPreviewTTLSeconds = intEnvironment(getenv, EnvEditPreviewTTLSeconds, cfg.Limits.EditPreviewTTLSeconds)
	cfg.Limits.MaxPatchPackageBytes = int64Environment(getenv, EnvMaxPatchPackageBytes, cfg.Limits.MaxPatchPackageBytes)
	cfg.Limits.MaxPatchPackagePreparedBytes = int64Environment(getenv, EnvMaxPatchPackagePreparedBytes, cfg.Limits.MaxPatchPackagePreparedBytes)
	cfg.Limits.MaxPatchPackagePreviews = intEnvironment(getenv, EnvMaxPatchPackagePreviews, cfg.Limits.MaxPatchPackagePreviews)
	cfg.Limits.MaxPatchPackagePreviewBytes = int64Environment(getenv, EnvMaxPatchPackagePreviewBytes, cfg.Limits.MaxPatchPackagePreviewBytes)
	cfg.Limits.PatchPackagePreviewTTLSeconds = intEnvironment(getenv, EnvPatchPackagePreviewTTLSeconds, cfg.Limits.PatchPackagePreviewTTLSeconds)
	cfg.Limits.MaxByteMutationPreviews = intEnvironment(getenv, EnvMaxByteMutationPreviews, cfg.Limits.MaxByteMutationPreviews)
	cfg.Limits.MaxByteMutationPreviewBytes = int64Environment(getenv, EnvMaxByteMutationPreviewBytes, cfg.Limits.MaxByteMutationPreviewBytes)
	cfg.Limits.ByteMutationPreviewTTLSeconds = intEnvironment(getenv, EnvByteMutationPreviewTTLSeconds, cfg.Limits.ByteMutationPreviewTTLSeconds)
	cfg.Limits.MaxSessions = intEnvironment(getenv, EnvMaxSessions, cfg.Limits.MaxSessions)

	cfg.Backup.Limits.MaxTotalBytes = boundedInt64Environment(getenv, EnvBackupMaxTotalBytes, cfg.Backup.Limits.MaxTotalBytes, HardMaxBackupTotalBytes)
	cfg.Backup.Limits.MaxObjectBytes = boundedInt64Environment(getenv, EnvBackupMaxObjectBytes, cfg.Backup.Limits.MaxObjectBytes, HardMaxBackupObjectBytes)
	cfg.Backup.Limits.MaxManifests = boundedIntEnvironment(getenv, EnvBackupMaxManifests, cfg.Backup.Limits.MaxManifests, HardMaxBackupManifests)
	cfg.Backup.Limits.MaxVersionsPerTarget = boundedIntEnvironment(getenv, EnvBackupMaxVersionsPerTarget, cfg.Backup.Limits.MaxVersionsPerTarget, HardMaxBackupVersionsPerTarget)
	cfg.Backup.Limits.MaxPinned = boundedIntEnvironment(getenv, EnvBackupMaxPinned, cfg.Backup.Limits.MaxPinned, HardMaxBackupPinned)
	cfg.Backup.Limits.RetentionDays = boundedIntEnvironment(getenv, EnvBackupRetentionDays, cfg.Backup.Limits.RetentionDays, HardMaxBackupRetentionDays)
	cfg.Backup.Limits.PlanTTLSeconds = boundedIntEnvironment(getenv, EnvBackupPlanTTLSeconds, cfg.Backup.Limits.PlanTTLSeconds, HardMaxBackupPlanTTLSeconds)
	cfg.Tasks.MaxConcurrency = boundedIntEnvironment(getenv, EnvTaskMaxConcurrency, cfg.Tasks.MaxConcurrency, HardMaxTaskConcurrency)
	cfg.Tasks.MaxQueued = boundedIntEnvironment(getenv, EnvTaskMaxQueued, cfg.Tasks.MaxQueued, HardMaxTaskQueued)
	cfg.Tasks.MaxLogBytesPerStream = boundedInt64Environment(getenv, EnvTaskMaxLogBytesPerStream, cfg.Tasks.MaxLogBytesPerStream, HardMaxTaskLogBytesPerStream)
	cfg.Tasks.MaxRuntimeSeconds = boundedNonNegativeIntEnvironment(getenv, EnvTaskMaxRuntimeSeconds, cfg.Tasks.MaxRuntimeSeconds, HardMaxTaskRuntimeSeconds)
	cfg.Tasks.RetentionDays = boundedIntEnvironment(getenv, EnvTaskRetentionDays, cfg.Tasks.RetentionDays, HardMaxTaskRetentionDays)
	cfg.Tasks.MaxTerminal = boundedIntEnvironment(getenv, EnvTaskMaxTerminal, cfg.Tasks.MaxTerminal, HardMaxTaskTerminal)
	cfg.Tasks.MaxTotalBytes = boundedInt64Environment(getenv, EnvTaskMaxTotalBytes, cfg.Tasks.MaxTotalBytes, HardMaxTaskTotalBytes)
	return cfg
}

func backupDefaultPolicyEnvironment(getenv func(string) string) string {
	value := strings.TrimSpace(getenv(EnvBackupDefaultPolicy))
	if value == "" {
		return DefaultBackupPolicy
	}
	if value == BackupPolicyDisabled || value == BackupPolicyRequired {
		return value
	}
	slog.Warn("invalid backup default policy, using fallback", "name", EnvBackupDefaultPolicy, "value", value, "fallback", DefaultBackupPolicy)
	return DefaultBackupPolicy
}

func boundedNonNegativeIntEnvironment(getenv func(string) string, name string, fallback, maximum int) int {
	value := getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 || int64(int(parsed)) != parsed || parsed > int64(maximum) {
		slog.Warn("invalid bounded non-negative integer environment value, using fallback", "name", name, "value", value, "maximum", maximum, "fallback", fallback)
		return fallback
	}
	return int(parsed)
}

func int64Environment(getenv func(string) string, name string, fallback int64) int64 {
	if value, ok := positiveInt64Environment(getenv, name); ok {
		return value
	}
	return fallback
}

func intEnvironment(getenv func(string) string, name string, fallback int) int {
	value := getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 || int64(int(parsed)) != parsed {
		slog.Warn("invalid positive integer environment value, using fallback", "name", name, "value", value, "fallback", fallback)
		return fallback
	}
	return int(parsed)
}

func boundedInt64Environment(getenv func(string) string, name string, fallback, maximum int64) int64 {
	value, ok := positiveInt64Environment(getenv, name)
	if !ok {
		return fallback
	}
	if value > maximum {
		slog.Warn("environment value exceeds hard maximum, using fallback", "name", name, "value", value, "maximum", maximum, "fallback", fallback)
		return fallback
	}
	return value
}

func boundedIntEnvironment(getenv func(string) string, name string, fallback, maximum int) int {
	value := intEnvironment(getenv, name, fallback)
	if value > maximum {
		slog.Warn("environment value exceeds hard maximum, using fallback", "name", name, "value", value, "maximum", maximum, "fallback", fallback)
		return fallback
	}
	return value
}

func positiveInt64Environment(getenv func(string) string, name string) (int64, bool) {
	value := getenv(name)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		slog.Warn("invalid positive integer environment value, using fallback", "name", name, "value", value)
		return 0, false
	}
	return parsed, true
}
