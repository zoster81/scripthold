//go:build windows

package deferredoperation

import (
	"os/exec"
	"syscall"
)

func configureDetachedHelper(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008}
}
