# User Story 3.3: Graceful Teardown

* **As the** Go Orchestrator,
* **I want to** gracefully terminate the `os/exec` process only when the session is archived or paused,
* **So that** zombie processes do not drain system resources.
* *Acceptance Criteria:*
    * Changing a session `status` to `1` (Archived) triggers a SIGTERM/SIGKILL sequence on the mapped ACP process.
