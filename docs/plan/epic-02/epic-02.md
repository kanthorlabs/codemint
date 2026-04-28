# CodeMint User Stories: EPIC-02 (Coding Workflow)

## Goal

A working **Coding Workflow** the user can run immediately. Eight phases, GROW-aligned, scoped tightly to v1 — no EPIC-03 / EPIC-04 / EPIC-05 plumbing in scope.

> **2026-04-28 redesign.** The earlier 11-story GROW pipeline (2.1–2.7 + 2.x.1 inserts) was scrapped: it sprawled, mixed v1 and v2 concerns, and lost the user's control over scope. The new spine maps 1:1 onto the spec below.

## Spec (authoritative)

```
1. Project Overview                       → 2.1
2. Clarify Goals       → GOAL             → 2.2
3. Goal-scoped context → REALITY          → 2.3
4. Suggest solutions   → OPTIONS          → 2.4
   5.1 user /modify  → back to #2 (Goal)
   5.2 user /confirm → next
6. Generate plan       → WAY FORWARD      → 2.5
   6.1 EPIC list   → DB
   6.2 STORY list  → DB
   6.3 TASK list   → DB
   6.4 Append SYSTEM-TASK: Verification + Confirmation
7. Worker picks first TASK; runs until a SYSTEM-TASK
8.1 If Verification    → run strategy     → 2.6
8.2 If Confirmation    → user / YOLO      → 2.7
```

Cross-cutting (see `appendings.md`):

- All progress reporting respects the `/verbosity` level (separate command, not a story).
- Any task `Failure` → reassign to **Human Agent**, pause workflow until human resolves → 2.8.

## Spine

| #     | Story                            | State        |
|-------|----------------------------------|--------------|
| 2.0   | Project & Session Bootstrap      | Done         |
| 2.0.1 | Workflow File Infrastructure     | Done         |
| 2.0.2 | Task Routing & Conditions        | Done         |
| 2.0.3 | Workflow Execution State         | Done         |
| 2.0.4 | Workflow Command                 | Done         |
| 2.0.5 | Skill Injection                  | Done         |
| 2.1   | Project Overview                 | Not started  |
| 2.2   | Goal Capture                     | Not started  |
| 2.3   | Goal-scoped Reality              | Not started  |
| 2.4   | Options + Confirm Loop           | Not started  |
| 2.5   | Plan Generation                  | Not started  |
| 2.6   | Verification Guardrail           | Not started  |
| 2.7   | Confirmation Guardrail           | Not started  |
| 2.8   | Error Escalation                 | Not started  |
| 2.9   | YOLO Agent Seed                  | Done         |
| 2.10  | YOLO Delegation                  | Not started  |

## WORKFLOW.yaml execution order

```
2.1 overview
  └── 2.2 goal_capture          ← /modify loops here
        └── 2.3 reality
              └── 2.4 options_and_confirm
                    │  /modify     → 2.2
                    │  /pick-option → 2.5
                    └── 2.5 plan
                          └── [scheduler runs each TASK]
                                ├── 2.6 verify  (auto-injected per story)
                                └── 2.7 confirm (auto-injected per story)
                                      │
                              any Failure → 2.8 escalate
```

## Stories

### 2.1 Project Overview
See `2.1-project-overview/index.md`.

Cheap full-tree pass: directory tree + a small fixed key-file set (`README.md`, `go.mod`, `Makefile`, top-level `package.json` / `pyproject.toml`). No grep, no recursive reads. Output is structured so 2.3 can append.

### 2.2 Goal Capture
See `2.2-goal-capture/index.md`.

Two passes: (1) one-sentence goal, (2) 1–5 testable success criteria. User locks via `/lock-goal`. Writes `workflow.goal_text` + `workflow.success_criteria` (columns from 2.0.3). Re-entered when user runs `/modify` in 2.4.

### 2.3 Goal-scoped Reality
See `2.3-goal-scoped-reality/index.md`.

Second pass, scoped to the locked Goal: grep keywords, read top hits, follow at most one import hop. Bounded by ~30k-token budget. Skips cleanly for greenfield Goals. Output appended to 2.1's structured context.

### 2.4 Options + Confirm Loop
See `2.4-options-and-confirm/index.md`.

Proposes 2–3 candidate approaches with tradeoffs (or 1 with explicit rationale).

- `/pick-option <id>` → writes `workflow.chosen_option`, advances to 2.5.
- `/modify` → clears `goal_text` + `success_criteria` + `chosen_option`, re-runs 2.2 → 2.3 → 2.4.

### 2.5 Plan Generation
See `2.5-plan-generation/index.md`.

Reads `chosen_option`, emits an Epic → Story → Task JSON plan. Tasks inserted into SQLite with `seq_epic`/`seq_story`/`seq_task` (flat schema, no Epic/Story tables). Auto-injects 2.6 Verification + 2.7 Confirmation per story.

### 2.6 Verification Guardrail
See `2.6-verification-guardrail/index.md`.

Auto-injected after each coding story. `TaskTypeVerification`. Runs configured command (default `go test ./...`). Non-zero exit → task `Failure` → 2.8 Error Escalation.

### 2.7 Confirmation Guardrail
See `2.7-confirmation-guardrail/index.md`.

Auto-injected at end of each story. `TaskTypeConfirmation`. Assigned to Human Agent (or `sys-auto-approve` under YOLO). Session transitions to `Awaiting`. Reject → task `Failure` → 2.8 Error Escalation.

### 2.8 Error Escalation
See `2.8-error-escalation/index.md`.

Any task that lands in `Failure` is automatically reassigned to the session's Human Agent. Scheduler halts. Human resolves via `/resolve retry|skip|abort`.

### 2.9 YOLO Agent Seed (DONE)
`agentRepo.EnsureSystemAgents` seeds `sys-auto-approve`.

### 2.10 YOLO Delegation
See `2.10-yolo-mode-delegation/index.md`.

`/yolo epic <id>` / `/yolo story <id>` reassigns that scope's Confirmation tasks (2.7) to `sys-auto-approve`. Scheduler auto-Successes those gates. Coding (2.5 outputs) and Verification (2.6) are unaffected.

## What was dropped (2026-04-28 redesign)

| Old story | Drop reason |
|---|---|
| 2.2 Living Spec | Clarification absorbed by 2.2 Goal + 2.3 Reality + 2.4 /modify loop. |
| 2.4.1 Goal Verification | Out of scope for v1; 2.6 covers story-level correctness. |
| 2.6 Epic Retrospective | EPIC-05 territory. |
| 2.7 Human Review & Activation | Confirm gate is 2.4 itself; no separate activation step. |
| 2.8 Mid-Flight Pivots | The 2.4 `/modify` loop is the only pivot v1 supports. |
| 2.11 Wiki Injection | EPIC-05 support. |
| 2.12 Clarifier Handoff | EPIC-04 support. |
| 2.13 ACP Payload | EPIC-03 support. |

The corresponding folders were removed from `docs/plan/epic-02/` in the same redesign commit.

## Priority

| Priority | Story | Rationale |
|---|---|---|
| P0 | 2.1 | First step of every workflow run |
| P0 | 2.2 | GROW Goal — anchors everything downstream |
| P0 | 2.3 | GROW Reality — feeds Options |
| P0 | 2.4 | GROW Options + the `/modify` loop |
| P0 | 2.5 | Plan generation; produces the executable backlog |
| P0 | 2.6 | Per-story correctness gate |
| P0 | 2.7 | Per-story approval gate |
| P0 | 2.8 | Error escalation — required by appendings |
| P1 | 2.10 | YOLO autonomy — quality-of-life |
