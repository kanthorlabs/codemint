// Package domain defines the core Go domain entities, enums, and structs
// for the CodeMint persistence layer.
package domain

import (
	"encoding/json"
	"errors"
)

// Sentinel errors for TaskInput parsing.
var (
	// ErrEmptyInput is returned when the input string is empty.
	ErrEmptyInput = errors.New("task input: empty input")

	// ErrLegacyText is returned when the input is plain text (not JSON).
	// The caller may decide whether to log a warning and continue.
	ErrLegacyText = errors.New("task input: legacy plain text")
)

// TaskInput represents the structured JSON schema for task input as defined in
// EPIC-02 §2.13. It provides a typed contract between the Task Generator and
// the ACP Executor.
type TaskInput struct {
	// Prompt is the main instruction for the agent. Required.
	Prompt string `json:"prompt"`

	// ContextFiles lists relative file paths to include as context.
	// Paths are resolved relative to Project.WorkingDir.
	ContextFiles []string `json:"context_files,omitempty"`

	// Tools hints which tools the agent should consider using.
	Tools []string `json:"tools,omitempty"`

	// Command is used for verification tasks. Optional.
	Command string `json:"command,omitempty"`

	// Cwd specifies the working directory for command execution. Optional.
	Cwd string `json:"cwd,omitempty"`

	// Metadata holds arbitrary key-value pairs for extensibility.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ParseTaskInput parses a raw string into a TaskInput struct.
//
// Behavior:
//   - Empty string: returns nil, ErrEmptyInput
//   - Valid JSON object: returns parsed TaskInput, nil
//   - Non-JSON text (legacy): returns TaskInput{Prompt: raw}, ErrLegacyText
//
// The legacy fallback ensures backward compatibility with pre-EPIC-02.13
// sessions that passed plain-text prompts. Callers should log a warning
// when ErrLegacyText is returned but may continue execution.
func ParseTaskInput(raw string) (*TaskInput, error) {
	if raw == "" {
		return nil, ErrEmptyInput
	}

	// Attempt JSON parse.
	var input TaskInput
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		// Not valid JSON - treat as legacy plain-text prompt.
		return &TaskInput{Prompt: raw}, ErrLegacyText
	}

	// Heuristic: if the JSON parsed but has no prompt field AND the raw
	// string doesn't look like a JSON object, it might be legacy text.
	// However, json.Unmarshal on a plain string fails, so if we reach here
	// we have valid JSON. The only edge case is an empty object {}.
	// We handle that in validation, not here.

	return &input, nil
}

// Validate checks that required fields are present.
// Returns an error if validation fails.
func (t *TaskInput) Validate() error {
	if t.Prompt == "" {
		return errors.New("task input: missing required field 'prompt'")
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (t TaskInput) MarshalJSON() ([]byte, error) {
	type Alias TaskInput
	return json.Marshal((Alias)(t))
}
