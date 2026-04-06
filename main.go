// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/tarikguney/claude-watch/internal/session"
	"github.com/tarikguney/claude-watch/internal/ui"
)

func main() {
	var refresh time.Duration
	var claudeDir string
	var compact bool

	rootCmd := &cobra.Command{
		Use:   "claude-watch",
		Short: "Monitor Claude Code sessions in real time",
		Long: `A zero-setup CLI dashboard for monitoring Claude Code agents.
Discovers sessions automatically from ~/.claude/projects/.

Source: https://github.com/tarikguney/claude-watch`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(claudeDir, refresh, compact)
		},
	}

	rootCmd.Flags().DurationVar(&refresh, "refresh", 2*time.Second, "Dashboard refresh interval")
	rootCmd.Flags().StringVar(&claudeDir, "claude-dir", defaultClaudeDir(), "Path to Claude config directory")
	rootCmd.Flags().BoolVar(&compact, "compact", false, "Compact mode for narrow terminals")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(claudeDir string, refresh time.Duration, compact bool) error {
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

	// Initial render
	render(scanner, compact)

	for range ticker.C {
		// Re-discover periodically to pick up new sessions
		scanner.Discover()
		scanner.LoadAll()
		render(scanner, compact)
	}

	return nil
}

func render(scanner *session.Scanner, compact bool) {
	// Clear screen and move cursor to top
	fmt.Print("\033[H\033[2J")
	sessions := scanner.Sessions()
	output := ui.Render(sessions, compact)
	fmt.Print(output)
}

func defaultClaudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", ".claude")
	}
	return filepath.Join(home, ".claude")
}
