# User Story 3.9: CUI Low-Bandwidth Pulse

* **As a** Developer using the CUI,
* **I want** my chat app to only receive terminal state changes or explicit blocks,
* **So that** I am not spammed with hundreds of notifications.
* *Acceptance Criteria:*
    * The `UIAdapter` implementation for the CUI filters out micro-events and only pushes events when a task hits `awaiting`, `success`, or `failure`.
