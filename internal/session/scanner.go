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
	"github.com/tarikguney/claude-watch/internal/process"
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
	lastAssistantWorking := isAssistantWorking(lastRec)
	lastIsSystemInjected := lastRec.IsSystemInjectedUser()
	lastHasToolResult := lastRec.HasToolResult()
	lastIsInterrupt := lastRec.IsInterruptRecord()
	status := StatusIdle
	if len(tailRecords) > 0 {
		status = DeriveStatus(lastRec, isError, now, state.PID > 0)
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
	lastPrompt := ExtractLastPrompt(tailRecords)
	lastResponse := ExtractLastResponse(tailRecords)

	s.mu.Lock()
	state.ProjectName = projectName
	state.Cwd = cwd
	state.OriginalTask = originalTask
	state.LastPrompt = lastPrompt
	state.LastResponse = lastResponse
	state.CurrentAction = action
	state.Status = status
	state.Model = model
	state.StartTime = startTime
	state.LastUpdate = now
	state.FileOffset = info.Size()
	state.FileModTime = info.ModTime()
	state.LastRecordType = lastRec.Type
	state.LastRecordTimestamp = lastRec.Timestamp
	state.LastToolResultError = isError
	state.LastAssistantIsWorking = lastAssistantWorking
	state.LastIsSystemInjectedUser = lastIsSystemInjected
	state.LastHasToolResult = lastHasToolResult
	state.LastIsInterrupt = lastIsInterrupt
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
	lastPrompt := ExtractLastPrompt(newRecords)
	lastResponse := ExtractLastResponse(newRecords)
	isError := CheckLastToolResultError(newRecords)
	lastAssistantWorking := isAssistantWorking(lastRec)
	status := DeriveStatus(lastRec, isError, now, state.PID > 0)

	s.mu.Lock()
	state.FileOffset = newOffset
	state.LastUpdate = now
	state.FileModTime = now
	state.Status = status
	state.LastRecordType = lastRec.Type
	state.LastRecordTimestamp = lastRec.Timestamp
	state.LastToolResultError = isError
	state.LastAssistantIsWorking = lastAssistantWorking
	state.LastIsSystemInjectedUser = lastRec.IsSystemInjectedUser()
	state.LastHasToolResult = lastRec.HasToolResult()
	state.LastIsInterrupt = lastRec.IsInterruptRecord()
	if action != "" {
		state.CurrentAction = action
	}
	if model != "" {
		state.Model = model
	}
	if lastPrompt != "" {
		state.LastPrompt = lastPrompt
	}
	if lastResponse != "" {
		state.LastResponse = lastResponse
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
		key = filepath.Clean(key)
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

// MatchProcesses associates running Claude processes with their session states.
// Strategy: use the process's --session-id to find which project directory the
// session lives in (~/.claude/projects/<encoded-project>/<session-id>.jsonl),
// then pick the most recently modified .jsonl in that same directory.
// This handles the case where Claude creates new sessions within the same process.
func (s *Scanner) MatchProcesses(procs []process.Info) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear all PIDs first (process may have exited).
	// Also remove placeholders — they'll be re-created below if still needed.
	for key, state := range s.sessions {
		state.PID = 0
		if strings.HasPrefix(key, "placeholder:") {
			delete(s.sessions, key)
		}
	}

	// Build lookup: filename (without .jsonl) → file path
	fileSessionToPath := make(map[string]string)
	for path := range s.sessions {
		base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if base != "" {
			fileSessionToPath[base] = path
		}
	}

	for _, proc := range procs {
		if proc.SessionID == "" {
			continue
		}

		// Find the original session file to determine the project directory
		origPath, ok := fileSessionToPath[proc.SessionID]
		if !ok {
			// Session file not yet discovered — try to find it on disk
			targetFile := proc.SessionID + ".jsonl"
			projectsDir := filepath.Join(s.claudeDir, "projects")
			filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
				if ok || err != nil || info == nil || info.IsDir() {
					return nil
				}
				if filepath.Base(path) == targetFile {
					origPath = path
					ok = true
					// Register it so LoadAll will pick it up
					if _, exists := s.sessions[path]; !exists {
						s.sessions[path] = &State{FilePath: path}
					}
				}
				return nil
			})
		}

		if !ok {
			// No session file on disk — process is running but hasn't written
			// a transcript yet (e.g., still initializing or waiting for first prompt).
			// Only create a placeholder if the process has a work directory,
			// otherwise it's likely a background/daemon process with nothing to show.
			if proc.WorkDir == "" {
				continue
			}
			placeholderKey := "placeholder:" + proc.SessionID
			s.sessions[placeholderKey] = &State{
				SessionID:   proc.SessionID,
				PID:         proc.PID,
				Cwd:         proc.WorkDir,
				ProjectName: filepath.Base(proc.WorkDir),
				Status:      StatusWaiting,
				StartTime:   proc.StartTime,
				LastUpdate:  time.Now(),
				FileModTime: time.Now(),
			}
			continue
		}

		// Found the project directory — now find the most recent session in it
		projectDir := filepath.Dir(origPath)
		bestPath := ""
		var bestMod time.Time
		for path, state := range s.sessions {
			if filepath.Dir(path) == projectDir && state.FileModTime.After(bestMod) {
				bestPath = path
				bestMod = state.FileModTime
			}
		}
		if bestPath != "" {
			// Remove any placeholder for this process since we found a real session
			delete(s.sessions, "placeholder:"+proc.SessionID)
			s.sessions[bestPath].PID = proc.PID
			if s.sessions[bestPath].StartTime.IsZero() && !proc.StartTime.IsZero() {
				s.sessions[bestPath].StartTime = proc.StartTime
			}
			if s.sessions[bestPath].Cwd == "" && proc.WorkDir != "" {
				s.sessions[bestPath].Cwd = proc.WorkDir
			}
			if s.sessions[bestPath].ProjectName == "" && proc.WorkDir != "" {
				s.sessions[bestPath].ProjectName = filepath.Base(proc.WorkDir)
			}
		}
	}

	// Re-derive status for sessions with a running process.
	// LoadSession may have computed status before PID was known.
	for _, state := range s.sessions {
		if state.PID <= 0 || state.LastRecordType == "" {
			continue
		}
		if state.Status == StatusDone {
			continue
		}
		if state.LastToolResultError {
			state.Status = StatusError
			continue
		}
		switch state.LastRecordType {
		case "assistant":
			if state.LastAssistantIsWorking {
				state.Status = StatusResponding
			} else {
				state.Status = StatusIdle
			}
		case "user":
			if state.LastIsInterrupt {
				state.Status = StatusInterrupted
			} else if state.LastIsSystemInjectedUser {
				// System-injected records (tool results, system reminders) with a
				// running process should show Responding — reminders get written
				// during long tool calls and don't mean Claude is idle.
				state.Status = StatusResponding
			} else {
				state.Status = StatusResponding
			}
		}
	}
}


// RunningSessions returns only sessions that have a running process (PID > 0).
func (s *Scanner) RunningSessions() []State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]State, 0)
	for _, state := range s.sessions {
		if state.PID > 0 {
			result = append(result, *state)
		}
	}
	return deduplicateByCwd(result)
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
