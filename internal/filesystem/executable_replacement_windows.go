//go:build windows

package filesystem

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32ReplacementDLL = windows.NewLazySystemDLL("kernel32.dll")
	procReplaceFileW       = kernel32ReplacementDLL.NewProc("ReplaceFileW")
)

func prepareExecutableReplacementCandidate(
	targetPath string,
	targetIdentity ObjectIdentity,
	candidatePath string,
	candidateIdentity ObjectIdentity,
) error {
	target, err := OpenVerifiedSingleLinkFile(targetPath, targetIdentity)
	if err != nil {
		return err
	}
	if err := target.Close(); err != nil {
		return err
	}
	candidate, err := OpenVerifiedSingleLinkFile(candidatePath, candidateIdentity)
	if err != nil {
		return err
	}
	if err := candidate.Close(); err != nil {
		return err
	}
	// ReplaceFileW preserves target security metadata during the later commit.
	// The commit primitive will reopen the candidate with write authority and
	// FlushFileBuffers immediately before replacement.
	return ValidateOwnerOnlyExecutable(candidatePath)
}

func commitExecutableReplacementCandidate(
	targetPath string,
	targetIdentity ObjectIdentity,
	candidatePath string,
	candidateIdentity ObjectIdentity,
) error {
	if err := flushVerifiedWindowsFile(candidatePath, candidateIdentity); err != nil {
		return fmt.Errorf("flush executable candidate before replacement: %w", err)
	}
	targetPtr, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return err
	}
	candidatePtr, err := windows.UTF16PtrFromString(candidatePath)
	if err != nil {
		return err
	}
	result, _, callErr := procReplaceFileW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		uintptr(unsafe.Pointer(candidatePtr)),
		0,
		0,
		0,
		0,
	)
	if result == 0 {
		if callErr == nil || errors.Is(callErr, syscall.Errno(0)) {
			callErr = windows.ERROR_GEN_FAILURE
		}
		return fmt.Errorf("ReplaceFileW failed: %w", callErr)
	}
	if err := flushVerifiedWindowsFile(targetPath, candidateIdentity); err != nil {
		return fmt.Errorf("flush installed executable after replacement: %w", err)
	}
	return nil
}

func flushVerifiedWindowsFile(path string, expected ObjectIdentity) error {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_WRITE|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)

	var details windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &details); err != nil {
		return err
	}
	if details.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return errors.New("replacement flush target is linked or not a regular file")
	}
	if details.NumberOfLinks != 1 {
		return errors.New("replacement flush target has multiple hard links")
	}
	volumeKey := fmt.Sprintf("win:%08x", details.VolumeSerialNumber)
	key := fmt.Sprintf("%s:%08x%08x", volumeKey, details.FileIndexHigh, details.FileIndexLow)
	if key != expected.StableKey() || volumeKey != expected.VolumeKey() {
		return errors.New("replacement flush target identity changed")
	}
	return windows.FlushFileBuffers(handle)
}
