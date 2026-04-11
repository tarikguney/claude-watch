package session

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/tarikguney/claude-watch/internal/parser"
)

func TestDeriveStatus_AllScenarios(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		rec     parser.Record
		isError bool
		running bool
		want    Status
	}{
		{
			name: "system-injected + running",
			rec: parser.Record{
				Type:      "user",
				Timestamp: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"user","content":"<local-command-stdout>Copied to clipboard</local-command-stdout>"}`),
			},
			running: true,
			want:    StatusIdle,
		},
		{
			name: "fresh real user prompt + no process",
			rec: parser.Record{
				Type:      "user",
				Timestamp: now.Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"user","content":"fix the bug"}`),
			},
			running: false,
			want:    StatusResponding,
		},
		{
			name: "old real user prompt + running",
			rec: parser.Record{
				Type:      "user",
				Timestamp: now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"user","content":"fix the bug"}`),
			},
			running: true,
			want:    StatusIdle,
		},
		{
			name: "tool result + running",
			rec: parser.Record{
				Type:      "user",
				Timestamp: now.Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}`),
			},
			running: true,
			want:    StatusIdle,
		},
		{
			name: "assistant tool_use + running",
			rec: parser.Record{
				Type:      "assistant",
				Timestamp: now.Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{}}]}`),
			},
			running: true,
			want:    StatusResponding,
		},
		{
			name: "assistant text-only + running",
			rec: parser.Record{
				Type:      "assistant",
				Timestamp: now.Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"done"}]}`),
			},
			running: true,
			want:    StatusIdle,
		},
		{
			name: "interrupt + running",
			rec: parser.Record{
				Type:      "user",
				Timestamp: now.Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"user","content":"[Request interrupted by user]"}`),
			},
			running: true,
			want:    StatusInterrupted,
		},
		{
			name: "result type always Done",
			rec: parser.Record{
				Type:      "result",
				Timestamp: now.Format(time.RFC3339Nano),
			},
			running: true,
			want:    StatusDone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveStatus(tt.rec, tt.isError, now, tt.running)
			if got != tt.want {
				fmt.Printf("  FAIL: got %s, want %s\n", got, tt.want)
			}
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}
