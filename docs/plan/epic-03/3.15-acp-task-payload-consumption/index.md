# User Story 3.15: ACP-Compliant Task Payload Consumption (Supports EPIC-02 §2.13)

* **As the** Executor,
* **I want to** parse the structured JSON written by the Task Generator into the `task.input` column and translate it into a typed `acp.SessionPromptParams`,
* **So that** EPIC-02's §2.13 contract ("input is JSON, not raw text") actually reaches the ACP worker without lossy string concatenation.
* *Acceptance Criteria:*
    * `task.Input` is decoded into a typed `domain.TaskInput` struct before any worker call.
    * Required fields (`prompt`, `context_files`) are validated; missing required fields fail the task with reason `invalid_input`.
    * `context_files` paths are resolved relative to `project.WorkingDir`, not the process CWD.
    * The resulting `session/prompt` payload includes `prompt`, an attached `context` array, and any `tools` hints supplied by the Task Generator.
    * Backward compatibility: a legacy plain-text `task.Input` (no JSON) is wrapped into `{"prompt": <text>}` with a `slog.Warn` so existing pre-EPIC-02.13 sessions don't crash.
