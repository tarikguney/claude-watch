// Copyright 2026 Tarik Guney
// Licensed under the MIT License.
// https://github.com/tarikguney/claude-watch

package session

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tarikguney/claude-watch/internal/parser"
)

// Scanner discovers and manages Claude Code session files.
type Scanner struct {
	claudeDir string
	mu        sync.RWMutex
	sessions  map[string]*State // keyed by file path
}

// NewScanner creates a Scanner that looks for sessions under the given Claude directory.
func NewScanner(claudeDir string) *Scanner {
	return &Scanner{
		claudeDir: claudeDir,
		sessions:  make(map[string]*State),
	}
}

// ClaudeDir returns the base Claude directory being scanned.
func (s *Scanner) ClaudeDir() string {
	return s.claudeDir
}

// SessionsDir returns the path pattern where session files live.
func (s *Scanner) SessionsDir() string {
	return filepath.Join(s.claudeDir, "projects")
}

// Discover walks the Claude projects directory and finds all .jsonl session files.
func (s *Scanner) Discover() error {
	projectsDir := s.SessionsDir()
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return nil
	}

	return filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		if info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		// Expect path like: .claude/projects/<encoded-project>/sessions/<uuid>.jsonl
		if !strings.Contains(path, "sessions") {
			return nil
		}

		s.mu.Lock()
		if _, exists := s.sessions[path]; !exists {
			s.sessions[path] = &State{
				FilePath: path,
			}
		}
		s.mu.Unlock()
		return nil
	})
}

// LoadSession reads the head and tail of a session file to populate its state.
func (s *Scanner) LoadSession(path string) error {
	s.mu.Lock()
	state, exists := s.sessions[path]
	if !exists {
		state = &State{FilePath: path}
		s.sessions[path] = state
	}
	s.mu.Unlock()

	// Extract project name from path
	projectName := extractProjectFromPath(path)

	// Read head for original task
	headRecords, err := parser.ReadHead(path)
	if err != nil {
		return err
	}
	originalTask := ExtractOriginalTask(headRecords)

	// Read tail for current state
	tailRecords, err := parser.ReadTail(path)
	if err != nil {
		return err
	}

	// Get file size for offset tracking
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	now := time.Now()
	var lastRec parser.Record
	if len(tailRecords) > 0 {
		lastRec = tailRecords[len(tailRecords)-1]
	}

	model := ExtractModel(tailRecords)
	action := ""
	for i := len(tailRecords) - 1; i >= 0; i-- {
		if tailRecords[i].Type == "assistant" {
			action = ExtractLastToolAction(tailRecords[i])
			break
		}
	}

	isError := CheckLastToolResultError(tailRecords)
	status := StatusIdle
	if len(tailRecords) > 0 {
		status = DeriveStatus(lastRec, isError, now)
	}

	var startTime time.Time
	if len(headRecords) > 0 {
		if t, err := time.Parse(time.RFC3339Nano, headRecords[0].Timestamp); err == nil {
			startTime = t
		}
	}

	s.mu.Lock()
	state.ProjectName = projectName
	state.OriginalTask = originalTask
	state.CurrentAction = action
	state.Status = status
	state.Model = model
	state.StartTime = startTime
	state.LastUpdate = now
	state.FileOffset = info.Size()
	if lastRec.SessionID != "" {
		state.SessionID = lastRec.SessionID
	}
	s.mu.Unlock()

	return nil
}

// UpdateSession reads only new bytes from a session file and updates state.
func (s *Scanner) UpdateSession(path string) error {
	s.mu.RLock()
	state, exists := s.sessions[path]
	if !exists {
		s.mu.RUnlock()
		return s.LoadSession(path)
	}
	offset := state.FileOffset
	s.mu.RUnlock()

	newRecords, newOffset, err := parser.ReadNewBytes(path, offset)
	if err != nil {
		return err
	}

	if len(newRecords) == 0 {
		return nil
	}

	now := time.Now()
	lastRec := newRecords[len(newRecords)-1]

	action := ""
	for i := len(newRecords) - 1; i >= 0; i-- {
		if newRecords[i].Type == "assistant" {
			a := ExtractLastToolAction(newRecords[i])
			if a != "" {
				action = a
			}
			break
		}
	}

	model := ExtractModel(newRecords)
	isError := CheckLastToolResultError(newRecords)
	status := DeriveStatus(lastRec, isError, now)

	s.mu.Lock()
	state.FileOffset = newOffset
	state.LastUpdate = now
	state.Status = status
	if action != "" {
		state.CurrentAction = action
	}
	if model != "" {
		state.Model = model
	}
	if lastRec.SessionID != "" {
		state.SessionID = lastRec.SessionID
	}
	s.mu.Unlock()

	return nil
}

// Sessions returns a snapshot of all known session states.
func (s *Scanner) Sessions() []State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]State, 0, len(s.sessions))
	for _, state := range s.sessions {
		result = append(result, *state)
	}
	return result
}

// LoadAll loads state for all discovered sessions.
func (s *Scanner) LoadAll() {
	s.mu.RLock()
	paths := make([]string, 0, len(s.sessions))
	for path := range s.sessions {
		paths = append(paths, path)
	}
	s.mu.RUnlock()

	for _, path := range paths {
		s.LoadSession(path)
	}
}

func extractProjectFromPath(path string) string {
	// Path: .../.claude/projects/<encoded-project>/sessions/<uuid>.jsonl
	dir := filepath.Dir(path) // .../sessions
	dir = filepath.Dir(dir)   // .../<encoded-project>
	encoded := filepath.Base(dir)
	return ExtractProjectName(encoded)
}
