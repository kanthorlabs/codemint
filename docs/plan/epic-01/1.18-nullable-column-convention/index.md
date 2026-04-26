# User Story 1.18: Nullable Column Convention Enforcement

* **As a** Developer,
* **I want** all nullable database columns to use `sql.NullString`, `sql.NullInt64`, or custom `Null<Type>` wrappers,
* **So that** NULL values are explicitly handled in Go code without relying on COALESCE workarounds or zero-value ambiguity.
* *Acceptance Criteria:*
    * `Agent.Assistant` uses `sql.NullString` instead of `string`.
    * `Task.WorkflowID` uses `sql.NullString` instead of `string`.
    * `Task.Input` and `Task.Output` use `sql.NullString` instead of `string`.
    * Repository queries remove `COALESCE` workarounds.
    * Coding guidelines document the nullable column convention.
