# User Story 1.7: Dynamic Workflow Registry

* **As the** Go Orchestrator,
* **I want to** load available workflows from a configuration file rather than hardcoding them,
* **So that** the system can be easily expanded with new behaviors.
* *Acceptance Criteria:*
    * Workflows are loaded from `config.yaml` at startup.
    * Workflows are registered into a Go map (`map[int]Workflow`).
    * At minimum, `Project Coding`, `Communication`, and `Daily Checking` workflows are scaffolded.

---

## Change Request

- Remove the Workflow concept, adapt the [Agent Skill](https://agentskills.io/specification)

---

## Design Exception: Skill ID

**Exception:** While CodeMint uses UUIDv7 by default for entity identifiers, Skill IDs use **MD5 hash of the full path to SKILL.md**.

**Rationale:** Skill IDs must be deterministic and consistent across different environments. Using path-based hashing ensures:
- Same skill loaded from same path always has same ID
- Reliable referencing regardless of load order
- No external ID generation dependency
- Collision handling when same skill name exists in multiple directories

**Implementation:** `ID = MD5(absolute_path_to_SKILL.md)`