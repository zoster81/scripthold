//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func externalChildProcessIDs(parentPID int) ([]int, error) {
	script := fmt.Sprintf(`Get-CimInstance Win32_Process -Filter "ParentProcessId = %d" | ForEach-Object { $_.ProcessId }`, parentPID)
	output, err := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(output))
	ids := make([]int, 0, len(fields))
	for _, field := range fields {
		id, parseErr := strconv.Atoi(field)
		if parseErr != nil {
			return nil, parseErr
		}
		ids = append(ids, id)
	}
	return ids, nil
}
