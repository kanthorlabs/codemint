# EPIC-01 MVP Test Guide

What you can test after EPIC-01 implementation. TUI only — CUI is stub (EPIC-02).

---

## What Works

| Feature | Status |
|---------|--------|
| TUI REPL (stdin/stdout) | ✅ |
| SQLite + migrations | ✅ |
| XDG directories | ✅ |
| UUIDv7 IDs | ✅ |
| Core commands (`/help`, `/exit`, `/clear`) | ✅ |
| Mode switching (`/mode`) | ✅ |
| Session resume (`/session-resume`) | ✅ |
| Activity log (`/activity`) | ✅ |
| Auto-resume on startup | ✅ |
| Session ownership + heartbeat | ✅ |
| CUI adapter | ⚠️ Stub only (no Telegram) |
| `/project-open` | ❌ Not implemented |
| Brainstormer / AI workflows | ❌ EPIC-02 |
| ACP agent execution | ❌ EPIC-03 |

---

## Setup

### 1. Build

```bash
make build
```

### 2. Verify Binary

```bash
./build/codemint --version
./build/codemint --help
```

Expected: version string, full flag list (`--config`, `--db`, `--mode`, etc.).

### 3. Inspect XDG Dirs

```bash
ls -la ~/.local/share/codemint/
ls -la ~/.config/codemint/
```

Expected: `database.db`, `config.yaml` (or empty if not configured).

---

## Test 1: Basic REPL Lifecycle

**Goal:** Verify startup, prompt, graceful shutdown.

```bash
./build/codemint
```

Steps:
1. Banner shows version + commit
2. Type `/help` → list of commands grouped by mode
3. Type `/clear` → screen clears
4. Type `/exit` → "Goodbye. Closing session..." → clean exit (code 0)

Re-run + Ctrl+C:
1. `./build/codemint`
2. Press `Ctrl+C` → "Received shutdown signal, exiting..." → exit cleanly

---

## Test 2: Database Migrations

**Goal:** Verify schema applied on first boot.

```bash
rm -f ~/.local/share/codemint/database.db
./build/codemint
# /exit
sqlite3 ~/.local/share/codemint/database.db ".tables"
```

Expected tables: `agent`, `project`, `session`, `task`, `workflow`, `project_permission`, `schema_migrations`.

```bash
sqlite3 ~/.local/share/codemint/database.db ".schema session"
sqlite3 ~/.local/share/codemint/database.db ".schema task"
```

Expected:
- `session` has `active_client`, `last_activity_at` columns
- `task` has `client_id` column

```bash
sqlite3 ~/.local/share/codemint/database.db "SELECT name, type FROM agent;"
```

Expected: seeded `human` agent (type=0).

---

## Test 3: Help Mode Filtering

**Goal:** Verify `/help` filters by `ClientMode`.

```bash
./build/codemint --mode=cli
```

Type `/help` → should show: `/help`, `/exit`, `/clear`, `/mode`, `/session-resume`, `/activity`.

```bash
./build/codemint --mode=daemon
```

Type `/help` → should NOT show `/exit`, `/clear` (CLI-only).

---

## Test 4: Mode Switching

**Goal:** Verify `/mode` runtime mode change.

```bash
./build/codemint
```

Steps:
1. `/mode` → "Current mode: cli"
2. `/mode daemon` → "Switched to daemon mode"
3. `/help` → daemon-compatible commands only (no `/exit`, `/clear`)
4. `/mode cli` → "Switched to cli mode" (works because TTY)
5. `/help` → all commands again
6. `/exit` → clean exit

Negative test:
```bash
./build/codemint < /dev/null
# Pipe stdin → not TTY
```

`/mode cli` should fail with "stdout is not a TTY".

---

## Test 5: Session Auto-Resume

**Goal:** Verify auto-resume on startup with active session.

**Setup:** Manually insert active session (since `/project-open` not implemented):

```bash
sqlite3 ~/.local/share/codemint/database.db <<EOF
INSERT INTO project (id, name, working_dir, created_at, updated_at)
VALUES ('proj-test1', 'testproj', '/tmp/testproj', strftime('%s','now'), strftime('%s','now'));

INSERT INTO session (id, project_id, status, created_at, updated_at)
VALUES ('sess-test1', 'proj-test1', 0, strftime('%s','now'), strftime('%s','now'));
EOF
```

Run:
```bash
./build/codemint
```

Expected: `"Resuming session sess-test1 for project testproj"`.

Type `/exit` → clean exit.

Run again:
```bash
./build/codemint
```

Should auto-resume same session.

---

## Test 6: Session Switching

**Goal:** Verify `/session-resume` switches between sessions.

**Setup:** Add second session:
```bash
sqlite3 ~/.local/share/codemint/database.db <<EOF
INSERT INTO project (id, name, working_dir, created_at, updated_at)
VALUES ('proj-test2', 'otherproj', '/tmp/otherproj', strftime('%s','now'), strftime('%s','now'));

INSERT INTO session (id, project_id, status, created_at, updated_at)
VALUES ('sess-test2', 'proj-test2', 0, strftime('%s','now'), strftime('%s','now'));
EOF
```

Run:
```bash
./build/codemint
```

Steps:
1. Auto-resumes one of the sessions
2. `/session-resume` → list both with current marker
3. `/session-resume sess-test2` → "Switched to session sess-test2 for project otherproj"
4. `/session-resume <invalid>` → "session not found" error
5. `/exit`

---

## Test 7: Heartbeat + Ownership

**Goal:** Verify `last_activity_at` updates every 15s.

```bash
./build/codemint
```

In another terminal, watch:
```bash
watch -n 5 'sqlite3 ~/.local/share/codemint/database.db "SELECT id, active_client, last_activity_at FROM session;"'
```

Expected: `active_client` set to `cli:<uuid>`, `last_activity_at` increments every 15s.

After `/exit`:
```bash
sqlite3 ~/.local/share/codemint/database.db "SELECT active_client FROM session;"
```

Expected: `active_client = NULL` (released on shutdown).

---

## Test 8: Multi-Client Handoff

**Goal:** Verify TUI ↔ daemon handoff + suspended mode.

**Terminal A:**
```bash
./build/codemint
```

Auto-resumes session. Type `/help`. Wait.

**Terminal B (within 60s):**
```bash
./build/codemint --mode=daemon
```

Expected:
- B auto-resumes same session
- A displays: `"Session taken over by daemon:<uuid>. Type anything to reclaim."`

**Terminal A:** Type any input (e.g. `/help`).

Expected:
- A reclaims session
- A displays: `"Activity while you were away:"` with B's interactions
- B displays: `"Session taken over by cli:<uuid>..."`

**Terminal B:** Type `/exit`.

---

## Test 9: Activity Log

**Goal:** Verify interactions persist as Coordination tasks (`type=3`).

```bash
./build/codemint
```

Steps:
1. `/help`
2. `/mode`
3. `/activity` → list of last interactions

Verify in DB:
```bash
sqlite3 ~/.local/share/codemint/database.db \
  "SELECT id, client_id, type, status, json_extract(input, '$.command') FROM task WHERE type = 3 ORDER BY id DESC LIMIT 5;"
```

Expected:
- `type = 3` (Coordination)
- `status = 5` (completed)
- `client_id` matches current client
- `input.command` matches typed commands

---

## Test 10: Stale Client Auto-Takeover

**Goal:** Verify takeover after staleness threshold (60s).

**Terminal A:**
```bash
./build/codemint
```

Auto-resumes. `kill -STOP <pid>` from another terminal (or pause heartbeat).

Wait 90s.

**Terminal B:**
```bash
./build/codemint --mode=daemon
```

Expected: silent takeover (no broadcast — A is stale).

`kill -CONT <pid>` to resume A. A should detect ownership lost on next heartbeat.

---

## Known Limitations (Out of Scope EPIC-01)

- `/project-open` not implemented → must seed projects via SQL
- No Brainstormer / natural-language workflow → returns "brainstormer not available"
- CUI adapter is stub → no real Telegram/chat integration
- ACP agents not wired → no actual coding tasks executed
- No TUI framework (Bubble Tea) → plain stdin/stdout REPL

---

## Quick Smoke Test (5 min)

```bash
# Build
go build -o codemint ./cmd/codemint

# Reset DB
rm -f ~/.local/share/codemint/database.db

# Run
./build/codemint
```

Inside REPL:
```
/help
/mode
/mode daemon
/help
/mode cli
/activity
/exit
```

Then check DB:
```bash
sqlite3 ~/.local/share/codemint/database.db "SELECT COUNT(*) FROM task WHERE type=3;"
```

Expected: count > 0 (commands logged as Coordination tasks).

If all above pass → EPIC-01 MVP is functional. Ready for EPIC-02 (Brainstormer + workflows).
