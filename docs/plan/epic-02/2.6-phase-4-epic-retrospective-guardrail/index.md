# User Story 2.6: Phase 4 - Epic Retrospective Guardrail

* **As the** Task Generator Agent,
* **I want to** append a Retrospective Task (Type 4) as the absolute final step of every Epic,
* **So that** the human can be politely asked for overarching feedback after a large batch of work is done.
* *Acceptance Criteria:*
    * A task with `type = 4` is inserted as the final task of a `seq_epic` block.
    * This task is assigned to the seeded `human` agent.
    * The UI presents a skippable, conversational prompt to the user (e.g., "Did I do anything annoying?").
    * User input (if provided) is saved to the `output` column of the task to be processed by the Archivist.