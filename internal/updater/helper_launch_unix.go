//go:build linux || darwin

package updater

import (
	"os/exec"
	"syscall"
)

func configureDetachedSelfUpdateHelper(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
