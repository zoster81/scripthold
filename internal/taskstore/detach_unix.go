//go:build linux || darwin

package taskstore

import (
	"os/exec"
	"syscall"
)

func configureDetachedHelper(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
