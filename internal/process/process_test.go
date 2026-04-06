package process

import (
	"fmt"
	"runtime"
	"testing"
)

func TestListClaude(t *testing.T) {
	procs, err := ListClaude()
	if err != nil {
		t.Fatalf("ListClaude error: %v", err)
	}
	fmt.Printf("Found %d Claude processes:\n", len(procs))
	for _, p := range procs {
		fmt.Printf("  PID=%d  SessionID=%s  WorkDir=%s  Start=%v\n", p.PID, p.SessionID, p.WorkDir, p.StartTime)
	}
}

func TestParseUnixLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantPID   int
		wantSID   string
		wantDir   string
		wantYear  int
	}{
		{
			name:     "standard lstart with double-space day",
			line:     "12345 Sun Apr  6 10:53:38 2026 /home/user/.local/share/claude/versions/2.1.92/claude --session-id 2c8d67fc-e59a-4a9e-af4d-2878b83ffe84 --dangerously-skip-permissions --add-dir /sources/project",
			wantPID:  12345,
			wantSID:  "2c8d67fc-e59a-4a9e-af4d-2878b83ffe84",
			wantDir:  "/sources/project",
			wantYear: 2026,
		},
		{
			name:     "two-digit day",
			line:     "  999 Mon Apr 16 09:01:02 2026 /usr/local/bin/claude --session-id abcdef01-2345-6789-abcd-ef0123456789 --add-dir /home/user/myapp",
			wantPID:  999,
			wantSID:  "abcdef01-2345-6789-abcd-ef0123456789",
			wantDir:  "/home/user/myapp",
			wantYear: 2026,
		},
		{
			name:     "no add-dir flag",
			line:     "54321 Tue Jan 10 14:30:00 2026 /home/user/.local/bin/claude --session-id 11111111-2222-3333-4444-555555555555 --mcp-config /tmp/claude-mcp-abc.json",
			wantPID:  54321,
			wantSID:  "11111111-2222-3333-4444-555555555555",
			wantDir:  "",
			wantYear: 2026,
		},
		{
			name:     "homebrew macOS path",
			line:     "67890 Wed Mar  5 08:15:30 2026 /opt/homebrew/bin/claude --session-id aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee --add-dir /Users/dev/project",
			wantPID:  67890,
			wantSID:  "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			wantDir:  "/Users/dev/project",
			wantYear: 2026,
		},
		{
			name:    "no session id — not a claude session",
			line:    "11111 Thu Feb 20 12:00:00 2026 /usr/bin/some-other-process",
			wantPID: 11111,
			wantSID: "",
			wantDir: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := parseUnixLine(tt.line)
			if info.PID != tt.wantPID {
				t.Errorf("PID: got %d, want %d", info.PID, tt.wantPID)
			}
			if info.SessionID != tt.wantSID {
				t.Errorf("SessionID: got %q, want %q", info.SessionID, tt.wantSID)
			}
			if info.WorkDir != tt.wantDir {
				t.Errorf("WorkDir: got %q, want %q", info.WorkDir, tt.wantDir)
			}
			if tt.wantYear > 0 && !info.StartTime.IsZero() && info.StartTime.Year() != tt.wantYear {
				t.Errorf("StartTime year: got %d, want %d", info.StartTime.Year(), tt.wantYear)
			}
		})
	}
}

func TestParsePipedLines(t *testing.T) {
	input := `33416|2026-04-06T10:53:38.3082190-06:00|"C:\Users\user\.claude-cli\2.1.92\claude.exe" --session-id 2c8d67fc-e59a-4a9e-af4d-2878b83ffe84 --add-dir Q:/sources/project
23208|2026-04-06T10:55:03.8656050-06:00|"C:\Users\user\.claude-cli\2.1.92\claude.exe" --session-id ee8e923a-24b3-46d5-a97e-153ca47848cd --add-dir Q:/sources/other
`

	results, err := parsePipedLines(input)
	if err != nil {
		t.Fatalf("parsePipedLines error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].PID != 33416 {
		t.Errorf("PID: got %d, want 33416", results[0].PID)
	}
	if results[0].SessionID != "2c8d67fc-e59a-4a9e-af4d-2878b83ffe84" {
		t.Errorf("SessionID: got %q", results[0].SessionID)
	}
	if results[0].WorkDir != "Q:/sources/project" {
		t.Errorf("WorkDir: got %q", results[0].WorkDir)
	}

	if results[1].PID != 23208 {
		t.Errorf("PID: got %d, want 23208", results[1].PID)
	}
}

func TestExtractFlag(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want string
	}{
		{"session-id present", "--session-id 2c8d67fc-e59a-4a9e-af4d-2878b83ffe84 --other", "2c8d67fc-e59a-4a9e-af4d-2878b83ffe84"},
		{"add-dir present", "--add-dir /home/user/project --other", "/home/user/project"},
		{"no match", "--other-flag value", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "add-dir present" {
				got := extractFlag(addDirRe, tt.cmd)
				if got != tt.want {
					t.Errorf("got %q, want %q", got, tt.want)
				}
			} else {
				got := extractFlag(sessionIDRe, tt.cmd)
				if got != tt.want {
					t.Errorf("got %q, want %q", got, tt.want)
				}
			}
		})
	}
}

func TestListClaude_Integration(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("unsupported platform")
	}
	// Just verify it doesn't error — may return 0 processes in CI
	_, err := ListClaude()
	if err != nil {
		t.Fatalf("ListClaude should not error: %v", err)
	}
}
