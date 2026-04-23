# User Story 2.5: Phase 4 - Confirmation Guardrail

* **As the** Task Generator Agent,
* **I want to** append a Confirmation Task (Type 2) as the final step of every User Story,
* **So that** the execution safely pauses for manual approval.
* *Acceptance Criteria:*
    * A task with `type = 2` is inserted as the absolute final task of a `seq_story` block.
    * This task is assigned to the seeded `human` agent, which naturally shifts the session to `awaiting`.
