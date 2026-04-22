# CodeMint PRD & Engineering Spec: EPIC-04 (Multi-Interface Support)

## 1. Overview
EPIC-04 defines the presentation layer of CodeMint. It establishes a unified Go `UIAdapter` interface and a `UIMediator` that pipes orchestration data to a high-bandwidth local Terminal UI (Bubble Tea) and a low-bandwidth, minimalist Chat UI (Telegram/Slack) simultaneously.

---

## 2. The Universal Interface Contract & The Mediator
The core orchestrator does not know about terminal rendering or Telegram APIs. UI interactions are entirely decoupled using the **Mediator Pattern**.

### 2.1 The `UIAdapter` Interface
Both the TUI and CUI implement a standard interface:
```go
type UIAdapter interface {
    NotifyEvent(event UIEvent) // Push informational updates (based on Verbosity)
    PromptDecision(prompt PromptData) (Response, error) // Blocking request for human input
    CancelPrompt(promptID string) // Instructs the UI to instantly dismiss a pending prompt
}
```

### 2.2 The `UIMediator`
To prevent race conditions when multiple UIs (e.g., Terminal and Telegram) are active simultaneously, a central `UIMediator` manages the adapters.

- The Core Orchestrator calls `PromptDecision` on the `UIMediator`.
- The Mediator broadcasts the request to all registered `UIAdapter` instances concurrently.
- Using a Go `select` block, the Mediator captures the first response received.
- It immediately calls `CancelPrompt` on the remaining adapters to dismiss their UI prompts. This ensures seamless "First-In-Wins" synchronization without bloating the core scheduler logic.

## 3. TUI Architecture (Local Command Center)

### 3.1 Layout (3-Panel Split)
- **Left Panel (Top):** Chat Log (history of commands, AI messages, and executed task milestones).
- **Left Panel (Bottom):** Multi-line input box and a sticky status bar displaying Project Name, Git Branch, and the current local time.
- **Right Panel:** Context Window. Displays the active Epic -> Story -> Task list with color-coded status indicators.

### 3.2 Advanced Interaction: The Multi-Question Tab
When an Agent batches multiple questions (e.g., OpenCode querying file selections and configs simultaneously):

- CodeMint intercepts the JSON-RPC array and renders a Tabbed Overlay in the TUI.
- **UX:** Tab cycles through questions. Up/Down highlights options. Space selects.
- **Submission:** The final tab is automatically injected by CodeMint as [Confirmation]. Selecting [ OK ] compiles the answers and returns them to the Agent.

## 4. CUI Architecture (Remote Minimalist Mode)

### 4.1 Philosophy: Minimalist Push, Maximalist Pull
The CUI leverages native chat apps (Telegram/Slack) but respects mobile UX constraints.

- **No Pinned State:** The CUI does not auto-update a pinned context message, as this creates notification fatigue.
- **Push (Events):** The CUI only sends messages when Human Input is strictly required (e.g., Auto-Approval Interceptor blocks, User Story Confirmations) or when a major milestone is reached.
- **Pull (Commands):** Users request state via slash commands:
  - `/tasks`: Returns the current hierarchy list.
  - `/status`: Returns the active task.
  - `/summary`: Returns the recent buffered Agent noise/thinking.

### 4.2 Interaction Primitives
- **Single Select:** Rendered as inline keyboard buttons.
- **Multi-Question:** Handled conversationally or via dynamic inline keyboards that update with ✅ emojis upon clicking, followed by a final [Submit] button.

## 5. Global UX Features

### 5.1 Conversational Revision
When a user wants to perform a Mid-Flight Pivot (defined in EPIC-02), both UIs use the same conversational flow:

1. User clicks [ ✏️ Revise ] on a task or story.
2. The UI asks: "What should I change?"
3. The user replies in natural language.
4. The Clarifier Agent interprets the request and overwrites the SQLite database directly, generating a new draft for the UI to display.

### 5.2 The Verbosity Filter
A global setting (`/verbosity <level>`) that dictates what the `UIAdapter` broadcasts.

- **Level 0 (Task):** Maximum granularity. Shows micro-events, thinking traces, and file edits. (Default for TUI).
- **Level 1 (User Story):** Only announces task success/failure and User Story completions.
- **Level 2 (Epic):** Executive view. Only announces Epic boundaries. Ideal for YOLO mode.