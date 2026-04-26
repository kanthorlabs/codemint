# Tasks: 1.17 Workflow Registry from Config

**Epic:** EPIC-01 (Foundation & Routing)
**Target Directory:** `1.17-workflow-registry-config/`
**Tech Stack:** Go, `gopkg.in/yaml.v3`, config validation
**Priority:** P2 (Nice to Have - Enables dynamic workflow routing)

---

## Task 1.17.1: Define Workflow Domain Types
* **Action:** Update `internal/domain/core.go` or create `internal/domain/workflow.go`.
* **Details:**
  * Define `WorkflowType` enum:
    ```go
    type WorkflowType int
    const (
        WorkflowTypeProjectCoding WorkflowType = iota // 0
        WorkflowTypeCommunication                      // 1
        WorkflowTypeDailyChecking                      // 2
    )
    ```
  * Define `WorkflowDefinition` struct:
    ```go
    type WorkflowDefinition struct {
        Type        WorkflowType
        Name        string   // Human-readable name
        Description string   // One-line description
        Triggers    []string // Keywords that route to this workflow
    }
    ```
* **Verification:**
  * Types compile without errors.
  * `WorkflowType` values match database `workflow.type` column.

## Task 1.17.2: Define Config File Schema
* **Action:** Create `internal/config/config.go`.
* **Details:**
  * Define `Config` struct with YAML tags:
    ```go
    type Config struct {
        Workflows []WorkflowConfig `yaml:"workflows"`
        Agents    []AgentConfig    `yaml:"agents,omitempty"`
    }
    
    type WorkflowConfig struct {
        Type        int      `yaml:"type"`
        Name        string   `yaml:"name"`
        Description string   `yaml:"description"`
        Triggers    []string `yaml:"triggers,omitempty"`
    }
    ```
  * Support `$XDG_CONFIG_HOME/codemint/config.yaml` as default path.
* **Verification:**
  * Sample YAML parses without errors.

## Task 1.17.3: Create Sample config.yaml
* **Action:** Create `configs/config.yaml.example`.
* **Details:**
  ```yaml
  # CodeMint Configuration
  # Copy to ~/.config/codemint/config.yaml
  
  workflows:
    - type: 0
      name: "Project Coding"
      description: "Context-aware coding tasks within a project"
      triggers: ["implement", "fix", "refactor", "add feature"]
    
    - type: 1
      name: "Communication"
      description: "General inquiries and explanations"
      triggers: ["explain", "what is", "how does", "tell me"]
    
    - type: 2
      name: "Daily Checking"
      description: "Status checks and routine operations"
      triggers: ["status", "check", "verify", "test"]
  ```
* **Verification:**
  * YAML is valid (use `yq` or online validator).
  * Covers all three core workflow types from epic-01.md.

## Task 1.17.4: Implement Config Loader
* **Action:** Create `internal/config/loader.go`.
* **Details:**
  * `Load(path string) (*Config, error)` - reads and parses YAML.
  * `LoadDefault() (*Config, error)` - loads from XDG config path.
  * Return descriptive errors with line numbers on parse failure.
  * Handle file-not-found gracefully (return empty config, not error).
* **Verification:**
  * Unit test: Load valid YAML → returns populated Config.
  * Unit test: Load invalid YAML → returns error with line info.
  * Unit test: Load nonexistent file → returns empty Config, nil error.

## Task 1.17.5: Implement Config Validation
* **Action:** Create `internal/config/validate.go`.
* **Details:**
  * `Validate(c *Config) error` checks:
    * No duplicate workflow types.
    * All workflows have non-empty `name`.
    * Workflow types are within valid range (0-2 for now).
  * Return `ValidationError` with all violations (not just first).
* **Verification:**
  * Unit test: Duplicate type → error lists both.
  * Unit test: Empty name → error identifies which workflow.
  * Unit test: Valid config → returns nil.

## Task 1.17.6: Create WorkflowRegistry
* **Action:** Create `internal/workflow/registry.go`.
* **Details:**
  * Define `WorkflowRegistry` struct:
    ```go
    type WorkflowRegistry struct {
        workflows map[domain.WorkflowType]domain.WorkflowDefinition
    }
    ```
  * Methods:
    * `NewWorkflowRegistry() *WorkflowRegistry`
    * `Register(def domain.WorkflowDefinition) error` - returns error on duplicate
    * `Lookup(t domain.WorkflowType) (domain.WorkflowDefinition, error)`
    * `All() []domain.WorkflowDefinition` - sorted by type
    * `FindByTrigger(keyword string) (domain.WorkflowDefinition, bool)` - matches triggers
* **Verification:**
  * Unit test: Register → Lookup → returns same definition.
  * Unit test: Register duplicate → returns error.
  * Unit test: Lookup unknown type → returns ErrWorkflowNotFound.

## Task 1.17.7: Implement LoadFromConfig Helper
* **Action:** Update `internal/workflow/registry.go`.
* **Details:**
  * `LoadFromConfig(cfg *config.Config) (*WorkflowRegistry, error)`:
    * Validates config first.
    * Converts `WorkflowConfig` to `WorkflowDefinition`.
    * Registers each workflow.
    * Returns populated registry.
* **Verification:**
  * Unit test: Valid config → registry has all workflows.
  * Unit test: Invalid config → returns validation error.

## Task 1.17.8: Integrate with Dispatcher
* **Action:** Update `internal/orchestrator/dispatcher.go`.
* **Details:**
  * Add `workflowRegistry *workflow.WorkflowRegistry` field.
  * In natural-language path (non-global), use `FindByTrigger` to route.
  * If no trigger matches, default to `WorkflowTypeProjectCoding`.
  * Replace `ErrNoBrainstormer` with workflow routing (prep for EPIC-02).
* **Verification:**
  * Integration test: Input "explain X" → routes to Communication workflow.
  * Integration test: Input "implement Y" → routes to ProjectCoding workflow.
  * Integration test: Unknown input → defaults to ProjectCoding.

## Task 1.17.9: Add Workflow Registry to Startup
* **Action:** Update `cmd/codemint/main.go`.
* **Details:**
  * Load config with `config.LoadDefault()`.
  * Create workflow registry with `workflow.LoadFromConfig(cfg)`.
  * Pass registry to `NewDispatcher`.
  * Log loaded workflows at INFO level.
* **Verification:**
  * Start app → logs show "loaded 3 workflows".
  * Missing config.yaml → app starts with empty registry (graceful).

## Task 1.17.10: Write End-to-End Test
* **Action:** Create `internal/workflow/registry_test.go`.
* **Details:**
  * Test full flow: YAML → Config → Validation → Registry → Lookup.
  * Test trigger matching with case-insensitivity.
  * Test that all three core workflow types are routable.
* **Verification:**
  * `go test ./internal/workflow/... -v` all pass.
  * `go test ./internal/config/... -v` all pass.
