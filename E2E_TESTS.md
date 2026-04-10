# End-to-End Test Plan

Run these tests after every change. Build first: `go build -o claude-watch-test.exe .`

## Step 1: Gather ground truth

Run these commands to collect the current environment state:

```bash
# All Claude PIDs and their parent PIDs
powershell -NoProfile -Command "Get-CimInstance Win32_Process -Filter 'Name=''claude.exe''' | ForEach-Object { \"$($_.ProcessId)|$($_.ParentProcessId)\" }"

# All psmux pane PIDs with session/window
for sess in $(psmux list-sessions -F "#{session_name}" 2>/dev/null); do
  psmux list-panes -s -t "$sess" -F "#{pane_pid} ${sess}/#{window_name}" 2>/dev/null
done

# IO read bytes for each Claude PID (sample twice with 2s gap)
# delta=0 means idle, delta>0 means active
```

This gives you three datasets:
- **Process list**: every Claude PID and its parent PID
- **Pane map**: every psmux pane PID and its session/window label
- **IO activity**: which processes are actually doing work

## Step 2: Derive expected state

For each Claude PID:

1. **Tmux mapping**: Walk parent PID chain (use `Get-CimInstance Win32_Process -Filter "ProcessId=<ppid>"` repeatedly). If any ancestor PID appears in the pane map, the expected TMUX SESSION/WINDOW is that pane's `session/window` label. Otherwise it should be empty.

2. **Status**: Check the IO delta for this PID.
   - delta > 0 → expected **Responding**
   - delta = 0 → expected **Idle** (unless the last JSONL record is a fresh real user prompt within 2 minutes, then **Responding**)

3. **Project**: The basename of the process CWD.

## Step 3: Run the dashboard and compare

Launch `./claude-watch-test.exe --max-age 168h` in a separate pane. For each row in the dashboard, verify against the expected state from Step 2:

- [ ] **PID** matches a real Claude process
- [ ] **TMUX SESSION/WINDOW** matches the expected pane label (or empty if not in tmux)
- [ ] **PROJECT** matches the basename of the process CWD
- [ ] **STATUS** matches the expected status (Idle/Responding) based on IO activity
- [ ] No idle process shows "Responding" (the false-Responding regression)
- [ ] No active process shows "Idle"
- [ ] Each CWD appears at most once (deduplication)
