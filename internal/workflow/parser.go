// Package workflow provides the WorkflowRegistry for managing workflow
// definitions loaded from configuration.
package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"codemint.kanthorlabs.com/internal/domain"
	"gopkg.in/yaml.v3"
)

// Parser errors.
var (
	ErrMissingName       = errors.New("workflow: missing required field: name")
	ErrMissingVersion    = errors.New("workflow: missing required field: version")
	ErrMissingEpics      = errors.New("workflow: missing required field: epics")
	ErrNameMismatch      = errors.New("workflow: name does not match directory name")
	ErrInvalidEpic       = errors.New("workflow: invalid epic definition")
	ErrInvalidStory      = errors.New("workflow: invalid story definition")
	ErrInvalidExitOn     = errors.New("workflow: invalid exit_on: must specify exactly one of command or commands")
)

// workflowYAML is the internal YAML representation of a WORKFLOW.yaml file.
// It maps directly to the YAML structure before conversion to domain types.
type workflowYAML struct {
	Name        string        `yaml:"name"`
	Version     string        `yaml:"version"`
	Description string        `yaml:"description"`
	Settings    *settingsYAML `yaml:"settings"`
	Epics       []epicYAML    `yaml:"epics"`
}

type settingsYAML struct {
	DefaultTimeout int64           `yaml:"default_timeout"`
	Guardrails     *guardrailsYAML `yaml:"guardrails"`
}

type guardrailsYAML struct {
	Verification  *bool `yaml:"verification"`
	Confirmation  *bool `yaml:"confirmation"`
	Retrospective *bool `yaml:"retrospective"`
}

type epicYAML struct {
	ID            string      `yaml:"id"`
	Name          string      `yaml:"name"`
	Description   string      `yaml:"description"`
	DependsOn     string      `yaml:"depends_on"`
	Retrospective *bool       `yaml:"retrospective"`
	Stories       []storyYAML `yaml:"stories"`
}

type storyYAML struct {
	ID         string               `yaml:"id"`
	Name       string               `yaml:"name"`
	Type       string               `yaml:"type"`
	Skill      string               `yaml:"skill"`
	ExitOn     *exitConditionYAML   `yaml:"exit_on"`
	Routes     map[string]string    `yaml:"routes"`
	DependsOn  string               `yaml:"depends_on"`
	Condition  string               `yaml:"condition"`
	Guardrails *guardrailsYAML      `yaml:"guardrails"`
	Output     *outputConfigYAML    `yaml:"output"`
}

type exitConditionYAML struct {
	Command     string   `yaml:"command"`
	Commands    []string `yaml:"commands"`
	Timeout     int64    `yaml:"timeout"`
	OutputValid bool     `yaml:"output_valid"`
}

type outputConfigYAML struct {
	Schema  string `yaml:"schema"`
	Handler string `yaml:"handler"`
}

// WorkflowParser parses WORKFLOW.yaml files into domain.WorkflowFile structs.
type WorkflowParser struct{}

// NewWorkflowParser creates a new WorkflowParser.
func NewWorkflowParser() *WorkflowParser {
	return &WorkflowParser{}
}

// Parse reads and parses a WORKFLOW.yaml file from the given directory.
// The directory must contain a WORKFLOW.yaml file, and the workflow name
// must match the directory name.
func (p *WorkflowParser) Parse(dirPath string) (*domain.WorkflowFile, error) {
	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, fmt.Errorf("workflow: resolve abs path %q: %w", dirPath, err)
	}

	workflowPath := filepath.Join(absDir, "WORKFLOW.yaml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		return nil, fmt.Errorf("workflow: read WORKFLOW.yaml at %q: %w", workflowPath, err)
	}

	return p.ParseBytes(data, absDir)
}

// ParseBytes parses WORKFLOW.yaml content from bytes.
// sourcePath should be the absolute path to the workflow directory.
func (p *WorkflowParser) ParseBytes(data []byte, sourcePath string) (*domain.WorkflowFile, error) {
	var raw workflowYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("workflow: parse YAML: %w", err)
	}

	// Validate required fields.
	if raw.Name == "" {
		return nil, ErrMissingName
	}
	if raw.Version == "" {
		return nil, ErrMissingVersion
	}
	if len(raw.Epics) == 0 {
		return nil, ErrMissingEpics
	}

	// Validate name matches directory name.
	dirName := filepath.Base(sourcePath)
	if raw.Name != dirName {
		return nil, fmt.Errorf("%w: %q != %q", ErrNameMismatch, raw.Name, dirName)
	}

	// Convert to domain types.
	wf := &domain.WorkflowFile{
		Name:        raw.Name,
		Version:     raw.Version,
		Description: raw.Description,
		Settings:    convertSettings(raw.Settings),
		SourcePath:  filepath.Join(sourcePath, "WORKFLOW.yaml"),
	}

	// Convert epics.
	epics, err := convertEpics(raw.Epics)
	if err != nil {
		return nil, err
	}
	wf.Epics = epics

	return wf, nil
}

// convertSettings converts settingsYAML to domain.WorkflowSettings.
func convertSettings(raw *settingsYAML) domain.WorkflowSettings {
	settings := domain.DefaultWorkflowSettings()

	if raw == nil {
		return settings
	}

	if raw.DefaultTimeout > 0 {
		settings.DefaultTimeout = raw.DefaultTimeout
	}

	if raw.Guardrails != nil {
		settings.Guardrails = convertGuardrails(raw.Guardrails)
	}

	return settings
}

// convertGuardrails converts guardrailsYAML to domain.GuardrailSettings.
func convertGuardrails(raw *guardrailsYAML) domain.GuardrailSettings {
	defaults := domain.DefaultGuardrailSettings()

	if raw.Verification != nil {
		defaults.Verification = *raw.Verification
	}
	if raw.Confirmation != nil {
		defaults.Confirmation = *raw.Confirmation
	}
	if raw.Retrospective != nil {
		defaults.Retrospective = *raw.Retrospective
	}

	return defaults
}

// convertEpics converts a slice of epicYAML to domain.EpicDefinition.
func convertEpics(raws []epicYAML) ([]domain.EpicDefinition, error) {
	epics := make([]domain.EpicDefinition, 0, len(raws))

	for i, raw := range raws {
		if raw.ID == "" {
			return nil, fmt.Errorf("%w: epic[%d] missing id", ErrInvalidEpic, i)
		}
		if raw.Name == "" {
			return nil, fmt.Errorf("%w: epic[%d] missing name", ErrInvalidEpic, i)
		}

		stories, err := convertStories(raw.Stories, i)
		if err != nil {
			return nil, err
		}

		epic := domain.EpicDefinition{
			ID:            raw.ID,
			Name:          raw.Name,
			Description:   raw.Description,
			DependsOn:     raw.DependsOn,
			Retrospective: raw.Retrospective,
			Stories:       stories,
		}
		epics = append(epics, epic)
	}

	return epics, nil
}

// convertStories converts a slice of storyYAML to domain.StoryDefinition.
func convertStories(raws []storyYAML, epicIdx int) ([]domain.StoryDefinition, error) {
	stories := make([]domain.StoryDefinition, 0, len(raws))

	for i, raw := range raws {
		if raw.ID == "" {
			return nil, fmt.Errorf("%w: epic[%d].story[%d] missing id", ErrInvalidStory, epicIdx, i)
		}
		if raw.Name == "" {
			return nil, fmt.Errorf("%w: epic[%d].story[%d] missing name", ErrInvalidStory, epicIdx, i)
		}

		story := domain.StoryDefinition{
			ID:        raw.ID,
			Name:      raw.Name,
			Type:      parseTaskType(raw.Type),
			Skill:     raw.Skill,
			DependsOn: raw.DependsOn,
		}

		// Convert exit condition.
		if raw.ExitOn != nil {
			// Validate: exactly one of command or commands must be set (if either is set).
			hasCommand := raw.ExitOn.Command != ""
			hasCommands := len(raw.ExitOn.Commands) > 0
			if hasCommand && hasCommands {
				return nil, fmt.Errorf("%w: epic[%d].story[%d] has both command and commands", ErrInvalidExitOn, epicIdx, i)
			}

			story.ExitOn = &domain.ExitCondition{
				Command:     raw.ExitOn.Command,
				Commands:    raw.ExitOn.Commands,
				Timeout:     raw.ExitOn.Timeout,
				OutputValid: raw.ExitOn.OutputValid,
			}
		}

		// Convert routes.
		if len(raw.Routes) > 0 {
			story.Routes = make(map[domain.TaskStatus]string)
			for statusStr, nextStory := range raw.Routes {
				status := parseTaskStatus(statusStr)
				story.Routes[status] = nextStory
			}
		}

		// Convert condition.
		if raw.Condition != "" {
			status := parseTaskStatus(raw.Condition)
			story.Condition = &status
		}

		// Convert guardrails.
		if raw.Guardrails != nil {
			gr := convertGuardrails(raw.Guardrails)
			story.Guardrails = &gr
		}

		// Convert output config.
		if raw.Output != nil {
			story.Output = &domain.OutputConfig{
				Schema:  raw.Output.Schema,
				Handler: raw.Output.Handler,
			}
		}

		stories = append(stories, story)
	}

	return stories, nil
}

// parseTaskType converts a string to domain.TaskType.
func parseTaskType(s string) domain.TaskType {
	switch s {
	case "coding":
		return domain.TaskTypeCoding
	case "verification":
		return domain.TaskTypeVerification
	case "confirmation":
		return domain.TaskTypeConfirmation
	case "coordination":
		return domain.TaskTypeCoordination
	case "retrospective":
		return domain.TaskTypeRetrospective
	default:
		return domain.TaskTypeCoding // Default to coding
	}
}

// parseTaskStatus converts a string to domain.TaskStatus.
func parseTaskStatus(s string) domain.TaskStatus {
	switch s {
	case "pending":
		return domain.TaskStatusPending
	case "processing":
		return domain.TaskStatusProcessing
	case "awaiting":
		return domain.TaskStatusAwaiting
	case "success":
		return domain.TaskStatusSuccess
	case "failure":
		return domain.TaskStatusFailure
	case "completed":
		return domain.TaskStatusCompleted
	case "reverted":
		return domain.TaskStatusReverted
	case "cancelled":
		return domain.TaskStatusCancelled
	default:
		return domain.TaskStatusPending
	}
}
