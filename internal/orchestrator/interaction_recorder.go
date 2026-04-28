package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"

	"codemint.kanthorlabs.com/internal/domain"
	"codemint.kanthorlabs.com/internal/repository"
	"codemint.kanthorlabs.com/internal/util/idgen"
)

// InteractionInputPayload is the JSON structure stored in task.input for Coordination tasks.
type InteractionInputPayload struct {
	Command string `json:"command"`            // The raw user input
	IsSlash bool   `json:"is_slash"`           // Whether this was a slash command
	CmdName string `json:"cmd_name,omitempty"` // The command name if slash command
	Text    string `json:"text,omitempty"`     // For chat: the user's prompt
	Source  string `json:"source,omitempty"`   // Input source: "tui", "cui-telegram", etc.
	UserID  string `json:"user_id,omitempty"`  // Source-specific user identifier
}

// InteractionOutputPayload is the JSON structure stored in task.output for Coordination tasks.
type InteractionOutputPayload struct {
	Message string `json:"message,omitempty"` // The response message
	Text    string `json:"text,omitempty"`    // For chat: the assistant's reply
	Error   string `json:"error,omitempty"`   // Error message if any
}

// InteractionRecorder records user interactions as Coordination tasks.
// It follows the "Human-as-an-Agent" pattern where every user command
// is persisted as a completed Coordination task.
type InteractionRecorder struct {
	taskRepo  repository.TaskRepository
	agentRepo repository.AgentRepository
}

// NewInteractionRecorder creates a new InteractionRecorder.
func NewInteractionRecorder(taskRepo repository.TaskRepository, agentRepo repository.AgentRepository) *InteractionRecorder {
	return &InteractionRecorder{
		taskRepo:  taskRepo,
		agentRepo: agentRepo,
	}
}

// Record creates a Coordination task to record a user interaction.
// This is called after command dispatch to capture what happened.
// Deprecated: Use RecordWithSource for source-aware recording.
func (r *InteractionRecorder) Record(ctx context.Context, active *ActiveSession, input string, isSlash bool, cmdName string, response string, err error) error {
	return r.RecordWithSource(ctx, active, input, isSlash, cmdName, response, "", "", err)
}

// RecordWithSource creates a Coordination task with input source metadata.
// source and userID are recorded for audit trails; they may be empty for
// legacy callers using the non-multiplexed Loop.
func (r *InteractionRecorder) RecordWithSource(ctx context.Context, active *ActiveSession, input string, isSlash bool, cmdName string, response string, source string, userID string, err error) error {
	// Only record if we have an active project session.
	// CodeMint sessions now persist as Coordination tasks under the CodeMint project.
	session := active.GetSession()
	project := active.GetProject()
	if session == nil || project == nil {
		return nil
	}

	// Get human agent ID.
	humanAgent, agentErr := r.agentRepo.FindByName(ctx, "human")
	if agentErr != nil || humanAgent == nil {
		// Can't record without human agent - silently skip.
		return nil
	}

	// Build input payload.
	inputPayload := InteractionInputPayload{
		Command: input,
		IsSlash: isSlash,
		CmdName: cmdName,
		Source:  source,
		UserID:  userID,
	}
	inputJSON, jsonErr := json.Marshal(inputPayload)
	if jsonErr != nil {
		return nil // Don't fail the main operation for recording errors.
	}

	// Build output payload.
	outputPayload := InteractionOutputPayload{
		Message: response,
	}
	if err != nil {
		outputPayload.Error = err.Error()
	}
	outputJSON, jsonErr := json.Marshal(outputPayload)
	if jsonErr != nil {
		return nil
	}

	// Create the coordination task.
	task := &domain.Task{
		ID:         idgen.MustNew(),
		ProjectID:  project.ID,
		SessionID:  session.ID,
		WorkflowID: sql.NullString{}, // No workflow for coordination tasks
		AssigneeID: humanAgent.ID,
		SeqEpic:    0,
		SeqStory:   0,
		SeqTask:    0,
		Type:       domain.TaskTypeCoordination,
		Status:     domain.TaskStatusCompleted, // Immediately completed
		Timeout:    domain.DefaultTaskTimeout,
		Input:      sql.NullString{String: string(inputJSON), Valid: true},
		Output:     sql.NullString{String: string(outputJSON), Valid: true},
		ClientID:   sql.NullString{String: active.ClientID, Valid: active.ClientID != ""},
	}

	// Save to database.
	if createErr := r.taskRepo.Create(ctx, task); createErr != nil {
		// Don't fail the main operation for recording errors.
		return nil
	}

	// Update last seen task ID for the active session.
	active.LastSeenTaskID = task.ID

	return nil
}

// RecordChat creates a Coordination task to record a conversational exchange.
// This is called after the system assistant responds to capture the round-trip.
// source identifies the client mode (cli | daemon).
// Deprecated: Use RecordChatWithSource for source-aware recording.
func (r *InteractionRecorder) RecordChat(ctx context.Context, active *ActiveSession, userText string, assistantText string, source string, err error) error {
	return r.RecordChatWithSource(ctx, active, userText, assistantText, source, "", err)
}

// RecordChatWithSource creates a Coordination task with input source metadata.
// source and userID are recorded for audit trails; userID may be empty.
func (r *InteractionRecorder) RecordChatWithSource(ctx context.Context, active *ActiveSession, userText string, assistantText string, source string, userID string, err error) error {
	// For global sessions, we still want to record chat but use a placeholder.
	// The session/project may be nil, so we handle gracefully.
	
	// Get human agent ID for chat recording.
	humanAgent, agentErr := r.agentRepo.FindByName(ctx, "human")
	if agentErr != nil || humanAgent == nil {
		// Can't record without human agent - silently skip.
		return nil
	}

	// Build input payload for chat.
	inputPayload := InteractionInputPayload{
		Command: "/chat",
		IsSlash: false,
		Text:    userText,
		Source:  source,
		UserID:  userID,
	}
	inputJSON, jsonErr := json.Marshal(inputPayload)
	if jsonErr != nil {
		return nil
	}

	// Build output payload.
	outputPayload := InteractionOutputPayload{
		Text: assistantText,
	}
	if err != nil {
		outputPayload.Error = err.Error()
	}
	outputJSON, jsonErr := json.Marshal(outputPayload)
	if jsonErr != nil {
		return nil
	}

	// Determine status based on success/failure.
	status := domain.TaskStatusCompleted
	if err != nil {
		status = domain.TaskStatusFailure
	}

	// Create the coordination task.
	task := &domain.Task{
		ID:         idgen.MustNew(),
		AssigneeID: humanAgent.ID,
		SeqEpic:    0,
		SeqStory:   0,
		SeqTask:    0,
		Type:       domain.TaskTypeCoordination,
		Status:     status,
		Timeout:    domain.DefaultTaskTimeout,
		Input:      sql.NullString{String: string(inputJSON), Valid: true},
		Output:     sql.NullString{String: string(outputJSON), Valid: true},
		ClientID:   sql.NullString{String: active.ClientID, Valid: active.ClientID != ""},
	}

	// Set project/session if available.
	project := active.GetProject()
	session := active.GetSession()
	if project != nil {
		task.ProjectID = project.ID
	}
	if session != nil {
		task.SessionID = session.ID
	}

	// Save to database.
	if createErr := r.taskRepo.Create(ctx, task); createErr != nil {
		// Don't fail the main operation for recording errors.
		return nil
	}

	// Update last seen task ID for the active session.
	active.LastSeenTaskID = task.ID

	return nil
}
