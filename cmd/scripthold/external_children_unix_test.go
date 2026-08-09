//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func externalChildProcessIDs(parentPID int) ([]int, error) {
	paths, err := filepath.Glob(fmt.Sprintf("/proc/%d/task/*/children", parentPID))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, os.ErrNotExist
	}
	unique := make(map[int]struct{})
	for _, path := range paths {
		output, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, readErr
		}
		for _, field := range strings.Fields(string(output)) {
			id, parseErr := strconv.Atoi(field)
			if parseErr != nil {
				return nil, parseErr
			}
			unique[id] = struct{}{}
		}
	}
	ids := make([]int, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids, nil
}
