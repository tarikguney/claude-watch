// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package session

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tarikguney/claude-watch/internal/parser"
)

func TestDeriveStatus_Result(t *testing.T) {
	rec := parser.Record{Type: "result", Timestamp: time.Now().Format(time.RFC3339Nano)}
	status := DeriveStatus(rec, false, time.Now(), false, false)
	if status != StatusDone {
		t.Errorf("expected Done, got %s", status)
	}
}

func TestDeriveStatus_Error(t *testing.T) {
	rec := parser.Record{
		Type:      "assistant",
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Message:   json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"oops"}]}`),
	}
	status := DeriveStatus(rec, true, time.Now(), false, false)
	if status != StatusError {
		t.Errorf("expected Error, got %s", status)
	}
}

func TestDeriveStatus_ToolUse(t *testing.T) {
	rec := parser.Record{
		Type:      "assistant",
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Message:   json.RawMessage(`{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"x.go"}}]}`),
	}
	status := DeriveStatus(rec, false, time.Now(), false, false)
	if status != StatusResponding {
		t.Errorf("expected Responding, got %s", status)
	}
}

func TestDeriveStatus_AssistantTextOnly_NoProcess(t *testing.T) {
	rec := parser.Record{
		Type:      "assistant",
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Message:   json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"Here is my answer"}]}`),
	}
	status := DeriveStatus(rec, false, time.Now(), false, false)
	if status != StatusIdle {
		t.Errorf("expected Idle, got %s", status)
	}
}

func TestDeriveStatus_AssistantTextOnly_WithProcess(t *testing.T) {
	rec := parser.Record{
		Type:      "assistant",
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Message:   json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"Here is my answer"}]}`),
	}
	// Text-only assistant is Idle even with a running process — we can't distinguish
	// "mid-stream" from "done, waiting for input" and the steady state is Idle.
	status := DeriveStatus(rec, false, time.Now(), true, false)
	if status != StatusIdle {
		t.Errorf("expected Idle, got %s", status)
	}
}

func TestDeriveStatus_ThinkingBlock(t *testing.T) {
	rec := parser.Record{
		Type:      "assistant",
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Message:   json.RawMessage(`{"role":"assistant","content":[{"type":"thinking","thinking":""}]}`),
	}
	status := DeriveStatus(rec, false, time.Now(), true, false)
	if status != StatusResponding {
		t.Errorf("expected Responding, got %s", status)
	}
}

func TestDeriveStatus_UserPrompt(t *testing.T) {
	rec := parser.Record{
		Type:      "user",
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Message:   json.RawMessage(`{"role":"user","content":"do something"}`),
	}
	status := DeriveStatus(rec, false, time.Now(), false, false)
	if status != StatusResponding {
		t.Errorf("expected Responding, got %s", status)
	}
}

func TestDeriveStatus_ToolResult_WithIO(t *testing.T) {
	rec := parser.Record{
		Type:      "user",
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Message:   json.RawMessage(`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"file contents"}]}`),
	}
	// Tool result with IO activity — Claude is processing the output
	status := DeriveStatus(rec, false, time.Now(), true, true)
	if status != StatusResponding {
		t.Errorf("expected Responding for tool_result with IO, got %s", status)
	}
}

func TestDeriveStatus_ToolResult_WithProcess_NoIO(t *testing.T) {
	rec := parser.Record{
		Type:      "user",
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Message:   json.RawMessage(`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"file contents"}]}`),
	}
	// Tool result with process but no IO — system-injected, Claude is idle
	status := DeriveStatus(rec, false, time.Now(), true, false)
	if status != StatusIdle {
		t.Errorf("expected Idle for tool_result without IO, got %s", status)
	}
}

func TestDeriveStatus_ToolResult_NoProcess(t *testing.T) {
	rec := parser.Record{
		Type:      "user",
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Message:   json.RawMessage(`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"file contents"}]}`),
	}
	// Tool result without running process — stale session
	status := DeriveStatus(rec, false, time.Now(), false, false)
	if status != StatusIdle {
		t.Errorf("expected Idle for tool_result without process, got %s", status)
	}
}

func TestDeriveStatus_Idle_OldTimestamp(t *testing.T) {
	oldTime := time.Now().Add(-10 * time.Minute)
	rec := parser.Record{
		Type:      "assistant",
		Timestamp: oldTime.Format(time.RFC3339Nano),
		Message:   json.RawMessage(`{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{}}]}`),
	}
	status := DeriveStatus(rec, false, time.Now(), false, false)
	if status != StatusIdle {
		t.Errorf("expected Idle, got %s", status)
	}
}

func TestDeriveStatus_ResultTakesPriority(t *testing.T) {
	rec := parser.Record{Type: "result", Timestamp: time.Now().Add(-10 * time.Minute).Format(time.RFC3339Nano)}
	status := DeriveStatus(rec, true, time.Now(), false, false)
	if status != StatusDone {
		t.Errorf("expected Done (result type takes priority), got %s", status)
	}
}

func TestFormatToolAction(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		input    parser.ToolInput
		expected string
	}{
		{"Read", "Read", parser.ToolInput{FilePath: "src/auth.ts"}, "Reading auth.ts"},
		{"Edit", "Edit", parser.ToolInput{FilePath: "src/main.go"}, "Editing main.go"},
		{"Write", "Write", parser.ToolInput{FilePath: "/tmp/out.txt"}, "Writing out.txt"},
		{"Bash", "Bash", parser.ToolInput{Command: "npm test"}, "Running npm test"},
		{"Grep", "Grep", parser.ToolInput{Pattern: "validateToken"}, "Searching for validateToken"},
		{"Glob", "Glob", parser.ToolInput{Pattern: "*.test.ts"}, "Finding *.test.ts"},
		{"Task", "Task", parser.ToolInput{Description: "explore auth flow"}, "Subagent: explore auth flow"},
		{"WebSearch", "WebSearch", parser.ToolInput{Query: "JWT best practices"}, "Searching: JWT best practices"},
		{"WebFetch", "WebFetch", parser.ToolInput{URL: "https://docs.example.com"}, "Fetching https://docs.example.com"},
		{"Unknown", "CustomTool", parser.ToolInput{}, "Using CustomTool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatToolAction(tt.tool, tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestEncodeProjectDir(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`Q:\sources\ci-seal-phase-2`, "Q--sources-ci-seal-phase-2"},
		{`C:\Users\abguney\tools\claude-watch`, "C--Users-abguney-tools-claude-watch"},
		{`C:\Users\abguney`, "C--Users-abguney"},
		{"Q:/sources/cs-master", "Q--sources-cs-master"},
		{"/home/user/myapp", "-home-user-myapp"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := EncodeProjectDir(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}


func TestExtractOriginalTask(t *testing.T) {
	records := []parser.Record{
		{Type: "system", Message: json.RawMessage(`{"role":"system","content":"system prompt"}`)},
		{Type: "user", Message: json.RawMessage(`{"role":"user","content":"Add auth to API endpoints"}`)},
		{Type: "assistant", Message: json.RawMessage(`{"role":"assistant","content":"Sure, I'll help"}`)},
	}
	task := ExtractOriginalTask(records)
	if task != "Add auth to API endpoints" {
		t.Errorf("expected 'Add auth to API endpoints', got %q", task)
	}
}

func TestExtractOriginalTask_NoUserRecord(t *testing.T) {
	records := []parser.Record{
		{Type: "system", Message: json.RawMessage(`{"role":"system","content":"system prompt"}`)},
	}
	task := ExtractOriginalTask(records)
	if task != "" {
		t.Errorf("expected empty task, got %q", task)
	}
}

func TestExtractModel(t *testing.T) {
	records := []parser.Record{
		{Type: "assistant", Message: json.RawMessage(`{"role":"assistant","model":"claude-3-opus","content":"hi"}`)},
		{Type: "assistant", Message: json.RawMessage(`{"role":"assistant","model":"claude-3.5-sonnet","content":"hello"}`)},
	}
	model := ExtractModel(records)
	if model != "claude-3.5-sonnet" {
		t.Errorf("expected 'claude-3.5-sonnet', got %q", model)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{12 * time.Minute, "12m"},
		{1*time.Hour + 5*time.Minute, "1h05m"},
		{2*time.Hour + 30*time.Minute, "2h30m"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatDuration(tt.d)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractLastToolAction(t *testing.T) {
	rec := parser.Record{
		Type: "assistant",
		Message: json.RawMessage(`{"role":"assistant","content":[
			{"type":"text","text":"I'll read the file"},
			{"type":"tool_use","name":"Read","input":{"file_path":"src/main.go"}},
			{"type":"tool_use","name":"Edit","input":{"file_path":"src/auth.ts"}}
		]}`),
	}
	action := ExtractLastToolAction(rec)
	if action != "Editing auth.ts" {
		t.Errorf("expected 'Editing auth.ts', got %q", action)
	}
}

func TestExtractLastToolAction_NoToolUse(t *testing.T) {
	rec := parser.Record{
		Type:    "assistant",
		Message: json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"Just text"}]}`),
	}
	action := ExtractLastToolAction(rec)
	if action != "" {
		t.Errorf("expected empty action, got %q", action)
	}
}

func TestCheckLastToolResultError_True(t *testing.T) {
	records := []parser.Record{
		{
			Type:    "user",
			Message: json.RawMessage(`{"role":"user","content":[{"type":"tool_result","is_error":true}]}`),
		},
	}
	if !CheckLastToolResultError(records) {
		t.Error("expected true for tool_result with is_error: true")
	}
}

func TestCheckLastToolResultError_False(t *testing.T) {
	records := []parser.Record{
		{
			Type:    "user",
			Message: json.RawMessage(`{"role":"user","content":[{"type":"tool_result","is_error":false}]}`),
		},
	}
	if CheckLastToolResultError(records) {
		t.Error("expected false for tool_result with is_error: false")
	}
}

func TestCheckLastToolResultError_NoToolResult(t *testing.T) {
	records := []parser.Record{
		{
			Type:    "assistant",
			Message: json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"hello"}]}`),
		},
	}
	if CheckLastToolResultError(records) {
		t.Error("expected false when no tool_result")
	}
}
