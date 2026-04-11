// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/tarikguney/claude-watch/internal/session"
)

func TestRender_EmptySessions(t *testing.T) {
	output := Render(nil, false)
	if !strings.Contains(output, "CLAUDE WATCH") {
		t.Error("expected header 'CLAUDE WATCH'")
	}
	if !strings.Contains(output, "No active sessions found") {
		t.Error("expected empty state message")
	}
}

func TestRender_WithSessions(t *testing.T) {
	now := time.Now()
	sessions := []session.State{
		{
			ProjectName:   "myapp",
			OriginalTask:  "Add auth to API endpoints",
			CurrentAction: "Editing src/middleware.ts",
			Status:        session.StatusResponding,
			StartTime:     now.Add(-12 * time.Minute),
			LastUpdate:    now,
		},
		{
			ProjectName:   "webapp",
			OriginalTask:  "Fix login page CSS",
			CurrentAction: "Running npm test",
			Status:        session.StatusResponding,
			StartTime:     now.Add(-5 * time.Minute),
			LastUpdate:    now,
		},
		{
			ProjectName:   "cli-tool",
			OriginalTask:  "Add --verbose flag",
			CurrentAction: "",
			Status:        session.StatusDone,
			StartTime:     now.Add(-8 * time.Minute),
			LastUpdate:    now.Add(-2 * time.Minute),
		},
	}

	output := Render(sessions, false)

	if !strings.Contains(output, "CLAUDE WATCH") {
		t.Error("expected header")
	}
	if !strings.Contains(output, "PROJECT") {
		t.Error("expected column header PROJECT")
	}
	if !strings.Contains(output, "myapp") {
		t.Error("expected project name 'myapp'")
	}
	if !strings.Contains(output, "webapp") {
		t.Error("expected project name 'webapp'")
	}
	if !strings.Contains(output, "cli-tool") {
		t.Error("expected project name 'cli-tool'")
	}
	if !strings.Contains(output, "Completed") {
		t.Error("expected 'Completed' for done session")
	}
}

func TestRender_Compact(t *testing.T) {
	now := time.Now()
	sessions := []session.State{
		{
			ProjectName:   "myapp",
			OriginalTask:  "Add auth",
			CurrentAction: "Editing main.go",
			Status:        session.StatusResponding,
			StartTime:     now.Add(-5 * time.Minute),
			LastUpdate:    now,
		},
	}

	output := Render(sessions, true)
	if !strings.Contains(output, "CLAUDE WATCH") {
		t.Error("expected header in compact mode")
	}
	// Compact mode should NOT have TASK column
	if strings.Contains(output, "TASK") {
		t.Error("compact mode should not show TASK column")
	}
	if !strings.Contains(output, "myapp") {
		t.Error("expected project name in compact mode")
	}
}

func TestRender_SortOrder(t *testing.T) {
	now := time.Now()
	sessions := []session.State{
		{
			ProjectName: "high-pid",
			PID:         300,
			Status:      session.StatusResponding,
			LastUpdate:  now,
			StartTime:   now.Add(-5 * time.Minute),
		},
		{
			ProjectName: "low-pid",
			PID:         100,
			Status:      session.StatusIdle,
			LastUpdate:  now,
			StartTime:   now.Add(-10 * time.Minute),
		},
		{
			ProjectName: "mid-pid",
			PID:         200,
			Status:      session.StatusDone,
			LastUpdate:  now,
			StartTime:   now.Add(-20 * time.Minute),
		},
	}

	output := Render(sessions, false)

	lowIdx := strings.Index(output, "low-pid")
	midIdx := strings.Index(output, "mid-pid")
	highIdx := strings.Index(output, "high-pid")

	if lowIdx > midIdx || midIdx > highIdx {
		t.Error("sessions should be sorted by PID ascending")
	}
}

func TestStatusPriority(t *testing.T) {
	if statusPriority(session.StatusResponding) >= statusPriority(session.StatusError) {
		t.Error("Responding should have higher priority than Error")
	}
	if statusPriority(session.StatusError) >= statusPriority(session.StatusIdle) {
		t.Error("Error should have higher priority than Idle")
	}
	if statusPriority(session.StatusIdle) >= statusPriority(session.StatusDone) {
		t.Error("Idle should have higher priority than Done")
	}
	// New statuses should have same priority as Responding
	if statusPriority(session.StatusThinking) != statusPriority(session.StatusResponding) {
		t.Error("Thinking should have same priority as Responding")
	}
	if statusPriority(session.StatusToolUse) != statusPriority(session.StatusResponding) {
		t.Error("ToolUse should have same priority as Responding")
	}
	if statusPriority(session.StatusStreaming) != statusPriority(session.StatusResponding) {
		t.Error("Streaming should have same priority as Responding")
	}
}

func TestActionForStatus_NewStatuses(t *testing.T) {
	tests := []struct {
		name     string
		state    session.State
		expected string
	}{
		{
			name:     "Thinking shows Thinking...",
			state:    session.State{Status: session.StatusThinking},
			expected: "Thinking...",
		},
		{
			name:     "ToolUse with action shows action",
			state:    session.State{Status: session.StatusToolUse, CurrentAction: "Reading main.go"},
			expected: "Reading main.go",
		},
		{
			name:     "ToolUse without action shows default",
			state:    session.State{Status: session.StatusToolUse},
			expected: "Executing tool...",
		},
		{
			name:     "Streaming shows Streaming response...",
			state:    session.State{Status: session.StatusStreaming},
			expected: "Streaming response...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := actionForStatus(tt.state)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestRender_NewStatuses(t *testing.T) {
	now := time.Now()
	sessions := []session.State{
		{
			PID:           101,
			ProjectName:   "thinking-project",
			Status:        session.StatusThinking,
			StartTime:     now.Add(-1 * time.Minute),
			LastUpdate:    now,
		},
		{
			PID:           102,
			ProjectName:   "tool-project",
			CurrentAction: "Editing auth.go",
			Status:        session.StatusToolUse,
			StartTime:     now.Add(-2 * time.Minute),
			LastUpdate:    now,
		},
		{
			PID:           103,
			ProjectName:   "stream-project",
			Status:        session.StatusStreaming,
			StartTime:     now.Add(-3 * time.Minute),
			LastUpdate:    now,
		},
	}

	output := Render(sessions, false)

	if !strings.Contains(output, "Thinking") {
		t.Error("expected 'Thinking' status in output")
	}
	if !strings.Contains(output, "Tool Use") {
		t.Error("expected 'Tool Use' status in output")
	}
	if !strings.Contains(output, "Streaming") {
		t.Error("expected 'Streaming' status in output")
	}
	if !strings.Contains(output, "Thinking...") {
		t.Error("expected 'Thinking...' action text")
	}
	if !strings.Contains(output, "Editing auth.go") {
		t.Error("expected tool action text")
	}
	if !strings.Contains(output, "Streaming response...") {
		t.Error("expected 'Streaming response...' action text")
	}
}
