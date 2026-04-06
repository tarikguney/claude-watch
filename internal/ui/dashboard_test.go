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
	output := Render(nil, false, 0)
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
			Status:        session.StatusActive,
			StartTime:     now.Add(-12 * time.Minute),
			LastUpdate:    now,
		},
		{
			ProjectName:   "webapp",
			OriginalTask:  "Fix login page CSS",
			CurrentAction: "Running npm test",
			Status:        session.StatusActive,
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

	output := Render(sessions, false, 0)

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
			Status:        session.StatusActive,
			StartTime:     now.Add(-5 * time.Minute),
			LastUpdate:    now,
		},
	}

	output := Render(sessions, true, 0)
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
			ProjectName: "done-proj",
			Status:      session.StatusDone,
			LastUpdate:  now,
			StartTime:   now.Add(-10 * time.Minute),
		},
		{
			ProjectName: "active-proj",
			Status:      session.StatusActive,
			LastUpdate:  now,
			StartTime:   now.Add(-5 * time.Minute),
		},
		{
			ProjectName: "idle-proj",
			Status:      session.StatusIdle,
			LastUpdate:  now.Add(-6 * time.Minute),
			StartTime:   now.Add(-20 * time.Minute),
		},
	}

	output := Render(sessions, false, 0)

	activeIdx := strings.Index(output, "active-proj")
	idleIdx := strings.Index(output, "idle-proj")
	doneIdx := strings.Index(output, "done-proj")

	if activeIdx > idleIdx || activeIdx > doneIdx {
		t.Error("active sessions should appear first")
	}
	if idleIdx > doneIdx {
		t.Error("idle sessions should appear before done")
	}
}

func TestStatusPriority(t *testing.T) {
	if statusPriority(session.StatusActive) >= statusPriority(session.StatusResponding) {
		t.Error("Active should have higher priority than Responding")
	}
	if statusPriority(session.StatusResponding) >= statusPriority(session.StatusThinking) {
		t.Error("Responding should have higher priority than Thinking")
	}
	if statusPriority(session.StatusThinking) >= statusPriority(session.StatusError) {
		t.Error("Thinking should have higher priority than Error")
	}
	if statusPriority(session.StatusError) >= statusPriority(session.StatusIdle) {
		t.Error("Error should have higher priority than Idle")
	}
	if statusPriority(session.StatusIdle) >= statusPriority(session.StatusDone) {
		t.Error("Idle should have higher priority than Done")
	}
}

func TestPad(t *testing.T) {
	if result := pad("abc", 5); result != "abc  " {
		t.Errorf("expected 'abc  ', got %q", result)
	}
	if result := pad("abcdef", 4); result != "abcd" {
		t.Errorf("expected 'abcd', got %q", result)
	}
	if result := pad("abc", 3); result != "abc" {
		t.Errorf("expected 'abc', got %q", result)
	}
}
