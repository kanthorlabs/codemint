# User Story 2.1: Project Overview

* **As the** Coding Workflow,
* **I want** a cheap full-tree pass over the project before goal-setting,
* **So that** the user locks a goal grounded in what actually exists, without burning tokens on broad file reads up front.

> **Spec slot:** Step 1 (Project Overview) — feeds 2.2 Goal Capture.

## Acceptance Criteria

1. Built-in `@codemint/gatherer` skill is embedded in the binary.
2. Skill reads:
   - The project directory tree, respecting `.gitignore`, depth-limited.
   - A small fixed set of key files: `README.md`, `go.mod`, `Makefile`, top-level `package.json` / `pyproject.toml` (best-effort; missing files are ignored, not errors).
3. **No grep, no recursive file reads, no per-package AST.** That work belongs to 2.3 Goal-scoped Reality.
4. Skill output is structured JSON (`project_summary`, `key_files`, `tech_stack`, `conventions`) so 2.3 can append to it without parsing markdown.
5. Output is written to the task's `output` column. Downstream stories read it from there.
6. Failure (e.g., the project directory is unreadable) → task `Failure` per the cross-cutting error contract — see 2.8.

## Workflow definition

```yaml
- id: gather
  name: "Project Overview"
  skill: "@codemint/gatherer"
```

## Dependencies

- 2.0.1 Workflow File Infrastructure (skill embedding)
- 2.0.4 Workflow Command (task creation)
- 2.0.5 Skill Injection (delivers the skill body to the ACP prompt)

## Out of Scope

- Targeted grep / Goal-scoped reads → 2.3.
- Cross-session context cache → not in EPIC-02.
- Wiki / memory injection → EPIC-05.
