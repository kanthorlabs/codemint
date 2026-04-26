# User Story 1.19: Session Continuity Across TUI/CUI Modes

* **As a** CodeMint user,
* **I want** my active session to automatically resume when I switch between TUI (desktop) and CUI (mobile),
* **So that** I can continue work immediately without manual commands regardless of location or device.
* *Acceptance Criteria:*
    * On startup, program auto-loads the most recent active session (if any).
    * User continues work immediately — no manual resume required.
    * `/session-resume <id>` available for switching to a different session.
    * Active client ownership tracked in DB; graceful handoff on interface switch.
    * Mode-specific commands enforced dynamically (not just at startup).
    * Pending `awaiting` tasks visible and actionable in both TUI and CUI.
    * All interactions persisted as Coordination tasks (`type=3`) in existing `task` table.
    * When client reconnects, it displays activity that occurred on other clients since last interaction.
    * Client in read-only mode auto-reclaims session when user types (no restart needed).
