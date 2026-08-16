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
	EnvDefaultEncoding                    = "MCP_DEFAULT_ENCODING"
	EnvMemoryThreshold                    = "MCP_MEMORY_THRESHOLD" // Deprecated 1.x fallback for file/output limits.
	EnvMaxFileBytes                       = "MCP_MAX_FILE_BYTES"
	EnvMaxDecodedCharacters               = "MCP_MAX_DECODED_CHARACTERS"
	EnvMaxLineBytes                       = "MCP_MAX_LINE_BYTES"
	EnvMaxBatchFiles                      = "MCP_MAX_BATCH_FILES"
	EnvMaxMatches                         = "MCP_MAX_MATCHES"
	EnvMaxOutputBytes                     = "MCP_MAX_OUTPUT_BYTES"
	EnvMaxFingerprintEntries              = "MCP_MAX_FINGERPRINT_ENTRIES"
	EnvMaxFingerprintEntryDetails         = "MCP_MAX_FINGERPRINT_ENTRY_DETAILS"
	EnvMaxEditPreviews                    = "MCP_MAX_EDIT_PREVIEWS"
	EnvMaxEditPreviewBytes                = "MCP_MAX_EDIT_PREVIEW_BYTES"
	EnvEditPreviewTTLSeconds              = "MCP_EDIT_PREVIEW_TTL_SECONDS"
	EnvMaxPatchPackageBytes               = "MCP_MAX_PATCH_PACKAGE_BYTES"
	EnvMaxPatchPackagePreparedBytes       = "MCP_MAX_PATCH_PACKAGE_PREPARED_BYTES"
	EnvMaxPatchPackagePreviews            = "MCP_MAX_PATCH_PACKAGE_PREVIEWS"
	EnvMaxPatchPackagePreviewBytes        = "MCP_MAX_PATCH_PACKAGE_PREVIEW_BYTES"
	EnvPatchPackagePreviewTTLSeconds      = "MCP_PATCH_PACKAGE_PREVIEW_TTL_SECONDS"
	EnvMaxByteMutationPreviews            = "MCP_MAX_BYTE_MUTATION_PREVIEWS"
	EnvMaxByteMutationPreviewBytes        = "MCP_MAX_BYTE_MUTATION_PREVIEW_BYTES"
	EnvByteMutationPreviewTTLSeconds      = "MCP_BYTE_MUTATION_PREVIEW_TTL_SECONDS"
	EnvMaxFilesystemPackageOperations     = "MCP_MAX_FILESYSTEM_PACKAGE_OPERATIONS"
	EnvMaxFilesystemPackageBytes          = "MCP_MAX_FILESYSTEM_PACKAGE_BYTES"
	EnvMaxFilesystemRecursiveEntries      = "MCP_MAX_FILESYSTEM_RECURSIVE_ENTRIES"
	EnvMaxFilesystemRecursiveDepth        = "MCP_MAX_FILESYSTEM_RECURSIVE_DEPTH"
	EnvMaxFilesystemAggregateBytes        = "MCP_MAX_FILESYSTEM_AGGREGATE_BYTES"
	EnvMaxFilesystemStagingBytes          = "MCP_MAX_FILESYSTEM_STAGING_BYTES"
	EnvMaxFilesystemPackagePreviews       = "MCP_MAX_FILESYSTEM_PACKAGE_PREVIEWS"
	EnvMaxFilesystemPackagePreviewBytes   = "MCP_MAX_FILESYSTEM_PACKAGE_PREVIEW_BYTES"
	EnvFilesystemPackagePreviewTTLSeconds = "MCP_FILESYSTEM_PACKAGE_PREVIEW_TTL_SECONDS"
	EnvMaxSessions                        = "MCP_MAX_SESSIONS" // Maximum live native Streamable HTTP sessions.
	EnvSourceMaxInputPaths                = "MCP_SOURCE_MAX_INPUT_PATHS"
	EnvSourceMaxFiles                     = "MCP_SOURCE_MAX_FILES"
	EnvSourceMaxAggregateBytes            = "MCP_SOURCE_MAX_AGGREGATE_BYTES"
	EnvSourceMaxFileBytes                 = "MCP_SOURCE_MAX_FILE_BYTES"
	EnvSourceMaxSymbols                   = "MCP_SOURCE_MAX_SYMBOLS"
	EnvSourceMaxSignatureBytes            = "MCP_SOURCE_MAX_SIGNATURE_BYTES"
	EnvSourceMaxShowBytes                 = "MCP_SOURCE_MAX_SHOW_BYTES"
	EnvSourceMaxDiagnostics               = "MCP_SOURCE_MAX_DIAGNOSTICS"
	EnvSourceMaxDetectorProbes            = "MCP_SOURCE_MAX_DETECTOR_PROBES"
	EnvSourceMaxNesting                   = "MCP_SOURCE_MAX_NESTING"
	EnvSourceMaxConcurrency               = "MCP_SOURCE_MAX_CONCURRENCY"
	EnvSourceMaxRequestSeconds            = "MCP_SOURCE_MAX_REQUEST_SECONDS"
	EnvSourceMaxOutputBytes               = "MCP_SOURCE_MAX_OUTPUT_BYTES"
	EnvSourceMaxResults                   = "MCP_SOURCE_MAX_RESULTS"
	EnvSourceMaxGraphNodes                = "MCP_SOURCE_MAX_GRAPH_NODES"
	EnvSourceMaxGraphEdges                = "MCP_SOURCE_MAX_GRAPH_EDGES"
	EnvSourceMaxGraphDepth                = "MCP_SOURCE_MAX_GRAPH_DEPTH"
	EnvSourceMaxContextBytes              = "MCP_SOURCE_MAX_CONTEXT_BYTES"
	EnvSourceMaxContextItems              = "MCP_SOURCE_MAX_CONTEXT_ITEMS"
	EnvSourceMaxIndexProjects             = "MCP_SOURCE_MAX_INDEX_PROJECTS"
	EnvSourceMaxIndexGenerations          = "MCP_SOURCE_MAX_INDEX_GENERATIONS"

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

	DefaultEncoding                           = "utf-8"
	DefaultMaxFileBytes                       = int64(64 * 1024 * 1024)
	DefaultMaxDecodedCharacters               = 16 * 1024 * 1024
	DefaultMaxLineBytes                       = 16 * 1024 * 1024
	DefaultMaxBatchFiles                      = 256
	DefaultMaxMatches                         = 10_000
	DefaultMaxOutputBytes                     = int64(64 * 1024 * 1024)
	DefaultMaxFingerprintEntries              = 100_000
	DefaultMaxFingerprintEntryDetails         = 1_000
	DefaultMaxEditPreviews                    = 128
	DefaultMaxEditPreviewBytes                = int64(64 * 1024 * 1024)
	DefaultEditPreviewTTLSeconds              = 15 * 60
	DefaultMaxPatchPackageBytes               = int64(16 * 1024 * 1024)
	DefaultMaxPatchPackagePreparedBytes       = int64(64 * 1024 * 1024)
	DefaultMaxPatchPackagePreviews            = 16
	DefaultMaxPatchPackagePreviewBytes        = int64(128 * 1024 * 1024)
	DefaultPatchPackagePreviewTTLSeconds      = 15 * 60
	DefaultMaxByteMutationPreviews            = 32
	DefaultMaxByteMutationPreviewBytes        = int64(256 * 1024 * 1024)
	DefaultByteMutationPreviewTTLSeconds      = 15 * 60
	DefaultMaxFilesystemPackageOperations     = 256
	DefaultMaxFilesystemPackageBytes          = int64(16 * 1024 * 1024)
	DefaultMaxFilesystemRecursiveEntries      = 100_000
	DefaultMaxFilesystemRecursiveDepth        = 128
	DefaultMaxFilesystemAggregateBytes        = int64(1024 * 1024 * 1024)
	DefaultMaxFilesystemStagingBytes          = int64(1024 * 1024 * 1024)
	DefaultMaxFilesystemPackagePreviews       = 16
	DefaultMaxFilesystemPackagePreviewBytes   = int64(128 * 1024 * 1024)
	DefaultFilesystemPackagePreviewTTLSeconds = 15 * 60
	DefaultMaxSessions                        = 128

	DefaultSourceMaxInputPaths       = 32
	DefaultSourceMaxFiles            = 256
	DefaultSourceMaxAggregateBytes   = int64(64 * 1024 * 1024)
	DefaultSourceMaxFileBytes        = int64(8 * 1024 * 1024)
	DefaultSourceMaxSymbols          = 10_000
	DefaultSourceMaxSignatureBytes   = 8 * 1024
	DefaultSourceMaxShowBytes        = 1024 * 1024
	DefaultSourceMaxDiagnostics      = 256
	DefaultSourceMaxDetectorProbes   = 4
	DefaultSourceMaxNesting          = 256
	DefaultSourceMaxConcurrency      = 4
	DefaultSourceMaxRequestSeconds   = 30
	DefaultSourceMaxOutputBytes      = int64(16 * 1024 * 1024)
	DefaultSourceMaxResults          = 10_000
	DefaultSourceMaxGraphNodes       = 5_000
	DefaultSourceMaxGraphEdges       = 20_000
	DefaultSourceMaxGraphDepth       = 8
	DefaultSourceMaxContextBytes     = 1024 * 1024
	DefaultSourceMaxContextItems     = 256
	DefaultSourceMaxIndexProjects    = 4
	DefaultSourceMaxIndexGenerations = 2

	HardMaxSourceInputPaths       = 256
	HardMaxSourceFiles            = 4_096
	HardMaxSourceAggregateBytes   = int64(512 * 1024 * 1024)
	HardMaxSourceFileBytes        = int64(64 * 1024 * 1024)
	HardMaxSourceSymbols          = 100_000
	HardMaxSourceSignatureBytes   = 64 * 1024
	HardMaxSourceShowBytes        = 8 * 1024 * 1024
	HardMaxSourceDiagnostics      = 4_096
	HardMaxSourceDetectorProbes   = 16
	HardMaxSourceNesting          = 2_048
	HardMaxSourceConcurrency      = 32
	HardMaxSourceRequestSeconds   = 300
	HardMaxSourceOutputBytes      = int64(64 * 1024 * 1024)
	HardMaxSourceResults          = 100_000
	HardMaxSourceGraphNodes       = 50_000
	HardMaxSourceGraphEdges       = 200_000
	HardMaxSourceGraphDepth       = 64
	HardMaxSourceContextBytes     = 8 * 1024 * 1024
	HardMaxSourceContextItems     = 4_096
	HardMaxSourceIndexProjects    = 16
	HardMaxSourceIndexGenerations = 4

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

	HardMaxFilesystemPackageOperations     = 4_096
	HardMaxFilesystemPackageBytes          = int64(64 * 1024 * 1024)
	HardMaxFilesystemRecursiveEntries      = 1_000_000
	HardMaxFilesystemRecursiveDepth        = 1_024
	HardMaxFilesystemAggregateBytes        = int64(1 << 40)
	HardMaxFilesystemStagingBytes          = int64(1 << 40)
	HardMaxFilesystemPackagePreviews       = 1_024
	HardMaxFilesystemPackagePreviewBytes   = int64(1 << 30)
	HardFilesystemPackagePreviewTTLSeconds = 24 * 60 * 60

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
	MaxFileBytes                       int64
	MaxDecodedCharacters               int
	MaxLineBytes                       int
	MaxBatchFiles                      int
	MaxMatches                         int
	MaxOutputBytes                     int64
	MaxFingerprintEntries              int
	MaxFingerprintEntryDetails         int
	MaxEditPreviews                    int
	MaxEditPreviewBytes                int64
	EditPreviewTTLSeconds              int
	MaxPatchPackageBytes               int64
	MaxPatchPackagePreparedBytes       int64
	MaxPatchPackagePreviews            int
	MaxPatchPackagePreviewBytes        int64
	PatchPackagePreviewTTLSeconds      int
	MaxByteMutationPreviews            int
	MaxByteMutationPreviewBytes        int64
	ByteMutationPreviewTTLSeconds      int
	MaxFilesystemPackageOperations     int
	MaxFilesystemPackageBytes          int64
	MaxFilesystemRecursiveEntries      int
	MaxFilesystemRecursiveDepth        int
	MaxFilesystemAggregateBytes        int64
	MaxFilesystemStagingBytes          int64
	MaxFilesystemPackagePreviews       int
	MaxFilesystemPackagePreviewBytes   int64
	FilesystemPackagePreviewTTLSeconds int
	MaxSessions                        int
}

// SourceConfig bounds R25 source-intelligence work independently from ordinary
// file operations. Effective shared limits are the minimum of these values and
// the corresponding server-wide limits.
type SourceConfig struct {
	MaxInputPaths       int
	MaxFiles            int
	MaxAggregateBytes   int64
	MaxFileBytes        int64
	MaxSymbols          int
	MaxSignatureBytes   int
	MaxShowBytes        int
	MaxDiagnostics      int
	MaxDetectorProbes   int
	MaxNesting          int
	MaxConcurrency      int
	MaxRequestSeconds   int
	MaxOutputBytes      int64
	MaxResults          int
	MaxGraphNodes       int
	MaxGraphEdges       int
	MaxGraphDepth       int
	MaxContextBytes     int
	MaxContextItems     int
	MaxIndexProjects    int
	MaxIndexGenerations int
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
	Source          SourceConfig
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
			MaxFileBytes:                       DefaultMaxFileBytes,
			MaxDecodedCharacters:               DefaultMaxDecodedCharacters,
			MaxLineBytes:                       DefaultMaxLineBytes,
			MaxBatchFiles:                      DefaultMaxBatchFiles,
			MaxMatches:                         DefaultMaxMatches,
			MaxOutputBytes:                     DefaultMaxOutputBytes,
			MaxFingerprintEntries:              DefaultMaxFingerprintEntries,
			MaxFingerprintEntryDetails:         DefaultMaxFingerprintEntryDetails,
			MaxEditPreviews:                    DefaultMaxEditPreviews,
			MaxEditPreviewBytes:                DefaultMaxEditPreviewBytes,
			EditPreviewTTLSeconds:              DefaultEditPreviewTTLSeconds,
			MaxPatchPackageBytes:               DefaultMaxPatchPackageBytes,
			MaxPatchPackagePreparedBytes:       DefaultMaxPatchPackagePreparedBytes,
			MaxPatchPackagePreviews:            DefaultMaxPatchPackagePreviews,
			MaxPatchPackagePreviewBytes:        DefaultMaxPatchPackagePreviewBytes,
			PatchPackagePreviewTTLSeconds:      DefaultPatchPackagePreviewTTLSeconds,
			MaxByteMutationPreviews:            DefaultMaxByteMutationPreviews,
			MaxByteMutationPreviewBytes:        DefaultMaxByteMutationPreviewBytes,
			ByteMutationPreviewTTLSeconds:      DefaultByteMutationPreviewTTLSeconds,
			MaxFilesystemPackageOperations:     DefaultMaxFilesystemPackageOperations,
			MaxFilesystemPackageBytes:          DefaultMaxFilesystemPackageBytes,
			MaxFilesystemRecursiveEntries:      DefaultMaxFilesystemRecursiveEntries,
			MaxFilesystemRecursiveDepth:        DefaultMaxFilesystemRecursiveDepth,
			MaxFilesystemAggregateBytes:        DefaultMaxFilesystemAggregateBytes,
			MaxFilesystemStagingBytes:          DefaultMaxFilesystemStagingBytes,
			MaxFilesystemPackagePreviews:       DefaultMaxFilesystemPackagePreviews,
			MaxFilesystemPackagePreviewBytes:   DefaultMaxFilesystemPackagePreviewBytes,
			FilesystemPackagePreviewTTLSeconds: DefaultFilesystemPackagePreviewTTLSeconds,
			MaxSessions:                        DefaultMaxSessions,
		},
		Source: SourceConfig{
			MaxInputPaths:       DefaultSourceMaxInputPaths,
			MaxFiles:            DefaultSourceMaxFiles,
			MaxAggregateBytes:   DefaultSourceMaxAggregateBytes,
			MaxFileBytes:        DefaultSourceMaxFileBytes,
			MaxSymbols:          DefaultSourceMaxSymbols,
			MaxSignatureBytes:   DefaultSourceMaxSignatureBytes,
			MaxShowBytes:        DefaultSourceMaxShowBytes,
			MaxDiagnostics:      DefaultSourceMaxDiagnostics,
			MaxDetectorProbes:   DefaultSourceMaxDetectorProbes,
			MaxNesting:          DefaultSourceMaxNesting,
			MaxConcurrency:      DefaultSourceMaxConcurrency,
			MaxRequestSeconds:   DefaultSourceMaxRequestSeconds,
			MaxOutputBytes:      DefaultSourceMaxOutputBytes,
			MaxResults:          DefaultSourceMaxResults,
			MaxGraphNodes:       DefaultSourceMaxGraphNodes,
			MaxGraphEdges:       DefaultSourceMaxGraphEdges,
			MaxGraphDepth:       DefaultSourceMaxGraphDepth,
			MaxContextBytes:     DefaultSourceMaxContextBytes,
			MaxContextItems:     DefaultSourceMaxContextItems,
			MaxIndexProjects:    DefaultSourceMaxIndexProjects,
			MaxIndexGenerations: DefaultSourceMaxIndexGenerations,
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
	cfg.Limits.MaxFilesystemPackageOperations = boundedIntEnvironment(getenv, EnvMaxFilesystemPackageOperations, cfg.Limits.MaxFilesystemPackageOperations, HardMaxFilesystemPackageOperations)
	cfg.Limits.MaxFilesystemPackageBytes = boundedInt64Environment(getenv, EnvMaxFilesystemPackageBytes, cfg.Limits.MaxFilesystemPackageBytes, HardMaxFilesystemPackageBytes)
	cfg.Limits.MaxFilesystemRecursiveEntries = boundedIntEnvironment(getenv, EnvMaxFilesystemRecursiveEntries, cfg.Limits.MaxFilesystemRecursiveEntries, HardMaxFilesystemRecursiveEntries)
	cfg.Limits.MaxFilesystemRecursiveDepth = boundedIntEnvironment(getenv, EnvMaxFilesystemRecursiveDepth, cfg.Limits.MaxFilesystemRecursiveDepth, HardMaxFilesystemRecursiveDepth)
	cfg.Limits.MaxFilesystemAggregateBytes = boundedInt64Environment(getenv, EnvMaxFilesystemAggregateBytes, cfg.Limits.MaxFilesystemAggregateBytes, HardMaxFilesystemAggregateBytes)
	cfg.Limits.MaxFilesystemStagingBytes = boundedInt64Environment(getenv, EnvMaxFilesystemStagingBytes, cfg.Limits.MaxFilesystemStagingBytes, HardMaxFilesystemStagingBytes)
	cfg.Limits.MaxFilesystemPackagePreviews = boundedIntEnvironment(getenv, EnvMaxFilesystemPackagePreviews, cfg.Limits.MaxFilesystemPackagePreviews, HardMaxFilesystemPackagePreviews)
	cfg.Limits.MaxFilesystemPackagePreviewBytes = boundedInt64Environment(getenv, EnvMaxFilesystemPackagePreviewBytes, cfg.Limits.MaxFilesystemPackagePreviewBytes, HardMaxFilesystemPackagePreviewBytes)
	cfg.Limits.FilesystemPackagePreviewTTLSeconds = boundedIntEnvironment(getenv, EnvFilesystemPackagePreviewTTLSeconds, cfg.Limits.FilesystemPackagePreviewTTLSeconds, HardFilesystemPackagePreviewTTLSeconds)
	cfg.Limits.MaxSessions = intEnvironment(getenv, EnvMaxSessions, cfg.Limits.MaxSessions)

	cfg.Source.MaxInputPaths = boundedIntEnvironment(getenv, EnvSourceMaxInputPaths, cfg.Source.MaxInputPaths, HardMaxSourceInputPaths)
	cfg.Source.MaxFiles = boundedIntEnvironment(getenv, EnvSourceMaxFiles, cfg.Source.MaxFiles, HardMaxSourceFiles)
	cfg.Source.MaxAggregateBytes = boundedInt64Environment(getenv, EnvSourceMaxAggregateBytes, cfg.Source.MaxAggregateBytes, HardMaxSourceAggregateBytes)
	cfg.Source.MaxFileBytes = boundedInt64Environment(getenv, EnvSourceMaxFileBytes, cfg.Source.MaxFileBytes, HardMaxSourceFileBytes)
	cfg.Source.MaxSymbols = boundedIntEnvironment(getenv, EnvSourceMaxSymbols, cfg.Source.MaxSymbols, HardMaxSourceSymbols)
	cfg.Source.MaxSignatureBytes = boundedIntEnvironment(getenv, EnvSourceMaxSignatureBytes, cfg.Source.MaxSignatureBytes, HardMaxSourceSignatureBytes)
	cfg.Source.MaxShowBytes = boundedIntEnvironment(getenv, EnvSourceMaxShowBytes, cfg.Source.MaxShowBytes, HardMaxSourceShowBytes)
	cfg.Source.MaxDiagnostics = boundedIntEnvironment(getenv, EnvSourceMaxDiagnostics, cfg.Source.MaxDiagnostics, HardMaxSourceDiagnostics)
	cfg.Source.MaxDetectorProbes = boundedIntEnvironment(getenv, EnvSourceMaxDetectorProbes, cfg.Source.MaxDetectorProbes, HardMaxSourceDetectorProbes)
	cfg.Source.MaxNesting = boundedIntEnvironment(getenv, EnvSourceMaxNesting, cfg.Source.MaxNesting, HardMaxSourceNesting)
	cfg.Source.MaxConcurrency = boundedIntEnvironment(getenv, EnvSourceMaxConcurrency, cfg.Source.MaxConcurrency, HardMaxSourceConcurrency)
	cfg.Source.MaxRequestSeconds = boundedIntEnvironment(getenv, EnvSourceMaxRequestSeconds, cfg.Source.MaxRequestSeconds, HardMaxSourceRequestSeconds)
	cfg.Source.MaxOutputBytes = boundedInt64Environment(getenv, EnvSourceMaxOutputBytes, cfg.Source.MaxOutputBytes, HardMaxSourceOutputBytes)
	cfg.Source.MaxResults = boundedIntEnvironment(getenv, EnvSourceMaxResults, cfg.Source.MaxResults, HardMaxSourceResults)
	cfg.Source.MaxGraphNodes = boundedIntEnvironment(getenv, EnvSourceMaxGraphNodes, cfg.Source.MaxGraphNodes, HardMaxSourceGraphNodes)
	cfg.Source.MaxGraphEdges = boundedIntEnvironment(getenv, EnvSourceMaxGraphEdges, cfg.Source.MaxGraphEdges, HardMaxSourceGraphEdges)
	cfg.Source.MaxGraphDepth = boundedIntEnvironment(getenv, EnvSourceMaxGraphDepth, cfg.Source.MaxGraphDepth, HardMaxSourceGraphDepth)
	cfg.Source.MaxContextBytes = boundedIntEnvironment(getenv, EnvSourceMaxContextBytes, cfg.Source.MaxContextBytes, HardMaxSourceContextBytes)
	cfg.Source.MaxContextItems = boundedIntEnvironment(getenv, EnvSourceMaxContextItems, cfg.Source.MaxContextItems, HardMaxSourceContextItems)
	cfg.Source.MaxIndexProjects = boundedIntEnvironment(getenv, EnvSourceMaxIndexProjects, cfg.Source.MaxIndexProjects, HardMaxSourceIndexProjects)
	cfg.Source.MaxIndexGenerations = boundedIntEnvironment(getenv, EnvSourceMaxIndexGenerations, cfg.Source.MaxIndexGenerations, HardMaxSourceIndexGenerations)

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
