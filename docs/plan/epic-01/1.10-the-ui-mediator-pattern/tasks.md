# Tasks: 1.10 The UI Mediator Pattern

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.10-ui-mediator/`
**Tech Stack:** Go (Interfaces, Goroutines, Channels, Context Cancellation)

---

## 🛠 Architectural Concept: Concurrent Broadcast & Race
The `UIMediator` manages multiple UI instances (`UIAdapter`) simultaneously (e.g., a local Terminal and a remote CUI). When the core loop needs a decision, the Mediator broadcasts the prompt to all adapters concurrently. It uses Go channels and `select` to race the responses. The first adapter to respond wins, and a context cancellation signal is instantly sent to the losers to dismiss their active prompts.

---

## Task 1.10.1: Define the UIAdapter Interface
* **Action:** Create `internal/ui/adapter.go`.
* **Details:**
    * Define the interface that all concrete UIs (CLI, Web, Daemon) must implement.
      ```go
      type PromptRequest struct {
          TaskID  string
          Message string
          Options []string // e.g., ["Accept", "Revert"]
      }

      type PromptResponse struct {
          SelectedOption string
          Error          error
      }

      type UIAdapter interface {
          // Receives the context; if ctx is canceled, the adapter must dismiss the prompt
          PromptDecision(ctx context.Context, req PromptRequest) PromptResponse
      }
      ```

## Task 1.10.2: Build the UIMediator and Registration
* **Action:** Create `internal/ui/mediator.go`.
* **Details:**
    * Create the `UIMediator` struct holding a slice of registered adapters.
      ```go
      type UIMediator struct {
          adapters []UIAdapter
          mu       sync.RWMutex
      }

      func NewUIMediator() *UIMediator { ... }
      func (m *UIMediator) RegisterAdapter(a UIAdapter) { ... }
      ```

## Task 1.10.3: Implement the Concurrent Broadcast Logic
* **Action:** Implement `PromptDecision` on the `UIMediator`.
* **Details:**
    * **Logic Steps:**
        1. Create a cancellable context from the parent: `ctx, cancel := context.WithCancel(parentCtx)`.
        2. Ensure `cancel()` is called on exit (`defer cancel()`) to clean up.
        3. Create a channel to catch the first response: `respChan := make(chan PromptResponse, 1)`.
        4. Loop through registered adapters and spin up a goroutine for each.
        5. Inside the goroutine, call `adapter.PromptDecision(ctx, req)`. If successful, push the result to `respChan`.
        6. Back in the main function, use a `select` block to block until `respChan` yields the first result (or the parent context times out).
        7. Returning the result implicitly triggers the `defer cancel()`, signaling all other slow adapters to dismiss their UI prompts.

## Task 1.10.4: Integrate with the Core Loop
* **Action:** Update `internal/project/review_commands.go` (and other components).
* **Details:**
    * When a task enters `TaskStatusAwaiting`, the Orchestrator no longer calls `fmt.Print` or blocks on `bufio.Reader`. 
    * Instead, it calls `mediator.PromptDecision(ctx, req)`. 
    * It processes the winning `PromptResponse` and proceeds with the State Machine.

## Task 1.10.5: Broadcast Unit Tests
* **Action:** Create `internal/ui/mediator_test.go`.
* **Details:**
    * Create a `FastMockAdapter` (returns in 10ms) and a `SlowMockAdapter` (returns in 100ms or blocks until ctx canceled).
    * Register both to the Mediator.
    * Call `PromptDecision`.
    * Assert that the response matches the `FastMockAdapter` and that the `SlowMockAdapter` correctly caught the context cancellation.