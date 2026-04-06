// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package session

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tarikguney/claude-watch/internal/parser"
)

type Status string

const (
	StatusActive     Status = "Tool Use"
	StatusResponding Status = "Responding"
	StatusThinking   Status = "Thinking"
	StatusIdle       Status = "Idle"
	StatusDone       Status = "Done"
	StatusError      Status = "Error"
)

// State holds the derived state for a single Claude Code session.
type State struct {
	SessionID   string
	PID         int // OS process ID if the session has a running claude process
	FilePath    string
	ProjectName string
	Cwd         string
	OriginalTask string
	LastPrompt  string
	CurrentAction string
	Status      Status
	Model       string
	StartTime   time.Time
	LastUpdate  time.Time
	FileOffset  int64
	FileModTime time.Time

	// Cached from last record for re-deriving status after PID changes
	LastRecordType      string
	LastRecordTimestamp  string
	LastToolResultError bool
	LastHasToolUse      bool
}

// idleThreshold is the duration after which a session with no result is considered Idle.
const idleThreshold = 5 * time.Minute

// activeThreshold is the max age of the last record to still be considered Active/Responding/Thinking.
const activeThreshold = 2 * time.Minute

// DeriveStatus computes the session status from the last record and its timestamp.
// processRunning indicates whether the session has a live OS process (PID > 0).
func DeriveStatus(rec parser.Record, lastToolResultIsError bool, now time.Time, processRunning bool) Status {
	recTime, err := time.Parse(time.RFC3339Nano, rec.Timestamp)
	if err != nil {
		recTime = time.Now()
	}
	age := now.Sub(recTime)

	if rec.Type == "result" {
		return StatusDone
	}

	// Only mark Idle from timestamp age if there's no running process.
	// A running process means Claude is alive — it just hasn't written to the file recently.
	if age > idleThreshold && !processRunning {
		return StatusIdle
	}

	if lastToolResultIsError {
		return StatusError
	}

	switch rec.Type {
	case "assistant":
		mc, err := parser.ParseMessageContent(rec)
		if err != nil {
			return StatusIdle
		}
		blocks, err := parser.ParseContentBlocks(mc)
		if err != nil {
			return StatusIdle
		}
		hasToolUse := false
		for _, b := range blocks {
			if b.Type == "tool_use" {
				hasToolUse = true
				break
			}
		}
		if hasToolUse {
			return StatusActive
		}
		// Text-only assistant message — Claude finished and is waiting for input
		return StatusIdle

	case "user":
		// User sent a message — Claude is thinking
		if age < activeThreshold || processRunning {
			return StatusThinking
		}
		return StatusIdle
	}

	return StatusIdle
}

// FormatToolAction produces a human-readable one-liner from a tool_use content block.
func FormatToolAction(toolName string, input parser.ToolInput) string {
	switch toolName {
	case "Read":
		return fmt.Sprintf("Reading %s", shortenPath(input.FilePath))
	case "Edit":
		return fmt.Sprintf("Editing %s", shortenPath(input.FilePath))
	case "Write":
		return fmt.Sprintf("Writing %s", shortenPath(input.FilePath))
	case "Bash":
		return fmt.Sprintf("Running %s", truncate(input.Command, 40))
	case "Grep":
		return fmt.Sprintf("Searching for %s", truncate(input.Pattern, 30))
	case "Glob":
		return fmt.Sprintf("Finding %s", truncate(input.Pattern, 30))
	case "Task":
		return fmt.Sprintf("Subagent: %s", truncate(input.Description, 35))
	case "WebSearch":
		return fmt.Sprintf("Searching: %s", truncate(input.Query, 35))
	case "WebFetch":
		return fmt.Sprintf("Fetching %s", truncate(input.URL, 40))
	default:
		return fmt.Sprintf("Using %s", toolName)
	}
}

// ExtractLastToolAction finds the last tool_use block in a record and returns a display string.
func ExtractLastToolAction(rec parser.Record) string {
	mc, err := parser.ParseMessageContent(rec)
	if err != nil {
		return ""
	}
	blocks, err := parser.ParseContentBlocks(mc)
	if err != nil {
		return ""
	}

	var lastToolName string
	var lastToolInput parser.ToolInput
	for _, b := range blocks {
		if b.Type == "tool_use" {
			lastToolName = b.Name
			ti, err := parser.ParseToolInput(b.Input)
			if err == nil {
				lastToolInput = ti
			}
		}
	}

	if lastToolName == "" {
		return ""
	}
	return FormatToolAction(lastToolName, lastToolInput)
}

// ExtractProjectName decodes a project name from the URL-encoded directory path.
// e.g., "-Users-tarik-myapp" -> "myapp"
func ExtractProjectName(encodedPath string) string {
	parts := strings.Split(encodedPath, "-")
	if len(parts) == 0 {
		return encodedPath
	}
	// Return the last non-empty segment
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return encodedPath
}

// ExtractOriginalTask finds the first real user message text, truncated.
// Skips system-injected messages (XML tags, command wrappers, etc.).
func ExtractOriginalTask(records []parser.Record) string {
	for _, rec := range records {
		if rec.Type != "user" {
			continue
		}
		mc, err := parser.ParseMessageContent(rec)
		if err != nil {
			continue
		}
		blocks, err := parser.ParseContentBlocks(mc)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				trimmed := strings.TrimSpace(b.Text)
				// Skip system-injected content (XML tags, command wrappers)
				if strings.HasPrefix(trimmed, "<") {
					continue
				}
				return truncate(trimmed, 50)
			}
		}
	}
	return ""
}

// ExtractLastPrompt finds the last real user message, returning the first few words.
func ExtractLastPrompt(records []parser.Record) string {
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Type != "user" {
			continue
		}
		mc, err := parser.ParseMessageContent(records[i])
		if err != nil {
			continue
		}
		blocks, err := parser.ParseContentBlocks(mc)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				trimmed := strings.TrimSpace(b.Text)
				if strings.HasPrefix(trimmed, "<") {
					continue
				}
				return truncate(trimmed, 60)
			}
		}
	}
	return ""
}

// ExtractModel returns the model string from assistant records.
func ExtractModel(records []parser.Record) string {
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Type != "assistant" {
			continue
		}
		mc, err := parser.ParseMessageContent(records[i])
		if err != nil {
			continue
		}
		if mc.Model != "" {
			return mc.Model
		}
	}
	return ""
}

// FormatDuration returns a human-friendly short duration string (e.g., "12m", "1h05m").
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%02dm", h, m)
}

// CheckLastToolResultError scans recent records for a tool_result with is_error: true.
// In Claude JSONL, tool_result blocks appear in content arrays and have an is_error field.
func CheckLastToolResultError(records []parser.Record) bool {
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		mc, err := parser.ParseMessageContent(rec)
		if err != nil {
			continue
		}

		// Parse content as raw JSON array to check for tool_result with is_error
		var rawBlocks []json.RawMessage
		if err := json.Unmarshal(mc.Content, &rawBlocks); err != nil {
			continue
		}

		for _, raw := range rawBlocks {
			var block struct {
				Type    string `json:"type"`
				IsError bool   `json:"is_error"`
			}
			if err := json.Unmarshal(raw, &block); err != nil {
				continue
			}
			if block.Type == "tool_result" && block.IsError {
				return true
			}
		}

		// Only check the last record with parseable content
		if len(rawBlocks) > 0 {
			break
		}
	}
	return false
}

func shortenPath(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Base(p)
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}
