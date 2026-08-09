//go:build windows

package taskstore

import (
	"os/exec"
	"syscall"
)

func configureDetachedHelper(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200}
}
