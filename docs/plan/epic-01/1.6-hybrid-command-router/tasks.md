# Tasks: 1.6 The Hybrid Command Router & Dispatcher

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.6-hybrid-command-router/`
**Tech Stack:** Go, `github.com/google/shlex`

---

## 🛠 Architectural Concept: The Active Session & Dual-Parser
CodeMint maintains a single `ActiveSession` (Global or Code Project). When a user inputs text, the Dispatcher utilizes a **Dual-Parser strategy**:
1.  **Strict Flags:** Tries to parse standard UNIX flags (e.g., `-p high`). If successful, it executes instantly.
2.  **System Assistant Fallback:** If flags are missing but natural language is present, the handler delegates to the internal **System Assistant** to extract the parameters into JSON, providing a seamless CUI experience.

---

# Architecture Standard: Declarative Command Capabilities

To ensure handlers remain clean and UI-Agnostic, CodeMint uses **Declarative Capability Constraints** to restrict commands based on the environment (CLI vs. Daemon/Chat).

## 1. Registry Definition
When registering a command, developers must explicitly define which `ClientMode`s the command supports via the `SupportedModes` field.

```go
// internal/repl/core_commands.go
commands := []registry.Command{
    {
        Name:           "exit",
        Description:    "Safely close the active session and exit CodeMint.",
        Usage:          "/exit",
        SupportedModes: []orchestrator.ClientMode{orchestrator.ClientModeCLI}, // CLI ONLY
        Handler:        exitHandler,
    },
    {
        Name:           "help",
        Description:    "Display this help menu.",
        Usage:          "/help",
        SupportedModes: []orchestrator.ClientMode{orchestrator.ClientModeCLI, orchestrator.ClientModeDaemon}, // BOTH
        Handler:        helpHandler(r),
    },
}
```

## 2. Dispatcher Enforcement
The `Dispatcher` acts as middleware. Before executing `cmd.Handler()`, it must verify that `active.ClientMode` exists within `cmd.SupportedModes`. 
* If unsupported, the Dispatcher intercepts the call and outputs a standardized UI message: `"⚠️ The /<cmd> command is not available in <mode> mode."`
* **Benefit:** Command handlers do not need internal `if` statements to check the environment.

## 3. Dynamic Help Menus
The `/help` command handler must be updated to filter the registry. It should only render commands where `SupportedModes` includes the current `active.ClientMode`.

---

## Task 1.6.1: The Rich Command Registry
* **Action:** Create `internal/registry/commands.go`.
* **Details:**
    * Define a `Command` struct to support a robust `/help` menu:
      ```go
      type Command struct {
          Name        string
          Description string
          Usage       string
          Handler     func(ctx context.Context, args []string, rawArgs string) error
      }
      ```
    * Create the registry map: `map[string]Command`.
    * Implement a `Register(c Command)` function.
    * This enables **Decentralized Registration** (domain packages register their own commands during boot).

## Task 1.6.2: Dual-State Tokenizer
* **Action:** Create `internal/repl/parser.go`.
* **Details:**
    * Implement `ParseInput(raw string) (isSlash bool, cmd string, args []string, rawArgs string, err error)`.
    * **Logic:** Use `strings.SplitN` to isolate the command word. Use `shlex.Split` to generate the strict `args` array, and keep the untouched `rawArgs` string for potential System Assistant extraction.

## Task 1.6.3: The ActiveSession State Holder
* **Action:** Create `internal/orchestrator/active_session.go`.
* **Details:**
    * Define the runtime struct for the current working round:
      ```go
      type ActiveSession struct {
          IsGlobal    bool
          Project     *domain.Project // Nil if IsGlobal == true
          Session     *domain.Session // Nil if IsGlobal == true
          YoloEnabled bool
      }
      ```

## Task 1.6.4: The Context-Aware Dispatcher
* **Action:** Create `internal/orchestrator/dispatcher.go`.
* **Details:**
    * Implement `Dispatch(ctx context.Context, active *ActiveSession, input string) error`.
    * **Flow:**
        1. Parse input.
        2. If `/command`: Look up in Registry. Call the `Handler(ctx, args, rawArgs)`.
        3. If Natural Language & `IsGlobal`: Route to the **System Assistant** for general conversational assistance.
        4. If Natural Language & `!IsGlobal`: Route to Brainstormer (EPIC-02) for code modifications.

## Task 1.6.5: Implement the System Assistant Extractor
* **Action:** Create `internal/orchestrator/extractor.go`.
* **Details:**
    * Implement a utility function for command handlers to use when strict flags are missing.
    * **Logic:** Create a background task assigned to the **System Assistant** to parse `rawArgs` into a required JSON schema (e.g., extracting ticket parameters). Return the parsed struct to the handler.

## Task 1.6.6: REPL Core Commands & Bootstrapping
* **Action:** Create `internal/repl/core_commands.go`.
* **Details:**
    * Implement a setup function: `func RegisterCoreCommands(r *registry.CommandRegistry)`.
    * Register strictly terminal-level utilities:
        * **`/help`:** Iterates through the registry and formats the usage table.
        * **`/exit`:** Safely closes the `ActiveSession` and exits the process.
        * **`/clear`:** Clears the terminal screen.
    * *(Note: Domain-specific commands like `/yolo` and `/status` will be registered by their respective packages in `main.go`).*

## Task 1.6.7: Router Logic Unit Tests
* **Action:** Create `internal/orchestrator/dispatcher_test.go`.
* **Details:**
    * *Test A (Strict Parse):* Ensure `args` are correctly passed to a mock handler.
    * *Test B (Fallback Flow):* Mock the System Assistant extraction and ensure the handler receives the JSON struct.
    * *Test C (Help Generation):* Ensure `/help` outputs the expected structured text based on registered commands.