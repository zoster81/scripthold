package handler

import "github.com/zoster81/scripthold/internal/encoding"

// ReadTextFileInput for reading files with encoding support.
// Offset/Limit are 1-indexed line numbers for partial reads.
type ReadTextFileInput struct {
	Path          string `json:"path"`
	Encoding      string `json:"encoding,omitempty"`
	Offset        *int   `json:"offset,omitempty"`
	Limit         *int   `json:"limit,omitempty"`
	MaxCharacters *int   `json:"maxCharacters,omitempty"`
	LineNumbers   bool   `json:"lineNumbers,omitempty"`

	maxOutputBytes  int
	outputLimitName string
}

type ReadTextFileOutput struct {
	Content            string `json:"content"`
	TotalLines         int    `json:"totalLines"`
	FileSizeBytes      int64  `json:"fileSizeBytes"`
	StartLine          int    `json:"startLine,omitempty"`
	EndLine            int    `json:"endLine,omitempty"`
	Truncated          bool   `json:"truncated,omitempty"`
	DetectedEncoding   string `json:"detectedEncoding,omitempty"`
	EncodingConfidence int    `json:"encodingConfidence,omitempty"`
	HasBOM             bool   `json:"hasBOM,omitempty"`
	BOMType            string `json:"bomType,omitempty"`
}

// WriteWholeFileInput replaces the complete target contents. New files default
// to UTF-8 when encoding is omitted and no existing encoding can be preserved.
// Legacy encodings remain explicit options. BOM accepts "auto" (default),
// "always", "never", or "preserve".
type WriteWholeFileInput struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding,omitempty"`
	BOM      string `json:"bom,omitempty"`
}

type WriteWholeFileOutput struct {
	Message           string `json:"message"`
	Encoding          string `json:"encoding"`
	BOMPolicy         string `json:"bomPolicy"`
	HasBOM            bool   `json:"hasBOM"`
	BOMType           string `json:"bomType,omitempty"`
	TargetFingerprint string `json:"targetFingerprint,omitempty"`
	ResultFingerprint string `json:"resultFingerprint,omitempty"`
	ActualFingerprint string `json:"actualFingerprint,omitempty"`
	State             string `json:"state,omitempty"`
	Changed           bool   `json:"changed"`
	Applied           bool   `json:"applied"`
}

type ListDirectoryInput struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern,omitempty"` // glob pattern, e.g. *.pas
	SortBy  string `json:"sortBy,omitempty"`  // name (default), mtime, size
	Reverse bool   `json:"reverse,omitempty"`
}

type ListDirectoryOutput struct {
	Files []string `json:"files"`
}

type ListEncodingsInput struct{}

type ListEncodingsOutput struct {
	Encodings []encoding.EncodingListItem `json:"encodings"`
}

// DetectEncodingInput supports three modes: "sample" (default), "chunked", "full"
type DetectEncodingInput struct {
	Path string `json:"path"`
	Mode string `json:"mode,omitempty"`
}

type DetectEncodingOutput struct {
	Encoding   string `json:"encoding"`
	Confidence int    `json:"confidence"`
	HasBOM     bool   `json:"hasBOM"`
	BOMType    string `json:"bomType,omitempty"`
	Ambiguous  bool   `json:"ambiguous,omitempty"`
	Assumed    bool   `json:"assumed,omitempty"`
}

type ListAllowedDirectoriesInput struct{}

type ListAllowedDirectoriesOutput struct {
	Directories []string `json:"directories"`
	Message     string   `json:"message,omitempty"`
}

type GetFileInfoInput struct {
	Path string `json:"path"`
}

type GetFileInfoOutput struct {
	Size        int64  `json:"size"`
	Created     string `json:"created"`
	Modified    string `json:"modified"`
	Accessed    string `json:"accessed"`
	IsDirectory bool   `json:"isDirectory"`
	IsFile      bool   `json:"isFile"`
	Permissions string `json:"permissions"`
}

type CreateDirectoryInput struct {
	Path string `json:"path"`
}

type CreateDirectoryOutput struct {
	Message string `json:"message"`
}

type MoveFileInput struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type MoveFileOutput struct {
	Message string `json:"message"`
}

// SearchFilesInput - pattern supports *.ext and **/*.ext syntax
type SearchFilesInput struct {
	Path             string   `json:"path"`
	Pattern          string   `json:"pattern"`
	ExcludePatterns  []string `json:"excludePatterns,omitempty"`
	RespectGitignore *bool    `json:"respectGitignore,omitempty"`
	MaxResults       int      `json:"maxResults,omitempty"`
	SortBy           string   `json:"sortBy,omitempty"` // name, mtime, size; omitted preserves traversal order
	Reverse          bool     `json:"reverse,omitempty"`
}

type SearchFilesOutput struct {
	Files     []string `json:"files"`
	Truncated bool     `json:"truncated,omitempty"`
}

type FingerprintPathsInput struct {
	Paths            []string `json:"paths"`
	RespectGitignore *bool    `json:"respectGitignore,omitempty"`
	IncludeEntries   bool     `json:"includeEntries,omitempty"`
	MaxEntryDetails  int      `json:"maxEntryDetails,omitempty"`
}

type FingerprintEntry struct {
	RootIndex int    `json:"rootIndex"`
	Path      string `json:"path"`
	Type      string `json:"type"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256,omitempty"`
}

type FingerprintPathsOutput struct {
	Algorithm        string             `json:"algorithm"`
	Mode             string             `json:"mode"`
	Fingerprint      string             `json:"fingerprint"`
	RootCount        int                `json:"rootCount"`
	FileCount        int                `json:"fileCount"`
	DirectoryCount   int                `json:"directoryCount"`
	TotalBytes       int64              `json:"totalBytes"`
	Entries          []FingerprintEntry `json:"entries,omitempty"`
	EntriesTruncated bool               `json:"entriesTruncated,omitempty"`
}

type EditOperation struct {
	OldText    string   `json:"oldText"`
	NewText    string   `json:"newText"`
	Similarity *float64 `json:"similarity,omitempty"`
}

// EditFileInput performs a direct edit by default, prepares one bounded preview,
// or applies one previously prepared preview capability.
type EditFileInput struct {
	Action        string          `json:"action,omitempty"` // direct (default), preview, apply
	PreviewID     string          `json:"previewId,omitempty"`
	Path          string          `json:"path,omitempty"`
	Edits         []EditOperation `json:"edits,omitempty"`
	Patch         string          `json:"patch,omitempty"`
	DryRun        bool            `json:"dryRun,omitempty"`
	Encoding      string          `json:"encoding,omitempty"`
	ForceWritable *bool           `json:"forceWritable,omitempty"` // default: false - fail on read-only files
	BackupPolicy  string          `json:"backupPolicy,omitempty"`  // preview only: required
}

type EditFileOutput struct {
	Action            string `json:"action,omitempty"`
	Diff              string `json:"diff"`
	ReadOnlyCleared   bool   `json:"readOnlyCleared,omitempty"` // true if read-only flag was cleared
	PreviewID         string `json:"previewId,omitempty"`
	CreatedAt         string `json:"createdAt,omitempty"`
	ExpiresAt         string `json:"expiresAt,omitempty"`
	TargetPath        string `json:"targetPath,omitempty"`
	TargetFingerprint string `json:"targetFingerprint,omitempty"`
	ResultFingerprint string `json:"resultFingerprint,omitempty"`
	ActualFingerprint string `json:"actualFingerprint,omitempty"`
	State             string `json:"state,omitempty"`
	Encoding          string `json:"encoding,omitempty"`
	HasBOM            bool   `json:"hasBOM,omitempty"`
	BOMType           string `json:"bomType,omitempty"`
	LineEndingStyle   string `json:"lineEndingStyle,omitempty"`
	BackupPolicy      string `json:"backupPolicy,omitempty"`
	BackupID          string `json:"backupId,omitempty"`
	Changed           bool   `json:"changed"`
	Applied           bool   `json:"applied"`
}

type ReadMultipleFilesInput struct {
	Paths    []string `json:"paths"`
	Encoding string   `json:"encoding,omitempty"`
}

// ErrorCodeMetaKey is the stable MCP _meta key used by single-tool errors.
const ErrorCodeMetaKey = "errorCode"

// Stable 2.0 error codes used by both single-tool metadata and batch items.
const (
	ErrCodeNone              = ""
	ErrCodeInvalidInput      = "INVALID_INPUT"
	ErrCodeInvalidPath       = "INVALID_PATH"
	ErrCodeAccessDenied      = "ACCESS_DENIED"
	ErrCodeSymlinkEscape     = "SYMLINK_ESCAPE"
	ErrCodeNotFound          = "NOT_FOUND"
	ErrCodePermission        = "PERMISSION"
	ErrCodeEncoding          = "ENCODING"
	ErrCodeEncodingAmbiguous = "ENCODING_AMBIGUOUS"
	ErrCodeConflict          = "CONFLICT"
	ErrCodePartialCommit     = "PARTIAL_COMMIT"
	ErrCodeUnsupported       = "UNSUPPORTED"
	ErrCodeCancelled         = "CANCELLED"
	ErrCodeLimit             = "LIMIT"
	ErrCodeIO                = "IO_ERROR"
	ErrCodeInternal          = "INTERNAL_ERROR"
	ErrCodeOperationFailed   = "OPERATION_FAILED"
)

type FileReadResult struct {
	Path               string `json:"path"`
	Content            string `json:"content,omitempty"`
	Error              string `json:"error,omitempty"`
	ErrorCode          string `json:"errorCode,omitempty"` // Machine-readable error code
	EncodingErrorCode  string `json:"encodingErrorCode,omitempty"`
	DetectedEncoding   string `json:"detectedEncoding,omitempty"`
	EncodingConfidence int    `json:"encodingConfidence,omitempty"`
	HasBOM             bool   `json:"hasBOM,omitempty"`
	BOMType            string `json:"bomType,omitempty"`
}

type ReadMultipleFilesOutput struct {
	Results         []FileReadResult `json:"results"`
	SuccessCount    int              `json:"successCount"`
	ErrorCount      int              `json:"errorCount"`
	Errors          []string         `json:"errors,omitempty"`
	ErrorsTruncated bool             `json:"errorsTruncated,omitempty"`
	ErrorsOmitted   int              `json:"errorsOmitted,omitempty"`
}

// TreeInput for compact tree view. MaxFiles defaults to 1000.
type TreeInput struct {
	Path             string   `json:"path"`
	MaxDepth         int      `json:"maxDepth,omitempty"`
	MaxFiles         int      `json:"maxFiles,omitempty"`
	DirsOnly         bool     `json:"dirsOnly,omitempty"`
	Exclude          []string `json:"exclude,omitempty"`
	ShowEncoding     bool     `json:"showEncoding,omitempty"`
	RespectGitignore *bool    `json:"respectGitignore,omitempty"`
}

type TreeOutput struct {
	Tree      string `json:"tree"`
	FileCount int    `json:"fileCount"`
	DirCount  int    `json:"dirCount"`
	Truncated bool   `json:"truncated,omitempty"`
}

type DeleteFileInput struct {
	Path string `json:"path"`
}

type DeleteFileOutput struct {
	Message string `json:"message"`
}

type CopyFileInput struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type CopyFileOutput struct {
	Message string `json:"message"`
}

// ConvertEncodingInput converts between encodings. From is auto-detected if empty.
// BOM accepts "auto" (default), "always", "never", or "preserve".
type ConvertEncodingInput struct {
	Path   string   `json:"path,omitempty"`
	Paths  []string `json:"paths,omitempty"`
	From   string   `json:"from,omitempty"`
	To     string   `json:"to"`
	Backup bool     `json:"backup,omitempty"`
	BOM    string   `json:"bom,omitempty"`
	DryRun bool     `json:"dryRun,omitempty"`
}

type UnsupportedCharacter struct {
	Rune   string `json:"rune"`
	Code   string `json:"code"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type ConvertFileResult struct {
	Path              string                 `json:"path"`
	TargetFingerprint string                 `json:"targetFingerprint,omitempty"`
	ResultFingerprint string                 `json:"resultFingerprint,omitempty"`
	BackupID          string                 `json:"backupId,omitempty"`
	Applied           bool                   `json:"applied,omitempty"`
	SourceEncoding    string                 `json:"sourceEncoding,omitempty"`
	Changed           bool                   `json:"changed"`
	Message           string                 `json:"message,omitempty"`
	Error             string                 `json:"error,omitempty"`
	ErrorCode         string                 `json:"errorCode,omitempty"`
	EncodingErrorCode string                 `json:"encodingErrorCode,omitempty"`
	BackupPath        string                 `json:"backupPath,omitempty"`
	BOMPolicy         string                 `json:"bomPolicy,omitempty"`
	HasBOM            bool                   `json:"hasBOM,omitempty"`
	BOMType           string                 `json:"bomType,omitempty"`
	Unsupported       []UnsupportedCharacter `json:"unsupported,omitempty"`
	UnsupportedCount  int                    `json:"unsupportedCount,omitempty"`
}

type ConvertEncodingOutput struct {
	Message           string              `json:"message"`
	PreviewID         string              `json:"previewId,omitempty"`
	CreatedAt         string              `json:"createdAt,omitempty"`
	ExpiresAt         string              `json:"expiresAt,omitempty"`
	BackupPolicy      string              `json:"backupPolicy,omitempty"`
	TargetFingerprint string              `json:"targetFingerprint,omitempty"`
	ResultFingerprint string              `json:"resultFingerprint,omitempty"`
	BackupID          string              `json:"backupId,omitempty"`
	Applied           bool                `json:"applied,omitempty"`
	PartialCommit     bool                `json:"partialCommit,omitempty"`
	CommittedCount    int                 `json:"committedCount,omitempty"`
	SourceEncoding    string              `json:"sourceEncoding"`
	TargetEncoding    string              `json:"targetEncoding"`
	BackupPath        string              `json:"backupPath,omitempty"`
	BOMPolicy         string              `json:"bomPolicy"`
	HasBOM            bool                `json:"hasBOM"`
	BOMType           string              `json:"bomType,omitempty"`
	Changed           bool                `json:"changed"`
	DryRun            bool                `json:"dryRun,omitempty"`
	Results           []ConvertFileResult `json:"results,omitempty"`
	SuccessCount      int                 `json:"successCount,omitempty"`
	ErrorCount        int                 `json:"errorCount,omitempty"`
	Errors            []string            `json:"errors,omitempty"`
	ErrorsTruncated   bool                `json:"errorsTruncated,omitempty"`
	ErrorsOmitted     int                 `json:"errorsOmitted,omitempty"`
}

// GrepInput for searching file contents with regex
type GrepInput struct {
	Pattern          string   `json:"pattern,omitempty"`
	Patterns         []string `json:"patterns,omitempty"`
	Paths            []string `json:"paths"`
	CaseSensitive    *bool    `json:"caseSensitive,omitempty"` // defaults to true
	ContextBefore    int      `json:"contextBefore,omitempty"`
	ContextAfter     int      `json:"contextAfter,omitempty"`
	MaxMatches       int      `json:"maxMatches,omitempty"` // defaults to 1000
	Include          string   `json:"include,omitempty"`
	Exclude          string   `json:"exclude,omitempty"`
	Includes         []string `json:"includes,omitempty"`
	Excludes         []string `json:"excludes,omitempty"`
	Encoding         string   `json:"encoding,omitempty"`
	OutputMode       string   `json:"outputMode,omitempty"` // content (default), files_with_matches, count
	MatchesOnly      bool     `json:"matchesOnly,omitempty"`
	Offset           int      `json:"offset,omitempty"`
	RespectGitignore *bool    `json:"respectGitignore,omitempty"`
}

type GrepMatch struct {
	Path     string   `json:"path"`
	Line     int      `json:"line"`
	Column   int      `json:"column"`
	Text     string   `json:"text"`
	Before   []string `json:"before,omitempty"`
	After    []string `json:"after,omitempty"`
	Encoding string   `json:"encoding,omitempty"`
}

type GrepFileCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

type PartialFileError struct {
	Path              string `json:"path"`
	Error             string `json:"error"`
	ErrorCode         string `json:"errorCode"`
	EncodingErrorCode string `json:"encodingErrorCode,omitempty"`
}

type GrepOutput struct {
	Matches               []GrepMatch        `json:"matches"`
	Files                 []string           `json:"files,omitempty"`
	Counts                []GrepFileCount    `json:"counts,omitempty"`
	TotalMatches          int                `json:"totalMatches"`
	FilesSearched         int                `json:"filesSearched"`
	FilesScanned          int                `json:"filesScanned"`
	FilesMatched          int                `json:"filesMatched"`
	FilesSkipped          int                `json:"filesSkipped"`
	SkippedFiles          []PartialFileError `json:"skippedFiles,omitempty"`
	SkippedFilesTruncated bool               `json:"skippedFilesTruncated,omitempty"`
	SkippedFilesOmitted   int                `json:"skippedFilesOmitted,omitempty"`
	CoverageComplete      bool               `json:"coverageComplete"`
	Truncated             bool               `json:"truncated,omitempty"`
	NextOffset            int                `json:"nextOffset,omitempty"`
}

type DetectLineEndingsInput struct {
	Path     string `json:"path"`
	Encoding string `json:"encoding,omitempty"`
}

// ChangeLineEndingsInput converts line endings while preserving the file encoding.
// Style must be "lf" or "crlf". Encoding is auto-detected when omitted.
type ChangeLineEndingsInput struct {
	Path     string `json:"path"`
	Style    string `json:"style"`
	Encoding string `json:"encoding,omitempty"`
}

type ChangeLineEndingsOutput struct {
	Message       string `json:"message"`
	OriginalStyle string `json:"originalStyle"`
	NewStyle      string `json:"newStyle"`
	LinesChanged  int    `json:"linesChanged"`
}

// ManageBomInput manages Unicode BOM (Byte Order Mark) in files.
// Action: "detect" (check for BOM), "strip" (remove BOM), "add" (prepend BOM).
// Encoding is required for "add" action: utf-8, utf-16-le, utf-16-be, utf-32-le, utf-32-be.
type ManageBomInput struct {
	Path     string `json:"path"`
	Action   string `json:"action"`
	Encoding string `json:"encoding,omitempty"`
}

type ManageBomOutput struct {
	Message           string `json:"message"`
	Action            string `json:"action,omitempty"`
	PreviewID         string `json:"previewId,omitempty"`
	CreatedAt         string `json:"createdAt,omitempty"`
	ExpiresAt         string `json:"expiresAt,omitempty"`
	TargetPath        string `json:"targetPath,omitempty"`
	TargetFingerprint string `json:"targetFingerprint,omitempty"`
	ResultFingerprint string `json:"resultFingerprint,omitempty"`
	BackupPolicy      string `json:"backupPolicy,omitempty"`
	BackupID          string `json:"backupId,omitempty"`
	Applied           bool   `json:"applied,omitempty"`
	HasBOM            bool   `json:"hasBOM"`
	BOMType           string `json:"bomType,omitempty"`  // e.g. "utf-8", "utf-16-le"
	BOMBytes          int    `json:"bomBytes,omitempty"` // size of BOM in bytes (2, 3, or 4)
	Changed           bool   `json:"changed"`
}

// DetectLineEndingsOutput - Style is "crlf", "lf", "mixed", or "none"
type DetectLineEndingsOutput struct {
	Style             string `json:"style"`
	TotalLines        int    `json:"totalLines"`
	InconsistentLines []int  `json:"inconsistentLines"`
}
