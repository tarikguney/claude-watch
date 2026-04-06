// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/tarikguney/claude-watch/internal/process"
	"github.com/tarikguney/claude-watch/internal/session"
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

func run(claudeDir string, refresh time.Duration, compact bool, maxAge time.Duration) error {
	// Use alternate screen buffer for clean repainting (like htop/less)
	output.AltScreen()
	defer output.ExitAltScreen()

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

	ticker := time.NewTicker(refresh)
	defer ticker.Stop()

	// Initial process discovery + render
	refreshProcesses(scanner)
	render(scanner, compact, maxAge)

	for range ticker.C {
		scanner.LoadAll()
		refreshProcesses(scanner)
		render(scanner, compact, maxAge)
	}

	return nil
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

func render(scanner *session.Scanner, compact bool, maxAge time.Duration) {
	output.ClearScreen()
	output.MoveCursor(1, 1)
	sessions := scanner.RunningSessions()
	dashboard := ui.Render(sessions, compact, 0)
	fmt.Print(dashboard)
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
