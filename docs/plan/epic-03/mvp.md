# EPIC-03 MVP Test Plan

This document is the manual acceptance script for the ACP Execution Layer (Stories 3.1–3.23). Run it after the last story merges to confirm the layer behaves end-to-end. Each suite maps to one or more stories from `epic-03.md`; the suite numbering is independent of story numbering so the script reads top-to-bottom.

Anything in the **Pass criteria** column is a hard gate — if it fails, EPIC-03 is not shippable.

---

## 0. Prerequisites

### Tooling
- Go 1.25+, `make`, `sqlite3` CLI (for inspecting the DB).
- At least one ACP provider binary on `PATH`. The default is **OpenCode** (`opencode acp`); `codex` and `claude-code` are also accepted.
- A throwaway git-initialized project directory to act as the test workspace, e.g. `~/tmp/codemint-mvp-project`.
  - `git init` is required because Coding-type projects gate on `git rev-parse --is-inside-work-tree`.

### Build & config
```sh
make build-race                 # race detector on for the whole MVP run
mkdir -p ~/.config/codemint
cp configs/config.yaml.example ~/.config/codemint/config.yaml
```

Edit `~/.config/codemint/config.yaml` to enable a non-default provider/model so Story 3.22 / 3.23 can be exercised:

```yaml
providers:
  - name: opencode
  - name: codex

assistants:
  system:
    provider: opencode
    model: github-copilot/claude-sonnet-4.6
```

### State reset between suites
```sh
rm -f ~/.local/share/codemint/codemint.db                  # nuke state
ps -A | grep -E 'opencode|codex|claude-code' | grep -v grep  # must be empty before each launch
```

---

## 1. Persistent Worker Lifecycle (Stories 3.1, 3.2, 3.3)

| Step | Action | Pass criteria |
|---|---|---|
| 1.1 | Launch `./build/codemint -mode cli`, run `/acp hello`. | A single `opencode` (or configured provider) child is spawned. PID is stable across multiple `/acp` calls in the same session. |
| 1.2 | Issue `/acp-reset` (story-boundary clear). | Worker PID **unchanged**; agent reports a fresh context window on the next prompt. |
| 1.3 | Send `SIGINT` (Ctrl+C). | Process logs `Shutting down gracefully`, `acpRuntime.Shutdown` runs within 5 s, no orphan ACP child remains (`pgrep opencode` empty). |
| 1.4 | Re-launch and run `/session-archive`. | Worker for that session is reaped; child process exits before the REPL prompt returns. |

---

## 2. Interceptor & Permission (Stories 3.4, 3.5, 3.6)

Pre-seed `project_permission` for the active project via SQLite:

```sh
sqlite3 ~/.local/share/codemint/codemint.db <<'SQL'
UPDATE project_permission
SET allowed_commands  = json('["go test","ls"]'),
    blocked_commands  = json('["rm -rf","curl"]')
WHERE project_id = (SELECT id FROM project ORDER BY rowid DESC LIMIT 1);
SQL
```

| Step | Action | Pass criteria |
|---|---|---|
| 2.1 | Prompt the agent to run `go test ./...`. | Tool call is intercepted, executed locally, output piped back to the agent on stdin. Task stays in `processing`. No UI prompt fires. |
| 2.2 | Prompt the agent to run `rm -rf /tmp/foo`. | Stream halts, task transitions to `awaiting`, mediator broadcasts a permission prompt. |
| 2.3 | Prompt the agent to run `cat README.md` (no match either list). | Same as 2.2 — defaults to manual approval. |
| 2.4 | `/approve <prompt-id> allow_once` resumes; `/deny <prompt-id>` aborts and the task lands at `failure`. | Status transitions match `epic-03.md` §4.1. |

---

## 3. Status Mapping (Story 3.7)

Watch DB transitions in another shell:

```sh
watch -n 0.5 'sqlite3 ~/.local/share/codemint/codemint.db \
  "SELECT id, type, status FROM task ORDER BY rowid DESC LIMIT 5;"'
```

| Event from agent | Expected DB status |
|---|---|
| `session.start` / `turn-start` | `processing` (1) |
| `tool-call` (not whitelisted) | `awaiting` (2) |
| `human-input-request` | `awaiting` (2) |
| `turn-end` / `task-complete` | `success` (3) |
| Worker error / non-zero exit | `failure` (4) |

---

## 4. TUI vs CUI Streaming (Stories 3.8, 3.9)

| Step | Action | Pass criteria |
|---|---|---|
| 4.1 | `./build/codemint -mode cli`, run `/acp explain this repo`. | TUI streams `agent_thought_chunk` and `tool_call` updates in real time. |
| 4.2 | `./build/codemint -mode daemon` (in a second shell, same DB). | CUI prints **only** terminal state pulses — `awaiting`, `success`, `failure`. No thinking spam. |
| 4.3 | `/tasks` and `/status` from the daemon REPL. | Show the running task and current session state. |

---

## 5. Circular Buffer & `/summary` (Story 3.10)

| Step | Action | Pass criteria |
|---|---|---|
| 5.1 | Run a multi-step `/acp` prompt that produces ≥ 20 update events. | Events captured by `BufferRegistry`. |
| 5.2 | `/summary <task_id>`. | One coherent `<thinking>` block aggregated from the buffer; no raw JSON-RPC noise. |
| 5.3 | `/summary` with no arg while another task is running. | Falls back to the most recent `processing` or `awaiting` task. |
| 5.4 | `/summary` with no active and no recent task. | Friendly error, no crash. |

---

## 6. Agent Memory Injection (Story 3.11)

| Step | Action | Pass criteria |
|---|---|---|
| 6.1 | Drop a `CLAUDE.md` (or equivalent) into the project root with a unique sentence (`THE_SECRET_TOKEN_IS_42`). | — |
| 6.2 | Start a new session, ask the agent: "what is the secret token?". | Agent responds with `42` on the first turn — proves the memory loader injected the file into the session. |

---

## 7. Runtime Wiring (Story 3.12)

| Step | Action | Pass criteria |
|---|---|---|
| 7.1 | Launch the REPL, **without** running `/acp`, then trigger a Coding task via the scheduler (Suite 8). | Pipeline / Interceptor / StatusMapper / Fanout / BufferRegistry / PipelineConsumer all spin up automatically; the task progresses without manual `/acp` poking. |
| 7.2 | `/acp-status`. | Reports the consumer/interceptor/pipeline as `running` for the active session. |

---

## 8. Pending-Task Scheduler (Story 3.13)

Seed three pending tasks in increasing `(seq_epic, seq_story, seq_task)` order, two of type Coding and one of type Confirmation between them:

```sh
sqlite3 ~/.local/share/codemint/codemint.db < scripts/seed-mvp-tasks.sql  # author this helper if missing
```

| Step | Action | Pass criteria |
|---|---|---|
| 8.1 | Watch the DB. | Tasks execute in strict seq order. |
| 8.2 | The Confirmation task. | Pauses the loop until `/approve` is issued; subsequent Coding task does not start meanwhile. |
| 8.3 | At a Story boundary. | Scheduler triggers `/acp-reset` automatically (Story 3.2 hook). |

---

## 9. Executor Type Routing (Story 3.14)

For each task type, seed one row and confirm the routing:

| Type | Expected execution path |
|---|---|
| Coding (0) | Dispatched to ACP worker; produces agent updates. |
| Verification (1) | Runs via `LocalRunner` (e.g. `go test ./...`); no ACP traffic. |
| Confirmation (2) | Pauses, mediator broadcast prompt; `/approve` or `/deny` advances. |
| Coordination (3) | No execution. Recorded in `task.output`. |
| Retrospective (4) | Conversational prompt rendered via mediator (`freeform` kind from Story 3.18). |

---

## 10. ACP Payload Consumption (Story 3.15)

| Step | Action | Pass criteria |
|---|---|---|
| 10.1 | Insert a Coding task with `task.input` containing `context_files: ["./README.md", "./go.mod"]`. | Files resolved relative to `project.working_dir` and sent in `acp.SessionPromptParams`. |
| 10.2 | Try `context_files: ["../../etc/passwd"]`. | Task lands at `failure` with a clear "path escape" error; agent never sees the path. |
| 10.3 | Malformed JSON in `task.input`. | Task → `failure`, error logged, scheduler proceeds with the next task. |

---

## 11. YOLO Auto-Approval (Story 3.16)

| Step | Action | Pass criteria |
|---|---|---|
| 11.1 | Insert a Confirmation task with `assignee_id = '<sys-auto-approve agent ID>'`. | Auto-approved without a UI prompt. `task.output` contains the bypass audit record. |
| 11.2 | Same with type Retrospective. | Auto-approved identically. |
| 11.3 | Same `assignee_id` but type Coding. | NOT auto-approved — YOLO only short-circuits Confirmation/Retrospective per `domain/core.go` `Task` doc. |

---

## 12. CUI Adapter Daemon Activation (Story 3.17)

| Step | Action | Pass criteria |
|---|---|---|
| 12.1 | `./build/codemint -mode daemon`. | `BuildAdapters(ClientModeDaemon)` registers `CUIAdapter`; `TUIAdapter` not registered (verify via `/activity` and absence of streaming output). |
| 12.2 | `./build/codemint -mode cli`. | Inverse — TUI registered, CUI not. |
| 12.3 | Daemon mode prompt (`awaiting` task). | `/approve` / `/deny` / `/reply` resolve via the CUI adapter only. |

---

## 13. Mediator First-In-Wins Broadcast (Story 3.18)

| Step | Action | Pass criteria |
|---|---|---|
| 13.1 | `./build/codemint -mode hybrid`. | Both TUI and CUI register on the same mediator. |
| 13.2 | Trigger an approval prompt; respond from the TUI **first**. | TUI response wins; CUI receives a `CancelPrompt` and clears its pending entry from `/tasks`. |
| 13.3 | Repeat, reply from CUI first. | CUI wins, TUI cancels. |
| 13.4 | Trigger a Retrospective task. | `freeform` prompt kind reaches both adapters; either can answer. |

---

## 14. System Assistant Conversational Pipeline (Story 3.19)

| Step | Action | Pass criteria |
|---|---|---|
| 14.1 | Launch with `-with-assistant` (default). Type a freeform sentence (no leading `/`). | Routed to the System Assistant; reply broadcast as `EventChatChunk` on every registered adapter. |
| 14.2 | Inspect DB. | A Coordination task is persisted for the user message (audit trail). |
| 14.3 | Launch with `-with-assistant=false`. | Freeform input returns `ErrSystemAssistantDisabled`; slash commands still work. |

---

## 15. Hybrid Adapter Mode (Story 3.20)

| Step | Action | Pass criteria |
|---|---|---|
| 15.1 | `-mode hybrid`. | TUI owns stdin; CUI receives every broadcast. |
| 15.2 | Run `/acp …` from the TUI. | Streaming visible in TUI; CUI sees only state pulses. |
| 15.3 | Send a reply from the CUI side via the input multiplexer (Suite 16). | Dispatcher accepts it as a non-stdin source. |

---

## 16. Inbound Adapter Multiplexer (Story 3.21)

The CUI ships with a stub inbound backend (`cui_inbound_stub.go`) so this can be tested without Telegram.

| Step | Action | Pass criteria |
|---|---|---|
| 16.1 | Push a message via the stub: `cuiInbound.Inject(InboundMessage{Source: "stub", UserID: "u1", Text: "ping"})`. | Dispatcher receives it via `MuxDispatcher.DispatchInbound`. |
| 16.2 | Inspect the resulting Coordination task. | `Source` and `UserID` recorded in the audit metadata. |
| 16.3 | Stop the stub backend mid-run. | Multiplexer keeps serving stdin without panic. |

---

## 17. Provider Registry (Story 3.22)

| Step | Action | Pass criteria |
|---|---|---|
| 17.1 | `/providers`. | Lists all providers from the merged catalog (built-ins + config overrides). |
| 17.2 | Stop the binary, edit config to set `assistants.system.provider: codex`, restart. | System Assistant resolves to `codex` without code changes. |
| 17.3 | Set `assistants.system.provider: bogus`. | Startup logs a warning and disables the System Assistant; slash commands still work. |
| 17.4 | `CODEMINT_ACP_CMD=/path/to/fake-acp ./build/codemint`. | Env override wins over config; provider name resolves to `env-override`. |

---

## 18. Assistants Config + Per-Provider Model Override (Story 3.23)

| Step | Action | Pass criteria |
|---|---|---|
| 18.1 | Set `assistants.system.model: github-copilot/claude-sonnet-4.6`. | Worker spawns as `opencode acp --model github-copilot/claude-sonnet-4.6` (verify via `ps -ef`). |
| 18.2 | Add a legacy `agents:` section to the config. | Config validation fails or logs that the section is ignored — confirm `agents:` is dropped per Story 3.23. |
| 18.3 | Override `Provider.ModelFlag` for a custom provider in config. | Spawn args reflect the override. |

---

## 19. End-to-End Smoke (everything together)

Run with a clean DB:

```sh
rm -f ~/.local/share/codemint/codemint.db
./build/codemint -mode hybrid
```

1. Type a freeform question → System Assistant responds, Coordination task recorded.
2. Issue `/acp implement a hello-world function`.
3. Whitelisted tool calls run silently; one non-whitelisted call pauses for `/approve`.
4. `/summary` returns a clean aggregated thinking block.
5. `/session-archive` reaps the worker.
6. `SIGINT`. No orphan child processes.

If all 19 suites pass, EPIC-03 MVP is signed off. File any partial failures as follow-up tickets referencing the suite number above.
