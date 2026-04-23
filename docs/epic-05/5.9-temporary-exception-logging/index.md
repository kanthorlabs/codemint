# User Story 5.9: Temporary Exception Logging

* **As** The Archivist,
* **I want to** classify prompt-overrides as temporary "Exceptions" rather than permanent rule changes,
* **So that** a one-off prototype request doesn't overwrite a strict global standard in `preferences.md`.
* *Acceptance Criteria:*
    * If the agent detects that the user explicitly contradicted a rule in `preferences.md`, it logs it in the Epic's `history/<slug>.md` file as an exception, but does *not* stage a preference change in the Inbox.
