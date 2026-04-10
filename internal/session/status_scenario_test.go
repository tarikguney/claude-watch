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
		name     string
		rec      parser.Record
		isError  bool
		running  bool
		ioActive bool
		want     Status
	}{
		{
			name: "system-injected + running + no IO (the bug)",
			rec: parser.Record{
				Type:      "user",
				Timestamp: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"user","content":"<local-command-stdout>Copied to clipboard</local-command-stdout>"}`),
			},
			running: true, ioActive: false,
			want: StatusIdle,
		},
		{
			name: "system-injected + running + IO active",
			rec: parser.Record{
				Type:      "user",
				Timestamp: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"user","content":"<local-command-stdout>Copied to clipboard</local-command-stdout>"}`),
			},
			running: true, ioActive: true,
			want: StatusResponding,
		},
		{
			name: "fresh real user prompt + no process + no IO",
			rec: parser.Record{
				Type:      "user",
				Timestamp: now.Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"user","content":"fix the bug"}`),
			},
			running: false, ioActive: false,
			want: StatusResponding,
		},
		{
			name: "old real user prompt + running + no IO",
			rec: parser.Record{
				Type:      "user",
				Timestamp: now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"user","content":"fix the bug"}`),
			},
			running: true, ioActive: false,
			want: StatusIdle,
		},
		{
			name: "tool result + running + IO",
			rec: parser.Record{
				Type:      "user",
				Timestamp: now.Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}`),
			},
			running: true, ioActive: true,
			want: StatusResponding,
		},
		{
			name: "tool result + running + no IO",
			rec: parser.Record{
				Type:      "user",
				Timestamp: now.Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}`),
			},
			running: true, ioActive: false,
			want: StatusIdle,
		},
		{
			name: "assistant tool_use + running + no IO",
			rec: parser.Record{
				Type:      "assistant",
				Timestamp: now.Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{}}]}`),
			},
			running: true, ioActive: false,
			want: StatusResponding,
		},
		{
			name: "assistant text-only + running + no IO",
			rec: parser.Record{
				Type:      "assistant",
				Timestamp: now.Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"done"}]}`),
			},
			running: true, ioActive: false,
			want: StatusIdle,
		},
		{
			name: "assistant text-only + running + IO",
			rec: parser.Record{
				Type:      "assistant",
				Timestamp: now.Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"done"}]}`),
			},
			running: true, ioActive: true,
			want: StatusResponding,
		},
		{
			name: "interrupt + running + IO",
			rec: parser.Record{
				Type:      "user",
				Timestamp: now.Format(time.RFC3339Nano),
				Message:   json.RawMessage(`{"role":"user","content":"[Request interrupted by user]"}`),
			},
			running: true, ioActive: true,
			want: StatusInterrupted,
		},
		{
			name: "result type always Done",
			rec: parser.Record{
				Type:      "result",
				Timestamp: now.Format(time.RFC3339Nano),
			},
			running: true, ioActive: true,
			want: StatusDone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveStatus(tt.rec, tt.isError, now, tt.running, tt.ioActive)
			if got != tt.want {
				fmt.Printf("  FAIL: got %s, want %s\n", got, tt.want)
			}
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}
