//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

func externalChildProcessIDs(parentPID int) ([]int, error) {
	output, err := exec.Command("ps", "-axo", "pid=,ppid=").Output()
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(output))
	if len(fields)%2 != 0 {
		return nil, fmt.Errorf("unexpected ps PID/PPID field count: %d", len(fields))
	}
	ids := make([]int, 0)
	for index := 0; index < len(fields); index += 2 {
		id, idErr := strconv.Atoi(fields[index])
		ppid, parentErr := strconv.Atoi(fields[index+1])
		if idErr != nil || parentErr != nil {
			return nil, fmt.Errorf("parse ps PID/PPID pair %q %q", fields[index], fields[index+1])
		}
		if ppid == parentPID {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids, nil
}
