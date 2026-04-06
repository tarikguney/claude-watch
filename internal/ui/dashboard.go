// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/tarikguney/claude-watch/internal/session"
)

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D4A0FF")) // Soft purple/lavender
	colHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6CB6FF")) // Soft blue
	separatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	projectStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#E0A458"))            // Warm amber
	promptStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7DC4A3"))             // Soft mint/teal
	responseStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A0A0D0"))             // Soft lavender
	actionStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	durationStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	pidStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	statusStyles = map[session.Status]lipgloss.Style{
		session.StatusActive:     lipgloss.NewStyle().Foreground(lipgloss.Color("10")),  // Green
		session.StatusResponding: lipgloss.NewStyle().Foreground(lipgloss.Color("14")),  // Cyan
		session.StatusThinking:   lipgloss.NewStyle().Foreground(lipgloss.Color("11")),  // Yellow
		session.StatusIdle:       lipgloss.NewStyle().Foreground(lipgloss.Color("240")), // Dim gray
		session.StatusDone:       lipgloss.NewStyle().Foreground(lipgloss.Color("12")),  // Blue
		session.StatusError:      lipgloss.NewStyle().Foreground(lipgloss.Color("9")),   // Red
	}
)

// Column widths
const (
	colPID     = 7
	colProject = 16
	colStatus  = 10
	colAction  = 36
	colDur     = 10
)

const (
	colPIDCompact     = 7
	colProjectCompact = 12
	colStatusCompact  = 10
	colTaskCompact    = 0
	colActionCompact  = 32
	colDurCompact     = 10
)

// joinCols joins pre-padded cells with a styled vertical bar separator.
func joinCols(cells []string) string {
	sep := separatorStyle.Render(" │ ")
	return strings.Join(cells, sep)
}

// totalWidth calculates the visible width of columns + separators.
func totalWidth(widths []int) int {
	w := 0
	for _, cw := range widths {
		w += cw
	}
	w += (len(widths) - 1) * 3 // " │ " = 3 visible chars
	return w
}

func hline(char string, width int) string {
	return separatorStyle.Render(strings.Repeat(char, width))
}

// Render produces the full dashboard string for the given sessions.
// hiddenCount is the number of older sessions not shown due to max-age filtering.
func Render(sessions []session.State, compact bool, hiddenCount int) string {
	now := time.Now()
	var b strings.Builder

	// Sort by PID for stable row ordering (no jumping when status changes)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].PID < sessions[j].PID
	})

	title := titleStyle.Render("CLAUDE WATCH")
	subtitle := durationStyle.Render("— monitoring your Claude Code sessions")
	timestamp := durationStyle.Render(now.Format("01/02 15:04:05"))
	hidden := ""
	if hiddenCount > 0 {
		hidden = durationStyle.Render(fmt.Sprintf("  (+%d hidden)", hiddenCount))
	}
	b.WriteString(title + " " + subtitle + "  " + timestamp + hidden + "\n")

	if compact {
		tw := totalWidth([]int{colPIDCompact, colProjectCompact, colStatusCompact, colActionCompact, colDurCompact})
		b.WriteString(hline("─", tw) + "\n")
		b.WriteString(joinCols([]string{
			colHeaderStyle.Render(pad("PID", colPIDCompact)),
			colHeaderStyle.Render(pad("PROJECT", colProjectCompact)),
			colHeaderStyle.Render(pad("STATUS", colStatusCompact)),
			colHeaderStyle.Render(pad("CURRENT ACTION", colActionCompact)),
			colHeaderStyle.Render(pad("DURATION", colDurCompact)),
		}) + "\n")
		b.WriteString(hline("─", tw) + "\n")

		for _, s := range sessions {
			b.WriteString(renderRowCompact(s, now) + "\n")
		}
	} else {
		tw := totalWidth([]int{colPID, colProject, colStatus, colAction, colDur})
		b.WriteString(hline("─", tw) + "\n")
		b.WriteString(joinCols([]string{
			colHeaderStyle.Render(pad("PID", colPID)),
			colHeaderStyle.Render(pad("PROJECT", colProject)),
			colHeaderStyle.Render(pad("STATUS", colStatus)),
			colHeaderStyle.Render(pad("CURRENT ACTION", colAction)),
			colHeaderStyle.Render(pad("DURATION", colDur)),
		}) + "\n")
		b.WriteString(hline("─", tw) + "\n")

		for i, s := range sessions {
			b.WriteString(renderRow(s, now) + "\n")
			prompt := s.LastPrompt
			if prompt == "" {
				prompt = s.OriginalTask
			}
			if prompt != "" {
				b.WriteString(durationStyle.Render("  » prompt: ") + promptStyle.Render(prompt) + "\n")
			}
			if s.LastResponse != "" {
				b.WriteString(durationStyle.Render("  » response: ") + responseStyle.Render(s.LastResponse) + "\n")
			}
			if i < len(sessions)-1 {
				b.WriteString(hline("─", tw) + "\n")
			}
		}
	}

	if len(sessions) == 0 {
		b.WriteString(durationStyle.Render("  No active sessions found. Watching for new sessions...") + "\n")
	}

	return b.String()
}

func renderRow(s session.State, now time.Time) string {
	dur := ""
	if !s.StartTime.IsZero() {
		dur = session.FormatDuration(now.Sub(s.StartTime))
	}

	action := actionForStatus(s)

	pidStr := ""
	if s.PID > 0 {
		pidStr = fmt.Sprintf("%d", s.PID)
	}

	return joinCols([]string{
		pidStyle.Render(pad(pidStr, colPID)),
		projectStyle.Render(pad(s.ProjectName, colProject)),
		styledStatus(s.Status, colStatus),
		actionStyle.Render(pad(action, colAction)),
		durationStyle.Render(pad(dur, colDur)),
	})
}

func renderRowCompact(s session.State, now time.Time) string {
	dur := ""
	if !s.StartTime.IsZero() {
		dur = session.FormatDuration(now.Sub(s.StartTime))
	}

	action := actionForStatus(s)

	pidStr := ""
	if s.PID > 0 {
		pidStr = fmt.Sprintf("%d", s.PID)
	}

	return joinCols([]string{
		pidStyle.Render(pad(pidStr, colPIDCompact)),
		projectStyle.Render(pad(s.ProjectName, colProjectCompact)),
		styledStatus(s.Status, colStatusCompact),
		actionStyle.Render(pad(action, colActionCompact)),
		durationStyle.Render(pad(dur, colDurCompact)),
	})
}

func styledStatus(status session.Status, width int) string {
	style, ok := statusStyles[status]
	if !ok {
		style = lipgloss.NewStyle()
	}
	return style.Render(pad(string(status), width))
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

// actionForStatus returns the action text appropriate for the session's status.
// Only show the current action when Claude is actively working.
func actionForStatus(s session.State) string {
	switch s.Status {
	case session.StatusActive, session.StatusThinking:
		return s.CurrentAction
	case session.StatusDone:
		return "Completed"
	default:
		return ""
	}
}

func statusPriority(s session.Status) int {
	switch s {
	case session.StatusActive:
		return 0
	case session.StatusResponding:
		return 1
	case session.StatusThinking:
		return 2
	case session.StatusError:
		return 3
	case session.StatusIdle:
		return 4
	case session.StatusDone:
		return 5
	default:
		return 6
	}
}
