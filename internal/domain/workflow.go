// Package domain defines the core Go domain entities, enums, and structs
// for the CodeMint persistence layer.
package domain

// WorkflowType represents the category of a workflow.
// These values must match the database workflow.type column.
type WorkflowType int

const (
	WorkflowTypeProjectCoding WorkflowType = iota // 0 - Context-aware coding tasks within a project.
	WorkflowTypeCommunication                     // 1 - General inquiries and explanations.
	WorkflowTypeDailyChecking                     // 2 - Status checks and routine operations.
)

// String returns the human-readable name for the workflow type.
func (w WorkflowType) String() string {
	switch w {
	case WorkflowTypeProjectCoding:
		return "ProjectCoding"
	case WorkflowTypeCommunication:
		return "Communication"
	case WorkflowTypeDailyChecking:
		return "DailyChecking"
	default:
		return "Unknown"
	}
}

// Valid returns true if the workflow type is within the valid range.
func (w WorkflowType) Valid() bool {
	return w >= WorkflowTypeProjectCoding && w <= WorkflowTypeDailyChecking
}

// WorkflowDefinition describes a workflow that can be registered in the system.
// It provides metadata for routing user requests to the appropriate handler.
type WorkflowDefinition struct {
	Type        WorkflowType // Unique identifier for the workflow type.
	Name        string       // Human-readable name (e.g., "Project Coding").
	Description string       // One-line description of the workflow purpose.
	Triggers    []string     // Keywords that route requests to this workflow.
}
