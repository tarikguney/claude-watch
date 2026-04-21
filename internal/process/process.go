package process

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Info holds metadata extracted from a running Claude process.
type Info struct {
	PID        int
	SessionID  string
	Cwd        string // current working directory read from OS
	StartTime  time.Time
	ParentPIDs []int // ancestor PIDs from parent up (for tmux pane matching)
}

var sessionIDRe = regexp.MustCompile(`--session-id\s+([0-9a-f-]{36})`)

// ListClaude returns info for all running claude / claude.exe processes.
func ListClaude() ([]Info, error) {
	switch runtime.GOOS {
	case "windows":
		return listWindows()
	default:
		return listUnix()
	}
}

func listWindows() ([]Info, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-CimInstance Win32_Process -Filter "Name='claude.exe'" | ForEach-Object { "$($_.ProcessId)|$($_.ParentProcessId)|$($_.CreationDate.ToString('o'))|$($_.CommandLine)" }`)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("powershell process query failed: %w", err)
	}
	return parsePipedLinesWithCache(string(out), buildParentPIDMap())
}

func listUnix() ([]Info, error) {
	// Use ps with a specific format. lstart gives a fixed-width date like:
	//   "Sun Apr  6 10:53:38 2026"
	// which is always 24 characters. The format is: pid (right-aligned), ppid, lstart (24 chars), command.
	cmd := exec.Command("ps", "axo", "pid=,ppid=,lstart=,command=")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps command failed: %w", err)
	}

	// First pass: build a pid->ppid map from every process (needed to walk
	// ancestors beyond the immediate parent without re-querying /proc).
	lines := strings.Split(string(out), "\n")
	cache := make(map[int]int, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, _ := strconv.Atoi(fields[1])
		cache[pid] = ppid
	}

	var results []Info
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Only consider lines that have --session-id (definitive Claude CLI sessions).
		// This avoids false positives from other processes with "claude" in the name.
		if !strings.Contains(line, "--session-id") {
			continue
		}

		// Skip claude-watch itself
		if strings.Contains(line, "claude-watch") {
			continue
		}

		info := parseUnixLineWithCache(line, cache)
		if info.SessionID != "" && info.PID > 0 {
			results = append(results, info)
		}
	}
	return results, nil
}

// parsePipedLines parses Windows PowerShell output in "PID|PPID|StartTime|CommandLine" format.
func parsePipedLines(output string) ([]Info, error) {
	return parsePipedLinesWithCache(output, nil)
}

// parsePipedLinesWithCache is like parsePipedLines but uses a prebuilt
// pid->ppid map for ancestor walks. Pass nil to fall back to per-call lookup.
func parsePipedLinesWithCache(output string, cache map[int]int) ([]Info, error) {
	var results []Info
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}

		pid, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}

		ppid, _ := strconv.Atoi(strings.TrimSpace(parts[1]))

		startTime, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(parts[2]))
		cmdLine := parts[3]

		sessionID := extractFlag(sessionIDRe, cmdLine)
		if sessionID == "" {
			continue
		}

		cwd, _ := GetProcessCwd(pid)
		parentPIDs := walkParentPIDs(ppid, 6, cache)
		results = append(results, Info{
			PID:        pid,
			SessionID:  sessionID,
			Cwd:        cwd,
			StartTime:  startTime,
			ParentPIDs: parentPIDs,
		})
	}
	return results, nil
}

// parseUnixLine parses a single line from `ps axo pid=,ppid=,lstart=,command=`.
// Format: "  12345  6789 Sun Apr  6 10:53:38 2026 /path/to/claude --session-id ..."
// The lstart field is always 24 characters wide.
func parseUnixLine(line string) Info {
	return parseUnixLineWithCache(line, nil)
}

// parseUnixLineWithCache is like parseUnixLine but uses a prebuilt pid->ppid
// map for ancestor walks. Pass nil to fall back to per-call /proc reads.
func parseUnixLineWithCache(line string, cache map[int]int) Info {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return Info{}
	}

	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return Info{}
	}

	ppid, _ := strconv.Atoi(fields[1])

	// After PID and PPID fields, the rest starts with lstart (24 chars)
	pidStr := fields[0]
	ppidStr := fields[1]
	afterPID := strings.TrimSpace(line[strings.Index(line, pidStr)+len(pidStr):])
	rest := strings.TrimSpace(afterPID[strings.Index(afterPID, ppidStr)+len(ppidStr):])

	var startTime time.Time
	// lstart is 24 chars: "Sun Apr  6 10:53:38 2026"
	if len(rest) >= 24 {
		dateStr := rest[:24]
		// Go reference time: Mon Jan 2 15:04:05 2006
		// lstart format:     Sun Apr  6 10:53:38 2026
		startTime, _ = time.Parse("Mon Jan  2 15:04:05 2006", dateStr)
		if startTime.IsZero() {
			// Try single-space day variant: "Mon Apr 16 10:53:38 2026"
			startTime, _ = time.Parse("Mon Jan 2 15:04:05 2006", strings.TrimSpace(dateStr))
		}
		rest = rest[24:]
	}

	// rest is now the command line
	cmdLine := strings.TrimSpace(rest)

	sessionID := extractFlag(sessionIDRe, cmdLine)
	cwd, _ := GetProcessCwd(pid)
	parentPIDs := walkParentPIDs(ppid, 6, cache)

	return Info{
		PID:        pid,
		SessionID:  sessionID,
		Cwd:        cwd,
		StartTime:  startTime,
		ParentPIDs: parentPIDs,
	}
}

func extractFlag(re *regexp.Regexp, cmdLine string) string {
	matches := re.FindStringSubmatch(cmdLine)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// walkParentPIDs collects ancestor PIDs starting from ppid, walking up to maxDepth levels.
// If cache is non-nil, ancestor lookups come from the map; otherwise each level
// calls getParentPID (which may be expensive, e.g. a full process-table snapshot
// on Windows).
func walkParentPIDs(ppid int, maxDepth int, cache map[int]int) []int {
	if ppid <= 0 {
		return nil
	}
	pids := []int{ppid}
	current := ppid
	for i := 1; i < maxDepth; i++ {
		var parent int
		if cache != nil {
			parent = cache[current]
		} else {
			parent = getParentPID(current)
		}
		if parent <= 0 || parent == current {
			break
		}
		pids = append(pids, parent)
		current = parent
	}
	return pids
}
