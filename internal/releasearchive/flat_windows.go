//go:build windows

package releasearchive

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// Windows support here provides real local tests of anchored path and hard-link
// behavior. The publication job uses Linux, where the owner-only root is enforced.
func verifyPrivateStagingRoot(_ os.FileInfo) error {
	return nil
}

func verifySingleLinkStagingFile(file *os.File) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 ||
		info.NumberOfLinks != 1 {
		return errors.New("staging file must be an ordinary single-link file")
	}
	return nil
}
