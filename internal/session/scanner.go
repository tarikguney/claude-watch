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
	"github.com/tarikguney/claude-watch/internal/tmux"
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
	processRunning := state.PID > 0
	s.mu.Unlock()

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
	lastRec := lastStatusRelevantRecord(tailRecords)

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
		status = DeriveStatus(lastRec, isError, now, processRunning)
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
	state.LastRecordSubtype = lastRec.Subtype
	state.LastRecordTimestamp = lastRec.Timestamp
	state.LastToolResultError = isError
	state.LastAssistantIsWorking = lastAssistantWorking
	state.LastIsSystemInjectedUser = lastIsSystemInjected
	state.LastHasToolResult = lastHasToolResult
	state.LastIsInterrupt = lastIsInterrupt
	state.LastStopReason = extractStopReason(lastRec)
	state.LastBlockTypes = extractBlockTypes(lastRec)
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
	processRunning := state.PID > 0
	prevStatus := state.Status
	s.mu.RUnlock()

	newRecords, newOffset, err := parser.ReadNewBytes(path, offset)
	if err != nil {
		return err
	}

	if len(newRecords) == 0 {
		return nil
	}

	now := time.Now()
	lastRec := lastStatusRelevantRecord(newRecords)

	// If none of the new records are status-relevant (e.g. only attachment or
	// file-history-snapshot), keep the previous status and just advance the offset.
	hasRelevant := lastRec.IsStatusRelevant()

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

	var status Status
	if hasRelevant {
		status = DeriveStatus(lastRec, isError, now, processRunning)
	} else {
		status = prevStatus // preserve previous status
	}

	newCwd := ""
	for _, rec := range newRecords {
		if rec.Cwd != "" {
			newCwd = rec.Cwd
			break
		}
	}

	s.mu.Lock()
	state.FileOffset = newOffset
	state.LastUpdate = now
	state.FileModTime = now
	state.Status = status
	// Only update cached record metadata when we have a meaningful record.
	// Otherwise preserve the previous state for re-derivation.
	if hasRelevant {
		state.LastRecordType = lastRec.Type
		state.LastRecordSubtype = lastRec.Subtype
		state.LastRecordTimestamp = lastRec.Timestamp
		state.LastToolResultError = isError
		state.LastAssistantIsWorking = lastAssistantWorking
		state.LastIsSystemInjectedUser = lastRec.IsSystemInjectedUser()
		state.LastHasToolResult = lastRec.HasToolResult()
		state.LastIsInterrupt = lastRec.IsInterruptRecord()
		state.LastStopReason = extractStopReason(lastRec)
		state.LastBlockTypes = extractBlockTypes(lastRec)
	}
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
	if state.Cwd == "" && newCwd != "" {
		state.Cwd = newCwd
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
// Primary strategy: encode the process's OS-level CWD to find the project directory
// (~/.claude/projects/<encoded-cwd>/), then pick the most recently modified .jsonl.
// Fallback: use --session-id to locate the session file directly.
// paneMap maps pane PIDs to tmux session/window info (nil if tmux unavailable).
func (s *Scanner) MatchProcesses(procs []process.Info, paneMap map[int]tmux.PaneInfo) {
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

		projectDir := ""

		// Primary: use OS CWD to derive the project directory
		if proc.Cwd != "" {
			encoded := EncodeProjectDir(proc.Cwd)
			candidate := filepath.Join(s.claudeDir, "projects", encoded)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				projectDir = candidate
				// Discover any session files not yet known
				entries, _ := os.ReadDir(candidate)
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
						continue
					}
					if strings.Contains(e.Name(), "subagents") {
						continue
					}
					fullPath := filepath.Join(candidate, e.Name())
					if _, exists := s.sessions[fullPath]; !exists {
						s.sessions[fullPath] = &State{FilePath: fullPath}
					}
				}
			}
		}

		// Fallback: use --session-id to find the project directory
		if projectDir == "" {
			if origPath, ok := fileSessionToPath[proc.SessionID]; ok {
				projectDir = filepath.Dir(origPath)
			} else {
				// Walk to find the session file on disk
				targetFile := proc.SessionID + ".jsonl"
				projectsDir := filepath.Join(s.claudeDir, "projects")
				filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
					if projectDir != "" || err != nil || info == nil || info.IsDir() {
						return nil
					}
					if filepath.Base(path) == targetFile {
						projectDir = filepath.Dir(path)
						if _, exists := s.sessions[path]; !exists {
							s.sessions[path] = &State{FilePath: path}
						}
					}
					return nil
				})
			}
		}

		// Resolve tmux session/window from parent PID chain
		tmuxSession, tmuxPaneID := tmux.Resolve(paneMap, proc.ParentPIDs)

		if projectDir == "" {
			s.createWaitingSession(proc, tmuxSession, tmuxPaneID)
			continue
		}

		// Find the most recently modified session in the project directory
		bestPath := ""
		var bestMod time.Time
		for path, state := range s.sessions {
			if filepath.Dir(path) == projectDir {
				if state.FileModTime.After(bestMod) || (bestPath == "" && !state.FileModTime.After(bestMod)) {
					bestPath = path
					bestMod = state.FileModTime
				}
			}
		}
		// If the best session file was modified before the process started,
		// it's from a previous session — use a placeholder instead.
		if bestPath != "" && !proc.StartTime.IsZero() && !bestMod.IsZero() &&
			bestMod.Before(proc.StartTime) {
			bestPath = ""
		}

		if bestPath != "" {
			delete(s.sessions, "placeholder:"+proc.SessionID)
			st := s.sessions[bestPath]
			st.PID = proc.PID
			// Preserve the last known tmux info when resolution transiently fails
			// (e.g. a single psmux list-panes call errors, or the Windows PID-chain
			// snapshot misses the pane PID for one tick). Overwriting with empty
			// would briefly render "not in tmux" until the next 2s poll recovers.
			if tmuxSession != "" {
				st.TmuxSession = tmuxSession
				st.TmuxPaneID = tmuxPaneID
			}
			if st.StartTime.IsZero() && !proc.StartTime.IsZero() {
				st.StartTime = proc.StartTime
			}
			// Always set CWD and ProjectName from the OS-level CWD
			if proc.Cwd != "" {
				st.Cwd = proc.Cwd
				st.ProjectName = filepath.Base(proc.Cwd)
			}
		} else {
			s.createWaitingSession(proc, tmuxSession, tmuxPaneID)
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
		// System turn_duration means the turn ended.
		if state.LastRecordType == "system" && state.LastRecordSubtype == "turn_duration" {
			state.Status = StatusIdle
			continue
		}
		switch state.LastRecordType {
		case "attachment":
			// Attachment only appears before assistant response — active.
			state.Status = StatusThinking
		case "assistant":
			state.Status = rederiveAssistantStatus(state.LastStopReason, state.LastBlockTypes, state.LastAssistantIsWorking)
		case "user":
			if state.LastIsInterrupt {
				state.Status = StatusInterrupted
			} else if state.LastHasToolResult {
				state.Status = StatusResponding
			} else if state.LastIsSystemInjectedUser {
				state.Status = StatusIdle
			} else {
				// Real user prompt with running process — Claude is thinking.
				recTime, err := time.Parse(time.RFC3339Nano, state.LastRecordTimestamp)
				if err == nil && time.Since(recTime) < activeThreshold {
					state.Status = StatusThinking
				} else {
					state.Status = StatusIdle
				}
			}
		}
	}
}


// createWaitingSession creates a placeholder session for a process that has no matching
// session file yet. Requires the caller to hold s.mu.
func (s *Scanner) createWaitingSession(proc process.Info, tmuxSession, tmuxPaneID string) {
	if proc.Cwd == "" {
		return
	}
	s.sessions["placeholder:"+proc.SessionID] = &State{
		SessionID:   proc.SessionID,
		PID:         proc.PID,
		Cwd:         proc.Cwd,
		ProjectName: filepath.Base(proc.Cwd),
		TmuxSession: tmuxSession,
		TmuxPaneID:  tmuxPaneID,
		Status:      StatusWaiting,
		StartTime:   proc.StartTime,
		LastUpdate:  time.Now(),
		FileModTime: time.Now(),
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

// lastStatusRelevantRecord returns the last record whose type is meaningful
// for status derivation, skipping metadata records like attachment and
// file-history-snapshot. Falls back to the very last record if none qualify.
func lastStatusRelevantRecord(records []parser.Record) parser.Record {
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].IsStatusRelevant() {
			return records[i]
		}
	}
	if len(records) > 0 {
		return records[len(records)-1]
	}
	return parser.Record{}
}

