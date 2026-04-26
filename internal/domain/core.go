// Package domain defines the core Go domain entities, enums, and structs
// for the CodeMint persistence layer.
package domain

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"

	"codemint.kanthorlabs.com/internal/util/idgen"
)

// NullableJSON is a json.RawMessage that supports NULL values from SQLite.
// It implements sql.Scanner and driver.Valuer for database operations.
type NullableJSON json.RawMessage

// Scan implements the sql.Scanner interface for NullableJSON.
func (n *NullableJSON) Scan(value any) error {
	if value == nil {
		*n = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*n = NullableJSON(v)
	case string:
		*n = NullableJSON(v)
	}
	return nil
}

// Value implements the driver.Valuer interface for NullableJSON.
func (n NullableJSON) Value() (driver.Value, error) {
	if n == nil {
		return nil, nil
	}
	return []byte(n), nil
}

// MarshalJSON implements json.Marshaler.
func (n NullableJSON) MarshalJSON() ([]byte, error) {
	if n == nil {
		return []byte("null"), nil
	}
	return json.RawMessage(n).MarshalJSON()
}

// UnmarshalJSON implements json.Unmarshaler.
func (n *NullableJSON) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*n = nil
		return nil
	}
	*n = NullableJSON(data)
	return nil
}

// TaskType represents the kind of work a task performs.
type TaskType int

const (
	TaskTypeCoding       TaskType = iota // 0
	TaskTypeVerification                 // 1
	TaskTypeConfirmation                 // 2
	TaskTypeCoordination                 // 3
)

// TaskStatus represents the lifecycle state of a task.
type TaskStatus int

const (
	TaskStatusPending    TaskStatus = iota // 0
	TaskStatusProcessing                   // 1
	TaskStatusAwaiting                     // 2
	TaskStatusSuccess                      // 3
	TaskStatusFailure                      // 4
	TaskStatusCompleted                    // 5
	TaskStatusReverted                     // 6
	TaskStatusCancelled                    // 7
)

// AgentType represents the category of an agent actor.
type AgentType int

const (
	AgentTypeHuman     AgentType = iota // 0
	AgentTypeAssistant                  // 1
	AgentTypeSystem                     // 2
)

// SessionStatus represents the lifecycle state of a session.
type SessionStatus int

const (
	SessionStatusActive   SessionStatus = iota // 0
	SessionStatusArchived                      // 1
)

// YoloMode represents the level of risk tolerance for a project.
type YoloMode int

const (
	YoloModeOff YoloMode = iota // 0 - Strict safety checks, no risky operations allowed.
	YoloModeOn  YoloMode = 1    // 1 - Permissive mode, allows potentially risky operations with warnings.
)

// Project is the top-level context entity representing a codebase workspace.
type Project struct {
	ID         string `db:"id"`
	Name       string `db:"name"`
	WorkingDir string `db:"working_dir"`
	YoloMode   int    `db:"yolo_mode"`
}

// ProjectPermission defines command-level access controls for a project.
type ProjectPermission struct {
	ID                 string       `db:"id"`
	ProjectID          string       `db:"project_id"`
	AllowedCommands    NullableJSON `db:"allowed_commands"`
	AllowedDirectories NullableJSON `db:"allowed_directories"`
	BlockedCommands    NullableJSON `db:"blocked_commands"`
}

// Agent represents an actor that can be assigned tasks.
type Agent struct {
	ID        string         `db:"id"`
	Name      string         `db:"name"`
	Type      AgentType      `db:"type"`
	Assistant sql.NullString `db:"assistant"`
}

// Session is a single execution instance tied to a project.
type Session struct {
	ID        string        `db:"id"`
	ProjectID string        `db:"project_id"`
	Status    SessionStatus `db:"status"`
}

// Workflow groups a set of related tasks within a session.
type Workflow struct {
	ID        string `db:"id"`
	SessionID string `db:"session_id"`
	Type      int    `db:"type"`
}

// DefaultTaskTimeout is the default timeout for a task in milliseconds (1 hour).
// It ensures that no agent process can hang indefinitely.
const DefaultTaskTimeout int64 = 3_600_000

// Task is the atomic unit of work assigned to an agent.
// Input and Output are stored as JSON text in SQLite; callers may
// json.Unmarshal them into concrete types as needed.
type Task struct {
	ID         string         `db:"id"`
	ProjectID  string         `db:"project_id"`
	SessionID  string         `db:"session_id"`
	WorkflowID sql.NullString `db:"workflow_id"`
	AssigneeID string         `db:"assignee_id"`
	SeqEpic    int            `db:"seq_epic"`
	SeqStory   int            `db:"seq_story"`
	SeqTask    int            `db:"seq_task"`
	Type       TaskType       `db:"type"`
	Status     TaskStatus     `db:"status"`
	// Timeout is the maximum duration in milliseconds the agent is allowed to
	// run before the process is killed and the task is transitioned to Failure.
	// Defaults to DefaultTaskTimeout (1 hour) when not explicitly set.
	Timeout int64          `db:"timeout"`
	Input   sql.NullString `db:"input"`
	Output  sql.NullString `db:"output"`
}

// --- Null Helpers ---

// NewNullString creates a sql.NullString from a string.
// If the string is empty, Valid is false (representing NULL).
func NewNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// --- Factory Constructors ---

// NewProject creates a new Project with a generated UUID v7 primary key.
func NewProject(name, workingDir string) *Project {
	return &Project{
		ID:         idgen.MustNew(),
		Name:       name,
		WorkingDir: workingDir,
		YoloMode:   int(YoloModeOff),
	}
}

// NewProjectPermission creates a new ProjectPermission linked to a project.
func NewProjectPermission(projectID string) *ProjectPermission {
	return &ProjectPermission{
		ID:        idgen.MustNew(),
		ProjectID: projectID,
	}
}

// NewAgent creates a new Agent with a generated UUID v7 primary key.
func NewAgent(name string, agentType AgentType, assistant string) *Agent {
	return &Agent{
		ID:        idgen.MustNew(),
		Name:      name,
		Type:      agentType,
		Assistant: sql.NullString{String: assistant, Valid: assistant != ""},
	}
}

// NewSession creates a new Session linked to a project, defaulting to Active status.
func NewSession(projectID string) *Session {
	return &Session{
		ID:        idgen.MustNew(),
		ProjectID: projectID,
		Status:    SessionStatusActive,
	}
}

// NewWorkflow creates a new Workflow linked to a session.
func NewWorkflow(sessionID string, workflowType int) *Workflow {
	return &Workflow{
		ID:        idgen.MustNew(),
		SessionID: sessionID,
		Type:      workflowType,
	}
}

// NewTask creates a new Task with a generated UUID v7 primary key, defaulting
// to Pending status and DefaultTaskTimeout (1 hour) to ensure no process hangs
// indefinitely.
func NewTask(projectID, sessionID, workflowID, assigneeID string, taskType TaskType) *Task {
	return &Task{
		ID:         idgen.MustNew(),
		ProjectID:  projectID,
		SessionID:  sessionID,
		WorkflowID: sql.NullString{String: workflowID, Valid: workflowID != ""},
		AssigneeID: assigneeID,
		Type:       taskType,
		Status:     TaskStatusPending,
		Timeout:    DefaultTaskTimeout,
	}
}
