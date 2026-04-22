# User Story 1.2: UUID v7 Integration

* **As the** Go Orchestrator,
* **I want to** generate UUID v7 for all primary and foreign keys,
* **So that** database insertions remain highly performant (lexicographically sorted by timestamp) while supporting decentralized ID generation.
* *Acceptance Criteria:*
    * All IDs inserted into the database are strictly UUID v7.
    * No auto-incrementing integer primary keys exist in the schema.