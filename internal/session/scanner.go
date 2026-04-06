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
		// Skip subagent session files
		if strings.Contains(path, "subagents") {
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

	// Read head for original task and project name
	headRecords, err := parser.ReadHead(path)
	if err != nil {
		return err
	}
	originalTask := ExtractOriginalTask(headRecords)

	// Extract project name from cwd in session records, fall back to path
	projectName := extractProjectFromCwd(headRecords)
	if projectName == "" {
		projectName = extractProjectFromPath(path)
	}

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
			a := ExtractLastToolAction(tailRecords[i])
			if a != "" {
				action = a
				break
			}
		}
	}

	isError := CheckLastToolResultError(tailRecords)
	status := StatusIdle
	if len(tailRecords) > 0 {
		status = DeriveStatus(lastRec, isError, now)
	}

	var startTime time.Time
	for _, rec := range headRecords {
		if rec.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, rec.Timestamp); err == nil {
				startTime = t
				break
			}
		}
	}

	// Extract cwd from head records
	cwd := extractCwd(headRecords)

	s.mu.Lock()
	state.ProjectName = projectName
	state.Cwd = cwd
	state.OriginalTask = originalTask
	if action != "" {
		state.CurrentAction = action
	}
	state.Status = status
	if model != "" {
		state.Model = model
	}
	state.StartTime = startTime
	state.LastUpdate = now
	state.FileOffset = info.Size()
	state.FileModTime = info.ModTime()
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
	state.FileModTime = now
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

// LoadAll loads state for sessions that haven't been loaded yet.
func (s *Scanner) LoadAll() {
	s.mu.RLock()
	paths := make([]string, 0)
	for path, state := range s.sessions {
		if state.FileOffset == 0 {
			paths = append(paths, path)
		}
	}
	s.mu.RUnlock()

	for _, path := range paths {
		s.LoadSession(path)
	}
}

// FilteredSessions returns sessions whose cwd matches the given directory.
// Comparison is done on cleaned/normalized paths.
func (s *Scanner) FilteredSessions(dir string, maxAge time.Duration) []State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cleanDir := filepath.Clean(dir)
	var cutoff time.Time
	if maxAge > 0 {
		cutoff = time.Now().Add(-maxAge)
	}

	result := make([]State, 0)
	for _, state := range s.sessions {
		if maxAge > 0 && !state.FileModTime.After(cutoff) {
			continue
		}
		if state.Cwd != "" && filepath.Clean(state.Cwd) == cleanDir {
			result = append(result, *state)
		}
	}
	return deduplicateByCwd(result)
}

// RecentSessions returns sessions whose file was modified within maxAge.
// If maxAge is 0, all sessions are returned.
// Only the most recent session per cwd is kept.
func (s *Scanner) RecentSessions(maxAge time.Duration) []State {
	if maxAge == 0 {
		return deduplicateByCwd(s.Sessions())
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-maxAge)
	result := make([]State, 0)
	for _, state := range s.sessions {
		if state.FileModTime.After(cutoff) {
			result = append(result, *state)
		}
	}
	return deduplicateByCwd(result)
}

// deduplicateByCwd keeps only the most recently modified session per cwd.
func deduplicateByCwd(sessions []State) []State {
	best := make(map[string]State)
	for _, s := range sessions {
		key := s.Cwd
		if key == "" {
			key = s.FilePath // fallback for sessions without cwd
		}
		existing, ok := best[key]
		if !ok || s.FileModTime.After(existing.FileModTime) {
			best[key] = s
		}
	}
	result := make([]State, 0, len(best))
	for _, s := range best {
		result = append(result, s)
	}
	return result
}

func extractCwd(records []parser.Record) string {
	for _, rec := range records {
		if rec.Cwd != "" {
			return rec.Cwd
		}
	}
	return ""
}

func extractProjectFromCwd(records []parser.Record) string {
	for _, rec := range records {
		if rec.Cwd != "" {
			return filepath.Base(rec.Cwd)
		}
	}
	return ""
}

func extractProjectFromPath(path string) string {
	// Path: .../.claude/projects/<encoded-project>/<uuid>.jsonl
	dir := filepath.Dir(path) // .../<encoded-project>
	encoded := filepath.Base(dir)
	return ExtractProjectName(encoded)
}
