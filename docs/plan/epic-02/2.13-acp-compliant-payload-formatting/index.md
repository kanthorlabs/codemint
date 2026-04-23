# User Story 2.12: ACP-Compliant Payload Formatting (Supports EPIC-03)

* **As the** Task Generator Agent,
* **I want to** format the generated tasks' `input` field as structured JSON,
* **So that** the Persistent ACP Worker (OpenCode) can seamlessly parse the request over standard I/O.
* *Acceptance Criteria:*
    * The `input` column in the database is populated with a JSON blob containing the context and prompt, not just raw text strings.
