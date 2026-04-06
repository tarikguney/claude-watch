# claude-watch

A zero-setup CLI dashboard for monitoring Claude Code agents in real time.

Run `claude-watch` and instantly see what all your Claude Code sessions are doing -- which project, what task, current action, and how long they've been running. Designed to live in a tmux pane as your agent task manager.

## How it works

Claude Code writes append-only JSONL transcripts to `~/.claude/projects/`. claude-watch watches these files, parses the tail of each session, and renders a continuously-updating dashboard:

```
CLAUDE WATCH                                           04/05 14:23:45
─────────────────────────────────────────────────────────────────────────────────────────────────
PROJECT        STATUS       TASK                           CURRENT ACTION                     DUR
myapp          Active       Add auth to API endpoints      Editing middleware.ts               12m
webapp         Active       Fix login page CSS             Running npm test                    5m
backend        Idle         Refactor database layer        Writing schema.ts                   18m
cli-tool       Done         Add --verbose flag             Completed                           8m
```

No hooks to configure, no agents to register, no setup. It reads what's already on disk.

## Installation

```bash
go install github.com/tarikguney/claude-watch@latest
```

## Usage

```bash
# Just run it
claude-watch

# Custom refresh interval
claude-watch --refresh 1s

# Custom Claude directory
claude-watch --claude-dir /path/to/.claude

# Compact mode for narrow tmux panes
claude-watch --compact
```

## Status indicators

| Status | Meaning |
|---|---|
| **Active** | Agent is executing tool calls (reading, editing, running commands) |
| **Responding** | Agent is generating a text response |
| **Thinking** | Model is processing (user message sent, waiting for response) |
| **Idle** | No activity for >5 minutes, session not completed |
| **Done** | Session completed |
| **Error** | Last tool call returned an error |

## License

MIT
