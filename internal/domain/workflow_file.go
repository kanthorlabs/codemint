// Package domain defines the core Go domain entities, enums, and structs
// for the CodeMint persistence layer.
package domain

// WorkflowFile represents a parsed WORKFLOW.yaml file.
// It defines the structure of a workflow with epics and stories.
type WorkflowFile struct {
	Name        string
	Version     string
	Description string
	Settings    WorkflowSettings
	Epics       []EpicDefinition
	SourcePath  string // Absolute path to WORKFLOW.yaml
}

// WorkflowSettings defines workflow-level configuration.
type WorkflowSettings struct {
	DefaultTimeout int64
	Guardrails     GuardrailSettings
}

// GuardrailSettings defines automatic guardrail injection settings.
type GuardrailSettings struct {
	Verification  bool
	Confirmation  bool
	Retrospective bool
}

// DefaultGuardrailSettings returns the default guardrail configuration
// with all guardrails enabled.
func DefaultGuardrailSettings() GuardrailSettings {
	return GuardrailSettings{
		Verification:  true,
		Confirmation:  true,
		Retrospective: true,
	}
}

// DefaultWorkflowSettings returns the default workflow settings
// with a 1-hour timeout and all guardrails enabled.
func DefaultWorkflowSettings() WorkflowSettings {
	return WorkflowSettings{
		DefaultTimeout: DefaultTaskTimeout,
		Guardrails:     DefaultGuardrailSettings(),
	}
}

// EpicDefinition represents a group of related stories within a workflow.
type EpicDefinition struct {
	ID            string
	Name          string
	Description   string
	DependsOn     string // "epic_id.story_id" format
	Retrospective *bool  // nil = use workflow default
	Stories       []StoryDefinition
}

// StoryDefinition represents a single executable step within an epic.
type StoryDefinition struct {
	ID         string
	Name       string
	Type       TaskType
	Skill      string             // Skill reference (e.g., "@codemint/gatherer", "./skills/local")
	ExitOn     *ExitCondition     // Conditions to exit this story
	Routes     map[TaskStatus]string // status → next_story_id
	DependsOn  string
	Condition  *TaskStatus
	Guardrails *GuardrailSettings // nil = use epic/workflow default
	Output     *OutputConfig
}

// ExitCondition defines when a story should exit.
type ExitCondition struct {
	Command     string // Slash command that triggers exit (e.g., "/generate")
	Timeout     int64  // Timeout in milliseconds
	OutputValid bool   // Exit when output schema validates
}

// OutputConfig defines the output handling for a story.
type OutputConfig struct {
	Schema  string // Path to JSON schema file relative to workflow dir
	Handler string // Handler function name
}
