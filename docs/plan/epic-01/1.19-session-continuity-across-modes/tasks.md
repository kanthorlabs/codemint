# Tasks: 1.19 Session Continuity Across TUI/CUI Modes

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.19-session-continuity-across-modes/`
**Tech Stack:** Go, SQLite, context, sync
**Priority:** P1 (Core workflow enabler)

---

## Task 1.19.1: Add Session Ownership Columns

* **Action:** Add `active_client` and `last_activity_at` columns to session table.
* **Details:**
  * Add migration in `internal/db/migrations.go`:
    ```sql
    ALTER TABLE session ADD COLUMN active_client TEXT;
    ALTER TABLE session ADD COLUMN last_activity_at INTEGER;
    ```
  * `active_client` format: `"{mode}:{uuid}"` e.g. `"cli:abc123"` or `"daemon:xyz789"`.
  * `last_activity_at`: Unix timestamp (seconds) for staleness detection.
  * NULL `active_client` means session is idle (no active client).
  * Update `internal/domain/session.go` with new fields.
  * Update `internal/repository/sqlite/session_repo.go` to read/write new columns.
* **Verification:**
  * Migration runs without error on existing DB.
  * `SessionRepo.Update()` persists `active_client` and `last_activity_at`.
  * Query returns correct values after update.

## Task 1.19.2: Persist ActiveSession State to Database

* **Action:** Add methods to save and load runtime session state.
* **Details:**
  * Add `SessionRepo.SaveState(ctx, sessionID, activeClient, lastActivityAt)` method.
  * Add `SessionRepo.GetMostRecentActive(ctx) (*Session, error)` method:
    ```sql
    SELECT * FROM session 
    WHERE status = 0 
    ORDER BY last_activity_at DESC NULLS LAST 
    LIMIT 1
    ```
  * Add `SessionRepo.ClearOwnership(ctx, sessionID)` to set `active_client = NULL`.
  * Call `SaveState` on: task completion, REPL command dispatch, heartbeat tick.
* **Verification:**
  * `GetMostRecentActive` returns session with latest `last_activity_at`.
  * Multiple sessions: returns correct one.
  * No active sessions: returns `nil, nil` (not error).

## Task 1.19.3: Auto-Resume on Startup

* **Action:** Modify `cmd/codemint/main.go` to auto-load active session.
* **Details:**
  * After migrations, call `SessionRepo.GetMostRecentActive(ctx)`.
  * If found:
    1. Load project via `ProjectRepo.Get(ctx, session.ProjectID)`.
    2. Populate `ActiveSession` with loaded data.
    3. Generate client ID: `fmt.Sprintf("%s:%s", clientMode, uuid.New())`.
    4. Perform handoff check (see 1.19.4).
    5. Display: `"Resuming session {id} for project {name}"`.
  * If not found:
    1. Start in global mode (`IsGlobal=true`).
    2. Display: `"No active session. Use /project-open to start."`.
  * Create `internal/orchestrator/session_loader.go` for reusable logic.
* **Startup order:**
  ```
  xdg.EnsureDirs() → db.Open() → migrations → SessionRepo.GetMostRecentActive()
  → load project if found → create ActiveSession → start REPL
  ```
* **Verification:**
  * Fresh DB: starts in global mode with correct message.
  * Existing active session: auto-loads and displays resume message.
  * Session with NULL project_id: handled gracefully (global mode).

## Task 1.19.4: Session Handoff Protocol

* **Action:** Implement client takeover when multiple clients connect.
* **Details:**
  * On session load, check `active_client` and `last_activity_at`.
  * Staleness threshold: 60 seconds.
  * If stale (`now - last_activity_at > 60s`): auto-takeover silently.
  * If fresh: auto-takeover anyway (latest client wins), but notify old client.
  * Add `EventSessionTakeover` to `registry.UIEventType`:
    ```go
    EventSessionTakeover UIEventType = "session_takeover"
    ```
  * Old client receives event via `UIMediator.NotifyAll()` and enters **suspended mode**.
  * **Suspended mode behavior:**
    * Client still running, but not actively owning session.
    * Display message: `"Session taken over by {other_client}. Type anything to reclaim."`
    * On any user input: trigger reclaim (takeover back).
    * No restart required — just type to reclaim.
  * Add `EventSessionReclaimed` for when suspended client takes back control.
  * Create `internal/orchestrator/heartbeat.go`:
    * Goroutine that ticks every 15 seconds.
    * Calls `SessionRepo.SaveState()` with current timestamp.
    * Stops when context canceled.
* **Verification:**
  * Start TUI, start CUI → CUI takes over, TUI shows "Session taken over..." message.
  * Type in suspended TUI → TUI reclaims, CUI receives `EventSessionTakeover`.
  * Heartbeat updates `last_activity_at` every 15s.

## Task 1.19.5: Implement `/session-resume <id>` Command

* **Action:** Create command for switching to a different session.
* **Details:**
  * `/session-resume` (no args): list all active sessions.
    ```
    Active sessions:
      abc123 - myproject (current)
      def456 - otherproject
    ```
  * `/session-resume <id>`: switch to specified session.
    1. Clear ownership on current session (`active_client = NULL`).
    2. Load target session and project.
    3. Perform handoff check.
    4. Update `ActiveSession` in memory.
    5. Display: `"Switched to session {id} for project {name}"`.
  * Add to `internal/repl/session_commands.go`.
  * Register with `SupportedModes: []ClientMode{ClientModeCLI, ClientModeDaemon}`.
* **Verification:**
  * `/session-resume` lists sessions with current marker.
  * `/session-resume <valid-id>` switches successfully.
  * `/session-resume <invalid-id>` returns "session not found" error.

## Task 1.19.6: Dynamic Mode Enforcement

* **Action:** Support mode changes at runtime without restart.
* **Details:**
  * Add `/mode` command in `internal/repl/mode_commands.go`.
  * `/mode`: show current mode.
  * `/mode cli`: switch to CLI mode (only if stdout is TTY).
  * `/mode daemon`: switch to daemon mode (always valid).
  * Update `ActiveSession.ClientMode` in memory.
  * Persist mode to session state via `SaveState`.
  * Re-filter `/help` output after mode change.
  * Register with `SupportedModes: []ClientMode{ClientModeCLI, ClientModeDaemon}`.
* **Verification:**
  * `/mode` displays current mode.
  * `/mode daemon` switches mode, `/help` shows daemon-compatible commands only.
  * `/mode cli` in non-TTY context returns error.

## Task 1.19.7: CUI Adapter Scaffold

* **Action:** Create placeholder CUI adapter for daemon mode.
* **Details:**
  * Create `internal/ui/cui_adapter.go`:
    ```go
    type CUIAdapter struct {
        // Placeholder for Telegram bot, WebSocket, etc.
    }
    
    func (a *CUIAdapter) NotifyEvent(event registry.UIEvent) {
        // TODO: EPIC-02 - Send to chat interface
    }
    
    func (a *CUIAdapter) PromptDecision(ctx context.Context, req registry.PromptRequest) registry.PromptResponse {
        // TODO: EPIC-02 - Display prompt in chat, await response
        <-ctx.Done()
        return registry.PromptResponse{Error: ui.ErrPromptCanceled}
    }
    ```
  * Register with mediator in `main.go` when `--mode=daemon`.
  * Create `internal/ui/cui_adapter_test.go` with interface compliance test.
* **Verification:**
  * `CUIAdapter` compiles and satisfies `ui.UIAdapter` interface.
  * Daemon mode registers adapter with mediator.
  * Test confirms interface satisfaction.

## Task 1.19.8: Add `client_id` Column to Task Table

* **Action:** Extend `task` table to track which client created each task.
* **Details:**
  * Add migration in `internal/db/migrations.go`:
    ```sql
    ALTER TABLE task ADD COLUMN client_id TEXT;
    ```
  * `client_id` format: `"{mode}:{uuid}"` e.g. `"cli:abc123"` or `"daemon:xyz789"`.
  * NULL for tasks created by AI agents (not user-initiated).
  * Update `internal/domain/task.go` with new field.
  * Update `internal/repository/sqlite/task_repo.go` to read/write `client_id`.
* **Verification:**
  * Migration runs without error on existing DB.
  * `TaskRepo.Create()` persists `client_id`.
  * Existing tasks have NULL `client_id` (backward compatible).

## Task 1.19.9: Record Interactions as Coordination Tasks

* **Action:** Persist every user command/message as a Coordination task (`type=3`).
* **Details:**
  * Reuse existing `task` table following "Human-as-an-Agent" pattern:
    * `type` = `3` (Coordination)
    * `assignee_id` = `human` agent ID
    * `input` = JSON with user command/message
    * `output` = JSON with system response
    * `status` = `5` (completed) immediately
    * `client_id` = current client identifier
  * Update `Dispatcher.Dispatch()` to record after execution:
    ```go
    task := &domain.Task{
        ID:         uuid.NewV7(),
        SessionID:  active.Session.ID,
        AssigneeID: humanAgentID,
        Type:       domain.TaskTypeCoordination,
        Status:     domain.TaskStatusCompleted,
        Input:      json.Marshal(InputPayload{Command: input}),
        Output:     json.Marshal(OutputPayload{Message: result.Message}),
        ClientID:   active.ClientID,
    }
    taskRepo.Create(ctx, task)
    ```
  * Add `ClientID` field to `ActiveSession` struct.
  * Add `TaskRepo.ListCoordinationAfter(ctx, sessionID, afterTaskID)` method:
    ```sql
    SELECT * FROM task 
    WHERE session_id = ? AND type = 3 AND id > ?
    ORDER BY id ASC
    ```
  * UUIDv7 lexicographical ordering = timestamp ordering (no separate timestamp column needed).
* **Verification:**
  * Run `/help` → task row created with type=3, status=5.
  * Run natural language → task row created similarly.
  * `client_id` correctly identifies which client.

## Task 1.19.10: Display Missed Interactions on Reconnection

* **Action:** Show missed activity when client reclaims session.
* **Details:**
  * Track `last_seen_task_id` (UUIDv7) per client in `ActiveSession`.
  * On session reclaim (suspended client types):
    1. Query `TaskRepo.ListCoordinationAfter(ctx, sessionID, lastSeenTaskID)`.
    2. Filter to tasks where `client_id != current_client_id`.
    3. If found, display header: `"Activity while you were away:"`.
    4. Render each interaction (extract timestamp from UUIDv7):
       ```
       [daemon:xyz @ 14:32] /status
       > Project: myproject, Tasks: 3 pending
       
       [daemon:xyz @ 14:35] fix the login bug
       > Created task t_abc: Fix login bug
       ```
    5. Update `last_seen_task_id` to latest task ID.
  * On fresh startup (Task 1.19.3), also check for missed activity.
  * Add `/activity` command to manually view recent Coordination tasks.
* **Verification:**
  * TUI suspended → CUI runs commands → TUI reclaims → TUI shows CUI activity.
  * No missed activity → no "Activity while you were away" message.
  * `/activity` shows last N Coordination tasks.
  * Timestamp displayed correctly extracted from UUIDv7.

---

## Dependencies

| Task | Depends On |
|------|------------|
| 1.19.2 | 1.19.1 |
| 1.19.3 | 1.19.2 |
| 1.19.4 | 1.19.2 |
| 1.19.5 | 1.19.3 |
| 1.19.6 | 1.19.3 |
| 1.19.7 | None (can parallel) |
| 1.19.8 | None (schema migration, can parallel) |
| 1.19.9 | 1.19.8, 1.19.3 |
| 1.19.10 | 1.19.4, 1.19.9 |

---

## User Flow

```
[First time]
$ codemint
> No active session. Use /project-open to start.
> /project-open myproject
> Opened project "myproject", session ses_1

[User works on TUI]
> /status
> Project: myproject, Tasks: 0 pending
> fix the login bug
> Created task t_abc: Fix login bug

[Leave desk, open CUI on mobile]
$ codemint --mode=daemon
> Resuming session ses_1 for project "myproject"
> (TUI shows: "Session taken over by daemon:xyz. Type anything to reclaim.")

[Work on CUI while away]
> /status
> Project: myproject, Tasks: 1 pending
> add unit tests for auth
> Created task t_def: Add unit tests for auth

[Return to desk, type in TUI to reclaim]
TUI> hello
> Activity while you were away:
> 
> [daemon:xyz @ 15:42] /status
> > Project: myproject, Tasks: 1 pending
> 
> [daemon:xyz @ 15:45] add unit tests for auth
> > Created task t_def: Add unit tests for auth
>
> Session reclaimed. Continuing session ses_1.
> (CUI shows: "Session taken over by cli:abc. Type anything to reclaim.")

[Continue working on TUI]
> /status
> Project: myproject, Tasks: 2 pending

[Switch to different project]
> /session-resume
> Active sessions:
>   ses_1 - myproject (current)
>   ses_2 - otherproject
> /session-resume ses_2
> Switched to session ses_2 for project "otherproject"
```

---

## Out of Scope

* Actual CUI implementation (Telegram bot, WebSocket) — EPIC-02
* Concurrent editing (two clients writing same file) — requires lock or CRDT
* Real-time sync between clients — EPIC-02
