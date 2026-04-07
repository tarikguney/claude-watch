// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/muesli/termenv"
	xterm "golang.org/x/term"
	"github.com/spf13/cobra"
	"github.com/tarikguney/claude-watch/internal/process"
	"github.com/tarikguney/claude-watch/internal/session"
	"github.com/tarikguney/claude-watch/internal/ui"
)

type keyEvent int

const (
	keyUp keyEvent = iota
	keyDown
	keyQuit
	keyToggle
	keyExpandAll
	keyCollapseAll
)

func readKeys(input *os.File, ch chan<- keyEvent) {
	for {
		var b [1]byte
		if _, err := input.Read(b[:]); err != nil {
			return
		}
		switch b[0] {
		case 'q', 'Q', 3: // q, Q, Ctrl+C
			ch <- keyQuit
			return
		case 0x0d, 0x20: // Enter, Space
			ch <- keyToggle
		case 'e':
			ch <- keyExpandAll
		case 'c':
			ch <- keyCollapseAll
		case 0x1b: // Escape — read the rest of the sequence
			var seq [2]byte
			if _, err := input.Read(seq[:]); err != nil {
				return
			}
			if seq[0] == '[' {
				switch seq[1] {
				case 'A':
					ch <- keyUp
				case 'B':
					ch <- keyDown
				}
			}
		}
	}
}

// setupRawInput puts the terminal in raw mode for keyboard input.
// Tries stdin first, then CONIN$ on Windows (for mintty/Git Bash).
func setupRawInput() (input *os.File, cleanup func(), err error) {
	// Try stdin first
	fd := int(os.Stdin.Fd())
	if xterm.IsTerminal(fd) {
		oldState, err := xterm.MakeRaw(fd)
		if err == nil {
			return os.Stdin, func() { xterm.Restore(fd, oldState) }, nil
		}
	}

	// On Windows, try CONIN$ (bypasses mintty pipe redirection)
	if runtime.GOOS == "windows" {
		f, ferr := os.Open("CONIN$")
		if ferr != nil {
			return nil, nil, fmt.Errorf("stdin not a terminal, CONIN$ open failed: %w", ferr)
		}
		cfd := int(f.Fd())
		oldState, err := xterm.MakeRaw(cfd)
		if err != nil {
			f.Close()
			return nil, nil, fmt.Errorf("CONIN$ MakeRaw failed: %w", err)
		}
		return f, func() {
			xterm.Restore(cfd, oldState)
			f.Close()
		}, nil
	}

	return nil, nil, fmt.Errorf("stdin is not a terminal")
}

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var refresh time.Duration
	var claudeDir string
	var compact bool
	var maxAge time.Duration

	rootCmd := &cobra.Command{
		Use:     "claude-watch",
		Short:   "Monitor Claude Code sessions in real time",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
		Long: `A zero-setup CLI dashboard for monitoring Claude Code agents.
Discovers sessions automatically from ~/.claude/projects/.

Source: https://github.com/tarikguney/claude-watch`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(claudeDir, refresh, compact, maxAge)
		},
	}

	rootCmd.Flags().DurationVar(&refresh, "refresh", 1*time.Second, "Dashboard refresh interval")
	rootCmd.Flags().StringVar(&claudeDir, "claude-dir", defaultClaudeDir(), "Path to Claude config directory")
	rootCmd.Flags().BoolVar(&compact, "compact", false, "Compact mode for narrow terminals")
	rootCmd.Flags().DurationVar(&maxAge, "max-age", 4*time.Hour, "Only show sessions modified within this duration (0 for all)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(claudeDir string, refresh time.Duration, compact bool, maxAge time.Duration) error {
	// Use alternate screen buffer for clean repainting (like htop/less)
	output.AltScreen()
	defer output.ExitAltScreen()

	// Raw mode for keyboard input (scrolling)
	inputFile, rawCleanup, rawErr := setupRawInput()
	scrollEnabled := rawErr == nil
	if scrollEnabled {
		defer rawCleanup()
	} else {
		fmt.Fprintf(os.Stderr, "Warning: keyboard input unavailable (%v)\n", rawErr)
	}

	scanner := session.NewScanner(claudeDir)

	if err := scanner.Discover(); err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}
	scanner.LoadAll()

	watcher, err := session.NewWatcher(scanner)
	if err != nil {
		return fmt.Errorf("watcher init failed: %w", err)
	}
	if err := watcher.Start(); err != nil {
		return fmt.Errorf("watcher start failed: %w", err)
	}
	defer watcher.Stop()

	keys := make(chan keyEvent, 10)
	if scrollEnabled {
		go readKeys(inputFile, keys)
	}

	var scrollOffset int
	cursorIdx := 0
	expanded := make(map[int]bool) // default collapsed; tracks explicitly expanded sessions

	ticker := time.NewTicker(refresh)
	defer ticker.Stop()

	// Background process discovery — keeps session PIDs up to date
	// without blocking the UI loop (PowerShell queries are slow on Windows).
	refreshProcesses(scanner)
	go func() {
		procTicker := time.NewTicker(2 * time.Second)
		defer procTicker.Stop()
		for range procTicker.C {
			refreshProcesses(scanner)
		}
	}()

	doRender := func() {
		sessions := scanner.RunningSessions()
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].PID < sessions[j].PID
		})
		if cursorIdx >= len(sessions) {
			cursorIdx = len(sessions) - 1
		}
		if cursorIdx < 0 {
			cursorIdx = 0
		}
		cursorPID := 0
		if len(sessions) > 0 {
			cursorPID = sessions[cursorIdx].PID
		}
		opts := ui.RenderOpts{
			Compact:   compact,
			CursorPID: cursorPID,
			Expanded: expanded,
		}
		renderWithScroll(sessions, opts, &scrollOffset, cursorPID)
	}
	doRender()

	for {
		select {
		case <-ticker.C:
			scanner.LoadAll()
			doRender()
		case key := <-keys:
			sessions := scanner.RunningSessions()
			count := len(sessions)
			sort.Slice(sessions, func(i, j int) bool {
				return sessions[i].PID < sessions[j].PID
			})
			handleKey := func(k keyEvent) bool {
				switch k {
				case keyUp:
					if cursorIdx > 0 {
						cursorIdx--
					}
				case keyDown:
					if cursorIdx < count-1 {
						cursorIdx++
					}
				case keyToggle:
					if cursorIdx < count {
						pid := sessions[cursorIdx].PID
						expanded[pid] = !expanded[pid]
					}
				case keyExpandAll:
					for _, s := range sessions {
						expanded[s.PID] = true
					}
				case keyCollapseAll:
					for k := range expanded {
						delete(expanded, k)
					}
				case keyQuit:
					return true
				}
				return false
			}
			if handleKey(key) {
				return nil
			}
			// Drain queued keys before rendering
		drain:
			for {
				select {
				case k := <-keys:
					if handleKey(k) {
						return nil
					}
				default:
					break drain
				}
			}
			doRender()
		}
	}
}

// refreshProcesses discovers running Claude processes and matches them to sessions.
func refreshProcesses(scanner *session.Scanner) {
	procs, err := process.ListClaude()
	if err != nil {
		return // silently ignore process discovery errors
	}
	scanner.MatchProcesses(procs)
	// Load any newly discovered sessions from process matching
	scanner.LoadAll()
}

var output = termenv.NewOutput(os.Stdout)

const headerLines = 4 // title, separator, column headers, separator

func renderWithScroll(sessions []session.State, opts ui.RenderOpts, scrollOffset *int, cursorPID int) {
	dashboard := ui.Render(sessions, opts, 0)

	lines := strings.Split(dashboard, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	_, termH, err := term.GetSize(os.Stdout.Fd())
	if err != nil || termH <= 0 {
		termH = 40
	}

	// Build entire frame in a buffer, then write once (avoids per-line syscalls)
	var buf strings.Builder
	buf.WriteString("\x1b[2J\x1b[H") // clear screen + cursor home

	if len(lines) <= termH {
		*scrollOffset = 0
		buf.WriteString(dashboard)
		os.Stdout.WriteString(buf.String())
		return
	}

	contentLines := lines[headerLines:]
	maxVisible := termH - headerLines - 1

	// Auto-scroll to keep cursor visible
	if cursorPID > 0 {
		pidStr := fmt.Sprintf("%d", cursorPID)
		for i, line := range contentLines {
			if strings.Contains(line, pidStr) {
				if i < *scrollOffset {
					*scrollOffset = i
				} else if i >= *scrollOffset+maxVisible {
					*scrollOffset = i - maxVisible + 1
				}
				break
			}
		}
	}

	// Clamp
	if max := len(contentLines) - maxVisible; max > 0 {
		if *scrollOffset > max {
			*scrollOffset = max
		}
	} else {
		*scrollOffset = 0
	}
	if *scrollOffset < 0 {
		*scrollOffset = 0
	}

	// Header
	for i := 0; i < headerLines && i < len(lines); i++ {
		buf.WriteString(lines[i])
		buf.WriteString("\r\n")
	}

	// Visible content
	end := *scrollOffset + maxVisible
	if end > len(contentLines) {
		end = len(contentLines)
	}
	for i := *scrollOffset; i < end; i++ {
		buf.WriteString(contentLines[i])
		buf.WriteString("\r\n")
	}

	// Scroll indicator
	if *scrollOffset > 0 {
		buf.WriteString("↑ ")
	}
	if end < len(contentLines) {
		buf.WriteString("↓ ")
	}
	if *scrollOffset > 0 || end < len(contentLines) {
		buf.WriteString("scroll")
	}

	os.Stdout.WriteString(buf.String())
}

func defaultClaudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", ".claude")
	}

	// On Windows, check %APPDATA%\.claude first, fall back to %USERPROFILE%\.claude
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			candidate := filepath.Join(appData, ".claude")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate
			}
		}
	}

	return filepath.Join(home, ".claude")
}
