package taskstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

type boundedLogWriter struct {
	mu         sync.Mutex
	taskDir    string
	stream     string
	head       *os.File
	headLimit  int64
	tail       []byte
	tailLimit  int
	tailStart  int
	tailLength int
	total      int64
}

func newBoundedLogWriter(taskDir, stream string, limit int64) (*boundedLogWriter, error) {
	if stream != "stdout" && stream != "stderr" {
		return nil, errors.New("invalid log stream")
	}
	headLimit := limit / 4
	if headLimit < 1024 {
		headLimit = 1024
	}
	if headLimit >= limit {
		headLimit = limit / 2
	}
	tailLimit := int(limit - headLimit)
	headPath := filepath.Join(taskDir, stream+".head")
	head, err := os.OpenFile(headPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if err := securePath(headPath, false); err != nil {
		_ = head.Close()
		return nil, err
	}
	return &boundedLogWriter{taskDir: taskDir, stream: stream, head: head, headLimit: headLimit, tail: make([]byte, tailLimit), tailLimit: tailLimit}, nil
}

func (writer *boundedLogWriter) Write(payload []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	original := len(payload)
	if writer.total < writer.headLimit {
		remaining := writer.headLimit - writer.total
		count := len(payload)
		if int64(count) > remaining {
			count = int(remaining)
		}
		if count > 0 {
			if _, err := writer.head.Write(payload[:count]); err != nil {
				return 0, err
			}
			writer.total += int64(count)
			payload = payload[count:]
		}
	}
	for _, value := range payload {
		if writer.tailLimit == 0 {
			writer.total++
			continue
		}
		if writer.tailLength < writer.tailLimit {
			index := (writer.tailStart + writer.tailLength) % writer.tailLimit
			writer.tail[index] = value
			writer.tailLength++
		} else {
			writer.tail[writer.tailStart] = value
			writer.tailStart = (writer.tailStart + 1) % writer.tailLimit
		}
		writer.total++
	}
	return original, nil
}

func (writer *boundedLogWriter) Snapshot() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.head != nil {
		_ = writer.head.Sync()
	}
	if writer.tailLength == 0 {
		return nil
	}
	payload := make([]byte, writer.tailLength)
	first := writer.tailLength
	if first > writer.tailLimit-writer.tailStart {
		first = writer.tailLimit - writer.tailStart
	}
	copy(payload, writer.tail[writer.tailStart:writer.tailStart+first])
	copy(payload[first:], writer.tail[:writer.tailLength-first])
	path := filepath.Join(writer.taskDir, fmt.Sprintf("%s.tail-%020d.bin", writer.stream, writer.total))
	if err := writeBytesExclusive(path, payload); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return pruneTailSnapshots(writer.taskDir, writer.stream, 2)
}

func (writer *boundedLogWriter) Close() error {
	snapshotErr := writer.Snapshot()
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.head == nil {
		return snapshotErr
	}
	err := errors.Join(snapshotErr, writer.head.Sync(), writer.head.Close())
	writer.head = nil
	return err
}

func writeBytesExclusive(path string, payload []byte) error {
	suffix, err := randomHex(8)
	if err != nil {
		return err
	}
	temporary := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"."+suffix+".tmp")
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporary)
		}
	}()
	if err := securePath(temporary, false); err != nil {
		_ = file.Close()
		return err
	}
	_, writeErr := file.Write(payload)
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return err
	}
	if err := filesystem.MoveNoReplace(temporary, path); err != nil {
		if errors.Is(err, filesystem.ErrDestinationExists) {
			return os.ErrExist
		}
		if secureFileMatches(path, payload) {
			committed = true
			return nil
		}
		return err
	}
	committed = true
	return nil
}

func pruneTailSnapshots(taskDir, stream string, keep int) error {
	entries, err := readDirectoryBounded(taskDir, maxStateRecords+128)
	if err != nil {
		return err
	}
	prefix := stream + ".tail-"
	names := make([]string, 0, 4)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ".bin") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		if err := os.Remove(filepath.Join(taskDir, names[0])); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		names = names[1:]
	}
	return nil
}

func (store *Store) Logs(ctx context.Context, taskID string, options LogOptions) (LogsResult, error) {
	if store == nil {
		return LogsResult{}, ErrDisabled
	}
	if err := ctx.Err(); err != nil {
		return LogsResult{}, err
	}
	if !validTaskID(taskID) {
		return LogsResult{}, ErrNotFound
	}
	if _, err := store.readPersistedRequest(taskID); err != nil {
		return LogsResult{}, ErrNotFound
	}
	limit := options.LimitBytes
	if limit == 0 {
		limit = defaultLogReadBytes
	}
	if limit < 1 || limit > maximumLogReadBytes {
		return LogsResult{}, operation.Wrap(operation.KindInvalidInput, "read task logs", "", fmt.Errorf("limitBytes must be between 1 and %d", maximumLogReadBytes))
	}
	if options.StdoutCursor < 0 || options.StderrCursor < 0 {
		return LogsResult{}, operation.Wrap(operation.KindInvalidInput, "read task logs", "", errors.New("log cursors must not be negative"))
	}
	stdout, err := readLogChunk(store.taskDir(taskID), "stdout", options.StdoutCursor, limit, store.limits.MaxLogBytesPerStream)
	if err != nil {
		return LogsResult{}, err
	}
	stderr, err := readLogChunk(store.taskDir(taskID), "stderr", options.StderrCursor, limit, store.limits.MaxLogBytesPerStream)
	if err != nil {
		return LogsResult{}, err
	}
	return LogsResult{TaskID: taskID, Stdout: stdout, Stderr: stderr}, nil
}

func readLogChunk(taskDir, stream string, requested int64, limit int, retainedLimit int64) (LogChunk, error) {
	if requested < 0 {
		return LogChunk{}, errors.New("log cursor must not be negative")
	}
	headPath := filepath.Join(taskDir, stream+".head")
	if info, statErr := os.Lstat(headPath); statErr == nil {
		if err := validateSecurePath(headPath, false); err != nil {
			return LogChunk{}, err
		}
		if info.Size() < 0 || info.Size() > retainedLimit {
			return LogChunk{}, errors.New("log head exceeds its configured bound")
		}
	}
	head, headErr := os.ReadFile(headPath)
	if headErr != nil && !errors.Is(headErr, os.ErrNotExist) {
		return LogChunk{}, headErr
	}
	tail, total, err := latestTailSnapshot(taskDir, stream, retainedLimit)
	if err != nil {
		return LogChunk{}, err
	}
	if total < int64(len(head)) {
		total = int64(len(head))
	}
	tailStart := total - int64(len(tail))
	cursor := requested
	truncated := false
	if cursor > total {
		return LogChunk{}, errors.New("log cursor is beyond the available stream")
	}
	data := make([]byte, 0, limit)
	position := cursor
	if position < int64(len(head)) {
		end := position + int64(limit)
		if end > int64(len(head)) {
			end = int64(len(head))
		}
		data = append(data, head[position:end]...)
		position = end
	}
	if len(data) < limit && position < total {
		if position < tailStart {
			position = tailStart
			truncated = true
		}
		if position >= tailStart {
			offset := position - tailStart
			if offset < int64(len(tail)) {
				count := limit - len(data)
				if count > len(tail)-int(offset) {
					count = len(tail) - int(offset)
				}
				data = append(data, tail[int(offset):int(offset)+count]...)
				position += int64(count)
			}
		}
	}
	dropped := tailStart - int64(len(head))
	if dropped < 0 {
		dropped = 0
	}
	return LogChunk{Data: string(data), Cursor: requested, NextCursor: position, AvailableEnd: total, DroppedBytes: dropped, Truncated: truncated}, nil
}

func latestTailSnapshot(taskDir, stream string, retainedLimit int64) ([]byte, int64, error) {
	entries, err := readDirectoryBounded(taskDir, maxStateRecords+128)
	if err != nil {
		return nil, 0, err
	}
	prefix := stream + ".tail-"
	var selected string
	var total int64
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".bin") {
			continue
		}
		value := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".bin")
		parsed, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr == nil && parsed >= total {
			total, selected = parsed, name
		}
	}
	if selected == "" {
		return nil, 0, nil
	}
	path := filepath.Join(taskDir, selected)
	if err := validateSecurePath(path, false); err != nil {
		return nil, 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, retainedLimit+1))
	if err != nil {
		return nil, 0, err
	}
	if int64(len(payload)) > retainedLimit {
		return nil, 0, errors.New("log tail exceeds its configured bound")
	}
	return payload, total, nil
}
