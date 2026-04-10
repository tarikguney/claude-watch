// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/tarikguney/claude-watch/internal/session"
)

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D4A0FF")) // Soft purple/lavender
	colHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6CB6FF")) // Soft blue
	separatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	projectStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#E0A458"))            // Warm amber
	promptStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7DC4A3"))             // Soft mint/teal
	responseStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A0A0D0"))             // Soft lavender
	tmuxStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#9EC8E0")) // Soft cyan
	actionStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	durationStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	pidStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D4A0FF"))  // Bright arrow indicator
	helpKeyStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6CB6FF")) // Soft blue for keys
	helpTextStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))                // Gray for descriptions

	statusStyles = map[session.Status]lipgloss.Style{
		session.StatusResponding:  lipgloss.NewStyle().Background(lipgloss.Color("10")).Foreground(lipgloss.Color("0")).Bold(true),  // Green bg
		session.StatusIdle:        lipgloss.NewStyle().Background(lipgloss.Color("240")).Foreground(lipgloss.Color("15")),           // Gray bg
		session.StatusDone:        lipgloss.NewStyle().Background(lipgloss.Color("12")).Foreground(lipgloss.Color("15")).Bold(true), // Blue bg
		session.StatusError:       lipgloss.NewStyle().Background(lipgloss.Color("9")).Foreground(lipgloss.Color("15")).Bold(true),  // Red bg
		session.StatusInterrupted: lipgloss.NewStyle().Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0")).Bold(true),  // Yellow bg
		session.StatusWaiting:    lipgloss.NewStyle().Background(lipgloss.Color("13")).Foreground(lipgloss.Color("0")).Bold(true),  // Magenta bg
	}
)

// RenderOpts controls per-render display options.
type RenderOpts struct {
	Compact   bool
	CursorPID int          // PID of selected session (0 = no cursor)
	Expanded  map[int]bool // keyed by PID; true = expanded (default is collapsed)
}

// cols holds computed column widths for a render pass.
type cols struct {
	pid     int
	tmux    int
	project int
	status  int
	action  int
	dur     int
}

func getTerminalWidth() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		return 120
	}
	return w
}

// computeCols calculates column widths from actual session data,
// then expands the ACTION column to fill the terminal width.
func computeCols(sessions []session.State, now time.Time) cols {
	// Check if any session has tmux info
	hasTmux := false
	for _, s := range sessions {
		if s.TmuxSession != "" {
			hasTmux = true
			break
		}
	}

	// Start with minimum widths based on header text + padding
	c := cols{
		pid:     len("PID") + 2,
		tmux:    0, // hidden unless tmux data exists
		project: len("PROJECT") + 2,
		status:  len("Interrupted") + 2, // fixed width — widest possible status
		action:  len("CURRENT ACTION") + 2,
		dur:     len("DURATION") + 2,
	}

	if hasTmux {
		c.tmux = len("TMUX SESSION/WINDOW") + 2
	}

	// Expand to fit content
	for _, s := range sessions {
		if w := len(s.ProjectName) + 2; w > c.project {
			c.project = w
		}
		if hasTmux {
			if w := len(s.TmuxSession) + 2; w > c.tmux {
				c.tmux = w
			}
		}
		action := actionForStatus(s)
		if w := len(action) + 2; w > c.action {
			c.action = w
		}
		dur := ""
		if !s.StartTime.IsZero() {
			dur = session.FormatDuration(now.Sub(s.StartTime))
		}
		if w := len(dur) + 2; w > c.dur {
			c.dur = w
		}
		pidStr := ""
		if s.PID > 0 {
			pidStr = fmt.Sprintf("%d", s.PID)
		}
		if w := len(pidStr) + 2; w > c.pid {
			c.pid = w
		}
	}

	// Cap columns to prevent one wide name from eating all space
	if c.project > 30 {
		c.project = 30
	}
	if c.tmux > 40 {
		c.tmux = 40
	}

	// Expand ACTION to fill remaining terminal width
	termW := getTerminalWidth()
	numSep := 4 // PID, PROJECT, STATUS, DURATION boundaries
	if hasTmux {
		numSep = 5 // + SESSION boundary
	}
	separators := numSep * 3 // " │ " = 3 chars each
	fixed := c.pid + c.tmux + c.project + c.status + c.dur + separators
	remaining := termW - fixed
	if remaining > c.action {
		c.action = remaining
	}

	return c
}

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
func Render(sessions []session.State, opts RenderOpts, hiddenCount int) string {
	now := time.Now()
	var b strings.Builder

	// Sort by PID for stable row ordering (no jumping when status changes)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].PID < sessions[j].PID
	})

	c := computeCols(sessions, now)

	title := titleStyle.Render("CLAUDE WATCH")
	subtitle := durationStyle.Render("— monitoring your Claude Code sessions")
	timestamp := durationStyle.Render(now.Format("01/02 15:04:05"))
	hidden := ""
	if hiddenCount > 0 {
		hidden = durationStyle.Render(fmt.Sprintf("  (+%d hidden)", hiddenCount))
	}
	b.WriteString(title + " " + subtitle + "  " + timestamp + hidden + "\n")

	widths := []int{c.pid, c.project, c.status, c.action, c.dur}
	headers := []string{
		colHeaderStyle.Render(pad("PID", c.pid)),
		colHeaderStyle.Render(pad("PROJECT", c.project)),
		colHeaderStyle.Render(pad("STATUS", c.status)),
		colHeaderStyle.Render(pad("CURRENT ACTION", c.action)),
		colHeaderStyle.Render(pad("DURATION", c.dur)),
	}
	if c.tmux > 0 {
		// Insert SESSION after PID
		widths = append(widths[:1], append([]int{c.tmux}, widths[1:]...)...)
		headers = append(headers[:1], append([]string{
			colHeaderStyle.Render(pad("TMUX SESSION/WINDOW", c.tmux)),
		}, headers[1:]...)...)
	}
	tw := totalWidth(widths)
	b.WriteString(hline("─", tw) + "\n")
	b.WriteString(joinCols(headers) + "\n")
	b.WriteString(hline("─", tw) + "\n")

	for i, s := range sessions {
		isCursor := opts.CursorPID != 0 && s.PID == opts.CursorPID
		isExpanded := opts.Expanded[s.PID]

		b.WriteString(renderRow(s, now, c, isCursor) + "\n")

		if !opts.Compact && isExpanded {
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
		}
		if i < len(sessions)-1 {
			b.WriteString(hline("─", tw) + "\n")
		}
	}

	if len(sessions) == 0 {
		b.WriteString(durationStyle.Render("  No active sessions found. Watching for new sessions...") + "\n")
	}

	// Help bar
	if !opts.Compact {
		b.WriteString("\n" + hline("─", tw) + "\n")
		b.WriteString(
			helpKeyStyle.Render("↑↓") + helpTextStyle.Render(" Navigate  ") +
				helpKeyStyle.Render("Enter") + helpTextStyle.Render(" Toggle  ") +
				helpKeyStyle.Render("e") + helpTextStyle.Render(" Expand All  ") +
				helpKeyStyle.Render("c") + helpTextStyle.Render(" Collapse All  ") +
				helpKeyStyle.Render("q") + helpTextStyle.Render(" Quit"),
		)
	}

	return b.String()
}

func renderRow(s session.State, now time.Time, c cols, isCursor bool) string {
	dur := ""
	if !s.StartTime.IsZero() {
		dur = session.FormatDuration(now.Sub(s.StartTime))
	}

	action := actionForStatus(s)

	pidStr := ""
	if s.PID > 0 {
		pidStr = fmt.Sprintf("%d", s.PID)
	}

	// Embed cursor indicator inside the PID cell
	if isCursor {
		pidStr = cursorStyle.Render(">") + " " + pidStyle.Render(pad(pidStr, c.pid-2))
	} else {
		pidStr = pidStyle.Render(pad(pidStr, c.pid))
	}

	cells := []string{
		pidStr,
		projectStyle.Render(pad(s.ProjectName, c.project)),
		styledStatus(s.Status, c.status),
		actionStyle.Render(pad(action, c.action)),
		durationStyle.Render(pad(dur, c.dur)),
	}
	if c.tmux > 0 {
		tmuxCell := tmuxStyle.Render(pad(s.TmuxSession, c.tmux))
		if s.TmuxSession == "" {
			tmuxCell = durationStyle.Render(pad("not in tmux", c.tmux))
		}
		// Insert SESSION after PID
		cells = append(cells[:1], append([]string{tmuxCell}, cells[1:]...)...)
	}
	return joinCols(cells)
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
	case session.StatusResponding:
		return s.CurrentAction
	case session.StatusDone:
		return "Completed"
	case session.StatusInterrupted:
		return "Interrupted by user"
	case session.StatusWaiting:
		return "Waiting for first prompt..."
	default:
		return ""
	}
}

func statusPriority(s session.Status) int {
	switch s {
	case session.StatusResponding:
		return 0
	case session.StatusError:
		return 1
	case session.StatusInterrupted:
		return 2
	case session.StatusIdle:
		return 3
	case session.StatusWaiting:
		return 4
	case session.StatusDone:
		return 5
	default:
		return 6
	}
}
