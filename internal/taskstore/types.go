// Package taskstore implements durable asynchronous command execution whose
// lifecycle is independent from any MCP transport connection.
package taskstore

import "time"

const (
	FormatVersion          = "scripthold-task-store-v1"
	maxNameBytes           = 120
	maxDescriptionBytes    = 2000
	maxIdempotencyKeyBytes = 256
	maxTags                = 16
	maxTagBytes            = 64
	maxLockKeys            = 16
	maxLockKeyBytes        = 128
	maxArgs                = 256
	maxArgumentBytes       = 4 * 1024
	maxCommandBytes        = 256 * 1024
	maxPathBytes           = 32 * 1024
	defaultPageSize        = 50
	maximumPageSize        = 200
	defaultLogReadBytes    = 64 * 1024
	maximumLogReadBytes    = 1024 * 1024
	maxStateRecords        = 32
	maxDispatchAttempts    = 8
)

type Status string

const (
	StatusQueued      Status = "queued"
	StatusStarting    Status = "starting"
	StatusRunning     Status = "running"
	StatusSucceeded   Status = "succeeded"
	StatusFailed      Status = "failed"
	StatusTimedOut    Status = "timed_out"
	StatusCancelled   Status = "cancelled"
	StatusInterrupted Status = "interrupted"
)

func (status Status) Terminal() bool {
	switch status {
	case StatusSucceeded, StatusFailed, StatusTimedOut, StatusCancelled, StatusInterrupted:
		return true
	default:
		return false
	}
}

type Kind string

const (
	KindShell  Kind = "shell"
	KindScript Kind = "script"
)

// Request is the durable, user-visible execution intent. Command text and
// arguments are stored only inside the owner-only task store and are never
// emitted by lifecycle logging.
type Request struct {
	Kind              Kind     `json:"kind"`
	Name              string   `json:"name,omitempty"`
	Description       string   `json:"description,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	IdempotencyKey    string   `json:"idempotencyKey"`
	LockKeys          []string `json:"lockKeys,omitempty"`
	WorkingDirectory  string   `json:"workingDirectory"`
	Command           string   `json:"command,omitempty"`
	Shell             string   `json:"shell,omitempty"`
	ScriptPath        string   `json:"scriptPath,omitempty"`
	ScriptDigest      string   `json:"scriptDigest,omitempty"`
	ScriptSize        int64    `json:"scriptSize,omitempty"`
	Args              []string `json:"args,omitempty"`
	MaxRuntimeSeconds int      `json:"maxRuntimeSeconds,omitempty"`
}

type Result struct {
	ExitCode       int    `json:"exitCode"`
	DurationMillis int64  `json:"durationMillis"`
	ErrorCode      string `json:"errorCode,omitempty"`
	Message        string `json:"message,omitempty"`
}

type Task struct {
	ID                string      `json:"taskId"`
	Kind              Kind        `json:"kind"`
	Name              string      `json:"name,omitempty"`
	Description       string      `json:"description,omitempty"`
	Tags              []string    `json:"tags,omitempty"`
	LockKeys          []string    `json:"lockKeys,omitempty"`
	Status            Status      `json:"status"`
	CreatedAt         time.Time   `json:"createdAt"`
	UpdatedAt         time.Time   `json:"updatedAt"`
	StartedAt         *time.Time  `json:"startedAt,omitempty"`
	FinishedAt        *time.Time  `json:"finishedAt,omitempty"`
	MaxRuntimeSeconds int         `json:"maxRuntimeSeconds"`
	CancelRequested   bool        `json:"cancelRequested,omitempty"`
	WorkerOnline      bool        `json:"workerOnline"`
	Result            *Result     `json:"result,omitempty"`
	Revision          uint64      `json:"revision"`
	History           []TaskEvent `json:"history,omitempty"`
}

// TaskEvent is the bounded immutable lifecycle audit retained for a task.
// It deliberately excludes commands, arguments, paths, environment, and logs.
type TaskEvent struct {
	Status    Status    `json:"status"`
	Revision  uint64    `json:"revision"`
	Timestamp time.Time `json:"timestamp"`
	ErrorCode string    `json:"errorCode,omitempty"`
}

type ListOptions struct {
	Statuses []Status
	Kinds    []Kind
	Tags     []string
	Cursor   string
	Limit    int
}

type ListResult struct {
	Tasks      []Task `json:"tasks"`
	NextCursor string `json:"nextCursor,omitempty"`
	Truncated  bool   `json:"truncated"`
}

type LogOptions struct {
	StdoutCursor int64
	StderrCursor int64
	LimitBytes   int
}

type LogChunk struct {
	Data         string `json:"data,omitempty"`
	Cursor       int64  `json:"cursor"`
	NextCursor   int64  `json:"nextCursor"`
	AvailableEnd int64  `json:"availableEnd"`
	DroppedBytes int64  `json:"droppedBytes"`
	Truncated    bool   `json:"truncated"`
}

type LogsResult struct {
	TaskID string   `json:"taskId"`
	Stdout LogChunk `json:"stdout"`
	Stderr LogChunk `json:"stderr"`
}

type SubmitResult struct {
	Task       Task `json:"task"`
	Duplicated bool `json:"duplicated"`
}

type Limits struct {
	MaxConcurrency       int   `json:"maxConcurrency"`
	MaxQueued            int   `json:"maxQueued"`
	MaxLogBytesPerStream int64 `json:"maxLogBytesPerStream"`
	MaxRuntimeSeconds    int   `json:"maxRuntimeSeconds"`
	RetentionDays        int   `json:"retentionDays"`
	MaxTerminal          int   `json:"maxTerminal"`
	MaxTotalBytes        int64 `json:"maxTotalBytes"`
}

type WorkerPolicy struct {
	AllowShell     bool
	AllowRunScript bool
}

type descriptor struct {
	Format    string    `json:"format"`
	Salt      string    `json:"salt"`
	CreatedAt time.Time `json:"createdAt"`
	Limits    Limits    `json:"limits"`
}

type stateRecord struct {
	Status        Status     `json:"status"`
	Revision      uint64     `json:"revision"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
	ExecutorPID   int        `json:"executorPid,omitempty"`
	ExecutorToken string     `json:"executorToken,omitempty"`
	Result        *Result    `json:"result,omitempty"`
}

type launchRecord struct {
	Program           string   `json:"program"`
	Args              []string `json:"args,omitempty"`
	WorkingDirectory  string   `json:"workingDirectory"`
	MaxRuntimeSeconds int      `json:"maxRuntimeSeconds"`
	ExecutorToken     string   `json:"executorToken"`
}
