// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/tarikguney/claude-watch/internal/process"
	"github.com/tarikguney/claude-watch/internal/session"
	"github.com/tarikguney/claude-watch/internal/tmux"
	"github.com/tarikguney/claude-watch/internal/ui"
)

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

func run(claudeDir string, refresh time.Duration, compact bool, _ time.Duration) error {
	scanner := session.NewScanner(claudeDir)

	stopLoading := showLoading("Discovering sessions...")
	if err := scanner.Discover(); err != nil {
		stopLoading()
		return fmt.Errorf("discovery failed: %w", err)
	}
	scanner.LoadAll()
	stopLoading()

	watcher, err := session.NewWatcher(scanner)
	if err != nil {
		return fmt.Errorf("watcher init failed: %w", err)
	}
	if err := watcher.Start(); err != nil {
		return fmt.Errorf("watcher start failed: %w", err)
	}
	defer watcher.Stop()

	stopLoading = showLoading("Checking running processes...")
	refreshProcesses(scanner)
	stopLoading()

	// Background process discovery — keeps session PIDs up to date
	// without blocking the UI loop (PowerShell queries are slow on Windows).
	go func() {
		procTicker := time.NewTicker(2 * time.Second)
		defer procTicker.Stop()
		for range procTicker.C {
			refreshProcesses(scanner)
		}
	}()

	m := ui.NewModel(scanner, compact, refresh)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// refreshProcesses discovers running Claude processes and matches them to sessions.
func refreshProcesses(scanner *session.Scanner) {
	procs, err := process.ListClaude()
	if err != nil {
		return // silently ignore process discovery errors
	}
	// Query tmux/psmux pane mapping (nil if not available)
	paneMap := tmux.ListPanes()
	scanner.MatchProcesses(procs, paneMap)
	// Load any newly discovered sessions from process matching
	scanner.LoadAll()
}

// showLoading displays an animated spinner with a message on stdout.
// It returns a stop function that halts the animation.
func showLoading(msg string) func() {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	style := termenv.Style{}.Foreground(output.Color("#D4A0FF"))
	var once sync.Once
	done := make(chan struct{})
	go func() {
		i := 0
		for {
			select {
			case <-done:
				return
			default:
				frame := style.Styled(frames[i%len(frames)])
				fmt.Fprintf(os.Stdout, "\x1b[H\x1b[2K  %s %s", frame, msg)
				i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
	return func() {
		once.Do(func() { close(done) })
	}
}

var output = termenv.NewOutput(os.Stdout)

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
