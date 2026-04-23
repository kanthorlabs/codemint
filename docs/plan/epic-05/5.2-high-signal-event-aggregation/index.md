# User Story 5.2: High-Signal Event Aggregation

* **As the** Go Orchestrator,
* **I want to** query the SQLite database for high-signal events at the end of every Epic,
* **So that** I don't waste AI tokens summarizing raw execution noise.
* *Acceptance Criteria:*
    * The backend runs a `SELECT` query filtering for tasks where `review_feedback != null`, `status = 'reverted'`, or `type IN (1, 2)` (Verification/Confirmation).
