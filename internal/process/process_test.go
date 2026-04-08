package process

import (
	"fmt"
	"os"
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
		fmt.Printf("  PID=%d  SessionID=%s  Cwd=%s  Start=%v\n", p.PID, p.SessionID, p.Cwd, p.StartTime)
	}
}

func TestParseUnixLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantPID  int
		wantSID  string
		wantYear int
	}{
		{
			name:     "standard lstart with double-space day",
			line:     "12345 Sun Apr  6 10:53:38 2026 /home/user/.local/share/claude/versions/2.1.92/claude --session-id 2c8d67fc-e59a-4a9e-af4d-2878b83ffe84 --dangerously-skip-permissions --add-dir /sources/project",
			wantPID:  12345,
			wantSID:  "2c8d67fc-e59a-4a9e-af4d-2878b83ffe84",
			wantYear: 2026,
		},
		{
			name:     "two-digit day",
			line:     "  999 Mon Apr 16 09:01:02 2026 /usr/local/bin/claude --session-id abcdef01-2345-6789-abcd-ef0123456789 --add-dir /home/user/myapp",
			wantPID:  999,
			wantSID:  "abcdef01-2345-6789-abcd-ef0123456789",
			wantYear: 2026,
		},
		{
			name:    "no add-dir flag",
			line:    "54321 Tue Jan 10 14:30:00 2026 /home/user/.local/bin/claude --session-id 11111111-2222-3333-4444-555555555555 --mcp-config /tmp/claude-mcp-abc.json",
			wantPID: 54321,
			wantSID: "11111111-2222-3333-4444-555555555555",
		},
		{
			name:    "no session id — not a claude session",
			line:    "11111 Thu Feb 20 12:00:00 2026 /usr/bin/some-other-process",
			wantPID: 11111,
			wantSID: "",
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
			if tt.wantYear > 0 && !info.StartTime.IsZero() && info.StartTime.Year() != tt.wantYear {
				t.Errorf("StartTime year: got %d, want %d", info.StartTime.Year(), tt.wantYear)
			}
			// Note: Cwd is read from OS via GetProcessCwd, not from the command line.
			// Can't test a specific value here since the PID doesn't exist.
		})
	}
}

func TestExtractFlag(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want string
	}{
		{"session-id present", "--session-id 2c8d67fc-e59a-4a9e-af4d-2878b83ffe84 --other", "2c8d67fc-e59a-4a9e-af4d-2878b83ffe84"},
		{"no match", "--other-flag value", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFlag(sessionIDRe, tt.cmd)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetProcessCwd(t *testing.T) {
	// Test with our own process — should return the current working directory
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("GetProcessCwd test only on windows/linux")
	}
	cwd, err := GetProcessCwd(os.Getpid())
	if err != nil {
		t.Fatalf("GetProcessCwd(%d) error: %v", os.Getpid(), err)
	}
	if cwd == "" {
		t.Error("expected non-empty CWD for own process")
	}
	t.Logf("Own process CWD: %s", cwd)
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
