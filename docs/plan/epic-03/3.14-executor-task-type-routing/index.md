# User Story 3.14: Executor Task-Type Routing & Confirmation Pause (Supports EPIC-02)

* **As the** Executor,
* **I want to** branch on `task.type` (Coding, Verification, Confirmation, Coordination, Retrospective) before forwarding to the ACP worker,
* **So that** Verification tasks run as automatic checks, Confirmation and Retrospective tasks correctly pause the session for human input, and only Coding tasks reach the ACP agent.
* *Acceptance Criteria:*
    * `type=0` (Coding): forwarded to ACP worker via `session/prompt`.
    * `type=1` (Verification): runs the task's `input.command` (e.g., `go test ./...`) through the existing `LocalRunner`; output captured into `task.output`; status maps to `success` on exit code 0, `failure` otherwise.
    * `type=2` (Confirmation): immediately transitions the task to `awaiting` and broadcasts a `PromptDecision` via the mediator; resumes on user action.
    * `type=3` (Coordination): no-op for the scheduler (these are command echoes from the user, not executable work).
    * `type=4` (Retrospective): same as Confirmation but renders the EPIC-02 §2.6 conversational template ("Did I do anything annoying?"); user input stored in `task.output`.
