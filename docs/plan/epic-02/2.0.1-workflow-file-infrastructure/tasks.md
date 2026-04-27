# Tasks for 2.0.1: Workflow File Infrastructure

## Task 2.0.1.1: Domain Types for Workflow Files

**Description:** Define Go structs for parsed WORKFLOW.yaml content.

**Files to create/modify:**
- `internal/domain/workflow_file.go` (NEW)

**Implementation:**
```go
package domain

// WorkflowFile represents a parsed WORKFLOW.yaml file.
type WorkflowFile struct {
    Name        string
    Version     string
    Description string
    Settings    WorkflowSettings
    Epics       []EpicDefinition
    SourcePath  string // Absolute path to WORKFLOW.yaml
}

type WorkflowSettings struct {
    DefaultTimeout int64
    Guardrails     GuardrailSettings
}

type GuardrailSettings struct {
    Verification  bool
    Confirmation  bool
    Retrospective bool
}

func DefaultGuardrailSettings() GuardrailSettings {
    return GuardrailSettings{
        Verification:  true,
        Confirmation:  true,
        Retrospective: true,
    }
}

type EpicDefinition struct {
    ID            string
    Name          string
    Description   string
    DependsOn     string // "epic_id.story_id" format
    Retrospective *bool  // nil = use workflow default
    Stories       []StoryDefinition
}

type StoryDefinition struct {
    ID         string
    Name       string
    Type       TaskType
    Skill      string
    ExitOn     *ExitCondition
    Routes     map[TaskStatus]string // status → next_story_id
    DependsOn  string
    Condition  *TaskStatus
    Guardrails *GuardrailSettings
    Output     *OutputConfig
}

type ExitCondition struct {
    Command     string
    Timeout     int64
    OutputValid bool
}

type OutputConfig struct {
    Schema  string
    Handler string
}
```

**Verification:**
- `go build ./...` succeeds
- Unit tests for `DefaultGuardrailSettings()`

**Estimated effort:** 0.5 day

---

## Task 2.0.1.2: WORKFLOW.yaml Parser

**Description:** Parse WORKFLOW.yaml files into domain structs with validation.

**Files to create/modify:**
- `internal/workflow/parser.go` (NEW)
- `internal/workflow/parser_test.go` (NEW)

**Implementation:**
- Use `gopkg.in/yaml.v3` for YAML parsing
- Validate required fields: `name`, `version`, `epics`
- Validate `name` matches parent directory name
- Resolve skill references (validate format, not existence)
- Return clear error messages for invalid files

**Verification:**
- Unit tests with valid WORKFLOW.yaml fixtures
- Unit tests for each validation error case
- `go test ./internal/workflow/...` passes

**Estimated effort:** 1.5 days

---

## Task 2.0.1.3: Workflow File Registry

**Description:** Registry that discovers and loads workflows from disk and embedded sources.

**Files to create/modify:**
- `internal/workflow/file_registry.go` (NEW)
- `internal/workflow/file_registry_test.go` (NEW)

**Implementation:**
```go
type FileRegistry struct {
    workflows map[string]*domain.WorkflowFile
}

func NewFileRegistry() *FileRegistry

func (r *FileRegistry) LoadAll() error
func (r *FileRegistry) Get(name string) (*domain.WorkflowFile, bool)
func (r *FileRegistry) All() []*domain.WorkflowFile
func (r *FileRegistry) Names() []string // For autocomplete
```

**Load order:**
1. External: `~/.local/share/codemint/workflows/`
2. Embedded: `internal/workflow/embedded/` (highest precedence)

**Verification:**
- Unit tests with mock filesystem
- Integration test with real embedded workflows
- `go test ./internal/workflow/...` passes

**Estimated effort:** 1 day

---

## Task 2.0.1.4: Embedded Workflows Setup

**Description:** Set up go:embed directive and directory structure for built-in workflows.

**Files to create/modify:**
- `internal/workflow/embedded.go` (NEW)
- `internal/workflow/embedded/brainstorming/WORKFLOW.yaml` (NEW)

**Implementation:**
```go
//go:embed embedded/*
var embeddedWorkflowFS embed.FS
```

Create placeholder `brainstorming` workflow with minimal structure.

**Verification:**
- `go build ./...` succeeds
- Embedded files are accessible at runtime

**Estimated effort:** 0.5 day

---

## Task 2.0.1.5: Wire File Registry into main.go

**Description:** Initialize FileRegistry at startup and make it available to commands.

**Files to modify:**
- `cmd/codemint/main.go`

**Implementation:**
- Create `workflow.NewFileRegistry()` after config load
- Call `fileRegistry.LoadAll()`
- Pass to command registration

**Verification:**
- Fresh launch loads embedded workflows
- `/workflow` command (when implemented) can list workflows

**Estimated effort:** 0.5 day

---

## Dependency Order

```
2.0.1.1 (Domain Types)
    │
    ▼
2.0.1.2 (Parser)
    │
    ├──► 2.0.1.4 (Embedded Setup)
    │         │
    ▼         ▼
2.0.1.3 (Registry) ◄── depends on both
    │
    ▼
2.0.1.5 (Wire to main.go)
```

## Total Estimated Effort: 4 days
