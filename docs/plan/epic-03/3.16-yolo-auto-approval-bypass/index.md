# User Story 3.16: YOLO Auto-Approval Bypass (Supports EPIC-02 §2.9, §2.10)

* **As the** Scheduler,
* **I want to** detect tasks whose `assignee_id` matches the seeded `sys-auto-approve` agent and short-circuit the human-approval pause,
* **So that** the YOLO Mode Delegation promised by EPIC-02 §2.10 actually runs without prompting the user.
* *Acceptance Criteria:*
    * The seeded `sys-auto-approve` agent's UUID is loaded once at startup and cached on the `Runtime`.
    * Any Confirmation (`type=2`) or Retrospective (`type=4`) task assigned to `sys-auto-approve.id` skips `mediator.PromptDecision` entirely.
    * The task is marked `success` with `task.Output = {"auto_approved": true, "agent": "sys-auto-approve"}` so the audit trail is preserved.
    * Coding (`type=0`) and Verification (`type=1`) tasks ignore the assignee field — YOLO only changes approval gates, not what gets executed.
    * Tool-call interception (Story 3.6) is unchanged: blocked commands still pause the agent regardless of YOLO; YOLO bypasses Story-level Confirmation, not per-tool security.
