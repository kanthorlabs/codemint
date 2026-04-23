# CodeMint PRD & Engineering Spec: EPIC-05 (Adaptive Learning System)

## 1. Overview
EPIC-05 introduces **The Archivist**, an agent dedicated to memory compaction and pattern recognition. It transitions CodeMint from a stateless orchestrator to an adaptive system that learns from bug fixes, decisions, and human preferences using a pure, version-controllable Markdown "LLM Wiki" architecture.

---

## 2. Storage Architecture (The LLM Wiki)
All memory is stored locally following the XDG Base Directory Specification, strictly using Markdown files. No hidden JSON files are permitted for knowledge storage.

**Base Path:** `~/.local/share/codemint/memory/<project_id>/`

### 2.1 Directory Structure
* `inbox/insights/`: The staging ground for human review.
  * `index.md`: The "Hot" inbox. It contains the **full text and context** of the most recent pending insights, not just a list. The UI reads this single file to instantly render the `/review` interface.
  * `unverified/<name>-<yyyyMMDD>.md`: Standalone insight files. These act as the backend state trackers. The date stamp ensures deterministic TTL (Time-To-Live) calculations.
  * `archive/`: Insights neglected for >14 days are moved here, keeping the `unverified/` directory and `index.md` clean.
* `history/`: Contains `<epic_number>-<slug>.md` files logging the chronological narrative, decisions, and exceptions made during specific Epics.
* `architecture/`:
  * `decisions.md`: "Hot" active Architecture Decision Records (ADRs). Contains an index + the full text of the last 10 ADRs.
  * `archive/`: "Cold" ADRs that are settled and no longer need daily context injection.
* `patterns/`:
  * `preferences.md`: Consolidated style and workflow preferences.
  * `bugs/index.md`: Fast-lookup table mapping bug short-names to one-line resolutions.
  * `bugs/<short-name>.md`: Deep-dive post-mortems of specific bugs.

---

## 3. The Compaction Pipeline (The Archivist)

### 3.1 Data Aggregation
At the completion of an Epic, The Archivist queries the SQLite database for high-signal events: tasks with explicit human feedback, reverted tasks, or major milestone completions.

### 3.2 Synthesis & Inbox Routing
1. The Archivist summarizes the raw database events.
2. It cross-references the summary against existing `preferences.md` and `decisions.md`.
3. It generates targeted Markdown files (e.g., `inbox/insights/unverified/pref-use-jwt-20260422.md`).
4. It prepends the **full content** of this new insight directly into `inbox/insights/index.md` for immediate UI rendering.

---

## 4. The Knowledge Inbox (Review UX)

### 4.1 Push Notifications (Configurable)
The Go backend utilizes a cron-style scheduler (`config.yaml` -> `memory.review_alert`). When triggered, CodeMint displays a non-intrusive alert (e.g., `[ 🔔 Review Project Memory ]` in the TUI, or a brief `/status` append in the CUI).

### 4.2 Pull Interaction (`/review`)
The human dictates when learning happens.
* **Review UI:** Executing `/review` prompts the UI to read `inbox/insights/index.md` and directly display the rich content of pending items.
* **Action:** The user can `[Accept]`, `[Dismiss]`, or `[Edit]`. Accepted items are merged by The Archivist into the core `patterns/` or `architecture/` files.
* **State Sync:** Upon action, the item is removed from `index.md` and the corresponding `<name>-<yyyyMMDD>.md` file is deleted or moved.
* **Neglected Inbox TTL:** A daily Go routine checks the `<yyyyMMDD>` filenames in the `unverified/` folder. Anything older than 14 days is moved to `inbox/insights/archive/` and its content is purged from `index.md`.

---

## 5. Context Injection & Guardrails

### 5.1 Injection Phase
When a Session boots, CodeMint reads the "Hot" Wiki files (`preferences.md`, `decisions.md`, `bugs/index.md`) and injects them into the System Prompts of both the Brainstormer Agent and the Execution Agent (OpenCode).

### 5.2 Hierarchy of Authority (Conflict Resolution)
To prevent prompt collisions, the LLM System Prompt strictly enforces precedence:
1. **Current Prompt / Living Spec** (Highest Authority)
2. **Project Memory / LLM Wiki** (Medium Authority)
3. **Global CodeMint Rules** (Lowest Authority)
*Note: Overrides dictated by the Current Prompt are logged by The Archivist as temporary "Exceptions" in the Epic's history, not as permanent preference changes.*

### 5.3 TODO: Scoped Memory Injection
*Future Implementation:* As `preferences.md` and `decisions.md` grow, injecting the entirety of these files will exhaust context windows. Future iterations must implement semantic or scope-based filtering to only inject memory relevant to the files/stack currently being modified.
