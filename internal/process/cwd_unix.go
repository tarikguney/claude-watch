//go:build !windows

package process

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// GetProcessCwd reads the current working directory of a process.
// On Linux, reads /proc/<pid>/cwd. On macOS, uses lsof.
func GetProcessCwd(pid int) (string, error) {
	// Linux: /proc/<pid>/cwd symlink
	link := fmt.Sprintf("/proc/%d/cwd", pid)
	if target, err := os.Readlink(link); err == nil {
		return target, nil
	}

	// macOS: use lsof
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("lsof", "-a", "-d", "cwd", "-Fn", "-p", fmt.Sprintf("%d", pid)).Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(line, "n") {
					return line[1:], nil
				}
			}
		}
	}

	return "", fmt.Errorf("cannot determine CWD for pid %d", pid)
}
