# User Story 2.0.1: Workflow File Infrastructure

* **As a** contributor or user,
* **I want to** define workflows via `WORKFLOW.yaml` files in `~/.local/share/codemint/workflows/`,
* **So that** I can create custom brainstorming/coding pipelines without modifying Go code.

## Acceptance Criteria

1. `WORKFLOW.yaml` schema is defined and documented.
2. `internal/workflow/parser.go` parses WORKFLOW.yaml into `domain.WorkflowFile` structs.
3. `internal/workflow/file_registry.go` discovers and loads workflows from disk.
4. Built-in workflows are embedded via `//go:embed` (same pattern as skills).
5. Workflows reference skills via `skill: "@codemint/gatherer"` or `skill: "./skills/local"`.
6. Invalid WORKFLOW.yaml files produce clear parse errors at load time.

## Technical Design

### Directory Structure

```
~/.local/share/codemint/workflows/
└── brainstorming/
    ├── WORKFLOW.yaml           # Execution sequence
    └── skills/                 # Optional local skills
        ├── gatherer/
        │   └── SKILL.md
        └── spec-writer/
            └── SKILL.md
```

### Embedded Built-in Workflows

```
internal/workflow/
├── embedded/
│   └── brainstorming/
│       ├── WORKFLOW.yaml
│       └── skills/
│           ├── gatherer/SKILL.md
│           ├── spec-writer/SKILL.md
│           └── task-generator/SKILL.md
├── embedded.go           # go:embed directive
├── parser.go             # WORKFLOW.yaml parser
└── file_registry.go      # WorkflowFile registry
```

### WORKFLOW.yaml Schema

```yaml
# Required metadata
name: brainstorming                    # Unique identifier (must match directory name)
version: "1.0"                         # Semantic version
description: |                         # Human-readable description
  5-phase brainstorming pipeline for feature planning.

# Optional: workflow-level settings
settings:
  default_timeout: 3600000             # 1 hour per task (ms)
  guardrails:
    verification: true                 # Auto-inject after coding stories
    confirmation: true                 # Auto-inject after stories  
    retrospective: true                # Auto-inject at end of epic

# Epic → Story hierarchy
epics:
  - id: planning
    name: "Feature Planning"
    description: "Gather context, clarify requirements, generate tasks"
    
    stories:
      - id: gather
        name: "Context Intake"
        skill: "@codemint/gatherer"
        
      - id: clarify
        name: "Specification Clarification"
        skill: "@codemint/spec-writer"
        exit_on:
          command: "/generate"
          
      - id: generate
        name: "Task Generation"  
        skill: "@codemint/task-generator"
        output:
          schema: "skills/task-generator/references/task-schema.json"
          handler: "create_implementation_tasks"
```

### Skill References

```yaml
# Built-in (embedded in binary)
skill: "@codemint/gatherer"

# From skills registry (searched in order)
skill: "my-custom-skill"

# Relative to WORKFLOW.yaml directory
skill: "./skills/local-skill"

# Absolute path
skill: "~/.agents/skills/external-skill"
```

## Dependencies

- None (foundational)

## Blocks

- 2.0.2 Task Routing
- 2.0.3 Execution State
- 2.0.4 Workflow Command
- All Phase 1-5 stories (2.1-2.7)
