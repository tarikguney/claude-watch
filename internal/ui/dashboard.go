// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tarikguney/claude-watch/internal/session"
	"github.com/tarikguney/claude-watch/internal/tmux"
)

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D4A0FF")) // Soft purple/lavender
	colHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6CB6FF")) // Soft blue
	separatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	projectStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#E0A458"))           // Warm amber
	promptStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7DC4A3"))            // Soft mint/teal
	responseStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A0A0D0"))            // Soft lavender
	tmuxStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#9EC8E0"))            // Soft cyan
	actionStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	durationStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	pidStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D4A0FF")) // Bright arrow indicator
	helpKeyStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6CB6FF")) // Soft blue for keys
	helpTextStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))                // Gray for descriptions

	statusStyles = map[session.Status]lipgloss.Style{
		session.StatusThinking:    lipgloss.NewStyle().Background(lipgloss.Color("#D4A017")).Foreground(lipgloss.Color("0")).Bold(true),  // Amber bg
		session.StatusToolUse:     lipgloss.NewStyle().Background(lipgloss.Color("10")).Foreground(lipgloss.Color("0")).Bold(true),      // Green bg
		session.StatusStreaming:   lipgloss.NewStyle().Background(lipgloss.Color("14")).Foreground(lipgloss.Color("0")).Bold(true),      // Cyan bg
		session.StatusResponding:  lipgloss.NewStyle().Background(lipgloss.Color("10")).Foreground(lipgloss.Color("0")).Bold(true),      // Green bg (fallback)
		session.StatusIdle:        lipgloss.NewStyle().Background(lipgloss.Color("240")).Foreground(lipgloss.Color("15")),               // Gray bg
		session.StatusDone:        lipgloss.NewStyle().Background(lipgloss.Color("12")).Foreground(lipgloss.Color("15")).Bold(true),     // Blue bg
		session.StatusError:       lipgloss.NewStyle().Background(lipgloss.Color("9")).Foreground(lipgloss.Color("15")).Bold(true),      // Red bg
		session.StatusInterrupted: lipgloss.NewStyle().Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0")).Bold(true),      // Yellow bg
		session.StatusWaiting:     lipgloss.NewStyle().Background(lipgloss.Color("13")).Foreground(lipgloss.Color("0")).Bold(true),      // Magenta bg
	}
)

// cols holds computed column widths for a render pass.
type cols struct {
	pid     int
	tmux    int
	project int
	status  int
	action  int
	dur     int
}

// computeCols calculates column widths from actual session data,
// then expands the ACTION column to fill the terminal width.
func computeCols(sessions []session.State, now time.Time, termW int) cols {
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

	if termW <= 0 {
		termW = 120
	}
	numSep := 4 // PID, PROJECT, STATUS, DURATION boundaries
	if hasTmux {
		numSep = 5 // + SESSION boundary
	}
	separators := numSep * 3 // " │ " = 3 chars each
	minAction := len("CURRENT ACTION") + 2

	// Shrink flexible columns (tmux, project) until action fits
	for {
		fixed := c.pid + c.tmux + c.project + c.status + c.dur + separators
		remaining := termW - fixed
		if remaining >= minAction {
			c.action = remaining
			break
		}
		// Shrink the wider of tmux/project first
		shrunk := false
		if c.tmux > len("TMUX")+2 {
			c.tmux--
			shrunk = true
		}
		if remaining < minAction && c.project > len("PROJECT")+2 {
			c.project--
			shrunk = true
		}
		if !shrunk {
			// Can't shrink further — give action whatever is left
			if remaining > 0 {
				c.action = remaining
			}
			break
		}
	}

	return c
}

type tickMsg time.Time

// Model is the Bubbletea model for the claude-watch dashboard.
type Model struct {
	scanner   *session.Scanner
	compact   bool
	refresh   time.Duration
	sessions  []session.State
	cursorIdx int
	expanded  map[int]bool
	termW     int
	termH     int
	statusMsg string
	statusExp time.Time
}

// NewModel creates a new dashboard Model, pre-populated with the scanner's
// current sessions so the first render is not blank.
func NewModel(scanner *session.Scanner, compact bool, refresh time.Duration) Model {
	sessions := scanner.RunningSessions()
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].PID < sessions[j].PID
	})
	return Model{
		scanner:  scanner,
		compact:  compact,
		refresh:  refresh,
		sessions: sessions,
		expanded: make(map[int]bool),
		termW:    120,
		termH:    40,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Every(m.refresh, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termW = msg.Width
		m.termH = msg.Height

	case tickMsg:
		m.scanner.LoadAll()
		m.sessions = m.scanner.RunningSessions()
		sort.Slice(m.sessions, func(i, j int) bool {
			return m.sessions[i].PID < m.sessions[j].PID
		})
		count := len(m.sessions)
		if count == 0 {
			m.cursorIdx = 0
		} else if m.cursorIdx >= count {
			m.cursorIdx = count - 1
		}
		return m, tea.Every(m.refresh, func(t time.Time) tea.Msg {
			return tickMsg(t)
		})

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursorIdx > 0 {
				m.cursorIdx--
			}
		case "down", "j":
			if m.cursorIdx < len(m.sessions)-1 {
				m.cursorIdx++
			}
		case "enter", " ":
			if m.cursorIdx < len(m.sessions) {
				pid := m.sessions[m.cursorIdx].PID
				m.expanded[pid] = !m.expanded[pid]
			}
		case "e":
			for _, s := range m.sessions {
				m.expanded[s.PID] = true
			}
		case "c":
			m.expanded = make(map[int]bool)
		case "g", "G":
			if m.cursorIdx < len(m.sessions) {
				s := m.sessions[m.cursorIdx]
				if s.TmuxSession == "" {
					m.statusMsg = "Session not in tmux"
					m.statusExp = time.Now().Add(3 * time.Second)
				} else {
					// Attempt switch-client (works in tmux, no-op in psmux 3.3.2)
					tmux.SwitchToPane(s.TmuxSession)
					// Show navigation hint — needed for psmux where switch-client is broken
					parts := strings.SplitN(s.TmuxSession, "/", 2)
					hint := "Ctrl+B, s"
					if len(parts) == 2 {
						hint = fmt.Sprintf("Ctrl+B, s → select \"%s\" → window \"%s\"", parts[0], parts[1])
					}
					m.statusMsg = fmt.Sprintf("Go to: %s  |  %s", s.TmuxSession, hint)
					m.statusExp = time.Now().Add(10 * time.Second)
				}
			}
		case "q", "Q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string {
	now := time.Now()
	c := computeCols(m.sessions, now, m.termW)
	var b strings.Builder

	// Title line
	title := titleStyle.Render("CLAUDE WATCH")
	subtitle := durationStyle.Render("— monitoring your Claude Code sessions")
	timestamp := durationStyle.Render(now.Format("01/02 15:04:05"))
	b.WriteString(title + " " + subtitle + "  " + timestamp + "\n")

	widths := []int{c.pid, c.project, c.status, c.action, c.dur}
	headers := []string{
		colHeaderStyle.Width(c.pid).Render("PID"),
		colHeaderStyle.Width(c.project).Render("PROJECT"),
		colHeaderStyle.Width(c.status).Render("STATUS"),
		colHeaderStyle.Width(c.action).Render("CURRENT ACTION"),
		colHeaderStyle.Width(c.dur).Render("DURATION"),
	}
	if c.tmux > 0 {
		widths = append(widths[:1], append([]int{c.tmux}, widths[1:]...)...)
		headers = append(headers[:1], append([]string{
			colHeaderStyle.Width(c.tmux).Render("TMUX SESSION/WINDOW"),
		}, headers[1:]...)...)
	}
	tw := totalWidth(widths)
	b.WriteString(hline(tw) + "\n")
	b.WriteString(joinCols(headers) + "\n")
	b.WriteString(hline(tw) + "\n")

	for i, s := range m.sessions {
		isCursor := i == m.cursorIdx
		isExpanded := m.expanded[s.PID]

		b.WriteString(renderRow(s, now, c, isCursor) + "\n")

		if !m.compact && isExpanded {
			prompt := s.LastPrompt
			if prompt == "" {
				prompt = s.OriginalTask
			}
			if prompt != "" {
				prefix := "  » prompt: "
				maxLen := m.termW - len(prefix)
				if maxLen < 1 {
					maxLen = 1
				}
				b.WriteString(
					durationStyle.Render(prefix) +
						promptStyle.MaxWidth(maxLen).Render(prompt) + "\n",
				)
			}
			if s.LastResponse != "" {
				prefix := "  » response: "
				maxLen := m.termW - len(prefix)
				if maxLen < 1 {
					maxLen = 1
				}
				b.WriteString(
					durationStyle.Render(prefix) +
						responseStyle.MaxWidth(maxLen).Render(s.LastResponse) + "\n",
				)
			}
		}

		if i < len(m.sessions)-1 {
			b.WriteString(hline(tw) + "\n")
		}
	}

	if len(m.sessions) == 0 {
		b.WriteString(durationStyle.Render("  No active sessions found. Watching for new sessions...") + "\n")
	}

	// Help bar
	if !m.compact {
		b.WriteString("\n" + hline(tw) + "\n")
		b.WriteString(
			helpKeyStyle.Render("↑↓") + helpTextStyle.Render(" Navigate  ") +
				helpKeyStyle.Render("Enter") + helpTextStyle.Render(" Toggle  ") +
				helpKeyStyle.Render("g") + helpTextStyle.Render(" Go to Window  ") +
				helpKeyStyle.Render("e") + helpTextStyle.Render(" Expand All  ") +
				helpKeyStyle.Render("c") + helpTextStyle.Render(" Collapse All  ") +
				helpKeyStyle.Render("q") + helpTextStyle.Render(" Quit"),
		)
		if m.statusMsg != "" && time.Now().Before(m.statusExp) {
			warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
			b.WriteString("\n" + warnStyle.Render("  "+m.statusMsg))
		}
	}

	return b.String()
}

// Render is a test-friendly wrapper: creates a Model with fixed terminal width
// and returns View(). This keeps test assertions working without a running program.
func Render(sessions []session.State, compact bool) string {
	m := Model{
		sessions: sessions,
		compact:  compact,
		expanded: make(map[int]bool),
		termW:    120,
		termH:    40,
	}
	sort.Slice(m.sessions, func(i, j int) bool {
		return m.sessions[i].PID < m.sessions[j].PID
	})
	return m.View()
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

	// Cursor indicator occupies 2 chars (">" + " "); remaining width goes to PID value.
	var pidCell string
	if isCursor {
		pidCell = cursorStyle.Render(">") + " " + pidStyle.Width(c.pid-2).Render(pidStr)
	} else {
		pidCell = pidStyle.Width(c.pid).Render(pidStr)
	}

	cells := []string{
		pidCell,
		projectStyle.Width(c.project).MaxWidth(c.project).Render(s.ProjectName),
		styledStatus(s.Status, c.status),
		actionStyle.Width(c.action).MaxWidth(c.action).Render(action),
		durationStyle.Width(c.dur).MaxWidth(c.dur).Render(dur),
	}

	if c.tmux > 0 {
		var tmuxCell string
		if s.TmuxSession == "" {
			tmuxCell = durationStyle.Width(c.tmux).MaxWidth(c.tmux).Render("not in tmux")
		} else {
			tmuxCell = tmuxStyle.Width(c.tmux).MaxWidth(c.tmux).Render(s.TmuxSession)
		}
		cells = append(cells[:1], append([]string{tmuxCell}, cells[1:]...)...)
	}

	return joinCols(cells)
}

func styledStatus(status session.Status, width int) string {
	style, ok := statusStyles[status]
	if !ok {
		style = lipgloss.NewStyle()
	}
	return style.Width(width).Render(string(status))
}

func joinCols(cells []string) string {
	sep := separatorStyle.Render(" │ ")
	return strings.Join(cells, sep)
}

func totalWidth(widths []int) int {
	w := 0
	for _, cw := range widths {
		w += cw
	}
	w += (len(widths) - 1) * 3 // " │ " = 3 visible chars each
	return w
}

func hline(width int) string {
	return separatorStyle.Render(strings.Repeat("─", width))
}

// actionForStatus returns the action text appropriate for the session's status.
// Only show the current action when Claude is actively working.
func actionForStatus(s session.State) string {
	switch s.Status {
	case session.StatusThinking:
		return "Thinking..."
	case session.StatusToolUse:
		if s.CurrentAction != "" {
			return s.CurrentAction
		}
		return "Executing tool..."
	case session.StatusStreaming:
		return "Streaming response..."
	case session.StatusResponding:
		if s.CurrentAction != "" {
			return s.CurrentAction
		}
		return "Processing..."
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
	case session.StatusThinking:
		return 0
	case session.StatusToolUse:
		return 0
	case session.StatusStreaming:
		return 0
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
