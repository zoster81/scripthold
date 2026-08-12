//go:build !windows && !linux && !darwin

package filesystem

import "github.com/zoster81/scripthold/internal/operation"

func nativeMovePathNoReplace(source, destination string) error {
	return operation.New(operation.KindUnsupported, "native same-volume no-replace move is unavailable on this platform")
}
