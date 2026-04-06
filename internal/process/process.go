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
	PID       int
	SessionID string
	WorkDir   string // from --add-dir flag
	StartTime time.Time
}

var sessionIDRe = regexp.MustCompile(`--session-id\s+([0-9a-f-]{36})`)
var addDirRe = regexp.MustCompile(`--add-dir\s+(\S+)`)

// ListClaude returns info for all running claude.exe / claude processes.
func ListClaude() ([]Info, error) {
	switch runtime.GOOS {
	case "windows":
		return listWindows()
	default:
		return listUnix()
	}
}

func listWindows() ([]Info, error) {
	// Use PowerShell to get process details including command line
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-CimInstance Win32_Process -Filter "Name='claude.exe'" | ForEach-Object { "$($_.ProcessId)|$($_.CreationDate.ToString('o'))|$($_.CommandLine)" }`)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("powershell process query failed: %w", err)
	}
	return parseLines(string(out))
}

func listUnix() ([]Info, error) {
	// Use ps to get PID, start time, and full command
	cmd := exec.Command("ps", "axo", "pid,lstart,command")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps command failed: %w", err)
	}

	var results []Info
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "claude") || strings.Contains(line, "claude-watch") {
			continue
		}
		// Skip the header and grep itself
		if strings.HasPrefix(line, "PID") {
			continue
		}
		info := parseUnixLine(line)
		if info.SessionID != "" {
			results = append(results, info)
		}
	}
	return results, nil
}

func parseLines(output string) ([]Info, error) {
	var results []Info
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}

		pid, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}

		startTime, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(parts[1]))
		cmdLine := parts[2]

		// Skip non-session processes (e.g. claude-watch itself, helper processes)
		sessionID := extractFlag(sessionIDRe, cmdLine)
		if sessionID == "" {
			continue
		}

		info := Info{
			PID:       pid,
			SessionID: sessionID,
			WorkDir:   extractFlag(addDirRe, cmdLine),
			StartTime: startTime,
		}
		results = append(results, info)
	}
	return results, nil
}

func parseUnixLine(line string) Info {
	// Extract session ID and add-dir from the command line portion
	sessionID := extractFlag(sessionIDRe, line)
	workDir := extractFlag(addDirRe, line)

	// Try to extract PID (first field)
	fields := strings.Fields(line)
	pid := 0
	if len(fields) > 0 {
		pid, _ = strconv.Atoi(fields[0])
	}

	return Info{
		PID:       pid,
		SessionID: sessionID,
		WorkDir:   workDir,
	}
}

func extractFlag(re *regexp.Regexp, cmdLine string) string {
	matches := re.FindStringSubmatch(cmdLine)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
