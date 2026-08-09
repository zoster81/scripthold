//go:build darwin

package main

import (
	"os/exec"
	"strconv"
	"strings"
)

func externalChildProcessIDs(parentPID int) ([]int, error) {
	output, err := exec.Command("ps", "-o", "pid=", "-P", strconv.Itoa(parentPID)).Output()
	if err != nil && len(output) == 0 {
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
