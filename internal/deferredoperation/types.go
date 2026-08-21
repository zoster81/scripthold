// Package deferredoperation owns durable read-only MCP operations independently
// from any frontend request or transport connection.
package deferredoperation

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/zoster81/scripthold/internal/config"
)

const (
	FormatVersion        = "scripthold-deferred-operation-store-v1"
	maxStateRecords      = 32
	executorStaleAfter   = 15 * time.Second
	storeLockWaitMaximum = 30 * time.Second
)

var (
	ErrDisabled       = errors.New("deferred operation store is disabled")
	ErrInvalidInput   = errors.New("deferred operation request is invalid")
	ErrNotFound       = errors.New("deferred operation not found")
	ErrAccessDenied   = errors.New("deferred operation is no longer authorized")
	ErrCapacity       = errors.New("deferred operation capacity exceeded")
	ErrAlreadyStarted = errors.New("deferred operation has already started")
	ErrTerminal       = errors.New("deferred operation is already terminal")
	ErrResultMissing  = errors.New("deferred operation result is unavailable")
)

type Status string

const (
	StatusQueued      Status = "queued"
	StatusStarting    Status = "starting"
	StatusRunning     Status = "running"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusTimedOut    Status = "timed_out"
	StatusCancelled   Status = "cancelled"
	StatusInterrupted Status = "interrupted"
)

func (status Status) Terminal() bool {
	switch status {
	case StatusCompleted, StatusFailed, StatusTimedOut, StatusCancelled, StatusInterrupted:
		return true
	default:
		return false
	}
}

type Limits struct {
	MaxConcurrency    int   `json:"maxConcurrency"`
	MaxQueued         int   `json:"maxQueued"`
	MaxRuntimeSeconds int   `json:"maxRuntimeSeconds"`
	RetentionSeconds  int   `json:"retentionSeconds"`
	MaxTerminal       int   `json:"maxTerminal"`
	MaxTotalBytes     int64 `json:"maxTotalBytes"`
	MaxResultBytes    int64 `json:"maxResultBytes"`
	MaxChunkBytes     int   `json:"maxChunkBytes"`
}

type ExecutionConfig struct {
	DefaultEncoding string              `json:"defaultEncoding"`
	Limits          config.Limits       `json:"limits"`
	Source          config.SourceConfig `json:"source"`
}

type Request struct {
	Tool               string          `json:"tool"`
	Arguments          json.RawMessage `json:"arguments"`
	AllowedDirectories []string        `json:"allowedDirectories"`
	OriginPaths        []string        `json:"originPaths,omitempty"`
	Config             ExecutionConfig `json:"config"`
	MaxRuntimeSeconds  int             `json:"maxRuntimeSeconds,omitempty"`
}

type ResultMetadata struct {
	OriginalIsError bool   `json:"originalIsError,omitempty"`
	ErrorCode       string `json:"errorCode,omitempty"`
}

type Operation struct {
	OperationID     string     `json:"operationId"`
	Tool            string     `json:"tool"`
	Status          Status     `json:"status"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
	Revision        uint64     `json:"revision"`
	Started         bool       `json:"started"`
	Exposed         bool       `json:"exposed"`
	CancelRequested bool       `json:"cancelRequested,omitempty"`
	ResultAvailable bool       `json:"resultAvailable"`
	TotalBytes      int64      `json:"totalBytes,omitempty"`
	OriginalIsError bool       `json:"originalIsError,omitempty"`
	ErrorCode       string     `json:"errorCode,omitempty"`
	Message         string     `json:"message,omitempty"`
}

type Claim struct {
	Request   Request
	Operation Operation
}

type Chunk struct {
	OperationID     string `json:"operationId"`
	Status          Status `json:"status"`
	OriginalIsError bool   `json:"originalIsError,omitempty"`
	ErrorCode       string `json:"errorCode,omitempty"`
	TotalBytes      int64  `json:"totalBytes"`
	Offset          int64  `json:"offset"`
	NextOffset      int64  `json:"nextOffset"`
	Data            string `json:"data"`
	Complete        bool   `json:"complete"`
}

type descriptor struct {
	Format    string    `json:"format"`
	CreatedAt time.Time `json:"createdAt"`
	Limits    Limits    `json:"limits"`
}

type persistedRequest struct {
	OperationID string    `json:"operationId"`
	CreatedAt   time.Time `json:"createdAt"`
	Request     Request   `json:"request"`
}

type stateRecord struct {
	Status     Status          `json:"status"`
	Revision   uint64          `json:"revision"`
	UpdatedAt  time.Time       `json:"updatedAt"`
	StartedAt  *time.Time      `json:"startedAt,omitempty"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
	Result     *ResultMetadata `json:"result,omitempty"`
	TotalBytes int64           `json:"totalBytes,omitempty"`
	Message    string          `json:"message,omitempty"`
}

func ValidOperationID(value string) bool {
	if len(value) != 67 || value[:3] != "op_" {
		return false
	}
	decoded, err := hex.DecodeString(value[3:])
	return err == nil && len(decoded) == 32
}
