package process

import (
	"fmt"
	"runtime"
	"testing"
)

func TestListClaude(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	procs, err := ListClaude()
	if err != nil {
		t.Fatalf("ListClaude error: %v", err)
	}
	fmt.Printf("Found %d Claude processes:\n", len(procs))
	for _, p := range procs {
		fmt.Printf("  PID=%d  SessionID=%s  WorkDir=%s  Start=%v\n", p.PID, p.SessionID, p.WorkDir, p.StartTime)
	}
}
