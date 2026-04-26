# Manual Test: CUI Adapter Daemon Activation

This document provides manual verification steps for Story 3.17 - CUI Adapter Activation in Daemon Mode.

## Prerequisites

- Build the codemint binary: `go build -o build/codemint ./cmd/codemint`
- Ensure no other codemint instances are running

## Test 1: Verify Daemon Mode Creates CUIAdapter

**Steps:**

1. Start codemint in daemon mode:
   ```bash
   ./build/codemint --mode=daemon
   ```

2. Verify the CUI log file is created:
   ```bash
   ls -la ~/.local/state/codemint/cui.log
   ```

3. Send a test event (e.g., open a project):
   ```
   /project-open <your-project-path>
   ```

4. Check the log file for entries:
   ```bash
   tail -f ~/.local/state/codemint/cui.log
   ```

**Expected Result:** Log file exists and contains event entries.

## Test 2: Verify /status Command Shows Pending Prompts

**Steps:**

1. Start codemint in daemon mode:
   ```bash
   ./build/codemint --mode=daemon
   ```

2. Run the status command:
   ```
   /status
   ```

**Expected Result:** Status output shows session, worker, and pending approvals sections.

## Test 3: Verify /approve and /deny Commands Work in Daemon Mode

**Prerequisites:** A session with a pending ACP prompt (requires ACP worker running).

**Steps:**

1. Start codemint in daemon mode:
   ```bash
   ./build/codemint --mode=daemon
   ```

2. Trigger an action that requires approval (e.g., a file modification via `/acp`).

3. Check for pending prompts:
   ```
   /status
   ```

4. Approve or deny the prompt:
   ```
   /approve <prompt-id> <option-id>
   # or
   /deny <prompt-id>
   ```

**Expected Result:** Prompt is resolved and the ACP worker continues.

## Test 4: Verify CLI Mode Does Not Have CUIAdapter

**Steps:**

1. Start codemint in CLI mode (default):
   ```bash
   ./build/codemint
   ```

2. Try to run /approve:
   ```
   /approve 1 allow_once
   ```

**Expected Result:** Message says "Run with --mode=daemon to use this command."

## Test 5: Verify Graceful Shutdown

**Steps:**

1. Start codemint in daemon mode:
   ```bash
   ./build/codemint --mode=daemon
   ```

2. Send SIGINT (Ctrl+C) or run `/exit`.

3. Check that the cui.log file is properly closed (no corruption).

**Expected Result:** Clean shutdown with no errors about log file.

---

## Verification Checklist

- [ ] Daemon mode creates CUIAdapter and registers it with mediator
- [ ] TUI adapter is NOT registered in daemon mode
- [ ] CUI log file is written to XDG state directory
- [ ] /status command shows pending prompts from CUIAdapter
- [ ] /approve command resolves pending prompts
- [ ] /deny command denies pending prompts
- [ ] CLI mode shows mode hint when using /approve or /deny
- [ ] Graceful shutdown properly closes the adapter
