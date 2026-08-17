//go:build windows

package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	atomicReplaceDiagnosticFormat = "windows-atomic-replace-v1"
	windowsFileAddFileAccess      = 0x00000002
	windowsFileDeleteChildAccess  = 0x00000040
	windowsDeleteAccess           = 0x00010000
	maxRestartManagerProcesses    = 32
)

type windowsAtomicReplaceAttemptDiagnostic struct {
	Phase     string `json:"phase"`
	Win32Code uint32 `json:"win32Code,omitempty"`
}

type windowsAtomicReplacePathProbe struct {
	Exists                  bool   `json:"exists"`
	Attributes              uint32 `json:"attributes,omitempty"`
	ReadOnly                bool   `json:"readOnly,omitempty"`
	ReparsePoint            bool   `json:"reparsePoint,omitempty"`
	Size                    int64  `json:"size,omitempty"`
	Links                   uint32 `json:"links,omitempty"`
	DeletePending           bool   `json:"deletePending,omitempty"`
	ReadAttributesErrorCode uint32 `json:"readAttributesErrorCode,omitempty"`
	DeleteAccessGranted     bool   `json:"deleteAccessGranted"`
	DeleteAccessErrorCode   uint32 `json:"deleteAccessErrorCode,omitempty"`
}

type windowsAtomicReplaceDirectoryProbe struct {
	AddFileGranted       bool   `json:"addFileGranted"`
	AddFileErrorCode     uint32 `json:"addFileErrorCode,omitempty"`
	DeleteChildGranted   bool   `json:"deleteChildGranted"`
	DeleteChildErrorCode uint32 `json:"deleteChildErrorCode,omitempty"`
}

type windowsRestartManagerProcess struct {
	PID             uint32 `json:"pid"`
	ApplicationType uint32 `json:"applicationType"`
	ApplicationName string `json:"applicationName,omitempty"`
	ServiceName     string `json:"serviceName,omitempty"`
	Restartable     bool   `json:"restartable"`
	CurrentProcess  bool   `json:"currentProcess,omitempty"`
}

type windowsRestartManagerDiagnostic struct {
	StartCode       uint32                         `json:"startCode,omitempty"`
	RegisterCode    uint32                         `json:"registerCode,omitempty"`
	GetListCode     uint32                         `json:"getListCode,omitempty"`
	ProcessesNeeded uint32                         `json:"processesNeeded,omitempty"`
	Truncated       bool                           `json:"truncated,omitempty"`
	Processes       []windowsRestartManagerProcess `json:"processes,omitempty"`
}

type windowsAtomicReplaceDiagnostic struct {
	FormatVersion   string                                  `json:"formatVersion"`
	Outcome         atomicReplaceRetryOutcome               `json:"outcome"`
	CommitAttempts  int                                     `json:"commitAttempts"`
	ElapsedMillis   int64                                   `json:"elapsedMillis"`
	TargetPathHash  string                                  `json:"targetPathHash"`
	StagedPathHash  string                                  `json:"stagedPathHash"`
	TargetExtension string                                  `json:"targetExtension,omitempty"`
	Attempts        []windowsAtomicReplaceAttemptDiagnostic `json:"attempts"`
	TargetProbe     *windowsAtomicReplacePathProbe          `json:"targetProbe,omitempty"`
	StagedProbe     *windowsAtomicReplacePathProbe          `json:"stagedProbe,omitempty"`
	ParentProbe     *windowsAtomicReplaceDirectoryProbe     `json:"parentProbe,omitempty"`
	RestartManager  *windowsRestartManagerDiagnostic        `json:"restartManager,omitempty"`
}

type windowsFileStandardInfo struct {
	AllocationSize int64
	EndOfFile      int64
	NumberOfLinks  uint32
	DeletePending  byte
	Directory      byte
}

var (
	restartManagerDLL       = windows.NewLazySystemDLL("rstrtmgr.dll")
	procRmStartSession      = restartManagerDLL.NewProc("RmStartSession")
	procRmRegisterResources = restartManagerDLL.NewProc("RmRegisterResources")
	procRmGetList           = restartManagerDLL.NewProc("RmGetList")
	procRmEndSession        = restartManagerDLL.NewProc("RmEndSession")
)

func reportAtomicReplaceRetry(targetPath, stagedPath string, report atomicReplaceRetryReport) {
	logger := slog.Default()
	debug := logger.Enabled(context.Background(), slog.LevelDebug)
	if report.Outcome != atomicReplaceRetryExhausted && !debug {
		return
	}

	// Diagnostics are best-effort evidence only. They must never change mutation
	// success/failure if logging or an OS diagnostic provider misbehaves.
	defer func() {
		_ = recover()
	}()

	diagnostic := buildWindowsAtomicReplaceDiagnostic(targetPath, stagedPath, report, report.Outcome == atomicReplaceRetryExhausted)
	payload, marshalErr := json.Marshal(diagnostic)
	if marshalErr != nil {
		payload = []byte(`{"formatVersion":"windows-atomic-replace-v1","encodingError":true}`)
	}
	if report.Outcome == atomicReplaceRetryExhausted {
		logger.Warn("windows atomic replace retry exhausted", "atomicReplace", string(payload))
		return
	}
	logger.Debug("windows atomic replace retry episode", "atomicReplace", string(payload))
}

func buildWindowsAtomicReplaceDiagnostic(targetPath, stagedPath string, report atomicReplaceRetryReport, deep bool) windowsAtomicReplaceDiagnostic {
	diagnostic := windowsAtomicReplaceDiagnostic{
		FormatVersion:   atomicReplaceDiagnosticFormat,
		Outcome:         report.Outcome,
		CommitAttempts:  report.CommitAttempts,
		ElapsedMillis:   report.Elapsed.Milliseconds(),
		TargetPathHash:  hashAtomicReplacePath(targetPath),
		StagedPathHash:  hashAtomicReplacePath(stagedPath),
		TargetExtension: strings.ToLower(filepath.Ext(targetPath)),
		Attempts:        make([]windowsAtomicReplaceAttemptDiagnostic, 0, len(report.Attempts)),
	}
	for _, attempt := range report.Attempts {
		diagnostic.Attempts = append(diagnostic.Attempts, windowsAtomicReplaceAttemptDiagnostic{
			Phase: attempt.Phase, Win32Code: atomicReplaceWin32Code(attempt.Err),
		})
	}
	if !deep {
		return diagnostic
	}
	targetProbe := probeWindowsAtomicReplacePath(targetPath, true)
	stagedProbe := probeWindowsAtomicReplacePath(stagedPath, true)
	parentProbe := probeWindowsAtomicReplaceDirectory(filepath.Dir(targetPath))
	diagnostic.TargetProbe = &targetProbe
	diagnostic.StagedProbe = &stagedProbe
	diagnostic.ParentProbe = &parentProbe
	restartManager := queryWindowsRestartManager(targetPath, stagedPath)
	diagnostic.RestartManager = &restartManager
	return diagnostic
}

func hashAtomicReplacePath(path string) string {
	normalized := strings.ToLower(filepath.Clean(path))
	digest := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(digest[:])
}

func atomicReplaceWin32Code(err error) uint32 {
	if err == nil {
		return 0
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return uint32(errno)
	}
	return 0
}

func probeWindowsAtomicReplacePath(path string, probeDeleteAccess bool) windowsAtomicReplacePathProbe {
	probe := windowsAtomicReplacePathProbe{}
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return probe
	}
	attributes, err := windows.GetFileAttributes(pathPtr)
	if err != nil {
		probe.ReadAttributesErrorCode = atomicReplaceWin32Code(err)
		return probe
	}
	probe.Exists = true
	probe.Attributes = attributes
	probe.ReadOnly = attributes&windows.FILE_ATTRIBUTE_READONLY != 0
	probe.ReparsePoint = attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0

	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	handle, openErr := windows.CreateFile(pathPtr, windows.FILE_READ_ATTRIBUTES, share, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if openErr != nil {
		probe.ReadAttributesErrorCode = atomicReplaceWin32Code(openErr)
	} else {
		var standard windowsFileStandardInfo
		if infoErr := windows.GetFileInformationByHandleEx(handle, windows.FileStandardInfo, (*byte)(unsafe.Pointer(&standard)), uint32(unsafe.Sizeof(standard))); infoErr != nil {
			probe.ReadAttributesErrorCode = atomicReplaceWin32Code(infoErr)
		} else {
			probe.Size = standard.EndOfFile
			probe.Links = standard.NumberOfLinks
			probe.DeletePending = standard.DeletePending != 0
		}
		_ = windows.CloseHandle(handle)
	}

	if !probeDeleteAccess {
		return probe
	}
	deleteHandle, deleteErr := windows.CreateFile(pathPtr, windowsDeleteAccess, share, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if deleteErr != nil {
		probe.DeleteAccessErrorCode = atomicReplaceWin32Code(deleteErr)
		return probe
	}
	probe.DeleteAccessGranted = true
	_ = windows.CloseHandle(deleteHandle)
	return probe
}

func probeWindowsAtomicReplaceDirectory(path string) windowsAtomicReplaceDirectoryProbe {
	probe := windowsAtomicReplaceDirectoryProbe{}
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		probe.AddFileErrorCode = uint32(windows.ERROR_INVALID_NAME)
		probe.DeleteChildErrorCode = uint32(windows.ERROR_INVALID_NAME)
		return probe
	}
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	open := func(access uint32) uint32 {
		handle, openErr := windows.CreateFile(pathPtr, access, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if openErr != nil {
			return atomicReplaceWin32Code(openErr)
		}
		_ = windows.CloseHandle(handle)
		return 0
	}
	probe.AddFileErrorCode = open(windowsFileAddFileAccess)
	probe.AddFileGranted = probe.AddFileErrorCode == 0
	probe.DeleteChildErrorCode = open(windowsFileDeleteChildAccess)
	probe.DeleteChildGranted = probe.DeleteChildErrorCode == 0
	return probe
}

func queryWindowsRestartManager(paths ...string) windowsRestartManagerDiagnostic {
	diagnostic := windowsRestartManagerDiagnostic{}
	if procRmStartSession.Find() != nil || procRmRegisterResources.Find() != nil || procRmGetList.Find() != nil || procRmEndSession.Find() != nil {
		diagnostic.StartCode = uint32(windows.ERROR_PROC_NOT_FOUND)
		return diagnostic
	}

	var session uint32
	var sessionKey [33]uint16
	startCode, _, _ := procRmStartSession.Call(uintptr(unsafe.Pointer(&session)), 0, uintptr(unsafe.Pointer(&sessionKey[0])))
	diagnostic.StartCode = uint32(startCode)
	if startCode != 0 {
		return diagnostic
	}
	defer procRmEndSession.Call(uintptr(session))

	if len(paths) == 0 {
		diagnostic.RegisterCode = uint32(windows.ERROR_BAD_ARGUMENTS)
		return diagnostic
	}
	pathPointers := make([]*uint16, 0, len(paths))
	for _, path := range paths {
		pathPtr, err := windows.UTF16PtrFromString(path)
		if err != nil {
			diagnostic.RegisterCode = uint32(windows.ERROR_INVALID_NAME)
			return diagnostic
		}
		pathPointers = append(pathPointers, pathPtr)
	}
	registerCode, _, _ := procRmRegisterResources.Call(
		uintptr(session),
		uintptr(len(pathPointers)),
		uintptr(unsafe.Pointer(&pathPointers[0])),
		0,
		0,
		0,
		0,
	)
	runtime.KeepAlive(pathPointers)
	diagnostic.RegisterCode = uint32(registerCode)
	if registerCode != 0 {
		return diagnostic
	}

	var needed uint32
	var count uint32
	var rebootReasons uint32
	getListCode, _, _ := procRmGetList.Call(
		uintptr(session),
		uintptr(unsafe.Pointer(&needed)),
		uintptr(unsafe.Pointer(&count)),
		0,
		uintptr(unsafe.Pointer(&rebootReasons)),
	)
	diagnostic.GetListCode = uint32(getListCode)
	diagnostic.ProcessesNeeded = needed
	if getListCode == 0 || needed == 0 {
		return diagnostic
	}
	if getListCode != uintptr(windows.ERROR_MORE_DATA) {
		return diagnostic
	}
	if needed > maxRestartManagerProcesses {
		diagnostic.Truncated = true
		return diagnostic
	}

	processes := make([]rmProcessInfo, needed)
	count = needed
	getListCode, _, _ = procRmGetList.Call(
		uintptr(session),
		uintptr(unsafe.Pointer(&needed)),
		uintptr(unsafe.Pointer(&count)),
		uintptr(unsafe.Pointer(&processes[0])),
		uintptr(unsafe.Pointer(&rebootReasons)),
	)
	diagnostic.GetListCode = uint32(getListCode)
	if getListCode != 0 {
		return diagnostic
	}
	if count > uint32(len(processes)) {
		count = uint32(len(processes))
	}
	diagnostic.Processes = make([]windowsRestartManagerProcess, 0, count)
	for index := uint32(0); index < count; index++ {
		process := processes[index]
		diagnostic.Processes = append(diagnostic.Processes, windowsRestartManagerProcess{
			PID:             process.Process.ProcessID,
			ApplicationType: process.ApplicationType,
			ApplicationName: windows.UTF16ToString(process.ApplicationName[:]),
			ServiceName:     windows.UTF16ToString(process.ServiceShortName[:]),
			Restartable:     process.Restartable != 0,
			CurrentProcess:  process.Process.ProcessID == uint32(os.Getpid()),
		})
	}
	return diagnostic
}

type rmUniqueProcess struct {
	ProcessID        uint32
	ProcessStartTime windows.Filetime
}

type rmProcessInfo struct {
	Process          rmUniqueProcess
	ApplicationName  [256]uint16
	ServiceShortName [64]uint16
	ApplicationType  uint32
	AppStatus        uint32
	TSSessionID      uint32
	Restartable      int32
}
