// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package tmux

import (
	"os/exec"
	"strconv"
	"strings"
)

// PaneInfo holds the tmux/psmux session and window name for a pane.
type PaneInfo struct {
	PanePID     int
	SessionName string
	WindowName  string
}

// tmuxBin returns the first available tmux-compatible binary, or "" if none found.
func tmuxBin() string {
	for _, name := range []string{"psmux", "tmux", "pmux"} {
		if p, err := exec.LookPath(name); err == nil && p != "" {
			return name
		}
	}
	return ""
}

// Available reports whether a tmux-compatible multiplexer is running.
func Available() bool {
	return tmuxBin() != ""
}

// ListPanes queries all panes across all tmux/psmux sessions.
// Returns a map from pane PID to PaneInfo.
// Works around psmux's broken `list-panes -a` by iterating sessions.
func ListPanes() map[int]PaneInfo {
	bin := tmuxBin()
	if bin == "" {
		return nil
	}

	sessions := listSessions(bin)
	if len(sessions) == 0 {
		return nil
	}

	result := make(map[int]PaneInfo)
	for _, sess := range sessions {
		panes := listSessionPanes(bin, sess)
		for _, p := range panes {
			result[p.PanePID] = p
		}
	}
	return result
}

// listSessions returns the names of all tmux sessions.
func listSessions(bin string) []string {
	out, err := exec.Command(bin, "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil
	}
	var sessions []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			sessions = append(sessions, line)
		}
	}
	return sessions
}

// listSessionPanes returns pane info for all panes in a given session.
func listSessionPanes(bin, sessionName string) []PaneInfo {
	out, err := exec.Command(bin, "list-panes", "-s", "-t", sessionName,
		"-F", "#{pane_pid} #{session_name} #{window_name}").Output()
	if err != nil {
		return nil
	}
	var panes []PaneInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 3 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		panes = append(panes, PaneInfo{
			PanePID:     pid,
			SessionName: parts[1],
			WindowName:  parts[2],
		})
	}
	return panes
}

// Resolve finds the tmux session/window for a process by walking its parent
// PID chain and matching against pane PIDs.
// Returns "session/window" or "" if no match.
func Resolve(paneMap map[int]PaneInfo, parentPIDs []int) string {
	for _, ppid := range parentPIDs {
		if info, ok := paneMap[ppid]; ok {
			return info.SessionName + "/" + info.WindowName
		}
	}
	return ""
}
