package filesystem

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"

	"github.com/zoster81/scripthold/internal/operation"
)

var ErrIncompleteRead = errors.New("read session did not consume the complete file")

// ReadSession keeps random-access sampling separate from one sequential,
// digesting pass. Finish returns a mutation-compatible snapshot only after the
// complete file, including any skipped prefix, has been consumed.
type ReadSession struct {
	path    string
	file    *os.File
	initial FileSnapshot
	hasher  hash.Hash

	started     bool
	finished    bool
	closed      bool
	prefixBytes int64
	readBytes   int64
}

func OpenReadSession(path string) (session *ReadSession, err error) {
	defer func() {
		err = operation.WrapFilesystem("open_read_session", path, err)
	}()

	// Use the same platform-aware read sharing as retained identities. On
	// Windows this includes FILE_SHARE_DELETE so a concurrent reader does not
	// become an implicit rename/delete lock for an otherwise valid mutation.
	file, err := openIdentityFile(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("path is not a regular file: %s", path)
	}

	return &ReadSession{
		path:    path,
		file:    file,
		initial: snapshotFromInfo(info),
		hasher:  sha256.New(),
	}, nil
}

func (session *ReadSession) Size() int64 {
	return session.initial.Size
}

func (session *ReadSession) Mode() fs.FileMode {
	return session.initial.Mode
}

func (session *ReadSession) ReadAt(buffer []byte, offset int64) (int, error) {
	if session.closed {
		return 0, os.ErrClosed
	}
	return session.file.ReadAt(buffer, offset)
}

// Start positions the sequential pass after prefixBytes. The skipped prefix is
// hashed through ReaderAt so the final snapshot still represents the full file.
func (session *ReadSession) Start(prefixBytes int64) error {
	if session.closed {
		return os.ErrClosed
	}
	if session.started {
		return errors.New("read session already started")
	}
	if prefixBytes < 0 || prefixBytes > session.initial.Size {
		return fmt.Errorf("invalid read prefix %d for file size %d", prefixBytes, session.initial.Size)
	}

	if prefixBytes > 0 {
		section := io.NewSectionReader(session.file, 0, prefixBytes)
		written, err := io.Copy(session.hasher, section)
		if err != nil {
			return fmt.Errorf("failed to hash skipped prefix: %w", err)
		}
		if written != prefixBytes {
			return fmt.Errorf("%w: hashed prefix %d of %d bytes", ErrIncompleteRead, written, prefixBytes)
		}
	}
	if _, err := session.file.Seek(prefixBytes, io.SeekStart); err != nil {
		return err
	}
	session.started = true
	session.prefixBytes = prefixBytes
	return nil
}

func (session *ReadSession) Read(buffer []byte) (int, error) {
	if session.closed {
		return 0, os.ErrClosed
	}
	if !session.started {
		return 0, errors.New("read session has not been started")
	}
	if session.finished {
		return 0, io.EOF
	}

	read, err := session.file.Read(buffer)
	if read > 0 {
		if _, hashErr := session.hasher.Write(buffer[:read]); hashErr != nil {
			return read, hashErr
		}
		session.readBytes += int64(read)
	}
	return read, err
}

func (session *ReadSession) Finish() (snapshot FileSnapshot, err error) {
	defer func() {
		err = operation.WrapFilesystem("finish_read_session", session.path, err)
	}()

	if session.closed {
		return FileSnapshot{}, os.ErrClosed
	}
	if !session.started {
		return FileSnapshot{}, errors.New("read session has not been started")
	}
	if session.finished {
		return FileSnapshot{}, errors.New("read session already finished")
	}
	if consumed := session.prefixBytes + session.readBytes; consumed != session.initial.Size {
		return FileSnapshot{}, fmt.Errorf("%w: consumed %d of %d bytes", ErrIncompleteRead, consumed, session.initial.Size)
	}

	info, err := session.file.Stat()
	if err != nil {
		return FileSnapshot{}, err
	}
	if err := session.initial.verifyInfo(session.path, info); err != nil {
		return FileSnapshot{}, err
	}
	currentInfo, err := os.Stat(session.path)
	if err != nil {
		if os.IsNotExist(err) {
			return FileSnapshot{}, fmt.Errorf("%w: path disappeared: %s", ErrConcurrentModification, session.path)
		}
		return FileSnapshot{}, err
	}
	if err := session.initial.verifyInfo(session.path, currentInfo); err != nil {
		return FileSnapshot{}, err
	}
	if !os.SameFile(info, currentInfo) {
		return FileSnapshot{}, fmt.Errorf("%w: filesystem object changed for %s", ErrConcurrentModification, session.path)
	}

	snapshot = session.initial
	copy(snapshot.digest[:], session.hasher.Sum(nil))
	snapshot.hasDigest = true
	session.finished = true
	return snapshot, nil
}

func (session *ReadSession) Close() error {
	if session == nil || session.closed {
		return nil
	}
	session.closed = true
	return session.file.Close()
}
