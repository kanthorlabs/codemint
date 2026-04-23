# User Story 2.2: Phase 2 - The Living Spec (Clarification)

* **As the** Coordinator Agent,
* **I want to** maintain a `specification.md` document in memory during the chat phase,
* **So that** I prevent infinite chat loops and hallucinations.
* *Acceptance Criteria:*
    * The system silently updates the `specification.md` document as decisions are made during the chat.
    * The chat loop continues until the user explicitly triggers the `[Generate Plan]` action.
