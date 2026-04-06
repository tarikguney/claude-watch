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
	colProject = 16
	colStatus  = 10
	colTask    = 32
	colAction  = 36
	colDur     = 7
)

const (
	colProjectCompact = 12
	colStatusCompact  = 10
	colTaskCompact    = 0
	colActionCompact  = 32
	colDurCompact     = 7
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
	b.WriteString(title + "  " + timestamp + "\n")

	if compact {
		tw := totalWidth([]int{colProjectCompact, colStatusCompact, colActionCompact, colDurCompact})
		b.WriteString(hline("─", tw) + "\n")
		b.WriteString(joinCols([]string{
			headerStyle.Render(pad("PROJECT", colProjectCompact)),
			headerStyle.Render(pad("STATUS", colStatusCompact)),
			headerStyle.Render(pad("CURRENT ACTION", colActionCompact)),
			headerStyle.Render(pad("DUR", colDurCompact)),
		}) + "\n")
		b.WriteString(hline("─", tw) + "\n")

		for i, s := range sessions {
			b.WriteString(renderRowCompact(s, now) + "\n")
			if i < len(sessions)-1 {
				b.WriteString(hline("─", tw) + "\n")
			}
		}
		if len(sessions) > 0 {
			b.WriteString(hline("─", tw) + "\n")
		}
	} else {
		tw := totalWidth([]int{colProject, colStatus, colTask, colAction, colDur})
		b.WriteString(hline("─", tw) + "\n")
		b.WriteString(joinCols([]string{
			headerStyle.Render(pad("PROJECT", colProject)),
			headerStyle.Render(pad("STATUS", colStatus)),
			headerStyle.Render(pad("CONTEXT", colTask)),
			headerStyle.Render(pad("CURRENT ACTION", colAction)),
			headerStyle.Render(pad("DUR", colDur)),
		}) + "\n")
		b.WriteString(hline("─", tw) + "\n")

		for i, s := range sessions {
			b.WriteString(renderRow(s, now) + "\n")
			if i < len(sessions)-1 {
				b.WriteString(hline("─", tw) + "\n")
			}
		}
		if len(sessions) > 0 {
			b.WriteString(hline("─", tw) + "\n")
		}
	}

	if len(sessions) == 0 {
		b.WriteString(durationStyle.Render("  No active sessions found. Watching for new sessions...") + "\n")
	}

	if hiddenCount > 0 {
		b.WriteString(durationStyle.Render(fmt.Sprintf("\n  %d older session(s) hidden. Use --max-age=0 to show all.", hiddenCount)) + "\n")
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

	return joinCols([]string{
		pad(s.ProjectName, colProject),
		styledStatus(s.Status, colStatus),
		pad(s.OriginalTask, colTask),
		actionStyle.Render(pad(action, colAction)),
		durationStyle.Render(pad(dur, colDur)),
	})
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

	return joinCols([]string{
		pad(s.ProjectName, colProjectCompact),
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
