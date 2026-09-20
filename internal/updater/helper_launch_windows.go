//go:build windows

package updater

import (
	"os/exec"
	"syscall"
)

func configureDetachedSelfUpdateHelper(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008,
	}
}
