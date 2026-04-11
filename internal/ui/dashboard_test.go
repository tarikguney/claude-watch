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
	output := Render(nil, RenderOpts{}, 0)
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

	output := Render(sessions, RenderOpts{}, 0)

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

	output := Render(sessions, RenderOpts{Compact: true}, 0)
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

	output := Render(sessions, RenderOpts{}, 0)

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
}

func TestPad(t *testing.T) {
	if result := pad("abc", 5); result != "abc  " {
		t.Errorf("expected 'abc  ', got %q", result)
	}
	if result := pad("abcdefgh", 6); result != "abc..." {
		t.Errorf("expected 'abc...', got %q", result)
	}
	if result := pad("abcd", 3); result != "abc" {
		t.Errorf("expected 'abc' (short width, no ellipsis), got %q", result)
	}
	if result := pad("abc", 3); result != "abc" {
		t.Errorf("expected 'abc', got %q", result)
	}
}
