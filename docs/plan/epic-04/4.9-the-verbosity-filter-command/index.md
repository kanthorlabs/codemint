# User Story 4.9: The Verbosity Filter Command

* **As a** Developer,
* **I want to** use a `/verbosity <level>` command to dictate the noise level of the `UIAdapter`,
* **So that** I can switch between micromanagement and executive overview.
* *Acceptance Criteria:*
    * Level 0 (Task) outputs all micro-events and file edits.
    * Level 1 (User Story) only outputs task success/failure and User Story completions.
    * Level 2 (Epic) only announces Epic boundaries.
