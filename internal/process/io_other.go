//go:build !windows

package process

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// GetIOReadBytes returns the cumulative number of bytes read by a process.
// On Linux, reads rchar from /proc/<pid>/io. On other platforms, returns an error.
func GetIOReadBytes(pid int) (uint64, error) {
	if runtime.GOOS != "linux" {
		return 0, errors.New("IO counters not supported on " + runtime.GOOS)
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/io", pid))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "rchar:") {
			val, err := strconv.ParseUint(strings.TrimSpace(line[6:]), 10, 64)
			if err != nil {
				return 0, err
			}
			return val, nil
		}
	}
	return 0, errors.New("rchar not found in /proc/io")
}
