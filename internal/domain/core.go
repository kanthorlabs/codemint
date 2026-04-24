// Package domain defines the core Go domain entities, enums, and structs
// for the CodeMint persistence layer.
package domain

import (
	"encoding/json"

	"codemint.kanthorlabs.com/internal/util/idgen"
)

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
	ID                 string          `db:"id"`
	ProjectID          string          `db:"project_id"`
	AllowedCommands    json.RawMessage `db:"allowed_commands"`
	AllowedDirectories json.RawMessage `db:"allowed_directories"`
	BlockedCommands    json.RawMessage `db:"blocked_commands"`
}

// Agent represents an actor that can be assigned tasks.
type Agent struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	Type      AgentType `db:"type"`
	Assistant string    `db:"assistant"`
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

// Task is the atomic unit of work assigned to an agent.
// Input and Output are stored as JSON text in SQLite; callers may
// json.Unmarshal them into concrete types as needed.
type Task struct {
	ID         string     `db:"id"`
	ProjectID  string     `db:"project_id"`
	SessionID  string     `db:"session_id"`
	WorkflowID string     `db:"workflow_id"`
	AssigneeID string     `db:"assignee_id"`
	SeqEpic    int        `db:"seq_epic"`
	SeqStory   int        `db:"seq_story"`
	SeqTask    int        `db:"seq_task"`
	Type       TaskType   `db:"type"`
	Status     TaskStatus `db:"status"`
	Input      string     `db:"input"`
	Output     string     `db:"output"`
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
		Assistant: assistant,
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

// NewTask creates a new Task with a generated UUID v7 primary key, defaulting to Pending status.
func NewTask(projectID, sessionID, workflowID, assigneeID string, taskType TaskType) *Task {
	return &Task{
		ID:         idgen.MustNew(),
		ProjectID:  projectID,
		SessionID:  sessionID,
		WorkflowID: workflowID,
		AssigneeID: assigneeID,
		Type:       taskType,
		Status:     TaskStatusPending,
	}
}
