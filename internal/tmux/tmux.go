// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package tmux

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// PaneInfo holds the tmux/psmux session and window name for a pane.
type PaneInfo struct {
	PanePID     int
	PaneID      string // tmux pane unique ID, e.g. "%5"
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
		"-F", "#{pane_pid} #{pane_id} #{session_name} #{window_name}").Output()
	if err != nil {
		return nil
	}
	var panes []PaneInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 4)
		if len(parts) < 4 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		panes = append(panes, PaneInfo{
			PanePID:     pid,
			PaneID:      parts[1],
			SessionName: parts[2],
			WindowName:  parts[3],
		})
	}
	return panes
}

// Resolve finds the tmux session/window for a process by walking its parent
// PID chain and matching against pane PIDs.
// Returns ("session/window", paneID) or ("", "") if no match.
func Resolve(paneMap map[int]PaneInfo, parentPIDs []int) (string, string) {
	for _, ppid := range parentPIDs {
		if info, ok := paneMap[ppid]; ok {
			return info.SessionName + "/" + info.WindowName, info.PaneID
		}
	}
	return "", ""
}

// SwitchToPane switches the current tmux client to the given session and window.
// target should be in "session/window" format (as stored in State.TmuxSession).
func SwitchToPane(target string) error {
	bin := tmuxBin()
	if bin == "" {
		return fmt.Errorf("no tmux binary available")
	}
	// Convert "session/window" to "session:window" for tmux target syntax
	tmuxTarget := strings.Replace(target, "/", ":", 1)
	if out, err := exec.Command(bin, "switch-client", "-t", tmuxTarget).CombinedOutput(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}
