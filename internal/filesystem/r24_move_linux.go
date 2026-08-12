//go:build linux

package filesystem

import (
	"errors"

	"github.com/zoster81/scripthold/internal/operation"
	"golang.org/x/sys/unix"
)

func nativeMovePathNoReplace(source, destination string) error {
	err := unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		return operation.New(operation.KindUnsupported, "native renameat2(RENAME_NOREPLACE) is unavailable")
	}
	return err
}
