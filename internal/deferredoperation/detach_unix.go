//go:build linux || darwin

package deferredoperation

import (
	"os/exec"
	"syscall"
)

func configureDetachedHelper(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
