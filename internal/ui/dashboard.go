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
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	separatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	projectStyle   = lipgloss.NewStyle().Bold(true)
	actionStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	durationStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

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
	colProject = 14
	colStatus  = 12
	colTask    = 30
	colAction  = 34
	colDur     = 7
)

const (
	colProjectCompact = 10
	colStatusCompact  = 10
	colTaskCompact    = 0
	colActionCompact  = 30
	colDurCompact     = 6
)

// Render produces the full dashboard string for the given sessions.
func Render(sessions []session.State, compact bool) string {
	now := time.Now()
	var b strings.Builder

	// Sort: Active/Responding/Thinking first, then by last update descending
	sort.Slice(sessions, func(i, j int) bool {
		pi := statusPriority(sessions[i].Status)
		pj := statusPriority(sessions[j].Status)
		if pi != pj {
			return pi < pj
		}
		return sessions[i].LastUpdate.After(sessions[j].LastUpdate)
	})

	title := headerStyle.Render("CLAUDE WATCH")
	timestamp := durationStyle.Render(now.Format("01/02 15:04:05"))

	if compact {
		b.WriteString(fmt.Sprintf("%s  %s\n", title, timestamp))
		sep := separatorStyle.Render(strings.Repeat("─", 60))
		b.WriteString(sep + "\n")
		header := fmt.Sprintf("%-*s %-*s %-*s %*s",
			colProjectCompact, "PROJECT",
			colStatusCompact, "STATUS",
			colActionCompact, "CURRENT ACTION",
			colDurCompact, "DUR")
		b.WriteString(headerStyle.Render(header) + "\n")

		for _, s := range sessions {
			b.WriteString(renderRowCompact(s, now) + "\n")
		}
	} else {
		b.WriteString(fmt.Sprintf("%-60s %s\n", title, timestamp))
		sep := separatorStyle.Render(strings.Repeat("─", 97))
		b.WriteString(sep + "\n")
		header := fmt.Sprintf("%-*s %-*s %-*s %-*s %*s",
			colProject, "PROJECT",
			colStatus, "STATUS",
			colTask, "TASK",
			colAction, "CURRENT ACTION",
			colDur, "DUR")
		b.WriteString(headerStyle.Render(header) + "\n")

		for _, s := range sessions {
			b.WriteString(renderRow(s, now) + "\n")
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

	action := s.CurrentAction
	if s.Status == session.StatusDone {
		action = "Completed"
	}

	return fmt.Sprintf("%-*s %s %-*s %-*s %*s",
		colProject, pad(s.ProjectName, colProject),
		renderStatus(s.Status, colStatus),
		colTask, pad(s.OriginalTask, colTask),
		colAction, actionStyle.Render(pad(action, colAction)),
		colDur, durationStyle.Render(dur))
}

func renderRowCompact(s session.State, now time.Time) string {
	dur := ""
	if !s.StartTime.IsZero() {
		dur = session.FormatDuration(now.Sub(s.StartTime))
	}

	action := s.CurrentAction
	if s.Status == session.StatusDone {
		action = "Completed"
	}

	return fmt.Sprintf("%-*s %s %-*s %*s",
		colProjectCompact, pad(s.ProjectName, colProjectCompact),
		renderStatus(s.Status, colStatusCompact),
		colActionCompact, actionStyle.Render(pad(action, colActionCompact)),
		colDurCompact, durationStyle.Render(dur))
}

func renderStatus(status session.Status, width int) string {
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
