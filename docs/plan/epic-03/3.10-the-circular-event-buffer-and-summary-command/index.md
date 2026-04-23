# User Story 3.10: The Circular Event Buffer & `/summary` Command

* **As a** Developer,
* **I want to** type `/summary [task_id]` to retrieve the recent "noise" of a task,
* **So that** I can debug what the agent is currently thinking without turning on the live stream.
* *Acceptance Criteria:*
    * The Go backend stores the last N raw events in a circular memory buffer per active task.
    * If `task_id` is omitted, it defaults to querying the active session's most recent `processing` or `awaiting` task.
    * The command aggregates the buffer into a clean Markdown `<thinking>` block and sends it to the UI.
